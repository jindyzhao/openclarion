package static

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderLookupResourceFromYAML(t *testing.T) {
	provider, err := NewProviderFromYAML([]byte(`
records:
  - match_labels:
      service: checkout
      cluster: prod
    resource:
      id: service/checkout
      kind: service
      name: Checkout API
      owners:
        - subject: team-checkout
          team: Checkout
          role: primary
      topology:
        - relation: depends_on
          target_id: database/postgres
          target_kind: database
          target_name: Checkout DB
      attributes:
        tier: frontend
`))
	if err != nil {
		t.Fatalf("NewProviderFromYAML: %v", err)
	}

	req := ports.CMDBLookupRequest{Labels: map[string]string{
		"service": "checkout",
		"cluster": "prod",
		"extra":   "ignored",
	}}
	got, err := provider.LookupResource(context.Background(), req)
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	if got.Resource.ID != "service/checkout" || got.Resource.Kind != "service" || got.Resource.Name != "Checkout API" {
		t.Fatalf("resource = %+v", got.Resource)
	}
	if len(got.Resource.Owners) != 1 || got.Resource.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owners = %+v", got.Resource.Owners)
	}
	if len(got.Resource.Topology) != 1 || got.Resource.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology = %+v", got.Resource.Topology)
	}
	if got.Resource.Attributes["tier"] != "frontend" {
		t.Fatalf("attributes = %+v", got.Resource.Attributes)
	}
}

func TestLookupResourceReturnsDeepCopy(t *testing.T) {
	provider, err := NewProvider([]Record{{
		MatchLabels: map[string]string{"service": "checkout"},
		Resource: ports.CMDBResource{
			ID:         "service/checkout",
			Kind:       "service",
			Name:       "Checkout",
			Owners:     []ports.CMDBOwner{{Subject: "team-checkout"}},
			Topology:   []ports.CMDBTopologyLink{{Relation: "depends_on", TargetID: "database/postgres", TargetKind: "database"}},
			Attributes: map[string]string{"tier": "frontend"},
		},
	}})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	first, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: map[string]string{"service": "checkout"}})
	if err != nil {
		t.Fatalf("first LookupResource: %v", err)
	}
	first.Resource.Owners[0].Subject = "mutated"
	first.Resource.Topology[0].TargetID = "mutated"
	first.Resource.Attributes["tier"] = "mutated"

	second, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: map[string]string{"service": "checkout"}})
	if err != nil {
		t.Fatalf("second LookupResource: %v", err)
	}
	if second.Resource.Owners[0].Subject != "team-checkout" {
		t.Fatalf("owner polluted: %q", second.Resource.Owners[0].Subject)
	}
	if second.Resource.Topology[0].TargetID != "database/postgres" {
		t.Fatalf("topology polluted: %q", second.Resource.Topology[0].TargetID)
	}
	if second.Resource.Attributes["tier"] != "frontend" {
		t.Fatalf("attributes polluted: %q", second.Resource.Attributes["tier"])
	}
}

func TestLookupResourceNoMatch(t *testing.T) {
	provider, err := NewProvider([]Record{{
		MatchLabels: map[string]string{"service": "checkout"},
		Resource: ports.CMDBResource{
			ID:     "service/checkout",
			Kind:   "service",
			Name:   "Checkout",
			Owners: []ports.CMDBOwner{{Team: "Checkout"}},
		},
	}})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: map[string]string{"service": "payments"}})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if got.Found {
		t.Fatalf("Found = true, want false")
	}
}

func TestLookupResourceIgnoresUnrelatedRequestLabels(t *testing.T) {
	provider, err := NewProvider([]Record{{
		MatchLabels: map[string]string{"service": "checkout"},
		Resource: ports.CMDBResource{
			ID:     "service/checkout",
			Kind:   "service",
			Name:   "Checkout",
			Owners: []ports.CMDBOwner{{Team: "Checkout"}},
		},
	}})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	labels := map[string]string{"service": "checkout", "empty_extra": ""}
	for i := 0; i < maxMatchLabels+1; i++ {
		labels[fmt.Sprintf("extra_%02d", i)] = "value"
	}

	got, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{Labels: labels})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if !got.Found {
		t.Fatalf("Found = false, want true")
	}
}

func TestLookupResourceHonorsContextCancellation(t *testing.T) {
	provider, err := NewProvider(nil)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = provider.LookupResource(ctx, ports.CMDBLookupRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestNewProviderRejectsInvalidRecords(t *testing.T) {
	tests := []struct {
		name    string
		records []Record
		want    string
	}{
		{
			name: "empty selector",
			records: []Record{{
				Resource: ports.CMDBResource{ID: "service/a", Kind: "service", Name: "A", Owners: []ports.CMDBOwner{{Team: "A"}}},
			}},
			want: "match_labels must be non-empty",
		},
		{
			name: "overlapping selectors",
			records: []Record{
				{
					MatchLabels: map[string]string{"service": "checkout"},
					Resource:    ports.CMDBResource{ID: "service/a", Kind: "service", Name: "A", Owners: []ports.CMDBOwner{{Team: "A"}}},
				},
				{
					MatchLabels: map[string]string{"cluster": "prod"},
					Resource:    ports.CMDBResource{ID: "service/b", Kind: "service", Name: "B", Owners: []ports.CMDBOwner{{Team: "B"}}},
				},
			},
			want: "overlapping match_labels",
		},
		{
			name: "owner without subject or team",
			records: []Record{{
				MatchLabels: map[string]string{"service": "checkout"},
				Resource: ports.CMDBResource{
					ID:     "service/checkout",
					Kind:   "service",
					Name:   "Checkout",
					Owners: []ports.CMDBOwner{{Role: "primary"}},
				},
			}},
			want: "subject or team must be non-empty",
		},
		{
			name: "empty enrichment",
			records: []Record{{
				MatchLabels: map[string]string{"service": "checkout"},
				Resource:    ports.CMDBResource{ID: "service/checkout", Kind: "service", Name: "Checkout"},
			}},
			want: "resource must include owners, topology, or attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProvider(tt.records)
			if err == nil {
				t.Fatal("NewProvider err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNewProviderAllowsAttributeLimitSeparateFromMatchLabels(t *testing.T) {
	attributes := make(map[string]string, maxMatchLabels+1)
	for i := 0; i < maxMatchLabels+1; i++ {
		attributes[fmt.Sprintf("attr_%02d", i)] = "value"
	}
	_, err := NewProvider([]Record{{
		MatchLabels: map[string]string{"service": "checkout"},
		Resource: ports.CMDBResource{
			ID:         "service/checkout",
			Kind:       "service",
			Name:       "Checkout",
			Attributes: attributes,
		},
	}})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
}

func TestNewProviderFromYAMLRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "duplicate key",
			raw: `
records:
  - match_labels:
      service: checkout
      service: payments
    resource:
      id: service/checkout
      kind: service
      name: Checkout
      owners:
        - team: Checkout
`,
			want: `duplicate YAML key "service"`,
		},
		{
			name: "unknown field",
			raw: `
records:
  - match_labels:
      service: checkout
    resource:
      id: service/checkout
      kind: service
      name: Checkout
      owners:
        - team: Checkout
      endpoint: https://example.com
`,
			want: "field endpoint not found",
		},
		{
			name: "multiple documents",
			raw: `
records: []
---
records: []
`,
			want: "multiple YAML documents",
		},
		{
			name: "alias",
			raw: `
records:
  - match_labels:
      service: checkout
    resource:
      id: service/checkout
      kind: service
      name: Checkout
      owners:
        - team: &team Checkout
          role: *team
`,
			want: "YAML anchors are not allowed",
		},
		{
			name: "anchored key",
			raw: `
records:
  - match_labels:
      &service service: checkout
    resource:
      id: service/checkout
      kind: service
      name: Checkout
      owners:
        - team: Checkout
`,
			want: "YAML anchors are not allowed",
		},
		{
			name: "merge key",
			raw: `
records:
  - match_labels:
      service: checkout
      <<:
        cluster: prod
    resource:
      id: service/checkout
      kind: service
      name: Checkout
      owners:
        - team: Checkout
`,
			want: "YAML merge keys are not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewProviderFromYAML([]byte(tt.raw))
			if err == nil {
				t.Fatal("NewProviderFromYAML err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}
