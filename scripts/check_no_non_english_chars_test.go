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

func TestDocsHygieneEnglishOnlyCheck(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, root string)
		wantOK bool
		want   []string
	}{
		{
			name: "english governed docs are accepted",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				writeDocsHygieneFile(t, root, "docs/design/architecture.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)
			},
			wantOK: true,
		},
		{
			name: "root governed doc with CJK is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				writeDocsHygieneFile(t, root, "README.md", "OpenClarion "+hanFixture()+"\n", 0o644)
			},
			want: []string{
				"README.md",
				"Non-English CJK characters found",
			},
		},
		{
			name: "nested governed doc with CJK is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				writeDocsHygieneFile(t, root, "docs/design/architecture.md", "OpenClarion "+hanFixture()+"\n", 0o644)
			},
			want: []string{
				"docs/design/architecture.md",
				"Non-English CJK characters found",
			},
		},
		{
			name: "missing governed root doc is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.Remove(filepath.Join(root, "SECURITY.md")); err != nil {
					t.Fatalf("remove governed doc: %v", err)
				}
			},
			want: []string{
				"governed documentation file is missing",
				"SECURITY.md",
			},
		},
		{
			name: "symlinked governed root doc is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.Remove(filepath.Join(root, "README.md")); err != nil {
					t.Fatalf("remove README.md: %v", err)
				}
				writeDocsHygieneFile(t, root, "README.target.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)
				createDocsHygieneSymlink(t, "README.target.md", filepath.Join(root, "README.md"))
			},
			want: []string{
				"governed documentation file must be a regular file",
				"README.md",
			},
		},
		{
			name: "dangling governed root doc is rejected as indirect",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.Remove(filepath.Join(root, "README.md")); err != nil {
					t.Fatalf("remove README.md: %v", err)
				}
				createDocsHygieneSymlink(t, "missing-readme.md", filepath.Join(root, "README.md"))
			},
			want: []string{
				"governed documentation file must be a regular file",
				"README.md",
			},
		},
		{
			name: "directory governed root doc is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				path := filepath.Join(root, "README.md")
				if err := os.Remove(path); err != nil {
					t.Fatalf("remove README.md: %v", err)
				}
				if err := os.Mkdir(path, 0o750); err != nil {
					t.Fatalf("mkdir README.md replacement: %v", err)
				}
			},
			want: []string{
				"governed documentation file must be a regular file",
				"README.md",
			},
		},
		{
			name: "symlinked docs entry is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				writeDocsHygieneFile(t, root, "docs/design/target.md", "OpenClarion remains intelligent alert analysis.\n", 0o644)
				createDocsHygieneSymlink(t, "target.md", filepath.Join(root, "docs", "design", "indirect.md"))
			},
			want: []string{
				"governed documentation paths must be regular files or directories",
				"docs/design/indirect.md",
			},
		},
		{
			name: "dangling docs entry is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				createDocsHygieneSymlink(t, "missing.md", filepath.Join(root, "docs", "design", "dangling.md"))
			},
			want: []string{
				"governed documentation paths must be regular files or directories",
				"docs/design/dangling.md",
			},
		},
		{
			name: "missing docs directory is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.RemoveAll(filepath.Join(root, "docs")); err != nil {
					t.Fatalf("remove docs dir: %v", err)
				}
			},
			want: []string{
				"governed documentation directory is missing",
				"docs",
			},
		},
		{
			name: "docs symlink is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.RemoveAll(filepath.Join(root, "docs")); err != nil {
					t.Fatalf("remove docs dir: %v", err)
				}
				if err := os.Mkdir(filepath.Join(root, "docs-target"), 0o750); err != nil {
					t.Fatalf("mkdir docs target: %v", err)
				}
				createDocsHygieneSymlink(t, "docs-target", filepath.Join(root, "docs"))
			},
			want: []string{
				"governed documentation directory must be a real directory",
				"docs",
			},
		},
		{
			name: "docs regular file is rejected",
			setup: func(t *testing.T, root string) {
				t.Helper()
				writeDocsHygieneGovernedDocs(t, root)
				if err := os.RemoveAll(filepath.Join(root, "docs")); err != nil {
					t.Fatalf("remove docs dir: %v", err)
				}
				writeDocsHygieneFile(t, root, "docs", "not a directory\n", 0o644)
			},
			want: []string{
				"governed documentation directory must be a real directory",
				"docs",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newDocsHygieneRepo(t)
			tc.setup(t, root)

			out, err := runDocsHygieneCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("docs-hygiene failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("docs-hygiene passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("docs-hygiene output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func newDocsHygieneRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeDocsHygieneFile(t, root, "scripts/check_no_non_english_chars.sh", docsHygieneScript(t), 0o750)
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

func writeDocsHygieneGovernedDocs(t *testing.T, root string) {
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
		writeDocsHygieneFile(t, root, path, "# "+strings.TrimSuffix(path, ".md")+"\n\nOpenClarion remains intelligent alert analysis.\n", 0o644)
	}
	writeDocsHygieneFile(t, root, "docs/README.md", "# Docs\n\nOpenClarion docs.\n", 0o644)
}

func writeDocsHygieneFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func createDocsHygieneSymlink(t *testing.T, oldname, newname string) {
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

func hanFixture() string {
	return "\u544a\u8b66"
}
