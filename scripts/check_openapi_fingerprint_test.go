package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPIFingerprintReadLockRejectsWeakJSON(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name:      "duplicate key",
			content:   `{"paths./api/v1/alerts.get":"old","paths./api/v1/alerts.get":"new"}`,
			wantError: `duplicate object key "paths./api/v1/alerts.get"`,
		},
		{
			name:      "trailing value",
			content:   `{"paths./api/v1/alerts.get":"abc"}[]`,
			wantError: "trailing JSON values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "openapi-critical.lock")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write lock: %v", err)
			}

			_, err := readLock(path)
			if err == nil {
				t.Fatal("readLock err = nil, want weak JSON rejection")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("readLock err = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestOpenAPIFingerprintReadYAMLRejectsWeakYAML(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name:      "duplicate top-level key",
			content:   "openapi: 3.1.0\nopenapi: 3.0.0\n",
			wantError: `duplicate YAML key "openapi"`,
		},
		{
			name: "duplicate nested key",
			content: `
openapi: 3.1.0
paths:
  /api/v1/alerts:
    get:
      responses: {}
    get:
      responses: {}
`,
			wantError: `duplicate YAML key "get"`,
		},
		{
			name:      "multiple documents",
			content:   "openapi: 3.1.0\n---\nopenapi: 3.1.0\n",
			wantError: "multiple YAML documents are not allowed",
		},
		{
			name: "merge key",
			content: `
openapi: 3.1.0
paths:
  /api/v1/alerts:
    <<:
      get:
        responses: {}
`,
			wantError: "YAML merge keys are not allowed",
		},
		{
			name: "anchor",
			content: `
openapi: 3.1.0
components:
  responses:
    Ok: &ok
      description: ok
paths: {}
`,
			wantError: "YAML anchors are not allowed",
		},
		{
			name: "anchor before alias",
			content: `
openapi: 3.1.0
components:
  responses:
    Ok: &ok
      description: ok
paths:
  /api/v1/alerts:
    get:
      responses:
        "200": *ok
`,
			wantError: "YAML anchors are not allowed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "openapi.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write spec: %v", err)
			}

			_, err := readYAML(path)
			if err == nil {
				t.Fatal("readYAML err = nil, want weak YAML rejection")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("readYAML err = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestOpenAPIFingerprintRejectWeakYAMLFeaturesRejectsAliasNode(t *testing.T) {
	err := rejectWeakYAMLFeatures(&yaml.Node{Kind: yaml.AliasNode, Line: 7})
	if err == nil {
		t.Fatal("rejectWeakYAMLFeatures err = nil, want alias rejection")
	}
	if !strings.Contains(err.Error(), "YAML aliases are not allowed at line 7") {
		t.Fatalf("rejectWeakYAMLFeatures err = %v, want alias rejection", err)
	}
}

func TestOpenAPIFingerprintReadYAMLAcceptsValidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.1.0\ninfo:\n  title: OpenClarion\n  version: 0.0.0\n"), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	got, err := readYAML(path)
	if err != nil {
		t.Fatalf("readYAML: %v", err)
	}
	root, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("readYAML root type = %T, want map[string]any", got)
	}
	if root["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %v, want 3.1.0", root["openapi"])
	}
}

func TestOpenAPIFingerprintReadYAMLAcceptsQuotedMergeLikeKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	content := "openapi: 3.1.0\nx-fixture:\n  \"<<\": literal\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	got, err := readYAML(path)
	if err != nil {
		t.Fatalf("readYAML: %v", err)
	}
	root, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("readYAML root type = %T, want map[string]any", got)
	}
	fixture, ok := root["x-fixture"].(map[string]any)
	if !ok {
		t.Fatalf("x-fixture type = %T, want map[string]any", root["x-fixture"])
	}
	if fixture["<<"] != "literal" {
		t.Fatalf("quoted merge-like key = %v, want literal", fixture["<<"])
	}
}

func TestOpenAPIFingerprintReadLockAcceptsValidLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi-critical.lock")
	if err := os.WriteFile(path, []byte(`{"paths./api/v1/alerts.get":"abc"}`), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	got, err := readLock(path)
	if err != nil {
		t.Fatalf("readLock: %v", err)
	}
	if got["paths./api/v1/alerts.get"] != "abc" {
		t.Fatalf("lock entry = %q, want abc", got["paths./api/v1/alerts.get"])
	}
}
