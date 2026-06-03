// Command linear_history_check rejects merge commits in new PR ranges.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

type checkRange struct {
	Spec         string
	SingleCommit string
}

type mergeCommit struct {
	SHA     string
	Subject string
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, realGit{}, os.Stderr))
}

func run(ctx context.Context, getenv func(string) string, git gitRunner, stderr io.Writer) int {
	r, err := resolveRange(ctx, getenv, git)
	if err != nil {
		fmt.Fprintf(stderr, "[linear-history] %v\n", err)
		return 2
	}
	merges, err := mergeCommits(ctx, git, r)
	if err != nil {
		fmt.Fprintf(stderr, "[linear-history] %v\n", err)
		return 2
	}
	if len(merges) == 0 {
		fmt.Fprintf(stderr, "[linear-history] OK (range: %s)\n", r.Description())
		return 0
	}
	fmt.Fprintln(stderr, "[linear-history] merge commits are not allowed in PR ranges:")
	for _, commit := range merges {
		fmt.Fprintf(stderr, "  %s %s\n", commit.SHA, commit.Subject)
	}
	fmt.Fprintln(stderr, "Rebase the branch or use squash merge so review history stays linear.")
	return 1
}

func resolveRange(ctx context.Context, getenv func(string) string, git gitRunner) (checkRange, error) {
	baseRef := strings.TrimSpace(getenv("LINEAR_HISTORY_BASE_REF"))
	headSHA := strings.TrimSpace(getenv("LINEAR_HISTORY_HEAD_SHA"))
	if baseRef != "" || headSHA != "" {
		if baseRef == "" || headSHA == "" {
			return checkRange{}, errors.New("LINEAR_HISTORY_BASE_REF and LINEAR_HISTORY_HEAD_SHA must be set together")
		}
		if err := ensureRef(ctx, git, baseRef); err != nil {
			return checkRange{}, err
		}
		return checkRange{Spec: baseRef + ".." + headSHA}, nil
	}

	if _, err := git.Run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		return checkRange{Spec: "@{u}..HEAD"}, nil
	}
	return checkRange{SingleCommit: "HEAD"}, nil
}

func ensureRef(ctx context.Context, git gitRunner, ref string) error {
	if _, err := git.Run(ctx, "rev-parse", "--verify", ref); err == nil {
		return nil
	}
	if isFullSHA(ref) {
		return fmt.Errorf("base SHA %q is not present; use a full-history checkout before running linear-history-check", ref)
	}
	if _, err := git.Run(ctx, "fetch", "--no-tags", "--prune", "origin", ref+":"+ref); err != nil {
		return fmt.Errorf("base ref %q is not present and fetch failed: %w", ref, err)
	}
	return nil
}

func mergeCommits(ctx context.Context, git gitRunner, r checkRange) ([]mergeCommit, error) {
	if r.SingleCommit != "" {
		return singleMergeCommit(ctx, git, r.SingleCommit)
	}
	out, err := git.Run(ctx, "log", "--merges", "--format=%h%x09%s", r.Spec)
	if err != nil {
		return nil, fmt.Errorf("cannot list merge commits for %q: %w", r.Spec, err)
	}
	return parseMergeLog(out), nil
}

func singleMergeCommit(ctx context.Context, git gitRunner, ref string) ([]mergeCommit, error) {
	parents, err := git.Run(ctx, "show", "-s", "--format=%P", ref)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect parents for %q: %w", ref, err)
	}
	if len(strings.Fields(parents)) < 2 {
		return nil, nil
	}
	out, err := git.Run(ctx, "show", "-s", "--format=%h%x09%s", ref)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect merge commit %q: %w", ref, err)
	}
	return parseMergeLog(out), nil
}

func parseMergeLog(out string) []mergeCommit {
	var commits []mergeCommit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sha, subject, ok := strings.Cut(line, "\t")
		if !ok {
			sha, subject = line, ""
		}
		commits = append(commits, mergeCommit{
			SHA:     strings.TrimSpace(sha),
			Subject: strings.TrimSpace(subject),
		})
	}
	return commits
}

func (r checkRange) Description() string {
	if r.SingleCommit != "" {
		return r.SingleCommit
	}
	return r.Spec
}

func isFullSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, r := range ref {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func (realGit) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- executable is fixed and args are not shell-expanded.
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return string(out), nil
}
