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

func TestRunAcceptsTrackedYAMLFiles(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "a.yml", "name: openclarion\nitems:\n  - one\n")
	writeYAMLSyntaxFile(t, root, "nested/b.yaml", "key: value\nsecond: true\n")
	writeYAMLSyntaxFile(t, root, "ignored.txt", "key: value\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	if err := run(config{Root: root}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[yaml-syntax] OK (2 files checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsInvalidYAML(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "bad.yaml", "key: [unterminated\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "bad.yaml") || !strings.Contains(err.Error(), "did not find expected") {
		t.Fatalf("run error = %v, want parse error with path", err)
	}
}

func TestRunRejectsMultipleYAMLDocuments(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "multi.yaml", "---\nfirst: true\n---\nsecond: true\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "multi.yaml") || !strings.Contains(err.Error(), "multiple YAML documents are not allowed") {
		t.Fatalf("run error = %v, want multi-document rejection", err)
	}
}

func TestRunRejectsDuplicateMappingKeys(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "dup.yml", "outer:\n  key: one\n  key: two\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	for _, want := range []string{"dup.yml", "duplicate key \"key\"", "first defined"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want %q", err, want)
		}
	}
}

func TestRunRejectsYAMLAnchors(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "anchor.yaml", "name: &service openclarion\ncopy: *service\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "anchor.yaml") || !strings.Contains(err.Error(), "YAML anchors are not allowed") {
		t.Fatalf("run error = %v, want anchor rejection", err)
	}
}

func TestRunRejectsYAMLMergeKeys(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "merge.yml", "merged:\n  <<: {key: value}\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "merge.yml") || !strings.Contains(err.Error(), "YAML merge keys are not allowed") {
		t.Fatalf("run error = %v, want merge-key rejection", err)
	}
}

func TestRunRejectsNonScalarMappingKeys(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "complex-key.yaml", "? [a, b]\n: value\n")
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "complex-key.yaml") || !strings.Contains(err.Error(), "must be scalar") {
		t.Fatalf("run error = %v, want non-scalar key rejection", err)
	}
}

func TestRunRejectsSymlinkYAML(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "target.yaml", "key: value\n")
	if err := os.Symlink("target.yaml", filepath.Join(root, "link.yaml")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "link.yaml") || !strings.Contains(err.Error(), "not symlinks") {
		t.Fatalf("run error = %v, want symlink rejection", err)
	}
}

func TestRunRejectsOversizedYAML(t *testing.T) {
	root := newYAMLSyntaxFixture(t)
	writeYAMLSyntaxFile(t, root, "large.yaml", strings.Repeat("a", maxYAMLBytes+1))
	gitAddAllYAML(t, root)

	var stdout bytes.Buffer
	err := run(config{Root: root}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "large.yaml") || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("run error = %v, want size rejection", err)
	}
}

func newYAMLSyntaxFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runYAMLSyntaxGit(t, root, "init")
	return root
}

func writeYAMLSyntaxFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func gitAddAllYAML(t *testing.T, root string) {
	t.Helper()
	runYAMLSyntaxGit(t, root, "add", "-A")
}

func runYAMLSyntaxGit(t *testing.T, root string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- tests invoke fixed git fixture commands.
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
