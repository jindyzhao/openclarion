package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const egressPinnedImage = "registry.example.com/openclarion/egress-smoke@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestProxyAllowsConfiguredHostAndDeniesOthers(t *testing.T) {
	allowedUpstream := httptest.NewServer(upstreamHandler("allowed"))
	defer allowedUpstream.Close()
	deniedUpstream := httptest.NewServer(upstreamHandler("denied"))
	defer deniedUpstream.Close()

	allowedURL, err := url.Parse(allowedUpstream.URL)
	if err != nil {
		t.Fatalf("parse allowed upstream URL: %v", err)
	}
	proxy := httptest.NewServer(proxyHandler(map[string]bool{allowedURL.Host: true}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   2 * time.Second,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, allowedUpstream.URL, nil)
	if err != nil {
		t.Fatalf("build allowed request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET allowed via proxy: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("allowed status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	_ = resp.Body.Close()

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, deniedUpstream.URL, nil)
	if err != nil {
		t.Fatalf("build denied request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET denied via proxy: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	_ = resp.Body.Close()
}

func TestRunClientAcceptsExpectedFailure(t *testing.T) {
	var stdout bytes.Buffer
	err := runClient([]string{
		"--url", "http://127.0.0.1:1",
		"--want-fail",
		"--timeout", "50ms",
	}, &stdout)
	if err != nil {
		t.Fatalf("runClient: %v", err)
	}
	if !strings.Contains(stdout.String(), "expected failure") {
		t.Fatalf("stdout = %q, want expected failure", stdout.String())
	}
}

func TestRunProofWritesRetainedArtifact(t *testing.T) {
	proofPath := filepath.Join(t.TempDir(), "proof", "egress-allowdeny-smoke.json")
	var stdout bytes.Buffer
	err := runProof([]string{
		"--proof-path", proofPath,
		"--image-ref", egressPinnedImage,
		"--source", "make egress-allowdeny-smoke",
		"--run-id", "egress-smoke-test",
		"--timeout-seconds", "9",
		"--allowed-target", "allowed.internal:8080",
		"--denied-target", "denied.internal:8080",
		"--proxy-target", "egress-proxy:18080",
	}, &stdout)
	if err != nil {
		t.Fatalf("runProof: %v", err)
	}
	if !strings.Contains(stdout.String(), "proof written") {
		t.Fatalf("stdout = %q, want proof written", stdout.String())
	}

	// #nosec G304 -- proofPath is created inside this test's temporary directory.
	raw, err := os.ReadFile(proofPath)
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var proof egressProofArtifact
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatalf("unmarshal proof: %v\n%s", err, raw)
	}
	if proof.Tool != "egress-allowdeny-smoke" {
		t.Fatalf("Tool = %q", proof.Tool)
	}
	if proof.Status != "pass" {
		t.Fatalf("Status = %q", proof.Status)
	}
	if proof.Source != "make egress-allowdeny-smoke" {
		t.Fatalf("Source = %q", proof.Source)
	}
	if proof.ImageRef != egressPinnedImage {
		t.Fatalf("ImageRef = %q", proof.ImageRef)
	}
	if proof.RunID != "egress-smoke-test" {
		t.Fatalf("RunID = %q", proof.RunID)
	}
	if proof.TimeoutSec != 9 {
		t.Fatalf("TimeoutSec = %d, want 9", proof.TimeoutSec)
	}
	if proof.Topology.SandboxNetwork != "internal" {
		t.Fatalf("SandboxNetwork = %q", proof.Topology.SandboxNetwork)
	}
	if proof.Topology.AllowedTarget != "allowed.internal:8080" {
		t.Fatalf("AllowedTarget = %q", proof.Topology.AllowedTarget)
	}
	if len(proof.Checks) != 8 {
		t.Fatalf("Checks = %d, want 8", len(proof.Checks))
	}
	if strings.Contains(string(raw), filepath.Dir(proofPath)) {
		t.Fatalf("proof contains host temp path:\n%s", raw)
	}
}

func TestRunProofRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing path",
			args:    []string{"--image-ref", egressPinnedImage},
			wantErr: "--proof-path is required",
		},
		{
			name:    "tagged image",
			args:    []string{"--proof-path", filepath.Join(t.TempDir(), "proof.json"), "--image-ref", "busybox:1.36.1"},
			wantErr: "pinned by sha256 digest",
		},
		{
			name:    "non canonical source",
			args:    []string{"--proof-path", filepath.Join(t.TempDir(), "proof.json"), "--image-ref", egressPinnedImage, "--source", "manual note"},
			wantErr: "--source",
		},
		{
			name:    "invalid timeout",
			args:    []string{"--proof-path", filepath.Join(t.TempDir(), "proof.json"), "--image-ref", egressPinnedImage, "--timeout-seconds", "0"},
			wantErr: "positive integer",
		},
		{
			name:    "missing target",
			args:    []string{"--proof-path", filepath.Join(t.TempDir(), "proof.json"), "--image-ref", egressPinnedImage, "--allowed-target", ""},
			wantErr: "targets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := runProof(tt.args, &stdout)
			if err == nil {
				t.Fatal("runProof err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("runProof err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunProofRejectsSymlinkProof(t *testing.T) {
	target := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(target, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "proof-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	var stdout bytes.Buffer
	err := runProof([]string{"--proof-path", link, "--image-ref", egressPinnedImage}, &stdout)
	if err == nil {
		t.Fatal("runProof err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("runProof err = %v, want symlink rejection", err)
	}
}

func TestRunProofRejectsSymlinkProofParent(t *testing.T) {
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "proof-link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	var stdout bytes.Buffer
	err := runProof([]string{
		"--proof-path", filepath.Join(linkDir, "egress-allowdeny-smoke.json"),
		"--image-ref", egressPinnedImage,
	}, &stdout)
	if err == nil {
		t.Fatal("runProof err = nil, want symlink parent rejection")
	}
	if !strings.Contains(err.Error(), "parent directory") || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("runProof err = %v, want symlink parent rejection", err)
	}
}

func TestOpenProofFileNoFollowRejectsFinalSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(target, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "proof-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	f, err := openProofFileNoFollow(link)
	if err == nil {
		_ = f.Close()
		t.Fatal("openProofFileNoFollow err = nil, want symlink rejection")
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run err = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run err = %v, want unknown command", err)
	}
}
