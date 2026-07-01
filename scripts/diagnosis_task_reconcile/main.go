// Command diagnosis_task_reconcile audits and optionally repairs diagnosis
// tasks left non-terminal after their diagnosis-room chat session closed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	toolName              = "diagnosis_task_reconcile"
	defaultLimit          = 200
	maxLimit              = 1000
	maxFailureReasonBytes = 1024

	actionSkipped     = "skipped"
	actionUpdated     = "updated"
	actionWouldUpdate = "would_update"
	statusClean       = "clean"
	statusNeedsRepair = "needs_repair"
	statusRepaired    = "repaired"
)

type config struct {
	apply       bool
	databaseURL string
	limit       int
}

type output struct {
	Tool    string       `json:"tool"`
	Status  string       `json:"status"`
	Apply   bool         `json:"apply"`
	Checked string       `json:"checked_at"`
	Summary summary      `json:"summary"`
	Items   []resultItem `json:"items"`
}

type summary struct {
	Scanned      int `json:"scanned"`
	Candidates   int `json:"candidates"`
	WouldUpdate  int `json:"would_update"`
	Updated      int `json:"updated"`
	Skipped      int `json:"skipped"`
	AlreadyClean int `json:"already_clean"`
}

type resultItem struct {
	Action                     string `json:"action"`
	Reason                     string `json:"reason,omitempty"`
	SessionID                  string `json:"session_id"`
	ChatSessionID              int64  `json:"chat_session_id"`
	DiagnosisTaskID            int64  `json:"diagnosis_task_id"`
	EvidenceSnapshotID         int64  `json:"evidence_snapshot_id"`
	TaskStatusBefore           string `json:"task_status_before"`
	TaskStatusAfter            string `json:"task_status_after,omitempty"`
	TaskFailureReasonAfter     string `json:"-"`
	TaskFailureReasonAfterSet  bool   `json:"task_failure_reason_after_set,omitempty"`
	TaskFailureReasonAfterSize int    `json:"task_failure_reason_after_bytes,omitempty"`
	FinishedAtAfter            string `json:"finished_at_after,omitempty"`
	CloseReason                string `json:"close_reason"`
	ClosedAt                   string `json:"closed_at,omitempty"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "[diagnosis-task-reconcile] FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	cfg, err := parseArgs(args, getenv)
	if err != nil {
		return err
	}
	client, err := repository.OpenPostgres(ctx, cfg.databaseURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() {
		if cerr := client.Close(); cerr != nil && stderr != nil {
			fmt.Fprintf(stderr, "[diagnosis-task-reconcile] close postgres: %v\n", cerr)
		}
	}()

	factory := repository.NewFactory(client)
	var out output
	err = factory.WithinTx(ctx, func(txCtx context.Context, uow ports.UnitOfWork) error {
		var rerr error
		out, rerr = reconcile(txCtx, uow.Diagnosis(), cfg, nowUTC())
		return rerr
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

func parseArgs(args []string, getenv func(string) string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&cfg.apply, "apply", false, "persist repairs; default is dry-run")
	fs.IntVar(&cfg.limit, "limit", defaultLimit, fmt.Sprintf("recent diagnosis-room session limit, 1-%d", maxLimit))
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("usage: diagnosis_task_reconcile [--apply] [--limit N]")
	}
	if cfg.limit <= 0 || cfg.limit > maxLimit {
		return config{}, fmt.Errorf("--limit must be between 1 and %d", maxLimit)
	}
	if getenv != nil {
		cfg.databaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	}
	if cfg.databaseURL == "" {
		return config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func reconcile(ctx context.Context, repo ports.DiagnosisRepository, cfg config, checkedAt time.Time) (output, error) {
	if repo == nil {
		return output{}, fmt.Errorf("diagnosis repository is required: %w", domain.ErrInvariantViolation)
	}
	sessions, err := repo.ListChatSessions(ctx, cfg.limit)
	if err != nil {
		return output{}, err
	}
	out := output{
		Tool:    toolName,
		Status:  statusClean,
		Apply:   cfg.apply,
		Checked: checkedAt.UTC().Format(time.RFC3339Nano),
		Summary: summary{Scanned: len(sessions)},
		Items:   []resultItem{},
	}
	for _, row := range sessions {
		item, ok := reconcileItem(row)
		if !ok {
			out.Summary.AlreadyClean++
			continue
		}
		out.Summary.Candidates++
		switch item.Action {
		case actionSkipped:
			out.Summary.Skipped++
		case actionWouldUpdate:
			out.Summary.WouldUpdate++
		case actionUpdated:
			out.Summary.Updated++
		}
		if cfg.apply && item.Action == actionWouldUpdate {
			updated, err := repairTask(ctx, repo, row, item)
			if err != nil {
				return output{}, err
			}
			item.Action = actionUpdated
			item.TaskStatusAfter = string(updated.Status)
			item.TaskFailureReasonAfter = updated.FailureReason
			out.Summary.WouldUpdate--
			out.Summary.Updated++
		}
		out.Items = append(out.Items, item)
	}
	if out.Summary.Updated > 0 {
		out.Status = statusRepaired
	} else if out.Summary.WouldUpdate > 0 || out.Summary.Skipped > 0 {
		out.Status = statusNeedsRepair
	}
	return out, nil
}

func reconcileItem(row domain.ChatSessionWithTask) (resultItem, bool) {
	session := row.Session
	task := row.Task
	if session.Status != domain.ChatSessionStatusClosed {
		return resultItem{}, false
	}
	if task.Status.IsTerminal() {
		return resultItem{}, false
	}
	item := resultItem{
		SessionID:          session.SessionKey,
		ChatSessionID:      int64(session.ID),
		DiagnosisTaskID:    int64(task.ID),
		EvidenceSnapshotID: int64(task.EvidenceSnapshotID),
		TaskStatusBefore:   string(task.Status),
		CloseReason:        session.CloseReason,
	}
	if session.ClosedAt != nil {
		item.ClosedAt = session.ClosedAt.UTC().Format(time.RFC3339Nano)
	}
	status, failureReason := terminalTaskStateForClosedSession(session, task)
	item.TaskStatusAfter = string(status)
	item.setTaskFailureReasonAfter(failureReason)
	if session.ClosedAt == nil || session.ClosedAt.IsZero() {
		item.Action = actionSkipped
		item.Reason = "closed session is missing closed_at"
		return item, true
	}
	if !task.Status.Valid() {
		item.Action = actionSkipped
		item.Reason = "task status is not recognized"
		return item, true
	}
	item.FinishedAtAfter = session.ClosedAt.UTC().Format(time.RFC3339Nano)
	item.Action = actionWouldUpdate
	return item, true
}

func repairTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	row domain.ChatSessionWithTask,
	item resultItem,
) (domain.DiagnosisTask, error) {
	if row.Session.ClosedAt == nil {
		return domain.DiagnosisTask{}, fmt.Errorf("repair diagnosis task %d: closed_at is required: %w", row.Task.ID, domain.ErrInvariantViolation)
	}
	status := domain.DiagnosisStatus(item.TaskStatusAfter)
	task, err := row.Task.Finish(status, *row.Session.ClosedAt, item.TaskFailureReasonAfter)
	if err != nil {
		return domain.DiagnosisTask{}, err
	}
	return repo.UpdateTask(ctx, task)
}

func (i *resultItem) setTaskFailureReasonAfter(reason string) {
	i.TaskFailureReasonAfter = reason
	i.TaskFailureReasonAfterSet = reason != ""
	i.TaskFailureReasonAfterSize = len(reason)
}

func terminalTaskStateForClosedSession(
	session domain.ChatSession,
	task domain.DiagnosisTask,
) (domain.DiagnosisStatus, string) {
	if failureReason := strings.TrimSpace(task.FailureReason); failureReason != "" {
		return domain.DiagnosisStatusFailed, truncateString(failureReason, maxFailureReasonBytes)
	}
	reason := strings.TrimSpace(session.CloseReason)
	switch reason {
	case "cancelled", "context_cancelled", "session_timeout", "idle_timeout":
		return domain.DiagnosisStatusCancelled, ""
	case "initial_turn_failed":
		return domain.DiagnosisStatusFailed, "diagnosis room closed before initial AI turn completed"
	default:
		return domain.DiagnosisStatusSucceeded, ""
	}
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	for end := limit; end >= 0; end-- {
		if utf8.ValidString(value[:end]) {
			return value[:end]
		}
	}
	return ""
}
