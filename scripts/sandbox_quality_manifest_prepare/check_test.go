package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesManifestFromPairedTree(t *testing.T) {
	root := t.TempDir()
	writeCasePair(t, root, "single_alert", "payments-cpu", "11", "alert:cpu", "metric:queue")
	writeCasePair(t, root, "cascade", "checkout-latency", "12", "metric:latency", "metric:errors")
	writeCasePair(t, root, "alert_storm", "billing-errors", "13", "alert:errors", "metric:rate")

	var stdout bytes.Buffer
	if err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert samples from retained replay window 2026-06-04",
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	var manifest manifestFile
	if err := json.NewDecoder(&stdout).Decode(&manifest); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if manifest.SampleBasis != "representative alert samples from retained replay window 2026-06-04" {
		t.Fatalf("SampleBasis = %q", manifest.SampleBasis)
	}
	if len(manifest.Cases) != 3 {
		t.Fatalf("Cases len = %d, want 3", len(manifest.Cases))
	}
	want := []manifestCase{
		{
			ID:                   "payments-cpu",
			Scenario:             "single_alert",
			RequiredEvidenceRefs: []string{"snapshot:11", "alert:cpu"},
			DirectSubReport:      "direct/single_alert/payments-cpu.json",
			SandboxSubReport:     "sandbox/single_alert/payments-cpu.json",
		},
		{
			ID:                   "checkout-latency",
			Scenario:             "cascade",
			RequiredEvidenceRefs: []string{"snapshot:12", "metric:latency"},
			DirectSubReport:      "direct/cascade/checkout-latency.json",
			SandboxSubReport:     "sandbox/cascade/checkout-latency.json",
		},
		{
			ID:                   "billing-errors",
			Scenario:             "alert_storm",
			RequiredEvidenceRefs: []string{"snapshot:13", "alert:errors"},
			DirectSubReport:      "direct/alert_storm/billing-errors.json",
			SandboxSubReport:     "sandbox/alert_storm/billing-errors.json",
		},
	}
	for i := range want {
		assertCase(t, manifest.Cases[i], want[i])
	}
}

func TestRunWritesManifestToNewOutputFile(t *testing.T) {
	root := t.TempDir()
	writeCasePair(t, root, "single_alert", "payments-cpu", "11", "alert:cpu", "metric:queue")
	writeCasePair(t, root, "cascade", "checkout-latency", "12", "metric:latency", "metric:errors")
	writeCasePair(t, root, "alert_storm", "billing-errors", "13", "alert:errors", "metric:rate")
	out := filepath.Join(root, "quality-manifest.json")

	var stdout bytes.Buffer
	if err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert samples from retained replay window 2026-06-04",
		"--out", out,
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when --out is used", stdout.String())
	}
	// #nosec G304 -- test reads the output path it just passed to the helper.
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var manifest manifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse output %q: %v", string(raw), err)
	}
	if len(manifest.Cases) != 3 {
		t.Fatalf("Cases len = %d, want 3", len(manifest.Cases))
	}
}

func TestRunRejectsMissingCounterpart(t *testing.T) {
	root := t.TempDir()
	writeReport(t, root, directRole, "single_alert", "payments-cpu", "11", "alert:cpu")
	if err := os.MkdirAll(filepath.Join(root, sandboxRole, "single_alert"), 0o700); err != nil {
		t.Fatalf("mkdir sandbox role: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert sample",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing counterpart error")
	}
	if !strings.Contains(err.Error(), "missing sandbox report") {
		t.Fatalf("run err = %v, want missing sandbox report", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on error", stdout.String())
	}
}

func TestRunRejectsMissingScenarioCoverage(t *testing.T) {
	root := t.TempDir()
	writeCasePair(t, root, "single_alert", "payments-cpu", "11", "alert:cpu", "metric:queue")
	writeCasePair(t, root, "cascade", "checkout-latency", "12", "metric:latency", "metric:errors")

	var stdout bytes.Buffer
	err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert samples from retained replay window 2026-06-04",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing scenario coverage error")
	}
	if !strings.Contains(err.Error(), "alert_storm") {
		t.Fatalf("run err = %v, want alert_storm coverage error", err)
	}
}

func TestRunRejectsCaseWithoutSharedSnapshotRef(t *testing.T) {
	root := t.TempDir()
	writeReport(t, root, directRole, "single_alert", "payments-cpu", "11", "alert:cpu")
	writeReport(t, root, sandboxRole, "single_alert", "payments-cpu", "99", "alert:cpu")
	writeCasePair(t, root, "cascade", "checkout-latency", "12", "metric:latency", "metric:errors")
	writeCasePair(t, root, "alert_storm", "billing-errors", "13", "alert:errors", "metric:rate")

	var stdout bytes.Buffer
	err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert samples from retained replay window 2026-06-04",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want shared snapshot error")
	}
	if !strings.Contains(err.Error(), "must share at least one snapshot:<positive-id>") {
		t.Fatalf("run err = %v, want shared snapshot error", err)
	}
}

func TestRunRejectsSymlinkReport(t *testing.T) {
	root := t.TempDir()
	target := writeReport(t, root, directRole, "single_alert", "payments-cpu-target", "11", "alert:cpu")
	link := filepath.Join(root, directRole, "single_alert", "payments-cpu.json")
	createSymlinkOrSkip(t, target, link)
	writeReport(t, root, sandboxRole, "single_alert", "payments-cpu", "11", "alert:cpu")

	var stdout bytes.Buffer
	err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert sample",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must not contain symlinks") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
}

func TestRunRejectsExistingOutput(t *testing.T) {
	root := t.TempDir()
	writeCasePair(t, root, "single_alert", "payments-cpu", "11", "alert:cpu", "metric:queue")
	writeCasePair(t, root, "cascade", "checkout-latency", "12", "metric:latency", "metric:errors")
	writeCasePair(t, root, "alert_storm", "billing-errors", "13", "alert:errors", "metric:rate")
	out := filepath.Join(root, "quality-manifest.json")
	writeFile(t, out, "{}")

	var stdout bytes.Buffer
	err := run([]string{
		"--root", root,
		"--sample-basis", "representative alert samples from retained replay window 2026-06-04",
		"--out", out,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want existing output rejection")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("run err = %v, want already exists", err)
	}
}

func assertCase(t *testing.T, got, want manifestCase) {
	t.Helper()
	if got.ID != want.ID || got.Scenario != want.Scenario || got.DirectSubReport != want.DirectSubReport || got.SandboxSubReport != want.SandboxSubReport {
		t.Fatalf("case = %+v, want %+v", got, want)
	}
	if strings.Join(got.RequiredEvidenceRefs, ",") != strings.Join(want.RequiredEvidenceRefs, ",") {
		t.Fatalf("case %q refs = %v, want %v", got.ID, got.RequiredEvidenceRefs, want.RequiredEvidenceRefs)
	}
}

func writeCasePair(t *testing.T, root, scenario, id, snapshotID, commonRef, sandboxOnlyRef string) {
	t.Helper()
	writeReport(t, root, directRole, scenario, id, snapshotID, commonRef)
	writeReportWithExtra(t, root, sandboxRole, scenario, id, snapshotID, commonRef, sandboxOnlyRef)
}

func writeReport(t *testing.T, root, role, scenario, id, snapshotID, ref string) string {
	t.Helper()
	return writeReportWithExtra(t, root, role, scenario, id, snapshotID, ref, "")
}

func writeReportWithExtra(t *testing.T, root, role, scenario, id, snapshotID, ref, extraRef string) string {
	t.Helper()
	dir := filepath.Join(root, role, scenario)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	findings := `{"label":"Primary signal","detail":"Primary signal stayed above threshold.","evidence_id":"` + ref + `"}`
	refs := `"snapshot:` + snapshotID + `","` + ref + `"`
	if extraRef != "" {
		findings += `,{"label":"Extra signal","detail":"Extra signal adds candidate context.","evidence_id":"` + extraRef + `"}`
		refs += `,"` + extraRef + `"`
	}
	path := filepath.Join(dir, id+".json")
	writeFile(t, path, `{
		"title": "Retained alert sample",
		"summary": "Retained sample output for manifest preparation.",
		"severity": "warning",
		"confidence": "high",
		"findings": [`+findings+`],
		"recommended_actions": [
			{"label": "Inspect service", "detail": "Inspect the affected service and validate mitigation.", "priority": "medium"}
		],
		"evidence_refs": [`+refs+`]
	}`)
	return path
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}
