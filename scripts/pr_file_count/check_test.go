package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type fakeGit struct {
	outputs map[string]string
	errs    map[string]error
	calls   []string
}

func (g *fakeGit) Run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, "\x00")
	g.calls = append(g.calls, key)
	if err := g.errs[key]; err != nil {
		return "", err
	}
	return g.outputs[key], nil
}

func TestEvaluateFileCountWithinBudget(t *testing.T) {
	result := evaluateFileCount([]string{"b.txt", "./a.txt", "b.txt"}, fileCountConfig{MaxFiles: 2, AllowLabel: defaultAllowLabel}, nil)
	if result.OverBudget {
		t.Fatal("OverBudget = true, want false")
	}
	if got := strings.Join(result.Files, ","); got != "a.txt,b.txt" {
		t.Fatalf("Files = %q, want sorted deduplicated paths", got)
	}
}

func TestEvaluateFileCountRejectsOverBudget(t *testing.T) {
	result := evaluateFileCount([]string{"a.txt", "b.txt", "c.txt"}, fileCountConfig{MaxFiles: 2, AllowLabel: defaultAllowLabel}, nil)
	if !result.OverBudget || result.AllowedOver {
		t.Fatalf("result = %+v, want over budget without allow label", result)
	}
}

func TestEvaluateFileCountAllowsMaintainerLabel(t *testing.T) {
	result := evaluateFileCount(
		[]string{"a.txt", "b.txt", "c.txt"},
		fileCountConfig{MaxFiles: 2, AllowLabel: "allow-large-pr"},
		[]string{"docs", "allow-large-pr"},
	)
	if !result.OverBudget || !result.AllowedOver || result.AllowedBy != "allow-large-pr" {
		t.Fatalf("result = %+v, want allowed over budget", result)
	}
}

func TestLoadConfigRejectsInvalidMax(t *testing.T) {
	for _, raw := range []string{"nope", "0", "-1"} {
		t.Run(raw, func(t *testing.T) {
			_, err := loadConfig(env(map[string]string{"PR_FILE_COUNT_MAX": raw}))
			if err == nil {
				t.Fatal("loadConfig() error = nil, want invalid max rejection")
			}
		})
	}
}

func TestLoadConfigRejectsMultilineLabel(t *testing.T) {
	_, err := loadConfig(env(map[string]string{"PR_FILE_COUNT_ALLOW_LABEL": "allow\nlarge"}))
	if err == nil {
		t.Fatal("loadConfig() error = nil, want multiline label rejection")
	}
}

func TestLoadPRLabelsReadsGitHubEvent(t *testing.T) {
	labels, err := loadPRLabels(
		env(map[string]string{"GITHUB_EVENT_PATH": "/tmp/event.json"}),
		func(path string) ([]byte, error) {
			if path != "/tmp/event.json" {
				t.Fatalf("path = %q, want event path", path)
			}
			return []byte(`{"pull_request":{"labels":[{"name":"allow-large-pr"},{"name":"docs"}]}}`), nil
		},
	)
	if err != nil {
		t.Fatalf("loadPRLabels() error = %v", err)
	}
	if got := strings.Join(labels, ","); got != "allow-large-pr,docs" {
		t.Fatalf("labels = %q, want sorted labels", got)
	}
}

func TestLoadPRLabelsRejectsUnreadableEvent(t *testing.T) {
	_, err := loadPRLabels(
		env(map[string]string{"GITHUB_EVENT_PATH": "/tmp/event.json"}),
		func(string) ([]byte, error) { return nil, errors.New("boom") },
	)
	if err == nil {
		t.Fatal("loadPRLabels() error = nil, want read failure")
	}
}

func TestChangedFilesUsesCIEnvRangeAndFetchesMissingBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00main..abc123": "b.txt\n./a.txt\n",
		},
		errs: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing"),
		},
	}
	files, err := changedFiles(context.Background(), env(map[string]string{
		"PR_FILE_COUNT_BASE_REF": "main",
		"PR_FILE_COUNT_HEAD_SHA": "abc123",
	}), git)
	if err != nil {
		t.Fatalf("changedFiles() error = %v", err)
	}
	if got := strings.Join(files, ","); got != "a.txt,b.txt" {
		t.Fatalf("files = %q, want sorted files", got)
	}
	wantFetch := "fetch\x00--no-tags\x00--prune\x00origin\x00main:main"
	if !contains(git.calls, wantFetch) {
		t.Fatalf("git calls = %#v, want fetch %q", git.calls, wantFetch)
	}
}

func TestChangedFilesRejectsPartialCIEnv(t *testing.T) {
	_, err := changedFiles(context.Background(), env(map[string]string{"PR_FILE_COUNT_BASE_REF": "main"}), &fakeGit{})
	if err == nil {
		t.Fatal("changedFiles() error = nil, want env pair rejection")
	}
}

func TestRunFailsWhenOverBudget(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/branch\n",
			"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": strings.Join([]string{"a.txt", "b.txt", "c.txt"}, "\n"),
		},
	}
	var stderr strings.Builder
	code := run(context.Background(), env(map[string]string{"PR_FILE_COUNT_MAX": "2"}), nilReadFile, git, &stderr)
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "exceeding max 2") || !strings.Contains(stderr.String(), "allow-large-pr") {
		t.Fatalf("stderr = %q, want over-budget detail", stderr.String())
	}
}

func TestRunAllowsOverBudgetWithLabel(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/branch\n",
			"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": strings.Join([]string{"a.txt", "b.txt", "c.txt"}, "\n"),
		},
	}
	var stderr strings.Builder
	code := run(
		context.Background(),
		env(map[string]string{"PR_FILE_COUNT_MAX": "2", "GITHUB_EVENT_PATH": "event.json"}),
		func(string) ([]byte, error) {
			return []byte(`{"pull_request":{"labels":[{"name":"allow-large-pr"}]}}`), nil
		},
		git,
		&stderr,
	)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "allowed by label") {
		t.Fatalf("stderr = %q, want allow-label detail", stderr.String())
	}
}

func env(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func nilReadFile(path string) ([]byte, error) {
	return nil, fmt.Errorf("unexpected read %s", path)
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
