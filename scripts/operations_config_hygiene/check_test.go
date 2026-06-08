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

func TestSplitNULPaths(t *testing.T) {
	got := splitNULPaths([]byte("api/openapi.yaml\x00web/src/app/page.tsx\x00"))
	want := []string{"api/openapi.yaml", "web/src/app/page.tsx"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("splitNULPaths = %#v, want %#v", got, want)
	}
}

func TestAllowedExampleHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{host: "prometheus.example.test", want: true},
		{host: "example.com", want: true},
		{host: "localhost", want: true},
		{host: "127.0.0.1", want: true},
		{host: "[::1]", want: true},
		{host: "prometheus.internal.company", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			if got := isAllowedExampleHost(tc.host); got != tc.want {
				t.Fatalf("isAllowedExampleHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestInspectFileAcceptsPlaceholderConfiguration(t *testing.T) {
	data := []byte(`servers:
  - url: http://localhost:8080
example:
  base_url: https://prometheus.example.test
  secret_ref: secret/openclarion/prometheus-bearer
`)
	if issues := inspectFile("api/openapi.yaml", data); len(issues) > 0 {
		t.Fatalf("inspectFile issues = %v", issues)
	}
}

func TestInspectFileRejectsNonPlaceholderURLHost(t *testing.T) {
	data := []byte(`const upstream = "https://prometheus.internal.company";`)
	issues := inspectFile("web/src/features/settings/alert-sources/api.ts", data)
	if joined := joinIssueMessages(issues); !strings.Contains(joined, "non-placeholder URL host") {
		t.Fatalf("issues = %q, want non-placeholder host rejection", joined)
	}
}

func TestInspectFileRejectsNonTestURLUserinfo(t *testing.T) {
	data := []byte(`const upstream = "https://operator:credential@prometheus.example.test";`)
	issues := inspectFile("web/src/features/settings/alert-sources/api.ts", data)
	if joined := joinIssueMessages(issues); !strings.Contains(joined, "URL userinfo outside a test fixture") {
		t.Fatalf("issues = %q, want userinfo rejection", joined)
	}
}

func TestInspectFileAllowsTestFixtureURLUserinfo(t *testing.T) {
	data := []byte(`rawEndpoint := "https://operator:credential@prometheus.example.test"`)
	issues := inspectFile("internal/usecases/alertsourcecheck/check_test.go", data)
	if len(issues) > 0 {
		t.Fatalf("inspectFile issues = %v", issues)
	}
}

func TestInspectFileRejectsBrowserDurableStorage(t *testing.T) {
	data := []byte(`localStorage.setItem("alert-source", JSON.stringify(profile));`)
	issues := inspectFile("web/src/features/settings/alert-sources/view.tsx", data)
	if joined := joinIssueMessages(issues); !strings.Contains(joined, "browser durable storage API") {
		t.Fatalf("issues = %q, want storage rejection", joined)
	}
}

func TestRunChecksTrackedOperationsSurface(t *testing.T) {
	dir := t.TempDir()
	runHygieneGit(t, dir, "init")
	runHygieneGit(t, dir, "config", "user.name", "OpenClarion Test")
	runHygieneGit(t, dir, "config", "user.email", "test@example.com")
	writeHygieneFile(t, dir, "api/openapi.yaml", []byte("servers:\n  - url: http://localhost:8080\n"))
	writeHygieneFile(t, dir, "web/src/features/settings/alert-sources/view.tsx", []byte("export const ok = true;\n"))
	runHygieneGit(t, dir, "add", ".")

	var stdout bytes.Buffer
	if err := run(context.Background(), config{RepoRoot: dir}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[operations-config-hygiene] OK (2 tracked configuration files checked)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunRejectsTrackedCustomerEndpoint(t *testing.T) {
	dir := t.TempDir()
	runHygieneGit(t, dir, "init")
	runHygieneGit(t, dir, "config", "user.name", "OpenClarion Test")
	runHygieneGit(t, dir, "config", "user.email", "test@example.com")
	writeHygieneFile(t, dir, "api/openapi.yaml", []byte("base_url: https://alerts.internal.company\n"))
	runHygieneGit(t, dir, "add", ".")

	var stdout bytes.Buffer
	err := run(context.Background(), config{RepoRoot: dir}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "non-placeholder URL host") {
		t.Fatalf("run error = %v, want customer endpoint rejection", err)
	}
}

func runHygieneGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- test helper passes fixed git subcommands.
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeHygieneFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func joinIssueMessages(issues []fileIssue) string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Message)
	}
	return strings.Join(messages, "\n")
}
