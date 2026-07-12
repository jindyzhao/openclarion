package egressproxy

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestHandlerAllowsHTTPAndDeniesUnlistedTarget(t *testing.T) {
	upstreamHosts := make(chan string, 1)
	upstreamViolations := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("X-Hop") != "" || r.Header.Get("Accept-Encoding") != "" {
			upstreamViolations <- "proxy-only or transport-injected headers reached upstream"
			http.Error(w, "invalid forwarded headers", http.StatusInternalServerError)
			return
		}
		upstreamHosts <- r.Host
		_, _ = io.WriteString(w, "allowed")
	}))
	defer upstream.Close()
	upstreamURL := mustURL(t, upstream.URL)
	proxy := newTestProxy(t, []string{upstreamURL.Host})
	defer proxy.Close()

	client := proxyClient(t, proxy.URL, nil)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Proxy-Authorization", "secret")
	req.Header.Set("Connection", "X-Hop")
	req.Header.Set("X-Hop", "remove-me")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("allowed request: %v", err)
	}
	assertBody(t, resp, http.StatusOK, "allowed")
	if got := <-upstreamHosts; got != upstreamURL.Host {
		t.Fatalf("upstream host = %q, want proxy target %q", got, upstreamURL.Host)
	}

	handler, err := NewHandler(Config{AllowedTargets: []string{upstreamURL.Host}})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	defer handler.Close()
	spoofedHostRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL, nil)
	spoofedHostRequest.Host = "unlisted-virtual-host.example.test"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, spoofedHostRequest)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "allowed") {
		t.Fatalf("spoofed Host response = %d %q, want 200 containing %q", recorder.Code, recorder.Body.String(), "allowed")
	}
	if got := <-upstreamHosts; got != upstreamURL.Host {
		t.Fatalf("spoofed upstream host = %q, want proxy target %q", got, upstreamURL.Host)
	}
	select {
	case violation := <-upstreamViolations:
		t.Fatal(violation)
	default:
	}

	deniedReached := make(chan struct{}, 1)
	denied := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		deniedReached <- struct{}{}
	}))
	defer denied.Close()
	deniedRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, denied.URL, nil)
	if err != nil {
		t.Fatalf("new denied request: %v", err)
	}
	resp, err = client.Do(deniedRequest)
	if err != nil {
		t.Fatalf("denied request: %v", err)
	}
	assertBody(t, resp, http.StatusForbidden, "egress target denied")
	select {
	case <-deniedReached:
		t.Fatal("denied upstream was reached")
	default:
	}
}

func TestHandlerSupportsHTTPSConnect(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "tls-ok")
	}))
	defer upstream.Close()
	upstreamURL := mustURL(t, upstream.URL)
	proxy := newTestProxy(t, []string{upstreamURL.Host})
	defer proxy.Close()

	client := proxyClient(t, proxy.URL, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- test-only TLS server.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatalf("new HTTPS request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTPS through CONNECT: %v", err)
	}
	assertBody(t, resp, http.StatusOK, "tls-ok")
}

func TestHandlerHealthAndConfigurationValidation(t *testing.T) {
	handler, err := NewHandler(Config{AllowedTargets: []string{"api.example.test:443"}})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	defer handler.Close()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok\n" {
		t.Fatalf("health response = %d %q", recorder.Code, recorder.Body.String())
	}

	if _, err := NewHandler(Config{AllowedTargets: []string{"*.example.test:443"}}); err == nil {
		t.Fatal("NewHandler wildcard err = nil, want rejection")
	}
	if _, err := NewHandler(Config{AllowedTargets: []string{"api.example.test:443"}, DialTimeout: -time.Second}); err == nil {
		t.Fatal("NewHandler negative timeout err = nil, want rejection")
	}
}

func newTestProxy(t *testing.T, allowed []string) *httptest.Server {
	t.Helper()
	handler, err := NewHandler(Config{AllowedTargets: allowed, MaxRequestDuration: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	t.Cleanup(handler.Close)
	return httptest.NewServer(handler)
}

func proxyClient(t *testing.T, proxyRawURL string, tlsConfig *tls.Config) *http.Client {
	t.Helper()
	proxyURL := mustURL(t, proxyRawURL)
	transport := &http.Transport{
		Proxy:              http.ProxyURL(proxyURL),
		TLSClientConfig:    tlsConfig,
		DisableCompression: true,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", raw, err)
	}
	return parsed
}

func assertBody(t *testing.T, resp *http.Response, wantStatus int, wantBody string) {
	t.Helper()
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if resp.StatusCode != wantStatus || !strings.Contains(string(raw), wantBody) {
		t.Fatalf("response = %d %q, want %d containing %q", resp.StatusCode, raw, wantStatus, wantBody)
	}
}
