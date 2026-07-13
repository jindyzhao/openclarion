// Package egressproxy implements the narrow forward-proxy boundary used by
// local Docker sandboxes. It permits only operator-configured host:port
// targets and supports plain HTTP plus HTTPS CONNECT tunneling.
package egressproxy

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
	"golang.org/x/sync/errgroup"
)

const (
	defaultDialTimeout                  = 10 * time.Second
	defaultResponseHeaderTimeout        = 30 * time.Second
	defaultMaxRequestDuration           = 5 * time.Minute
	defaultMaxResponseHeaderBytes int64 = 32 * 1024
)

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// Config controls one allowlist proxy handler.
type Config struct {
	AllowedTargets         []string
	DialTimeout            time.Duration
	ResponseHeaderTimeout  time.Duration
	MaxRequestDuration     time.Duration
	MaxResponseHeaderBytes int64
}

// Handler is an HTTP forward proxy with an exact outbound target allowlist.
type Handler struct {
	allowed              map[string]struct{}
	allowlistFingerprint string
	dialContext          func(context.Context, string, string) (net.Conn, error)
	transport            *http.Transport
	maxRequestDuration   time.Duration
}

// NewHandler validates cfg and returns a reusable proxy handler.
func NewHandler(cfg Config) (*Handler, error) {
	allowedTargets, err := ports.NormalizeContainerEgressTargets(cfg.AllowedTargets)
	if err != nil {
		return nil, fmt.Errorf("egress proxy allowlist: %w", err)
	}
	allowlistFingerprint, err := ports.ContainerEgressAllowlistFingerprint(allowedTargets)
	if err != nil {
		return nil, fmt.Errorf("egress proxy allowlist fingerprint: %w", err)
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = defaultDialTimeout
	}
	if cfg.ResponseHeaderTimeout == 0 {
		cfg.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if cfg.MaxRequestDuration == 0 {
		cfg.MaxRequestDuration = defaultMaxRequestDuration
	}
	if cfg.MaxResponseHeaderBytes == 0 {
		cfg.MaxResponseHeaderBytes = defaultMaxResponseHeaderBytes
	}
	if cfg.DialTimeout < 0 || cfg.ResponseHeaderTimeout < 0 || cfg.MaxRequestDuration < 0 {
		return nil, fmt.Errorf("egress proxy timeouts must be positive")
	}
	if cfg.MaxResponseHeaderBytes < 0 {
		return nil, fmt.Errorf("egress proxy max response header bytes must be positive")
	}

	allowed := make(map[string]struct{}, len(allowedTargets))
	for _, target := range allowedTargets {
		allowed[target] = struct{}{}
	}
	dialer := &net.Dialer{Timeout: cfg.DialTimeout, KeepAlive: 30 * time.Second}
	dialContext := dialer.DialContext
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.DialContext = dialContext
	transport.ResponseHeaderTimeout = cfg.ResponseHeaderTimeout
	transport.MaxResponseHeaderBytes = cfg.MaxResponseHeaderBytes
	transport.ForceAttemptHTTP2 = false

	return &Handler{
		allowed:              allowed,
		allowlistFingerprint: allowlistFingerprint,
		dialContext:          dialContext,
		transport:            transport,
		maxRequestDuration:   cfg.MaxRequestDuration,
	}, nil
}

// Close releases idle upstream connections held by the handler.
func (h *Handler) Close() {
	if h != nil && h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

// ServeHTTP handles local health/readiness endpoints, HTTP forward requests,
// and HTTPS CONNECT tunnels. Error responses intentionally omit upstream details.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.Error(w, "proxy unavailable", http.StatusServiceUnavailable)
		return
	}
	if !r.URL.IsAbs() && r.URL.Path == "/healthz" {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, "ok\n")
		}
		return
	}
	if !r.URL.IsAbs() && r.URL.Path == ports.ContainerEgressProxyReadinessPath {
		h.serveReadiness(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.maxRequestDuration)
	defer cancel()
	r = r.WithContext(ctx)
	if r.Method == http.MethodConnect {
		h.serveConnect(w, r)
		return
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(h.maxRequestDuration)
	}
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(deadline); err != nil {
		http.Error(w, "proxy unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	h.serveHTTPForward(w, r)
}

func (h *Handler) serveReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	provided := r.Header.Values(ports.ContainerEgressProxyReadinessFingerprintHeader)
	if r.URL.RawQuery != "" || len(provided) != 1 ||
		subtle.ConstantTimeCompare([]byte(provided[0]), []byte(h.allowlistFingerprint)) != 1 {
		http.Error(w, "proxy not ready", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, "ready\n")
	}
}

func (h *Handler) serveConnect(w http.ResponseWriter, r *http.Request) {
	target, err := canonicalTarget(r.Host, "443")
	if err != nil || !h.targetAllowed(target) {
		http.Error(w, "egress target denied", http.StatusForbidden)
		return
	}
	dialTarget := target
	if !strings.Contains(target, ":") {
		dialTarget = net.JoinHostPort(target, "443")
	}
	upstream, err := h.dialContext(r.Context(), "tcp", dialTarget)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	downstream, buffered, err := http.NewResponseController(w).Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	defer downstream.Close()
	defer upstream.Close()
	deadline, ok := r.Context().Deadline()
	if !ok {
		deadline = time.Now().Add(h.maxRequestDuration)
	}
	_ = downstream.SetDeadline(deadline)
	_ = upstream.SetDeadline(deadline)
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := buffered.Flush(); err != nil {
		return
	}

	var closeOnce sync.Once
	closeTunnel := func() {
		_ = downstream.Close()
		_ = upstream.Close()
	}
	var group errgroup.Group
	group.Go(func() error {
		defer closeOnce.Do(closeTunnel)
		return copyTunnel(upstream, buffered)
	})
	group.Go(func() error {
		defer closeOnce.Do(closeTunnel)
		return copyTunnel(downstream, upstream)
	})
	_ = group.Wait()
}

func (h *Handler) serveHTTPForward(w http.ResponseWriter, r *http.Request) {
	if !r.URL.IsAbs() || !strings.EqualFold(r.URL.Scheme, "http") || r.URL.User != nil {
		http.Error(w, "absolute http URL required", http.StatusBadRequest)
		return
	}
	target, err := canonicalTarget(r.URL.Host, "80")
	if err != nil || !h.targetAllowed(target) {
		http.Error(w, "egress target denied", http.StatusForbidden)
		return
	}

	out := r.Clone(r.Context())
	out.RequestURI = ""
	out.Host = out.URL.Host
	out.Close = false
	out.Header = r.Header.Clone()
	removeHopByHopHeaders(out.Header)
	if _, ok := out.Header["User-Agent"]; !ok {
		out.Header.Set("User-Agent", "")
	}
	resp, err := h.transport.RoundTrip(out)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	removeHopByHopHeaders(resp.Header)
	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	copyAndFlush(w, resp.Body)
}

func (h *Handler) targetAllowed(target string) bool {
	_, ok := h.allowed[target]
	return ok
}

func canonicalTarget(raw, defaultPort string) (string, error) {
	if defaultPort != "" && !strings.Contains(raw, ":") {
		raw = net.JoinHostPort(raw, defaultPort)
	}
	normalized, err := ports.NormalizeContainerEgressTargets([]string{raw})
	if err != nil {
		return "", err
	}
	return normalized[0], nil
}

func removeHopByHopHeaders(header http.Header) {
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			if name = strings.TrimSpace(name); name != "" {
				header.Del(name)
			}
		}
	}
	for _, name := range hopByHopHeaders {
		header.Del(name)
	}
}

func copyHeaders(dst, src http.Header) {
	for name, values := range src {
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func copyAndFlush(w http.ResponseWriter, src io.Reader) {
	buf := make([]byte, 32*1024)
	controller := http.NewResponseController(w)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			_ = controller.Flush()
		}
		if err != nil {
			return
		}
	}
}

func copyTunnel(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

var _ http.Handler = (*Handler)(nil)
