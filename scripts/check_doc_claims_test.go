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

func TestDocClaimsRejectsNonRegularCurrentState(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, root string) {
				docClaimsReplaceWithSymlink(t, root, "docs/design/CURRENT_STATE.md")
			},
			want: "docs/design/CURRENT_STATE.md must be a regular file, not a symlink",
		},
		{
			name: "directory",
			setup: func(t *testing.T, root string) {
				docClaimsReplaceWithDirectory(t, root, "docs/design/CURRENT_STATE.md")
			},
			want: "missing or non-regular docs/design/CURRENT_STATE.md",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newDocClaimsFixture(t)
			tc.setup(t, root)

			out, err := runDocClaimsCheck(t, root)
			if err == nil {
				t.Fatalf("doc claims check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("doc claims output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestDocClaimsAcceptsRegularCurrentState(t *testing.T) {
	root := newDocClaimsFixture(t)

	out, err := runDocClaimsCheck(t, root)
	if err != nil {
		t.Fatalf("doc claims check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[doc-claims] OK") {
		t.Fatalf("doc claims output = %q, want OK", out)
	}
}

func newDocClaimsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	docClaimsWriteFile(t, root, "scripts/check_doc_claims.sh", docClaimsScript(t), 0o750)
	docClaimsWriteFile(t, root, "docs/design/CURRENT_STATE.md", "| Item | Status |\n|---|---|\n| none | planned |\n", 0o644)
	return root
}

func docClaimsScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_doc_claims.sh")
	if err != nil {
		t.Fatalf("read doc claims script: %v", err)
	}
	return string(raw)
}

func docClaimsWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func docClaimsReplaceWithSymlink(t *testing.T, root, name string) {
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

func docClaimsReplaceWithDirectory(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
}

func runDocClaimsCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_doc_claims.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
