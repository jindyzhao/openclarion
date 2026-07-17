package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

const (
	reportTaskFailureGeneration = "subreport_generation_failed"
)

// EnsureReportTaskInput binds one report fan-out workflow execution to its
// immutable EvidenceSnapshot before generation starts.
type EnsureReportTaskInput struct {
	EvidenceSnapshotID int64
	WorkflowID         string
	RunID              string
	Scenario           string
	GroupIndex         int
	StartedAt          time.Time
}

// EnsureReportTaskResult returns the durable task identity.
type EnsureReportTaskResult struct {
	DiagnosisTaskID int64
}

// FinishReportTaskInput records the terminal outcome of one report fan-out
// workflow. FailureReason is a stable code, never a provider error message.
type FinishReportTaskInput struct {
	DiagnosisTaskID    int64
	EvidenceSnapshotID int64
	Scenario           string
	GroupIndex         int
	SubReportID        int64
	Status             string
	FailureReason      string
	FinishedAt         time.Time
}

// FinishReportTaskResult returns the append-only terminal event identity.
type FinishReportTaskResult struct {
	LifecycleEventID int64
}

type reportTaskEventPayload struct {
	Source             string `json:"source"`
	EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
	Scenario           string `json:"scenario"`
	GroupIndex         int    `json:"group_index"`
	Status             string `json:"status"`
	SubReportID        int64  `json:"sub_report_id,omitempty"`
	FailureReason      string `json:"failure_reason,omitempty"`
}

// EnsureReportTask creates and starts the workflow-bound DiagnosisTask, then
// appends an idempotent lifecycle event. Activity retries and concurrent
// duplicate execution collapse onto the same (workflow_id, run_id) row.
func (a *Activities) EnsureReportTask(ctx context.Context, req EnsureReportTaskInput) (EnsureReportTaskResult, error) {
	if a == nil || a.uowFactory == nil {
		return EnsureReportTaskResult{}, temporalsdk.NewNonRetryableApplicationError(
			"ensure-report-task: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if err := validateEnsureReportTaskInput(req); err != nil {
		return EnsureReportTaskResult{}, mapActivityError(err, "ensure-report-task input")
	}

	task, found, err := a.lookupDiagnosisTaskByExecution(ctx, req.WorkflowID, req.RunID)
	if err != nil {
		return EnsureReportTaskResult{}, mapActivityError(err, "ensure-report-task pre-check")
	}
	var event domain.DiagnosisTaskEvent
	if found {
		task, err = a.ensureExistingReportTaskStarted(ctx, req, task)
		if err != nil {
			return EnsureReportTaskResult{}, mapActivityError(err, "ensure-report-task existing")
		}
		event, err = a.appendReportTaskEvent(
			ctx,
			task.ID,
			domain.DiagnosisTaskEventKindSubReportStarted,
			req.StartedAt,
			reportTaskStartedPayload(req),
		)
	} else {
		task, event, err = a.createReportTask(ctx, req)
		if err != nil {
			return EnsureReportTaskResult{}, mapActivityError(err, "ensure-report-task create")
		}
	}
	if err != nil {
		return EnsureReportTaskResult{}, mapActivityError(err, "ensure-report-task event")
	}
	if event.TaskID != task.ID {
		return EnsureReportTaskResult{}, mapActivityError(
			fmt.Errorf("lifecycle event task %d does not match task %d: %w", event.TaskID, task.ID, domain.ErrInvariantViolation),
			"ensure-report-task event",
		)
	}
	return EnsureReportTaskResult{DiagnosisTaskID: int64(task.ID)}, nil
}

// FinishReportTask persists the terminal task state and an idempotent
// lifecycle event. A retry after the state update but before event insertion
// safely completes the missing event.
func (a *Activities) FinishReportTask(ctx context.Context, req FinishReportTaskInput) (FinishReportTaskResult, error) {
	if a == nil || a.uowFactory == nil {
		return FinishReportTaskResult{}, temporalsdk.NewNonRetryableApplicationError(
			"finish-report-task: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	status, eventKind, err := validateFinishReportTaskInput(req)
	if err != nil {
		return FinishReportTaskResult{}, mapActivityError(err, "finish-report-task input")
	}

	var saved domain.DiagnosisTask
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
		if err != nil {
			return err
		}
		if task.EvidenceSnapshotID != domain.EvidenceSnapshotID(req.EvidenceSnapshotID) {
			return fmt.Errorf(
				"task %d evidence_snapshot_id %d does not match %d: %w",
				task.ID, task.EvidenceSnapshotID, req.EvidenceSnapshotID, domain.ErrInvariantViolation,
			)
		}
		startedEvent, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, task.ID, domain.DiagnosisTaskEventKindSubReportStarted)
		if err != nil {
			return err
		}
		if err := validateReportTaskStartedEvent(task, startedEvent, req); err != nil {
			return err
		}
		if status == domain.DiagnosisStatusSucceeded {
			report, err := uow.Reports().FindSubReportByID(ctx, domain.SubReportID(req.SubReportID))
			if err != nil {
				return err
			}
			if report.EvidenceSnapshotID != task.EvidenceSnapshotID || report.Scenario != strings.TrimSpace(req.Scenario) {
				return fmt.Errorf("subreport %d does not match task snapshot/scenario: %w", report.ID, domain.ErrInvariantViolation)
			}
			expectedKey := reportprompt.SubReportIdempotencyKey(
				task.EvidenceSnapshotID,
				req.GroupIndex,
				reportprompt.Scenario(strings.TrimSpace(req.Scenario)),
			)
			if report.IdempotencyKey != expectedKey {
				return fmt.Errorf("subreport %d does not match task prompt identity: %w", report.ID, domain.ErrInvariantViolation)
			}
		}
		finished, err := task.Finish(status, req.FinishedAt, strings.TrimSpace(req.FailureReason))
		if err != nil {
			return err
		}
		saved, err = uow.Diagnosis().UpdateTask(ctx, finished)
		return err
	})
	if err != nil {
		return FinishReportTaskResult{}, mapActivityError(err, "finish-report-task persist")
	}

	event, err := a.appendReportTaskEvent(ctx, saved.ID, eventKind, req.FinishedAt, reportTaskEventPayload{
		Source:             "ReportFanOutWorkflow",
		EvidenceSnapshotID: req.EvidenceSnapshotID,
		Scenario:           strings.TrimSpace(req.Scenario),
		GroupIndex:         req.GroupIndex,
		Status:             string(status),
		SubReportID:        req.SubReportID,
		FailureReason:      strings.TrimSpace(req.FailureReason),
	})
	if err != nil {
		return FinishReportTaskResult{}, mapActivityError(err, "finish-report-task event")
	}
	return FinishReportTaskResult{LifecycleEventID: int64(event.ID)}, nil
}

func validateReportTaskStartedEvent(
	task domain.DiagnosisTask,
	event domain.DiagnosisTaskEvent,
	req FinishReportTaskInput,
) error {
	if task.StartedAt == nil || event.TaskID != task.ID || event.Kind != domain.DiagnosisTaskEventKindSubReportStarted ||
		event.DedupeKey == nil || *event.DedupeKey != domain.DiagnosisTaskEventKindSubReportStarted ||
		!event.OccurredAt.Equal(*task.StartedAt) {
		return fmt.Errorf("task %d has an invalid report start event: %w", task.ID, domain.ErrInvariantViolation)
	}
	expected, err := json.Marshal(reportTaskEventPayload{
		Source:             "ReportFanOutWorkflow",
		EvidenceSnapshotID: req.EvidenceSnapshotID,
		Scenario:           strings.TrimSpace(req.Scenario),
		GroupIndex:         req.GroupIndex,
		Status:             string(domain.DiagnosisStatusRunning),
	})
	if err != nil {
		return fmt.Errorf("marshal expected report start event: %w", err)
	}
	if !equalJSONValue(event.Payload, expected) {
		return fmt.Errorf("task %d report prompt identity does not match its start event: %w", task.ID, domain.ErrInvariantViolation)
	}
	return nil
}

func validateEnsureReportTaskInput(req EnsureReportTaskInput) error {
	if req.EvidenceSnapshotID == 0 {
		return fmt.Errorf("evidence_snapshot_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.WorkflowID) == "" || strings.TrimSpace(req.RunID) == "" {
		return fmt.Errorf("workflow_id and run_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.StartedAt.IsZero() {
		return fmt.Errorf("started_at must be set: %w", domain.ErrInvariantViolation)
	}
	return validateReportTaskPromptIdentity(req.Scenario, req.GroupIndex)
}

func validateFinishReportTaskInput(req FinishReportTaskInput) (domain.DiagnosisStatus, string, error) {
	if req.DiagnosisTaskID == 0 || req.EvidenceSnapshotID == 0 {
		return "", "", fmt.Errorf("task and snapshot ids must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if req.FinishedAt.IsZero() {
		return "", "", fmt.Errorf("finished_at must be set: %w", domain.ErrInvariantViolation)
	}
	if err := validateReportTaskPromptIdentity(req.Scenario, req.GroupIndex); err != nil {
		return "", "", err
	}
	status := domain.DiagnosisStatus(strings.TrimSpace(req.Status))
	switch status {
	case domain.DiagnosisStatusSucceeded:
		if req.SubReportID == 0 || strings.TrimSpace(req.FailureReason) != "" {
			return "", "", fmt.Errorf("succeeded status requires sub_report_id and no failure_reason: %w", domain.ErrInvariantViolation)
		}
		return status, domain.DiagnosisTaskEventKindSubReportSucceeded, nil
	case domain.DiagnosisStatusFailed:
		if req.SubReportID != 0 || strings.TrimSpace(req.FailureReason) != reportTaskFailureGeneration {
			return "", "", fmt.Errorf("failed status requires the stable generation failure code: %w", domain.ErrInvariantViolation)
		}
		return status, domain.DiagnosisTaskEventKindSubReportFailed, nil
	case domain.DiagnosisStatusCancelled:
		if req.SubReportID != 0 || strings.TrimSpace(req.FailureReason) != "" {
			return "", "", fmt.Errorf("cancelled status cannot include report or failure metadata: %w", domain.ErrInvariantViolation)
		}
		return status, domain.DiagnosisTaskEventKindSubReportCancelled, nil
	default:
		return "", "", fmt.Errorf("unsupported terminal status %q: %w", status, domain.ErrInvariantViolation)
	}
}

func validateReportTaskPromptIdentity(scenario string, groupIndex int) error {
	if groupIndex < 0 {
		return fmt.Errorf("group_index must be non-negative: %w", domain.ErrInvariantViolation)
	}
	if !reportprompt.Scenario(strings.TrimSpace(scenario)).Valid() {
		return fmt.Errorf("scenario %q is unsupported: %w", scenario, domain.ErrInvariantViolation)
	}
	return nil
}

func (a *Activities) createReportTask(
	ctx context.Context,
	req EnsureReportTaskInput,
) (domain.DiagnosisTask, domain.DiagnosisTaskEvent, error) {
	candidate, err := domain.NewDiagnosisTask(
		domain.EvidenceSnapshotID(req.EvidenceSnapshotID),
		strings.TrimSpace(req.WorkflowID),
		strings.TrimSpace(req.RunID),
	)
	if err != nil {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, err
	}
	candidate, err = candidate.Start(req.StartedAt)
	if err != nil {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, err
	}

	var saved domain.DiagnosisTask
	var savedEvent domain.DiagnosisTaskEvent
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if _, err := uow.Evidence().FindByID(ctx, candidate.EvidenceSnapshotID); err != nil {
			return err
		}
		var err error
		saved, err = uow.Diagnosis().SaveTask(ctx, candidate)
		if err != nil {
			return err
		}
		event, err := newReportTaskEvent(
			saved.ID,
			domain.DiagnosisTaskEventKindSubReportStarted,
			req.StartedAt,
			reportTaskStartedPayload(req),
		)
		if err != nil {
			return err
		}
		savedEvent, err = uow.Diagnosis().AppendEvent(ctx, event)
		return err
	})
	if err == nil {
		return saved, savedEvent, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, err
	}
	existing, found, lookupErr := a.lookupDiagnosisTaskByExecution(ctx, req.WorkflowID, req.RunID)
	if lookupErr != nil {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, lookupErr
	}
	if !found {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, fmt.Errorf("duplicate task was not found after insert race: %w", domain.ErrInvariantViolation)
	}
	existing, err = a.ensureExistingReportTaskStarted(ctx, req, existing)
	if err != nil {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, err
	}
	savedEvent, err = a.appendReportTaskEvent(
		ctx,
		existing.ID,
		domain.DiagnosisTaskEventKindSubReportStarted,
		req.StartedAt,
		reportTaskStartedPayload(req),
	)
	if err != nil {
		return domain.DiagnosisTask{}, domain.DiagnosisTaskEvent{}, err
	}
	return existing, savedEvent, nil
}

func reportTaskStartedPayload(req EnsureReportTaskInput) reportTaskEventPayload {
	return reportTaskEventPayload{
		Source:             "ReportFanOutWorkflow",
		EvidenceSnapshotID: req.EvidenceSnapshotID,
		Scenario:           strings.TrimSpace(req.Scenario),
		GroupIndex:         req.GroupIndex,
		Status:             string(domain.DiagnosisStatusRunning),
	}
}

func (a *Activities) ensureExistingReportTaskStarted(ctx context.Context, req EnsureReportTaskInput, task domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	if task.EvidenceSnapshotID != domain.EvidenceSnapshotID(req.EvidenceSnapshotID) {
		return domain.DiagnosisTask{}, fmt.Errorf("task %d is bound to snapshot %d, not %d: %w",
			task.ID, task.EvidenceSnapshotID, req.EvidenceSnapshotID, domain.ErrInvariantViolation)
	}
	if task.Status.IsTerminal() {
		return domain.DiagnosisTask{}, fmt.Errorf("task %d is already terminal with status %q: %w",
			task.ID, task.Status, domain.ErrInvariantViolation)
	}
	started, err := task.Start(req.StartedAt)
	if err != nil {
		return domain.DiagnosisTask{}, err
	}
	if task.Status == started.Status && task.StartedAt != nil && started.StartedAt != nil && task.StartedAt.Equal(*started.StartedAt) {
		return task, nil
	}
	var saved domain.DiagnosisTask
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Diagnosis().UpdateTask(ctx, started)
		return err
	})
	return saved, err
}

func (a *Activities) appendReportTaskEvent(
	ctx context.Context,
	taskID domain.DiagnosisTaskID,
	kind string,
	occurredAt time.Time,
	payload reportTaskEventPayload,
) (domain.DiagnosisTaskEvent, error) {
	event, err := newReportTaskEvent(taskID, kind, occurredAt, payload)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	raw := event.Payload
	if _, found, err := a.lookupExisting(ctx, taskID, kind); err != nil {
		return domain.DiagnosisTaskEvent{}, err
	} else if found {
		existing, err := a.lookupReportTaskEvent(ctx, taskID, kind)
		if err != nil {
			return domain.DiagnosisTaskEvent{}, err
		}
		return validateExistingReportTaskEvent(existing, kind, raw, occurredAt)
	}
	var saved domain.DiagnosisTaskEvent
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Diagnosis().AppendEvent(ctx, event)
		return err
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.DiagnosisTaskEvent{}, err
	}
	var found bool
	if _, found, err = a.lookupExisting(ctx, taskID, kind); err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	if !found {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("duplicate event was not found after insert race: %w", domain.ErrInvariantViolation)
	}
	existing, err := a.lookupReportTaskEvent(ctx, taskID, kind)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return validateExistingReportTaskEvent(existing, kind, raw, occurredAt)
}

func newReportTaskEvent(
	taskID domain.DiagnosisTaskID,
	kind string,
	occurredAt time.Time,
	payload reportTaskEventPayload,
) (domain.DiagnosisTaskEvent, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("marshal report task event: %w", err)
	}
	return domain.NewDiagnosisTaskEvent(taskID, kind, raw, &kind, occurredAt)
}

func (a *Activities) lookupReportTaskEvent(ctx context.Context, taskID domain.DiagnosisTaskID, kind string) (domain.DiagnosisTaskEvent, error) {
	var event domain.DiagnosisTaskEvent
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		event, err = uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskID, kind)
		return err
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	if event.Kind != kind {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("event dedupe %q has kind %q: %w", kind, event.Kind, domain.ErrInvariantViolation)
	}
	return event, nil
}

func validateExistingReportTaskEvent(
	event domain.DiagnosisTaskEvent,
	kind string,
	payload json.RawMessage,
	occurredAt time.Time,
) (domain.DiagnosisTaskEvent, error) {
	if event.Kind != kind ||
		!event.OccurredAt.Equal(domain.NormalizeUTCMicro(occurredAt)) ||
		!equalJSONValue(event.Payload, payload) {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf(
			"event dedupe %q does not match the requested lifecycle transition: %w",
			kind, domain.ErrInvariantViolation,
		)
	}
	return event, nil
}

func equalJSONValue(left, right json.RawMessage) bool {
	var leftValue, rightValue reportTaskEventPayload
	if err := strictjson.Unmarshal(left, &leftValue); err != nil {
		return false
	}
	if err := strictjson.Unmarshal(right, &rightValue); err != nil {
		return false
	}
	return leftValue == rightValue
}
