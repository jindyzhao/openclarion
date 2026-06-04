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

func TestOpenAPILintInvokesVacuumWithRegularInputs(t *testing.T) {
	root := newOpenAPILintRepo(t)

	out, err := runOpenAPILintCheck(t, root)
	if err != nil {
		t.Fatalf("openapi lint check failed: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(root, "vacuum-args.txt")) // #nosec G304 -- test reads a controlled temp fixture.
	if err != nil {
		t.Fatalf("read vacuum args: %v", err)
	}
	want := "tool\ngithub.com/daveshanley/vacuum\nlint\n-r\ndocs/design/ci/vacuum/.vacuum.yaml\n--details\n--fail-severity\nerror\napi/openapi.yaml\n"
	if got := string(raw); got != want {
		t.Fatalf("vacuum args = %q, want %q", got, want)
	}
}

func TestOpenAPILintRejectsNonRegularInputs(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		setup func(t *testing.T, root, path string)
		want  string
	}{
		{
			name: "ruleset symlink",
			path: "docs/design/ci/vacuum/.vacuum.yaml",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithSymlink(t, root, path)
			},
			want: "docs/design/ci/vacuum/.vacuum.yaml must be a regular file, not a symlink",
		},
		{
			name: "spec symlink",
			path: "api/openapi.yaml",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithSymlink(t, root, path)
			},
			want: "api/openapi.yaml must be a regular file, not a symlink",
		},
		{
			name: "ruleset directory",
			path: "docs/design/ci/vacuum/.vacuum.yaml",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithDirectory(t, root, path)
			},
			want: "docs/design/ci/vacuum/.vacuum.yaml must be a regular file",
		},
		{
			name: "spec directory",
			path: "api/openapi.yaml",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithDirectory(t, root, path)
			},
			want: "api/openapi.yaml must be a regular file",
		},
		{
			name: "ruleset parent symlink",
			path: "docs/design/ci/vacuum",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithSymlink(t, root, path)
			},
			want: "docs/design/ci/vacuum/.vacuum.yaml parent directory docs/design/ci/vacuum must not be a symlink",
		},
		{
			name: "spec parent symlink",
			path: "api",
			setup: func(t *testing.T, root, path string) {
				openAPILintReplaceWithSymlink(t, root, path)
			},
			want: "api/openapi.yaml parent directory api must not be a symlink",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPILintRepo(t)
			tc.setup(t, root, tc.path)

			out, err := runOpenAPILintCheck(t, root)
			if err == nil {
				t.Fatalf("openapi lint check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("openapi lint output = %q, want %q", out, tc.want)
			}
		})
	}
}

func newOpenAPILintRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	openAPILintWriteFile(t, root, "scripts/check_openapi_lint.sh", openAPILintScript(t), 0o750)
	openAPILintWriteFile(t, root, "docs/design/ci/vacuum/.vacuum.yaml", "rules:\n  spec: true\n", 0o644)
	openAPILintWriteFile(t, root, "api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Fixture\n  version: 0.0.0\npaths: {}\n", 0o644)
	openAPILintWriteFile(t, root, "bin/go", fakeOpenAPILintGo(), 0o750)
	openAPILintInitGit(t, root)
	return root
}

func openAPILintScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_openapi_lint.sh")
	if err != nil {
		t.Fatalf("read openapi lint script: %v", err)
	}
	return string(raw)
}

func fakeOpenAPILintGo() string {
	return `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" > vacuum-args.txt
`
}

func openAPILintWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func openAPILintReplaceWithSymlink(t *testing.T, root, name string) {
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

func openAPILintReplaceWithDirectory(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
}

func openAPILintInitGit(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
}

func runOpenAPILintCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_openapi_lint.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
