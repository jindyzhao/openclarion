package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsIsolatedManualTargets(t *testing.T) {
	root := t.TempDir()
	writeManualIsolationFile(t, root, "Makefile", `ci: workflow-parity docs
workflow-parity: ## Check workflow parity
docs: ## Check docs
live-smoke: ## Manual smoke: requires real services
`)
	writeManualIsolationFile(t, root, "docs/design/ci/manual-targets.tsv", "target\treason\nlive-smoke\trequires real services\n")
	writeManualIsolationFile(t, root, "docs/design/ci/README.md", "Run `make live-smoke` only by hand.\n")
	writeManualIsolationFile(t, root, ".github/workflows/ci.yml", "jobs:\n  docs:\n    steps:\n      - run: make docs\n")

	var stdout bytes.Buffer
	err := run(config{
		MakefilePath: filepath.Join(root, "Makefile"),
		PolicyPath:   filepath.Join(root, "docs/design/ci/manual-targets.tsv"),
		CIReadmePath: filepath.Join(root, "docs/design/ci/README.md"),
		WorkflowsDir: filepath.Join(root, ".github/workflows"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[manual-target-isolation] OK (1 manual targets isolated; 1 workflow make runs checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsManualTargetReachableFromCI(t *testing.T) {
	root := t.TempDir()
	writeManualIsolationFile(t, root, "Makefile", `ci: aggregate
aggregate: live-smoke
live-smoke: ## Manual smoke: requires real services
`)
	writeManualIsolationFile(t, root, "docs/design/ci/manual-targets.tsv", "target\treason\nlive-smoke\trequires real services\n")
	writeManualIsolationFile(t, root, "docs/design/ci/README.md", "`make live-smoke`\n")
	writeManualIsolationFile(t, root, ".github/workflows/ci.yml", "jobs: {}\n")

	err := runManualIsolationCheck(t, root)
	if err == nil || !strings.Contains(err.Error(), `make ci must not reach manual target "live-smoke"`) {
		t.Fatalf("run error = %v, want ci reachability failure", err)
	}
}

func TestRunRejectsManualTargetReachableFromWorkflow(t *testing.T) {
	root := t.TempDir()
	writeManualIsolationFile(t, root, "Makefile", `ci: docs
docs: ## Check docs
aggregate: live-smoke
live-smoke: ## Manual smoke: requires real services
`)
	writeManualIsolationFile(t, root, "docs/design/ci/manual-targets.tsv", "target\treason\nlive-smoke\trequires real services\n")
	writeManualIsolationFile(t, root, "docs/design/ci/README.md", "`make live-smoke`\n")
	writeManualIsolationFile(t, root, ".github/workflows/ci.yml", "jobs:\n  bad:\n    steps:\n      - run: make aggregate\n")

	err := runManualIsolationCheck(t, root)
	if err == nil || !strings.Contains(err.Error(), "workflow run `make aggregate` must not reach manual target") {
		t.Fatalf("run error = %v, want workflow reachability failure", err)
	}
}

func TestRunRejectsUnregisteredManualHelpTarget(t *testing.T) {
	root := t.TempDir()
	writeManualIsolationFile(t, root, "Makefile", `ci: docs
docs: ## Check docs
other-smoke: ## Manual smoke: requires real services
`)
	writeManualIsolationFile(t, root, "docs/design/ci/manual-targets.tsv", "target\treason\n")
	writeManualIsolationFile(t, root, "docs/design/ci/README.md", "`make other-smoke`\n")
	writeManualIsolationFile(t, root, ".github/workflows/ci.yml", "jobs: {}\n")

	err := runManualIsolationCheck(t, root)
	if err == nil || !strings.Contains(err.Error(), "no manual targets registered") {
		t.Fatalf("run error = %v, want empty policy failure", err)
	}
}

func TestRunRejectsPolicyTargetWithoutManualHelpText(t *testing.T) {
	root := t.TempDir()
	writeManualIsolationFile(t, root, "Makefile", `ci: docs
docs: ## Check docs
live-smoke: ## Smoke requiring real services
`)
	writeManualIsolationFile(t, root, "docs/design/ci/manual-targets.tsv", "target\treason\nlive-smoke\trequires real services\n")
	writeManualIsolationFile(t, root, "docs/design/ci/README.md", "`make live-smoke`\n")
	writeManualIsolationFile(t, root, ".github/workflows/ci.yml", "jobs: {}\n")

	err := runManualIsolationCheck(t, root)
	if err == nil || !strings.Contains(err.Error(), `target "live-smoke" help text must start with Manual`) {
		t.Fatalf("run error = %v, want help text failure", err)
	}
}

func TestReadPolicyRejectsMalformedRows(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "policy.tsv")
	if err := os.WriteFile(path, []byte("target\treason\n live-smoke\treason\n"), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	_, err := readPolicy(path)
	if err == nil || !strings.Contains(err.Error(), "must not have leading or trailing whitespace") {
		t.Fatalf("readPolicy error = %v, want whitespace failure", err)
	}
}

func runManualIsolationCheck(t *testing.T, root string) error {
	t.Helper()
	var stdout bytes.Buffer
	return run(config{
		MakefilePath: filepath.Join(root, "Makefile"),
		PolicyPath:   filepath.Join(root, "docs/design/ci/manual-targets.tsv"),
		CIReadmePath: filepath.Join(root, "docs/design/ci/README.md"),
		WorkflowsDir: filepath.Join(root, ".github/workflows"),
	}, &stdout)
}

func writeManualIsolationFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
