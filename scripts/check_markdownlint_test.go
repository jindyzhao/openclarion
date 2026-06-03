package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkdownlintCheckInvokesPinnedLocalBinary(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	markdownlintWriteFile(t, root, "web/node_modules/.bin/markdownlint-cli2", `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PWD/markdownlint-args.txt"
`, 0o750)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err != nil {
		t.Fatalf("markdownlint check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[markdownlint] OK") {
		t.Fatalf("markdownlint output = %q, want OK", out)
	}
	args, err := os.ReadFile(filepath.Join(root, "markdownlint-args.txt")) // #nosec G304 -- test reads a controlled temp fixture.
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	if got, want := string(args), "--config\ndocs/design/ci/markdownlint/.markdownlint-cli2.jsonc\n"; got != want {
		t.Fatalf("captured args = %q, want %q", got, want)
	}
}

func TestMarkdownlintCheckRejectsMissingLocalBinary(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err == nil {
		t.Fatalf("markdownlint check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{
		"missing web/node_modules/.bin/markdownlint-cli2",
		"make frontend-install",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdownlint output = %q, want substring %q", out, want)
		}
	}
}

func TestMarkdownlintCheckRejectsSymlinkConfig(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	config := markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	target := filepath.Join(filepath.Dir(config), "markdownlint-target.jsonc")
	if err := os.Rename(config, target); err != nil {
		t.Fatalf("rename markdownlint config: %v", err)
	}
	if err := os.Symlink(target, config); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	markdownlintWriteFile(t, root, "web/node_modules/.bin/markdownlint-cli2", `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PWD/markdownlint-args.txt"
`, 0o750)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err == nil {
		t.Fatalf("markdownlint check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc must be a regular file, not a symlink") {
		t.Fatalf("markdownlint output = %q, want symlink config rejection", out)
	}
}

func TestMarkdownlintCheckRejectsSymlinkConfigParent(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	parent := filepath.Join(root, "docs", "design", "ci", "markdownlint")
	target := filepath.Join(root, "docs", "design", "ci", "markdownlint-target")
	if err := os.Rename(parent, target); err != nil {
		t.Fatalf("rename markdownlint config parent: %v", err)
	}
	if err := os.Symlink(target, parent); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	markdownlintWriteFile(t, root, "web/node_modules/.bin/markdownlint-cli2", `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PWD/markdownlint-args.txt"
`, 0o750)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err == nil {
		t.Fatalf("markdownlint check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc parent directory docs/design/ci/markdownlint must not be a symlink") {
		t.Fatalf("markdownlint output = %q, want symlink config parent rejection", out)
	}
}

func TestMarkdownlintCheckRejectsNonDirectoryConfigParent(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	parent := filepath.Join(root, "docs", "design", "ci", "markdownlint")
	target := filepath.Join(root, "docs", "design", "ci", "markdownlint-target")
	if err := os.Rename(parent, target); err != nil {
		t.Fatalf("rename markdownlint config parent: %v", err)
	}
	if err := os.WriteFile(parent, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("write non-directory markdownlint config parent: %v", err)
	}
	markdownlintWriteFile(t, root, "web/node_modules/.bin/markdownlint-cli2", `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PWD/markdownlint-args.txt"
`, 0o750)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err == nil {
		t.Fatalf("markdownlint check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc parent directory docs/design/ci/markdownlint must be a directory") {
		t.Fatalf("markdownlint output = %q, want non-directory config parent rejection", out)
	}
}

func TestMarkdownlintCheckRejectsNonRegularConfig(t *testing.T) {
	root := t.TempDir()
	markdownlintWriteFile(t, root, "scripts/check_markdownlint.sh", markdownlintScript(t), 0o750)
	config := markdownlintWriteFile(t, root, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc", `{"config":{"default":false}}`+"\n", 0o644)
	if err := os.Remove(config); err != nil {
		t.Fatalf("remove markdownlint config: %v", err)
	}
	if err := os.Mkdir(config, 0o750); err != nil {
		t.Fatalf("mkdir markdownlint config path: %v", err)
	}
	markdownlintWriteFile(t, root, "web/node_modules/.bin/markdownlint-cli2", `#!/usr/bin/env bash
printf '%s\n' "$@" > "$PWD/markdownlint-args.txt"
`, 0o750)
	markdownlintInitGit(t, root)

	out, err := runMarkdownlintCheck(t, root)
	if err == nil {
		t.Fatalf("markdownlint check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/markdownlint/.markdownlint-cli2.jsonc must be a regular file") {
		t.Fatalf("markdownlint output = %q, want regular-file config rejection", out)
	}
}

func markdownlintScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_markdownlint.sh")
	if err != nil {
		t.Fatalf("read markdownlint script: %v", err)
	}
	return string(raw)
}

func markdownlintWriteFile(t *testing.T, root, name, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func markdownlintInitGit(t *testing.T, root string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "init", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
}

func runMarkdownlintCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_markdownlint.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
