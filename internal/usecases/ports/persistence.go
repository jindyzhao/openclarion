// Package ports defines the persistence and provider contracts
// owned by the usecase layer. Usecase code depends on these
// interfaces; concrete implementations live under
// `internal/persistence/repository` (persistence) and
// `internal/providers/<kind>/<impl>` (providers). Following
// architecture.md's layering rules, this package:
//
//   - depends ONLY on `internal/domain` and the Go standard library
//   - MUST NOT import the generated Ent client, Temporal SDK, or any
//     transport package
//   - is the sole place where repository and provider signatures
//     evolve; adding a method here is a deliberate contract change
//     reviewed alongside the usecase that needs it
//
// Aggregate-root boundaries (per the M1-PR2 design decision):
//
//   - AlertRepository:     AlertEvent + AlertGroup + the M2N
//     event<->group link + active/window queries
//   - EvidenceRepository:  EvidenceSnapshot persistence and per-group
//     lookup
//   - DiagnosisRepository: DiagnosisTask + the append-only
//     DiagnosisTaskEvent lifecycle log
//
// Five entity-level repositories were rejected because they would
// project the Ent table layout into the usecase layer, encouraging
// CRUD orchestration over aggregate behaviour.
//
// Provider ports (e.g. MetricsProvider) live in providers.go and
// follow the same layering rules: concrete implementations must
// import this package, never the other way around.
//
// UnitOfWork groups the three repositories under one Postgres
// transaction. Two entry points are provided:
//
//   - WithinTx(ctx, fn): the recommended default for usecase code.
//     fn returning nil commits; fn returning an error or panicking
//     rolls back. This is the form that prevents "forgot to roll
//     back" bugs.
//   - Begin(ctx): the escape hatch for the rare case where a
//     transaction's lifetime spans multiple control-flow boundaries
//     (e.g. multi-step orchestration). Callers are responsible for
//     exactly one Commit/Rollback.
//
// Repositories MUST translate Postgres unique-violations
// (SQLSTATE 23505) into domain.ErrAlreadyExists so usecases can
// implement idempotent producers without inspecting database error
// codes. "Not found" lookups MUST wrap domain.ErrNotFound.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// ErrNestedTransaction is returned by UnitOfWorkFactory.WithinTx
// when the caller invokes WithinTx with a context that is already
// inside an active transaction managed by the same package's
// factory implementation. Usecases that compose two helpers which
// both call WithinTx must restructure so only the outer call owns
// the transaction (and pass the inner-tx ctx + UoW down explicitly),
// or open the inner call from a context derived outside any
// existing WithinTx boundary.
var ErrNestedTransaction = errors.New("ports: nested WithinTx is not supported")

// AlertRepository covers the AlertEvent and AlertGroup aggregate
// root, including the many-to-many event<->group link materialised
// via the alert_event_groups join table and the active/window
// queries needed by the grouping stage.
type AlertRepository interface {
	// SaveEvent inserts a new AlertEvent. A duplicate
	// (source, canonical_fingerprint, starts_at) returns a wrapped
	// domain.ErrAlreadyExists. The returned AlertEvent has its ID
	// and CreatedAt populated.
	SaveEvent(ctx context.Context, e domain.AlertEvent) (domain.AlertEvent, error)

	// UpdateEventResolution persists an EndsAt + Status transition
	// to "resolved" for the AlertEvent identified by ID. Other
	// fields are ignored. Returns domain.ErrNotFound if the row is
	// missing.
	UpdateEventResolution(ctx context.Context, e domain.AlertEvent) (domain.AlertEvent, error)

	// FindEventByID returns the AlertEvent with the given ID, or
	// domain.ErrNotFound.
	FindEventByID(ctx context.Context, id domain.AlertEventID) (domain.AlertEvent, error)

	// FindEventByNaturalKey returns the AlertEvent matching the
	// natural unique key (source, canonical_fingerprint,
	// starts_at). startsAt is normalised via
	// domain.NormalizeUTCMicro before the lookup. Returns
	// domain.ErrNotFound when no such row exists.
	FindEventByNaturalKey(ctx context.Context, source, canonicalFingerprint string, startsAt time.Time) (domain.AlertEvent, error)

	// ListEventsByStartsAtRange returns AlertEvents whose StartsAt
	// falls in the half-open interval [startInclusive, endExclusive),
	// ordered by (starts_at ASC, id ASC) for replay determinism, and
	// capped by limit. The interval bounds are normalised via
	// domain.NormalizeUTCMicro before the predicate is built so callers
	// do not need to pre-truncate.
	//
	// As a boundary self-defence (callers higher up are expected to
	// validate too) the implementation MUST reject:
	//   - zero startInclusive or zero endExclusive
	//   - limit <= 0
	//   - normalised endExclusive not strictly after normalised
	//     startInclusive (avoids spurious "empty" or "overlap"
	//     windows when sub-microsecond differences are erased by
	//     the normaliser)
	// with a wrapped domain.ErrInvariantViolation.
	ListEventsByStartsAtRange(ctx context.Context, startInclusive, endExclusive time.Time, limit int) ([]domain.AlertEvent, error)

	// SaveGroup inserts a new AlertGroup header (without the M2N
	// link). A duplicate (group_key, first_seen_at) returns a
	// wrapped domain.ErrAlreadyExists. Use LinkEventsToGroup to
	// attach AlertEvents.
	SaveGroup(ctx context.Context, g domain.AlertGroup) (domain.AlertGroup, error)

	// UpdateGroup persists mutable fields of an AlertGroup
	// (severity, event_count, status, last_seen_at). Immutable
	// fields (group_key, first_seen_at, created_at) are ignored.
	// Returns domain.ErrNotFound if the row is missing.
	UpdateGroup(ctx context.Context, g domain.AlertGroup) (domain.AlertGroup, error)

	// FindGroupByID returns the AlertGroup with the given ID, or
	// domain.ErrNotFound. EventIDs is left nil; use
	// ListEventIDsForGroup to materialise the M2N link if needed.
	FindGroupByID(ctx context.Context, id domain.AlertGroupID) (domain.AlertGroup, error)

	// FindGroupByNaturalKey looks up an AlertGroup by its natural
	// unique key (group_key, first_seen_at) and returns it, or a
	// wrapped domain.ErrNotFound. firstSeenAt is normalised via
	// domain.NormalizeUTCMicro before the lookup. Used by the alert
	// window replay harness as the pre-check that decides whether to
	// SaveGroup or UpdateGroup; pre-checking is required because a
	// SQLSTATE 23505 raised inside a multi-step transaction aborts
	// the entire transaction and Ent does not wrap inserts in their
	// own SAVEPOINT.
	//
	// As a boundary self-defence the implementation MUST reject
	// empty groupKey or zero firstSeenAt with a wrapped
	// domain.ErrInvariantViolation. EventIDs on the returned group
	// is left nil; callers materialise it via ListEventIDsForGroup
	// when needed.
	FindGroupByNaturalKey(ctx context.Context, groupKey string, firstSeenAt time.Time) (domain.AlertGroup, error)

	// LinkEventsToGroup attaches the given AlertEventIDs to the
	// AlertGroup via the M2N edge. Re-linking an existing
	// (group, event) pair is a no-op; the operation is therefore
	// idempotent. Empty eventIDs is a valid no-op.
	LinkEventsToGroup(ctx context.Context, groupID domain.AlertGroupID, eventIDs []domain.AlertEventID) error

	// ListEventIDsForGroup returns the AlertEventIDs linked to the
	// AlertGroup via the M2N edge, ordered by AlertEvent.starts_at
	// ascending. Returns an empty slice (not domain.ErrNotFound)
	// when the group exists but has no events linked.
	ListEventIDsForGroup(ctx context.Context, groupID domain.AlertGroupID) ([]domain.AlertEventID, error)

	// ListActiveGroups returns AlertGroups whose status == "active",
	// ordered by last_seen_at descending. limit MUST be > 0; the
	// implementation MAY cap it to a sane upper bound to protect
	// the database.
	ListActiveGroups(ctx context.Context, limit int) ([]domain.AlertGroup, error)
}

// EvidenceRepository covers EvidenceSnapshot persistence and the
// per-group queries needed by the diagnosis stage.
type EvidenceRepository interface {
	// Save inserts a new EvidenceSnapshot. A duplicate
	// (alert_group_id, digest) returns a wrapped
	// domain.ErrAlreadyExists. The returned snapshot has ID and
	// CreatedAt populated.
	Save(ctx context.Context, s domain.EvidenceSnapshot) (domain.EvidenceSnapshot, error)

	// FindByID returns the EvidenceSnapshot with the given ID, or
	// domain.ErrNotFound.
	FindByID(ctx context.Context, id domain.EvidenceSnapshotID) (domain.EvidenceSnapshot, error)

	// FindByGroupAndDigest returns the EvidenceSnapshot matching
	// the per-group idempotency key (alert_group_id, digest), or
	// domain.ErrNotFound. Producers use this to retrieve a row
	// after a duplicate-key insert without re-deriving the ID.
	FindByGroupAndDigest(ctx context.Context, groupID domain.AlertGroupID, digest string) (domain.EvidenceSnapshot, error)

	// ListByGroup returns EvidenceSnapshots for a given AlertGroup
	// ordered by created_at descending, capped by limit. limit
	// MUST be > 0.
	ListByGroup(ctx context.Context, groupID domain.AlertGroupID, limit int) ([]domain.EvidenceSnapshot, error)
}

// DiagnosisRepository covers DiagnosisTask plus the append-only
// DiagnosisTaskEvent lifecycle log.
type DiagnosisRepository interface {
	// SaveTask inserts a new DiagnosisTask. A duplicate
	// (workflow_id, run_id) returns a wrapped
	// domain.ErrAlreadyExists. The returned task has ID,
	// CreatedAt, and UpdatedAt populated.
	SaveTask(ctx context.Context, t domain.DiagnosisTask) (domain.DiagnosisTask, error)

	// UpdateTask persists mutable fields (status, started_at,
	// finished_at, failure_reason). Immutable fields
	// (evidence_snapshot_id, workflow_id, run_id, created_at) are
	// ignored. Returns domain.ErrNotFound if the row is missing.
	UpdateTask(ctx context.Context, t domain.DiagnosisTask) (domain.DiagnosisTask, error)

	// FindTaskByID returns the DiagnosisTask with the given ID, or
	// domain.ErrNotFound.
	FindTaskByID(ctx context.Context, id domain.DiagnosisTaskID) (domain.DiagnosisTask, error)

	// FindTaskByExecution returns the DiagnosisTask matching the
	// natural identity (workflow_id, run_id), or
	// domain.ErrNotFound.
	FindTaskByExecution(ctx context.Context, workflowID, runID string) (domain.DiagnosisTask, error)

	// AppendEvent appends a lifecycle event to the task. When
	// DedupeKey is set, a duplicate (task_id, dedupe_key) returns
	// a wrapped domain.ErrAlreadyExists; when DedupeKey is nil,
	// multiple events with the same Kind are allowed (standard
	// Postgres multi-NULL UNIQUE semantics). Append is the only
	// write path: events are immutable once persisted.
	AppendEvent(ctx context.Context, e domain.DiagnosisTaskEvent) (domain.DiagnosisTaskEvent, error)

	// FindEventByTaskAndDedupeKey returns the DiagnosisTaskEvent
	// matching the per-task idempotency key (task_id, dedupe_key),
	// or domain.ErrNotFound. dedupeKey MUST be non-empty: the
	// (task_id, dedupe_key) UNIQUE index has Postgres multi-NULL
	// semantics, so the empty string would be a misuse, not a
	// lookup. Required for the Update-driven idempotent producer
	// pattern: a duplicate AppendEvent (23505) is recovered by
	// looking up the original row in a fresh transaction (the
	// failed insert tx is poisoned and cannot be reused for the
	// SELECT).
	FindEventByTaskAndDedupeKey(ctx context.Context, taskID domain.DiagnosisTaskID, dedupeKey string) (domain.DiagnosisTaskEvent, error)

	// ListEvents returns the events for a task ordered by
	// occurred_at ascending, capped by limit. limit MUST be > 0.
	ListEvents(ctx context.Context, taskID domain.DiagnosisTaskID, limit int) ([]domain.DiagnosisTaskEvent, error)
}

// UnitOfWork bundles the three aggregate-root repositories under a
// single Postgres transaction.
//
// Lifecycle: exactly one of Commit / Rollback MUST be called. After
// Commit or Rollback, calling any method on the bundled
// repositories or the UoW itself MUST return an error; concrete
// implementations enforce this via a closed flag.
//
// Concurrency: a UnitOfWork is bound to one transaction and is NOT
// safe for concurrent use across goroutines.
type UnitOfWork interface {
	// Alerts returns the AlertRepository bound to this transaction.
	Alerts() AlertRepository

	// Evidence returns the EvidenceRepository bound to this
	// transaction.
	Evidence() EvidenceRepository

	// Diagnosis returns the DiagnosisRepository bound to this
	// transaction.
	Diagnosis() DiagnosisRepository

	// Commit finalises the transaction. After a successful Commit
	// the UoW is closed; subsequent Commit / Rollback calls return
	// an error.
	Commit(ctx context.Context) error

	// Rollback aborts the transaction. After Rollback the UoW is
	// closed; subsequent Commit / Rollback calls return an error.
	// Rollback on an already-committed UoW returns an error;
	// callers should pair Rollback with `defer` and inspect Commit
	// status accordingly, or prefer WithinTx.
	Rollback(ctx context.Context) error
}

// UnitOfWorkFactory creates UnitOfWork instances bound to a fresh
// Postgres transaction. Two entry points are provided to balance
// safety (default WithinTx) against flexibility (escape-hatch Begin).
type UnitOfWorkFactory interface {
	// Begin starts a new transaction and returns a UnitOfWork
	// scoped to it. Callers MUST exactly one of Commit / Rollback
	// before the context is cancelled or the program exits. Use
	// WithinTx unless the transaction's lifetime must span control
	// boundaries that prevent a single function from owning it.
	Begin(ctx context.Context) (UnitOfWork, error)

	// WithinTx runs fn inside a Begin / Commit / Rollback
	// boundary. Semantics:
	//
	//   - if fn returns nil, the transaction is committed and the
	//     commit error (if any) is returned;
	//   - if fn returns a non-nil error, the transaction is rolled
	//     back and fn's error is returned;
	//   - if fn panics, the transaction is rolled back and the
	//     panic is re-raised after rollback completes.
	//
	// Implementations MUST NOT support nested WithinTx calls on the
	// same context: when ctx is already inside an active WithinTx
	// boundary, the inner call MUST return ErrNestedTransaction
	// (wrapped or bare; callers detect via errors.Is) without
	// opening a second transaction or invoking fn. This protects
	// the UoW atomicity guarantee against accidental composition
	// where an inner commit could outlive an outer rollback.
	WithinTx(ctx context.Context, fn func(context.Context, UnitOfWork) error) error
}
