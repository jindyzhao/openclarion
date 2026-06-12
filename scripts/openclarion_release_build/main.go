// Command openclarion_release_build builds the OpenClarion service binary into
// a local ignored path or an explicitly external path.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultOutputPath = ".openclarion-private/release/openclarion"
	defaultPackage    = "./cmd/openclarion"
	defaultTimeout    = 5 * time.Minute
	maxBuildLogBytes  = 64 * 1024
)

type config struct {
	Root      string
	Output    string
	SHA256Out string
	Package   string
	GoCommand string
	Timeout   time.Duration
}

func main() {
	os.Exit(mainWithArgs(os.Args[1:], os.Stdout, os.Stderr))
}

func mainWithArgs(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "[openclarion-release-build] %v\n", err)
		return 2
	}
	if err := run(context.Background(), cfg, stdout); err != nil {
		fmt.Fprintf(stderr, "[openclarion-release-build] %v\n", err)
		return 1
	}
	return 0
}

func parseArgs(args []string, stderr io.Writer) (config, error) {
	output := envOrDefault("OPENCLARION_RELEASE_OUT", defaultOutputPath)
	cfg := config{
		Root:      ".",
		Output:    output,
		SHA256Out: os.Getenv("OPENCLARION_RELEASE_SHA256_OUT"),
		Package:   envOrDefault("OPENCLARION_RELEASE_PACKAGE", defaultPackage),
		GoCommand: envOrDefault("OPENCLARION_RELEASE_GO", "go"),
		Timeout:   defaultTimeout,
	}
	fs := flag.NewFlagSet("openclarion-release-build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Root, "root", cfg.Root, "repository root")
	fs.StringVar(&cfg.Output, "out", cfg.Output, "release binary output path")
	fs.StringVar(&cfg.SHA256Out, "sha256-out", cfg.SHA256Out, "release binary sha256 output path")
	fs.StringVar(&cfg.Package, "package", cfg.Package, "Go package to build")
	fs.StringVar(&cfg.GoCommand, "go", cfg.GoCommand, "Go command path")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "maximum go build duration")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	if cfg.SHA256Out == "" {
		cfg.SHA256Out = cfg.Output + ".sha256"
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, stdout io.Writer) error {
	if cfg.Package == "" || strings.ContainsAny(cfg.Package, "\x00\r\n") {
		return fmt.Errorf("package must be a non-empty single-line value")
	}
	if cfg.GoCommand == "" || strings.ContainsAny(cfg.GoCommand, "\x00\r\n") {
		return fmt.Errorf("go command must be a non-empty single-line value")
	}
	if cfg.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	root, err := cleanExistingRoot(cfg.Root)
	if err != nil {
		return err
	}
	output, err := resolveOutputPath(root, cfg.Output, "release binary")
	if err != nil {
		return err
	}
	shaPath, err := resolveOutputPath(root, cfg.SHA256Out, "sha256 file")
	if err != nil {
		return err
	}
	if output == shaPath {
		return fmt.Errorf("release binary output and sha256 output must be different paths")
	}

	if err := prepareOutputParent(output); err != nil {
		return err
	}
	if err := prepareOutputParent(shaPath); err != nil {
		return err
	}

	buildCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	var buildLog cappedBuffer
	buildLog.Limit = maxBuildLogBytes
	cmd := exec.CommandContext(buildCtx, cfg.GoCommand, "build", "-trimpath", "-o", output, cfg.Package) // #nosec G204 -- release helper intentionally invokes the configured Go tool without shell expansion.
	cmd.Dir = root
	cmd.Stdout = &buildLog
	cmd.Stderr = &buildLog
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("go build timed out after %s", cfg.Timeout)
		}
		msg := strings.TrimSpace(buildLog.String())
		if msg == "" {
			return fmt.Errorf("go build failed: %w", err)
		}
		return fmt.Errorf("go build failed: %w: %s", err, msg)
	}

	size, digest, err := finalizeBinaryAndDigest(output, shaPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[openclarion-release-build] OK (%d bytes, sha256=%s)\n", size, digest)
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func cleanExistingRoot(raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return "", fmt.Errorf("repository root must be a non-empty single-line path")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repository root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root must be a directory")
	}
	return filepath.Clean(abs), nil
}

func resolveOutputPath(repoRoot, raw, label string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n") {
		return "", fmt.Errorf("%s output must be a non-empty single-line path", label)
	}
	if raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("%s output path must not have leading or trailing whitespace", label)
	}
	if strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "\\") {
		return "", fmt.Errorf("%s output path must name a file, not a directory", label)
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s output path: %w", label, err)
	}
	abs = filepath.Clean(abs)
	if err := ensureExistingAncestorsAreSafe(abs); err != nil {
		return "", fmt.Errorf("%s output path: %w", label, err)
	}
	if err := validateRepoLocalOutput(repoRoot, abs); err != nil {
		return "", fmt.Errorf("%s output path: %w", label, err)
	}
	if err := rejectExistingIndirectOutput(abs); err != nil {
		return "", fmt.Errorf("%s output path: %w", label, err)
	}
	return abs, nil
}

func validateRepoLocalOutput(repoRoot, output string) error {
	rel, err := filepath.Rel(repoRoot, output)
	if err != nil {
		return err
	}
	if rel == "." {
		return fmt.Errorf("must not point at the repository root")
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return nil
	}
	rel = filepath.ToSlash(rel)
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return fmt.Errorf("must not write inside .git")
	}
	if allowedRepoLocalOutput(rel) {
		return nil
	}
	return fmt.Errorf("repository-local release output must live under .openclarion-private/, dist/, or bin/")
}

func allowedRepoLocalOutput(rel string) bool {
	for _, prefix := range []string{".openclarion-private/", "dist/", "bin/"} {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func ensureExistingAncestorsAreSafe(path string) error {
	dir := filepath.Dir(path)
	for {
		info, err := os.Lstat(dir)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("parent %s must not be a symlink", dir)
			}
			if !info.IsDir() {
				return fmt.Errorf("parent %s must be a directory", dir)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat parent %s: %w", dir, err)
		}
		next := filepath.Dir(dir)
		if next == dir {
			return nil
		}
		dir = next
	}
}

func rejectExistingIndirectOutput(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat existing output: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("existing output must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("existing output must be a regular file")
	}
	return nil
}

func prepareOutputParent(path string) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	return ensureExistingAncestorsAreSafe(path)
}

func finalizeBinaryAndDigest(output, shaPath string) (int64, string, error) {
	info, err := os.Lstat(output)
	if err != nil {
		return 0, "", fmt.Errorf("stat release binary: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, "", fmt.Errorf("release binary must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("release binary must be a regular file")
	}
	if info.Size() <= 0 {
		return 0, "", fmt.Errorf("release binary must not be empty")
	}
	if err := os.Chmod(output, 0o750); err != nil { // #nosec G302 -- release binary must be executable by the service group.
		return 0, "", fmt.Errorf("chmod release binary: %w", err)
	}
	digest, err := fileSHA256(output)
	if err != nil {
		return 0, "", err
	}
	line := fmt.Sprintf("%s  %s\n", digest, filepath.Base(output))
	if err := os.WriteFile(shaPath, []byte(line), 0o600); err != nil {
		return 0, "", fmt.Errorf("write sha256 file: %w", err)
	}
	if err := os.Chmod(shaPath, 0o600); err != nil {
		return 0, "", fmt.Errorf("chmod sha256 file: %w", err)
	}
	return info.Size(), digest, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path has already been validated as the requested output file.
	if err != nil {
		return "", fmt.Errorf("open release binary for hashing: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash release binary: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type cappedBuffer struct {
	bytes.Buffer
	Limit     int
	Truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Limit <= 0 || b.Buffer.Len() >= b.Limit {
		b.Truncated = b.Truncated || len(p) > 0
		return len(p), nil
	}
	remaining := b.Limit - b.Buffer.Len()
	if len(p) > remaining {
		b.Truncated = true
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	text := b.Buffer.String()
	if b.Truncated {
		return text + "\n... build output truncated"
	}
	return text
}
