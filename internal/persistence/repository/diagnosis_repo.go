package repository

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistask"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistaskevent"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// diagnosisRepo is the Ent-backed implementation of
// ports.DiagnosisRepository.
type diagnosisRepo struct {
	tx     *ent.Tx
	closed *atomic.Int32
}

// Compile-time assertion that the implementation satisfies the port.
var _ ports.DiagnosisRepository = (*diagnosisRepo)(nil)

// SaveTask inserts a new DiagnosisTask. The (workflow_id, run_id)
// natural unique key surfaces SQLSTATE 23505 as
// domain.ErrAlreadyExists.
func (r *diagnosisRepo) SaveTask(ctx context.Context, t domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTask{}, err
	}
	builder := r.tx.DiagnosisTask.Create().
		SetEvidenceSnapshotID(int(t.EvidenceSnapshotID)).
		SetWorkflowID(t.WorkflowID).
		SetRunID(t.RunID)
	if t.Status != "" {
		builder = builder.SetStatus(string(t.Status))
	}
	if t.FailureReason != "" {
		builder = builder.SetFailureReason(t.FailureReason)
	}
	if t.StartedAt != nil {
		builder = builder.SetStartedAt(*t.StartedAt)
	}
	if t.FinishedAt != nil {
		builder = builder.SetFinishedAt(*t.FinishedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DiagnosisTask{}, asAlreadyExists(err)
	}
	return diagnosisTaskToDomain(saved), nil
}

// UpdateTask writes mutable fields (status, started_at, finished_at,
// failure_reason). Immutable fields (snapshot, workflow, run,
// created_at) are ignored. updated_at is stamped automatically by
// the Ent UpdateDefault hook.
func (r *diagnosisRepo) UpdateTask(ctx context.Context, t domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTask{}, err
	}
	if t.ID == 0 {
		return domain.DiagnosisTask{}, fmt.Errorf("update diagnosis task: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.DiagnosisTask.UpdateOneID(int(t.ID)).
		SetStatus(string(t.Status)).
		SetFailureReason(t.FailureReason)
	if t.StartedAt != nil {
		builder = builder.SetStartedAt(*t.StartedAt)
	} else {
		builder = builder.ClearStartedAt()
	}
	if t.FinishedAt != nil {
		builder = builder.SetFinishedAt(*t.FinishedAt)
	} else {
		builder = builder.ClearFinishedAt()
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DiagnosisTask{}, asNotFound(err)
	}
	return diagnosisTaskToDomain(saved), nil
}

// FindTaskByID returns the DiagnosisTask or domain.ErrNotFound.
func (r *diagnosisRepo) FindTaskByID(ctx context.Context, id domain.DiagnosisTaskID) (domain.DiagnosisTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTask{}, err
	}
	row, err := r.tx.DiagnosisTask.Get(ctx, int(id))
	if err != nil {
		return domain.DiagnosisTask{}, asNotFound(err)
	}
	return diagnosisTaskToDomain(row), nil
}

// FindTaskByExecution returns the DiagnosisTask matching the natural
// identity (workflow_id, run_id), or domain.ErrNotFound.
func (r *diagnosisRepo) FindTaskByExecution(ctx context.Context, workflowID, runID string) (domain.DiagnosisTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTask{}, err
	}
	if workflowID == "" {
		return domain.DiagnosisTask{}, fmt.Errorf("find task by execution: workflow id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if runID == "" {
		return domain.DiagnosisTask{}, fmt.Errorf("find task by execution: run id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DiagnosisTask.Query().
		Where(
			diagnosistask.WorkflowIDEQ(workflowID),
			diagnosistask.RunIDEQ(runID),
		).
		Only(ctx)
	if err != nil {
		return domain.DiagnosisTask{}, asNotFound(err)
	}
	return diagnosisTaskToDomain(row), nil
}

// AppendEvent inserts a new DiagnosisTaskEvent. When DedupeKey is
// non-nil, a duplicate (task_id, dedupe_key) returns
// domain.ErrAlreadyExists. When DedupeKey is nil, multiple events
// with the same Kind are allowed (Postgres multi-NULL UNIQUE
// semantics).
func (r *diagnosisRepo) AppendEvent(ctx context.Context, e domain.DiagnosisTaskEvent) (domain.DiagnosisTaskEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	builder := r.tx.DiagnosisTaskEvent.Create().
		SetTaskID(int(e.TaskID)).
		SetKind(e.Kind).
		SetOccurredAt(e.OccurredAt)
	if len(e.Payload) > 0 {
		builder = builder.SetPayload(e.Payload)
	}
	if e.DedupeKey != nil {
		builder = builder.SetDedupeKey(*e.DedupeKey)
	}
	if !e.RecordedAt.IsZero() {
		builder = builder.SetRecordedAt(e.RecordedAt)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, asAlreadyExists(err)
	}
	return diagnosisTaskEventToDomain(saved), nil
}

// ListEvents returns the events for a task ordered by occurred_at
// ascending, capped by limit.
func (r *diagnosisRepo) ListEvents(ctx context.Context, taskID domain.DiagnosisTaskID, limit int) ([]domain.DiagnosisTaskEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if taskID == 0 {
		return nil, fmt.Errorf("list diagnosis events: task id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list diagnosis events: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DiagnosisTaskEvent.Query().
		Where(diagnosistaskevent.TaskIDEQ(int(taskID))).
		Order(diagnosistaskevent.ByOccurredAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis events: %w", err)
	}
	out := make([]domain.DiagnosisTaskEvent, len(rows))
	for i, row := range rows {
		out[i] = diagnosisTaskEventToDomain(row)
	}
	return out, nil
}
