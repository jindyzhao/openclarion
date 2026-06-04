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

func TestDocsHygieneRejectsCJKAndIndirectDocs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "root governed doc with CJK",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				docsHygieneWriteFile(t, root, "README.md", "OpenClarion \u544a\u8b66\n", 0o644)
			},
			want: "Non-English CJK characters found",
		},
		{
			name: "nested governed doc with CJK",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				docsHygieneWriteFile(t, root, "docs/design/architecture.md", "OpenClarion \u544a\u8b66\n", 0o644)
			},
			want: "docs/design/architecture.md",
		},
		{
			name: "missing governed root doc",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				if err := os.Remove(filepath.Join(root, "SECURITY.md")); err != nil {
					t.Fatalf("remove SECURITY.md: %v", err)
				}
			},
			want: "governed documentation file is missing: SECURITY.md",
		},
		{
			name: "symlinked governed root doc",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				if err := os.Remove(filepath.Join(root, "README.md")); err != nil {
					t.Fatalf("remove README.md: %v", err)
				}
				docsHygieneWriteFile(t, root, "README.target.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)
				docsHygieneSymlink(t, "README.target.md", filepath.Join(root, "README.md"))
			},
			want: "governed documentation file must be a regular file: README.md",
		},
		{
			name: "symlinked docs entry",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				docsHygieneWriteFile(t, root, "docs/design/target.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)
				docsHygieneSymlink(t, "target.md", filepath.Join(root, "docs", "design", "indirect.md"))
			},
			want: "governed documentation paths must be regular files or directories",
		},
		{
			name: "docs directory symlink",
			setup: func(t *testing.T, root string) {
				docsHygieneWriteGovernedDocs(t, root)
				if err := os.RemoveAll(filepath.Join(root, "docs")); err != nil {
					t.Fatalf("remove docs dir: %v", err)
				}
				if err := os.Mkdir(filepath.Join(root, "docs-target"), 0o750); err != nil {
					t.Fatalf("mkdir docs target: %v", err)
				}
				docsHygieneSymlink(t, "docs-target", filepath.Join(root, "docs"))
			},
			want: "governed documentation directory must be a real directory: docs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := docsHygieneFixture(t)
			tc.setup(t, root)

			out, err := runDocsHygieneCheck(t, root)
			if err == nil {
				t.Fatalf("docs hygiene check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("docs hygiene output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestDocsHygieneAcceptsEnglishGovernedDocs(t *testing.T) {
	root := docsHygieneFixture(t)
	docsHygieneWriteGovernedDocs(t, root)
	docsHygieneWriteFile(t, root, "docs/design/architecture.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)

	out, err := runDocsHygieneCheck(t, root)
	if err != nil {
		t.Fatalf("docs hygiene check failed: %v\n%s", err, out)
	}
}

func docsHygieneFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	docsHygieneWriteFile(t, root, "scripts/check_no_non_english_chars.sh", docsHygieneScript(t), 0o750)
	return root
}

func docsHygieneScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_no_non_english_chars.sh")
	if err != nil {
		t.Fatalf("read docs hygiene script: %v", err)
	}
	return string(raw)
}

func docsHygieneWriteGovernedDocs(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{
		"README.md",
		"DEVELOPMENT_WORKFLOW.md",
		"CONTRIBUTING.md",
		"GOVERNANCE.md",
		"SECURITY.md",
		"CODE_OF_CONDUCT.md",
		"DCO.md",
		"MAINTAINERS.md",
	} {
		docsHygieneWriteFile(t, root, path, "# "+strings.TrimSuffix(path, ".md")+"\n\nOpenClarion remains intelligent alert analysis.\n", 0o644)
	}
	docsHygieneWriteFile(t, root, "docs/README.md", "# Docs\n\nOpenClarion docs.\n", 0o644)
}

func docsHygieneWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func docsHygieneSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runDocsHygieneCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_no_non_english_chars.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
