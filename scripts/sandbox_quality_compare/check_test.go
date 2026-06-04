package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOutputsImprovedCandidate(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["alert:cpu"]
	}`)
	sandbox := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU and queue latency indicate customer-impacting degradation.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"},
			{"label": "Check upstream", "detail": "Review checkout error budget for correlated errors.", "priority": "medium"}
		],
		"evidence_refs": ["alert:cpu", "metric:queue", "snapshot:payments"]
	}`)

	out := runCompare(t, []string{
		"--direct-sub-report", direct,
		"--sandbox-sub-report", sandbox,
	})

	if out.Schema != "openclarion_sub_report" {
		t.Fatalf("Schema = %q", out.Schema)
	}
	if out.Recommendation != "sandbox_candidate_improved" {
		t.Fatalf("Recommendation = %q", out.Recommendation)
	}
	if !out.ReviewRequired {
		t.Fatal("ReviewRequired = false, want true")
	}
	if out.Delta.FindingCount != 1 || out.Delta.UniqueEvidenceRefCount != 2 || out.Delta.ConfidenceRank != 1 {
		t.Fatalf("Delta = %+v", out.Delta)
	}
}

func TestRunFailsOnRegressionWhenRequested(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"}
		],
		"evidence_refs": ["alert:cpu", "metric:queue"]
	}`)
	sandbox := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)

	var stdout bytes.Buffer
	err := run([]string{
		"--direct-sub-report", direct,
		"--sandbox-sub-report", sandbox,
		"--fail-on-regression",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want regression error")
	}
	if !strings.Contains(err.Error(), "regressed") {
		t.Fatalf("run err = %v, want regression", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on fail-on-regression", stdout.String())
	}
}

func TestRunOutputsEquivalentMetrics(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["alert:cpu"]
	}`)

	out := runCompare(t, []string{
		"--direct-sub-report", direct,
		"--sandbox-sub-report", direct,
	})
	if out.Recommendation != "equivalent_metrics" {
		t.Fatalf("Recommendation = %q", out.Recommendation)
	}
	if out.Delta != (deltas{}) {
		t.Fatalf("Delta = %+v, want zero", out.Delta)
	}
}

func TestRunManifestOutputsAggregateComparison(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "direct-improved.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu"]
	}`)
	writeFile(t, filepath.Join(dir, "sandbox-improved.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU and queue latency indicate customer-impacting degradation.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"},
			{"label": "Check upstream", "detail": "Review checkout error budget for correlated errors.", "priority": "medium"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu", "metric:queue"]
	}`)
	writeFile(t, filepath.Join(dir, "equivalent.json"), `{
		"title": "Checkout latency",
		"summary": "Checkout latency remains within warning bounds.",
		"severity": "info",
		"confidence": "high",
		"findings": [
			{"label": "Latency", "detail": "p95 latency is elevated but stable.", "evidence_id": "metric:latency"}
		],
		"recommended_actions": [],
		"evidence_refs": ["snapshot:12", "metric:latency"]
	}`)
	writeFile(t, filepath.Join(dir, "equivalent-sandbox.json"), `{
		"title": "Checkout latency",
		"summary": "Checkout latency remains within warning bounds.",
		"severity": "info",
		"confidence": "high",
		"findings": [
			{"label": "Latency", "detail": "p95 latency is elevated but stable.", "evidence_id": "metric:latency"}
		],
		"recommended_actions": [],
		"evidence_refs": ["snapshot:12", "metric:latency"]
	}`)
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "representative alert samples for sandbox quality comparison",
		"cases": [
			{
				"id": "payments-cpu",
				"scenario": "single_alert",
				"required_evidence_refs": ["snapshot:11", "alert:cpu"],
				"direct_sub_report": "direct-improved.json",
				"sandbox_sub_report": "sandbox-improved.json"
			},
			{
				"id": "checkout-latency",
				"scenario": "cascade",
				"required_evidence_refs": ["snapshot:12", "metric:latency"],
				"direct_sub_report": "equivalent.json",
				"sandbox_sub_report": "equivalent-sandbox.json"
			}
		]
	}`)

	var stdout bytes.Buffer
	if err := run([]string{"--manifest", manifest}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out batchComparisonOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Mode != "manifest" {
		t.Fatalf("Mode = %q", out.Mode)
	}
	if out.CaseCount != 2 || len(out.Cases) != 2 {
		t.Fatalf("CaseCount = %d len(Cases) = %d", out.CaseCount, len(out.Cases))
	}
	if out.SampleBasis != "representative alert samples for sandbox quality comparison" {
		t.Fatalf("SampleBasis = %q", out.SampleBasis)
	}
	if got, want := strings.Join(out.ScenarioCoverage, ","), "single_alert,cascade"; got != want {
		t.Fatalf("ScenarioCoverage = %v, want %s", out.ScenarioCoverage, want)
	}
	if out.Recommendation != "sandbox_batch_candidate_improved" {
		t.Fatalf("Recommendation = %q", out.Recommendation)
	}
	if out.Summary.ImprovedCount != 1 || out.Summary.EquivalentCount != 1 || out.Summary.RegressedCount != 0 {
		t.Fatalf("Summary = %+v", out.Summary)
	}
	if out.Cases[0].ID != "payments-cpu" || out.Cases[0].Recommendation != "sandbox_candidate_improved" {
		t.Fatalf("Cases[0] = %+v", out.Cases[0])
	}
	if out.Cases[0].Scenario != "single_alert" {
		t.Fatalf("Cases[0].Scenario = %q", out.Cases[0].Scenario)
	}
	if got, want := strings.Join(out.Cases[0].RequiredEvidenceRefs, ","), "snapshot:11,alert:cpu"; got != want {
		t.Fatalf("Cases[0].RequiredEvidenceRefs = %v, want %s", out.Cases[0].RequiredEvidenceRefs, want)
	}
}

func TestRunManifestWritesOutputFile(t *testing.T) {
	manifest := writeOneCaseImprovedManifest(t)
	outPath := filepath.Join(t.TempDir(), "quality-comparison.json")

	var stdout bytes.Buffer
	if err := run([]string{"--manifest", manifest, "--out", outPath}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when --out is used", stdout.String())
	}
	// #nosec G304 -- test reads the temp output path produced by this run.
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var out batchComparisonOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output %q: %v", raw, err)
	}
	if out.Mode != "manifest" || out.CaseCount != 1 || out.Summary.ImprovedCount != 1 {
		t.Fatalf("output = %+v, want one improved manifest case", out)
	}
}

func TestRunRejectsExistingOutputFile(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)
	outPath := filepath.Join(t.TempDir(), "quality-comparison.json")
	writeFile(t, outPath, "{}\n")

	var stdout bytes.Buffer
	err := run([]string{
		"--direct-sub-report", direct,
		"--sandbox-sub-report", direct,
		"--out", outPath,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want existing output rejection")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("run err = %v, want already exists", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on output rejection", stdout.String())
	}
}

func TestRunRejectsSymlinkManifest(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "representative alert sample",
		"cases": [
			{"id":"payments-cpu", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
		]
	}`)
	manifestLink := filepath.Join(dir, "quality-manifest-link.json")
	createSymlinkOrSkip(t, manifest, manifestLink)

	var stdout bytes.Buffer
	err := run([]string{"--manifest", manifestLink}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink manifest rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on symlink manifest", stdout.String())
	}
}

func TestRunRejectsSymlinkSubReport(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["alert:cpu"]
	}`)
	directLink := filepath.Join(t.TempDir(), "direct-link.json")
	createSymlinkOrSkip(t, direct, directLink)

	var stdout bytes.Buffer
	err := run([]string{
		"--direct-sub-report", directLink,
		"--sandbox-sub-report", direct,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink subreport rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on symlink subreport", stdout.String())
	}
}

func TestRunManifestFailsOnRegressionWhenRequested(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "direct.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu", "metric:queue"]
	}`)
	writeFile(t, filepath.Join(dir, "sandbox.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["snapshot:11", "alert:cpu"]
	}`)
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "regression sample",
		"cases": [
			{"id": "payments-regression", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "direct_sub_report": "direct.json", "sandbox_sub_report": "sandbox.json"}
		]
	}`)

	var stdout bytes.Buffer
	err := run([]string{"--manifest", manifest, "--fail-on-regression"}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want regression error")
	}
	if !strings.Contains(err.Error(), "1 manifest case") {
		t.Fatalf("run err = %v, want manifest regression count", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on fail-on-regression", stdout.String())
	}
}

func TestRunManifestRegressionWithOutputDoesNotWriteFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "direct.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu", "metric:queue"]
	}`)
	writeFile(t, filepath.Join(dir, "sandbox.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["snapshot:11", "alert:cpu"]
	}`)
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "regression sample",
		"cases": [
			{"id": "payments-regression", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "direct_sub_report": "direct.json", "sandbox_sub_report": "sandbox.json"}
		]
	}`)
	outPath := filepath.Join(dir, "quality-comparison.json")

	var stdout bytes.Buffer
	err := run([]string{"--manifest", manifest, "--fail-on-regression", "--out", outPath}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want regression error")
	}
	if !strings.Contains(err.Error(), "1 manifest case") {
		t.Fatalf("run err = %v, want manifest regression count", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on fail-on-regression", stdout.String())
	}
	if _, err := os.Lstat(outPath); !os.IsNotExist(err) {
		t.Fatalf("output stat err = %v, want missing output file", err)
	}
}

func TestRunManifestRejectsEvidenceRefMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "direct.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["snapshot:11", "alert:cpu"]
	}`)
	writeFile(t, filepath.Join(dir, "sandbox.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "evidence-bound representative sample",
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "direct_sub_report": "direct.json", "sandbox_sub_report": "sandbox.json"}
		]
	}`)

	var stdout bytes.Buffer
	err := run([]string{"--manifest", manifest}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want required evidence ref mismatch")
	}
	if !strings.Contains(err.Error(), `sandbox subreport missing required evidence ref "snapshot:11"`) {
		t.Fatalf("run err = %v, want sandbox missing required evidence ref", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on required evidence ref mismatch", stdout.String())
	}
}

func TestRunManifestRejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{
			name:     "empty cases",
			manifest: `{"sample_basis":"empty sample","cases":[]}`,
			wantErr:  "at least one",
		},
		{
			name: "missing sample basis",
			manifest: `{
				"cases": [
					{"id":"one", "scenario":"single_alert", "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "sample_basis",
		},
		{
			name: "whitespace padded sample basis",
			manifest: `{
				"sample_basis": " representative sample ",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "sample_basis must not contain leading or trailing whitespace",
		},
		{
			name: "multiline sample basis",
			manifest: `{
				"sample_basis": "representative sample\ncontinued note",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "sample_basis must be a single-line value",
		},
		{
			name: "oversized sample basis",
			manifest: `{
				"sample_basis": "` + strings.Repeat("a", maxManifestSampleBasisBytes+1) + `",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "sample_basis exceeds 2048 bytes",
		},
		{
			name: "duplicate manifest key",
			manifest: `{
				"sample_basis": "stale sample",
				"sample_basis": "representative sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "duplicate object key",
		},
		{
			name: "unknown manifest field",
			manifest: `{
				"sample_basis": "representative sample",
				"unexpected": "stale evidence",
				"cases": [
					{"id":"one", "scenario":"single_alert", "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: `unknown field "unexpected"`,
		},
		{
			name: "missing required evidence refs",
			manifest: `{
				"sample_basis": "missing required evidence refs sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "required_evidence_refs",
		},
		{
			name: "duplicate required evidence refs",
			manifest: `{
				"sample_basis": "duplicate required evidence refs sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "duplicate required_evidence_refs",
		},
		{
			name: "missing snapshot evidence ref",
			manifest: `{
				"sample_basis": "missing snapshot evidence ref sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "must include one snapshot:<positive-id> reference",
		},
		{
			name: "invalid snapshot evidence ref",
			manifest: `{
				"sample_basis": "invalid snapshot evidence ref sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:001", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "snapshot:<positive-id>",
		},
		{
			name: "multiline required evidence ref",
			manifest: `{
				"sample_basis": "multiline required evidence ref sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu\ncontinued"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "required_evidence_refs[1] must be a single-line value",
		},
		{
			name: "duplicate id",
			manifest: `{
				"sample_basis": "duplicate sample",
				"cases": [
					{"id":"same", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"},
					{"id":"same", "scenario":"cascade", "required_evidence_refs":["snapshot:12", "alert:latency"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "duplicate",
		},
		{
			name: "multiline case id",
			manifest: `{
				"sample_basis": "multiline case id sample",
				"cases": [
					{"id":"one\ncontinued", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "id must be a single-line value",
		},
		{
			name: "oversized case id",
			manifest: `{
				"sample_basis": "oversized case id sample",
				"cases": [
					{"id":"` + strings.Repeat("a", maxManifestCaseIDBytes+1) + `", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "id exceeds 128 bytes",
		},
		{
			name: "unsupported scenario",
			manifest: `{
				"sample_basis": "unsupported scenario sample",
				"cases": [
					{"id":"bad-scenario", "scenario":"business_signal", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "unsupported",
		},
		{
			name: "missing sandbox path",
			manifest: `{
				"sample_basis": "missing sandbox sample",
				"cases": [
					{"id":"missing-sandbox", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json"}
				]
			}`,
			wantErr: "sandbox_sub_report",
		},
		{
			name: "absolute path",
			manifest: `{
				"sample_basis": "absolute sample path",
				"cases": [
					{"id":"absolute-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"/tmp/direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "must be relative",
		},
		{
			name: "parent traversal",
			manifest: `{
				"sample_basis": "traversal sample path",
				"cases": [
					{"id":"traversal-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"../direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "parent directory traversal",
		},
		{
			name: "same direct sandbox path",
			manifest: `{
				"sample_basis": "same sample path",
				"cases": [
					{"id":"same-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"same.json", "sandbox_sub_report":"./same.json"}
				]
			}`,
			wantErr: "must be distinct",
		},
		{
			name: "multiline report path",
			manifest: `{
				"sample_basis": "multiline report path sample",
				"cases": [
					{"id":"multiline-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct\none.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "direct_sub_report must be a single-line slash-separated relative path",
		},
		{
			name: "oversized report path",
			manifest: `{
				"sample_basis": "oversized report path sample",
				"cases": [
					{"id":"oversized-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"` + strings.Repeat("a", maxManifestReportPathBytes+1) + `.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "direct_sub_report exceeds 512 bytes",
		},
		{
			name: "reused direct report path across cases",
			manifest: `{
				"sample_basis": "reused direct path sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct.json", "sandbox_sub_report":"sandbox-one.json"},
					{"id":"two", "scenario":"cascade", "required_evidence_refs":["snapshot:12", "alert:latency"], "direct_sub_report":"./direct.json", "sandbox_sub_report":"sandbox-two.json"}
				]
			}`,
			wantErr: "repeats report path",
		},
		{
			name: "reused sandbox report path across cases",
			manifest: `{
				"sample_basis": "reused sandbox path sample",
				"cases": [
					{"id":"one", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"direct-one.json", "sandbox_sub_report":"sandbox.json"},
					{"id":"two", "scenario":"cascade", "required_evidence_refs":["snapshot:12", "alert:latency"], "direct_sub_report":"direct-two.json", "sandbox_sub_report":"./sandbox.json"}
				]
			}`,
			wantErr: "repeats report path",
		},
		{
			name: "drive path",
			manifest: `{
				"sample_basis": "drive sample path",
				"cases": [
					{"id":"drive-path", "scenario":"single_alert", "required_evidence_refs":["snapshot:11", "alert:cpu"], "direct_sub_report":"C:/tmp/direct.json", "sandbox_sub_report":"sandbox.json"}
				]
			}`,
			wantErr: "slash-separated relative path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := writeSubReport(t, tt.manifest)
			var stdout bytes.Buffer
			err := run([]string{"--manifest", manifest}, &stdout)
			if err == nil {
				t.Fatal("run err = nil, want manifest error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsMixedManifestAndSinglePairFlags(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{
		"--manifest", "manifest.json",
		"--direct-sub-report", "direct.json",
		"--sandbox-sub-report", "sandbox.json",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want mixed flag error")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("run err = %v, want cannot be combined", err)
	}
}

func TestRunRejectsInvalidSubReport(t *testing.T) {
	invalid := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Missing findings violates the SubReport schema.",
		"severity": "warning",
		"confidence": "high",
		"recommended_actions": [],
		"evidence_refs": []
	}`)
	valid := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)

	var stdout bytes.Buffer
	err := run([]string{
		"--direct-sub-report", invalid,
		"--sandbox-sub-report", valid,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "direct subreport") {
		t.Fatalf("run err = %v, want direct subreport context", err)
	}
}

func TestRunRejectsDuplicateSubReportKeys(t *testing.T) {
	direct := writeSubReport(t, `{
		"title": "stale title",
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)
	valid := writeSubReport(t, `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [],
		"evidence_refs": ["alert:cpu"]
	}`)

	var stdout bytes.Buffer
	err := run([]string{
		"--direct-sub-report", direct,
		"--sandbox-sub-report", valid,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate key rejection")
	}
	if !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("run err = %v, want duplicate key error", err)
	}
}

func TestRunRejectsMissingRequiredFlags(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want flag error")
	}
	if !strings.Contains(err.Error(), "--manifest") {
		t.Fatalf("run err = %v, want manifest/direct flag error", err)
	}
}

func runCompare(t *testing.T, args []string) comparisonOutput {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out comparisonOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	return out
}

func writeSubReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "subreport.json")
	writeFile(t, path, content)
	return path
}

func writeOneCaseImprovedManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "direct.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU exceeded the warning threshold.",
		"severity": "warning",
		"confidence": "medium",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu"]
	}`)
	writeFile(t, filepath.Join(dir, "sandbox.json"), `{
		"title": "Payments CPU saturation",
		"summary": "Payments CPU and queue latency indicate customer-impacting degradation.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:cpu"},
			{"label": "Queue latency", "detail": "Queue latency increased while CPU stayed saturated.", "evidence_id": "metric:queue"}
		],
		"recommended_actions": [
			{"label": "Scale payments", "detail": "Add one payments replica and monitor queue latency.", "priority": "high"}
		],
		"evidence_refs": ["snapshot:11", "alert:cpu", "metric:queue"]
	}`)
	manifest := filepath.Join(dir, "quality-manifest.json")
	writeFile(t, manifest, `{
		"sample_basis": "representative alert sample",
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "direct_sub_report": "direct.json", "sandbox_sub_report": "sandbox.json"}
		]
	}`)
	return manifest
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
