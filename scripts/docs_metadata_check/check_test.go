// Package main tests the docs metadata freshness checker.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAcceptsFreshLastUpdatedMetadata(t *testing.T) {
	root := t.TempDir()
	writeDocMetadataFile(t, root, "docs/current.md", "# Current\n\n> Last updated: 2026-05-30\n\n| Date | Change |\n|------|--------|\n| 2026-05-29 | shipped |\n")

	result, err := Check([]string{filepath.Join(root, "docs")})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Files != 1 || result.MetadataFiles != 1 {
		t.Fatalf("result = %+v, want 1 file and 1 metadata file", result)
	}
}

func TestCheckRejectsStaleLastUpdatedMetadata(t *testing.T) {
	root := t.TempDir()
	writeDocMetadataFile(t, root, "docs/roadmap.md", "# Roadmap\n\n> Last updated: 2026-05-29\n\n| Date | Change |\n|------|--------|\n| 2026-05-30 | shipped |\n")

	_, err := Check([]string{filepath.Join(root, "docs")})
	if err == nil {
		t.Fatal("Check passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "Last updated 2026-05-29 is older than latest dated table row 2026-05-30") {
		t.Fatalf("error = %q, want stale metadata message", err)
	}
}

func TestCheckIgnoresFutureDatesOutsideDatedTableRows(t *testing.T) {
	root := t.TempDir()
	writeDocMetadataFile(t, root, "docs/current.md", "# Current\n\n> Last updated: 2026-05-29\n\nTemporary allowlist expires on 2026-08-31.\n\n| Expires | Owner |\n|---------|-------|\n| 2026-08-31 | security |\n\n| Date | Change |\n|------|--------|\n| 2026-05-29 | shipped |\n")

	if _, err := Check([]string{filepath.Join(root, "docs")}); err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
}

func TestCheckRejectsDuplicateLastUpdatedMetadata(t *testing.T) {
	root := t.TempDir()
	writeDocMetadataFile(t, root, "docs/current.md", "# Current\n\n> Last updated: 2026-05-29\n> Last updated: 2026-05-30\n")

	_, err := Check([]string{filepath.Join(root, "docs")})
	if err == nil {
		t.Fatal("Check passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "duplicate Last updated metadata") {
		t.Fatalf("error = %q, want duplicate metadata message", err)
	}
}

func TestCheckRejectsSymlinkMarkdownInputs(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	if err := os.WriteFile(target, []byte("# Target\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(root, "docs", "link.md")
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, err := Check([]string{filepath.Join(root, "docs")})
	if err == nil {
		t.Fatal("Check passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("error = %q, want symlink rejection", err)
	}
}

func writeDocMetadataFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
