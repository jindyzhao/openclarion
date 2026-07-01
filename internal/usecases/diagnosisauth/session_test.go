package diagnosisauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
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

	session, err := service.AuthenticateSession(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("AuthenticateSession: %v", err)
	}
	if session.Provider != "oidc" || session.Subject != "operator-1" {
		t.Fatalf("session = %+v", session)
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
