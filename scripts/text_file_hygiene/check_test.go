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

func TestSplitNULPaths(t *testing.T) {
	got := splitNULPaths([]byte("README.md\x00docs/a b.md\x00"))
	want := []string{"README.md", "docs/a b.md"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("splitNULPaths = %#v, want %#v", got, want)
	}
}

func TestIsTextPolicyFile(t *testing.T) {
	cases := []struct {
		file string
		want bool
	}{
		{file: "README.md", want: true},
		{file: "web/package-lock.json", want: true},
		{file: "scripts/check.sh", want: true},
		{file: "Makefile", want: true},
		{file: "scripts/custom_thin_runner/Dockerfile", want: true},
		{file: "docs/image.png", want: false},
		{file: "archive.tar.gz", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			if got := isTextPolicyFile(tc.file); got != tc.want {
				t.Fatalf("isTextPolicyFile(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}

func TestInspectTextFileAcceptsUTF8LFAndTabs(t *testing.T) {
	issues := inspectTextFile("README.md", []byte("ok\ttext\nunicode: clarion\n"))
	if len(issues) > 0 {
		t.Fatalf("inspectTextFile issues = %v", issues)
	}
}

func TestInspectTextFileRejectsHiddenTextDrift(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "bom", data: []byte{0xEF, 0xBB, 0xBF, 'o', 'k', '\n'}, want: "UTF-8 BOM"},
		{name: "invalid utf8", data: []byte{0xff, '\n'}, want: "valid UTF-8"},
		{name: "crlf", data: []byte("one\r\ntwo\n"), want: "not CRLF"},
		{name: "bare cr", data: []byte("one\rtwo\n"), want: "bare carriage"},
		{name: "nul", data: []byte("one\x00two\n"), want: "NUL"},
		{name: "escape", data: []byte("one\x1btwo\n"), want: "0x1B"},
		{name: "del", data: []byte("one\x7ftwo\n"), want: "0x7F"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := inspectTextFile("fixture.txt", tc.data)
			joined := joinIssueMessages(issues)
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("issues = %q, want %q", joined, tc.want)
			}
		})
	}
}

func TestLineColumn(t *testing.T) {
	line, column := lineColumn([]byte("one\ntwo\nthree"), 8)
	if line != 3 || column != 1 {
		t.Fatalf("lineColumn = (%d, %d), want (3, 1)", line, column)
	}
}

func TestRunChecksTrackedTextFilesAndSkipsBinaryExtensions(t *testing.T) {
	dir := t.TempDir()
	runTextHygieneGit(t, dir, "init")
	runTextHygieneGit(t, dir, "config", "user.name", "OpenClarion Test")
	runTextHygieneGit(t, dir, "config", "user.email", "test@example.com")
	writeTextHygieneTestFile(t, dir, "README.md", []byte("ok\n"))
	writeTextHygieneTestFile(t, dir, "web/package-lock.json", []byte("{\"ok\":true}\n"))
	writeTextHygieneTestFile(t, dir, "assets/blob.bin", []byte{0, 1, 2, 3})
	runTextHygieneGit(t, dir, "add", ".")

	var stdout bytes.Buffer
	if err := run(context.Background(), config{RepoRoot: dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[text-file-hygiene] OK (2 tracked text files checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsTrackedCRLFTextFile(t *testing.T) {
	dir := t.TempDir()
	runTextHygieneGit(t, dir, "init")
	runTextHygieneGit(t, dir, "config", "user.name", "OpenClarion Test")
	runTextHygieneGit(t, dir, "config", "user.email", "test@example.com")
	writeTextHygieneTestFile(t, dir, "README.md", []byte("bad\r\n"))
	runTextHygieneGit(t, dir, "add", ".")

	var stdout bytes.Buffer
	err := run(context.Background(), config{RepoRoot: dir}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "README.md: must use LF line endings") {
		t.Fatalf("run error = %v, want CRLF rejection", err)
	}
}

func runTextHygieneGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- test helper passes fixed git subcommands.
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeTextHygieneTestFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func joinIssueMessages(issues []fileIssue) string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Message)
	}
	return strings.Join(messages, "\n")
}
