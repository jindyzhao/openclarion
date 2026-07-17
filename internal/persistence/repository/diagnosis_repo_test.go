package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// makeSnapshotForDiagnosis seeds an AlertGroup + EvidenceSnapshot
// because DiagnosisTask has a NOT NULL FK on evidence_snapshot_id.
func makeSnapshotForDiagnosis(t *testing.T, label string) domain.EvidenceSnapshotID {
	t.Helper()
	groupID := makeGroupForEvidence(t, "diag-grp-"+label)
	var snapID domain.EvidenceSnapshotID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		s, err := uow.Evidence().Save(ctx, mustNewSnapshot(t, groupID, "diag-digest-"+label))
		if err != nil {
			t.Fatalf("Save snapshot: %v", err)
		}
		snapID = s.ID
	})
	return snapID
}

func mustNewTask(t *testing.T, snapID domain.EvidenceSnapshotID, wfID, runID string) domain.DiagnosisTask {
	t.Helper()
	task, err := domain.NewDiagnosisTask(snapID, wfID, runID)
	if err != nil {
		t.Fatalf("NewDiagnosisTask: %v", err)
	}
	return task
}

func makeDiagnosisTaskForChat(t *testing.T, label string) domain.DiagnosisTaskID {
	t.Helper()
	snapID := makeSnapshotForDiagnosis(t, "chat-"+label)
	var taskID domain.DiagnosisTaskID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		task, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapID, "wf-chat-"+label, "run-chat-"+label))
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		taskID = task.ID
	})
	return taskID
}

func makeChatSessionForDiagnosis(t *testing.T, taskID domain.DiagnosisTaskID, sessionKey string) domain.ChatSession {
	t.Helper()
	startedAt := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	session, err := domain.NewChatSession(taskID, sessionKey, "owner-1", startedAt)
	if err != nil {
		t.Fatalf("NewChatSession: %v", err)
	}
	return session
}

func TestDiagnosisRepository_SaveTaskAndQuery(t *testing.T) {
	resetDB(t)
	snapID := makeSnapshotForDiagnosis(t, "save")

	var saved domain.DiagnosisTask
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapID, "wf-1", "run-1"))
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Errorf("saved.ID = 0, want non-zero")
	}
	if saved.Status != domain.DiagnosisStatusPending {
		t.Errorf("saved.Status = %q, want pending", saved.Status)
	}
	if saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Errorf("CreatedAt/UpdatedAt = (%v,%v), want non-zero", saved.CreatedAt, saved.UpdatedAt)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Diagnosis().FindTaskByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindTaskByID: %v", err)
		}
		if byID.WorkflowID != "wf-1" {
			t.Errorf("FindTaskByID.WorkflowID = %q, want wf-1", byID.WorkflowID)
		}
		byExec, err := uow.Diagnosis().FindTaskByExecution(ctx, "wf-1", "run-1")
		if err != nil {
			t.Fatalf("FindTaskByExecution: %v", err)
		}
		if byExec.ID != saved.ID {
			t.Errorf("FindTaskByExecution.ID = %d, want %d", byExec.ID, saved.ID)
		}
	})
}

func TestDiagnosisRepository_SaveTask_DuplicateExecutionKey(t *testing.T) {
	resetDB(t)
	snapID := makeSnapshotForDiagnosis(t, "dup")
	base := mustNewTask(t, snapID, "wf-dup", "run-dup")

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Diagnosis().SaveTask(ctx, base); err != nil {
			t.Fatalf("first SaveTask: %v", err)
		}
	})
	ctx := context.Background()
	err := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Diagnosis().SaveTask(ctx, base)
		return serr
	})
	if err == nil {
		t.Fatalf("second SaveTask: want error, got nil")
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("second SaveTask: want errors.Is ErrAlreadyExists, got %v", err)
	}
}

func TestDiagnosisRepository_UpdateTaskTransitions(t *testing.T) {
	resetDB(t)
	snapID := makeSnapshotForDiagnosis(t, "tx")

	startedAt := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)

	var saved domain.DiagnosisTask
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapID, "wf-tx", "run-tx"))
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		saved = got
	})

	running, err := saved.Start(startedAt)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	finished, err := running.Finish(domain.DiagnosisStatusSucceeded, finishedAt, "")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Apply the running transition first to exercise the
		// non-nil StartedAt / nil FinishedAt branch of UpdateTask.
		if _, err := uow.Diagnosis().UpdateTask(ctx, running); err != nil {
			t.Fatalf("UpdateTask running: %v", err)
		}
		// Then the terminal transition.
		out, err := uow.Diagnosis().UpdateTask(ctx, finished)
		if err != nil {
			t.Fatalf("UpdateTask finished: %v", err)
		}
		if out.Status != domain.DiagnosisStatusSucceeded {
			t.Errorf("UpdateTask.Status = %q, want succeeded", out.Status)
		}
		if out.FinishedAt == nil || !out.FinishedAt.Equal(domain.NormalizeUTCMicro(finishedAt)) {
			t.Errorf("UpdateTask.FinishedAt = %v, want %v", out.FinishedAt, finishedAt)
		}
	})
}

func TestDiagnosisRepository_AppendEventAndList(t *testing.T) {
	resetDB(t)
	snapID := makeSnapshotForDiagnosis(t, "ev")

	var taskID domain.DiagnosisTaskID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		t0, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapID, "wf-ev", "run-ev"))
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		taskID = t0.ID
	})

	occurred := time.Date(2026, 5, 22, 15, 0, 0, 0, time.UTC)

	// Two events with deliberately reversed insertion order; ListEvents
	// MUST return them sorted by occurred_at ascending.
	dedupe := "user-action-1"
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		later, err := domain.NewDiagnosisTaskEvent(taskID, "later", json.RawMessage(`{"i":2}`), nil, occurred.Add(time.Minute))
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent later: %v", err)
		}
		earlier, err := domain.NewDiagnosisTaskEvent(taskID, "earlier", json.RawMessage(`{"i":1}`), &dedupe, occurred)
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent earlier: %v", err)
		}
		if _, err := uow.Diagnosis().AppendEvent(ctx, later); err != nil {
			t.Fatalf("AppendEvent later: %v", err)
		}
		if _, err := uow.Diagnosis().AppendEvent(ctx, earlier); err != nil {
			t.Fatalf("AppendEvent earlier: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Diagnosis().ListEvents(ctx, taskID, 10)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("ListEvents len = %d, want 2", len(out))
		}
		if out[0].Kind != "earlier" || out[1].Kind != "later" {
			t.Errorf("ListEvents kinds = [%s,%s], want [earlier,later]", out[0].Kind, out[1].Kind)
		}
		if out[0].DedupeKey == nil || *out[0].DedupeKey != dedupe {
			t.Errorf("earlier.DedupeKey = %v, want %q", out[0].DedupeKey, dedupe)
		}
	})

	// Re-appending the same dedupe key MUST collide; nil dedupe keys
	// MUST coexist (Postgres multi-NULL UNIQUE semantics).
	ctx := context.Background()
	dupErr := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		dup, err := domain.NewDiagnosisTaskEvent(taskID, "earlier", json.RawMessage(`{}`), &dedupe, occurred)
		if err != nil {
			return err
		}
		_, serr := uow.Diagnosis().AppendEvent(ctx, dup)
		return serr
	})
	if !errors.Is(dupErr, domain.ErrAlreadyExists) {
		t.Fatalf("AppendEvent duplicate dedupe: want errors.Is ErrAlreadyExists, got %v", dupErr)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Two more events with nil dedupe key MUST both succeed.
		for _, kind := range []string{"nil-dup-1", "nil-dup-2"} {
			ev, err := domain.NewDiagnosisTaskEvent(taskID, kind, nil, nil, occurred.Add(2*time.Minute))
			if err != nil {
				t.Fatalf("NewDiagnosisTaskEvent %s: %v", kind, err)
			}
			if _, err := uow.Diagnosis().AppendEvent(ctx, ev); err != nil {
				t.Fatalf("AppendEvent %s: %v", kind, err)
			}
		}
	})
}

func TestDiagnosisRepository_ListTasksByEvidenceSnapshot(t *testing.T) {
	resetDB(t)
	snapAID := makeSnapshotForDiagnosis(t, "task-list-a")
	snapBID := makeSnapshotForDiagnosis(t, "task-list-b")

	var firstA, secondA domain.DiagnosisTask
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		firstA, err = uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapAID, "wf-task-list-a1", "run-task-list-a1"))
		if err != nil {
			t.Fatalf("SaveTask firstA: %v", err)
		}
	})
	time.Sleep(2 * time.Millisecond)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		secondA, err = uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapAID, "wf-task-list-a2", "run-task-list-a2"))
		if err != nil {
			t.Fatalf("SaveTask secondA: %v", err)
		}
		if _, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapBID, "wf-task-list-b1", "run-task-list-b1")); err != nil {
			t.Fatalf("SaveTask other snapshot: %v", err)
		}
		reportTask, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapAID, "wf-task-list-report", "run-task-list-report"))
		if err != nil {
			t.Fatalf("SaveTask report task: %v", err)
		}
		markerKind := domain.DiagnosisTaskEventKindSubReportStarted
		marker, err := domain.NewDiagnosisTaskEvent(
			reportTask.ID,
			markerKind,
			json.RawMessage(`{"source":"ReportFanOutWorkflow"}`),
			&markerKind,
			time.Now(),
		)
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent report marker: %v", err)
		}
		if _, err := uow.Diagnosis().AppendEvent(ctx, marker); err != nil {
			t.Fatalf("AppendEvent report marker: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Diagnosis().ListTasksByEvidenceSnapshot(ctx, snapAID, 10)
		if err != nil {
			t.Fatalf("ListTasksByEvidenceSnapshot: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("ListTasksByEvidenceSnapshot len = %d, want 2", len(out))
		}
		if out[0].ID != secondA.ID || out[1].ID != firstA.ID {
			t.Fatalf("ListTasksByEvidenceSnapshot IDs = [%d,%d], want [%d,%d]", out[0].ID, out[1].ID, secondA.ID, firstA.ID)
		}
		limited, err := uow.Diagnosis().ListTasksByEvidenceSnapshot(ctx, snapAID, 1)
		if err != nil {
			t.Fatalf("ListTasksByEvidenceSnapshot limit: %v", err)
		}
		if len(limited) != 1 || limited[0].ID != secondA.ID {
			t.Fatalf("limited tasks = %+v, want only %d", limited, secondA.ID)
		}
	})

	for _, tc := range []struct {
		name     string
		snapshot domain.EvidenceSnapshotID
		limit    int
	}{
		{name: "zero snapshot", snapshot: 0, limit: 1},
		{name: "zero limit", snapshot: snapAID, limit: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
				_, err := uow.Diagnosis().ListTasksByEvidenceSnapshot(ctx, tc.snapshot, tc.limit)
				return err
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("ListTasksByEvidenceSnapshot error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestDiagnosisRepository_ListSnapshotHistories(t *testing.T) {
	resetDB(t)
	snapAID := makeSnapshotForDiagnosis(t, "history-a")
	snapBID := makeSnapshotForDiagnosis(t, "history-b")
	snapEmptyID := makeSnapshotForDiagnosis(t, "history-empty")

	var taskB domain.DiagnosisTask
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for i := 1; i <= 2; i++ {
			if _, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(
				t,
				snapAID,
				fmt.Sprintf("wf-history-a%d", i),
				fmt.Sprintf("run-history-a%d", i),
			)); err != nil {
				t.Fatalf("SaveTask snapshot A %d: %v", i, err)
			}
		}
		var err error
		taskB, err = uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapBID, "wf-history-b", "run-history-b"))
		if err != nil {
			t.Fatalf("SaveTask snapshot B: %v", err)
		}
		reportTask, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapBID, "wf-history-report", "run-history-report"))
		if err != nil {
			t.Fatalf("SaveTask snapshot B report task: %v", err)
		}
		markerKind := domain.DiagnosisTaskEventKindSubReportStarted
		marker, err := domain.NewDiagnosisTaskEvent(
			reportTask.ID,
			markerKind,
			json.RawMessage(`{"source":"ReportFanOutWorkflow"}`),
			&markerKind,
			time.Now(),
		)
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent history report marker: %v", err)
		}
		if _, err := uow.Diagnosis().AppendEvent(ctx, marker); err != nil {
			t.Fatalf("AppendEvent history report marker: %v", err)
		}
	})

	base := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, spec := range []struct {
			kind string
			body string
			when time.Time
		}{
			{kind: "target", body: `{"sequence":2}`, when: base.Add(2 * time.Minute)},
			{kind: "other", body: `{"sequence":3}`, when: base.Add(3 * time.Minute)},
			{kind: "target", body: `{"sequence":1}`, when: base.Add(time.Minute)},
		} {
			event, err := domain.NewDiagnosisTaskEvent(taskB.ID, spec.kind, json.RawMessage(spec.body), nil, spec.when)
			if err != nil {
				t.Fatalf("NewDiagnosisTaskEvent %s: %v", spec.kind, err)
			}
			if _, err := uow.Diagnosis().AppendEvent(ctx, event); err != nil {
				t.Fatalf("AppendEvent %s: %v", spec.kind, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		histories, err := uow.Diagnosis().ListSnapshotHistories(
			ctx,
			[]domain.EvidenceSnapshotID{snapBID, snapAID, snapBID, snapEmptyID},
			1,
			[]string{"target", "other", "target"},
		)
		if err != nil {
			t.Fatalf("ListSnapshotHistories: %v", err)
		}
		if len(histories) != 3 ||
			histories[0].EvidenceSnapshotID != snapBID ||
			histories[1].EvidenceSnapshotID != snapAID ||
			histories[2].EvidenceSnapshotID != snapEmptyID {
			t.Fatalf("history order = %+v, want B, A, empty with duplicate removed", histories)
		}
		if len(histories[0].Tasks) != 1 || histories[0].Tasks[0].ID != taskB.ID || histories[0].TasksTruncated {
			t.Fatalf("snapshot B history = %+v, want one complete task", histories[0])
		}
		if len(histories[0].LatestEvents) != 2 {
			t.Fatalf("snapshot B latest events = %+v, want one event per requested kind", histories[0].LatestEvents)
		}
		latestByKind := make(map[string]domain.DiagnosisTaskEvent, len(histories[0].LatestEvents))
		for _, event := range histories[0].LatestEvents {
			latestByKind[event.Kind] = event
		}
		if !latestByKind["target"].OccurredAt.Equal(base.Add(2*time.Minute)) ||
			!latestByKind["other"].OccurredAt.Equal(base.Add(3*time.Minute)) {
			t.Fatalf("latest events = %+v, want latest occurred_at for each kind", latestByKind)
		}
		if !histories[1].TasksTruncated || len(histories[1].Tasks) != 0 || len(histories[1].LatestEvents) != 0 {
			t.Fatalf("snapshot A history = %+v, want explicit truncation without partial data", histories[1])
		}
		if histories[2].TasksTruncated || len(histories[2].Tasks) != 0 || len(histories[2].LatestEvents) != 0 {
			t.Fatalf("empty snapshot history = %+v, want complete empty history", histories[2])
		}
	})

	for _, tc := range []struct {
		name  string
		ids   []domain.EvidenceSnapshotID
		limit int
		kinds []string
	}{
		{name: "zero snapshot", ids: []domain.EvidenceSnapshotID{0}, limit: 1, kinds: []string{"target"}},
		{name: "zero limit", ids: []domain.EvidenceSnapshotID{snapAID}, limit: 0, kinds: []string{"target"}},
		{name: "blank kind", ids: []domain.EvidenceSnapshotID{snapAID}, limit: 1, kinds: []string{" "}},
		{name: "non canonical kind", ids: []domain.EvidenceSnapshotID{snapAID}, limit: 1, kinds: []string{" target"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
				_, err := uow.Diagnosis().ListSnapshotHistories(ctx, tc.ids, tc.limit, tc.kinds)
				return err
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("ListSnapshotHistories error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestDiagnosisRepository_ListEventsByTaskAndKind(t *testing.T) {
	resetDB(t)
	snapID := makeSnapshotForDiagnosis(t, "event-kind")

	var taskID domain.DiagnosisTaskID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		task, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapID, "wf-event-kind", "run-event-kind"))
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		taskID = task.ID
	})

	occurred := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		for _, spec := range []struct {
			kind string
			when time.Time
			body string
		}{
			{kind: "target", when: occurred, body: `{"i":1}`},
			{kind: "other", when: occurred.Add(30 * time.Second), body: `{"i":99}`},
			{kind: "target", when: occurred.Add(time.Minute), body: `{"i":2}`},
		} {
			ev, err := domain.NewDiagnosisTaskEvent(taskID, spec.kind, json.RawMessage(spec.body), nil, spec.when)
			if err != nil {
				t.Fatalf("NewDiagnosisTaskEvent %s: %v", spec.kind, err)
			}
			if _, err := uow.Diagnosis().AppendEvent(ctx, ev); err != nil {
				t.Fatalf("AppendEvent %s: %v", spec.kind, err)
			}
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, taskID, " target ", 10)
		if err != nil {
			t.Fatalf("ListEventsByTaskAndKind: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("ListEventsByTaskAndKind len = %d, want 2", len(out))
		}
		if !out[0].OccurredAt.After(out[1].OccurredAt) {
			t.Fatalf("events not in descending occurred_at order: %+v", out)
		}
		limited, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, taskID, "target", 1)
		if err != nil {
			t.Fatalf("ListEventsByTaskAndKind limit: %v", err)
		}
		if len(limited) != 1 || !limited[0].OccurredAt.Equal(occurred.Add(time.Minute)) {
			t.Fatalf("limited events = %+v, want latest target event", limited)
		}
	})

	for _, tc := range []struct {
		name   string
		taskID domain.DiagnosisTaskID
		kind   string
		limit  int
	}{
		{name: "zero task", taskID: 0, kind: "target", limit: 1},
		{name: "blank kind", taskID: taskID, kind: " ", limit: 1},
		{name: "zero limit", taskID: taskID, kind: "target", limit: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
				_, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, tc.taskID, tc.kind, tc.limit)
				return err
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("ListEventsByTaskAndKind error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

// TestDiagnosisRepository_FindEventByTaskAndDedupeKey covers the
// idempotent producer pattern relied on by the Temporal Activity:
// hit, miss, cross-task isolation, and the two invariant guards.
func TestDiagnosisRepository_FindEventByTaskAndDedupeKey(t *testing.T) {
	resetDB(t)
	snapAID := makeSnapshotForDiagnosis(t, "find-a")
	snapBID := makeSnapshotForDiagnosis(t, "find-b")

	var taskA, taskB domain.DiagnosisTaskID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		tA, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapAID, "wf-find-a", "run-find-a"))
		if err != nil {
			t.Fatalf("SaveTask A: %v", err)
		}
		tB, err := uow.Diagnosis().SaveTask(ctx, mustNewTask(t, snapBID, "wf-find-b", "run-find-b"))
		if err != nil {
			t.Fatalf("SaveTask B: %v", err)
		}
		taskA, taskB = tA.ID, tB.ID
	})

	occurred := time.Date(2026, 5, 22, 16, 0, 0, 0, time.UTC)
	sharedKey := "shared-key"
	onlyOnA := "only-on-a"

	// Same dedupe_key on both tasks must coexist: the UNIQUE index
	// is (task_id, dedupe_key), not dedupe_key alone.
	var idA, idB domain.DiagnosisTaskEventID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		evA, err := domain.NewDiagnosisTaskEvent(taskA, "shared-on-a", json.RawMessage(`{}`), &sharedKey, occurred)
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent A: %v", err)
		}
		savedA, err := uow.Diagnosis().AppendEvent(ctx, evA)
		if err != nil {
			t.Fatalf("AppendEvent A: %v", err)
		}
		idA = savedA.ID

		evB, err := domain.NewDiagnosisTaskEvent(taskB, "shared-on-b", json.RawMessage(`{}`), &sharedKey, occurred)
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent B: %v", err)
		}
		savedB, err := uow.Diagnosis().AppendEvent(ctx, evB)
		if err != nil {
			t.Fatalf("AppendEvent B: %v", err)
		}
		idB = savedB.ID

		onlyEv, err := domain.NewDiagnosisTaskEvent(taskA, "only-a", json.RawMessage(`{}`), &onlyOnA, occurred.Add(time.Second))
		if err != nil {
			t.Fatalf("NewDiagnosisTaskEvent only-a: %v", err)
		}
		if _, err := uow.Diagnosis().AppendEvent(ctx, onlyEv); err != nil {
			t.Fatalf("AppendEvent only-a: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		// Hit on task A returns the A row, not the B row.
		gotA, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskA, sharedKey)
		if err != nil {
			t.Fatalf("FindEventByTaskAndDedupeKey A: %v", err)
		}
		if gotA.ID != idA || gotA.Kind != "shared-on-a" {
			t.Fatalf("hit A returned %+v, want id=%d kind=shared-on-a", gotA, idA)
		}

		// Hit on task B returns the B row even though they share key.
		gotB, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskB, sharedKey)
		if err != nil {
			t.Fatalf("FindEventByTaskAndDedupeKey B: %v", err)
		}
		if gotB.ID != idB || gotB.Kind != "shared-on-b" {
			t.Fatalf("hit B returned %+v, want id=%d kind=shared-on-b", gotB, idB)
		}

		// Cross-task lookup must miss: the only-on-a key must not
		// resolve under task B.
		_, err = uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskB, onlyOnA)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("cross-task lookup: want ErrNotFound, got %v", err)
		}

		// Plain miss.
		_, err = uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskA, "never-inserted")
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("miss: want ErrNotFound, got %v", err)
		}

		// Empty dedupe_key is a misuse, not a lookup.
		_, err = uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskA, "")
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("empty dedupe_key: want ErrInvariantViolation, got %v", err)
		}

		// Zero task id is a misuse.
		_, err = uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, 0, sharedKey)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("zero task id: want ErrInvariantViolation, got %v", err)
		}
	})
}

func TestDiagnosisRepository_SaveChatSessionAndQuery(t *testing.T) {
	resetDB(t)
	taskID := makeDiagnosisTaskForChat(t, "session")
	session := makeChatSessionForDiagnosis(t, taskID, "session-1")
	session.ApprovalMode = domain.DiagnosisApprovalModeOwnerAndLeader

	var saved domain.ChatSession
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().SaveChatSession(ctx, session)
		if err != nil {
			t.Fatalf("SaveChatSession: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 {
		t.Fatalf("saved.ID = 0, want non-zero")
	}
	if saved.Status != domain.ChatSessionStatusOpen || saved.TurnCount != 0 || saved.ApprovalMode != domain.DiagnosisApprovalModeOwnerAndLeader {
		t.Fatalf("saved status/count/mode = (%q,%d,%q), want (open,0,owner_and_leader)", saved.Status, saved.TurnCount, saved.ApprovalMode)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byID, err := uow.Diagnosis().FindChatSessionByID(ctx, saved.ID)
		if err != nil {
			t.Fatalf("FindChatSessionByID: %v", err)
		}
		if byID.SessionKey != "session-1" || byID.OwnerSubject != "owner-1" {
			t.Fatalf("FindChatSessionByID = %+v", byID)
		}
		byKey, err := uow.Diagnosis().FindChatSessionByKey(ctx, "session-1")
		if err != nil {
			t.Fatalf("FindChatSessionByKey: %v", err)
		}
		if byKey.ID != saved.ID {
			t.Fatalf("FindChatSessionByKey.ID = %d, want %d", byKey.ID, saved.ID)
		}
	})

	advanced, err := saved.RecordTurn(saved.StartedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("RecordTurn: %v", err)
	}
	closed, err := advanced.Close(saved.StartedAt.Add(5*time.Minute), "user_requested")
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().UpdateChatSession(ctx, closed)
		if err != nil {
			t.Fatalf("UpdateChatSession: %v", err)
		}
		if got.Status != domain.ChatSessionStatusClosed || got.ClosedAt == nil || got.TurnCount != 1 {
			t.Fatalf("updated session = %+v", got)
		}
	})
}

func TestDiagnosisRepository_SaveChatSession_DuplicateKeys(t *testing.T) {
	resetDB(t)
	taskA := makeDiagnosisTaskForChat(t, "dup-a")
	taskB := makeDiagnosisTaskForChat(t, "dup-b")

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskA, "session-dup")); err != nil {
			t.Fatalf("SaveChatSession first: %v", err)
		}
	})

	ctx := context.Background()
	dupKeyErr := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskB, "session-dup"))
		return err
	})
	if !errors.Is(dupKeyErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate session_key: want ErrAlreadyExists, got %v", dupKeyErr)
	}

	dupTaskErr := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskA, "session-dup-task"))
		return err
	})
	if !errors.Is(dupTaskErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate diagnosis_task_id: want ErrAlreadyExists, got %v", dupTaskErr)
	}
}

func TestDiagnosisRepository_ListChatSessions(t *testing.T) {
	resetDB(t)
	taskA := makeDiagnosisTaskForChat(t, "list-a")
	taskB := makeDiagnosisTaskForChat(t, "list-b")

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskA, "session-list-a")); err != nil {
			t.Fatalf("SaveChatSession A: %v", err)
		}
		if _, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskB, "session-list-b")); err != nil {
			t.Fatalf("SaveChatSession B: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		out, err := uow.Diagnosis().ListChatSessions(ctx, 1)
		if err != nil {
			t.Fatalf("ListChatSessions: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("ListChatSessions len = %d, want 1", len(out))
		}
		if out[0].Session.SessionKey != "session-list-b" ||
			out[0].Session.DiagnosisTaskID != taskB ||
			out[0].Task.ID != taskB ||
			out[0].Task.EvidenceSnapshotID == 0 {
			t.Fatalf("ListChatSessions[0] = %+v", out[0])
		}

		paged, err := uow.Diagnosis().ListChatSessionsPage(ctx, 1, 1)
		if err != nil {
			t.Fatalf("ListChatSessionsPage: %v", err)
		}
		if len(paged) != 1 || paged[0].Session.SessionKey != "session-list-a" {
			t.Fatalf("ListChatSessionsPage = %+v, want second session", paged)
		}

		_, err = uow.Diagnosis().ListChatSessions(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ListChatSessions zero limit: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().ListChatSessionsPage(ctx, 1, -1)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ListChatSessionsPage negative offset: want ErrInvariantViolation, got %v", err)
		}
	})
}

func TestDiagnosisRepository_SaveChatTurnAndList(t *testing.T) {
	resetDB(t)
	taskID := makeDiagnosisTaskForChat(t, "turn")

	var sessionID domain.ChatSessionID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		session, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskID, "session-turn"))
		if err != nil {
			t.Fatalf("SaveChatSession: %v", err)
		}
		sessionID = session.ID
	})

	occurred := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	turn := func(messageID string, sequence int, role domain.ChatRole, content string) domain.ChatTurn {
		t.Helper()
		out, err := domain.NewChatTurn(domain.ChatTurn{
			SessionID:    sessionID,
			MessageID:    messageID,
			Sequence:     sequence,
			Role:         role,
			ActorSubject: "owner-1",
			Content:      content,
			Metadata:     json.RawMessage(`{"source":"test"}`),
			OccurredAt:   occurred.Add(time.Duration(sequence) * time.Second),
		})
		if err != nil {
			t.Fatalf("NewChatTurn: %v", err)
		}
		return out
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Diagnosis().SaveChatTurn(ctx, turn("assistant-1", 2, domain.ChatRoleAssistant, "diagnosis")); err != nil {
			t.Fatalf("SaveChatTurn assistant: %v", err)
		}
		if _, err := uow.Diagnosis().SaveChatTurn(ctx, turn("user-1", 1, domain.ChatRoleUser, "what happened?")); err != nil {
			t.Fatalf("SaveChatTurn user: %v", err)
		}
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().FindChatTurnBySessionAndMessageID(ctx, sessionID, "user-1")
		if err != nil {
			t.Fatalf("FindChatTurnBySessionAndMessageID: %v", err)
		}
		if got.Sequence != 1 || got.Role != domain.ChatRoleUser {
			t.Fatalf("found turn = %+v", got)
		}
		out, err := uow.Diagnosis().ListChatTurnsBySession(ctx, sessionID, 10)
		if err != nil {
			t.Fatalf("ListChatTurnsBySession: %v", err)
		}
		if len(out) != 2 || out[0].MessageID != "user-1" || out[1].MessageID != "assistant-1" {
			t.Fatalf("ordered turns = %+v", out)
		}
	})

	ctx := context.Background()
	dupMessageErr := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatTurn(ctx, turn("user-1", 3, domain.ChatRoleUser, "retry"))
		return err
	})
	if !errors.Is(dupMessageErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate message_id: want ErrAlreadyExists, got %v", dupMessageErr)
	}
	dupSequenceErr := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatTurn(ctx, turn("user-2", 1, domain.ChatRoleUser, "same sequence"))
		return err
	})
	if !errors.Is(dupSequenceErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate sequence: want ErrAlreadyExists, got %v", dupSequenceErr)
	}
}

func TestDiagnosisRepository_SaveChatSessionSummaryAndFind(t *testing.T) {
	resetDB(t)
	taskID := makeDiagnosisTaskForChat(t, "summary")
	var sessionID domain.ChatSessionID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		session, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskID, "session-summary"))
		if err != nil {
			t.Fatalf("SaveChatSession: %v", err)
		}
		sessionID = session.ID
	})

	summary := func(version int, digest string) domain.ChatSessionSummary {
		t.Helper()
		got, err := domain.NewChatSessionSummary(domain.ChatSessionSummary{
			SessionID:           sessionID,
			Version:             version,
			SchemaVersion:       "diagnosis-conversation-summary.v1",
			SourceFirstSequence: 1,
			SourceLastSequence:  2,
			SourceTurnCount:     2,
			SourceDigest:        digest,
			Content:             json.RawMessage(`{"schema_version":"diagnosis-conversation-summary.v1","compression_method":"deterministic-extractive","source_turn_count":2}`),
			GeneratedAt:         time.Date(2026, 7, 11, 12, version, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("NewChatSessionSummary: %v", err)
		}
		return got
	}
	digest1 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	digest2 := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	var saved domain.ChatSessionSummary
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		got, err := uow.Diagnosis().SaveChatSessionSummary(ctx, summary(1, digest1))
		if err != nil {
			t.Fatalf("SaveChatSessionSummary: %v", err)
		}
		saved = got
	})
	if saved.ID == 0 || saved.CreatedAt.IsZero() {
		t.Fatalf("saved summary = %+v", saved)
	}

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		byVersion, err := uow.Diagnosis().FindChatSessionSummaryBySessionAndVersion(ctx, sessionID, 1)
		if err != nil {
			t.Fatalf("FindChatSessionSummaryBySessionAndVersion: %v", err)
		}
		if byVersion.SourceDigest != digest1 || byVersion.ID != saved.ID {
			t.Fatalf("summary by version = %+v", byVersion)
		}
		if _, err := uow.Diagnosis().SaveChatSessionSummary(ctx, summary(2, digest2)); err != nil {
			t.Fatalf("SaveChatSessionSummary v2: %v", err)
		}
		latest, err := uow.Diagnosis().FindLatestChatSessionSummary(ctx, sessionID)
		if err != nil {
			t.Fatalf("FindLatestChatSessionSummary: %v", err)
		}
		if latest.Version != 2 || latest.SourceDigest != digest2 {
			t.Fatalf("latest summary = %+v", latest)
		}
	})

	ctx := context.Background()
	duplicateVersion := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSessionSummary(ctx, summary(1, digest2))
		return err
	})
	if !errors.Is(duplicateVersion, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate version: want ErrAlreadyExists, got %v", duplicateVersion)
	}
	duplicateDigest := integration.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSessionSummary(ctx, summary(3, digest1))
		return err
	})
	if !errors.Is(duplicateDigest, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate digest: want ErrAlreadyExists, got %v", duplicateDigest)
	}
}

func TestDiagnosisRepository_SaveChatSessionApprovalAndList(t *testing.T) {
	resetDB(t)
	taskID := makeDiagnosisTaskForChat(t, "approval")
	var session domain.ChatSession
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		session, err = uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskID, "session-approval"))
		if err != nil {
			t.Fatalf("SaveChatSession: %v", err)
		}
	})
	digest := strings.Repeat("a", 64)
	approvedAt := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	approval := func(actor string, authority domain.DiagnosisApprovalAuthority, at time.Time) domain.ChatSessionApproval {
		got, err := domain.NewChatSessionApproval(domain.ChatSessionApproval{
			SessionID:        session.ID,
			ConclusionDigest: digest,
			ActorSubject:     actor,
			Authority:        authority,
			Reason:           "human_confirmed",
			ApprovedAt:       at,
		})
		if err != nil {
			t.Fatalf("NewChatSessionApproval: %v", err)
		}
		return got
	}
	var owner, leader domain.ChatSessionApproval
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		var err error
		owner, err = uow.Diagnosis().SaveChatSessionApproval(ctx, approval("owner-1", domain.DiagnosisApprovalAuthorityOwner, approvedAt))
		if err != nil {
			t.Fatalf("SaveChatSessionApproval owner: %v", err)
		}
		leader, err = uow.Diagnosis().SaveChatSessionApproval(ctx, approval("leader-1", domain.DiagnosisApprovalAuthorityLeader, approvedAt.Add(time.Second)))
		if err != nil {
			t.Fatalf("SaveChatSessionApproval leader: %v", err)
		}
	})
	if owner.ID == 0 || leader.ID == 0 || owner.CreatedAt.IsZero() || leader.CreatedAt.IsZero() {
		t.Fatalf("saved approvals owner=%+v leader=%+v", owner, leader)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		hasApprovals, err := uow.Diagnosis().HasChatSessionApprovals(ctx, session.ID)
		if err != nil {
			t.Fatalf("HasChatSessionApprovals: %v", err)
		}
		if !hasApprovals {
			t.Fatal("HasChatSessionApprovals = false, want true")
		}
		found, err := uow.Diagnosis().FindChatSessionApproval(ctx, session.ID, digest, "owner-1")
		if err != nil {
			t.Fatalf("FindChatSessionApproval: %v", err)
		}
		if found.ID != owner.ID || found.Authority != domain.DiagnosisApprovalAuthorityOwner {
			t.Fatalf("found approval = %+v", found)
		}
		rows, err := uow.Diagnosis().ListChatSessionApprovals(ctx, session.ID, digest, 3)
		if err != nil {
			t.Fatalf("ListChatSessionApprovals: %v", err)
		}
		if len(rows) != 2 || rows[0].ID != owner.ID || rows[1].ID != leader.ID {
			t.Fatalf("approval rows = %+v", rows)
		}
	})
	duplicateErr := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSessionApproval(ctx, approval("owner-1", domain.DiagnosisApprovalAuthorityOwner, approvedAt))
		return err
	})
	if !errors.Is(duplicateErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate approval error = %v, want ErrAlreadyExists", duplicateErr)
	}
	duplicateAuthorityErr := integration.factory.WithinTx(context.Background(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, err := uow.Diagnosis().SaveChatSessionApproval(ctx, approval("leader-2", domain.DiagnosisApprovalAuthorityLeader, approvedAt.Add(2*time.Second)))
		return err
	})
	if !errors.Is(duplicateAuthorityErr, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate approval authority error = %v, want ErrAlreadyExists", duplicateAuthorityErr)
	}
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		if _, err := uow.Diagnosis().HasChatSessionApprovals(ctx, 0); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("HasChatSessionApprovals zero session error = %v", err)
		}
		if _, err := uow.Diagnosis().FindChatSessionApproval(ctx, 0, digest, "owner-1"); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatSessionApproval zero session error = %v", err)
		}
		if _, err := uow.Diagnosis().ListChatSessionApprovals(ctx, session.ID, digest, 0); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ListChatSessionApprovals zero limit error = %v", err)
		}
	})
}

func TestDiagnosisRepository_ChatInvariantGuards(t *testing.T) {
	resetDB(t)
	taskID := makeDiagnosisTaskForChat(t, "guards")
	var sessionID domain.ChatSessionID
	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		session, err := uow.Diagnosis().SaveChatSession(ctx, makeChatSessionForDiagnosis(t, taskID, "session-guards"))
		if err != nil {
			t.Fatalf("SaveChatSession: %v", err)
		}
		sessionID = session.ID
	})

	withTx(t, func(ctx context.Context, uow ports.UnitOfWork) {
		_, err := uow.Diagnosis().FindChatSessionByID(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatSessionByID zero: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().FindChatSessionByKey(ctx, "")
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatSessionByKey empty: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().FindChatTurnBySessionAndMessageID(ctx, 0, "m")
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatTurn zero session: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().FindChatTurnBySessionAndMessageID(ctx, sessionID, "")
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatTurn empty message: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().ListChatTurnsBySession(ctx, 0, 1)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ListChatTurns zero session: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().ListChatTurnsBySession(ctx, sessionID, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ListChatTurns zero limit: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().FindChatSessionSummaryBySessionAndVersion(ctx, sessionID, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindChatSessionSummaryBySessionAndVersion zero version: want ErrInvariantViolation, got %v", err)
		}
		_, err = uow.Diagnosis().FindLatestChatSessionSummary(ctx, 0)
		if !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("FindLatestChatSessionSummary zero session: want ErrInvariantViolation, got %v", err)
		}
	})
}
