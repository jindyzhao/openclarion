package repository

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsession"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatturn"
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

// ListTasksByEvidenceSnapshot returns recent task executions for one
// EvidenceSnapshot.
func (r *diagnosisRepo) ListTasksByEvidenceSnapshot(ctx context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.DiagnosisTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("list diagnosis tasks by evidence snapshot: snapshot id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list diagnosis tasks by evidence snapshot: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DiagnosisTask.Query().
		Where(diagnosistask.EvidenceSnapshotIDEQ(int(snapshotID))).
		Order(
			diagnosistask.ByCreatedAt(sql.OrderDesc()),
			diagnosistask.ByID(sql.OrderDesc()),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis tasks by evidence snapshot: %w", err)
	}
	out := make([]domain.DiagnosisTask, len(rows))
	for i, row := range rows {
		out[i] = diagnosisTaskToDomain(row)
	}
	return out, nil
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

// FindEventByTaskAndDedupeKey returns the DiagnosisTaskEvent matching
// the per-task idempotency key (task_id, dedupe_key), or
// domain.ErrNotFound. The (task_id, dedupe_key) UNIQUE index uses
// Postgres multi-NULL semantics; an empty dedupeKey is therefore an
// invariant violation, not a lookup, and rejected up front.
func (r *diagnosisRepo) FindEventByTaskAndDedupeKey(ctx context.Context, taskID domain.DiagnosisTaskID, dedupeKey string) (domain.DiagnosisTaskEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	if taskID == 0 {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("find diagnosis event by dedupe key: task id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if dedupeKey == "" {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("find diagnosis event by dedupe key: dedupe_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.DiagnosisTaskEvent.Query().
		Where(
			diagnosistaskevent.TaskIDEQ(int(taskID)),
			diagnosistaskevent.DedupeKeyEQ(dedupeKey),
		).
		Only(ctx)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, asNotFound(err)
	}
	return diagnosisTaskEventToDomain(row), nil
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

// ListEventsByTaskAndKind returns matching task events in reverse
// occurrence order.
func (r *diagnosisRepo) ListEventsByTaskAndKind(ctx context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if taskID == 0 {
		return nil, fmt.Errorf("list diagnosis events by kind: task id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, fmt.Errorf("list diagnosis events by kind: kind must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list diagnosis events by kind: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.DiagnosisTaskEvent.Query().
		Where(
			diagnosistaskevent.TaskIDEQ(int(taskID)),
			diagnosistaskevent.KindEQ(kind),
		).
		Order(
			diagnosistaskevent.ByOccurredAt(sql.OrderDesc()),
			diagnosistaskevent.ByID(sql.OrderDesc()),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list diagnosis events by kind: %w", err)
	}
	out := make([]domain.DiagnosisTaskEvent, len(rows))
	for i, row := range rows {
		out[i] = diagnosisTaskEventToDomain(row)
	}
	return out, nil
}

// SaveChatSession inserts a new M5 diagnosis-room ChatSession. The
// unique session_key and diagnosis_task_id constraints are both
// surfaced as domain.ErrAlreadyExists.
func (r *diagnosisRepo) SaveChatSession(ctx context.Context, s domain.ChatSession) (domain.ChatSession, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSession{}, err
	}
	builder := r.tx.ChatSession.Create().
		SetDiagnosisTaskID(int(s.DiagnosisTaskID)).
		SetSessionKey(s.SessionKey).
		SetOwnerSubject(s.OwnerSubject).
		SetStartedAt(s.StartedAt).
		SetLastActivityAt(s.LastActivityAt)
	if s.Status != "" {
		builder = builder.SetStatus(string(s.Status))
	}
	if s.TurnCount != 0 {
		builder = builder.SetTurnCount(s.TurnCount)
	}
	if s.ClosedAt != nil {
		builder = builder.SetClosedAt(*s.ClosedAt)
	}
	if s.CloseReason != "" {
		builder = builder.SetCloseReason(s.CloseReason)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ChatSession{}, asAlreadyExists(err)
	}
	return chatSessionToDomain(saved), nil
}

// UpdateChatSession writes mutable lifecycle fields. Immutable fields
// are intentionally ignored.
func (r *diagnosisRepo) UpdateChatSession(ctx context.Context, s domain.ChatSession) (domain.ChatSession, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSession{}, err
	}
	if s.ID == 0 {
		return domain.ChatSession{}, fmt.Errorf("update chat session: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	builder := r.tx.ChatSession.UpdateOneID(int(s.ID)).
		SetStatus(string(s.Status)).
		SetTurnCount(s.TurnCount).
		SetLastActivityAt(s.LastActivityAt).
		SetCloseReason(s.CloseReason)
	if s.ClosedAt != nil {
		builder = builder.SetClosedAt(*s.ClosedAt)
	} else {
		builder = builder.ClearClosedAt()
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ChatSession{}, asNotFound(err)
	}
	return chatSessionToDomain(saved), nil
}

// FindChatSessionByID returns the ChatSession or domain.ErrNotFound.
func (r *diagnosisRepo) FindChatSessionByID(ctx context.Context, id domain.ChatSessionID) (domain.ChatSession, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSession{}, err
	}
	if id == 0 {
		return domain.ChatSession{}, fmt.Errorf("find chat session by id: id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatSession.Get(ctx, int(id))
	if err != nil {
		return domain.ChatSession{}, asNotFound(err)
	}
	return chatSessionToDomain(row), nil
}

// FindChatSessionByKey returns the ChatSession matching the external
// WebSocket/session key.
func (r *diagnosisRepo) FindChatSessionByKey(ctx context.Context, sessionKey string) (domain.ChatSession, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSession{}, err
	}
	if sessionKey == "" {
		return domain.ChatSession{}, fmt.Errorf("find chat session by key: session_key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatSession.Query().
		Where(chatsession.SessionKeyEQ(sessionKey)).
		Only(ctx)
	if err != nil {
		return domain.ChatSession{}, asNotFound(err)
	}
	return chatSessionToDomain(row), nil
}

// SaveChatTurn appends one immutable transcript row.
func (r *diagnosisRepo) SaveChatTurn(ctx context.Context, turn domain.ChatTurn) (domain.ChatTurn, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatTurn{}, err
	}
	builder := r.tx.ChatTurn.Create().
		SetChatSessionID(int(turn.SessionID)).
		SetMessageID(turn.MessageID).
		SetSequence(turn.Sequence).
		SetRole(string(turn.Role)).
		SetActorSubject(turn.ActorSubject).
		SetContent(turn.Content).
		SetOccurredAt(turn.OccurredAt)
	if len(turn.Metadata) > 0 {
		builder = builder.SetMetadata(turn.Metadata)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		return domain.ChatTurn{}, asAlreadyExists(err)
	}
	return chatTurnToDomain(saved), nil
}

// FindChatTurnBySessionAndMessageID returns the per-session idempotency hit.
func (r *diagnosisRepo) FindChatTurnBySessionAndMessageID(ctx context.Context, sessionID domain.ChatSessionID, messageID string) (domain.ChatTurn, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatTurn{}, err
	}
	if sessionID == 0 {
		return domain.ChatTurn{}, fmt.Errorf("find chat turn by message id: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if messageID == "" {
		return domain.ChatTurn{}, fmt.Errorf("find chat turn by message id: message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatTurn.Query().
		Where(
			chatturn.ChatSessionIDEQ(int(sessionID)),
			chatturn.MessageIDEQ(messageID),
		).
		Only(ctx)
	if err != nil {
		return domain.ChatTurn{}, asNotFound(err)
	}
	return chatTurnToDomain(row), nil
}

// ListChatTurnsBySession returns turns ordered by transcript sequence.
func (r *diagnosisRepo) ListChatTurnsBySession(ctx context.Context, sessionID domain.ChatSessionID, limit int) ([]domain.ChatTurn, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if sessionID == 0 {
		return nil, fmt.Errorf("list chat turns: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list chat turns: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ChatTurn.Query().
		Where(chatturn.ChatSessionIDEQ(int(sessionID))).
		Order(chatturn.BySequence()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list chat turns: %w", err)
	}
	out := make([]domain.ChatTurn, len(rows))
	for i, row := range rows {
		out[i] = chatTurnToDomain(row)
	}
	return out, nil
}
