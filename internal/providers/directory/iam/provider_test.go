package iam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	fakeClientID    = "openclarion"
	fakeClientProof = "fixture-value"
	fakeAccessToken = "directory-access-fixture"
)

func TestProviderListsDirectoryRecordsWithClientCredentials(t *testing.T) {
	var sawTokenRequest bool
	var sawUserRequest bool
	var sawDepartmentRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case discoveryPath:
			writeJSON(t, w, map[string]string{
				"issuer":         "http://" + r.Host,
				"token_endpoint": "http://" + r.Host + tokenPath,
			})
		case tokenPath:
			sawTokenRequest = true
			clientID, clientSecret, ok := r.BasicAuth()
			if !ok || clientID != fakeClientID || clientSecret != fakeClientProof {
				t.Fatalf("unexpected token auth: id=%q ok=%v", clientID, ok)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "client_credentials" {
				t.Fatalf("grant_type = %q", got)
			}
			if got := r.Form.Get("scope"); got != defaultScope {
				t.Fatalf("scope = %q", got)
			}
			writeJSON(t, w, map[string]any{
				"access_token": fakeAccessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case usersPath:
			sawUserRequest = true
			if got := r.Header.Get("Authorization"); got != "Bearer "+fakeAccessToken {
				t.Fatalf("user authorization = %q", got)
			}
			if got := r.URL.Query().Get("p"); got != "2" {
				t.Fatalf("user page = %q", got)
			}
			if got := r.URL.Query().Get("pageSize"); got != "50" {
				t.Fatalf("user pageSize = %q", got)
			}
			if got := r.URL.Query().Get("include_disabled"); got != "true" {
				t.Fatalf("user include_disabled = %q", got)
			}
			if got := r.URL.Query().Get("updated_after"); got == "" {
				t.Fatalf("updated_after missing")
			}
			active := true
			inactive := false
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"msg":    "",
				"data": map[string]any{
					"items": []any{
						map[string]any{
							"sub":                "iam-subject-1",
							"name":               "alice",
							"preferred_username": "alice",
							"displayName":        "Alice",
							"email":              "alice@example.test",
							"job_title":          "SRE",
							"department":         "IT",
							"section":            "Platform/SRE",
							"department_path":    "IT/Platform/SRE",
							"department_paths":   []string{"IT/Platform/SRE", "IT/Shared"},
							"department_ids":     []string{"dep-2", "dep-1"},
							"department_external_ids": []string{
								"dep-1",
								"legacy-dep",
							},
							"wecom_userid": "wecom-alice",
							"active":       active,
							"updated_at":   "2026-06-26T12:00:00+08:00",
						},
						map[string]any{
							"sub":                "iam-subject-2",
							"name":               "bob",
							"preferred_username": "bob",
							"displayName":        "Bob",
							"department_ids":     []string{"dep-2"},
							"active":             inactive,
							"updated_at":         "2026-06-26T12:30:00+08:00",
						},
					},
					"page":     2,
					"pageSize": 50,
					"hasMore":  true,
				},
			})
		case departmentsPath:
			sawDepartmentRequest = true
			if got := r.Header.Get("Authorization"); got != "Bearer "+fakeAccessToken {
				t.Fatalf("department authorization = %q", got)
			}
			if got := r.URL.Query().Get("include_disabled"); got != "true" {
				t.Fatalf("department include_disabled = %q", got)
			}
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"msg":    "",
				"data": map[string]any{
					"items": []any{map[string]any{
						"id":           "dep-2",
						"name":         "SRE",
						"displayName":  "SRE",
						"path":         "IT/Platform/SRE",
						"parent_id":    "dep-1",
						"parent_path":  "IT/Platform",
						"level":        3,
						"source":       "wecom",
						"member_count": 4,
						"updated_at":   "2026-06-26T12:00:00+08:00",
					}},
					"page":     1,
					"pageSize": 50,
					"hasMore":  false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(context.Background(), Config{
		IssuerURL:    server.URL,
		ClientID:     fakeClientID,
		ClientSecret: "  " + fakeClientProof + "\n",
		HTTPClient:   server.Client(),
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	updatedAfter := time.Date(2026, 6, 26, 1, 2, 3, 0, time.UTC)
	users, err := provider.ListUsers(context.Background(), directoryRequest("2", 50, &updatedAfter))
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users.Users) != 2 || users.Users[0].Subject != "iam-subject-1" || users.Users[0].ExternalID != "wecom-alice" || !users.Users[0].Active {
		t.Fatalf("users = %+v", users)
	}
	if users.NextCursor != "3" {
		t.Fatalf("NextCursor = %q, want 3", users.NextCursor)
	}
	if len(users.Users[0].DepartmentPaths) != 2 || users.Users[0].DepartmentPaths[1] != "IT/Shared" {
		t.Fatalf("department_paths = %+v", users.Users[0].DepartmentPaths)
	}
	if got := users.Users[0].DepartmentExternalIDs; len(got) != 3 || got[0] != "dep-2" || got[1] != "dep-1" || got[2] != "legacy-dep" {
		t.Fatalf("department ids = %+v", got)
	}
	if users.Users[1].Subject != "iam-subject-2" || users.Users[1].ExternalID != "iam-subject-2" || users.Users[1].Active {
		t.Fatalf("inactive user = %+v", users.Users[1])
	}
	departments, err := provider.ListDepartments(context.Background(), directoryRequest("", 50, nil))
	if err != nil {
		t.Fatalf("ListDepartments: %v", err)
	}
	if len(departments.Departments) != 1 || departments.Departments[0].ExternalID != "dep-2" || departments.Departments[0].ParentExternalID != "dep-1" {
		t.Fatalf("departments = %+v", departments)
	}
	if !sawTokenRequest || !sawUserRequest || !sawDepartmentRequest {
		t.Fatalf("requests token=%v user=%v department=%v", sawTokenRequest, sawUserRequest, sawDepartmentRequest)
	}
}

func TestProviderTokenFetchOutlivesSetupContext(t *testing.T) {
	var sawTokenRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case discoveryPath:
			writeJSON(t, w, map[string]string{
				"issuer":         "http://" + r.Host,
				"token_endpoint": "http://" + r.Host + tokenPath,
			})
		case tokenPath:
			sawTokenRequest = true
			writeJSON(t, w, map[string]any{
				"access_token": fakeAccessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case usersPath:
			if got := r.URL.Query().Get("pageSize"); got != "100" {
				t.Fatalf("user pageSize = %q, want default 100", got)
			}
			active := true
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"data": map[string]any{
					"items": []any{map[string]any{
						"sub":                "iam-subject-1",
						"preferred_username": "alice",
						"active":             active,
					}},
					"page":     1,
					"pageSize": 100,
					"hasMore":  false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupCtx, cancel := context.WithCancel(context.Background())
	provider, err := NewProvider(setupCtx, Config{
		IssuerURL:    server.URL,
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient:   server.Client(),
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	cancel()

	users, err := provider.ListUsers(context.Background(), directoryRequest("", 0, nil))
	if err != nil {
		t.Fatalf("ListUsers after setup context cancellation: %v", err)
	}
	if !sawTokenRequest {
		t.Fatal("token request was not observed")
	}
	if len(users.Users) != 1 || users.Users[0].Subject != "iam-subject-1" {
		t.Fatalf("users = %+v", users)
	}
}

func TestProviderTokenFetchUsesListContext(t *testing.T) {
	tokenStarted := make(chan struct{}, 1)
	provider, err := NewProvider(context.Background(), Config{
		IssuerURL:    "http://iam.example.test",
		TokenURL:     "http://iam.example.test" + tokenPath,
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient: &http.Client{
			Transport: tokenContextRoundTripper{t: t, started: tokenStarted},
		},
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = provider.ListUsers(ctx, directoryRequest("", 100, nil))
	if err == nil {
		t.Fatal("ListUsers succeeded, want token context timeout")
	}
	select {
	case <-tokenStarted:
	default:
		t.Fatal("token request did not start")
	}
}

func TestProviderRejectsInvalidPageRequest(t *testing.T) {
	provider := &Provider{
		client:       &http.Client{Transport: failingRoundTripper{t: t}},
		directoryURL: mustURL(t, "http://example.test"),
	}

	tests := []ports.DirectoryListRequest{
		directoryRequest("not-a-page", 100, nil),
		directoryRequest("0", 100, nil),
		directoryRequest("", -1, nil),
		directoryRequest("", maxPageSize+1, nil),
	}
	for _, req := range tests {
		_, err := provider.ListUsers(context.Background(), req)
		if err == nil {
			t.Fatalf("ListUsers(%+v) succeeded, want page request validation error", req)
		}
	}
}

func TestProviderRejectsUserRecordWithoutActiveField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case tokenPath:
			writeJSON(t, w, map[string]any{
				"access_token": fakeAccessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case usersPath:
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"data": map[string]any{
					"items": []any{map[string]any{
						"sub":                "iam-subject-1",
						"preferred_username": "alice",
					}},
					"page":     1,
					"pageSize": 100,
					"hasMore":  false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(context.Background(), Config{
		IssuerURL:    server.URL,
		TokenURL:     server.URL + tokenPath,
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient:   server.Client(),
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = provider.ListUsers(context.Background(), directoryRequest("", 100, nil))
	if err == nil {
		t.Fatalf("ListUsers succeeded, want missing active field error")
	}
}

func TestProviderRejectsDiscoveryIssuerMismatch(t *testing.T) {
	var sawTokenRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case discoveryPath:
			writeJSON(t, w, map[string]string{
				"issuer":         "http://" + r.Host + "/other",
				"token_endpoint": "http://" + r.Host + tokenPath,
			})
		case tokenPath:
			sawTokenRequest = true
			writeJSON(t, w, map[string]any{
				"access_token": fakeAccessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := NewProvider(context.Background(), Config{
		IssuerURL:    server.URL,
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient:   server.Client(),
	})
	if err == nil {
		t.Fatal("NewProvider succeeded, want discovery issuer mismatch")
	}
	if sawTokenRequest {
		t.Fatal("token endpoint was called after discovery issuer mismatch")
	}
}

func TestProviderPreservesDiscoveredTokenEndpointQuery(t *testing.T) {
	var sawTokenRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case discoveryPath:
			writeJSON(t, w, map[string]string{
				"issuer":         "http://" + r.Host,
				"token_endpoint": "http://" + r.Host + tokenPath + "?tenant=ops&route=primary",
			})
		case tokenPath:
			sawTokenRequest = true
			if got := r.URL.Query().Get("tenant"); got != "ops" {
				t.Fatalf("token tenant query = %q", got)
			}
			if got := r.URL.Query().Get("route"); got != "primary" {
				t.Fatalf("token route query = %q", got)
			}
			writeJSON(t, w, map[string]any{
				"access_token": fakeAccessToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case usersPath:
			active := true
			writeJSON(t, w, map[string]any{
				"status": "ok",
				"data": map[string]any{
					"items": []any{map[string]any{
						"sub":                "iam-subject-1",
						"preferred_username": "alice",
						"active":             active,
					}},
					"page":     1,
					"pageSize": 100,
					"hasMore":  false,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(context.Background(), Config{
		IssuerURL:    server.URL,
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient:   server.Client(),
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, err := provider.ListUsers(context.Background(), directoryRequest("", 100, nil)); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if !sawTokenRequest {
		t.Fatal("token request was not observed")
	}
}

func TestProviderRejectsTokenEndpointFragment(t *testing.T) {
	_, err := NewProvider(context.Background(), Config{
		IssuerURL:    "http://iam.example.test",
		TokenURL:     "http://iam.example.test" + tokenPath + "#fragment",
		ClientID:     fakeClientID,
		ClientSecret: fakeClientProof,
		HTTPClient:   &http.Client{Transport: failingRoundTripper{t: t}},
	})
	if err == nil {
		t.Fatal("NewProvider succeeded, want token endpoint fragment error")
	}
}

func directoryRequest(cursor string, pageSize int, updatedAfter *time.Time) ports.DirectoryListRequest {
	return ports.DirectoryListRequest{Cursor: cursor, PageSize: pageSize, UpdatedAfter: updatedAfter}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return parsed
}

type failingRoundTripper struct {
	t *testing.T
}

func (rt failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	rt.t.Helper()
	rt.t.Fatal("unexpected HTTP request")
	return nil, nil
}

type tokenContextRoundTripper struct {
	t       *testing.T
	started chan<- struct{}
}

func (rt tokenContextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.t.Helper()
	if req.URL.Path != tokenPath {
		rt.t.Fatalf("unexpected token request path: %s", req.URL.Path)
	}
	select {
	case rt.started <- struct{}{}:
	default:
	}
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(time.Second):
		rt.t.Fatal("token request context was not cancelled")
		return nil, context.DeadlineExceeded
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
