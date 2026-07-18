package alertmanager

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
)

const alertsResponse = `[
  {
    "labels": {"alertname": "HighCPU", "instance": "i-1"},
    "annotations": {"summary": "cpu high"},
    "startsAt": "2026-06-05T04:00:00Z",
    "updatedAt": "2026-06-05T04:01:00Z",
    "endsAt": "2026-06-05T05:00:00Z",
    "receivers": [{"name": "team"}],
    "fingerprint": "abc123",
    "status": {
      "state": "active",
      "silencedBy": [],
      "inhibitedBy": [],
      "mutedBy": []
    }
  },
  {
    "labels": {"alertname": "Muted"},
    "annotations": {},
    "startsAt": "2026-06-05T04:00:00Z",
    "updatedAt": "2026-06-05T04:01:00Z",
    "endsAt": "2026-06-05T05:00:00Z",
    "receivers": [{"name": "team"}],
    "fingerprint": "def456",
    "status": {
      "state": "suppressed",
      "silencedBy": ["silence-1"],
      "inhibitedBy": [],
      "mutedBy": ["silence-1"]
    }
  }
]`

func newAlertmanagerServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/v2/alerts", handler)
	mux.Handle("/api/v2/alerts/", handler)
	mux.Handle("/alertmanager/api/v2/alerts", handler)
	mux.Handle("/alertmanager/api/v2/alerts/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestProviderListActiveAlerts(t *testing.T) {
	var query url.Values
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(alertsResponse))
	})

	provider, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	alerts, err := provider.ListActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("len(alerts) = %d, want 1", len(alerts))
	}
	alert := alerts[0]
	if alert.Source != "alertmanager" {
		t.Fatalf("Source = %q, want alertmanager", alert.Source)
	}
	if !reflect.DeepEqual(alert.Labels, map[string]string{"alertname": "HighCPU", "instance": "i-1"}) {
		t.Fatalf("Labels = %v", alert.Labels)
	}
	if !reflect.DeepEqual(alert.Annotations, map[string]string{"summary": "cpu high"}) {
		t.Fatalf("Annotations = %v", alert.Annotations)
	}
	wantStartsAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	if !alert.StartsAt.Equal(wantStartsAt) {
		t.Fatalf("StartsAt = %s, want %s", alert.StartsAt, wantStartsAt)
	}
	if !json.Valid(alert.RawPayload) {
		t.Fatalf("RawPayload is invalid JSON: %s", string(alert.RawPayload))
	}
	wantQuery := map[string]string{
		"active":      "true",
		"silenced":    "false",
		"inhibited":   "false",
		"unprocessed": "false",
	}
	for key, want := range wantQuery {
		if got := query.Get(key); got != want {
			t.Fatalf("query %s = %q, want %q", key, got, want)
		}
	}
}

func TestProviderListActiveAlertsFiltersSuppressedMarkers(t *testing.T) {
	responseBody := `[
	  {
	    "labels": {"alertname": "SilencedEvenIfActive"},
	    "annotations": {},
	    "startsAt": "2026-06-05T04:00:00Z",
	    "status": {
	      "state": "active",
	      "silencedBy": ["silence-1"],
	      "inhibitedBy": [],
	      "mutedBy": []
	    }
	  },
	  {
	    "labels": {"alertname": "InhibitedEvenIfActive"},
	    "annotations": {},
	    "startsAt": "2026-06-05T04:00:00Z",
	    "status": {
	      "state": "active",
	      "silencedBy": [],
	      "inhibitedBy": ["inhibit-1"],
	      "mutedBy": []
	    }
	  },
	  {
	    "labels": {"alertname": "MutedEvenIfActive"},
	    "annotations": {},
	    "startsAt": "2026-06-05T04:00:00Z",
	    "status": {
	      "state": "active",
	      "silencedBy": [],
	      "inhibitedBy": [],
	      "mutedBy": ["mute-1"]
	    }
	  }
	]`
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseBody))
	})

	provider, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	alerts, err := provider.ListActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Fatalf("len(alerts) = %d, want 0", len(alerts))
	}
}

func TestProviderAppendsAlertsPathBelowRoutePrefix(t *testing.T) {
	var seenPath string
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	provider, err := NewProvider(srv.URL + "/alertmanager/")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seenPath != "/alertmanager/api/v2/alerts" {
		t.Fatalf("path = %q, want /alertmanager/api/v2/alerts", seenPath)
	}
}

func TestNewProviderTrimsPastedAddressWhitespace(t *testing.T) {
	var seenPath string
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	provider, err := NewProvider(" \t" + srv.URL + "/alertmanager/ \n")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seenPath != "/alertmanager/api/v2/alerts" {
		t.Fatalf("path = %q, want /alertmanager/api/v2/alerts", seenPath)
	}
}

func TestProviderAcceptsAPIv2PrefixAndFullAlertsEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantPath string
	}{
		{name: "api prefix", path: "/api/v2", wantPath: "/api/v2/alerts"},
		{name: "api prefix slash", path: "/api/v2/", wantPath: "/api/v2/alerts"},
		{name: "full endpoint", path: "/api/v2/alerts", wantPath: "/api/v2/alerts"},
		{name: "full endpoint slash", path: "/api/v2/alerts/", wantPath: "/api/v2/alerts"},
		{name: "ui alerts", path: "/alerts", wantPath: "/api/v2/alerts"},
		{name: "ui silences", path: "/silences", wantPath: "/api/v2/alerts"},
		{name: "ui status", path: "/status", wantPath: "/api/v2/alerts"},
		{name: "ui receivers", path: "/receivers", wantPath: "/api/v2/alerts"},
		{name: "groups endpoint", path: "/api/v2/alerts/groups", wantPath: "/api/v2/alerts"},
		{name: "route api prefix", path: "/alertmanager/api/v2", wantPath: "/alertmanager/api/v2/alerts"},
		{name: "route full endpoint", path: "/alertmanager/api/v2/alerts", wantPath: "/alertmanager/api/v2/alerts"},
		{name: "route ui alerts", path: "/alertmanager/alerts", wantPath: "/alertmanager/api/v2/alerts"},
		{name: "route groups endpoint", path: "/alertmanager/api/v2/alerts/groups", wantPath: "/alertmanager/api/v2/alerts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seenPath string
			srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
				seenPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			})

			provider, err := NewProvider(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			if _, err := provider.ListActiveAlerts(context.Background()); err != nil {
				t.Fatalf("ListActiveAlerts: %v", err)
			}
			if seenPath != tc.wantPath {
				t.Fatalf("path = %q, want %q", seenPath, tc.wantPath)
			}
		})
	}
}

func TestProviderWithBearerSendsAuthorizationHeader(t *testing.T) {
	const token = "alertmanager-token-123"
	var seen string
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	provider, err := NewProvider(srv.URL, WithBearer(token))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seen != "Bearer "+token {
		t.Fatalf("Authorization = %q", seen)
	}
}

func TestProviderPropagatesRequestID(t *testing.T) {
	var seen string
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(correlation.RequestIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	provider, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := correlation.ContextWithRequestID(context.Background(), "request-alertmanager-1")
	if _, err := provider.ListActiveAlerts(ctx); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seen != "request-alertmanager-1" {
		t.Fatalf("%s = %q", correlation.RequestIDHeader, seen)
	}
}

func TestProviderRoundTripperDecoratorWrapsTransport(t *testing.T) {
	var seen string
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Test-Decorator")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	provider, err := NewProvider(srv.URL, WithRoundTripperDecorator(func(base http.RoundTripper) http.RoundTripper {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.Header = req.Header.Clone()
			clone.Header.Set("X-Test-Decorator", "applied")
			return base.RoundTrip(clone)
		})
	}))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.ListActiveAlerts(context.Background()); err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if seen != "applied" {
		t.Fatalf("X-Test-Decorator = %q, want applied", seen)
	}
}

func TestNewProviderRejectsUnsafeAddresses(t *testing.T) {
	credentialed := (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "credential-value"),
		Host:   "example.invalid",
	}).String()
	rawMarker := "opaque-marker"
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "userinfo", addr: credentialed, want: "must not include userinfo"},
		{name: "invalid scheme", addr: "ftp://example.invalid", want: "scheme must be http or https"},
		{name: "missing host", addr: "https:///api", want: "host must be non-empty"},
		{name: "query", addr: "https://example.invalid?x=1", want: "must not include query or fragment"},
		{name: "fragment", addr: "https://example.invalid#frag", want: "must not include query or fragment"},
		{name: "malformed", addr: "https://operator:%" + rawMarker + "@example.invalid", want: "valid URL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.addr)
			if err == nil {
				t.Fatal("NewProvider err = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewProvider err = %q, want substring %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "credential-value") || strings.Contains(err.Error(), rawMarker) {
				t.Fatalf("NewProvider error leaked credential marker: %v", err)
			}
		})
	}
}

func TestProviderListActiveAlertsRejectsBadResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: "upstream down", want: "status 500"},
		{name: "malformed", status: http.StatusOK, body: `[`, want: "validate response JSON"},
		{name: "duplicate key", status: http.StatusOK, body: `[{"labels":{"alertname":"old","alertname":"new"},"annotations":{},"startsAt":"2026-06-05T04:00:00Z","status":{"state":"active"}}]`, want: "duplicate object key"},
		{name: "object", status: http.StatusOK, body: `{"status":"success"}`, want: "decode alerts"},
		{name: "active missing startsAt", status: http.StatusOK, body: `[{"labels":{"alertname":"HighCPU"},"annotations":{},"status":{"state":"active"}}]`, want: "missing startsAt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newAlertmanagerServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			provider, err := NewProvider(srv.URL)
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = provider.ListActiveAlerts(context.Background())
			if err == nil {
				t.Fatal("ListActiveAlerts err = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ListActiveAlerts err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestProviderListActiveAlertsReturnsDecodedPrefixOnSemanticFailure(t *testing.T) {
	body := `[
		{"labels":{"alertname":"Valid"},"annotations":{},"startsAt":"2026-06-05T04:00:00Z","status":{"state":"active","silencedBy":[],"inhibitedBy":[],"mutedBy":[]}},
		{"labels":{"alertname":"Invalid"},"annotations":{},"status":{"state":"active","silencedBy":[],"inhibitedBy":[],"mutedBy":[]}}
	]`
	srv := newAlertmanagerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	provider, err := NewProvider(srv.URL)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	alerts, err := provider.ListActiveAlerts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing startsAt") {
		t.Fatalf("ListActiveAlerts error = %v, want missing startsAt", err)
	}
	if len(alerts) != 1 || alerts[0].Labels["alertname"] != "Valid" {
		t.Fatalf("partial alerts = %+v, want decoded valid prefix", alerts)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
