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

func TestGoLicensesCheckUsesDependencyPolicyAllowlist(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, BSD-3-Clause, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	callsPath := filepath.Join(root, "calls.txt")

	out, err := runGoLicensesCheck(t, root, callsPath, "")
	if err != nil {
		t.Fatalf("go licenses check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[go-licenses] OK (allowed: Apache-2.0,BSD-3-Clause,MIT)") {
		t.Fatalf("output = %q, want normalized allowlist", out)
	}

	callsRaw, err := os.ReadFile(callsPath) // #nosec G304 -- test reads a temp file it created.
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	calls := string(callsRaw)
	for _, want := range []string{
		"run github.com/google/go-licenses@v1.6.0 check --include_tests --ignore=github.com/openclarion/openclarion --allowed_licenses=Apache-2.0,BSD-3-Clause,MIT ./cmd/openclarion ./api/... ./internal/... ./scripts/...",
		"run github.com/google/go-licenses@v1.6.0 check --include_tests --ignore=github.com/openclarion/openclarion/tools/openclarion-linter --allowed_licenses=Apache-2.0,BSD-3-Clause,MIT ./...",
	} {
		if !strings.Contains(calls, want) {
			t.Fatalf("calls = %q, want %q", calls, want)
		}
	}
}

func TestGoLicensesCheckRejectsMissingAllowlist(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", "# Dependency Policy\n", 0o644)

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "must contain 'go-license-allow: <SPDX>[,<SPDX>...]; owner: <owner>; reviewed: YYYY-MM-DD; reason: <reason>'") {
		t.Fatalf("output = %q, want missing allowlist guidance", out)
	}
}

func TestGoLicensesCheckRejectsMissingPolicyMetadata(t *testing.T) {
	tests := []struct {
		name   string
		policy string
		want   string
	}{
		{
			name:   "missing owner",
			policy: "go-license-allow: Apache-2.0, MIT; reviewed: 2026-05-29; reason: accepted licenses\n",
			want:   "owner: <owner>",
		},
		{
			name:   "missing reviewed",
			policy: "go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reason: accepted licenses\n",
			want:   "reviewed: YYYY-MM-DD",
		},
		{
			name:   "missing reason",
			policy: "go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29\n",
			want:   "reason: <reason>",
		},
		{
			name:   "empty owner",
			policy: "go-license-allow: Apache-2.0, MIT; owner: ; reviewed: 2026-05-29; reason: accepted licenses\n",
			want:   "owner: <owner>",
		},
		{
			name:   "empty reason",
			policy: "go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: \n",
			want:   "reason: <reason>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newGoLicensesFixture(t)
			goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", "# Dependency Policy\n\n"+tt.policy, 0o644)

			out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
			if err == nil {
				t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestGoLicensesCheckRejectsInvalidPolicyReviewedDate(t *testing.T) {
	tests := []struct {
		name     string
		reviewed string
		want     string
	}{
		{name: "invalid calendar date", reviewed: "2026-02-31", want: "is invalid"},
		{name: "future date", reviewed: "2026-05-31", want: "is in the future"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newGoLicensesFixture(t)
			goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: `+tt.reviewed+`; reason: accepted Go dependency licenses
`, 0o644)

			out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
			if err == nil {
				t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestGoLicensesCheckPropagatesToolFailure(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "1")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "simulated go-licenses failure") {
		t.Fatalf("output = %q, want fake tool failure", out)
	}
}

func newGoLicensesFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	goLicensesWriteFile(t, root, "scripts/check_go_licenses.sh", goLicensesScript(t), 0o750)
	goLicensesWriteFile(t, root, "go.mod", "module github.com/openclarion/openclarion\n", 0o644)
	goLicensesWriteFile(t, root, "tools/openclarion-linter/go.mod", "module github.com/openclarion/openclarion/tools/openclarion-linter\n", 0o644)
	goLicensesWriteFile(t, root, "cmd/openclarion/.keep", "", 0o644)
	goLicensesWriteFile(t, root, "api/.keep", "", 0o644)
	goLicensesWriteFile(t, root, "internal/.keep", "", 0o644)
	goLicensesWriteFile(t, root, "scripts/.keep", "", 0o644)
	goLicensesWriteFile(t, root, "bin/go", fakeGoLicensesGo(), 0o750)
	return root
}

func goLicensesScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_go_licenses.sh")
	if err != nil {
		t.Fatalf("read go licenses script: %v", err)
	}
	return string(raw)
}

func fakeGoLicensesGo() string {
	return `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${GO_LICENSES_CALLS:?}"
if [[ "${GO_LICENSES_FAKE_FAIL:-}" == "1" ]]; then
  echo "simulated go-licenses failure" >&2
  exit 42
fi
`
}

func goLicensesWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGoLicensesCheck(t *testing.T, root, callsPath, fakeFail string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_go_licenses.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GO_LICENSES_CALLS="+callsPath,
		"GO_LICENSES_FAKE_FAIL="+fakeFail,
		"GO_LICENSES_REVIEW_TODAY=2026-05-30",
	)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
