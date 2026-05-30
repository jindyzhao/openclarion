package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderAuthenticatesBearerAndMapsRoles(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []string{"owner", "admin", "ignored"},
		"email": "user@example.com",
	})

	principal, err := provider.AuthenticateBearer(context.Background(), "Bearer "+rawToken)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
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

	principal, err := provider.AuthenticateBearer(context.Background(), rawToken)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if len(principal.Roles) != 1 || principal.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("Roles = %#v, want owner", principal.Roles)
	}
}

func TestProviderRejectsInvalidBearerValues(t *testing.T) {
	ts, _ := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	for _, token := range []string{"", "Bearer", "Bearer one two", "raw token"} {
		t.Run(token, func(t *testing.T) {
			if _, err := provider.AuthenticateBearer(context.Background(), token); err == nil {
				t.Fatalf("AuthenticateBearer(%q): want error", token)
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

	if _, err := provider.AuthenticateBearer(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateBearer with wrong audience: want error")
	}
}

func TestProviderRejectsExpiredToken(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(-time.Hour), map[string]any{
		"roles": []string{"owner"},
	})

	if _, err := provider.AuthenticateBearer(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateBearer with expired token: want error")
	}
}

func TestProviderRejectsMalformedRoleClaim(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	rawToken := signIDToken(t, priv, ts.URL, "openclarion", time.Now().Add(time.Hour), map[string]any{
		"roles": []any{"owner", 42},
	})

	if _, err := provider.AuthenticateBearer(context.Background(), rawToken); err == nil {
		t.Fatal("AuthenticateBearer with malformed roles: want error")
	}
}

func TestProviderRejectsDuplicateClaimNames(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	claims := fmt.Sprintf(
		`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["ignored"],"roles":["owner"]}`,
		ts.URL,
		"openclarion",
		time.Now().Add(time.Hour).Unix(),
	)
	rawToken := oidctest.SignIDToken(priv, "test-key", gooidc.RS256, claims)

	_, err := provider.AuthenticateBearer(context.Background(), rawToken)
	if err == nil {
		t.Fatal("AuthenticateBearer with duplicate claim names: want error")
	}
	if !strings.Contains(err.Error(), `duplicate object key "roles"`) {
		t.Fatalf("error = %q, want duplicate roles claim error", err.Error())
	}
}

func TestProviderRejectsTrailingClaimPayloadValue(t *testing.T) {
	ts, priv := newOIDCTestServer(t)
	provider := newTestProvider(t, ts.URL, Config{ClientID: "openclarion"})
	claims := fmt.Sprintf(
		`{"iss":%q,"aud":%q,"sub":"user-123","exp":%d,"roles":["owner"]}[]`,
		ts.URL,
		"openclarion",
		time.Now().Add(time.Hour).Unix(),
	)
	rawToken := oidctest.SignIDToken(priv, "test-key", gooidc.RS256, claims)

	_, err := provider.AuthenticateBearer(context.Background(), rawToken)
	if err == nil {
		t.Fatal("AuthenticateBearer with trailing claim payload value: want error")
	}
	if !strings.Contains(err.Error(), "trailing JSON values") {
		t.Fatalf("error = %q, want trailing JSON values error", err.Error())
	}
}

func TestNewProviderRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
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
