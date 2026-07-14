package diagnosisauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestSessionTokenServiceIssuesAndAuthenticatesBearer(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	service := newTestSessionTokenService(t, now)

	issued, err := service.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if issued.Token == "" || strings.ContainsAny(issued.Token, "+/=") {
		t.Fatalf("Token = %q, want raw URL-safe base64 parts", issued.Token)
	}
	if issued.ExpiresAt.Sub(issued.IssuedAt) != DefaultSessionTTL {
		t.Fatalf("TTL = %s, want %s", issued.ExpiresAt.Sub(issued.IssuedAt), DefaultSessionTTL)
	}

	principal, err := service.AuthenticateBearer(context.Background(), "Bearer "+issued.Token)
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if principal.Subject != "operator-1" || len(principal.Roles) != 1 || principal.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("principal = %+v", principal)
	}
	if principal.TenantID != tenancy.DefaultIdentity().ID || principal.TenantKey != tenancy.DefaultIdentity().Key {
		t.Fatalf("principal tenant = %d/%q", principal.TenantID, principal.TenantKey)
	}

	session, err := service.AuthenticateSession(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if session.Provider != "oidc" || session.Subject != "operator-1" {
		t.Fatalf("session = %+v", session)
	}
}

func TestSessionTokenServiceRebindTenantPreservesLifetime(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	current := now
	service := newTestSessionTokenServiceWithClock(t, func() time.Time { return current })
	issued, err := service.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	current = now.Add(2 * time.Hour)
	rebound, err := service.RebindTenant(context.Background(), issued.Token, tenancy.Identity{ID: 7, Key: "team-seven"})
	if err != nil {
		t.Fatalf("RebindTenant: %v", err)
	}
	if !rebound.IssuedAt.Equal(issued.IssuedAt) || !rebound.ExpiresAt.Equal(issued.ExpiresAt) {
		t.Fatalf("rebound lifetime = %s..%s, want %s..%s", rebound.IssuedAt, rebound.ExpiresAt, issued.IssuedAt, issued.ExpiresAt)
	}
	if rebound.TenantID != 7 || rebound.TenantKey != "team-seven" {
		t.Fatalf("rebound tenant = %d/%q", rebound.TenantID, rebound.TenantKey)
	}
	verified, err := service.AuthenticateSession(context.Background(), rebound.Token)
	if err != nil {
		t.Fatalf("AuthenticateSession rebound token: %v", err)
	}
	if verified.TenantID != 7 || verified.TenantKey != "team-seven" || !verified.ExpiresAt.Equal(issued.ExpiresAt) {
		t.Fatalf("verified rebound session = %+v", verified)
	}
}

func TestSessionTokenServiceAcceptsLegacyTokenOnlyAsDefaultTenant(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	service := newTestSessionTokenService(t, now)
	payload := sessionTokenPayload{
		Version:   legacySessionTokenVersion,
		Type:      sessionTokenType,
		Provider:  "oidc",
		Subject:   "operator-1",
		Roles:     []ports.AuthRole{ports.AuthRoleOwner},
		TenantID:  99,
		TenantKey: "must-be-ignored",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	}
	legacyToken := signTestSessionToken(t, service, legacySessionTokenVersion, payload)

	session, err := service.AuthenticateSession(context.Background(), legacyToken)
	if err != nil {
		t.Fatalf("AuthenticateSession legacy token: %v", err)
	}
	if session.TenantID != tenancy.DefaultIdentity().ID || session.TenantKey != tenancy.DefaultIdentity().Key {
		t.Fatalf("legacy tenant = %d/%q", session.TenantID, session.TenantKey)
	}

	mismatched := signTestSessionToken(t, service, sessionTokenVersion, payload)
	if _, err := service.AuthenticateSession(context.Background(), mismatched); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("mismatched versions err = %v, want ErrUnauthenticated", err)
	}
}

func TestSessionTokenServiceRejectsTamperedAndExpiredTokens(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	current := now
	service := newTestSessionTokenServiceWithClock(t, func() time.Time { return current })
	issued, err := service.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
	}, "oidc")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	tampered := issued.Token[:len(issued.Token)-1] + "A"
	if _, err := service.AuthenticateBearer(context.Background(), "Bearer "+tampered); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("tampered AuthenticateBearer err = %v, want ErrUnauthenticated", err)
	}

	current = now.Add(DefaultSessionTTL)
	if _, err := service.AuthenticateBearer(context.Background(), "Bearer "+issued.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired AuthenticateBearer err = %v, want ErrUnauthenticated", err)
	}
}

func TestValidateSessionTokenPolicyRejectsUnsafeSettings(t *testing.T) {
	tests := []struct {
		name   string
		policy SessionTokenPolicy
	}{
		{name: "long ttl", policy: SessionTokenPolicy{TTL: HardMaxSessionTTL + time.Second, SigningKey: strings.Repeat("s", MinSessionSigningKeyBytes)}},
		{name: "short key", policy: SessionTokenPolicy{TTL: time.Hour, SigningKey: "short"}},
		{name: "trimmed key", policy: SessionTokenPolicy{TTL: time.Hour, SigningKey: strings.Repeat("s", MinSessionSigningKeyBytes) + "\n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSessionTokenPolicy(tc.policy); !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("ValidateSessionTokenPolicy err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestSessionTokenServiceRejectsUnsupportedRoles(t *testing.T) {
	service := newTestSessionTokenService(t, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if _, err := service.IssueToken(context.Background(), ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{"viewer"},
	}, "oidc"); err == nil {
		t.Fatal("IssueToken with unsupported role err = nil, want error")
	}
}

func newTestSessionTokenService(t *testing.T, now time.Time) *SessionTokenService {
	t.Helper()
	return newTestSessionTokenServiceWithClock(t, func() time.Time { return now })
}

func newTestSessionTokenServiceWithClock(t *testing.T, clock func() time.Time) *SessionTokenService {
	t.Helper()
	service, err := NewSessionTokenService(
		DefaultSessionTokenPolicy(strings.Repeat("s", MinSessionSigningKeyBytes)),
		clock,
	)
	if err != nil {
		t.Fatalf("NewSessionTokenService: %v", err)
	}
	return service
}

func signTestSessionToken(t *testing.T, service *SessionTokenService, headerVersion int, payload sessionTokenPayload) string {
	t.Helper()
	rawHeader, err := json.Marshal(sessionTokenHeader{Version: headerVersion, Type: sessionTokenType, Alg: sessionTokenAlg})
	if err != nil {
		t.Fatalf("marshal test token header: %v", err)
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal test token payload: %v", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(rawHeader)
	encodedPayload := base64.RawURLEncoding.EncodeToString(rawPayload)
	signedContent := encodedHeader + "." + encodedPayload
	signature := base64.RawURLEncoding.EncodeToString(sessionSignature(service.signingKey, signedContent))
	return signedContent + "." + signature
}
