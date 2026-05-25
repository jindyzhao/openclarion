package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// withTx is a tiny helper so individual cases stay focused on the
// repository call under test rather than the WithinTx boilerplate.
// It fatals on commit errors so the assertion fixture is the
// repository, not the UoW plumbing.
func withTx(t *testing.T, fn func(ctx context.Context, uow ports.UnitOfWork)) {
	t.Helper()
	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		fn(ctx, uow)
		return nil
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
}

func mustNewAlertEvent(t *testing.T, source, fp, canon string, startsAt time.Time) domain.AlertEvent {
	t.Helper()
	e, err := domain.NewAlertEvent(
		source, fp, canon,
		map[string]string{"k": "v"},
		map[string]string{"summary": "s"},
		json.RawMessage(`{"raw":1}`),
		startsAt,
	)
	if err != nil {
		t.Fatalf("NewAlertEvent: %v", err)
	}
	return e
}

func mustNewAlertGroup(t *testing.T, key string, first, last time.Time) domain.AlertGroup {
	t.Helper()
	g, err := domain.NewAlertGroup(
		key,
		json.RawMessage(`{"region":"us"}`),
		domain.GroupSeverityWarning,
		1,
		first, last,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertGroup: %v", err)
	}
	return g
}

func TestAlertRepository_SaveEventAndQuery(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	e := mustNewAlertEvent(t, "prometheus", "fp-1", "canon-A", startsAt)

	var saved domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().SaveEvent(ctx, e)
		if err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Errorf("saved.ID = 0, want non-zero")
	}
	if saved.CreatedAt.IsZero() {
		t.Errorf("saved.CreatedAt is zero, want non-zero")
	}
	if saved.Status != domain.AlertStatusFiring {
		t.Errorf("saved.Status = %q, want %q", saved.Status, domain.AlertStatusFiring)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Alerts().FindEventByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindEventByID: %v", err)
		}
		if byID.CanonicalFingerprint != "canon-A" {
			t.Errorf("FindEventByID.CanonicalFingerprint = %q, want %q", byID.CanonicalFingerprint, "canon-A")
		}
		// Natural key lookup uses the same normalised timestamp the
		// repository writes; passing the original (already UTC)
		// startsAt is fine because NormalizeUTCMicro is idempotent.
		byKey, err := uow.Alerts().FindEventByNaturalKey(ctx, "prometheus", "canon-A", startsAt)
		if err != nil {
			t.Fatalf("FindEventByNaturalKey: %v", err)
		}
		if byKey.ID != saved.ID {
			t.Errorf("FindEventByNaturalKey.ID = %d, want %d", byKey.ID, saved.ID)
		}
	})
}

func TestAlertRepository_SaveEvent_DuplicateNaturalKey(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	e := mustNewAlertEvent(t, "prometheus", "fp-X", "canon-X", startsAt)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Alerts().SaveEvent(ctx, e); err != nil {
			t.Fatalf("first SaveEvent: %v", err)
		}
	})

	// Second SaveEvent with the same (source, canonical_fingerprint,
	// starts_at) MUST collapse to a wrapped ErrAlreadyExists. This
	// is what allows ingestion Activities to retry safely.
	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Alerts().SaveEvent(ctx, e)
		return serr
	})
	if err == nil {
		t.Fatalf("second SaveEvent: want error, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("second SaveEvent: want errors.Is ErrAlreadyExists, got %v", err)
	}
}

func TestAlertRepository_UpdateEventResolution(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	endsAt := startsAt.Add(15 * time.Minute)
	e := mustNewAlertEvent(t, "prometheus", "fp-r", "canon-r", startsAt)

	var saved domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().SaveEvent(ctx, e)
		if err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
		saved = got
	})

	resolved, err := saved.Resolve(endsAt)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		updated, uerr := uow.Alerts().UpdateEventResolution(ctx, resolved)
		if uerr != nil {
			t.Fatalf("UpdateEventResolution: %v", uerr)
		}
		if updated.Status != domain.AlertStatusResolved {
			t.Errorf("updated.Status = %q, want resolved", updated.Status)
		}
		if updated.EndsAt == nil || !updated.EndsAt.Equal(domain.NormalizeUTCMicro(endsAt)) {
			t.Errorf("updated.EndsAt = %v, want %v", updated.EndsAt, endsAt)
		}
	})
}

func TestAlertRepository_GroupAndLinkEvents(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	first := startsAt
	last := startsAt.Add(5 * time.Minute)

	var groupID domain.AlertGroupID
	var eventIDs []domain.AlertEventID

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		g, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "key-A", first, last))
		if err != nil {
			t.Fatalf("SaveGroup: %v", err)
		}
		groupID = g.ID

		// Two events with deliberately out-of-order starts_at so we
		// can verify ListEventIDsForGroup orders ascending.
		e1, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prom", "fp-2", "c-2", startsAt.Add(2*time.Minute)))
		if err != nil {
			t.Fatalf("SaveEvent e1: %v", err)
		}
		e0, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prom", "fp-1", "c-1", startsAt))
		if err != nil {
			t.Fatalf("SaveEvent e0: %v", err)
		}
		eventIDs = []domain.AlertEventID{e1.ID, e0.ID}

		if err := uow.Alerts().LinkEventsToGroup(ctx, groupID, eventIDs); err != nil {
			t.Fatalf("LinkEventsToGroup: %v", err)
		}
		// Re-link: the M2N edge MUST be idempotent so that ingestion
		// Activity retries do not double-count or fail. We re-pass
		// the same slice in reversed order to be sure no ordering
		// dependence sneaks in.
		if err := uow.Alerts().LinkEventsToGroup(ctx, groupID, []domain.AlertEventID{eventIDs[1], eventIDs[0]}); err != nil {
			t.Fatalf("LinkEventsToGroup (idempotent re-link): %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		ids, err := uow.Alerts().ListEventIDsForGroup(ctx, groupID)
		if err != nil {
			t.Fatalf("ListEventIDsForGroup: %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("ListEventIDsForGroup len = %d, want 2 (idempotent re-link must not duplicate)", len(ids))
		}
		// Ordered by AlertEvent.starts_at ascending: e0 first
		// (startsAt), then e1 (startsAt+2m).
		if ids[0] != eventIDs[1] || ids[1] != eventIDs[0] {
			t.Errorf("ListEventIDsForGroup order = %v, want [e0=%d, e1=%d]", ids, eventIDs[1], eventIDs[0])
		}
	})
}

func TestAlertRepository_ListActiveGroups_OrdersByLastSeenDesc(t *testing.T) {
	resetDB(t)
	t0 := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Three groups with distinct last_seen_at so ordering is
		// observable. Distinct group_keys avoid the natural-key
		// unique constraint.
		for i, mins := range []int{10, 30, 20} {
			g := mustNewAlertGroup(t, "k-"+string(rune('A'+i)), t0, t0.Add(time.Duration(mins)*time.Minute))
			if _, err := uow.Alerts().SaveGroup(ctx, g); err != nil {
				t.Fatalf("SaveGroup: %v", err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Alerts().ListActiveGroups(ctx, 10)
		if err != nil {
			t.Fatalf("ListActiveGroups: %v", err)
		}
		if len(out) != 3 {
			t.Fatalf("ListActiveGroups len = %d, want 3", len(out))
		}
		// Expected order by last_seen_at desc: 30m, 20m, 10m.
		if out[0].LastSeenAt.Sub(t0) != 30*time.Minute {
			t.Errorf("out[0] last_seen_at delta = %v, want 30m", out[0].LastSeenAt.Sub(t0))
		}
		if out[1].LastSeenAt.Sub(t0) != 20*time.Minute {
			t.Errorf("out[1] last_seen_at delta = %v, want 20m", out[1].LastSeenAt.Sub(t0))
		}
		if out[2].LastSeenAt.Sub(t0) != 10*time.Minute {
			t.Errorf("out[2] last_seen_at delta = %v, want 10m", out[2].LastSeenAt.Sub(t0))
		}
	})
}

func TestAlertRepository_ListEventIDsForGroup_MissingGroupReturnsNotFound(t *testing.T) {
	resetDB(t)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Alerts().ListEventIDsForGroup(ctx, 999999)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("ListEventIDsForGroup missing group: want errors.Is ErrNotFound, got %v", err)
		}
	})
}

func TestAlertRepository_ListEventIDsForGroup_ExistingEmptyGroupReturnsEmptySlice(t *testing.T) {
	resetDB(t)
	first := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	last := first.Add(time.Minute)

	var groupID domain.AlertGroupID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		g, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "empty-group", first, last))
		if err != nil {
			t.Fatalf("SaveGroup: %v", err)
		}
		groupID = g.ID
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		ids, err := uow.Alerts().ListEventIDsForGroup(ctx, groupID)
		if err != nil {
			t.Fatalf("ListEventIDsForGroup existing empty group: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("ListEventIDsForGroup len = %d, want 0", len(ids))
		}
	})
}
