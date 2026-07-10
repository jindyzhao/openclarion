package netbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// #nosec G101 -- test-only placeholder, not a production credential.
const testV2Credential = "nbt_test-key.test-value"

type observedRequest struct {
	path          string
	query         url.Values
	authorization string
}

func TestLookupResourceMapsDevice(t *testing.T) {
	requests := make(chan observedRequest, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observedRequest{
			path:          r.URL.Path,
			query:         r.URL.Query(),
			authorization: r.Header.Get("Authorization"),
		}
		w.Header().Set("API-Version", "4.6.1")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/dcim/devices/":
			_, _ = io.WriteString(w, `{
				"count": 1,
				"next": null,
				"previous": null,
				"results": [{
					"id": 42,
					"name": "checkout-01",
					"role": {"id": 10, "name": "Application"},
					"tenant": {"id": 11, "name": "Acme"},
					"platform": {"id": 12, "name": "Linux"},
					"site": {"id": 1, "name": "Hong Kong"},
					"location": {"id": 2, "name": "DC1"},
					"rack": {"id": 3, "name": "R1"},
					"cluster": {"id": 4, "name": "Production"},
					"status": {"value": "active", "label": "Active"},
					"primary_ip": {"address": "10.0.0.5/24"},
					"tags": [{"name": "production"}, {"name": "critical"}, {"name": "production"}],
					"custom_fields": {
						"criticality": "high",
						"replicas": 3,
						"enabled": true,
						"ignored": {"private": "value"}
					}
				}]
			}`)
		case "/api/tenancy/contact-assignments/":
			_, _ = io.WriteString(w, `{
				"count": 3,
				"results": [
					{"id": 3, "contact": {"id": 11, "name": "Former Owner"}, "role": {"id": 5, "name": "Primary"}, "priority": "inactive"},
					{"id": 2, "contact": {"id": 9, "display": "SRE"}, "role": {"id": 5, "name": "Primary"}, "priority": {"value": "primary", "label": "Primary"}},
					{"id": 1, "contact": {"id": 7, "name": "Platform"}, "role": {"id": 6, "name": "Technical"}, "priority": "secondary"}
				]
			}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{
		BaseURL:               srv.URL,
		APIToken:              testV2Credential,
		LookupLabel:           "service",
		ObjectType:            ObjectTypeDevice,
		AttributeCustomFields: []string{"replicas", "criticality", "enabled"},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"service": "checkout-01"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	want := ports.CMDBLookupResult{
		Found: true,
		Resource: ports.CMDBResource{
			ID:   "netbox:dcim.device:42",
			Kind: "device",
			Name: "checkout-01",
			Owners: []ports.CMDBOwner{
				{Subject: "netbox:tenancy.contact:7", Team: "Platform", Role: "Technical"},
				{Subject: "netbox:tenancy.contact:9", Team: "SRE", Role: "Primary"},
			},
			Topology: []ports.CMDBTopologyLink{
				{Relation: "located_at", TargetID: "netbox:dcim.site:1", TargetKind: "dcim.site", TargetName: "Hong Kong"},
				{Relation: "located_at", TargetID: "netbox:dcim.location:2", TargetKind: "dcim.location", TargetName: "DC1"},
				{Relation: "installed_in", TargetID: "netbox:dcim.rack:3", TargetKind: "dcim.rack", TargetName: "R1"},
				{Relation: "member_of", TargetID: "netbox:virtualization.cluster:4", TargetKind: "virtualization.cluster", TargetName: "Production"},
			},
			Attributes: map[string]string{
				"netbox.id":                 "42",
				"netbox.object_type":        "dcim.device",
				"netbox.role":               "Application",
				"netbox.tenant":             "Acme",
				"netbox.platform":           "Linux",
				"netbox.site":               "Hong Kong",
				"netbox.location":           "DC1",
				"netbox.rack":               "R1",
				"netbox.cluster":            "Production",
				"netbox.status":             "active",
				"netbox.primary_ip":         "10.0.0.5/24",
				"netbox.tags":               "critical,production",
				"netbox.custom.criticality": "high",
				"netbox.custom.enabled":     "true",
				"netbox.custom.replicas":    "3",
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LookupResource result = %#v, want %#v", got, want)
	}

	deviceRequest := <-requests
	if deviceRequest.path != "/api/dcim/devices/" {
		t.Fatalf("first request path = %q", deviceRequest.path)
	}
	if deviceRequest.authorization != "Bearer "+testV2Credential {
		t.Fatalf("device authorization = %q", deviceRequest.authorization)
	}
	assertQueryValue(t, deviceRequest.query, "name", "checkout-01")
	assertQueryValue(t, deviceRequest.query, "limit", "2")
	assertQueryValue(t, deviceRequest.query, "exclude", "config_context")
	if fields := deviceRequest.query.Get("fields"); !strings.Contains(fields, "custom_fields") || !strings.Contains(fields, "primary_ip") {
		t.Fatalf("device fields = %q", fields)
	}

	contactRequest := <-requests
	if contactRequest.path != "/api/tenancy/contact-assignments/" {
		t.Fatalf("second request path = %q", contactRequest.path)
	}
	if contactRequest.authorization != "Bearer "+testV2Credential {
		t.Fatalf("contact authorization = %q", contactRequest.authorization)
	}
	assertQueryValue(t, contactRequest.query, "object_type", "dcim.device")
	assertQueryValue(t, contactRequest.query, "object_id", "42")
	assertQueryValue(t, contactRequest.query, "limit", "33")
	assertQueryValue(t, contactRequest.query, "fields", "id,contact,role,priority")
}

func TestLookupResourceMapsVirtualMachineWithLegacyToken(t *testing.T) {
	requests := make(chan observedRequest, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observedRequest{path: r.URL.Path, query: r.URL.Query(), authorization: r.Header.Get("Authorization")}
		w.Header().Set("API-Version", "4.5")
		switch r.URL.Path {
		case "/api/virtualization/virtual-machines/":
			_, _ = io.WriteString(w, `{
				"count": 1,
				"results": [{
					"id": 73,
					"display": "billing-vm",
					"site": {"id": 1, "name": "Hong Kong"},
					"cluster": {"id": 4, "name": "Production"},
					"device": {"id": 42, "name": "hypervisor-01"},
					"status": "active",
					"custom_fields": {}
				}]
			}`)
		case "/api/tenancy/contact-assignments/":
			_, _ = io.WriteString(w, `{"count": 0, "results": []}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{
		BaseURL:     srv.URL + "/api/",
		APIToken:    "legacy-test-value",
		LookupLabel: "instance",
		ObjectType:  ObjectTypeVirtualMachine,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "billing-vm"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if !got.Found || got.Resource.ID != "netbox:virtualization.virtualmachine:73" || got.Resource.Kind != "virtual_machine" {
		t.Fatalf("result = %+v", got)
	}
	if got.Resource.Attributes["netbox.status"] != "active" || got.Resource.Attributes["netbox.device"] != "hypervisor-01" {
		t.Fatalf("attributes = %+v", got.Resource.Attributes)
	}
	if len(got.Resource.Topology) != 3 || got.Resource.Topology[2].TargetID != "netbox:dcim.device:42" {
		t.Fatalf("topology = %+v", got.Resource.Topology)
	}
	for i := 0; i < 2; i++ {
		request := <-requests
		if request.authorization != "Token legacy-test-value" {
			t.Fatalf("request[%d] authorization = %q", i, request.authorization)
		}
		if i == 0 {
			fields := request.query.Get("fields")
			if !strings.Contains(fields, "display") || strings.Contains(fields, "custom_fields") {
				t.Fatalf("virtual machine fields = %q", fields)
			}
		}
	}
}

func TestLookupResourceReturnsNoMatchWithoutLookupLabel(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		_, _ = io.WriteString(w, `{"count": 0, "results": []}`)
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{BaseURL: srv.URL, LookupLabel: "service"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "checkout-01"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if got.Found || requestCount.Load() != 0 {
		t.Fatalf("result = %+v, request count = %d", got, requestCount.Load())
	}
}

func TestLookupResourceReturnsNoMatchFromNetBox(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != "/api/dcim/devices/" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"count":0,"results":[]}`)
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{
		BaseURL:     srv.URL,
		LookupLabel: "instance",
		ObjectType:  ObjectTypeDevice,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "missing"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if got.Found || requestCount.Load() != 1 {
		t.Fatalf("result = %+v, request count = %d", got, requestCount.Load())
	}
}

func TestLookupResourceAutoSelectsSingleObjectType(t *testing.T) {
	requests := make(chan string, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		switch r.URL.Path {
		case "/api/dcim/devices/":
			_, _ = io.WriteString(w, `{"count":0,"results":[]}`)
		case "/api/virtualization/virtual-machines/":
			_, _ = io.WriteString(w, `{"count":1,"results":[{"id":73,"name":"billing-vm"}]}`)
		case "/api/tenancy/contact-assignments/":
			_, _ = io.WriteString(w, `{"count":0,"results":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{BaseURL: srv.URL, LookupLabel: "instance"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "billing-vm"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if !got.Found || got.Resource.ID != "netbox:virtualization.virtualmachine:73" {
		t.Fatalf("result = %+v", got)
	}
	wantPaths := []string{
		"/api/dcim/devices/",
		"/api/virtualization/virtual-machines/",
		"/api/tenancy/contact-assignments/",
	}
	for i, want := range wantPaths {
		if gotPath := <-requests; gotPath != want {
			t.Fatalf("request[%d] path = %q, want %q", i, gotPath, want)
		}
	}
}

func TestLookupResourceRejectsCrossTypeAmbiguity(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		switch r.URL.Path {
		case "/api/dcim/devices/":
			_, _ = io.WriteString(w, `{"count":1,"results":[{"id":1,"name":"shared"}]}`)
		case "/api/virtualization/virtual-machines/":
			_, _ = io.WriteString(w, `{"count":1,"results":[{"id":2,"name":"shared"}]}`)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	provider, err := NewProvider(Config{BaseURL: srv.URL, LookupLabel: "instance"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "shared"},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple object types") {
		t.Fatalf("LookupResource error = %v", err)
	}
	if requestCount.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requestCount.Load())
	}
}

func TestLookupResourceRejectsInvalidUpstreamData(t *testing.T) {
	tests := []struct {
		name       string
		objectBody string
		ownerBody  string
		status     int
		apiVersion string
		custom     []string
		want       string
	}{
		{
			name:       "multiple objects",
			objectBody: `{"count":2,"results":[{"id":1,"name":"a"},{"id":2,"name":"b"}]}`,
			want:       "more than 1 result",
		},
		{
			name:       "invalid object id",
			objectBody: `{"count":1,"results":[{"id":0,"name":"a"}]}`,
			want:       "id must be positive",
		},
		{
			name:       "invalid topology id",
			objectBody: `{"count":1,"results":[{"id":1,"name":"a","site":{"id":0,"name":"invalid"}}]}`,
			want:       "target id must be positive",
		},
		{
			name:       "non scalar custom attribute",
			objectBody: `{"count":1,"results":[{"id":1,"name":"a","custom_fields":{"owner":{"name":"hidden"}}}]}`,
			custom:     []string{"owner"},
			want:       "must be a string, number, boolean, or null",
		},
		{
			name:       "invalid contact id",
			objectBody: `{"count":1,"results":[{"id":1,"name":"a"}]}`,
			ownerBody:  `{"count":1,"results":[{"contact":{"id":0,"name":"invalid"}}]}`,
			want:       "invalid contact id",
		},
		{
			name:       "invalid contact priority",
			objectBody: `{"count":1,"results":[{"id":1,"name":"a"}]}`,
			ownerBody:  `{"count":1,"results":[{"contact":{"id":1,"name":"owner"},"priority":{"label":"Inactive"}}]}`,
			want:       "invalid priority",
		},
		{
			name:       "too many contacts",
			objectBody: `{"count":1,"results":[{"id":1,"name":"a"}]}`,
			ownerBody:  `{"count":33,"results":[]}`,
			want:       "more than 32 result",
		},
		{
			name:       "old API version",
			objectBody: `{"count":0,"results":[]}`,
			status:     http.StatusBadRequest,
			apiVersion: "4.5.1",
			want:       "unsupported",
		},
		{
			name:       "malformed API version",
			objectBody: `{"count":0,"results":[]}`,
			apiVersion: "4.x",
			want:       "API-Version header is invalid",
		},
		{
			name:       "HTTP failure",
			objectBody: `{"detail":"unavailable"}`,
			status:     http.StatusServiceUnavailable,
			want:       "HTTP 503",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.apiVersion != "" {
					w.Header().Set("API-Version", tc.apiVersion)
				}
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				if r.URL.Path == "/api/tenancy/contact-assignments/" {
					_, _ = io.WriteString(w, tc.ownerBody)
					return
				}
				_, _ = io.WriteString(w, tc.objectBody)
			}))
			defer srv.Close()

			provider, err := NewProvider(Config{
				BaseURL:               srv.URL,
				LookupLabel:           "instance",
				ObjectType:            ObjectTypeDevice,
				AttributeCustomFields: tc.custom,
			})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
				Labels: map[string]string{"instance": "a"},
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LookupResource error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLookupResourceHonorsCanceledContext(t *testing.T) {
	var requestCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		_, _ = io.WriteString(w, `{"count":0,"results":[]}`)
	}))
	defer srv.Close()
	provider, err := NewProvider(Config{BaseURL: srv.URL, LookupLabel: "instance"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.LookupResource(ctx, ports.CMDBLookupRequest{Labels: map[string]string{"instance": "a"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LookupResource error = %v, want context.Canceled", err)
	}
	if requestCount.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requestCount.Load())
	}
}

func TestLookupResourceDoesNotFollowRedirects(t *testing.T) {
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		_, _ = io.WriteString(w, `{"count":0,"results":[]}`)
	}))
	defer redirectTarget.Close()

	redirectSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/api/dcim/devices/", http.StatusFound)
	}))
	defer redirectSource.Close()

	provider, err := NewProvider(Config{
		BaseURL:     redirectSource.URL,
		APIToken:    testV2Credential,
		LookupLabel: "instance",
		ObjectType:  ObjectTypeDevice,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "checkout-01"},
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("LookupResource error = %v, want HTTP 302", err)
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("redirect target requests = %d, want 0", redirectedRequests.Load())
	}
}

func TestLookupResourceSanitizesTransportErrors(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("test transport failure")
	})}
	provider, err := NewProvider(Config{
		BaseURL:     "https://netbox.example.test",
		LookupLabel: "instance",
		ObjectType:  ObjectTypeDevice,
		HTTPClient:  client,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"instance": "sensitive-instance"},
	})
	if err == nil || !strings.Contains(err.Error(), "test transport failure") {
		t.Fatalf("LookupResource error = %v", err)
	}
	if strings.Contains(err.Error(), "sensitive-instance") || strings.Contains(err.Error(), "netbox.example.test") {
		t.Fatalf("LookupResource error exposes request URL: %v", err)
	}
}

func TestDecodePageRejectsAmbiguousOrMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "duplicate key", raw: `{"count":0,"count":1,"results":[]}`, want: "duplicate object key"},
		{name: "trailing value", raw: `{"count":0,"results":[]} {}`, want: "trailing JSON values"},
		{name: "missing count", raw: `{"results":[]}`, want: "count must be a non-null integer"},
		{name: "null count", raw: `{"count":null,"results":[]}`, want: "count must be a non-null integer"},
		{name: "negative count", raw: `{"count":-1,"results":[]}`, want: "count must be non-negative"},
		{name: "count mismatch", raw: `{"count":1,"results":[]}`, want: "count does not match"},
		{name: "wrong count type", raw: `{"count":"0","results":[]}`, want: "cannot unmarshal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodePage[deviceResponse]([]byte(tc.raw), 1)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("decodePage error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestReadBoundedBodyRejectsOversizedResponse(t *testing.T) {
	_, err := readBoundedBody(strings.NewReader(strings.Repeat("x", maxResponseBodySize+1)))
	if err == nil || !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("readBoundedBody error = %v", err)
	}
}

func TestAddTagsAttributeBoundsValue(t *testing.T) {
	prefix := []tagReference{
		{Name: strings.Repeat("a", 100)},
		{Name: strings.Repeat("b", 100)},
	}
	tests := []struct {
		name        string
		lastBytes   int
		wantPresent bool
	}{
		{name: "at limit", lastBytes: 54, wantPresent: true},
		{name: "over limit", lastBytes: 55},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attributes := map[string]string{}
			tags := append(append([]tagReference(nil), prefix...), tagReference{Name: strings.Repeat("c", tc.lastBytes)})
			addTagsAttribute(attributes, tags)
			value, present := attributes["netbox.tags"]
			if present != tc.wantPresent {
				t.Fatalf("netbox.tags present = %t, want %t (value %q)", present, tc.wantPresent, value)
			}
			if present && len(value) != 256 {
				t.Fatalf("len(netbox.tags) = %d, want 256", len(value))
			}
		})
	}
}

func TestNewProviderValidatesConfiguration(t *testing.T) {
	valid := func() Config {
		return Config{BaseURL: "https://netbox.example.test", LookupLabel: "instance"}
	}
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{name: "empty URL", mutate: func(c *Config) { c.BaseURL = "" }, want: "scheme must be http or https"},
		{name: "unsupported URL scheme", mutate: func(c *Config) { c.BaseURL = "file:///tmp/netbox" }, want: "scheme must be http or https"},
		{name: "missing URL host", mutate: func(c *Config) { c.BaseURL = "https:///netbox" }, want: "host must be non-empty"},
		{name: "URL userinfo", mutate: func(c *Config) { c.BaseURL = "https://user@example.test" }, want: "must not include userinfo"},
		{name: "URL query", mutate: func(c *Config) { c.BaseURL += "?token=value" }, want: "must not include query or fragment"},
		{name: "URL empty query marker", mutate: func(c *Config) { c.BaseURL += "?" }, want: "must not include query or fragment"},
		{name: "missing lookup label", mutate: func(c *Config) { c.LookupLabel = "" }, want: "lookup label must be non-empty"},
		{name: "invalid lookup label", mutate: func(c *Config) { c.LookupLabel = "9instance" }, want: "unsupported characters"},
		{name: "invalid lookup filter", mutate: func(c *Config) { c.LookupFilter = "name[]" }, want: "unsupported characters"},
		{name: "reserved lookup filter", mutate: func(c *Config) { c.LookupFilter = "limit" }, want: "is reserved"},
		{name: "brief lookup filter", mutate: func(c *Config) { c.LookupFilter = "brief" }, want: "is reserved"},
		{name: "omit lookup filter", mutate: func(c *Config) { c.LookupFilter = "omit" }, want: "is reserved"},
		{name: "format lookup filter", mutate: func(c *Config) { c.LookupFilter = "format" }, want: "is reserved"},
		{name: "lookup filter whitespace", mutate: func(c *Config) { c.LookupFilter = " name" }, want: "leading or trailing whitespace"},
		{name: "invalid object type", mutate: func(c *Config) { c.ObjectType = "server" }, want: "object type must be"},
		{name: "invalid custom field", mutate: func(c *Config) { c.AttributeCustomFields = []string{"Owner"} }, want: "is invalid"},
		{name: "duplicate custom field", mutate: func(c *Config) { c.AttributeCustomFields = []string{"owner", "owner"} }, want: "is duplicated"},
		{name: "too many custom fields", mutate: func(c *Config) { c.AttributeCustomFields = make([]string, maxCustomAttributeFields+1) }, want: "exceed 32 entries"},
		{name: "token whitespace", mutate: func(c *Config) { c.APIToken = " token" }, want: "leading or trailing whitespace"},
		{name: "token control", mutate: func(c *Config) { c.APIToken = "test\nvalue" }, want: "whitespace or control"},
		{name: "token too large", mutate: func(c *Config) { c.APIToken = strings.Repeat("x", maxTokenBytes+1) }, want: "exceeds 4096 bytes"},
		{name: "scheme without token", mutate: func(c *Config) { c.TokenScheme = TokenSchemeBearer }, want: "requires an API token"},
		{name: "invalid scheme", mutate: func(c *Config) { c.APIToken = "value"; c.TokenScheme = "basic" }, want: "scheme must be auto, bearer, or token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid()
			tc.mutate(&cfg)
			_, err := NewProvider(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewProvider error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestNormalizeAPIBaseURLPreservesDeploymentPrefix(t *testing.T) {
	base, err := normalizeAPIBaseURL("https://netbox.example.test/platform/netbox")
	if err != nil {
		t.Fatalf("normalizeAPIBaseURL: %v", err)
	}
	got := joinAPIEndpoint(base, "dcim", "devices")
	if got != "https://netbox.example.test/platform/netbox/api/dcim/devices/" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestValidateAPIVersion(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{value: ""},
		{value: "4.5"},
		{value: "4.5.2"},
		{value: "4.6"},
		{value: "4.6.1"},
		{value: "3.7", wantErr: true},
		{value: "4", wantErr: true},
		{value: "4.5.1", wantErr: true},
		{value: "5.0", wantErr: true},
		{value: "4.", wantErr: true},
		{value: "4.6.1.2", wantErr: true},
		{value: "v4.6", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("version_%s", tc.value), func(t *testing.T) {
			err := validateAPIVersion(tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateAPIVersion(%q) error = %v", tc.value, err)
			}
		})
	}
}

func assertQueryValue(t *testing.T, query url.Values, key, want string) {
	t.Helper()
	if got := query.Get(key); got != want {
		t.Fatalf("query[%q] = %q, want %q", key, got, want)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
