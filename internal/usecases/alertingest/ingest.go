// Package alertingest persists upstream MetricsProvider output as
// domain.AlertEvent rows. It exposes a single entry point,
// IngestOnce, which queries the provider once and writes each
// returned alert in its own UnitOfWork transaction.
//
// Trigger choice is intentional out of scope at this PR: callers
// (CLI subcommand, scheduled workflow, HTTP endpoint, etc.) are
// expected to land in a later PR alongside the grouping algorithm.
// Until then this package is consumed by tests only.
package alertingest

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Stats summarises one IngestOnce invocation.
//
//   - Total     is the number of alerts the provider returned (i.e.
//     the number of attempts made; not the number of successes).
//   - Saved     counts newly inserted AlertEvent rows.
//   - Duplicate counts pre-existing rows that collapsed via the
//     unique (source, canonical_fingerprint, starts_at) constraint.
//   - Failed    counts alerts that could not be processed for any
//     other reason (invariant violations, repository errors, ...).
//
// The invariant Total == Saved + Duplicate + Failed holds today.
// Future categories (e.g. "Skipped" for provider-side rate limiting)
// will land as new fields rather than by redefining Total, so
// callers can rely on Total === "what the provider gave us".
type Stats struct {
	Total     int
	Saved     int
	Duplicate int
	Failed    int
}

// IngestOnce queries the provider once and persists each returned
// alert as a domain.AlertEvent. Per-alert work runs in its own
// UnitOfWork transaction so a failure on one alert does not affect
// the others.
//
// Error semantics:
//
//   - if provider.ListActiveAlerts fails, the function returns
//     Stats{}, wrapped error; no per-alert work runs;
//   - per-alert errors wrapping domain.ErrAlreadyExists are treated
//     as a Duplicate (success). They MUST be propagated out of the
//     WithinTx callback so the surrounding Postgres tx rolls back:
//     a unique-violation (SQLSTATE 23505) aborts the transaction,
//     so swallowing the error inside the callback would make Commit
//     fail with "current transaction is aborted";
//   - all other per-alert errors are counted as Failed, logged via
//     slog.Warn (without leaking labels / annotations / raw
//     payload), and accumulated; on return they are joined via
//     errors.Join so the caller sees every reason.
//
// Concurrency: not safe for concurrent invocation against the same
// UnitOfWorkFactory.
func IngestOnce(ctx context.Context, provider ports.MetricsProvider, factory ports.UnitOfWorkFactory) (Stats, error) {
	alerts, err := provider.ListActiveAlerts(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("list active alerts: %w", err)
	}

	stats := Stats{Total: len(alerts)}
	var failures []error

	for _, a := range alerts {
		err := ingestOne(ctx, factory, a)
		switch {
		case err == nil:
			stats.Saved++
		case errors.Is(err, domain.ErrAlreadyExists):
			stats.Duplicate++
		default:
			stats.Failed++
			slog.WarnContext(ctx, "alertingest: per-alert ingest failed",
				slog.String("source", a.Source),
				slog.Time("starts_at", a.StartsAt),
				slog.String("canonical_fingerprint", canonicalFingerprint(a.Labels)),
				slog.Any("error", err),
			)
			failures = append(failures, err)
		}
	}

	if len(failures) > 0 {
		return stats, errors.Join(failures...)
	}
	return stats, nil
}

// ingestOne persists a single ActiveAlert inside its own
// transaction. The WithinTx callback returns the SaveEvent error
// verbatim so domain.ErrAlreadyExists rolls back the transaction;
// IngestOnce's outer switch then maps the wrapped error to the
// Duplicate counter.
func ingestOne(ctx context.Context, factory ports.UnitOfWorkFactory, a ports.ActiveAlert) error {
	return factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		evt, err := domain.NewAlertEvent(
			a.Source,
			sourceFingerprint(a.Labels),
			canonicalFingerprint(a.Labels),
			a.Labels,
			a.Annotations,
			a.RawPayload,
			a.StartsAt,
		)
		if err != nil {
			return fmt.Errorf("build alert event: %w", err)
		}
		if _, err := uow.Alerts().SaveEvent(ctx, evt); err != nil {
			// Propagate verbatim so ErrAlreadyExists (or any
			// other repository error) reaches the caller; the
			// surrounding WithinTx rolls back the transaction.
			return err
		}
		return nil
	})
}

// canonicalLabelsJSON serialises `labels` as the byte-stable input
// to both fingerprint hashes. Two behaviours matter:
//
//  1. nil is normalised to an empty map so json.Marshal returns the
//     literal "{}" rather than "null"; otherwise a nil-labels alert
//     and an empty-labels alert would produce different fingerprints
//     despite being semantically identical.
//  2. encoding/json's Marshal sorts map keys lexicographically since
//     Go 1.12, so we do not need to pre-sort the input ourselves.
//
// json.Marshal of map[string]string cannot return an error (no value
// in the map is unrepresentable), so the err branch is a defensive
// panic rather than a returned error: there is no plausible recovery
// at the call site.
func canonicalLabelsJSON(labels map[string]string) []byte {
	if labels == nil {
		labels = map[string]string{}
	}
	b, err := json.Marshal(labels)
	if err != nil {
		panic(fmt.Sprintf("alertingest: canonical label marshal: %v", err))
	}
	return b
}

// sourceFingerprint is the provider-local fingerprint stored in
// AlertEvent.SourceFingerprint. The current implementation uses
// sha1; the dual-track scheme (sha1 source + sha256 canonical) is
// designed so the two can evolve independently if upstream providers
// eventually expose their own fingerprint format we want to mirror.
func sourceFingerprint(labels map[string]string) string {
	sum := sha1.Sum(canonicalLabelsJSON(labels))
	return hex.EncodeToString(sum[:])
}

// canonicalFingerprint is the global cross-source fingerprint stored
// in AlertEvent.CanonicalFingerprint and participates in the natural
// unique key. sha256 keeps the collision probability astronomically
// low across providers.
func canonicalFingerprint(labels map[string]string) string {
	sum := sha256.Sum256(canonicalLabelsJSON(labels))
	return hex.EncodeToString(sum[:])
}
