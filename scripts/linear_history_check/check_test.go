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

func TestRunAcceptsLinearPRRange(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--verify\x00main":                        "main\n",
		"log\x00--merges\x00--format=%h%x09%s\x00main..abc123": "",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{
		"LINEAR_HISTORY_BASE_REF": "main",
		"LINEAR_HISTORY_HEAD_SHA": "abc123",
	}), git, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "OK (range: main..abc123)") {
		t.Fatalf("stderr = %q, want range summary", stderr.String())
	}
}

func TestRunRejectsMergeCommitInRange(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--verify\x00main":                        "main\n",
		"log\x00--merges\x00--format=%h%x09%s\x00main..abc123": "deadbee\tMerge branch 'main'\n",
	}}
	var stderr bytes.Buffer

	code := run(context.Background(), mapEnv(map[string]string{
		"LINEAR_HISTORY_BASE_REF": "main",
		"LINEAR_HISTORY_HEAD_SHA": "abc123",
	}), git, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%s", code, stderr.String())
	}
	for _, want := range []string{
		"merge commits are not allowed",
		"deadbee Merge branch 'main'",
		"Rebase the branch or use squash merge",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
}

func TestResolveRangeFetchesMissingNamedBase(t *testing.T) {
	git := &fakeGit{
		outputs: map[string]string{
			"fetch\x00--no-tags\x00--prune\x00origin\x00main:main": "",
		},
		errors: map[string]error{
			"rev-parse\x00--verify\x00main": errors.New("missing"),
		},
	}

	r, err := resolveRange(context.Background(), mapEnv(map[string]string{
		"LINEAR_HISTORY_BASE_REF": "main",
		"LINEAR_HISTORY_HEAD_SHA": "abc123",
	}), git)

	if err != nil {
		t.Fatalf("resolveRange: %v", err)
	}
	if r.Spec != "main..abc123" {
		t.Fatalf("range = %#v, want main..abc123", r)
	}
	if len(git.calls) != 2 {
		t.Fatalf("git calls = %#v, want rev-parse and fetch", git.calls)
	}
}

func TestResolveRangeRejectsMissingBaseSHA(t *testing.T) {
	_, err := resolveRange(context.Background(), mapEnv(map[string]string{
		"LINEAR_HISTORY_BASE_REF": "0123456789abcdef0123456789abcdef01234567",
		"LINEAR_HISTORY_HEAD_SHA": "abc123",
	}), &fakeGit{errors: map[string]error{
		"rev-parse\x00--verify\x000123456789abcdef0123456789abcdef01234567": errors.New("missing"),
	}})

	if err == nil {
		t.Fatal("resolveRange err = nil, want missing SHA error")
	}
	if !strings.Contains(err.Error(), "use a full-history checkout") {
		t.Fatalf("err = %v, want full-history message", err)
	}
}

func TestResolveRangeRequiresEnvPair(t *testing.T) {
	_, err := resolveRange(context.Background(), mapEnv(map[string]string{
		"LINEAR_HISTORY_BASE_REF": "main",
	}), &fakeGit{})
	if err == nil {
		t.Fatal("resolveRange err = nil, want pair error")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("err = %v, want pair message", err)
	}
}

func TestResolveRangeUsesUpstreamFallback(t *testing.T) {
	git := &fakeGit{outputs: map[string]string{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": "origin/main\n",
	}}

	r, err := resolveRange(context.Background(), mapEnv(nil), git)
	if err != nil {
		t.Fatalf("resolveRange: %v", err)
	}
	if r.Spec != "@{u}..HEAD" {
		t.Fatalf("range = %#v, want upstream range", r)
	}
}

func TestResolveRangeFallsBackToHeadWithoutUpstream(t *testing.T) {
	git := &fakeGit{errors: map[string]error{
		"rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{u}": errors.New("no upstream"),
	}}

	r, err := resolveRange(context.Background(), mapEnv(nil), git)
	if err != nil {
		t.Fatalf("resolveRange: %v", err)
	}
	if r.SingleCommit != "HEAD" {
		t.Fatalf("range = %#v, want single HEAD", r)
	}
}

func TestSingleMergeCommitDetection(t *testing.T) {
	tests := []struct {
		name    string
		parents string
		want    int
	}{
		{name: "linear commit", parents: "parent1\n"},
		{name: "root commit", parents: "\n"},
		{name: "merge commit", parents: "parent1 parent2\n", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			git := &fakeGit{outputs: map[string]string{
				"show\x00-s\x00--format=%P\x00HEAD":       tt.parents,
				"show\x00-s\x00--format=%h%x09%s\x00HEAD": "deadbee\tMerge branch 'topic'\n",
			}}
			merges, err := singleMergeCommit(context.Background(), git, "HEAD")
			if err != nil {
				t.Fatalf("singleMergeCommit: %v", err)
			}
			if len(merges) != tt.want {
				t.Fatalf("merge count = %d, want %d", len(merges), tt.want)
			}
		})
	}
}

func TestParseMergeLog(t *testing.T) {
	merges := parseMergeLog("abc123\tMerge one\nbadline\n")
	if len(merges) != 2 {
		t.Fatalf("merge count = %d, want 2", len(merges))
	}
	if merges[0].SHA != "abc123" || merges[0].Subject != "Merge one" {
		t.Fatalf("first merge = %#v", merges[0])
	}
	if merges[1].SHA != "badline" || merges[1].Subject != "" {
		t.Fatalf("second merge = %#v", merges[1])
	}
}

func TestSHAHelpers(t *testing.T) {
	if !isFullSHA("0123456789abcdef0123456789ABCDEF01234567") {
		t.Fatal("isFullSHA rejected valid full SHA")
	}
	if isFullSHA("0123456789abcdef0123456789abcdef0123456g") {
		t.Fatal("isFullSHA accepted non-hex")
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
