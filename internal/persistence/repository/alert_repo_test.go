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

func alertEventIDsForTest(events []domain.AlertEvent) []domain.AlertEventID {
	ids := make([]domain.AlertEventID, len(events))
	for i := range events {
		ids[i] = events[i].ID
	}
	return ids
}

func makeAlertSourceProfiles(t *testing.T, count int) []domain.AlertSourceProfileID {
	t.Helper()
	ids := make([]domain.AlertSourceProfileID, 0, count)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for i := range count {
			profile, err := uow.Config().SaveAlertSourceProfile(
				ctx,
				mustNewAlertSourceProfile(t, fmt.Sprintf("Alert source %d", i+1)),
			)
			if err != nil {
				t.Fatalf("SaveAlertSourceProfile[%d]: %v", i, err)
			}
			ids = append(ids, profile.ID)
		}
	})
	return ids
}

func TestAlertRepository_SaveEventAndQuery(t *testing.T) {
	resetDB(t)
	profileID := makeAlertSourceProfiles(t, 1)[0]
	startsAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	e := mustNewAlertEvent(t, "prometheus", "fp-1", "canon-A", startsAt)
	e, err := e.WithAlertSourceProfile(profileID)
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
	if saved.AlertSourceProfileID != profileID {
		t.Errorf("saved.AlertSourceProfileID = %d, want %d", saved.AlertSourceProfileID, profileID)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Alerts().FindEventByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindEventByID: %v", err)
		}
		if byID.CanonicalFingerprint != "canon-A" {
			t.Errorf("FindEventByID.CanonicalFingerprint = %q, want %q", byID.CanonicalFingerprint, "canon-A")
		}
		if byID.AlertSourceProfileID != profileID {
			t.Errorf("FindEventByID.AlertSourceProfileID = %d, want %d", byID.AlertSourceProfileID, profileID)
		}
		_, err = uow.Alerts().FindEventByNaturalKey(ctx, "prometheus", "canon-A", startsAt)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("FindEventByNaturalKey for profiled event = %v, want ErrNotFound", err)
		}
	})
}

func TestAlertRepository_ListEventsByNaturalKeysMatchesFullScopedKey(t *testing.T) {
	resetDB(t)
	profileIDs := makeAlertSourceProfiles(t, 2)
	startsAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	events := []domain.AlertEvent{
		mustNewAlertEvent(t, "prometheus", "fp-1", "canon-A", startsAt),
		mustNewAlertEvent(t, "prometheus", "fp-2", "canon-B", startsAt),
		mustNewAlertEvent(t, "prometheus", "fp-3", "canon-A", startsAt.Add(time.Microsecond)),
	}
	for i := range events {
		var err error
		events[i], err = events[i].WithAlertSourceProfile(profileIDs[0])
		if err != nil {
			t.Fatalf("WithAlertSourceProfile event[%d]: %v", i, err)
		}
	}
	otherProfile := mustNewAlertEvent(t, "prometheus", "fp-4", "canon-A", startsAt)
	otherProfile, err := otherProfile.WithAlertSourceProfile(profileIDs[1])
	if err != nil {
		t.Fatalf("WithAlertSourceProfile other profile: %v", err)
	}
	events = append(events, otherProfile)

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for i := range events {
			if _, err := uow.Alerts().SaveEvent(ctx, events[i]); err != nil {
				t.Fatalf("SaveEvent[%d]: %v", i, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		rows, err := uow.Alerts().ListEventsByNaturalKeys(ctx, []ports.AlertEventNaturalKey{
			{
				AlertSourceProfileID: profileIDs[0],
				Source:               "prometheus",
				CanonicalFingerprint: "canon-A",
				StartsAt:             startsAt,
			},
			{
				AlertSourceProfileID: profileIDs[0],
				Source:               "prometheus",
				CanonicalFingerprint: "canon-A",
				StartsAt:             startsAt.Add(time.Microsecond),
			},
		}, 3)
		if err != nil {
			t.Fatalf("ListEventsByNaturalKeys: %v", err)
		}
		if len(rows) != 2 ||
			rows[0].AlertSourceProfileID != profileIDs[0] ||
			rows[0].CanonicalFingerprint != "canon-A" ||
			!rows[0].StartsAt.Equal(domain.NormalizeUTCMicro(startsAt)) ||
			rows[1].AlertSourceProfileID != profileIDs[0] ||
			rows[1].CanonicalFingerprint != "canon-A" ||
			!rows[1].StartsAt.Equal(domain.NormalizeUTCMicro(startsAt.Add(time.Microsecond))) {
			t.Fatalf("rows = %+v, want only exact profile/fingerprint/start matches", rows)
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
	profileIDs := makeAlertSourceProfiles(t, 2)
	startsAt := time.Date(2026, 5, 22, 11, 30, 0, 0, time.UTC)
	first := mustNewAlertEvent(t, "prometheus", "fp-X", "canon-X", startsAt)
	first, err := first.WithAlertSourceProfile(profileIDs[0])
	if err != nil {
		t.Fatalf("WithAlertSourceProfile first: %v", err)
	}
	second := mustNewAlertEvent(t, "prometheus", "fp-X", "canon-X", startsAt)
	second, err = second.WithAlertSourceProfile(profileIDs[1])
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
	beforeStart := resolved
	invalidEndsAt := startsAt.Add(-time.Minute)
	beforeStart.StartsAt = time.Time{}
	beforeStart.EndsAt = &invalidEndsAt
	err = integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, updateErr := uow.Alerts().UpdateEventResolution(ctx, beforeStart)
		return updateErr
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("before-start UpdateEventResolution err = %v, want ErrInvariantViolation", err)
	}

	// Non-mutable fields are ignored by the repository contract.
	resolved.StartsAt = time.Time{}
	var updated domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var uerr error
		updated, uerr = uow.Alerts().UpdateEventResolution(ctx, resolved)
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

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		repeated, uerr := uow.Alerts().UpdateEventResolution(ctx, resolved)
		if uerr != nil {
			t.Fatalf("repeated UpdateEventResolution: %v", uerr)
		}
		if repeated.EndsAt == nil || !repeated.EndsAt.Equal(endsAt) {
			t.Fatalf("repeated EndsAt = %v, want %v", repeated.EndsAt, endsAt)
		}
	})

	conflicting, err := saved.Resolve(endsAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("Resolve conflicting candidate: %v", err)
	}
	err = integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, updateErr := uow.Alerts().UpdateEventResolution(ctx, conflicting)
		return updateErr
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("conflicting UpdateEventResolution err = %v, want ErrInvariantViolation", err)
	}
	if updated.EndsAt == nil || !updated.EndsAt.Equal(endsAt) {
		t.Fatalf("initial updated EndsAt = %v, want %v", updated.EndsAt, endsAt)
	}
}

func TestAlertRepository_UpdateEventResolutionConcurrentConflict(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	var saved domain.AlertEvent
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		saved, err = uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prometheus", "fp-race", "canon-race", startsAt))
		if err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
	})

	type updateResult struct {
		endsAt time.Time
		err    error
	}
	results := make(chan updateResult, 2)
	start := make(chan struct{})
	for _, endsAt := range []time.Time{startsAt.Add(time.Minute), startsAt.Add(2 * time.Minute)} {
		candidate, err := saved.Resolve(endsAt)
		if err != nil {
			t.Fatalf("Resolve candidate: %v", err)
		}
		go func() {
			<-start
			updateErr := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
				_, err := uow.Alerts().UpdateEventResolution(ctx, candidate)
				return err
			})
			results <- updateResult{endsAt: endsAt, err: updateErr}
		}()
	}
	close(start)

	var winningEnd time.Time
	var successes, conflicts int
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			winningEnd = result.endsAt
		case errors.Is(result.err, domain.ErrInvariantViolation):
			conflicts++
		default:
			t.Fatalf("concurrent UpdateEventResolution err = %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		stored, err := uow.Alerts().FindEventByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindEventByID: %v", err)
		}
		if stored.EndsAt == nil || !stored.EndsAt.Equal(winningEnd) {
			t.Fatalf("stored EndsAt = %v, want winning %s", stored.EndsAt, winningEnd)
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
		ids, err := uow.Alerts().ListEventIDsForGroup(ctx, groupID, 10)
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
		limited, lerr := uow.Alerts().ListEventIDsForGroup(ctx, groupID, 1)
		if lerr != nil {
			t.Fatalf("ListEventIDsForGroup limit 1: %v", lerr)
		}
		if len(limited) != 1 || limited[0] != eventIDs[1] {
			t.Fatalf("ListEventIDsForGroup limit 1 = %v, want [%d]", limited, eventIDs[1])
		}
	})
}

func TestAlertRepository_FindGroupByEventIDAndGroupKeyReturnsEarliestLinkedGroup(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)

	var eventID domain.AlertEventID
	var earliestGroupID domain.AlertGroupID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		event, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prom", "fp-linked", "canon-linked", startsAt))
		if err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
		eventID = event.ID

		earliest, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "linked-key", startsAt, startsAt))
		if err != nil {
			t.Fatalf("SaveGroup earliest: %v", err)
		}
		earliestGroupID = earliest.ID
		later, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "linked-key", startsAt.Add(time.Hour), startsAt.Add(time.Hour)))
		if err != nil {
			t.Fatalf("SaveGroup later: %v", err)
		}
		otherKey, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "other-key", startsAt, startsAt))
		if err != nil {
			t.Fatalf("SaveGroup other-key: %v", err)
		}
		for _, groupID := range []domain.AlertGroupID{later.ID, earliest.ID, otherKey.ID} {
			if err := uow.Alerts().LinkEventsToGroup(ctx, groupID, []domain.AlertEventID{eventID}); err != nil {
				t.Fatalf("LinkEventsToGroup %d: %v", groupID, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().FindGroupByEventIDAndGroupKey(ctx, eventID, "linked-key")
		if err != nil {
			t.Fatalf("FindGroupByEventIDAndGroupKey: %v", err)
		}
		if got.ID != earliestGroupID {
			t.Fatalf("got.ID = %d, want earliest group %d", got.ID, earliestGroupID)
		}
		_, err = uow.Alerts().FindGroupByEventIDAndGroupKey(ctx, eventID, "missing-key")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("wrong key: want ErrNotFound, got %v", err)
		}
	})
}

func TestAlertRepository_ListEventsForGroupByStartsAtRangeFilteredScopesLinkedEvents(t *testing.T) {
	resetDB(t)
	startsAt := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC)

	var groupID domain.AlertGroupID
	var wantID domain.AlertEventID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		group, err := uow.Alerts().SaveGroup(ctx, mustNewAlertGroup(t, "scope-key", startsAt, startsAt.Add(2*time.Hour)))
		if err != nil {
			t.Fatalf("SaveGroup: %v", err)
		}
		groupID = group.ID

		prom, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prometheus", "fp-prom", "canon-prom", startsAt))
		if err != nil {
			t.Fatalf("SaveEvent prom: %v", err)
		}
		wantID = prom.ID
		alertmanager, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "alertmanager", "fp-am", "canon-am", startsAt.Add(time.Minute)))
		if err != nil {
			t.Fatalf("SaveEvent alertmanager: %v", err)
		}
		outside, err := uow.Alerts().SaveEvent(ctx, mustNewAlertEvent(t, "prometheus", "fp-out", "canon-out", startsAt.Add(2*time.Hour)))
		if err != nil {
			t.Fatalf("SaveEvent outside: %v", err)
		}
		if err := uow.Alerts().LinkEventsToGroup(ctx, groupID, []domain.AlertEventID{alertmanager.ID, outside.ID, prom.ID}); err != nil {
			t.Fatalf("LinkEventsToGroup: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Alerts().ListEventsForGroupByStartsAtRangeFiltered(
			ctx,
			groupID,
			startsAt,
			startsAt.Add(time.Hour),
			ports.AlertEventFilter{Sources: []string{"prometheus"}},
			10,
		)
		if err != nil {
			t.Fatalf("ListEventsForGroupByStartsAtRangeFiltered: %v", err)
		}
		if len(got) != 1 || got[0].ID != wantID {
			t.Fatalf("got event IDs = %v, want only %d", alertEventIDsForTest(got), wantID)
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
		_, err := uow.Alerts().ListEventIDsForGroup(ctx, 999999, 10)
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
		ids, err := uow.Alerts().ListEventIDsForGroup(ctx, groupID, 10)
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
