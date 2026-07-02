package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	e, err := e.WithAlertSourceProfile(7)
	if err != nil {
		t.Fatalf("WithAlertSourceProfile: %v", err)
	}

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
	if saved.AlertSourceProfileID != 7 {
		t.Errorf("saved.AlertSourceProfileID = %d, want 7", saved.AlertSourceProfileID)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Alerts().FindEventByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindEventByID: %v", err)
		}
		if byID.CanonicalFingerprint != "canon-A" {
			t.Errorf("FindEventByID.CanonicalFingerprint = %q, want %q", byID.CanonicalFingerprint, "canon-A")
		}
		if byID.AlertSourceProfileID != 7 {
			t.Errorf("FindEventByID.AlertSourceProfileID = %d, want 7", byID.AlertSourceProfileID)
		}
		_, err = uow.Alerts().FindEventByNaturalKey(ctx, "prometheus", "canon-A", startsAt)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("FindEventByNaturalKey for profiled event = %v, want ErrNotFound", err)
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

func TestAlertRepository_SaveEvent_DedupesWithinAlertSourceProfile(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 11, 30, 0, 0, time.UTC)
	first := mustNewAlertEvent(t, "prometheus", "fp-X", "canon-X", startsAt)
	first, err := first.WithAlertSourceProfile(7)
	if err != nil {
		t.Fatalf("WithAlertSourceProfile first: %v", err)
	}
	second := mustNewAlertEvent(t, "prometheus", "fp-X", "canon-X", startsAt)
	second, err = second.WithAlertSourceProfile(8)
	if err != nil {
		t.Fatalf("WithAlertSourceProfile second: %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Alerts().SaveEvent(ctx, first); err != nil {
			t.Fatalf("SaveEvent first profile: %v", err)
		}
		if _, err := uow.Alerts().SaveEvent(ctx, second); err != nil {
			t.Fatalf("SaveEvent second profile: %v", err)
		}
	})

	ctx := context.Background()
	err = integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Alerts().SaveEvent(ctx, first)
		return serr
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate profile SaveEvent err = %v, want ErrAlreadyExists", err)
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

// seedEventAt is a tiny helper that inserts one event with a unique
// fingerprint at the given starts_at and returns the persisted event.
// Used by ListEventsByStartsAtRange tests where what matters is just
// where the event lands on the timeline.
func seedEventAt(ctx context.Context, t *testing.T, uow ports.UnitOfWork, suffix string, startsAt time.Time) domain.AlertEvent {
	t.Helper()
	e := mustNewAlertEvent(t, "prom", "fp-"+suffix, "canon-"+suffix, startsAt)
	saved, err := uow.Alerts().SaveEvent(ctx, e)
	if err != nil {
		t.Fatalf("SaveEvent %s: %v", suffix, err)
	}
	return saved
}

func TestAlertRepository_ListEventsByStartsAtRange_HalfOpenIntervalAndOrder(t *testing.T) {
	resetDB(t)
	windowStart := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour) // exclusive

	var before, atStart, mid, atEndMinus1, atEnd domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Five events laid out across the window boundary:
		//   before     -> windowStart - 1m  (excluded by GTE)
		//   atStart    -> windowStart        (included, GTE)
		//   mid        -> windowStart + 30m  (included)
		//   atEndMinus1-> windowEnd - 1us    (included)
		//   atEnd      -> windowEnd          (excluded, LT)
		before = seedEventAt(ctx, t, uow, "before", windowStart.Add(-time.Minute))
		atStart = seedEventAt(ctx, t, uow, "start", windowStart)
		mid = seedEventAt(ctx, t, uow, "mid", windowStart.Add(30*time.Minute))
		atEndMinus1 = seedEventAt(ctx, t, uow, "endm1", windowEnd.Add(-time.Microsecond))
		atEnd = seedEventAt(ctx, t, uow, "end", windowEnd)
	})
	_ = before
	_ = atEnd

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().ListEventsByStartsAtRange(ctx, windowStart, windowEnd, 100)
		if err != nil {
			t.Fatalf("ListEventsByStartsAtRange: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("len(got) = %d, want 3 (atStart, mid, atEndMinus1)", len(got))
		}
		wantIDs := []domain.AlertEventID{atStart.ID, mid.ID, atEndMinus1.ID}
		for i, w := range wantIDs {
			if got[i].ID != w {
				t.Errorf("got[%d].ID = %d, want %d (order should be starts_at ASC)", i, got[i].ID, w)
			}
		}
	})
}

func TestAlertRepository_ListEvents_OrdersByStartsAtDescAndLimit(t *testing.T) {
	resetDB(t)
	t0 := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)

	var earlier, laterA, middle, laterB domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		earlier = seedEventAt(ctx, t, uow, "recent-10", t0.Add(10*time.Minute))
		laterA = seedEventAt(ctx, t, uow, "recent-30a", t0.Add(30*time.Minute))
		middle = seedEventAt(ctx, t, uow, "recent-20", t0.Add(20*time.Minute))
		laterB = seedEventAt(ctx, t, uow, "recent-30b", t0.Add(30*time.Minute))
	})
	_, _ = earlier, middle

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().ListEvents(ctx, 2)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		wantIDs := []domain.AlertEventID{laterB.ID, laterA.ID}
		for i, want := range wantIDs {
			if got[i].ID != want {
				t.Errorf("got[%d].ID = %d, want %d (order should be starts_at DESC, id DESC)", i, got[i].ID, want)
			}
		}
	})
}

func TestAlertRepository_ListEvents_RejectsBadLimit(t *testing.T) {
	resetDB(t)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Alerts().ListEvents(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("want errors.Is ErrInvariantViolation, got %v", err)
		}
	})
}

func TestAlertRepository_ListEventsByStartsAtRange_LimitTruncates(t *testing.T) {
	resetDB(t)
	windowStart := time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for i := 0; i < 5; i++ {
			seedEventAt(ctx, t, uow, fmt.Sprintf("l%d", i), windowStart.Add(time.Duration(i)*time.Minute))
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().ListEventsByStartsAtRange(ctx, windowStart, windowEnd, 3)
		if err != nil {
			t.Fatalf("ListEventsByStartsAtRange: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("len(got) = %d, want 3 (capped by limit)", len(got))
		}
	})
}

func TestAlertRepository_ListEventsByStartsAtRange_EmptyWindowReturnsEmptyResult(t *testing.T) {
	resetDB(t)
	t0 := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

	// Seed one event well outside the test window so the table is
	// not empty; the query should still return a zero-length result.
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		seedEventAt(ctx, t, uow, "out", t0.Add(2*time.Hour))
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().ListEventsByStartsAtRange(ctx, t0, t0.Add(time.Hour), 10)
		if err != nil {
			t.Fatalf("ListEventsByStartsAtRange: %v", err)
		}
		// The Ent generated client typically returns an empty slice
		// rather than nil for empty result sets; we accept either by
		// asserting on len, not on nil-ness.
		if len(got) != 0 {
			t.Errorf("len(got) = %d, want 0", len(got))
		}
	})
}

func TestAlertRepository_ListEventsByStartsAtRange_RejectsBadInput(t *testing.T) {
	resetDB(t)
	t0 := time.Date(2026, 5, 26, 13, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		start time.Time
		end   time.Time
		limit int
	}{
		{"zero start", time.Time{}, t0, 10},
		{"zero end", t0, time.Time{}, 10},
		{"limit zero", t0, t0.Add(time.Hour), 0},
		{"limit negative", t0, t0.Add(time.Hour), -1},
		{"end equals start (after normalisation)", t0, t0, 10},
		{"end before start", t0.Add(time.Hour), t0, 10},
		{"normalised collapse", t0.Add(500 * time.Nanosecond), t0.Add(800 * time.Nanosecond), 10},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
				_, err := uow.Alerts().ListEventsByStartsAtRange(ctx, tc.start, tc.end, tc.limit)
				if !errors.Is(err, domain.ErrInvariantViolation) {
					t.Fatalf("want errors.Is ErrInvariantViolation, got %v", err)
				}
			})
		})
	}
}

func TestAlertRepository_FindGroupByNaturalKey_FoundAndNotFound(t *testing.T) {
	resetDB(t)
	first := time.Date(2026, 5, 26, 14, 0, 0, 0, time.UTC)
	last := first.Add(10 * time.Minute)

	var saved domain.AlertGroup
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		g, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "natural-A", first, last))
		if err != nil {
			t.Fatalf("SaveGroup: %v", err)
		}
		saved = g
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().FindGroupByNaturalKey(ctx, "natural-A", first)
		if err != nil {
			t.Fatalf("FindGroupByNaturalKey (found): %v", err)
		}
		if got.ID != saved.ID {
			t.Errorf("got.ID = %d, want %d", got.ID, saved.ID)
		}
		if got.GroupKey != "natural-A" {
			t.Errorf("got.GroupKey = %q, want %q", got.GroupKey, "natural-A")
		}
		// EventIDs is intentionally left nil; callers materialise via
		// ListEventIDsForGroup when needed.
		if got.EventIDs != nil {
			t.Errorf("got.EventIDs = %v, want nil (mapper does not materialise M2N)", got.EventIDs)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Different group_key, same first_seen_at -> NotFound.
		_, err := uow.Alerts().FindGroupByNaturalKey(ctx, "natural-Z", first)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("FindGroupByNaturalKey wrong key: want errors.Is ErrNotFound, got %v", err)
		}
		// Same group_key, different first_seen_at -> NotFound.
		_, err = uow.Alerts().FindGroupByNaturalKey(ctx, "natural-A", first.Add(time.Hour))
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("FindGroupByNaturalKey wrong first_seen_at: want errors.Is ErrNotFound, got %v", err)
		}
	})
}

func TestAlertRepository_FindGroupByNaturalKey_RejectsBadInput(t *testing.T) {
	resetDB(t)
	t0 := time.Date(2026, 5, 26, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		key   string
		first time.Time
	}{
		{"empty key", "", t0},
		{"zero first_seen_at", "some-key", time.Time{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
				_, err := uow.Alerts().FindGroupByNaturalKey(ctx, tc.key, tc.first)
				if !errors.Is(err, domain.ErrInvariantViolation) {
					t.Fatalf("want errors.Is ErrInvariantViolation, got %v", err)
				}
			})
		})
	}
}
