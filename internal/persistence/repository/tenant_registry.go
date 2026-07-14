package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/tenant"
	"github.com/openclarion/openclarion/internal/persistence/ent/tenantmembership"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// TenantRegistry is the Ent-backed global tenant registry.
type TenantRegistry struct {
	client *ent.Client
}

var _ ports.TenantRegistry = (*TenantRegistry)(nil)

// NewTenantRegistry constructs a registry over an existing Ent client.
func NewTenantRegistry(client *ent.Client) (*TenantRegistry, error) {
	if client == nil {
		return nil, fmt.Errorf("tenant registry: ent client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return &TenantRegistry{client: client}, nil
}

// FindTenantByKey returns one tenant by its normalized registry key.
func (r *TenantRegistry) FindTenantByKey(ctx context.Context, key string) (domain.Tenant, error) {
	normalized, err := domain.NormalizeTenantKey(key)
	if err != nil || normalized != key {
		return domain.Tenant{}, fmt.Errorf("tenant registry: invalid tenant key: %w", domain.ErrInvariantViolation)
	}
	row, err := r.client.Tenant.Query().Where(tenant.KeyEQ(key)).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.Tenant{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("tenant registry: find tenant: %w", err)
	}
	return tenantToDomain(row), nil
}

// ListTenants returns a deterministic bounded tenant registry page.
func (r *TenantRegistry) ListTenants(ctx context.Context, limit int) ([]domain.Tenant, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("tenant registry: limit must be positive: %w", domain.ErrInvariantViolation)
	}
	rows, err := r.client.Tenant.Query().
		Order(ent.Asc(tenant.FieldName), ent.Asc(tenant.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant registry: list tenants: %w", err)
	}
	return tenantsToDomain(rows), nil
}

// ListTenantsForSubject returns active memberships visible to one subject.
func (r *TenantRegistry) ListTenantsForSubject(ctx context.Context, subject string, limit int) ([]domain.Tenant, error) {
	if err := validateTenantSubject(subject); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("tenant registry: limit must be positive: %w", domain.ErrInvariantViolation)
	}
	rows, err := r.client.Tenant.Query().
		Where(tenant.HasMembershipsWith(
			tenantmembership.SubjectEQ(subject),
			tenantmembership.EnabledEQ(true),
		)).
		Order(ent.Asc(tenant.FieldName), ent.Asc(tenant.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant registry: list tenants for subject: %w", err)
	}
	return tenantsToDomain(rows), nil
}

// FindTenantMembership returns one subject's membership in a tenant.
func (r *TenantRegistry) FindTenantMembership(ctx context.Context, tenantID domain.TenantID, subject string) (domain.TenantMembership, error) {
	if tenantID <= 0 {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: tenant id must be positive: %w", domain.ErrInvariantViolation)
	}
	if err := validateTenantSubject(subject); err != nil {
		return domain.TenantMembership{}, err
	}
	row, err := r.client.TenantMembership.Query().
		Where(
			tenantmembership.TenantIDEQ(int(tenantID)),
			tenantmembership.SubjectEQ(subject),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return domain.TenantMembership{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: find membership: %w", err)
	}
	return tenantMembershipToDomain(row), nil
}

// CreateTenantWithOwner atomically creates a tenant and its first owner.
func (r *TenantRegistry) CreateTenantWithOwner(
	ctx context.Context,
	input domain.Tenant,
	ownerSubject string,
) (domain.Tenant, domain.TenantMembership, error) {
	key, err := domain.NormalizeTenantKey(input.Key)
	if err != nil || key != input.Key {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant registry: tenant key must be normalized: %w", domain.ErrInvariantViolation)
	}
	name, err := domain.NormalizeTenantName(input.Name)
	if err != nil || name != input.Name {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant registry: tenant name must be normalized: %w", domain.ErrInvariantViolation)
	}
	if input.Status != domain.TenantStatusActive {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant registry: new tenant must be active: %w", domain.ErrInvariantViolation)
	}
	if err := validateTenantSubject(ownerSubject); err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, err
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant registry: begin create transaction: %w", err)
	}
	rollback := func(cause error) (domain.Tenant, domain.TenantMembership, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return domain.Tenant{}, domain.TenantMembership{}, errors.Join(
				cause,
				fmt.Errorf("tenant registry: rollback create transaction: %w", rollbackErr),
			)
		}
		return domain.Tenant{}, domain.TenantMembership{}, cause
	}
	tenantRow, err := tx.Tenant.Create().SetKey(key).SetName(name).SetStatus(string(input.Status)).Save(ctx)
	if err != nil {
		return rollback(fmt.Errorf("tenant registry: create tenant: %w", asAlreadyExists(err)))
	}
	membershipRow, err := tx.TenantMembership.Create().
		SetTenantID(tenantRow.ID).
		SetSubject(ownerSubject).
		SetRole(string(domain.TenantMembershipRoleOwner)).
		SetEnabled(true).
		SetCreatedBy(ownerSubject).
		Save(ctx)
	if err != nil {
		return rollback(fmt.Errorf("tenant registry: create owner membership: %w", asAlreadyExists(err)))
	}
	if err := tx.Commit(); err != nil {
		return domain.Tenant{}, domain.TenantMembership{}, fmt.Errorf("tenant registry: commit create transaction: %w", err)
	}
	return tenantToDomain(tenantRow), tenantMembershipToDomain(membershipRow), nil
}

// UpdateTenantStatus changes a non-bootstrap tenant's lifecycle state.
func (r *TenantRegistry) UpdateTenantStatus(ctx context.Context, id domain.TenantID, status domain.TenantStatus) (domain.Tenant, error) {
	if id <= 0 {
		return domain.Tenant{}, fmt.Errorf("tenant registry: tenant id must be positive: %w", domain.ErrInvariantViolation)
	}
	if err := domain.ValidateTenantStatus(status); err != nil {
		return domain.Tenant{}, err
	}
	if id == domain.DefaultTenantID && status != domain.TenantStatusActive {
		return domain.Tenant{}, fmt.Errorf("tenant registry: default tenant cannot be disabled: %w", domain.ErrPreconditionFailed)
	}
	row, err := r.client.Tenant.UpdateOneID(int(id)).SetStatus(string(status)).Save(ctx)
	if ent.IsNotFound(err) {
		return domain.Tenant{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("tenant registry: update status: %w", err)
	}
	return tenantToDomain(row), nil
}

// ListTenantMemberships returns a deterministic bounded membership page.
func (r *TenantRegistry) ListTenantMemberships(ctx context.Context, tenantID domain.TenantID, limit int) ([]domain.TenantMembership, error) {
	if tenantID <= 0 || limit <= 0 {
		return nil, fmt.Errorf("tenant registry: tenant id and limit must be positive: %w", domain.ErrInvariantViolation)
	}
	exists, err := r.client.Tenant.Query().Where(tenant.IDEQ(int(tenantID))).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant registry: verify tenant before listing memberships: %w", err)
	}
	if !exists {
		return nil, domain.ErrNotFound
	}
	rows, err := r.client.TenantMembership.Query().
		Where(tenantmembership.TenantIDEQ(int(tenantID))).
		Order(ent.Asc(tenantmembership.FieldSubject), ent.Asc(tenantmembership.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant registry: list memberships: %w", err)
	}
	out := make([]domain.TenantMembership, len(rows))
	for i, row := range rows {
		out[i] = tenantMembershipToDomain(row)
	}
	return out, nil
}

// SetTenantMembership creates or updates one tenant membership atomically.
func (r *TenantRegistry) SetTenantMembership(ctx context.Context, input domain.TenantMembership) (domain.TenantMembership, error) {
	if input.TenantID <= 0 {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: tenant id must be positive: %w", domain.ErrInvariantViolation)
	}
	if err := validateTenantSubject(input.Subject); err != nil {
		return domain.TenantMembership{}, err
	}
	if err := validateTenantSubject(input.CreatedBy); err != nil {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: created_by: %w", err)
	}
	if err := domain.ValidateTenantMembershipRole(input.Role); err != nil {
		return domain.TenantMembership{}, err
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: begin membership transaction: %w", err)
	}
	rollback := func(cause error) (domain.TenantMembership, error) {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return domain.TenantMembership{}, errors.Join(
				cause,
				fmt.Errorf("tenant registry: rollback membership transaction: %w", rollbackErr),
			)
		}
		return domain.TenantMembership{}, cause
	}
	if _, err := tx.Tenant.Query().
		Where(tenant.IDEQ(int(input.TenantID))).
		ForUpdate().
		Only(ctx); ent.IsNotFound(err) {
		return rollback(domain.ErrNotFound)
	} else if err != nil {
		return rollback(fmt.Errorf("tenant registry: lock tenant: %w", err))
	}
	existing, findErr := tx.TenantMembership.Query().
		Where(
			tenantmembership.TenantIDEQ(int(input.TenantID)),
			tenantmembership.SubjectEQ(input.Subject),
		).
		Only(ctx)
	if findErr != nil && !ent.IsNotFound(findErr) {
		return rollback(fmt.Errorf("tenant registry: find membership for update: %w", findErr))
	}
	if findErr == nil && existing.Enabled && existing.Role == string(domain.TenantMembershipRoleOwner) &&
		(!input.Enabled || input.Role != domain.TenantMembershipRoleOwner) {
		owners, countErr := tx.TenantMembership.Query().
			Where(
				tenantmembership.TenantIDEQ(int(input.TenantID)),
				tenantmembership.RoleEQ(string(domain.TenantMembershipRoleOwner)),
				tenantmembership.EnabledEQ(true),
			).
			Count(ctx)
		if countErr != nil {
			return rollback(fmt.Errorf("tenant registry: count enabled owners: %w", countErr))
		}
		if owners <= 1 {
			return rollback(fmt.Errorf("tenant registry: cannot remove the last enabled owner: %w", domain.ErrPreconditionFailed))
		}
	}
	id, err := tx.TenantMembership.Create().
		SetTenantID(int(input.TenantID)).
		SetSubject(input.Subject).
		SetRole(string(input.Role)).
		SetEnabled(input.Enabled).
		SetCreatedBy(input.CreatedBy).
		OnConflictColumns(tenantmembership.FieldTenantID, tenantmembership.FieldSubject).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		return rollback(fmt.Errorf("tenant registry: set membership: %w", asAlreadyExists(err)))
	}
	row, err := tx.TenantMembership.Get(ctx, id)
	if err != nil {
		return rollback(fmt.Errorf("tenant registry: load set membership: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return domain.TenantMembership{}, fmt.Errorf("tenant registry: commit membership transaction: %w", err)
	}
	return tenantMembershipToDomain(row), nil
}

func validateTenantSubject(subject string) error {
	if strings.TrimSpace(subject) == "" || subject != strings.TrimSpace(subject) || len(subject) > 256 || strings.ContainsAny(subject, "\x00\r\n") {
		return fmt.Errorf("tenant registry: subject is invalid: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func tenantToDomain(row *ent.Tenant) domain.Tenant {
	return domain.Tenant{
		ID:        domain.TenantID(row.ID),
		Key:       row.Key,
		Name:      row.Name,
		Status:    domain.TenantStatus(row.Status),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func tenantsToDomain(rows []*ent.Tenant) []domain.Tenant {
	out := make([]domain.Tenant, len(rows))
	for i, row := range rows {
		out[i] = tenantToDomain(row)
	}
	return out
}

func tenantMembershipToDomain(row *ent.TenantMembership) domain.TenantMembership {
	return domain.TenantMembership{
		ID:        domain.TenantMembershipID(row.ID),
		TenantID:  domain.TenantID(row.TenantID),
		Subject:   row.Subject,
		Role:      domain.TenantMembershipRole(row.Role),
		Enabled:   row.Enabled,
		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
