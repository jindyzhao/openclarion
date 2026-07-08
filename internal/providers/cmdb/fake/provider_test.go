package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestLookupResourceReturnsDeepCopy(t *testing.T) {
	provider := New(ports.CMDBLookupResult{
		Found: true,
		Resource: ports.CMDBResource{
			ID:   "service/checkout",
			Kind: "service",
			Name: "Checkout",
			Owners: []ports.CMDBOwner{{
				Subject: "team-checkout",
				Team:    "Checkout",
				Role:    "primary",
			}},
			Topology: []ports.CMDBTopologyLink{{
				Relation:   "depends_on",
				TargetID:   "database/postgres",
				TargetKind: "database",
				TargetName: "Postgres",
			}},
			Attributes: map[string]string{"tier": "frontend"},
		},
	})

	req := ports.CMDBLookupRequest{Labels: map[string]string{"service": "checkout"}}
	got, err := provider.LookupResource(context.Background(), req)
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	req.Labels["service"] = "mutated"
	got.Resource.Owners[0].Subject = "mutated"
	got.Resource.Topology[0].TargetID = "mutated"
	got.Resource.Attributes["tier"] = "mutated"

	again, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: map[string]string{"service": "checkout"}})
	if err != nil {
		t.Fatalf("LookupResource again: %v", err)
	}
	if again.Resource.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owner subject polluted: %q", again.Resource.Owners[0].Subject)
	}
	if again.Resource.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology polluted: %q", again.Resource.Topology[0].TargetID)
	}
	if again.Resource.Attributes["tier"] != "frontend" {
		t.Fatalf("attributes polluted: %q", again.Resource.Attributes["tier"])
	}
	requests := provider.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests len = %d, want 2", len(requests))
	}
	if requests[0].Labels["service"] != "checkout" {
		t.Fatalf("recorded request polluted: %q", requests[0].Labels["service"])
	}
}

func TestLookupResourceReturnsConfiguredError(t *testing.T) {
	want := errors.New("provider unavailable")
	provider := NewError(want)

	_, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestLookupResourceHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := New(ports.CMDBLookupResult{Found: true})

	_, err := provider.LookupResource(ctx, ports.CMDBLookupRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
