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

func TestOpenAPILintRejectsSymlinkInputs(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "ruleset",
			path: "docs/design/ci/vacuum/.vacuum.yaml",
			want: "docs/design/ci/vacuum/.vacuum.yaml must be a regular file, not a symlink",
		},
		{
			name: "spec",
			path: "api/openapi.yaml",
			want: "api/openapi.yaml must be a regular file, not a symlink",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPILintRepo(t)
			input := filepath.Join(root, filepath.FromSlash(tc.path))
			target := filepath.Join(filepath.Dir(input), "target.yaml")
			if err := os.Rename(input, target); err != nil {
				t.Fatalf("rename %s: %v", tc.path, err)
			}
			if err := os.Symlink(target, input); err != nil {
				t.Skipf("symlink unsupported: %v", err)
			}

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

func TestOpenAPILintRejectsMissingInputs(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "ruleset",
			path: "docs/design/ci/vacuum/.vacuum.yaml",
			want: "missing docs/design/ci/vacuum/.vacuum.yaml",
		},
		{
			name: "spec",
			path: "api/openapi.yaml",
			want: "missing api/openapi.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPILintRepo(t)
			input := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.Remove(input); err != nil {
				t.Fatalf("remove %s: %v", tc.path, err)
			}

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

func TestOpenAPILintRejectsNonRegularInputs(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "ruleset",
			path: "docs/design/ci/vacuum/.vacuum.yaml",
			want: "docs/design/ci/vacuum/.vacuum.yaml must be a regular file",
		},
		{
			name: "spec",
			path: "api/openapi.yaml",
			want: "api/openapi.yaml must be a regular file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newOpenAPILintRepo(t)
			input := filepath.Join(root, filepath.FromSlash(tc.path))
			if err := os.Remove(input); err != nil {
				t.Fatalf("remove %s: %v", tc.path, err)
			}
			if err := os.Mkdir(input, 0o750); err != nil {
				t.Fatalf("mkdir %s: %v", tc.path, err)
			}

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
	openAPILintWriteFile(t, root, "docs/design/ci/vacuum/.vacuum.yaml", "rules: {}\n", 0o644)
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
printf '%s\n' "$@" > "$PWD/vacuum-args.txt"
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
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
