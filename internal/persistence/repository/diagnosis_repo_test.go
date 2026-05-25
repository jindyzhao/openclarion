package repository

import (
	"context"
	"encoding/json"
	"errors"
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
