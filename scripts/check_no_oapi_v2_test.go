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

func TestForbiddenOAPIV2RejectsCompatBridgePath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, root string) {
				forbiddenOAPIV2WriteFile(t, root, "api/openapi.compat.yaml", "openapi: 3.0.3\n", 0o644)
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "api", "openapi.compat.yaml"), 0o750); err != nil {
					t.Fatalf("mkdir compat path: %v", err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, root string) {
				forbiddenOAPIV2WriteFile(t, root, "api/compat-target.yaml", "openapi: 3.0.3\n", 0o644)
				if err := os.Symlink("compat-target.yaml", filepath.Join(root, "api", "openapi.compat.yaml")); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "api"), 0o750); err != nil {
					t.Fatalf("mkdir api dir: %v", err)
				}
				if err := os.Symlink("missing.yaml", filepath.Join(root, "api", "openapi.compat.yaml")); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := forbiddenOAPIV2Fixture(t)
			tc.setup(t, root)

			out, err := runForbiddenOAPIV2(t, root)
			if err == nil {
				t.Fatalf("forbidden-oapi-v2 passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, "api/openapi.compat.yaml exists") {
				t.Fatalf("forbidden-oapi-v2 output = %q, want compat bridge rejection", out)
			}
		})
	}
}

func forbiddenOAPIV2Fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	forbiddenOAPIV2WriteFile(t, root, "scripts/check_no_oapi_v2.sh", forbiddenOAPIV2Script(t), 0o750)
	forbiddenOAPIV2WriteFile(t, root, "go.mod", "module example.com/openclarion-fixture\n\ngo 1.25.0\n", 0o644)
	return root
}

func forbiddenOAPIV2Script(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_no_oapi_v2.sh")
	if err != nil {
		t.Fatalf("read forbidden-oapi-v2 script: %v", err)
	}
	return string(raw)
}

func forbiddenOAPIV2WriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runForbiddenOAPIV2(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_no_oapi_v2.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
