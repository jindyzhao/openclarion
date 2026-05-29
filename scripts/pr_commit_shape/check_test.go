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

func TestRunRejectsMergeCommits(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": "origin/main\n",
		"log\x00--merges\x00--format=%H%x00%s\x00@{u}..HEAD": strings.Join([]string{
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00Merge branch 'main' into feature",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00Merge pull request #123",
		}, "\n"),
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(nil), git, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa Merge pull request #123") {
		t.Fatalf("stderr = %q, want sorted merge commit details", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Rebase the branch") {
		t.Fatalf("stderr = %q, want remediation hint", stderr.String())
	}
}

func TestRunAcceptsLinearHistory(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": "",
		"log\x00--merges\x00--format=%H%x00%s\x00@{u}..HEAD":        "",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(nil), git, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "OK (0 merge commits)") {
		t.Fatalf("stderr = %q, want success message", stderr.String())
	}
}

func TestCommitRangeUsesPRRangeAndFetchesMissingBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"fetch\x00--no-tags\x00--prune\x00origin\x00main:main": "",
		},
		errors: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing ref"),
		},
	}

	got, err := commitRange(context.Background(), mapEnv(map[string]string{
		baseRefEnv: "main",
		headSHAEnv: "abc123",
	}), git)

	if err != nil {
		t.Fatalf("commitRange: %v", err)
	}
	if got != "main..abc123" {
		t.Fatalf("range = %q, want main..abc123", got)
	}
	if len(git.calls) != 2 {
		t.Fatalf("git calls = %#v, want rev-parse and fetch", git.calls)
	}
}

func TestCommitRangeRequiresPRRangePair(t *testing.T) {
	_, err := commitRange(context.Background(), mapEnv(map[string]string{
		baseRefEnv: "main",
	}), &fakeGit{})
	if err == nil {
		t.Fatal("commitRange err = nil, want env pair error")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("err = %v, want env pair message", err)
	}
}

func TestCommitRangeFallsBackToHeadWithoutUpstream(t *testing.T) {
	git := &fakeGit{
		errors: map[string]error{
			"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": errors.New("no upstream"),
		},
	}

	got, err := commitRange(context.Background(), mapEnv(nil), git)

	if err != nil {
		t.Fatalf("commitRange: %v", err)
	}
	if got != "HEAD^!" {
		t.Fatalf("range = %q, want HEAD^!", got)
	}
}

func TestParseMergeCommitsNormalizesDedupesAndSorts(t *testing.T) {
	got, err := parseMergeCommits(strings.Join([]string{
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00 Merge branch 'main' ",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x00",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00duplicate subject wins",
		"",
	}, "\n"))
	if err != nil {
		t.Fatalf("parseMergeCommits: %v", err)
	}

	want := []mergeCommit{
		{SHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Subject: "(no subject)"},
		{SHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Subject: "duplicate subject wins"},
	}
	if len(got) != len(want) {
		t.Fatalf("commits = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commits[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseMergeCommitsRejectsMalformedGitLogOutput(t *testing.T) {
	_, err := parseMergeCommits("not-a-valid-record\n")
	if err == nil {
		t.Fatal("parseMergeCommits err = nil, want malformed output error")
	}
	if !strings.Contains(err.Error(), "missing NUL separator") {
		t.Fatalf("err = %v, want separator message", err)
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
