// Package tenancy carries the authenticated tenant identity across transport,
// orchestration, and persistence boundaries.
package tenancy

import (
	"context"
	"fmt"

	"github.com/openclarion/openclarion/internal/domain"
)

// Identity is the immutable tenant binding attached to one operation.
type Identity struct {
	ID  domain.TenantID
	Key string
}

// DefaultIdentity returns the immutable bootstrap tenant binding used by
// compatibility paths that predate explicit tenant selection.
func DefaultIdentity() Identity {
	return Identity{ID: domain.DefaultTenantID, Key: domain.DefaultTenantKey}
}

type contextKey struct{}

// NewIdentity validates a tenant identity received from a trusted registry.
func NewIdentity(id domain.TenantID, key string) (Identity, error) {
	if id <= 0 {
		return Identity{}, fmt.Errorf("tenancy: tenant id must be positive: %w", domain.ErrInvariantViolation)
	}
	normalized, err := domain.NormalizeTenantKey(key)
	if err != nil {
		return Identity{}, fmt.Errorf("tenancy: %w", err)
	}
	if normalized != key {
		return Identity{}, fmt.Errorf("tenancy: tenant key must already be normalized: %w", domain.ErrInvariantViolation)
	}
	return Identity{ID: id, Key: key}, nil
}

// WithTenant returns a child context bound to the validated identity.
func WithTenant(parent context.Context, identity Identity) (context.Context, error) {
	if parent == nil {
		return nil, fmt.Errorf("tenancy: parent context must be non-nil: %w", domain.ErrInvariantViolation)
	}
	validated, err := NewIdentity(identity.ID, identity.Key)
	if err != nil {
		return nil, err
	}
	return context.WithValue(parent, contextKey{}, validated), nil
}

// FromContext reads the current tenant identity.
func FromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	identity, ok := ctx.Value(contextKey{}).(Identity)
	if !ok || identity.ID <= 0 || identity.Key == "" {
		return Identity{}, false
	}
	return identity, true
}

// Require returns the current identity or a fail-closed error.
func Require(ctx context.Context) (Identity, error) {
	identity, ok := FromContext(ctx)
	if !ok {
		return Identity{}, fmt.Errorf("tenancy: tenant identity is missing: %w", domain.ErrPreconditionFailed)
	}
	return identity, nil
}

// EnsureDefault binds unscoped local/background operations to the bootstrap
// tenant while preserving an explicitly authenticated tenant.
func EnsureDefault(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	if _, ok := FromContext(ctx); ok {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, DefaultIdentity())
}
