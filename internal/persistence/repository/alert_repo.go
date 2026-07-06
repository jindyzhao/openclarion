package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertevent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertgroup"
	"github.com/openclarion/openclarion/internal/persistence/ent/predicate"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// alertRepo is the Ent-backed implementation of ports.AlertRepository.
// All methods run inside the parent UoW's *ent.Tx; closure of the
// parent UoW is detected via the shared atomic flag.
type alertRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

// Compile-time assertion that the implementation satisfies the port.
var _ ports.AlertRepository = (*alertRepo)(nil)

// SaveEvent inserts a new AlertEvent. ID and CreatedAt are populated
// on success. Schema defaults (Status="firing", CreatedAt=time.Now)
// fire when the domain entity has the zero value for those fields,
// so this method respects domain semantics regardless of whether the
// caller explicitly set them.
func (r *alertRepo) SaveEvent(ctx context.Context, e domain.AlertEvent) (domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertEvent{}, err
	}
	builder := r.tx.AlertEvent.Create().
		SetSource(e.Source).
		SetSourceFingerprint(e.SourceFingerprint).
		SetCanonicalFingerprint(e.CanonicalFingerprint).
		SetLabels(e.Labels).
		SetAnnotations(e.Annotations).
		SetRawPayload(e.RawPayload).
		SetStartsAt(e.StartsAt)
	if e.AlertSourceProfileID > 0 {
		builder = builder.SetAlertSourceProfileID(int(e.AlertSourceProfileID))
	}
	if e.Status != "" {
		builder = builder.SetStatus(string(e.Status))
	}
	if e.EndsAt != nil {
		builder = builder.SetEndsAt(*e.EndsAt)
	}
	if !e.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(e.CreatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertEvent{}, asAlreadyExists(err)
	}
	return alertEventToDomain(saved), nil
}

// UpdateEventResolution writes Status + EndsAt for an existing event.
// Per the port contract, all other fields on `e` are ignored.
func (r *alertRepo) UpdateEventResolution(ctx context.Context, e domain.AlertEvent) (domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertEvent{}, err
	}
	if e.ID == 0 {
		return domain.AlertEvent{}, fmt.Errorf("update event resolution: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.AlertEvent.UpdateOneID(int(e.ID)).
		SetStatus(string(e.Status))
	if e.EndsAt != nil {
		builder = builder.SetEndsAt(*e.EndsAt)
	} else {
		builder = builder.ClearEndsAt()
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertEvent{}, asNotFound(err)
	}
	return alertEventToDomain(saved), nil
}

// FindEventByID returns the AlertEvent or domain.ErrNotFound.
func (r *alertRepo) FindEventByID(ctx context.Context, id domain.AlertEventID) (domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertEvent{}, err
	}
	row, err := r.tx.AlertEvent.Get(ctx, int(id))
	if err != nil {
		return domain.AlertEvent{}, asNotFound(err)
	}
	return alertEventToDomain(row), nil
}

// FindEventByNaturalKey looks up by the legacy unscoped key
// (source, canonical_fingerprint, starts_at). startsAt is normalised before
// the lookup so callers do not need to pre-truncate.
func (r *alertRepo) FindEventByNaturalKey(ctx context.Context, source, canonicalFingerprint string, startsAt time.Time) (domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertEvent{}, err
	}
	normalised := domain.NormalizeUTCMicro(startsAt)
	row, err := r.tx.AlertEvent.Query().
		Where(
			alertevent.AlertSourceProfileIDEQ(0),
			alertevent.SourceEQ(source),
			alertevent.CanonicalFingerprintEQ(canonicalFingerprint),
			alertevent.StartsAtEQ(normalised),
		).
		Only(ctx)
	if err != nil {
		return domain.AlertEvent{}, asNotFound(err)
	}
	return alertEventToDomain(row), nil
}

// ListEventsByNaturalKeys returns AlertEvents matching exact scoped natural
// keys. The OR-of-ANDs predicate keeps the full unique key in SQL before the
// result cap is applied.
func (r *alertRepo) ListEventsByNaturalKeys(ctx context.Context, keys []ports.AlertEventNaturalKey, limit int) ([]domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list events by natural keys: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	predicates := make([]predicate.AlertEvent, 0, len(keys))
	seen := make(map[ports.AlertEventNaturalKey]struct{}, len(keys))
	for i, key := range keys {
		normalised, err := normaliseAlertEventNaturalKey(key)
		if err != nil {
			return nil, fmt.Errorf("list events by natural keys: key[%d]: %w", i, err)
		}
		if _, ok := seen[normalised]; ok {
			continue
		}
		seen[normalised] = struct{}{}
		predicates = append(predicates, alertevent.And(
			alertevent.AlertSourceProfileIDEQ(int(normalised.AlertSourceProfileID)),
			alertevent.SourceEQ(normalised.Source),
			alertevent.CanonicalFingerprintEQ(normalised.CanonicalFingerprint),
			alertevent.StartsAtEQ(normalised.StartsAt),
		))
	}
	if len(predicates) == 0 {
		return nil, nil
	}
	rows, err := r.tx.AlertEvent.Query().
		Where(alertevent.Or(predicates...)).
		Order(alertevent.ByStartsAt(), alertevent.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list events by natural keys: %w", err)
	}
	out := make([]domain.AlertEvent, len(rows))
	for i, row := range rows {
		out[i] = alertEventToDomain(row)
	}
	return out, nil
}

// ListEventsByStartsAtRange returns AlertEvents whose StartsAt falls
// in the half-open interval [startInclusive, endExclusive), ordered
// by (starts_at ASC, id ASC), capped by limit.
//
// The half-open interval is what callers (notably the replay harness)
// need so adjacent windows do not double-count boundary events. Both
// bounds are normalised via domain.NormalizeUTCMicro before the
// predicate is built so the comparison happens at the same precision
// as the persisted column. We reject zero bounds, non-positive limit,
// and a non-strictly-after end bound here as boundary self-defence:
// the same constraints are validated by the usecase layer, but a
// repository invariant violation is a bug regardless of who called us.
func (r *alertRepo) ListEventsByStartsAtRange(ctx context.Context, startInclusive, endExclusive time.Time, limit int) ([]domain.AlertEvent, error) {
	return r.ListEventsByStartsAtRangeFiltered(ctx, startInclusive, endExclusive, ports.AlertEventFilter{}, limit)
}

// ListEventsByStartsAtRangeFiltered returns AlertEvents whose StartsAt falls
// in the half-open interval after applying source/profile predicates.
func (r *alertRepo) ListEventsByStartsAtRangeFiltered(ctx context.Context, startInclusive, endExclusive time.Time, filter ports.AlertEventFilter, limit int) ([]domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if startInclusive.IsZero() {
		return nil, fmt.Errorf("list events by starts_at range: start_inclusive must be set: %w", domain.ErrInvariantViolation)
	}
	if endExclusive.IsZero() {
		return nil, fmt.Errorf("list events by starts_at range: end_exclusive must be set: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list events by starts_at range: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	nStart := domain.NormalizeUTCMicro(startInclusive)
	nEnd := domain.NormalizeUTCMicro(endExclusive)
	// Compare on the normalised values so that sub-microsecond
	// differences in the raw inputs (which are erased by
	// NormalizeUTCMicro) cannot smuggle through an effectively-empty
	// or inverted window.
	if !nEnd.After(nStart) {
		return nil, fmt.Errorf("list events by starts_at range: end_exclusive %s must be strictly after start_inclusive %s (after normalisation): %w", nEnd, nStart, domain.ErrInvariantViolation)
	}
	predicates, err := alertEventPredicates(filter)
	if err != nil {
		return nil, err
	}
	predicates = append(predicates,
		alertevent.StartsAtGTE(nStart),
		alertevent.StartsAtLT(nEnd),
	)
	rows, err := r.tx.AlertEvent.Query().
		Where(predicates...).
		Order(alertevent.ByStartsAt(), alertevent.ByID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list events by starts_at range: %w", err)
	}
	out := make([]domain.AlertEvent, len(rows))
	for i, row := range rows {
		out[i] = alertEventToDomain(row)
	}
	return out, nil
}

// ListEvents returns the most recent events, ordered by starts_at
// descending with id as the deterministic tie-breaker.
func (r *alertRepo) ListEvents(ctx context.Context, limit int) ([]domain.AlertEvent, error) {
	return r.ListEventsFiltered(ctx, ports.AlertEventFilter{}, limit)
}

// ListEventsFiltered returns recent events after applying source/profile
// predicates before ordering and limiting.
func (r *alertRepo) ListEventsFiltered(ctx context.Context, filter ports.AlertEventFilter, limit int) ([]domain.AlertEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list events: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	predicates, err := alertEventPredicates(filter)
	if err != nil {
		return nil, err
	}
	rows, err := r.tx.AlertEvent.Query().
		Where(predicates...).
		Order(alertevent.ByStartsAt(entsql.OrderDesc()), alertevent.ByID(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	out := make([]domain.AlertEvent, len(rows))
	for i, row := range rows {
		out[i] = alertEventToDomain(row)
	}
	return out, nil
}

func alertEventPredicates(filter ports.AlertEventFilter) ([]predicate.AlertEvent, error) {
	predicates := make([]predicate.AlertEvent, 0, 3)
	for _, id := range filter.IDs {
		if id <= 0 {
			return nil, fmt.Errorf("alert event filter: alert event id %d must be positive: %w", id, domain.ErrInvariantViolation)
		}
	}
	ids := positiveAlertEventIDs(filter.IDs)
	if len(ids) > 0 {
		predicates = append(predicates, alertevent.IDIn(ids...))
	}
	sources := nonEmptyStrings(filter.Sources)
	if len(sources) > 0 {
		predicates = append(predicates, alertevent.SourceIn(sources...))
	}
	profileIDs := positiveAlertSourceProfileIDs(filter.AlertSourceProfileIDs)
	if len(profileIDs) > 0 {
		predicates = append(predicates, alertevent.AlertSourceProfileIDIn(profileIDs...))
	}
	for _, id := range filter.AlertSourceProfileIDs {
		if id < 0 {
			return nil, fmt.Errorf("alert event filter: alert source profile id %d must be non-negative: %w", id, domain.ErrInvariantViolation)
		}
	}
	return predicates, nil
}

func normaliseAlertEventNaturalKey(key ports.AlertEventNaturalKey) (ports.AlertEventNaturalKey, error) {
	if key.AlertSourceProfileID < 0 {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("alert source profile id %d must be non-negative: %w", key.AlertSourceProfileID, domain.ErrInvariantViolation)
	}
	source := strings.TrimSpace(key.Source)
	if source == "" {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("source must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if source != key.Source {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("source must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	canonical := strings.TrimSpace(key.CanonicalFingerprint)
	if canonical == "" {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("canonical fingerprint must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if canonical != key.CanonicalFingerprint {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("canonical fingerprint must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	startsAt := domain.NormalizeUTCMicro(key.StartsAt)
	if startsAt.IsZero() {
		return ports.AlertEventNaturalKey{}, fmt.Errorf("starts_at must be set: %w", domain.ErrInvariantViolation)
	}
	return ports.AlertEventNaturalKey{
		AlertSourceProfileID: key.AlertSourceProfileID,
		Source:               source,
		CanonicalFingerprint: canonical,
		StartsAt:             startsAt,
	}, nil
}

func positiveAlertEventIDs(ids []domain.AlertEventID) []int {
	out := make([]int, 0, len(ids))
	seen := map[int]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		value := int(id)
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func positiveAlertSourceProfileIDs(ids []domain.AlertSourceProfileID) []int {
	out := make([]int, 0, len(ids))
	seen := map[int]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		intID := int(id)
		if _, ok := seen[intID]; ok {
			continue
		}
		seen[intID] = struct{}{}
		out = append(out, intID)
	}
	return out
}

// SaveGroup inserts a new AlertGroup HEADER (no event link). Use
// LinkEventsToGroup separately to materialise the M2N edge.
func (r *alertRepo) SaveGroup(ctx context.Context, g domain.AlertGroup) (domain.AlertGroup, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertGroup{}, err
	}
	builder := r.tx.AlertGroup.Create().
		SetGroupKey(g.GroupKey).
		SetDimensions(g.Dimensions).
		SetFirstSeenAt(g.FirstSeenAt).
		SetLastSeenAt(g.LastSeenAt)
	if g.Severity != "" {
		builder = builder.SetSeverity(string(g.Severity))
	}
	if g.EventCount > 0 {
		builder = builder.SetEventCount(g.EventCount)
	}
	if g.Status != "" {
		builder = builder.SetStatus(string(g.Status))
	}
	if !g.CreatedAt.IsZero() {
		builder = builder.SetCreatedAt(g.CreatedAt)
	}
	if !g.UpdatedAt.IsZero() {
		builder = builder.SetUpdatedAt(g.UpdatedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertGroup{}, asAlreadyExists(err)
	}
	return alertGroupToDomain(saved), nil
}

// UpdateGroup writes mutable fields (severity, event_count, status,
// last_seen_at). Immutable fields are ignored. updated_at is stamped
// automatically by the Ent UpdateDefault hook.
func (r *alertRepo) UpdateGroup(ctx context.Context, g domain.AlertGroup) (domain.AlertGroup, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertGroup{}, err
	}
	if g.ID == 0 {
		return domain.AlertGroup{}, fmt.Errorf("update group: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.AlertGroup.UpdateOneID(int(g.ID)).
		SetSeverity(string(g.Severity)).
		SetEventCount(g.EventCount).
		SetStatus(string(g.Status)).
		SetLastSeenAt(g.LastSeenAt)
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.AlertGroup{}, asNotFound(err)
	}
	return alertGroupToDomain(saved), nil
}

// FindGroupByID returns the AlertGroup or domain.ErrNotFound.
// EventIDs is left nil; callers materialise it via
// ListEventIDsForGroup when needed.
func (r *alertRepo) FindGroupByID(ctx context.Context, id domain.AlertGroupID) (domain.AlertGroup, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertGroup{}, err
	}
	row, err := r.tx.AlertGroup.Get(ctx, int(id))
	if err != nil {
		return domain.AlertGroup{}, asNotFound(err)
	}
	return alertGroupToDomain(row), nil
}

// FindGroupByNaturalKey looks up an AlertGroup by (group_key,
// first_seen_at). firstSeenAt is normalised before the lookup so
// callers do not need to pre-truncate. EventIDs on the returned row
// is left nil; callers materialise it via ListEventIDsForGroup when
// needed.
//
// This is the pre-check the replay harness runs at the start of each
// per-group transaction to decide whether the next step is a SaveGroup
// (NotFound branch) or an UpdateGroup (Found branch). Doing the
// pre-check matters because a SQLSTATE 23505 raised inside a
// multi-step transaction aborts the entire transaction (Ent does not
// wrap inserts in their own SAVEPOINT), so an insert-fail-recover
// pattern in the same tx is not viable.
//
// Empty groupKey or zero firstSeenAt is a programmer error; we surface
// it as a wrapped domain.ErrInvariantViolation rather than letting the
// query degenerate into an unintended match.
func (r *alertRepo) FindGroupByNaturalKey(ctx context.Context, groupKey string, firstSeenAt time.Time) (domain.AlertGroup, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.AlertGroup{}, err
	}
	if groupKey == "" {
		return domain.AlertGroup{}, fmt.Errorf("find group by natural key: group_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if firstSeenAt.IsZero() {
		return domain.AlertGroup{}, fmt.Errorf("find group by natural key: first_seen_at must be set: %w", domain.ErrInvariantViolation)
	}
	normalised := domain.NormalizeUTCMicro(firstSeenAt)
	row, err := r.tx.AlertGroup.Query().
		Where(
			alertgroup.GroupKeyEQ(groupKey),
			alertgroup.FirstSeenAtEQ(normalised),
		).
		Only(ctx)
	if err != nil {
		return domain.AlertGroup{}, asNotFound(err)
	}
	return alertGroupToDomain(row), nil
}

// LinkEventsToGroup attaches AlertEventIDs to the AlertGroup via the
// M2N edge. Re-linking an existing pair is a no-op (Ent's
// AddEventIDs translates to ON CONFLICT DO NOTHING for the join
// table). Empty eventIDs returns nil.
//
// The M2N join row write is the failure surface most likely to hit
// FK violations (event or group missing). We propagate the raw error
// in that case because it indicates a programming bug, not an
// idempotency boundary.
func (r *alertRepo) LinkEventsToGroup(ctx context.Context, groupID domain.AlertGroupID, eventIDs []domain.AlertEventID) error {
	if err := checkOpen(r.closed); err != nil {
		return err
	}
	if len(eventIDs) == 0 {
		return nil
	}
	if groupID == 0 {
		return fmt.Errorf("link events to group: group id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	entIDs := alertEventIDsToEnt(eventIDs)
	_, err := r.tx.AlertGroup.UpdateOneID(int(groupID)).
		AddEventIDs(entIDs...).
		Save(ctx)
	if err != nil {
		// A unique-violation here means the (group, event) pair
		// already exists; treat as a no-op for idempotency.
		// FK violations and other errors propagate.
		pgErr := asAlreadyExists(err)
		if errors.Is(pgErr, domain.ErrAlreadyExists) {
			return nil
		}
		return asNotFound(err)
	}
	return nil
}

// ListEventIDsForGroup returns the AlertEventIDs linked to the
// AlertGroup ordered by AlertEvent.starts_at ascending and capped by limit.
//
// Implementation note: we cannot use Ent's IDs(ctx) shortcut here.
// On an M2N traversal, Ent emits SELECT DISTINCT id ... ORDER BY
// starts_at, which Postgres rejects (SQLSTATE 42P10) because the
// ORDER BY column must also appear in the select list when DISTINCT
// is present. The (alert_event_id, alert_group_id) link row already
// carries a uniqueness invariant, so the DISTINCT pass would be a
// no-op anyway. Materialising the rows and projecting their IDs is
// both correct and equivalent in cost given the bounded fan-out of
// events per group.
func (r *alertRepo) ListEventIDsForGroup(ctx context.Context, groupID domain.AlertGroupID, limit int) ([]domain.AlertEventID, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if groupID == 0 {
		return nil, fmt.Errorf("list event ids for group: group id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list event ids for group: limit %d must be > 0: %w", limit, domain.ErrInvariantViolation)
	}
	if _, err := r.tx.AlertGroup.Get(ctx, int(groupID)); err != nil {
		return nil, asNotFound(err)
	}
	rows, err := r.tx.AlertGroup.Query().
		Where(alertgroup.IDEQ(int(groupID))).
		QueryEvents().
		Order(alertevent.ByStartsAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list event ids for group %d: %w", groupID, err)
	}
	out := make([]domain.AlertEventID, len(rows))
	for i, row := range rows {
		out[i] = domain.AlertEventID(row.ID)
	}
	return out, nil
}

// ListActiveGroups returns groups whose status == "active", ordered
// by last_seen_at descending. limit MUST be > 0; we surface that as
// a domain invariant error so callers see a consistent message.
func (r *alertRepo) ListActiveGroups(ctx context.Context, limit int) ([]domain.AlertGroup, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list active groups: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.AlertGroup.Query().
		Where(alertgroup.StatusEQ(string(domain.AlertGroupStatusActive))).
		Order(alertgroup.ByLastSeenAt(entsql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups: %w", err)
	}
	out := make([]domain.AlertGroup, len(rows))
	for i, row := range rows {
		out[i] = alertGroupToDomain(row)
	}
	return out, nil
}
