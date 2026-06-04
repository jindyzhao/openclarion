// Command sandbox_quality_compare compares a direct M2 SubReport with a
// sandbox-augmented SubReport after validating both against the production
// reportdraft contract. It is an offline quality-gate aid, not an automated
// product decision.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	maxInputBytes               int64 = 1024 * 1024
	maxManifestRequiredRefs           = 20
	maxManifestRequiredRefRunes       = 120
	requiredSnapshotRefPrefix         = "snapshot:"
	maxManifestSampleBasisBytes       = 2048
	maxManifestCaseIDBytes            = 128
	maxManifestReportPathBytes        = 512
)

var requiredManifestScenarios = []reportprompt.Scenario{
	reportprompt.ScenarioSingleAlert,
	reportprompt.ScenarioCascade,
	reportprompt.ScenarioAlertStorm,
}

type config struct {
	ManifestPath     string
	DirectPath       string
	SandboxPath      string
	OutPath          string
	FailOnRegression bool
}

type reportMetrics struct {
	FindingCount           int `json:"finding_count"`
	RecommendedActionCount int `json:"recommended_action_count"`
	// High-priority action and severity deltas are review context, not
	// automatic quality wins.
	HighPriorityActionCount int `json:"high_priority_action_count"`
	UniqueEvidenceRefCount  int `json:"unique_evidence_ref_count"`
	ConfidenceRank          int `json:"confidence_rank"`
	SeverityRank            int `json:"severity_rank"`
}

type deltas struct {
	FindingCount           int `json:"finding_count"`
	RecommendedActionCount int `json:"recommended_action_count"`
	// High-priority action and severity deltas are review context, not
	// automatic quality wins.
	HighPriorityActionCount int `json:"high_priority_action_count"`
	UniqueEvidenceRefCount  int `json:"unique_evidence_ref_count"`
	ConfidenceRank          int `json:"confidence_rank"`
	SeverityRank            int `json:"severity_rank"`
}

type comparisonOutput struct {
	Tool           string        `json:"tool"`
	Schema         string        `json:"schema"`
	Direct         reportMetrics `json:"direct"`
	Sandbox        reportMetrics `json:"sandbox"`
	Delta          deltas        `json:"delta"`
	Recommendation string        `json:"recommendation"`
	ReviewRequired bool          `json:"review_required"`
	Notes          []string      `json:"notes,omitempty"`
}

type manifestFile struct {
	SampleBasis string         `json:"sample_basis"`
	Cases       []manifestCase `json:"cases"`
}

type manifestCase struct {
	ID                   string   `json:"id"`
	Scenario             string   `json:"scenario"`
	RequiredEvidenceRefs []string `json:"required_evidence_refs"`
	DirectSubReport      string   `json:"direct_sub_report"`
	SandboxSubReport     string   `json:"sandbox_sub_report"`
}

type batchComparisonOutput struct {
	Tool             string                 `json:"tool"`
	Schema           string                 `json:"schema"`
	Mode             string                 `json:"mode"`
	CaseCount        int                    `json:"case_count"`
	SampleBasis      string                 `json:"sample_basis"`
	ScenarioCoverage []string               `json:"scenario_coverage"`
	Summary          batchComparisonSummary `json:"summary"`
	Recommendation   string                 `json:"recommendation"`
	ReviewRequired   bool                   `json:"review_required"`
	Cases            []caseComparisonOutput `json:"cases"`
}

type batchComparisonSummary struct {
	ImprovedCount         int `json:"improved_count"`
	EquivalentCount       int `json:"equivalent_count"`
	RegressedCount        int `json:"regressed_count"`
	NeedsHumanReviewCount int `json:"needs_human_review_count"`
}

type caseComparisonOutput struct {
	ID                   string        `json:"id"`
	Scenario             string        `json:"scenario"`
	RequiredEvidenceRefs []string      `json:"required_evidence_refs"`
	Direct               reportMetrics `json:"direct"`
	Sandbox              reportMetrics `json:"sandbox"`
	Delta                deltas        `json:"delta"`
	Recommendation       string        `json:"recommendation"`
	ReviewRequired       bool          `json:"review_required"`
	Notes                []string      `json:"notes,omitempty"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-quality-compare] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	if cfg.ManifestPath != "" {
		out, err := compareManifest(cfg.ManifestPath)
		if err != nil {
			return err
		}
		if cfg.FailOnRegression && out.Summary.RegressedCount > 0 {
			return fmt.Errorf("sandbox report regressed in %d manifest case(s)", out.Summary.RegressedCount)
		}
		return writeJSONOutput(stdout, cfg.OutPath, out)
	}
	out, err := compareFiles(cfg)
	if err != nil {
		return err
	}
	if cfg.FailOnRegression && out.Recommendation == "sandbox_candidate_regressed" {
		return errors.New("sandbox report regressed against direct report")
	}
	return writeJSONOutput(stdout, cfg.OutPath, out)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("sandbox_quality_compare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	manifest := fs.String("manifest", "", "JSON manifest of direct/sandbox SubReport comparison cases")
	direct := fs.String("direct-sub-report", "", "direct M2 SubReport JSON path")
	sandbox := fs.String("sandbox-sub-report", "", "sandbox-augmented SubReport JSON path")
	out := fs.String("out", "", "optional output comparison JSON path; stdout is used when omitted")
	failOnRegression := fs.Bool("fail-on-regression", false, "exit non-zero when sandbox metrics regress")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	cfg := config{
		ManifestPath:     *manifest,
		DirectPath:       *direct,
		SandboxPath:      *sandbox,
		OutPath:          strings.TrimSpace(*out),
		FailOnRegression: *failOnRegression,
	}
	if cfg.ManifestPath != "" {
		if cfg.DirectPath != "" || cfg.SandboxPath != "" {
			return config{}, errors.New("--manifest cannot be combined with --direct-sub-report or --sandbox-sub-report")
		}
		return cfg, nil
	}
	if cfg.DirectPath == "" && cfg.SandboxPath == "" {
		return config{}, errors.New("--manifest or --direct-sub-report/--sandbox-sub-report is required")
	}
	if cfg.DirectPath == "" {
		return config{}, errors.New("--direct-sub-report is required when --manifest is not used")
	}
	if cfg.SandboxPath == "" {
		return config{}, errors.New("--sandbox-sub-report is required when --manifest is not used")
	}
	return cfg, nil
}

func writeJSONOutput(stdout io.Writer, outPath string, value any) error {
	if outPath == "" || outPath == "-" {
		return encodeJSON(stdout, value)
	}
	var buf bytes.Buffer
	if err := encodeJSON(&buf, value); err != nil {
		return err
	}
	return writeNewOutputFile(outPath, buf.Bytes())
}

func encodeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeNewOutputFile(path string, raw []byte) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." || clean == string(filepath.Separator) {
		return errors.New("output file must not be empty, current directory, or filesystem root")
	}
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode().IsRegular() {
			return fmt.Errorf("output file %s already exists", clean)
		}
		return fmt.Errorf("output path %s must be absent before helper output is written", clean)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output file %s: %w", clean, err)
	}
	parent := filepath.Dir(clean)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("output parent directory %s does not exist", parent)
	}
	if err != nil {
		return fmt.Errorf("stat output parent directory %s: %w", parent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output parent path %s must be a directory", parent)
	}
	// #nosec G304 -- this offline comparison tool intentionally writes an
	// operator-supplied retained evidence path after refusing overwrites.
	f, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create output file %s: %w", clean, err)
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return fmt.Errorf("write output file %s: %w", clean, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close output file %s: %w", clean, err)
	}
	return nil
}

func compareFiles(cfg config) (comparisonOutput, error) {
	comparison, _, _, err := compareFilesWithReports(cfg)
	return comparison, err
}

func compareFilesWithReports(cfg config) (comparisonOutput, reportdraft.SubReport, reportdraft.SubReport, error) {
	direct, err := parseSubReportFile(cfg.DirectPath)
	if err != nil {
		return comparisonOutput{}, reportdraft.SubReport{}, reportdraft.SubReport{}, fmt.Errorf("direct subreport: %w", err)
	}
	sandbox, err := parseSubReportFile(cfg.SandboxPath)
	if err != nil {
		return comparisonOutput{}, reportdraft.SubReport{}, reportdraft.SubReport{}, fmt.Errorf("sandbox subreport: %w", err)
	}
	return compareReports(direct, sandbox), direct, sandbox, nil
}

func compareManifest(path string) (batchComparisonOutput, error) {
	manifest, baseDir, err := parseManifestFile(path)
	if err != nil {
		return batchComparisonOutput{}, err
	}
	out := batchComparisonOutput{
		Tool:             "sandbox_quality_compare",
		Schema:           reportdraft.SubReportSchemaID,
		Mode:             "manifest",
		CaseCount:        len(manifest.Cases),
		SampleBasis:      manifest.SampleBasis,
		ScenarioCoverage: scenarioCoverage(manifest.Cases),
		ReviewRequired:   true,
		Cases:            make([]caseComparisonOutput, 0, len(manifest.Cases)),
	}
	for _, item := range manifest.Cases {
		cfg := config{
			DirectPath:  resolveManifestPath(baseDir, item.DirectSubReport),
			SandboxPath: resolveManifestPath(baseDir, item.SandboxSubReport),
		}
		comparison, direct, sandbox, err := compareFilesWithReports(cfg)
		if err != nil {
			return batchComparisonOutput{}, fmt.Errorf("case %q: %w", item.ID, err)
		}
		if err := validateCaseEvidenceBinding(item.ID, item.RequiredEvidenceRefs, direct, sandbox); err != nil {
			return batchComparisonOutput{}, err
		}
		out.Cases = append(out.Cases, caseComparisonOutput{
			ID:                   item.ID,
			Scenario:             item.Scenario,
			RequiredEvidenceRefs: append([]string(nil), item.RequiredEvidenceRefs...),
			Direct:               comparison.Direct,
			Sandbox:              comparison.Sandbox,
			Delta:                comparison.Delta,
			Recommendation:       comparison.Recommendation,
			ReviewRequired:       comparison.ReviewRequired,
			Notes:                comparison.Notes,
		})
		switch comparison.Recommendation {
		case "sandbox_candidate_improved":
			out.Summary.ImprovedCount++
		case "equivalent_metrics":
			out.Summary.EquivalentCount++
		case "sandbox_candidate_regressed":
			out.Summary.RegressedCount++
		case "needs_human_review":
			out.Summary.NeedsHumanReviewCount++
		default:
			return batchComparisonOutput{}, fmt.Errorf("case %q: unsupported recommendation %q", item.ID, comparison.Recommendation)
		}
	}
	out.Recommendation = batchRecommendation(out.Summary)
	return out, nil
}

func parseManifestFile(path string) (manifestFile, string, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return manifestFile{}, "", err
	}
	// #nosec G304 -- this offline comparison tool intentionally opens
	// operator-supplied quality evidence manifests.
	f, err := os.Open(clean)
	if err != nil {
		return manifestFile{}, "", fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()
	raw, err := readCapped(f, maxInputBytes)
	if err != nil {
		return manifestFile{}, "", fmt.Errorf("read manifest: %w", err)
	}
	var manifest manifestFile
	if err := strictjson.Unmarshal(raw, &manifest); err != nil {
		return manifestFile{}, "", fmt.Errorf("parse manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return manifestFile{}, "", err
	}
	return manifest, filepath.Dir(clean), nil
}

func validateManifest(manifest manifestFile) error {
	sampleBasis := strings.TrimSpace(manifest.SampleBasis)
	if sampleBasis == "" {
		return errors.New("manifest sample_basis is required")
	}
	if err := validateManifestSampleBasis(manifest.SampleBasis); err != nil {
		return err
	}
	if len(manifest.Cases) == 0 {
		return errors.New("manifest cases must contain at least one item")
	}
	if len(manifest.Cases) > 100 {
		return fmt.Errorf("manifest cases contains %d items, max 100", len(manifest.Cases))
	}
	seen := map[string]bool{}
	reportPaths := map[string]string{}
	for i, item := range manifest.Cases {
		id, err := validateManifestCaseID(i, item.ID)
		if err != nil {
			return err
		}
		if seen[id] {
			return fmt.Errorf("duplicate manifest case id %q", id)
		}
		seen[id] = true
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return fmt.Errorf("manifest case %q scenario is required", id)
		}
		if scenario != item.Scenario {
			return fmt.Errorf("manifest case %q scenario must not contain leading or trailing whitespace", id)
		}
		if !reportprompt.Scenario(scenario).Valid() {
			return fmt.Errorf("manifest case %q scenario %q is unsupported", id, scenario)
		}
		if err := validateManifestRequiredEvidenceRefs(id, item.RequiredEvidenceRefs); err != nil {
			return err
		}
		if strings.TrimSpace(item.DirectSubReport) == "" {
			return fmt.Errorf("manifest case %q direct_sub_report is required", id)
		}
		directPath, err := validateManifestRelativePath(id, "direct_sub_report", item.DirectSubReport)
		if err != nil {
			return err
		}
		if strings.TrimSpace(item.SandboxSubReport) == "" {
			return fmt.Errorf("manifest case %q sandbox_sub_report is required", id)
		}
		sandboxPath, err := validateManifestRelativePath(id, "sandbox_sub_report", item.SandboxSubReport)
		if err != nil {
			return err
		}
		if directPath == sandboxPath {
			return fmt.Errorf("manifest case %q direct_sub_report and sandbox_sub_report must be distinct files", id)
		}
		if err := rememberManifestReportPath(reportPaths, id, "direct_sub_report", directPath); err != nil {
			return err
		}
		if err := rememberManifestReportPath(reportPaths, id, "sandbox_sub_report", sandboxPath); err != nil {
			return err
		}
	}
	return nil
}

func validateManifestCaseID(index int, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("manifest cases[%d].id is required", index)
	}
	if value != raw {
		return "", fmt.Errorf("manifest case id %q must not contain leading or trailing whitespace", raw)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("manifest cases[%d].id must be a single-line value", index)
	}
	if len(raw) > maxManifestCaseIDBytes {
		return "", fmt.Errorf("manifest cases[%d].id exceeds %d bytes", index, maxManifestCaseIDBytes)
	}
	return value, nil
}

func validateManifestSampleBasis(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("manifest sample_basis must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("manifest sample_basis is required")
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return errors.New("manifest sample_basis must be a single-line value")
	}
	if len(raw) > maxManifestSampleBasisBytes {
		return fmt.Errorf("manifest sample_basis exceeds %d bytes", maxManifestSampleBasisBytes)
	}
	return nil
}

func validateManifestRequiredEvidenceRefs(caseID string, refs []string) error {
	if len(refs) == 0 {
		return fmt.Errorf("manifest case %q required_evidence_refs must contain at least one item", caseID)
	}
	if len(refs) > maxManifestRequiredRefs {
		return fmt.Errorf("manifest case %q required_evidence_refs contains %d items, max %d", caseID, len(refs), maxManifestRequiredRefs)
	}
	seen := map[string]bool{}
	hasSnapshotRef := false
	for i, ref := range refs {
		value := strings.TrimSpace(ref)
		if value == "" {
			return fmt.Errorf("manifest case %q required_evidence_refs[%d] is required", caseID, i)
		}
		if value != ref {
			return fmt.Errorf("manifest case %q required_evidence_refs[%d] must not contain leading or trailing whitespace", caseID, i)
		}
		if strings.ContainsAny(value, "\r\n\t") {
			return fmt.Errorf("manifest case %q required_evidence_refs[%d] must be a single-line value", caseID, i)
		}
		if len([]rune(value)) > maxManifestRequiredRefRunes {
			return fmt.Errorf("manifest case %q required_evidence_refs[%d] exceeds %d runes", caseID, i, maxManifestRequiredRefRunes)
		}
		if seen[value] {
			return fmt.Errorf("manifest case %q duplicate required_evidence_refs value %q", caseID, value)
		}
		if strings.HasPrefix(value, requiredSnapshotRefPrefix) {
			if !validManifestSnapshotRef(value) {
				return fmt.Errorf("manifest case %q required_evidence_refs[%d] snapshot evidence ref %q must use snapshot:<positive-id>", caseID, i, value)
			}
			hasSnapshotRef = true
		}
		seen[value] = true
	}
	if !hasSnapshotRef {
		return fmt.Errorf("manifest case %q required_evidence_refs must include one snapshot:<positive-id> reference", caseID)
	}
	return nil
}

func validManifestSnapshotRef(ref string) bool {
	rawID := strings.TrimPrefix(ref, requiredSnapshotRefPrefix)
	id, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == rawID
}

func validateCaseEvidenceBinding(caseID string, requiredRefs []string, direct, sandbox reportdraft.SubReport) error {
	directRefs := evidenceRefSet(direct)
	sandboxRefs := evidenceRefSet(sandbox)
	for _, ref := range requiredRefs {
		if _, ok := directRefs[ref]; !ok {
			return fmt.Errorf("case %q direct subreport missing required evidence ref %q", caseID, ref)
		}
		if _, ok := sandboxRefs[ref]; !ok {
			return fmt.Errorf("case %q sandbox subreport missing required evidence ref %q", caseID, ref)
		}
	}
	return nil
}

func evidenceRefSet(report reportdraft.SubReport) map[string]struct{} {
	refs := make(map[string]struct{}, len(report.EvidenceRefs)+len(report.Findings))
	for _, ref := range report.EvidenceRefs {
		refs[ref] = struct{}{}
	}
	for _, finding := range report.Findings {
		refs[finding.EvidenceID] = struct{}{}
	}
	return refs
}

func rememberManifestReportPath(seen map[string]string, caseID, field, path string) error {
	owner := caseID + "." + field
	if previous, ok := seen[path]; ok {
		return fmt.Errorf("manifest case %q %s repeats report path %q already used by %s", caseID, field, path, previous)
	}
	seen[path] = owner
	return nil
}

func validateManifestRelativePath(caseID, field, value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		return "", fmt.Errorf("manifest case %q %s is required", caseID, field)
	}
	if path != value {
		return "", fmt.Errorf("manifest case %q %s must not contain leading or trailing whitespace", caseID, field)
	}
	if strings.ContainsAny(path, "\r\n\t") {
		return "", fmt.Errorf("manifest case %q %s must be a single-line slash-separated relative path", caseID, field)
	}
	if len(path) > maxManifestReportPathBytes {
		return "", fmt.Errorf("manifest case %q %s exceeds %d bytes", caseID, field, maxManifestReportPathBytes)
	}
	if strings.ContainsAny(path, "\\:") {
		return "", fmt.Errorf("manifest case %q %s must be a slash-separated relative path", caseID, field)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("manifest case %q %s must be relative", caseID, field)
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return "", fmt.Errorf("manifest case %q %s must not contain parent directory traversal", caseID, field)
		}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("manifest case %q %s must point to a file under the manifest directory", caseID, field)
	}
	return clean, nil
}

func resolveManifestPath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func scenarioCoverage(cases []manifestCase) []string {
	seen := map[string]bool{}
	for _, item := range cases {
		seen[item.Scenario] = true
	}
	out := make([]string, 0, len(requiredManifestScenarios))
	for _, scenario := range requiredManifestScenarios {
		value := string(scenario)
		if seen[value] {
			out = append(out, value)
		}
	}
	return out
}

func batchRecommendation(summary batchComparisonSummary) string {
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

func parseSubReportFile(path string) (reportdraft.SubReport, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return reportdraft.SubReport{}, err
	}
	// #nosec G304 -- this offline comparison tool intentionally opens
	// operator-supplied report JSON files.
	f, err := os.Open(clean)
	if err != nil {
		return reportdraft.SubReport{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	raw, err := readCapped(f, maxInputBytes)
	if err != nil {
		return reportdraft.SubReport{}, err
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return reportdraft.SubReport{}, fmt.Errorf("parse: %w", err)
	}
	return reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      json.RawMessage(raw),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "offline-quality-compare",
	})
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

func compareReports(direct, sandbox reportdraft.SubReport) comparisonOutput {
	directMetrics := metricsFor(direct)
	sandboxMetrics := metricsFor(sandbox)
	delta := deltas{
		FindingCount:            sandboxMetrics.FindingCount - directMetrics.FindingCount,
		RecommendedActionCount:  sandboxMetrics.RecommendedActionCount - directMetrics.RecommendedActionCount,
		HighPriorityActionCount: sandboxMetrics.HighPriorityActionCount - directMetrics.HighPriorityActionCount,
		UniqueEvidenceRefCount:  sandboxMetrics.UniqueEvidenceRefCount - directMetrics.UniqueEvidenceRefCount,
		ConfidenceRank:          sandboxMetrics.ConfidenceRank - directMetrics.ConfidenceRank,
		SeverityRank:            sandboxMetrics.SeverityRank - directMetrics.SeverityRank,
	}
	recommendation, notes := recommend(delta)
	return comparisonOutput{
		Tool:           "sandbox_quality_compare",
		Schema:         reportdraft.SubReportSchemaID,
		Direct:         directMetrics,
		Sandbox:        sandboxMetrics,
		Delta:          delta,
		Recommendation: recommendation,
		ReviewRequired: true,
		Notes:          notes,
	}
}

func metricsFor(report reportdraft.SubReport) reportMetrics {
	evidenceRefs := make(map[string]struct{}, len(report.EvidenceRefs)+len(report.Findings))
	for _, ref := range report.EvidenceRefs {
		evidenceRefs[ref] = struct{}{}
	}
	for _, finding := range report.Findings {
		evidenceRefs[finding.EvidenceID] = struct{}{}
	}
	highPriorityActions := 0
	for _, action := range report.RecommendedActions {
		if action.Priority == reportdraft.PriorityHigh {
			highPriorityActions++
		}
	}
	return reportMetrics{
		FindingCount:            len(report.Findings),
		RecommendedActionCount:  len(report.RecommendedActions),
		HighPriorityActionCount: highPriorityActions,
		UniqueEvidenceRefCount:  len(evidenceRefs),
		ConfidenceRank:          confidenceRank(report.Confidence),
		SeverityRank:            severityRank(report.Severity),
	}
}

func recommend(delta deltas) (string, []string) {
	notes := make([]string, 0, 4)
	if delta.FindingCount < 0 {
		notes = append(notes, "sandbox has fewer findings")
	}
	if delta.UniqueEvidenceRefCount < 0 {
		notes = append(notes, "sandbox cites fewer unique evidence references")
	}
	if delta.RecommendedActionCount < 0 {
		notes = append(notes, "sandbox has fewer recommended actions")
	}
	if delta.ConfidenceRank < 0 {
		notes = append(notes, "sandbox confidence is lower")
	}
	if delta.HighPriorityActionCount != 0 {
		notes = append(notes, "high-priority action count changed and requires human review")
	}
	if delta.SeverityRank != 0 {
		notes = append(notes, "severity changed and requires human review")
	}
	sort.Strings(notes)
	if delta.FindingCount < 0 || delta.UniqueEvidenceRefCount < 0 || delta.RecommendedActionCount < 0 || delta.ConfidenceRank < 0 {
		return "sandbox_candidate_regressed", notes
	}
	if delta.FindingCount > 0 || delta.UniqueEvidenceRefCount > 0 || delta.RecommendedActionCount > 0 || delta.ConfidenceRank > 0 {
		return "sandbox_candidate_improved", notes
	}
	if delta.SeverityRank != 0 || delta.HighPriorityActionCount != 0 {
		return "needs_human_review", notes
	}
	return "equivalent_metrics", nil
}

func confidenceRank(confidence reportdraft.Confidence) int {
	switch confidence {
	case reportdraft.ConfidenceLow:
		return 1
	case reportdraft.ConfidenceMedium:
		return 2
	case reportdraft.ConfidenceHigh:
		return 3
	default:
		return 0
	}
}

func severityRank(severity reportdraft.Severity) int {
	switch severity {
	case reportdraft.SeverityInfo:
		return 1
	case reportdraft.SeverityWarning:
		return 2
	case reportdraft.SeverityCritical:
		return 3
	default:
		return 0
	}
}

func readCapped(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read report JSON: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("report JSON exceeds maximum %d bytes", maxBytes)
	}
	return raw, nil
}
