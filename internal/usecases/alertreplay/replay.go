// Package alertreplay drives the M1 "alert window replay harness":
// it queries the upstream MetricsProvider once, persists the result
// as AlertEvents (delegating to alertingest.IngestOnce), then re-reads
// the half-open window [WindowStart, WindowEnd) from the database and
// runs grouping + evidence-snapshot building over that slice.
//
// ReplayWindow is an application/usecase orchestrator: it depends on
// MetricsProvider and UnitOfWorkFactory ports, but never on the
// generated Ent client, pgx, or the Temporal SDK.
//
// # Closed -> refresh tension (D7)
//
// AlertGroup uses the natural unique key (group_key, first_seen_at).
// "Closed" is documented in [internal/domain/alert.go] as "sealed",
// but the replay harness intentionally allows updating mutable
// fields (severity, event_count, last_seen_at) of an already-closed
// group when the same window converges on the same natural key.
// The interpretation is: closed means "an EvidenceSnapshot has been
// produced", not "no further state may change". A later, larger
// window that produces a different digest yields a NEW snapshot row;
// the group header is refreshed in place.
//
// Refresh is strictly bounded: it only applies when the replayed
// window produces the SAME (group_key, first_seen_at) natural key as
// an existing row. A later, narrower window whose first_seen_at
// differs is a NEW AlertGroup row by design. This boundary is what
// keeps refresh from blurring into reopen.
//
// Status is intentionally not reopened: refresh does not transition
// closed back to active. GroupsClosed is incremented only when this
// run sealed an active group for the first time.
package alertreplay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/evidencebuild"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Request describes one replay invocation.
//
// WindowStart is inclusive, WindowEnd is exclusive: AlertEvents whose
// StartsAt falls in [WindowStart, WindowEnd) participate. Both bounds
// are normalised to UTC microseconds via domain.NormalizeUTCMicro
// before comparison; the half-open shape avoids counting boundary
// events twice when adjacent windows are replayed.
//
// Grouping is forwarded verbatim to alertgrouping.GroupEvents, which
// validates it lazily: an empty config is rejected only if there is
// at least one in-window event to group (PR5 fast path).
//
// Limit caps the number of events Step 2 reads from the database.
// It is a safety valve, not a silent truncation: the replay loads
// Limit+1 rows and fails the entire run with
// domain.ErrInvariantViolation if the cap was exceeded so callers
// cannot mistake a partial result for a complete one. Limit must be
// strictly between 0 and math.MaxInt to keep Limit+1 from
// overflowing.
type Request struct {
	WindowStart       time.Time
	WindowEnd         time.Time
	Grouping          alertgrouping.Config
	CreatedByWorkflow string
	Limit             int
}

// Result is the replay output needed by downstream report dispatch.
// Stats keeps the existing counter-only summary; Snapshots records the
// persisted EvidenceSnapshot rows in deterministic group order.
type Result struct {
	Stats     Stats
	Snapshots []SnapshotRef
}

// SnapshotRef identifies one snapshot produced or found by replay.
type SnapshotRef struct {
	ID         domain.EvidenceSnapshotID
	GroupIndex int
	EventCount int
}

// Stats summarises one ReplayWindow invocation. The intent is that
// every counter answers a different question and downstream callers
// (CLI, Temporal, future HTTP) can render a self-explanatory report
// without re-deriving anything.
//
// The invariant
//
//	GroupsBuilt == GroupsSaved + GroupsRefreshed + GroupsExisting + Failed
//
// holds when GroupsBuilt > 0 and the safety valve did not trip.
// Snapshot counters track an orthogonal axis (a refreshed group can
// produce either a new or a duplicate snapshot row) so they are not
// included in the equality.
type Stats struct {
	// Ingested is alertingest.IngestOnce's report. It includes
	// out-of-window alerts because IngestOnce never filters by
	// the replay window.
	Ingested alertingest.Stats

	// EventsLoaded is len(events) returned by Step 2's window
	// query. It is set BEFORE the safety valve check so a
	// caller can see why ReplayWindow failed.
	EventsLoaded int

	// GroupsBuilt is len(GroupEvents output) for this window.
	GroupsBuilt int

	// GroupsSaved counts groups whose natural key was not yet
	// in the database -> SaveGroup succeeded.
	GroupsSaved int

	// GroupsRefreshed counts groups whose natural key already
	// existed and at least one of (severity, event_count,
	// last_seen_at) differed -> UpdateGroup ran.
	GroupsRefreshed int

	// GroupsExisting counts groups whose natural key already
	// existed and no mutable field differed -> no-op.
	GroupsExisting int

	// SnapshotsSaved counts new EvidenceSnapshot rows written
	// in this run.
	SnapshotsSaved int

	// SnapshotsDuplicate counts EvidenceSnapshot rows that
	// already existed for the same (group_id, digest).
	SnapshotsDuplicate int

	// GroupsClosed counts groups that this run transitioned
	// from active to closed. Closing an already-closed group
	// is a no-op and not counted here.
	GroupsClosed int

	// Failed counts per-group pipeline transactions that rolled
	// back. It does NOT count Step 1/2/3 failures, which short-
	// circuit the whole run.
	Failed int
}

// groupResult holds the boolean outcome of one processGroup call.
// It is filled inside the per-group transaction and merged into
// Stats only after WithinTx commits successfully; this keeps Stats
// immune to half-applied tx state.
type groupResult struct {
	savedGroup        bool
	refreshedGroup    bool
	existingGroup     bool
	savedSnapshot     bool
	duplicateSnapshot bool
	closedGroup       bool
	snapshotID        domain.EvidenceSnapshotID
}

// ReplayWindow runs one end-to-end replay over the configured
// window. See package doc for the closed->refresh interpretation.
//
// Steps:
//
//  1. IngestOnce(provider, factory) so any newly-firing alerts are
//     persisted before the window read.
//  2. Short read tx: ListEventsByStartsAtRange(WindowStart,
//     WindowEnd, Limit+1). EventsLoaded is set to len(events)
//     unconditionally so a tripped safety valve still leaves a
//     diagnosable Stats.
//  3. GroupEvents(events, Grouping). Empty events short-circuits
//     here (config is NOT consumed -- PR5 fast path).
//  4. Reconstruct each draft's events slice from a local
//     map[AlertEventID]AlertEvent. A missing id is treated as an
//     invariant violation here so the orchestration layer fails
//     with a precise error instead of letting BuildSnapshot
//     report a count mismatch downstream.
//  5. processGroup runs in its own transaction per draft. Per-
//     group failures are collected and joined; they do NOT abort
//     the rest of the run.
//
// Returns (stats, nil) when every per-group tx committed; otherwise
// returns (stats, errors.Join(...)) so callers see every reason.
func ReplayWindow(
	ctx context.Context,
	provider ports.MetricsProvider,
	factory ports.UnitOfWorkFactory,
	req Request,
) (Stats, error) {
	result, err := ReplayWindowForReport(ctx, provider, factory, req)
	return result.Stats, err
}

// ReplayWindowForReport runs ReplayWindow and also returns the
// persisted EvidenceSnapshot identities needed to start report
// generation. It is intentionally separate from ReplayWindow so
// existing counter-only callers keep comparable Stats values.
func ReplayWindowForReport(
	ctx context.Context,
	provider ports.MetricsProvider,
	factory ports.UnitOfWorkFactory,
	req Request,
) (Result, error) {
	var result Result
	if err := validateRequest(provider, factory, req); err != nil {
		return result, err
	}

	// Step 1: ingest.
	ingested, err := alertingest.IngestOnce(ctx, provider, factory)
	result.Stats.Ingested = ingested
	if err != nil {
		return result, fmt.Errorf("alertreplay: ingest: %w", err)
	}

	// Step 2: short read tx for the window.
	var events []domain.AlertEvent
	nStart := domain.NormalizeUTCMicro(req.WindowStart)
	nEnd := domain.NormalizeUTCMicro(req.WindowEnd)
	if err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		rows, lerr := uow.Alerts().ListEventsByStartsAtRange(ctx, nStart, nEnd, req.Limit+1)
		if lerr != nil {
			return lerr
		}
		events = rows
		return nil
	}); err != nil {
		return result, fmt.Errorf("alertreplay: list events by window: %w", err)
	}
	result.Stats.EventsLoaded = len(events)
	if len(events) > req.Limit {
		return result, fmt.Errorf(
			"alertreplay: window contains more than limit (%d) events: %w",
			req.Limit, domain.ErrInvariantViolation,
		)
	}

	// Step 3: deterministic grouping.
	drafts, err := alertgrouping.GroupEvents(events, req.Grouping)
	if err != nil {
		return result, fmt.Errorf("alertreplay: group events: %w", err)
	}
	result.Stats.GroupsBuilt = len(drafts)
	if len(drafts) == 0 {
		return result, nil
	}

	// Step 4: reconstruct per-draft events.
	eventByID := make(map[domain.AlertEventID]domain.AlertEvent, len(events))
	for i := range events {
		eventByID[events[i].ID] = events[i]
	}

	// Step 5: per-group pipeline.
	var failures []error
	for i := range drafts {
		draft := drafts[i]

		eventsForGroup := make([]domain.AlertEvent, 0, len(draft.EventIDs))
		for _, id := range draft.EventIDs {
			ev, ok := eventByID[id]
			if !ok {
				return result, fmt.Errorf(
					"alertreplay: draft group_key=%q references event id %d not present in window: %w",
					draft.GroupKey, id, domain.ErrInvariantViolation,
				)
			}
			eventsForGroup = append(eventsForGroup, ev)
		}

		groupOutcome, perr := processGroup(ctx, factory, draft, eventsForGroup, req.CreatedByWorkflow)
		if perr != nil {
			result.Stats.Failed++
			slog.WarnContext(ctx, "alertreplay: per-group pipeline failed",
				slog.String("group_key", draft.GroupKey),
				slog.Any("error", perr),
			)
			failures = append(failures, perr)
			continue
		}

		mergeGroupResult(&result.Stats, groupOutcome)
		if groupOutcome.snapshotID != 0 {
			result.Snapshots = append(result.Snapshots, SnapshotRef{
				ID:         groupOutcome.snapshotID,
				GroupIndex: i,
				EventCount: len(draft.EventIDs),
			})
		}
	}

	if len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}

// validateRequest enforces orchestration-level invariants. Each
// branch wraps domain.ErrInvariantViolation so callers can pattern-
// match without parsing the message. Range comparison happens after
// normalisation so a sub-microsecond gap that the normaliser erases
// does not slip past as a "valid" window.
func validateRequest(provider ports.MetricsProvider, factory ports.UnitOfWorkFactory, req Request) error {
	if provider == nil {
		return fmt.Errorf("alertreplay: provider must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if factory == nil {
		return fmt.Errorf("alertreplay: factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if req.WindowStart.IsZero() {
		return fmt.Errorf("alertreplay: WindowStart must be set: %w", domain.ErrInvariantViolation)
	}
	if req.WindowEnd.IsZero() {
		return fmt.Errorf("alertreplay: WindowEnd must be set: %w", domain.ErrInvariantViolation)
	}
	nStart := domain.NormalizeUTCMicro(req.WindowStart)
	nEnd := domain.NormalizeUTCMicro(req.WindowEnd)
	if !nEnd.After(nStart) {
		return fmt.Errorf(
			"alertreplay: WindowEnd %s must be strictly after WindowStart %s after normalisation: %w",
			nEnd, nStart, domain.ErrInvariantViolation,
		)
	}
	if req.Limit <= 0 {
		return fmt.Errorf("alertreplay: Limit %d must be > 0: %w", req.Limit, domain.ErrInvariantViolation)
	}
	if req.Limit >= math.MaxInt {
		// Limit+1 would overflow; the safety valve relies on a
		// well-defined upper bound.
		return fmt.Errorf("alertreplay: Limit %d must be < math.MaxInt: %w", req.Limit, domain.ErrInvariantViolation)
	}
	return nil
}

// processGroup runs the per-draft pipeline inside its own
// transaction. Mutating Stats from the callback would leak partial
// state if the tx aborts, so the result is returned verbatim and
// merged by the caller only on commit.
//
// The transaction must NOT swallow a SQLSTATE 23505 (unique
// violation): in Postgres an aborted transaction cannot run further
// queries until rollback. FindGroupByNaturalKey is therefore the
// pre-check that decides which write path to take, and a duplicate
// from a concurrent writer is propagated verbatim so WithinTx rolls
// back; the outer loop counts that group as Failed.
func processGroup(
	ctx context.Context,
	factory ports.UnitOfWorkFactory,
	draft domain.AlertGroup,
	eventsForGroup []domain.AlertEvent,
	createdByWorkflow string,
) (groupResult, error) {
	var result groupResult

	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		alerts := uow.Alerts()
		evidence := uow.Evidence()

		// Pre-check: decide saved vs refreshed vs existing.
		existing, ferr := alerts.FindGroupByNaturalKey(ctx, draft.GroupKey, draft.FirstSeenAt)
		var persisted domain.AlertGroup
		switch {
		case ferr == nil:
			// Existing row: diff mutable fields.
			if mutableFieldsDiffer(existing, draft) {
				merged := existing
				merged.Severity = draft.Severity
				merged.EventCount = draft.EventCount
				merged.LastSeenAt = domain.NormalizeUTCMicro(draft.LastSeenAt)
				// Status intentionally preserved: refresh
				// does NOT reopen a closed group.
				updated, uerr := alerts.UpdateGroup(ctx, merged)
				if uerr != nil {
					return fmt.Errorf("update existing group: %w", uerr)
				}
				persisted = updated
				result.refreshedGroup = true
			} else {
				persisted = existing
				result.existingGroup = true
			}
		case errors.Is(ferr, domain.ErrNotFound):
			saved, serr := alerts.SaveGroup(ctx, draft)
			if serr != nil {
				return fmt.Errorf("save group: %w", serr)
			}
			persisted = saved
			result.savedGroup = true
		default:
			return fmt.Errorf("find group by natural key: %w", ferr)
		}

		// Materialise the M2N edge. LinkEventsToGroup is
		// idempotent on (group, event) pairs.
		eventIDs := make([]domain.AlertEventID, len(eventsForGroup))
		for i := range eventsForGroup {
			eventIDs[i] = eventsForGroup[i].ID
		}
		if err := alerts.LinkEventsToGroup(ctx, persisted.ID, eventIDs); err != nil {
			return fmt.Errorf("link events to group: %w", err)
		}

		// Snapshot input: the persisted group does NOT carry
		// EventIDs (the mapper does not materialise the M2N).
		// Re-attach the draft's EventIDs so BuildSnapshot's
		// group-vs-events ID set check runs.
		snapshotGroup := persisted
		snapshotGroup.EventIDs = draft.EventIDs

		snapshot, berr := evidencebuild.BuildSnapshot(evidencebuild.Input{
			Group:             snapshotGroup,
			Events:            eventsForGroup,
			CreatedByWorkflow: createdByWorkflow,
		})
		if berr != nil {
			return fmt.Errorf("build snapshot: %w", berr)
		}

		// Decide saved vs duplicate via the per-group
		// idempotency key (group_id, digest).
		existingSnapshot, lookupErr := evidence.FindByGroupAndDigest(ctx, snapshotGroup.ID, snapshot.Digest)
		switch {
		case lookupErr == nil:
			result.snapshotID = existingSnapshot.ID
			result.duplicateSnapshot = true
		case errors.Is(lookupErr, domain.ErrNotFound):
			savedSnapshot, sErr := evidence.Save(ctx, snapshot)
			if sErr != nil {
				return fmt.Errorf("save snapshot: %w", sErr)
			}
			result.snapshotID = savedSnapshot.ID
			result.savedSnapshot = true
		default:
			return fmt.Errorf("find snapshot by digest: %w", lookupErr)
		}

		// Close only on first transition active -> closed.
		if persisted.Status == domain.AlertGroupStatusActive {
			closed := persisted.Close()
			if _, cErr := alerts.UpdateGroup(ctx, closed); cErr != nil {
				return fmt.Errorf("close group: %w", cErr)
			}
			result.closedGroup = true
		}

		return nil
	})

	if err != nil {
		return groupResult{}, err
	}
	return result, nil
}

// mutableFieldsDiffer reports whether any of (Severity, EventCount,
// LastSeenAt) differ between the persisted row and a freshly built
// draft. LastSeenAt is normalised on both sides so a draft that
// merely re-derives the same timestamp at higher precision is not
// counted as a diff.
func mutableFieldsDiffer(existing, draft domain.AlertGroup) bool {
	if existing.Severity != draft.Severity {
		return true
	}
	if existing.EventCount != draft.EventCount {
		return true
	}
	if !domain.NormalizeUTCMicro(existing.LastSeenAt).Equal(domain.NormalizeUTCMicro(draft.LastSeenAt)) {
		return true
	}
	return false
}

// mergeGroupResult folds a per-group result into the Stats counters.
// Centralising the merge keeps the invariant
// (GroupsBuilt == saved + refreshed + existing + Failed) close to
// where the counters are updated.
func mergeGroupResult(stats *Stats, r groupResult) {
	switch {
	case r.savedGroup:
		stats.GroupsSaved++
	case r.refreshedGroup:
		stats.GroupsRefreshed++
	case r.existingGroup:
		stats.GroupsExisting++
	}
	if r.savedSnapshot {
		stats.SnapshotsSaved++
	}
	if r.duplicateSnapshot {
		stats.SnapshotsDuplicate++
	}
	if r.closedGroup {
		stats.GroupsClosed++
	}
}
