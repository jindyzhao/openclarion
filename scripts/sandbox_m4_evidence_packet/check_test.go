package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"reflect"
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

func TestRunAssemblesEvidencePacket(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner); err != nil {
		t.Fatalf("runWithRunner: %v", err)
	}

	var out packetOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Tool != "sandbox_m4_evidence_packet" {
		t.Fatalf("Tool = %q", out.Tool)
	}
	if out.Decision != "proceed" || !out.ReviewRequired {
		t.Fatalf("Decision = %q ReviewRequired = %v", out.Decision, out.ReviewRequired)
	}
	if out.OutDir != "." {
		t.Fatalf("OutDir = %q, want packet-local root", out.OutDir)
	}
	if len(out.Commands) != 3 {
		t.Fatalf("Commands = %d, want 3", len(out.Commands))
	}
	wantCommands := [][]string{
		{"run", "./scripts/sandbox_baseline_audit"},
		{"run", "./scripts/sandbox_quality_compare", "--manifest", qualityManifest, "--fail-on-regression"},
		{"run", "./scripts/sandbox_m4_decision", "--baseline-audit", packetArtifactPath(outDir, out.Artifacts.BaselineAudit), "--quality-comparison", packetArtifactPath(outDir, out.Artifacts.QualityComparison), "--review-evidence", packetArtifactPath(outDir, out.Artifacts.ReviewEvidence), "--min-cases", "3"},
	}
	if !reflect.DeepEqual(runner.calls, wantCommands) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, wantCommands)
	}
	for _, ref := range []string{
		out.Artifacts.BaselineAudit,
		out.Artifacts.QualityComparison,
		out.Artifacts.ReviewEvidence,
		out.Artifacts.Decision,
		out.Artifacts.Packet,
	} {
		assertPortablePacketPath(t, ref)
		if _, err := os.Stat(packetArtifactPath(outDir, ref)); err != nil {
			t.Fatalf("artifact %s missing: %v", ref, err)
		}
	}
	for _, command := range out.Commands {
		assertPortablePacketPath(t, command.OutputPath)
	}
	if !containsAll(out.Commands[1].Args, out.QualityInputs.Manifest.Path) {
		t.Fatalf("quality command args = %#v, want packet-local quality manifest ref %q", out.Commands[1].Args, out.QualityInputs.Manifest.Path)
	}
	if !containsAll(out.Commands[2].Args, out.Artifacts.BaselineAudit, out.Artifacts.QualityComparison, out.Artifacts.ReviewEvidence) {
		t.Fatalf("decision command args = %#v, want packet-local artifact refs", out.Commands[2].Args)
	}
	packetRaw, err := os.ReadFile(packetArtifactPath(outDir, out.Artifacts.Packet))
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if !bytes.Contains(packetRaw, []byte(`"decision": "proceed"`)) {
		t.Fatalf("packet.json = %s", string(packetRaw))
	}
	if !bytes.Contains(packetRaw, []byte(`"artifact_sha256"`)) {
		t.Fatalf("packet.json = %s, want artifact_sha256", string(packetRaw))
	}
	wantDigests := packetArtifactDigests{
		BaselineAudit:     testFileSHA256Hex(t, packetArtifactPath(outDir, out.Artifacts.BaselineAudit)),
		QualityComparison: testFileSHA256Hex(t, packetArtifactPath(outDir, out.Artifacts.QualityComparison)),
		ReviewEvidence:    testFileSHA256Hex(t, packetArtifactPath(outDir, out.Artifacts.ReviewEvidence)),
		Decision:          testFileSHA256Hex(t, packetArtifactPath(outDir, out.Artifacts.Decision)),
	}
	if out.ArtifactSHA256 != wantDigests {
		t.Fatalf("ArtifactSHA256 = %+v, want %+v", out.ArtifactSHA256, wantDigests)
	}
	assertPortablePacketPath(t, out.QualityInputs.Manifest.Path)
	if out.QualityInputs.Manifest.Path != qualityManifestRef {
		t.Fatalf("QualityInputs.Manifest.Path = %q, want %q", out.QualityInputs.Manifest.Path, qualityManifestRef)
	}
	if out.QualityInputs.Manifest.SHA256 != testFileSHA256Hex(t, packetArtifactPath(outDir, out.QualityInputs.Manifest.Path)) {
		t.Fatalf("QualityInputs.Manifest.SHA256 = %q, want copied manifest digest", out.QualityInputs.Manifest.SHA256)
	}
	if len(out.QualityInputs.Reports) != 6 {
		t.Fatalf("QualityInputs.Reports = %d, want 6", len(out.QualityInputs.Reports))
	}
	seenQualityReports := map[string]bool{}
	for _, artifact := range out.QualityInputs.Reports {
		if artifact.CaseID == "" || artifact.ManifestRef == "" {
			t.Fatalf("quality input report artifact is incomplete: %+v", artifact)
		}
		if artifact.Role != "direct" && artifact.Role != "sandbox" {
			t.Fatalf("quality input report role = %q, want direct or sandbox", artifact.Role)
		}
		assertPortablePacketPath(t, artifact.ManifestRef)
		assertPortablePacketPath(t, artifact.Path)
		if wantPrefix := qualityReportsDir + "/"; !strings.HasPrefix(artifact.Path, wantPrefix) {
			t.Fatalf("quality input report path = %q, want prefix %q", artifact.Path, wantPrefix)
		}
		if seenQualityReports[artifact.Path] {
			t.Fatalf("duplicate quality input report path %q", artifact.Path)
		}
		seenQualityReports[artifact.Path] = true
		copiedPath := packetArtifactPath(outDir, artifact.Path)
		if _, err := os.Stat(copiedPath); err != nil {
			t.Fatalf("quality input report %s missing: %v", copiedPath, err)
		}
		if gotDigest := testFileSHA256Hex(t, copiedPath); gotDigest != artifact.SHA256 {
			t.Fatalf("quality input report %s digest = %q, want %q", copiedPath, gotDigest, artifact.SHA256)
		}
	}
	if len(out.RuntimeSmokeArtifacts) != len(runtimeSmokeArtifactBodies) {
		t.Fatalf("RuntimeSmokeArtifacts = %d, want %d", len(out.RuntimeSmokeArtifacts), len(runtimeSmokeArtifactBodies))
	}
	for _, artifact := range out.RuntimeSmokeArtifacts {
		wantDigest := runtimeSmokeArtifactSHA256(artifact.EvidenceRef)
		if artifact.EvidenceSHA256 != wantDigest {
			t.Fatalf("runtime smoke artifact %+v has digest %q, want %q", artifact, artifact.EvidenceSHA256, wantDigest)
		}
		wantPath := path.Join(runtimeSmokeArtifactsDir, artifact.EvidenceRef)
		if artifact.Path != wantPath {
			t.Fatalf("runtime smoke artifact path = %q, want packet-local path %q", artifact.Path, wantPath)
		}
		if path.IsAbs(artifact.Path) || strings.Contains(artifact.Path, "\\") {
			t.Fatalf("runtime smoke artifact path = %q, want portable slash-separated relative path", artifact.Path)
		}
		copiedPath := packetArtifactPath(outDir, artifact.Path)
		if _, err := os.Stat(copiedPath); err != nil {
			t.Fatalf("runtime smoke artifact %s missing: %v", copiedPath, err)
		}
		if gotDigest := testFileSHA256Hex(t, copiedPath); gotDigest != wantDigest {
			t.Fatalf("runtime smoke artifact %s digest = %q, want %q", copiedPath, gotDigest, wantDigest)
		}
	}

	var packet packetOutput
	if err := json.Unmarshal(packetRaw, &packet); err != nil {
		t.Fatalf("decode packet artifact: %v", err)
	}
	if packet.OutDir != "." {
		t.Fatalf("packet OutDir = %q, want packet-local root", packet.OutDir)
	}
	if !reflect.DeepEqual(packet.Artifacts, out.Artifacts) {
		t.Fatalf("packet Artifacts = %+v, want %+v", packet.Artifacts, out.Artifacts)
	}
	if packet.ArtifactSHA256 != wantDigests {
		t.Fatalf("packet ArtifactSHA256 = %+v, want %+v", packet.ArtifactSHA256, wantDigests)
	}
	if !reflect.DeepEqual(packet.QualityInputs, out.QualityInputs) {
		t.Fatalf("packet QualityInputs = %+v, want %+v", packet.QualityInputs, out.QualityInputs)
	}
	if !reflect.DeepEqual(packet.RuntimeSmokeArtifacts, out.RuntimeSmokeArtifacts) {
		t.Fatalf("packet RuntimeSmokeArtifacts = %+v, want %+v", packet.RuntimeSmokeArtifacts, out.RuntimeSmokeArtifacts)
	}
}

func TestRunVerifiesExistingEvidencePacketDirectory(t *testing.T) {
	outDir, want := assembleValidPacketFixture(t)

	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{}); err != nil {
		t.Fatalf("verify packet: %v", err)
	}
	var got packetOutput
	if err := json.NewDecoder(&stdout).Decode(&got); err != nil {
		t.Fatalf("decode verify stdout %q: %v", stdout.String(), err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verified packet = %+v, want %+v", got, want)
	}
}

func TestRunVerifiesExistingEvidencePacketFile(t *testing.T) {
	outDir, want := assembleValidPacketFixture(t)

	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--verify-packet", packetArtifactPath(outDir, want.Artifacts.Packet),
	}, &stdout, &fakeRunner{}); err != nil {
		t.Fatalf("verify packet file: %v", err)
	}
	var got packetOutput
	if err := json.NewDecoder(&stdout).Decode(&got); err != nil {
		t.Fatalf("decode verify stdout %q: %v", stdout.String(), err)
	}
	if got.Decision != want.Decision || got.ArtifactSHA256 != want.ArtifactSHA256 {
		t.Fatalf("verified packet = %+v, want decision/digests from %+v", got, want)
	}
}

func TestRunVerifyRejectsTamperedQualityInputReport(t *testing.T) {
	outDir, packet := assembleValidPacketFixture(t)
	reportPath := packetArtifactPath(outDir, packet.QualityInputs.Reports[0].Path)
	if err := os.WriteFile(reportPath, []byte(qualityInputReportBodies[packet.QualityInputs.Reports[0].ManifestRef]+"\n"), 0o600); err != nil {
		t.Fatalf("tamper report: %v", err)
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want tampered quality input rejection")
	}
	if !strings.Contains(err.Error(), "quality input report") || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("verify err = %v, want quality input digest error", err)
	}
}

func TestRunVerifyRejectsTamperedRuntimeSmokeArtifact(t *testing.T) {
	outDir, packet := assembleValidPacketFixture(t)
	smokePath := packetArtifactPath(outDir, packet.RuntimeSmokeArtifacts[0].Path)
	if err := os.WriteFile(smokePath, []byte(runtimeSmokeArtifactBodies[packet.RuntimeSmokeArtifacts[0].EvidenceRef]+"\n"), 0o600); err != nil {
		t.Fatalf("tamper runtime smoke: %v", err)
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want tampered runtime smoke rejection")
	}
	if !strings.Contains(err.Error(), "runtime smoke artifact") || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("verify err = %v, want runtime smoke digest error", err)
	}
}

func TestRunVerifyRejectsStaleCommandManifestPath(t *testing.T) {
	outDir, packet := assembleValidPacketFixture(t)
	for i := range packet.Commands {
		if len(packet.Commands[i].Args) > 1 && packet.Commands[i].Args[1] == "./scripts/sandbox_quality_compare" {
			for j := range packet.Commands[i].Args {
				if packet.Commands[i].Args[j] == packet.QualityInputs.Manifest.Path {
					packet.Commands[i].Args[j] = "quality-manifest.json"
				}
			}
		}
	}
	writePacketOutput(t, outDir, packet)

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want stale command path rejection")
	}
	if !strings.Contains(err.Error(), "quality command --manifest") {
		t.Fatalf("verify err = %v, want quality command manifest error", err)
	}
}

func TestRunVerifyRejectsUnexpectedPacketFile(t *testing.T) {
	outDir, _ := assembleValidPacketFixture(t)
	if err := os.WriteFile(filepath.Join(outDir, "operator-note.txt"), []byte("manual note"), 0o600); err != nil {
		t.Fatalf("write unexpected file: %v", err)
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want unexpected file rejection")
	}
	if !strings.Contains(err.Error(), `unexpected file "operator-note.txt"`) {
		t.Fatalf("verify err = %v, want unexpected file error", err)
	}
}

func TestRunVerifyRejectsUnexpectedPacketDirectory(t *testing.T) {
	outDir, _ := assembleValidPacketFixture(t)
	if err := os.Mkdir(filepath.Join(outDir, "unreferenced"), 0o700); err != nil {
		t.Fatalf("mkdir unexpected dir: %v", err)
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--verify-packet", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want unexpected directory rejection")
	}
	if !strings.Contains(err.Error(), `unexpected directory "unreferenced"`) {
		t.Fatalf("verify err = %v, want unexpected directory error", err)
	}
}

func TestRunVerifyRejectsPacketJSONAlias(t *testing.T) {
	outDir, packet := assembleValidPacketFixture(t)
	packetRaw, err := os.ReadFile(packetArtifactPath(outDir, packet.Artifacts.Packet))
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	aliasPath := filepath.Join(outDir, "packet-copy.json")
	// #nosec G703 -- test writes a retained-packet alias under t.TempDir.
	if err := os.WriteFile(aliasPath, packetRaw, 0o600); err != nil {
		t.Fatalf("write packet alias: %v", err)
	}

	var stdout bytes.Buffer
	err = runWithRunner(context.Background(), []string{
		"--verify-packet", aliasPath,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("verify err = nil, want packet alias rejection")
	}
	if !strings.Contains(err.Error(), "must match packet artifact path") {
		t.Fatalf("verify err = %v, want packet artifact path error", err)
	}
}

func TestRunRejectsSymlinkQualityManifest(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	qualityManifestLink := filepath.Join(dir, "quality-manifest-link.json")
	createSymlinkOrSkip(t, qualityManifest, qualityManifestLink)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifestLink,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want symlink quality manifest rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
}

func TestRunRejectsSymlinkQualityInputReport(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	directReport := filepath.Join(dir, "direct", "single-alert.json")
	if err := os.Remove(directReport); err != nil {
		t.Fatalf("remove direct report: %v", err)
	}
	createSymlinkOrSkip(t, filepath.Join(dir, "sandbox", "single-alert.json"), directReport)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want symlink quality input report rejection")
	}
	if !strings.Contains(err.Error(), "quality input case") || !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want quality input symlink rejection", err)
	}
}

func TestRunRejectsSymlinkRuntimeSmokeArtifact(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	smokeRef := "artifacts/m4/runtime/agent-runtime-smoke.json"
	smokePath := filepath.Join(dir, filepath.FromSlash(smokeRef))
	if err := os.Remove(smokePath); err != nil {
		t.Fatalf("remove smoke artifact: %v", err)
	}
	createSymlinkOrSkip(t, reviewEvidence, smokePath)
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want symlink runtime smoke artifact rejection")
	}
	if !strings.Contains(err.Error(), "runtime smoke") || !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want runtime smoke symlink rejection", err)
	}
}

func TestRunRejectsOversizedRuntimeSmokeArtifact(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	smokeRef := "artifacts/m4/runtime/agent-runtime-smoke.json"
	smokePath := filepath.Join(dir, filepath.FromSlash(smokeRef))
	if err := os.WriteFile(smokePath, bytes.Repeat([]byte("x"), int(maxInputBytes)+1), 0o600); err != nil {
		t.Fatalf("write oversized smoke artifact: %v", err)
	}
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want oversized runtime smoke artifact rejection")
	}
	if !strings.Contains(err.Error(), "runtime smoke") || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("run err = %v, want oversized runtime smoke artifact rejection", err)
	}
}

func TestRunAssemblesDeferPacketFromFailClosedReviewEvidence(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", failClosedReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: deferDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner); err != nil {
		t.Fatalf("runWithRunner: %v", err)
	}

	var out packetOutput
	if err := json.NewDecoder(&stdout).Decode(&out); err != nil {
		t.Fatalf("decode stdout %q: %v", stdout.String(), err)
	}
	if out.Decision != "defer" || !out.ReviewRequired {
		t.Fatalf("Decision = %q ReviewRequired = %v, want defer and review_required", out.Decision, out.ReviewRequired)
	}
	packet, err := verifyPacket(outDir)
	if err != nil {
		t.Fatalf("verifyPacket: %v", err)
	}
	if packet.Decision != "defer" {
		t.Fatalf("verified packet decision = %q, want defer", packet.Decision)
	}
}

func TestRunPropagatesFailUnlessToDecision(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
		"--fail-unless", "proceed",
	}, &stdout, runner); err != nil {
		t.Fatalf("runWithRunner: %v", err)
	}
	last := runner.calls[len(runner.calls)-1]
	if !containsAll(last, "--fail-unless", "proceed") {
		t.Fatalf("decision args = %#v, want fail-unless proceed", last)
	}
}

func TestRunRejectsNonEmptyOutputDirectory(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	if err := os.Mkdir(outDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, outDir, "old.json", `{}`)

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("run err = nil, want non-empty output dir error")
	}
	if !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("run err = %v, want must be empty", err)
	}
}

func TestRunStopsOnQualityCommandFailure(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stderr: "quality regressed", err: errors.New("exit status 1")},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want quality command failure")
	}
	if !strings.Contains(err.Error(), "quality regressed") {
		t.Fatalf("run err = %v, want stderr context", err)
	}
	decisionPath := filepath.Join(dir, "packet", artifactNames["decision"])
	if _, statErr := os.Stat(decisionPath); !os.IsNotExist(statErr) {
		t.Fatalf("decision artifact exists after quality failure: %v", statErr)
	}
}

func TestRunRejectsInvalidReviewEvidenceJSON(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", `{not-json`)
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse review evidence") {
		t.Fatalf("run err = %v, want invalid JSON", err)
	}
}

func TestRunRejectsWeakReviewEvidenceShape(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", `{"tool":"sandbox_m4_review_evidence"}`)
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want weak review evidence rejection")
	}
	if !strings.Contains(err.Error(), "evidence date") {
		t.Fatalf("run err = %v, want evidence date error", err)
	}
}

func TestRunRejectsReviewEvidenceWithNonCanonicalRuntimeSource(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(validReviewEvidence(), `"source": "make agent-runtime-smoke"`, `"source": "manual note"`, 1))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want non-canonical runtime source rejection")
	}
	if !strings.Contains(err.Error(), `source = "manual note", want "make agent-runtime-smoke"`) {
		t.Fatalf("run err = %v, want canonical source error", err)
	}
}

func TestRunRejectsReviewEvidenceWithTagOnlyRuntimeCandidate(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(validReviewEvidence(), runtimeCandidateRef, "runtime-candidate-a:latest", 1))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want tag-only runtime candidate rejection")
	}
	if !strings.Contains(err.Error(), "runtime_candidate must be an immutable image reference") {
		t.Fatalf("run err = %v, want immutable runtime candidate error", err)
	}
}

func TestRunRejectsReviewEvidenceWithWhitespacePaddedSelectedCandidate(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"selected_candidate": "runtime-candidate-a"`,
		`"selected_candidate": " runtime-candidate-a "`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want whitespace-padded selected_candidate rejection")
	}
	if !strings.Contains(err.Error(), "selected_candidate must not contain leading or trailing whitespace") {
		t.Fatalf("run err = %v, want selected_candidate whitespace error", err)
	}
}

func TestRunRejectsReviewEvidenceWithoutSelectedCandidateRuntimeCandidate(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`, "runtime_candidate": "`+runtimeCandidateRef+`"`,
		"",
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing selected candidate runtime ref rejection")
	}
	if !strings.Contains(err.Error(), `candidate "runtime-candidate-a" runtime_candidate is required when status is pass`) {
		t.Fatalf("run err = %v, want selected candidate runtime ref error", err)
	}
}

func TestRunRejectsReviewEvidenceWithoutSelectedCandidateRuntimeSmokeRefs(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`, "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"]`,
		"",
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing selected candidate runtime smoke refs rejection")
	}
	if !strings.Contains(err.Error(), `candidate "runtime-candidate-a" runtime_smoke_refs must include "candidate_runtime_file_contract"`) {
		t.Fatalf("run err = %v, want selected candidate runtime smoke refs error", err)
	}
}

func TestRunRejectsReviewEvidenceWithDuplicateCandidateRuntimeSmokeRefs(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"egress_allowdeny"], "source": "candidate runtime smoke review"`,
		`"container_provider_lifecycle"], "source": "candidate runtime smoke review"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate selected candidate runtime smoke ref rejection")
	}
	if !strings.Contains(err.Error(), `duplicate runtime_smoke_refs value "container_provider_lifecycle"`) {
		t.Fatalf("run err = %v, want duplicate runtime smoke ref error", err)
	}
}

func TestRunRejectsReviewEvidenceWithUnexpectedRuntimeSmokeName(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`{"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "`+runtimeSmokeArtifactSHA256("artifacts/m4/runtime/egress-allowdeny-smoke.json")+`"}`,
		`{"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "`+runtimeSmokeArtifactSHA256("artifacts/m4/runtime/egress-allowdeny-smoke.json")+`"},
	    {"name": "manual_runtime_note", "status": "pass", "source": "manual note"}`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want unexpected runtime smoke name rejection")
	}
	if !strings.Contains(err.Error(), `runtime_smokes[5].name = "manual_runtime_note" is not a required runtime smoke`) {
		t.Fatalf("run err = %v, want unexpected runtime smoke name error", err)
	}
}

func TestRunRejectsReviewEvidenceWithoutRuntimeSmokeEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`, "evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		"",
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_ref is required`) {
		t.Fatalf("run err = %v, want runtime smoke evidence_ref error", err)
	}
}

func TestRunRejectsReviewEvidenceWithInvalidRuntimeSmokeEvidenceSHA(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"evidence_sha256": "`+runtimeSmokeArtifactSHA256("artifacts/m4/runtime/agent-runtime-smoke.json")+`"`,
		`"evidence_sha256": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want invalid runtime smoke evidence_sha256 rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_sha256 must be 64 lowercase hex characters`) {
		t.Fatalf("run err = %v, want runtime smoke evidence_sha256 error", err)
	}
}

func TestRunRejectsReviewEvidenceWithTraversalRuntimeSmokeEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		`"evidence_ref": "artifacts/m4/runtime/../agent-runtime-smoke.json"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want traversal runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" evidence_ref must be a normalized relative artifact path`) {
		t.Fatalf("run err = %v, want normalized evidence_ref error", err)
	}
}

func TestRunRejectsReviewEvidenceWithDuplicateRuntimeSmokeEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"evidence_ref": "artifacts/m4/runtime/container-provider-smoke.json"`,
		`"evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate runtime smoke evidence_ref rejection")
	}
	if !strings.Contains(err.Error(), `evidence_ref "artifacts/m4/runtime/agent-runtime-smoke.json" duplicates another runtime smoke`) {
		t.Fatalf("run err = %v, want duplicate evidence_ref error", err)
	}
}

func TestRunRejectsMissingRuntimeSmokeArtifact(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	if err := os.Remove(filepath.Join(dir, "artifacts/m4/runtime/agent-runtime-smoke.json")); err != nil {
		t.Fatalf("remove smoke artifact: %v", err)
	}
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing runtime smoke artifact rejection")
	}
	if !strings.Contains(err.Error(), `hash runtime smoke "candidate_runtime_file_contract" artifact "artifacts/m4/runtime/agent-runtime-smoke.json"`) {
		t.Fatalf("run err = %v, want missing runtime smoke artifact error", err)
	}
}

func TestRunRejectsRuntimeSmokeArtifactDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	tamperedPath := filepath.Join(dir, "artifacts/m4/runtime/agent-runtime-smoke.json")
	if err := os.WriteFile(tamperedPath, []byte("tampered artifact\n"), 0o600); err != nil {
		t.Fatalf("tamper smoke artifact: %v", err)
	}
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want runtime smoke artifact digest mismatch")
	}
	if !strings.Contains(err.Error(), `runtime smoke "candidate_runtime_file_contract" artifact "artifacts/m4/runtime/agent-runtime-smoke.json" sha256`) {
		t.Fatalf("run err = %v, want runtime smoke artifact digest mismatch", err)
	}
}

func TestRunRejectsReviewEvidenceWhenSelectedCandidateRuntimeCandidateDoesNotMatchTopLevel(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	otherRef := "localhost:5000/openclarion/other-runtime@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"runtime_candidate": "`+runtimeCandidateRef+`", "runtime_smoke_refs": [`,
		`"runtime_candidate": "`+otherRef+`", "runtime_smoke_refs": [`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want selected candidate runtime mismatch rejection")
	}
	if !strings.Contains(err.Error(), `selected candidate "runtime-candidate-a" runtime_candidate`) {
		t.Fatalf("run err = %v, want selected candidate runtime mismatch error", err)
	}
}

func TestRunRejectsReviewEvidenceWithoutHumanReviewNotes(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		",\n    \"notes\": \"Representative alert-analysis scenarios reviewed.\"",
		"",
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing human review notes rejection")
	}
	if !strings.Contains(err.Error(), "human_review.notes is required") {
		t.Fatalf("run err = %v, want notes required error", err)
	}
}

func TestRunRejectsReviewEvidenceWithWhitespacePaddedHumanReviewNotes(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"notes": "Representative alert-analysis scenarios reviewed."`,
		`"notes": " Representative alert-analysis scenarios reviewed. "`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want whitespace-padded human review notes rejection")
	}
	if !strings.Contains(err.Error(), "human_review.notes must not contain leading or trailing whitespace") {
		t.Fatalf("run err = %v, want notes whitespace error", err)
	}
}

func TestRunRejectsReviewEvidenceWithMultilineCandidateEvaluationSource(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"source": "candidate runtime smoke review"`,
		`"source": "candidate runtime smoke review\nmanual note"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want multiline candidate source rejection")
	}
	if !strings.Contains(err.Error(), `candidate_evaluations candidate "runtime-candidate-a" source must be a single-line value`) {
		t.Fatalf("run err = %v, want single-line candidate source error", err)
	}
}

func TestRunRejectsReviewEvidenceWithOversizedReviewedCaseNotes(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		"single-alert sample preserves evidence traceability",
		strings.Repeat("a", maxReviewEvidenceTextBytes+1),
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want oversized reviewed case notes rejection")
	}
	if !strings.Contains(err.Error(), `review evidence reviewed case "single-alert" notes exceeds 2048 bytes`) {
		t.Fatalf("run err = %v, want oversized reviewed case notes error", err)
	}
}

func TestRunRejectsReviewEvidenceForDifferentSampleBasis(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"sample_basis": "representative alert-analysis scenarios"`,
		`"sample_basis": "stale representative alert-analysis scenarios"`,
		1,
	))
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want sample basis mismatch rejection")
	}
	if !strings.Contains(err.Error(), `sample_basis "stale representative alert-analysis scenarios" must match quality comparison sample_basis`) {
		t.Fatalf("run err = %v, want sample basis mismatch error", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %#v, want baseline and quality only", runner.calls)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["review"])); !os.IsNotExist(statErr) {
		t.Fatalf("review artifact exists after sample-basis rejection: %v", statErr)
	}
}

func TestRunRejectsReviewEvidenceWithDuplicateReviewedCase(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"id": "cascade"`,
		`"id": "single-alert"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate reviewed case rejection")
	}
	if !strings.Contains(err.Error(), `duplicate review evidence reviewed case "single-alert"`) {
		t.Fatalf("run err = %v, want duplicate reviewed case error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid review evidence", stdout.String())
	}
}

func TestRunRejectsReviewEvidenceWithMultilineReviewedCaseID(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"id": "single-alert"`,
		`"id": "single-alert\ncontinued"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want multiline reviewed case id rejection")
	}
	if !strings.Contains(err.Error(), "review evidence reviewed_cases[0].id must be a single-line value") {
		t.Fatalf("run err = %v, want reviewed case id single-line error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invalid review evidence", stdout.String())
	}
}

func TestRunRejectsWeakBaselineCommandOutput(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: `{"tool":"sandbox_baseline_audit","status":"pass","checks":[]}`},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want weak baseline artifact rejection")
	}
	if !strings.Contains(err.Error(), "baseline audit checks must not be empty") {
		t.Fatalf("run err = %v, want baseline checks error", err)
	}
}

func TestRunRejectsDuplicateBaselineCommandOutputChecks(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: strings.Replace(
				validBaselineOutput(),
				`{"name": "raw_result_validation", "status": "pass"}`,
				`{"name": "fixed_file_contract", "status": "pass"}`,
				1,
			)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate baseline check rejection")
	}
	if !strings.Contains(err.Error(), `duplicate baseline audit check "fixed_file_contract"`) {
		t.Fatalf("run err = %v, want duplicate baseline check error", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["baseline"])); !os.IsNotExist(statErr) {
		t.Fatalf("baseline artifact exists after duplicate check rejection: %v", statErr)
	}
}

func TestRunRejectsWeakQualityCommandOutput(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: `{"tool":"sandbox_quality_compare","mode":"manifest","case_count":3,"summary":{"improved_count":3},"recommendation":"sandbox_batch_candidate_improved","review_required":true,"cases":[]}`},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want weak quality artifact rejection")
	}
	if !strings.Contains(err.Error(), `schema = "", want "openclarion_sub_report"`) {
		t.Fatalf("run err = %v, want schema error", err)
	}
}

func TestRunRejectsQualityManifestThatDoesNotMatchOutput(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(
		t,
		dir,
		"quality-manifest.json",
		strings.Replace(validQualityManifest(), `"id": "single-alert"`, `"id": "different-sample"`, 1),
	)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want manifest/output mismatch rejection")
	}
	if !strings.Contains(err.Error(), `quality manifest cases[0].id = "different-sample"`) {
		t.Fatalf("run err = %v, want manifest/output mismatch error", err)
	}
}

func TestRunRejectsMissingQualityInputReport(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	if err := os.Remove(filepath.Join(dir, "direct", "single-alert.json")); err != nil {
		t.Fatalf("remove quality input: %v", err)
	}
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing quality input report rejection")
	}
	if !strings.Contains(err.Error(), `quality input case "single-alert" direct_sub_report "direct/single-alert.json"`) {
		t.Fatalf("run err = %v, want missing quality input context", err)
	}
}

func TestRunRejectsInvalidQualityInputReport(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	writeFile(t, dir, "sandbox/cascade.json", `{"title":"missing report fields"}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want invalid quality input report rejection")
	}
	if !strings.Contains(err.Error(), `quality input case "cascade" sandbox_sub_report "sandbox/cascade.json"`) {
		t.Fatalf("run err = %v, want invalid quality input context", err)
	}
}

func TestRunRejectsQualityCommandOutputWithMultilineCaseID(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"id": "single-alert"`, `"id": "single-alert\ncontinued"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want multiline quality case id rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison cases[0].id must be a single-line value") {
		t.Fatalf("run err = %v, want quality case id single-line error", err)
	}
}

func TestRunRejectsQualityCommandOutputWithMultilineSampleBasis(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"sample_basis": "representative alert-analysis scenarios"`, `"sample_basis": "representative alert-analysis scenarios\ncontinued"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want multiline quality sample_basis rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison sample_basis must be a single-line value") {
		t.Fatalf("run err = %v, want quality sample_basis single-line error", err)
	}
}

func TestRunRejectsQualityCommandOutputWithOversizedSampleBasis(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `representative alert-analysis scenarios`, strings.Repeat("a", maxReviewEvidenceTextBytes+1), 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want oversized quality sample_basis rejection")
	}
	if !strings.Contains(err.Error(), "quality comparison sample_basis exceeds 2048 bytes") {
		t.Fatalf("run err = %v, want quality sample_basis size error", err)
	}
}

func TestRunRejectsQualityCommandOutputWithMultilineEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"required_evidence_refs": ["snapshot:11", "alert:single"]`, `"required_evidence_refs": ["snapshot:11", "alert:single\ncontinued"]`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want multiline quality evidence ref rejection")
	}
	if !strings.Contains(err.Error(), "required_evidence_refs[1] must be a single-line value") {
		t.Fatalf("run err = %v, want evidence ref single-line error", err)
	}
}

func TestRunRejectsDuplicateQualityCommandOutputKeys(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"schema": "openclarion_sub_report"`, `"schema": "wrong_schema", "schema": "openclarion_sub_report"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate quality key rejection")
	}
	if !strings.Contains(err.Error(), `duplicate object key "schema"`) {
		t.Fatalf("run err = %v, want duplicate key error", err)
	}
}

func TestRunRejectsUnknownQualityCommandOutputFields(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"mode": "manifest"`, `"unexpected": "stale evidence", "mode": "manifest"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want unknown quality field rejection")
	}
	if !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("run err = %v, want unknown field error", err)
	}
}

func TestRunRejectsQualitySchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"schema": "openclarion_sub_report"`, `"schema": "wrong_schema"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want schema mismatch")
	}
	if !strings.Contains(err.Error(), `schema = "wrong_schema", want "openclarion_sub_report"`) {
		t.Fatalf("run err = %v, want schema mismatch", err)
	}
}

func TestRunRejectsQualitySummaryThatDoesNotMatchCases(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(
				validQualityOutput(),
				`{"id": "alert-storm", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:storm"], "recommendation": "sandbox_candidate_improved", "review_required": true}`,
				`{"id": "alert-storm", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:storm"], "recommendation": "sandbox_candidate_regressed", "review_required": true}`,
				1,
			)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want summary/case mismatch")
	}
	if !strings.Contains(err.Error(), "case-derived summary") {
		t.Fatalf("run err = %v, want case-derived summary error", err)
	}
}

func TestRunRejectsQualityCaseWithoutRequiredEvidenceRefs(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `, "required_evidence_refs": ["snapshot:11", "alert:single"]`, "", 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
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

func TestRunRejectsQualityCaseWithoutSnapshotEvidenceRef(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(
				validQualityOutput(),
				`"required_evidence_refs": ["snapshot:11", "alert:single"]`,
				`"required_evidence_refs": ["alert:single"]`,
				1,
			)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
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
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(
				validQualityOutput(),
				`"required_evidence_refs": ["snapshot:11", "alert:single"]`,
				`"required_evidence_refs": ["snapshot:001", "alert:single"]`,
				1,
			)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
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

func TestRunRejectsDuplicateQualityCaseIDBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `"id": "cascade"`, `"id": "single-alert"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate case id rejection")
	}
	if !strings.Contains(err.Error(), `duplicate quality comparison case id "single-alert"`) {
		t.Fatalf("run err = %v, want duplicate case id error", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["quality"])); !os.IsNotExist(statErr) {
		t.Fatalf("quality artifact exists after duplicate case id rejection: %v", statErr)
	}
}

func TestRunRejectsQualityCaseWithoutReviewRequired(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.Replace(validQualityOutput(), `, "review_required": true`, ``, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want missing case review_required")
	}
	if !strings.Contains(err.Error(), `case "single-alert" review_required must be true`) {
		t.Fatalf("run err = %v, want case review_required error", err)
	}
}

func TestRunRejectsQualityCoverageWithoutMatchingCases(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: strings.ReplaceAll(validQualityOutput(), `"scenario": "alert_storm"`, `"scenario": "single_alert"`)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want mismatched quality coverage rejection")
	}
	if !strings.Contains(err.Error(), `scenario_coverage = [single_alert cascade alert_storm], want [single_alert cascade]`) {
		t.Fatalf("run err = %v, want coverage mismatch error", err)
	}
}

func TestRunRejectsFutureReviewEvidenceDate(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(validReviewEvidence(), `"evidence_date": "2026-05-29"`, `"evidence_date": "2999-01-01"`, 1))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want future evidence date rejection")
	}
	if !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("run err = %v, want future-date error", err)
	}
}

func TestRunRejectsDuplicateReviewEvidenceKeys(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"runtime_candidate": "`+runtimeCandidateRef+`"`,
		`"runtime_candidate": "stale", "runtime_candidate": "`+runtimeCandidateRef+`"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want duplicate review key rejection")
	}
	if !strings.Contains(err.Error(), `duplicate object key "runtime_candidate"`) {
		t.Fatalf("run err = %v, want duplicate key error", err)
	}
}

func TestRunRejectsUnknownReviewEvidenceFields(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"runtime_candidate": "`+runtimeCandidateRef+`"`,
		`"unexpected": "stale evidence", "runtime_candidate": "`+runtimeCandidateRef+`"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want unknown review field rejection")
	}
	if !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("run err = %v, want unknown field error", err)
	}
}

func TestRunRejectsUnsupportedReviewStatus(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"status": "pass",
    "reviewer": "reviewer@example.com"`,
		`"status": "maybe",
    "reviewer": "reviewer@example.com"`,
		1,
	))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want unsupported review status")
	}
	if !strings.Contains(err.Error(), "want pass or fail") {
		t.Fatalf("run err = %v, want status enum error", err)
	}
}

func TestRunRejectsWeakDecisionCommandOutput(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: `{"tool":"sandbox_m4_decision","decision":"ship","review_required":true,"reasons":["bad"]}`},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want weak decision artifact rejection")
	}
	if !strings.Contains(err.Error(), "want proceed, iterate, or defer") {
		t.Fatalf("run err = %v, want decision enum error", err)
	}
}

func TestRunRejectsProceedDecisionWithNonCanonicalReason(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: strings.Replace(validDecisionOutput(), canonicalProceedReason, "manual proceed note", 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want non-canonical proceed reason rejection")
	}
	if !strings.Contains(err.Error(), "proceed decision reasons must be exactly") {
		t.Fatalf("run err = %v, want proceed reason consistency error", err)
	}
}

func TestRunRejectsProceedDecisionWithLoopbackRuntimeCandidate(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	loopbackRef := "localhost:5000/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.ReplaceAll(validReviewEvidence(), runtimeCandidateRef, loopbackRef))
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: strings.ReplaceAll(validDecisionOutput(), runtimeCandidateRef, loopbackRef)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want loopback proceed runtime candidate rejection")
	}
	if !strings.Contains(err.Error(), loopbackRuntimeCandidateReason) {
		t.Fatalf("run err = %v, want loopback runtime candidate error", err)
	}
}

func TestRunRejectsNonProceedDecisionWithProceedReason(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: strings.Replace(validDecisionOutput(), `"decision": "proceed"`, `"decision": "defer"`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", filepath.Join(dir, "packet"),
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want non-proceed success reason rejection")
	}
	if !strings.Contains(err.Error(), "defer decision must not use the proceed success reason") {
		t.Fatalf("run err = %v, want non-proceed success reason error", err)
	}
}

func TestValidateDecisionReasons(t *testing.T) {
	tests := []struct {
		name     string
		decision string
		reasons  []string
		wantErr  string
	}{
		{
			name:     "valid proceed",
			decision: decisionProceed,
			reasons:  []string{canonicalProceedReason},
		},
		{
			name:     "empty reasons",
			decision: decisionProceed,
			wantErr:  "must not be empty",
		},
		{
			name:     "blank reason",
			decision: decisionDefer,
			reasons:  []string{" "},
			wantErr:  "decision reasons[0] is required",
		},
		{
			name:     "whitespace padded reason",
			decision: decisionDefer,
			reasons:  []string{" missing evidence"},
			wantErr:  "must not contain leading or trailing whitespace",
		},
		{
			name:     "duplicate reason",
			decision: decisionIterate,
			reasons:  []string{"runtime smoke failed", "runtime smoke failed"},
			wantErr:  `duplicate decision reason "runtime smoke failed"`,
		},
		{
			name:     "proceed with non-canonical reason",
			decision: decisionProceed,
			reasons:  []string{"manual proceed note"},
			wantErr:  "proceed decision reasons must be exactly",
		},
		{
			name:     "proceed with extra reason",
			decision: decisionProceed,
			reasons:  []string{canonicalProceedReason, "manual proceed note"},
			wantErr:  "proceed decision reasons must be exactly",
		},
		{
			name:     "defer reuses proceed reason",
			decision: decisionDefer,
			reasons:  []string{canonicalProceedReason},
			wantErr:  "defer decision must not use the proceed success reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDecisionReasons(tt.decision, tt.reasons)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDecisionReasons() err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateDecisionReasons() err = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateDecisionReasons() err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsDecisionEvidenceThatDoesNotMatchArtifacts(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: strings.Replace(validDecisionOutput(), `"case_count": 3`, `"case_count": 2`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want decision evidence mismatch")
	}
	if !strings.Contains(err.Error(), `decision evidence case_count = 2, want quality comparison case_count 3`) {
		t.Fatalf("run err = %v, want decision evidence case_count mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["packet"])); !os.IsNotExist(statErr) {
		t.Fatalf("packet artifact exists after decision evidence mismatch: %v", statErr)
	}
}

func TestRunRejectsDecisionReviewedCaseCountMismatch(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: strings.Replace(validDecisionOutput(), `"reviewed_case_count": 3`, `"reviewed_case_count": 2`, 1)},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want reviewed case count mismatch")
	}
	if !strings.Contains(err.Error(), `decision evidence reviewed_case_count = 2`) {
		t.Fatalf("run err = %v, want reviewed_case_count mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["packet"])); !os.IsNotExist(statErr) {
		t.Fatalf("packet artifact exists after reviewed case count mismatch: %v", statErr)
	}
}

func TestRunRejectsReviewCaseIDsThatDoNotMatchQualityCases(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"id": "single-alert", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:single"], "status": "pass"`,
		`"id": "stale-single-alert", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:single"], "status": "pass"`,
		1,
	))
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want reviewed case ID mismatch")
	}
	if !strings.Contains(err.Error(), `review evidence missing reviewed case "single-alert"`) {
		t.Fatalf("run err = %v, want missing reviewed case error", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["packet"])); !os.IsNotExist(statErr) {
		t.Fatalf("packet artifact exists after reviewed case ID mismatch: %v", statErr)
	}
}

func TestRunRejectsReviewCaseScenarioThatDoesNotMatchQualityCase(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"id": "cascade", "scenario": "cascade"`,
		`"id": "cascade", "scenario": "single_alert"`,
		1,
	))
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want reviewed case scenario mismatch")
	}
	if !strings.Contains(err.Error(), `review evidence reviewed case "cascade" scenario = "single_alert", want quality comparison scenario "cascade"`) {
		t.Fatalf("run err = %v, want reviewed case scenario mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["packet"])); !os.IsNotExist(statErr) {
		t.Fatalf("packet artifact exists after reviewed case scenario mismatch: %v", statErr)
	}
}

func TestRunRejectsReviewCaseRequiredEvidenceRefsThatDoNotMatchQualityCase(t *testing.T) {
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", strings.Replace(
		validReviewEvidence(),
		`"id": "cascade", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:cascade"]`,
		`"id": "cascade", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:stale-cascade"]`,
		1,
	))
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}

	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner)
	if err == nil {
		t.Fatal("run err = nil, want reviewed case required_evidence_refs mismatch")
	}
	if !strings.Contains(err.Error(), `review evidence reviewed case "cascade" required_evidence_refs = [snapshot:12 alert:stale-cascade], want quality comparison refs [snapshot:12 alert:cascade]`) {
		t.Fatalf("run err = %v, want reviewed case refs mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, artifactNames["packet"])); !os.IsNotExist(statErr) {
		t.Fatalf("packet artifact exists after reviewed case refs mismatch: %v", statErr)
	}
}

func TestRunRequiresFlags(t *testing.T) {
	var stdout bytes.Buffer
	err := runWithRunner(context.Background(), nil, &stdout, &fakeRunner{})
	if err == nil {
		t.Fatal("run err = nil, want missing flag error")
	}
	if !strings.Contains(err.Error(), "--quality-manifest is required") {
		t.Fatalf("run err = %v, want quality-manifest requirement", err)
	}
}

type fakeRunner struct {
	responses []fakeResponse
	calls     [][]string
}

type fakeResponse struct {
	stdout string
	stderr string
	err    error
}

var runtimeSmokeArtifactBodies = map[string]string{
	"artifacts/m4/runtime/agent-runtime-smoke.json":                 `{"tool":"agent-runtime-smoke","status":"pass"}` + "\n",
	"artifacts/m4/runtime/container-provider-smoke.json":            `{"tool":"container-provider-smoke","status":"pass"}` + "\n",
	"artifacts/m4/runtime/container-provider-timeout-smoke.json":    `{"tool":"container-provider-timeout-smoke","status":"pass"}` + "\n",
	"artifacts/m4/runtime/container-provider-output-cap-smoke.json": `{"tool":"container-provider-output-cap-smoke","status":"pass"}` + "\n",
	"artifacts/m4/runtime/egress-allowdeny-smoke.json":              `{"tool":"egress-allowdeny-smoke","status":"pass"}` + "\n",
}

func (r *fakeRunner) Run(_ context.Context, spec commandSpec) ([]byte, []byte, error) {
	r.calls = append(r.calls, append([]string(nil), spec.Args...))
	if len(r.responses) == 0 {
		return nil, []byte("unexpected command"), errors.New("unexpected command")
	}
	next := r.responses[0]
	r.responses = r.responses[1:]
	return []byte(next.stdout), []byte(next.stderr), next.err
}

func assembleValidPacketFixture(t *testing.T) (string, packetOutput) {
	t.Helper()
	dir := t.TempDir()
	qualityManifest := writeFile(t, dir, "quality-manifest.json", `{"cases":[{"id":"payments-cpu"}]}`)
	reviewEvidence := writeFile(t, dir, "review-evidence.json", validReviewEvidence())
	outDir := filepath.Join(dir, "packet")
	runner := &fakeRunner{
		responses: []fakeResponse{
			{stdout: validBaselineOutput()},
			{stdout: validQualityOutput()},
			{stdout: validDecisionOutput()},
		},
	}
	var stdout bytes.Buffer
	if err := runWithRunner(context.Background(), []string{
		"--quality-manifest", qualityManifest,
		"--review-evidence", reviewEvidence,
		"--out-dir", outDir,
	}, &stdout, runner); err != nil {
		t.Fatalf("assemble valid packet: %v", err)
	}
	var packet packetOutput
	if err := json.NewDecoder(&stdout).Decode(&packet); err != nil {
		t.Fatalf("decode packet stdout %q: %v", stdout.String(), err)
	}
	return outDir, packet
}

func writePacketOutput(t *testing.T, outDir string, packet packetOutput) {
	t.Helper()
	raw, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(packetArtifactPath(outDir, packet.Artifacts.Packet), raw, 0o600); err != nil {
		t.Fatalf("write packet: %v", err)
	}
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	if name == "quality-manifest.json" && strings.TrimSpace(body) == `{"cases":[{"id":"payments-cpu"}]}` {
		body = validQualityManifest()
	}
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if name == "quality-manifest.json" && strings.Contains(body, `"direct_sub_report"`) {
		writeQualityInputReports(t, dir)
	}
	if name == "review-evidence.json" && strings.Contains(body, `"tool": "sandbox_m4_review_evidence"`) {
		writeRuntimeSmokeArtifacts(t, dir)
	}
	return path
}

func writeQualityInputReports(t *testing.T, root string) {
	t.Helper()
	for ref, body := range qualityInputReportBodies {
		writeFile(t, root, ref, body)
	}
}

func writeRuntimeSmokeArtifacts(t *testing.T, root string) {
	t.Helper()
	for ref, body := range runtimeSmokeArtifactBodies {
		writeFile(t, root, ref, body)
	}
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runtimeSmokeArtifactSHA256(ref string) string {
	sum := sha256.Sum256([]byte(runtimeSmokeArtifactBodies[ref]))
	return hex.EncodeToString(sum[:])
}

func packetArtifactPath(root, ref string) string {
	return filepath.Join(root, filepath.FromSlash(ref))
}

func assertPortablePacketPath(t *testing.T, ref string) {
	t.Helper()
	if ref == "" {
		t.Fatal("packet path is empty")
	}
	if path.IsAbs(ref) || strings.Contains(ref, "\\") || path.Clean(ref) != ref || strings.HasPrefix(ref, "../") || ref == ".." {
		t.Fatalf("packet path = %q, want normalized slash-separated relative path", ref)
	}
}

func testFileSHA256Hex(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- tests hash artifact paths produced in t.TempDir by the
	// packet helper under test.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func containsAll(values []string, want ...string) bool {
	for _, item := range want {
		found := false
		for _, value := range values {
			if value == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func validBaselineOutput() string {
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

func validQualityManifest() string {
	return `{
  "sample_basis": "representative alert-analysis scenarios",
  "cases": [
    {"id": "single-alert", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:single"], "direct_sub_report": "direct/single-alert.json", "sandbox_sub_report": "sandbox/single-alert.json"},
    {"id": "cascade", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:cascade"], "direct_sub_report": "direct/cascade.json", "sandbox_sub_report": "sandbox/cascade.json"},
    {"id": "alert-storm", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:storm"], "direct_sub_report": "direct/alert-storm.json", "sandbox_sub_report": "sandbox/alert-storm.json"}
  ]
}`
}

func validQualityOutput() string {
	return `{
  "tool": "sandbox_quality_compare",
  "schema": "openclarion_sub_report",
  "mode": "manifest",
  "case_count": 3,
  "sample_basis": "representative alert-analysis scenarios",
  "scenario_coverage": ["single_alert", "cascade", "alert_storm"],
  "summary": {
    "improved_count": 3,
    "equivalent_count": 0,
    "regressed_count": 0,
    "needs_human_review_count": 0
  },
  "recommendation": "sandbox_batch_candidate_improved",
  "review_required": true,
  "cases": [
    {"id": "single-alert", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:single"], "recommendation": "sandbox_candidate_improved", "review_required": true},
    {"id": "cascade", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:cascade"], "recommendation": "sandbox_candidate_improved", "review_required": true},
    {"id": "alert-storm", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:storm"], "recommendation": "sandbox_candidate_improved", "review_required": true}
  ]
}`
}

var qualityInputReportBodies = map[string]string{
	"direct/single-alert.json": `{
  "title": "Single alert direct report",
  "summary": "The direct report summarizes the single alert signal.",
  "severity": "warning",
  "confidence": "medium",
  "findings": [
    {"label": "Alert", "detail": "The single alert crossed the configured warning threshold.", "evidence_id": "alert:single"}
  ],
  "recommended_actions": [
    {"label": "Review alert", "detail": "Review the alert and confirm the affected service owner.", "priority": "medium"}
  ],
  "evidence_refs": ["snapshot:11", "alert:single"]
}`,
	"sandbox/single-alert.json": `{
  "title": "Single alert sandbox report",
  "summary": "The sandbox report adds correlated queue evidence for the single alert.",
  "severity": "warning",
  "confidence": "high",
  "findings": [
    {"label": "Alert", "detail": "The single alert crossed the configured warning threshold.", "evidence_id": "alert:single"},
    {"label": "Queue", "detail": "Queue latency increased during the alert window.", "evidence_id": "metric:queue"}
  ],
  "recommended_actions": [
    {"label": "Review alert", "detail": "Review the alert and confirm the affected service owner.", "priority": "high"},
    {"label": "Inspect queue", "detail": "Inspect queue latency and recent deploys for correlation.", "priority": "medium"}
  ],
  "evidence_refs": ["snapshot:11", "alert:single", "metric:queue"]
}`,
	"direct/cascade.json": `{
  "title": "Cascade direct report",
  "summary": "The direct report summarizes the initial cascade signal.",
  "severity": "warning",
  "confidence": "medium",
  "findings": [
    {"label": "Cascade", "detail": "A downstream cascade was detected from the alert stream.", "evidence_id": "alert:cascade"}
  ],
  "recommended_actions": [
    {"label": "Review cascade", "detail": "Review upstream dependencies for the impacted group.", "priority": "medium"}
  ],
  "evidence_refs": ["snapshot:12", "alert:cascade"]
}`,
	"sandbox/cascade.json": `{
  "title": "Cascade sandbox report",
  "summary": "The sandbox report adds topology context to the cascade signal.",
  "severity": "warning",
  "confidence": "high",
  "findings": [
    {"label": "Cascade", "detail": "A downstream cascade was detected from the alert stream.", "evidence_id": "alert:cascade"},
    {"label": "Topology", "detail": "Topology shows the affected dependency chain.", "evidence_id": "topology:edge"}
  ],
  "recommended_actions": [
    {"label": "Review cascade", "detail": "Review upstream dependencies for the impacted group.", "priority": "high"},
    {"label": "Notify owner", "detail": "Notify the upstream owner with the affected topology edge.", "priority": "medium"}
  ],
  "evidence_refs": ["snapshot:12", "alert:cascade", "topology:edge"]
}`,
	"direct/alert-storm.json": `{
  "title": "Alert storm direct report",
  "summary": "The direct report summarizes the alert storm volume.",
  "severity": "critical",
  "confidence": "medium",
  "findings": [
    {"label": "Storm", "detail": "Alert volume exceeded the storm threshold.", "evidence_id": "alert:storm"}
  ],
  "recommended_actions": [
    {"label": "Open incident", "detail": "Open an incident channel for coordinated triage.", "priority": "high"}
  ],
  "evidence_refs": ["snapshot:13", "alert:storm"]
}`,
	"sandbox/alert-storm.json": `{
  "title": "Alert storm sandbox report",
  "summary": "The sandbox report adds grouped symptoms and blast-radius context for the alert storm.",
  "severity": "critical",
  "confidence": "high",
  "findings": [
    {"label": "Storm", "detail": "Alert volume exceeded the storm threshold.", "evidence_id": "alert:storm"},
    {"label": "Blast radius", "detail": "Grouped symptoms point to a shared dependency.", "evidence_id": "topology:blast-radius"}
  ],
  "recommended_actions": [
    {"label": "Open incident", "detail": "Open an incident channel for coordinated triage.", "priority": "high"},
    {"label": "Page dependency owner", "detail": "Page the shared dependency owner with grouped symptoms.", "priority": "high"}
  ],
  "evidence_refs": ["snapshot:13", "alert:storm", "topology:blast-radius"]
}`,
}

func validDecisionOutput() string {
	return `{
  "tool": "sandbox_m4_decision",
  "decision": "proceed",
  "review_required": true,
  "evidence": {
    "baseline_audit_status": "pass",
    "quality_recommendation": "sandbox_batch_candidate_improved",
    "case_count": 3,
    "minimum_case_count": 3,
    "sample_basis": "representative alert-analysis scenarios",
    "scenario_coverage": ["single_alert", "cascade", "alert_storm"],
    "selected_candidate": "runtime-candidate-a",
    "runtime_candidate": "` + runtimeCandidateRef + `",
    "candidate_evaluation_count": 3,
    "runtime_smoke_passed_count": 5,
    "reviewed_case_count": 3,
    "representative_sample": true,
    "human_review_status": "pass"
  },
  "reasons": ["` + canonicalProceedReason + `"]
}`
}

func deferDecisionOutput() string {
	out := validDecisionOutput()
	out = strings.Replace(out, `"decision": "proceed"`, `"decision": "defer"`, 1)
	out = strings.Replace(out, `"representative_sample": true`, `"representative_sample": false`, 1)
	out = strings.Replace(out, `"human_review_status": "pass"`, `"human_review_status": "fail"`, 1)
	out = strings.Replace(out,
		`"reasons": ["`+canonicalProceedReason+`"]`,
		`"reasons": ["review evidence representative_sample must be true", "selected candidate \"runtime-candidate-a\" evaluation status = \"fail\", want pass", "human review status = \"fail\", want pass"]`,
		1,
	)
	return out
}

func validReviewEvidence() string {
	return `{
  "tool": "sandbox_m4_review_evidence",
  "evidence_date": "2026-05-29",
  "selected_candidate": "runtime-candidate-a",
  "runtime_candidate": "` + runtimeCandidateRef + `",
  "representative_sample": true,
  "sample_basis": "representative alert-analysis scenarios",
  "candidate_evaluations": [
    {"candidate": "runtime-a", "status": "not_fit", "source": "candidate smoke review", "notes": "candidate still needs a bounded one-shot JSON-file proof"},
    {"candidate": "runtime-b", "status": "fail", "source": "candidate smoke review", "notes": "candidate did not satisfy the current readonly file-contract smoke"},
    {"candidate": "runtime-candidate-a", "status": "pass", "runtime_candidate": "` + runtimeCandidateRef + `", "runtime_smoke_refs": ["candidate_runtime_file_contract", "container_provider_lifecycle", "container_provider_timeout_cleanup", "container_provider_output_cap", "egress_allowdeny"], "source": "candidate runtime smoke review", "notes": "candidate runtime passed contract and lifecycle smoke as retained review evidence"}
	  ],
	  "runtime_smokes": [
	    {"name": "candidate_runtime_file_contract", "status": "pass", "source": "make agent-runtime-smoke", "evidence_ref": "artifacts/m4/runtime/agent-runtime-smoke.json", "evidence_sha256": "` + runtimeSmokeArtifactSHA256("artifacts/m4/runtime/agent-runtime-smoke.json") + `"},
	    {"name": "container_provider_lifecycle", "status": "pass", "source": "make container-provider-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-smoke.json", "evidence_sha256": "` + runtimeSmokeArtifactSHA256("artifacts/m4/runtime/container-provider-smoke.json") + `"},
	    {"name": "container_provider_timeout_cleanup", "status": "pass", "source": "make container-provider-timeout-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-timeout-smoke.json", "evidence_sha256": "` + runtimeSmokeArtifactSHA256("artifacts/m4/runtime/container-provider-timeout-smoke.json") + `"},
	    {"name": "container_provider_output_cap", "status": "pass", "source": "make container-provider-output-cap-smoke", "evidence_ref": "artifacts/m4/runtime/container-provider-output-cap-smoke.json", "evidence_sha256": "` + runtimeSmokeArtifactSHA256("artifacts/m4/runtime/container-provider-output-cap-smoke.json") + `"},
	    {"name": "egress_allowdeny", "status": "pass", "source": "make egress-allowdeny-smoke", "evidence_ref": "artifacts/m4/runtime/egress-allowdeny-smoke.json", "evidence_sha256": "` + runtimeSmokeArtifactSHA256("artifacts/m4/runtime/egress-allowdeny-smoke.json") + `"}
  ],
  "reviewed_cases": [
    {"id": "single-alert", "scenario": "single_alert", "required_evidence_refs": ["snapshot:11", "alert:single"], "status": "pass", "notes": "single-alert sample preserves evidence traceability"},
    {"id": "cascade", "scenario": "cascade", "required_evidence_refs": ["snapshot:12", "alert:cascade"], "status": "pass", "notes": "cascade sample preserves evidence traceability"},
    {"id": "alert-storm", "scenario": "alert_storm", "required_evidence_refs": ["snapshot:13", "alert:storm"], "status": "pass", "notes": "alert-storm sample preserves evidence traceability"}
  ],
  "human_review": {
    "status": "pass",
    "reviewer": "reviewer@example.com",
    "notes": "Representative alert-analysis scenarios reviewed."
  }
}`
}

func failClosedReviewEvidence() string {
	out := validReviewEvidence()
	out = strings.Replace(out, `"representative_sample": true`, `"representative_sample": false`, 1)
	out = strings.Replace(out,
		`{"candidate": "runtime-candidate-a", "status": "pass", "runtime_candidate": "`+runtimeCandidateRef+`"`,
		`{"candidate": "runtime-candidate-a", "status": "fail", "runtime_candidate": "`+runtimeCandidateRef+`"`,
		1,
	)
	out = strings.Replace(out,
		`"status": "pass",
    "reviewer": "reviewer@example.com"`,
		`"status": "fail",
    "reviewer": "reviewer@example.com"`,
		1,
	)
	for _, note := range []string{
		"single-alert sample preserves evidence traceability",
		"cascade sample preserves evidence traceability",
		"alert-storm sample preserves evidence traceability",
	} {
		out = strings.Replace(out,
			`"status": "pass", "notes": "`+note+`"`,
			`"status": "fail", "notes": "`+note+`"`,
			1,
		)
	}
	return out
}
