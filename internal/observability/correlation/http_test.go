package correlation

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestMiddlewarePreservesValidRequestID(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requestID, ok := RequestIDFromContext(r.Context())
		if !ok {
			t.Fatalf("request ID missing from context")
		}
		if requestID != "incident-42" {
			t.Fatalf("request ID = %q, want incident-42", requestID)
		}
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "incident-42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "incident-42" {
		t.Fatalf("response request ID = %q, want incident-42", got)
	}
}

func TestMiddlewareReplacesInvalidRequestID(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requestID, ok := RequestIDFromContext(r.Context())
		if !ok {
			t.Fatalf("request ID missing from context")
		}
		if requestID == "bad\nid" {
			t.Fatalf("invalid request ID was preserved")
		}
		if !ValidRequestID(requestID) {
			t.Fatalf("generated request ID %q is invalid", requestID)
		}
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "bad\nid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got == "" || got == "bad\nid" {
		t.Fatalf("response request ID = %q, want generated ID", got)
	}
}

func TestLogAttrsIncludesRequestAndTraceIDs(t *testing.T) {
	traceID := trace.TraceID{0x01, 0x02, 0x03}
	spanID := trace.SpanID{0x04, 0x05, 0x06}
	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
	ctx := trace.ContextWithSpanContext(
		ContextWithRequestID(context.Background(), "request-1"),
		spanCtx,
	)

	attrs := LogAttrs(ctx)
	got := attrsToMap(attrs)
	if got["request_id"] != "request-1" {
		t.Fatalf("request_id = %q, want request-1", got["request_id"])
	}
	if got["trace_id"] != traceID.String() {
		t.Fatalf("trace_id = %q, want %s", got["trace_id"], traceID.String())
	}
	if got["span_id"] != spanID.String() {
		t.Fatalf("span_id = %q, want %s", got["span_id"], spanID.String())
	}
}

func TestRoundTripperPropagatesRequestID(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(RequestIDHeader)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: RoundTripper(nil)}
	ctx := ContextWithRequestID(context.Background(), "request-1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if seen != "request-1" {
		t.Fatalf("%s = %q, want request-1", RequestIDHeader, seen)
	}
}

func TestRoundTripperPreservesExplicitRequestID(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(RequestIDHeader)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: RoundTripper(nil)}
	ctx := ContextWithRequestID(context.Background(), "request-1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	req.Header.Set(RequestIDHeader, "explicit-1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if seen != "explicit-1" {
		t.Fatalf("%s = %q, want explicit-1", RequestIDHeader, seen)
	}
}

func TestValidRequestIDRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "bad id", "bad\nid", strings.Repeat("a", maxRequestIDLen+1)} {
		if ValidRequestID(value) {
			t.Fatalf("ValidRequestID(%q) = true, want false", value)
		}
	}
	for _, value := range []string{"abc123", "incident-42", "trace_1", "tenant:case.1"} {
		if !ValidRequestID(value) {
			t.Fatalf("ValidRequestID(%q) = false, want true", value)
		}
	}
}

func attrsToMap(attrs []slog.Attr) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.String()
	}
	return out
}
