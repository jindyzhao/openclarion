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
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsessionapproval"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsessionsummary"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatturn"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistask"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistaskevent"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisHistorySnapshotChunkSize = 10000
	diagnosisHistoryTaskChunkSize     = 5000
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

// ListTasksByEvidenceSnapshot returns recent diagnosis task executions for one
// EvidenceSnapshot. Report fan-out tasks share the lifecycle tables but carry
// a subreport.started marker and must not consume diagnosis-room history slots.
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
		Where(
			diagnosistask.EvidenceSnapshotIDEQ(int(snapshotID)),
			diagnosistask.Not(diagnosistask.HasEventsWith(
				diagnosistaskevent.KindEQ(domain.DiagnosisTaskEventKindSubReportStarted),
			)),
		).
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

// ListSnapshotHistories batches bounded task history and latest event reads
// for many EvidenceSnapshots without per-snapshot repository round trips.
func (r *diagnosisRepo) ListSnapshotHistories(
	ctx context.Context,
	snapshotIDs []domain.EvidenceSnapshotID,
	taskLimit int,
	eventKinds []string,
) ([]ports.DiagnosisSnapshotHistory, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	ids, err := normalizedDiagnosisHistorySnapshotIDs(snapshotIDs)
	if err != nil {
		return nil, err
	}
	if taskLimit <= 0 {
		return nil, fmt.Errorf("list diagnosis snapshot histories: task limit must be > 0 (got %d): %w", taskLimit, domain.ErrInvariantViolation)
	}
	kinds, err := normalizedDiagnosisHistoryEventKinds(eventKinds)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	histories := make([]ports.DiagnosisSnapshotHistory, len(ids))
	historyIndex := make(map[domain.EvidenceSnapshotID]int, len(ids))
	for i, id := range ids {
		histories[i].EvidenceSnapshotID = id
		historyIndex[id] = i
	}

	for start := 0; start < len(ids); start += diagnosisHistorySnapshotChunkSize {
		end := min(start+diagnosisHistorySnapshotChunkSize, len(ids))
		if err := r.loadDiagnosisHistoryTaskChunk(ctx, histories, historyIndex, ids[start:end], taskLimit); err != nil {
			return nil, err
		}
	}

	if len(kinds) == 0 {
		return histories, nil
	}
	taskHistoryIndex := make(map[domain.DiagnosisTaskID]int)
	taskIDs := make([]domain.DiagnosisTaskID, 0)
	for i := range histories {
		if histories[i].TasksTruncated {
			continue
		}
		for _, task := range histories[i].Tasks {
			taskHistoryIndex[task.ID] = i
			taskIDs = append(taskIDs, task.ID)
		}
	}
	for start := 0; start < len(taskIDs); start += diagnosisHistoryTaskChunkSize {
		end := min(start+diagnosisHistoryTaskChunkSize, len(taskIDs))
		events, err := r.listLatestDiagnosisEvents(ctx, taskIDs[start:end], kinds)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			index, ok := taskHistoryIndex[event.TaskID]
			if !ok {
				return nil, fmt.Errorf("list diagnosis snapshot histories: event %d references unexpected task %d: %w", event.ID, event.TaskID, domain.ErrInvariantViolation)
			}
			histories[index].LatestEvents = append(histories[index].LatestEvents, event)
		}
	}
	return histories, nil
}

type diagnosisHistoryTaskCount struct {
	EvidenceSnapshotID int `json:"evidence_snapshot_id"`
	Count              int `json:"count"`
}

func (r *diagnosisRepo) loadDiagnosisHistoryTaskChunk(
	ctx context.Context,
	histories []ports.DiagnosisSnapshotHistory,
	historyIndex map[domain.EvidenceSnapshotID]int,
	ids []domain.EvidenceSnapshotID,
	taskLimit int,
) error {
	storageIDs := evidenceSnapshotIDsToInts(ids)
	var counts []diagnosisHistoryTaskCount
	if err := r.tx.DiagnosisTask.Query().
		Where(
			diagnosistask.EvidenceSnapshotIDIn(storageIDs...),
			diagnosistask.Not(diagnosistask.HasEventsWith(
				diagnosistaskevent.KindEQ(domain.DiagnosisTaskEventKindSubReportStarted),
			)),
		).
		GroupBy(diagnosistask.FieldEvidenceSnapshotID).
		Aggregate(ent.Count()).
		Scan(ctx, &counts); err != nil {
		return fmt.Errorf("list diagnosis snapshot history task counts: %w", err)
	}
	countBySnapshotID := make(map[domain.EvidenceSnapshotID]int, len(counts))
	for _, count := range counts {
		countBySnapshotID[domain.EvidenceSnapshotID(count.EvidenceSnapshotID)] = count.Count
	}

	eligibleStorageIDs := make([]int, 0, len(ids))
	for _, id := range ids {
		if countBySnapshotID[id] > taskLimit {
			histories[historyIndex[id]].TasksTruncated = true
			continue
		}
		eligibleStorageIDs = append(eligibleStorageIDs, int(id))
	}
	if len(eligibleStorageIDs) == 0 {
		return nil
	}

	rows, err := r.tx.DiagnosisTask.Query().
		Where(
			diagnosistask.EvidenceSnapshotIDIn(eligibleStorageIDs...),
			diagnosistask.Not(diagnosistask.HasEventsWith(
				diagnosistaskevent.KindEQ(domain.DiagnosisTaskEventKindSubReportStarted),
			)),
		).
		Order(
			diagnosistask.ByEvidenceSnapshotID(),
			diagnosistask.ByCreatedAt(sql.OrderDesc()),
			diagnosistask.ByID(sql.OrderDesc()),
		).
		Limit((taskLimit + 1) * len(eligibleStorageIDs)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list diagnosis snapshot history tasks: %w", err)
	}
	for _, row := range rows {
		id := domain.EvidenceSnapshotID(row.EvidenceSnapshotID)
		index, ok := historyIndex[id]
		if !ok {
			return fmt.Errorf("list diagnosis snapshot histories: task %d references unexpected snapshot %d: %w", row.ID, id, domain.ErrInvariantViolation)
		}
		history := &histories[index]
		if len(history.Tasks) >= taskLimit {
			history.Tasks = nil
			history.TasksTruncated = true
			continue
		}
		if !history.TasksTruncated {
			history.Tasks = append(history.Tasks, diagnosisTaskToDomain(row))
		}
	}
	return nil
}

func (r *diagnosisRepo) listLatestDiagnosisEvents(
	ctx context.Context,
	taskIDs []domain.DiagnosisTaskID,
	kinds []string,
) ([]domain.DiagnosisTaskEvent, error) {
	storageTaskIDs := make([]int, len(taskIDs))
	for i, id := range taskIDs {
		storageTaskIDs[i] = int(id)
	}
	rows, err := r.tx.DiagnosisTaskEvent.Query().
		Where(
			diagnosistaskevent.TaskIDIn(storageTaskIDs...),
			diagnosistaskevent.KindIn(kinds...),
			func(selector *sql.Selector) {
				newer := sql.Table(diagnosistaskevent.Table).As("newer_diagnosis_task_events")
				selector.Where(sql.Not(sql.Exists(
					sql.Select(newer.C(diagnosistaskevent.FieldID)).
						From(newer).
						Where(sql.And(
							sql.ColumnsEQ(newer.C(diagnosistaskevent.FieldTaskID), selector.C(diagnosistaskevent.FieldTaskID)),
							sql.ColumnsEQ(newer.C(diagnosistaskevent.FieldKind), selector.C(diagnosistaskevent.FieldKind)),
							sql.Or(
								sql.ColumnsGT(newer.C(diagnosistaskevent.FieldOccurredAt), selector.C(diagnosistaskevent.FieldOccurredAt)),
								sql.And(
									sql.ColumnsEQ(newer.C(diagnosistaskevent.FieldOccurredAt), selector.C(diagnosistaskevent.FieldOccurredAt)),
									sql.ColumnsGT(newer.C(diagnosistaskevent.FieldID), selector.C(diagnosistaskevent.FieldID)),
								),
							),
						)),
				)))
			},
		).
		Order(
			diagnosistaskevent.ByTaskID(),
			diagnosistaskevent.ByKind(),
			diagnosistaskevent.ByOccurredAt(sql.OrderDesc()),
			diagnosistaskevent.ByID(sql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list latest diagnosis snapshot history events: %w", err)
	}
	out := make([]domain.DiagnosisTaskEvent, len(rows))
	for i, row := range rows {
		out[i] = diagnosisTaskEventToDomain(row)
	}
	return out, nil
}

func normalizedDiagnosisHistorySnapshotIDs(ids []domain.EvidenceSnapshotID) ([]domain.EvidenceSnapshotID, error) {
	out := make([]domain.EvidenceSnapshotID, 0, len(ids))
	seen := make(map[domain.EvidenceSnapshotID]struct{}, len(ids))
	for i, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("list diagnosis snapshot histories: snapshot_ids[%d] must be positive: %w", i, domain.ErrInvariantViolation)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func normalizedDiagnosisHistoryEventKinds(kinds []string) ([]string, error) {
	out := make([]string, 0, len(kinds))
	seen := make(map[string]struct{}, len(kinds))
	for i, kind := range kinds {
		trimmed := strings.TrimSpace(kind)
		if trimmed == "" || trimmed != kind {
			return nil, fmt.Errorf("list diagnosis snapshot histories: event_kinds[%d] must be canonical non-empty text: %w", i, domain.ErrInvariantViolation)
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	return out, nil
}

func evidenceSnapshotIDsToInts(ids []domain.EvidenceSnapshotID) []int {
	out := make([]int, len(ids))
	for i, id := range ids {
		out[i] = int(id)
	}
	return out
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

// ListEvents returns the events for a task ordered by occurred_at and ID
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
		Order(
			diagnosistaskevent.ByOccurredAt(),
			diagnosistaskevent.ByID(),
		).
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
	if s.ApprovalMode != "" {
		builder = builder.SetApprovalMode(string(s.ApprovalMode))
	}
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

// ListChatSessions returns recent operator-facing room lifecycle rows with
// their backing DiagnosisTask loaded for evidence/workflow references.
func (r *diagnosisRepo) ListChatSessions(ctx context.Context, limit int) ([]domain.ChatSessionWithTask, error) {
	return r.ListChatSessionsPage(ctx, limit, 0)
}

// ListChatSessionsPage returns a deterministic page of operator-facing room
// lifecycle rows with their backing DiagnosisTask loaded.
func (r *diagnosisRepo) ListChatSessionsPage(ctx context.Context, limit int, offset int) ([]domain.ChatSessionWithTask, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list chat sessions: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	if offset < 0 {
		return nil, fmt.Errorf("list chat sessions: offset must be >= 0 (got %d): %w", offset, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ChatSession.Query().
		WithTask().
		Order(
			chatsession.ByUpdatedAt(sql.OrderDesc()),
			chatsession.ByID(sql.OrderDesc()),
		).
		Offset(offset).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list chat sessions: %w", err)
	}
	out := make([]domain.ChatSessionWithTask, len(rows))
	for i, row := range rows {
		task, err := row.Edges.TaskOrErr()
		if err != nil {
			return nil, fmt.Errorf("list chat sessions: load task: %w", err)
		}
		out[i] = domain.ChatSessionWithTask{
			Session: chatSessionToDomain(row),
			Task:    diagnosisTaskToDomain(task),
		}
	}
	return out, nil
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

// SaveChatSessionSummary appends one immutable conversation summary.
func (r *diagnosisRepo) SaveChatSessionSummary(ctx context.Context, summary domain.ChatSessionSummary) (domain.ChatSessionSummary, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSessionSummary{}, err
	}
	saved, err := r.tx.ChatSessionSummary.Create().
		SetChatSessionID(int(summary.SessionID)).
		SetVersion(summary.Version).
		SetSchemaVersion(summary.SchemaVersion).
		SetSourceFirstSequence(summary.SourceFirstSequence).
		SetSourceLastSequence(summary.SourceLastSequence).
		SetSourceTurnCount(summary.SourceTurnCount).
		SetSourceDigest(summary.SourceDigest).
		SetContent(summary.Content).
		SetGeneratedAt(summary.GeneratedAt).
		Save(ctx)
	if err != nil {
		return domain.ChatSessionSummary{}, asAlreadyExists(err)
	}
	return chatSessionSummaryToDomain(saved), nil
}

// FindChatSessionSummaryBySessionAndVersion returns one exact checkpoint.
func (r *diagnosisRepo) FindChatSessionSummaryBySessionAndVersion(ctx context.Context, sessionID domain.ChatSessionID, version int) (domain.ChatSessionSummary, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSessionSummary{}, err
	}
	if sessionID == 0 {
		return domain.ChatSessionSummary{}, fmt.Errorf("find chat session summary by version: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if version <= 0 {
		return domain.ChatSessionSummary{}, fmt.Errorf("find chat session summary by version: version must be > 0 (got %d): %w", version, domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatSessionSummary.Query().
		Where(
			chatsessionsummary.ChatSessionIDEQ(int(sessionID)),
			chatsessionsummary.VersionEQ(version),
		).
		Only(ctx)
	if err != nil {
		return domain.ChatSessionSummary{}, asNotFound(err)
	}
	return chatSessionSummaryToDomain(row), nil
}

// FindLatestChatSessionSummary returns the highest persisted revision.
func (r *diagnosisRepo) FindLatestChatSessionSummary(ctx context.Context, sessionID domain.ChatSessionID) (domain.ChatSessionSummary, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSessionSummary{}, err
	}
	if sessionID == 0 {
		return domain.ChatSessionSummary{}, fmt.Errorf("find latest chat session summary: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatSessionSummary.Query().
		Where(chatsessionsummary.ChatSessionIDEQ(int(sessionID))).
		Order(
			chatsessionsummary.ByVersion(sql.OrderDesc()),
			chatsessionsummary.ByID(sql.OrderDesc()),
		).
		First(ctx)
	if err != nil {
		return domain.ChatSessionSummary{}, asNotFound(err)
	}
	return chatSessionSummaryToDomain(row), nil
}

// SaveChatSessionApproval appends one immutable stakeholder approval.
func (r *diagnosisRepo) SaveChatSessionApproval(ctx context.Context, approval domain.ChatSessionApproval) (domain.ChatSessionApproval, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSessionApproval{}, err
	}
	saved, err := r.tx.ChatSessionApproval.Create().
		SetChatSessionID(int(approval.SessionID)).
		SetConclusionDigest(approval.ConclusionDigest).
		SetActorSubject(approval.ActorSubject).
		SetAuthority(string(approval.Authority)).
		SetReason(approval.Reason).
		SetApprovedAt(approval.ApprovedAt).
		Save(ctx)
	if err != nil {
		return domain.ChatSessionApproval{}, asAlreadyExists(err)
	}
	return chatSessionApprovalToDomain(saved), nil
}

// FindChatSessionApproval returns an exact actor/conclusion approval.
func (r *diagnosisRepo) FindChatSessionApproval(ctx context.Context, sessionID domain.ChatSessionID, conclusionDigest, actorSubject string) (domain.ChatSessionApproval, error) {
	if err := checkOpen(r.closed); err != nil {
		return domain.ChatSessionApproval{}, err
	}
	conclusionDigest = strings.TrimSpace(conclusionDigest)
	actorSubject = strings.TrimSpace(actorSubject)
	if sessionID == 0 || conclusionDigest == "" || actorSubject == "" {
		return domain.ChatSessionApproval{}, fmt.Errorf("find chat session approval: session_id, conclusion_digest, and actor_subject must be set: %w", domain.ErrInvariantViolation)
	}
	row, err := r.tx.ChatSessionApproval.Query().
		Where(
			chatsessionapproval.ChatSessionIDEQ(int(sessionID)),
			chatsessionapproval.ConclusionDigestEQ(conclusionDigest),
			chatsessionapproval.ActorSubjectEQ(actorSubject),
		).
		Only(ctx)
	if err != nil {
		return domain.ChatSessionApproval{}, asNotFound(err)
	}
	return chatSessionApprovalToDomain(row), nil
}

// HasChatSessionApprovals reports whether a session has any approval history.
func (r *diagnosisRepo) HasChatSessionApprovals(ctx context.Context, sessionID domain.ChatSessionID) (bool, error) {
	if err := checkOpen(r.closed); err != nil {
		return false, err
	}
	if sessionID == 0 {
		return false, fmt.Errorf("has chat session approvals: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	exists, err := r.tx.ChatSessionApproval.Query().
		Where(chatsessionapproval.ChatSessionIDEQ(int(sessionID))).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("has chat session approvals: %w", err)
	}
	return exists, nil
}

// ListChatSessionApprovals returns the approval quorum for one conclusion.
func (r *diagnosisRepo) ListChatSessionApprovals(ctx context.Context, sessionID domain.ChatSessionID, conclusionDigest string, limit int) ([]domain.ChatSessionApproval, error) {
	if err := checkOpen(r.closed); err != nil {
		return nil, err
	}
	conclusionDigest = strings.TrimSpace(conclusionDigest)
	if sessionID == 0 || conclusionDigest == "" {
		return nil, fmt.Errorf("list chat session approvals: session_id and conclusion_digest must be set: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("list chat session approvals: limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	rows, err := r.tx.ChatSessionApproval.Query().
		Where(
			chatsessionapproval.ChatSessionIDEQ(int(sessionID)),
			chatsessionapproval.ConclusionDigestEQ(conclusionDigest),
		).
		Order(
			chatsessionapproval.ByApprovedAt(),
			chatsessionapproval.ByID(),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list chat session approvals: %w", err)
	}
	out := make([]domain.ChatSessionApproval, len(rows))
	for i, row := range rows {
		out[i] = chatSessionApprovalToDomain(row)
	}
	return out, nil
}
