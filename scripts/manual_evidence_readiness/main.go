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
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const toolName = "manual_evidence_readiness"

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
	Name                    string             `json:"name"`
	Status                  string             `json:"status"`
	Command                 string             `json:"command"`
	MissingEnv              []string           `json:"missing_env,omitempty"`
	UnsatisfiedAlternatives []envAlternative   `json:"unsatisfied_alternatives,omitempty"`
	AlternateCommands       []alternateCommand `json:"alternate_commands,omitempty"`
	InvalidEnv              []invalidEnv       `json:"invalid_env,omitempty"`
	FileChecks              []fileCheck        `json:"file_checks,omitempty"`
	DirectoryChecks         []directoryCheck   `json:"directory_checks,omitempty"`
	OptionalDirectoryChecks []directoryCheck   `json:"optional_directory_checks,omitempty"`
	Notes                   []string           `json:"notes,omitempty"`
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
	target := fs.String("target", "all", "target to check: all, report-live-smoke, sandbox-m4-runtime-smoke-artifacts, sandbox-m4-review-evidence-template, sandbox-m4-decision, sandbox-m4-evidence-packet, diagnosis-live-browser-smoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	env := environMap(environ)
	targets := []targetReadiness{
		reportLiveSmokeReadiness(env),
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
		Command: "make sandbox-m4-review-evidence-template QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE=... REVIEWER=...",
		Notes: []string{
			"Preflight validates local draft-generation inputs only; generated review evidence remains fail-closed until operator review.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"SELECTED_CANDIDATE",
		"RUNTIME_CANDIDATE",
		"REVIEWER",
	)
	if envPresent(env, "RUNTIME_CANDIDATE") && !immutableImageReference(env["RUNTIME_CANDIDATE"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "RUNTIME_CANDIDATE",
			Reason: "must be an immutable image reference name@sha256:<64-lowercase-hex-digest>",
		})
	}
	target.FileChecks = append(target.FileChecks, requiredRegularFileEnv(env, "QUALITY_COMPARISON"))
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
		directoryChecksReady(target.OptionalDirectoryChecks) {
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

func oneOf(value string, allowed ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
