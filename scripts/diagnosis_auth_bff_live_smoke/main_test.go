package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWritesSanitizedLDAPBFFSessionProof(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/diagnosis/auth/session":
			switch r.Method {
			case http.MethodPost:
				if got := r.Header.Get("Authorization"); got != "Basic "+base64.StdEncoding.EncodeToString([]byte("operator-1:ldap-password")) {
					t.Fatalf("authorization = %q, want LDAP basic fixture", got)
				}
				http.SetCookie(w, &http.Cookie{
					Name:     diagnosisSessionCookieName,
					Value:    "session.token.one",
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   true,
				})
				writeBFFAuthJSONStatus(t, w, http.StatusCreated, map[string]any{
					"authenticated":   true,
					"checked_at":      "2026-06-22T11:59:00Z",
					"mode":            "ldap",
					"role_authorized": true,
					"roles":           []string{"owner"},
					"subject":         "operator-1",
					"tenant_id":       int64(1),
					"tenant_key":      "default",
				})
			case http.MethodGet:
				cookie, err := r.Cookie(diagnosisSessionCookieName)
				if err != nil {
					writeBFFAuthJSON(t, w, map[string]any{
						"authenticated": false,
					})
					return
				}
				if cookie.Value != "session.token.one" {
					t.Fatalf("session cookie = %q, want fixture token", cookie.Value)
				}
				writeBFFAuthJSON(t, w, map[string]any{
					"authenticated":   true,
					"checked_at":      "2026-06-22T11:59:30Z",
					"mode":            "ldap",
					"role_authorized": true,
					"roles":           []string{"owner"},
					"subject":         "operator-1",
					"tenant_id":       int64(1),
					"tenant_key":      "default",
				})
			case http.MethodDelete:
				cookie, err := r.Cookie(diagnosisSessionCookieName)
				if err != nil {
					t.Fatalf("missing diagnosis session cookie on clear: %v", err)
				}
				if cookie.Value != "session.token.one" {
					t.Fatalf("clear session cookie = %q, want fixture token", cookie.Value)
				}
				http.SetCookie(w, &http.Cookie{
					Name:     diagnosisSessionCookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   true,
				})
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("OPENCLARION_TEST_LDAP_USERNAME", "operator-1")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "ldap-password")
	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--web-base-url", server.URL,
		"--auth-mode", "ldap",
		"--ldap-username-env", "OPENCLARION_TEST_LDAP_USERNAME",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads proof path it created.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	for _, leaked := range []string{
		"ldap-password",
		"session.token.one",
		"Authorization",
		"Basic ",
		diagnosisSessionCookieName + "=",
	} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("proof leaked %q: %s", leaked, raw)
		}
	}
	var out proof
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.AuthMode != "ldap" ||
		out.Issue.HTTPStatus != http.StatusCreated ||
		out.Issue.Subject != "operator-1" ||
		out.Issue.Mode != "ldap" ||
		!out.Issue.RoleAuthorized ||
		out.Issue.RoleCount != 1 ||
		out.Issue.TenantID != 1 ||
		out.Issue.TenantKey != "default" ||
		!out.Issue.SessionCookie ||
		!out.Issue.SessionCookieAttrs.HTTPOnly ||
		out.Issue.SessionCookieAttrs.SameSite != "lax" ||
		out.Issue.SessionCookieAttrs.Path != "/" ||
		!out.Issue.SessionCookieAttrs.Secure {
		t.Fatalf("issue proof = %+v", out.Issue)
	}
	if out.Check.HTTPStatus != http.StatusOK ||
		out.Check.Subject != out.Issue.Subject ||
		out.Check.Mode != out.Issue.Mode ||
		out.Check.RoleCount != out.Issue.RoleCount ||
		out.Check.TenantID != out.Issue.TenantID ||
		out.Check.TenantKey != out.Issue.TenantKey {
		t.Fatalf("check proof = %+v", out.Check)
	}
	if out.Clear.HTTPStatus != http.StatusNoContent ||
		!out.Clear.SessionCookieCleared ||
		!out.Clear.SessionCookieAttrs.HTTPOnly ||
		out.Clear.SessionCookieAttrs.SameSite != "lax" ||
		out.Clear.SessionCookieAttrs.Path != "/" ||
		!out.Clear.SessionCookieAttrs.Secure {
		t.Fatalf("clear proof = %+v", out.Clear)
	}
	if out.PostClear.HTTPStatus != http.StatusOK || out.PostClear.Authenticated {
		t.Fatalf("post-clear proof = %+v", out.PostClear)
	}
}

func TestRunWritesSanitizedBearerBFFSessionProof(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/diagnosis/auth/session":
			switch r.Method {
			case http.MethodPost:
				if got := r.Header.Get("Authorization"); got != "Bearer oidc-token-one" {
					t.Fatalf("authorization = %q, want OIDC bearer fixture", got)
				}
				http.SetCookie(w, &http.Cookie{
					Name:     diagnosisSessionCookieName,
					Value:    "session.token.one",
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   true,
				})
				writeBFFAuthJSONStatus(t, w, http.StatusCreated, map[string]any{
					"authenticated":   true,
					"checked_at":      "2026-06-22T11:59:00Z",
					"mode":            "oidc",
					"role_authorized": true,
					"roles":           []string{"owner", "admin"},
					"subject":         "operator-oidc",
					"tenant_id":       int64(2),
					"tenant_key":      "platform",
				})
			case http.MethodGet:
				cookie, err := r.Cookie(diagnosisSessionCookieName)
				if err != nil {
					writeBFFAuthJSON(t, w, map[string]any{
						"authenticated": false,
					})
					return
				}
				if cookie.Value != "session.token.one" {
					t.Fatalf("session cookie = %q, want fixture token", cookie.Value)
				}
				writeBFFAuthJSON(t, w, map[string]any{
					"authenticated":   true,
					"checked_at":      "2026-06-22T11:59:30Z",
					"mode":            "oidc",
					"role_authorized": true,
					"roles":           []string{"admin", "owner"},
					"subject":         "operator-oidc",
					"tenant_id":       int64(2),
					"tenant_key":      "platform",
				})
			case http.MethodDelete:
				cookie, err := r.Cookie(diagnosisSessionCookieName)
				if err != nil {
					t.Fatalf("missing diagnosis session cookie on clear: %v", err)
				}
				if cookie.Value != "session.token.one" {
					t.Fatalf("clear session cookie = %q, want fixture token", cookie.Value)
				}
				http.SetCookie(w, &http.Cookie{
					Name:     diagnosisSessionCookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Secure:   true,
				})
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("OPENCLARION_TEST_BEARER_TOKEN", "Bearer oidc-token-one")
	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--web-base-url", server.URL,
		"--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads proof path it created.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	for _, leaked := range []string{
		"oidc-token-one",
		"session.token.one",
		"Authorization",
		"Bearer ",
		diagnosisSessionCookieName + "=",
	} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("proof leaked %q: %s", leaked, raw)
		}
	}
	var out proof
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.AuthMode != "bearer" ||
		out.Issue.HTTPStatus != http.StatusCreated ||
		out.Issue.Subject != "operator-oidc" ||
		out.Issue.Mode != "oidc" ||
		!out.Issue.RoleAuthorized ||
		out.Issue.RoleCount != 2 ||
		out.Issue.TenantID != 2 ||
		out.Issue.TenantKey != "platform" ||
		!out.Issue.SessionCookie ||
		!out.Issue.SessionCookieAttrs.HTTPOnly ||
		out.Issue.SessionCookieAttrs.SameSite != "lax" ||
		out.Issue.SessionCookieAttrs.Path != "/" ||
		!out.Issue.SessionCookieAttrs.Secure {
		t.Fatalf("issue proof = %+v", out.Issue)
	}
	if out.Check.HTTPStatus != http.StatusOK ||
		out.Check.Subject != out.Issue.Subject ||
		out.Check.Mode != out.Issue.Mode ||
		out.Check.RoleCount != out.Issue.RoleCount ||
		out.Check.TenantID != out.Issue.TenantID ||
		out.Check.TenantKey != out.Issue.TenantKey {
		t.Fatalf("check proof = %+v", out.Check)
	}
	if out.Clear.HTTPStatus != http.StatusNoContent ||
		!out.Clear.SessionCookieCleared ||
		!out.Clear.SessionCookieAttrs.HTTPOnly ||
		out.Clear.SessionCookieAttrs.SameSite != "lax" ||
		out.Clear.SessionCookieAttrs.Path != "/" ||
		!out.Clear.SessionCookieAttrs.Secure {
		t.Fatalf("clear proof = %+v", out.Clear)
	}
	if out.PostClear.HTTPStatus != http.StatusOK || out.PostClear.Authenticated {
		t.Fatalf("post-clear proof = %+v", out.PostClear)
	}
}

func TestParseArgsRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "base url with userinfo",
			args: []string{"--web-base-url", "https://user@example.test", "--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN", "--output", "proof.json"},
			want: "userinfo",
		},
		{
			name: "direct password",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "ldap", "--ldap-username", "operator-1", "--ldap-password", "secret", "--output", "proof.json"},
			want: "avoid leaking credentials",
		},
		{
			name: "bad username",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "ldap", "--ldap-username", "operator 1", "--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD", "--output", "proof.json"},
			want: "malformed",
		},
		{
			name: "username surrounding whitespace",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "ldap", "--ldap-username", " operator-1 ", "--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD", "--output", "proof.json"},
			want: "malformed",
		},
		{
			name: "username env name whitespace",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "ldap", "--ldap-username-env", " OPENCLARION_TEST_LDAP_USERNAME ", "--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD", "--output", "proof.json"},
			want: "valid environment variable name",
		},
		{
			name: "password env name whitespace",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "ldap", "--ldap-username-env", "OPENCLARION_TEST_LDAP_USERNAME", "--ldap-password-env", " OPENCLARION_TEST_LDAP_PASSWORD ", "--output", "proof.json"},
			want: "valid environment variable name",
		},
		{
			name: "direct bearer token",
			args: []string{"--web-base-url", "https://console.example.test", "--bearer-token", "secret-token", "--output", "proof.json"},
			want: "avoid leaking credentials",
		},
		{
			name: "bearer token with whitespace",
			args: []string{"--web-base-url", "https://console.example.test", "--bearer-token-env", "OPENCLARION_TEST_BAD_BEARER_TOKEN", "--output", "proof.json"},
			want: "malformed",
		},
		{
			name: "bearer with ldap args",
			args: []string{"--web-base-url", "https://console.example.test", "--auth-mode", "bearer", "--bearer-token-env", "OPENCLARION_TEST_BEARER_TOKEN", "--ldap-username", "operator-1", "--output", "proof.json"},
			want: "LDAP credentials require",
		},
	}
	t.Setenv("OPENCLARION_TEST_LDAP_USERNAME", "operator-1")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "ldap-password")
	t.Setenv("OPENCLARION_TEST_BEARER_TOKEN", "oidc-token-one")
	t.Setenv("OPENCLARION_TEST_BAD_BEARER_TOKEN", "oidc token one")
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(tc.args)
			if err == nil {
				t.Fatal("parseArgs succeeded; want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseArgsRejectsWhitespaceLDAPUsernameFromEnvWithoutLeak(t *testing.T) {
	t.Setenv("OPENCLARION_TEST_LDAP_USERNAME", "operator-1\n")
	t.Setenv("OPENCLARION_TEST_LDAP_PASSWORD", "ldap-password")
	_, err := parseArgs([]string{
		"--web-base-url", "https://console.example.test",
		"--auth-mode", "ldap",
		"--ldap-username-env", "OPENCLARION_TEST_LDAP_USERNAME",
		"--ldap-password-env", "OPENCLARION_TEST_LDAP_PASSWORD",
		"--output", "proof.json",
	})
	if err == nil {
		t.Fatal("parseArgs succeeded; want error")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("error = %v, want malformed credentials", err)
	}
	for _, leaked := range []string{"operator-1", "ldap-password"} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked credential value %q: %v", leaked, err)
		}
	}
}

func TestValidateProofRejectsIncompleteCookieProof(t *testing.T) {
	valid := validBFFAuthProofFixture()
	if err := validateProof(valid); err != nil {
		t.Fatalf("validateProof(valid): %v", err)
	}
	valid.Issue.SessionCookieAttrs.HTTPOnly = false
	valid.Evidence = bffAuthEvidence(valid)
	err := validateProof(valid)
	if err == nil {
		t.Fatal("validateProof succeeded; want cookie failure")
	}
	if !strings.Contains(err.Error(), "cookie proof") {
		t.Fatalf("error = %v, want cookie proof failure", err)
	}
}

func TestValidateProofRejectsIncompleteClearCookieProof(t *testing.T) {
	valid := validBFFAuthProofFixture()
	valid.Clear.SessionCookieCleared = false
	valid.Evidence = bffAuthEvidence(valid)
	err := validateProof(valid)
	if err == nil {
		t.Fatal("validateProof succeeded; want clear cookie failure")
	}
	if !strings.Contains(err.Error(), "session_cookie_cleared") {
		t.Fatalf("error = %v, want clear cookie failure", err)
	}
}

func TestValidateProofRejectsAuthenticatedPostClearProof(t *testing.T) {
	valid := validBFFAuthProofFixture()
	valid.PostClear.Authenticated = true
	valid.Evidence = bffAuthEvidence(valid)
	err := validateProof(valid)
	if err == nil {
		t.Fatal("validateProof succeeded; want post-clear failure")
	}
	if !strings.Contains(err.Error(), "post_clear.authenticated") {
		t.Fatalf("error = %v, want post-clear auth failure", err)
	}
}

func TestValidateProofRejectsChangedTenantBinding(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*proof)
	}{
		{
			name: "tenant id",
			mutate: func(out *proof) {
				out.Check.TenantID = 2
			},
		},
		{
			name: "tenant key",
			mutate: func(out *proof) {
				out.Check.TenantKey = "platform"
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			valid := validBFFAuthProofFixture()
			tc.mutate(&valid)
			valid.Evidence = bffAuthEvidence(valid)
			err := validateProof(valid)
			if err == nil {
				t.Fatal("validateProof succeeded; want tenant binding failure")
			}
			if !strings.Contains(err.Error(), "must match issued session principal") {
				t.Fatalf("error = %v, want tenant binding mismatch", err)
			}
		})
	}
}

func TestValidateProofRejectsInvalidTenantBinding(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*proof)
		wantErr string
	}{
		{
			name: "issue tenant id",
			mutate: func(out *proof) {
				out.Issue.TenantID = 0
			},
			wantErr: "issue tenant binding is invalid",
		},
		{
			name: "issue tenant key",
			mutate: func(out *proof) {
				out.Issue.TenantKey = "Default"
			},
			wantErr: "issue tenant binding is invalid",
		},
		{
			name: "check tenant id",
			mutate: func(out *proof) {
				out.Check.TenantID = 0
			},
			wantErr: "check tenant binding is invalid",
		},
		{
			name: "check tenant key",
			mutate: func(out *proof) {
				out.Check.TenantKey = "default "
			},
			wantErr: "check tenant binding is invalid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			valid := validBFFAuthProofFixture()
			tc.mutate(&valid)
			valid.Evidence = bffAuthEvidence(valid)
			err := validateProof(valid)
			if err == nil {
				t.Fatal("validateProof succeeded; want invalid tenant binding failure")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func validBFFAuthProofFixture() proof {
	out := proof{
		Passed:    true,
		CheckedAt: "2026-06-22T12:00:00Z",
		Request: proofRequest{
			WebBaseURL: "https://console.example.test",
			AuthMode:   "ldap",
			Timeout:    "15s",
		},
		Issue: sessionProof{
			HTTPStatus:     http.StatusCreated,
			Subject:        "operator-1",
			Roles:          []string{"owner"},
			Mode:           "ldap",
			CheckedAt:      "2026-06-22T11:59:00Z",
			RoleAuthorized: true,
			RoleCount:      1,
			TenantID:       1,
			TenantKey:      "default",
			SessionCookie:  true,
			SessionCookieAttrs: cookieProof{
				HTTPOnly: true,
				Path:     "/",
				SameSite: "lax",
				Secure:   true,
			},
		},
		Check: checkProof{
			HTTPStatus:     http.StatusOK,
			Subject:        "operator-1",
			Roles:          []string{"owner"},
			Mode:           "ldap",
			CheckedAt:      "2026-06-22T11:59:30Z",
			RoleAuthorized: true,
			RoleCount:      1,
			TenantID:       1,
			TenantKey:      "default",
		},
		Clear: clearProof{
			HTTPStatus:           http.StatusNoContent,
			SessionCookieCleared: true,
			SessionCookieAttrs: cookieProof{
				HTTPOnly: true,
				Path:     "/",
				SameSite: "lax",
				Secure:   true,
			},
		},
		PostClear: postClearProof{
			HTTPStatus:    http.StatusOK,
			Authenticated: false,
		},
	}
	out.Evidence = bffAuthEvidence(out)
	return out
}

func writeBFFAuthJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	writeBFFAuthJSONStatus(t, w, http.StatusOK, value)
}

func writeBFFAuthJSONStatus(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
