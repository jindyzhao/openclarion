// Package main tests the E2E verification verdict taxonomy checker.
package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAcceptsDefinedVerdicts(t *testing.T) {
	path := writeDoc(t, `# End-to-End Chain Verification

## Verdict Scale

| Verdict | Meaning |
|---------|---------|
| **proven** | direct proof |
| **proven-local** | local proof |
| **partial** | incomplete proof |

## Chain A

| Node | Verdict |
|------|---------|
| A1 | proven-local |
| A2 | **partial** |
`)

	if err := Check(path); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestCheckRejectsUndefinedVerdicts(t *testing.T) {
	path := writeDoc(t, `# End-to-End Chain Verification

## Verdict Scale

| Verdict | Meaning |
|---------|---------|
| proven | direct proof |

## Chain A

| Node | Verdict |
|------|---------|
| A1 | partial |
`)

	err := Check(path)
	if err == nil {
		t.Fatal("Check() passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "undefined verdict(s): partial") {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestCheckRejectsMissingScale(t *testing.T) {
	path := writeDoc(t, `# End-to-End Chain Verification

## Chain A

| Node | Verdict |
|------|---------|
| A1 | proven |
`)

	err := Check(path)
	if err == nil {
		t.Fatal("Check() passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "missing Verdict Scale table") {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestCheckRejectsMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.md")

	err := Check(path)
	if err == nil {
		t.Fatal("Check() passed unexpectedly")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Check() error = %v, want fs.ErrNotExist", err)
	}
}

func TestCheckRejectsDirectory(t *testing.T) {
	path := t.TempDir()

	err := Check(path)
	if err == nil {
		t.Fatal("Check() passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "must be a regular file") {
		t.Fatalf("Check() error = %v", err)
	}
}

func TestCheckRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	if err := os.WriteFile(target, []byte("# Target\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	err := Check(link)
	if err == nil {
		t.Fatal("Check() passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("Check() error = %v", err)
	}
}

func writeDoc(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "END_TO_END_VERIFICATION.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	return path
}
