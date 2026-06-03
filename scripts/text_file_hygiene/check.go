// Command text_file_hygiene validates encoding and line-ending hygiene for
// tracked repository text files.
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
	"sort"
	"strings"
	"unicode/utf8"
)

const defaultRepoRoot = "."

var textExtensions = map[string]struct{}{
	".css":   {},
	".go":    {},
	".js":    {},
	".json":  {},
	".jsonc": {},
	".lock":  {},
	".md":    {},
	".mjs":   {},
	".mod":   {},
	".sh":    {},
	".sql":   {},
	".sum":   {},
	".toml":  {},
	".ts":    {},
	".tsv":   {},
	".tsx":   {},
	".txt":   {},
	".yaml":  {},
	".yml":   {},
}

var textBasenames = map[string]struct{}{
	".custom-gcl.yml": {},
	".gitignore":      {},
	".gitleaks.toml":  {},
	".golangci.yml":   {},
	"Dockerfile":      {},
	"LICENSE":         {},
	"Makefile":        {},
	"OWNERS":          {},
}

type config struct {
	RepoRoot string
}

type fileIssue struct {
	Path    string
	Message string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.RepoRoot, "repo-root", defaultRepoRoot, "repository root")
	flag.Parse()

	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[text-file-hygiene] %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	files, err := trackedFiles(ctx, cfg.RepoRoot)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("git index has no tracked files")
	}

	var checked int
	var issues []fileIssue
	for _, file := range files {
		if !isTextPolicyFile(file) {
			continue
		}
		checked++
		data, err := readTrackedFile(cfg.RepoRoot, file)
		if err != nil {
			issues = append(issues, fileIssue{Path: file, Message: err.Error()})
			continue
		}
		issues = append(issues, inspectTextFile(file, data)...)
	}
	if checked == 0 {
		return fmt.Errorf("no tracked text-policy files found")
	}
	if len(issues) > 0 {
		sort.Slice(issues, func(i, j int) bool {
			if issues[i].Path == issues[j].Path {
				return issues[i].Message < issues[j].Message
			}
			return issues[i].Path < issues[j].Path
		})
		lines := make([]string, 0, len(issues))
		for _, issue := range issues {
			lines = append(lines, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
		}
		return fmt.Errorf("%s", strings.Join(lines, "\n"))
	}

	fmt.Fprintf(out, "[text-file-hygiene] OK (%d tracked text files checked)\n", checked)
	return nil
}

func trackedFiles(ctx context.Context, repoRoot string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return nil, fmt.Errorf("git ls-files failed: %s", stderr)
			}
		}
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}
	return splitNULPaths(out), nil
}

func splitNULPaths(raw []byte) []string {
	records := bytes.Split(raw, []byte{0})
	paths := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		paths = append(paths, string(record))
	}
	return paths
}

func isTextPolicyFile(file string) bool {
	base := path.Base(file)
	if _, ok := textBasenames[base]; ok {
		return true
	}
	_, ok := textExtensions[strings.ToLower(path.Ext(base))]
	return ok
}

func readTrackedFile(repoRoot, file string) ([]byte, error) {
	if file == "" || strings.HasPrefix(file, "/") || strings.Contains(file, "\\") {
		return nil, fmt.Errorf("invalid tracked path")
	}
	clean := path.Clean(file)
	if clean == "." || clean != file || strings.HasPrefix(clean, "../") {
		return nil, fmt.Errorf("invalid tracked path")
	}
	fullPath := repoRoot + string(os.PathSeparator) + file
	data, err := os.ReadFile(fullPath) // #nosec G304 -- path comes from git ls-files in the selected repo.
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	return data, nil
}

func inspectTextFile(file string, data []byte) []fileIssue {
	var issues []fileIssue
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		issues = append(issues, fileIssue{Path: file, Message: "must not start with UTF-8 BOM"})
	}
	if !utf8.Valid(data) {
		issues = append(issues, fileIssue{Path: file, Message: "must be valid UTF-8"})
	}

	reported := map[string]struct{}{}
	addAt := func(key, message string, offset int) {
		if _, ok := reported[key]; ok {
			return
		}
		reported[key] = struct{}{}
		line, column := lineColumn(data, offset)
		issues = append(issues, fileIssue{
			Path:    file,
			Message: fmt.Sprintf("%s at line %d, column %d", message, line, column),
		})
	}

	for i, b := range data {
		switch {
		case b == 0:
			addAt("nul", "must not contain NUL bytes", i)
		case b == '\r':
			if i+1 < len(data) && data[i+1] == '\n' {
				addAt("crlf", "must use LF line endings, not CRLF", i)
			} else {
				addAt("bare-cr", "must not contain bare carriage returns", i)
			}
		case b == '\n' || b == '\t':
			continue
		case b < 0x20 || b == 0x7F:
			addAt(fmt.Sprintf("control-%02x", b), fmt.Sprintf("must not contain control byte 0x%02X", b), i)
		}
	}
	return issues
}

func lineColumn(data []byte, offset int) (int, int) {
	line := 1
	column := 1
	for i := 0; i < len(data) && i < offset; i++ {
		if data[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}
