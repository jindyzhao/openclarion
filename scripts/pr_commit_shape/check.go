// Command pr_commit_shape rejects merge commits in pull request history.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	baseRefEnv = "PR_COMMIT_SHAPE_BASE_REF"
	headSHAEnv = "PR_COMMIT_SHAPE_HEAD_SHA"
)

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

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
	commits, err := mergeCommits(ctx, getenv, git)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-commit-shape] %v\n", err)
		return 2
	}
	if len(commits) > 0 {
		fmt.Fprintln(stderr, "[pr-commit-shape] PR branches must not contain merge commits:")
		for _, commit := range commits {
			fmt.Fprintf(stderr, "  %s %s\n", commit.SHA, commit.Subject)
		}
		fmt.Fprintln(stderr, "Rebase the branch onto the target branch and keep repository merges squash-only.")
		return 1
	}
	fmt.Fprintln(stderr, "[pr-commit-shape] OK (0 merge commits)")
	return 0
}

func mergeCommits(ctx context.Context, getenv func(string) string, git gitRunner) ([]mergeCommit, error) {
	rangeSpec, err := commitRange(ctx, getenv, git)
	if err != nil {
		return nil, err
	}
	return listMergeCommits(ctx, git, rangeSpec)
}

func commitRange(ctx context.Context, getenv func(string) string, git gitRunner) (string, error) {
	baseRef := strings.TrimSpace(getenv(baseRefEnv))
	headSHA := strings.TrimSpace(getenv(headSHAEnv))
	if baseRef != "" || headSHA != "" {
		if baseRef == "" || headSHA == "" {
			return "", fmt.Errorf("%s and %s must be set together", baseRefEnv, headSHAEnv)
		}
		if err := ensureRef(ctx, git, baseRef); err != nil {
			return "", err
		}
		return baseRef + ".." + headSHA, nil
	}

	if _, err := git.Run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		return "@{u}..HEAD", nil
	}
	return "HEAD^!", nil
}

func ensureRef(ctx context.Context, git gitRunner, ref string) error {
	if _, err := git.Run(ctx, "rev-parse", "--verify", ref); err == nil {
		return nil
	}
	if _, err := git.Run(ctx, "fetch", "--no-tags", "--prune", "origin", ref+":"+ref); err != nil {
		return fmt.Errorf("base ref %q is not present and fetch failed: %w", ref, err)
	}
	return nil
}

func listMergeCommits(ctx context.Context, git gitRunner, rangeSpec string) ([]mergeCommit, error) {
	out, err := git.Run(ctx, "log", "--merges", "--format=%H%x00%s", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("cannot list merge commits for %q: %w", rangeSpec, err)
	}
	commits, err := parseMergeCommits(out)
	if err != nil {
		return nil, fmt.Errorf("cannot parse merge commits for %q: %w", rangeSpec, err)
	}
	return commits, nil
}

func parseMergeCommits(out string) ([]mergeCommit, error) {
	seen := map[string]mergeCommit{}
	for _, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		sha, subject, ok := strings.Cut(line, "\x00")
		if !ok {
			return nil, errors.New("git log output missing NUL separator")
		}
		sha = strings.TrimSpace(sha)
		subject = strings.TrimSpace(subject)
		if sha == "" {
			return nil, errors.New("git log output contains empty commit SHA")
		}
		if subject == "" {
			subject = "(no subject)"
		}
		seen[sha] = mergeCommit{SHA: sha, Subject: subject}
	}

	commits := make([]mergeCommit, 0, len(seen))
	for _, commit := range seen {
		commits = append(commits, commit)
	}
	sort.Slice(commits, func(i, j int) bool {
		return commits[i].SHA < commits[j].SHA
	})
	return commits, nil
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
