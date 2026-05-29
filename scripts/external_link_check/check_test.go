package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunInventoriesExternalLinksWithoutLiveChecks(t *testing.T) {
	root := t.TempDir()
	writeExternalLinkFile(t, root, "README.md", "See https://example.com/docs.\n")
	writeExternalLinkFile(t, root, "docs/design.md", "[API](https://example.com/api), raw https://example.com/api\n")

	var stdout bytes.Buffer
	if err := run(config{Root: root, Timeout: time.Second}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[external-links] OK (2 unique external links inventoried across 2 files; live=false)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLiveChecksExternalLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusNoContent)
		case "/head-blocked":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	writeExternalLinkFile(t, root, "README.md", fmt.Sprintf("%s/ok\n%s/head-blocked\n", server.URL, server.URL))

	var stdout bytes.Buffer
	if err := run(config{Root: root, Live: true, Timeout: time.Second}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[external-links] OK (2 unique external links checked across 1 files)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLiveSkipsReservedExampleLinks(t *testing.T) {
	root := t.TempDir()
	writeExternalLinkFile(t, root, "README.md", "Example runbook: https://runbooks.example/payments\n")

	var stdout bytes.Buffer
	if err := run(config{Root: root, Live: true, Timeout: time.Second}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "[external-links] OK (0 unique external links checked across 1 files; 1 reserved example links skipped)") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunLiveRejectsMissingExternalLink(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	root := t.TempDir()
	writeExternalLinkFile(t, root, "docs/design.md", server.URL+"/missing\n")

	var stdout bytes.Buffer
	err := run(config{Root: root, Live: true, Timeout: time.Second}, &stdout)
	if err == nil {
		t.Fatal("run passed unexpectedly")
	}
	if !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "docs/design.md") {
		t.Fatalf("run error = %v, want 404 with source file", err)
	}
}

func TestRunRejectsInvalidTimeout(t *testing.T) {
	var stdout bytes.Buffer
	err := run(config{Root: t.TempDir()}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "--timeout must be greater than zero") {
		t.Fatalf("run error = %v, want timeout validation", err)
	}
}

func writeExternalLinkFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
