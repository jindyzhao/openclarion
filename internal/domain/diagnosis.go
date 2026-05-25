package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// DiagnosisStatus is the lifecycle status of a DiagnosisTask. The
// vocabulary is text-typed (not a database enum) so M5 can add new
// states (e.g. "awaiting_user") without a migration.
type DiagnosisStatus string

// DiagnosisStatus enumeration. The terminal subset is enforced by
// IsTerminal: a row is non-mutable once it reaches a terminal state
// at the application layer.
const (
	DiagnosisStatusPending   DiagnosisStatus = "pending"
	DiagnosisStatusRunning   DiagnosisStatus = "running"
	DiagnosisStatusSucceeded DiagnosisStatus = "succeeded"
	DiagnosisStatusFailed    DiagnosisStatus = "failed"
	DiagnosisStatusCancelled DiagnosisStatus = "cancelled"
)

// Valid reports whether s is a known DiagnosisStatus value.
func (s DiagnosisStatus) Valid() bool {
	switch s {
	case DiagnosisStatusPending,
		DiagnosisStatusRunning,
		DiagnosisStatusSucceeded,
		DiagnosisStatusFailed,
		DiagnosisStatusCancelled:
		return true
	}
	return false
}

// IsTerminal reports whether s is a "no further mutation expected"
// state. The DiagnosisTask.FinishedAt field is non-nil iff IsTerminal
// returns true.
func (s DiagnosisStatus) IsTerminal() bool {
	switch s {
	case DiagnosisStatusSucceeded,
		DiagnosisStatusFailed,
		DiagnosisStatusCancelled:
		return true
	}
	return false
}

// DiagnosisTask is the workflow-bound lifecycle record for one
// DiagnosisWorkflow execution against one EvidenceSnapshot.
//
// Identity matches Temporal's own identity model: the natural unique
// key at the persistence boundary is (WorkflowID, RunID). A new RunID
// (Temporal retry, continue-as-new, reset, or scheduled retry)
// produces a NEW DiagnosisTask row, NOT an update to an existing one.
// This preserves a per-execution audit trail; WorkflowID alone has a
// non-unique chain index for "list all executions of this workflow".
type DiagnosisTask struct {
	ID                 DiagnosisTaskID
	EvidenceSnapshotID EvidenceSnapshotID
	WorkflowID         string
	RunID              string
	Status             DiagnosisStatus
	FailureReason      string
	StartedAt          *time.Time
	FinishedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// NewDiagnosisTask constructs a DiagnosisTask in the "pending" state
// with no started_at / finished_at set. Cross-field invariants
// enforced:
//
//   - EvidenceSnapshotID must be non-zero
//   - WorkflowID / RunID must be non-empty (immutable per Temporal
//     semantics)
//
// The repository fills ID / CreatedAt / UpdatedAt on insert.
func NewDiagnosisTask(snapshotID EvidenceSnapshotID, workflowID, runID string) (DiagnosisTask, error) {
	if snapshotID == 0 {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: evidence_snapshot_id must be non-zero: %w", ErrInvariantViolation)
	}
	if workflowID == "" {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: workflow_id must be non-empty: %w", ErrInvariantViolation)
	}
	if runID == "" {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: run_id must be non-empty: %w", ErrInvariantViolation)
	}
	return DiagnosisTask{
		EvidenceSnapshotID: snapshotID,
		WorkflowID:         workflowID,
		RunID:              runID,
		Status:             DiagnosisStatusPending,
	}, nil
}

// Start transitions the task from "pending" to "running" and stamps
// StartedAt = startedAt (UTC microsecond). Calling Start on an
// already-running task with the same startedAt is a no-op; on a
// task already in a terminal state it is an invariant violation.
func (t DiagnosisTask) Start(startedAt time.Time) (DiagnosisTask, error) {
	if startedAt.IsZero() {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: started_at must be set on Start: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(startedAt)
	if t.Status.IsTerminal() {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: cannot Start from terminal status %q: %w", t.Status, ErrInvariantViolation)
	}
	if t.Status == DiagnosisStatusRunning && t.StartedAt != nil {
		if t.StartedAt.Equal(normalised) {
			return t, nil
		}
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: started_at is immutable once set (%s -> %s): %w", *t.StartedAt, normalised, ErrInvariantViolation)
	}
	t.Status = DiagnosisStatusRunning
	t.StartedAt = &normalised
	return t, nil
}

// Finish transitions the task to a terminal state with FinishedAt =
// finishedAt (UTC microsecond) and FailureReason set when status is
// "failed". Cross-field invariants enforced:
//
//   - status must be terminal (succeeded / failed / cancelled)
//   - finishedAt must be non-zero and >= StartedAt when StartedAt is set
//   - failureReason MUST be non-empty when status == failed; MUST be
//     empty otherwise (cancelled / succeeded carry no failure reason)
//
// Re-finishing an already-terminal task with the same arguments is a
// no-op; with different arguments it is an invariant violation
// because terminal state is immutable.
func (t DiagnosisTask) Finish(status DiagnosisStatus, finishedAt time.Time, failureReason string) (DiagnosisTask, error) {
	if !status.IsTerminal() {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: Finish requires a terminal status, got %q: %w", status, ErrInvariantViolation)
	}
	if finishedAt.IsZero() {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: finished_at must be set on Finish: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(finishedAt)
	if t.StartedAt != nil && normalised.Before(*t.StartedAt) {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: finished_at %s precedes started_at %s: %w", normalised, *t.StartedAt, ErrInvariantViolation)
	}
	if status == DiagnosisStatusFailed && failureReason == "" {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: failure_reason must be non-empty when status==failed: %w", ErrInvariantViolation)
	}
	if status != DiagnosisStatusFailed && failureReason != "" {
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: failure_reason must be empty when status==%q: %w", status, ErrInvariantViolation)
	}
	if t.Status.IsTerminal() {
		if t.Status == status && t.FinishedAt != nil && t.FinishedAt.Equal(normalised) && t.FailureReason == failureReason {
			return t, nil
		}
		return DiagnosisTask{}, fmt.Errorf("diagnosis task: terminal status is immutable (%q -> %q): %w", t.Status, status, ErrInvariantViolation)
	}
	t.Status = status
	t.FinishedAt = &normalised
	t.FailureReason = failureReason
	return t, nil
}

// DiagnosisTaskEvent is one entry in the append-only lifecycle log
// of a DiagnosisTask. Rows are immutable; producers that need to
// express "X superseded Y" write a new event with the relation in
// Payload.
//
// Idempotency: DedupeKey is optional. When set, (TaskID, DedupeKey)
// is UNIQUE at the persistence boundary; multiple NULLs are allowed
// (standard Postgres UNIQUE NULL semantics) so producers without
// idempotency requirements can record events without supplying a
// key.
type DiagnosisTaskEvent struct {
	ID         DiagnosisTaskEventID
	TaskID     DiagnosisTaskID
	Kind       string
	Payload    json.RawMessage
	DedupeKey  *string
	OccurredAt time.Time
	RecordedAt time.Time
}

// NewDiagnosisTaskEvent constructs a DiagnosisTaskEvent. Cross-field
// invariants enforced:
//
//   - TaskID must be non-zero
//   - Kind must be non-empty
//   - OccurredAt must be non-zero (producer-supplied wall-clock)
//   - When DedupeKey is non-nil, the empty string is rejected (use a
//     nil pointer to mean "no idempotency key")
//
// Payload may be nil (typical for events whose meaning is fully
// captured by Kind). RecordedAt is filled by the persistence layer.
func NewDiagnosisTaskEvent(
	taskID DiagnosisTaskID,
	kind string,
	payload json.RawMessage,
	dedupeKey *string,
	occurredAt time.Time,
) (DiagnosisTaskEvent, error) {
	if taskID == 0 {
		return DiagnosisTaskEvent{}, fmt.Errorf("diagnosis task event: task_id must be non-zero: %w", ErrInvariantViolation)
	}
	if kind == "" {
		return DiagnosisTaskEvent{}, fmt.Errorf("diagnosis task event: kind must be non-empty: %w", ErrInvariantViolation)
	}
	if occurredAt.IsZero() {
		return DiagnosisTaskEvent{}, fmt.Errorf("diagnosis task event: occurred_at must be set: %w", ErrInvariantViolation)
	}
	if dedupeKey != nil && *dedupeKey == "" {
		return DiagnosisTaskEvent{}, fmt.Errorf("diagnosis task event: dedupe_key must be nil for no-idempotency producers, not the empty string: %w", ErrInvariantViolation)
	}
	return DiagnosisTaskEvent{
		TaskID:     taskID,
		Kind:       kind,
		Payload:    payload,
		DedupeKey:  dedupeKey,
		OccurredAt: NormalizeUTCMicro(occurredAt),
	}, nil
}
