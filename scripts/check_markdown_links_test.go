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

func TestMarkdownLinksCheckRejectsSymlinkedMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/real.md", "# Docs\n", 0o644)
	if err := os.Symlink("real.md", filepath.Join(root, "docs", "README.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"non-regular markdown files detected",
		"docs/README.md",
		"is a symlink",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsSymlinkedDocsDirectory(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "real-docs/README.md", "# Docs\n", 0o644)
	if err := os.Symlink("real-docs", filepath.Join(root, "docs")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"non-regular markdown files detected",
		"docs/README.md",
		"contains symlink path component docs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsSymlinkedMarkdownPathComponents(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[target](alias/target.md)
`, 0o644)
	mdWriteFile(t, root, "docs/real/target.md", "# Target\n", 0o644)
	if err := os.Symlink("real", filepath.Join(root, "docs", "alias")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"non-regular markdown files detected",
		"docs/alias/target.md",
		"contains symlink path component docs/alias",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsNonRegularMarkdownTargets(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[directory target](target.md)
`, 0o644)
	if err := os.Mkdir(filepath.Join(root, "docs", "target.md"), 0o750); err != nil {
		t.Fatalf("mkdir target.md: %v", err)
	}

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"non-regular markdown files detected",
		"docs/target.md",
		"is not a regular file",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownLinksCheckRejectsTargetsOutsideRepository(t *testing.T) {
	root := t.TempDir()
	mdWriteFile(t, root, "scripts/check_markdown_links.sh", markdownLinksScript(t), 0o750)
	mdWriteFile(t, root, "docs/README.md", `# Docs

[outside](../../outside.md#outside)
`, 0o644)
	outside := filepath.Join(root, "..", "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o600); err != nil {
		t.Fatalf("write outside.md: %v", err)
	}

	out, err := runMarkdownLinksCheck(t, root)
	if err == nil {
		t.Fatalf("links check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"markdown links resolve outside the repository",
		"docs/README.md",
		"../../outside.md#outside",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("links check output = %q, want substring %q", out, want)
		}
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

func runMarkdownLinksCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_markdown_links.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
