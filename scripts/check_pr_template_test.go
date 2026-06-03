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

func TestPRTemplateCheckAcceptsRequiredSections(t *testing.T) {
	root := newPRTemplateFixture(t, `## Summary

-

## Risk

-

## Rollback

-

## Local verification

- [ ] `+"`make pr`"+`

## DCO

- [ ] Signed off
`)
	out, err := runPRTemplateCheck(t, root)
	if err != nil {
		t.Fatalf("pr template check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[pr-template-check] OK") {
		t.Fatalf("output = %q, want OK", out)
	}
}

func TestPRTemplateCheckRejectsMissingRollback(t *testing.T) {
	root := newPRTemplateFixture(t, `## Summary

-

## Risk

-

## Local verification

- [ ] `+"`make pr`"+`

## DCO

- [ ] Signed off
`)
	out, err := runPRTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("pr template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "missing ## Rollback") {
		t.Fatalf("output = %q, want missing Rollback", out)
	}
}

func TestPRTemplateCheckRejectsEmptyRequiredSection(t *testing.T) {
	root := newPRTemplateFixture(t, `## Summary

<!-- fill this in -->

## Risk

-

## Rollback

-

## Local verification

- [ ] `+"`make pr`"+`

## DCO

- [ ] Signed off
`)
	out, err := runPRTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("pr template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "empty ## Summary") {
		t.Fatalf("output = %q, want empty Summary", out)
	}
}

func TestPRTemplateCheckRejectsHeadingInCodeFence(t *testing.T) {
	root := newPRTemplateFixture(t, "```\n## Rollback\n-\n```\n\n## Summary\n-\n\n## Risk\n-\n\n## Local verification\n-\n\n## DCO\n-\n")
	out, err := runPRTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("pr template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "missing ## Rollback") {
		t.Fatalf("output = %q, want missing Rollback", out)
	}
}

func newPRTemplateFixture(t *testing.T, template string) string {
	t.Helper()
	root := t.TempDir()
	writePRTemplateFile(t, root, "scripts/check_pr_template.sh", prTemplateScript(t), 0o750)
	writePRTemplateFile(t, root, ".github/pull_request_template.md", template, 0o600)
	return root
}

func prTemplateScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_pr_template.sh")
	if err != nil {
		t.Fatalf("read PR template script: %v", err)
	}
	return string(raw)
}

func writePRTemplateFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runPRTemplateCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_pr_template.sh")
	cmd.Dir = root
	cmd.Env = minimalPRTemplateCheckEnv()
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func minimalPRTemplateCheckEnv() []string {
	if path := os.Getenv("PATH"); path != "" {
		return []string{"PATH=" + path}
	}
	return nil
}
