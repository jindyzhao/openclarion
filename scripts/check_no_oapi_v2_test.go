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

func TestForbiddenOAPIV2AcceptsNativeV3OnlyRepo(t *testing.T) {
	root := newForbiddenOAPIV2Repo(t, map[string]string{
		"go.mod":           "module example.com/openclarion-fixture\n\ngo 1.25.0\n",
		"api/openapi.yaml": "openapi: 3.1.0\ninfo:\n  title: Fixture\n  version: 0.0.0\npaths: {}\n",
	})

	out, err := runForbiddenOAPIV2(t, root)
	if err != nil {
		t.Fatalf("forbidden-oapi-v2 failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[forbidden-oapi-v2] OK") {
		t.Fatalf("forbidden-oapi-v2 output = %q, want OK", out)
	}
}

func TestForbiddenOAPIV2RejectsCompatBridgePath(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
	}{
		{
			name: "regular file",
			setup: func(t *testing.T, root string) {
				writeForbiddenOAPIV2File(t, root, "api/openapi.compat.yaml", "openapi: 3.0.3\n", 0o644)
			},
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				path := filepath.Join(root, "api", "openapi.compat.yaml")
				if err := os.MkdirAll(path, 0o750); err != nil {
					t.Fatalf("mkdir compat path: %v", err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, root string) {
				writeForbiddenOAPIV2File(t, root, "api/compat-target.yaml", "openapi: 3.0.3\n", 0o644)
				target := filepath.Join(root, "api", "compat-target.yaml")
				link := filepath.Join(root, "api", "openapi.compat.yaml")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
		},
		{
			name: "dangling symlink",
			setup: func(t *testing.T, root string) {
				apiDir := filepath.Join(root, "api")
				if err := os.MkdirAll(apiDir, 0o750); err != nil {
					t.Fatalf("mkdir api dir: %v", err)
				}
				target := filepath.Join(apiDir, "missing-compat.yaml")
				link := filepath.Join(apiDir, "openapi.compat.yaml")
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newForbiddenOAPIV2Repo(t, nil)
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

func TestForbiddenOAPIV2RejectsModuleAndSourceV2References(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "go mod v2 module",
			files: map[string]string{
				"go.mod": "module example.com/openclarion-fixture\n\ngo 1.25.0\n\nrequire github.com/oapi-codegen/" + "oapi-codegen/v2 v2.5.0\n",
			},
			want: "go.mod references oapi-codegen v2",
		},
		{
			name: "go source v2 import",
			files: map[string]string{
				"go.mod":                 "module example.com/openclarion-fixture\n\ngo 1.25.0\n",
				"internal/bad/import.go": "package bad\n\nimport _ \"github.com/oapi-codegen/" + "oapi-codegen/v2/pkg/codegen\"\n",
			},
			want: "forbidden v2 imports detected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newForbiddenOAPIV2Repo(t, tc.files)

			out, err := runForbiddenOAPIV2(t, root)
			if err == nil {
				t.Fatalf("forbidden-oapi-v2 passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("forbidden-oapi-v2 output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func newForbiddenOAPIV2Repo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeForbiddenOAPIV2File(t, root, "scripts/check_no_oapi_v2.sh", forbiddenOAPIV2Script(t), 0o750)
	for path, body := range files {
		writeForbiddenOAPIV2File(t, root, path, body, 0o644)
	}
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

func writeForbiddenOAPIV2File(t *testing.T, root, name, body string, mode os.FileMode) {
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
