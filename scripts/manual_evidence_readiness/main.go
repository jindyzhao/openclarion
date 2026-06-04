// Command manual_evidence_readiness reports whether remaining manual evidence
// targets have their local prerequisites configured. It never prints
// environment values, so operators can share the output without leaking tokens
// or service URLs.
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
	"strings"
)

const (
	toolName                       = "manual_evidence_readiness"
	maxReadinessSampleBasisBytes   = 2048
	maxReadinessQualityCaseIDBytes = 128
	directRole                     = "direct"
	sandboxRole                    = "sandbox"
)

var requiredQualitySampleScenarios = []string{
	"single_alert",
	"cascade",
	"alert_storm",
}

type readinessOutput struct {
	Tool    string            `json:"tool"`
	Status  string            `json:"status"`
	Summary readinessSummary  `json:"summary"`
	Targets []targetReadiness `json:"targets"`
}

type readinessSummary struct {
	ReadyCount   int `json:"ready_count"`
	BlockedCount int `json:"blocked_count"`
}

type targetReadiness struct {
	Name                    string               `json:"name"`
	Status                  string               `json:"status"`
	Command                 string               `json:"command"`
	MissingEnv              []string             `json:"missing_env,omitempty"`
	UnsatisfiedAlternatives []envAlternative     `json:"unsatisfied_alternatives,omitempty"`
	AlternateCommands       []alternateCommand   `json:"alternate_commands,omitempty"`
	InvalidEnv              []invalidEnv         `json:"invalid_env,omitempty"`
	FileChecks              []fileCheck          `json:"file_checks,omitempty"`
	DirectoryChecks         []directoryCheck     `json:"directory_checks,omitempty"`
	OptionalDirectoryChecks []directoryCheck     `json:"optional_directory_checks,omitempty"`
	QualitySampleChecks     []qualitySampleCheck `json:"quality_sample_checks,omitempty"`
	Notes                   []string             `json:"notes,omitempty"`
}

type envAlternative struct {
	Description string   `json:"description"`
	Options     []string `json:"options"`
}

type alternateCommand struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

type invalidEnv struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type fileCheck struct {
	Env    string `json:"env"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type directoryCheck struct {
	Env    string `json:"env"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type qualitySampleCheck struct {
	Env                   string   `json:"env"`
	Status                string   `json:"status"`
	Reason                string   `json:"reason,omitempty"`
	DirectReports         int      `json:"direct_reports,omitempty"`
	SandboxReports        int      `json:"sandbox_reports,omitempty"`
	PairedCases           int      `json:"paired_cases,omitempty"`
	MissingDirectReports  int      `json:"missing_direct_reports,omitempty"`
	MissingSandboxReports int      `json:"missing_sandbox_reports,omitempty"`
	MissingScenarios      []string `json:"missing_scenarios,omitempty"`
}

type envMap map[string]string

func main() {
	if err := run(os.Args[1:], os.Environ(), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[manual-evidence-readiness] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, environ []string, stdout io.Writer) error {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	target := fs.String("target", "all", "target to check: all, report-live-smoke, sandbox-m4-quality-manifest-prepare, sandbox-m4-runtime-smoke-artifacts, sandbox-m4-review-evidence-template, sandbox-m4-decision, sandbox-m4-evidence-packet, diagnosis-live-browser-smoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	env := environMap(environ)
	targets := []targetReadiness{
		reportLiveSmokeReadiness(env),
		sandboxM4QualityManifestPrepareReadiness(env),
		sandboxM4RuntimeSmokeArtifactsReadiness(env),
		sandboxM4ReviewEvidenceTemplateReadiness(env),
		sandboxM4DecisionReadiness(env),
		sandboxM4EvidencePacketReadiness(env),
		diagnosisLiveBrowserSmokeReadiness(env),
	}
	selected, err := selectTargets(targets, strings.TrimSpace(*target))
	if err != nil {
		return err
	}
	out := readinessOutput{
		Tool:    toolName,
		Status:  "ready",
		Targets: selected,
	}
	for _, target := range selected {
		if target.Status == "ready" {
			out.Summary.ReadyCount++
		} else {
			out.Summary.BlockedCount++
			out.Status = "blocked"
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func sandboxM4QualityManifestPrepareReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-quality-manifest-prepare",
		Command: "make sandbox-m4-quality-manifest-prepare ROOT=... SAMPLE_BASIS=... OUT=...",
		Notes: []string{
			"Preflight scans retained direct/sandbox SubReport sample layout only; the manifest helper still parses both reports through the production SubReport parser.",
			"Sample readiness requires paired cases across single_alert, cascade, and alert_storm before the real quality comparison can support an M4 decision.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"ROOT",
		"SAMPLE_BASIS",
		"OUT",
	)
	if envPresent(env, "SAMPLE_BASIS") {
		if err := validateReadinessSampleBasis(env["SAMPLE_BASIS"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "SAMPLE_BASIS",
				Reason: err.Error(),
			})
		}
	}
	target.FileChecks = append(target.FileChecks, requiredAbsentOutputFileEnv(env, "OUT"))
	target.QualitySampleChecks = append(target.QualitySampleChecks, qualitySampleRootEnv(env, "ROOT"))
	return finalize(target)
}

func selectTargets(targets []targetReadiness, target string) ([]targetReadiness, error) {
	if target == "" || target == "all" {
		return targets, nil
	}
	for _, candidate := range targets {
		if candidate.Name == target {
			return []targetReadiness{candidate}, nil
		}
	}
	var names []string
	for _, candidate := range targets {
		names = append(names, candidate.Name)
	}
	sort.Strings(names)
	return nil, fmt.Errorf("unknown target %q; expected all or one of: %s", target, strings.Join(names, ", "))
}

func reportLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "report-live-smoke",
		Command: "make report-live-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not connect to PostgreSQL, Temporal, Prometheus, LLM, or Webhook services.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"DATABASE_URL",
		"TEMPORAL_HOST_PORT",
		"OPENCLARION_PROMETHEUS_URL",
		"REPORT_WINDOW_START",
		"REPORT_WINDOW_END",
	)
	if !allPresent(env, "OPENCLARION_LLM_MODEL", "OPENCLARION_IM_WEBHOOK_URL") && !envEquals(env, "REPORT_LIVE_SMOKE_ASSUME_WORKER_READY", "1") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "worker provider configuration",
			Options: []string{
				"OPENCLARION_LLM_MODEL + OPENCLARION_IM_WEBHOOK_URL",
				"REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1",
			},
		})
	}
	return finalize(target)
}

func sandboxM4RuntimeSmokeArtifactsReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-runtime-smoke-artifacts",
		Command: "make sandbox-m4-runtime-smoke-artifacts OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=... OPENCLARION_AGENT_RUNTIME_IMAGE=...",
		Notes: []string{
			"Preflight checks only local configuration; the target still runs Docker-backed runtime, provider, and egress smokes.",
			"Provider timeout, output-cap, and egress proofs use their existing smoke harness images unless explicitly overridden.",
			"The primary command still requires OPENCLARION_AGENT_RUNTIME_IMAGE; for the local custom thin runner candidate, the alternate command can resolve the digest-pinned image and retain the same artifact set while its ephemeral registry is alive.",
		},
		AlternateCommands: []alternateCommand{
			{
				Description: "Build the local custom thin runner candidate and retain the same runtime-smoke artifact set.",
				Command:     "make custom-thin-runner-smoke OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=...",
			},
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_AGENT_RUNTIME_IMAGE",
	)
	if envPresent(env, "OPENCLARION_AGENT_RUNTIME_IMAGE") && !immutableImageReference(env["OPENCLARION_AGENT_RUNTIME_IMAGE"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_AGENT_RUNTIME_IMAGE",
			Reason: "must be an immutable image reference name@sha256:<64-lowercase-hex-digest>",
		})
	}
	if envPresent(env, "OPENCLARION_M4_RUNTIME_SMOKE_PULL") && !oneOf(env["OPENCLARION_M4_RUNTIME_SMOKE_PULL"], "always", "missing", "never") {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_M4_RUNTIME_SMOKE_PULL",
			Reason: "must be one of always, missing, never when set",
		})
	}
	target.DirectoryChecks = append(target.DirectoryChecks, requiredEmptyOutputDirEnv(env, "OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR"))
	if envPresent(env, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR") {
		target.OptionalDirectoryChecks = append(target.OptionalDirectoryChecks, requiredEmptyOutputDirEnv(env, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR"))
	}
	return finalize(target)
}

func sandboxM4ReviewEvidenceTemplateReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-review-evidence-template",
		Command: "make sandbox-m4-review-evidence-template QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE[_FILE]=... REVIEWER=...",
		Notes: []string{
			"Preflight validates local draft-generation inputs only; generated review evidence remains fail-closed until operator review.",
			"RUNTIME_CANDIDATE_FILE may be used instead of RUNTIME_CANDIDATE when the runtime-smoke artifact directory contains a retained digest-ref.txt file.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"SELECTED_CANDIDATE",
		"REVIEWER",
	)
	runtimeCandidateSet := envPresent(env, "RUNTIME_CANDIDATE")
	runtimeCandidateFileSet := envPresent(env, "RUNTIME_CANDIDATE_FILE")
	if !runtimeCandidateSet && !runtimeCandidateFileSet {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "runtime candidate source",
			Options: []string{
				"RUNTIME_CANDIDATE",
				"RUNTIME_CANDIDATE_FILE",
			},
		})
	}
	if runtimeCandidateSet && runtimeCandidateFileSet {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "RUNTIME_CANDIDATE_FILE",
			Reason: "set exactly one of RUNTIME_CANDIDATE or RUNTIME_CANDIDATE_FILE",
		})
	}
	if envPresent(env, "RUNTIME_CANDIDATE") && !immutableImageReference(env["RUNTIME_CANDIDATE"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "RUNTIME_CANDIDATE",
			Reason: "must be an immutable image reference name@sha256:<64-lowercase-hex-digest>",
		})
	}
	target.FileChecks = append(target.FileChecks, requiredRegularFileEnv(env, "QUALITY_COMPARISON"))
	if runtimeCandidateFileSet {
		check := requiredRegularFileEnv(env, "RUNTIME_CANDIDATE_FILE")
		target.FileChecks = append(target.FileChecks, check)
		if check.Status == "ok" {
			if !runtimeCandidateFileReference(env["RUNTIME_CANDIDATE_FILE"]) {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   "RUNTIME_CANDIDATE_FILE",
					Reason: "file must contain exactly one immutable image reference name@sha256:<64-lowercase-hex-digest> followed by an optional newline",
				})
			}
		}
	}
	target.DirectoryChecks = append(target.DirectoryChecks, requiredDirectoryEnv(env, "RUNTIME_SMOKE_ARTIFACTS_ROOT"))
	return finalize(target)
}

func sandboxM4DecisionReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-decision",
		Command: "make sandbox-m4-decision BASELINE_AUDIT=... QUALITY_COMPARISON=... REVIEW_EVIDENCE=...",
		Notes: []string{
			"Preflight validates evidence file presence only; the decision helper still performs strict schema and consistency validation.",
		},
	}
	target.FileChecks = append(target.FileChecks,
		requiredRegularFileEnv(env, "BASELINE_AUDIT"),
		requiredRegularFileEnv(env, "QUALITY_COMPARISON"),
		requiredRegularFileEnv(env, "REVIEW_EVIDENCE"),
	)
	return finalize(target)
}

func sandboxM4EvidencePacketReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-evidence-packet",
		Command: "make sandbox-m4-evidence-packet QUALITY_MANIFEST=... REVIEW_EVIDENCE=... OUT_DIR=...",
		Notes: []string{
			"OUT_DIR may be absent or an existing empty directory; packet assembly still refuses to mix artifacts in a non-empty directory.",
		},
	}
	target.FileChecks = append(target.FileChecks,
		requiredRegularFileEnv(env, "QUALITY_MANIFEST"),
		requiredRegularFileEnv(env, "REVIEW_EVIDENCE"),
	)
	target.DirectoryChecks = append(target.DirectoryChecks, requiredEmptyOutputDirEnv(env, "OUT_DIR"))
	if envPresent(env, "RUNTIME_SMOKE_ARTIFACTS_ROOT") {
		target.OptionalDirectoryChecks = append(target.OptionalDirectoryChecks, optionalDirectoryEnv(env, "RUNTIME_SMOKE_ARTIFACTS_ROOT"))
	}
	return finalize(target)
}

func diagnosisLiveBrowserSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "diagnosis-live-browser-smoke",
		Command: "make diagnosis-live-browser-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not authenticate, create a room, install browsers, or contact the live backend.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
		"OPENCLARION_LIVE_BEARER_TOKEN",
	)
	if !envPresent(env, "OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID") && !envPresent(env, "OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "diagnosis room input",
			Options: []string{
				"OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID",
				"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID",
			},
		})
	}
	if envPresent(env, "OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID") && !positiveInteger(env["OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID",
			Reason: "must be a positive integer when set",
		})
	}
	return finalize(target)
}

func finalize(target targetReadiness) targetReadiness {
	if len(target.MissingEnv) == 0 &&
		len(target.UnsatisfiedAlternatives) == 0 &&
		len(target.InvalidEnv) == 0 &&
		fileChecksReady(target.FileChecks) &&
		directoryChecksReady(target.DirectoryChecks) &&
		directoryChecksReady(target.OptionalDirectoryChecks) &&
		qualitySampleChecksReady(target.QualitySampleChecks) {
		target.Status = "ready"
	} else {
		target.Status = "blocked"
	}
	return target
}

func fileChecksReady(checks []fileCheck) bool {
	for _, check := range checks {
		if check.Status != "ok" {
			return false
		}
	}
	return true
}

func directoryChecksReady(checks []directoryCheck) bool {
	for _, check := range checks {
		if check.Status != "ok" {
			return false
		}
	}
	return true
}

func qualitySampleChecksReady(checks []qualitySampleCheck) bool {
	for _, check := range checks {
		if check.Status != "ok" {
			return false
		}
	}
	return true
}

func missingEnv(env envMap, names ...string) []string {
	var missing []string
	for _, name := range names {
		if !envPresent(env, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func requiredRegularFileEnv(env envMap, name string) fileCheck {
	if !envPresent(env, name) {
		return fileCheck{Env: name, Status: "missing", Reason: "environment variable is unset or empty"}
	}
	clean := filepath.Clean(strings.TrimSpace(env[name]))
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return fileCheck{Env: name, Status: "missing_file", Reason: "path does not exist"}
	}
	if err != nil {
		return fileCheck{Env: name, Status: "error", Reason: "path cannot be inspected"}
	}
	if !info.Mode().IsRegular() {
		return fileCheck{Env: name, Status: "not_regular", Reason: "path must be a direct regular file"}
	}
	return fileCheck{Env: name, Status: "ok"}
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("path must be a direct regular file")
	}
	return nil
}

func requiredAbsentOutputFileEnv(env envMap, name string) fileCheck {
	if !envPresent(env, name) {
		return fileCheck{Env: name, Status: "missing", Reason: "environment variable is unset or empty"}
	}
	clean := filepath.Clean(strings.TrimSpace(env[name]))
	if clean == "." || clean == string(filepath.Separator) {
		return fileCheck{Env: name, Status: "invalid", Reason: "output file must not be repository root, current directory, or filesystem root"}
	}
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode().IsRegular() {
			return fileCheck{Env: name, Status: "exists", Reason: "output file must not exist before helper output is written"}
		}
		return fileCheck{Env: name, Status: "not_regular", Reason: "output path must be absent before helper output is written"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fileCheck{Env: name, Status: "error", Reason: "output path cannot be inspected"}
	}
	parent := filepath.Dir(clean)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return fileCheck{Env: name, Status: "parent_missing", Reason: "output parent directory does not exist"}
	}
	if err != nil {
		return fileCheck{Env: name, Status: "error", Reason: "output parent directory cannot be inspected"}
	}
	if !info.IsDir() {
		return fileCheck{Env: name, Status: "parent_not_directory", Reason: "output parent path must be a direct directory"}
	}
	return fileCheck{Env: name, Status: "ok", Reason: "output file will be created by the helper"}
}

func requiredEmptyOutputDirEnv(env envMap, name string) directoryCheck {
	if !envPresent(env, name) {
		return directoryCheck{Env: name, Status: "missing", Reason: "environment variable is unset or empty"}
	}
	clean := filepath.Clean(strings.TrimSpace(env[name]))
	if clean == "." || clean == string(filepath.Separator) {
		return directoryCheck{Env: name, Status: "invalid", Reason: "output directory must not be repository root, current directory, or filesystem root"}
	}
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return directoryCheck{Env: name, Status: "ok", Reason: "directory will be created by the helper"}
	}
	if err != nil {
		return directoryCheck{Env: name, Status: "error", Reason: "directory cannot be inspected"}
	}
	if !info.IsDir() {
		return directoryCheck{Env: name, Status: "not_directory", Reason: "path must be an empty directory or absent"}
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return directoryCheck{Env: name, Status: "error", Reason: "directory cannot be read"}
	}
	if len(entries) > 0 {
		return directoryCheck{Env: name, Status: "not_empty", Reason: "directory must be empty before helper output is written"}
	}
	return directoryCheck{Env: name, Status: "ok"}
}

func optionalDirectoryEnv(env envMap, name string) directoryCheck {
	if !envPresent(env, name) {
		return directoryCheck{Env: name, Status: "ok"}
	}
	clean := filepath.Clean(strings.TrimSpace(env[name]))
	info, err := os.Lstat(clean)
	if errors.Is(err, os.ErrNotExist) {
		return directoryCheck{Env: name, Status: "missing_directory", Reason: "path does not exist"}
	}
	if err != nil {
		return directoryCheck{Env: name, Status: "error", Reason: "directory cannot be inspected"}
	}
	if !info.IsDir() {
		return directoryCheck{Env: name, Status: "not_directory", Reason: "path must be a direct directory"}
	}
	return directoryCheck{Env: name, Status: "ok"}
}

func requiredDirectoryEnv(env envMap, name string) directoryCheck {
	if !envPresent(env, name) {
		return directoryCheck{Env: name, Status: "missing", Reason: "environment variable is unset or empty"}
	}
	return optionalDirectoryEnv(env, name)
}

type qualitySampleKey struct {
	Scenario string
	ID       string
}

type qualitySamplePair struct {
	Direct  bool
	Sandbox bool
}

func qualitySampleRootEnv(env envMap, name string) qualitySampleCheck {
	check := qualitySampleCheck{Env: name}
	if !envPresent(env, name) {
		check.Status = "missing"
		check.Reason = "environment variable is unset or empty"
		return check
	}
	root := filepath.Clean(strings.TrimSpace(env[name]))
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		check.Status = "missing_directory"
		check.Reason = "path does not exist"
		return check
	}
	if err != nil {
		check.Status = "error"
		check.Reason = "path cannot be inspected"
		return check
	}
	if !info.IsDir() {
		check.Status = "not_directory"
		check.Reason = "path must be a direct directory"
		return check
	}
	pairs := map[qualitySampleKey]qualitySamplePair{}
	for _, role := range []string{directRole, sandboxRole} {
		if err := scanQualitySampleRole(root, role, pairs, &check); err != nil {
			check.Status = "invalid_tree"
			check.Reason = err.Error()
			return check
		}
	}
	covered := map[string]bool{}
	for _, pair := range pairs {
		if pair.Direct && pair.Sandbox {
			check.PairedCases++
			continue
		}
		if pair.Direct {
			check.MissingSandboxReports++
		}
		if pair.Sandbox {
			check.MissingDirectReports++
		}
	}
	for key, pair := range pairs {
		if pair.Direct && pair.Sandbox {
			covered[key.Scenario] = true
		}
	}
	check.MissingScenarios = missingQualitySampleScenarios(covered)
	switch {
	case check.DirectReports == 0 || check.SandboxReports == 0:
		check.Status = "missing_reports"
		check.Reason = "sample root must contain direct and sandbox report files"
	case check.MissingDirectReports != 0 || check.MissingSandboxReports != 0:
		check.Status = "missing_counterparts"
		check.Reason = "every retained direct report must have a sandbox counterpart and every sandbox report must have a direct counterpart"
	case check.PairedCases == 0:
		check.Status = "missing_pairs"
		check.Reason = "sample root contains no paired direct/sandbox report cases"
	case len(check.MissingScenarios) != 0:
		check.Status = "missing_scenario_coverage"
		check.Reason = "paired report cases must cover single_alert, cascade, and alert_storm"
	default:
		check.Status = "ok"
	}
	return check
}

func scanQualitySampleRole(root, role string, pairs map[qualitySampleKey]qualitySamplePair, check *qualitySampleCheck) error {
	base := filepath.Join(root, role)
	info, err := os.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s report directory is missing", role)
	}
	if err != nil {
		return fmt.Errorf("%s report directory cannot be inspected", role)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s report path must be a direct directory", role)
	}
	return filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == base {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("sample root must not contain symlinks")
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return fmt.Errorf("sample path cannot be resolved")
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if entry.IsDir() {
			if len(parts) != 1 {
				return fmt.Errorf("%s reports must use <scenario>/<case>.json layout", role)
			}
			if !qualitySampleScenario(parts[0]) {
				return fmt.Errorf("%s reports contain an unsupported scenario directory", role)
			}
			return nil
		}
		if len(parts) != 2 {
			return fmt.Errorf("%s reports must use <scenario>/<case>.json layout", role)
		}
		scenario := parts[0]
		if !qualitySampleScenario(scenario) {
			return fmt.Errorf("%s reports contain an unsupported scenario directory", role)
		}
		filename := parts[1]
		if filepath.Ext(filename) != ".json" {
			return fmt.Errorf("%s reports must be JSON files", role)
		}
		if err := requireRegularFile(filepath.Clean(path)); err != nil {
			return fmt.Errorf("%s reports must be direct regular files", role)
		}
		id := filename[:len(filename)-len(".json")]
		if !qualitySampleCaseID(id) {
			return fmt.Errorf("%s reports contain an invalid case id", role)
		}
		key := qualitySampleKey{Scenario: scenario, ID: id}
		pair := pairs[key]
		switch role {
		case directRole:
			check.DirectReports++
			pair.Direct = true
		case sandboxRole:
			check.SandboxReports++
			pair.Sandbox = true
		}
		pairs[key] = pair
		return nil
	})
}

func missingQualitySampleScenarios(covered map[string]bool) []string {
	var missing []string
	for _, scenario := range requiredQualitySampleScenarios {
		if !covered[scenario] {
			missing = append(missing, scenario)
		}
	}
	return missing
}

func environMap(environ []string) envMap {
	env := envMap{}
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[name] = value
	}
	return env
}

func envPresent(env envMap, name string) bool {
	value, ok := env[name]
	return ok && strings.TrimSpace(value) != ""
}

func allPresent(env envMap, names ...string) bool {
	for _, name := range names {
		if !envPresent(env, name) {
			return false
		}
	}
	return true
}

func envEquals(env envMap, name, want string) bool {
	if !envPresent(env, name) {
		return false
	}
	return strings.TrimSpace(env[name]) == want
}

func positiveInteger(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validateReadinessSampleBasis(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("is required")
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return errors.New("must be a single-line value")
	}
	if len(raw) > maxReadinessSampleBasisBytes {
		return fmt.Errorf("exceeds %d bytes", maxReadinessSampleBasisBytes)
	}
	return nil
}

func qualitySampleScenario(value string) bool {
	for _, scenario := range requiredQualitySampleScenarios {
		if value == scenario {
			return true
		}
	}
	return false
}

func qualitySampleCaseID(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw {
		return false
	}
	if strings.ContainsAny(raw, "\r\n\t/\\:") {
		return false
	}
	return len(raw) <= maxReadinessQualityCaseIDBytes
}

func immutableImageReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \r\n\t") {
		return false
	}
	name, digest, ok := strings.Cut(value, "@sha256:")
	if !ok || name == "" || digest == "" {
		return false
	}
	if len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func runtimeCandidateFileReference(filePath string) bool {
	clean := filepath.Clean(strings.TrimSpace(filePath))
	// #nosec G304 -- manual readiness inspects an operator-supplied digest-ref
	// file after requiredRegularFileEnv has accepted it as a direct file.
	f, err := os.Open(clean)
	if err != nil {
		return false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return false
	}
	value := strings.TrimSuffix(string(raw), "\n")
	if value == "" || strings.ContainsAny(value, " \r\n\t") {
		return false
	}
	return immutableImageReference(value)
}

func oneOf(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
