// Command workflow_change_guard keeps GitHub Actions workflow edits focused.
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

	"github.com/openclarion/openclarion/scripts/internal/changedfiles"
)

const workflowDir = ".github/workflows/"

type gitRunner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

type realGit struct{}

type isolationResult struct {
	WorkflowFiles []string
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, realGit{}, os.Stderr))
}

func run(ctx context.Context, getenv func(string) string, git gitRunner, stderr io.Writer) int {
	files, err := changedFiles(ctx, getenv, git)
	if err != nil {
		fmt.Fprintf(stderr, "[workflow-change-guard] %v\n", err)
		return 2
	}
	result := evaluateWorkflowIsolation(files)
	if len(result.WorkflowFiles) > 1 {
		fmt.Fprintln(stderr, "[workflow-change-guard] workflow file changes must be isolated to one workflow file per PR:")
		for _, path := range result.WorkflowFiles {
			fmt.Fprintf(stderr, "  %s\n", path)
		}
		fmt.Fprintln(stderr, "Split workflow edits so reviewers can evaluate each GitHub Actions workflow independently.")
		return 1
	}
	fmt.Fprintf(stderr, "[workflow-change-guard] OK (changed workflow files: %d)\n", len(result.WorkflowFiles))
	return 0
}

func changedFiles(ctx context.Context, getenv func(string) string, git gitRunner) ([]string, error) {
	baseRef := strings.TrimSpace(getenv("WORKFLOW_CHANGE_BASE_REF"))
	headSHA := strings.TrimSpace(getenv("WORKFLOW_CHANGE_HEAD_SHA"))
	if baseRef != "" || headSHA != "" {
		if baseRef == "" || headSHA == "" {
			return nil, errors.New("WORKFLOW_CHANGE_BASE_REF and WORKFLOW_CHANGE_HEAD_SHA must be set together")
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

func evaluateWorkflowIsolation(files []string) isolationResult {
	seen := map[string]struct{}{}
	for _, file := range files {
		if !isWorkflowFile(file) {
			continue
		}
		seen[file] = struct{}{}
	}
	workflowFiles := make([]string, 0, len(seen))
	for path := range seen {
		workflowFiles = append(workflowFiles, path)
	}
	sort.Strings(workflowFiles)
	return isolationResult{WorkflowFiles: workflowFiles}
}

func isWorkflowFile(path string) bool {
	if !strings.HasPrefix(path, workflowDir) {
		return false
	}
	if strings.TrimPrefix(path, workflowDir) == "" {
		return false
	}
	return strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")
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
