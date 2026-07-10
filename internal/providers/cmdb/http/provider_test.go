package httpcmdb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const testHeaderValue = "value"

func TestLookupResourcePostsLabelsAndDecodesResource(t *testing.T) {
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testHeaderValue {
			t.Fatalf("authorization = %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(raw) != `{"labels":{"cluster":"prod","service":"checkout"}}` {
			t.Fatalf("body = %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"found": true,
			"resource": {
				"id": "service/checkout",
				"kind": "service",
				"name": "Checkout API",
				"owners": [{"subject": "team-checkout", "team": "Checkout", "role": "primary"}],
				"topology": [{"relation": "depends_on", "target_id": "database/postgres", "target_kind": "database", "target_name": "Checkout DB"}],
				"attributes": {"tier": "frontend"}
			}
		}`)
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{URL: srv.URL, BearerToken: testHeaderValue})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"service": "checkout", "cluster": "prod"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("requestCount = %d, want 1", requestCount)
	}
	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	if got.Resource.ID != "service/checkout" || got.Resource.Kind != "service" || got.Resource.Name != "Checkout API" {
		t.Fatalf("resource = %+v", got.Resource)
	}
	if len(got.Resource.Owners) != 1 || got.Resource.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owners = %+v", got.Resource.Owners)
	}
	if len(got.Resource.Topology) != 1 || got.Resource.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology = %+v", got.Resource.Topology)
	}
	if got.Resource.Attributes["tier"] != "frontend" {
		t.Fatalf("attributes = %+v", got.Resource.Attributes)
	}
}

func TestLookupResourceNoMatch(t *testing.T) {
	provider := newHTTPTestProvider(t, `{"found": false}`, http.StatusOK)
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if got.Found {
		t.Fatalf("Found = true, want false")
	}
}

func TestLookupResourceAllowsEmptyLabelValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(raw) != `{"labels":{"service":""}}` {
			t.Fatalf("body = %s", raw)
		}
		_, _ = io.WriteString(w, `{"found": false}`)
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"service": ""},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if got.Found {
		t.Fatalf("Found = true, want false")
	}
}

func TestLookupResourceRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate key",
			body: `{"found": false, "found": true}`,
			want: `duplicate object key "found"`,
		},
		{
			name: "unknown field",
			body: `{"found": false, "unexpected": true}`,
			want: "unknown field",
		},
		{
			name: "trailing value",
			body: `{"found": false} []`,
			want: "trailing JSON values",
		},
		{
			name: "missing found",
			body: `{}`,
			want: "response must include found",
		},
		{
			name: "null response",
			body: `null`,
			want: "response must include found",
		},
		{
			name: "null found",
			body: `{"found": null}`,
			want: "response must include found",
		},
		{
			name: "found without resource",
			body: `{"found": true}`,
			want: "found=true response must include resource",
		},
		{
			name: "not found with resource",
			body: `{"found": false, "resource": {"id": "service/checkout"}}`,
			want: "found=false response must not include resource",
		},
		{
			name: "invalid resource",
			body: `{"found": true, "resource": {"id": "service/checkout", "kind": "service", "name": "Checkout", "owners": [{"role": "primary"}]}}`,
			want: "subject or team must be non-empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newHTTPTestProvider(t, tt.body, http.StatusOK)
			_, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
			if err == nil {
				t.Fatal("LookupResource err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestLookupResourceRejectsOversizedResponse(t *testing.T) {
	provider := newHTTPTestProvider(t, strings.Repeat(" ", maxResponseBodySize+1), http.StatusOK)
	_, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if err == nil {
		t.Fatal("LookupResource err = nil, want error")
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLookupResourceReturnsHTTPStatusError(t *testing.T) {
	provider := newHTTPTestProvider(t, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
	_, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if err == nil {
		t.Fatal("LookupResource err = nil, want error")
	}
	if !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLookupResourceRejectsRedirects(t *testing.T) {
	var followed atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lookup":
			http.Redirect(w, r, "/target", http.StatusTemporaryRedirect)
		case "/target":
			followed.Store(true)
			_, _ = io.WriteString(w, `{"found":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{
		URL:         srv.URL + "/lookup",
		BearerToken: testHeaderValue,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("LookupResource err = %v, want HTTP 307 rejection", err)
	}
	if followed.Load() {
		t.Fatal("redirect target was called; bearer-authenticated CMDB requests must not follow redirects")
	}
}

func TestLookupResourceSanitizesRequestURLFromErrors(t *testing.T) {
	const endpoint = "https://cmdb.internal.example/private/lookup"
	provider, err := NewProvider(Config{
		URL: endpoint,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial backend failed")
		})},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if err == nil || !strings.Contains(err.Error(), "dial backend failed") {
		t.Fatalf("LookupResource err = %v, want sanitized transport failure", err)
	}
	if strings.Contains(err.Error(), endpoint) || strings.Contains(err.Error(), "cmdb.internal.example") {
		t.Fatalf("LookupResource err = %q, want endpoint removed", err)
	}
}

func TestLookupResourceRejectsInvalidLookupLabels(t *testing.T) {
	labels := map[string]string{}
	for i := 0; i < maxRequestLabels+1; i++ {
		labels[fmt.Sprintf("label_%03d", i)] = "value"
	}
	provider := newHTTPTestProvider(t, `{"found": false}`, http.StatusOK)
	_, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: labels})
	if err == nil {
		t.Fatal("LookupResource err = nil, want error")
	}
	if !strings.Contains(err.Error(), "lookup labels exceed") {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLookupResourceHonorsContextCancellation(t *testing.T) {
	provider := newHTTPTestProvider(t, `{"found": false}`, http.StatusOK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.LookupResource(ctx, ports.CMDBLookupRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNewProviderRejectsUnsafeEndpoints(t *testing.T) {
	fragmentURL := "https://cmdb.example.invalid/lookup" + string(rune(35)) + "x"
	tests := []string{
		"",
		"ftp://cmdb.example.invalid/lookup",
		"http://user:pw@cmdb.example.invalid/lookup",
		"https://cmdb.example.invalid/lookup?debug=value",
		fragmentURL,
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			_, err := NewProvider(Config{URL: raw})
			if err == nil {
				t.Fatal("NewProvider err = nil, want error")
			}
		})
	}
}

func newHTTPTestProvider(t *testing.T, body string, status int) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	provider, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return provider
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
