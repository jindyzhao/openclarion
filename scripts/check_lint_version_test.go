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

func TestLintVersionCheckRejectsNonRegularInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "binary symlink",
			setup: func(t *testing.T, root string) {
				lintVersionReplaceWithSymlink(t, root, "bin/golangci-lint")
			},
			want: "golangci-lint binary must be a regular file, not a symlink",
		},
		{
			name: "binary directory",
			setup: func(t *testing.T, root string) {
				lintVersionReplaceWithDirectory(t, root, "bin/golangci-lint")
			},
			want: "golangci-lint binary not found or not executable",
		},
		{
			name: "module directory symlink",
			setup: func(t *testing.T, root string) {
				lintVersionReplaceWithSymlink(t, root, "tools/openclarion-linter")
			},
			want: "linter module directory must not be a symlink",
		},
		{
			name: "module go.mod symlink",
			setup: func(t *testing.T, root string) {
				lintVersionReplaceWithSymlink(t, root, "tools/openclarion-linter/go.mod")
			},
			want: "linter module go.mod must be a regular file, not a symlink",
		},
		{
			name: "module go.mod directory",
			setup: func(t *testing.T, root string) {
				lintVersionReplaceWithDirectory(t, root, "tools/openclarion-linter/go.mod")
			},
			want: "linter module go.mod not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newLintVersionFixture(t)
			tc.setup(t, root)

			out, err := runLintVersionCheck(t, root)
			if err == nil {
				t.Fatalf("lint version check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("lint version output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestLintVersionCheckAcceptsMatchingToolsVersion(t *testing.T) {
	root := newLintVersionFixture(t)

	out, err := runLintVersionCheck(t, root)
	if err != nil {
		t.Fatalf("lint version check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[lint-version-check] OK golang.org/x/tools v0.1.0") {
		t.Fatalf("lint version output = %q, want OK", out)
	}
}

func newLintVersionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	lintVersionWriteFile(t, root, "scripts/check_lint_version.sh", lintVersionScript(t), 0o750)
	lintVersionWriteFile(t, root, "bin/golangci-lint", "#!/usr/bin/env bash\nexit 0\n", 0o750)
	lintVersionWriteFile(t, root, "bin/go", fakeLintVersionGo(), 0o750)
	lintVersionWriteFile(t, root, "tools/openclarion-linter/go.mod", "module example.com/linter\n\nrequire golang.org/x/tools v0.1.0\n", 0o644)
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
if [[ "$1" == "version" && "$2" == "-m" ]]; then
  printf 'dep\tgolang.org/x/tools\tv0.1.0\n'
  exit 0
fi
if [[ "$1" == "list" && "$2" == "-m" ]]; then
  printf 'v0.1.0\n'
  exit 0
fi
echo "unexpected go command: $*" >&2
exit 1
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

func lintVersionReplaceWithSymlink(t *testing.T, root, name string) {
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

func lintVersionReplaceWithDirectory(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
}

func runLintVersionCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_lint_version.sh")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
