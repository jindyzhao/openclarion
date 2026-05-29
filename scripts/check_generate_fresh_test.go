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

func TestGenerateFreshCheckPassesWhenGenerateIsStable(t *testing.T) {
	root := t.TempDir()
	generateFreshWriteFile(t, root, "scripts/check_generate_fresh.sh", generateFreshScript(t), 0o750)
	generateFreshWriteFile(t, root, "Makefile", "generate:\n\t@printf 'stable\\n' > generated.txt\n", 0o644)
	generateFreshWriteFile(t, root, "generated.txt", "stable\n", 0o644)
	generateFreshInitGit(t, root)

	out, err := runGenerateFreshCheck(t, root)
	if err != nil {
		t.Fatalf("generate fresh check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[generate-fresh] OK") {
		t.Fatalf("generate fresh output = %q, want OK", out)
	}
}

func TestGenerateFreshCheckRejectsTrackedGeneratorDiff(t *testing.T) {
	root := t.TempDir()
	generateFreshWriteFile(t, root, "scripts/check_generate_fresh.sh", generateFreshScript(t), 0o750)
	generateFreshWriteFile(t, root, "Makefile", "generate:\n\t@printf 'new\\n' > generated.txt\n", 0o644)
	generateFreshWriteFile(t, root, "generated.txt", "old\n", 0o644)
	generateFreshInitGit(t, root)

	out, err := runGenerateFreshCheck(t, root)
	if err == nil {
		t.Fatalf("generate fresh check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"make generate changed repository files",
		"generated.txt",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("generate fresh output = %q, want substring %q", out, want)
		}
	}
}

func TestGenerateFreshCheckRejectsUntrackedGeneratorDiff(t *testing.T) {
	root := t.TempDir()
	generateFreshWriteFile(t, root, "scripts/check_generate_fresh.sh", generateFreshScript(t), 0o750)
	generateFreshWriteFile(t, root, "Makefile", "generate:\n\t@printf 'new\\n' > generated.txt\n", 0o644)
	generateFreshInitGit(t, root)

	out, err := runGenerateFreshCheck(t, root)
	if err == nil {
		t.Fatalf("generate fresh check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"make generate changed repository files",
		"generated.txt",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("generate fresh output = %q, want substring %q", out, want)
		}
	}
}

func TestGenerateFreshCheckRejectsSecondRunDiff(t *testing.T) {
	root := t.TempDir()
	generateFreshWriteFile(t, root, "scripts/check_generate_fresh.sh", generateFreshScript(t), 0o750)
	generateFreshWriteFile(t, root, "Makefile", "generate:\n\t@if test -f .git/openclarion-second-generate; then printf 'changed\\n' > generated.txt; else touch .git/openclarion-second-generate; fi\n", 0o644)
	generateFreshWriteFile(t, root, "generated.txt", "stable\n", 0o644)
	generateFreshInitGit(t, root)

	out, err := runGenerateFreshCheck(t, root)
	if err == nil {
		t.Fatalf("generate fresh check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"make generate is not idempotent",
		"generated.txt",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("generate fresh output = %q, want substring %q", out, want)
		}
	}
}

func generateFreshScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_generate_fresh.sh")
	if err != nil {
		t.Fatalf("read generate fresh script: %v", err)
	}
	return string(raw)
}

func generateFreshWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func generateFreshInitGit(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"add", "."},
	} {
		cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed test command with controlled args.
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func runGenerateFreshCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_generate_fresh.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
