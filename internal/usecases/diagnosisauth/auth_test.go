package diagnosisauth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestAuthorizeSessionAccess(t *testing.T) {
	session := SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	tests := []struct {
		name      string
		principal ports.AuthPrincipal
		wantError error
	}{
		{
			name: "owner can access own session",
			principal: ports.AuthPrincipal{
				Subject: "owner-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
			},
		},
		{
			name: "admin can access any session",
			principal: ports.AuthPrincipal{
				Subject: "admin-1",
				Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
			},
		},
		{
			name: "owner cannot access another owner session",
			principal: ports.AuthPrincipal{
				Subject: "owner-2",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
			},
			wantError: ErrUnauthorized,
		},
		{
			name: "missing subject unauthenticated",
			principal: ports.AuthPrincipal{
				Roles: []ports.AuthRole{ports.AuthRoleAdmin},
			},
			wantError: ErrUnauthenticated,
		},
		{
			name: "leader role not accepted in V1",
			principal: ports.AuthPrincipal{
				Subject: "leader-1",
				Roles:   []ports.AuthRole{"leader"},
			},
			wantError: ErrUnauthorized,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := AuthorizeSessionAccess(tc.principal, session)
			if tc.wantError == nil {
				if err != nil {
					t.Fatalf("AuthorizeSessionAccess: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("AuthorizeSessionAccess err = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestValidateTicketPolicyRejectsUnsafeSettings(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TicketPolicy)
		want   string
	}{
		{
			name: "ttl above hard cap",
			mutate: func(p *TicketPolicy) {
				p.TTL = HardMaxTicketTTL + time.Second
			},
			want: "ttl",
		},
		{
			name: "token bytes too small",
			mutate: func(p *TicketPolicy) {
				p.TokenBytes = MinTokenBytes - 1
			},
			want: "token bytes",
		},
		{
			name: "empty scope",
			mutate: func(p *TicketPolicy) {
				p.Scope = " "
			},
			want: "scope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := DefaultTicketPolicy()
			tc.mutate(&policy)
			err := ValidateTicketPolicy(policy)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("ValidateTicketPolicy err = %v, want ErrInvariantViolation", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateTicketPolicy err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestServiceIssuesAndConsumesSingleUseTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x42}, DefaultTokenBytes)))
	session := SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	principal := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}

	ticket, err := service.IssueTicket(context.Background(), principal, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	if ticket.Token == "" || strings.ContainsAny(ticket.Token, "+/=") {
		t.Fatalf("Token = %q, want raw URL-safe base64", ticket.Token)
	}
	if ticket.ExpiresAt.Sub(ticket.IssuedAt) != DefaultTicketTTL {
		t.Fatalf("TTL = %s, want %s", ticket.ExpiresAt.Sub(ticket.IssuedAt), DefaultTicketTTL)
	}

	consumed, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeTicket first: %v", err)
	}
	if consumed.ConsumedAt == nil || !consumed.ConsumedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("ConsumedAt = %v", consumed.ConsumedAt)
	}
	_, err = service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second))
	if !errors.Is(err, ErrTicketConsumed) {
		t.Fatalf("ConsumeTicket second err = %v, want ErrTicketConsumed", err)
	}
}

func TestServiceIssuesAndConsumesAuthorizedTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x62}, DefaultTokenBytes)))
	principal := ports.AuthPrincipal{Subject: "responder-1"}

	ticket, err := service.IssueAuthorizedTicket(context.Background(), principal, "session-1", now)
	if err != nil {
		t.Fatalf("IssueAuthorizedTicket: %v", err)
	}
	consumed, err := service.ConsumeAuthorizedTicket(context.Background(), ticket.Token, "session-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeAuthorizedTicket: %v", err)
	}
	if consumed.Token != "" || consumed.Subject != "responder-1" || consumed.SessionID != "session-1" {
		t.Fatalf("consumed ticket = %+v", consumed)
	}

	otherService := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x63}, DefaultTokenBytes)))
	otherTicket, err := otherService.IssueAuthorizedTicket(context.Background(), principal, "session-1", now)
	if err != nil {
		t.Fatalf("IssueAuthorizedTicket wrong-session setup: %v", err)
	}
	_, err = otherService.ConsumeAuthorizedTicket(context.Background(), otherTicket.Token, "session-2", now.Add(time.Second))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ConsumeAuthorizedTicket wrong session err = %v, want ErrUnauthorized", err)
	}
}

func TestServiceRejectsExpiredTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x24}, DefaultTokenBytes)))
	session := SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	principal := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}
	ticket, err := service.IssueTicket(context.Background(), principal, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	_, err = service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(DefaultTicketTTL))
	if !errors.Is(err, ErrTicketExpired) {
		t.Fatalf("ConsumeTicket err = %v, want ErrTicketExpired", err)
	}
}

func TestServiceRejectsWrongSessionAndUnauthorizedIssue(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x35}, DefaultTokenBytes)))
	session := SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	otherSession := SessionRef{SessionID: "session-2", OwnerSubject: "owner-2"}
	owner := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}
	otherOwner := ports.AuthPrincipal{Subject: "owner-2", Roles: []ports.AuthRole{ports.AuthRoleOwner}}

	if _, err := service.IssueTicket(context.Background(), otherOwner, session, now); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("IssueTicket unauthorized err = %v, want ErrUnauthorized", err)
	}
	ticket, err := service.IssueTicket(context.Background(), owner, session, now)
	if err != nil {
		t.Fatalf("IssueTicket owner: %v", err)
	}
	_, err = service.ConsumeTicket(context.Background(), ticket.Token, otherSession, now.Add(time.Second))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ConsumeTicket wrong session err = %v, want ErrUnauthorized", err)
	}
}

func TestServiceUsesAdminRoleForIssueAndConsume(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newTestService(t, bytes.NewReader(bytes.Repeat([]byte{0x55}, DefaultTokenBytes)))
	session := SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	admin := ports.AuthPrincipal{Subject: "admin-1", Roles: []ports.AuthRole{ports.AuthRoleAdmin}}
	ticket, err := service.IssueTicket(context.Background(), admin, session, now)
	if err != nil {
		t.Fatalf("IssueTicket admin: %v", err)
	}
	if _, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(time.Second)); err != nil {
		t.Fatalf("ConsumeTicket admin: %v", err)
	}
}

func TestMemoryStoreDefensivelyCopiesRolesAndConsumedAt(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	ticket := Ticket{
		Token:     "token-1",
		Subject:   "owner-1",
		Roles:     []ports.AuthRole{ports.AuthRoleOwner},
		SessionID: "session-1",
		Scope:     DefaultTicketScope,
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Minute),
	}
	if err := store.SaveTicket(context.Background(), ticket); err != nil {
		t.Fatalf("SaveTicket: %v", err)
	}
	ticket.Roles[0] = ports.AuthRoleAdmin
	consumed, err := store.ConsumeTicket(context.Background(), "token-1", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeTicket: %v", err)
	}
	if consumed.Roles[0] != ports.AuthRoleOwner {
		t.Fatalf("Roles[0] = %q, want owner", consumed.Roles[0])
	}
	*consumed.ConsumedAt = now.Add(10 * time.Second)
	again, err := store.ConsumeTicket(context.Background(), "token-1", now.Add(2*time.Second))
	if !errors.Is(err, ErrTicketConsumed) {
		t.Fatalf("ConsumeTicket second err = %v, want ErrTicketConsumed", err)
	}
	if again.ConsumedAt != nil {
		t.Fatalf("again.ConsumedAt = %v, want nil on error", again.ConsumedAt)
	}
}

func newTestService(t *testing.T, random *bytes.Reader) Service {
	t.Helper()
	service, err := NewService(NewMemoryStore(), DefaultTicketPolicy(), random)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}
