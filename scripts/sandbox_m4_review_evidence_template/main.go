// Command sandbox_m4_review_evidence_template builds a draft M4 review
// evidence JSON file from a retained quality comparison and runtime-smoke
// artifacts. It does not accept a runtime baseline; generated review and
// candidate statuses are fail-closed until an operator edits the evidence.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	toolName                     = "sandbox_m4_review_evidence_template"
	maxInputBytes                = 1024 * 1024
	maxRuntimeCandidateFileBytes = 4096
	maxTextBytes                 = 2048
	maxCandidateBytes            = 128
	maxEvidenceRefRunes          = 120
	requiredSnapshotRefPrefix    = "snapshot:"
)

var digestPinnedImageRE = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

var todayUTC = func() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

type config struct {
	QualityComparisonPath     string
	RuntimeSmokeArtifactsRoot string
	RuntimeSmokeRefPrefix     string
	SelectedCandidate         string
	RuntimeCandidate          string
	RuntimeCandidateFile      string
	Reviewer                  string
	EvidenceDate              string
	RepresentativeSample      bool
	OutPath                   string
}

type qualityComparison struct {
	Tool        string        `json:"tool"`
	Mode        string        `json:"mode"`
	CaseCount   int           `json:"case_count"`
	SampleBasis string        `json:"sample_basis"`
	Cases       []qualityCase `json:"cases"`
}

type qualityCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
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

type runtimeSmoke struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	EvidenceRef    string `json:"evidence_ref"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type reviewedCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	Status               string   `json:"status"`
	Notes                string   `json:"notes"`
}

type humanReview struct {
	Status   string `json:"status"`
	Reviewer string `json:"reviewer"`
	Notes    string `json:"notes"`
}

type runtimeSmokeSpec struct {
	Name     string
	Source   string
	FileName string
}

var runtimeSmokeSpecs = []runtimeSmokeSpec{
	{Name: "candidate_runtime_file_contract", Source: "make agent-runtime-smoke", FileName: "agent-runtime-smoke.json"},
	{Name: "container_provider_lifecycle", Source: "make container-provider-smoke", FileName: "container-provider-smoke.json"},
	{Name: "container_provider_timeout_cleanup", Source: "make container-provider-timeout-smoke", FileName: "container-provider-timeout-smoke.json"},
	{Name: "container_provider_output_cap", Source: "make container-provider-output-cap-smoke", FileName: "container-provider-output-cap-smoke.json"},
	{Name: "egress_allowdeny", Source: "make egress-allowdeny-smoke", FileName: "egress-allowdeny-smoke.json"},
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-m4-review-evidence-template] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	out, err := buildTemplate(cfg)
	if err != nil {
		return err
	}
	if cfg.OutPath != "" {
		return writeJSONFile(cfg.OutPath, out)
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		EvidenceDate: todayUTC().Format("2006-01-02"),
	}
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.QualityComparisonPath, "quality-comparison", "", "sandbox_quality_compare --manifest output JSON")
	fs.StringVar(&cfg.RuntimeSmokeArtifactsRoot, "runtime-smoke-artifacts-root", "", "directory containing retained runtime-smoke artifacts")
	fs.StringVar(&cfg.RuntimeSmokeRefPrefix, "runtime-smoke-ref-prefix", "", "optional slash-separated prefix for smoke artifact refs")
	fs.StringVar(&cfg.SelectedCandidate, "selected-candidate", "", "operator-supplied candidate evidence ID")
	fs.StringVar(&cfg.RuntimeCandidate, "runtime-candidate", "", "digest-pinned runtime image reference")
	fs.StringVar(&cfg.RuntimeCandidateFile, "runtime-candidate-file", "", "direct regular file containing the digest-pinned runtime image reference")
	fs.StringVar(&cfg.Reviewer, "reviewer", "", "reviewer identity for the draft human_review block")
	fs.StringVar(&cfg.EvidenceDate, "evidence-date", cfg.EvidenceDate, "review evidence date in YYYY-MM-DD format")
	fs.BoolVar(&cfg.RepresentativeSample, "representative-sample", false, "set review evidence representative_sample to true")
	fs.StringVar(&cfg.OutPath, "out", "", "optional output review-evidence JSON path; stdout is used when omitted")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg.QualityComparisonPath = strings.TrimSpace(cfg.QualityComparisonPath)
	cfg.RuntimeSmokeArtifactsRoot = strings.TrimSpace(cfg.RuntimeSmokeArtifactsRoot)
	cfg.RuntimeSmokeRefPrefix = strings.Trim(strings.TrimSpace(cfg.RuntimeSmokeRefPrefix), "/")
	cfg.SelectedCandidate = strings.TrimSpace(cfg.SelectedCandidate)
	cfg.RuntimeCandidate = strings.TrimSpace(cfg.RuntimeCandidate)
	cfg.RuntimeCandidateFile = strings.TrimSpace(cfg.RuntimeCandidateFile)
	cfg.Reviewer = strings.TrimSpace(cfg.Reviewer)
	cfg.EvidenceDate = strings.TrimSpace(cfg.EvidenceDate)
	cfg.OutPath = strings.TrimSpace(cfg.OutPath)
	if cfg.QualityComparisonPath == "" {
		return config{}, errors.New("--quality-comparison is required")
	}
	if cfg.RuntimeSmokeArtifactsRoot == "" {
		return config{}, errors.New("--runtime-smoke-artifacts-root is required")
	}
	if !validCandidateID(cfg.SelectedCandidate) {
		return config{}, fmt.Errorf("--selected-candidate must be a non-whitespace value up to %d bytes", maxCandidateBytes)
	}
	if cfg.RuntimeCandidate == "" && cfg.RuntimeCandidateFile == "" {
		return config{}, errors.New("--runtime-candidate or --runtime-candidate-file is required")
	}
	if cfg.RuntimeCandidate != "" && cfg.RuntimeCandidateFile != "" {
		return config{}, errors.New("set only one of --runtime-candidate or --runtime-candidate-file")
	}
	if cfg.RuntimeCandidateFile != "" {
		runtimeCandidate, err := readRuntimeCandidateFile(cfg.RuntimeCandidateFile)
		if err != nil {
			return config{}, err
		}
		cfg.RuntimeCandidate = runtimeCandidate
	}
	if !digestPinnedImageRE.MatchString(cfg.RuntimeCandidate) {
		return config{}, errors.New("--runtime-candidate must be an immutable image reference `name@sha256:<64-hex-digest>`")
	}
	if err := validateText("--reviewer", cfg.Reviewer); err != nil {
		return config{}, err
	}
	if evidenceDate, err := time.Parse("2006-01-02", cfg.EvidenceDate); err != nil {
		return config{}, fmt.Errorf("--evidence-date %q must be YYYY-MM-DD", cfg.EvidenceDate)
	} else if evidenceDate.After(todayUTC()) {
		return config{}, fmt.Errorf("--evidence-date %q must not be in the future", cfg.EvidenceDate)
	}
	if cfg.RuntimeSmokeRefPrefix != "" {
		if err := validateArtifactRef("--runtime-smoke-ref-prefix", cfg.RuntimeSmokeRefPrefix); err != nil {
			return config{}, err
		}
	}
	return cfg, nil
}

func buildTemplate(cfg config) (reviewEvidence, error) {
	quality, err := parseQualityComparison(cfg.QualityComparisonPath)
	if err != nil {
		return reviewEvidence{}, err
	}
	if err := requireDirectory(cfg.RuntimeSmokeArtifactsRoot); err != nil {
		return reviewEvidence{}, err
	}
	smokes, smokeRefs, err := runtimeSmokeEvidence(cfg.RuntimeSmokeArtifactsRoot, cfg.RuntimeSmokeRefPrefix)
	if err != nil {
		return reviewEvidence{}, err
	}
	reviewed := make([]reviewedCase, 0, len(quality.Cases))
	for _, item := range quality.Cases {
		reviewed = append(reviewed, reviewedCase{
			ID:                   item.ID,
			Scenario:             item.Scenario,
			RequiredEvidenceRefs: append([]string(nil), item.RequiredEvidenceRefs...),
			Status:               "fail",
			Notes:                "draft case review requires operator judgement before candidate acceptance",
		})
	}
	return reviewEvidence{
		Tool:                 "sandbox_m4_review_evidence",
		EvidenceDate:         cfg.EvidenceDate,
		SelectedCandidate:    cfg.SelectedCandidate,
		RuntimeCandidate:     cfg.RuntimeCandidate,
		RepresentativeSample: cfg.RepresentativeSample,
		SampleBasis:          quality.SampleBasis,
		CandidateEvaluations: []candidateEvaluation{{
			Candidate:        cfg.SelectedCandidate,
			Status:           "fail",
			RuntimeCandidate: cfg.RuntimeCandidate,
			RuntimeSmokeRefs: smokeRefs,
			Source:           "draft generated from quality comparison and retained runtime smoke artifacts",
			Notes:            "draft candidate evaluation requires operator judgement before candidate acceptance",
		}},
		RuntimeSmokes: smokes,
		ReviewedCases: reviewed,
		HumanReview: humanReview{
			Status:   "fail",
			Reviewer: cfg.Reviewer,
			Notes:    "draft human review requires operator judgement before candidate acceptance",
		},
	}, nil
}

func parseQualityComparison(filePath string) (qualityComparison, error) {
	raw, err := readRegularFileCapped(filePath, maxInputBytes)
	if err != nil {
		return qualityComparison{}, fmt.Errorf("quality comparison: %w", err)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return qualityComparison{}, fmt.Errorf("quality comparison parse: %w", err)
	}
	var quality qualityComparison
	if err := json.Unmarshal(raw, &quality); err != nil {
		return qualityComparison{}, fmt.Errorf("quality comparison parse: %w", err)
	}
	if quality.Tool != "sandbox_quality_compare" {
		return qualityComparison{}, fmt.Errorf("quality comparison tool = %q, want sandbox_quality_compare", quality.Tool)
	}
	if quality.Mode != "manifest" {
		return qualityComparison{}, fmt.Errorf("quality comparison mode = %q, want manifest", quality.Mode)
	}
	if err := validateText("quality comparison sample_basis", quality.SampleBasis); err != nil {
		return qualityComparison{}, err
	}
	if quality.CaseCount <= 0 {
		return qualityComparison{}, errors.New("quality comparison case_count must be greater than zero")
	}
	if len(quality.Cases) != quality.CaseCount {
		return qualityComparison{}, fmt.Errorf("quality comparison has %d cases, want case_count %d", len(quality.Cases), quality.CaseCount)
	}
	seen := map[string]bool{}
	for i, item := range quality.Cases {
		id, err := validateQualityCaseID(i, item.ID)
		if err != nil {
			return qualityComparison{}, err
		}
		if seen[id] {
			return qualityComparison{}, fmt.Errorf("duplicate quality comparison case id %q", id)
		}
		seen[id] = true
		scenario, err := validateQualityCaseScenario(id, item.Scenario)
		if err != nil {
			return qualityComparison{}, err
		}
		quality.Cases[i].Scenario = scenario
		refs, err := validateQualityRequiredEvidenceRefs(id, item.RequiredEvidenceRefs)
		if err != nil {
			return qualityComparison{}, err
		}
		quality.Cases[i].RequiredEvidenceRefs = refs
	}
	return quality, nil
}

func validateQualityCaseID(index int, raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", fmt.Errorf("quality comparison cases[%d].id is required", index)
	}
	if id != raw {
		return "", fmt.Errorf("quality comparison case id %q must not contain leading or trailing whitespace", raw)
	}
	if !validSingleLineBounded(id, maxCandidateBytes) {
		return "", fmt.Errorf("quality comparison cases[%d].id %q must be single-line and up to %d bytes", index, id, maxCandidateBytes)
	}
	return id, nil
}

func validateQualityCaseScenario(caseID, raw string) (string, error) {
	scenario := strings.TrimSpace(raw)
	if scenario == "" {
		return "", fmt.Errorf("quality comparison case %q scenario is required", caseID)
	}
	if scenario != raw {
		return "", fmt.Errorf("quality comparison case %q scenario must not contain leading or trailing whitespace", caseID)
	}
	if !reportprompt.Scenario(scenario).Valid() {
		return "", fmt.Errorf("quality comparison case %q scenario %q is unsupported", caseID, scenario)
	}
	return scenario, nil
}

func validateQualityRequiredEvidenceRefs(caseID string, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("quality comparison case %q required_evidence_refs must contain at least one item", caseID)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	out := make([]string, 0, len(refs))
	for i, ref := range refs {
		value, err := validateEvidenceRef(fmt.Sprintf("quality comparison case %q required_evidence_refs[%d]", caseID, i), ref)
		if err != nil {
			return nil, err
		}
		if seen[value] {
			return nil, fmt.Errorf("quality comparison case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validQualitySnapshotRef(value) {
				return nil, fmt.Errorf("quality comparison case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
		out = append(out, value)
	}
	if !hasSnapshotRef {
		return nil, fmt.Errorf("quality comparison case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return out, nil
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

func validQualitySnapshotRef(ref string) bool {
	rawID := strings.TrimPrefix(ref, requiredSnapshotRefPrefix)
	id, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == rawID
}

func readRuntimeCandidateFile(filePath string) (string, error) {
	raw, err := readRegularFileCapped(filePath, maxRuntimeCandidateFileBytes)
	if err != nil {
		return "", fmt.Errorf("--runtime-candidate-file: %w", err)
	}
	candidate := strings.TrimSuffix(string(raw), "\n")
	if candidate == "" {
		return "", errors.New("--runtime-candidate-file must contain one immutable image reference")
	}
	if strings.ContainsAny(candidate, "\r\n\t ") || !digestPinnedImageRE.MatchString(candidate) {
		return "", errors.New("--runtime-candidate-file must contain exactly one immutable image reference `name@sha256:<64-hex-digest>` followed by an optional newline")
	}
	return candidate, nil
}

func runtimeSmokeEvidence(root, refPrefix string) ([]runtimeSmoke, []string, error) {
	smokes := make([]runtimeSmoke, 0, len(runtimeSmokeSpecs))
	refs := make([]string, 0, len(runtimeSmokeSpecs))
	for _, spec := range runtimeSmokeSpecs {
		ref, err := runtimeSmokeRef(refPrefix, spec.FileName)
		if err != nil {
			return nil, nil, err
		}
		raw, err := readRegularFileCapped(filepath.Join(root, filepath.FromSlash(ref)), maxInputBytes)
		if err != nil {
			return nil, nil, fmt.Errorf("runtime smoke %q artifact %q: %w", spec.Name, ref, err)
		}
		if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
			return nil, nil, fmt.Errorf("runtime smoke %q artifact %q parse: %w", spec.Name, ref, err)
		}
		var artifact struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return nil, nil, fmt.Errorf("runtime smoke %q artifact %q parse: %w", spec.Name, ref, err)
		}
		status := strings.TrimSpace(artifact.Status)
		if status != artifact.Status || (status != "pass" && status != "fail") {
			return nil, nil, fmt.Errorf("runtime smoke %q artifact %q status = %q, want pass or fail", spec.Name, ref, artifact.Status)
		}
		sum := sha256.Sum256(raw)
		smokes = append(smokes, runtimeSmoke{
			Name:           spec.Name,
			Status:         status,
			Source:         spec.Source,
			EvidenceRef:    ref,
			EvidenceSHA256: hex.EncodeToString(sum[:]),
		})
		refs = append(refs, spec.Name)
	}
	return smokes, refs, nil
}

func runtimeSmokeRef(prefix, fileName string) (string, error) {
	ref := fileName
	if prefix != "" {
		ref = path.Join(prefix, fileName)
	}
	if err := validateArtifactRef("runtime smoke evidence_ref", ref); err != nil {
		return "", err
	}
	return ref, nil
}

func validateArtifactRef(field, ref string) error {
	value := strings.TrimSpace(ref)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != ref {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(value, "\\: \r\n\t") {
		return fmt.Errorf("%s must be a slash-separated relative artifact path without spaces", field)
	}
	if path.IsAbs(value) {
		return fmt.Errorf("%s must be relative", field)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%s must be a normalized relative artifact path", field)
		}
	}
	if path.Clean(value) != value {
		return fmt.Errorf("%s must be normalized", field)
	}
	return nil
}

func validateText(field, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if value != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if !validSingleLineBounded(raw, maxTextBytes) {
		return fmt.Errorf("%s must be single-line and up to %d bytes", field, maxTextBytes)
	}
	return nil
}

func validCandidateID(raw string) bool {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return false
	}
	if strings.ContainsAny(raw, " \r\n\t") {
		return false
	}
	return len(raw) <= maxCandidateBytes
}

func validSingleLineBounded(raw string, maxBytes int) bool {
	return !strings.ContainsAny(raw, "\r\n\t") && len(raw) <= maxBytes
}

func requireDirectory(dir string) error {
	info, err := os.Lstat(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("stat runtime smoke artifacts root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("runtime smoke artifacts root must be a direct directory")
	}
	return nil
}

func readRegularFileCapped(filePath string, maxBytes int64) ([]byte, error) {
	clean := filepath.Clean(filePath)
	info, err := os.Lstat(clean)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", clean)
	}
	// #nosec G304 -- this offline evidence helper opens operator-supplied
	// retained artifact paths after direct regular-file checks.
	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", clean, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds maximum %d bytes", clean, maxBytes)
	}
	return raw, nil
}

func writeJSONFile(filePath string, value any) error {
	clean := filepath.Clean(filePath)
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s must be absent before review evidence output is written, not a symlink", clean)
		}
		return fmt.Errorf("%s must be absent before review evidence output is written", clean)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	// #nosec G304 -- this manual helper writes the operator-supplied output path.
	f, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = f.Close()
		return fmt.Errorf("write output: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
