// Command shell_syntax_check validates tracked repository shell scripts with
// bash's parser.
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
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxCandidateFirstLineBytes = 4096

type config struct {
	Root     string
	BashPath string
	Timeout  time.Duration
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Root, "root", ".", "repository root")
	flag.StringVar(&cfg.BashPath, "bash", "bash", "bash executable path")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "overall syntax-check timeout")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[shell-syntax] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.Timeout <= 0 {
		return errors.New("--timeout must be greater than zero")
	}
	if strings.TrimSpace(cfg.BashPath) == "" {
		return errors.New("--bash must not be empty")
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	scripts, err := discoverShellScripts(root)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	var failures []string
	for _, script := range scripts {
		if out, err := bashSyntaxCheck(ctx, cfg.BashPath, filepath.Join(root, filepath.FromSlash(script))); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v\n%s", script, err, strings.TrimSpace(string(out))))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("shell syntax failures:\n%s", strings.Join(failures, "\n"))
	}
	fmt.Fprintf(stdout, "[shell-syntax] OK (%d scripts checked)\n", len(scripts))
	return nil
}

func discoverShellScripts(root string) ([]string, error) {
	files, err := trackedFiles(root)
	if err != nil {
		return nil, err
	}
	var scripts []string
	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if strings.HasSuffix(rel, ".sh") {
				return nil, fmt.Errorf("%s: shell scripts must be regular files, not symlinks", rel)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			if strings.HasSuffix(rel, ".sh") {
				return nil, fmt.Errorf("%s: shell scripts must be regular files", rel)
			}
			continue
		}
		isShell, err := shellScriptCandidate(path, rel)
		if err != nil {
			return nil, err
		}
		if isShell {
			scripts = append(scripts, rel)
		}
	}
	sort.Strings(scripts)
	return scripts, nil
}

func trackedFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z") // #nosec G204 -- fixed git invocation for repository file discovery.
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git ls-files failed: %w\n%s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		rel := filepath.ToSlash(string(part))
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return nil, fmt.Errorf("git returned unsafe path %q", rel)
		}
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}

func shellScriptCandidate(path, rel string) (bool, error) {
	if strings.HasSuffix(rel, ".sh") {
		return true, nil
	}
	line, truncated, err := readFirstLine(path)
	if err != nil {
		return false, err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#!") {
		return false, nil
	}
	if truncated {
		return false, fmt.Errorf("%s: shebang line exceeds %d bytes", rel, maxCandidateFirstLineBytes)
	}
	return shellShebangUsesBashOrSh(line), nil
}

func readFirstLine(path string) (string, bool, error) {
	file, err := os.Open(path) // #nosec G304 -- path comes from git ls-files under the requested root.
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maxCandidateFirstLineBytes+1))
	if err != nil {
		return "", false, err
	}
	truncated := len(raw) > maxCandidateFirstLineBytes
	if truncated {
		raw = raw[:maxCandidateFirstLineBytes]
	}
	if idx := bytes.IndexByte(raw, '\n'); idx >= 0 {
		return string(raw[:idx]), false, nil
	}
	return string(raw), truncated, nil
}

func shellShebangUsesBashOrSh(line string) bool {
	fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "#!")))
	if len(fields) == 0 {
		return false
	}
	interpreter := filepath.Base(fields[0])
	if interpreter == "bash" || interpreter == "sh" {
		return true
	}
	if interpreter != "env" {
		return false
	}
	args := fields[1:]
	for len(args) > 0 {
		if args[0] == "-S" {
			args = args[1:]
			break
		}
		if strings.HasPrefix(args[0], "-") {
			args = args[1:]
			continue
		}
		break
	}
	if len(args) == 0 {
		return false
	}
	target := filepath.Base(args[0])
	return target == "bash" || target == "sh"
}

func bashSyntaxCheck(ctx context.Context, bashPath, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bashPath, "-n", script) // #nosec G204 -- this gate intentionally runs the configured bash parser on tracked repository scripts.
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if err != nil {
		return out, fmt.Errorf("bash -n failed: %w", err)
	}
	return out, nil
}
