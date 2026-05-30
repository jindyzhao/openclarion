// Command pr_impact_reference requires decision references for high-impact PRs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/openclarion/openclarion/scripts/internal/changedfiles"
)

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

type bodyLoader func(string) ([]byte, error)

type impactResult struct {
	Paths []string
}

var referenceRe = regexp.MustCompile(`(?i)(\bADR-[0-9]{4}\b|docs/adr/ADR-[0-9]{4}[-A-Za-z0-9]*\.md|(^|[^A-Za-z0-9_])#[0-9]+|https://github\.com/[^[:space:]/]+/[^[:space:]/]+/(issues|pull)/[0-9]+)`)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, os.ReadFile, realGit{}, os.Stderr))
}

func run(ctx context.Context, getenv func(string) string, readFile bodyLoader, git gitRunner, stderr io.Writer) int {
	files, err := changedFiles(ctx, getenv, git)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-impact-reference] %v\n", err)
		return 2
	}
	result := evaluateImpact(files)
	if len(result.Paths) == 0 {
		fmt.Fprintln(stderr, "[pr-impact-reference] OK (no high-impact paths changed)")
		return 0
	}
	body, err := loadPRBody(getenv, readFile)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-impact-reference] %v\n", err)
		return 2
	}
	if !hasImpactReference(body) {
		fmt.Fprintln(stderr, "[pr-impact-reference] high-impact paths require a PR body reference to an issue or ADR:")
		for _, path := range result.Paths {
			fmt.Fprintf(stderr, "  %s\n", path)
		}
		fmt.Fprintln(stderr, "Add a reference such as `#123`, a GitHub issue URL, or `ADR-0001` to the PR description.")
		return 1
	}
	fmt.Fprintf(stderr, "[pr-impact-reference] OK (%d high-impact paths referenced)\n", len(result.Paths))
	return 0
}

func changedFiles(ctx context.Context, getenv func(string) string, git gitRunner) ([]string, error) {
	baseRef := strings.TrimSpace(getenv("IMPACT_REFERENCE_BASE_REF"))
	headSHA := strings.TrimSpace(getenv("IMPACT_REFERENCE_HEAD_SHA"))
	if baseRef != "" || headSHA != "" {
		if baseRef == "" || headSHA == "" {
			return nil, errors.New("IMPACT_REFERENCE_BASE_REF and IMPACT_REFERENCE_HEAD_SHA must be set together")
		}
		if err := ensureRef(ctx, git, baseRef); err != nil {
			return nil, err
		}
		return diffNameOnly(ctx, git, baseRef+".."+headSHA)
	}

	if _, err := git.Run(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		return diffNameOnly(ctx, git, "@{u}..HEAD")
	}
	out, err := git.Run(ctx, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("cannot determine local changed files: %w", err)
	}
	files, err := splitChangedFiles(out)
	if err != nil {
		return nil, fmt.Errorf("invalid changed file path for HEAD: %w", err)
	}
	return files, nil
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

func diffNameOnly(ctx context.Context, git gitRunner, rangeSpec string) ([]string, error) {
	out, err := git.Run(ctx, "diff", "--name-only", "--diff-filter=ACMRTUXB", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("cannot list changed files for %q: %w", rangeSpec, err)
	}
	files, err := splitChangedFiles(out)
	if err != nil {
		return nil, fmt.Errorf("invalid changed file path for %q: %w", rangeSpec, err)
	}
	return files, nil
}

func splitChangedFiles(out string) ([]string, error) {
	return changedfiles.SplitNameOnlyOutput(out)
}

func evaluateImpact(files []string) impactResult {
	seen := map[string]struct{}{}
	for _, file := range files {
		if isHighImpactPath(file) {
			seen[file] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return impactResult{Paths: paths}
}

func isHighImpactPath(path string) bool {
	return strings.HasPrefix(path, "docs/adr/") || strings.HasPrefix(path, "internal/sandbox/")
}

func loadPRBody(getenv func(string) string, readFile bodyLoader) (string, error) {
	if body := getenv("PR_BODY"); body != "" {
		return body, nil
	}
	eventPath := strings.TrimSpace(getenv("GITHUB_EVENT_PATH"))
	if eventPath == "" {
		return "", errors.New("PR_BODY or GITHUB_EVENT_PATH is required when high-impact paths change")
	}
	raw, err := readFile(eventPath)
	if err != nil {
		return "", fmt.Errorf("cannot read GITHUB_EVENT_PATH: %w", err)
	}
	var event struct {
		PullRequest struct {
			Body string `json:"body"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return "", fmt.Errorf("invalid GITHUB_EVENT_PATH JSON: %w", err)
	}
	return event.PullRequest.Body, nil
}

func hasImpactReference(body string) bool {
	return referenceRe.MatchString(body)
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
