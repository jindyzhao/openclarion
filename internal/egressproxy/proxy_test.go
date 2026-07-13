package egressproxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestHandlerAllowsHTTPAndDeniesUnlistedTarget(t *testing.T) {
	upstreamHosts := make(chan string, 1)
	upstreamViolations := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" ||
			r.Header.Get("X-Hop") != "" ||
			r.Header.Get("Accept-Encoding") != "" ||
			r.Header.Get("User-Agent") != "" ||
			r.Close {
			upstreamViolations <- "downstream-only or transport-injected request state reached upstream"
			http.Error(w, "invalid forwarded headers", http.StatusInternalServerError)
			return
		}
		upstreamHosts <- r.Host
		_, _ = io.WriteString(w, "allowed")
	}))
	defer upstream.Close()
	localUpstreamURL := mustURL(t, upstream.URL)
	upstreamAddress := localUpstreamURL.Host
	upstreamURL := *localUpstreamURL
	upstreamURL.Host = "allowed-http.example.test"
	targetAddress := net.JoinHostPort(upstreamURL.Host, "80")
	proxy := newMappedTestProxy(t, Config{
		AllowedTargets:     []string{targetAddress},
		MaxRequestDuration: 2 * time.Second,
	}, targetAddress, upstreamAddress)
	defer proxy.Close()

	client := proxyClient(t, proxy.URL, nil)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstreamURL.String(), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Proxy-Authorization", "secret")
	req.Header.Set("Connection", "X-Hop, User-Agent")
	req.Header.Set("X-Hop", "remove-me")
	req.Header.Set("User-Agent", "")
	req.Close = true
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("allowed request: %v", err)
	}
	assertBody(t, resp, http.StatusOK, "allowed")
	if got := <-upstreamHosts; got != upstreamURL.Host {
		t.Fatalf("upstream host = %q, want proxy target %q", got, upstreamURL.Host)
	}

	handler := newMappedHandler(
		t,
		Config{AllowedTargets: []string{targetAddress}},
		targetAddress,
		upstreamAddress,
	)
	defer handler.Close()
	spoofedHostRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, upstreamURL.String(), nil)
	spoofedHostRequest.Host = "unlisted-virtual-host.example.test"
	recorder := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
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
	upstreamURL, upstreamAddress := mappedUpstreamURL(t, upstream.URL, "allowed-https.example.test")
	proxy := newMappedTestProxy(t, Config{
		AllowedTargets:     []string{upstreamURL.Host},
		MaxRequestDuration: 2 * time.Second,
	}, upstreamURL.Host, upstreamAddress)
	defer proxy.Close()

	client := proxyClient(t, proxy.URL, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- test-only TLS server.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstreamURL.String(), nil)
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
	if _, err := NewHandler(Config{AllowedTargets: []string{"api.example.test"}}); err == nil {
		t.Fatal("NewHandler missing port err = nil, want rejection")
	}
	if _, err := NewHandler(Config{AllowedTargets: []string{"api.example.test:443"}, DialTimeout: -time.Second}); err == nil {
		t.Fatal("NewHandler negative timeout err = nil, want rejection")
	}
	if _, err := NewHandler(Config{AllowedTargets: []string{"api.example.test:443"}, MaxResponseHeaderBytes: -1}); err == nil {
		t.Fatal("NewHandler negative max response header bytes err = nil, want rejection")
	}
}

func TestHandlerReadinessRequiresMatchingLiveAllowlist(t *testing.T) {
	allowed := []string{"api.example.test:443", "metrics.example.test:9090"}
	handler, err := NewHandler(Config{AllowedTargets: allowed})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	defer handler.Close()
	fingerprint, err := ports.ContainerEgressAllowlistFingerprint([]string{
		"metrics.example.test:9090",
		"API.EXAMPLE.TEST:443",
	})
	if err != nil {
		t.Fatalf("ContainerEgressAllowlistFingerprint: %v", err)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, ports.ContainerEgressProxyReadinessPath, nil)
	request.Header.Set(ports.ContainerEgressProxyReadinessFingerprintHeader, fingerprint)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ready\n" {
		t.Fatalf("readiness response = %d %q", recorder.Code, recorder.Body.String())
	}

	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, ports.ContainerEgressProxyReadinessPath, nil)
	request.Header.Set(ports.ContainerEgressProxyReadinessFingerprintHeader, strings.Repeat("0", 64))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "proxy not ready") {
		t.Fatalf("stale readiness response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestRemoveHopByHopHeadersUsesEveryConnectionValue(t *testing.T) {
	header := http.Header{}
	header.Add("Connection", "keep-alive, X-First-Hop")
	header.Add("Connection", "X-Second-Hop")
	header.Set("Keep-Alive", "timeout=5")
	header.Set("X-First-Hop", "remove-me")
	header.Set("X-Second-Hop", "remove-me-too")

	removeHopByHopHeaders(header)
	for _, name := range []string{"Connection", "Keep-Alive", "X-First-Hop", "X-Second-Hop"} {
		if got := header.Get(name); got != "" {
			t.Fatalf("header %s = %q, want removal", name, got)
		}
	}
}

func TestHandlerBoundsAndClearsDownstreamWriteDeadline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "response body")
	}))
	defer upstream.Close()
	upstreamURL, upstreamAddress := mappedUpstreamURL(t, upstream.URL, "deadline.example.test")
	handler := newMappedHandler(t, Config{
		AllowedTargets:     []string{upstreamURL.Host},
		MaxRequestDuration: 100 * time.Millisecond,
	}, upstreamURL.Host, upstreamAddress)
	defer handler.Close()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, upstreamURL.String(), nil)
	writer := newDeadlineBlockingWriter()

	started := time.Now()
	handler.ServeHTTP(writer, request)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("ServeHTTP elapsed = %s, want bounded downstream write", elapsed)
	}
	deadlines := writer.deadlineHistory()
	if len(deadlines) < 2 || deadlines[0].IsZero() || !deadlines[len(deadlines)-1].IsZero() {
		t.Fatalf("write deadlines = %v, want non-zero deadline followed by clear", deadlines)
	}
}

func TestHandlerBoundsUpstreamResponseHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Oversized", strings.Repeat("x", 2*1024))
		_, _ = io.WriteString(w, "must not be forwarded")
	}))
	defer upstream.Close()
	upstreamURL, upstreamAddress := mappedUpstreamURL(t, upstream.URL, "headers.example.test")
	proxy := newMappedTestProxy(t, Config{
		AllowedTargets:         []string{upstreamURL.Host},
		MaxRequestDuration:     2 * time.Second,
		MaxResponseHeaderBytes: 1024,
	}, upstreamURL.Host, upstreamAddress)
	defer proxy.Close()

	client := proxyClient(t, proxy.URL, nil)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, upstreamURL.String(), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	assertBody(t, response, http.StatusBadGateway, "upstream unavailable")
}

func newMappedTestProxy(
	t *testing.T,
	cfg Config,
	targetAddress string,
	upstreamAddress string,
) *httptest.Server {
	t.Helper()
	handler := newMappedHandler(t, cfg, targetAddress, upstreamAddress)
	return httptest.NewServer(handler)
}

func newMappedHandler(
	t *testing.T,
	cfg Config,
	targetAddress string,
	upstreamAddress string,
) *Handler {
	t.Helper()
	handler, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	t.Cleanup(handler.Close)
	dialer := &net.Dialer{Timeout: time.Second}
	dialContext := func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != targetAddress {
			return nil, fmt.Errorf("unexpected test proxy dial target %q", address)
		}
		return dialer.DialContext(ctx, network, upstreamAddress)
	}
	handler.dialContext = dialContext
	handler.transport.DialContext = dialContext
	return handler
}

func mappedUpstreamURL(t *testing.T, rawURL, hostname string) (*url.URL, string) {
	t.Helper()
	upstream := mustURL(t, rawURL)
	mapped := *upstream
	mapped.Host = net.JoinHostPort(hostname, upstream.Port())
	return &mapped, upstream.Host
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

type deadlineBlockingWriter struct {
	mu        sync.Mutex
	header    http.Header
	deadline  time.Time
	deadlines []time.Time
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
}

func (*deadlineRecorder) SetWriteDeadline(time.Time) error {
	return nil
}

func newDeadlineBlockingWriter() *deadlineBlockingWriter {
	return &deadlineBlockingWriter{header: make(http.Header)}
}

func (w *deadlineBlockingWriter) Header() http.Header {
	return w.header
}

func (*deadlineBlockingWriter) WriteHeader(int) {}

func (w *deadlineBlockingWriter) Write([]byte) (int, error) {
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	if deadline.IsZero() {
		time.Sleep(2 * time.Second)
		return 0, os.ErrDeadlineExceeded
	}
	if delay := time.Until(deadline); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
	}
	return 0, os.ErrDeadlineExceeded
}

func (w *deadlineBlockingWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadline = deadline
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (w *deadlineBlockingWriter) deadlineHistory() []time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]time.Time(nil), w.deadlines...)
}
