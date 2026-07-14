package temporal

import (
	"context"
	"fmt"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/workflow"
)

const tenantContextHeader = "openclarion-tenant-v1"

type workflowTenantContextKey struct{}

type tenantContextPayload struct {
	ID  int64  `json:"id"`
	Key string `json:"key"`
}

// TenantContextPropagators returns the propagator set that must be installed
// on every OpenClarion Temporal client. Workers inherit the set from clients.
func TenantContextPropagators() []workflow.ContextPropagator {
	return []workflow.ContextPropagator{tenantContextPropagator{}}
}

type tenantContextPropagator struct{}

func (tenantContextPropagator) Inject(ctx context.Context, writer workflow.HeaderWriter) error {
	identity, ok := tenancy.FromContext(ctx)
	if !ok {
		identity = tenancy.DefaultIdentity()
	}
	return writeTenantContextHeader(identity, writer)
}

func (tenantContextPropagator) Extract(ctx context.Context, reader workflow.HeaderReader) (context.Context, error) {
	identity, err := readTenantContextHeader(reader)
	if err != nil {
		return ctx, err
	}
	return tenancy.WithTenant(ctx, identity)
}

func (tenantContextPropagator) InjectFromWorkflow(ctx workflow.Context, writer workflow.HeaderWriter) error {
	identity, ok := ctx.Value(workflowTenantContextKey{}).(tenancy.Identity)
	if !ok {
		identity = tenancy.DefaultIdentity()
	}
	return writeTenantContextHeader(identity, writer)
}

func (tenantContextPropagator) ExtractToWorkflow(ctx workflow.Context, reader workflow.HeaderReader) (workflow.Context, error) {
	identity, err := readTenantContextHeader(reader)
	if err != nil {
		return ctx, err
	}
	return workflow.WithValue(ctx, workflowTenantContextKey{}, identity), nil
}

func writeTenantContextHeader(identity tenancy.Identity, writer workflow.HeaderWriter) error {
	validated, err := tenancy.NewIdentity(identity.ID, identity.Key)
	if err != nil {
		return fmt.Errorf("temporal tenant propagation: invalid identity: %w", err)
	}
	payload, err := converter.GetDefaultDataConverter().ToPayload(tenantContextPayload{
		ID:  int64(validated.ID),
		Key: validated.Key,
	})
	if err != nil {
		return fmt.Errorf("temporal tenant propagation: encode header: %w", err)
	}
	writer.Set(tenantContextHeader, payload)
	return nil
}

func readTenantContextHeader(reader workflow.HeaderReader) (tenancy.Identity, error) {
	payload, ok := reader.Get(tenantContextHeader)
	if !ok {
		// Existing workflow histories predate tenant propagation and belong to
		// the bootstrap tenant.
		return tenancy.DefaultIdentity(), nil
	}
	var decoded tenantContextPayload
	if err := converter.GetDefaultDataConverter().FromPayload(payload, &decoded); err != nil {
		return tenancy.Identity{}, fmt.Errorf("temporal tenant propagation: decode header: %w", err)
	}
	identity, err := tenancy.NewIdentity(domain.TenantID(decoded.ID), decoded.Key)
	if err != nil {
		return tenancy.Identity{}, fmt.Errorf("temporal tenant propagation: validate header: %w", err)
	}
	return identity, nil
}
