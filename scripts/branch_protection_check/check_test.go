package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchProtectionCheckAcceptsMatchingPolicy(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".github/workflows/ci.yml", workflowYAML([]string{"Docs hygiene", "Go checks"}))
	writePolicy(t, root, []string{"Docs hygiene", "Go checks"})

	var stdout bytes.Buffer
	err := run(config{
		PolicyPath:  filepath.Join(root, defaultPolicyPath),
		WorkflowDir: filepath.Join(root, defaultWorkflowDir),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[branch-protection] OK (2 required checks for main)") {
		t.Fatalf("stdout = %q, want OK", stdout.String())
	}
}

func TestBranchProtectionCheckRejectsPolicyDrift(t *testing.T) {
	tests := []struct {
		name     string
		contexts []string
		wantErr  string
	}{
		{
			name:     "missing context",
			contexts: []string{"Docs hygiene"},
			wantErr:  `missing required check context "Go checks"`,
		},
		{
			name:     "stale context",
			contexts: []string{"Docs hygiene", "Go checks", "Old check"},
			wantErr:  `stale required check context "Old check"`,
		},
		{
			name:     "duplicate context",
			contexts: []string{"Docs hygiene", "Docs hygiene", "Go checks"},
			wantErr:  `duplicate required check context "Docs hygiene"`,
		},
		{
			name:     "unsorted contexts",
			contexts: []string{"Go checks", "Docs hygiene"},
			wantErr:  "contexts must be sorted lexicographically",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".github/workflows/ci.yml", workflowYAML([]string{"Docs hygiene", "Go checks"}))
			writePolicy(t, root, tc.contexts)

			var stdout bytes.Buffer
			err := run(config{
				PolicyPath:  filepath.Join(root, defaultPolicyPath),
				WorkflowDir: filepath.Join(root, defaultWorkflowDir),
			}, &stdout)
			if err == nil {
				t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBranchProtectionCheckRejectsWeakInputs(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		wantErr string
	}{
		{
			name:    "unknown policy field",
			policy:  strings.Replace(policyJSON([]string{"Docs hygiene"}), `"contexts":`, `"unexpected": true, "contexts":`, 1),
			wantErr: `unknown field "unexpected"`,
		},
		{
			name:    "duplicate json key",
			policy:  strings.Replace(policyJSON([]string{"Docs hygiene"}), `"branch": "main",`, `"branch": "main", "branch": "main",`, 1),
			wantErr: `duplicate object key "branch"`,
		},
		{
			name:    "strict disabled",
			policy:  strings.Replace(policyJSON([]string{"Docs hygiene"}), `"strict": true`, `"strict": false`, 1),
			wantErr: "strict must be true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".github/workflows/ci.yml", workflowYAML([]string{"Docs hygiene"}))
			writeFile(t, root, defaultPolicyPath, tc.policy)

			var stdout bytes.Buffer
			err := run(config{
				PolicyPath:  filepath.Join(root, defaultPolicyPath),
				WorkflowDir: filepath.Join(root, defaultWorkflowDir),
			}, &stdout)
			if err == nil {
				t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBranchProtectionCheckRejectsWorkflowProblems(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		contexts []string
		wantErr  string
	}{
		{
			name: "duplicate job names",
			workflow: `name: ci
on:
  pull_request:
jobs:
  docs:
    name: Docs hygiene
  docs_copy:
    name: Docs hygiene
`,
			contexts: []string{"Docs hygiene"},
			wantErr:  `duplicate PR workflow job name "Docs hygiene"`,
		},
		{
			name: "missing job name",
			workflow: `name: ci
on:
  pull_request:
jobs:
  docs:
    runs-on: ubuntu-24.04
`,
			contexts: []string{},
			wantErr:  "jobs.docs: missing name",
		},
		{
			name: "duplicate yaml key",
			workflow: `name: ci
on:
  pull_request:
jobs:
  docs:
    name: Docs hygiene
    name: Docs hygiene
`,
			contexts: []string{"Docs hygiene"},
			wantErr:  "duplicate YAML key",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".github/workflows/ci.yml", tc.workflow)
			writePolicy(t, root, tc.contexts)

			var stdout bytes.Buffer
			err := run(config{
				PolicyPath:  filepath.Join(root, defaultPolicyPath),
				WorkflowDir: filepath.Join(root, defaultWorkflowDir),
			}, &stdout)
			if err == nil {
				t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestBranchProtectionCheckIgnoresNonPRWorkflows(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".github/workflows/ci.yml", workflowYAML([]string{"Docs hygiene"}))
	writeFile(t, root, ".github/workflows/scheduled.yml", `name: scheduled
on:
  schedule:
    - cron: "0 0 * * *"
jobs:
  live:
    name: Scheduled live check
`)
	writePolicy(t, root, []string{"Docs hygiene"})

	var stdout bytes.Buffer
	if err := run(config{
		PolicyPath:  filepath.Join(root, defaultPolicyPath),
		WorkflowDir: filepath.Join(root, defaultWorkflowDir),
	}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestBranchProtectionCheckRejectsNonRegularPolicyFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".github/workflows/ci.yml", workflowYAML([]string{"Docs hygiene"}))
	writePolicy(t, root, []string{"Docs hygiene"})
	if err := os.Rename(
		filepath.Join(root, defaultPolicyPath),
		filepath.Join(root, "docs/design/ci/branch-protection-real.json"),
	); err != nil {
		t.Fatalf("rename policy: %v", err)
	}
	if err := os.Symlink("branch-protection-real.json", filepath.Join(root, defaultPolicyPath)); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	var stdout bytes.Buffer
	err := run(config{
		PolicyPath:  filepath.Join(root, defaultPolicyPath),
		WorkflowDir: filepath.Join(root, defaultWorkflowDir),
	}, &stdout)
	if err == nil {
		t.Fatalf("run passed unexpectedly:\n%s", stdout.String())
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run error = %q, want symlink rejection", err.Error())
	}
}

func writePolicy(t *testing.T, root string, contexts []string) {
	t.Helper()
	writeFile(t, root, defaultPolicyPath, policyJSON(contexts))
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func policyJSON(contexts []string) string {
	var b strings.Builder
	b.WriteString(`{
  "schema": "openclarion.branch_protection_required_checks.v1",
  "branch": "main",
  "strict": true,
  "source_app": "github-actions",
  "contexts": [`)
	for i, context := range contexts {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n    ")
		b.WriteString(strconvQuote(context))
	}
	if len(contexts) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString(`]
}`)
	return b.String()
}

func workflowYAML(jobNames []string) string {
	var b strings.Builder
	b.WriteString(`name: ci
on:
  pull_request:
jobs:
`)
	for i, jobName := range jobNames {
		fmt.Fprintf(&b, "  job_%d:\n    name: %s\n", i, jobName)
	}
	return b.String()
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
