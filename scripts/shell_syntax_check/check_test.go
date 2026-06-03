package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunAcceptsTrackedShellScripts(t *testing.T) {
	root := newShellSyntaxFixture(t)
	writeShellSyntaxFile(t, root, "scripts/ok.sh", "#!/usr/bin/env bash\nset -euo pipefail\necho ok\n")
	writeShellSyntaxFile(t, root, "scripts/no_extension", "#!/bin/sh\nset -eu\necho ok\n")
	writeShellSyntaxFile(t, root, "scripts/env_s", "#!/usr/bin/env -S bash -e\ntrue\n")
	writeShellSyntaxFile(t, root, "scripts/not-shell.txt", "plain text\n")
	gitAddAll(t, root)

	var stdout bytes.Buffer
	if err := run(config{Root: root, BashPath: "bash", Timeout: time.Second}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[shell-syntax] OK (3 scripts checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsShellSyntaxError(t *testing.T) {
	root := newShellSyntaxFixture(t)
	writeShellSyntaxFile(t, root, "scripts/bad.sh", "#!/usr/bin/env bash\nif true; then\necho missing fi\n")
	gitAddAll(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root, BashPath: "bash", Timeout: time.Second}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "scripts/bad.sh") || !strings.Contains(err.Error(), "bash -n failed") {
		t.Fatalf("run error = %v, want failing script path", err)
	}
}

func TestRunRejectsSymlinkShellScript(t *testing.T) {
	root := newShellSyntaxFixture(t)
	writeShellSyntaxFile(t, root, "scripts/target.sh", "#!/usr/bin/env bash\necho ok\n")
	link := filepath.Join(root, "scripts/link.sh")
	if err := os.Symlink("target.sh", link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	gitAddAll(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root, BashPath: "bash", Timeout: time.Second}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "scripts/link.sh") || !strings.Contains(err.Error(), "not symlinks") {
		t.Fatalf("run error = %v, want symlink rejection", err)
	}
}

func TestRunRejectsOverlongShebangLine(t *testing.T) {
	root := newShellSyntaxFixture(t)
	writeShellSyntaxFile(t, root, "scripts/huge", "#!"+strings.Repeat("x", maxCandidateFirstLineBytes+1))
	gitAddAll(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root, BashPath: "bash", Timeout: time.Second}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "scripts/huge") || !strings.Contains(err.Error(), "shebang line exceeds") {
		t.Fatalf("run error = %v, want overlong shebang rejection", err)
	}
}

func TestShellScriptCandidateAvoidsLooseBashSubstring(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fake")
	if err := os.WriteFile(path, []byte("#!/usr/bin/fakebash\necho no\n"), 0o600); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	got, err := shellScriptCandidate(path, "fake")
	if err != nil {
		t.Fatalf("shellScriptCandidate: %v", err)
	}
	if got {
		t.Fatal("fakebash shebang was treated as a shell script")
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(config{Root: t.TempDir(), BashPath: "bash"}, &stdout); err == nil || !strings.Contains(err.Error(), "--timeout") {
		t.Fatalf("run error = %v, want timeout validation", err)
	}
	if err := run(config{Root: t.TempDir(), Timeout: time.Second}, &stdout); err == nil || !strings.Contains(err.Error(), "--bash") {
		t.Fatalf("run error = %v, want bash path validation", err)
	}
}

func newShellSyntaxFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	runShellSyntaxGit(t, root, "init")
	return root
}

func writeShellSyntaxFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func gitAddAll(t *testing.T, root string) {
	t.Helper()
	runShellSyntaxGit(t, root, "add", "-A")
}

func runShellSyntaxGit(t *testing.T, root string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- tests invoke fixed git fixture commands.
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
