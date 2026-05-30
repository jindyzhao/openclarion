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

func TestParseStageOutputAcceptsNULSeparatedRecords(t *testing.T) {
	raw := []byte("100644 abc123 0\tREADME.md\x00100755 def456 0\tscripts/check thing.sh\x00")
	entries, err := parseStageOutput(raw)
	if err != nil {
		t.Fatalf("parseStageOutput: %v", err)
	}
	if got, want := len(entries), 2; got != want {
		t.Fatalf("len(entries) = %d, want %d", got, want)
	}
	if entries[1].Mode != "100755" || entries[1].Path != "scripts/check thing.sh" {
		t.Fatalf("entry[1] = %#v", entries[1])
	}
}

func TestParseStageOutputRejectsMalformedRecords(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{name: "missing tab", raw: []byte("100644 abc123 0 README.md\x00"), want: "missing tab"},
		{name: "short header", raw: []byte("100644 abc123\tREADME.md\x00"), want: "header"},
		{name: "empty path", raw: []byte("100644 abc123 0\t\x00"), want: "empty path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseStageOutput(tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseStageOutput error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckEntriesAcceptsRegularFilesAndScriptEntrypoints(t *testing.T) {
	problems := checkEntries([]indexEntry{
		{Mode: "100644", Path: "README.md"},
		{Mode: "100644", Path: "scripts/lib_atlas.sh"},
		{Mode: "100755", Path: "scripts/check_workflow_make_parity.sh"},
	})
	if len(problems) > 0 {
		t.Fatalf("checkEntries problems = %v", problems)
	}
}

func TestCheckEntriesRejectsUnexpectedModes(t *testing.T) {
	problems := checkEntries([]indexEntry{
		{Mode: "100755", Path: "README.md"},
		{Mode: "100755", Path: "scripts/nested/check.sh"},
		{Mode: "120000", Path: "docs/latest.md"},
		{Mode: "160000", Path: "third_party/tool"},
		{Mode: "100666", Path: "odd"},
	})
	joined := strings.Join(problems, "\n")
	for _, want := range []string{
		"README.md has executable bit",
		"scripts/nested/check.sh has executable bit",
		"docs/latest.md is a tracked symlink",
		"third_party/tool is a git submodule",
		"odd has unsupported git mode 100666",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("problems = %q, want %q", joined, want)
		}
	}
}

func TestRunUsesGitIndexModes(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "OpenClarion Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	writeFileModeCheckTestFile(t, dir, "README.md", "ok\n", 0o644)
	writeFileModeCheckTestFile(t, dir, "scripts/check_ok.sh", "#!/usr/bin/env bash\n", 0o755)
	runGit(t, dir, "add", ".")

	var stdout bytes.Buffer
	if err := run(context.Background(), config{RepoRoot: dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[file-mode] OK (2 tracked files checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func runGit(t *testing.T, dir string, args ...string) {
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

func writeFileModeCheckTestFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
