package tenantops

import (
	"context"
	"errors"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestResolveAccess(t *testing.T) {
	t.Parallel()

	registry := &fakeRegistry{
		tenants: []domain.Tenant{
			{ID: 1, Key: domain.DefaultTenantKey, Name: domain.DefaultTenantName, Status: domain.TenantStatusActive},
			{ID: 2, Key: "platform", Name: "Platform", Status: domain.TenantStatusActive},
			{ID: 3, Key: "disabled", Name: "Disabled", Status: domain.TenantStatusDisabled},
		},
		memberships: []domain.TenantMembership{{ID: 1, TenantID: 2, Subject: "operator-1", Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: "admin"}},
	}
	service, err := NewService(registry)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	identity, err := service.ResolveAccess(context.Background(), "operator-1", "", false)
	if err != nil || identity.ID != 1 {
		t.Fatalf("resolve default = %+v, %v", identity, err)
	}
	identity, err = service.ResolveAccess(context.Background(), "operator-1", "platform", false)
	if err != nil || identity.ID != 2 || identity.Key != "platform" {
		t.Fatalf("resolve membership = %+v, %v", identity, err)
	}
	if _, err := service.ResolveAccess(context.Background(), "other", "platform", false); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("non-member err = %v, want access denied", err)
	}
	identity, err = service.ResolveAccess(context.Background(), "global-admin", "platform", true)
	if err != nil || identity.ID != 2 {
		t.Fatalf("resolve global admin = %+v, %v", identity, err)
	}
	if _, err := service.ResolveAccess(context.Background(), "global-admin", "disabled", true); !errors.Is(err, ErrTenantDisabled) {
		t.Fatalf("disabled tenant err = %v, want disabled", err)
	}
}

func TestListAccessibleIncludesDefaultAndVisibleDisabledTenants(t *testing.T) {
	t.Parallel()

	registry := &fakeRegistry{
		tenants: []domain.Tenant{
			{ID: 1, Key: domain.DefaultTenantKey, Name: domain.DefaultTenantName, Status: domain.TenantStatusActive},
			{ID: 2, Key: "platform", Name: "Platform", Status: domain.TenantStatusActive},
			{ID: 3, Key: "disabled", Name: "Disabled", Status: domain.TenantStatusDisabled},
		},
		memberships: []domain.TenantMembership{
			{ID: 1, TenantID: 2, Subject: "operator-1", Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: "admin"},
			{ID: 2, TenantID: 3, Subject: "operator-1", Role: domain.TenantMembershipRoleMember, Enabled: true, CreatedBy: "admin"},
		},
	}
	service, _ := NewService(registry)
	got, err := service.ListAccessible(context.Background(), "operator-1", false)
	if err != nil {
		t.Fatalf("ListAccessible: %v", err)
	}
	if len(got) != 3 || got[0].ID != 1 || got[1].ID != 2 || got[2].ID != 3 {
		t.Fatalf("accessible tenants = %+v", got)
	}
	if registry.lastSubjectLimit != maxTenantList-1 {
		t.Fatalf("subject tenant limit = %d, want %d", registry.lastSubjectLimit, maxTenantList-1)
	}
	if _, err := service.ListAccessible(context.Background(), "global-admin", true); err != nil {
		t.Fatalf("ListAccessible global admin: %v", err)
	}
	if registry.lastListLimit != maxTenantList-1 {
		t.Fatalf("global tenant limit = %d, want %d", registry.lastListLimit, maxTenantList-1)
	}
}

func TestSetMembershipPreservesLastEnabledOwner(t *testing.T) {
	t.Parallel()

	registry := &fakeRegistry{
		tenants: []domain.Tenant{{
			ID: 2, Key: "platform", Name: "Platform", Status: domain.TenantStatusActive,
		}},
		memberships: []domain.TenantMembership{{
			ID: 1, TenantID: 2, Subject: "owner-1", Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: "admin",
		}},
	}
	service, _ := NewService(registry)

	if _, err := service.SetMembership(
		context.Background(),
		2,
		"owner-1",
		domain.TenantMembershipRoleOwner,
		false,
		"owner-1",
	); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("disable last owner err = %v, want precondition failed", err)
	}
	if _, err := service.SetMembership(
		context.Background(),
		2,
		"owner-2",
		domain.TenantMembershipRoleOwner,
		true,
		"owner-1",
	); err != nil {
		t.Fatalf("add second owner: %v", err)
	}
	if _, err := service.SetMembership(
		context.Background(),
		2,
		"owner-1",
		domain.TenantMembershipRoleMember,
		true,
		"owner-2",
	); err != nil {
		t.Fatalf("demote owner with successor: %v", err)
	}
}

type fakeRegistry struct {
	tenants          []domain.Tenant
	memberships      []domain.TenantMembership
	lastListLimit    int
	lastSubjectLimit int
}

func (f *fakeRegistry) FindTenantByKey(_ context.Context, key string) (domain.Tenant, error) {
	for _, tenant := range f.tenants {
		if tenant.Key == key {
			return tenant, nil
		}
	}
	return domain.Tenant{}, domain.ErrNotFound
}

func (f *fakeRegistry) ListTenants(_ context.Context, limit int) ([]domain.Tenant, error) {
	f.lastListLimit = limit
	return append([]domain.Tenant(nil), f.tenants...), nil
}

func (f *fakeRegistry) ListTenantsForSubject(_ context.Context, subject string, limit int) ([]domain.Tenant, error) {
	f.lastSubjectLimit = limit
	allowed := make(map[domain.TenantID]bool)
	for _, membership := range f.memberships {
		if membership.Subject == subject && membership.Enabled {
			allowed[membership.TenantID] = true
		}
	}
	var out []domain.Tenant
	for _, tenant := range f.tenants {
		if allowed[tenant.ID] {
			out = append(out, tenant)
		}
	}
	return out, nil
}

func (f *fakeRegistry) FindTenantMembership(_ context.Context, tenantID domain.TenantID, subject string) (domain.TenantMembership, error) {
	for _, membership := range f.memberships {
		if membership.TenantID == tenantID && membership.Subject == subject {
			return membership, nil
		}
	}
	return domain.TenantMembership{}, domain.ErrNotFound
}

func (f *fakeRegistry) CreateTenantWithOwner(_ context.Context, tenant domain.Tenant, owner string) (domain.Tenant, domain.TenantMembership, error) {
	tenant.ID = domain.TenantID(len(f.tenants) + 1)
	f.tenants = append(f.tenants, tenant)
	membership := domain.TenantMembership{ID: domain.TenantMembershipID(len(f.memberships) + 1), TenantID: tenant.ID, Subject: owner, Role: domain.TenantMembershipRoleOwner, Enabled: true, CreatedBy: owner}
	f.memberships = append(f.memberships, membership)
	return tenant, membership, nil
}

func (f *fakeRegistry) UpdateTenantStatus(_ context.Context, id domain.TenantID, status domain.TenantStatus) (domain.Tenant, error) {
	for i := range f.tenants {
		if f.tenants[i].ID == id {
			f.tenants[i].Status = status
			return f.tenants[i], nil
		}
	}
	return domain.Tenant{}, domain.ErrNotFound
}

func (f *fakeRegistry) ListTenantMemberships(_ context.Context, tenantID domain.TenantID, _ int) ([]domain.TenantMembership, error) {
	var out []domain.TenantMembership
	for _, membership := range f.memberships {
		if membership.TenantID == tenantID {
			out = append(out, membership)
		}
	}
	return out, nil
}

func (f *fakeRegistry) SetTenantMembership(_ context.Context, membership domain.TenantMembership) (domain.TenantMembership, error) {
	for i := range f.memberships {
		if f.memberships[i].TenantID == membership.TenantID && f.memberships[i].Subject == membership.Subject {
			if f.memberships[i].Enabled && f.memberships[i].Role == domain.TenantMembershipRoleOwner &&
				(!membership.Enabled || membership.Role != domain.TenantMembershipRoleOwner) {
				owners := 0
				for _, current := range f.memberships {
					if current.TenantID == membership.TenantID && current.Enabled && current.Role == domain.TenantMembershipRoleOwner {
						owners++
					}
				}
				if owners <= 1 {
					return domain.TenantMembership{}, domain.ErrPreconditionFailed
				}
			}
			f.memberships[i].Role = membership.Role
			f.memberships[i].Enabled = membership.Enabled
			return f.memberships[i], nil
		}
	}
	membership.ID = domain.TenantMembershipID(len(f.memberships) + 1)
	f.memberships = append(f.memberships, membership)
	return membership, nil
}
