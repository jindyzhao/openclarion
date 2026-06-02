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

func TestProjectTerminology(t *testing.T) {
	tests := []struct {
		name   string
		files  map[string]string
		wantOK bool
		want   []string
	}{
		{
			name: "canonical product language and architecture aliases are accepted",
			files: map[string]string{
				"README.md":                   "OpenClarion remains an intelligent alert analysis product.\n",
				"docs/design/architecture.md": "Architecture discussions may use SignalEvent, SignalGroup, or CaseGroup as aliases.\n",
				"docs/design/alert-first-signal-extension.md": "SignalEvent and SignalGroup stay architecture aliases only.\n",
			},
			wantOK: true,
			want:   []string{"[terminology] OK"},
		},
		{
			name: "hyphenated product phrase is rejected",
			files: map[string]string{
				"README.md": "OpenClarion remains an intelligent alert-analysis product.\n",
			},
			want: []string{
				"Use \"intelligent alert analysis\" without a hyphen",
			},
		},
		{
			name: "generic platform positioning is rejected",
			files: map[string]string{
				"docs/design/README.md": "OpenClarion is a business signal governance platform.\n",
			},
			want: []string{
				"must remain positioned as intelligent alert analysis",
			},
		},
		{
			name: "signal aliases outside architecture docs are rejected",
			files: map[string]string{
				"README.md": "The public API should expose SignalEvent and SignalGroup.\n",
			},
			want: []string{
				"SignalEvent",
				"architecture aliases only",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newTerminologyRepo(t, tc.files)

			out, err := runTerminologyCheck(t, root)
			if tc.wantOK && err != nil {
				t.Fatalf("terminology check failed: %v\n%s", err, out)
			}
			if !tc.wantOK && err == nil {
				t.Fatalf("terminology check passed unexpectedly:\n%s", out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("terminology output = %q, want substring %q", out, want)
				}
			}
		})
	}
}

func TestProjectTerminologyRejectsSymlinkRulesFile(t *testing.T) {
	root := newTerminologyRepo(t, map[string]string{
		"README.md": "OpenClarion remains an intelligent alert analysis product.\n",
	})
	rules := filepath.Join(root, "docs", "design", "ci", "terminology.tsv")
	target := filepath.Join(root, "docs", "design", "ci", "terminology-target.tsv")
	if err := os.Rename(rules, target); err != nil {
		t.Fatalf("rename terminology rules: %v", err)
	}
	if err := os.Symlink(target, rules); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runTerminologyCheck(t, root)
	if err == nil {
		t.Fatalf("terminology check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/terminology.tsv must be a regular file, not a symlink") {
		t.Fatalf("terminology output = %q, want symlink rules rejection", out)
	}
}

func TestProjectTerminologyRejectsSymlinkRulesParent(t *testing.T) {
	root := newTerminologyRepo(t, map[string]string{
		"README.md": "OpenClarion remains an intelligent alert analysis product.\n",
	})
	ciDir := filepath.Join(root, "docs", "design", "ci")
	targetDir := filepath.Join(root, "docs", "design", "ci-target")
	if err := os.Rename(ciDir, targetDir); err != nil {
		t.Fatalf("rename terminology rules parent: %v", err)
	}
	if err := os.Symlink(targetDir, ciDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	out, err := runTerminologyCheck(t, root)
	if err == nil {
		t.Fatalf("terminology check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{"docs/design/ci/terminology.tsv parent directory docs/design/ci", "must not be a symlink"} {
		if !strings.Contains(out, want) {
			t.Fatalf("terminology output = %q, want %q", out, want)
		}
	}
}

func TestProjectTerminologyRejectsNonDirectoryRulesParent(t *testing.T) {
	root := newTerminologyRepo(t, map[string]string{
		"README.md": "OpenClarion remains an intelligent alert analysis product.\n",
	})
	ciDir := filepath.Join(root, "docs", "design", "ci")
	if err := os.RemoveAll(ciDir); err != nil {
		t.Fatalf("remove terminology rules parent: %v", err)
	}
	if err := os.WriteFile(ciDir, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("write non-directory terminology rules parent: %v", err)
	}

	out, err := runTerminologyCheck(t, root)
	if err == nil {
		t.Fatalf("terminology check passed unexpectedly:\n%s", out)
	}
	for _, want := range []string{"docs/design/ci/terminology.tsv parent path docs/design/ci", "must be a directory"} {
		if !strings.Contains(out, want) {
			t.Fatalf("terminology output = %q, want %q", out, want)
		}
	}
}

func TestProjectTerminologyRejectsNonRegularRulesFile(t *testing.T) {
	root := newTerminologyRepo(t, map[string]string{
		"README.md": "OpenClarion remains an intelligent alert analysis product.\n",
	})
	rules := filepath.Join(root, "docs", "design", "ci", "terminology.tsv")
	if err := os.Remove(rules); err != nil {
		t.Fatalf("remove terminology rules: %v", err)
	}
	if err := os.Mkdir(rules, 0o750); err != nil {
		t.Fatalf("mkdir terminology rules path: %v", err)
	}

	out, err := runTerminologyCheck(t, root)
	if err == nil {
		t.Fatalf("terminology check passed unexpectedly:\n%s", out)
	}
	if !strings.Contains(out, "docs/design/ci/terminology.tsv must be a regular file") {
		t.Fatalf("terminology output = %q, want regular-file rules rejection", out)
	}
}

func newTerminologyRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	terminologyWriteFile(t, root, "scripts/check_project_terminology.sh", terminologyScript(t), 0o750)
	terminologyWriteFile(t, root, "docs/design/ci/terminology.tsv", terminologyRules(t), 0o644)
	for name, body := range files {
		terminologyWriteFile(t, root, name, body, 0o644)
	}
	return root
}

func terminologyScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("check_project_terminology.sh")
	if err != nil {
		t.Fatalf("read terminology script: %v", err)
	}
	return string(raw)
}

func terminologyRules(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "docs", "design", "ci", "terminology.tsv"))
	if err != nil {
		t.Fatalf("read terminology rules: %v", err)
	}
	return string(raw)
}

func terminologyWriteFile(t *testing.T, root, name, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runTerminologyCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "scripts/check_project_terminology.sh")
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	return string(raw), err
}
