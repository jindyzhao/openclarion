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
)

func TestRunAcceptsJSONObject(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok","findings":[]}`)

	var stdout bytes.Buffer
	if err := run([]string{path}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Fatalf("stdout = %q, want OK", stdout.String())
	}
}

func TestRunWritesProofArtifact(t *testing.T) {
	content := `{"summary":"ok","findings":[]}`
	path := writeOutput(t, content)
	proofPath := filepath.Join(t.TempDir(), "proof", "agent-runtime-smoke.json")

	var stdout bytes.Buffer
	err := run([]string{
		"--proof", proofPath,
		"--runtime-candidate", "registry.example.com/openclarion/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--source", "make agent-runtime-smoke",
		"--output-max-bytes", "4096",
		path,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// #nosec G304 -- proofPath is created inside this test's temporary directory.
	raw, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var proof proofArtifact
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatalf("unmarshal proof: %v\n%s", err, raw)
	}
	sum := sha256.Sum256([]byte(content))
	if proof.Tool != "agent-runtime-smoke" {
		t.Fatalf("proof tool = %q", proof.Tool)
	}
	if proof.Status != "pass" {
		t.Fatalf("proof status = %q", proof.Status)
	}
	if proof.Source != "make agent-runtime-smoke" {
		t.Fatalf("proof source = %q", proof.Source)
	}
	if proof.RuntimeCandidate != "registry.example.com/openclarion/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("proof runtime candidate = %q", proof.RuntimeCandidate)
	}
	if proof.Output.Path != "/workspace/out/output.json" {
		t.Fatalf("proof output path = %q, want container path", proof.Output.Path)
	}
	if strings.Contains(string(raw), filepath.Dir(path)) {
		t.Fatalf("proof contains host temp path:\n%s", raw)
	}
	if proof.Output.Bytes != int64(len(content)) {
		t.Fatalf("proof output bytes = %d, want %d", proof.Output.Bytes, len(content))
	}
	if proof.Output.MaxBytes != 4096 {
		t.Fatalf("proof output max bytes = %d, want 4096", proof.Output.MaxBytes)
	}
	if proof.Output.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("proof output sha256 = %q, want %q", proof.Output.SHA256, hex.EncodeToString(sum[:]))
	}
	if len(proof.Checks) != 5 {
		t.Fatalf("proof checks = %d, want 5", len(proof.Checks))
	}
}

func TestRunRejectsSymlinkOutput(t *testing.T) {
	target := writeOutput(t, `{"summary":"ok"}`)
	link := filepath.Join(t.TempDir(), "output-link.json")
	createSymlinkOrSkip(t, target, link)

	var stdout bytes.Buffer
	err := run([]string{link}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty on symlink rejection", stdout.String())
	}
}

func TestRunRejectsSymlinkOutputParent(t *testing.T) {
	realDir := t.TempDir()
	target := filepath.Join(realDir, "output.json")
	if err := os.WriteFile(target, []byte(`{"summary":"ok"}`), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	linkDir := filepath.Join(t.TempDir(), "out-link")
	createSymlinkOrSkip(t, realDir, linkDir)

	var stdout bytes.Buffer
	err := run([]string{filepath.Join(linkDir, "output.json")}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink parent rejection")
	}
	if !strings.Contains(err.Error(), "parent directory") || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("run err = %v, want symlink parent rejection", err)
	}
}

func TestRunRejectsSymlinkProof(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok"}`)
	target := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(target, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write proof target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "proof-link.json")
	createSymlinkOrSkip(t, target, link)

	var stdout bytes.Buffer
	err := run([]string{"--proof", link, path}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink proof rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
}

func TestRunRejectsSymlinkProofParent(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok"}`)
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "proof-link")
	createSymlinkOrSkip(t, realDir, linkDir)

	var stdout bytes.Buffer
	err := run([]string{"--proof", filepath.Join(linkDir, "proof.json"), path}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink proof parent rejection")
	}
	if !strings.Contains(err.Error(), "parent directory") || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("run err = %v, want symlink parent rejection", err)
	}
}

func TestRunRejectsInvalidOutputs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty", content: "", wantErr: "empty"},
		{name: "invalid JSON", content: "{", wantErr: "invalid JSON"},
		{name: "array", content: `[]`, wantErr: "JSON object"},
		{name: "empty object", content: `{}`, wantErr: "non-empty JSON object"},
		{name: "duplicate key", content: `{"summary":"stale","summary":"ok"}`, wantErr: "duplicate object key"},
		{name: "trailing JSON", content: `{"summary":"ok"} {"summary":"other"}`, wantErr: "trailing JSON values"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeOutput(t, tt.content)
			var stdout bytes.Buffer
			err := run([]string{path}, &stdout)
			if err == nil {
				t.Fatal("run err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsConfiguredOutputCapOverflow(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok"}`)

	var stdout bytes.Buffer
	err := run([]string{"--output-max-bytes", "4", path}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want output cap rejection")
	}
	if !strings.Contains(err.Error(), "exceeds maximum 4") {
		t.Fatalf("run err = %v, want output cap rejection", err)
	}
}

func TestRunRejectsInvalidRuntimeCandidate(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok"}`)

	var stdout bytes.Buffer
	err := run([]string{"--runtime-candidate", "openclarion/agent:latest", path}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want runtime candidate rejection")
	}
	if !strings.Contains(err.Error(), "pinned by sha256 digest") {
		t.Fatalf("run err = %v, want digest-pinned rejection", err)
	}
}

func TestRunRejectsWrongUsage(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want usage error")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("run err = %v, want usage", err)
	}
}

func writeOutput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}
