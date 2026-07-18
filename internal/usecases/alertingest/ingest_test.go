package alertingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/providers/metrics/fake"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type activeAlertProviderFunc func(context.Context) ([]ports.ActiveAlert, error)

func (f activeAlertProviderFunc) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	return f(ctx)
}

// firingSeed builds a deterministic batch of N firing-like
// ActiveAlerts whose label sets differ (so the canonical fingerprint
// + starts_at + source natural key is unique per alert and the
// unique constraint does not collapse them).
//
// The base timestamp is fixed so test failures point to a stable
// "expected vs actual" rather than a moving wall-clock value.
func firingSeed(n int) []ports.ActiveAlert {
	base := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	out := make([]ports.ActiveAlert, n)
	for i := 0; i < n; i++ {
		out[i] = ports.ActiveAlert{
			Source: "prometheus",
			Labels: map[string]string{
				"alertname": "HighCPU",
				"instance":  itoaInstance(i),
			},
			Annotations: map[string]string{"summary": "cpu high"},
			StartsAt:    base.Add(time.Duration(i) * time.Minute),
			RawPayload:  json.RawMessage(`{"raw":1}`),
		}
	}
	return out
}

// itoaInstance keeps the helper dependency-free; strconv.Itoa would
// work equally well but the import noise is not worth it for a
// two-digit counter.
func itoaInstance(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return string(digits[i])
	}
	return string(digits[i/10]) + string(digits[i%10])
}

// countAlertEvents reads the row count directly through the Ent
// client. We bypass the repository so the test asserts on the
// "ground truth" (what landed in the table) rather than re-using
// the code path under test.
func countAlertEvents(ctx context.Context, t *testing.T) int {
	t.Helper()
	n, err := integration.client.AlertEvent.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count alert_event rows: %v", err)
	}
	return n
}

// TestIngestOnce_SavesAllFiringAlerts covers the happy path: every
// alert returned by the provider becomes one AlertEvent row and the
// Stats add up to Saved=N.
func TestIngestOnce_SavesAllFiringAlerts(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())
	const n = 3

	provider := fake.New(firingSeed(n))
	stats, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if err != nil {
		t.Fatalf("IngestOnce: %v", err)
	}
	if stats != (alertingest.Stats{Total: n, Saved: n}) {
		t.Errorf("stats = %+v, want {Total:%d,Saved:%d,Duplicate:0,Failed:0}", stats, n, n)
	}
	if got := countAlertEvents(ctx, t); got != n {
		t.Errorf("alert_event row count = %d, want %d", got, n)
	}
}

func TestIngestAlerts_SavesMaterializedAlerts(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())
	const n = 2

	stats, err := alertingest.IngestAlerts(ctx, firingSeed(n), integration.factory)
	if err != nil {
		t.Fatalf("IngestAlerts: %v", err)
	}
	if stats != (alertingest.Stats{Total: n, Saved: n}) {
		t.Errorf("stats = %+v, want {Total:%d,Saved:%d,Duplicate:0,Failed:0}", stats, n, n)
	}
	if got := countAlertEvents(ctx, t); got != n {
		t.Errorf("alert_event row count = %d, want %d", got, n)
	}
}

// TestIngestOnce_DuplicateRunCountsAsDuplicate runs the same seed
// through IngestOnce twice. The second pass MUST collapse every
// alert into the Duplicate counter via the natural unique key, and
// the row count MUST stay at N. This is the regression guard for
// the "duplicate must propagate out of WithinTx so the tx rolls
// back" rule documented in ingest.go.
func TestIngestOnce_DuplicateRunCountsAsDuplicate(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())
	const n = 3

	provider := fake.New(firingSeed(n))

	first, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if err != nil {
		t.Fatalf("first IngestOnce: %v", err)
	}
	if first.Saved != n {
		t.Fatalf("first.Saved = %d, want %d", first.Saved, n)
	}

	second, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if err != nil {
		t.Fatalf("second IngestOnce: %v", err)
	}
	if second != (alertingest.Stats{Total: n, Duplicate: n}) {
		t.Errorf("second stats = %+v, want {Total:%d,Saved:0,Duplicate:%d,Failed:0}", second, n, n)
	}
	if got := countAlertEvents(ctx, t); got != n {
		t.Errorf("alert_event row count after duplicate run = %d, want %d (a leaked save means tx did not roll back)", got, n)
	}
}

func TestIngestOnce_PersistsPartialPullAndRetryConverges(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())
	fullBatch := firingSeed(3)
	providerErr := errors.New("upstream page failed")
	partialProvider := activeAlertProviderFunc(func(context.Context) ([]ports.ActiveAlert, error) {
		return fullBatch[:2], providerErr
	})

	partial, err := alertingest.IngestOnce(ctx, partialProvider, integration.factory)
	if !errors.Is(err, alertingest.ErrIncompletePull) || !errors.Is(err, providerErr) {
		t.Fatalf("partial pull error = %v, want ErrIncompletePull and provider cause", err)
	}
	if partial != (alertingest.Stats{Total: 2, Saved: 2}) {
		t.Fatalf("partial stats = %+v, want two saved alerts", partial)
	}
	if got := countAlertEvents(ctx, t); got != 2 {
		t.Fatalf("alert_event count after partial pull = %d, want 2", got)
	}

	complete, err := alertingest.IngestOnce(ctx, fake.New(fullBatch), integration.factory)
	if err != nil {
		t.Fatalf("complete retry: %v", err)
	}
	if complete != (alertingest.Stats{Total: 3, Saved: 1, Duplicate: 2}) {
		t.Fatalf("complete retry stats = %+v, want one saved and two duplicates", complete)
	}
	if got := countAlertEvents(ctx, t); got != 3 {
		t.Fatalf("alert_event count after complete retry = %d, want 3", got)
	}
}

func TestIngestOnce_CanceledPartialPullDoesNotPersist(t *testing.T) {
	resetDB(t)
	baseCtx := tenancy.EnsureDefault(context.Background())
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()
	provider := activeAlertProviderFunc(func(context.Context) ([]ports.ActiveAlert, error) {
		cancel()
		return firingSeed(1), context.Canceled
	})

	stats, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IngestOnce error = %v, want context.Canceled", err)
	}
	if stats != (alertingest.Stats{}) {
		t.Fatalf("stats = %+v, want zero on canceled request", stats)
	}
	if got := countAlertEvents(baseCtx, t); got != 0 {
		t.Fatalf("alert_event count = %d, want 0", got)
	}
}

func TestIngestOnce_ProviderDeadlinePartialPullDoesNotPersist(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())
	provider := activeAlertProviderFunc(func(context.Context) ([]ports.ActiveAlert, error) {
		return firingSeed(1), fmt.Errorf("provider timeout: %w", context.DeadlineExceeded)
	})

	stats, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("IngestOnce error = %v, want context.DeadlineExceeded", err)
	}
	if stats != (alertingest.Stats{}) {
		t.Fatalf("stats = %+v, want zero on provider deadline", stats)
	}
	if got := countAlertEvents(ctx, t); got != 0 {
		t.Fatalf("alert_event count = %d, want 0", got)
	}
}

// TestIngestOnce_InvariantViolationCountsAsFailedAndDoesNotBlockOthers
// seeds a batch where one alert is invalid (Source==""), and
// verifies:
//
//   - the invalid alert is counted as Failed (not Saved, not
//     Duplicate);
//   - the surrounding valid alerts still land as Saved;
//   - the returned error unwraps to domain.ErrInvariantViolation
//     via errors.Is, which is what tests / callers MUST rely on
//     instead of pattern-matching error strings.
//
// Source="" is the chosen invalid surface because empty labels are
// explicitly NOT invalid (domain.NewAlertEvent normalises nil
// labels to an empty map; the resulting SHA-256 fingerprints of `{}` are
// non-empty fingerprints).
func TestIngestOnce_InvariantViolationCountsAsFailedAndDoesNotBlockOthers(t *testing.T) {
	resetDB(t)
	ctx := tenancy.EnsureDefault(context.Background())

	seed := firingSeed(2)
	bad := ports.ActiveAlert{
		Source: "", // invariant violation: Source must be non-empty
		Labels: map[string]string{"alertname": "Invalid"},
		// StartsAt deliberately set so that, were Source valid, the
		// alert would be syntactically fine; this isolates the
		// "Source rejected" failure mode.
		StartsAt: time.Date(2026, 5, 26, 13, 0, 0, 0, time.UTC),
	}
	// Inject the bad alert in the middle to prove the loop keeps
	// going past a failure.
	batch := []ports.ActiveAlert{seed[0], bad, seed[1]}
	provider := fake.New(batch)

	stats, err := alertingest.IngestOnce(ctx, provider, integration.factory)
	if err == nil {
		t.Fatalf("IngestOnce: want non-nil error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("IngestOnce error: want errors.Is ErrInvariantViolation, got %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("stats.Total = %d, want 3", stats.Total)
	}
	if stats.Saved != 2 {
		t.Errorf("stats.Saved = %d, want 2 (failure on one alert must not block others)", stats.Saved)
	}
	if stats.Failed < 1 {
		t.Errorf("stats.Failed = %d, want >= 1", stats.Failed)
	}
	if stats.Duplicate != 0 {
		t.Errorf("stats.Duplicate = %d, want 0", stats.Duplicate)
	}
	if got := countAlertEvents(ctx, t); got != 2 {
		t.Errorf("alert_event row count = %d, want 2 (only the two valid alerts should have been persisted)", got)
	}
}
