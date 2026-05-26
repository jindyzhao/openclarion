// Package fake provides a deterministic in-memory MetricsProvider
// implementation for tests and local development. It accepts a seed
// slice of ports.ActiveAlert at construction time and returns a
// deep copy of that slice on every ListActiveAlerts call.
//
// Two layers of deep copy are intentional:
//
//   - on construction (New): the seed is copied so callers cannot
//     mutate provider state by holding the original slice;
//   - on read (ListActiveAlerts): the returned slice is copied so
//     consumers cannot mutate provider state by modifying the
//     returned value (which would surface in the next call).
//
// time.Time is a value type and is copied implicitly by the struct
// copy; only the reference-typed fields (slice header, map values,
// json.RawMessage) need explicit cloning.
package fake

import (
	"context"
	"encoding/json"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Provider is the in-memory MetricsProvider implementation.
type Provider struct {
	seed []ports.ActiveAlert
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.MetricsProvider = (*Provider)(nil)

// New constructs a Provider from the given seed slice. The seed is
// deep-copied so subsequent mutations of `alerts` (or any of its
// element fields) do not affect the provider's internal state.
func New(alerts []ports.ActiveAlert) *Provider {
	return &Provider{seed: cloneAlerts(alerts)}
}

// ListActiveAlerts returns a deep copy of the provider's seed
// slice. Each call returns an independent slice so mutations by one
// caller do not leak to the next call.
func (p *Provider) ListActiveAlerts(_ context.Context) ([]ports.ActiveAlert, error) {
	return cloneAlerts(p.seed), nil
}

// cloneAlerts deep-copies the slice + each ActiveAlert's
// reference-typed fields (Labels, Annotations, RawPayload). Nil
// input is preserved as nil output so the caller can distinguish
// "provider returned nothing" from "provider returned an empty
// slice"; in practice IngestOnce treats both identically.
func cloneAlerts(in []ports.ActiveAlert) []ports.ActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]ports.ActiveAlert, len(in))
	for i, a := range in {
		out[i] = ports.ActiveAlert{
			Source:      a.Source,
			Labels:      cloneStringMap(a.Labels),
			Annotations: cloneStringMap(a.Annotations),
			StartsAt:    a.StartsAt,
			RawPayload:  cloneRawPayload(a.RawPayload),
		}
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneRawPayload(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
