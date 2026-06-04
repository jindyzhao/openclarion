// Command manual_evidence_readiness reports whether remaining manual evidence
// targets have their local prerequisites configured. It never prints
// environment values, so operators can share the output without leaking tokens
// or service URLs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	toolName                       = "manual_evidence_readiness"
	maxReadinessArtifactBytes      = 8 * 1024 * 1024
	maxReadinessSampleBasisBytes   = 2048
	maxReadinessQualityCaseIDBytes = 128
	maxReadinessCloseReasonBytes   = 128
	maxReadinessReportIDBytes      = 256
	m4PacketVerificationTimeout    = 2 * time.Minute
	directRole                     = "direct"
	sandboxRole                    = "sandbox"
)

var requiredQualitySampleScenarios = []string{
	"single_alert",
	"cascade",
	"alert_storm",
}

var requiredM4EvidenceArtifacts = []m4EvidenceArtifact{
	{Name: "baseline_audit", Artifact: "baseline-audit.json", JSON: true},
	{Name: "runtime_candidate_digest_ref", Artifact: "runtime-smokes/digest-ref.txt", RuntimeCandidateRef: true},
	{Name: "candidate_runtime_file_contract", Artifact: "runtime-smokes/agent-runtime-smoke.json", JSON: true},
	{Name: "docker_provider_lifecycle", Artifact: "runtime-smokes/container-provider-smoke.json", JSON: true},
	{Name: "docker_provider_timeout_cleanup", Artifact: "runtime-smokes/container-provider-timeout-smoke.json", JSON: true},
	{Name: "docker_provider_output_cap", Artifact: "runtime-smokes/container-provider-output-cap-smoke.json", JSON: true},
	{Name: "egress_allowdeny", Artifact: "runtime-smokes/egress-allowdeny-smoke.json", JSON: true},
	{Name: "direct_quality_samples", Artifact: "direct", Directory: true},
	{Name: "sandbox_quality_samples", Artifact: "sandbox", Directory: true},
	{Name: "quality_manifest", Artifact: "quality-manifest.json", JSON: true},
	{Name: "quality_comparison", Artifact: "quality-comparison.json", JSON: true},
	{Name: "review_evidence", Artifact: "review-evidence.json", JSON: true},
	{Name: "packet_summary", Artifact: "packet.json", JSON: true},
}

var verifyM4EvidencePacket = verifyM4EvidencePacketWithGoRun

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
	EvidenceChainChecks     []evidenceChainCheck `json:"evidence_chain_checks,omitempty"`
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

type evidenceChainCheck struct {
	Name     string `json:"name"`
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

type m4EvidenceArtifact struct {
	Name                string
	Artifact            string
	JSON                bool
	Directory           bool
	RuntimeCandidateRef bool
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
	target := fs.String("target", "all", "target to check: all, report-live-smoke, sandbox-m4-baseline-audit, sandbox-m4-quality-sample-export, sandbox-m4-quality-manifest-prepare, sandbox-m4-quality-compare, sandbox-m4-runtime-smoke-artifacts, sandbox-m4-review-evidence-template, sandbox-m4-decision, sandbox-m4-evidence-packet, sandbox-m4-evidence-chain, diagnosis-live-browser-smoke")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	env := environMap(environ)
	targets := []targetReadiness{
		reportLiveSmokeReadiness(env),
		sandboxM4BaselineAuditReadiness(env),
		sandboxM4QualitySampleExportReadiness(env),
		sandboxM4QualityManifestPrepareReadiness(env),
		sandboxM4QualityCompareReadiness(env),
		sandboxM4RuntimeSmokeArtifactsReadiness(env),
		sandboxM4ReviewEvidenceTemplateReadiness(env),
		sandboxM4DecisionReadiness(env),
		sandboxM4EvidencePacketReadiness(env),
		sandboxM4EvidenceChainReadiness(env),
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

func sandboxM4BaselineAuditReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-baseline-audit",
		Command: "make sandbox-m4-baseline-audit OUT=...",
		Notes: []string{
			"Preflight validates only the retained output path; the audit helper still runs the same code-level sandbox baseline checks.",
			"The manual target writes a new retained baseline-audit JSON file for the M4 decision evidence chain.",
		},
	}
	target.FileChecks = append(target.FileChecks, requiredAbsentOutputFileEnv(env, "OUT"))
	return finalize(target)
}

func sandboxM4QualitySampleExportReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-quality-sample-export",
		Command: "DATABASE_URL=... make sandbox-m4-quality-sample-export SELECTION=... ROOT=...",
		Notes: []string{
			"Preflight checks only the local database URL presence, operator selection file, and empty sample-root output path.",
			"The manual target still validates persisted SubReport rows through the production SubReport parser before writing samples.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"DATABASE_URL",
		"SELECTION",
		"ROOT",
	)
	target.FileChecks = append(target.FileChecks, requiredRegularFileEnv(env, "SELECTION"))
	target.DirectoryChecks = append(target.DirectoryChecks, requiredCreatableEmptyOutputDirEnv(env, "ROOT"))
	return finalize(target)
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

func sandboxM4QualityCompareReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-quality-compare",
		Command: "make sandbox-m4-quality-compare QUALITY_MANIFEST=... OUT=...",
		Notes: []string{
			"Preflight validates the retained manifest and output path only; the comparison helper still parses every direct/sandbox SubReport through the production parser.",
			"The manual target runs manifest mode with fail-on-regression and writes a new retained quality-comparison JSON file.",
		},
	}
	target.FileChecks = append(target.FileChecks,
		requiredRegularFileEnv(env, "QUALITY_MANIFEST"),
		requiredAbsentOutputFileEnv(env, "OUT"),
	)
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
	if envPresent(env, "OPENCLARION_PROMETHEUS_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_PROMETHEUS_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_PROMETHEUS_URL",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") {
		if err := validateReadinessWebhookURL(env["OPENCLARION_IM_WEBHOOK_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_IM_WEBHOOK_URL",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_WINDOW_START") || envPresent(env, "REPORT_WINDOW_END") {
		if err := validateReadinessReportWindow(env["REPORT_WINDOW_START"], env["REPORT_WINDOW_END"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WINDOW_START/REPORT_WINDOW_END",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_REPLAY_LIMIT") {
		if err := validateReadinessPositiveInteger(env["REPORT_REPLAY_LIMIT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_REPLAY_LIMIT",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_SCENARIO") {
		if err := validateReadinessScenario(env["REPORT_SCENARIO"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_SCENARIO",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_WAIT_TIMEOUT") {
		if err := validateReadinessPositiveDuration(env["REPORT_WAIT_TIMEOUT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WAIT_TIMEOUT",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_CORRELATION_KEY") {
		if err := validateReadinessOptionalID(env["REPORT_CORRELATION_KEY"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_CORRELATION_KEY",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_WORKFLOW_ID") {
		if err := validateReadinessOptionalID(env["REPORT_WORKFLOW_ID"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WORKFLOW_ID",
				Reason: err.Error(),
			})
		}
	}
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

func sandboxM4EvidenceChainReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "sandbox-m4-evidence-chain",
		Command: "OPENCLARION_M4_EVIDENCE_ROOT=... make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=sandbox-m4-evidence-chain",
		Notes: []string{
			"Preflight checks a retained M4 evidence working directory for the canonical artifact chain without printing local paths.",
			"JSON artifacts are checked for duplicate object keys and trailing JSON before the packet verifier performs semantic validation when all canonical artifacts are present.",
			"The target reports presence, SHA-256 digests, and packet verification status only; it does not judge sample representativeness or accept a runtime baseline.",
		},
	}
	target.DirectoryChecks = append(target.DirectoryChecks, requiredDirectoryEnv(env, "OPENCLARION_M4_EVIDENCE_ROOT"))
	if directoryChecksReady(target.DirectoryChecks) {
		target.EvidenceChainChecks = m4EvidenceChainChecks(filepath.Clean(strings.TrimSpace(env["OPENCLARION_M4_EVIDENCE_ROOT"])))
	}
	return finalize(target)
}

func diagnosisLiveBrowserSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:    "diagnosis-live-browser-smoke",
		Command: "make diagnosis-live-browser-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not authenticate, create a room, install browsers, or contact the live backend.",
			"When close-notification proof is required, the local close CLI still signals Temporal and loads PostgreSQL lifecycle events while the running worker must be configured to send the IM notification.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
		"OPENCLARION_LIVE_BEARER_TOKEN",
	)
	if envPresent(env, "OPENCLARION_LIVE_API_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_API_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_API_BASE_URL",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_WEB_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_WEB_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_WEB_BASE_URL",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_BEARER_TOKEN") && !validReadinessBearerToken(env["OPENCLARION_LIVE_BEARER_TOKEN"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_BEARER_TOKEN",
			Reason: "must be a single bearer token or Bearer header without embedded whitespace",
		})
	}
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
	if closeNotificationProofRequired(env) {
		target.MissingEnv = append(target.MissingEnv, missingEnv(env,
			"DATABASE_URL",
			"TEMPORAL_HOST_PORT",
		)...)
		if !envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") && !envEquals(env, "DIAGNOSIS_LIVE_SMOKE_ASSUME_WORKER_READY", "1") {
			target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
				Description: "close-notification worker IM configuration",
				Options: []string{
					"OPENCLARION_IM_WEBHOOK_URL",
					"DIAGNOSIS_LIVE_SMOKE_ASSUME_WORKER_READY=1",
				},
			})
		}
		if envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") {
			if err := validateReadinessWebhookURL(env["OPENCLARION_IM_WEBHOOK_URL"]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   "OPENCLARION_IM_WEBHOOK_URL",
					Reason: err.Error(),
				})
			}
		}
		if envPresent(env, "OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT") {
			if _, err := time.ParseDuration(strings.TrimSpace(env["OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT"])); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   "OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT",
					Reason: "must be a valid Go duration such as 2m",
				})
			}
		}
		if envPresent(env, "OPENCLARION_LIVE_CLOSE_REASON") {
			if err := validateReadinessCloseReason(env["OPENCLARION_LIVE_CLOSE_REASON"]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   "OPENCLARION_LIVE_CLOSE_REASON",
					Reason: err.Error(),
				})
			}
		}
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
		qualitySampleChecksReady(target.QualitySampleChecks) &&
		evidenceChainChecksReady(target.EvidenceChainChecks) {
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

func evidenceChainChecksReady(checks []evidenceChainCheck) bool {
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

func requiredCreatableEmptyOutputDirEnv(env envMap, name string) directoryCheck {
	check := requiredEmptyOutputDirEnv(env, name)
	if check.Status != "ok" || !envPresent(env, name) {
		return check
	}
	clean := filepath.Clean(strings.TrimSpace(env[name]))
	if _, err := os.Lstat(clean); err == nil {
		return check
	} else if !errors.Is(err, os.ErrNotExist) {
		return directoryCheck{Env: name, Status: "error", Reason: "directory cannot be inspected"}
	}
	parent := filepath.Dir(clean)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return directoryCheck{Env: name, Status: "parent_missing", Reason: "output parent directory does not exist"}
	}
	if err != nil {
		return directoryCheck{Env: name, Status: "error", Reason: "output parent directory cannot be inspected"}
	}
	if !info.IsDir() {
		return directoryCheck{Env: name, Status: "parent_not_directory", Reason: "output parent path must be a direct directory"}
	}
	return check
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

func closeNotificationProofRequired(env envMap) bool {
	return envTruthy(env, "OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION") ||
		envTruthy(env, "DIAGNOSIS_LIVE_REQUIRE_CLOSE_NOTIFICATION")
}

func envTruthy(env envMap, name string) bool {
	if !envPresent(env, name) {
		return false
	}
	switch strings.TrimSpace(env[name]) {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
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

func validateReadinessPositiveInteger(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("must be non-empty")
	}
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(raw, " \r\n\t") {
		return errors.New("must not contain whitespace")
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return errors.New("must be a positive integer")
	}
	return nil
}

func validateReadinessPositiveDuration(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("must be non-empty")
	}
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return errors.New("must be a valid Go duration such as 20m")
	}
	if duration <= 0 {
		return errors.New("must be greater than zero")
	}
	return nil
}

func validateReadinessScenario(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("must be non-empty")
	}
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if !qualitySampleScenario(value) {
		return errors.New("must be one of single_alert, cascade, or alert_storm")
	}
	return nil
}

func validateReadinessOptionalID(raw string) error {
	if raw == "" {
		return nil
	}
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(raw, " \r\n\t") {
		return errors.New("must not contain whitespace")
	}
	if len(raw) > maxReadinessReportIDBytes {
		return fmt.Errorf("exceeds %d bytes", maxReadinessReportIDBytes)
	}
	return nil
}

func validateReadinessReportWindow(rawStart, rawEnd string) error {
	if strings.TrimSpace(rawStart) == "" || strings.TrimSpace(rawEnd) == "" {
		return nil
	}
	start, err := parseReadinessCanonicalTime(rawStart)
	if err != nil {
		return fmt.Errorf("window start %w", err)
	}
	end, err := parseReadinessCanonicalTime(rawEnd)
	if err != nil {
		return fmt.Errorf("window end %w", err)
	}
	if !end.After(start) {
		return errors.New("window end must be after window start")
	}
	if end.After(time.Now().UTC()) {
		return errors.New("window end must not be in the future")
	}
	return nil
}

func parseReadinessCanonicalTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, errors.New("must be non-empty")
	}
	if value != raw {
		return time.Time{}, errors.New("must not contain leading or trailing whitespace")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New("must be RFC3339")
	}
	if parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, errors.New("must be canonical UTC RFC3339")
	}
	return parsed.UTC(), nil
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

func validateReadinessCloseReason(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("must be non-empty")
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return errors.New("must be a single-line value")
	}
	if len(raw) > maxReadinessCloseReasonBytes {
		return fmt.Errorf("exceeds %d bytes", maxReadinessCloseReasonBytes)
	}
	return nil
}

func validateReadinessHTTPURL(raw string) error {
	parsed, err := parseReadinessURL(raw)
	if err != nil {
		return err
	}
	if parsed.RawQuery != "" {
		return errors.New("must not include a query string")
	}
	if parsed.Fragment != "" {
		return errors.New("must not include a fragment")
	}
	return nil
}

func validateReadinessWebhookURL(raw string) error {
	_, err := parseReadinessURL(raw)
	return err
}

func parseReadinessURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value != raw {
		return nil, errors.New("must not contain leading or trailing whitespace")
	}
	if value == "" {
		return nil, errors.New("must be non-empty")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, errors.New("must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("must not include user info")
	}
	return parsed, nil
}

func validReadinessBearerToken(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	return value != "" && !strings.ContainsAny(value, " \r\n\t")
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

func m4EvidenceChainChecks(root string) []evidenceChainCheck {
	checks := make([]evidenceChainCheck, 0, len(requiredM4EvidenceArtifacts))
	for _, artifact := range requiredM4EvidenceArtifacts {
		checks = append(checks, checkM4EvidenceArtifact(root, artifact))
	}
	if evidenceChainChecksReady(checks) {
		checks = append(checks, checkM4PacketSemanticVerification(root))
	}
	return checks
}

func checkM4PacketSemanticVerification(root string) evidenceChainCheck {
	check := evidenceChainCheck{
		Name:     "packet_semantic_verification",
		Artifact: "packet.json",
	}
	if err := verifyM4EvidencePacket(root); err != nil {
		check.Status = "invalid_packet"
		check.Reason = err.Error()
		return check
	}
	check.Status = "ok"
	return check
}

func verifyM4EvidencePacketWithGoRun(root string) error {
	repoRoot, err := findRepositoryRoot()
	if err != nil {
		return errors.New("packet verifier could not be launched")
	}
	ctx, cancel := context.WithTimeout(context.Background(), m4PacketVerificationTimeout)
	defer cancel()
	// #nosec G204 -- the executable and verifier arguments are fixed; root is
	// an operator-selected evidence directory and verifier output is discarded.
	cmd := exec.CommandContext(ctx, "go", "run", "./scripts/sandbox_m4_evidence_packet", "--verify-packet", root)
	cmd.Dir = repoRoot
	if _, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("packet verifier timed out")
		}
		return errors.New("packet verifier rejected retained packet")
	}
	return nil
}

func findRepositoryRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		modPath := filepath.Join(dir, "go.mod")
		// #nosec G304 -- repository discovery only reads go.mod while walking
		// parents from the current working directory.
		raw, err := os.ReadFile(modPath)
		if err == nil && strings.Contains(string(raw), "module github.com/openclarion/openclarion") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root not found")
		}
		dir = parent
	}
}

func checkM4EvidenceArtifact(root string, artifact m4EvidenceArtifact) evidenceChainCheck {
	check := evidenceChainCheck{
		Name:     artifact.Name,
		Artifact: artifact.Artifact,
	}
	artifactPath := filepath.Join(root, filepath.FromSlash(artifact.Artifact))
	info, err := os.Lstat(artifactPath)
	if errors.Is(err, os.ErrNotExist) {
		check.Status = "missing"
		check.Reason = "artifact is not present"
		return check
	}
	if err != nil {
		check.Status = "error"
		check.Reason = "artifact cannot be inspected"
		return check
	}
	if artifact.Directory {
		if !info.IsDir() {
			check.Status = "not_directory"
			check.Reason = "artifact must be a direct directory"
			return check
		}
		check.Status = "ok"
		return check
	}
	if !info.Mode().IsRegular() {
		check.Status = "not_regular"
		check.Reason = "artifact must be a direct regular file"
		return check
	}
	raw, err := readBoundedArtifact(artifactPath)
	if err != nil {
		check.Status = "error"
		check.Reason = "artifact cannot be read"
		return check
	}
	if len(raw) > maxReadinessArtifactBytes {
		check.Status = "too_large"
		check.Reason = "artifact exceeds the readiness read cap"
		return check
	}
	sum := sha256.Sum256(raw)
	check.SHA256 = hex.EncodeToString(sum[:])
	if artifact.JSON {
		if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
			check.Status = "invalid_json"
			check.Reason = "artifact must be duplicate-key-free JSON with no trailing values"
			check.SHA256 = ""
			return check
		}
	}
	if artifact.RuntimeCandidateRef && !runtimeCandidateBytesReference(raw) {
		check.Status = "invalid_runtime_candidate"
		check.Reason = "artifact must contain exactly one immutable image reference"
		check.SHA256 = ""
		return check
	}
	check.Status = "ok"
	return check
}

func readBoundedArtifact(path string) ([]byte, error) {
	// #nosec G304 -- manual readiness inspects operator-selected evidence paths
	// without printing them, then reports only status and artifact digests.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxReadinessArtifactBytes+1))
}

func runtimeCandidateBytesReference(raw []byte) bool {
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
