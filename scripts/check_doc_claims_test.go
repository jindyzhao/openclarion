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

func TestDocClaimsCheckValidatesShippedPathHints(t *testing.T) {
	root := newDocClaimsFixture(t, docClaimsCurrentState("scripts/check_doc_claims.sh"))

	out, err := runDocClaimsCheck(t, root)
	if err != nil {
		t.Fatalf("doc-claims check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[doc-claims] OK (1 shipped path hints)") {
		t.Fatalf("doc-claims output = %q, want OK", out)
	}
}

func TestDocClaimsCheckRejectsMissingShippedPathHints(t *testing.T) {
	root := newDocClaimsFixture(t, docClaimsCurrentState("scripts/missing.sh"))

	out, err := runDocClaimsCheck(t, root)
	if err == nil {
		t.Fatalf("doc-claims check passed unexpectedly:\n%s", out)
	}
	want := "docs/design/CURRENT_STATE.md claims shipped path that does not exist: scripts/missing.sh"
	if !strings.Contains(out, want) {
		t.Fatalf("doc-claims output = %q, want substring %q", out, want)
	}
}

func TestDocClaimsCheckRejectsNonRegularCurrentState(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, root string)
		wantErr string
	}{
		{
			name: "current state symlink",
			mutate: func(t *testing.T, root string) {
				docClaimsWriteFile(t, root, "docs/design/REAL_CURRENT_STATE.md", docClaimsCurrentState("scripts/check_doc_claims.sh"), 0o644)
				docClaimsReplaceWithSymlink(t, root, "docs/design/REAL_CURRENT_STATE.md", "docs/design/CURRENT_STATE.md")
			},
			wantErr: "docs/design/CURRENT_STATE.md must be a regular file, not a symlink",
		},
		{
			name: "current state directory",
			mutate: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "docs/design/CURRENT_STATE.md")); err != nil {
					t.Fatalf("remove current state: %v", err)
				}
				if err := os.Mkdir(filepath.Join(root, "docs/design/CURRENT_STATE.md"), 0o750); err != nil {
					t.Fatalf("mkdir current state path: %v", err)
				}
			},
			wantErr: "missing or non-regular docs/design/CURRENT_STATE.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newDocClaimsFixture(t, docClaimsCurrentState("scripts/check_doc_claims.sh"))
			tt.mutate(t, root)

			out, err := runDocClaimsCheck(t, root)
			if err == nil {
				t.Fatalf("doc-claims check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tt.wantErr) {
				t.Fatalf("doc-claims output = %q, want substring %q", out, tt.wantErr)
			}
		})
	}
}

func newDocClaimsFixture(t *testing.T, currentState string) string {
	t.Helper()
	root := t.TempDir()
	docClaimsWriteFile(t, root, "scripts/check_doc_claims.sh", docClaimsScript(t), 0o750)
	docClaimsWriteFile(t, root, "docs/design/CURRENT_STATE.md", currentState, 0o644)
	return root
}

func docClaimsCurrentState(pathHint string) string {
	return `# Current State

## Implementation Status

| Area | Status | Notes |
|------|--------|-------|
| Test gate | shipped | ` + "`" + pathHint + "`" + ` |
`
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

func docClaimsReplaceWithSymlink(t *testing.T, root, target, link string) {
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

func runDocClaimsCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_doc_claims.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
