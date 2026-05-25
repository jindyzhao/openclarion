package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewDiagnosisTask(t *testing.T) {
	t.Parallel()

	t.Run("happy path defaults to pending", func(t *testing.T) {
		t.Parallel()
		got, err := NewDiagnosisTask(11, "wf", "run-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != DiagnosisStatusPending {
			t.Fatalf("status = %q, want %q", got.Status, DiagnosisStatusPending)
		}
	})

	cases := []struct {
		name       string
		snapshotID EvidenceSnapshotID
		workflowID string
		runID      string
	}{
		{name: "zero snapshot id", workflowID: "wf", runID: "run"},
		{name: "empty workflow id", snapshotID: 1, runID: "run"},
		{name: "empty run id", snapshotID: 1, workflowID: "wf"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewDiagnosisTask(tc.snapshotID, tc.workflowID, tc.runID)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestDiagnosisTask_Start(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	pending, _ := NewDiagnosisTask(1, "wf", "run")

	t.Run("happy path transitions to running", func(t *testing.T) {
		t.Parallel()
		got, err := pending.Start(startedAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != DiagnosisStatusRunning || got.StartedAt == nil || !got.StartedAt.Equal(startedAt) {
			t.Fatalf("unexpected post-Start state: %+v", got)
		}
	})

	t.Run("zero started_at is invariant violation", func(t *testing.T) {
		t.Parallel()
		_, err := pending.Start(time.Time{})
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("re-start with same time is idempotent", func(t *testing.T) {
		t.Parallel()
		running, err := pending.Start(startedAt)
		if err != nil {
			t.Fatalf("first start: %v", err)
		}
		again, err := running.Start(startedAt)
		if err != nil {
			t.Fatalf("second start: %v", err)
		}
		if !again.StartedAt.Equal(startedAt) {
			t.Fatalf("idempotent re-start drifted started_at")
		}
	})

	t.Run("re-start with different time is invariant violation", func(t *testing.T) {
		t.Parallel()
		running, err := pending.Start(startedAt)
		if err != nil {
			t.Fatalf("first start: %v", err)
		}
		_, err = running.Start(startedAt.Add(time.Second))
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("start from terminal is invariant violation", func(t *testing.T) {
		t.Parallel()
		running, err := pending.Start(startedAt)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		finished, err := running.Finish(DiagnosisStatusSucceeded, startedAt.Add(time.Minute), "")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		_, err = finished.Start(startedAt)
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestDiagnosisTask_Finish(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	pending, _ := NewDiagnosisTask(1, "wf", "run")
	running, err := pending.Start(startedAt)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("succeeded with empty failure_reason", func(t *testing.T) {
		t.Parallel()
		got, err := running.Finish(DiagnosisStatusSucceeded, finishedAt, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Status != DiagnosisStatusSucceeded || got.FinishedAt == nil {
			t.Fatalf("unexpected post-Finish state: %+v", got)
		}
	})

	t.Run("failed requires failure_reason", func(t *testing.T) {
		t.Parallel()
		_, err := running.Finish(DiagnosisStatusFailed, finishedAt, "")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
		got, err := running.Finish(DiagnosisStatusFailed, finishedAt, "boom")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.FailureReason != "boom" {
			t.Fatalf("failure_reason = %q", got.FailureReason)
		}
	})

	t.Run("succeeded with non-empty failure_reason rejected", func(t *testing.T) {
		t.Parallel()
		_, err := running.Finish(DiagnosisStatusSucceeded, finishedAt, "boom")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("non-terminal status rejected", func(t *testing.T) {
		t.Parallel()
		_, err := running.Finish(DiagnosisStatusRunning, finishedAt, "")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("finished_at before started_at rejected", func(t *testing.T) {
		t.Parallel()
		_, err := running.Finish(DiagnosisStatusSucceeded, startedAt.Add(-time.Second), "")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("re-finish same args idempotent", func(t *testing.T) {
		t.Parallel()
		first, err := running.Finish(DiagnosisStatusSucceeded, finishedAt, "")
		if err != nil {
			t.Fatalf("first finish: %v", err)
		}
		second, err := first.Finish(DiagnosisStatusSucceeded, finishedAt, "")
		if err != nil {
			t.Fatalf("second finish: %v", err)
		}
		if !second.FinishedAt.Equal(finishedAt) {
			t.Fatalf("idempotent re-finish drifted finished_at")
		}
	})

	t.Run("re-finish with different status rejected", func(t *testing.T) {
		t.Parallel()
		first, err := running.Finish(DiagnosisStatusSucceeded, finishedAt, "")
		if err != nil {
			t.Fatalf("first finish: %v", err)
		}
		_, err = first.Finish(DiagnosisStatusFailed, finishedAt, "boom")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestNewDiagnosisTaskEvent(t *testing.T) {
	t.Parallel()

	occurred := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	keyEmpty := ""
	keyOK := "k1"

	t.Run("happy path with nil dedupe_key", func(t *testing.T) {
		t.Parallel()
		got, err := NewDiagnosisTaskEvent(1, "task.started", nil, nil, occurred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.DedupeKey != nil {
			t.Fatalf("DedupeKey = %v, want nil", got.DedupeKey)
		}
	})

	t.Run("happy path with set dedupe_key", func(t *testing.T) {
		t.Parallel()
		got, err := NewDiagnosisTaskEvent(1, "task.started", nil, &keyOK, occurred)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.DedupeKey == nil || *got.DedupeKey != keyOK {
			t.Fatalf("DedupeKey not preserved: %v", got.DedupeKey)
		}
	})

	cases := []struct {
		name       string
		taskID     DiagnosisTaskID
		kind       string
		occurredAt time.Time
		dedupeKey  *string
	}{
		{name: "zero task id", kind: "k", occurredAt: occurred},
		{name: "empty kind", taskID: 1, occurredAt: occurred},
		{name: "zero occurred_at", taskID: 1, kind: "k"},
		{name: "empty-string dedupe_key pointer", taskID: 1, kind: "k", occurredAt: occurred, dedupeKey: &keyEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewDiagnosisTaskEvent(tc.taskID, tc.kind, nil, tc.dedupeKey, tc.occurredAt)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestDiagnosisStatus_IsTerminal(t *testing.T) {
	t.Parallel()
	terminal := []DiagnosisStatus{DiagnosisStatusSucceeded, DiagnosisStatusFailed, DiagnosisStatusCancelled}
	nonTerminal := []DiagnosisStatus{DiagnosisStatusPending, DiagnosisStatusRunning}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Fatalf("%q must be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Fatalf("%q must NOT be terminal", s)
		}
	}
}
