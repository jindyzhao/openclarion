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

func TestBuildLocalEgressProxyRejectsRegistryPortWithoutTag(t *testing.T) {
	out, err := runLocalEgressProxyBuild(t, "localhost:5000/openclarion/proxy")
	if err == nil {
		t.Fatalf("build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must include an explicit non-latest tag") {
		t.Fatalf("build output = %q, want missing-tag rejection", out)
	}
}

func TestBuildLocalEgressProxyRejectsLatestTag(t *testing.T) {
	out, err := runLocalEgressProxyBuild(t, "openclarion/local-egress-proxy:latest")
	if err == nil {
		t.Fatalf("build passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must not be latest") {
		t.Fatalf("build output = %q, want latest rejection", out)
	}
}

func runLocalEgressProxyBuild(t *testing.T, image string) (string, error) {
	t.Helper()
	binDir := t.TempDir()
	for _, tool := range []string{"docker", "go"} {
		path := filepath.Join(binDir, tool)
		if err := os.WriteFile(path, []byte("#!/usr/bin/env sh\nexit 99\n"), 0o600); err != nil {
			t.Fatalf("write fake %s: %v", tool, err)
		}
		// #nosec G302 -- the owner-only test fixture must be executable as a fake tool.
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatalf("chmod fake %s: %v", tool, err)
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/build_local_egress_proxy.sh") // #nosec G204 -- controlled repository script.
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCLARION_LOCAL_EGRESS_PROXY_IMAGE="+image,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
