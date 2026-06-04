// Command sandbox_m4_decision combines sandbox baseline, quality-comparison,
// and human-review evidence into a conservative M4 proceed/iterate/defer
// recommendation. It does not run Docker, an LLM, or an agent runtime.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	maxInputBytes                   int64 = 1024 * 1024
	defaultMinCases                       = 3
	decisionProceed                       = "proceed"
	decisionIterate                       = "iterate"
	decisionDefer                         = "defer"
	requiredSnapshotRefPrefix             = "snapshot:"
	maxReviewEvidenceTextBytes            = 2048
	maxEvidenceCaseIDBytes                = 128
	maxEvidenceRefRunes                   = 120
	maxRuntimeSmokeEvidenceRefBytes       = 256
	loopbackRuntimeCandidateReason        = "review evidence runtime_candidate uses a loopback registry reference; publishable runtime baselines must use a non-loopback registry reference"
)

var requiredBaselineChecks = []string{
	"fixed_file_contract",
	"batch_network_none_spec",
	"m5_turn_input_mounts",
	"docker_security_posture",
	"allowlist_enforcer_subset",
	"allowlist_enforcer_drift_rejection",
	"raw_result_validation",
}

var requiredRuntimeSmokes = []string{
	"candidate_runtime_file_contract",
	"container_provider_lifecycle",
	"container_provider_timeout_cleanup",
	"container_provider_output_cap",
	"egress_allowdeny",
}

var requiredRuntimeSmokeSources = map[string]string{
	"candidate_runtime_file_contract":    "make agent-runtime-smoke",
	"container_provider_lifecycle":       "make container-provider-smoke",
	"container_provider_timeout_cleanup": "make container-provider-timeout-smoke",
	"container_provider_output_cap":      "make container-provider-output-cap-smoke",
	"egress_allowdeny":                   "make egress-allowdeny-smoke",
}

var requiredQualityScenarios = []reportprompt.Scenario{
	reportprompt.ScenarioSingleAlert,
	reportprompt.ScenarioCascade,
	reportprompt.ScenarioAlertStorm,
}

var imageDigestHexRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

var todayUTC = func() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

type config struct {
	BaselineAuditPath     string
	QualityComparisonPath string
	ReviewEvidencePath    string
	MinCases              int
	FailUnless            string
}

type statusCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type baselineAudit struct {
	Tool   string        `json:"tool"`
	Status string        `json:"status"`
	Checks []statusCheck `json:"checks"`
}

type qualityComparison struct {
	Tool             string         `json:"tool"`
	Schema           string         `json:"schema"`
	Mode             string         `json:"mode"`
	CaseCount        int            `json:"case_count"`
	SampleBasis      string         `json:"sample_basis"`
	ScenarioCoverage []string       `json:"scenario_coverage"`
	Summary          qualitySummary `json:"summary"`
	Recommendation   string         `json:"recommendation"`
	ReviewRequired   bool           `json:"review_required"`
	Cases            []qualityCase  `json:"cases"`
}

type qualitySummary struct {
	ImprovedCount         int `json:"improved_count"`
	EquivalentCount       int `json:"equivalent_count"`
	RegressedCount        int `json:"regressed_count"`
	NeedsHumanReviewCount int `json:"needs_human_review_count"`
}

type qualityCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	Recommendation       string   `json:"recommendation"`
	ReviewRequired       bool     `json:"review_required"`
}

type reviewEvidence struct {
	Tool                 string                `json:"tool"`
	EvidenceDate         string                `json:"evidence_date"`
	SelectedCandidate    string                `json:"selected_candidate"`
	RuntimeCandidate     string                `json:"runtime_candidate"`
	RepresentativeSample bool                  `json:"representative_sample"`
	SampleBasis          string                `json:"sample_basis"`
	CandidateEvaluations []candidateEvaluation `json:"candidate_evaluations"`
	RuntimeSmokes        []runtimeSmoke        `json:"runtime_smokes"`
	ReviewedCases        []reviewedCase        `json:"reviewed_cases"`
	HumanReview          humanReview           `json:"human_review"`
}

type candidateEvaluation struct {
	Candidate        string   `json:"candidate"`
	Status           string   `json:"status"`
	RuntimeCandidate string   `json:"runtime_candidate,omitempty"`
	RuntimeSmokeRefs []string `json:"runtime_smoke_refs,omitempty"`
	Source           string   `json:"source"`
	Notes            string   `json:"notes"`
}

type reviewedCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	Status               string   `json:"status"`
	Notes                string   `json:"notes"`
}

type runtimeSmoke struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	EvidenceRef    string `json:"evidence_ref"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type humanReview struct {
	Status   string `json:"status"`
	Reviewer string `json:"reviewer"`
	Notes    string `json:"notes,omitempty"`
}

type decisionOutput struct {
	Tool           string          `json:"tool"`
	Decision       string          `json:"decision"`
	ReviewRequired bool            `json:"review_required"`
	Evidence       evidenceSummary `json:"evidence"`
	Reasons        []string        `json:"reasons"`
}

type evidenceSummary struct {
	BaselineAuditStatus      string   `json:"baseline_audit_status"`
	QualityRecommendation    string   `json:"quality_recommendation"`
	CaseCount                int      `json:"case_count"`
	MinimumCaseCount         int      `json:"minimum_case_count"`
	SampleBasis              string   `json:"sample_basis"`
	ScenarioCoverage         []string `json:"scenario_coverage"`
	SelectedCandidate        string   `json:"selected_candidate"`
	RuntimeCandidate         string   `json:"runtime_candidate"`
	CandidateEvaluationCount int      `json:"candidate_evaluation_count"`
	RuntimeSmokePassedCount  int      `json:"runtime_smoke_passed_count"`
	ReviewedCaseCount        int      `json:"reviewed_case_count"`
	RepresentativeSample     bool     `json:"representative_sample"`
	HumanReviewStatus        string   `json:"human_review_status"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-m4-decision] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	out, err := evaluate(cfg)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if cfg.FailUnless != "" && out.Decision != cfg.FailUnless {
		return fmt.Errorf("M4 decision = %q, want %q", out.Decision, cfg.FailUnless)
	}
	return nil
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("sandbox_m4_decision", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baselineAuditPath := fs.String("baseline-audit", "", "sandbox_baseline_audit JSON output")
	qualityComparisonPath := fs.String("quality-comparison", "", "sandbox_quality_compare --manifest JSON output")
	reviewEvidencePath := fs.String("review-evidence", "", "M4 human review and runtime smoke evidence JSON")
	minCases := fs.Int("min-cases", defaultMinCases, "minimum representative quality cases required")
	failUnless := fs.String("fail-unless", "", "exit non-zero unless decision equals proceed, iterate, or defer")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		BaselineAuditPath:     *baselineAuditPath,
		QualityComparisonPath: *qualityComparisonPath,
		ReviewEvidencePath:    *reviewEvidencePath,
		MinCases:              *minCases,
		FailUnless:            strings.TrimSpace(*failUnless),
	}
	if cfg.BaselineAuditPath == "" {
		return config{}, errors.New("--baseline-audit is required")
	}
	if cfg.QualityComparisonPath == "" {
		return config{}, errors.New("--quality-comparison is required")
	}
	if cfg.ReviewEvidencePath == "" {
		return config{}, errors.New("--review-evidence is required")
	}
	if cfg.MinCases <= 0 {
		return config{}, errors.New("--min-cases must be greater than zero")
	}
	if cfg.FailUnless != "" && !validDecision(cfg.FailUnless) {
		return config{}, fmt.Errorf("--fail-unless must be one of %q, %q, %q", decisionProceed, decisionIterate, decisionDefer)
	}
	return cfg, nil
}

func evaluate(cfg config) (decisionOutput, error) {
	var baseline baselineAudit
	if err := readJSONFile(cfg.BaselineAuditPath, &baseline); err != nil {
		return decisionOutput{}, fmt.Errorf("baseline audit: %w", err)
	}
	var quality qualityComparison
	if err := readJSONFile(cfg.QualityComparisonPath, &quality); err != nil {
		return decisionOutput{}, fmt.Errorf("quality comparison: %w", err)
	}
	var review reviewEvidence
	if err := readJSONFile(cfg.ReviewEvidencePath, &review); err != nil {
		return decisionOutput{}, fmt.Errorf("review evidence: %w", err)
	}

	var deferReasons []string
	var iterateReasons []string
	issues, err := validateBaselineAudit(baseline)
	if err != nil {
		return decisionOutput{}, err
	}
	deferReasons = append(deferReasons, issues...)
	issues, err = validateQualityComparison(quality, cfg.MinCases)
	if err != nil {
		return decisionOutput{}, err
	}
	deferReasons = append(deferReasons, issues...)
	reviewDeferReasons, reviewIterateReasons, err := validateReviewEvidence(review)
	if err != nil {
		return decisionOutput{}, err
	}
	deferReasons = append(deferReasons, reviewDeferReasons...)
	iterateReasons = append(iterateReasons, reviewIterateReasons...)
	if quality.SampleBasis != review.SampleBasis {
		deferReasons = append(deferReasons, fmt.Sprintf("review evidence sample_basis %q must match quality comparison sample_basis %q", review.SampleBasis, quality.SampleBasis))
	}
	reviewCoverageDefer, reviewCoverageIterate := validateReviewCoverage(quality.Cases, review.ReviewedCases)
	deferReasons = append(deferReasons, reviewCoverageDefer...)
	iterateReasons = append(iterateReasons, reviewCoverageIterate...)

	if quality.Summary.RegressedCount > 0 {
		iterateReasons = append(iterateReasons, fmt.Sprintf("quality comparison has %d regressed case(s)", quality.Summary.RegressedCount))
	}
	if quality.Summary.ImprovedCount == 0 && quality.Summary.NeedsHumanReviewCount == 0 {
		deferReasons = append(deferReasons, "quality comparison has no improved or human-reviewed candidate case")
	}

	decision := decisionProceed
	reasons := []string{"all required M4 evidence is present and no regression was detected"}
	switch {
	case len(deferReasons) > 0:
		decision = decisionDefer
		reasons = append([]string{}, deferReasons...)
		reasons = append(reasons, iterateReasons...)
	case len(iterateReasons) > 0:
		decision = decisionIterate
		reasons = iterateReasons
	}

	return decisionOutput{
		Tool:           "sandbox_m4_decision",
		Decision:       decision,
		ReviewRequired: true,
		Evidence: evidenceSummary{
			BaselineAuditStatus:      baseline.Status,
			QualityRecommendation:    quality.Recommendation,
			CaseCount:                quality.CaseCount,
			MinimumCaseCount:         cfg.MinCases,
			SampleBasis:              quality.SampleBasis,
			ScenarioCoverage:         quality.ScenarioCoverage,
			SelectedCandidate:        review.SelectedCandidate,
			RuntimeCandidate:         review.RuntimeCandidate,
			CandidateEvaluationCount: len(review.CandidateEvaluations),
			RuntimeSmokePassedCount:  countPassedRuntimeSmokes(review.RuntimeSmokes),
			ReviewedCaseCount:        len(review.ReviewedCases),
			RepresentativeSample:     review.RepresentativeSample,
			HumanReviewStatus:        review.HumanReview.Status,
		},
		Reasons: reasons,
	}, nil
}

func validateBaselineAudit(audit baselineAudit) ([]string, error) {
	if audit.Tool != "sandbox_baseline_audit" {
		return nil, fmt.Errorf("baseline audit tool = %q, want sandbox_baseline_audit", audit.Tool)
	}
	var issues []string
	if audit.Status != "pass" {
		issues = append(issues, fmt.Sprintf("baseline audit status = %q, want pass", audit.Status))
	}
	checks, err := mapStatusChecks(audit.Checks)
	if err != nil {
		return nil, err
	}
	for _, name := range requiredBaselineChecks {
		status, ok := checks[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("baseline audit missing check %q", name))
			continue
		}
		if status != "pass" {
			issues = append(issues, fmt.Sprintf("baseline audit check %q status = %q, want pass", name, status))
		}
	}
	return issues, nil
}

func validateQualityComparison(quality qualityComparison, minCases int) ([]string, error) {
	if quality.Tool != "sandbox_quality_compare" {
		return nil, fmt.Errorf("quality comparison tool = %q, want sandbox_quality_compare", quality.Tool)
	}
	if quality.Schema != reportdraft.SubReportSchemaID {
		return nil, fmt.Errorf("quality comparison schema = %q, want %q", quality.Schema, reportdraft.SubReportSchemaID)
	}
	if quality.Mode != "manifest" {
		return nil, fmt.Errorf("quality comparison mode = %q, want manifest", quality.Mode)
	}
	if !quality.ReviewRequired {
		return nil, errors.New("quality comparison review_required must be true")
	}
	if err := validateReviewEvidenceText("quality comparison sample_basis", quality.SampleBasis); err != nil {
		return nil, err
	}
	if quality.CaseCount <= 0 {
		return nil, errors.New("quality comparison case_count must be greater than zero")
	}
	if len(quality.Cases) != quality.CaseCount {
		return nil, fmt.Errorf("quality comparison has %d cases, want case_count %d", len(quality.Cases), quality.CaseCount)
	}
	if err := validateQualityCases(quality.Cases); err != nil {
		return nil, err
	}
	if err := validateQualitySummary(quality.Summary, quality.Cases, quality.CaseCount); err != nil {
		return nil, err
	}
	if expected := batchRecommendation(quality.Summary); quality.Recommendation != expected {
		return nil, fmt.Errorf("quality comparison recommendation = %q, want %q", quality.Recommendation, expected)
	}
	derivedCoverage := qualityScenarioCoverage(quality.Cases)
	if err := validateScenarioCoverageField(quality.ScenarioCoverage, derivedCoverage); err != nil {
		return nil, err
	}
	var issues []string
	if quality.CaseCount < minCases {
		issues = append(issues, fmt.Sprintf("quality comparison case_count %d is below required minimum %d", quality.CaseCount, minCases))
	}
	for _, scenario := range requiredQualityScenarios {
		if !containsString(derivedCoverage, string(scenario)) {
			issues = append(issues, fmt.Sprintf("quality comparison missing required scenario %q", scenario))
		}
	}
	return issues, nil
}

func validateQualityCases(cases []qualityCase) error {
	seen := map[string]bool{}
	for i, item := range cases {
		id, err := validateEvidenceCaseID(fmt.Sprintf("quality comparison cases[%d].id", i), item.ID)
		if err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("duplicate quality comparison case id %q", id)
		}
		seen[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return fmt.Errorf("quality comparison case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return fmt.Errorf("quality comparison case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return fmt.Errorf("quality comparison case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateQualityRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return err
		}
		if !item.ReviewRequired {
			return fmt.Errorf("quality comparison case %q review_required must be true", id)
		}
		if !validCaseRecommendation(item.Recommendation) {
			return fmt.Errorf("quality comparison case %q has unsupported recommendation %q", id, item.Recommendation)
		}
	}
	return nil
}

func validateQualityRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("quality comparison case %q required_evidence_refs must contain at least one item", caseID)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	for i, ref := range refs {
		value, err := validateEvidenceRef(fmt.Sprintf("quality comparison case %q required_evidence_refs[%d]", caseID, i), ref)
		if err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("quality comparison case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validQualitySnapshotRef(value) {
				return fmt.Errorf("quality comparison case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
	}
	if !hasSnapshotRef {
		return fmt.Errorf("quality comparison case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return nil
}

func validQualitySnapshotRef(ref string) bool {
	rawID := strings.TrimPrefix(ref, requiredSnapshotRefPrefix)
	id, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == rawID
}

func validateQualitySummary(summary qualitySummary, cases []qualityCase, caseCount int) error {
	sum := summary.ImprovedCount + summary.EquivalentCount + summary.RegressedCount + summary.NeedsHumanReviewCount
	if sum != caseCount {
		return fmt.Errorf("quality comparison summary count = %d, want case_count %d", sum, caseCount)
	}
	want := summarizeQualityCases(cases)
	if summary != want {
		return fmt.Errorf("quality comparison summary = %+v, want case-derived summary %+v", summary, want)
	}
	return nil
}

func summarizeQualityCases(cases []qualityCase) qualitySummary {
	var out qualitySummary
	for _, item := range cases {
		switch item.Recommendation {
		case "sandbox_candidate_improved":
			out.ImprovedCount++
		case "equivalent_metrics":
			out.EquivalentCount++
		case "sandbox_candidate_regressed":
			out.RegressedCount++
		case "needs_human_review":
			out.NeedsHumanReviewCount++
		}
	}
	return out
}

func qualityScenarioCoverage(cases []qualityCase) []string {
	seen := map[string]bool{}
	for _, item := range cases {
		seen[item.Scenario] = true
	}
	out := make([]string, 0, len(requiredQualityScenarios))
	for _, scenario := range requiredQualityScenarios {
		value := string(scenario)
		if seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func validateScenarioCoverageField(got, want []string) error {
	if len(got) != len(want) {
		return fmt.Errorf("quality comparison scenario_coverage = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("quality comparison scenario_coverage = %v, want %v", got, want)
		}
	}
	return nil
}

func validateReviewEvidence(review reviewEvidence) ([]string, []string, error) {
	if review.Tool != "sandbox_m4_review_evidence" {
		return nil, nil, fmt.Errorf("review evidence tool = %q, want sandbox_m4_review_evidence", review.Tool)
	}
	var deferReasons []string
	var iterateReasons []string
	if evidenceDate, err := time.Parse("2006-01-02", review.EvidenceDate); err != nil {
		deferReasons = append(deferReasons, fmt.Sprintf("review evidence date %q must be YYYY-MM-DD", review.EvidenceDate))
	} else if evidenceDate.After(todayUTC()) {
		deferReasons = append(deferReasons, fmt.Sprintf("review evidence date %q must not be in the future", review.EvidenceDate))
	}
	if strings.TrimSpace(review.RuntimeCandidate) == "" {
		deferReasons = append(deferReasons, "review evidence runtime_candidate is required")
	} else if strings.TrimSpace(review.RuntimeCandidate) != review.RuntimeCandidate {
		return nil, nil, errors.New("review evidence runtime_candidate must not contain leading or trailing whitespace")
	} else if !immutableImageReference(review.RuntimeCandidate) {
		return nil, nil, errors.New("review evidence runtime_candidate must be an immutable image reference `name@sha256:<64-hex-digest>`")
	} else if loopbackImageReference(review.RuntimeCandidate) {
		deferReasons = append(deferReasons, loopbackRuntimeCandidateReason)
	}
	candidateDeferReasons, candidateIterateReasons, err := validateCandidateEvaluations(review.SelectedCandidate, review.RuntimeCandidate, review.CandidateEvaluations)
	if err != nil {
		return nil, nil, err
	}
	deferReasons = append(deferReasons, candidateDeferReasons...)
	iterateReasons = append(iterateReasons, candidateIterateReasons...)
	if !review.RepresentativeSample {
		deferReasons = append(deferReasons, "review evidence representative_sample must be true")
	}
	if strings.TrimSpace(review.SampleBasis) == "" {
		deferReasons = append(deferReasons, "review evidence sample_basis is required")
	}
	if review.SampleBasis != "" {
		if err := validateReviewEvidenceText("review evidence sample_basis", review.SampleBasis); err != nil {
			return nil, nil, err
		}
	}
	smokeIssues, err := validateRuntimeSmokes(review.RuntimeSmokes)
	if err != nil {
		return nil, nil, err
	}
	iterateReasons = append(iterateReasons, smokeIssues...)
	reviewedCaseDeferReasons, err := validateReviewedCases(review.ReviewedCases)
	if err != nil {
		return nil, nil, err
	}
	deferReasons = append(deferReasons, reviewedCaseDeferReasons...)
	if strings.TrimSpace(review.HumanReview.Reviewer) == "" {
		deferReasons = append(deferReasons, "review evidence human_review.reviewer is required")
	}
	if review.HumanReview.Reviewer != "" {
		if err := validateReviewEvidenceText("review evidence human_review.reviewer", review.HumanReview.Reviewer); err != nil {
			return nil, nil, err
		}
	}
	if strings.TrimSpace(review.HumanReview.Notes) == "" {
		deferReasons = append(deferReasons, "review evidence human_review.notes is required")
	}
	if review.HumanReview.Notes != "" {
		if err := validateReviewEvidenceText("review evidence human_review.notes", review.HumanReview.Notes); err != nil {
			return nil, nil, err
		}
	}
	if !validReviewStatus(review.HumanReview.Status) {
		return nil, nil, fmt.Errorf("review evidence human_review.status = %q, want pass or fail", review.HumanReview.Status)
	}
	if review.HumanReview.Status != "pass" {
		iterateReasons = append(iterateReasons, fmt.Sprintf("human review status = %q, want pass", review.HumanReview.Status))
	}
	return deferReasons, iterateReasons, nil
}

func validateCandidateEvaluations(selected, runtimeCandidate string, evaluations []candidateEvaluation) ([]string, []string, error) {
	var deferReasons []string
	var iterateReasons []string
	selectedValue := strings.TrimSpace(selected)
	if selectedValue == "" {
		deferReasons = append(deferReasons, "review evidence selected_candidate is required")
	} else if selectedValue != selected {
		return nil, nil, errors.New("review evidence selected_candidate must not contain leading or trailing whitespace")
	} else if !validCandidateID(selected) {
		return nil, nil, fmt.Errorf("review evidence selected_candidate %q must be a non-whitespace candidate id up to %d bytes", selected, maxEvidenceCaseIDBytes)
	}
	selected = selectedValue

	if len(evaluations) == 0 {
		deferReasons = append(deferReasons, "review evidence candidate_evaluations must contain at least one candidate")
		return deferReasons, iterateReasons, nil
	}

	byCandidate := map[string]candidateEvaluation{}
	for i, item := range evaluations {
		candidate := strings.TrimSpace(item.Candidate)
		if candidate == "" {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations[%d].candidate is required", i)
		}
		if candidate != item.Candidate {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations candidate %q must not contain leading or trailing whitespace", item.Candidate)
		}
		if !validCandidateID(candidate) {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations[%d].candidate %q must be a non-whitespace candidate id up to %d bytes", i, candidate, maxEvidenceCaseIDBytes)
		}
		if _, ok := byCandidate[candidate]; ok {
			return nil, nil, fmt.Errorf("duplicate review evidence candidate_evaluations candidate %q", candidate)
		}
		status := strings.TrimSpace(item.Status)
		if status == "" {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations[%d].status is required", i)
		}
		if status != item.Status {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations candidate %q status must not contain leading or trailing whitespace", candidate)
		}
		if !validCandidateEvaluationStatus(status) {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations candidate %q status = %q, want pass, fail, or not_fit", candidate, item.Status)
		}
		itemRuntimeCandidate := strings.TrimSpace(item.RuntimeCandidate)
		if status == "pass" && itemRuntimeCandidate == "" {
			deferReasons = append(deferReasons, fmt.Sprintf("review evidence candidate_evaluations candidate %q runtime_candidate is required when status is pass", candidate))
		}
		if itemRuntimeCandidate != item.RuntimeCandidate {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_candidate must not contain leading or trailing whitespace", candidate)
		}
		if itemRuntimeCandidate != "" && !immutableImageReference(itemRuntimeCandidate) {
			return nil, nil, fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_candidate must be an immutable image reference `name@sha256:<64-hex-digest>`", candidate)
		}
		smokeRefDeferReasons, err := validateCandidateRuntimeSmokeRefs(candidate, status, item.RuntimeSmokeRefs)
		if err != nil {
			return nil, nil, err
		}
		deferReasons = append(deferReasons, smokeRefDeferReasons...)
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence candidate_evaluations candidate %q source", candidate), item.Source); err != nil {
			return nil, nil, err
		}
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence candidate_evaluations candidate %q notes", candidate), item.Notes); err != nil {
			return nil, nil, err
		}
		byCandidate[candidate] = item
	}
	if selected == "" {
		return deferReasons, iterateReasons, nil
	}
	selectedEval, ok := byCandidate[selected]
	if !ok {
		deferReasons = append(deferReasons, fmt.Sprintf("review evidence selected_candidate %q must have a matching candidate_evaluations entry", selected))
		return deferReasons, iterateReasons, nil
	}
	if selectedEval.Status != "pass" {
		iterateReasons = append(iterateReasons, fmt.Sprintf("selected candidate %q evaluation status = %q, want pass", selected, selectedEval.Status))
	}
	if runtimeCandidate != "" && selectedEval.RuntimeCandidate != "" && selectedEval.RuntimeCandidate != runtimeCandidate {
		deferReasons = append(deferReasons, fmt.Sprintf("selected candidate %q runtime_candidate %q must match review evidence runtime_candidate %q", selected, selectedEval.RuntimeCandidate, runtimeCandidate))
	}
	return deferReasons, iterateReasons, nil
}

func validateCandidateRuntimeSmokeRefs(candidate, status string, refs []string) ([]string, error) {
	seen := map[string]bool{}
	for i, ref := range refs {
		value := strings.TrimSpace(ref)
		if value == "" {
			return nil, fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] is required", candidate, i)
		}
		if value != ref {
			return nil, fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] must not contain leading or trailing whitespace", candidate, i)
		}
		if !requiredRuntimeSmokeName(value) {
			return nil, fmt.Errorf("review evidence candidate_evaluations candidate %q runtime_smoke_refs[%d] = %q is not a required runtime smoke", candidate, i, value)
		}
		if seen[value] {
			return nil, fmt.Errorf("review evidence candidate_evaluations candidate %q duplicate runtime_smoke_refs value %q", candidate, value)
		}
		seen[value] = true
	}
	if status != "pass" {
		return nil, nil
	}
	var reasons []string
	for _, name := range requiredRuntimeSmokes {
		if !seen[name] {
			reasons = append(reasons, fmt.Sprintf("review evidence candidate_evaluations candidate %q runtime_smoke_refs must include %q when status is pass", candidate, name))
		}
	}
	return reasons, nil
}

func validateReviewedCases(cases []reviewedCase) ([]string, error) {
	if len(cases) == 0 {
		return []string{"review evidence reviewed_cases must contain at least one item"}, nil
	}
	seen := map[string]bool{}
	for i, item := range cases {
		id, err := validateEvidenceCaseID(fmt.Sprintf("review evidence reviewed_cases[%d].id", i), item.ID)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate review evidence reviewed case %q", id)
		}
		seen[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return nil, fmt.Errorf("review evidence reviewed case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return nil, fmt.Errorf("review evidence reviewed case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return nil, fmt.Errorf("review evidence reviewed case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateReviewedCaseRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.Status) == "" {
			return nil, fmt.Errorf("review evidence reviewed case %q status is required", id)
		}
		if strings.TrimSpace(item.Status) != item.Status {
			return nil, fmt.Errorf("review evidence reviewed case %q status must not contain leading or trailing whitespace", id)
		}
		if !validReviewStatus(item.Status) {
			return nil, fmt.Errorf("review evidence reviewed case %q status = %q, want pass or fail", id, item.Status)
		}
		if err := validateReviewEvidenceText(fmt.Sprintf("review evidence reviewed case %q notes", id), item.Notes); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func validateReviewedCaseRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("review evidence reviewed case %q required_evidence_refs must contain at least one item", caseID)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	for i, ref := range refs {
		value, err := validateEvidenceRef(fmt.Sprintf("review evidence reviewed case %q required_evidence_refs[%d]", caseID, i), ref)
		if err != nil {
			return err
		}
		if seen[value] {
			return fmt.Errorf("review evidence reviewed case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validQualitySnapshotRef(value) {
				return fmt.Errorf("review evidence reviewed case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
	}
	if !hasSnapshotRef {
		return fmt.Errorf("review evidence reviewed case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return nil
}

func validateEvidenceCaseID(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("%s must be a single-line value", field)
	}
	if len(raw) > maxEvidenceCaseIDBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxEvidenceCaseIDBytes)
	}
	return raw, nil
}

func validateEvidenceRef(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("%s must be a single-line value", field)
	}
	if len([]rune(raw)) > maxEvidenceRefRunes {
		return "", fmt.Errorf("%s exceeds %d runes", field, maxEvidenceRefRunes)
	}
	return raw, nil
}

func validateReviewEvidenceText(field, raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("%s must be a single-line value", field)
	}
	if len(raw) > maxReviewEvidenceTextBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxReviewEvidenceTextBytes)
	}
	return nil
}

func validateReviewCoverage(qualityCases []qualityCase, reviewedCases []reviewedCase) ([]string, []string) {
	reviewedByID := map[string]reviewedCase{}
	for _, item := range reviewedCases {
		reviewedByID[item.ID] = item
	}
	qualityIDs := map[string]bool{}
	var deferReasons []string
	var iterateReasons []string
	for _, item := range qualityCases {
		qualityIDs[item.ID] = true
		review, ok := reviewedByID[item.ID]
		if !ok {
			deferReasons = append(deferReasons, fmt.Sprintf("review evidence missing reviewed case %q", item.ID))
			continue
		}
		if review.Status != "pass" {
			iterateReasons = append(iterateReasons, fmt.Sprintf("review evidence reviewed case %q status = %q, want pass", item.ID, review.Status))
		}
		if review.Scenario != item.Scenario {
			deferReasons = append(deferReasons, fmt.Sprintf("review evidence reviewed case %q scenario = %q, want quality comparison scenario %q", item.ID, review.Scenario, item.Scenario))
		}
		if !equalStrings(review.RequiredEvidenceRefs, item.RequiredEvidenceRefs) {
			deferReasons = append(deferReasons, fmt.Sprintf("review evidence reviewed case %q required_evidence_refs = %v, want quality comparison refs %v", item.ID, review.RequiredEvidenceRefs, item.RequiredEvidenceRefs))
		}
	}
	for _, review := range reviewedCases {
		if !qualityIDs[review.ID] {
			deferReasons = append(deferReasons, fmt.Sprintf("review evidence reviewed case %q does not match a quality comparison case", review.ID))
		}
	}
	return deferReasons, iterateReasons
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateRuntimeSmokes(smokes []runtimeSmoke) ([]string, error) {
	seen := map[string]runtimeSmoke{}
	seenEvidenceRefs := map[string]bool{}
	for i, smoke := range smokes {
		name := strings.TrimSpace(smoke.Name)
		if name == "" {
			return nil, fmt.Errorf("runtime_smokes[%d].name is required", i)
		}
		if name != smoke.Name {
			return nil, fmt.Errorf("runtime smoke name %q must not contain leading or trailing whitespace", smoke.Name)
		}
		if !requiredRuntimeSmokeName(name) {
			return nil, fmt.Errorf("runtime_smokes[%d].name = %q is not a required runtime smoke", i, name)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate runtime smoke %q", name)
		}
		evidenceRef, err := validateRuntimeSmokeEvidence(name, smoke.EvidenceRef, smoke.EvidenceSHA256)
		if err != nil {
			return nil, err
		}
		if seenEvidenceRefs[evidenceRef] {
			return nil, fmt.Errorf("runtime smoke %q evidence_ref %q duplicates another runtime smoke", name, evidenceRef)
		}
		seenEvidenceRefs[evidenceRef] = true
		seen[name] = smoke
	}
	var issues []string
	for _, name := range requiredRuntimeSmokes {
		smoke, ok := seen[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("runtime smoke %q is missing", name))
			continue
		}
		source := strings.TrimSpace(smoke.Source)
		if source == "" {
			issues = append(issues, fmt.Sprintf("runtime smoke %q source is required", name))
		} else if source != smoke.Source {
			return nil, fmt.Errorf("runtime smoke %q source must not contain leading or trailing whitespace", name)
		} else if want := requiredRuntimeSmokeSources[name]; source != want {
			issues = append(issues, fmt.Sprintf("runtime smoke %q source = %q, want %q", name, source, want))
		}
		if !validReviewStatus(smoke.Status) {
			return nil, fmt.Errorf("runtime smoke %q status = %q, want pass or fail", name, smoke.Status)
		}
		if smoke.Status != "pass" {
			issues = append(issues, fmt.Sprintf("runtime smoke %q status = %q, want pass", name, smoke.Status))
		}
	}
	return issues, nil
}

func validateRuntimeSmokeEvidence(name, ref, digest string) (string, error) {
	value := strings.TrimSpace(ref)
	if value == "" {
		return "", fmt.Errorf("runtime smoke %q evidence_ref is required", name)
	}
	if value != ref {
		return "", fmt.Errorf("runtime smoke %q evidence_ref must not contain leading or trailing whitespace", name)
	}
	if strings.ContainsAny(ref, "\r\n\t") {
		return "", fmt.Errorf("runtime smoke %q evidence_ref must be a single-line value", name)
	}
	if len(ref) > maxRuntimeSmokeEvidenceRefBytes {
		return "", fmt.Errorf("runtime smoke %q evidence_ref exceeds %d bytes", name, maxRuntimeSmokeEvidenceRefBytes)
	}
	if err := validatePortableEvidenceRef(name, ref); err != nil {
		return "", err
	}
	if !imageDigestHexRe.MatchString(digest) {
		return "", fmt.Errorf("runtime smoke %q evidence_sha256 must be 64 lowercase hex characters", name)
	}
	return ref, nil
}

func validatePortableEvidenceRef(name, ref string) error {
	if strings.HasPrefix(ref, "/") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a relative artifact path", name)
	}
	if strings.ContainsAny(ref, "\\: ") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a slash-separated relative artifact path without spaces or URI syntax", name)
	}
	clean := path.Clean(ref)
	if clean != ref || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("runtime smoke %q evidence_ref must be a normalized relative artifact path", name)
	}
	return nil
}

func readJSONFile(path string, dst any) error {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return err
	}
	// #nosec G304 -- this offline decision helper intentionally opens
	// operator-supplied evidence files.
	f, err := os.Open(clean)
	if err != nil {
		return fmt.Errorf("open %s: %w", clean, err)
	}
	defer f.Close()
	raw, err := readCapped(f, maxInputBytes)
	if err != nil {
		return fmt.Errorf("read %s: %w", clean, err)
	}
	if err := strictjson.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("parse %s: %w", clean, err)
	}
	return nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
	}
	return nil
}

func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func mapStatusChecks(checks []statusCheck) (map[string]string, error) {
	out := make(map[string]string, len(checks))
	for i, check := range checks {
		name := strings.TrimSpace(check.Name)
		if name == "" {
			return nil, fmt.Errorf("baseline audit checks[%d].name is required", i)
		}
		if name != check.Name {
			return nil, fmt.Errorf("baseline audit check name %q must not contain leading or trailing whitespace", check.Name)
		}
		if _, ok := out[name]; ok {
			return nil, fmt.Errorf("duplicate baseline audit check %q", name)
		}
		out[name] = check.Status
	}
	return out, nil
}

func countPassedRuntimeSmokes(smokes []runtimeSmoke) int {
	count := 0
	for _, smoke := range smokes {
		if smoke.Status == "pass" {
			count++
		}
	}
	return count
}

func batchRecommendation(summary qualitySummary) string {
	switch {
	case summary.RegressedCount > 0:
		return "sandbox_batch_has_regressions"
	case summary.NeedsHumanReviewCount > 0:
		return "needs_human_review"
	case summary.ImprovedCount > 0:
		return "sandbox_batch_candidate_improved"
	default:
		return "equivalent_metrics"
	}
}

func validCaseRecommendation(recommendation string) bool {
	switch recommendation {
	case "sandbox_candidate_improved", "equivalent_metrics", "sandbox_candidate_regressed", "needs_human_review":
		return true
	default:
		return false
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validDecision(decision string) bool {
	switch decision {
	case decisionProceed, decisionIterate, decisionDefer:
		return true
	default:
		return false
	}
}

func validReviewStatus(status string) bool {
	switch status {
	case "pass", "fail":
		return true
	default:
		return false
	}
}

func validCandidateID(candidate string) bool {
	return candidate != "" && len(candidate) <= maxEvidenceCaseIDBytes && !strings.ContainsAny(candidate, " \t\r\n")
}

func validCandidateEvaluationStatus(status string) bool {
	switch status {
	case "pass", "fail", "not_fit":
		return true
	default:
		return false
	}
}

func requiredRuntimeSmokeName(name string) bool {
	for _, required := range requiredRuntimeSmokes {
		if name == required {
			return true
		}
	}
	return false
}

func immutableImageReference(value string) bool {
	name, digest, ok := strings.Cut(value, "@sha256:")
	if !ok || name == "" {
		return false
	}
	if strings.ContainsAny(name, " \t\r\n@") {
		return false
	}
	return imageDigestHexRe.MatchString(digest)
}

func loopbackImageReference(value string) bool {
	name, _, ok := strings.Cut(value, "@sha256:")
	if !ok {
		return false
	}
	registry := imageReferenceRegistry(name)
	if registry == "" {
		return false
	}
	host := registry
	if h, _, err := net.SplitHostPort(registry); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func imageReferenceRegistry(name string) string {
	first, _, _ := strings.Cut(name, "/")
	if first == "" {
		return ""
	}
	if strings.EqualFold(first, "localhost") || strings.ContainsAny(first, ".:") {
		return first
	}
	return ""
}
