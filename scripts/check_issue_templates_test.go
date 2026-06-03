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

func TestIssueTemplateCheckAcceptsRequiredTemplates(t *testing.T) {
	root := newIssueTemplateFixture(t)
	out, err := runIssueTemplateCheck(t, root)
	if err != nil {
		t.Fatalf("issue template check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[issue-template-check] OK") {
		t.Fatalf("output = %q, want OK", out)
	}
}

func TestIssueTemplateCheckRejectsFrontMatterDrift(t *testing.T) {
	root := newIssueTemplateFixture(t)
	body := strings.Replace(validBugIssueTemplate(), "labels: bug", "labels: defect", 1)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/bug_report.md", body, 0o600)

	out, err := runIssueTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("issue template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "front matter labels = 'defect', want 'bug'") {
		t.Fatalf("output = %q, want label drift", out)
	}
}

func TestIssueTemplateCheckRejectsMissingRequiredSection(t *testing.T) {
	root := newIssueTemplateFixture(t)
	body := strings.Replace(validFeatureIssueTemplate(), "\n## Alternatives considered\n\n-\n", "\n", 1)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/feature_request.md", body, 0o600)

	out, err := runIssueTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("issue template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "missing ## Alternatives considered") {
		t.Fatalf("output = %q, want missing section", out)
	}
}

func TestIssueTemplateCheckRejectsEmptyRequiredSection(t *testing.T) {
	root := newIssueTemplateFixture(t)
	body := strings.Replace(validBugIssueTemplate(), "## What happened?\n\n-\n", "## What happened?\n\n<!-- fill this in -->\n", 1)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/bug_report.md", body, 0o600)

	out, err := runIssueTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("issue template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "empty ## What happened?") {
		t.Fatalf("output = %q, want empty section", out)
	}
}

func TestIssueTemplateCheckRejectsHeadingInCodeFence(t *testing.T) {
	root := newIssueTemplateFixture(t)
	body := strings.Replace(validFeatureIssueTemplate(), "## Proposal\n\n-\n", "```\n## Proposal\n-\n```\n", 1)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/feature_request.md", body, 0o600)

	out, err := runIssueTemplateCheck(t, root)
	if err == nil {
		t.Fatalf("issue template check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "missing ## Proposal") {
		t.Fatalf("output = %q, want fenced heading rejection", out)
	}
}

func newIssueTemplateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeIssueTemplateFile(t, root, "scripts/check_issue_templates.sh", issueTemplateScript(t), 0o750)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/bug_report.md", validBugIssueTemplate(), 0o600)
	writeIssueTemplateFile(t, root, ".github/ISSUE_TEMPLATE/feature_request.md", validFeatureIssueTemplate(), 0o600)
	return root
}

func issueTemplateScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_issue_templates.sh")
	if err != nil {
		t.Fatalf("read issue template script: %v", err)
	}
	return string(raw)
}

func validBugIssueTemplate() string {
	return `---
name: Bug report
about: Report a reproducible defect
title: "bug: "
labels: bug
---

## What happened?

-

## What did you expect?

-

## How can we reproduce it?

-

## Environment

- OpenClarion version or commit:
- Deployment mode:
- Browser, if applicable:

## Additional context

-
`
}

func validFeatureIssueTemplate() string {
	return `---
name: Feature request
about: Propose one focused capability
title: "feat: "
labels: enhancement
---

## Problem

-

## Proposal

-

## Alternatives considered

-

## Acceptance criteria

-
`
}

func writeIssueTemplateFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runIssueTemplateCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_issue_templates.sh")
	cmd.Dir = root
	cmd.Env = minimalIssueTemplateCheckEnv()
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}

func minimalIssueTemplateCheckEnv() []string {
	if path := os.Getenv("PATH"); path != "" {
		return []string{"PATH=" + path}
	}
	return nil
}
