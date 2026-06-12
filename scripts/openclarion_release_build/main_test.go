package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultBuildWritesIgnoredPrivateBinaryAndDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	fakeGo := writeFakeGo(t, root, 0)

	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--go", fakeGo,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mainWithArgs code = %d, stderr = %s", code, stderr.String())
	}

	binary := filepath.Join(root, ".openclarion-private", "release", "openclarion")
	digestFile := binary + ".sha256"
	raw := readFile(t, binary)
	if string(raw) != "#!/bin/sh\necho openclarion\n" {
		t.Fatalf("unexpected binary content: %q", string(raw))
	}
	assertMode(t, binary, 0o750)
	assertMode(t, digestFile, 0o600)
	wantDigest := sha256Hex(raw)
	wantLine := wantDigest + "  openclarion\n"
	if got := string(readFile(t, digestFile)); got != wantLine {
		t.Fatalf("digest file = %q, want %q", got, wantLine)
	}
	if !strings.Contains(stdout.String(), "[openclarion-release-build] OK") {
		t.Fatalf("stdout did not report success: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), wantDigest) {
		t.Fatalf("stdout did not include digest: %s", stdout.String())
	}
}

func TestBuildAllowsExternalOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	external := t.TempDir()
	fakeGo := writeFakeGo(t, root, 0)
	output := filepath.Join(external, "openclarion")

	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--go", fakeGo,
		"--out", output,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mainWithArgs code = %d, stderr = %s", code, stderr.String())
	}
	if len(readFile(t, output)) == 0 {
		t.Fatalf("external output was empty")
	}
	assertMode(t, output, 0o750)
	assertMode(t, output+".sha256", 0o600)
}

func TestBuildAllowsIgnoredRepoOutputRoots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	fakeGo := writeFakeGo(t, root, 0)

	allowed := []string{
		filepath.Join(root, "dist", "openclarion"),
		filepath.Join(root, "bin", "openclarion"),
	}
	for _, output := range allowed {
		t.Run(filepath.ToSlash(output), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := mainWithArgs([]string{
				"--root", root,
				"--go", fakeGo,
				"--out", output,
			}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("mainWithArgs code = %d, stderr = %s", code, stderr.String())
			}
			assertMode(t, output, 0o750)
		})
	}
}

func TestBuildRejectsRepoLocalUnignoredOutputPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	fakeGo := writeFakeGo(t, root, 0)

	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--go", fakeGo,
		"--out", filepath.Join(root, "release", "openclarion"),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("mainWithArgs code = %d, want 1; stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "repository-local release output must live under .openclarion-private/") {
		t.Fatalf("stderr did not explain ignored-output boundary: %s", stderr.String())
	}
}

func TestBuildRejectsDirectoryOutputPath(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--out", filepath.Join(root, ".openclarion-private", "release") + string(os.PathSeparator),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("mainWithArgs code = %d, want 1; stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "must name a file, not a directory") {
		t.Fatalf("stderr did not explain directory rejection: %s", stderr.String())
	}
}

func TestBuildRejectsSymlinkOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, ".openclarion-private", "target")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".openclarion-private", "release", "openclarion")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--go", writeFakeGo(t, root, 0),
		"--out", link,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("mainWithArgs code = %d, want 1; stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "existing output must not be a symlink") {
		t.Fatalf("stderr did not explain symlink rejection: %s", stderr.String())
	}
}

func TestBuildFailureReturnsBoundedLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := t.TempDir()
	fakeGo := writeFakeGo(t, root, 23)

	var stdout, stderr bytes.Buffer
	code := mainWithArgs([]string{
		"--root", root,
		"--go", fakeGo,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("mainWithArgs code = %d, want 1; stdout = %s stderr = %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "go build failed") || !strings.Contains(stderr.String(), "fake go failure") {
		t.Fatalf("stderr did not include bounded build failure: %s", stderr.String())
	}
}

func writeFakeGo(t *testing.T, root string, exitCode int) string {
	t.Helper()
	path := filepath.Join(root, "fake-go")
	template := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "build" ]]; then
  echo "unexpected command: $*" >&2
  exit 21
fi
if [[ "__EXIT__" != "0" ]]; then
  echo "fake go failure" >&2
  exit __EXIT__
fi
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$out" ]]; then
  echo "missing -o" >&2
  exit 22
fi
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\necho openclarion\n' > "$out"
chmod 0755 "$out"
`
	script := strings.ReplaceAll(template, "__EXIT__", strconv.Itoa(exitCode))
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { // #nosec G306 -- fake Go tool must be executable by the test process.
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test helper reads paths created by the test.
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
