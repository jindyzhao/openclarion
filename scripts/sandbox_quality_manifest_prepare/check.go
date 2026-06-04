// Command sandbox_quality_manifest_prepare builds a sandbox quality manifest
// from paired direct and sandbox SubReport files. It prepares retained M4
// evidence inputs only; it does not judge sample representativeness or accept a
// runtime candidate.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
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
	maxManifestSampleBasisBytes       = 2048
	maxManifestCaseIDBytes            = 128
	maxManifestReportPathBytes        = 512
	requiredSnapshotRefPrefix         = "snapshot:"
	directRole                        = "direct"
	sandboxRole                       = "sandbox"
)

var requiredManifestScenarios = []reportprompt.Scenario{
	reportprompt.ScenarioSingleAlert,
	reportprompt.ScenarioCascade,
	reportprompt.ScenarioAlertStorm,
}

type config struct {
	RootPath    string
	SampleBasis string
	OutPath     string
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

type caseKey struct {
	Scenario string
	ID       string
}

type reportPair struct {
	DirectPath      string
	DirectManifest  string
	SandboxPath     string
	SandboxManifest string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-quality-manifest-prepare] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	manifest, err := buildManifest(cfg.RootPath, cfg.SampleBasis)
	if err != nil {
		return err
	}
	if cfg.OutPath == "" || cfg.OutPath == "-" {
		return encodeManifest(stdout, manifest)
	}
	clean := filepath.Clean(cfg.OutPath)
	if err := writeManifestFile(clean, manifest); err != nil {
		return err
	}
	return nil
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("sandbox_quality_manifest_prepare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "quality sample root containing direct/<scenario>/<case>.json and sandbox/<scenario>/<case>.json")
	sampleBasis := fs.String("sample-basis", "", "single-line representative sample basis")
	out := fs.String("out", "", "manifest output path; defaults to stdout, use - for stdout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	cfg := config{
		RootPath:    strings.TrimSpace(*root),
		SampleBasis: *sampleBasis,
		OutPath:     strings.TrimSpace(*out),
	}
	if cfg.RootPath == "" {
		return config{}, errors.New("--root is required")
	}
	if err := validateManifestSampleBasis(cfg.SampleBasis); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func buildManifest(rootPath, sampleBasis string) (manifestFile, error) {
	root, err := requireDirectory(rootPath)
	if err != nil {
		return manifestFile{}, err
	}
	pairs, err := discoverPairs(root)
	if err != nil {
		return manifestFile{}, err
	}
	keys := sortedCaseKeys(pairs)
	if len(keys) == 0 {
		return manifestFile{}, errors.New("quality sample root contains no paired direct/sandbox report cases")
	}
	if len(keys) > 100 {
		return manifestFile{}, fmt.Errorf("quality sample root contains %d paired cases, max 100", len(keys))
	}
	if err := requireScenarioCoverage(keys); err != nil {
		return manifestFile{}, err
	}
	if err := requireUniqueCaseIDs(keys); err != nil {
		return manifestFile{}, err
	}
	manifest := manifestFile{
		SampleBasis: sampleBasis,
		Cases:       make([]manifestCase, 0, len(keys)),
	}
	for _, key := range keys {
		pair := pairs[key]
		direct, err := parseSubReportFile(pair.DirectPath)
		if err != nil {
			return manifestFile{}, fmt.Errorf("case %q direct subreport: %w", key.ID, err)
		}
		sandbox, err := parseSubReportFile(pair.SandboxPath)
		if err != nil {
			return manifestFile{}, fmt.Errorf("case %q sandbox subreport: %w", key.ID, err)
		}
		refs, err := selectRequiredEvidenceRefs(key.ID, direct, sandbox)
		if err != nil {
			return manifestFile{}, err
		}
		manifest.Cases = append(manifest.Cases, manifestCase{
			ID:                   key.ID,
			Scenario:             key.Scenario,
			RequiredEvidenceRefs: refs,
			DirectSubReport:      pair.DirectManifest,
			SandboxSubReport:     pair.SandboxManifest,
		})
	}
	return manifest, nil
}

func discoverPairs(root string) (map[caseKey]reportPair, error) {
	pairs := map[caseKey]reportPair{}
	for _, role := range []string{directRole, sandboxRole} {
		if err := scanRole(root, role, pairs); err != nil {
			return nil, err
		}
	}
	for key, pair := range pairs {
		if pair.DirectPath == "" {
			return nil, fmt.Errorf("case %q scenario %q is missing direct report", key.ID, key.Scenario)
		}
		if pair.SandboxPath == "" {
			return nil, fmt.Errorf("case %q scenario %q is missing sandbox report", key.ID, key.Scenario)
		}
	}
	return pairs, nil
}

func scanRole(root, role string, pairs map[caseKey]reportPair) error {
	base, err := requireDirectory(filepath.Join(root, role))
	if err != nil {
		return err
	}
	return filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s must not contain symlinks: %s", role, filepath.Clean(path))
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return fmt.Errorf("resolve %s path: %w", role, err)
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if entry.IsDir() {
			if len(parts) != 1 {
				return fmt.Errorf("%s contains nested directory %q; expected %s/<scenario>/<case>.json", role, filepath.ToSlash(rel), role)
			}
			if !reportprompt.Scenario(parts[0]).Valid() {
				return fmt.Errorf("%s scenario directory %q is unsupported", role, parts[0])
			}
			return nil
		}
		if len(parts) != 2 {
			return fmt.Errorf("%s file %q must be under %s/<scenario>/<case>.json", role, filepath.ToSlash(rel), role)
		}
		scenario := parts[0]
		if !reportprompt.Scenario(scenario).Valid() {
			return fmt.Errorf("%s scenario directory %q is unsupported", role, scenario)
		}
		filename := parts[1]
		if filepath.Ext(filename) != ".json" {
			return fmt.Errorf("%s file %q must end with .json", role, filepath.ToSlash(rel))
		}
		if err := requireRegularFile(path); err != nil {
			return err
		}
		caseID, err := validateManifestCaseID(filename[:len(filename)-len(".json")])
		if err != nil {
			return err
		}
		manifestPath, err := manifestRelativePath(root, path)
		if err != nil {
			return err
		}
		key := caseKey{Scenario: scenario, ID: caseID}
		pair := pairs[key]
		switch role {
		case directRole:
			if pair.DirectPath != "" {
				return fmt.Errorf("duplicate direct report for case %q scenario %q", key.ID, key.Scenario)
			}
			pair.DirectPath = filepath.Clean(path)
			pair.DirectManifest = manifestPath
		case sandboxRole:
			if pair.SandboxPath != "" {
				return fmt.Errorf("duplicate sandbox report for case %q scenario %q", key.ID, key.Scenario)
			}
			pair.SandboxPath = filepath.Clean(path)
			pair.SandboxManifest = manifestPath
		default:
			return fmt.Errorf("unsupported report role %q", role)
		}
		pairs[key] = pair
		return nil
	})
}

func sortedCaseKeys(pairs map[caseKey]reportPair) []caseKey {
	keys := make([]caseKey, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	scenarioRank := map[string]int{}
	for i, scenario := range requiredManifestScenarios {
		scenarioRank[string(scenario)] = i
	}
	sort.Slice(keys, func(i, j int) bool {
		left := keys[i]
		right := keys[j]
		if scenarioRank[left.Scenario] != scenarioRank[right.Scenario] {
			return scenarioRank[left.Scenario] < scenarioRank[right.Scenario]
		}
		return left.ID < right.ID
	})
	return keys
}

func requireScenarioCoverage(keys []caseKey) error {
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key.Scenario] = true
	}
	var missing []string
	for _, scenario := range requiredManifestScenarios {
		value := string(scenario)
		if !seen[value] {
			missing = append(missing, value)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("quality sample root is missing paired report cases for required scenario(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func requireUniqueCaseIDs(keys []caseKey) error {
	seen := map[string]string{}
	for _, key := range keys {
		if previousScenario, exists := seen[key.ID]; exists {
			return fmt.Errorf("quality sample case id %q is used by both scenario %q and scenario %q; case ids must be globally unique for downstream review evidence", key.ID, previousScenario, key.Scenario)
		}
		seen[key.ID] = key.Scenario
	}
	return nil
}

func selectRequiredEvidenceRefs(caseID string, direct, sandbox reportdraft.SubReport) ([]string, error) {
	directRefs := evidenceRefSet(direct)
	sandboxRefs := evidenceRefSet(sandbox)
	var snapshots []string
	var others []string
	for ref := range directRefs {
		if _, ok := sandboxRefs[ref]; !ok {
			continue
		}
		if strings.HasPrefix(ref, requiredSnapshotRefPrefix) {
			if validManifestSnapshotRef(ref) {
				snapshots = append(snapshots, ref)
			}
			continue
		}
		if err := validateRequiredEvidenceRefValue(caseID, ref); err != nil {
			continue
		}
		others = append(others, ref)
	}
	sort.Strings(snapshots)
	sort.Strings(others)
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("case %q direct and sandbox reports must share at least one snapshot:<positive-id> evidence ref", caseID)
	}
	if len(snapshots) > maxManifestRequiredRefs {
		return nil, fmt.Errorf("case %q has %d shared snapshot refs, max required refs %d", caseID, len(snapshots), maxManifestRequiredRefs)
	}
	refs := append([]string(nil), snapshots...)
	for _, ref := range others {
		if len(refs) == maxManifestRequiredRefs {
			break
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func parseSubReportFile(path string) (reportdraft.SubReport, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return reportdraft.SubReport{}, err
	}
	// #nosec G304 -- this offline manifest helper intentionally opens
	// operator-supplied retained report JSON files.
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
		Model:        "offline-quality-manifest-prepare",
	})
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

func validateManifestCaseID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("manifest case id is required")
	}
	if value != raw {
		return "", fmt.Errorf("manifest case id %q must not contain leading or trailing whitespace", raw)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", errors.New("manifest case id must be a single-line value")
	}
	if strings.ContainsAny(raw, `/\:`) {
		return "", fmt.Errorf("manifest case id %q must not contain path separators or drive syntax", raw)
	}
	if len(raw) > maxManifestCaseIDBytes {
		return "", fmt.Errorf("manifest case id exceeds %d bytes", maxManifestCaseIDBytes)
	}
	return value, nil
}

func validateRequiredEvidenceRefValue(caseID, ref string) error {
	value := strings.TrimSpace(ref)
	if value == "" {
		return fmt.Errorf("case %q evidence ref is required", caseID)
	}
	if value != ref {
		return fmt.Errorf("case %q evidence ref must not contain leading or trailing whitespace", caseID)
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("case %q evidence ref must be a single-line value", caseID)
	}
	if len([]rune(value)) > maxManifestRequiredRefRunes {
		return fmt.Errorf("case %q evidence ref exceeds %d runes", caseID, maxManifestRequiredRefRunes)
	}
	return nil
}

func validManifestSnapshotRef(ref string) bool {
	rawID := strings.TrimPrefix(ref, requiredSnapshotRefPrefix)
	id, err := strconv.ParseInt(rawID, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == rawID
}

func manifestRelativePath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("make manifest path: %w", err)
	}
	ref := filepath.ToSlash(rel)
	if err := validateManifestRelativePath(ref); err != nil {
		return "", err
	}
	return ref, nil
}

func validateManifestRelativePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("manifest report path is required")
	}
	if strings.TrimSpace(path) != path {
		return fmt.Errorf("manifest report path %q must not contain leading or trailing whitespace", path)
	}
	if strings.ContainsAny(path, "\r\n\t") {
		return fmt.Errorf("manifest report path %q must be a single-line slash-separated relative path", path)
	}
	if len(path) > maxManifestReportPathBytes {
		return fmt.Errorf("manifest report path %q exceeds %d bytes", path, maxManifestReportPathBytes)
	}
	if strings.ContainsAny(path, "\\:") {
		return fmt.Errorf("manifest report path %q must be a slash-separated relative path", path)
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("manifest report path %q must be relative", path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return fmt.Errorf("manifest report path %q must not contain parent directory traversal", path)
		}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean != path || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("manifest report path %q must be normalized under the sample root", path)
	}
	return nil
}

func requireDirectory(path string) (string, error) {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s must be a directory, not a symlink", clean)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s must be a directory", clean)
	}
	return clean, nil
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

func writeManifestFile(path string, manifest manifestFile) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("output path %s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output path %s: %w", path, err)
	}
	parent := filepath.Dir(path)
	if _, err := requireDirectory(parent); err != nil {
		return fmt.Errorf("output parent: %w", err)
	}
	// #nosec G304 -- this manual helper writes an operator-selected artifact.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer f.Close()
	if err := encodeManifest(f, manifest); err != nil {
		return err
	}
	return nil
}

func encodeManifest(w io.Writer, manifest manifestFile) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(manifest)
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
