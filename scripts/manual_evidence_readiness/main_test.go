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
	outDir := filepath.Join(root, "packet")

	var stdout bytes.Buffer
	err := run(nil, []string{
		"BASELINE_AUDIT=" + baseline,
		"QUALITY_COMPARISON=" + quality,
		"REVIEW_EVIDENCE=" + review,
		"QUALITY_MANIFEST=" + manifest,
		"OUT_DIR=" + outDir,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "sandbox-m4-decision").Status; got != "ready" {
		t.Fatalf("decision status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-evidence-packet").Status; got != "ready" {
		t.Fatalf("packet status = %q, want ready", got)
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

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
}
