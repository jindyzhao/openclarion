package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	todayUTC = func() time.Time {
		return time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	}
}

const runtimeCandidateRef = "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestRunOutputsProceedDecision(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	if err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out decisionOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Tool != "sandbox_m4_decision" {
		t.Fatalf("Tool = %q", out.Tool)
	}
	if out.Decision != "proceed" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !out.ReviewRequired {
		t.Fatal("ReviewRequired = false, want true")
	}
	if out.Evidence.CaseCount != 3 || out.Evidence.RuntimeSmokePassedCount != len(requiredRuntimeSmokes) {
		t.Fatalf("Evidence = %+v", out.Evidence)
	}
	if out.Evidence.SampleBasis != "single-alert, cascade, and alert-storm representative alert cases" {
		t.Fatalf("Evidence.SampleBasis = %q", out.Evidence.SampleBasis)
	}
	if out.Evidence.SelectedCandidate != "runtime-candidate-a" || out.Evidence.CandidateEvaluationCount != 3 {
		t.Fatalf("Evidence candidate summary = %+v", out.Evidence)
	}
	if got, want := strings.Join(out.Evidence.ScenarioCoverage, ","), "single_alert,cascade,alert_storm"; got != want {
		t.Fatalf("ScenarioCoverage = %v, want %s", out.Evidence.ScenarioCoverage, want)
	}
}

func TestRunRejectsSymlinkEvidenceFiles(t *testing.T) {
	tests := []struct {
		name    string
		replace string
	}{
		{name: "baseline", replace: "baseline"},
		{name: "quality", replace: "quality"},
		{name: "review", replace: "review"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
			quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
			review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())
			link := filepath.Join(dir, tc.replace+"-link.json")
			switch tc.replace {
			case "baseline":
				createSymlinkOrSkip(t, baseline, link)
				baseline = link
			case "quality":
				createSymlinkOrSkip(t, quality, link)
				quality = link
			case "review":
				createSymlinkOrSkip(t, review, link)
				review = link
			}

			var stdout bytes.Buffer
			err := run([]string{
				"--baseline-audit", baseline,
				"--quality-comparison", quality,
				"--review-evidence", review,
			}, &stdout)
			if err == nil {
				t.Fatal("run err = nil, want symlink evidence rejection")
			}
			if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
				t.Fatalf("run err = %v, want symlink rejection", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty on symlink evidence", stdout.String())
			}
		})
	}
}

func TestRunDefersWhenCandidateEvaluationsAreMissing(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	reviewRaw := passingReviewEvidenceJSON()
	start := strings.Index(reviewRaw, `"candidate_evaluations": [`)
	end := strings.Index(reviewRaw, `"runtime_smokes": [`)
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("test fixture missing candidate_evaluations/runtime_smokes fields")
	}
	review := writeEvidence(t, dir, "review.json", reviewRaw[:start]+reviewRaw[end:])

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, "candidate_evaluations must contain at least one candidate") {
		t.Fatalf("Reasons = %v, want missing candidate evaluations reason", out.Reasons)
	}
}

func TestRunDefersWhenSelectedCandidateIsMissingFromEvaluations(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"selected_candidate": "runtime-candidate-a"`,
		`"selected_candidate": "not-reviewed-runtime"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `selected_candidate "not-reviewed-runtime" must have a matching candidate_evaluations entry`) {
		t.Fatalf("Reasons = %v, want selected-candidate mismatch reason", out.Reasons)
	}
}

func TestRunRejectsWhitespacePaddedSelectedCandidate(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"selected_candidate": "runtime-candidate-a"`,
		`"selected_candidate": " runtime-candidate-a "`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want whitespace-padded selected_candidate rejection")
	}
	if !strings.Contains(err.Error(), "selected_candidate must not contain leading or trailing whitespace") {
		t.Fatalf("run err = %v, want selected_candidate whitespace error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunDefersWhenSelectedCandidateRuntimeCandidateIsMissing(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`, "runtime_candidate": "`+runtimeCandidateRef+`"`,
		"",
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `candidate "runtime-candidate-a" runtime_candidate is required when status is pass`) {
		t.Fatalf("Reasons = %v, want selected candidate runtime ref reason", out.Reasons)
	}
}

func TestRunDefersWhenSelectedCandidateRuntimeCandidateDoesNotMatchTopLevel(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	otherRef := "localhost:5000/openclarion/other-runtime@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"runtime_candidate": "`+runtimeCandidateRef+`", "runtime_smoke_refs": [`,
		`"runtime_candidate": "`+otherRef+`", "runtime_smoke_refs": [`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `selected candidate "runtime-candidate-a" runtime_candidate`) {
		t.Fatalf("Reasons = %v, want selected candidate runtime mismatch reason", out.Reasons)
	}
}

func TestRunDefersWhenSelectedCandidateRuntimeSmokeRefsAreMissing(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`, "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"]`,
		"",
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `candidate "runtime-candidate-a" runtime_smoke_refs must include "candidate_runtime_file_contract"`) {
		t.Fatalf("Reasons = %v, want runtime smoke ref reason", out.Reasons)
	}
}

func TestRunIteratesWhenSelectedCandidateEvaluationDoesNotPass(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`{"candidate": "runtime-candidate-a", "status": "pass", "runtime_candidate": "`+runtimeCandidateRef+`", "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"], "source": "candidate runtime smoke review", "notes": "candidate runtime passed contract and lifecycle smoke as retained review evidence"}`,
		`{"candidate": "runtime-candidate-a", "status": "fail", "runtime_candidate": "`+runtimeCandidateRef+`", "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"], "source": "candidate runtime smoke review", "notes": "candidate runtime smoke failed"}`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "iterate" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `selected candidate "runtime-candidate-a" evaluation status = "fail", want pass`) {
		t.Fatalf("Reasons = %v, want selected candidate failure reason", out.Reasons)
	}
}

func TestRunRejectsTagOnlyRuntimeCandidate(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		runtimeCandidateRef,
		"runtime-candidate-a:latest",
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want tag-only runtime candidate rejection")
	}
	if !strings.Contains(err.Error(), "runtime_candidate must be an immutable image reference") {
		t.Fatalf("run err = %v, want immutable runtime candidate error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunDefersWhenRuntimeCandidateUsesLoopbackRegistry(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	loopbackRef := "localhost:5000/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	review := writeEvidence(t, dir, "review.json", strings.ReplaceAll(passingReviewEvidenceJSON(), runtimeCandidateRef, loopbackRef))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, loopbackRuntimeCandidateReason) {
		t.Fatalf("Reasons = %v, want loopback runtime candidate reason", out.Reasons)
	}
}

func TestLoopbackImageReference(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{
			name: "localhost registry",
			ref:  "localhost:5000/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: true,
		},
		{
			name: "ipv4 loopback registry",
			ref:  "127.0.0.1:5000/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: true,
		},
		{
			name: "ipv6 loopback registry",
			ref:  "[::1]:5000/openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: true,
		},
		{
			name: "remote registry",
			ref:  runtimeCandidateRef,
			want: false,
		},
		{
			name: "docker hub implicit registry",
			ref:  "openclarion/runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := loopbackImageReference(tc.ref); got != tc.want {
				t.Fatalf("loopbackImageReference(%q) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

func TestRunRejectsDuplicateCandidateRuntimeSmokeRefs(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"egress_allowdeny"], "source": "candidate runtime smoke review"`,
		`"container_provider_lifecycle"], "source": "candidate runtime smoke review"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate candidate runtime smoke ref rejection")
	}
	if !strings.Contains(err.Error(), `duplicate runtime_smoke_refs value "container_provider_lifecycle"`) {
		t.Fatalf("run err = %v, want duplicate runtime smoke ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsUnexpectedRuntimeSmokeName(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`{"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "5555555555555555555555555555555555555555555555555555555555555555"}`,
		`{"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "5555555555555555555555555555555555555555555555555555555555555555"},
				{"name": "manual_runtime_note", "status": "pass", "source": "manual note"}`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unexpected runtime smoke name rejection")
	}
	if !strings.Contains(err.Error(), `runtime_smokes[5].name = "manual_runtime_note" is not a required runtime smoke`) {
		t.Fatalf("run err = %v, want unexpected runtime smoke name error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsRuntimeSmokeWithoutEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`, "evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		"",
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_ref is required`) {
		t.Fatalf("run err = %v, want runtime smoke evidence_ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsRuntimeSmokeInvalidEvidenceSHA(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"evidence_sha256": "1111111111111111111111111111111111111111111111111111111111111111"`,
		`"evidence_sha256": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want invalid runtime smoke evidence_sha256 rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_sha256 must be 64 lowercase hex characters`) {
		t.Fatalf("run err = %v, want runtime smoke evidence_sha256 error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsRuntimeSmokeTraversalEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		`"evidence_ref": "artifacts/m4/runtime/../agent-runtime-smoke.json"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want traversal runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_ref must be a normalized relative artifact path`) {
		t.Fatalf("run err = %v, want normalized evidence_ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsDuplicateRuntimeSmokeEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"evidence_ref": "artifacts/m4/runtime/container-provider-smoke.json"`,
		`"evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `evidence_ref "artifacts/m4/runtime/agent-runtime-smoke.json" duplicates another runtime smoke`) {
		t.Fatalf("run err = %v, want duplicate evidence_ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunDefersWhenHumanReviewNotesAreMissing(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		",\n\t\t\t\"notes\": \"sample reports preserve evidence traceability and improve operational usefulness\"",
		"",
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, "human_review.notes is required") {
		t.Fatalf("Reasons = %v, want missing notes reason", out.Reasons)
	}
}

func TestRunRejectsWhitespacePaddedHumanReviewNotes(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"notes": "sample reports preserve evidence traceability and improve operational usefulness"`,
		`"notes": " sample reports preserve evidence traceability and improve operational usefulness "`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want whitespace-padded human_review.notes rejection")
	}
	if !strings.Contains(err.Error(), "human_review.notes must not contain leading or trailing whitespace") {
		t.Fatalf("run err = %v, want notes whitespace error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsMultilineHumanReviewNotes(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"notes": "sample reports preserve evidence traceability and improve operational usefulness"`,
		`"notes": "sample reports preserve evidence traceability and improve operational usefulness\ncontinued note"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want multiline human_review.notes rejection")
	}
	if !strings.Contains(err.Error(), "human_review.notes must be a single-line value") {
		t.Fatalf("run err = %v, want single-line notes error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsOversizedCandidateEvaluationNotes(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		"candidate runtime passed contract and lifecycle smoke as retained review evidence",
		strings.Repeat("a", maxReviewEvidenceTextBytes+1),
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want oversized candidate notes rejection")
	}
	if !strings.Contains(err.Error(), `candidate_evaluations candidate "runtime-candidate-a" notes exceeds 2048 bytes`) {
		t.Fatalf("run err = %v, want oversized candidate notes error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsDuplicateEvidenceKeys(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "wrong_schema",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate key rejection")
	}
	if !strings.Contains(err.Error(), `duplicate object key "schema"`) {
		t.Fatalf("run err = %v, want duplicate key error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsDuplicateBaselineAuditChecks(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", strings.Replace(
		passingBaselineAuditJSON(),
		`{"name": "raw_result_validation", "status": "pass"}`,
		`{"name": "fixed_file_contract", "status": "pass"}`,
		1,
	))
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want duplicate baseline check rejection")
	}
	if !strings.Contains(err.Error(), `duplicate baseline audit check "fixed_file_contract"`) {
		t.Fatalf("run err = %v, want duplicate baseline check error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsUnknownEvidenceFields(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(passingQualityComparisonJSON(), `"mode": "manifest"`, `"unexpected": "stale evidence", "mode": "manifest"`, 1))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unknown field rejection")
	}
	if !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("run err = %v, want unknown field error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunDefersWhenSampleIsTooSmall(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 1,
		"sample_basis": "single-alert exploratory alert sample",
		"scenario_coverage": ["single_alert"],
		"summary": {
			"improved_count": 1,
			"equivalent_count": 0,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, "below required minimum") {
		t.Fatalf("Reasons = %v, want sample-size reason", out.Reasons)
	}
}

func TestRunDefersWhenQualityScenarioCoverageIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert and cascade samples without alert storm coverage",
		"scenario_coverage": ["single_alert", "cascade"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "cascade", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `missing required scenario "alert_storm"`) {
		t.Fatalf("Reasons = %v, want alert_storm coverage reason", out.Reasons)
	}
}

func TestRunIteratesOnRegressionAndFailUnless(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 1,
			"equivalent_count": 1,
			"regressed_count": 1,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_has_regressions",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "equivalent_metrics", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "sandbox_candidate_regressed", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
		"--fail-unless", "proceed",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want fail-unless error")
	}
	if !strings.Contains(err.Error(), `M4 decision = "iterate"`) {
		t.Fatalf("run err = %v, want iterate decision mismatch", err)
	}
	var out decisionOutput
	if decodeErr := json.NewDecoder(&stdout).Decode(&out); decodeErr != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), decodeErr)
	}
	if out.Decision != "iterate" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, "regressed case") {
		t.Fatalf("Reasons = %v, want regression reason", out.Reasons)
	}
}

func TestRunIteratesWhenRuntimeSmokeSourceIsNotCanonical(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"source": "make container-provider-smoke"`,
		`"source": "manual notes"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "iterate" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `source = "manual notes", want "make container-provider-smoke"`) {
		t.Fatalf("Reasons = %v, want canonical source reason", out.Reasons)
	}
}

func TestRunDefersWhenReviewSampleBasisDoesNotMatchQuality(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"sample_basis": "single-alert, cascade, and alert-storm representative alert cases"`,
		`"sample_basis": "stale representative alert cases"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `sample_basis "stale representative alert cases" must match quality comparison sample_basis`) {
		t.Fatalf("Reasons = %v, want sample basis mismatch reason", out.Reasons)
	}
}

func TestRunDefersButRetainsIterateReasonsWhenEvidenceIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	reviewJSON := strings.Replace(
		passingReviewEvidenceJSON(),
		`"sample_basis": "single-alert, cascade, and alert-storm representative alert cases"`,
		`"sample_basis": "stale representative alert cases"`,
		1,
	)
	reviewJSON = strings.Replace(
		reviewJSON,
		`"source": "make container-provider-smoke"`,
		`"source": "manual note"`,
		1,
	)
	review := writeEvidence(t, dir, "review.json", reviewJSON)

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `sample_basis "stale representative alert cases" must match quality comparison sample_basis`) {
		t.Fatalf("Reasons = %v, want sample basis mismatch reason", out.Reasons)
	}
	if !containsReason(out.Reasons, `source = "manual note", want "make container-provider-smoke"`) {
		t.Fatalf("Reasons = %v, want retained iterate reason", out.Reasons)
	}
}

func TestRunDefersWhenReviewEvidenceDoesNotMatchQualityCases(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "pass"`,
		`"id": "stale-case", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "pass"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `missing reviewed case "checkout-latency"`) {
		t.Fatalf("Reasons = %v, want missing reviewed case reason", out.Reasons)
	}
	if !containsReason(out.Reasons, `reviewed case "stale-case" does not match`) {
		t.Fatalf("Reasons = %v, want stale reviewed case reason", out.Reasons)
	}
}

func TestRunDefersWhenReviewedCaseScenarioDoesNotMatchQuality(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"id": "checkout-latency", "scenario": "cascade"`,
		`"id": "checkout-latency", "scenario": "single_alert"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `reviewed case "checkout-latency" scenario = "single_alert", want quality comparison scenario "cascade"`) {
		t.Fatalf("Reasons = %v, want reviewed case scenario mismatch", out.Reasons)
	}
}

func TestRunDefersWhenReviewedCaseRequiredEvidenceRefsDoNotMatchQuality(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"]`,
		`"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:stale-latency"]`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `reviewed case "checkout-latency" required_evidence_refs = [snapshot:12 alert:stale-latency], want quality comparison refs [snapshot:12 alert:latency]`) {
		t.Fatalf("Reasons = %v, want reviewed case required_evidence_refs mismatch", out.Reasons)
	}
}

func TestRunDefersWhenReviewEvidenceHasNoReviewedCases(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"reviewed_cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "status": "pass", "notes": "direct and sandbox outputs preserve the payments CPU evidence chain"},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "pass", "notes": "cascade output keeps latency evidence traceability"},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "status": "pass", "notes": "alert-storm output remains equivalent and evidence-bound"}
		],`,
		`"reviewed_cases": [],`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, "reviewed_cases must contain at least one item") {
		t.Fatalf("Reasons = %v, want reviewed_cases reason", out.Reasons)
	}
}

func TestRunRejectsReviewEvidenceWithMultilineReviewedCaseID(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"id": "payments-cpu"`,
		`"id": "payments-cpu\ncontinued"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want multiline reviewed case id rejection")
	}
	if !strings.Contains(err.Error(), "review evidence reviewed_cases[0].id must be a single-line value") {
		t.Fatalf("run err = %v, want reviewed case id single-line error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunIteratesWhenReviewedCaseFails(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", passingQualityComparisonJSON())
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "pass"`,
		`"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "fail"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "iterate" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `reviewed case "checkout-latency" status = "fail", want pass`) {
		t.Fatalf("Reasons = %v, want reviewed case fail reason", out.Reasons)
	}
}

func TestRunRejectsQualitySchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "wrong_schema",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want schema mismatch")
	}
	if !strings.Contains(err.Error(), `schema = "wrong_schema", want "openclarion_sub_report"`) {
		t.Fatalf("run err = %v, want schema mismatch", err)
	}
}

func TestRunRejectsQualitySummaryThatDoesNotMatchCases(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}`,
		`{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "sandbox_candidate_regressed", "review_required": true}`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want summary/case mismatch")
	}
	if !strings.Contains(err.Error(), "case-derived summary") {
		t.Fatalf("run err = %v, want case-derived summary error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsMultilineQualitySampleBasis(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`"sample_basis": "single-alert, cascade, and alert-storm representative alert cases"`,
		`"sample_basis": "single-alert, cascade, and alert-storm representative alert cases\ncontinued"`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want multiline quality sample_basis rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison sample_basis must be a single-line value") {
		t.Fatalf("run err = %v, want quality sample_basis single-line error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsOversizedQualitySampleBasis(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`single-alert, cascade, and alert-storm representative alert cases`,
		strings.Repeat("a", maxReviewEvidenceTextBytes+1),
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want oversized quality sample_basis rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison sample_basis exceeds 2048 bytes") {
		t.Fatalf("run err = %v, want quality sample_basis size error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsQualityCaseWithMultilineID(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`"id": "payments-cpu"`,
		`"id": "payments-cpu\ncontinued"`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want multiline quality case id rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison cases[0].id must be a single-line value") {
		t.Fatalf("run err = %v, want quality case id single-line error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsQualityCaseWithoutRequiredEvidenceRefs(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`, "required_evidence_refs": ["snapshot:11", "alert:cpu"]`,
		"",
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing required evidence refs rejection")
	}
	if !strings.Contains(err.Error(), "required_evidence_refs") {
		t.Fatalf("run err = %v, want required_evidence_refs error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsQualityCaseWithMultilineRequiredEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`"required_evidence_refs": ["snapshot:11", "alert:cpu"]`,
		`"required_evidence_refs": ["snapshot:11", "alert:cpu\ncontinued"]`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want multiline required evidence ref rejection")
	}
	if !strings.Contains(err.Error(), "required_evidence_refs[1] must be a single-line value") {
		t.Fatalf("run err = %v, want required evidence ref single-line error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsQualityCaseWithoutSnapshotEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`"required_evidence_refs": ["snapshot:11", "alert:cpu"]`,
		`"required_evidence_refs": ["alert:cpu"]`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing snapshot evidence ref rejection")
	}
	if !strings.Contains(err.Error(), "snapshot:<positive-id>") {
		t.Fatalf("run err = %v, want snapshot evidence ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRejectsQualityCaseWithInvalidSnapshotEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", strings.Replace(
		passingQualityComparisonJSON(),
		`"required_evidence_refs": ["snapshot:11", "alert:cpu"]`,
		`"required_evidence_refs": ["snapshot:001", "alert:cpu"]`,
		1,
	))
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want invalid snapshot evidence ref rejection")
	}
	if !strings.Contains(err.Error(), "snapshot:<positive-id>") {
		t.Fatalf("run err = %v, want snapshot evidence ref error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunDefersWhenReviewEvidenceDateIsFuture(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"evidence_date": "2026-05-28"`,
		`"evidence_date": "2999-01-01"`,
		1,
	))

	out := runDecision(t, []string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	})
	if out.Decision != "defer" {
		t.Fatalf("Decision = %q reasons=%v", out.Decision, out.Reasons)
	}
	if !containsReason(out.Reasons, `must not be in the future`) {
		t.Fatalf("Reasons = %v, want future-date reason", out.Reasons)
	}
}

func TestRunRejectsUnsupportedReviewStatus(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", passingBaselineAuditJSON())
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`)
	review := writeEvidence(t, dir, "review.json", strings.Replace(
		passingReviewEvidenceJSON(),
		`"status": "pass",
			"reviewer": "openclarion-maintainer"`,
		`"status": "maybe",
			"reviewer": "openclarion-maintainer"`,
		1,
	))

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unsupported status")
	}
	if !strings.Contains(err.Error(), `want pass or fail`) {
		t.Fatalf("run err = %v, want status enum error", err)
	}
}

func TestRunRejectsInvalidEvidenceShape(t *testing.T) {
	dir := t.TempDir()
	baseline := writeEvidence(t, dir, "baseline.json", `{"tool":"wrong","status":"pass","checks":[]}`)
	quality := writeEvidence(t, dir, "quality.json", `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 1,
		"summary": {"improved_count":1},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [{"id":"one","recommendation":"sandbox_candidate_improved","review_required":true}]
	}`)
	review := writeEvidence(t, dir, "review.json", passingReviewEvidenceJSON())

	var stdout bytes.Buffer
	err := run([]string{
		"--baseline-audit", baseline,
		"--quality-comparison", quality,
		"--review-evidence", review,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want invalid tool error")
	}
	if !strings.Contains(err.Error(), "want sandbox_baseline_audit") {
		t.Fatalf("run err = %v, want baseline tool error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid evidence", stdout.String())
	}
}

func TestRunRequiresInputPaths(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want missing flags error")
	}
	if !strings.Contains(err.Error(), "--baseline-audit is required") {
		t.Fatalf("run err = %v, want baseline-audit requirement", err)
	}
}

func runDecision(t *testing.T, args []string) decisionOutput {
	t.Helper()
	var stdout bytes.Buffer
	if err := run(args, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out decisionOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	return out
}

func writeEvidence(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func passingBaselineAuditJSON() string {
	return `{
		"tool": "sandbox_baseline_audit",
		"status": "pass",
		"checks": [
			{"name": "fixed_file_contract", "status": "pass"},
			{"name": "batch_network_none_spec", "status": "pass"},
			{"name": "m5_turn_input_mounts", "status": "pass"},
			{"name": "docker_security_posture", "status": "pass"},
			{"name": "allowlist_enforcer_subset", "status": "pass"},
			{"name": "allowlist_enforcer_drift_rejection", "status": "pass"},
			{"name": "raw_result_validation", "status": "pass"}
		]
	}`
}

func passingQualityComparisonJSON() string {
	return `{
		"tool": "sandbox_quality_compare",
		"schema": "openclarion_sub_report",
		"mode": "manifest",
		"case_count": 3,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"scenario_coverage": ["single_alert", "cascade", "alert_storm"],
		"summary": {
			"improved_count": 2,
			"equivalent_count": 1,
			"regressed_count": 0,
			"needs_human_review_count": 0
		},
		"recommendation": "sandbox_batch_candidate_improved",
		"review_required": true,
		"cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "recommendation": "sandbox_candidate_improved", "review_required": true},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "recommendation": "equivalent_metrics", "review_required": true}
		]
	}`
}

func passingReviewEvidenceJSON() string {
	return `{
		"tool": "sandbox_m4_review_evidence",
		"evidence_date": "2026-05-28",
		"selected_candidate": "runtime-candidate-a",
		"runtime_candidate": "` + runtimeCandidateRef + `",
		"representative_sample": true,
		"sample_basis": "single-alert, cascade, and alert-storm representative alert cases",
		"candidate_evaluations": [
			{"candidate": "runtime-a", "status": "not_fit", "source": "candidate smoke review", "notes": "candidate still needs a bounded one-shot JSON-file proof"},
			{"candidate": "runtime-b", "status": "fail", "source": "candidate smoke review", "notes": "candidate did not satisfy the current readonly file-contract smoke"},
			{"candidate": "runtime-candidate-a", "status": "pass", "runtime_candidate": "` + runtimeCandidateRef + `", "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"], "source": "candidate runtime smoke review", "notes": "candidate runtime passed contract and lifecycle smoke as retained review evidence"}
			],
			"runtime_smokes": [
				{"name": "candidate_runtime_file_contract", "status": "pass", "source": "make agent-runtime-smoke", "evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json", "evidence_sha256": "1111111111111111111111111111111111111111111111111111111111111111"},
				{"name": "container_provider_lifecycle", "status": "pass", "source": "make container-provider-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-smoke.json", "evidence_sha256": "2222222222222222222222222222222222222222222222222222222222222222"},
				{"name": "container_provider_timeout_cleanup", "status": "pass", "source": "make container-provider-timeout-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-timeout-smoke.json", "evidence_sha256": "3333333333333333333333333333333333333333333333333333333333333333"},
				{"name": "container_provider_output_cap", "status": "pass", "source": "make container-provider-output-cap-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-output-cap-smoke.json", "evidence_sha256": "4444444444444444444444444444444444444444444444444444444444444444"},
				{"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "5555555555555555555555555555555555555555555555555555555555555555"}
			],
		"reviewed_cases": [
			{"id": "payments-cpu", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:cpu"], "status": "pass", "notes": "direct and sandbox outputs preserve the payments CPU evidence chain"},
			{"id": "checkout-latency", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:latency"], "status": "pass", "notes": "cascade output keeps latency evidence traceability"},
			{"id": "billing-errors", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:errors"], "status": "pass", "notes": "alert-storm output remains equivalent and evidence-bound"}
		],
		"human_review": {
			"status": "pass",
			"reviewer": "openclarion-maintainer",
			"notes": "sample reports preserve evidence traceability and improve operational usefulness"
		}
	}`
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}
