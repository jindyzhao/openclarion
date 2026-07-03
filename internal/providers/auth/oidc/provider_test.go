package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"
	"github.com/go-jose/go-jose/v4"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderAuthenticatesBearerAndMapsRoles(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []string{"owner", "admin", "ignored"},
		"email": "user@example.com",
	})

	principal, err := provider.AuthenticateAuthorization(context.Background(), "Bearer "+rawToken)
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "user-123" {
		t.Fatalf("Subject = %q, want user-123", principal.Subject)
	}
	if !slices.Contains(principal.Roles, ports.AuthRoleOwner) || !slices.Contains(principal.Roles, ports.AuthRoleAdmin) {
		t.Fatalf("Roles = %#v, want owner and admin", principal.Roles)
	}
	var claims map[string]any
	if err := json.Unmarshal(principal.Claims, &claims); err != nil {
		t.Fatalf("claims JSON: %v", err)
	}
	if claims["email"] != "user@example.com" {
		t.Fatalf("email claim = %#v", claims["email"])
	}
	if strings.Contains(string(principal.Claims), rawToken) {
		t.Fatalf("claims unexpectedly contain raw token")
	}
}

func TestProviderSupportsCustomRoleClaimAndValues(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{
		ClientID:        "openclarion",
		RoleClaim:       "groups",
		OwnerRoleValues: []string{"claims-owner"},
		AdminRoleValues: []string{"claims-admin"},
	})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"groups": "claims-owner",
	})

	principal, err := provider.AuthenticateAuthorization(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("Roles = %#v, want owner", principal.Roles)
	}
}

func TestProviderEnrichesClaimsFromUserInfo(t *testing.T) {
	ts, priv := newOIDCTestServerWithUserInfo(t, "access-token-1", map[string]any{
		"sub":                "user-123",
		"roles":              []string{"owner"},
		"email":              "operator@example.com",
		"preferred_username": "operator",
	})
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []string{"ignored"},
	})

	principal, err := provider.AuthenticateAuthorizationWithAuxiliaryCredentials(
		context.Background(),
		"Bearer "+rawToken,
		ports.AuthAuxiliaryCredentials{OIDCAccessToken: "access-token-1"},
	)
	if err != nil {
		t.Fatalf("AuthenticateAuthorizationWithAuxiliaryCredentials: %v", err)
	}
	if !slices.Contains(principal.Roles, ports.AuthRoleOwner) {
		t.Fatalf("Roles = %#v, want owner from UserInfo", principal.Roles)
	}
	var claims map[string]any
	if err := json.Unmarshal(principal.Claims, &claims); err != nil {
		t.Fatalf("claims JSON: %v", err)
	}
	if claims["email"] != "operator@example.com" ||
		claims["preferred_username"] != "operator" {
		t.Fatalf("claims = %#v, want UserInfo profile claims", claims)
	}
}

func TestProviderRejectsMismatchedUserInfoSubject(t *testing.T) {
	ts, priv := newOIDCTestServerWithUserInfo(t, "access-token-1", map[string]any{
		"sub":   "other-user",
		"roles": []string{"owner"},
	})
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []string{"owner"},
	})

	_, err := provider.AuthenticateAuthorizationWithAuxiliaryCredentials(
		context.Background(),
		"Bearer "+rawToken,
		ports.AuthAuxiliaryCredentials{OIDCAccessToken: "access-token-1"},
	)
	if err == nil {
		t.Fatal("AuthenticateAuthorizationWithAuxiliaryCredentials: want subject mismatch error")
	}
	if !strings.Contains(err.Error(), "userinfo subject") {
		t.Fatalf("error = %q, want userinfo subject mismatch", err.Error())
	}
}

func TestProviderRejectsInvalidBearerValues(t *testing.T) {
	ts, _ := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	for _, token := range []string{"", "Bearer", "Bearer one two", "raw token"} {
		t.Run(token, func(t *testing.T) {
			if _, err := provider.AuthenticateAuthorization(context.Background(), token); err == nil {
				t.Fatalf("AuthenticateAuthorization(%q): want error", token)
			}
		})
	}
}

func TestProviderRejectsInvalidToken(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "other-client", time.Now().Add(time.Hour), map[string]any{
		"roles": []string{"owner"},
	})

	if _, err := provider.AuthenticateAuthorization(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateAuthorization with wrong audience: want error")
	}
}

func TestProviderRejectsExpiredToken(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(-time.Hour), map[string]any{
		"roles": []string{"owner"},
	})

	if _, err := provider.AuthenticateAuthorization(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateAuthorization with expired token: want error")
	}
}

func TestProviderRejectsMalformedRoleClaim(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []any{"owner", 42},
	})

	if _, err := provider.AuthenticateAuthorization(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateAuthorization with malformed roles: want error")
	}
}

func TestProviderRejectsDuplicateClaimNames(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	tests := []struct {
		name   string
		claims string
		want   string
	}{
		{
			name: "top-level duplicate",
			claims: fmt.Sprintf(
				`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["ignored"],"roles":["owner"]}`,
				ts.URL,
				"openclarion",
				time.Now().Add(time.Hour).Unix(),
			),
			want: `duplicate object key "roles"`,
		},
		{
			name: "nested duplicate",
			claims: fmt.Sprintf(
				`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["owner"],"profile":{"department":"old","department":"new"}}`,
				ts.URL,
				"openclarion",
				time.Now().Add(time.Hour).Unix(),
			),
			want: `duplicate object key "department"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawToken := oidctest.SignIDToken(priv, "test-key", gooidc.RS256, tt.claims)

			_, err := provider.AuthenticateAuthorization(context.Background(), rawToken)
			if err == nil {
				t.Fatal("AuthenticateAuthorization with duplicate claim names: want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestProviderVerifiesTokenBeforeStrictClaimScan(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	claims := fmt.Sprintf(
		`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["ignored"],"roles":["owner"]}`,
		ts.URL,
		"other-client",
		time.Now().Add(time.Hour).Unix(),
	)
	rawToken := oidctest.SignIDToken(priv, "test-key", gooidc.RS256, claims)

	_, err := provider.AuthenticateAuthorization(context.Background(), rawToken)
	if err == nil {
		t.Fatal("AuthenticateAuthorization with wrong audience and duplicate claims: want error")
	}
	if !strings.Contains(err.Error(), "verify token") {
		t.Fatalf("error = %q, want verify error", err.Error())
	}
	if strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("error = %q, strict claim scan ran before token verification", err.Error())
	}
}

func TestProviderRejectsTrailingClaimPayloadValueDuringVerification(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	claims := fmt.Sprintf(
		`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["owner"]}[]`,
		ts.URL,
		"openclarion",
		time.Now().Add(time.Hour).Unix(),
	)
	rawToken := oidctest.SignIDToken(priv, "test-key", gooidc.RS256, claims)

	_, err := provider.AuthenticateAuthorization(context.Background(), rawToken)
	if err == nil {
		t.Fatal("AuthenticateAuthorization with trailing claim payload value: want error")
	}
	if !strings.Contains(err.Error(), "verify token") {
		t.Fatalf("error = %q, want verify error", err.Error())
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("error = %q, want malformed claim payload error", err.Error())
	}
}

func TestNewProviderRejectsInvalidConfig(t *testing.T) {
	passwordIssuer := (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "opaque"),
		Host:   "issuer.example",
	}).String()
	rawMarker := "raw-marker"
	tests := []struct {
		name    string
		cfg     Config
		want    string
		wantNot string
	}{
		{
			name: "empty issuer",
			cfg:  Config{ClientID: "client"},
			want: "issuer",
		},
		{
			name: "empty client",
			cfg:  Config{IssuerURL: "https://issuer.example"},
			want: "client id",
		},
		{
			name:    "malformed credentialed issuer does not leak raw input",
			cfg:     Config{IssuerURL: "https://operator:" + rawMarker + "@issuer.example/\nrealm", ClientID: "client"},
			want:    "parse issuer url",
			wantNot: rawMarker,
		},
		{
			name: "issuer username userinfo",
			cfg:  Config{IssuerURL: "https://operator@issuer.example", ClientID: "client"},
			want: "userinfo",
		},
		{
			name: "issuer password userinfo",
			cfg:  Config{IssuerURL: passwordIssuer, ClientID: "client"},
			want: "userinfo",
		},
		{
			name: "issuer escaped userinfo",
			cfg:  Config{IssuerURL: "https://%6fperator@issuer.example", ClientID: "client"},
			want: "userinfo",
		},
		{
			name: "bad role values",
			cfg: Config{
				IssuerURL:       "https://issuer.example",
				ClientID:        "client",
				OwnerRoleValues: []string{""},
			},
			want: "owner role values",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(context.Background(), tc.cfg)
			if err == nil {
				t.Fatalf("NewProvider: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if tc.wantNot != "" && strings.Contains(err.Error(), tc.wantNot) {
				t.Fatalf("error = %q, must not contain %q", err.Error(), tc.wantNot)
			}
		})
	}
}

func newOIDCTestServer(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	srv := &oidctest.Server{
		PublicKeys: []oidctest.PublicKey{
			{PublicKey: priv.Public(), KeyID: "test-key", Algorithm: gooidc.RS256},
		},
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	srv.SetIssuer(ts.URL)
	return ts, priv
}

func newOIDCTestServerWithUserInfo(t *testing.T, accessToken string, userInfoClaims map[string]any) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	var issuer string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "/auth",
				"token_endpoint":                        issuer + "/token",
				"jwks_uri":                              issuer + "/keys",
				"userinfo_endpoint":                     issuer + "/userinfo",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{gooidc.RS256},
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
				Keys: []jose.JSONWebKey{{
					Algorithm: gooidc.RS256,
					Key:       priv.Public(),
					KeyID:     "test-key",
					Use:       "sig",
				}},
			})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer "+accessToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(userInfoClaims)
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = ts.URL
	t.Cleanup(ts.Close)
	return ts, priv
}

func newTestProvider(t *testing.T, issuer string, cfg Config) *Provider {
	t.Helper()
	cfg.IssuerURL = issuer
	provider, err := NewProvider(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return provider
}

func signIDToken(t *testing.T, priv *rsa.PrivateKey, issuer, audience string, expiresAt time.Time, extra map[string]any) string {
	t.Helper()
	claims := map[string]any{
		"iss": issuer,
		"aud": audience,
		"sub": "user-123",
		"exp": expiresAt.Unix(),
	}
	for key, value := range extra {
		claims[key] = value
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("Marshal claims: %v", err)
	}
	return oidctest.SignIDToken(priv, "test-key", gooidc.RS256, string(raw))
}
