// Command pr_file_count enforces a changed-file count budget for pull requests.
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
	defaultMaxFiles   = 50
	defaultAllowLabel = "allow-large-pr"
	maxListedFiles    = 25
)

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

type fileCountConfig struct {
	MaxFiles   int
	AllowLabel string
}

type fileCountResult struct {
	Files       []string
	AllowedBy   string
	OverBudget  bool
	AllowedOver bool
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, os.ReadFile, realGit{}, os.Stderr))
}

func run(ctx context.Context, getenv func(string) string, readFile func(string) ([]byte, error), git gitRunner, stderr io.Writer) int {
	cfg, err := loadConfig(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-file-count] %v\n", err)
		return 2
	}
	files, err := changedFiles(ctx, getenv, git)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-file-count] %v\n", err)
		return 2
	}
	labels, err := loadPRLabels(getenv, readFile)
	if err != nil {
		fmt.Fprintf(stderr, "[pr-file-count] %v\n", err)
		return 2
	}
	result := evaluateFileCount(files, cfg, labels)
	if !result.OverBudget {
		fmt.Fprintf(stderr, "[pr-file-count] OK (%d changed files, max %d)\n", len(result.Files), cfg.MaxFiles)
		return 0
	}
	if result.AllowedOver {
		fmt.Fprintf(stderr, "[pr-file-count] OK (%d changed files exceeds max %d, allowed by label %q)\n",
			len(result.Files), cfg.MaxFiles, result.AllowedBy)
		return 0
	}

	fmt.Fprintf(stderr, "[pr-file-count] PR changes %d files, exceeding max %d. Split the PR or apply maintainer label %q.\n",
		len(result.Files), cfg.MaxFiles, cfg.AllowLabel)
	for i, path := range result.Files {
		if i >= maxListedFiles {
			fmt.Fprintf(stderr, "  ... %d more files\n", len(result.Files)-maxListedFiles)
			break
		}
		fmt.Fprintf(stderr, "  %s\n", path)
	}
	return 1
}

func loadConfig(getenv func(string) string) (fileCountConfig, error) {
	cfg := fileCountConfig{
		MaxFiles:   defaultMaxFiles,
		AllowLabel: defaultAllowLabel,
	}
	if raw := strings.TrimSpace(getenv("PR_FILE_COUNT_MAX")); raw != "" {
		maxFiles, err := strconv.Atoi(raw)
		if err != nil {
			return fileCountConfig{}, fmt.Errorf("PR_FILE_COUNT_MAX must be an integer: %w", err)
		}
		if maxFiles <= 0 {
			return fileCountConfig{}, fmt.Errorf("PR_FILE_COUNT_MAX must be positive, got %d", maxFiles)
		}
		cfg.MaxFiles = maxFiles
	}
	if raw := strings.TrimSpace(getenv("PR_FILE_COUNT_ALLOW_LABEL")); raw != "" {
		if strings.ContainsAny(raw, "\r\n\t") {
			return fileCountConfig{}, errors.New("PR_FILE_COUNT_ALLOW_LABEL must be a single-line label")
		}
		cfg.AllowLabel = raw
	}
	return cfg, nil
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
	out, err := git.Run(ctx, "diff", "--name-only", "--diff-filter=ACMRTUXB", rangeSpec)
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

func loadPRLabels(getenv func(string) string, readFile func(string) ([]byte, error)) ([]string, error) {
	eventPath := strings.TrimSpace(getenv("GITHUB_EVENT_PATH"))
	if eventPath == "" {
		return nil, nil
	}
	raw, err := readFile(eventPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read GITHUB_EVENT_PATH: %w", err)
	}
	var event struct {
		PullRequest struct {
			Labels []struct {
				Name string `json:"name"`
			} `json:"labels"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("invalid GITHUB_EVENT_PATH JSON: %w", err)
	}
	labels := make([]string, 0, len(event.PullRequest.Labels))
	for _, label := range event.PullRequest.Labels {
		name := strings.TrimSpace(label.Name)
		if name != "" {
			labels = append(labels, name)
		}
	}
	sort.Strings(labels)
	return labels, nil
}

func evaluateFileCount(files []string, cfg fileCountConfig, labels []string) fileCountResult {
	normalized := make([]string, 0, len(files))
	seen := map[string]struct{}{}
	for _, file := range files {
		path := normalizeChangedPath(file)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	result := fileCountResult{
		Files:      normalized,
		OverBudget: len(normalized) > cfg.MaxFiles,
	}
	if !result.OverBudget {
		return result
	}
	for _, label := range labels {
		if label == cfg.AllowLabel {
			result.AllowedBy = label
			result.AllowedOver = true
			return result
		}
	}
	return result
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
