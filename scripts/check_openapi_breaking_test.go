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

func TestOpenAPIBreakingSoftFailsBeforeSunset(t *testing.T) {
	root := newOpenAPIBreakingFixture(t)

	out, err := runOpenAPIBreakingCheck(t, root, map[string]string{
		"OPENAPI_BREAKING_TODAY":       "2026-06-09",
		"OPENAPI_BREAKING_FAKE_STATUS": "1",
	})
	if err != nil {
		t.Fatalf("openapi breaking check failed before sunset: %v\n%s", err, out)
	}
	if !strings.Contains(out, "WARNING: breaking-change gate is soft-fail until 2026-06-10") {
		t.Fatalf("output = %q, want soft-fail warning", out)
	}
	if !strings.Contains(out, "breaking change: removed GET /reports") {
		t.Fatalf("output = %q, want fake breaking output", out)
	}
}

func TestOpenAPIBreakingHardFailsOnAndAfterSunset(t *testing.T) {
	tests := []struct {
		name  string
		today string
	}{
		{name: "on sunset", today: "2026-06-10"},
		{name: "after sunset", today: "2026-06-11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newOpenAPIBreakingFixture(t)

			out, err := runOpenAPIBreakingCheck(t, root, map[string]string{
				"OPENAPI_BREAKING_TODAY":       tt.today,
				"OPENAPI_BREAKING_FAKE_STATUS": "1",
			})
			if err == nil {
				t.Fatalf("openapi breaking check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, "FAIL: breaking OpenAPI changes detected after soft-fail sunset 2026-06-10") {
				t.Fatalf("output = %q, want hard-fail message", out)
			}
		})
	}
}

func TestOpenAPIBreakingAcceptsNonBreakingDiff(t *testing.T) {
	root := newOpenAPIBreakingFixture(t)

	out, err := runOpenAPIBreakingCheck(t, root, map[string]string{
		"OPENAPI_BREAKING_TODAY":       "2026-06-11",
		"OPENAPI_BREAKING_FAKE_STATUS": "0",
	})
	if err != nil {
		t.Fatalf("openapi breaking check failed for non-breaking diff: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[openapi-breaking] OK") {
		t.Fatalf("output = %q, want OK", out)
	}
}

func TestOpenAPIBreakingRejectsInvalidDatesBeforeToolRun(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "bad sunset format",
			env:  map[string]string{"OPENAPI_BREAKING_SOFT_FAIL_UNTIL": "2026/06/10"},
			want: "SOFT_FAIL_UNTIL must be YYYY-MM-DD",
		},
		{
			name: "invalid sunset date",
			env:  map[string]string{"OPENAPI_BREAKING_SOFT_FAIL_UNTIL": "2026-02-31"},
			want: "SOFT_FAIL_UNTIL is not a valid date",
		},
		{
			name: "invalid today date",
			env:  map[string]string{"OPENAPI_BREAKING_TODAY": "2026-02-31"},
			want: "today is not a valid YYYY-MM-DD date",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newOpenAPIBreakingFixture(t)

			out, err := runOpenAPIBreakingCheck(t, root, tt.env)
			if err == nil {
				t.Fatalf("openapi breaking check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
			callsPath := filepath.Join(root, "calls.txt")
			if info, statErr := os.Stat(callsPath); statErr == nil && info.Size() > 0 {
				t.Fatalf("fake go was called before date validation completed")
			}
		})
	}
}

func newOpenAPIBreakingFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	openAPIBreakingWriteFile(t, root, "scripts/check_openapi_breaking.sh", openAPIBreakingScript(t), 0o750)
	openAPIBreakingWriteFile(t, root, "api/openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Current\n  version: 1.0.0\npaths: {}\n", 0o644)
	openAPIBreakingWriteFile(t, root, "base-openapi.yaml", "openapi: 3.1.0\ninfo:\n  title: Base\n  version: 1.0.0\npaths: {}\n", 0o644)
	openAPIBreakingWriteFile(t, root, "bin/go", fakeOpenAPIBreakingGo(), 0o750)
	return root
}

func openAPIBreakingScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_openapi_breaking.sh")
	if err != nil {
		t.Fatalf("read openapi breaking script: %v", err)
	}
	return string(raw)
}

func fakeOpenAPIBreakingGo() string {
	return `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${OPENAPI_BREAKING_CALLS:?}"
status="${OPENAPI_BREAKING_FAKE_STATUS:-0}"
if [[ "$status" == "0" ]]; then
  echo "no breaking changes"
  exit 0
fi
echo "breaking change: removed GET /reports"
exit "$status"
`
}

func openAPIBreakingWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runOpenAPIBreakingCheck(t *testing.T, root string, env map[string]string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_openapi_breaking.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENAPI_BASE_SPEC="+filepath.Join(root, "base-openapi.yaml"),
		"OPENAPI_BREAKING_CALLS="+filepath.Join(root, "calls.txt"),
	)
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
