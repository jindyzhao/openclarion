package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMetricsExposesRequestMetrics(t *testing.T) {
	metrics := NewHTTPMetrics()
	handler := metrics.Middleware("api")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	scrape := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(scrape, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	if scrape.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want 200; body=%s", scrape.Code, scrape.Body.String())
	}
	body := scrape.Body.String()
	for _, want := range []string{
		"openclarion_http_requests_total",
		`code="204"`,
		`handler="api"`,
		"openclarion_http_request_duration_seconds",
		"openclarion_http_in_flight_requests",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}

func TestNilHTTPMetricsMiddlewareIsPassThrough(t *testing.T) {
	var metrics *HTTPMetrics
	called := false
	handler := metrics.Middleware("api")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if !called || rec.Code != http.StatusAccepted {
		t.Fatalf("pass-through handler called=%v status=%d", called, rec.Code)
	}
}
