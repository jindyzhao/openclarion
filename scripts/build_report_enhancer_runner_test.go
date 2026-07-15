package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportEnhancerRunnerBuildRejectsInvalidEarlyConfiguration(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "architecture",
			env:  map[string]string{"OPENCLARION_REPORT_ENHANCER_RUNNER_TARGET_ARCH": "386"},
			want: "target architecture must be amd64 or arm64",
		},
		{
			name: "registry timeout",
			env:  map[string]string{"OPENCLARION_REPORT_ENHANCER_RUNNER_REGISTRY_READY_TIMEOUT_SECONDS": "121"},
			want: "registry readiness timeout must be an integer from 1 to 120 seconds",
		},
		{
			name: "mutable registry image",
			env: map[string]string{
				"OPENCLARION_REPORT_ENHANCER_RUNNER_DIGEST_REF_OUT": filepath.Join(t.TempDir(), "digest-ref"),
				"OPENCLARION_REPORT_ENHANCER_REGISTRY_IMAGE":        "registry:latest",
			},
			want: "registry image must be an immutable lowercase digest reference",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runReportEnhancerBuildValidation(t, tt.env)
			if err == nil || !strings.Contains(output, tt.want) {
				t.Fatalf("build validation err=%v output=%q, want %q", err, output, tt.want)
			}
		})
	}
}

func TestReportEnhancerRunnerBuildRejectsSymlinkedOutputParent(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	output, err := runReportEnhancerBuildValidation(t, map[string]string{
		"OPENCLARION_REPORT_ENHANCER_RUNNER_DIGEST_REF_OUT": filepath.Join(linkDir, "digest-ref"),
	})
	if err == nil || !strings.Contains(output, "digest ref output parent must not be a symlink") {
		t.Fatalf("build validation err=%v output=%q", err, output)
	}
}

func runReportEnhancerBuildValidation(t *testing.T, values map[string]string) (string, error) {
	t.Helper()
	binDir := t.TempDir()
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 97\n"), 0o700); err != nil { // #nosec G306 -- executable test shim.
		t.Fatalf("write fake docker: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "build_report_enhancer_runner.sh")
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	for key, value := range values {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
