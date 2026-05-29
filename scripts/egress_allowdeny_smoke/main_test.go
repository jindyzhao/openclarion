package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProxyAllowsConfiguredHostAndDeniesOthers(t *testing.T) {
	allowedUpstream := httptest.NewServer(upstreamHandler("allowed"))
	defer allowedUpstream.Close()
	deniedUpstream := httptest.NewServer(upstreamHandler("denied"))
	defer deniedUpstream.Close()

	allowedURL, err := url.Parse(allowedUpstream.URL)
	if err != nil {
		t.Fatalf("parse allowed upstream URL: %v", err)
	}
	proxy := httptest.NewServer(proxyHandler(map[string]bool{allowedURL.Host: true}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   2 * time.Second,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, allowedUpstream.URL, nil)
	if err != nil {
		t.Fatalf("build allowed request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET allowed via proxy: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("allowed status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	_ = resp.Body.Close()

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, deniedUpstream.URL, nil)
	if err != nil {
		t.Fatalf("build denied request: %v", err)
	}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET denied via proxy: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	_ = resp.Body.Close()
}

func TestRunClientAcceptsExpectedFailure(t *testing.T) {
	var stdout bytes.Buffer
	err := runClient([]string{
		"--url", "http://127.0.0.1:1",
		"--want-fail",
		"--timeout", "50ms",
	}, &stdout)
	if err != nil {
		t.Fatalf("runClient: %v", err)
	}
	if !strings.Contains(stdout.String(), "expected failure") {
		t.Fatalf("stdout = %q, want expected failure", stdout.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run err = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run err = %v, want unknown command", err)
	}
}
