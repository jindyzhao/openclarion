package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestEvaluateImpact(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "no high impact paths",
			files: []string{"Makefile", "internal/domain/report.go"},
		},
		{
			name:  "adr path",
			files: []string{"docs/adr/ADR-0013-per-turn-container-invocation.md"},
			want:  []string{"docs/adr/ADR-0013-per-turn-container-invocation.md"},
		},
		{
			name:  "sandbox path",
			files: []string{"internal/sandbox/runtime.go"},
			want:  []string{"internal/sandbox/runtime.go"},
		},
		{
			name:  "deduplicates and normalizes",
			files: []string{`.\docs\adr\ADR-0001-postgresql-single-source.md`, "docs/adr/ADR-0001-postgresql-single-source.md"},
			want:  []string{"docs/adr/ADR-0001-postgresql-single-source.md"},
		},
		{
			name:  "similar path ignored",
			files: []string{"docs/adr-notes/example.md", "internal/sandboxed/readme.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateImpact(tt.files).Paths
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Fatalf("impact paths = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHasImpactReference(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{body: "See #123 for the decision.", want: true},
		{body: "Tracked in https://github.com/openclarion/openclarion/issues/42.", want: true},
		{body: "Supersedes ADR-0013.", want: true},
		{body: "See docs/adr/ADR-0004-temporal-workflow-engine.md.", want: true},
		{body: "No reference here.", want: false},
		{body: "abc#123 should not count inside an identifier.", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			if got := hasImpactReference(tt.body); got != tt.want {
				t.Fatalf("hasImpactReference(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

func TestRunRejectsHighImpactChangeWithoutReference(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "docs/adr/ADR-0013-per-turn-container-invocation.md\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"PR_BODY": "## Risk\n\nLow.\n"}), nilReadFile, git, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "require a PR body reference") {
		t.Fatalf("stderr = %q, want reference failure", stderr.String())
	}
}

func TestRunAcceptsHighImpactChangeWithReference(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "internal/sandbox/runtime.go\nMakefile\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"PR_BODY": "Decision context: ADR-0013."}), nilReadFile, git, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "high-impact paths referenced") {
		t.Fatalf("stderr = %q, want referenced OK", stderr.String())
	}
}

func TestRunSkipsBodyRequirementWithoutHighImpactPaths(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "Makefile\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(nil), nilReadFile, git, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no high-impact paths") {
		t.Fatalf("stderr = %q, want no-impact OK", stderr.String())
	}
}

func TestLoadPRBodyReadsGitHubEvent(t *testing.T) {
	body, err := loadPRBody(mapEnv(map[string]string{"GITHUB_EVENT_PATH": "event.json"}), func(path string) ([]byte, error) {
		if path != "event.json" {
			t.Fatalf("path = %q, want event.json", path)
		}
		return []byte(`{"pull_request":{"body":"See #123."}}`), nil
	})
	if err != nil {
		t.Fatalf("loadPRBody: %v", err)
	}
	if body != "See #123." {
		t.Fatalf("body = %q", body)
	}
}

func TestRunRejectsSymlinkGitHubEventPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target-event.json")
	if err := os.WriteFile(target, []byte(`{"pull_request":{"body":"See #123."}}`), 0o600); err != nil {
		t.Fatalf("write target event: %v", err)
	}
	eventPath := filepath.Join(dir, "event.json")
	if err := os.Symlink(target, eventPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "docs/adr/ADR-0013-per-turn-container-invocation.md\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": eventPath}), readRegularFile, git, &stderr)
	if code != 2 {
		t.Fatalf("run code = %d, want 2; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "event.json must be a regular file, not a symlink") {
		t.Fatalf("stderr = %q, want symlink event rejection", stderr.String())
	}
}

func TestRunRejectsSymlinkParentGitHubEventPath(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "event.json"), []byte(`{"pull_request":{"body":"See #123."}}`), 0o600); err != nil {
		t.Fatalf("write target event: %v", err)
	}
	eventDir := filepath.Join(dir, "event-dir")
	if err := os.Symlink(targetDir, eventDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	eventPath := filepath.Join(eventDir, "event.json")
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "docs/adr/ADR-0013-per-turn-container-invocation.md\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": eventPath}), readRegularFile, git, &stderr)
	if code != 2 {
		t.Fatalf("run code = %d, want 2; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"parent directory", "must not be a symlink"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsNonDirectoryParentGitHubEventPath(t *testing.T) {
	dir := t.TempDir()
	eventParent := filepath.Join(dir, "event-parent")
	if err := os.WriteFile(eventParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write event parent: %v", err)
	}
	eventPath := filepath.Join(eventParent, "event.json")
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "docs/adr/ADR-0013-per-turn-container-invocation.md\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": eventPath}), readRegularFile, git, &stderr)
	if code != 2 {
		t.Fatalf("run code = %d, want 2; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"parent path", "must be a directory"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestRunRejectsNonRegularGitHubEventPath(t *testing.T) {
	dir := t.TempDir()
	eventPath := filepath.Join(dir, "event.json")
	if err := os.MkdirAll(eventPath, 0o750); err != nil {
		t.Fatalf("mkdir event path: %v", err)
	}
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}":   "origin/main\n",
		"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00@{u}..HEAD": "internal/sandbox/runtime.go\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{"GITHUB_EVENT_PATH": eventPath}), readRegularFile, git, &stderr)
	if code != 2 {
		t.Fatalf("run code = %d, want 2; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "event.json must be a regular file") {
		t.Fatalf("stderr = %q, want non-regular event rejection", stderr.String())
	}
}

func TestChangedFilesUsesPRRangeAndFetchesMissingBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"fetch\x00--no-tags\x00--prune\x00origin\x00main:main":          "",
			"diff\x00--name-only\x00--diff-filter=ACMRTUXB\x00main..abc123": "docs/adr/ADR-0013-per-turn-container-invocation.md\n",
		},
		errors: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing ref"),
		},
	}

	files, err := changedFiles(context.Background(), mapEnv(map[string]string{
		"IMPACT_REFERENCE_BASE_REF": "main",
		"IMPACT_REFERENCE_HEAD_SHA": "abc123",
	}), git)

	if err != nil {
		t.Fatalf("changedFiles: %v", err)
	}
	if strings.Join(files, "\n") != "docs/adr/ADR-0013-per-turn-container-invocation.md" {
		t.Fatalf("files = %#v", files)
	}
	if len(git.calls) != 3 {
		t.Fatalf("git calls = %#v, want rev-parse, fetch, diff", git.calls)
	}
}

func TestChangedFilesRequiresPRRangePair(t *testing.T) {
	_, err := changedFiles(context.Background(), mapEnv(map[string]string{
		"IMPACT_REFERENCE_BASE_REF": "main",
	}), &fakeGit{})
	if err == nil {
		t.Fatal("changedFiles err = nil, want env pair error")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("err = %v, want env pair message", err)
	}
}

func nilReadFile(string) ([]byte, error) {
	return nil, errors.New("unexpected read")
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
