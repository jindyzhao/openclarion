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

func TestMarkdownLinksCheckValidatesAnchors(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[index](index.md)
`, 0o644)
	mdWriteFile(t, root, "docs/index.md", `# Project Status

[local](#project-status)
[remote heading](other.md#target-section)
[explicit anchor](other.md#custom-anchor)
`, 0o644)
	mdWriteFile(t, root, "docs/other.md", `# Target Section

<a id="custom-anchor"></a>
`, 0o644)

	out, err := runMarkdownLinksCheck(t, root)
	if err != nil {
		t.Fatalf("links check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[links-check] OK") {
		t.Fatalf("links check output = %q, want OK", out)
	}
}

func TestMarkdownLinksCheckRejectsMissingAnchors(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[index](index.md)
`, 0o644)
	mdWriteFile(t, root, "docs/index.md", `# Existing Section

[missing local](#missing-section)
[missing remote](other.md#missing-section)
`, 0o644)
	mdWriteFile(t, root, "docs/other.md", `# Other Section
`, 0o644)

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"broken markdown anchors detected",
		"#missing-section",
		"other.md#missing-section",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsOrphanDocs(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[reachable](reachable.md)
`, 0o644)
	mdWriteFile(t, root, "docs/reachable.md", "# Reachable\n", 0o644)
	mdWriteFile(t, root, "docs/orphan.md", "# Orphan\n", 0o644)

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"orphan docs detected",
		"docs/orphan.md",
		"not reachable from docs/README.md",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsIndirectMarkdownInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "symlinked markdown target",
			setup: func(t *testing.T, root string) {
				mdWriteFile(t, root, "docs/README.md", "# Docs\n\n[target](target.md)\n", 0o644)
				mdWriteFile(t, root, "docs/real-target.md", "# Target\n", 0o644)
				mdSymlink(t, "real-target.md", filepath.Join(root, "docs", "target.md"))
			},
			want: "docs/target.md uses symlink path component docs/target.md",
		},
		{
			name: "symlinked docs directory",
			setup: func(t *testing.T, root string) {
				mdWriteFile(t, root, "docs-target/README.md", "# Docs\n", 0o644)
				mdSymlink(t, "docs-target", filepath.Join(root, "docs"))
			},
			want: "docs must be a real directory, not a symlink or file",
		},
		{
			name: "symlinked markdown path component",
			setup: func(t *testing.T, root string) {
				mdWriteFile(t, root, "docs/README.md", "# Docs\n\n[target](linked/target.md)\n", 0o644)
				mdWriteFile(t, root, "docs/real/target.md", "# Target\n", 0o644)
				mdSymlink(t, "real", filepath.Join(root, "docs", "linked"))
			},
			want: "docs/linked/target.md uses symlink path component docs/linked",
		},
		{
			name: "markdown target directory",
			setup: func(t *testing.T, root string) {
				mdWriteFile(t, root, "docs/README.md", "# Docs\n\n[target](target.md)\n", 0o644)
				if err := os.Mkdir(filepath.Join(root, "docs", "target.md"), 0o750); err != nil {
					t.Fatalf("mkdir markdown target replacement: %v", err)
				}
			},
			want: "docs/target.md must be a regular markdown file",
		},
		{
			name: "relative link outside repository",
			setup: func(t *testing.T, root string) {
				mdWriteFile(t, root, "docs/README.md", "# Docs\n\n[outside](../../outside.md)\n", 0o644)
			},
			want: "relative markdown links resolve outside the repository",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
			tc.setup(t, root)

			out, err := runMarkdownLinksCheck(t, root)
			if err == nil {
				t.Fatalf("links check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("links check output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func markdownLinksScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_markdown_links.sh")
	if err != nil {
		t.Fatalf("read markdown links script: %v", err)
	}
	return string(raw)
}

func mdWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mdSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func runMarkdownLinksCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_markdown_links.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
