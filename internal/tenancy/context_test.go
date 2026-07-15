package tenancy

import (
	"context"
	"errors"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestTenantContext(t *testing.T) {
	t.Parallel()

	if _, err := Require(context.Background()); !errors.Is(err, domain.ErrPreconditionFailed) {
		t.Fatalf("Require missing err = %v, want precondition failed", err)
	}
	identity := Identity{ID: 7, Key: "team-seven"}
	ctx, err := WithTenant(context.Background(), identity)
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	got, ok := FromContext(ctx)
	if !ok || got != identity {
		t.Fatalf("FromContext = %+v, %t, want %+v", got, ok, identity)
	}
	if ensured := EnsureDefault(ctx); ensured != ctx {
		t.Fatal("EnsureDefault replaced an explicitly scoped context")
	}
}

func TestEnsureDefault(t *testing.T) {
	t.Parallel()

	if EnsureDefault(nil) != nil {
		t.Fatal("EnsureDefault(nil) must remain nil")
	}
	got, ok := FromContext(EnsureDefault(context.Background()))
	if !ok || got != DefaultIdentity() {
		t.Fatalf("default identity = %+v, %t", got, ok)
	}
}
