package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testRuntimeCandidate = "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func init() {
	todayUTC = func() time.Time {
		return time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	}
}

func TestRunOutputsFailClosedDraftReviewEvidence(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateFile(t, dir, "quality-comparison.json", `{
		"tool": "sandbox_quality_compare",
		"mode": "manifest",
		"case_count": 2,
		"sample_basis": "single-alert and cascade representative alert cases",
		"cases": [
			{"id": "single-alert"},
			{"id": "cascade"}
		]
	}`)
	writeSmokeArtifacts(t, filepath.Join(dir, "artifacts"), "runtime")

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", filepath.Join(dir, "artifacts"),
		"--runtime-smoke-ref-prefix", "runtime",
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
		"--evidence-date", "2026-06-04",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	var out reviewEvidence
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Tool != "sandbox_m4_review_evidence" {
		t.Fatalf("Tool = %q", out.Tool)
	}
	if out.RepresentativeSample {
		t.Fatal("RepresentativeSample = true, want fail-closed false by default")
	}
	if out.SampleBasis != "single-alert and cascade representative alert cases" {
		t.Fatalf("SampleBasis = %q", out.SampleBasis)
	}
	if len(out.CandidateEvaluations) != 1 {
		t.Fatalf("CandidateEvaluations = %d, want 1", len(out.CandidateEvaluations))
	}
	candidate := out.CandidateEvaluations[0]
	if candidate.Candidate != "runtime-candidate-a" || candidate.Status != "fail" {
		t.Fatalf("candidate evaluation = %+v, want fail-closed selected candidate", candidate)
	}
	if got, want := strings.Join(candidate.RuntimeSmokeRefs, ","), "candidate_runtime_file_contract,container_provider_lifecycle,container_provider_timeout_cleanup,container_provider_output_cap,egress_allowdeny"; got != want {
		t.Fatalf("RuntimeSmokeRefs = %v, want %s", candidate.RuntimeSmokeRefs, want)
	}
	if len(out.RuntimeSmokes) != len(runtimeSmokeSpecs) {
		t.Fatalf("RuntimeSmokes = %d, want %d", len(out.RuntimeSmokes), len(runtimeSmokeSpecs))
	}
	for i, smoke := range out.RuntimeSmokes {
		spec := runtimeSmokeSpecs[i]
		wantRef := "runtime/" + spec.FileName
		if smoke.Name != spec.Name || smoke.Source != spec.Source || smoke.EvidenceRef != wantRef || smoke.Status != "pass" {
			t.Fatalf("RuntimeSmokes[%d] = %+v, want spec=%+v ref=%s pass", i, smoke, spec, wantRef)
		}
		wantSHA := testFileSHA256(t, filepath.Join(dir, "artifacts", filepath.FromSlash(wantRef)))
		if smoke.EvidenceSHA256 != wantSHA {
			t.Fatalf("RuntimeSmokes[%d].EvidenceSHA256 = %q, want %q", i, smoke.EvidenceSHA256, wantSHA)
		}
	}
	if len(out.ReviewedCases) != 2 || out.ReviewedCases[0].ID != "single-alert" || out.ReviewedCases[0].Status != "fail" {
		t.Fatalf("ReviewedCases = %+v, want fail-closed quality cases", out.ReviewedCases)
	}
	if out.HumanReview.Status != "fail" || out.HumanReview.Reviewer != "openclarion-maintainer" {
		t.Fatalf("HumanReview = %+v, want fail-closed reviewer block", out.HumanReview)
	}
}

func TestRunCanAssertRepresentativeSample(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")

	var stdout bytes.Buffer
	if err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
		"--representative-sample",
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out reviewEvidence
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if !out.RepresentativeSample {
		t.Fatal("RepresentativeSample = false, want true when requested")
	}
}

func TestRunReadsRuntimeCandidateFile(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	candidateFile := writeTemplateFile(t, dir, "digest-ref.txt", testRuntimeCandidate+"\n")

	var stdout bytes.Buffer
	if err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate-file", candidateFile,
		"--reviewer", "openclarion-maintainer",
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	var out reviewEvidence
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if out.RuntimeCandidate != testRuntimeCandidate {
		t.Fatalf("RuntimeCandidate = %q, want file value", out.RuntimeCandidate)
	}
	if got := out.CandidateEvaluations[0].RuntimeCandidate; got != testRuntimeCandidate {
		t.Fatalf("candidate evaluation runtime candidate = %q, want file value", got)
	}
}

func TestRunRejectsAmbiguousRuntimeCandidateSources(t *testing.T) {
	dir := t.TempDir()
	candidateFile := writeTemplateFile(t, dir, "digest-ref.txt", testRuntimeCandidate+"\n")

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", "quality.json",
		"--runtime-smoke-artifacts-root", "artifacts",
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--runtime-candidate-file", candidateFile,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want ambiguous runtime candidate source rejection")
	}
	if !strings.Contains(err.Error(), "set only one of --runtime-candidate or --runtime-candidate-file") {
		t.Fatalf("run err = %v, want ambiguous source rejection", err)
	}
}

func TestRunRejectsInvalidRuntimeCandidateFile(t *testing.T) {
	dir := t.TempDir()
	candidateFile := writeTemplateFile(t, dir, "digest-ref.txt", "runtime-candidate-a:latest\n")

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", "quality.json",
		"--runtime-smoke-artifacts-root", "artifacts",
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate-file", candidateFile,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want invalid runtime candidate file rejection")
	}
	if !strings.Contains(err.Error(), "--runtime-candidate-file") {
		t.Fatalf("run err = %v, want runtime candidate file rejection", err)
	}
	if strings.Contains(err.Error(), "latest") {
		t.Fatalf("run err leaked runtime candidate file content: %v", err)
	}
}

func TestRunRejectsSymlinkRuntimeCandidateFile(t *testing.T) {
	dir := t.TempDir()
	target := writeTemplateFile(t, dir, "digest-ref-target.txt", testRuntimeCandidate+"\n")
	link := filepath.Join(dir, "digest-ref.txt")
	createTemplateSymlinkOrSkip(t, target, link)

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", "quality.json",
		"--runtime-smoke-artifacts-root", "artifacts",
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate-file", link,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink runtime candidate file rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
}

func TestRunWritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	outPath := filepath.Join(dir, "review-evidence.json")

	var stdout bytes.Buffer
	if err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
		"--out", outPath,
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when --out is used", stdout.String())
	}
	// #nosec G304 -- outPath is created inside this test's temporary directory.
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var out reviewEvidence
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode output file: %v\n%s", err, raw)
	}
	if out.Tool != "sandbox_m4_review_evidence" {
		t.Fatalf("Tool = %q", out.Tool)
	}
}

func TestRunRejectsExistingOutputFile(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	outPath := writeTemplateFile(t, dir, "review-evidence.json", `{"existing":true}`+"\n")

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
		"--out", outPath,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want existing output rejection")
	}
	if !strings.Contains(err.Error(), "must be absent before review evidence output is written") {
		t.Fatalf("run err = %v, want existing output rejection", err)
	}
	// #nosec G304 -- outPath is created inside this test's temporary directory.
	raw, readErr := os.ReadFile(outPath)
	if readErr != nil {
		t.Fatalf("read existing output: %v", readErr)
	}
	if got := string(raw); got != `{"existing":true}`+"\n" {
		t.Fatalf("existing output changed to %q", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on error", stdout.String())
	}
}

func TestRunRejectsSymlinkOutputFile(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	target := writeTemplateFile(t, dir, "target-review-evidence.json", `{"existing":true}`+"\n")
	outPath := filepath.Join(dir, "review-evidence.json")
	createTemplateSymlinkOrSkip(t, target, outPath)

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
		"--out", outPath,
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink output rejection")
	}
	if !strings.Contains(err.Error(), "must be absent before review evidence output is written, not a symlink") {
		t.Fatalf("run err = %v, want symlink output rejection", err)
	}
	// #nosec G304 -- target is created inside this test's temporary directory.
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read symlink target: %v", readErr)
	}
	if got := string(raw); got != `{"existing":true}`+"\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestRunRejectsUnsupportedRuntimeSmokeStatus(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	writeTemplateFile(t, dir, "egress-allowdeny-smoke.json", `{"tool":"egress-allowdeny-smoke","status":"unknown"}`)

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unsupported runtime smoke status")
	}
	if !strings.Contains(err.Error(), "status") || !strings.Contains(err.Error(), "want pass or fail") {
		t.Fatalf("run err = %v, want status enum error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on error", stdout.String())
	}
}

func TestRunRejectsSymlinkRuntimeSmokeArtifact(t *testing.T) {
	dir := t.TempDir()
	quality := writeTemplateQuality(t, dir)
	writeSmokeArtifacts(t, dir, "")
	target := filepath.Join(dir, "agent-runtime-smoke.json")
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}
	createTemplateSymlinkOrSkip(t, filepath.Join(dir, "container-provider-smoke.json"), target)

	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", quality,
		"--runtime-smoke-artifacts-root", dir,
		"--selected-candidate", "runtime-candidate-a",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink artifact rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
}

func TestRunRejectsInvalidCandidateID(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{
		"--quality-comparison", "quality.json",
		"--runtime-smoke-artifacts-root", "artifacts",
		"--selected-candidate", "runtime candidate",
		"--runtime-candidate", testRuntimeCandidate,
		"--reviewer", "openclarion-maintainer",
	}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want invalid selected candidate")
	}
	if !strings.Contains(err.Error(), "--selected-candidate") {
		t.Fatalf("run err = %v, want selected candidate validation", err)
	}
}

func writeTemplateQuality(t *testing.T, dir string) string {
	t.Helper()
	return writeTemplateFile(t, dir, "quality-comparison.json", `{
		"tool": "sandbox_quality_compare",
		"mode": "manifest",
		"case_count": 1,
		"sample_basis": "single-alert representative alert case",
		"cases": [
			{"id": "single-alert"}
		]
	}`)
}

func writeSmokeArtifacts(t *testing.T, root, prefix string) {
	t.Helper()
	for _, spec := range runtimeSmokeSpecs {
		name := spec.FileName
		if prefix != "" {
			name = prefix + "/" + name
		}
		writeTemplateFile(t, root, name, `{"tool":"`+spec.Name+`","status":"pass"}`+"\n")
	}
}

func writeTemplateFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	filePath := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(filePath), err)
	}
	if err := os.WriteFile(filePath, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", filePath, err)
	}
	return filePath
}

func testFileSHA256(t *testing.T, filePath string) string {
	t.Helper()
	// #nosec G304 -- filePath is created inside this test's temporary directory.
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func createTemplateSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
}
