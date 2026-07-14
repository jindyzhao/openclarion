// Package tenantops owns tenant selection and global tenant administration.
package tenantops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const maxTenantList = 500

var (
	// ErrAccessDenied hides tenant membership details from unauthorized users.
	ErrAccessDenied = errors.New("tenant access denied")
	// ErrTenantDisabled prevents new authenticated sessions in a disabled tenant.
	ErrTenantDisabled = errors.New("tenant is disabled")
)

// Service resolves authenticated tenant selection and manages registry rows.
type Service struct {
	registry ports.TenantRegistry
}

// NewService constructs tenant operations.
func NewService(registry ports.TenantRegistry) (*Service, error) {
	if registry == nil {
		return nil, fmt.Errorf("tenant operations: registry must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return &Service{registry: registry}, nil
}

// ResolveAccess binds an authenticated subject to one active tenant. Every
// authenticated subject retains access to the bootstrap default tenant.
func (s *Service) ResolveAccess(ctx context.Context, subject, requestedKey string, globalAdmin bool) (tenancy.Identity, error) {
	subject, err := normalizeSubject(subject)
	if err != nil {
		return tenancy.Identity{}, err
	}
	rawRequestedKey := requestedKey
	requestedKey = strings.TrimSpace(requestedKey)
	if requestedKey != rawRequestedKey {
		return tenancy.Identity{}, fmt.Errorf("tenant operations: requested tenant key must not contain surrounding whitespace: %w", domain.ErrInvariantViolation)
	}
	if requestedKey == "" {
		requestedKey = domain.DefaultTenantKey
	}
	key, err := domain.NormalizeTenantKey(requestedKey)
	if err != nil || key != requestedKey {
		return tenancy.Identity{}, fmt.Errorf("tenant operations: requested tenant key is invalid: %w", domain.ErrInvariantViolation)
	}
	tenant, err := s.registry.FindTenantByKey(ctx, key)
	if err != nil {
		return tenancy.Identity{}, err
	}
	if tenant.Status != domain.TenantStatusActive {
		return tenancy.Identity{}, ErrTenantDisabled
	}
	if tenant.Key != domain.DefaultTenantKey && !globalAdmin {
		membership, membershipErr := s.registry.FindTenantMembership(ctx, tenant.ID, subject)
		if errors.Is(membershipErr, domain.ErrNotFound) || membershipErr == nil && !membership.Enabled {
			return tenancy.Identity{}, ErrAccessDenied
		}
		if membershipErr != nil {
			return tenancy.Identity{}, membershipErr
		}
	}
	return tenancy.NewIdentity(tenant.ID, tenant.Key)
}

// ResolveIngress resolves an active tenant namespace for a profile-bound
// machine credential. The profile's own authorization remains mandatory for
// non-default tenants.
func (s *Service) ResolveIngress(ctx context.Context, requestedKey string) (tenancy.Identity, error) {
	rawRequestedKey := requestedKey
	requestedKey = strings.TrimSpace(requestedKey)
	if requestedKey != rawRequestedKey {
		return tenancy.Identity{}, fmt.Errorf("tenant operations: requested tenant key must not contain surrounding whitespace: %w", domain.ErrInvariantViolation)
	}
	if requestedKey == "" {
		requestedKey = domain.DefaultTenantKey
	}
	key, err := domain.NormalizeTenantKey(requestedKey)
	if err != nil || key != requestedKey {
		return tenancy.Identity{}, fmt.Errorf("tenant operations: requested tenant key is invalid: %w", domain.ErrInvariantViolation)
	}
	tenant, err := s.registry.FindTenantByKey(ctx, key)
	if err != nil {
		return tenancy.Identity{}, err
	}
	if tenant.Status != domain.TenantStatusActive {
		return tenancy.Identity{}, ErrTenantDisabled
	}
	return tenancy.NewIdentity(tenant.ID, tenant.Key)
}

// ListAccessible returns tenants visible to an authenticated subject. Disabled
// tenants remain visible to members so an owner can re-enable them from the
// default tenant; ResolveAccess still rejects new sessions for them.
func (s *Service) ListAccessible(ctx context.Context, subject string, globalAdmin bool) ([]domain.Tenant, error) {
	subject, err := normalizeSubject(subject)
	if err != nil {
		return nil, err
	}
	var tenants []domain.Tenant
	if globalAdmin {
		tenants, err = s.registry.ListTenants(ctx, maxTenantList-1)
	} else {
		tenants, err = s.registry.ListTenantsForSubject(ctx, subject, maxTenantList-1)
	}
	if err != nil {
		return nil, err
	}
	defaultTenant, err := s.registry.FindTenantByKey(ctx, domain.DefaultTenantKey)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Tenant, 0, len(tenants)+1)
	seen := make(map[domain.TenantID]struct{}, len(tenants)+1)
	out = append(out, defaultTenant)
	seen[defaultTenant.ID] = struct{}{}
	for _, tenant := range tenants {
		if _, exists := seen[tenant.ID]; exists {
			continue
		}
		seen[tenant.ID] = struct{}{}
		out = append(out, tenant)
	}
	return out, nil
}

// CreateTenant creates an active tenant and grants its creator membership.
func (s *Service) CreateTenant(ctx context.Context, key, name, actorSubject string) (domain.Tenant, domain.TenantMembership, error) {
	actorSubject, err := normalizeSubject(actorSubject)
	if err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, err
	}
	key, err = domain.NormalizeTenantKey(key)
	if err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, err
	}
	name, err = domain.NormalizeTenantName(name)
	if err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, err
	}
	if key == domain.DefaultTenantKey {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant operations: default key is reserved: %w", domain.ErrAlreadyExists)
	}
	return s.registry.CreateTenantWithOwner(ctx, domain.Tenant{Key: key, Name: name, Status: domain.TenantStatusActive}, actorSubject)
}

// UpdateStatus changes a non-default tenant lifecycle state.
func (s *Service) UpdateStatus(ctx context.Context, id domain.TenantID, status domain.TenantStatus) (domain.Tenant, error) {
	return s.registry.UpdateTenantStatus(ctx, id, status)
}

// ListMemberships returns one tenant's membership records.
func (s *Service) ListMemberships(ctx context.Context, tenantID domain.TenantID) ([]domain.TenantMembership, error) {
	return s.registry.ListTenantMemberships(ctx, tenantID, maxTenantList)
}

// CanManage reports whether a subject owns the tenant registry entry.
func (s *Service) CanManage(ctx context.Context, tenantID domain.TenantID, subject string, globalAdmin bool) (bool, error) {
	if globalAdmin {
		return true, nil
	}
	subject, err := normalizeSubject(subject)
	if err != nil {
		return false, err
	}
	membership, err := s.registry.FindTenantMembership(ctx, tenantID, subject)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return membership.Enabled && membership.Role == domain.TenantMembershipRoleOwner, nil
}

// SetMembership creates or enables/disables one tenant membership.
func (s *Service) SetMembership(
	ctx context.Context,
	tenantID domain.TenantID,
	subject string,
	role domain.TenantMembershipRole,
	enabled bool,
	actorSubject string,
) (domain.TenantMembership, error) {
	subject, err := normalizeSubject(subject)
	if err != nil {
		return domain.TenantMembership{}, err
	}
	actorSubject, err = normalizeSubject(actorSubject)
	if err != nil {
		return domain.TenantMembership{}, err
	}
	if err := domain.ValidateTenantMembershipRole(role); err != nil {
		return domain.TenantMembership{}, err
	}
	return s.registry.SetTenantMembership(ctx, domain.TenantMembership{
		TenantID:  tenantID,
		Subject:   subject,
		Role:      role,
		Enabled:   enabled,
		CreatedBy: actorSubject,
	})
}

func normalizeSubject(raw string) (string, error) {
	subject := strings.TrimSpace(raw)
	if subject == "" || subject != raw || len(subject) > 256 || strings.ContainsAny(subject, "\x00\r\n") {
		return "", fmt.Errorf("tenant operations: subject is invalid: %w", domain.ErrInvariantViolation)
	}
	return subject, nil
}
