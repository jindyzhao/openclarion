// Command file_mode_check validates tracked Git file modes.
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
	"sort"
	"strings"
)

const defaultRepoRoot = "."

type config struct {
	RepoRoot string
}

type indexEntry struct {
	Mode string
	Path string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.RepoRoot, "repo-root", defaultRepoRoot, "repository root")
	flag.Parse()

	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[file-mode] %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	raw, err := gitLSFilesStage(ctx, cfg.RepoRoot)
	if err != nil {
		return err
	}
	entries, err := parseStageOutput(raw)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("git index has no tracked files")
	}
	if problems := checkEntries(entries); len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	fmt.Fprintf(out, "[file-mode] OK (%d tracked files checked)\n", len(entries))
	return nil
}

func gitLSFilesStage(ctx context.Context, repoRoot string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-s", "-z")
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
	return out, nil
}

func parseStageOutput(raw []byte) ([]indexEntry, error) {
	records := bytes.Split(raw, []byte{0})
	entries := make([]indexEntry, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		header, pathBytes, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("malformed git ls-files record %q: missing tab before path", record)
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed git ls-files record header %q", header)
		}
		path := string(pathBytes)
		if path == "" {
			return nil, fmt.Errorf("malformed git ls-files record %q: empty path", record)
		}
		entries = append(entries, indexEntry{
			Mode: fields[0],
			Path: path,
		})
	}
	return entries, nil
}

func checkEntries(entries []indexEntry) []string {
	var problems []string
	for _, entry := range entries {
		switch entry.Mode {
		case "100644":
			continue
		case "100755":
			if !allowedExecutable(entry.Path) {
				problems = append(problems, fmt.Sprintf("%s has executable bit; only scripts/*.sh may be tracked as 100755", entry.Path))
			}
		case "120000":
			problems = append(problems, fmt.Sprintf("%s is a tracked symlink; commit a regular file or document a reviewed exception first", entry.Path))
		case "160000":
			problems = append(problems, fmt.Sprintf("%s is a git submodule; vendor through an explicit dependency policy instead", entry.Path))
		default:
			problems = append(problems, fmt.Sprintf("%s has unsupported git mode %s", entry.Path, entry.Mode))
		}
	}
	return problems
}

func allowedExecutable(path string) bool {
	rest, ok := strings.CutPrefix(path, "scripts/")
	return ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".sh")
}
