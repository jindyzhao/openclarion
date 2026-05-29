// Command repo_size_check enforces a simple repository file-size budget.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultMaxFileBytes  int64 = 1 << 20
	defaultMaxTotalBytes int64 = 25 << 20
)

type config struct {
	Root             string
	MaxFileBytes     int64
	MaxTotalBytes    int64
	IncludeUntracked bool
}

type fileRecord struct {
	Path string
	Size int64
}

type sizeSummary struct {
	Files      []fileRecord
	TotalBytes int64
}

func main() {
	os.Exit(mainWithArgs(os.Args[1:], os.Stdout, os.Stderr))
}

func mainWithArgs(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "[repo-size-check] %v\n", err)
		return 2
	}
	if err := run(context.Background(), cfg, stdout); err != nil {
		fmt.Fprintf(stderr, "[repo-size-check] %v\n", err)
		return 1
	}
	return 0
}

func parseArgs(args []string, stderr io.Writer) (config, error) {
	cfg := config{
		Root:             ".",
		MaxFileBytes:     defaultMaxFileBytes,
		MaxTotalBytes:    defaultMaxTotalBytes,
		IncludeUntracked: true,
	}
	fs := flag.NewFlagSet("repo-size-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Root, "root", cfg.Root, "repository root")
	fs.Int64Var(&cfg.MaxFileBytes, "max-file-bytes", cfg.MaxFileBytes, "maximum bytes allowed for one Git-visible file")
	fs.Int64Var(&cfg.MaxTotalBytes, "max-total-bytes", cfg.MaxTotalBytes, "maximum bytes allowed across all Git-visible files")
	fs.BoolVar(&cfg.IncludeUntracked, "include-untracked", cfg.IncludeUntracked, "include non-ignored untracked files")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	paths, err := listGitVisibleFiles(ctx, cfg.Root, cfg.IncludeUntracked)
	if err != nil {
		return err
	}
	summary, err := checkFiles(cfg.Root, paths, cfg.MaxFileBytes, cfg.MaxTotalBytes)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "[repo-size-check] OK (%d files, %d bytes total, max_file_bytes=%d, max_total_bytes=%d)\n",
		len(summary.Files), summary.TotalBytes, cfg.MaxFileBytes, cfg.MaxTotalBytes)
	return nil
}

func listGitVisibleFiles(ctx context.Context, root string, includeUntracked bool) ([]string, error) {
	args := []string{"-C", root, "ls-files", "-z", "--cached"}
	if includeUntracked {
		args = append(args, "--others", "--exclude-standard")
	}
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- executable is fixed and args are not shell-expanded.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("list Git-visible files: %s", msg)
	}
	return splitNULPaths(raw), nil
}

func splitNULPaths(raw []byte) []string {
	parts := bytes.Split(raw, []byte{0})
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		p := string(part)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

func checkFiles(root string, paths []string, maxFileBytes, maxTotalBytes int64) (sizeSummary, error) {
	if maxFileBytes <= 0 {
		return sizeSummary{}, fmt.Errorf("max-file-bytes must be positive, got %d", maxFileBytes)
	}
	if maxTotalBytes <= 0 {
		return sizeSummary{}, fmt.Errorf("max-total-bytes must be positive, got %d", maxTotalBytes)
	}
	root = filepath.Clean(root)
	records := make([]fileRecord, 0, len(paths))
	var total int64
	for _, repoPath := range paths {
		clean, err := cleanRepoPath(repoPath)
		if err != nil {
			return sizeSummary{}, err
		}
		fullPath := filepath.Join(root, filepath.FromSlash(clean))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return sizeSummary{}, fmt.Errorf("%s: stat Git-visible file: %w", clean, err)
		}
		if !info.Mode().IsRegular() {
			return sizeSummary{}, fmt.Errorf("%s: Git-visible path must be a regular file, got mode %s", clean, info.Mode())
		}
		records = append(records, fileRecord{Path: clean, Size: info.Size()})
		total += info.Size()
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Path < records[j].Path
	})

	var problems []string
	var oversized []fileRecord
	for _, record := range records {
		if record.Size > maxFileBytes {
			oversized = append(oversized, record)
		}
	}
	sort.Slice(oversized, func(i, j int) bool {
		if oversized[i].Size != oversized[j].Size {
			return oversized[i].Size > oversized[j].Size
		}
		return oversized[i].Path < oversized[j].Path
	})
	for _, record := range oversized {
		problems = append(problems, fmt.Sprintf("%s is %d bytes, exceeds max-file-bytes %d", record.Path, record.Size, maxFileBytes))
	}
	if total > maxTotalBytes {
		problems = append(problems, fmt.Sprintf("repository total is %d bytes, exceeds max-total-bytes %d", total, maxTotalBytes))
	}
	if len(problems) > 0 {
		return sizeSummary{}, fmt.Errorf("size budget exceeded:\n%s", strings.Join(problems, "\n"))
	}

	return sizeSummary{Files: records, TotalBytes: total}, nil
}

func cleanRepoPath(repoPath string) (string, error) {
	if repoPath == "" {
		return "", errors.New("Git-visible file path is empty")
	}
	if strings.Contains(repoPath, "\x00") {
		return "", fmt.Errorf("%q: path contains NUL", repoPath)
	}
	if path.IsAbs(repoPath) {
		return "", fmt.Errorf("%s: Git-visible path must be relative", repoPath)
	}
	clean := path.Clean(repoPath)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("%s: Git-visible path escapes repository root", repoPath)
	}
	if clean != repoPath {
		return "", fmt.Errorf("%s: Git-visible path must be normalized", repoPath)
	}
	return clean, nil
}
