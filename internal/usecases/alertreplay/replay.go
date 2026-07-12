// Package alertreplay drives the M1 "alert window replay harness":
// it queries the upstream ActiveAlertProvider once, persists the result
// as AlertEvents (delegating to alertingest.IngestOnce), then re-reads
// the half-open window [WindowStart, WindowEnd) from the database and
// runs grouping + evidence-snapshot building over that slice.
//
// ReplayWindow is an application/usecase orchestrator: it depends on
// ActiveAlertProvider and UnitOfWorkFactory ports, but never on the
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
// Refresh first tries the SAME (group_key, first_seen_at) natural key
// as an existing row. If an ID-scoped replay contains a later event
// that is already linked to a row with the same group_key, replay
// reuses that linked row instead of creating a duplicate group. The
// expansion still honours the current replay window/source/profile
// scope; AlertEventIDFilter selects the triggering/current events but
// does not shrink an already-materialised group by itself.
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
	"sort"
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
	WindowStart              time.Time
	WindowEnd                time.Time
	Grouping                 alertgrouping.Config
	AlertEventIDFilter       []domain.AlertEventID
	SourceFilter             []string
	AlertSourceProfileFilter []domain.AlertSourceProfileID
	CreatedByWorkflow        string
	// CMDBProvider is optional. When configured, replay looks up each grouped
	// event's labels before opening the per-group transaction and embeds any
	// matches in the EvidenceSnapshot payload.
	CMDBProvider ports.CMDBProvider
	Limit        int
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

	// EventsLoaded is len(events) returned by Step 2's filtered window query.
	// It is set BEFORE the safety valve check so a caller can see why
	// ReplayWindow failed.
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
	savedGroup         bool
	refreshedGroup     bool
	existingGroup      bool
	savedSnapshot      bool
	duplicateSnapshot  bool
	closedGroup        bool
	snapshotID         domain.EvidenceSnapshotID
	snapshotEventCount int
}

type replayExpansionScope struct {
	window                 domain.AlertWindow
	filter                 ports.AlertEventFilter
	grouping               alertgrouping.Config
	limit                  int
	allowLinkedGroupLookup bool
}

// ReplayWindow runs one end-to-end replay over the configured
// window. See package doc for the closed->refresh interpretation.
//
// Steps:
//
//  1. IngestOnce(provider, factory) so any newly-firing alerts are
//     persisted before the window read.
//  2. Short read tx: ListEventsByStartsAtRangeFiltered(WindowStart,
//     WindowEnd, filters, Limit+1). EventsLoaded is set to len(events)
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
	provider ports.ActiveAlertProvider,
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
	provider ports.ActiveAlertProvider,
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

	persisted, err := ReplayPersistedWindowForReport(ctx, factory, req)
	persisted.Stats.Ingested = ingested
	return persisted, err
}

// ReplayPersistedWindowForReport runs the grouping and evidence-snapshot
// stages over alerts that have already been persisted. Push-style intake paths
// such as Alertmanager webhooks use this after ingesting their request body so
// they do not need to construct a provider only to read back local rows.
func ReplayPersistedWindowForReport(
	ctx context.Context,
	factory ports.UnitOfWorkFactory,
	req Request,
) (Result, error) {
	var result Result
	window, err := validatePersistedWindowRequest(factory, req)
	if err != nil {
		return result, err
	}

	// Step 1 for persisted windows: short read tx for the filtered window.
	var events []domain.AlertEvent
	if err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		rows, lerr := uow.Alerts().ListEventsByStartsAtRangeFiltered(
			ctx,
			window.StartInclusive(),
			window.EndExclusive(),
			ports.AlertEventFilter{
				IDs:                   req.AlertEventIDFilter,
				Sources:               req.SourceFilter,
				AlertSourceProfileIDs: req.AlertSourceProfileFilter,
			},
			req.Limit+1,
		)
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

	matchedEvents := events

	// Step 2: deterministic grouping.
	drafts, err := alertgrouping.GroupEvents(matchedEvents, req.Grouping)
	if err != nil {
		return result, fmt.Errorf("alertreplay: group events: %w", err)
	}
	result.Stats.GroupsBuilt = len(drafts)
	if len(drafts) == 0 {
		return result, nil
	}

	// Step 3: reconstruct per-draft events.
	eventByID := make(map[domain.AlertEventID]domain.AlertEvent, len(matchedEvents))
	for i := range matchedEvents {
		eventByID[matchedEvents[i].ID] = matchedEvents[i]
	}

	expansionScope := replayExpansionScope{
		window: window,
		filter: ports.AlertEventFilter{
			Sources:               req.SourceFilter,
			AlertSourceProfileIDs: req.AlertSourceProfileFilter,
		},
		grouping:               req.Grouping,
		limit:                  req.Limit,
		allowLinkedGroupLookup: len(req.AlertEventIDFilter) > 0,
	}

	// Step 4: per-group pipeline.
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

		eventsForCMDB := eventsForGroup
		if req.CMDBProvider != nil {
			expanded, xerr := snapshotEventsForCMDBLookup(ctx, factory, draft, eventsForGroup, expansionScope)
			if xerr != nil {
				result.Stats.Failed++
				slog.WarnContext(ctx, "alertreplay: cmdb enrichment failed",
					slog.String("group_key", draft.GroupKey),
					slog.Any("error", xerr),
				)
				failures = append(failures, xerr)
				continue
			}
			eventsForCMDB = expanded
		}
		cmdbMatches, cerr := lookupCMDBMatches(ctx, req.CMDBProvider, eventsForCMDB)
		if cerr != nil {
			result.Stats.Failed++
			slog.WarnContext(ctx, "alertreplay: cmdb enrichment failed",
				slog.String("group_key", draft.GroupKey),
				slog.Any("error", cerr),
			)
			failures = append(failures, cerr)
			continue
		}

		groupOutcome, perr := processGroup(ctx, factory, draft, eventsForGroup, req.CreatedByWorkflow, cmdbMatches, expansionScope)
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
				EventCount: groupOutcome.snapshotEventCount,
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
func validateRequest(provider ports.ActiveAlertProvider, factory ports.UnitOfWorkFactory, req Request) error {
	if provider == nil {
		return fmt.Errorf("alertreplay: provider must be non-nil: %w", domain.ErrInvariantViolation)
	}
	_, err := validatePersistedWindowRequest(factory, req)
	return err
}

func validatePersistedWindowRequest(factory ports.UnitOfWorkFactory, req Request) (domain.AlertWindow, error) {
	if factory == nil {
		return domain.AlertWindow{}, fmt.Errorf("alertreplay: factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	window, err := domain.NewAlertWindow(req.WindowStart, req.WindowEnd)
	if err != nil {
		return domain.AlertWindow{}, fmt.Errorf("alertreplay: replay window: %w", err)
	}
	if req.Limit <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("alertreplay: Limit %d must be > 0: %w", req.Limit, domain.ErrInvariantViolation)
	}
	if req.Limit >= math.MaxInt {
		// Limit+1 would overflow; the safety valve relies on a
		// well-defined upper bound.
		return domain.AlertWindow{}, fmt.Errorf("alertreplay: Limit %d must be < math.MaxInt: %w", req.Limit, domain.ErrInvariantViolation)
	}
	for _, id := range req.AlertEventIDFilter {
		if id <= 0 {
			return domain.AlertWindow{}, fmt.Errorf("alertreplay: alert event id filter contains non-positive id %d: %w", id, domain.ErrInvariantViolation)
		}
	}
	for _, id := range req.AlertSourceProfileFilter {
		if id < 0 {
			return domain.AlertWindow{}, fmt.Errorf("alertreplay: alert source profile filter contains negative id %d: %w", id, domain.ErrInvariantViolation)
		}
	}
	return window, nil
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
	cmdbMatches []evidencebuild.CMDBMatch,
	scope replayExpansionScope,
) (groupResult, error) {
	var result groupResult

	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		alerts := uow.Alerts()
		evidence := uow.Evidence()

		// Pre-check: decide saved vs refreshed vs existing.
		eventsForSnapshot := eventsForGroup
		draftForPersistence := draft
		existing, foundExisting, ferr := findExistingGroupForDraft(ctx, alerts, draft, eventsForGroup, scope.allowLinkedGroupLookup)
		var persisted domain.AlertGroup
		switch {
		case ferr != nil:
			return ferr
		case foundExisting:
			expanded, xerr := existingGroupSnapshotEvents(ctx, alerts, existing.ID, eventsForGroup, scope)
			if xerr != nil {
				return fmt.Errorf("load existing group events: %w", xerr)
			}
			eventsForSnapshot = expanded
			mergedDraft, merr := mergeDraftWithExistingGroup(existing, draft, eventsForSnapshot, scope.grouping)
			if merr != nil {
				return fmt.Errorf("merge existing group draft: %w", merr)
			}
			draftForPersistence = mergedDraft
			// Existing row: diff mutable fields.
			if mutableFieldsDiffer(existing, draftForPersistence) {
				merged := existing
				merged.Severity = draftForPersistence.Severity
				merged.EventCount = draftForPersistence.EventCount
				merged.LastSeenAt = domain.NormalizeUTCMicro(draftForPersistence.LastSeenAt)
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
		default:
			saved, serr := alerts.SaveGroup(ctx, draft)
			if serr != nil {
				return fmt.Errorf("save group: %w", serr)
			}
			persisted = saved
			result.savedGroup = true
		}

		// Materialise the M2N edge. LinkEventsToGroup is
		// idempotent on (group, event) pairs.
		eventIDs := make([]domain.AlertEventID, len(eventsForSnapshot))
		for i := range eventsForSnapshot {
			eventIDs[i] = eventsForSnapshot[i].ID
		}
		if err := alerts.LinkEventsToGroup(ctx, persisted.ID, eventIDs); err != nil {
			return fmt.Errorf("link events to group: %w", err)
		}

		// Snapshot input: the persisted group does NOT carry
		// EventIDs (the mapper does not materialise the M2N).
		// Re-attach the draft's EventIDs so BuildSnapshot's
		// group-vs-events ID set check runs.
		snapshotGroup := persisted
		snapshotGroup.EventIDs = eventIDs

		snapshot, berr := evidencebuild.BuildSnapshot(evidencebuild.Input{
			Group:             snapshotGroup,
			Events:            eventsForSnapshot,
			CreatedByWorkflow: createdByWorkflow,
			CMDBMatches:       cmdbMatches,
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
			result.snapshotEventCount = len(eventsForSnapshot)
			result.duplicateSnapshot = true
		case errors.Is(lookupErr, domain.ErrNotFound):
			savedSnapshot, sErr := evidence.Save(ctx, snapshot)
			if sErr != nil {
				return fmt.Errorf("save snapshot: %w", sErr)
			}
			result.snapshotID = savedSnapshot.ID
			result.snapshotEventCount = len(eventsForSnapshot)
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

func snapshotEventsForCMDBLookup(
	ctx context.Context,
	factory ports.UnitOfWorkFactory,
	draft domain.AlertGroup,
	eventsForGroup []domain.AlertEvent,
	scope replayExpansionScope,
) ([]domain.AlertEvent, error) {
	eventsForLookup := eventsForGroup
	if err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		alerts := uow.Alerts()
		existing, foundExisting, err := findExistingGroupForDraft(ctx, alerts, draft, eventsForGroup, scope.allowLinkedGroupLookup)
		if err != nil {
			return err
		}
		if !foundExisting {
			return nil
		}
		expanded, err := existingGroupSnapshotEvents(ctx, alerts, existing.ID, eventsForGroup, scope)
		if err != nil {
			return fmt.Errorf("load existing group events for cmdb lookup: %w", err)
		}
		eventsForLookup = expanded
		return nil
	}); err != nil {
		return nil, fmt.Errorf("alertreplay: prepare cmdb lookup events: %w", err)
	}
	return eventsForLookup, nil
}

func lookupCMDBMatches(
	ctx context.Context,
	provider ports.CMDBProvider,
	events []domain.AlertEvent,
) ([]evidencebuild.CMDBMatch, error) {
	if provider == nil {
		return nil, nil
	}
	sorted := append([]domain.AlertEvent(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})
	matches := make([]evidencebuild.CMDBMatch, 0, len(sorted))
	for i := range sorted {
		event := sorted[i]
		result, err := provider.LookupResource(ctx, ports.CMDBLookupRequest{
			Labels: cloneStringMap(event.Labels),
		})
		if err != nil {
			return nil, fmt.Errorf("alertreplay: cmdb lookup for event %d: %w", event.ID, err)
		}
		if !result.Found {
			continue
		}
		matches = append(matches, evidencebuild.CMDBMatch{
			EventID:  event.ID,
			Resource: result.Resource,
		})
	}
	return matches, nil
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

func findExistingGroupForDraft(
	ctx context.Context,
	alerts ports.AlertRepository,
	draft domain.AlertGroup,
	eventsForGroup []domain.AlertEvent,
	allowLinkedGroupLookup bool,
) (domain.AlertGroup, bool, error) {
	existing, err := alerts.FindGroupByNaturalKey(ctx, draft.GroupKey, draft.FirstSeenAt)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.AlertGroup{}, false, fmt.Errorf("find group by natural key: %w", err)
	}
	if !allowLinkedGroupLookup {
		return domain.AlertGroup{}, false, nil
	}
	existing, err = findExistingGroupByLinkedEvents(ctx, alerts, draft.GroupKey, eventsForGroup)
	if err == nil {
		return existing, true, nil
	}
	if errors.Is(err, domain.ErrNotFound) {
		return domain.AlertGroup{}, false, nil
	}
	return domain.AlertGroup{}, false, err
}

func findExistingGroupByLinkedEvents(
	ctx context.Context,
	alerts ports.AlertRepository,
	groupKey string,
	eventsForGroup []domain.AlertEvent,
) (domain.AlertGroup, error) {
	var found domain.AlertGroup
	for _, event := range eventsForGroup {
		if event.ID <= 0 {
			return domain.AlertGroup{}, fmt.Errorf("current alert event id %d must be positive: %w", event.ID, domain.ErrInvariantViolation)
		}
		candidate, err := alerts.FindGroupByEventIDAndGroupKey(ctx, event.ID, groupKey)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return domain.AlertGroup{}, fmt.Errorf("find group by linked alert event %d: %w", event.ID, err)
		}
		if found.ID == 0 {
			found = candidate
			continue
		}
		if candidate.ID != found.ID {
			return domain.AlertGroup{}, fmt.Errorf(
				"draft group_key=%q maps to multiple existing groups (%d and %d): %w",
				groupKey, found.ID, candidate.ID, domain.ErrInvariantViolation,
			)
		}
	}
	if found.ID == 0 {
		return domain.AlertGroup{}, fmt.Errorf("find group by linked events: %w", domain.ErrNotFound)
	}
	return found, nil
}

func existingGroupSnapshotEvents(
	ctx context.Context,
	alerts ports.AlertRepository,
	groupID domain.AlertGroupID,
	current []domain.AlertEvent,
	scope replayExpansionScope,
) ([]domain.AlertEvent, error) {
	byID := make(map[domain.AlertEventID]domain.AlertEvent, len(current))
	seenIDs := make(map[domain.AlertEventID]struct{}, len(current))
	for _, event := range current {
		if event.ID <= 0 {
			return nil, fmt.Errorf("current alert event id %d must be positive: %w", event.ID, domain.ErrInvariantViolation)
		}
		byID[event.ID] = event
		seenIDs[event.ID] = struct{}{}
	}
	if len(seenIDs) > scope.limit {
		return nil, fmt.Errorf(
			"existing alert group %d expansion contains more than limit (%d) events: %w",
			groupID, scope.limit, domain.ErrInvariantViolation,
		)
	}
	linkedEvents, err := alerts.ListEventsForGroupByStartsAtRangeFiltered(
		ctx,
		groupID,
		scope.window.StartInclusive(),
		scope.window.EndExclusive(),
		scope.filter,
		scope.limit+1,
	)
	if err != nil {
		return nil, err
	}
	for _, event := range linkedEvents {
		if event.ID <= 0 {
			return nil, fmt.Errorf("linked alert event id %d must be positive: %w", event.ID, domain.ErrInvariantViolation)
		}
		if _, ok := seenIDs[event.ID]; ok {
			continue
		}
		if len(seenIDs)+1 > scope.limit {
			return nil, fmt.Errorf(
				"existing alert group %d expansion contains more than limit (%d) events: %w",
				groupID, scope.limit, domain.ErrInvariantViolation,
			)
		}
		seenIDs[event.ID] = struct{}{}
		byID[event.ID] = event
	}
	events := make([]domain.AlertEvent, 0, len(byID))
	for _, event := range byID {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		left := domain.NormalizeUTCMicro(events[i].StartsAt)
		right := domain.NormalizeUTCMicro(events[j].StartsAt)
		if !left.Equal(right) {
			return left.Before(right)
		}
		return events[i].ID < events[j].ID
	})
	return events, nil
}

func mergeDraftWithExistingGroup(
	existing, draft domain.AlertGroup,
	events []domain.AlertEvent,
	grouping alertgrouping.Config,
) (domain.AlertGroup, error) {
	scopedDrafts, err := alertgrouping.GroupEvents(events, grouping)
	if err != nil {
		return domain.AlertGroup{}, err
	}
	if len(scopedDrafts) != 1 {
		return domain.AlertGroup{}, fmt.Errorf(
			"existing group %d expansion for group_key=%q produced %d scoped drafts: %w",
			existing.ID, draft.GroupKey, len(scopedDrafts), domain.ErrInvariantViolation,
		)
	}
	merged := scopedDrafts[0]
	if merged.GroupKey != draft.GroupKey {
		return domain.AlertGroup{}, fmt.Errorf(
			"existing group %d expansion changed group_key from %q to %q: %w",
			existing.ID, draft.GroupKey, merged.GroupKey, domain.ErrInvariantViolation,
		)
	}
	merged.EventIDs = alertEventIDs(events)
	merged.EventCount = len(events)
	merged.FirstSeenAt = domain.NormalizeUTCMicro(existing.FirstSeenAt)
	if len(existing.Dimensions) > 0 {
		merged.Dimensions = append([]byte(nil), existing.Dimensions...)
	}
	return merged, nil
}

func alertEventIDs(events []domain.AlertEvent) []domain.AlertEventID {
	ids := make([]domain.AlertEventID, len(events))
	for i := range events {
		ids[i] = events[i].ID
	}
	return ids
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
