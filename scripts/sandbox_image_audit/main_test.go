package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testDigestRef = "registry.example.test/openclarion/diagnosis-assistant-runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestAuditImageAcceptsDiagnosisEntrypoint(t *testing.T) {
	checkedAt := time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC)
	out, err := auditImage(
		context.Background(),
		config{imageRef: testDigestRef, expectedEntrypoint: defaultExpectedEntrypoint},
		fakeInspector{cfg: dockerImageConfig{
			Entrypoint: []string{defaultExpectedEntrypoint},
			User:       "65532:65532",
			WorkingDir: "/workspace",
			Labels: map[string]string{
				"org.opencontainers.image.title": "diagnosis-assistant-runner",
			},
		}},
		checkedAt,
	)
	if err != nil {
		t.Fatalf("auditImage: %v", err)
	}
	if out.Tool != toolName ||
		out.Status != "pass" ||
		out.ImageDigest != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" ||
		out.ImageRefSHA256 == "" ||
		out.CheckedAt != checkedAt.Format(time.RFC3339Nano) ||
		out.Entrypoint[0] != defaultExpectedEntrypoint ||
		out.GenericContractRunner ||
		out.SecretValuesRetained {
		t.Fatalf("audit output = %+v", out)
	}
}

func TestAuditImageRejectsGenericContractRunner(t *testing.T) {
	_, err := auditImage(
		context.Background(),
		config{imageRef: testDigestRef, expectedEntrypoint: defaultExpectedEntrypoint},
		fakeInspector{cfg: dockerImageConfig{Entrypoint: []string{"/custom-thin-runner"}}},
		time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("auditImage: want generic runner rejection")
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Fatalf("error = %q, want entrypoint rejection", err.Error())
	}
}

func TestAuditImageRejectsGenericRunnerInCmd(t *testing.T) {
	_, err := auditImage(
		context.Background(),
		config{imageRef: testDigestRef, expectedEntrypoint: defaultExpectedEntrypoint},
		fakeInspector{cfg: dockerImageConfig{
			Entrypoint: []string{defaultExpectedEntrypoint},
			Cmd:        []string{"--runtime=custom-thin-runner"},
		}},
		time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("auditImage: want generic runner rejection")
	}
	if !strings.Contains(err.Error(), "generic custom-thin-runner") {
		t.Fatalf("error = %q, want generic runner rejection", err.Error())
	}
}

func TestAuditImageRequiresDigestRef(t *testing.T) {
	_, err := auditImage(
		context.Background(),
		config{imageRef: "registry.example.test/openclarion/diagnosis-assistant-runner:dev", expectedEntrypoint: defaultExpectedEntrypoint},
		fakeInspector{},
		time.Date(2026, 6, 19, 1, 2, 3, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("auditImage: want digest ref rejection")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("error = %q, want digest rejection", err.Error())
	}
}

func TestRunWritesProof(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof.json")
	var stdout strings.Builder
	err := run(
		context.Background(),
		[]string{"--image-ref", testDigestRef, "--proof-out", path},
		nil,
		fakeInspector{cfg: dockerImageConfig{Entrypoint: []string{defaultExpectedEntrypoint}}},
		&stdout,
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat proof: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("proof mode = %v, want 0600", info.Mode().Perm())
	}
	var proof auditOutput
	if err := json.Unmarshal([]byte(stdout.String()), &proof); err != nil {
		t.Fatalf("decode stdout proof: %v", err)
	}
	if proof.Tool != toolName {
		t.Fatalf("stdout proof = %+v", proof)
	}
}

func TestWriteProofFileRejectsExistingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write existing proof: %v", err)
	}
	err := writeProofFile(path, auditOutput{Tool: toolName})
	if err == nil {
		t.Fatal("writeProofFile: want existing path error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %q, want existing path rejection", err.Error())
	}
}

func TestRunPropagatesInspectorError(t *testing.T) {
	err := run(
		context.Background(),
		[]string{"--image-ref", testDigestRef},
		nil,
		fakeInspector{err: errors.New("docker unavailable")},
		ioDiscard{},
	)
	if err == nil {
		t.Fatal("run: want inspector error")
	}
	if !strings.Contains(err.Error(), "docker unavailable") {
		t.Fatalf("error = %q, want inspector error", err.Error())
	}
}

type fakeInspector struct {
	cfg dockerImageConfig
	err error
}

func (i fakeInspector) InspectImageConfig(context.Context, string) (dockerImageConfig, error) {
	if i.err != nil {
		return dockerImageConfig{}, i.err
	}
	return i.cfg, nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
