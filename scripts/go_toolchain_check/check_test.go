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

func TestRunRejectsDuplicateGolangCIKeys(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n  go: \"1.24\"\n",
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
	assertOutputContains(t, out.String(), `.golangci.yml: invalid YAML: duplicate YAML key "go"`)
}

func TestRunRejectsGolangCIYAMLAnchors(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun: &run_config\n  go: \"1.25\"\n",
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
	assertOutputContains(t, out.String(), `.golangci.yml: invalid YAML: YAML anchors are not allowed`)
}

func TestRunToleratesUnconsumedGolangCIFields(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n  modules-download-mode: readonly\nissues:\n  max-issues-per-linter: 0\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
	})

	var out bytes.Buffer
	if err := run(root, &out); err != nil {
		t.Fatalf("run() error = %v\noutput:\n%s", err, out.String())
	}
	assertOutputContains(t, out.String(), "[go-toolchain-check] OK (2 go.mod files, 1 setup-go steps)")
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

func TestRunRejectsDuplicateWorkflowSetupGoWithKeys(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
          go-version-file: tools/go.mod
`),
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.github/workflows/ci.yml: invalid YAML: duplicate YAML key "go-version-file"`)
}

func TestRunRejectsWorkflowYAMLMergeKeys(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": `name: CI
x-go-with: &go_with
  go-version-file: go.mod
jobs:
  go-checks:
    steps:
      - uses: actions/checkout@abc123
      - uses: actions/setup-go@def456
        with:
          <<: *go_with
`,
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.github/workflows/ci.yml: invalid YAML: YAML anchors are not allowed`)
}

func TestRunRejectsWorkflowInlineYAMLMergeKeys(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": `name: CI
jobs:
  go-checks:
    steps:
      - uses: actions/checkout@abc123
      - uses: actions/setup-go@def456
        with:
          <<: {go-version-file: go.mod}
`,
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.github/workflows/ci.yml: invalid YAML: YAML merge keys are not allowed`)
}

func TestRunToleratesUnconsumedWorkflowFields(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": `name: CI
run-name: ${{ github.actor }} is testing
on: [push]
env:
  GOFLAGS: -mod=mod
jobs:
  go-checks:
    runs-on: ubuntu-latest
    outputs:
      binary: ${{ steps.build.outputs.path }}
    steps:
      - uses: actions/checkout@abc123
      - uses: actions/setup-go@def456
        continue-on-error: ${{ false }}
        with:
          go-version-file: go.mod
      - id: build
        run: echo "path=bin/app" >> "$GITHUB_OUTPUT"
  reused:
    uses: owner/repo/.github/workflows/reusable.yml@main
    with:
      input: value
    secrets: inherit
`,
	})

	var out bytes.Buffer
	if err := run(root, &out); err != nil {
		t.Fatalf("run() error = %v\noutput:\n%s", err, out.String())
	}
	assertOutputContains(t, out.String(), "[go-toolchain-check] OK (2 go.mod files, 1 setup-go steps)")
}

func TestRunRejectsNonRegularToolchainInputs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "root go.mod symlink",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithSymlink(t, root, "go.mod")
			},
			want: `go.mod: must be a regular file, not a symlink`,
		},
		{
			name: "root go.mod directory",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithDirectory(t, root, "go.mod")
			},
			want: `go.mod: must be a regular file`,
		},
		{
			name: "submodule go.mod symlink",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithSymlink(t, root, "tools/openclarion-linter/go.mod")
			},
			want: `tools/openclarion-linter/go.mod: must be a regular file, not a symlink`,
		},
		{
			name: "submodule go.mod directory",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithDirectory(t, root, "tools/openclarion-linter/go.mod")
			},
			want: `tools/openclarion-linter/go.mod: must be a regular file`,
		},
		{
			name: "golangci config symlink",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithSymlink(t, root, ".golangci.yml")
			},
			want: `.golangci.yml: must be a regular file, not a symlink`,
		},
		{
			name: "golangci config directory",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithDirectory(t, root, ".golangci.yml")
			},
			want: `.golangci.yml: must be a regular file`,
		},
		{
			name: "workflow directory symlink",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithSymlink(t, root, ".github/workflows")
			},
			want: `.github/workflows: .github/workflows must be a directory, not a symlink`,
		},
		{
			name: "workflow directory file",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithFile(t, root, ".github/workflows", "not a directory\n")
			},
			want: `.github/workflows: .github/workflows must be a directory`,
		},
		{
			name: "workflow file symlink",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithSymlink(t, root, ".github/workflows/ci.yml")
			},
			want: `.github/workflows: .github/workflows/ci.yml must be a regular file, not a symlink`,
		},
		{
			name: "workflow file directory",
			setup: func(t *testing.T, root string) {
				goToolchainReplaceWithDirectory(t, root, ".github/workflows/ci.yml")
			},
			want: `.github/workflows: .github/workflows/ci.yml must be a regular file`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := writeRepo(t, repoFiles{
				"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
				"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
				".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
				".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`),
			})
			tc.setup(t, root)

			var out bytes.Buffer
			err := run(root, &out)
			if err == nil {
				t.Fatalf("run() error = nil\noutput:\n%s", out.String())
			}
			assertOutputContains(t, out.String(), tc.want)
		})
	}
}

func TestRunRejectsMultiDocumentWorkflowYAML(t *testing.T) {
	root := writeRepo(t, repoFiles{
		"go.mod":                          "module example.test/root\n\ngo 1.25.10\n",
		"tools/openclarion-linter/go.mod": "module example.test/root/tools/openclarion-linter\n\ngo 1.25.10\n",
		".golangci.yml":                   "version: \"2\"\nrun:\n  go: \"1.25\"\n",
		".github/workflows/ci.yml": workflowWithSetupGo(`
        with:
          go-version-file: go.mod
`) + "\n---\nname: shadow\n",
	})

	var out bytes.Buffer
	err := run(root, &out)
	if err == nil {
		t.Fatalf("run() error = nil\noutput:\n%s", out.String())
	}
	assertOutputContains(t, out.String(), `.github/workflows/ci.yml: invalid YAML: multiple YAML documents are not allowed`)
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

func goToolchainReplaceWithSymlink(t *testing.T, root, name string) {
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

func goToolchainReplaceWithDirectory(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.Mkdir(path, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
}

func goToolchainReplaceWithFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
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
