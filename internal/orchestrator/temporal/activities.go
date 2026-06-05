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
	"github.com/openclarion/openclarion/internal/usecases/llmretry"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

// Activities contains Temporal activity handlers and their dependencies.
type Activities struct {
	uowFactory                   ports.UnitOfWorkFactory
	llmProvider                  ports.LLMProvider
	imProvider                   ports.IMProvider
	notificationProviderResolver ports.NotificationChannelProviderResolver
	containerProvider            ports.ContainerProvider
	reportPolicyReplayer         reportPolicyReplayer
}

// ActivityOption configures optional dependencies for activity handlers.
type ActivityOption func(*Activities)

// WithLLMProvider injects the provider used by report-generation
// activities. Diagnosis-only workers may omit it; report workflows fail
// fast with a non-retryable configuration error until the provider is
// wired.
func WithLLMProvider(provider ports.LLMProvider) ActivityOption {
	return func(a *Activities) {
		a.llmProvider = provider
	}
}

// WithIMProvider injects the provider used by report notification
// activities.
func WithIMProvider(provider ports.IMProvider) ActivityOption {
	return func(a *Activities) {
		a.imProvider = provider
	}
}

// WithNotificationChannelProviderResolver injects the resolver used by report
// notification Activities when a workflow input names a notification channel
// profile.
func WithNotificationChannelProviderResolver(resolver ports.NotificationChannelProviderResolver) ActivityOption {
	return func(a *Activities) {
		a.notificationProviderResolver = resolver
	}
}

// WithContainerProvider injects the sandbox runtime used by per-turn
// diagnosis-room activities.
func WithContainerProvider(provider ports.ContainerProvider) ActivityOption {
	return func(a *Activities) {
		a.containerProvider = provider
	}
}

// NewActivities constructs the activity handler set for a worker.
func NewActivities(uowFactory ports.UnitOfWorkFactory, opts ...ActivityOption) *Activities {
	activities := &Activities{uowFactory: uowFactory}
	for _, opt := range opts {
		opt(activities)
	}
	return activities
}

// StartDiagnosisTask verifies that the workflow input's TaskID is
// bound to the requested EvidenceSnapshotID, then marks the task
// running. The transition is idempotent for activity retries: if the
// task is already running with StartedAt set, the activity returns nil
// without attempting to restamp the immutable start time.
func (a *Activities) StartDiagnosisTask(ctx context.Context, req startDiagnosisTaskActivityInput) error {
	if req.TaskID == 0 {
		return temporalsdk.NewNonRetryableApplicationError(
			"start-diagnosis-task: task_id must be non-zero", errTypeInvalidInput, nil)
	}
	if req.EvidenceSnapshotID == 0 {
		return temporalsdk.NewNonRetryableApplicationError(
			"start-diagnosis-task: evidence_snapshot_id must be non-zero", errTypeInvalidInput, nil)
	}

	taskID := domain.DiagnosisTaskID(req.TaskID)
	snapshotID := domain.EvidenceSnapshotID(req.EvidenceSnapshotID)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, taskID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("start-diagnosis-task: task %d not found: %w", taskID, domain.ErrInvariantViolation)
			}
			return err
		}
		if task.EvidenceSnapshotID != snapshotID {
			return fmt.Errorf(
				"start-diagnosis-task: task %d evidence_snapshot_id %d does not match input evidence_snapshot_id %d: %w",
				taskID, task.EvidenceSnapshotID, snapshotID, domain.ErrInvariantViolation)
		}
		if task.Status == domain.DiagnosisStatusRunning && task.StartedAt != nil {
			return nil
		}
		started, err := task.Start(time.Now())
		if err != nil {
			return err
		}
		_, err = uow.Diagnosis().UpdateTask(ctx, started)
		return err
	})
	if err != nil {
		return mapActivityError(err, "start-diagnosis-task")
	}
	return nil
}

// RecordDiagnosisEvent appends a DiagnosisTaskEvent for the bound
// task and is idempotent on (task_id, dedupe_key): a duplicate
// invocation returns the existing event's ID instead of failing.
//
// The flow uses three independent transactions because Postgres
// poisons a transaction after a 23505 unique violation — the same
// tx cannot be reused to SELECT the conflicting row, so retries
// must run in fresh transactions:
//
//  1. Pre-check: look up (task_id, dedupe_key); short-circuit on hit.
//  2. Insert:    append event in its own tx.
//  3. Race-lost: on ErrAlreadyExists, re-fetch in a fresh tx.
//
// Permanent input/invariant failures are wrapped as non-retryable
// application errors so Temporal's RetryPolicy stops retrying. The
// error type strings are kept in sync with workflow.go via the
// errType* constants.
func (a *Activities) RecordDiagnosisEvent(ctx context.Context, req recordEventActivityInput) (RecordEventResult, error) {
	if req.TaskID == 0 {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: task_id must be non-zero", errTypeInvalidInput, nil)
	}
	if req.Kind == "" {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: kind must be non-empty", errTypeInvalidInput, nil)
	}
	if req.DedupeKey == nil || *req.DedupeKey == "" {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: dedupe_key must be non-empty", errTypeInvalidInput, nil)
	}
	dedupeKey := *req.DedupeKey
	taskID := domain.DiagnosisTaskID(req.TaskID)

	// 1) Pre-check in its own tx: cheapest path on duplicates.
	if id, found, err := a.lookupExisting(ctx, taskID, dedupeKey); err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event pre-check")
	} else if found {
		return RecordEventResult{EventID: id}, nil
	}

	// 2) Build the domain event then attempt insert in its own tx.
	evt, err := domain.NewDiagnosisTaskEvent(
		taskID,
		req.Kind,
		json.RawMessage("{}"),
		req.DedupeKey,
		time.Now(),
	)
	if err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event build")
	}

	var insertedID int64
	insertErr := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		saved, appendErr := uow.Diagnosis().AppendEvent(ctx, evt)
		if appendErr != nil {
			return appendErr
		}
		insertedID = int64(saved.ID)
		return nil
	})
	if insertErr == nil {
		return RecordEventResult{EventID: insertedID}, nil
	}
	if !errors.Is(insertErr, domain.ErrAlreadyExists) {
		return RecordEventResult{}, mapActivityError(insertErr, "record-event append")
	}

	// 3) Race lost: another caller inserted between our pre-check
	// and this insert. Re-fetch in a fresh tx (the failed insert
	// tx is poisoned and cannot serve the SELECT).
	id, found, err := a.lookupExisting(ctx, taskID, dedupeKey)
	if err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event re-fetch")
	}
	if !found {
		return RecordEventResult{}, fmt.Errorf(
			"record-event: race-lost re-fetch missing for task %d dedupe %q",
			req.TaskID, dedupeKey)
	}
	return RecordEventResult{EventID: id}, nil
}

// lookupExisting runs FindEventByTaskAndDedupeKey inside its own tx
// and translates ErrNotFound into (0, false, nil) so callers can
// branch without inspecting domain sentinels.
func (a *Activities) lookupExisting(ctx context.Context, taskID domain.DiagnosisTaskID, dedupeKey string) (int64, bool, error) {
	var (
		id    int64
		found bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, ferr := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskID, dedupeKey)
		if ferr != nil {
			if errors.Is(ferr, domain.ErrNotFound) {
				return nil
			}
			return ferr
		}
		id = int64(existing.ID)
		found = true
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return id, found, nil
}

// GenerateSubReport builds a SubReport prompt from an EvidenceSnapshot,
// calls the configured LLMProvider through the validation retry loop, and
// persists the accepted report. It is idempotent on
// (evidence_snapshot_id, idempotency_key), so Temporal Activity retries
// return the original SubReport ID instead of duplicating rows.
func (a *Activities) GenerateSubReport(ctx context.Context, req ReportFanOutWorkflowInput) (ReportFanOutWorkflowResult, error) {
	if a.llmProvider == nil {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"generate-sub-report: llm provider is not configured", errTypeInvalidInput, nil)
	}
	if req.EvidenceSnapshotID == 0 {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"generate-sub-report: evidence_snapshot_id must be non-zero", errTypeInvalidInput, nil)
	}
	if req.GroupIndex < 0 {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"generate-sub-report: group_index must be >= 0", errTypeInvalidInput, nil)
	}
	scenario := reportprompt.Scenario(req.Scenario)
	if !scenario.Valid() {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf("generate-sub-report: scenario %q is unsupported", req.Scenario), errTypeInvalidInput, nil)
	}

	snapshot, llmReq, err := a.buildSubReportRequest(ctx, domain.EvidenceSnapshotID(req.EvidenceSnapshotID), scenario, req.GroupIndex)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report request")
	}
	if id, found, err := a.lookupSubReport(ctx, snapshot.ID, llmReq.IdempotencyKey); err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report pre-check")
	} else if found {
		return ReportFanOutWorkflowResult{SubReportID: id}, nil
	}

	result, err := llmretry.GenerateValidated(ctx, llmretry.Request{
		Provider:   a.llmProvider,
		LLMRequest: llmReq,
	})
	if err != nil {
		return ReportFanOutWorkflowResult{}, fmt.Errorf("generate-sub-report llm: %w", err)
	}
	draft, err := reportdraft.ParseSubReport(result.Accepted)
	if err != nil {
		return ReportFanOutWorkflowResult{}, fmt.Errorf("generate-sub-report parse accepted output: %w", err)
	}
	report, err := subReportDomainFromDraft(snapshot.ID, llmReq.IdempotencyKey, scenario, draft, result)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report build domain")
	}

	savedID, err := a.saveSubReport(ctx, report)
	if err == nil {
		return ReportFanOutWorkflowResult{SubReportID: savedID}, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report persist")
	}
	id, found, err := a.lookupSubReport(ctx, snapshot.ID, llmReq.IdempotencyKey)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report re-fetch")
	}
	if !found {
		return ReportFanOutWorkflowResult{}, fmt.Errorf(
			"generate-sub-report: duplicate re-fetch missing for snapshot %d idempotency_key %q",
			snapshot.ID, llmReq.IdempotencyKey)
	}
	return ReportFanOutWorkflowResult{SubReportID: id}, nil
}

// GenerateFinalReport reduces persisted SubReports into one FinalReport,
// persists it, and links the fan-in SubReport edge before any future
// notification activity can run.
func (a *Activities) GenerateFinalReport(ctx context.Context, req FinalReportWorkflowInput) (FinalReportWorkflowResult, error) {
	if a.llmProvider == nil {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"generate-final-report: llm provider is not configured", errTypeInvalidInput, nil)
	}
	correlationKey := strings.TrimSpace(req.CorrelationKey)
	if correlationKey == "" {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"generate-final-report: correlation_key must be non-empty", errTypeInvalidInput, nil)
	}
	subReportIDs, err := subReportIDsFromWorkflow(req.SubReportIDs)
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report input")
	}
	idempotencyKey := finalReportIdempotencyKey(correlationKey)
	if id, found, err := a.lookupFinalReport(ctx, idempotencyKey); err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report pre-check")
	} else if found {
		return FinalReportWorkflowResult{FinalReportID: id}, nil
	}

	drafts, err := a.loadSubReportsForFinalReport(ctx, subReportIDs)
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report load subreports")
	}
	llmReq, err := reportprompt.BuildFinalReportRequest(reportprompt.FinalReportInput{
		CorrelationKey: correlationKey,
		SubReports:     drafts,
	})
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report request")
	}

	result, err := llmretry.GenerateValidated(ctx, llmretry.Request{
		Provider:   a.llmProvider,
		LLMRequest: llmReq,
	})
	if err != nil {
		return FinalReportWorkflowResult{}, fmt.Errorf("generate-final-report llm: %w", err)
	}
	draft, err := reportdraft.ParseFinalReport(result.Accepted)
	if err != nil {
		return FinalReportWorkflowResult{}, fmt.Errorf("generate-final-report parse accepted output: %w", err)
	}
	report, err := finalReportDomainFromDraft(correlationKey, llmReq.IdempotencyKey, draft, result)
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report build domain")
	}

	savedID, err := a.saveFinalReport(ctx, report, subReportIDs)
	if err == nil {
		return FinalReportWorkflowResult{FinalReportID: savedID}, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report persist")
	}
	id, found, err := a.lookupFinalReport(ctx, llmReq.IdempotencyKey)
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report re-fetch")
	}
	if !found {
		return FinalReportWorkflowResult{}, fmt.Errorf(
			"generate-final-report: duplicate re-fetch missing for idempotency_key %q",
			llmReq.IdempotencyKey)
	}
	return FinalReportWorkflowResult{FinalReportID: id}, nil
}

// SendReportNotification sends the persisted FinalReport notification through
// the selected IMProvider. It loads the provider and report inside the Activity
// so Workflow code only carries immutable IDs and notification can only happen
// after persistence has produced a concrete FinalReport row.
func (a *Activities) SendReportNotification(ctx context.Context, req ReportNotificationActivityInput) (ReportNotificationResult, error) {
	if req.FinalReportID == 0 {
		return ReportNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-report-notification: final_report_id must be non-zero", errTypeInvalidInput, nil)
	}
	if req.ReportNotificationChannelProfileID < 0 {
		return ReportNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-report-notification: report_notification_channel_profile_id must be >= 0", errTypeInvalidInput, nil)
	}
	imProvider, err := a.reportNotificationProvider(ctx, domain.NotificationChannelProfileID(req.ReportNotificationChannelProfileID))
	if err != nil {
		return ReportNotificationResult{}, err
	}

	report, err := a.loadFinalReportForNotification(ctx, domain.FinalReportID(req.FinalReportID))
	if err != nil {
		return ReportNotificationResult{}, mapActivityError(err, "send-report-notification load report")
	}
	notification := ports.IMNotification{
		IdempotencyKey: reportNotificationIdempotencyKey(report.ID),
		FinalReportID:  int64(report.ID),
		CorrelationKey: report.CorrelationKey,
		Title:          report.Title,
		Body:           report.NotificationText,
		Severity:       string(report.Severity),
	}
	deliveryLog, err := a.ensureNotificationDelivery(ctx, report.ID, notification.IdempotencyKey)
	if err != nil {
		return ReportNotificationResult{}, mapActivityError(err, "send-report-notification prepare delivery")
	}
	if deliveryLog.Status == domain.ReportNotificationDeliveryStatusDelivered {
		return reportNotificationResultFromDelivery(deliveryLog), nil
	}

	delivery, err := imProvider.SendNotification(ctx, notification)
	if err != nil {
		if persistErr := a.markNotificationDeliveryFailed(ctx, deliveryLog, err); persistErr != nil {
			return ReportNotificationResult{}, fmt.Errorf(
				"send-report-notification persist failure after provider error: %w", persistErr)
		}
		return ReportNotificationResult{}, mapNotificationError(err)
	}
	delivered, err := deliveryLog.MarkDelivered(
		truncateString(delivery.ProviderMessageID, 256),
		truncateString(delivery.Status, 64),
		defaultJSONObject(delivery.Raw),
		time.Now(),
	)
	if err != nil {
		return ReportNotificationResult{}, mapActivityError(err, "send-report-notification build delivered delivery")
	}
	saved, err := a.updateNotificationDelivery(ctx, delivered)
	if err != nil {
		return ReportNotificationResult{}, mapActivityError(err, "send-report-notification persist delivered delivery")
	}
	return reportNotificationResultFromDelivery(saved), nil
}

func (a *Activities) reportNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	if channelProfileID == 0 {
		if a.imProvider == nil {
			return nil, temporalsdk.NewNonRetryableApplicationError(
				"send-report-notification: im provider is not configured", errTypeInvalidInput, nil)
		}
		return a.imProvider, nil
	}
	if a.notificationProviderResolver == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-report-notification: notification channel provider resolver is not configured", errTypeInvalidInput, nil)
	}
	provider, err := a.notificationProviderResolver.ResolveReportNotificationProvider(ctx, channelProfileID)
	if err != nil {
		return nil, mapActivityError(err, "send-report-notification resolve notification channel")
	}
	if provider == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-report-notification: notification channel provider resolver returned nil provider", errTypeInvalidInput, nil)
	}
	return provider, nil
}

func (a *Activities) loadFinalReportForNotification(ctx context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	var report domain.FinalReport
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().FindFinalReportByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("final report %d not found: %w", id, domain.ErrInvariantViolation)
			}
			return err
		}
		report = got
		return nil
	})
	return report, err
}

func (a *Activities) ensureNotificationDelivery(ctx context.Context, finalReportID domain.FinalReportID, idempotencyKey string) (domain.ReportNotificationDelivery, error) {
	existing, found, err := a.lookupNotificationDelivery(ctx, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	if found {
		return existing, nil
	}
	pending, err := domain.NewReportNotificationDelivery(finalReportID, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	saved, err := a.saveNotificationDelivery(ctx, pending)
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.ReportNotificationDelivery{}, err
	}
	existing, found, err = a.lookupNotificationDelivery(ctx, idempotencyKey)
	if err != nil {
		return domain.ReportNotificationDelivery{}, err
	}
	if !found {
		return domain.ReportNotificationDelivery{}, fmt.Errorf(
			"notification delivery duplicate re-fetch missing for idempotency_key %q",
			idempotencyKey)
	}
	return existing, nil
}

func (a *Activities) markNotificationDeliveryFailed(ctx context.Context, delivery domain.ReportNotificationDelivery, providerErr error) error {
	failed, err := delivery.MarkFailed(
		truncateString(providerErr.Error(), 2000),
		notificationFailureRaw(providerErr),
	)
	if err != nil {
		return err
	}
	_, err = a.updateNotificationDelivery(ctx, failed)
	return err
}

func (a *Activities) buildSubReportRequest(ctx context.Context, snapshotID domain.EvidenceSnapshotID, scenario reportprompt.Scenario, groupIndex int) (domain.EvidenceSnapshot, ports.LLMRequest, error) {
	var snapshot domain.EvidenceSnapshot
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Evidence().FindByID(ctx, snapshotID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("evidence snapshot %d not found: %w", snapshotID, domain.ErrInvariantViolation)
			}
			return err
		}
		snapshot = got
		return nil
	})
	if err != nil {
		return domain.EvidenceSnapshot{}, ports.LLMRequest{}, err
	}
	llmReq, err := reportprompt.BuildSubReportRequest(reportprompt.SubReportInput{
		Snapshot:   snapshot,
		Scenario:   scenario,
		GroupIndex: groupIndex,
	})
	if err != nil {
		return domain.EvidenceSnapshot{}, ports.LLMRequest{}, err
	}
	return snapshot, llmReq, nil
}

func (a *Activities) lookupSubReport(ctx context.Context, snapshotID domain.EvidenceSnapshotID, idempotencyKey string) (int64, bool, error) {
	var (
		id    int64
		found bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, snapshotID, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		id = int64(existing.ID)
		found = true
		return nil
	})
	return id, found, err
}

func (a *Activities) saveSubReport(ctx context.Context, report domain.SubReport) (int64, error) {
	var id int64
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		saved, err := uow.Reports().SaveSubReport(ctx, report)
		if err != nil {
			return err
		}
		id = int64(saved.ID)
		return nil
	})
	return id, err
}

func (a *Activities) lookupFinalReport(ctx context.Context, idempotencyKey string) (int64, bool, error) {
	var (
		id    int64
		found bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindFinalReportByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		id = int64(existing.ID)
		found = true
		return nil
	})
	return id, found, err
}

func (a *Activities) loadSubReportsForFinalReport(ctx context.Context, ids []domain.SubReportID) ([]reportdraft.SubReport, error) {
	drafts := make([]reportdraft.SubReport, 0, len(ids))
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		for _, id := range ids {
			report, err := uow.Reports().FindSubReportByID(ctx, id)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return fmt.Errorf("subreport %d not found: %w", id, domain.ErrInvariantViolation)
				}
				return err
			}
			var draft reportdraft.SubReport
			if err := json.Unmarshal(report.Content, &draft); err != nil {
				return fmt.Errorf("subreport %d content is not a SubReport draft: %w", id, err)
			}
			drafts = append(drafts, draft)
		}
		return nil
	})
	return drafts, err
}

func (a *Activities) saveFinalReport(ctx context.Context, report domain.FinalReport, subReportIDs []domain.SubReportID) (int64, error) {
	var id int64
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		saved, err := uow.Reports().SaveFinalReport(ctx, report, subReportIDs)
		if err != nil {
			return err
		}
		id = int64(saved.ID)
		return nil
	})
	return id, err
}

func (a *Activities) lookupNotificationDelivery(ctx context.Context, idempotencyKey string) (domain.ReportNotificationDelivery, bool, error) {
	var (
		delivery domain.ReportNotificationDelivery
		found    bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		delivery = existing
		found = true
		return nil
	})
	return delivery, found, err
}

func (a *Activities) saveNotificationDelivery(ctx context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	var saved domain.ReportNotificationDelivery
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().SaveNotificationDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		saved = got
		return nil
	})
	return saved, err
}

func (a *Activities) updateNotificationDelivery(ctx context.Context, delivery domain.ReportNotificationDelivery) (domain.ReportNotificationDelivery, error) {
	var saved domain.ReportNotificationDelivery
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().UpdateNotificationDelivery(ctx, delivery)
		if err != nil {
			return err
		}
		saved = got
		return nil
	})
	return saved, err
}

func subReportDomainFromDraft(snapshotID domain.EvidenceSnapshotID, idempotencyKey string, scenario reportprompt.Scenario, draft reportdraft.SubReport, result llmretry.Result) (domain.SubReport, error) {
	findings, err := marshalRaw("subreport findings", draft.Findings)
	if err != nil {
		return domain.SubReport{}, err
	}
	actions, err := marshalRaw("subreport recommended_actions", draft.RecommendedActions)
	if err != nil {
		return domain.SubReport{}, err
	}
	return domain.NewSubReport(domain.SubReport{
		EvidenceSnapshotID: snapshotID,
		IdempotencyKey:     idempotencyKey,
		Scenario:           string(scenario),
		Title:              draft.Title,
		Summary:            draft.Summary,
		Severity:           domain.ReportSeverity(draft.Severity),
		Confidence:         domain.ReportConfidence(draft.Confidence),
		Findings:           findings,
		RecommendedActions: actions,
		EvidenceRefs:       draft.EvidenceRefs,
		Content:            result.Output.Content,
		Model:              result.Accepted.Model,
		OutputMode:         string(result.Accepted.OutputMode),
		CreatedByWorkflow:  "ReportFanOutWorkflow",
	})
}

func finalReportDomainFromDraft(correlationKey, idempotencyKey string, draft reportdraft.FinalReport, result llmretry.Result) (domain.FinalReport, error) {
	subReports, err := marshalRaw("final report sub_reports", draft.SubReports)
	if err != nil {
		return domain.FinalReport{}, err
	}
	actions, err := marshalRaw("final report recommended_actions", draft.RecommendedActions)
	if err != nil {
		return domain.FinalReport{}, err
	}
	return domain.NewFinalReport(domain.FinalReport{
		CorrelationKey:     correlationKey,
		IdempotencyKey:     idempotencyKey,
		Title:              draft.Title,
		ExecutiveSummary:   draft.ExecutiveSummary,
		Severity:           domain.ReportSeverity(draft.Severity),
		Confidence:         domain.ReportConfidence(draft.Confidence),
		SubReports:         subReports,
		RecommendedActions: actions,
		NotificationText:   draft.NotificationText,
		Content:            result.Output.Content,
		Model:              result.Accepted.Model,
		OutputMode:         string(result.Accepted.OutputMode),
		CreatedByWorkflow:  "FinalReportWorkflow",
	})
}

func marshalRaw(label string, value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return raw, nil
}

func subReportIDsFromWorkflow(ids []int64) ([]domain.SubReportID, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("subreport ids must be non-empty: %w", domain.ErrInvariantViolation)
	}
	out := make([]domain.SubReportID, len(ids))
	for i, id := range ids {
		if id == 0 {
			return nil, fmt.Errorf("subreport ids must be non-zero: %w", domain.ErrInvariantViolation)
		}
		out[i] = domain.SubReportID(id)
	}
	return out, nil
}

func finalReportIdempotencyKey(correlationKey string) string {
	return "final_report:" + correlationKey
}

func reportNotificationIdempotencyKey(id domain.FinalReportID) string {
	return fmt.Sprintf("final_report:%d/notification", id)
}

func reportNotificationResultFromDelivery(delivery domain.ReportNotificationDelivery) ReportNotificationResult {
	status := delivery.ProviderStatus
	if status == "" {
		status = string(delivery.Status)
	}
	return ReportNotificationResult{
		FinalReportID:              int64(delivery.FinalReportID),
		NotificationIdempotencyKey: delivery.IdempotencyKey,
		ProviderMessageID:          delivery.ProviderMessageID,
		Status:                     status,
	}
}

func notificationFailureRaw(err error) json.RawMessage {
	payload := struct {
		Error      string `json:"error"`
		Retryable  *bool  `json:"retryable,omitempty"`
		StatusCode int    `json:"status_code,omitempty"`
	}{
		Error: err.Error(),
	}
	var imErr *ports.IMError
	if errors.As(err, &imErr) {
		retryable := imErr.Retryable
		payload.Retryable = &retryable
		payload.StatusCode = imErr.StatusCode
	}
	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// mapActivityError classifies a domain/persistence error as either a
// non-retryable application error (input/invariant) or a generic
// retryable error (infrastructure). The non-retryable type strings
// are matched by ActivityOptions.RetryPolicy.NonRetryableErrorTypes.
func mapActivityError(err error, where string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		return temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf("%s: %v", where, err), errTypeInvariantViolation, err)
	}
	return fmt.Errorf("%s: %w", where, err)
}

func mapNotificationError(err error) error {
	if err == nil {
		return nil
	}
	var imErr *ports.IMError
	if errors.As(err, &imErr) && !imErr.Retryable {
		return temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf("send-report-notification: %v", err), errTypeInvalidInput, err)
	}
	return fmt.Errorf("send-report-notification: %w", err)
}
