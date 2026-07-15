package repository

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosisauthticket"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestDiagnosisAuthTicketStore_IssueConsumeSingleUse(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	store := mustNewDiagnosisAuthTicketStore(t)
	service := mustNewDiagnosisAuthService(t, store, bytes.NewReader(bytes.Repeat([]byte{0x61}, diagnosisauth.DefaultTokenBytes)))
	now := time.Date(2026, 5, 28, 12, 0, 0, 123456789, time.UTC)
	session := diagnosisauth.SessionRef{SessionID: "diag-session-1", OwnerSubject: "owner-1"}
	principal := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}

	ticket, err := service.IssueTicket(ctx, principal, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	row, err := integration.client.DiagnosisAuthTicket.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query persisted ticket: %v", err)
	}
	if row.TokenHash != ticketTokenHash(ticket.Token) {
		t.Fatalf("TokenHash = %q, want hash of issued token", row.TokenHash)
	}
	if row.TokenHash == ticket.Token {
		t.Fatalf("TokenHash unexpectedly equals raw token")
	}
	if !row.IssuedAt.Equal(domain.NormalizeUTCMicro(now)) {
		t.Fatalf("IssuedAt = %s, want %s", row.IssuedAt, domain.NormalizeUTCMicro(now))
	}
	if len(row.Roles) != 1 || row.Roles[0] != string(ports.AuthRoleOwner) {
		t.Fatalf("Roles = %#v, want owner", row.Roles)
	}

	consumed, err := service.ConsumeTicket(ctx, ticket.Token, session, now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeTicket first: %v", err)
	}
	if consumed.Token != "" {
		t.Fatalf("consumed.Token = %q, want empty token after consume", consumed.Token)
	}
	if consumed.ConsumedAt == nil || !consumed.ConsumedAt.Equal(domain.NormalizeUTCMicro(now.Add(time.Second))) {
		t.Fatalf("ConsumedAt = %v, want %s", consumed.ConsumedAt, domain.NormalizeUTCMicro(now.Add(time.Second)))
	}
	_, err = service.ConsumeTicket(ctx, ticket.Token, session, now.Add(2*time.Second))
	if !errors.Is(err, diagnosisauth.ErrTicketConsumed) {
		t.Fatalf("ConsumeTicket second err = %v, want ErrTicketConsumed", err)
	}
	if strings.Contains(err.Error(), ticket.Token) {
		t.Fatalf("consume error leaked raw token: %v", err)
	}
}

func TestDiagnosisAuthTicketStore_IssuesAuthorizedTicketWithoutLegacyRoles(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	store := mustNewDiagnosisAuthTicketStore(t)
	service := mustNewDiagnosisAuthService(t, store, bytes.NewReader(bytes.Repeat([]byte{0x64}, diagnosisauth.DefaultTokenBytes)))
	now := time.Date(2026, 6, 28, 9, 30, 0, 0, time.UTC)
	principal := ports.AuthPrincipal{Subject: "responder-1"}

	ticket, err := service.IssueAuthorizedTicket(ctx, principal, "diag-session-rbac", now)
	if err != nil {
		t.Fatalf("IssueAuthorizedTicket: %v", err)
	}
	row, err := integration.client.DiagnosisAuthTicket.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query persisted ticket: %v", err)
	}
	if len(row.Roles) != 0 {
		t.Fatalf("Roles = %#v, want empty roles for local RBAC ticket", row.Roles)
	}
	consumed, err := service.ConsumeAuthorizedTicket(ctx, ticket.Token, "diag-session-rbac", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeAuthorizedTicket: %v", err)
	}
	if consumed.Subject != "responder-1" || len(consumed.Roles) != 0 || consumed.Token != "" {
		t.Fatalf("consumed ticket = %+v", consumed)
	}
}

func TestDiagnosisAuthTicketStore_RejectsExpiredAndUnknownTickets(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	store := mustNewDiagnosisAuthTicketStore(t)
	service := mustNewDiagnosisAuthService(t, store, bytes.NewReader(bytes.Repeat([]byte{0x62}, diagnosisauth.DefaultTokenBytes)))
	now := time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC)
	session := diagnosisauth.SessionRef{SessionID: "diag-session-expired", OwnerSubject: "owner-1"}
	principal := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}

	ticket, err := service.IssueTicket(ctx, principal, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	_, err = service.ConsumeTicket(ctx, ticket.Token, session, now.Add(diagnosisauth.DefaultTicketTTL))
	if !errors.Is(err, diagnosisauth.ErrTicketExpired) {
		t.Fatalf("ConsumeTicket expired err = %v, want ErrTicketExpired", err)
	}
	row, err := integration.client.DiagnosisAuthTicket.Query().
		Where(diagnosisauthticket.TokenHashEQ(ticketTokenHash(ticket.Token))).
		Only(ctx)
	if err != nil {
		t.Fatalf("query expired ticket: %v", err)
	}
	if row.ConsumedAt != nil {
		t.Fatalf("expired ticket ConsumedAt = %v, want nil", row.ConsumedAt)
	}

	_, err = service.ConsumeTicket(ctx, "unknown-ticket", session, now.Add(time.Second))
	if !errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		t.Fatalf("ConsumeTicket unknown err = %v, want ErrUnauthenticated", err)
	}
}

func TestDiagnosisAuthTicketStore_RejectsDuplicateSaveAndBadConstruction(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	if _, err := NewDiagnosisAuthTicketStore(nil); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("NewDiagnosisAuthTicketStore nil err = %v, want ErrInvariantViolation", err)
	}
	store := mustNewDiagnosisAuthTicketStore(t)
	ticket := diagnosisauth.Ticket{
		Token:     "raw-duplicate-token",
		Subject:   "owner-1",
		Roles:     []ports.AuthRole{ports.AuthRoleOwner},
		TenantID:  1,
		TenantKey: domain.DefaultTenantKey,
		SessionID: "diag-session-dup",
		Scope:     diagnosisauth.DefaultTicketScope,
		IssuedAt:  time.Date(2026, 5, 28, 13, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 5, 28, 13, 0, 30, 0, time.UTC),
	}
	if err := store.SaveTicket(ctx, ticket); err != nil {
		t.Fatalf("SaveTicket first: %v", err)
	}
	err := store.SaveTicket(ctx, ticket)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("SaveTicket duplicate err = %v, want ErrAlreadyExists", err)
	}
	if strings.Contains(err.Error(), ticket.Token) {
		t.Fatalf("duplicate error leaked raw token: %v", err)
	}
}

func TestDiagnosisAuthTicketStore_ConcurrentConsumeAllowsOneWinner(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	store := mustNewDiagnosisAuthTicketStore(t)
	service := mustNewDiagnosisAuthService(t, store, bytes.NewReader(bytes.Repeat([]byte{0x63}, diagnosisauth.DefaultTokenBytes)))
	now := time.Date(2026, 5, 28, 13, 30, 0, 0, time.UTC)
	session := diagnosisauth.SessionRef{SessionID: "diag-session-race", OwnerSubject: "owner-1"}
	principal := ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}
	ticket, err := service.IssueTicket(ctx, principal, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}

	const workers = 8
	start := make(chan struct{})
	var successes atomic.Int32
	var consumedErrors atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			<-start
			_, cerr := service.ConsumeTicket(ctx, ticket.Token, session, now.Add(time.Second))
			if cerr == nil {
				successes.Add(1)
				return
			}
			if errors.Is(cerr, diagnosisauth.ErrTicketConsumed) {
				consumedErrors.Add(1)
				return
			}
			t.Errorf("ConsumeTicket concurrent err = %v", cerr)
		}()
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successes = %d, want 1", successes.Load())
	}
	if consumedErrors.Load() != workers-1 {
		t.Fatalf("consumedErrors = %d, want %d", consumedErrors.Load(), workers-1)
	}
}

func mustNewDiagnosisAuthTicketStore(t *testing.T) *DiagnosisAuthTicketStore {
	t.Helper()
	store, err := NewDiagnosisAuthTicketStore(integration.client)
	if err != nil {
		t.Fatalf("NewDiagnosisAuthTicketStore: %v", err)
	}
	return store
}

func mustNewDiagnosisAuthService(t *testing.T, store diagnosisauth.Store, random *bytes.Reader) diagnosisauth.Service {
	t.Helper()
	service, err := diagnosisauth.NewService(store, diagnosisauth.DefaultTicketPolicy(), random)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}
