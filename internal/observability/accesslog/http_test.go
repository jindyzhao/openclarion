package accesslog

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/observability/correlation"
)

func TestMiddlewareLogsStableAccessRecord(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))
	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	}))

	ctx := correlation.ContextWithRequestID(context.Background(), "request-1")
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/reports/42?state=redacted", nil)
	req.Pattern = "POST /api/v1/reports/{report_id}"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	for _, want := range []string{
		`msg="http access"`,
		`request_id=request-1`,
		`method=POST`,
		`route="POST /api/v1/reports/{report_id}"`,
		`status=202`,
		`response_bytes=2`,
	} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line %q missing %s", logLine, want)
		}
	}
	if strings.Contains(logLine, "state=redacted") || strings.Contains(logLine, "/api/v1/reports/42") {
		t.Fatalf("access log leaked raw request target: %q", logLine)
	}
}

func TestMiddlewareDefaultsImplicitStatusToOK(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: dropTime}))
	handler := Middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "status=200") {
		t.Fatalf("log line %q missing implicit 200 status", buf.String())
	}
}

func dropTime(groups []string, attr slog.Attr) slog.Attr {
	if len(groups) == 0 && attr.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attr
}
