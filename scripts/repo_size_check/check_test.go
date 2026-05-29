package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckFilesPassesWithinBudget(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, filepath.Join(dir, "a.txt"), 3)
	writeSizedFile(t, filepath.Join(dir, "nested", "b.txt"), 5)

	summary, err := checkFiles(dir, []string{"nested/b.txt", "a.txt"}, 8, 10)
	if err != nil {
		t.Fatalf("checkFiles() error = %v", err)
	}
	if summary.TotalBytes != 8 {
		t.Fatalf("TotalBytes = %d, want 8", summary.TotalBytes)
	}
	if got := []string{summary.Files[0].Path, summary.Files[1].Path}; got[0] != "a.txt" || got[1] != "nested/b.txt" {
		t.Fatalf("Files sorted by path = %#v, want a.txt then nested/b.txt", got)
	}
}

func TestCheckFilesRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, filepath.Join(dir, "small.txt"), 4)
	writeSizedFile(t, filepath.Join(dir, "large.bin"), 9)

	_, err := checkFiles(dir, []string{"small.txt", "large.bin"}, 5, 20)
	if err == nil {
		t.Fatal("checkFiles() error = nil, want oversized file rejection")
	}
	if !strings.Contains(err.Error(), "large.bin is 9 bytes, exceeds max-file-bytes 5") {
		t.Fatalf("error = %v, want oversized file detail", err)
	}
}

func TestCheckFilesRejectsOversizedTotal(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, filepath.Join(dir, "a.txt"), 4)
	writeSizedFile(t, filepath.Join(dir, "b.txt"), 5)

	_, err := checkFiles(dir, []string{"a.txt", "b.txt"}, 10, 8)
	if err == nil {
		t.Fatal("checkFiles() error = nil, want oversized total rejection")
	}
	if !strings.Contains(err.Error(), "repository total is 9 bytes, exceeds max-total-bytes 8") {
		t.Fatalf("error = %v, want total detail", err)
	}
}

func TestCheckFilesRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, filepath.Join(dir, "target.txt"), 1)
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := checkFiles(dir, []string{"link.txt"}, 10, 10)
	if err == nil {
		t.Fatal("checkFiles() error = nil, want non-regular rejection")
	}
	if !strings.Contains(err.Error(), "Git-visible path must be a regular file") {
		t.Fatalf("error = %v, want regular-file detail", err)
	}
}

func TestCheckFilesRejectsUnsafePaths(t *testing.T) {
	dir := t.TempDir()
	for _, repoPath := range []string{"", "../outside.txt", "a/../b.txt", "/abs.txt"} {
		t.Run(repoPath, func(t *testing.T) {
			_, err := checkFiles(dir, []string{repoPath}, 10, 10)
			if err == nil {
				t.Fatal("checkFiles() error = nil, want unsafe path rejection")
			}
		})
	}
}

func TestCheckFilesRejectsInvalidBudgets(t *testing.T) {
	dir := t.TempDir()
	writeSizedFile(t, filepath.Join(dir, "a.txt"), 1)

	if _, err := checkFiles(dir, []string{"a.txt"}, 0, 10); err == nil {
		t.Fatal("checkFiles() max-file-bytes error = nil, want rejection")
	}
	if _, err := checkFiles(dir, []string{"a.txt"}, 10, 0); err == nil {
		t.Fatal("checkFiles() max-total-bytes error = nil, want rejection")
	}
}

func TestSplitNULPathsSortsAndDeduplicates(t *testing.T) {
	got := splitNULPaths([]byte("b.txt\x00a.txt\x00b.txt\x00"))
	if len(got) != 2 || got[0] != "a.txt" || got[1] != "b.txt" {
		t.Fatalf("splitNULPaths() = %#v, want sorted unique paths", got)
	}
}

func TestListGitVisibleFilesIncludesTrackedAndUntracked(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	writeTextFile(t, filepath.Join(dir, ".gitignore"), "ignored.bin\n")
	writeSizedFile(t, filepath.Join(dir, "tracked.txt"), 1)
	runGit(t, dir, "add", ".gitignore", "tracked.txt")
	writeSizedFile(t, filepath.Join(dir, "untracked.txt"), 1)
	writeSizedFile(t, filepath.Join(dir, "ignored.bin"), 1)

	paths, err := listGitVisibleFiles(context.Background(), dir, true)
	if err != nil {
		t.Fatalf("listGitVisibleFiles(includeUntracked=true) error = %v", err)
	}
	for _, want := range []string{".gitignore", "tracked.txt", "untracked.txt"} {
		if !contains(paths, want) {
			t.Fatalf("paths = %#v, want %s", paths, want)
		}
	}
	if contains(paths, "ignored.bin") {
		t.Fatalf("paths = %#v, want ignored.bin excluded", paths)
	}

	trackedOnly, err := listGitVisibleFiles(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("listGitVisibleFiles(includeUntracked=false) error = %v", err)
	}
	if contains(trackedOnly, "untracked.txt") {
		t.Fatalf("trackedOnly = %#v, want untracked file excluded", trackedOnly)
	}
}

func TestMainWithArgsRejectsUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{"unexpected"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected positional arguments") {
		t.Fatalf("stderr = %q, want unexpected argument detail", stderr.String())
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// #nosec G204 -- tests invoke the fixed git executable with test-controlled args.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func writeTextFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
