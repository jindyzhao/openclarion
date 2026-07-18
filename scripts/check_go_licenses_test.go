package main

import (
	"context"
	"crypto/sha256"
	"fmt"
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
	calls := strings.Split(strings.TrimSpace(string(callsRaw)), "\n")
	wants := []string{
		root + "|run github.com/google/go-licenses@v1.6.0 check --include_tests --ignore=github.com/openclarion/openclarion --allowed_licenses=Apache-2.0,BSD-3-Clause,MIT ./cmd/openclarion ./api/... ./internal/... ./scripts/...",
		filepath.Join(root, "tools/openclarion-linter") + "|run github.com/google/go-licenses@v1.6.0 check --include_tests --ignore=github.com/openclarion/openclarion/tools/openclarion-linter --allowed_licenses=Apache-2.0,BSD-3-Clause,MIT ./...",
		filepath.Join(root, "scripts/diagnosis_assistant_runner") + "|run github.com/google/go-licenses@v1.6.0 check --include_tests --ignore=github.com/openclarion/openclarion --allowed_licenses=Apache-2.0,BSD-3-Clause,MIT ./...",
	}
	if len(calls) != len(wants) {
		t.Fatalf("calls = %q, want %d calls", calls, len(wants))
	}
	for index, want := range wants {
		if calls[index] != want {
			t.Fatalf("calls[%d] = %q, want %q", index, calls[index], want)
		}
	}
}

func TestGoLicensesCheckClassifiesAuditedAssemblyAsInfo(t *testing.T) {
	root := newGoLicensesFixture(t)
	assembly := []byte("#include \"textflag.h\"\n")
	relativePath := "example.com/asm@v1.0.0/asm_amd64.s"
	goLicensesWriteFile(t, root, "module-cache/"+relativePath, string(assembly), 0o644)
	digest := sha256.Sum256(assembly)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", fmt.Sprintf(`# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
go-license-non-go-allow: example.com/asm|%s|%x; owner: CI maintainers; reviewed: 2026-05-29; reason: audited Go assembly has no external dependency declarations
`, relativePath, digest), 0o644)
	toolStderr := fmt.Sprintf("go: downloading example.com/tool v1.2.3\nW0718 17:40:51.432546 123 library.go:101] \"example.com/asm\" contains non-Go code that can't be inspected for further dependencies:\n%s\n", filepath.Join(root, "module-cache", relativePath))

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "", "GO_LICENSES_FAKE_STDERR="+toolStderr)
	if err != nil {
		t.Fatalf("go licenses check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[go-licenses] INFO (1 hash-pinned assembly files audited)") {
		t.Fatalf("output = %q, want audited assembly info", out)
	}
	if !strings.Contains(out, "[go-licenses] INFO (1 Go module downloads completed)") {
		t.Fatalf("output = %q, want Go module download info", out)
	}
	if strings.Contains(out, "W0718") || strings.Contains(strings.ToLower(out), "warning") {
		t.Fatalf("output retained warning-level tool noise: %q", out)
	}
}

func TestGoLicensesCheckRejectsUnreviewedNonGoFile(t *testing.T) {
	root := newGoLicensesFixture(t)
	relativePath := "example.com/asm@v1.0.0/asm_amd64.s"
	goLicensesWriteFile(t, root, "module-cache/"+relativePath, "", 0o644)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	toolStderr := fmt.Sprintf("W0718 17:40:51.432546 123 library.go:101] \"example.com/asm\" contains non-Go code that can't be inspected for further dependencies:\n%s\n", filepath.Join(root, "module-cache", relativePath))

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "", "GO_LICENSES_FAKE_STDERR="+toolStderr)
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "unreviewed non-Go dependency file") {
		t.Fatalf("output = %q, want unreviewed non-Go file failure", out)
	}
}

func TestGoLicensesCheckRejectsChangedAuditedAssembly(t *testing.T) {
	root := newGoLicensesFixture(t)
	relativePath := "example.com/asm@v1.0.0/asm_amd64.s"
	goLicensesWriteFile(t, root, "module-cache/"+relativePath, "changed\n", 0o644)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", fmt.Sprintf(`# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
go-license-non-go-allow: example.com/asm|%s|%s; owner: CI maintainers; reviewed: 2026-05-29; reason: audited Go assembly has no external dependency declarations
`, relativePath, strings.Repeat("0", 64)), 0o644)
	toolStderr := fmt.Sprintf("W0718 17:40:51.432546 123 library.go:101] \"example.com/asm\" contains non-Go code that can't be inspected for further dependencies:\n%s\n", filepath.Join(root, "module-cache", relativePath))

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "", "GO_LICENSES_FAKE_STDERR="+toolStderr)
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "reviewed non-Go dependency content changed") {
		t.Fatalf("output = %q, want content drift failure", out)
	}
}

func TestGoLicensesCheckRejectsStaleAssemblyPolicy(t *testing.T) {
	root := newGoLicensesFixture(t)
	relativePath := "example.com/asm@v1.0.0/asm_amd64.s"
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", fmt.Sprintf(`# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
go-license-non-go-allow: example.com/asm|%s|%s; owner: CI maintainers; reviewed: 2026-05-29; reason: audited Go assembly has no external dependency declarations
`, relativePath, strings.Repeat("0", 64)), 0o644)

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "stale go-license-non-go-allow:") {
		t.Fatalf("output = %q, want stale policy failure", out)
	}
}

func TestGoLicensesCheckRejectsAssemblyOutsideConfiguredModuleCache(t *testing.T) {
	root := newGoLicensesFixture(t)
	relativePath := "example.com/asm@v1.0.0/asm_amd64.s"
	assembly := []byte("#include \"textflag.h\"\n")
	goLicensesWriteFile(t, root, "outside/"+relativePath, string(assembly), 0o644)
	digest := sha256.Sum256(assembly)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", fmt.Sprintf(`# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
go-license-non-go-allow: example.com/asm|%s|%x; owner: CI maintainers; reviewed: 2026-05-29; reason: audited Go assembly has no external dependency declarations
`, relativePath, digest), 0o644)
	toolStderr := fmt.Sprintf("W0718 17:40:51.432546 123 library.go:101] \"example.com/asm\" contains non-Go code that can't be inspected for further dependencies:\n%s\n", filepath.Join(root, "outside", relativePath))

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "", "GO_LICENSES_FAKE_STDERR="+toolStderr)
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "outside the configured Go module cache") {
		t.Fatalf("output = %q, want module-cache boundary failure", out)
	}
}

func TestGoLicensesCheckRejectsUnexpectedSuccessfulToolStderr(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "", "GO_LICENSES_FAKE_STDERR=unexpected successful stderr\n")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "unexpected tool stderr") {
		t.Fatalf("output = %q, want unexpected stderr failure", out)
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

func TestGoLicensesCheckRejectsSymlinkPolicyFile(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	policy := filepath.Join(root, "docs", "design", "DEPENDENCIES.md")
	target := filepath.Join(root, "docs", "design", "DEPENDENCIES-target.md")
	if err := os.Rename(policy, target); err != nil {
		t.Fatalf("rename dependency policy: %v", err)
	}
	if err := os.Symlink(target, policy); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md must be a regular file, not a symlink") {
		t.Fatalf("output = %q, want symlink policy rejection", out)
	}
}

func TestGoLicensesCheckRejectsSymlinkPolicyParent(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	design := filepath.Join(root, "docs", "design")
	target := filepath.Join(root, "docs", "design-target")
	if err := os.Rename(design, target); err != nil {
		t.Fatalf("rename dependency policy parent: %v", err)
	}
	if err := os.Symlink(target, design); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md parent directory docs/design must not be a symlink") {
		t.Fatalf("output = %q, want symlink policy parent rejection", out)
	}
}

func TestGoLicensesCheckRejectsNonDirectoryPolicyParent(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	design := filepath.Join(root, "docs", "design")
	target := filepath.Join(root, "docs", "design-target")
	if err := os.Rename(design, target); err != nil {
		t.Fatalf("rename dependency policy parent: %v", err)
	}
	if err := os.WriteFile(design, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("write non-directory dependency policy parent: %v", err)
	}

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md parent directory docs/design must be a directory") {
		t.Fatalf("output = %q, want non-directory policy parent rejection", out)
	}
}

func TestGoLicensesCheckRejectsNonRegularPolicyFile(t *testing.T) {
	root := newGoLicensesFixture(t)
	goLicensesWriteFile(t, root, "docs/design/DEPENDENCIES.md", `# Dependency Policy

go-license-allow: Apache-2.0, MIT; owner: CI maintainers; reviewed: 2026-05-29; reason: accepted Go dependency licenses
`, 0o644)
	policy := filepath.Join(root, "docs", "design", "DEPENDENCIES.md")
	if err := os.Remove(policy); err != nil {
		t.Fatalf("remove dependency policy: %v", err)
	}
	if err := os.Mkdir(policy, 0o750); err != nil {
		t.Fatalf("mkdir dependency policy path: %v", err)
	}

	out, err := runGoLicensesCheck(t, root, filepath.Join(root, "calls.txt"), "")
	if err == nil {
		t.Fatalf("go licenses check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/DEPENDENCIES.md must be a regular file") {
		t.Fatalf("output = %q, want regular-file policy rejection", out)
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
	goLicensesWriteFile(t, root, "scripts/diagnosis_assistant_runner/go.mod", "module github.com/openclarion/openclarion/runtime/diagnosis-assistant\n", 0o644)
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
if [[ "$*" == "env GOMODCACHE" ]]; then
  printf '%s\n' "${GO_LICENSES_FAKE_GOMODCACHE:?}"
  exit 0
fi
printf '%s|%s\n' "$PWD" "$*" >>"${GO_LICENSES_CALLS:?}"
if [[ "${GO_LICENSES_FAKE_FAIL:-}" == "1" ]]; then
  echo "simulated go-licenses failure" >&2
  exit 42
fi
if [[ -n "${GO_LICENSES_FAKE_STDERR:-}" ]]; then
  printf '%s' "$GO_LICENSES_FAKE_STDERR" >&2
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

func runGoLicensesCheck(t *testing.T, root, callsPath, fakeFail string, extraEnv ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_go_licenses.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GO_LICENSES_CALLS="+callsPath,
		"GO_LICENSES_FAKE_FAIL="+fakeFail,
		"GO_LICENSES_FAKE_GOMODCACHE="+filepath.Join(root, "module-cache"),
		"GO_LICENSES_REVIEW_TODAY=2026-05-30",
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
