package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	authoidc "github.com/openclarion/openclarion/internal/providers/auth/oidc"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestIssuedTokenAuthenticatesThroughRuntimeOIDCProvider(t *testing.T) {
	issuer := newTestIssuer(t)
	server := httptest.NewServer(issuer.handler())
	t.Cleanup(server.Close)
	issuer.issuer = server.URL

	provider, err := authoidc.NewProvider(context.Background(), authoidc.Config{
		IssuerURL: server.URL,
		ClientID:  issuer.clientID,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	resp, err := getTestURL(t, server, "/token?subject=operator-42&roles=owner,admin&ttl=5m")
	if err != nil {
		t.Fatalf("GET /token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /token status = %d, want 200", resp.StatusCode)
	}
	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if token.IDToken == "" || !strings.HasPrefix(token.AuthorizationHeader, "Bearer ") {
		t.Fatalf("token response missing bearer fields: %+v", token)
	}

	principal, err := provider.AuthenticateBearer(context.Background(), token.AuthorizationHeader)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if principal.Subject != "operator-42" {
		t.Fatalf("subject = %q, want operator-42", principal.Subject)
	}
	if !slices.Contains(principal.Roles, ports.AuthRoleOwner) || !slices.Contains(principal.Roles, ports.AuthRoleAdmin) {
		t.Fatalf("roles = %#v, want owner and admin", principal.Roles)
	}
	if strings.Contains(string(principal.Claims), token.IDToken) {
		t.Fatalf("claims unexpectedly contain raw token")
	}
}

func TestDiscoveryAndJWKSExposePublicMetadata(t *testing.T) {
	issuer := newTestIssuer(t)
	server := httptest.NewServer(issuer.handler())
	t.Cleanup(server.Close)
	issuer.issuer = server.URL

	discoveryResp, err := getTestURL(t, server, "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("GET discovery: %v", err)
	}
	defer discoveryResp.Body.Close()
	var discovery discoveryResponse
	if err := json.NewDecoder(discoveryResp.Body).Decode(&discovery); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if discovery.Issuer != server.URL || discovery.JWKSURI != server.URL+"/keys" {
		t.Fatalf("discovery = %+v", discovery)
	}

	keysResp, err := getTestURL(t, server, "/keys")
	if err != nil {
		t.Fatalf("GET keys: %v", err)
	}
	defer keysResp.Body.Close()
	var keys jwksResponse
	if err := json.NewDecoder(keysResp.Body).Decode(&keys); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	if len(keys.Keys) != 1 || keys.Keys[0].Kty != "RSA" || keys.Keys[0].Alg != "RS256" ||
		keys.Keys[0].N == "" || keys.Keys[0].E == "" {
		t.Fatalf("keys = %+v", keys.Keys)
	}
}

func TestTokenEndpointRejectsInvalidInput(t *testing.T) {
	issuer := newTestIssuer(t)
	server := httptest.NewServer(issuer.handler())
	t.Cleanup(server.Close)
	issuer.issuer = server.URL

	for _, path := range []string{
		"/token?ttl=3h",
		"/token?subject=operator%2042",
		"/token?roles=owner,,admin",
	} {
		resp, err := getTestURL(t, server, path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close response body for %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("GET %s status = %d, want 400", path, resp.StatusCode)
		}
	}
}

func TestLoopbackValidationRequiresExplicitOptIn(t *testing.T) {
	if err := validateLoopbackAddr("127.0.0.1:18080", false); err != nil {
		t.Fatalf("loopback listen rejected: %v", err)
	}
	if err := validateLoopbackAddr("0.0.0.0:18080", false); err == nil {
		t.Fatal("non-loopback listen without opt-in: want error")
	}
	if err := validateLoopbackAddr("0.0.0.0:18080", true); err != nil {
		t.Fatalf("non-loopback listen with opt-in rejected: %v", err)
	}
	if _, err := normalizeIssuer("http://issuer.example.test", false); err == nil {
		t.Fatal("non-loopback issuer without opt-in: want error")
	}
	if got, err := normalizeIssuer("http://LOCALHOST:18080/", false); err != nil || got != "http://localhost:18080" {
		t.Fatalf("normalize localhost issuer = %q, %v", got, err)
	}
}

func newTestIssuer(t *testing.T) *devIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	issuer, err := newDevIssuer(config{
		Issuer:         "http://127.0.0.1:18080",
		ClientID:       defaultClientID,
		KeyID:          defaultKeyID,
		DefaultSubject: defaultSubject,
		DefaultRoles:   []string{"owner"},
		DefaultTTL:     defaultTokenTTL,
	}, key)
	if err != nil {
		t.Fatalf("newDevIssuer: %v", err)
	}
	issuer.now = func() time.Time {
		return time.Now().UTC().Add(-time.Second)
	}
	return issuer
}

func getTestURL(t *testing.T, server *httptest.Server, path string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext(%s): %v", path, err)
	}
	return server.Client().Do(req)
}
