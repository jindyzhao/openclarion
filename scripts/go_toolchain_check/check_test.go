package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsAlignedToolchainDeclarations(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
		".github/workflows/external-links.yml": "name: External links\njobs: {}\n",
	})

	var out bytes.Buffer
	if err := run(root, &out); err != nil {
		t.Fatalf("run() error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "[go-toolchain-check] OK (2 go.mod files, 1 setup-go steps)") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunRejectsSubmoduleGoDirectiveDrift(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.9\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `tools/openclarion-linter/go.mod: go directive "1.25.9" must match root go.mod "1.25.10"`)
}

func TestRunRejectsFloatingPatchlessRootGoDirective(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `go.mod: go directive "1.25" must include an explicit patch version`)
}

func TestRunRejectsGolangCILanguageDrift(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.24\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.golangci.yml: run.go "1.24" must match root Go language version "1.25"`)
}

func TestRunRejectsGolangCIPatchVersion(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25.10\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.golangci.yml: run.go "1.25.10" must match root Go language version "1.25"`)
}

func TestRunRejectsHardCodedSetupGoVersion(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version: "1.25.10"
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), "actions/setup-go must not use hard-coded go-version")
	assertOutputContains(t, out.String(), "actions/setup-go must set go-version-file: go.mod")
}

func TestFindGoModFilesSkipsGeneratedAndDependencyDirs(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                    "module example.test/root\n\ngo 1.25.10\n",
		"bin/tool/go.mod":           "module example.test/bin\n\ngo 1.24.0\n",
		"web/node_modules/x/go.mod": "module example.test/node\n\ngo 1.24.0\n",
		".git/hooks/go.mod":         "module example.test/git\n\ngo 1.24.0\n",
	})

	paths, err := findGoModFiles(root)
	if err != nil {
		t.Fatalf("findGoModFiles() error = %v", err)
	}
	if got, want := strings.Join(paths, ","), "go.mod"; got != want {
		t.Fatalf("findGoModFiles() = %q, want %q", got, want)
	}
}

func TestLanguageVersionUsesGoLanguageVersion(t *testing.T) {
	got, err := languageVersion("1.25.10")
	if err != nil {
		t.Fatalf("languageVersion() error = %v", err)
	}
	if got != "1.25" {
		t.Fatalf("languageVersion() = %q, want %q", got, "1.25")
	}
}

type repoFiles map[string]string

func writeRepo(t *testing.T, files repoFiles) string {
	t.Helper()
	root := t.TempDir()
	for path, contents := range files {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatalf("MkdirAll(%s): %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(contents), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", abs, err)
		}
	}
	return root
}

func workflowWithSetupGo(withBlock string) string {
	return `name: CI
jobs:
  go-checks:
    steps:
      - uses: actions/checkout@abc123
      - uses: actions/setup-go@def456
` + withBlock
}

func assertOutputContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("output missing %q:\n%s", want, output)
	}
}
