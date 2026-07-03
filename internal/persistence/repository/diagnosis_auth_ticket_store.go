package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosisauthticket"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// DiagnosisAuthTicketStore is the PostgreSQL-backed implementation of
// diagnosisauth.Store.
type DiagnosisAuthTicketStore struct {
	client *ent.Client
}

var _ diagnosisauth.Store = (*DiagnosisAuthTicketStore)(nil)

// NewDiagnosisAuthTicketStore constructs a ticket store backed by an Ent
// client. The caller owns the client lifecycle.
func NewDiagnosisAuthTicketStore(client *ent.Client) (*DiagnosisAuthTicketStore, error) {
	if client == nil {
		return nil, fmt.Errorf("diagnosis auth ticket store: ent client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return &DiagnosisAuthTicketStore{client: client}, nil
}

// SaveTicket persists a newly issued ticket. The raw token is never stored;
// only SHA-256(token) is written.
func (s *DiagnosisAuthTicketStore) SaveTicket(ctx context.Context, ticket diagnosisauth.Ticket) error {
	if err := validateTicketForPersistence(ticket); err != nil {
		return err
	}
	issuedAt := domain.NormalizeUTCMicro(ticket.IssuedAt)
	expiresAt := domain.NormalizeUTCMicro(ticket.ExpiresAt)
	_, err := s.client.DiagnosisAuthTicket.Create().
		SetTokenHash(ticketTokenHash(ticket.Token)).
		SetSubject(ticket.Subject).
		SetRoles(authRolesToStrings(ticket.Roles)).
		SetSessionID(ticket.SessionID).
		SetScope(ticket.Scope).
		SetIssuedAt(issuedAt).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("save diagnosis auth ticket: %w", asAlreadyExists(err))
	}
	return nil
}

// ConsumeTicket marks an unexpired ticket as consumed with a conditional
// update. Concurrent callers racing for the same token can only produce one
// successful consume.
func (s *DiagnosisAuthTicketStore) ConsumeTicket(ctx context.Context, token string, now time.Time) (diagnosisauth.Ticket, error) {
	if strings.TrimSpace(token) == "" {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: token is required: %w", diagnosisauth.ErrUnauthenticated)
	}
	if now.IsZero() {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: now must be set: %w", domain.ErrInvariantViolation)
	}
	now = domain.NormalizeUTCMicro(now)
	hash := ticketTokenHash(token)
	row, err := s.client.DiagnosisAuthTicket.Query().
		Where(diagnosisauthticket.TokenHashEQ(hash)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: ticket not found: %w", diagnosisauth.ErrUnauthenticated)
	}
	if err != nil {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: lookup ticket: %w", err)
	}
	if row.ConsumedAt != nil {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: ticket already consumed: %w", diagnosisauth.ErrTicketConsumed)
	}
	if !now.Before(row.ExpiresAt) {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: ticket expired at %s: %w", row.ExpiresAt, diagnosisauth.ErrTicketExpired)
	}
	updated, err := s.client.DiagnosisAuthTicket.Update().
		Where(
			diagnosisauthticket.IDEQ(row.ID),
			diagnosisauthticket.ConsumedAtIsNil(),
			diagnosisauthticket.ExpiresAtGT(now),
		).
		SetConsumedAt(now).
		Save(ctx)
	if err != nil {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: consume ticket: %w", err)
	}
	if updated != 1 {
		return diagnosisauth.Ticket{}, s.classifyConsumeMiss(ctx, row.ID, now)
	}
	consumed, err := s.client.DiagnosisAuthTicket.Get(ctx, row.ID)
	if err != nil {
		return diagnosisauth.Ticket{}, fmt.Errorf("diagnosis auth ticket store: reload consumed ticket: %w", err)
	}
	return diagnosisAuthTicketToUsecase(consumed), nil
}

func (s *DiagnosisAuthTicketStore) classifyConsumeMiss(ctx context.Context, id int, now time.Time) error {
	row, err := s.client.DiagnosisAuthTicket.Get(ctx, id)
	if ent.IsNotFound(err) {
		return fmt.Errorf("diagnosis auth ticket store: ticket not found: %w", diagnosisauth.ErrUnauthenticated)
	}
	if err != nil {
		return fmt.Errorf("diagnosis auth ticket store: reload ticket after consume miss: %w", err)
	}
	if row.ConsumedAt != nil {
		return fmt.Errorf("diagnosis auth ticket store: ticket already consumed: %w", diagnosisauth.ErrTicketConsumed)
	}
	if !now.Before(row.ExpiresAt) {
		return fmt.Errorf("diagnosis auth ticket store: ticket expired at %s: %w", row.ExpiresAt, diagnosisauth.ErrTicketExpired)
	}
	return fmt.Errorf("diagnosis auth ticket store: ticket consume lost update: %w", domain.ErrInvariantViolation)
}

func validateTicketForPersistence(ticket diagnosisauth.Ticket) error {
	if strings.TrimSpace(ticket.Token) == "" {
		return fmt.Errorf("diagnosis auth ticket store: token is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.Subject) == "" {
		return fmt.Errorf("diagnosis auth ticket store: subject is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.SessionID) == "" {
		return fmt.Errorf("diagnosis auth ticket store: session id is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.Scope) == "" {
		return fmt.Errorf("diagnosis auth ticket store: scope is required: %w", domain.ErrInvariantViolation)
	}
	for _, role := range ticket.Roles {
		if strings.TrimSpace(string(role)) == "" {
			return fmt.Errorf("diagnosis auth ticket store: roles must be non-empty: %w", domain.ErrInvariantViolation)
		}
	}
	if ticket.IssuedAt.IsZero() {
		return fmt.Errorf("diagnosis auth ticket store: issued_at is required: %w", domain.ErrInvariantViolation)
	}
	if !ticket.ExpiresAt.After(ticket.IssuedAt) {
		return fmt.Errorf("diagnosis auth ticket store: expires_at must be after issued_at: %w", domain.ErrInvariantViolation)
	}
	if ticket.ConsumedAt != nil {
		return fmt.Errorf("diagnosis auth ticket store: new tickets must not be pre-consumed: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func ticketTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func diagnosisAuthTicketToUsecase(row *ent.DiagnosisAuthTicket) diagnosisauth.Ticket {
	ticket := diagnosisauth.Ticket{
		Subject:   row.Subject,
		Roles:     authRolesFromStrings(row.Roles),
		SessionID: row.SessionID,
		Scope:     row.Scope,
		IssuedAt:  row.IssuedAt,
		ExpiresAt: row.ExpiresAt,
	}
	if row.ConsumedAt != nil {
		consumedAt := *row.ConsumedAt
		ticket.ConsumedAt = &consumedAt
	}
	return ticket
}

func authRolesToStrings(roles []ports.AuthRole) []string {
	out := make([]string, len(roles))
	for i, role := range roles {
		out[i] = string(role)
	}
	return out
}

func authRolesFromStrings(roles []string) []ports.AuthRole {
	out := make([]ports.AuthRole, len(roles))
	for i, role := range roles {
		out[i] = ports.AuthRole(role)
	}
	return out
}
