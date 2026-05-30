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

func TestLintVersionCheckAcceptsMatchingToolsVersion(t *testing.T) {
	root := newLintVersionFixture(t)

	out, err := runLintVersionCheck(t, root, "v0.44.0", "v0.44.0")
	if err != nil {
		t.Fatalf("lint-version check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[lint-version-check] OK golang.org/x/tools v0.44.0") {
		t.Fatalf("lint-version output = %q, want OK", out)
	}
}

func TestLintVersionCheckRejectsToolsVersionMismatch(t *testing.T) {
	root := newLintVersionFixture(t)

	out, err := runLintVersionCheck(t, root, "v0.44.0", "v0.45.0")
	if err == nil {
		t.Fatalf("lint-version check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"golang.org/x/tools version mismatch",
		"golangci-lint binary: v0.44.0",
		"openclarion-linter:  v0.45.0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("lint-version output = %q, want substring %q", out, want)
		}
	}
}

func TestLintVersionCheckRejectsNonRegularInputs(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, root string)
		wantErr string
	}{
		{
			name: "binary symlink",
			mutate: func(t *testing.T, root string) {
				lintVersionWriteFile(t, root, "bin/real-golangci-lint", "#!/usr/bin/env bash\n", 0o750)
				lintVersionReplaceWithSymlink(t, root, "bin/real-golangci-lint", "bin/golangci-lint")
			},
			wantErr: "golangci-lint binary must be a regular file, not a symlink: bin/golangci-lint",
		},
		{
			name: "binary not executable",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "bin/golangci-lint")); err != nil {
					t.Fatalf("remove binary: %v", err)
				}
				lintVersionWriteFile(t, root, "bin/golangci-lint", "#!/usr/bin/env bash\n", 0o640)
			},
			wantErr: "golangci-lint binary not found, not regular, or not executable: bin/golangci-lint",
		},
		{
			name: "module directory symlink",
			mutate: func(t *testing.T, root string) {
				lintVersionWriteFile(t, root, "tools/real-linter/go.mod", "module github.com/openclarion/openclarion/tools/openclarion-linter\n", 0o644)
				if err := os.RemoveAll(filepath.Join(root, "tools/openclarion-linter")); err != nil {
					t.Fatalf("remove module dir: %v", err)
				}
				lintVersionReplaceWithSymlink(t, root, "tools/real-linter", "tools/openclarion-linter")
			},
			wantErr: "linter module directory must not be a symlink: tools/openclarion-linter",
		},
		{
			name: "module go.mod symlink",
			mutate: func(t *testing.T, root string) {
				lintVersionWriteFile(t, root, "tools/openclarion-linter/real-go.mod", "module github.com/openclarion/openclarion/tools/openclarion-linter\n", 0o644)
				lintVersionReplaceWithSymlink(t, root, "tools/openclarion-linter/real-go.mod", "tools/openclarion-linter/go.mod")
			},
			wantErr: "linter module go.mod must be a regular file, not a symlink: tools/openclarion-linter/go.mod",
		},
		{
			name: "module go.mod directory",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "tools/openclarion-linter/go.mod")); err != nil {
					t.Fatalf("remove go.mod: %v", err)
				}
				if err := os.Mkdir(filepath.Join(root, "tools/openclarion-linter/go.mod"), 0o750); err != nil {
					t.Fatalf("mkdir go.mod path: %v", err)
				}
			},
			wantErr: "linter module go.mod not found or not regular: tools/openclarion-linter/go.mod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newLintVersionFixture(t)
			tt.mutate(t, root)

			out, err := runLintVersionCheck(t, root, "v0.44.0", "v0.44.0")
			if err == nil {
				t.Fatalf("lint-version check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tt.wantErr) {
				t.Fatalf("lint-version output = %q, want substring %q", out, tt.wantErr)
			}
		})
	}
}

func newLintVersionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	lintVersionWriteFile(t, root, "scripts/check_lint_version.sh", lintVersionScript(t), 0o750)
	lintVersionWriteFile(t, root, "bin/golangci-lint", "#!/usr/bin/env bash\n", 0o750)
	lintVersionWriteFile(t, root, "tools/openclarion-linter/go.mod", "module github.com/openclarion/openclarion/tools/openclarion-linter\n", 0o644)
	lintVersionWriteFile(t, root, "testbin/go", fakeLintVersionGo(), 0o750)
	return root
}

func lintVersionScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_lint_version.sh")
	if err != nil {
		t.Fatalf("read lint version script: %v", err)
	}
	return string(raw)
}

func fakeLintVersionGo() string {
	return `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FAKE_GO_CALLS:?}"
if [[ "$1" == "version" && "$2" == "-m" ]]; then
  printf '%s\n' "$3"
  printf '%s\n' "dep golang.org/x/tools ${FAKE_BINARY_TOOLS_VERSION:?}"
  exit 0
fi
if [[ "$1" == "list" && "$2" == "-m" ]]; then
  printf '%s\n' "${FAKE_MODULE_TOOLS_VERSION:?}"
  exit 0
fi
echo "unexpected go command: $*" >&2
exit 42
`
}

func lintVersionWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func lintVersionReplaceWithSymlink(t *testing.T, root, target, link string) {
	t.Helper()
	linkPath := filepath.Join(root, filepath.FromSlash(link))
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove %s: %v", linkPath, err)
	}
	relTarget, err := filepath.Rel(filepath.Dir(linkPath), filepath.Join(root, filepath.FromSlash(target)))
	if err != nil {
		t.Fatalf("relative symlink target: %v", err)
	}
	if err := os.Symlink(relTarget, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runLintVersionCheck(t *testing.T, root, binaryTools, moduleTools string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_lint_version.sh", "bin/golangci-lint", "tools/openclarion-linter")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Join(root, "testbin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GO_CALLS="+filepath.Join(root, "go-calls.txt"),
		"FAKE_BINARY_TOOLS_VERSION="+binaryTools,
		"FAKE_MODULE_TOOLS_VERSION="+moduleTools,
	)
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
