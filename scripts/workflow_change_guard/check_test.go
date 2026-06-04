package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeGit struct {
	outputs map[string]string
	errors  map[string]error
	calls   []string
}

func (g *fakeGit) Run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, "\x00")
	g.calls = append(g.calls, key)
	if err := g.errors[key]; err != nil {
		return "", err
	}
	return g.outputs[key], nil
}

func TestEvaluateWorkflowIsolation(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "no workflow files",
			files: []string{"Makefile", "internal/domain/report.go"},
		},
		{
			name:  "one workflow file with companion gate files",
			files: []string{".github/workflows/ci.yml", "Makefile", "scripts/check.go", "docs/design/ci/README.md"},
			want:  []string{".github/workflows/ci.yml"},
		},
		{
			name:  "duplicate workflow path counts once",
			files: []string{".github/workflows/ci.yml", ".github/workflows/ci.yml"},
			want:  []string{".github/workflows/ci.yml"},
		},
		{
			name:  "multiple workflow files are sorted",
			files: []string{".github/workflows/external-links.yml", ".github/workflows/ci.yml"},
			want:  []string{".github/workflows/ci.yml", ".github/workflows/external-links.yml"},
		},
		{
			name:  "non yaml workflow path ignored",
			files: []string{".github/workflows/README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateWorkflowIsolation(tt.files).WorkflowFiles
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("workflow files = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestChangedFilesRejectsMalformedDiffPaths(t *testing.T) {
	tests := []string{
		"./.github/workflows/ci.yml\n",
		`.\.github\workflows\ci.yml` + "\n",
		"../.github/workflows/ci.yml\n",
		"/tmp/ci.yml\n",
		"C:/.github/workflows/ci.yml\n",
		`".github/workflows/ci.yml"` + "\n",
		".github//workflows/ci.yml\n",
		".github/workflows/ci.yml\t\n",
	}
	for _, output := range tests {
		t.Run(output, func(t *testing.T) {
			git := &fakeGit{outputs: map[string]string{
				"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
				"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": output,
			}}

			_, err := changedFiles(context.Background(), mapEnv(nil), git)
			if err == nil {
				t.Fatal("changedFiles err = nil, want malformed path rejection")
			}
			if !strings.Contains(err.Error(), "invalid changed file path") {
				t.Fatalf("err = %v, want invalid changed file path", err)
			}
		})
	}
}

func TestRunFailsWhenMultipleWorkflowFilesChanged(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": strings.Join([]string{
			".github/workflows/ci.yml",
			".github/workflows/external-links.yml",
			"Makefile",
		}, "\n"),
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(nil), git, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "workflow file changes must be isolated") {
		t.Fatalf("stderr = %q, want isolation failure", stderr.String())
	}
}

func TestRunAcceptsOneWorkflowFileChanged(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": ".github/workflows/ci.yml\nMakefile\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(nil), git, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "changed workflow files: 1") {
		t.Fatalf("stderr = %q, want changed workflow count", stderr.String())
	}
}

func TestChangedFilesUsesPRRangeAndFetchesMissingBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"fetch\x00--no-tags\x00--prune\x00origin\x00main:main":          "",
			"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00main..abc123": ".github/workflows/ci.yml\n",
		},
		errors: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing ref"),
		},
	}

	files, err := changedFiles(context.Background(), mapEnv(map[string]string{
		"WORKFLOW_CHANGE_BASE_REF": "main",
		"WORKFLOW_CHANGE_HEAD_SHA": "abc123",
	}), git)

	if err != nil {
		t.Fatalf("changedFiles: %v", err)
	}
	if strings.Join(files, "\n") != ".github/workflows/ci.yml" {
		t.Fatalf("files = %#v", files)
	}
	if len(git.calls) != 3 {
		t.Fatalf("git calls = %#v, want rev-parse, fetch, diff", git.calls)
	}
}

func TestChangedFilesRequiresPRRangePair(t *testing.T) {
	_, err := changedFiles(context.Background(), mapEnv(map[string]string{
		"WORKFLOW_CHANGE_BASE_REF": "main",
	}), &fakeGit{})
	if err == nil {
		t.Fatal("changedFiles err = nil, want env pair error")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("err = %v, want env pair message", err)
	}
}

func TestChangedFilesFallsBackToHeadWithoutUpstream(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"diff-tree\x00--no-commit-id\x00--name-only\x00-r\x00HEAD": "Makefile\n",
		},
		errors: map[string]error{
			"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": errors.New("no upstream"),
		},
	}

	files, err := changedFiles(context.Background(), mapEnv(nil), git)

	if err != nil {
		t.Fatalf("changedFiles: %v", err)
	}
	if strings.Join(files, "\n") != "Makefile" {
		t.Fatalf("files = %#v", files)
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
