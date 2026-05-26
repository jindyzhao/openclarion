package prometheus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// alertsEnvelope is the shape Prometheus's /api/v1/alerts actually
// returns. The client_golang library expects a top-level
// {status, data:{alerts:[...]}} envelope and rejects bare arrays,
// so the mock MUST mirror it exactly. activeAt uses RFC3339Nano
// because that is what Prometheus emits on the wire and what the
// upstream JSON tag (`json:"activeAt"`) decodes back into time.Time.
const alertsEnvelope = `{
  "status": "success",
  "data": {
    "alerts": [
      {
        "labels":      {"alertname": "HighCPU", "instance": "i-1"},
        "annotations": {"summary": "cpu high"},
        "state":       "firing",
        "activeAt":    "2026-05-26T10:00:00.000000000Z",
        "value":       "1e+00"
      },
      {
        "labels":      {"alertname": "WarmUp", "instance": "i-2"},
        "annotations": {"summary": "warming"},
        "state":       "pending",
        "activeAt":    "2026-05-26T10:05:00.000000000Z",
        "value":       "5e-01"
      }
    ]
  }
}`

// mustParseTime fails the test with a clear "fixture is broken"
// message if the constant string fails to parse; this keeps
// test-data typos from surfacing as a misleading equality failure.
func mustParseTime(t *testing.T, layout, s string) time.Time {
	t.Helper()
	v, err := time.Parse(layout, s)
	if err != nil {
		t.Fatalf("parse fixture time %q: %v", s, err)
	}
	return v
}

// newAlertsServer spins up an httptest server whose /api/v1/alerts
// handler is the supplied function and whose other paths 404. We
// return the *httptest.Server so individual tests can plug
// per-case handlers without re-stating the path muxing.
func newAlertsServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/v1/alerts", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestProvider_ListActiveAlerts_FiltersFiringOnly verifies the
// firing-only contract and the field projection produced by
// labelSetToMap + the RawPayload re-marshal path.
//
// The non-firing alert in the envelope MUST be dropped silently
// (i.e. without raising an error) so callers do not have to defend
// against pending/inactive leaking through.
func TestProvider_ListActiveAlerts_FiltersFiringOnly(t *testing.T) {
	srv := newAlertsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(alertsEnvelope))
	})

	p, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := p.ListActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (pending alerts must be filtered)", len(got))
	}
	a := got[0]
	if a.Source != "prometheus" {
		t.Errorf("Source = %q, want %q", a.Source, "prometheus")
	}
	wantLabels := map[string]string{"alertname": "HighCPU", "instance": "i-1"}
	if !mapsEqual(a.Labels, wantLabels) {
		t.Errorf("Labels = %v, want %v", a.Labels, wantLabels)
	}
	wantAnn := map[string]string{"summary": "cpu high"}
	if !mapsEqual(a.Annotations, wantAnn) {
		t.Errorf("Annotations = %v, want %v", a.Annotations, wantAnn)
	}
	wantStartsAt := mustParseTime(t, time.RFC3339Nano, "2026-05-26T10:00:00.000000000Z")
	if !a.StartsAt.Equal(wantStartsAt) {
		t.Errorf("StartsAt = %v, want %v", a.StartsAt, wantStartsAt)
	}
	// RawPayload is the re-marshal of v1.Alert. v1.Alert only tags
	// ActiveAt as json:"activeAt", so the other field casings are
	// implementation details of client_golang and MUST NOT be
	// asserted here. json.Valid is the strongest portable check.
	if !json.Valid(a.RawPayload) {
		t.Errorf("RawPayload is not valid JSON: %s", string(a.RawPayload))
	}
}

// TestProvider_WithBearer_SendsAuthorizationHeader is the only
// behavioural guard the WithBearer option has. Without it, removing
// the Bearer wiring inside NewProvider would still type-check and
// every other test would still pass.
//
// We use a sentinel token that contains characters illegal in a
// header value would not appear by accident, so an unrelated change
// that re-introduces a leading "Basic " etc. shows up here.
func TestProvider_WithBearer_SendsAuthorizationHeader(t *testing.T) {
	const token = "test-bearer-token-MQ97" // arbitrary, only used in-process

	var seen string
	srv := newAlertsServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"alerts":[]}}`))
	})

	p, err := NewProvider(srv.URL, WithBearer(token))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := p.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	want := "Bearer " + token
	if seen != want {
		t.Errorf("Authorization header = %q, want %q", seen, want)
	}
}

// TestProvider_WithBearer_EmptyTokenIsNoop documents the "empty
// string is no auth" contract that lets callers write
// WithBearer(os.Getenv(...)) without a guard. Without this test,
// silently inserting a "Bearer " (with empty credentials) would
// pass review.
func TestProvider_WithBearer_EmptyTokenIsNoop(t *testing.T) {
	var seen string
	srv := newAlertsServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"alerts":[]}}`))
	})

	p, err := NewProvider(srv.URL, WithBearer(""))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := p.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seen != "" {
		t.Errorf("Authorization header = %q, want empty (empty token must be no-op)", seen)
	}
}

// TestProvider_ListActiveAlerts_WrapsServerError asserts that 5xx
// responses from Prometheus surface as wrapped errors with the
// package prefix, so log lines / error matching can identify the
// failing layer.
func TestProvider_ListActiveAlerts_WrapsServerError(t *testing.T) {
	srv := newAlertsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "prometheus down", http.StatusServiceUnavailable)
	})

	p, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.ListActiveAlerts(context.Background())
	if err == nil {
		t.Fatalf("ListActiveAlerts: want non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "prometheus: list alerts") {
		t.Errorf("error %q missing 'prometheus: list alerts' wrap prefix", err.Error())
	}
}

// TestProvider_ListActiveAlerts_WrapsMalformedJSON covers the case
// where the upstream replies 200 OK but with a body the client_golang
// decoder rejects. The library returns an error from Alerts() and
// we wrap it identically to the network-error path.
func TestProvider_ListActiveAlerts_WrapsMalformedJSON(t *testing.T) {
	srv := newAlertsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"alerts":[ not json `))
	})

	p, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.ListActiveAlerts(context.Background())
	if err == nil {
		t.Fatalf("ListActiveAlerts: want non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "prometheus: list alerts") {
		t.Errorf("error %q missing 'prometheus: list alerts' wrap prefix", err.Error())
	}
}

// mapsEqual is a small, dependency-free helper. We avoid reflect
// here so a failure in the test's expectation logic is visible in
// the trace.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

