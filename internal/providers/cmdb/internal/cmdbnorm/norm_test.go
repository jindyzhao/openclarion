package cmdbnorm

import (
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestNormalizeResourceValidatesAndCopies(t *testing.T) {
	in := ports.CMDBResource{
		ID:     "service/checkout",
		Kind:   "service",
		Name:   "Checkout",
		Owners: []ports.CMDBOwner{{Subject: "team-checkout", Team: "Checkout", Role: "primary"}},
		Topology: []ports.CMDBTopologyLink{{
			Relation:   "depends_on",
			TargetID:   "database/postgres",
			TargetKind: "database",
			TargetName: "Checkout DB",
		}},
		Attributes: map[string]string{"tier": "frontend"},
	}

	got, err := NormalizeResource(in)
	if err != nil {
		t.Fatalf("NormalizeResource: %v", err)
	}
	in.Owners[0].Subject = "mutated"
	in.Topology[0].TargetID = "mutated"
	in.Attributes["tier"] = "mutated"

	if got.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owner polluted: %+v", got.Owners)
	}
	if got.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology polluted: %+v", got.Topology)
	}
	if got.Attributes["tier"] != "frontend" {
		t.Fatalf("attributes polluted: %+v", got.Attributes)
	}
}

func TestNormalizeResourceRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		resource ports.CMDBResource
		want     string
	}{
		{
			name:     "missing id",
			resource: ports.CMDBResource{Kind: "service", Name: "Checkout", Owners: []ports.CMDBOwner{{Team: "Checkout"}}},
			want:     "resource id must be non-empty",
		},
		{
			name:     "owner without subject or team",
			resource: ports.CMDBResource{ID: "service/checkout", Kind: "service", Name: "Checkout", Owners: []ports.CMDBOwner{{Role: "primary"}}},
			want:     "subject or team must be non-empty",
		},
		{
			name: "bad topology",
			resource: ports.CMDBResource{
				ID:       "service/checkout",
				Kind:     "service",
				Name:     "Checkout",
				Topology: []ports.CMDBTopologyLink{{Relation: "depends_on", TargetKind: "database"}},
			},
			want: "topology target id must be non-empty",
		},
		{
			name:     "empty enrichment",
			resource: ports.CMDBResource{ID: "service/checkout", Kind: "service", Name: "Checkout"},
			want:     "resource must include owners, topology, or attributes",
		},
		{
			name: "attribute whitespace",
			resource: ports.CMDBResource{
				ID:         "service/checkout",
				Kind:       "service",
				Name:       "Checkout",
				Attributes: map[string]string{" tier": "frontend"},
			},
			want: "attribute key must not contain leading or trailing whitespace",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeResource(tt.resource)
			if err == nil {
				t.Fatal("NormalizeResource err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNormalizeLabelMap(t *testing.T) {
	got, err := NormalizeLabelMap(map[string]string{"service": "checkout"}, true)
	if err != nil {
		t.Fatalf("NormalizeLabelMap: %v", err)
	}
	if got["service"] != "checkout" {
		t.Fatalf("labels = %+v", got)
	}

	if optional, err := NormalizeLabelMap(nil, false); err != nil || optional != nil {
		t.Fatalf("optional nil labels = %+v, err=%v; want nil, nil", optional, err)
	}
	if _, err := NormalizeLabelMap(nil, true); err == nil || !strings.Contains(err.Error(), "match_labels must be non-empty") {
		t.Fatalf("required nil labels err = %v", err)
	}
	if _, err := NormalizeLabelMap(map[string]string{" service": "checkout"}, true); err == nil || !strings.Contains(err.Error(), "label key") {
		t.Fatalf("invalid label err = %v", err)
	}
}

func TestCloneResourceDeepCopiesMutableFields(t *testing.T) {
	in := ports.CMDBResource{
		ID:         "service/checkout",
		Kind:       "service",
		Name:       "Checkout",
		Owners:     []ports.CMDBOwner{{Subject: "team-checkout"}},
		Topology:   []ports.CMDBTopologyLink{{Relation: "depends_on", TargetID: "database/postgres", TargetKind: "database"}},
		Attributes: map[string]string{"tier": "frontend"},
	}

	got := CloneResource(in)
	got.Owners[0].Subject = "mutated"
	got.Topology[0].TargetID = "mutated"
	got.Attributes["tier"] = "mutated"

	if in.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owner source polluted: %+v", in.Owners)
	}
	if in.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology source polluted: %+v", in.Topology)
	}
	if in.Attributes["tier"] != "frontend" {
		t.Fatalf("attribute source polluted: %+v", in.Attributes)
	}
}
