package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ldapStatusBody() map[string]any {
	return map[string]any{
		"configured": true,
		"mode":       "ldap",
		"role_mapping": map[string]any{
			"admin_mapping_count": 0,
			"configured":          true,
			"default_roles":       []string{},
			"owner_mapping_count": 1,
		},
	}
}

func TestRunWritesLDAPProofWithoutCredentials(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_USERNAME", "operator-1")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			if r.Method != http.MethodGet {
				t.Fatalf("status method = %s, want GET", r.Method)
			}
			writeJSON(t, w, ldapStatusBody())
		case "/api/v1/diagnosis/auth/check":
			if r.Method != http.MethodPost {
				t.Fatalf("check method = %s, want POST", r.Method)
			}
			gotAuth = r.Header.Get("Authorization")
			writeJSON(t, w, map[string]any{
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username-env", "OPENCLARION_TEST_LDAP_USERNAME",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--expected-backend-mode", "ldap",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotAuth == "" || !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("authorization header = %q, want Basic", gotAuth)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	if strings.Contains(string(raw), "fixture-password") ||
		strings.Contains(string(raw), "OPENCLARION_TEST_LDAP_PASSWORD") ||
		strings.Contains(string(raw), "operator-1:") ||
		strings.Contains(string(raw), gotAuth) {
		t.Fatalf("proof leaked credentials: %s", raw)
	}
	var out proof
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.AuthMode != "ldap" ||
		!out.Passed ||
		out.Status.Mode != "ldap" ||
		out.Status.RoleMapping == nil ||
		!out.Status.RoleMapping.Configured ||
		out.Status.RoleMapping.OwnerMappingCount != 1 ||
		out.Check.Subject != "operator-1" ||
		!out.Check.RoleAuthorized ||
		out.Check.RoleCount != 1 ||
		out.Evidence != "diagnosis_auth_check success:ldap:ldap:owner" {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunWritesLDAPSessionProofWithoutToken(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 1, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_USERNAME", "operator-1")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	var checkAuth string
	var sessionAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, ldapStatusBody())
		case "/api/v1/diagnosis/auth/check":
			checkAuth = r.Header.Get("Authorization")
			writeJSON(t, w, map[string]any{
				"checked_at":      "2026-06-21T15:01:00Z",
				"mode":            "ldap",
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		case "/api/v1/diagnosis/auth/session":
			sessionAuth = r.Header.Get("Authorization")
			writeJSONStatus(t, w, http.StatusCreated, map[string]any{
				"checked_at":      "2026-06-21T15:01:00Z",
				"expires_at":      "2026-06-21T23:01:00Z",
				"mode":            "ldap",
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
				// #nosec G101 -- test-only session token fixture.
				"token": "ldap.session.token",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username-env", "OPENCLARION_TEST_LDAP_USERNAME",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--expected-backend-mode", "ldap",
		"--issue-session",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if checkAuth == "" || checkAuth != sessionAuth {
		t.Fatalf("check auth = %q, session auth = %q, want same Basic authorization", checkAuth, sessionAuth)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	for _, leaked := range []string{
		"fixture-password",
		"OPENCLARION_TEST_LDAP_PASSWORD",
		"operator-1:",
		"ldap.session.token",
		checkAuth,
	} {
		if leaked != "" && strings.Contains(string(raw), leaked) {
			t.Fatalf("proof leaked %q: %s", leaked, raw)
		}
	}
	var out proof
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Session == nil ||
		!out.Request.IssueSession ||
		out.Session.HTTPStatus != http.StatusCreated ||
		!out.Session.TokenPresent ||
		out.Session.Mode != "ldap" ||
		out.Session.Subject != "operator-1" ||
		out.Session.RoleCount != 1 ||
		out.Session.CheckedAt != "2026-06-21T15:01:00Z" ||
		out.Session.ExpiresAt != "2026-06-21T23:01:00Z" ||
		out.Evidence != "diagnosis_auth_check success:ldap:ldap:ldap:ldap:owner:session" {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunWritesBearerProofWithoutToken(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 5, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_BEARER_TOKEN", "fixture-token")

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, map[string]any{"configured": true, "mode": "static"})
		case "/api/v1/diagnosis/auth/check":
			gotAuth = r.Header.Get("Authorization")
			writeJSON(t, w, map[string]any{
				"role_authorized": true,
				"roles":           []string{"admin", "owner"},
				"subject":         "local-operator",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "bearer",
		"--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN",
		"--expected-backend-mode", "static",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotAuth != "Bearer fixture-token" {
		t.Fatalf("authorization header = %q, want bearer token", gotAuth)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	if strings.Contains(string(raw), "fixture-token") {
		t.Fatalf("proof leaked bearer token: %s", raw)
	}
	if strings.Contains(string(raw), "OPENCLARION_TEST_BEARER_TOKEN") {
		t.Fatalf("proof leaked bearer env name: %s", raw)
	}
	var out proof
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if !out.Passed ||
		!out.Check.RoleAuthorized ||
		out.Evidence != "diagnosis_auth_check success:static:bearer:admin,owner" {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunRejectsLDAPStatusWithoutRoleMappingProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 6, 30, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, map[string]any{"configured": true, "mode": "ldap"})
		case "/api/v1/diagnosis/auth/check":
			writeJSON(t, w, map[string]any{
				"mode":            "ldap",
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username", "operator-1",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want missing role mapping proof failure")
	}
	if !strings.Contains(err.Error(), "status.role_mapping") {
		t.Fatalf("error = %v, want role mapping failure", err)
	}
}

func TestRunRejectsLegacyWeComSessionAuthMode(t *testing.T) {
	err := run([]string{
		"--api-base-url", "http://127.0.0.1:32101",
		"--auth-mode", "wecom_session",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, unusedHTTPClient(t))
	if err == nil {
		t.Fatal("run succeeded; want legacy WeCom session auth mode failure")
	}
	if !strings.Contains(err.Error(), "--auth-mode must be ldap or bearer") {
		t.Fatalf("error = %v, want legacy auth mode failure", err)
	}
}

func TestRunRejectsUnauthorizedDiagnosisRoleCheck(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 7, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, ldapStatusBody())
		case "/api/v1/diagnosis/auth/check":
			writeJSON(t, w, map[string]any{
				"role_authorized": false,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username", "operator-1",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want role authorization failure")
	}
	if !strings.Contains(err.Error(), "role_authorized") {
		t.Fatalf("error = %v, want role_authorized failure", err)
	}
}

func TestRunRejectsBackendModeMismatch(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 10, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, map[string]any{"configured": true, "mode": "static"})
		case "/api/v1/diagnosis/auth/check":
			writeJSON(t, w, map[string]any{
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username", "operator-1",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want backend mode mismatch")
	}
	if !strings.Contains(err.Error(), `want ldap`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsMissingRequiredSupportedMode(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 11, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnosis/auth/status":
			writeJSON(t, w, map[string]any{
				"configured":      true,
				"mode":            "static",
				"supported_modes": []string{"static"},
			})
		case "/api/v1/diagnosis/auth/check":
			writeJSON(t, w, map[string]any{
				"mode":            "ldap",
				"role_authorized": true,
				"roles":           []string{"owner"},
				"subject":         "operator-1",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username", "operator-1",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--required-supported-modes", "ldap,oidc",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want missing supported mode failure")
	}
	if !strings.Contains(err.Error(), `missing required mode "ldap"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsLegacyWeComLoginEntryFlag(t *testing.T) {
	_, err := parseArgs([]string{
		"--api-base-url", "http://127.0.0.1:32101",
		"--auth-mode", "bearer",
		"--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN",
		"--required-wecom-login-entries", "pc_web_qr,wecom_client",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	})
	if err == nil {
		t.Fatal("parseArgs succeeded; want legacy WeCom login entry flag failure")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("error = %v, want legacy flag failure", err)
	}
}

func TestParseArgsRejectsMalformedCredentials(t *testing.T) {
	t.Setenv("OPENCLARION_TEST_EMPTY_PASSWORD", "")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "fixture-password")
	t.Setenv("OPENCLARION_TEST_BEARER_TOKEN", "token-1")
	t.Setenv("OPENCLARION_TEST_BEARER_WITH_SPACE", "token 1")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "ldap empty password",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "ldap",
				"--ldap-username", "operator-1",
				"--ldap-password-env", "OPENCLARION_TEST_EMPTY_PASSWORD",
				"--output", "proof.json",
			},
			want: "ldap-password",
		},
		{
			name: "ldap username whitespace",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "ldap",
				"--ldap-username", "operator 1",
				"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
				"--output", "proof.json",
			},
			want: "malformed",
		},
		{
			name: "bearer token whitespace",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "bearer",
				"--bearer-token-env", "OPENCLARION_TEST_BEARER_WITH_SPACE",
				"--output", "proof.json",
			},
			want: "bearer-token",
		},
		{
			name: "legacy wecom session auth mode",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "wecom_session",
				"--output", "proof.json",
			},
			want: "--auth-mode",
		},
		{
			name: "unsupported required supported mode",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "bearer",
				"--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN",
				"--required-supported-modes", "none",
				"--output", "proof.json",
			},
			want: "unsupported mode",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(tc.args)
			if err == nil {
				t.Fatal("parseArgs error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseArgsRejectsDirectSecretValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "direct ldap password",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "ldap",
				"--ldap-username", "operator-1",
				"--ldap-password", "fixture-password",
				"--output", "proof.json",
			},
			want: "avoid leaking credentials in process arguments",
		},
		{
			name: "direct bearer token",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "bearer",
				"--bearer-token", "fixture-token",
				"--output", "proof.json",
			},
			want: "avoid leaking credentials in process arguments",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(tc.args)
			if err == nil {
				t.Fatal("parseArgs error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseArgsRejectsUnsafeEnvReferences(t *testing.T) {
	t.Setenv("OPENCLARION_TEST_PASSWORD", "fixture-password")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid env name",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "ldap",
				"--ldap-username-env", "OPENCLARION-USER",
				"--ldap-password-env", "OPENCLARION_TEST_PASSWORD",
				"--output", "proof.json",
			},
			want: "valid environment variable name",
		},
		{
			name: "missing env value",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "bearer",
				"--bearer-token-env", "OPENCLARION_TEST_MISSING_BEARER",
				"--output", "proof.json",
			},
			want: "must be set and non-empty",
		},
		{
			name: "duplicate credential source",
			args: []string{
				"--api-base-url", "http://127.0.0.1:32101",
				"--auth-mode", "ldap",
				"--ldap-username", "operator-1",
				"--ldap-username-env", "OPENCLARION_TEST_PASSWORD",
				"--ldap-password-env", "OPENCLARION_TEST_PASSWORD",
				"--output", "proof.json",
			},
			want: "cannot both be set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(tc.args)
			if err == nil {
				t.Fatal("parseArgs error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeJSONStatus(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func unusedHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be used for rejected inputs")
		return nil, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
