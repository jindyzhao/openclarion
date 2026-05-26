package ports

import (
	"context"
	"encoding/json"
	"time"
)

// ActiveAlert is the minimal projection of an upstream metrics
// provider's active (firing) alert. Concrete providers translate
// their native payload into this DTO before the ingestion library
// converts it to a domain.AlertEvent.
//
// Field semantics:
//
//   - Source: provider source identifier (e.g. "prometheus"). The
//     ingestion library forwards this value verbatim into
//     domain.AlertEvent.Source where it participates in the
//     (source, canonical_fingerprint, starts_at) natural unique key.
//   - Labels / Annotations: free-form key/value metadata. Both MAY be
//     nil or empty; the ingestion library normalises nil to an empty
//     map before fingerprinting and before constructing the domain
//     entity, so downstream behaviour is identical either way.
//   - StartsAt: alert activation time. MUST be non-zero; the
//     ingestion library forwards it to domain.NewAlertEvent which
//     rejects the zero value as an invariant violation. Time-zone is
//     normalised to UTC by the domain constructor.
//   - RawPayload: provider's native JSON representation of the
//     alert. MAY be nil; the persistence layer treats the column as
//     optional JSONB.
type ActiveAlert struct {
	Source      string
	Labels      map[string]string
	Annotations map[string]string
	StartsAt    time.Time
	RawPayload  json.RawMessage
}

// MetricsProvider is the upstream alert source contract. Each call
// to ListActiveAlerts independently queries the upstream system and
// returns the currently-firing alerts; the provider MUST NOT carry
// across-call state that affects the returned set.
//
// Layering rules:
//
//   - This package (usecase-facing DTOs / ports) MUST depend only
//     on the Go standard library and the domain package, so the
//     usecase layer stays import-clean.
//   - Concrete providers live under internal/providers/metrics/<impl>
//     and MAY import third-party SDKs (e.g. github.com/prometheus/
//     client_golang) as needed; in exchange they MUST NOT be
//     imported by anything inside internal/usecases or
//     internal/domain, which enforces one-way dependency from
//     concrete providers towards this port.
//
// Provider-side filtering policy: implementations are responsible
// for filtering out non-firing states (Prometheus "pending" /
// "inactive", etc.) so the DTO never carries alerts the domain
// model would reject. Consumers MAY assume every returned alert is
// firing.
type MetricsProvider interface {
	ListActiveAlerts(ctx context.Context) ([]ActiveAlert, error)
}
