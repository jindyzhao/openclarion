// Command pr_file_count keeps pull requests small enough for focused review.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMaxChangedFiles = 50
	defaultOverrideLabel   = "large-pr-approved"
)

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

type fileCountConfig struct {
	MaxChangedFiles int
	OverrideLabel   string
}

type prFacts struct {
	ChangedFiles int
	Labels       []string
	Source       string
}

type eventFacts struct {
	ChangedFiles *int
	Labels       []string
}

type bodyLoader func(string) ([]byte, error)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, os.ReadFile, realGit{}, os.Stderr))
}

func run(ctx context.Context, getenv func(string) string, readFile bodyLoader, git gitRunner, stderr io.Writer) int {
	cfg, err := loadConfig(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-file-count] %v\n", err)
		return 2
	}
	facts, err := loadPRFacts(ctx, getenv, readFile, git)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-file-count] %v\n", err)
		return 2
	}
	if facts.ChangedFiles <= cfg.MaxChangedFiles {
		fmt.Fprintf(stderr, "[pr-file-count] OK (%d changed files, limit %d, source %s)\n",
			facts.ChangedFiles, cfg.MaxChangedFiles, facts.Source)
		return 0
	}
	if hasLabel(facts.Labels, cfg.OverrideLabel) {
		fmt.Fprintf(stderr, "[pr-file-count] OK (%d changed files exceeds limit %d, override label %q present)\n",
			facts.ChangedFiles, cfg.MaxChangedFiles, cfg.OverrideLabel)
		return 0
	}
	fmt.Fprintf(stderr, "[pr-file-count] PR changes %d files, exceeding the limit of %d.\n",
		facts.ChangedFiles, cfg.MaxChangedFiles)
	fmt.Fprintf(stderr, "Split the PR or apply the maintainer override label %q.\n", cfg.OverrideLabel)
	return 1
}

func loadConfig(getenv func(string) string) (fileCountConfig, error) {
	cfg := fileCountConfig{
		MaxChangedFiles: defaultMaxChangedFiles,
		OverrideLabel:   defaultOverrideLabel,
	}
	if raw := strings.TrimSpace(getenv("PR_FILE_COUNT_MAX")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("PR_FILE_COUNT_MAX must be a positive integer, got %q", raw)
		}
		cfg.MaxChangedFiles = n
	}
	if raw := getenv("PR_FILE_COUNT_OVERRIDE_LABEL"); raw != "" {
		label := strings.TrimSpace(raw)
		if label == "" {
			return cfg, errors.New("PR_FILE_COUNT_OVERRIDE_LABEL must not be blank")
		}
		cfg.OverrideLabel = label
	}
	return cfg, nil
}

func loadPRFacts(ctx context.Context, getenv func(string) string, readFile bodyLoader, git gitRunner) (prFacts, error) {
	var labels []string
	if envLabels := splitLabels(getenv("PR_FILE_COUNT_LABELS")); len(envLabels) > 0 {
		labels = append(labels, envLabels...)
	}

	eventPath := strings.TrimSpace(getenv("GITHUB_EVENT_PATH"))
	var fromEvent eventFacts
	if eventPath != "" {
		event, err := readEventFacts(eventPath, readFile)
		if err != nil {
			return prFacts{}, err
		}
		fromEvent = event
		labels = append(labels, event.Labels...)
	}

	if raw := strings.TrimSpace(getenv("PR_FILE_COUNT_CHANGED_FILES")); raw != "" {
		count, err := parseChangedFileCount(raw, "PR_FILE_COUNT_CHANGED_FILES")
		if err != nil {
			return prFacts{}, err
		}
		return prFacts{ChangedFiles: count, Labels: labels, Source: "PR_FILE_COUNT_CHANGED_FILES"}, nil
	}
	if fromEvent.ChangedFiles != nil {
		count := *fromEvent.ChangedFiles
		if count < 0 {
			return prFacts{}, fmt.Errorf("GITHUB_EVENT_PATH pull_request.changed_files must be non-negative, got %d", count)
		}
		return prFacts{ChangedFiles: count, Labels: labels, Source: "GITHUB_EVENT_PATH"}, nil
	}

	files, err := changedFiles(ctx, getenv, git)
	if err != nil {
		return prFacts{}, err
	}
	return prFacts{ChangedFiles: len(files), Labels: labels, Source: "git diff"}, nil
}

func readEventFacts(path string, readFile bodyLoader) (eventFacts, error) {
	raw, err := readFile(path)
	if err != nil {
		return eventFacts{}, fmt.Errorf("cannot read GITHUB_EVENT_PATH: %w", err)
	}
	var event struct {
		PullRequest struct {
			ChangedFiles *int `json:"changed_files"`
			Labels       []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return eventFacts{}, fmt.Errorf("invalid GITHUB_EVENT_PATH JSON: %w", err)
	}
	labels := make([]string, 0, len(event.PullRequest.Labels))
	for _, label := range event.PullRequest.Labels {
		if name := strings.TrimSpace(label.Name); name != "" {
			labels = append(labels, name)
		}
	}
	return eventFacts{ChangedFiles: event.PullRequest.ChangedFiles, Labels: labels}, nil
}

func parseChangedFileCount(raw, source string) (int, error) {
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer, got %q", source, raw)
	}
	return count, nil
}

func changedFiles(ctx context.Context, getenv func(string) string, git gitRunner) ([]string, error) {
	baseRef := strings.TrimSpace(getenv("PR_FILE_COUNT_BASE_REF"))
	headSHA := strings.TrimSpace(getenv("PR_FILE_COUNT_HEAD_SHA"))
	if baseRef != "" || headSHA != "" {
		if baseRef == "" || headSHA == "" {
			return nil, errors.New("PR_FILE_COUNT_BASE_REF and PR_FILE_COUNT_HEAD_SHA must be set together")
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
	return splitChangedFiles(out), nil
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
	out, err := git.Run(ctx, "diff", "--name-only", "--diff-filter=ACDMRTUXB", rangeSpec)
	if err != nil {
		return nil, fmt.Errorf("cannot list changed files for %q: %w", rangeSpec, err)
	}
	return splitChangedFiles(out), nil
}

func splitChangedFiles(out string) []string {
	seen := map[string]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		path := normalizeChangedPath(line)
		if path != "" {
			seen[path] = struct{}{}
		}
	}
	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func normalizeChangedPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	return path
}

func splitLabels(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		if label := strings.TrimSpace(field); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func hasLabel(labels []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label), wanted) {
			return true
		}
	}
	return false
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
