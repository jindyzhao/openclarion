package repository

import (
	"context"
	"fmt"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/tenant"
)

// EnsureDefaultTenant idempotently creates and verifies the bootstrap tenant.
// Production migrations call the same logical operation in SQL; this helper is
// also used by Schema.Create-based integration fixtures.
func EnsureDefaultTenant(ctx context.Context, client *ent.Client) error {
	if client == nil {
		return fmt.Errorf("ensure default tenant: ent client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	const defaultTenantID = int(domain.DefaultTenantID)
	_, err := client.Tenant.Create().
		SetKey(domain.DefaultTenantKey).
		SetName(domain.DefaultTenantName).
		SetStatus(string(domain.TenantStatusActive)).
		Save(ctx)
	if err != nil && !ent.IsConstraintError(err) {
		return fmt.Errorf("ensure default tenant: create: %w", err)
	}
	row, err := client.Tenant.Query().
		Where(tenant.KeyEQ(domain.DefaultTenantKey)).
		Only(ctx)
	if err != nil {
		return fmt.Errorf("ensure default tenant: verify: %w", err)
	}
	if row.ID != defaultTenantID || row.Name != domain.DefaultTenantName || row.Status != string(domain.TenantStatusActive) {
		return fmt.Errorf("ensure default tenant: key %q is bound to incompatible registry data: %w", domain.DefaultTenantKey, domain.ErrInvariantViolation)
	}
	return nil
}
