package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsJSONObject(t *testing.T) {
	path := writeOutput(t, `{"summary":"ok","findings":[]}`)

	var stdout bytes.Buffer
	if err := run([]string{path}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Fatalf("stdout = %q, want OK", stdout.String())
	}
}

func TestRunRejectsSymlinkOutput(t *testing.T) {
	target := writeOutput(t, `{"summary":"ok"}`)
	link := filepath.Join(t.TempDir(), "output-link.json")
	createSymlinkOrSkip(t, target, link)

	var stdout bytes.Buffer
	err := run([]string{link}, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want symlink rejection", err)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty on symlink rejection", stdout.String())
	}
}

func TestRunRejectsInvalidOutputs(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "empty", content: "", wantErr: "empty"},
		{name: "invalid JSON", content: "{", wantErr: "invalid JSON"},
		{name: "array", content: `[]`, wantErr: "JSON object"},
		{name: "empty object", content: `{}`, wantErr: "non-empty JSON object"},
		{name: "duplicate key", content: `{"summary":"stale","summary":"ok"}`, wantErr: "duplicate object key"},
		{name: "trailing JSON", content: `{"summary":"ok"} {"summary":"other"}`, wantErr: "trailing JSON values"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeOutput(t, tt.content)
			var stdout bytes.Buffer
			err := run([]string{path}, &stdout)
			if err == nil {
				t.Fatal("run err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunRejectsWrongUsage(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want usage error")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Fatalf("run err = %v, want usage", err)
	}
}

func writeOutput(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}
