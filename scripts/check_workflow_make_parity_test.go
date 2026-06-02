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

const validWorkflowYAML = `name: ci

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

defaults:
  run:
    shell: bash

jobs:
  workflow-parity:
    runs-on: ubuntu-24.04
    timeout-minutes: 5
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd # v6.0.2
      - run: make workflow-parity
`

func TestWorkflowMakeParityAcceptsRegisteredWorkflow(t *testing.T) {
	root := writeWorkflowParityRepo(t, map[string]string{
		".github/workflows/ci.yml": validWorkflowYAML,
	}, []string{".github/workflows/ci.yml"})

	out, err := runWorkflowParity(t, root)
	if err != nil {
		t.Fatalf("workflow parity failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[workflow-parity] OK") {
		t.Fatalf("workflow parity output = %q, want OK", out)
	}
}

func TestWorkflowMakeParityRejectsIndirectInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "makefile symlink",
			setup: func(t *testing.T, root string) {
				replaceWithSymlink(t, root, "Makefile")
			},
			want: "Makefile must be a regular file, not a symlink",
		},
		{
			name: "makefile directory",
			setup: func(t *testing.T, root string) {
				replaceWithDirectory(t, root, "Makefile")
			},
			want: "Makefile must be a regular file",
		},
		{
			name: "ci readme symlink",
			setup: func(t *testing.T, root string) {
				replaceWithSymlink(t, root, "docs/design/ci/README.md")
			},
			want: "docs/design/ci/README.md must be a regular file, not a symlink",
		},
		{
			name: "ci readme directory",
			setup: func(t *testing.T, root string) {
				replaceWithDirectory(t, root, "docs/design/ci/README.md")
			},
			want: "docs/design/ci/README.md must be a regular file",
		},
		{
			name: "workflow file symlink",
			setup: func(t *testing.T, root string) {
				replaceWithSymlink(t, root, ".github/workflows/ci.yml")
			},
			want: ".github/workflows/ci.yml must be a regular file, not a symlink",
		},
		{
			name: "workflow file directory",
			setup: func(t *testing.T, root string) {
				replaceWithDirectory(t, root, ".github/workflows/ci.yml")
			},
			want: ".github/workflows/ci.yml must be a regular file",
		},
		{
			name: "workflow directory symlink",
			setup: func(t *testing.T, root string) {
				workflowsDir := filepath.Join(root, ".github", "workflows")
				target := filepath.Join(root, ".github", "workflows-target")
				if err := os.Rename(workflowsDir, target); err != nil {
					t.Fatalf("rename workflow directory: %v", err)
				}
				if err := os.Symlink(target, workflowsDir); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
			want: ".github/workflows must be a directory, not a symlink",
		},
		{
			name: "workflow directory parent symlink",
			setup: func(t *testing.T, root string) {
				githubDir := filepath.Join(root, ".github")
				target := filepath.Join(root, ".github-target")
				if err := os.Rename(githubDir, target); err != nil {
					t.Fatalf("rename workflow directory parent: %v", err)
				}
				if err := os.Symlink(target, githubDir); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
			want: ".github/workflows parent directory .github must not be a symlink",
		},
		{
			name: "ci readme parent symlink",
			setup: func(t *testing.T, root string) {
				ciDir := filepath.Join(root, "docs", "design", "ci")
				target := filepath.Join(root, "docs", "design", "ci-target")
				if err := os.Rename(ciDir, target); err != nil {
					t.Fatalf("rename CI README parent: %v", err)
				}
				if err := os.Symlink(target, ciDir); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
			want: "docs/design/ci/README.md parent directory docs/design/ci must not be a symlink",
		},
		{
			name: "dangling workflow directory symlink",
			setup: func(t *testing.T, root string) {
				workflowsDir := filepath.Join(root, ".github", "workflows")
				if err := os.RemoveAll(workflowsDir); err != nil {
					t.Fatalf("remove workflow directory: %v", err)
				}
				target := filepath.Join(root, ".github", "missing-workflows")
				if err := os.Symlink(target, workflowsDir); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
			},
			want: ".github/workflows must be a directory, not a symlink",
		},
		{
			name: "workflow directory regular file",
			setup: func(t *testing.T, root string) {
				workflowsDir := filepath.Join(root, ".github", "workflows")
				if err := os.RemoveAll(workflowsDir); err != nil {
					t.Fatalf("remove workflow directory: %v", err)
				}
				if err := os.WriteFile(workflowsDir, []byte("not a directory\n"), 0o600); err != nil {
					t.Fatalf("write workflow directory replacement: %v", err)
				}
			},
			want: ".github/workflows must be a directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeWorkflowParityRepo(t, map[string]string{
				".github/workflows/ci.yml": validWorkflowYAML,
			}, []string{".github/workflows/ci.yml"})
			tc.setup(t, root)

			out, err := runWorkflowParity(t, root)
			if err == nil {
				t.Fatalf("workflow parity passed unexpectedly:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("workflow parity output = %q, want substring %q", out, tc.want)
			}
		})
	}
}

func TestWorkflowMakeParityAcceptsJustifiedPermissionExpansion(t *testing.T) {
	workflow := strings.Replace(
		validWorkflowYAML,
		"permissions:\n  contents: read",
		"permissions:\n  contents: read\n  id-token: write # parity-allow: release OIDC token",
		1,
	)
	root := writeWorkflowParityRepo(t, map[string]string{
		".github/workflows/ci.yml": workflow,
	}, []string{".github/workflows/ci.yml"})

	out, err := runWorkflowParity(t, root)
	if err != nil {
		t.Fatalf("workflow parity failed: %v\n%s", err, out)
	}
}

func TestWorkflowMakeParityPRSecretsBoundary(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		wantOK   bool
		want     []string
	}{
		{
			name: "pull request workflow rejects secrets",
			workflow: strings.Replace(
				validWorkflowYAML,
				"      - run: make workflow-parity\n",
				"      - run: make workflow-parity\n        env:\n          TOKEN: ${{ secrets.OPENCLARION_TOKEN }}\n",
				1,
			),
			want: []string{
				"pull_request workflow must not reference GitHub secrets",
				"secrets.OPENCLARION_TOKEN",
			},
		},
		{
			name: "pull request target requires reviewer policy",
			workflow: strings.Replace(
				validWorkflowYAML,
				"  pull_request:\n",
				"  pull_request_target:\n",
				1,
			),
			want: []string{
				"pull_request_target workflow must include",
				"pull-request-target-review-policy",
			},
		},
		{
			name: "pull request target with reviewer policy is accepted",
			workflow: strings.Replace(
				strings.Replace(
					validWorkflowYAML,
					"name: ci\n\n",
					"name: ci\n\n# pull-request-target-review-policy: maintainers review workflow changes before secrets are exposed\n\n",
					1,
				),
				"  pull_request:\n",
				"  pull_request_target:\n",
				1,
			),
			wantOK: true,
			want:   []string{"[workflow-parity] OK"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeWorkflowParityRepo(t, map[string]string{
				".github/workflows/ci.yml": tc.workflow,
			}, []string{".github/workflows/ci.yml"})

			out, err := runWorkflowParity(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("workflow parity failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("workflow parity passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("workflow parity output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestWorkflowMakeParityRejectsRegistryDrift(t *testing.T) {
	tests := []struct {
		name         string
		workflows    map[string]string
		registryRows []string
		want         []string
	}{
		{
			name: "unregistered duplicate workflow name",
			workflows: map[string]string{
				".github/workflows/ci.yml":        validWorkflowYAML,
				".github/workflows/duplicate.yml": validWorkflowYAML,
			},
			registryRows: []string{".github/workflows/ci.yml"},
			want: []string{
				"Workflow File Registry",
				"duplicate workflow name `ci`",
			},
		},
		{
			name: "invalid filename",
			workflows: map[string]string{
				".github/workflows/ci.yml":       validWorkflowYAML,
				".github/workflows/custom.yaml":  strings.Replace(validWorkflowYAML, "name: ci", "name: custom", 1),
				".github/workflows/Bad_Name.yml": strings.Replace(validWorkflowYAML, "name: ci", "name: bad name", 1),
			},
			registryRows: []string{
				".github/workflows/ci.yml",
				".github/workflows/custom.yaml",
				".github/workflows/Bad_Name.yml",
			},
			want: []string{
				"custom.yaml: workflow filename must be `ci.yml` or `<gate>.yml`",
				"Bad_Name.yml: workflow filename must be `ci.yml` or `<gate>.yml`",
			},
		},
		{
			name: "missing workflow name",
			workflows: map[string]string{
				".github/workflows/ci.yml": validWorkflowYAML,
				".github/workflows/missing-name.yml": strings.Replace(
					validWorkflowYAML,
					"name: ci\n\n",
					"",
					1,
				),
			},
			registryRows: []string{
				".github/workflows/ci.yml",
				".github/workflows/missing-name.yml",
			},
			want: []string{"missing top-level workflow `name:`"},
		},
		{
			name: "broad permission without justification",
			workflows: map[string]string{
				".github/workflows/ci.yml": strings.Replace(
					validWorkflowYAML,
					"contents: read",
					"contents: write",
					1,
				),
			},
			registryRows: []string{".github/workflows/ci.yml"},
			want: []string{
				"permission `contents: write` exceeds `contents: read`",
				"# parity-allow: <reason>",
			},
		},
		{
			name: "inline permissions",
			workflows: map[string]string{
				".github/workflows/ci.yml": strings.Replace(
					validWorkflowYAML,
					"permissions:\n  contents: read",
					"permissions: read-all",
					1,
				),
			},
			registryRows: []string{".github/workflows/ci.yml"},
			want:         []string{"permissions must be a block with explicit entries"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeWorkflowParityRepo(t, tc.workflows, tc.registryRows)

			out, err := runWorkflowParity(t, root)
			if err == nil {
				t.Fatalf("workflow parity passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("workflow parity output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func writeWorkflowParityRepo(t *testing.T, workflows map[string]string, registryRows []string) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, root, "scripts/check_workflow_make_parity.sh", workflowParityScript(t), 0o750)
	writeFile(t, root, "Makefile", `.PHONY: workflow-parity
workflow-parity:
	@bash scripts/check_workflow_make_parity.sh
`, 0o644)
	writeFile(t, root, "docs/design/ci/README.md", workflowRegistry(registryRows), 0o644)
	for path, body := range workflows {
		writeFile(t, root, path, body, 0o644)
	}
	return root
}

func workflowParityScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_workflow_make_parity.sh")
	if err != nil {
		t.Fatalf("read workflow parity script: %v", err)
	}
	return string(raw)
}

func workflowRegistry(rows []string) string {
	var b strings.Builder
	b.WriteString("# CI\n\n## Workflow File Registry\n\n")
	b.WriteString("| Workflow file | Purpose |\n")
	b.WriteString("|---|---|\n")
	for _, row := range rows {
		b.WriteString("| `")
		b.WriteString(row)
		b.WriteString("` | test workflow |\n")
	}
	return b.String()
}

func writeFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func replaceWithSymlink(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	target := path + ".target"
	if err := os.Rename(path, target); err != nil {
		t.Fatalf("rename %s: %v", name, err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func replaceWithDirectory(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
}

func runWorkflowParity(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_workflow_make_parity.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
