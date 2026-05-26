package fake

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// seedFor returns a single-element seed slice whose every reference
// field is freshly allocated so each test case can mutate its inputs
// without leaking into a sibling case.
func seedFor(t *testing.T) []ports.ActiveAlert {
	t.Helper()
	return []ports.ActiveAlert{{
		Source:      "prometheus",
		Labels:      map[string]string{"alertname": "HighCPU", "severity": "warning"},
		Annotations: map[string]string{"summary": "cpu high"},
		StartsAt:    time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC),
		RawPayload:  json.RawMessage(`{"raw":"original"}`),
	}}
}

// TestNew_DeepCopiesSeed_PreventsPostConstructPollution covers the
// "construction" half of the deep-copy contract: after the caller
// hands the seed to fake.New, any subsequent mutation of the
// original slice (or the maps / RawPayload inside its elements)
// MUST NOT change what ListActiveAlerts later returns.
//
// Without this guarantee a test that builds a seed, calls New, and
// then keeps editing the same slice (e.g. to stage a second
// scenario) would silently bleed those edits into the provider.
func TestNew_DeepCopiesSeed_PreventsPostConstructPollution(t *testing.T) {
	seed := seedFor(t)
	p := New(seed)

	// Mutate every reference-typed field of the original seed AFTER
	// New has returned. None of these mutations should be visible.
	seed[0].Source = "MUTATED"
	seed[0].Labels["alertname"] = "MUTATED"
	seed[0].Labels["new_key"] = "MUTATED"
	seed[0].Annotations["summary"] = "MUTATED"
	seed[0].RawPayload[0] = 'X'
	seed = append(seed, ports.ActiveAlert{Source: "extra"}) // grow original slice header

	got, err := p.ListActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (mutating seed after New must not grow provider state)", len(got))
	}
	if got[0].Source != "prometheus" {
		t.Errorf("got[0].Source = %q, want %q (Source value-typed copy lost)", got[0].Source, "prometheus")
	}
	if got[0].Labels["alertname"] != "HighCPU" {
		t.Errorf("got[0].Labels[alertname] = %q, want %q (Labels map shared with seed)", got[0].Labels["alertname"], "HighCPU")
	}
	if _, ok := got[0].Labels["new_key"]; ok {
		t.Errorf("got[0].Labels contains new_key, want absent (Labels map shared with seed)")
	}
	if got[0].Annotations["summary"] != "cpu high" {
		t.Errorf("got[0].Annotations[summary] = %q, want %q (Annotations map shared with seed)", got[0].Annotations["summary"], "cpu high")
	}
	if string(got[0].RawPayload) != `{"raw":"original"}` {
		t.Errorf("got[0].RawPayload = %s, want original (RawPayload bytes shared with seed)", string(got[0].RawPayload))
	}
}

// TestListActiveAlerts_DeepCopiesReturn_PreventsConsumerPollution
// covers the "read" half of the deep-copy contract: after List
// hands a slice to the caller, any mutation of that returned slice
// (or the maps / RawPayload inside its elements) MUST NOT change
// what the NEXT call to ListActiveAlerts returns.
//
// Without this guarantee a consumer that calls List, defensively
// edits its copy, and re-calls List would observe the edits as if
// the provider had changed.
func TestListActiveAlerts_DeepCopiesReturn_PreventsConsumerPollution(t *testing.T) {
	p := New(seedFor(t))
	ctx := context.Background()

	first, err := p.ListActiveAlerts(ctx)
	if err != nil {
		t.Fatalf("first ListActiveAlerts: %v", err)
	}

	// Mutate every reference-typed field of the FIRST return.
	first[0].Source = "MUTATED"
	first[0].Labels["alertname"] = "MUTATED"
	first[0].Labels["new_key"] = "MUTATED"
	first[0].Annotations["summary"] = "MUTATED"
	first[0].RawPayload[0] = 'X'

	second, err := p.ListActiveAlerts(ctx)
	if err != nil {
		t.Fatalf("second ListActiveAlerts: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("len(second) = %d, want 1", len(second))
	}
	if second[0].Source != "prometheus" {
		t.Errorf("second[0].Source = %q, want %q (return Source shared across calls)", second[0].Source, "prometheus")
	}
	if second[0].Labels["alertname"] != "HighCPU" {
		t.Errorf("second[0].Labels[alertname] = %q, want %q (return Labels map shared across calls)", second[0].Labels["alertname"], "HighCPU")
	}
	if _, ok := second[0].Labels["new_key"]; ok {
		t.Errorf("second[0].Labels contains new_key, want absent (return Labels map shared across calls)")
	}
	if second[0].Annotations["summary"] != "cpu high" {
		t.Errorf("second[0].Annotations[summary] = %q, want %q (return Annotations map shared across calls)", second[0].Annotations["summary"], "cpu high")
	}
	if string(second[0].RawPayload) != `{"raw":"original"}` {
		t.Errorf("second[0].RawPayload = %s, want original (return RawPayload bytes shared across calls)", string(second[0].RawPayload))
	}
}

// TestListActiveAlerts_PreservesNilFields documents that the fake
// provider does NOT normalise nil maps / nil RawPayload to empty
// values. The IngestOnce pipeline handles nil at the boundary; the
// provider's job is only to round-trip the seed faithfully.
func TestListActiveAlerts_PreservesNilFields(t *testing.T) {
	p := New([]ports.ActiveAlert{{
		Source:   "prometheus",
		StartsAt: time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC),
		// Labels / Annotations / RawPayload all nil.
	}})
	got, err := p.ListActiveAlerts(context.Background())
	if err != nil {
		t.Fatalf("ListActiveAlerts: %v", err)
	}
	if got[0].Labels != nil {
		t.Errorf("got[0].Labels = %v, want nil", got[0].Labels)
	}
	if got[0].Annotations != nil {
		t.Errorf("got[0].Annotations = %v, want nil", got[0].Annotations)
	}
	if got[0].RawPayload != nil {
		t.Errorf("got[0].RawPayload = %v, want nil", got[0].RawPayload)
	}
}
