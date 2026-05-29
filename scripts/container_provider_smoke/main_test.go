package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const smokePinnedImage = "registry.example.com/openclarion/provider-smoke@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestConfigFromEnvBuildsDefaultProviderSmoke(t *testing.T) {
	cfg, cleanup, err := configFromEnv([]string{
		envImageRef + "=" + smokePinnedImage,
		envInvocationID + "=container-provider-smoke-test",
		envCommandJSON + `=["sh","-c","true"]`,
	})
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}()

	if cfg.ImageRef != smokePinnedImage {
		t.Fatalf("ImageRef = %q", cfg.ImageRef)
	}
	if strings.Join(cfg.Command, " ") != "sh -c true" {
		t.Fatalf("Command = %v", cfg.Command)
	}
	if cfg.AgentName != defaultAgentName {
		t.Fatalf("AgentName = %q, want %q", cfg.AgentName, defaultAgentName)
	}
	if cfg.Timeout != defaultTimeoutSeconds*time.Second {
		t.Fatalf("Timeout = %s", cfg.Timeout)
	}
	if cfg.OutputMax != ports.DefaultContainerOutputBytes {
		t.Fatalf("OutputMax = %d", cfg.OutputMax)
	}
	if cfg.ExpectError != "" {
		t.Fatalf("ExpectError = %q, want empty", cfg.ExpectError)
	}
	if cfg.ProofSource != defaultProofSource {
		t.Fatalf("ProofSource = %q, want %q", cfg.ProofSource, defaultProofSource)
	}
	if !json.Valid(cfg.Evidence) || !json.Valid(cfg.Conversation) || !json.Valid(cfg.Message) {
		t.Fatalf("default JSON fixtures must be valid")
	}
	if _, err := os.Stat(filepath.Join(cfg.AgentConfigRoot, cfg.AgentName, "agent.yaml")); err != nil {
		t.Fatalf("generated agent config missing: %v", err)
	}

	req := requestFromConfig(cfg)
	if err := req.Validate(); err != nil {
		t.Fatalf("request Validate: %v", err)
	}
	if req.Network.EffectiveMode() != ports.ContainerNetworkNone {
		t.Fatalf("Network = %q, want none", req.Network.EffectiveMode())
	}
}

func TestConfigFromEnvRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		wantErr string
	}{
		{
			name:    "missing image",
			env:     nil,
			wantErr: envImageRef,
		},
		{
			name: "tagged image",
			env: []string{
				envImageRef + "=registry.example.com/openclarion/provider-smoke:latest",
				envInvocationID + "=container-provider-smoke-test",
			},
			wantErr: "digest",
		},
		{
			name: "empty command argument",
			env: []string{
				envImageRef + "=" + smokePinnedImage,
				envInvocationID + "=container-provider-smoke-test",
				envCommandJSON + `=["sh",""]`,
			},
			wantErr: "non-empty",
		},
		{
			name: "invalid timeout",
			env: []string{
				envImageRef + "=" + smokePinnedImage,
				envInvocationID + "=container-provider-smoke-test",
				envTimeoutSeconds + "=0",
			},
			wantErr: "positive integer",
		},
		{
			name: "invalid evidence",
			env: []string{
				envImageRef + "=" + smokePinnedImage,
				envInvocationID + "=container-provider-smoke-test",
				envEvidenceJSON + "=[]",
			},
			wantErr: "evidence JSON must be an object",
		},
		{
			name: "non canonical proof source",
			env: []string{
				envImageRef + "=" + smokePinnedImage,
				envInvocationID + "=container-provider-smoke-test",
				envProofSource + "=manual note",
			},
			wantErr: "canonical container provider smoke make target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cleanup, err := configFromEnv(tt.env)
			if cleanup != nil {
				defer cleanup()
			}
			if err == nil {
				t.Fatal("configFromEnv err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("configFromEnv err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigFromEnvAcceptsExpectedErrorMode(t *testing.T) {
	cfg, cleanup, err := configFromEnv([]string{
		envImageRef + "=" + smokePinnedImage,
		envInvocationID + "=container-provider-smoke-timeout-test",
		envCommandJSON + `=["sh","-c","sleep 30"]`,
		envTimeoutSeconds + "=2",
		envExpectError + "=context deadline exceeded",
		envProofSource + "=make container-provider-timeout-smoke",
	})
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}()

	if cfg.ExpectError != "context deadline exceeded" {
		t.Fatalf("ExpectError = %q", cfg.ExpectError)
	}
	if cfg.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %s, want 2s", cfg.Timeout)
	}
	if cfg.ProofSource != "make container-provider-timeout-smoke" {
		t.Fatalf("ProofSource = %q", cfg.ProofSource)
	}
}

func TestHandleProviderRunResult(t *testing.T) {
	tests := []struct {
		name    string
		cfg     smokeConfig
		result  ports.ContainerRunResult
		runErr  error
		wantErr string
		wantOut string
	}{
		{
			name:    "success",
			result:  ports.ContainerRunResult{Output: json.RawMessage(`{"provider_smoke":"ok"}`)},
			wantOut: "output_bytes",
		},
		{
			name:    "unexpected provider error",
			runErr:  errors.New("create failed"),
			wantErr: "provider run",
		},
		{
			name:    "expected provider error allows alternatives",
			cfg:     smokeConfig{ExpectError: "exited with code||exceeds maximum"},
			runErr:  errors.New("docker container output size exceeds maximum"),
			wantOut: "expected provider error",
		},
		{
			name:    "expected provider error but success",
			cfg:     smokeConfig{ExpectError: "context deadline exceeded"},
			wantErr: "want error",
		},
		{
			name:    "expected provider error mismatch",
			cfg:     smokeConfig{ExpectError: "context deadline exceeded"},
			runErr:  errors.New("exited with code 42"),
			wantErr: "does not contain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			outcome, err := handleProviderRunResult(tt.cfg, tt.result, tt.runErr, &stdout)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("handleProviderRunResult: %v", err)
				}
				if outcome.Mode == "" {
					t.Fatal("outcome mode is empty")
				}
				if !strings.Contains(stdout.String(), tt.wantOut) {
					t.Fatalf("stdout = %q, want containing %q", stdout.String(), tt.wantOut)
				}
				return
			}
			if err == nil {
				t.Fatal("handleProviderRunResult err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("handleProviderRunResult err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWriteProofForSuccessfulRun(t *testing.T) {
	output := json.RawMessage(`{"provider_smoke":"ok"}`)
	sum := sha256.Sum256(output)
	proofPath := filepath.Join(t.TempDir(), "proof", "container-provider-smoke.json")
	cfg := smokeConfig{
		ImageRef:     smokePinnedImage,
		InvocationID: "container-provider-smoke-test",
		Timeout:      60 * time.Second,
		OutputMax:    4096,
		ProofPath:    proofPath,
		ProofSource:  "make container-provider-smoke",
	}
	outcome := runOutcome{
		Mode:         "success",
		RuntimeID:    "runtime-1",
		OutputBytes:  len(output),
		OutputSHA256: hex.EncodeToString(sum[:]),
	}
	if err := writeProof(cfg, outcome); err != nil {
		t.Fatalf("writeProof: %v", err)
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
	if proof.Tool != "container-provider-smoke" {
		t.Fatalf("Tool = %q", proof.Tool)
	}
	if proof.Source != "make container-provider-smoke" {
		t.Fatalf("Source = %q", proof.Source)
	}
	if proof.ImageRef != smokePinnedImage {
		t.Fatalf("ImageRef = %q", proof.ImageRef)
	}
	if proof.Mode != "success" {
		t.Fatalf("Mode = %q", proof.Mode)
	}
	if proof.Output == nil {
		t.Fatal("Output is nil")
	}
	if proof.Output.Bytes != len(output) {
		t.Fatalf("Output.Bytes = %d, want %d", proof.Output.Bytes, len(output))
	}
	if proof.Output.MaxBytes != 4096 {
		t.Fatalf("Output.MaxBytes = %d, want 4096", proof.Output.MaxBytes)
	}
	if proof.Output.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("Output.SHA256 = %q", proof.Output.SHA256)
	}
	if strings.Contains(string(raw), filepath.Dir(proofPath)) {
		t.Fatalf("proof contains host temp path:\n%s", raw)
	}
}

func TestWriteProofForExpectedError(t *testing.T) {
	proofPath := filepath.Join(t.TempDir(), "proof.json")
	cfg := smokeConfig{
		ImageRef:    smokePinnedImage,
		Timeout:     2 * time.Second,
		OutputMax:   64,
		ExpectError: "context deadline exceeded||exceeds maximum",
		ProofPath:   proofPath,
		ProofSource: "make container-provider-timeout-smoke",
	}
	outcome := runOutcome{
		Mode:                "expected_error",
		MatchedErrorPattern: "context deadline exceeded",
	}
	if err := writeProof(cfg, outcome); err != nil {
		t.Fatalf("writeProof: %v", err)
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
	if proof.Mode != "expected_error" {
		t.Fatalf("Mode = %q", proof.Mode)
	}
	if proof.Expected == nil {
		t.Fatal("Expected is nil")
	}
	if proof.Expected.Matched != "context deadline exceeded" {
		t.Fatalf("Expected.Matched = %q", proof.Expected.Matched)
	}
	if proof.Output != nil {
		t.Fatalf("Output = %+v, want nil in expected error mode", proof.Output)
	}
}

func TestWriteProofRejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(target, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "proof-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	cfg := smokeConfig{
		ImageRef:    smokePinnedImage,
		Timeout:     time.Second,
		OutputMax:   64,
		ProofPath:   link,
		ProofSource: defaultProofSource,
	}
	err := writeProof(cfg, runOutcome{Mode: "expected_error", MatchedErrorPattern: "context deadline exceeded"})
	if err == nil {
		t.Fatal("writeProof err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("writeProof err = %v, want symlink rejection", err)
	}
}

func TestWriteProofRejectsSymlinkParent(t *testing.T) {
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "proof-link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	cfg := smokeConfig{
		ImageRef:    smokePinnedImage,
		Timeout:     time.Second,
		OutputMax:   64,
		ProofPath:   filepath.Join(linkDir, "container-provider-smoke.json"),
		ProofSource: defaultProofSource,
	}
	err := writeProof(cfg, runOutcome{Mode: "expected_error", MatchedErrorPattern: "context deadline exceeded"})
	if err == nil {
		t.Fatal("writeProof err = nil, want symlink parent rejection")
	}
	if !strings.Contains(err.Error(), "parent directory") || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("writeProof err = %v, want symlink parent rejection", err)
	}
}

func TestErrorContainsAny(t *testing.T) {
	err := errors.New("docker container exited with code 153")
	if !errorContainsAny(err, "context deadline exceeded || exited with code") {
		t.Fatal("errorContainsAny = false, want true")
	}
	if errorContainsAny(err, "context deadline exceeded || exceeds maximum") {
		t.Fatal("errorContainsAny = true, want false")
	}
}

func TestValidateSmokeOutput(t *testing.T) {
	tests := []struct {
		name    string
		raw     json.RawMessage
		wantErr string
	}{
		{name: "object", raw: json.RawMessage(`{"provider_smoke":"ok"}`)},
		{name: "empty", raw: nil, wantErr: "empty"},
		{name: "array", raw: json.RawMessage(`[]`), wantErr: "object"},
		{name: "trailing", raw: json.RawMessage(`{} {}`), wantErr: "trailing"},
		{name: "duplicate key", raw: json.RawMessage(`{"provider_smoke":"stale","provider_smoke":"ok"}`), wantErr: "duplicate object key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSmokeOutput(tt.raw)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSmokeOutput: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validateSmokeOutput err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateSmokeOutput err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
