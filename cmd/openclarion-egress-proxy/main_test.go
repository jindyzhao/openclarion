package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" api.example.test:443,metrics.example.test:9090 ")
	want := []string{"api.example.test:443", "metrics.example.test:9090"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCSV = %#v, want %#v", got, want)
	}
	if got := splitCSV("  "); got != nil {
		t.Fatalf("splitCSV empty = %#v, want nil", got)
	}
}

func TestHealthcheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := healthcheck(server.URL + "/healthz"); err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	if err := healthcheck("https://example.test/healthz"); err == nil {
		t.Fatal("healthcheck https err = nil, want rejection")
	}
	if err := healthcheck("http://192.0.2.1/healthz"); err == nil {
		t.Fatal("healthcheck non-loopback err = nil, want rejection")
	}
	if err := healthcheck("http://127.0.0.1/admin"); err == nil {
		t.Fatal("healthcheck non-health path err = nil, want rejection")
	}
}

func TestHealthcheckRejectsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, nil, "http://192.0.2.1/healthz", http.StatusFound)
	}))
	defer server.Close()

	if err := healthcheck(server.URL + "/healthz"); err == nil {
		t.Fatal("healthcheck redirect err = nil, want rejection")
	}
}

func TestRunValidatesCommandsAndConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := run([]string{"healthcheck", server.URL + "/healthz"}); err != nil {
		t.Fatalf("run healthcheck: %v", err)
	}
	for _, args := range [][]string{
		{"healthcheck"},
		{"unknown"},
		{"serve", "extra"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("run(%q) error = nil, want argument rejection", args)
		}
	}

	t.Setenv(allowedTargetsEnv, "")
	if err := run(nil); err == nil {
		t.Fatal("run without allowlist error = nil")
	}
}
