package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReportsMissingEvidencePrerequisitesWithoutValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, []string{
		"DATABASE_URL=postgres://secret@example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(stdout.String(), "secret@example.test") || strings.Contains(stdout.String(), "127.0.0.1:7233") {
		t.Fatalf("output leaked environment value: %s", stdout.String())
	}

	out := decodeOutput(t, stdout.Bytes())
	if out.Status != "blocked" {
		t.Fatalf("Status = %q, want blocked", out.Status)
	}
	report := targetByName(t, out, "report-live-smoke")
	if !contains(report.MissingEnv, "OPENCLARION_PROMETHEUS_URL") {
		t.Fatalf("report missing env = %v, want OPENCLARION_PROMETHEUS_URL", report.MissingEnv)
	}
	if len(report.UnsatisfiedAlternatives) != 1 {
		t.Fatalf("report alternatives = %v, want worker provider alternative", report.UnsatisfiedAlternatives)
	}
}

func TestRunReportsReadyLiveTargets(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_PROMETHEUS_URL=https://prometheus.example.test",
		"REPORT_WINDOW_START=2026-06-04T00:00:00Z",
		"REPORT_WINDOW_END=2026-06-04T01:00:00Z",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test",
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "report-live-smoke").Status; got != "ready" {
		t.Fatalf("report status = %q, want ready", got)
	}
	if got := targetByName(t, out, "diagnosis-live-browser-smoke").Status; got != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", got)
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("output leaked bearer token: %s", stdout.String())
	}
}

func TestRunValidatesM4EvidenceFilesAndPacketOutputDir(t *testing.T) {
	root := t.TempDir()
	baseline := writeFile(t, root, "baseline.json")
	quality := writeFile(t, root, "quality.json")
	review := writeFile(t, root, "review.json")
	manifest := writeFile(t, root, "manifest.json")
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")
	writeQualitySamplePair(t, sampleRoot, "alert_storm", "billing-errors")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	collectedRuntimeArtifacts := filepath.Join(root, "collected-runtime-artifacts")
	outDir := filepath.Join(root, "packet")
	manifestOut := filepath.Join(root, "prepared-quality-manifest.json")

	var stdout bytes.Buffer
	err := run(nil, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + manifestOut,
		"BASELINE_AUDIT=" + baseline,
		"QUALITY_COMPARISON=" + quality,
		"REVIEW_EVIDENCE=" + review,
		"QUALITY_MANIFEST=" + manifest,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + collectedRuntimeArtifacts,
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"REVIEWER=openclarion-maintainer",
		"OUT_DIR=" + outDir,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts").Status; got != "ready" {
		t.Fatalf("runtime smoke artifacts status = %q, want ready", got)
	}
	manifestTarget := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if manifestTarget.Status != "ready" {
		t.Fatalf("quality manifest prepare status = %q, want ready", manifestTarget.Status)
	}
	if len(manifestTarget.QualitySampleChecks) != 1 || manifestTarget.QualitySampleChecks[0].PairedCases != 3 {
		t.Fatalf("quality sample checks = %#v, want three paired cases", manifestTarget.QualitySampleChecks)
	}
	if got := targetByName(t, out, "sandbox-m4-baseline-audit").Status; got != "ready" {
		t.Fatalf("baseline audit status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-quality-compare").Status; got != "ready" {
		t.Fatalf("quality compare status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-review-evidence-template").Status; got != "ready" {
		t.Fatalf("review evidence template status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-decision").Status; got != "ready" {
		t.Fatalf("decision status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-evidence-packet").Status; got != "ready" {
		t.Fatalf("packet status = %q, want ready", got)
	}
}

func TestRunReportsM4QualityManifestSampleReadiness(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")
	writeQualitySamplePair(t, sampleRoot, "alert_storm", "billing-errors")
	outPath := filepath.Join(root, "quality-manifest.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if len(target.QualitySampleChecks) != 1 {
		t.Fatalf("quality sample checks = %#v, want one", target.QualitySampleChecks)
	}
	check := target.QualitySampleChecks[0]
	if check.Status != "ok" || check.DirectReports != 3 || check.SandboxReports != 3 || check.PairedCases != 3 {
		t.Fatalf("quality sample check = %#v, want ok counts", check)
	}
	fileCheck := fileCheckByEnv(t, target.FileChecks, "OUT")
	if fileCheck.Status != "ok" {
		t.Fatalf("OUT check = %#v, want ok", fileCheck)
	}
	if strings.Contains(stdout.String(), sampleRoot) || strings.Contains(stdout.String(), outPath) || strings.Contains(stdout.String(), "payments-cpu") {
		t.Fatalf("output leaked sample path or case id: %s", stdout.String())
	}
}

func TestRunReportsM4QualitySampleExportReadiness(t *testing.T) {
	root := t.TempDir()
	selection := writeFile(t, root, "selection.json")
	outRoot := filepath.Join(root, "exported-quality-samples")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-sample-export"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"SELECTION=" + selection,
		"ROOT=" + outRoot,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-sample-export")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "SELECTION").Status; got != "ok" {
		t.Fatalf("SELECTION status = %q, want ok", got)
	}
	if got := directoryCheckByEnv(t, target.DirectoryChecks, "ROOT").Status; got != "ok" {
		t.Fatalf("ROOT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), "example.test") || strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked environment values: %s", stdout.String())
	}
}

func TestRunReportsM4BaselineAuditReadiness(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "baseline-audit.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-baseline-audit"}, []string{
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-baseline-audit")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "ok" {
		t.Fatalf("OUT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked baseline audit output path: %s", stdout.String())
	}
}

func TestRunBlocksM4BaselineAuditExistingOutputWithoutLeakingPath(t *testing.T) {
	root := t.TempDir()
	outPath := writeFile(t, root, "baseline-audit.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-baseline-audit"}, []string{
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-baseline-audit")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "exists" {
		t.Fatalf("OUT status = %q, want exists", got)
	}
	if strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked baseline audit output path: %s", stdout.String())
	}
}

func TestRunReportsM4QualityCompareReadiness(t *testing.T) {
	root := t.TempDir()
	manifest := writeFile(t, root, "quality-manifest.json")
	outPath := filepath.Join(root, "quality-comparison.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-compare"}, []string{
		"QUALITY_MANIFEST=" + manifest,
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-compare")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "QUALITY_MANIFEST").Status; got != "ok" {
		t.Fatalf("QUALITY_MANIFEST status = %q, want ok", got)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "ok" {
		t.Fatalf("OUT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), manifest) || strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked quality compare paths: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityCompareExistingOutputWithoutLeakingPath(t *testing.T) {
	root := t.TempDir()
	manifest := writeFile(t, root, "quality-manifest.json")
	outPath := writeFile(t, root, "quality-comparison.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-compare"}, []string{
		"QUALITY_MANIFEST=" + manifest,
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-compare")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "exists" {
		t.Fatalf("OUT status = %q, want exists", got)
	}
	if strings.Contains(stdout.String(), manifest) || strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked quality compare paths: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityManifestSampleGapsWithoutLeakingCases(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySampleReport(t, sampleRoot, "direct", "cascade", "checkout-latency")
	if err := os.MkdirAll(filepath.Join(sampleRoot, "sandbox", "cascade"), 0o700); err != nil {
		t.Fatalf("mkdir sandbox cascade: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + filepath.Join(root, "quality-manifest.json"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	check := target.QualitySampleChecks[0]
	if check.Status != "missing_counterparts" {
		t.Fatalf("sample status = %q, want missing_counterparts", check.Status)
	}
	if check.MissingSandboxReports != 1 {
		t.Fatalf("missing sandbox reports = %d, want 1", check.MissingSandboxReports)
	}
	if strings.Contains(stdout.String(), sampleRoot) || strings.Contains(stdout.String(), "checkout-latency") {
		t.Fatalf("output leaked sample path or case id: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityManifestMissingScenario(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + filepath.Join(root, "quality-manifest.json"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	check := target.QualitySampleChecks[0]
	if check.Status != "missing_scenario_coverage" {
		t.Fatalf("sample status = %q, want missing_scenario_coverage", check.Status)
	}
	if !contains(check.MissingScenarios, "alert_storm") {
		t.Fatalf("missing scenarios = %#v, want alert_storm", check.MissingScenarios)
	}
}

func TestRunRejectsBadM4RuntimeSmokeArtifactEnv(t *testing.T) {
	root := t.TempDir()
	artifactsDir := filepath.Join(root, "runtime-artifacts")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + artifactsDir,
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime-candidate-a:latest",
		"OPENCLARION_M4_RUNTIME_SMOKE_PULL=sometimes",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 2 {
		t.Fatalf("invalid env = %#v, want image and pull rejections", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "latest") || strings.Contains(stdout.String(), "sometimes") {
		t.Fatalf("output leaked invalid environment value: %s", stdout.String())
	}
}

func TestRunReportsCustomThinRunnerArtifactAlternative(t *testing.T) {
	root := t.TempDir()
	customArtifacts := filepath.Join(root, "custom-runtime-artifacts")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + filepath.Join(root, "direct-runtime-artifacts"),
		"OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=" + customArtifacts,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked without primary runtime image", target.Status)
	}
	if !contains(target.MissingEnv, "OPENCLARION_AGENT_RUNTIME_IMAGE") {
		t.Fatalf("missing env = %#v, want primary runtime image", target.MissingEnv)
	}
	if len(target.AlternateCommands) != 1 || !strings.Contains(target.AlternateCommands[0].Command, "custom-thin-runner-smoke") {
		t.Fatalf("alternate commands = %#v, want custom thin runner artifact command", target.AlternateCommands)
	}
	check := directoryCheckByEnv(t, target.OptionalDirectoryChecks, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR")
	if check.Status != "ok" {
		t.Fatalf("custom artifact dir status = %q, want ok", check.Status)
	}
	if strings.Contains(stdout.String(), customArtifacts) {
		t.Fatalf("output leaked custom artifact path: %s", stdout.String())
	}
}

func TestRunRejectsReusedCustomThinRunnerArtifactDir(t *testing.T) {
	root := t.TempDir()
	customArtifacts := filepath.Join(root, "custom-runtime-artifacts")
	if err := os.Mkdir(customArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir custom artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customArtifacts, "old.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write old artifact: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + filepath.Join(root, "direct-runtime-artifacts"),
		"OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=" + customArtifacts,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	check := directoryCheckByEnv(t, target.OptionalDirectoryChecks, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR")
	if check.Status != "not_empty" {
		t.Fatalf("custom artifact dir status = %q, want not_empty", check.Status)
	}
	if strings.Contains(stdout.String(), customArtifacts) {
		t.Fatalf("output leaked custom artifact path: %s", stdout.String())
	}
}

func TestRunRejectsBadM4RuntimeCandidateEnv(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE=runtime-candidate-a:latest",
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "RUNTIME_CANDIDATE" {
		t.Fatalf("invalid env = %#v, want runtime candidate rejection", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "latest") {
		t.Fatalf("output leaked runtime candidate value: %s", stdout.String())
	}
}

func TestRunAcceptsM4RuntimeCandidateFile(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	candidateFile := filepath.Join(root, "digest-ref.txt")
	if err := os.WriteFile(candidateFile, []byte("registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatalf("write runtime candidate file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE_FILE=" + candidateFile,
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	check := fileCheckByEnv(t, target.FileChecks, "RUNTIME_CANDIDATE_FILE")
	if check.Status != "ok" {
		t.Fatalf("runtime candidate file status = %q, want ok", check.Status)
	}
	if strings.Contains(stdout.String(), candidateFile) || strings.Contains(stdout.String(), "sha256:0123456789abcdef") {
		t.Fatalf("output leaked runtime candidate file path or value: %s", stdout.String())
	}
}

func TestRunRejectsBadM4RuntimeCandidateFile(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	candidateFile := filepath.Join(root, "digest-ref.txt")
	if err := os.WriteFile(candidateFile, []byte("runtime-candidate-a:latest\n"), 0o600); err != nil {
		t.Fatalf("write runtime candidate file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE_FILE=" + candidateFile,
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "RUNTIME_CANDIDATE_FILE" {
		t.Fatalf("invalid env = %#v, want runtime candidate file rejection", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), candidateFile) || strings.Contains(stdout.String(), "latest") {
		t.Fatalf("output leaked runtime candidate file path or value: %s", stdout.String())
	}
}

func TestRunRejectsIndirectOrReusedM4EvidencePaths(t *testing.T) {
	root := t.TempDir()
	target := writeFile(t, root, "target.json")
	link := filepath.Join(root, "linked.json")
	createSymlinkOrSkip(t, target, link)
	outDir := filepath.Join(root, "packet")
	if err := os.Mkdir(outDir, 0o700); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "old.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write old packet file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-packet"}, []string{
		"QUALITY_MANIFEST=" + link,
		"REVIEW_EVIDENCE=" + target,
		"OUT_DIR=" + outDir,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	packet := targetByName(t, out, "sandbox-m4-evidence-packet")
	if packet.Status != "blocked" {
		t.Fatalf("packet status = %q, want blocked", packet.Status)
	}
	if got := packet.FileChecks[0].Status; got != "not_regular" {
		t.Fatalf("QUALITY_MANIFEST status = %q, want not_regular", got)
	}
	if got := packet.DirectoryChecks[0].Status; got != "not_empty" {
		t.Fatalf("OUT_DIR status = %q, want not_empty", got)
	}
}

func TestRunRejectsBadDiagnosisSnapshotID(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=007",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	diagnosis := targetByName(t, out, "diagnosis-live-browser-smoke")
	if diagnosis.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", diagnosis.Status)
	}
	if len(diagnosis.InvalidEnv) != 1 || diagnosis.InvalidEnv[0].Name != "OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID" {
		t.Fatalf("invalid env = %#v, want snapshot id rejection", diagnosis.InvalidEnv)
	}
}

func TestRunRejectsUnknownTarget(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "unknown"}, nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unknown target error")
	}
	if !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("run err = %v, want unknown target", err)
	}
}

func decodeOutput(t *testing.T, raw []byte) readinessOutput {
	t.Helper()
	var out readinessOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, raw)
	}
	return out
}

func targetByName(t *testing.T, out readinessOutput, name string) targetReadiness {
	t.Helper()
	for _, target := range out.Targets {
		if target.Name == name {
			return target
		}
	}
	t.Fatalf("target %q not found in %#v", name, out.Targets)
	return targetReadiness{}
}

func directoryCheckByEnv(t *testing.T, checks []directoryCheck, name string) directoryCheck {
	t.Helper()
	for _, check := range checks {
		if check.Env == name {
			return check
		}
	}
	t.Fatalf("directory check %q not found in %#v", name, checks)
	return directoryCheck{}
}

func fileCheckByEnv(t *testing.T, checks []fileCheck, name string) fileCheck {
	t.Helper()
	for _, check := range checks {
		if check.Env == name {
			return check
		}
	}
	t.Fatalf("file check %q not found in %#v", name, checks)
	return fileCheck{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func writeQualitySamplePair(t *testing.T, root, scenario, id string) {
	t.Helper()
	writeQualitySampleReport(t, root, directRole, scenario, id)
	writeQualitySampleReport(t, root, sandboxRole, scenario, id)
}

func writeQualitySampleReport(t *testing.T, root, role, scenario, id string) {
	t.Helper()
	dir := filepath.Join(root, role, scenario)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir quality sample dir: %v", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write quality sample report: %v", err)
	}
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
}
