package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestReconcileAllTenantReportWorkflowSchedulesScopesAndPausesDisabledTenants(t *testing.T) {
	t.Parallel()

	activeSchedule := testReportWorkflowSchedule(t)
	disabledSchedule := testReportWorkflowSchedule(t)
	disabledSchedule.ID = activeSchedule.ID + 1
	registry := tenantScheduleRegistry{tenants: []domain.Tenant{
		{ID: 1, Key: domain.DefaultTenantKey, Name: domain.DefaultTenantName, Status: domain.TenantStatusActive},
		{ID: 2, Key: "platform", Name: "Platform", Status: domain.TenantStatusDisabled},
	}}
	factory := tenantScheduleFactory{byTenant: map[domain.TenantID][]domain.ReportWorkflowSchedule{
		1: {activeSchedule},
		2: {disabledSchedule},
	}}
	reconciler := &recordingTenantScheduleReconciler{}

	result, err := reconcileAllTenantReportWorkflowSchedules(context.Background(), registry, factory, reconciler)
	if err != nil {
		t.Fatalf("reconcileAllTenantReportWorkflowSchedules: %v", err)
	}
	if result.Total != 2 || result.Updated != 2 || len(reconciler.calls) != 2 {
		t.Fatalf("result=%+v calls=%+v", result, reconciler.calls)
	}
	if reconciler.calls[0].identity.ID != 1 || !reconciler.calls[0].schedules[0].Enabled {
		t.Fatalf("active tenant call = %+v", reconciler.calls[0])
	}
	if reconciler.calls[1].identity.ID != 2 || reconciler.calls[1].schedules[0].Enabled {
		t.Fatalf("disabled tenant call = %+v", reconciler.calls[1])
	}
}

func TestReconcileAllTenantReportWorkflowSchedulesRejectsUnboundedTenantSet(t *testing.T) {
	t.Parallel()

	tenants := make([]domain.Tenant, 501)
	for i := range tenants {
		tenants[i] = domain.Tenant{
			ID:     domain.TenantID(i + 1),
			Key:    fmt.Sprintf("tenant-%d", i+1),
			Name:   fmt.Sprintf("Tenant %d", i+1),
			Status: domain.TenantStatusActive,
		}
	}
	_, err := reconcileAllTenantReportWorkflowSchedules(
		context.Background(),
		tenantScheduleRegistry{tenants: tenants},
		tenantScheduleFactory{},
		&recordingTenantScheduleReconciler{},
	)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("reconcile tenant cap err = %v, want invariant violation", err)
	}
}

type tenantScheduleRegistry struct {
	ports.TenantRegistry
	tenants []domain.Tenant
}

func (r tenantScheduleRegistry) ListTenants(context.Context, int) ([]domain.Tenant, error) {
	return append([]domain.Tenant(nil), r.tenants...), nil
}

type tenantScheduleFactory struct {
	byTenant map[domain.TenantID][]domain.ReportWorkflowSchedule
}

func (tenantScheduleFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, fmt.Errorf("unexpected Begin call")
}

func (f tenantScheduleFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	identity, err := tenancy.Require(ctx)
	if err != nil {
		return err
	}
	config := tenantScheduleConfig{
		schedules: append([]domain.ReportWorkflowSchedule(nil), f.byTenant[identity.ID]...),
	}
	return fn(ctx, tenantScheduleUnitOfWork{config: config})
}

type tenantScheduleUnitOfWork struct {
	ports.UnitOfWork
	config ports.ConfigurationRepository
}

func (u tenantScheduleUnitOfWork) Config() ports.ConfigurationRepository {
	return u.config
}

type tenantScheduleConfig struct {
	ports.ConfigurationRepository
	schedules []domain.ReportWorkflowSchedule
}

func (c tenantScheduleConfig) ListReportWorkflowSchedules(_ context.Context, limit int) ([]domain.ReportWorkflowSchedule, error) {
	if limit <= 0 {
		return nil, domain.ErrInvariantViolation
	}
	if len(c.schedules) > limit {
		return append([]domain.ReportWorkflowSchedule(nil), c.schedules[:limit]...), nil
	}
	return append([]domain.ReportWorkflowSchedule(nil), c.schedules...), nil
}

type tenantScheduleReconcileCall struct {
	identity  tenancy.Identity
	schedules []domain.ReportWorkflowSchedule
}

type recordingTenantScheduleReconciler struct {
	calls []tenantScheduleReconcileCall
}

func (r *recordingTenantScheduleReconciler) Reconcile(
	ctx context.Context,
	schedules []domain.ReportWorkflowSchedule,
) (temporalpkg.ReportWorkflowScheduleReconcileResult, error) {
	identity, err := tenancy.Require(ctx)
	if err != nil {
		return temporalpkg.ReportWorkflowScheduleReconcileResult{}, err
	}
	r.calls = append(r.calls, tenantScheduleReconcileCall{
		identity:  identity,
		schedules: append([]domain.ReportWorkflowSchedule(nil), schedules...),
	})
	return temporalpkg.ReportWorkflowScheduleReconcileResult{
		Total:   len(schedules),
		Updated: len(schedules),
	}, nil
}
