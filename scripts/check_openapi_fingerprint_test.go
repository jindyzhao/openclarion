package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
