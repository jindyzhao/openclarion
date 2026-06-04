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

func TestPRTitleCheckAcceptsConventionalCommitHeaders(t *testing.T) {
	tests := []string{
		"feat: add report trigger",
		"fix(alerting): handle duplicate events",
		"chore(ci)!: tighten workflow policy",
		"revert: feat(api): add legacy endpoint",
		"docs(adr-0013): clarify sandbox inputs",
	}

	for _, title := range tests {
		t.Run(title, func(t *testing.T) {
			root := newPRTitleFixture(t)
			out, err := runPRTitleCheck(t, root, title)
			if err != nil {
				t.Fatalf("pr title check failed: %v\n%s", err, out)
			}
			if !strings.Contains(out, "[pr-title-check] OK") {
				t.Fatalf("pr title output = %q, want OK", out)
			}
		})
	}
}

func TestPRTitleCheckRejectsInvalidTitles(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "uppercase type", title: "Feat: add report trigger", want: "not a Conventional Commit"},
		{name: "missing colon space", title: "feat:add report trigger", want: "not a Conventional Commit"},
		{name: "empty description", title: "fix: ", want: "not a Conventional Commit"},
		{name: "scope contains space", title: "feat(alert group): add trigger", want: "not a Conventional Commit"},
		{name: "trailing whitespace", title: "docs: update README ", want: "not a Conventional Commit"},
		{name: "multi line", title: "feat: add trigger\nbody", want: "single line"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newPRTitleFixture(t)
			out, err := runPRTitleCheck(t, root, tc.title)
			if err == nil {
				t.Fatalf("pr title check passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("pr title output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestPRTitleCheckRejectsOverlongTitle(t *testing.T) {
	root := newPRTitleFixture(t)
	title := "docs: " + strings.Repeat("a", 115)
	if got := len(title); got != 121 {
		t.Fatalf("test title length = %d, want 121", got)
	}

	out, err := runPRTitleCheck(t, root, title)
	if err == nil {
		t.Fatalf("pr title check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"PR title is 121 characters",
		"maximum is 120",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pr title output = %q, want substring %q", out, want)
		}
	}
}

func TestPRTitleCheckAcceptsMaxLengthTitle(t *testing.T) {
	root := newPRTitleFixture(t)
	title := "docs: " + strings.Repeat("a", 114)
	if got := len(title); got != 120 {
		t.Fatalf("test title length = %d, want 120", got)
	}

	out, err := runPRTitleCheck(t, root, title)
	if err != nil {
		t.Fatalf("pr title check failed at max length: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[pr-title-check] OK") {
		t.Fatalf("pr title output = %q, want OK", out)
	}
}

func TestPRTitleCheckRequiresTitle(t *testing.T) {
	root := newPRTitleFixture(t)
	out, err := runPRTitleCheck(t, root, "")
	if err == nil {
		t.Fatalf("pr title check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"PR_TITLE is required",
		"make pr-title-check",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("pr title output = %q, want substring %q", out, want)
		}
	}
}

func newPRTitleFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	prTitleWriteFile(t, root, "scripts/check_pr_title.sh", prTitleScript(t), 0o750)
	return root
}

func prTitleScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_pr_title.sh")
	if err != nil {
		t.Fatalf("read PR title script: %v", err)
	}
	return string(raw)
}

func prTitleWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runPRTitleCheck(t *testing.T, root, title string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_pr_title.sh")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if title != "" {
		cmd.Env = append(cmd.Env, "PR_TITLE="+title)
	}
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
