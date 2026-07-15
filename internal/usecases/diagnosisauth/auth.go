// Package diagnosisauth owns M5 diagnosis-room authentication support that is
// independent of HTTP, WebSocket, OIDC, and persistence adapters.
package diagnosisauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// HardMaxTicketTTL is the maximum accepted WebSocket ticket lifetime.
	HardMaxTicketTTL = 30 * time.Second

	// MinTokenBytes is the minimum accepted random-token entropy size.
	MinTokenBytes = 16

	// MaxTokenBytes is the maximum accepted random-token entropy size.
	MaxTokenBytes = 64

	// DefaultTicketTTL is the default WebSocket ticket lifetime.
	DefaultTicketTTL = 30 * time.Second
	// DefaultTokenBytes is the default random-token entropy size.
	DefaultTokenBytes = 32
	// DefaultTicketScope is the default purpose marker for WebSocket tickets.
	DefaultTicketScope = "diagnosis_ws"
)

var (
	// ErrUnauthenticated is returned when no valid identity or ticket exists.
	ErrUnauthenticated = errors.New("diagnosis auth: unauthenticated")
	// ErrUnauthorized is returned when an authenticated principal lacks access.
	ErrUnauthorized = errors.New("diagnosis auth: unauthorized")
	// ErrTicketConsumed is returned when a single-use ticket was already used.
	ErrTicketConsumed = errors.New("diagnosis auth: ticket already consumed")
	// ErrTicketExpired is returned when a ticket is used after its expiry.
	ErrTicketExpired = errors.New("diagnosis auth: ticket expired")
)

// SessionRef is the minimum session ownership data required for V1 RBAC.
type SessionRef struct {
	SessionID    string
	OwnerSubject string
}

// TicketPolicy constrains WebSocket ticket issuance.
type TicketPolicy struct {
	TTL        time.Duration
	TokenBytes int
	Scope      string
}

// Ticket is the short-lived single-use credential passed in the WebSocket
// query string. It must not contain long-lived bearer credentials.
type Ticket struct {
	Token      string
	Subject    string
	Roles      []ports.AuthRole
	TenantID   domain.TenantID
	TenantKey  string
	SessionID  string
	Scope      string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

// Store persists issued tickets and consumes them atomically.
type Store interface {
	SaveTicket(ctx context.Context, ticket Ticket) error
	ConsumeTicket(ctx context.Context, token string, now time.Time) (Ticket, error)
}

// Service issues and consumes M5 WebSocket tickets.
type Service struct {
	store  Store
	random io.Reader
	policy TicketPolicy
}

// NewService constructs a ticket service. The random reader is injectable for
// tests; production should pass nil to use crypto/rand.Reader.
func NewService(store Store, policy TicketPolicy, random io.Reader) (Service, error) {
	if store == nil {
		return Service{}, fmt.Errorf("diagnosis auth: ticket store must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if err := ValidateTicketPolicy(policy); err != nil {
		return Service{}, err
	}
	if random == nil {
		random = rand.Reader
	}
	return Service{store: store, random: random, policy: policy}, nil
}

// DefaultTicketPolicy returns the V1 WebSocket ticket boundary: short-lived,
// single-purpose, and backed by at least 256 bits of entropy by default.
func DefaultTicketPolicy() TicketPolicy {
	return TicketPolicy{
		TTL:        DefaultTicketTTL,
		TokenBytes: DefaultTokenBytes,
		Scope:      DefaultTicketScope,
	}
}

// ValidateTicketPolicy rejects ticket settings that widen the replay window or
// reduce token entropy below the V1 security boundary.
func ValidateTicketPolicy(policy TicketPolicy) error {
	if policy.TTL <= 0 || policy.TTL > HardMaxTicketTTL {
		return fmt.Errorf("diagnosis auth: ticket ttl %s must be in (0,%s]: %w", policy.TTL, HardMaxTicketTTL, domain.ErrInvariantViolation)
	}
	if policy.TokenBytes < MinTokenBytes || policy.TokenBytes > MaxTokenBytes {
		return fmt.Errorf("diagnosis auth: token bytes %d must be in [%d,%d]: %w", policy.TokenBytes, MinTokenBytes, MaxTokenBytes, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(policy.Scope) == "" {
		return fmt.Errorf("diagnosis auth: ticket scope must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return nil
}

// AuthorizeSessionAccess enforces V1 owner/admin RBAC.
func AuthorizeSessionAccess(principal ports.AuthPrincipal, session SessionRef) error {
	if strings.TrimSpace(principal.Subject) == "" {
		return fmt.Errorf("diagnosis auth: principal subject is required: %w", ErrUnauthenticated)
	}
	if strings.TrimSpace(session.SessionID) == "" {
		return fmt.Errorf("diagnosis auth: session id is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(session.OwnerSubject) == "" {
		return fmt.Errorf("diagnosis auth: session owner is required: %w", domain.ErrInvariantViolation)
	}
	if hasRole(principal, ports.AuthRoleAdmin) {
		return nil
	}
	if hasRole(principal, ports.AuthRoleOwner) && principal.Subject == session.OwnerSubject {
		return nil
	}
	return fmt.Errorf("diagnosis auth: subject %q cannot access session %q: %w", principal.Subject, session.SessionID, ErrUnauthorized)
}

// IssueTicket authorizes the principal for the session and stores a new ticket.
func (s Service) IssueTicket(ctx context.Context, principal ports.AuthPrincipal, session SessionRef, now time.Time) (Ticket, error) {
	if now.IsZero() {
		return Ticket{}, fmt.Errorf("diagnosis auth: now must be set: %w", domain.ErrInvariantViolation)
	}
	if err := AuthorizeSessionAccess(principal, session); err != nil {
		return Ticket{}, err
	}
	return s.issueAuthorizedTicket(ctx, principal, session.SessionID, now)
}

// IssueAuthorizedTicket stores a new ticket after the caller has already
// authorized the principal for the diagnosis session.
func (s Service) IssueAuthorizedTicket(ctx context.Context, principal ports.AuthPrincipal, sessionID string, now time.Time) (Ticket, error) {
	if now.IsZero() {
		return Ticket{}, fmt.Errorf("diagnosis auth: now must be set: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(principal.Subject) == "" {
		return Ticket{}, fmt.Errorf("diagnosis auth: principal subject is required: %w", ErrUnauthenticated)
	}
	if strings.TrimSpace(sessionID) == "" {
		return Ticket{}, fmt.Errorf("diagnosis auth: session id is required: %w", domain.ErrInvariantViolation)
	}
	return s.issueAuthorizedTicket(ctx, principal, sessionID, now)
}

func (s Service) issueAuthorizedTicket(ctx context.Context, principal ports.AuthPrincipal, sessionID string, now time.Time) (Ticket, error) {
	tenantIdentity, err := sessionTenantIdentity(principal)
	if err != nil {
		return Ticket{}, err
	}
	token, err := randomToken(s.random, s.policy.TokenBytes)
	if err != nil {
		return Ticket{}, err
	}
	ticket := Ticket{
		Token:     token,
		Subject:   principal.Subject,
		Roles:     append([]ports.AuthRole(nil), principal.Roles...),
		TenantID:  tenantIdentity.ID,
		TenantKey: tenantIdentity.Key,
		SessionID: sessionID,
		Scope:     s.policy.Scope,
		IssuedAt:  now.UTC(),
		ExpiresAt: now.Add(s.policy.TTL).UTC(),
	}
	if err := validateTicket(ticket); err != nil {
		return Ticket{}, err
	}
	if err := s.store.SaveTicket(ctx, ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// ConsumeTicket atomically consumes a ticket and rechecks that it belongs to
// the requested session. The returned Ticket includes ConsumedAt.
func (s Service) ConsumeTicket(ctx context.Context, token string, session SessionRef, now time.Time) (Ticket, error) {
	if strings.TrimSpace(session.SessionID) == "" {
		return Ticket{}, fmt.Errorf("diagnosis auth: session id is required: %w", domain.ErrInvariantViolation)
	}
	ticket, err := s.consumeAuthorizedTicket(ctx, token, session.SessionID, now)
	if err != nil {
		return Ticket{}, err
	}
	principal := ports.AuthPrincipal{
		Subject:   ticket.Subject,
		Roles:     ticket.Roles,
		TenantID:  ticket.TenantID,
		TenantKey: ticket.TenantKey,
	}
	if err := AuthorizeSessionAccess(principal, session); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// ConsumeAuthorizedTicket atomically consumes a ticket after the requested
// session has already been authorized by the caller. The returned Ticket
// includes ConsumedAt and has Token redacted.
func (s Service) ConsumeAuthorizedTicket(ctx context.Context, token string, sessionID string, now time.Time) (Ticket, error) {
	if strings.TrimSpace(sessionID) == "" {
		return Ticket{}, fmt.Errorf("diagnosis auth: session id is required: %w", domain.ErrInvariantViolation)
	}
	return s.consumeAuthorizedTicket(ctx, token, sessionID, now)
}

func (s Service) consumeAuthorizedTicket(ctx context.Context, token string, sessionID string, now time.Time) (Ticket, error) {
	if strings.TrimSpace(token) == "" {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket token is required: %w", ErrUnauthenticated)
	}
	if now.IsZero() {
		return Ticket{}, fmt.Errorf("diagnosis auth: now must be set: %w", domain.ErrInvariantViolation)
	}
	ticket, err := s.store.ConsumeTicket(ctx, token, now.UTC())
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Scope != s.policy.Scope {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket scope %q does not match %q: %w", ticket.Scope, s.policy.Scope, ErrUnauthorized)
	}
	if ticket.SessionID != sessionID {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket session %q does not match %q: %w", ticket.SessionID, sessionID, ErrUnauthorized)
	}
	ticket.Token = ""
	return ticket, nil
}

func randomToken(r io.Reader, bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("diagnosis auth: generate ticket token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validateTicket(ticket Ticket) error {
	if strings.TrimSpace(ticket.Token) == "" {
		return fmt.Errorf("diagnosis auth: ticket token is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.Subject) == "" {
		return fmt.Errorf("diagnosis auth: ticket subject is required: %w", domain.ErrInvariantViolation)
	}
	if _, err := tenancy.NewIdentity(ticket.TenantID, ticket.TenantKey); err != nil {
		return fmt.Errorf("diagnosis auth: ticket tenant binding is invalid: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.SessionID) == "" {
		return fmt.Errorf("diagnosis auth: ticket session id is required: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(ticket.Scope) == "" {
		return fmt.Errorf("diagnosis auth: ticket scope is required: %w", domain.ErrInvariantViolation)
	}
	if ticket.IssuedAt.IsZero() {
		return fmt.Errorf("diagnosis auth: ticket issued_at is required: %w", domain.ErrInvariantViolation)
	}
	if !ticket.ExpiresAt.After(ticket.IssuedAt) {
		return fmt.Errorf("diagnosis auth: ticket expiry must be after issue time: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func hasRole(principal ports.AuthPrincipal, role ports.AuthRole) bool {
	return slices.Contains(principal.Roles, role)
}

// MemoryStore is a deterministic in-memory Store for unit tests and local
// development. Production deployments should back Store with PostgreSQL.
type MemoryStore struct {
	mu      sync.Mutex
	tickets map[string]Ticket
}

// NewMemoryStore constructs an empty in-memory ticket store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tickets: map[string]Ticket{}}
}

// SaveTicket stores a new ticket by token.
func (s *MemoryStore) SaveTicket(_ context.Context, ticket Ticket) error {
	if err := validateTicket(ticket); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tickets[ticket.Token]; exists {
		return fmt.Errorf("diagnosis auth: ticket already exists: %w", domain.ErrAlreadyExists)
	}
	s.tickets[ticket.Token] = cloneTicket(ticket)
	return nil
}

// ConsumeTicket marks a ticket consumed if it exists, is unexpired, and has not
// already been used.
func (s *MemoryStore) ConsumeTicket(_ context.Context, token string, now time.Time) (Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticket, exists := s.tickets[token]
	if !exists {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket not found: %w", ErrUnauthenticated)
	}
	if ticket.ConsumedAt != nil {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket already consumed: %w", ErrTicketConsumed)
	}
	if !now.Before(ticket.ExpiresAt) {
		return Ticket{}, fmt.Errorf("diagnosis auth: ticket expired at %s: %w", ticket.ExpiresAt, ErrTicketExpired)
	}
	consumedAt := now.UTC()
	ticket.ConsumedAt = &consumedAt
	s.tickets[token] = ticket
	return cloneTicket(ticket), nil
}

func cloneTicket(ticket Ticket) Ticket {
	ticket.Roles = append([]ports.AuthRole(nil), ticket.Roles...)
	if ticket.ConsumedAt != nil {
		consumedAt := *ticket.ConsumedAt
		ticket.ConsumedAt = &consumedAt
	}
	return ticket
}
