package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/tenancy"
)

func TestTenantScopeIsolatesQueriesMutationsAndUniqueKeys(t *testing.T) {
	resetDB(t)

	ctx := context.Background()
	second, err := integration.client.Tenant.Create().
		SetKey("second").
		SetName("Second").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second tenant: %v", err)
	}
	defaultCtx := tenancy.EnsureDefault(ctx)
	secondCtx, err := tenancy.WithTenant(ctx, tenancy.Identity{
		ID:  domain.TenantID(second.ID),
		Key: second.Key,
	})
	if err != nil {
		t.Fatalf("scope second tenant: %v", err)
	}

	defaultPolicy, err := integration.client.GroupingPolicy.Create().
		SetName("shared-name").
		SetDimensionKeys([]string{"service"}).
		SetSeverityKey("severity").
		SetSourceFilter([]string{}).
		Save(defaultCtx)
	if err != nil {
		t.Fatalf("create default policy: %v", err)
	}
	secondPolicy, err := integration.client.GroupingPolicy.Create().
		SetName("shared-name").
		SetDimensionKeys([]string{"service"}).
		SetSeverityKey("severity").
		SetSourceFilter([]string{}).
		Save(secondCtx)
	if err != nil {
		t.Fatalf("create second policy with tenant-local name: %v", err)
	}
	if defaultPolicy.TenantID != int(tenancy.DefaultIdentity().ID) || secondPolicy.TenantID != second.ID {
		t.Fatalf("assigned tenant ids = %d, %d", defaultPolicy.TenantID, secondPolicy.TenantID)
	}

	for name, scopedCtx := range map[string]context.Context{"default": defaultCtx, "second": secondCtx} {
		count, err := integration.client.GroupingPolicy.Query().Count(scopedCtx)
		if err != nil {
			t.Fatalf("%s count: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("%s count = %d, want 1", name, count)
		}
	}

	if _, err := integration.client.GroupingPolicy.UpdateOneID(secondPolicy.ID).
		SetEnabled(true).
		Save(defaultCtx); !ent.IsNotFound(err) {
		t.Fatalf("cross-tenant update err = %v, want not found", err)
	}
	if _, err := integration.client.GroupingPolicy.Create().
		SetTenantID(second.ID).
		SetName("mismatch").
		SetDimensionKeys([]string{"service"}).
		SetSeverityKey("severity").
		SetSourceFilter([]string{}).
		Save(defaultCtx); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("explicit tenant mismatch err = %v, want precondition failed", err)
	}
	if _, err := integration.client.GroupingPolicy.Query().Count(ctx); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("unscoped query err = %v, want precondition failed", err)
	}

	profile, err := integration.client.AlertSourceProfile.Create().
		SetName("default-source").
		SetKind("prometheus").
		SetBaseURL("https://prometheus.example.com").
		SetLabels(map[string]string{}).
		Save(defaultCtx)
	if err != nil {
		t.Fatalf("create default source profile: %v", err)
	}
	if _, err := integration.client.DiagnosisToolTemplate.Create().
		SetName("cross-tenant-template").
		SetAlertSourceProfileID(profile.ID).
		SetTool("active_alerts").
		SetDefaultLimit(10).
		SetDefaultWindowNs(0).
		SetMaxWindowNs(0).
		SetDefaultStepNs(0).
		Save(secondCtx); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("cross-tenant foreign key err = %v, want precondition failed", err)
	}

	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	event, err := integration.client.AlertEvent.Create().
		SetSource("prometheus").
		SetSourceFingerprint("source-fingerprint").
		SetCanonicalFingerprint("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef").
		SetLabels(map[string]string{"service": "api"}).
		SetAnnotations(map[string]string{}).
		SetStartsAt(now).
		Save(defaultCtx)
	if err != nil {
		t.Fatalf("create default alert event: %v", err)
	}
	if _, err := integration.client.AlertGroup.Create().
		SetGroupKey("cross-tenant-group").
		SetDimensions(json.RawMessage(`{"service":"api"}`)).
		SetFirstSeenAt(now).
		SetLastSeenAt(now).
		AddEventIDs(event.ID).
		Save(secondCtx); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("cross-tenant edge err = %v, want precondition failed", err)
	}
}
