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

func TestOpenAPITSFreshRejectsNonRegularInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string) map[string]string
		want  string
	}{
		{
			name: "web directory symlink",
			setup: func(t *testing.T, root string) map[string]string {
				openAPITSReplaceWithSymlink(t, root, "web")
				return nil
			},
			want: "web/ must be a real directory",
		},
		{
			name: "openapi spec symlink",
			setup: func(t *testing.T, root string) map[string]string {
				openAPITSReplaceWithSymlink(t, root, "api/openapi.yaml")
				return nil
			},
			want: "OpenAPI spec must be a regular file, not a symlink",
		},
		{
			name: "package json symlink",
			setup: func(t *testing.T, root string) map[string]string {
				openAPITSReplaceWithSymlink(t, root, "web/package.json")
				return nil
			},
			want: "web/package.json must be a regular file, not a symlink",
		},
		{
			name: "generated file symlink",
			setup: func(t *testing.T, root string) map[string]string {
				openAPITSReplaceWithSymlink(t, root, "web/src/lib/api/openapi.ts")
				return nil
			},
			want: "generated TypeScript file must be a regular file, not a symlink",
		},
		{
			name: "generated output becomes symlink",
			setup: func(_ *testing.T, _ string) map[string]string {
				return map[string]string{"OPENAPI_TS_FAKE_GENERATED_SYMLINK": "1"}
			},
			want: "generated TypeScript file must be a regular file, not a symlink",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPITSFixture(t)
			env := tc.setup(t, root)

			out, err := runOpenAPITSFreshCheck(t, root, env)
			if err == nil {
				t.Fatalf("openapi ts fresh check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("openapi ts fresh output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestOpenAPITSFreshAcceptsStableGeneration(t *testing.T) {
	root := newOpenAPITSFixture(t)

	out, err := runOpenAPITSFreshCheck(t, root, nil)
	if err != nil {
		t.Fatalf("openapi ts fresh check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "generated TypeScript types are up-to-date") {
		t.Fatalf("openapi ts fresh output = %q, want OK", out)
	}
}

func newOpenAPITSFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	openAPITSWriteFile(t, root, "scripts/check_openapi_ts_fresh.sh", openAPITSScript(t), 0o750)
	openAPITSWriteFile(t, root, "api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Fixture\n  version: 0.0.0\npaths: {}\n", 0o644)
	openAPITSWriteFile(t, root, "web/package.json", `{"scripts":{"api:generate":"true"}}`+"\n", 0o644)
	openAPITSWriteFile(t, root, "web/src/lib/api/openapi.ts", "export type Fixture = string;\n", 0o644)
	openAPITSWriteFile(t, root, "bin/npm", fakeOpenAPITSNPM(), 0o750)
	return root
}

func openAPITSScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_openapi_ts_fresh.sh")
	if err != nil {
		t.Fatalf("read openapi ts fresh script: %v", err)
	}
	return string(raw)
}

func fakeOpenAPITSNPM() string {
	return `#!/usr/bin/env bash
set -euo pipefail
if [[ "${OPENAPI_TS_FAKE_GENERATED_SYMLINK:-}" == "1" ]]; then
  rm -f src/lib/api/openapi.ts
  printf 'export type Fixture = string;\n' > src/lib/api/openapi.target.ts
  ln -s openapi.target.ts src/lib/api/openapi.ts
fi
`
}

func openAPITSWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func openAPITSReplaceWithSymlink(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	target := path + ".target"
	if err := os.Rename(path, target); err != nil {
		t.Fatalf("rename %s: %v", name, err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runOpenAPITSFreshCheck(t *testing.T, root string, env map[string]string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_openapi_ts_fresh.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
