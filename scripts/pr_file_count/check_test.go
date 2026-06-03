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

func TestRunAcceptsAtLimitFromEvent(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": "event.json"}), readFiles(map[string]string{
		"event.json": `{"pull_request":{"changed_files":50,"labels":[]}}`,
	}), &fakeGit{}, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "50 changed files, limit 50") {
		t.Fatalf("stderr = %q, want count summary", stderr.String())
	}
}

func TestRunRejectsOverLimitWithoutLabel(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": "event.json"}), readFiles(map[string]string{
		"event.json": `{"pull_request":{"changed_files":51,"labels":[{"name":"needs-review"}]}}`,
	}), &fakeGit{}, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "exceeding the limit of 50") {
		t.Fatalf("stderr = %q, want limit failure", stderr.String())
	}
	if !strings.Contains(stderr.String(), `large-pr-approved`) {
		t.Fatalf("stderr = %q, want override label", stderr.String())
	}
}

func TestRunAcceptsOverLimitWithOverrideLabel(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": "event.json"}), readFiles(map[string]string{
		"event.json": `{"pull_request":{"changed_files":75,"labels":[{"name":"Large-PR-Approved"}]}}`,
	}), &fakeGit{}, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "override label") {
		t.Fatalf("stderr = %q, want override acceptance", stderr.String())
	}
}

func TestRunUsesEnvCountLabelsAndMax(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), mapEnv(map[string]string{
		"PR_FILE_COUNT_CHANGED_FILES": "4",
		"PR_FILE_COUNT_LABELS":        "large-pr-approved, needs-review",
		"PR_FILE_COUNT_MAX":           "3",
	}), readFiles(nil), &fakeGit{}, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "4 changed files exceeds limit 3") {
		t.Fatalf("stderr = %q, want env count override", stderr.String())
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "invalid max",
			env:  map[string]string{"PR_FILE_COUNT_MAX": "0"},
			want: "PR_FILE_COUNT_MAX must be a positive integer",
		},
		{
			name: "blank override label",
			env:  map[string]string{"PR_FILE_COUNT_OVERRIDE_LABEL": " \t "},
			want: "PR_FILE_COUNT_OVERRIDE_LABEL must not be blank",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			code := run(context.Background(), mapEnv(tt.env), readFiles(nil), &fakeGit{}, &stderr)
			if code != 2 {
				t.Fatalf("run code = %d, want 2; stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestLoadPRFactsRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		body string
		want string
	}{
		{
			name: "negative env count",
			env:  map[string]string{"PR_FILE_COUNT_CHANGED_FILES": "-1"},
			want: "PR_FILE_COUNT_CHANGED_FILES must be a non-negative integer",
		},
		{
			name: "invalid event json",
			env:  map[string]string{"GITHUB_EVENT_PATH": "event.json"},
			body: `{`,
			want: "invalid GITHUB_EVENT_PATH JSON",
		},
		{
			name: "negative event count",
			env:  map[string]string{"GITHUB_EVENT_PATH": "event.json"},
			body: `{"pull_request":{"changed_files":-1}}`,
			want: "pull_request.changed_files must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{}
			if tt.body != "" {
				files["event.json"] = tt.body
			}
			_, err := loadPRFacts(context.Background(), mapEnv(tt.env), readFiles(files), &fakeGit{})
			if err == nil {
				t.Fatal("loadPRFacts err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadPRFactsUsesPRRangeAndFetchesMissingBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"fetch\x00--no-tags\x00--prune\x00origin\x00main:main": "",
			"diff\x00--name-only\x00--diff-filter=ACDMRTUXB\x00main..abc123": strings.Join([]string{
				"Makefile",
				"docs/design/ci/README.md",
				"./Makefile",
			}, "\n"),
		},
		errors: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing ref"),
		},
	}

	facts, err := loadPRFacts(context.Background(), mapEnv(map[string]string{
		"PR_FILE_COUNT_BASE_REF": "main",
		"PR_FILE_COUNT_HEAD_SHA": "abc123",
	}), readFiles(nil), git)

	if err != nil {
		t.Fatalf("loadPRFacts: %v", err)
	}
	if facts.ChangedFiles != 2 {
		t.Fatalf("changed files = %d, want 2", facts.ChangedFiles)
	}
	if len(git.calls) != 3 {
		t.Fatalf("git calls = %#v, want rev-parse, fetch, diff", git.calls)
	}
}

func TestChangedFilesRequiresPRRangePair(t *testing.T) {
	_, err := changedFiles(context.Background(), mapEnv(map[string]string{
		"PR_FILE_COUNT_BASE_REF": "main",
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

func readFiles(files map[string]string) bodyLoader {
	return func(path string) ([]byte, error) {
		body, ok := files[path]
		if !ok {
			return nil, errors.New("missing file")
		}
		return []byte(body), nil
	}
}
