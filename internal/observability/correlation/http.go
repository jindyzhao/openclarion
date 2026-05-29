// Package correlation owns request/log correlation primitives shared by
// transports and observability wiring.
package correlation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

const (
	// RequestIDHeader is the HTTP header used for request correlation.
	RequestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128
)

type requestIDKey struct{}

// Middleware attaches a bounded request ID to the request context and response
// header. Valid incoming IDs are preserved; invalid or missing IDs are replaced
// with a generated ID to prevent log/header injection.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			return nil
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get(RequestIDHeader))
			if !ValidRequestID(requestID) {
				requestID = newRequestID()
			}
			w.Header().Set(RequestIDHeader, requestID)
			next.ServeHTTP(w, r.WithContext(ContextWithRequestID(r.Context(), requestID)))
		})
	}
}

// ContextWithRequestID stores a request ID in ctx.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if !ValidRequestID(requestID) {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext returns the request ID stored in ctx, if present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	requestID, ok := ctx.Value(requestIDKey{}).(string)
	if !ok || !ValidRequestID(requestID) {
		return "", false
	}
	return requestID, true
}

// LogAttrs returns stable structured log attributes for request and trace
// correlation. Callers should pass the same ctx to slog.LogAttrs so handlers
// that understand context can also consume the trace context directly.
func LogAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 3)
	if requestID, ok := RequestIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return attrs
}

// RoundTripper propagates the current request ID to outbound HTTP requests.
// It preserves any caller-supplied X-Request-ID header and clones the request
// before mutation so retries or other transports do not observe side effects.
func RoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return requestIDRoundTripper{base: base}
}

// ValidRequestID reports whether requestID is safe to echo in a header and log.
func ValidRequestID(requestID string) bool {
	if requestID == "" || len(requestID) > maxRequestIDLen {
		return false
	}
	for _, r := range requestID {
		if r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("request-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

type requestIDRoundTripper struct {
	base http.RoundTripper
}

func (t requestIDRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return t.base.RoundTrip(req)
	}
	requestID, ok := RequestIDFromContext(req.Context())
	if !ok || req.Header.Get(RequestIDHeader) != "" {
		return t.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set(RequestIDHeader, requestID)
	return t.base.RoundTrip(clone)
}
