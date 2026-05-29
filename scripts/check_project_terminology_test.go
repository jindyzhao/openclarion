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
