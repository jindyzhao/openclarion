package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestParseArgsRequiresDatabaseURL(t *testing.T) {
	_, err := parseArgs(nil, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("parseArgs error = %v, want DATABASE_URL requirement", err)
	}
}

func TestParseArgsDefaultsDatabaseURLFromEnv(t *testing.T) {
	cfg, err := parseArgs([]string{"--limit", "25"}, func(key string) string {
		if key == "DATABASE_URL" {
			return "postgres://example.test/openclarion"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.databaseURL != "postgres://example.test/openclarion" || cfg.limit != 25 || cfg.apply {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestParseArgsRejectsUnsafeOrInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "direct database URL flag",
			args: []string{"--database-url", "postgres://example.test/openclarion"},
			want: "flag provided but not defined",
		},
		{
			name: "zero limit",
			args: []string{"--limit", "0"},
			want: "--limit must be between",
		},
		{
			name: "over max limit",
			args: []string{"--limit", "1001"},
			want: "--limit must be between",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseArgs(tc.args, func(key string) string {
				if key == "DATABASE_URL" {
					return "postgres://example.test/openclarion"
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseArgs error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestReconcileDryRunDoesNotUpdateTasks(t *testing.T) {
	closedAt := time.Date(2026, 6, 20, 1, 2, 3, 0, time.UTC)
	repo := &fakeDiagnosisRepo{
		sessions: []domain.ChatSessionWithTask{
			closedSessionWithTask(1, "idle_timeout", domain.DiagnosisStatusRunning, &closedAt),
			closedSessionWithTask(2, "operator_closed", domain.DiagnosisStatusSucceeded, &closedAt),
			openSessionWithTask(3, domain.DiagnosisStatusRunning),
		},
	}

	out, err := reconcile(context.Background(), repo, config{limit: 10}, closedAt)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if out.Status != statusNeedsRepair ||
		out.Summary.Scanned != 3 ||
		out.Summary.Candidates != 1 ||
		out.Summary.WouldUpdate != 1 ||
		out.Summary.Updated != 0 ||
		out.Summary.AlreadyClean != 2 ||
		len(repo.updated) != 0 {
		t.Fatalf("unexpected dry-run output: %+v updated=%d", out, len(repo.updated))
	}
	if len(out.Items) != 1 ||
		out.Items[0].Action != actionWouldUpdate ||
		out.Items[0].TaskStatusAfter != string(domain.DiagnosisStatusCancelled) {
		t.Fatalf("unexpected dry-run item: %+v", out.Items)
	}
}

func TestReconcileCleanOutputKeepsEmptyItems(t *testing.T) {
	checkedAt := time.Date(2026, 6, 20, 1, 30, 0, 0, time.UTC)
	repo := &fakeDiagnosisRepo{
		sessions: []domain.ChatSessionWithTask{
			openSessionWithTask(1, domain.DiagnosisStatusRunning),
		},
	}

	out, err := reconcile(context.Background(), repo, config{limit: 10}, checkedAt)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if out.Status != statusClean || out.Items == nil || len(out.Items) != 0 {
		t.Fatalf("unexpected clean output: %+v", out)
	}
}

func TestReconcileApplyRepairsClosedRunningTasks(t *testing.T) {
	closedAt := time.Date(2026, 6, 20, 2, 3, 4, 0, time.UTC)
	failedWithReason := closedSessionWithTask(4, "operator_closed", domain.DiagnosisStatusRunning, &closedAt)
	failedWithReason.Task.FailureReason = "llm gateway timeout"
	blankFailureReason := closedSessionWithTask(5, "initial_turn_failed", domain.DiagnosisStatusRunning, &closedAt)
	blankFailureReason.Task.FailureReason = "   "
	repo := &fakeDiagnosisRepo{
		sessions: []domain.ChatSessionWithTask{
			closedSessionWithTask(1, "idle_timeout", domain.DiagnosisStatusRunning, &closedAt),
			closedSessionWithTask(2, "initial_turn_failed", domain.DiagnosisStatusRunning, &closedAt),
			closedSessionWithTask(3, "operator_closed", domain.DiagnosisStatusPending, &closedAt),
			failedWithReason,
			blankFailureReason,
		},
	}

	out, err := reconcile(context.Background(), repo, config{apply: true, limit: 10}, closedAt)
	if err != nil {
		t.Fatalf("reconcile apply: %v", err)
	}
	if out.Status != statusRepaired ||
		out.Summary.Candidates != 5 ||
		out.Summary.Updated != 5 ||
		out.Summary.WouldUpdate != 0 ||
		len(repo.updated) != 5 {
		t.Fatalf("unexpected apply output: %+v updated=%d", out, len(repo.updated))
	}
	assertUpdatedTask(t, repo.updated[0], domain.DiagnosisStatusCancelled, "")
	assertUpdatedTask(t, repo.updated[1], domain.DiagnosisStatusFailed, "diagnosis room closed before initial AI turn completed")
	assertUpdatedTask(t, repo.updated[2], domain.DiagnosisStatusSucceeded, "")
	assertUpdatedTask(t, repo.updated[3], domain.DiagnosisStatusFailed, "llm gateway timeout")
	assertUpdatedTask(t, repo.updated[4], domain.DiagnosisStatusFailed, "diagnosis room closed before initial AI turn completed")
}

func TestReconcileOutputDoesNotSerializeFailureReasonText(t *testing.T) {
	closedAt := time.Date(2026, 6, 20, 2, 30, 0, 0, time.UTC)
	row := closedSessionWithTask(1, "operator_closed", domain.DiagnosisStatusRunning, &closedAt)
	row.Task.FailureReason = "llm gateway timeout with internal endpoint"
	repo := &fakeDiagnosisRepo{sessions: []domain.ChatSessionWithTask{row}}

	out, err := reconcile(context.Background(), repo, config{limit: 10}, closedAt)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if strings.Contains(string(raw), "internal endpoint") ||
		strings.Contains(string(raw), "llm gateway timeout") ||
		strings.Contains(string(raw), "task_failure_reason_after\":\"") {
		t.Fatalf("output leaked failure reason text: %s", raw)
	}
	if len(out.Items) != 1 ||
		!out.Items[0].TaskFailureReasonAfterSet ||
		out.Items[0].TaskFailureReasonAfterSize == 0 ||
		out.Items[0].TaskFailureReasonAfter == "" ||
		out.Items[0].FinishedAtAfter != closedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected redacted item metadata: %+v", out.Items)
	}
}

func TestReconcileSkipsClosedSessionWithoutClosedAt(t *testing.T) {
	checkedAt := time.Date(2026, 6, 20, 3, 4, 5, 0, time.UTC)
	repo := &fakeDiagnosisRepo{
		sessions: []domain.ChatSessionWithTask{
			closedSessionWithTask(1, "idle_timeout", domain.DiagnosisStatusRunning, nil),
		},
	}

	out, err := reconcile(context.Background(), repo, config{apply: true, limit: 10}, checkedAt)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if out.Status != statusNeedsRepair ||
		out.Summary.Skipped != 1 ||
		len(repo.updated) != 0 ||
		len(out.Items) != 1 ||
		out.Items[0].Reason != "closed session is missing closed_at" {
		t.Fatalf("unexpected skipped output: %+v updated=%d", out, len(repo.updated))
	}
}

func TestTruncateStringPreservesUTF8Boundary(t *testing.T) {
	got := truncateString("ab\u00e9cd", 3)
	if got != "ab" || !utf8.ValidString(got) {
		t.Fatalf("truncateString = %q, want valid UTF-8 prefix %q", got, "ab")
	}
}

func assertUpdatedTask(t *testing.T, task domain.DiagnosisTask, status domain.DiagnosisStatus, failureReason string) {
	t.Helper()
	if task.Status != status || task.FailureReason != failureReason || task.FinishedAt == nil {
		t.Fatalf("updated task = %+v, want status=%s failure_reason=%q with finished_at", task, status, failureReason)
	}
}

func closedSessionWithTask(
	id int64,
	closeReason string,
	taskStatus domain.DiagnosisStatus,
	closedAt *time.Time,
) domain.ChatSessionWithTask {
	startedAt := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Minute)
	lastActivityAt := startedAt.Add(30 * time.Second)
	if closedAt != nil {
		lastActivityAt = *closedAt
	}
	return domain.ChatSessionWithTask{
		Session: domain.ChatSession{
			ID:              domain.ChatSessionID(id),
			DiagnosisTaskID: domain.DiagnosisTaskID(id + 100),
			SessionKey:      "diagnosis-session-test",
			OwnerSubject:    "operator",
			Status:          domain.ChatSessionStatusClosed,
			TurnCount:       0,
			StartedAt:       startedAt,
			LastActivityAt:  lastActivityAt,
			ClosedAt:        closedAt,
			CloseReason:     closeReason,
			CreatedAt:       startedAt,
			UpdatedAt:       lastActivityAt,
		},
		Task: domain.DiagnosisTask{
			ID:                 domain.DiagnosisTaskID(id + 100),
			EvidenceSnapshotID: domain.EvidenceSnapshotID(id + 200),
			WorkflowID:         "diagnosis-workflow-test",
			RunID:              "run-test",
			Status:             taskStatus,
			StartedAt:          &startedAt,
			CreatedAt:          startedAt,
			UpdatedAt:          lastActivityAt,
		},
	}
}

func openSessionWithTask(id int64, taskStatus domain.DiagnosisStatus) domain.ChatSessionWithTask {
	row := closedSessionWithTask(id, "", taskStatus, nil)
	row.Session.Status = domain.ChatSessionStatusOpen
	row.Session.CloseReason = ""
	return row
}

type fakeDiagnosisRepo struct {
	ports.DiagnosisRepository
	sessions []domain.ChatSessionWithTask
	updated  []domain.DiagnosisTask
}

func (r *fakeDiagnosisRepo) ListChatSessions(_ context.Context, limit int) ([]domain.ChatSessionWithTask, error) {
	if limit > len(r.sessions) {
		limit = len(r.sessions)
	}
	return r.sessions[:limit], nil
}

func (r *fakeDiagnosisRepo) UpdateTask(_ context.Context, task domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	r.updated = append(r.updated, task)
	return task, nil
}
