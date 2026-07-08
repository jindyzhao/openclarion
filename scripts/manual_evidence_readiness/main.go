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
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	gldap "github.com/go-ldap/ldap/v3"

	"github.com/openclarion/openclarion/internal/providers/secrets/envmap"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	toolName                       = "manual_evidence_readiness"
	maxReadinessArtifactBytes      = 8 * 1024 * 1024
	maxReadinessSampleBasisBytes   = 2048
	maxReadinessQualityCaseIDBytes = 128
	maxReadinessCloseReasonBytes   = 128
	maxReadinessSupplementalBytes  = 4096
	maxReadinessToolRequestsBytes  = 16 * 1024
	maxReadinessToolRequestItems   = 5
	maxReadinessToolReasonBytes    = 500
	maxReadinessToolQueryBytes     = 500
	minReadinessToolRangeSeconds   = 15
	maxReadinessToolRangeSeconds   = 6 * 60 * 60
	maxReadinessToolAlertLimit     = 10
	maxReadinessToolMetricLimit    = 20
	maxReadinessReportIDBytes      = 256
	m4PacketVerificationTimeout    = 2 * time.Minute
	directRole                     = "direct"
	sandboxRole                    = "sandbox"
	readinessWeComWebhookHost      = "qyapi.weixin.qq.com"
	readinessWeComWebhookPath      = "/cgi-bin/webhook/send"
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

var requiredM4PacketEvidenceArtifacts = []m4EvidenceArtifact{
	{Name: "baseline_audit", Artifact: "baseline-audit.json", JSON: true},
	{Name: "quality_comparison", Artifact: "quality-comparison.json", JSON: true},
	{Name: "review_evidence", Artifact: "review-evidence.json", JSON: true},
	{Name: "decision", Artifact: "decision.json", JSON: true},
	{Name: "packet_summary", Artifact: "packet.json", JSON: true},
	{Name: "quality_manifest", Artifact: "quality-inputs/quality-manifest.json", JSON: true},
	{Name: "quality_reports", Artifact: "quality-inputs/reports", Directory: true},
	{Name: "runtime_smoke_artifacts", Artifact: "runtime-smoke-artifacts", Directory: true},
}

var verifyM4EvidencePacket = verifyM4EvidencePacketWithGoRun

type readinessOutput struct {
	Tool    string            `json:"tool"`
	Status  string            `json:"status"`
	Summary readinessSummary  `json:"summary"`
	Targets []targetReadiness `json:"targets"`
}

type readinessSummary struct {
	ReadyCount   int             `json:"ready_count"`
	BlockedCount int             `json:"blocked_count"`
	NextTarget   *nextTargetHint `json:"next_target,omitempty"`
}

type nextTargetHint struct {
	Name         string `json:"name"`
	Milestone    string `json:"milestone,omitempty"`
	Command      string `json:"command"`
	EvidenceGoal string `json:"evidence_goal,omitempty"`
}

type targetReadiness struct {
	Name                    string               `json:"name"`
	Status                  string               `json:"status"`
	Milestone               string               `json:"milestone,omitempty"`
	Sequence                int                  `json:"sequence,omitempty"`
	EvidenceGoal            string               `json:"evidence_goal,omitempty"`
	DependsOn               []string             `json:"depends_on,omitempty"`
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

type targetListFlag struct {
	values []string
}

func (f *targetListFlag) Set(raw string) error {
	for _, part := range strings.Split(raw, ",") {
		f.values = append(f.values, strings.TrimSpace(part))
	}
	return nil
}

func (f *targetListFlag) String() string {
	if f == nil || len(f.values) == 0 {
		return "all"
	}
	return strings.Join(f.values, ",")
}

func (f *targetListFlag) Values() []string {
	if f == nil || len(f.values) == 0 {
		return []string{"all"}
	}
	return append([]string(nil), f.values...)
}

func main() {
	if err := run(os.Args[1:], os.Environ(), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[manual-evidence-readiness] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, environ []string, stdout io.Writer) error {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var targetsFlag targetListFlag
	fs.Var(&targetsFlag, "target", "target to check; may be repeated or comma-separated: all, alert-operations-live-inputs, notification-channel-live-smoke, report-live-smoke, report-policy-live-smoke, report-schedule-live-smoke, sandbox-m4-baseline-audit, sandbox-m4-quality-sample-export, sandbox-m4-quality-manifest-prepare, sandbox-m4-quality-compare, sandbox-m4-runtime-smoke-artifacts, sandbox-m4-review-evidence-template, sandbox-m4-decision, sandbox-m4-evidence-packet, sandbox-m4-evidence-chain, diagnosis-auth-live-smoke, diagnosis-live-browser-smoke, diagnosis-live-convergence-smoke, alertmanager-auto-diagnosis-live-smoke")
	requireReady := fs.Bool("require-ready", false, "return a non-zero exit status when any selected target is blocked")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	env := environMap(environ)
	targets := []targetReadiness{
		alertOperationsLiveInputsReadiness(env),
		notificationChannelLiveSmokeReadiness(env),
		reportLiveSmokeReadiness(env),
		reportPolicyLiveSmokeReadiness(env),
		reportScheduleLiveSmokeReadiness(env),
		sandboxM4BaselineAuditReadiness(env),
		sandboxM4QualitySampleExportReadiness(env),
		sandboxM4QualityManifestPrepareReadiness(env),
		sandboxM4QualityCompareReadiness(env),
		sandboxM4RuntimeSmokeArtifactsReadiness(env),
		sandboxM4ReviewEvidenceTemplateReadiness(env),
		sandboxM4DecisionReadiness(env),
		sandboxM4EvidencePacketReadiness(env),
		sandboxM4EvidenceChainReadiness(env),
		diagnosisAuthLiveSmokeReadiness(env),
		diagnosisLiveBrowserSmokeReadiness(env),
		diagnosisLiveConvergenceSmokeReadiness(env),
		alertmanagerAutoDiagnosisLiveSmokeReadiness(env),
	}
	selected, err := selectTargets(targets, targetsFlag.Values())
	if err != nil {
		return err
	}
	selected = orderedTargetsBySequence(selected)
	out := readinessOutput{
		Tool:    toolName,
		Status:  "ready",
		Targets: selected,
	}
	var blockedTargets []targetReadiness
	for _, target := range selected {
		if target.Status == "ready" {
			out.Summary.ReadyCount++
		} else {
			out.Summary.BlockedCount++
			out.Status = "blocked"
			blockedTargets = append(blockedTargets, target)
		}
	}
	out.Summary.NextTarget = nextBlockedTarget(blockedTargets)
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	if *requireReady && out.Status != "ready" {
		return fmt.Errorf("selected readiness target is blocked")
	}
	return nil
}

func orderedTargetsBySequence(targets []targetReadiness) []targetReadiness {
	ordered := append([]targetReadiness(nil), targets...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i].Sequence
		right := ordered[j].Sequence
		if left == right {
			return false
		}
		if left == 0 {
			return false
		}
		if right == 0 {
			return true
		}
		return left < right
	})
	return ordered
}

func nextBlockedTarget(targets []targetReadiness) *nextTargetHint {
	if len(targets) == 0 {
		return nil
	}
	next := targets[0]
	for _, target := range targets[1:] {
		if target.Sequence != 0 && (next.Sequence == 0 || target.Sequence < next.Sequence) {
			next = target
		}
	}
	return &nextTargetHint{
		Name:         next.Name,
		Milestone:    next.Milestone,
		Command:      next.Command,
		EvidenceGoal: next.EvidenceGoal,
	}
}

func sandboxM4BaselineAuditReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "sandbox-m4-baseline-audit",
		Milestone:    "M4",
		Sequence:     30,
		EvidenceGoal: "Retain the code-level sandbox baseline audit artifact used by the M4 decision evidence chain.",
		Command:      "make sandbox-m4-baseline-audit OUT=...",
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
		Name:         "sandbox-m4-quality-sample-export",
		Milestone:    "M4",
		Sequence:     50,
		EvidenceGoal: "Export operator-selected persisted direct and sandbox SubReport rows into a retained sample layout.",
		DependsOn:    []string{"sandbox-m4-subreport-generate"},
		Command:      "DATABASE_URL=... make sandbox-m4-quality-sample-export SELECTION=... ROOT=...",
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
		Name:         "sandbox-m4-quality-manifest-prepare",
		Milestone:    "M4",
		Sequence:     60,
		EvidenceGoal: "Prepare a portable retained quality manifest from paired direct and sandbox report samples.",
		DependsOn:    []string{"sandbox-m4-quality-sample-export"},
		Command:      "make sandbox-m4-quality-manifest-prepare ROOT=... SAMPLE_BASIS=... OUT=...",
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
		Name:         "sandbox-m4-quality-compare",
		Milestone:    "M4",
		Sequence:     70,
		EvidenceGoal: "Run the retained direct-vs-sandbox report quality comparison artifact for representative samples.",
		DependsOn:    []string{"sandbox-m4-quality-manifest-prepare"},
		Command:      "make sandbox-m4-quality-compare QUALITY_MANIFEST=... OUT=...",
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

func selectTargets(targets []targetReadiness, targetNames []string) ([]targetReadiness, error) {
	if len(targetNames) == 0 {
		return targets, nil
	}
	wantAll := false
	requested := make(map[string]struct{}, len(targetNames))
	for _, name := range targetNames {
		name = strings.TrimSpace(name)
		if name == "" || name == "all" {
			wantAll = true
			continue
		}
		requested[name] = struct{}{}
	}
	if wantAll {
		if len(requested) > 0 {
			return nil, fmt.Errorf("target %q cannot be combined with specific targets", "all")
		}
		return targets, nil
	}
	var selected []targetReadiness
	for _, candidate := range targets {
		if _, ok := requested[candidate.Name]; !ok {
			continue
		}
		selected = append(selected, candidate)
		delete(requested, candidate.Name)
	}
	if len(requested) == 0 {
		return selected, nil
	}
	var unknown []string
	for name := range requested {
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	var names []string
	for _, candidate := range targets {
		names = append(names, candidate.Name)
	}
	sort.Strings(names)
	return nil, fmt.Errorf("unknown target %q; expected all or one of: %s", strings.Join(unknown, ","), strings.Join(names, ", "))
}

func alertOperationsLiveInputsReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "alert-operations-live-inputs",
		Milestone:    "M2-M5",
		Sequence:     5,
		EvidenceGoal: "Preflight the environment-provided alert, LLM, and notification endpoints before retained live proof runs.",
		Command:      "make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=alert-operations-live-inputs",
		Notes: []string{
			"Preflight validates only local environment shape; it does not connect to Alertmanager, Thanos, LLM, or Webhook services.",
			"Output intentionally names only missing or invalid environment variables and never prints endpoint, token, or webhook values.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_PROMETHEUS_URL",
		"OPENCLARION_LLM_MODEL",
		"OPENCLARION_IM_WEBHOOK_URL",
	)
	for _, name := range []string{
		"OPENCLARION_PROMETHEUS_URL",
		"OPENCLARION_ALERTMANAGER_URL",
		"OPENCLARION_THANOS_RULE_URL",
		"OPENCLARION_LLM_BASE_URL",
	} {
		if envPresent(env, name) {
			if err := validateReadinessHTTPURL(env[name]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: err.Error(),
				})
			}
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
	for _, name := range []string{"OPENCLARION_LLM_MODEL", "OPENCLARION_LLM_API_KEY"} {
		if envPresent(env, name) {
			if err := validateReadinessOptionalID(env[name]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: err.Error(),
				})
			}
		}
	}
	if envPresent(env, "OPENCLARION_IM_WEBHOOK_FORMAT") &&
		!oneOf(strings.ToLower(strings.TrimSpace(env["OPENCLARION_IM_WEBHOOK_FORMAT"])), "generic", "wecom", "dingtalk", "feishu", "slack") {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_IM_WEBHOOK_FORMAT",
			Reason: "must be generic, wecom, dingtalk, feishu, or slack when set",
		})
	}
	if envPresent(env, "OPENCLARION_IM_WEBHOOK_BEARER_TOKEN") &&
		readinessWebhookFormatDisallowsBearer(env["OPENCLARION_IM_WEBHOOK_FORMAT"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_IM_WEBHOOK_BEARER_TOKEN",
			Reason: "must not be set when OPENCLARION_IM_WEBHOOK_FORMAT is wecom, dingtalk, feishu, or slack",
		})
	}
	return finalize(target)
}

func notificationChannelLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "notification-channel-live-smoke",
		Milestone:    "M2-M5",
		Sequence:     8,
		EvidenceGoal: "Retain focused proof that one persisted notification channel profile resolves its secret and delivers through the configured IM provider.",
		Command:      "make notification-channel-live-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not call the API, resolve secret refs, or send notifications.",
			"The running backend must already have server-side notification channel secret resolver wiring for the selected profile.",
			"Use this smoke before workflow-level proof when validating a WeCom or other webhook delivery target.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
	)
	if !envPresent(env, "NOTIFICATION_CHANNEL_PROFILE_ID") && !envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "notification channel profile ID",
			Options: []string{
				"NOTIFICATION_CHANNEL_PROFILE_ID",
				"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID",
			},
		})
	}
	if envPresent(env, "OPENCLARION_LIVE_API_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_API_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_API_BASE_URL",
				Reason: err.Error(),
			})
		}
	}
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_PROFILE_ID",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID",
	} {
		if envPresent(env, name) && !positiveInteger(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a positive integer",
			})
		}
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_PROFILE_ID") &&
		envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID") &&
		strings.TrimSpace(env["NOTIFICATION_CHANNEL_PROFILE_ID"]) != strings.TrimSpace(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_PROFILE_ID",
			Reason: "must match OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID when both are set",
		})
	}
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_EXPECTED_KIND",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND",
	} {
		if envPresent(env, name) && !validNotificationChannelKind(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be webhook, wecom, dingtalk, feishu, slack, or email when set",
			})
		}
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_KIND") &&
		envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND") &&
		!strings.EqualFold(
			strings.TrimSpace(env["NOTIFICATION_CHANNEL_EXPECTED_KIND"]),
			strings.TrimSpace(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND"]),
		) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_KIND",
			Reason: "must match OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND when both are set",
		})
	}
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND",
	} {
		if envPresent(env, name) && !validNotificationChannelContentKind(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be transport_sample, ai_diagnosis_sample, or diagnosis_close_sample when set",
			})
		}
	}
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS",
	} {
		if envPresent(env, name) && !validNotificationChannelContentKinds(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be comma-separated transport_sample, ai_diagnosis_sample, or diagnosis_close_sample values without duplicates",
			})
		}
	}
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
	} {
		if envPresent(env, name) && !validReadinessBoolean(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be true or false when set",
			})
		}
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND") &&
		envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND") &&
		!strings.EqualFold(
			strings.TrimSpace(env["NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND"]),
			strings.TrimSpace(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND"]),
		) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND",
			Reason: "must match OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND when both are set",
		})
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS") &&
		envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS") &&
		!strings.EqualFold(
			strings.TrimSpace(env["NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS"]),
			strings.TrimSpace(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS"]),
		) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS",
			Reason: "must match OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS when both are set",
		})
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF") &&
		envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF") &&
		!strings.EqualFold(
			strings.TrimSpace(env["NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF"]),
			strings.TrimSpace(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF"]),
		) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
			Reason: "must match OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF when both are set",
		})
	}
	if (envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND") || envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND")) &&
		(envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS") || envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS")) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND",
			Reason: "must not be set with NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS",
		})
	}
	if notificationChannelAIProofRequired(env) &&
		(envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND") ||
			envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND") ||
			envPresent(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS") ||
			envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS")) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
			Reason: "must not be set with explicit notification channel expected content kinds",
		})
	}
	expectedKind := notificationChannelExpectedValue(env, "NOTIFICATION_CHANNEL_EXPECTED_KIND", "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND")
	expectedContentKind := notificationChannelExpectedValue(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND", "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND")
	expectedContentKinds := notificationChannelExpectedValues(env, "NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS", "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS")
	if diagnosisNotificationTestContentKind(expectedContentKind) && expectedKind != "" && expectedKind != "wecom" {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_KIND",
			Reason: "must be wecom when diagnosis notification content is expected",
		})
	}
	if anyDiagnosisNotificationTestContentKind(expectedContentKinds) && expectedKind != "" && expectedKind != "wecom" {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_KIND",
			Reason: "must be wecom when diagnosis notification content is expected",
		})
	}
	if notificationChannelAIProofRequired(env) && expectedKind != "" && expectedKind != "wecom" {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "NOTIFICATION_CHANNEL_EXPECTED_KIND",
			Reason: "must be wecom when AI proof is required",
		})
	}
	if envPresent(env, "OPENCLARION_LIVE_BEARER_TOKEN") && !validReadinessBearerToken(env["OPENCLARION_LIVE_BEARER_TOKEN"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_BEARER_TOKEN",
			Reason: "must be a single bearer token or Bearer header without embedded whitespace",
		})
	}
	if envPresent(env, "NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT") {
		if err := validateReadinessPositiveDuration(env["NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT",
				Reason: err.Error(),
			})
		}
	}
	return finalize(target)
}

func reportLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "report-live-smoke",
		Milestone:    "M2",
		Sequence:     10,
		EvidenceGoal: "Retain the real Prometheus-to-Temporal-to-Webhook proof for the headless report loop.",
		Command:      "make report-live-smoke",
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
	if !allPresent(env, "OPENCLARION_LLM_MODEL", "OPENCLARION_IM_WEBHOOK_URL") && !envFlagEnabled(env, "REPORT_LIVE_SMOKE_ASSUME_WORKER_READY") {
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

func reportPolicyLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "report-policy-live-smoke",
		Milestone:    "M3.1",
		Sequence:     15,
		EvidenceGoal: "Retain the profile-driven report workflow policy replay proof for alert operations configuration.",
		DependsOn:    []string{"report-live-smoke"},
		Command:      "make report-policy-live-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not connect to PostgreSQL, Temporal, alert source providers, LLM, or notification services.",
			"The stored report workflow policy owns the report scenario; this target intentionally has no REPORT_SCENARIO override.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"DATABASE_URL",
		"TEMPORAL_HOST_PORT",
		"REPORT_WORKFLOW_POLICY_ID",
		"REPORT_WINDOW_START",
		"REPORT_WINDOW_END",
		"REPORT_POLICY_LIVE_SMOKE_OUTPUT",
	)
	if envPresent(env, "REPORT_WORKFLOW_POLICY_ID") {
		if err := validateReadinessPositiveInteger(env["REPORT_WORKFLOW_POLICY_ID"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WORKFLOW_POLICY_ID",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON") {
		if err := validateReadinessSecretRefsJSON(env["OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		if err := validateReadinessSecretRefsJSON(env["OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
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
	workerNotificationConfigured := envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") ||
		envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON")
	if (!envPresent(env, "OPENCLARION_LLM_MODEL") || !workerNotificationConfigured) &&
		!envFlagEnabled(env, "REPORT_LIVE_SMOKE_ASSUME_WORKER_READY") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "report-capable worker configuration",
			Options: []string{
				"OPENCLARION_LLM_MODEL + (OPENCLARION_IM_WEBHOOK_URL or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON)",
				"REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1",
			},
		})
	}
	target.FileChecks = append(target.FileChecks, requiredAbsentOutputFileEnv(env, "REPORT_POLICY_LIVE_SMOKE_OUTPUT"))
	return finalize(target)
}

func reportScheduleLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "report-schedule-live-smoke",
		Milestone:    "M3.1",
		Sequence:     16,
		EvidenceGoal: "Prepare retained scheduled-trigger proof for report workflow schedules.",
		DependsOn:    []string{"report-policy-live-smoke"},
		Command:      "make report-schedule-live-smoke",
		Notes: []string{
			"Readiness only checks local configuration; it does not connect to PostgreSQL, Temporal, alert source providers, LLM, or notification services.",
			"The target command waits for a real Temporal Schedule action at or after REPORT_SCHEDULE_OBSERVED_AFTER and validates retained JSON.",
			"Scheduled-trigger live proof remains pending until an operator runs the target against a real enabled schedule and retains delivery evidence.",
			"Use Temporal Schedule Describe or the Temporal UI to confirm the stored schedule is registered, unpaused when enabled, uses skip overlap, and has upcoming action times.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"DATABASE_URL",
		"TEMPORAL_HOST_PORT",
		"REPORT_WORKFLOW_SCHEDULE_ID",
		"REPORT_WORKFLOW_POLICY_ID",
		"REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT",
	)
	if envPresent(env, "REPORT_WORKFLOW_SCHEDULE_ID") {
		if err := validateReadinessPositiveInteger(env["REPORT_WORKFLOW_SCHEDULE_ID"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WORKFLOW_SCHEDULE_ID",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "REPORT_WORKFLOW_POLICY_ID") {
		if err := validateReadinessPositiveInteger(env["REPORT_WORKFLOW_POLICY_ID"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_WORKFLOW_POLICY_ID",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "TEMPORAL_SCHEDULE_ID") {
		if err := validateReadinessOptionalID(env["TEMPORAL_SCHEDULE_ID"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "TEMPORAL_SCHEDULE_ID",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON") {
		if err := validateReadinessSecretRefsJSON(env["OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		if err := validateReadinessSecretRefsJSON(env["OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
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
	if envPresent(env, "REPORT_SCHEDULE_WAIT_TIMEOUT") {
		if err := validateReadinessPositiveDuration(env["REPORT_SCHEDULE_WAIT_TIMEOUT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "REPORT_SCHEDULE_WAIT_TIMEOUT",
				Reason: err.Error(),
			})
		}
	}
	workerNotificationConfigured := envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") ||
		envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON")
	if (!envPresent(env, "OPENCLARION_LLM_MODEL") || !workerNotificationConfigured) &&
		!envFlagEnabled(env, "REPORT_LIVE_SMOKE_ASSUME_WORKER_READY") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "scheduled report-capable worker configuration",
			Options: []string{
				"OPENCLARION_LLM_MODEL + (OPENCLARION_IM_WEBHOOK_URL or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON)",
				"REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1",
			},
		})
	}
	target.FileChecks = append(target.FileChecks, requiredAbsentOutputFileEnv(env, "REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT"))
	return finalize(target)
}

func sandboxM4RuntimeSmokeArtifactsReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "sandbox-m4-runtime-smoke-artifacts",
		Milestone:    "M4",
		Sequence:     40,
		EvidenceGoal: "Retain digest-bound runtime, provider lifecycle, timeout, output-cap, and egress smoke artifacts.",
		Command:      "make sandbox-m4-runtime-smoke-artifacts OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=... OPENCLARION_AGENT_RUNTIME_IMAGE=...",
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
		Name:         "sandbox-m4-review-evidence-template",
		Milestone:    "M4",
		Sequence:     80,
		EvidenceGoal: "Prepare the human-review evidence draft that binds quality cases to the selected runtime candidate.",
		DependsOn:    []string{"sandbox-m4-quality-compare", "sandbox-m4-runtime-smoke-artifacts"},
		Command:      "make sandbox-m4-review-evidence-template QUALITY_COMPARISON=... RUNTIME_SMOKE_ARTIFACTS_ROOT=... SELECTED_CANDIDATE=... RUNTIME_CANDIDATE[_FILE]=... REVIEWER=...",
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
		Name:         "sandbox-m4-decision",
		Milestone:    "M4",
		Sequence:     90,
		EvidenceGoal: "Record the M4 proceed, iterate, or defer decision from retained baseline, quality, runtime, and review evidence.",
		DependsOn:    []string{"sandbox-m4-baseline-audit", "sandbox-m4-quality-compare", "sandbox-m4-review-evidence-template"},
		Command:      "make sandbox-m4-decision BASELINE_AUDIT=... QUALITY_COMPARISON=... REVIEW_EVIDENCE=...",
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
		Name:         "sandbox-m4-evidence-packet",
		Milestone:    "M4",
		Sequence:     100,
		EvidenceGoal: "Freeze the retained M4 baseline, quality, review, runtime-smoke, decision, and sample inputs into one packet.",
		DependsOn:    []string{"sandbox-m4-decision"},
		Command:      "make sandbox-m4-evidence-packet QUALITY_MANIFEST=... REVIEW_EVIDENCE=... OUT_DIR=...",
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
		Name:         "sandbox-m4-evidence-chain",
		Milestone:    "M4",
		Sequence:     110,
		EvidenceGoal: "Audit the retained M4 evidence directory and semantic packet verification status without rerunning the evidence-producing commands.",
		DependsOn:    []string{"sandbox-m4-evidence-packet"},
		Command:      "OPENCLARION_M4_EVIDENCE_ROOT=... make manual-evidence-readiness MANUAL_EVIDENCE_TARGET=sandbox-m4-evidence-chain",
		Notes: []string{
			"Preflight checks a retained M4 evidence working directory or packet directory for the canonical artifact chain without printing local paths.",
			"JSON artifacts are checked for duplicate object keys and trailing JSON before the packet verifier performs semantic validation when all canonical packet artifacts are present.",
			"The target reports presence, SHA-256 digests, and packet verification status only; it does not judge sample representativeness or accept a runtime baseline.",
		},
	}
	target.DirectoryChecks = append(target.DirectoryChecks, requiredDirectoryEnv(env, "OPENCLARION_M4_EVIDENCE_ROOT"))
	if directoryChecksReady(target.DirectoryChecks) {
		target.EvidenceChainChecks = m4EvidenceChainChecks(filepath.Clean(strings.TrimSpace(env["OPENCLARION_M4_EVIDENCE_ROOT"])))
	}
	return finalize(target)
}

func diagnosisAuthLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "diagnosis-auth-live-smoke",
		Milestone:    "M5",
		Sequence:     19,
		EvidenceGoal: "Retain proof that the running backend diagnosis auth status and credential check path accepts LDAP or bearer credentials without storing secrets.",
		Command:      "make diagnosis-auth-live-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not authenticate or contact the live backend.",
			"LDAP credentials are passed to the live smoke through environment-variable references, not command-line argv.",
			"Bearer mode accepts OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN, OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN, or OPENCLARION_LIVE_BEARER_TOKEN.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
	)
	if envPresent(env, "OPENCLARION_LIVE_API_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_API_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_API_BASE_URL",
				Reason: err.Error(),
			})
		}
	}
	mode := diagnosisAuthLiveSmokeAuthMode(env)
	switch mode {
	case "":
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_AUTH_MODE",
			Reason: "must be ldap or bearer when set",
		})
	case "ldap":
		target.MissingEnv = append(target.MissingEnv, missingEnv(env,
			"OPENCLARION_LIVE_LDAP_USERNAME",
			"OPENCLARION_LIVE_LDAP_PASSWORD",
		)...)
		if envPresent(env, "OPENCLARION_LIVE_LDAP_USERNAME") && !validReadinessLDAPUsername(env["OPENCLARION_LIVE_LDAP_USERNAME"]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_LDAP_USERNAME",
				Reason: "must be non-empty without leading/trailing whitespace or embedded whitespace",
			})
		}
		if envPresent(env, "OPENCLARION_LIVE_LDAP_PASSWORD") && !validReadinessLDAPPassword(env["OPENCLARION_LIVE_LDAP_PASSWORD"]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_LDAP_PASSWORD",
				Reason: "must be non-empty and must not contain CR or LF",
			})
		}
	case "bearer":
		if !envPresent(env, "OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN") &&
			!envPresent(env, "OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN") &&
			!envPresent(env, "OPENCLARION_LIVE_BEARER_TOKEN") {
			target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
				Description: "diagnosis auth check credentials",
				Options: []string{
					"OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN",
					"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN",
					"OPENCLARION_LIVE_BEARER_TOKEN",
					"OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD",
				},
			})
		}
		for _, name := range []string{
			"OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN",
			"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN",
			"OPENCLARION_LIVE_BEARER_TOKEN",
		} {
			if envPresent(env, name) && !validReadinessBearerToken(env[name]) {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: "must be a single bearer token or Bearer header without embedded whitespace",
				})
			}
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE") &&
		!validDiagnosisAuthBackendMode(env["OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE",
			Reason: "must be ldap, static, oidc, unknown, or none",
		})
	}
	if envPresent(env, "DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT") {
		if err := validateReadinessPositiveDuration(env["DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT",
				Reason: err.Error(),
			})
		}
	}
	if diagnosisLDAPBackendReadinessRequired(env) {
		applyDiagnosisLDAPBackendReadiness(&target, env)
	}
	return finalize(target)
}

func diagnosisLiveBrowserSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "diagnosis-live-browser-smoke",
		Milestone:    "M5",
		Sequence:     20,
		EvidenceGoal: "Retain the real diagnosis-room browser proof, including close-notification IM evidence when explicitly required.",
		Command:      "make diagnosis-live-browser-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not authenticate, create a room, install browsers, or contact the live backend.",
			"When close-notification proof is required, the local close CLI still signals Temporal and loads PostgreSQL lifecycle events while the running worker must be configured to send the IM notification.",
			"When planned evidence collection proof is requested, the browser smoke must be able to find and execute a Use collection plan action after the first assistant turn.",
			"When supplemental evidence proof is requested, the browser smoke must be able to find a Use follow-up action after the first assistant turn.",
			"OPENCLARION_LIVE_LDAP_USERNAME and OPENCLARION_LIVE_LDAP_PASSWORD may replace bearer auth for LDAP-backed live diagnosis-room authentication.",
			"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL may replace OPENCLARION_LIVE_BEARER_TOKEN only for a loopback dev OIDC token endpoint; the smoke harness fetches the token later and the readiness check does not contact the endpoint.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
	)
	applyDiagnosisLiveAuthReadiness(&target, env)
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
	if envPresent(env, "OPENCLARION_LIVE_BROWSER_WS_BASE_URL") {
		if err := validateReadinessBrowserWSBaseURL(env["OPENCLARION_LIVE_BROWSER_WS_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_BROWSER_WS_BASE_URL",
				Reason: err.Error(),
			})
		}
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
	for _, name := range []string{
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE",
		"OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE",
	} {
		if envPresent(env, name) {
			if err := validateReadinessBooleanFlag(env[name]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: err.Error(),
				})
			}
		}
	}
	if supplementalEvidenceRequired(env) && !supplementalEvidenceSubmitRequested(env) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE",
			Reason: "requires OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE to be enabled",
		})
	}
	if envPresent(env, "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT") {
		if err := validateReadinessSupplementalEvidenceText(env["OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE") {
		if err := validateReadinessSupplementalEvidenceText(env["OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_TOOL_REQUESTS_JSON") {
		if err := validateReadinessLiveToolRequestsJSON(env["OPENCLARION_LIVE_TOOL_REQUESTS_JSON"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_TOOL_REQUESTS_JSON",
				Reason: err.Error(),
			})
		}
	}
	if closeNotificationProofRequired(env) {
		target.MissingEnv = append(target.MissingEnv, missingEnv(env,
			"DATABASE_URL",
			"TEMPORAL_HOST_PORT",
		)...)
		workerNotificationConfigured := envPresent(env, "OPENCLARION_IM_WEBHOOK_URL") ||
			envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON")
		if !workerNotificationConfigured && !envFlagEnabled(env, "DIAGNOSIS_LIVE_SMOKE_ASSUME_WORKER_READY") {
			target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
				Description: "close-notification worker IM configuration",
				Options: []string{
					"OPENCLARION_IM_WEBHOOK_URL or OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
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
		if envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
			appendWeComSecretRefsInvalidEnv(&target, env)
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

func diagnosisLiveConvergenceSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "diagnosis-live-convergence-smoke",
		Milestone:    "M5",
		Sequence:     21,
		EvidenceGoal: "Retain the backend-only diagnosis-room convergence proof for AI turn, executable evidence collection, residual evidence boundary, ready_for_review, and confirmation.",
		Command:      "make diagnosis-live-convergence-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not authenticate, create a room, or contact the live backend.",
			"The smoke talks directly to the diagnosis WebSocket and validates the multi-step AI diagnosis loop without launching a browser.",
			"Set OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID to bind created rooms to a notification channel and require assistant update, final conclusion, and close notification AI content proof in the retained room timeline.",
			"OPENCLARION_LIVE_LDAP_USERNAME and OPENCLARION_LIVE_LDAP_PASSWORD may replace bearer auth for LDAP-backed live diagnosis-room authentication.",
			"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL may replace OPENCLARION_LIVE_BEARER_TOKEN only for a loopback dev OIDC token endpoint; the smoke harness fetches the token later and the readiness check does not contact the endpoint.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
	)
	applyDiagnosisLiveAuthReadiness(&target, env)
	if envPresent(env, "OPENCLARION_LIVE_API_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_API_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_API_BASE_URL",
				Reason: err.Error(),
			})
		}
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
	if envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID") && !positiveInteger(env["OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID",
			Reason: "must be a positive integer when set",
		})
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE",
		"OPENCLARION_LIVE_CONFIRM_CONCLUSION",
	} {
		if envPresent(env, name) {
			if err := validateReadinessBooleanFlag(env[name]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: err.Error(),
				})
			}
		}
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_DEFAULT_TURN_TIMEOUT_MS",
		"OPENCLARION_LIVE_TURN_TIMEOUT_MS",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS",
	} {
		if envPresent(env, name) && !positiveInteger(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a positive integer when set",
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT") {
		if err := validateReadinessSupplementalEvidenceText(env["OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE") {
		if err := validateReadinessSupplementalEvidenceText(env["OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE",
				Reason: err.Error(),
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID") &&
		envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		appendWeComSecretRefsInvalidEnv(&target, env)
	}
	return finalize(target)
}

func alertmanagerAutoDiagnosisLiveSmokeReadiness(env envMap) targetReadiness {
	target := targetReadiness{
		Name:         "alertmanager-auto-diagnosis-live-smoke",
		Milestone:    "M5",
		Sequence:     22,
		EvidenceGoal: "Retain proof that a live Alertmanager webhook starts an auto_room diagnosis and records interim assistant-message plus final-conclusion AI notification delivery in the room timeline.",
		Command:      "make alertmanager-auto-diagnosis-live-smoke",
		Notes: []string{
			"Preflight only checks local configuration; it does not post a webhook, poll a room, authenticate to Alertmanager, or contact the live backend.",
			"The running backend and worker must already be wired to Temporal, the LLM provider, the Alertmanager alert-source profile, and a notification channel profile that supports report, diagnosis_consultation, and diagnosis_close scopes.",
			"The live smoke defaults to requiring the first assistant_message AI notification proof; set ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS explicitly when validating later final_conclusion delivery.",
			"The optional webhook bearer token is only used for POST /api/v1/alert-sources/{source_id}/webhooks/alertmanager; the retained proof does not include token values.",
		},
	}
	target.MissingEnv = missingEnv(env,
		"OPENCLARION_LIVE_API_BASE_URL",
	)
	if !envPresent(env, "ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID") && !envPresent(env, "OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "Alertmanager webhook source profile ID",
			Options: []string{
				"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID",
				"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID",
			},
		})
	}
	if envPresent(env, "OPENCLARION_LIVE_API_BASE_URL") {
		if err := validateReadinessHTTPURL(env["OPENCLARION_LIVE_API_BASE_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_API_BASE_URL",
				Reason: err.Error(),
			})
		}
	}
	for _, name := range []string{
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID",
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID",
	} {
		if envPresent(env, name) && !positiveInteger(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a positive integer",
			})
		}
	}
	if envPresent(env, "ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID") &&
		envPresent(env, "OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID") &&
		strings.TrimSpace(env["ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID"]) != strings.TrimSpace(env["OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID",
			Reason: "must match OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID when both are set",
		})
	}
	for _, name := range []string{
		"ALERTMANAGER_WEBHOOK_BEARER_TOKEN",
		"OPENCLARION_ALERTMANAGER_WEBHOOK_BEARER_TOKEN",
	} {
		if envPresent(env, name) && !validReadinessBearerToken(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a single bearer token or Bearer header without embedded whitespace",
			})
		}
	}
	if envPresent(env, "ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID") &&
		!positiveInteger(env["ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID",
			Reason: "must be a positive integer",
		})
	}
	if envPresent(env, "ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND") &&
		!validDiagnosisNotificationContentKind(env["ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND",
			Reason: "must be assistant_message or final_conclusion when set",
		})
	}
	if envPresent(env, "ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS") &&
		!validDiagnosisNotificationContentKinds(env["ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS",
			Reason: "must be comma-separated assistant_message or final_conclusion values without duplicates",
		})
	}
	for _, name := range []string{
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ROOM_TIMEOUT",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_POLL_INTERVAL",
	} {
		if envPresent(env, name) {
			if err := validateReadinessPositiveDuration(env[name]); err != nil {
				target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
					Name:   name,
					Reason: err.Error(),
				})
			}
		}
	}
	if envPresent(env, "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME") &&
		!validReadinessAutoDiagnosisAlertName(env["ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME",
			Reason: "must be 1-128 characters using only letters, digits, underscore, colon, or hyphen",
		})
	}
	if envPresent(env, "ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID") &&
		envPresent(env, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		appendWeComSecretRefsInvalidEnv(&target, env)
	}
	return finalize(target)
}

func appendWeComSecretRefsInvalidEnv(target *targetReadiness, env envMap) {
	secretRefs := env["OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON"]
	if err := validateReadinessSecretRefsJSON(secretRefs); err != nil {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
			Reason: err.Error(),
		})
	} else if err := validateReadinessWeComSecretRefsJSON(secretRefs); err != nil {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
			Reason: err.Error(),
		})
	}
}

func applyDiagnosisLiveAuthReadiness(target *targetReadiness, env envMap) {
	mode := diagnosisLiveAuthMode(env)
	if mode == "" {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_LIVE_AUTH_MODE",
			Reason: "must be ldap or bearer when set",
		})
		return
	}
	if diagnosisLDAPBackendReadinessRequired(env) {
		applyDiagnosisLDAPBackendReadiness(target, env)
	}
	if mode == "ldap" {
		target.MissingEnv = append(target.MissingEnv, missingEnv(env,
			"OPENCLARION_LIVE_LDAP_USERNAME",
			"OPENCLARION_LIVE_LDAP_PASSWORD",
		)...)
		if envPresent(env, "OPENCLARION_LIVE_LDAP_USERNAME") && !validReadinessLDAPUsername(env["OPENCLARION_LIVE_LDAP_USERNAME"]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_LDAP_USERNAME",
				Reason: "must be non-empty without leading/trailing whitespace or embedded whitespace",
			})
		}
		if envPresent(env, "OPENCLARION_LIVE_LDAP_PASSWORD") && !validReadinessLDAPPassword(env["OPENCLARION_LIVE_LDAP_PASSWORD"]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_LDAP_PASSWORD",
				Reason: "must be non-empty and must not contain CR or LF",
			})
		}
		return
	}
	if !envPresent(env, "OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN") &&
		!envPresent(env, "OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN") &&
		!envPresent(env, "OPENCLARION_LIVE_BEARER_TOKEN") &&
		!envPresent(env, "OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "diagnosis room authorization credentials",
			Options: []string{
				"OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN",
				"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN",
				"OPENCLARION_LIVE_BEARER_TOKEN",
				"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL",
				"OPENCLARION_LIVE_LDAP_USERNAME + OPENCLARION_LIVE_LDAP_PASSWORD",
			},
		})
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN",
		"OPENCLARION_LIVE_BEARER_TOKEN",
	} {
		if envPresent(env, name) && !validReadinessBearerToken(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a single bearer token or Bearer header without embedded whitespace",
			})
		}
	}
	if envPresent(env, "OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL") {
		if err := validateReadinessLoopbackHTTPURL(env["OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL",
				Reason: err.Error(),
			})
		}
	}
}

func applyDiagnosisLDAPBackendReadiness(target *targetReadiness, env envMap) {
	target.Notes = append(target.Notes,
		"When OPENCLARION_DIAGNOSIS_AUTH_MODE=ldap or LDAP backend env is present, this preflight also checks the local worker LDAP provider configuration shape without contacting LDAP.",
	)
	target.MissingEnv = append(target.MissingEnv, missingEnv(env,
		"OPENCLARION_DIAGNOSIS_LDAP_URL",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
	)...)
	if !envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES") &&
		!envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES") &&
		!envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES") {
		target.UnsatisfiedAlternatives = append(target.UnsatisfiedAlternatives, envAlternative{
			Description: "diagnosis LDAP role mapping",
			Options: []string{
				"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
				"OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
				"OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
			},
		})
	}
	if envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_URL") {
		if err := validateReadinessLDAPURL(env["OPENCLARION_DIAGNOSIS_LDAP_URL"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_DIAGNOSIS_LDAP_URL",
				Reason: err.Error(),
			})
		}
		if readinessLDAPURLUsesPlaintext(env["OPENCLARION_DIAGNOSIS_LDAP_URL"]) &&
			!readinessBooleanTrue(env["OPENCLARION_DIAGNOSIS_LDAP_START_TLS"]) &&
			!readinessBooleanTrue(env["OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT"]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_DIAGNOSIS_LDAP_URL",
				Reason: "ldap:// requires OPENCLARION_DIAGNOSIS_LDAP_START_TLS=true or OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT=true",
			})
		}
	}
	if envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN") && !validReadinessSingleLineTrimmed(env["OPENCLARION_DIAGNOSIS_LDAP_BASE_DN"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
			Reason: "must be non-empty without leading/trailing whitespace or line breaks",
		})
	}
	bindDNSet := envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN")
	bindPasswordSet := envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD")
	if bindDNSet && !bindPasswordSet {
		target.MissingEnv = append(target.MissingEnv, "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD")
	}
	if bindPasswordSet && !bindDNSet {
		target.MissingEnv = append(target.MissingEnv, "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN")
	}
	if bindDNSet && !validReadinessSingleLineTrimmed(env["OPENCLARION_DIAGNOSIS_LDAP_BIND_DN"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
			Reason: "must be non-empty without leading/trailing whitespace or line breaks",
		})
	}
	if bindPasswordSet && strings.ContainsAny(env["OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD"], "\x00\r\n") {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
			Reason: "must not contain NUL, CR, or LF",
		})
	}
	for _, name := range []string{
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
		"OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT",
	} {
		if envPresent(env, name) && !validReadinessBoolean(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be true or false when set",
			})
		}
	}
	if envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER") {
		if err := validateReadinessLDAPUserFilter(env["OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER"]); err != nil {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
				Reason: err.Error(),
			})
		}
	}
	for _, name := range []string{
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE",
	} {
		if envPresent(env, name) && !validReadinessLDAPAttribute(env[name]) {
			target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
				Name:   name,
				Reason: "must be a single LDAP attribute name without whitespace",
			})
		}
	}
	if envPresent(env, "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES") && !validReadinessAuthRoleCSV(env["OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES"]) {
		target.InvalidEnv = append(target.InvalidEnv, invalidEnv{
			Name:   "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
			Reason: "must contain owner and/or admin values",
		})
	}
}

func diagnosisLDAPBackendReadinessRequired(env envMap) bool {
	if strings.EqualFold(strings.TrimSpace(env["OPENCLARION_DIAGNOSIS_AUTH_MODE"]), "ldap") {
		return true
	}
	for _, name := range []string{
		"OPENCLARION_DIAGNOSIS_LDAP_URL",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
		"OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
		"OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
	} {
		if envPresent(env, name) {
			return true
		}
	}
	return false
}

func diagnosisLiveAuthMode(env envMap) string {
	if envPresent(env, "OPENCLARION_LIVE_AUTH_MODE") {
		mode := strings.ToLower(env["OPENCLARION_LIVE_AUTH_MODE"])
		if mode == "ldap" || mode == "bearer" {
			return mode
		}
		return ""
	}
	if envPresent(env, "OPENCLARION_LIVE_LDAP_USERNAME") || envPresent(env, "OPENCLARION_LIVE_LDAP_PASSWORD") {
		return "ldap"
	}
	if strings.EqualFold(strings.TrimSpace(env["OPENCLARION_DIAGNOSIS_AUTH_MODE"]), "ldap") {
		return "ldap"
	}
	return "bearer"
}

func diagnosisAuthLiveSmokeAuthMode(env envMap) string {
	if envPresent(env, "OPENCLARION_LIVE_AUTH_MODE") {
		mode := strings.ToLower(env["OPENCLARION_LIVE_AUTH_MODE"])
		if mode == "ldap" || mode == "bearer" {
			return mode
		}
		return ""
	}
	if envPresent(env, "OPENCLARION_LIVE_LDAP_USERNAME") || envPresent(env, "OPENCLARION_LIVE_LDAP_PASSWORD") {
		return "ldap"
	}
	if strings.EqualFold(strings.TrimSpace(env["OPENCLARION_DIAGNOSIS_AUTH_MODE"]), "ldap") {
		return "ldap"
	}
	return "bearer"
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

func envFlagEnabled(env envMap, name string) bool {
	if !envPresent(env, name) {
		return false
	}
	return strings.TrimSpace(env[name]) == "1"
}

func closeNotificationProofRequired(env envMap) bool {
	return envTruthy(env, "OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION") ||
		envTruthy(env, "DIAGNOSIS_LIVE_REQUIRE_CLOSE_NOTIFICATION")
}

func supplementalEvidenceSubmitRequested(env envMap) bool {
	return envTruthyLenient(env, "OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE")
}

func supplementalEvidenceRequired(env envMap) bool {
	return envTruthyLenient(env, "OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE")
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

func envTruthyLenient(env envMap, name string) bool {
	if !envPresent(env, name) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(env[name])) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func validReadinessBoolean(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "true", "false")
}

func notificationChannelAIProofRequired(env envMap) bool {
	for _, name := range []string{
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
	} {
		if envPresent(env, name) {
			return strings.EqualFold(strings.TrimSpace(env[name]), "true")
		}
	}
	return false
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

func validateReadinessSecretRefsJSON(raw string) error {
	_, err := readinessSecretRefsFromJSON(raw)
	return err
}

func validateReadinessWeComSecretRefsJSON(raw string) error {
	secrets, err := readinessSecretRefsFromJSON(raw)
	if err != nil {
		return err
	}
	for _, value := range secrets {
		if validReadinessWeComWebhookEndpoint(value) {
			return nil
		}
	}
	return errors.New("must include at least one Enterprise WeChat group robot webhook endpoint")
}

func readinessSecretRefsFromJSON(raw string) (map[string]string, error) {
	if _, err := envmap.NewResolverFromJSON(raw); err != nil {
		return nil, errors.New("must be a strict JSON object with non-empty single-token string keys and values")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var secrets map[string]string
	if err := strictjson.Unmarshal([]byte(raw), &secrets); err != nil {
		return nil, errors.New("must be a strict JSON object with non-empty single-token string keys and values")
	}
	return secrets, nil
}

func validReadinessWeComWebhookEndpoint(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), readinessWeComWebhookHost) ||
		!strings.EqualFold(parsed.EscapedPath(), readinessWeComWebhookPath) {
		return false
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	keys, ok := values["key"]
	if !ok || len(values) != 1 || len(keys) != 1 {
		return false
	}
	key := keys[0]
	return key != "" && !strings.ContainsAny(key, " \t\r\n\x00")
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

func validateReadinessBooleanFlag(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	switch strings.ToLower(value) {
	case "1", "0", "true", "false", "yes", "no":
		return nil
	default:
		return errors.New("must be one of 1, 0, true, false, yes, or no")
	}
}

func validateReadinessSupplementalEvidenceText(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("must be non-empty when set")
	}
	if len(raw) > maxReadinessSupplementalBytes {
		return fmt.Errorf("exceeds %d bytes", maxReadinessSupplementalBytes)
	}
	return nil
}

type readinessLiveToolRequest struct {
	TemplateID           int64  `json:"template_id,omitempty"`
	AlertSourceProfileID int64  `json:"alert_source_profile_id,omitempty"`
	Tool                 string `json:"tool"`
	Reason               string `json:"reason"`
	Query                string `json:"query,omitempty"`
	WindowSeconds        int    `json:"window_seconds,omitempty"`
	StepSeconds          int    `json:"step_seconds,omitempty"`
	Limit                int    `json:"limit,omitempty"`
}

func validateReadinessLiveToolRequestsJSON(raw string) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return errors.New("must not contain leading or trailing whitespace")
	}
	if value == "" {
		return errors.New("must be a non-empty JSON array when set")
	}
	if len(raw) > maxReadinessToolRequestsBytes {
		return fmt.Errorf("exceeds %d bytes", maxReadinessToolRequestsBytes)
	}
	if err := strictjson.RejectDuplicateObjectKeys([]byte(raw)); err != nil {
		return errors.New("must be duplicate-key-free JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var requests []readinessLiveToolRequest
	if err := decoder.Decode(&requests); err != nil {
		return errors.New("must be a strict JSON array of supported evidence requests")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("must contain exactly one JSON value")
	}
	if len(requests) == 0 {
		return errors.New("must contain at least one evidence request")
	}
	if len(requests) > maxReadinessToolRequestItems {
		return fmt.Errorf("must contain no more than %d evidence requests", maxReadinessToolRequestItems)
	}
	for i, req := range requests {
		if err := validateReadinessLiveToolRequest(i, req); err != nil {
			return err
		}
	}
	return nil
}

func validateReadinessLiveToolRequest(index int, req readinessLiveToolRequest) error {
	if req.TemplateID < 0 || req.AlertSourceProfileID < 0 {
		return fmt.Errorf("request %d identifiers must be positive when set", index)
	}
	if req.TemplateID > 0 && req.AlertSourceProfileID == 0 {
		return fmt.Errorf("request %d template_id requires alert_source_profile_id", index)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("request %d reason must be non-empty", index)
	}
	if len([]byte(req.Reason)) > maxReadinessToolReasonBytes {
		return fmt.Errorf("request %d reason exceeds %d bytes", index, maxReadinessToolReasonBytes)
	}
	if strings.TrimSpace(req.Reason) != req.Reason || strings.ContainsAny(req.Reason, "\r\n\t") {
		return fmt.Errorf("request %d reason must be a single trimmed line", index)
	}
	if len([]byte(req.Query)) > maxReadinessToolQueryBytes {
		return fmt.Errorf("request %d query exceeds %d bytes", index, maxReadinessToolQueryBytes)
	}
	if strings.TrimSpace(req.Query) != req.Query || strings.ContainsAny(req.Query, "\r\n\t") {
		return fmt.Errorf("request %d query must be a single trimmed line", index)
	}
	switch req.Tool {
	case "active_alerts":
		if req.Query != "" || req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return fmt.Errorf("request %d active_alerts must not include query, window_seconds, or step_seconds", index)
		}
		if !readinessToolLimitValid(req.Limit, maxReadinessToolAlertLimit) {
			return fmt.Errorf("request %d active_alerts limit must be between 1 and %d when set", index, maxReadinessToolAlertLimit)
		}
	case "metric_query":
		if req.TemplateID == 0 && req.Query == "" {
			return fmt.Errorf("request %d metric_query requires query or template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return fmt.Errorf("request %d metric_query must not include window_seconds or step_seconds", index)
		}
		if !readinessToolLimitValid(req.Limit, maxReadinessToolMetricLimit) {
			return fmt.Errorf("request %d metric_query limit must be between 1 and %d when set", index, maxReadinessToolMetricLimit)
		}
	case "metric_range_query":
		if req.TemplateID == 0 && req.Query == "" {
			return fmt.Errorf("request %d metric_range_query requires query or template_id", index)
		}
		if req.TemplateID == 0 && (req.WindowSeconds == 0 || req.StepSeconds == 0) {
			return fmt.Errorf("request %d metric_range_query requires window_seconds and step_seconds without template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			if err := validateReadinessToolRange(index, req.WindowSeconds, req.StepSeconds); err != nil {
				return err
			}
		}
		if !readinessToolLimitValid(req.Limit, maxReadinessToolMetricLimit) {
			return fmt.Errorf("request %d metric_range_query limit must be between 1 and %d when set", index, maxReadinessToolMetricLimit)
		}
	default:
		return fmt.Errorf("request %d tool is unsupported", index)
	}
	return nil
}

func readinessToolLimitValid(limit int, maxLimit int) bool {
	return limit == 0 || (limit >= 1 && limit <= maxLimit)
}

func validateReadinessToolRange(index int, windowSeconds int, stepSeconds int) error {
	if windowSeconds < minReadinessToolRangeSeconds || windowSeconds > maxReadinessToolRangeSeconds {
		return fmt.Errorf("request %d window_seconds must be between %d and %d", index, minReadinessToolRangeSeconds, maxReadinessToolRangeSeconds)
	}
	if stepSeconds < minReadinessToolRangeSeconds || stepSeconds > maxReadinessToolRangeSeconds {
		return fmt.Errorf("request %d step_seconds must be between %d and %d", index, minReadinessToolRangeSeconds, maxReadinessToolRangeSeconds)
	}
	if stepSeconds > windowSeconds {
		return fmt.Errorf("request %d step_seconds must not exceed window_seconds", index)
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

func validateReadinessLoopbackHTTPURL(raw string) error {
	parsed, err := parseReadinessURL(raw)
	if err != nil {
		return err
	}
	if parsed.Fragment != "" {
		return errors.New("must not include a fragment")
	}
	host := strings.Trim(parsed.Hostname(), "[]")
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return errors.New("must use a loopback host")
}

func validateReadinessWebhookURL(raw string) error {
	_, err := parseReadinessURL(raw)
	return err
}

func validateReadinessBrowserWSBaseURL(raw string) error {
	parsed, err := parseReadinessURLWithSchemes(raw, map[string]struct{}{
		"http":  {},
		"https": {},
		"ws":    {},
		"wss":   {},
	})
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

func parseReadinessURL(raw string) (*url.URL, error) {
	return parseReadinessURLWithSchemes(raw, map[string]struct{}{
		"http":  {},
		"https": {},
	})
}

func parseReadinessURLWithSchemes(raw string, allowedSchemes map[string]struct{}) (*url.URL, error) {
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
	if _, ok := allowedSchemes[parsed.Scheme]; !ok {
		return nil, errors.New("must use an allowed URL scheme")
	}
	if parsed.Host == "" {
		return nil, errors.New("must include a host")
	}
	if parsed.User != nil {
		return nil, errors.New("must not include user info")
	}
	return parsed, nil
}

func validateReadinessLDAPURL(raw string) error {
	parsed, err := parseReadinessURLWithSchemes(raw, map[string]struct{}{
		"ldap":  {},
		"ldaps": {},
	})
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

func readinessLDAPURLUsesPlaintext(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Scheme, "ldap")
}

func readinessBooleanTrue(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "true")
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

func validNotificationChannelKind(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "webhook", "wecom", "dingtalk", "feishu", "slack", "email")
}

func readinessWebhookFormatDisallowsBearer(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "wecom", "dingtalk", "feishu", "slack")
}

func validNotificationChannelContentKind(raw string) bool {
	return oneOf(
		strings.ToLower(strings.TrimSpace(raw)),
		"transport_sample",
		"ai_diagnosis_sample",
		"diagnosis_close_sample",
	)
}

func validNotificationChannelContentKinds(raw string) bool {
	values := notificationChannelContentKindValues(raw)
	if len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !validNotificationChannelContentKind(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func notificationChannelExpectedValue(env envMap, names ...string) string {
	for _, name := range names {
		if envPresent(env, name) {
			return strings.ToLower(strings.TrimSpace(env[name]))
		}
	}
	return ""
}

func notificationChannelExpectedValues(env envMap, names ...string) []string {
	for _, name := range names {
		if envPresent(env, name) {
			return notificationChannelContentKindValues(env[name])
		}
	}
	return nil
}

func notificationChannelContentKindValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			return nil
		}
		values = append(values, value)
	}
	return values
}

func diagnosisNotificationTestContentKind(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "ai_diagnosis_sample", "diagnosis_close_sample")
}

func anyDiagnosisNotificationTestContentKind(values []string) bool {
	for _, value := range values {
		if diagnosisNotificationTestContentKind(value) {
			return true
		}
	}
	return false
}

func validDiagnosisNotificationContentKind(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "assistant_message", "final_conclusion")
}

func validDiagnosisNotificationContentKinds(raw string) bool {
	values := diagnosisNotificationContentKindValues(raw)
	if len(values) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !validDiagnosisNotificationContentKind(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func diagnosisNotificationContentKindValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			return nil
		}
		values = append(values, value)
	}
	return values
}

func validDiagnosisAuthBackendMode(raw string) bool {
	return oneOf(strings.ToLower(strings.TrimSpace(raw)), "ldap", "static", "oidc", "unknown", "none")
}

func validReadinessAutoDiagnosisAlertName(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '_', ':', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func validReadinessLDAPUsername(raw string) bool {
	value := strings.TrimSpace(raw)
	return value != "" && value == raw && !strings.ContainsAny(raw, " \r\n\t\x00")
}

func validReadinessLDAPPassword(raw string) bool {
	return strings.TrimSpace(raw) != "" && !strings.ContainsAny(raw, "\r\n\x00")
}

func validReadinessSingleLineTrimmed(raw string) bool {
	value := strings.TrimSpace(raw)
	return value != "" && value == raw && !strings.ContainsAny(raw, "\r\n\x00")
}

func validateReadinessLDAPUserFilter(raw string) error {
	filter := strings.TrimSpace(raw)
	if filter == "" {
		return errors.New("must be non-empty when set")
	}
	if !strings.Contains(filter, "{username}") {
		return errors.New("must contain {username}")
	}
	compiled := strings.ReplaceAll(filter, "{username}", gldap.EscapeFilter("fixture"))
	if _, err := gldap.CompileFilter(compiled); err != nil {
		return errors.New("must be a valid LDAP filter after {username} substitution")
	}
	return nil
}

func validReadinessLDAPAttribute(raw string) bool {
	value := strings.TrimSpace(raw)
	return value != "" && value == raw && !strings.ContainsAny(raw, " \t\r\n\x00")
}

func validReadinessAuthRoleCSV(raw string) bool {
	values := csvReadinessValues(raw)
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !oneOf(strings.ToLower(value), "owner", "admin") {
			return false
		}
	}
	return true
}

func csvReadinessValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
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
	artifacts := requiredM4EvidenceArtifacts
	if m4EvidencePacketRoot(root) {
		artifacts = requiredM4PacketEvidenceArtifacts
	}
	checks := make([]evidenceChainCheck, 0, len(artifacts)+1)
	for _, artifact := range artifacts {
		checks = append(checks, checkM4EvidenceArtifact(root, artifact))
	}
	if evidenceChainChecksReady(checks) {
		checks = append(checks, checkM4PacketSemanticVerification(root))
	}
	return checks
}

func m4EvidencePacketRoot(root string) bool {
	if m4PacketSummaryTool(root) {
		return true
	}
	for _, artifact := range []string{"quality-inputs", "runtime-smoke-artifacts"} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(artifact)))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func m4PacketSummaryTool(root string) bool {
	raw, err := readBoundedArtifact(filepath.Join(root, "packet.json"))
	if err != nil || len(raw) > maxReadinessArtifactBytes {
		return false
	}
	var summary struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(raw, &summary); err != nil {
		return false
	}
	return summary.Tool == "sandbox_m4_evidence_packet"
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
