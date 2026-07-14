package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestTenantRegistryLifecycle(t *testing.T) {
	resetDB(t)

	registry, err := NewTenantRegistry(integration.client)
	if err != nil {
		t.Fatalf("NewTenantRegistry: %v", err)
	}
	ctx := context.Background()
	defaultTenant, err := registry.FindTenantByKey(ctx, domain.DefaultTenantKey)
	if err != nil || defaultTenant.ID != 1 {
		t.Fatalf("find default tenant = %+v, %v", defaultTenant, err)
	}

	created, owner, err := registry.CreateTenantWithOwner(ctx, domain.Tenant{
		Key:    "platform-team",
		Name:   "Platform Team",
		Status: domain.TenantStatusActive,
	}, "owner-1")
	if err != nil {
		t.Fatalf("CreateTenantWithOwner: %v", err)
	}
	if created.ID <= 1 || owner.TenantID != created.ID || owner.Subject != "owner-1" || !owner.Enabled {
		t.Fatalf("created tenant/member = %+v / %+v", created, owner)
	}
	if _, _, err := registry.CreateTenantWithOwner(ctx, domain.Tenant{
		Key:    created.Key,
		Name:   "Duplicate",
		Status: domain.TenantStatusActive,
	}, "owner-1"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate create err = %v, want already exists", err)
	}

	visible, err := registry.ListTenantsForSubject(ctx, "owner-1", 10)
	if err != nil || len(visible) != 1 || visible[0].ID != created.ID {
		t.Fatalf("visible tenants = %+v, %v", visible, err)
	}
	member, err := registry.SetTenantMembership(ctx, domain.TenantMembership{
		TenantID:  created.ID,
		Subject:   "operator-2",
		Role:      domain.TenantMembershipRoleMember,
		Enabled:   true,
		CreatedBy: "owner-1",
	})
	if err != nil || !member.Enabled {
		t.Fatalf("upsert member = %+v, %v", member, err)
	}
	member.Enabled = false
	member.CreatedBy = "owner-1"
	member, err = registry.SetTenantMembership(ctx, member)
	if err != nil || member.Enabled {
		t.Fatalf("disable member = %+v, %v", member, err)
	}

	const concurrentWriters = 8
	start := make(chan struct{})
	errs := make(chan error, concurrentWriters)
	var writers sync.WaitGroup
	for range concurrentWriters {
		writers.Add(1)
		go func() {
			defer writers.Done()
			<-start
			_, upsertErr := registry.SetTenantMembership(ctx, domain.TenantMembership{
				TenantID:  created.ID,
				Subject:   "operator-concurrent",
				Role:      domain.TenantMembershipRoleMember,
				Enabled:   true,
				CreatedBy: "owner-1",
			})
			errs <- upsertErr
		}()
	}
	close(start)
	writers.Wait()
	close(errs)
	for upsertErr := range errs {
		if upsertErr != nil {
			t.Fatalf("concurrent upsert: %v", upsertErr)
		}
	}
	memberships, err := registry.ListTenantMemberships(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListTenantMemberships: %v", err)
	}
	concurrentCount := 0
	for _, membership := range memberships {
		if membership.Subject == "operator-concurrent" {
			concurrentCount++
		}
	}
	if concurrentCount != 1 {
		t.Fatalf("concurrent membership rows = %d, want 1", concurrentCount)
	}

	secondOwner, err := registry.SetTenantMembership(ctx, domain.TenantMembership{
		TenantID:  created.ID,
		Subject:   "owner-2",
		Role:      domain.TenantMembershipRoleOwner,
		Enabled:   true,
		CreatedBy: "owner-1",
	})
	if err != nil {
		t.Fatalf("add second owner: %v", err)
	}
	owners := []domain.TenantMembership{owner, secondOwner}
	start = make(chan struct{})
	errs = make(chan error, len(owners))
	writers = sync.WaitGroup{}
	for _, currentOwner := range owners {
		writers.Add(1)
		go func(membership domain.TenantMembership) {
			defer writers.Done()
			<-start
			membership.Enabled = false
			_, setErr := registry.SetTenantMembership(ctx, membership)
			errs <- setErr
		}(currentOwner)
	}
	close(start)
	writers.Wait()
	close(errs)
	succeeded := 0
	preconditionFailed := 0
	for setErr := range errs {
		switch {
		case setErr == nil:
			succeeded++
		case errors.Is(setErr, domain.ErrPreconditionFailed):
			preconditionFailed++
		default:
			t.Fatalf("concurrent owner update: %v", setErr)
		}
	}
	if succeeded != 1 || preconditionFailed != 1 {
		t.Fatalf("owner updates succeeded=%d precondition_failed=%d, want 1/1", succeeded, preconditionFailed)
	}
	memberships, err = registry.ListTenantMemberships(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("ListTenantMemberships after owner updates: %v", err)
	}
	enabledOwners := 0
	for _, membership := range memberships {
		if membership.Enabled && membership.Role == domain.TenantMembershipRoleOwner {
			enabledOwners++
		}
	}
	if enabledOwners != 1 {
		t.Fatalf("enabled owners = %d, want 1", enabledOwners)
	}

	if _, err := registry.UpdateTenantStatus(ctx, 1, domain.TenantStatusDisabled); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("disable default err = %v, want precondition failed", err)
	}
	disabled, err := registry.UpdateTenantStatus(ctx, created.ID, domain.TenantStatusDisabled)
	if err != nil || disabled.Status != domain.TenantStatusDisabled {
		t.Fatalf("disable tenant = %+v, %v", disabled, err)
	}
	if _, err := registry.ListTenantMemberships(ctx, 999999, 20); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("list memberships for missing tenant err = %v, want not found", err)
	}
}

func TestTenantSchemaRejectsUnnormalizedRegistryValues(t *testing.T) {
	resetDB(t)

	ctx := context.Background()
	if _, err := integration.client.Tenant.Create().
		SetKey(" spaced ").
		SetName("Spaced").
		Save(ctx); err == nil {
		t.Fatal("unnormalized tenant key was accepted")
	}
	if _, err := integration.client.Tenant.Create().
		SetKey("spaced-name").
		SetName(" Spaced ").
		Save(ctx); err == nil {
		t.Fatal("unnormalized tenant name was accepted")
	}
	if _, err := integration.client.Tenant.Create().
		SetKey("unicode-name").
		SetName(strings.Repeat("中", domain.MaxTenantNameLength)).
		Save(ctx); err != nil {
		t.Fatalf("valid Unicode tenant name was rejected: %v", err)
	}
	if _, err := integration.client.Tenant.Create().
		SetKey("unicode-name-too-long").
		SetName(strings.Repeat("中", domain.MaxTenantNameLength+1)).
		Save(ctx); err == nil {
		t.Fatal("overlong Unicode tenant name was accepted")
	}
}
