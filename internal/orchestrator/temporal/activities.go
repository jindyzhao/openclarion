package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	temporalactivity "go.temporal.io/sdk/activity"
	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/llmretry"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reportnotification"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/retrieval"
)

// Activities contains Temporal activity handlers and their dependencies.
type Activities struct {
	uowFactory                   ports.UnitOfWorkFactory
	llmProvider                  ports.LLMProvider
	embeddingProvider            ports.EmbeddingProvider
	imProvider                   ports.IMProvider
	notificationProviderResolver ports.NotificationChannelProviderResolver
	containerProvider            ports.ContainerProvider
	diagnosisTurnStreamSink      ports.DiagnosisTurnStreamSink
	containerCredentials         []ContainerCredentialTemplate
	containerNetwork             ports.ContainerNetworkPolicy
	alertSourceProviders         *alertsourceprovider.Builder
	reportPolicyReplayer         reportPolicyReplayer
	publicBaseURL                *url.URL
}

// ActivityOption configures optional dependencies for activity handlers.
type ActivityOption func(*Activities)

// ContainerCredentialTemplate is a runtime-only environment value injected
// into one sandbox invocation. Values must never be logged or persisted.
type ContainerCredentialTemplate struct {
	Name  string
	Value string
}

// WithLLMProvider injects the provider used by report-generation
// activities. Diagnosis-only workers may omit it; report workflows fail
// fast with a non-retryable configuration error until the provider is
// wired.
func WithLLMProvider(provider ports.LLMProvider) ActivityOption {
	return func(a *Activities) {
		a.llmProvider = provider
	}
}

// WithEmbeddingProvider enables immutable report indexing and bounded
// historical-report context for report activities.
func WithEmbeddingProvider(provider ports.EmbeddingProvider) ActivityOption {
	return func(a *Activities) {
		a.embeddingProvider = provider
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

// WithDiagnosisTurnStreamSink enables transient diagnosis previews for the
// co-located HTTP/worker runtime. It does not affect durable Workflow state.
func WithDiagnosisTurnStreamSink(sink ports.DiagnosisTurnStreamSink) ActivityOption {
	return func(a *Activities) {
		a.diagnosisTurnStreamSink = sink
	}
}

// WithContainerCredentials injects runtime-only environment values for
// per-turn diagnosis sandbox invocations.
func WithContainerCredentials(credentials []ContainerCredentialTemplate) ActivityOption {
	return func(a *Activities) {
		a.containerCredentials = cloneContainerCredentialTemplates(credentials)
	}
}

// WithContainerNetworkPolicy injects the sandbox egress policy used by
// per-turn diagnosis sandbox invocations.
func WithContainerNetworkPolicy(policy ports.ContainerNetworkPolicy) ActivityOption {
	return func(a *Activities) {
		a.containerNetwork = cloneContainerNetworkPolicy(policy)
	}
}

// WithAlertSourceProviderBuilder injects alert source provider construction for
// diagnosis evidence collection activities.
func WithAlertSourceProviderBuilder(builder *alertsourceprovider.Builder) ActivityOption {
	return func(a *Activities) {
		a.alertSourceProviders = builder
	}
}

// WithPublicBaseURL injects the externally reachable browser base URL used in
// operator-facing notification links.
func WithPublicBaseURL(baseURL *url.URL) ActivityOption {
	return func(a *Activities) {
		a.publicBaseURL = clonePublicBaseURL(baseURL)
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

func clonePublicBaseURL(baseURL *url.URL) *url.URL {
	if baseURL == nil {
		return nil
	}
	clone := *baseURL
	return &clone
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

	snapshotID := domain.EvidenceSnapshotID(req.EvidenceSnapshotID)
	idempotencyKey := reportprompt.SubReportIdempotencyKey(snapshotID, req.GroupIndex, scenario)
	if existing, found, err := a.lookupSubReport(ctx, snapshotID, idempotencyKey); err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report pre-check")
	} else if found {
		if existing.Scenario != string(scenario) {
			return ReportFanOutWorkflowResult{}, mapActivityError(
				fmt.Errorf("existing SubReport scenario %q does not match requested %q: %w", existing.Scenario, scenario, domain.ErrInvariantViolation),
				"generate-sub-report pre-check",
			)
		}
		return ReportFanOutWorkflowResult{SubReportID: int64(existing.ID)}, nil
	}

	snapshot, llmReq, retrievalRefs, err := a.buildSubReportRequest(ctx, snapshotID, scenario, req.GroupIndex)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report request")
	}
	if llmReq.IdempotencyKey != idempotencyKey {
		return ReportFanOutWorkflowResult{}, mapActivityError(
			fmt.Errorf("generated idempotency key %q does not match %q: %w", llmReq.IdempotencyKey, idempotencyKey, domain.ErrInvariantViolation),
			"generate-sub-report request",
		)
	}

	result, err := llmretry.GenerateValidated(ctx, llmretry.Request{
		Provider:   a.llmProvider,
		LLMRequest: llmReq,
		Validator:  reportdraft.ValidateSubReportResponse,
	})
	if err != nil {
		return ReportFanOutWorkflowResult{}, fmt.Errorf("generate-sub-report llm: %w", err)
	}
	draft, err := reportdraft.ParseSubReport(result.Accepted)
	if err != nil {
		return ReportFanOutWorkflowResult{}, fmt.Errorf("generate-sub-report parse accepted output: %w", err)
	}
	report, err := subReportDomainFromDraft(snapshot.ID, llmReq.IdempotencyKey, scenario, draft, retrievalRefs, result)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report build domain")
	}

	saved, err := a.saveSubReport(ctx, report)
	if err == nil {
		return ReportFanOutWorkflowResult{SubReportID: int64(saved.ID)}, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report persist")
	}
	existing, found, err := a.lookupSubReport(ctx, snapshot.ID, llmReq.IdempotencyKey)
	if err != nil {
		return ReportFanOutWorkflowResult{}, mapActivityError(err, "generate-sub-report re-fetch")
	}
	if !found {
		return ReportFanOutWorkflowResult{}, fmt.Errorf(
			"generate-sub-report: duplicate re-fetch missing for snapshot %d idempotency_key %q",
			snapshot.ID, llmReq.IdempotencyKey)
	}
	if existing.Scenario != string(scenario) {
		return ReportFanOutWorkflowResult{}, mapActivityError(
			fmt.Errorf("existing SubReport scenario %q does not match requested %q after duplicate: %w", existing.Scenario, scenario, domain.ErrInvariantViolation),
			"generate-sub-report re-fetch",
		)
	}
	return ReportFanOutWorkflowResult{SubReportID: int64(existing.ID)}, nil
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
	if existing, found, err := a.lookupFinalReport(ctx, idempotencyKey); err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report pre-check")
	} else if found {
		if err := a.indexLinkedReportCorpus(ctx, existing, subReportIDs); err != nil {
			return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report index existing corpus")
		}
		return FinalReportWorkflowResult{FinalReportID: int64(existing.ID)}, nil
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
		Validator:  reportdraft.ValidateFinalReportResponse,
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

	saved, err := a.saveFinalReport(ctx, report, subReportIDs)
	if err == nil {
		if err := a.indexLinkedReportCorpus(ctx, saved, subReportIDs); err != nil {
			return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report index corpus")
		}
		return FinalReportWorkflowResult{FinalReportID: int64(saved.ID)}, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report persist")
	}
	existing, found, err := a.lookupFinalReport(ctx, llmReq.IdempotencyKey)
	if err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report re-fetch")
	}
	if !found {
		return FinalReportWorkflowResult{}, fmt.Errorf(
			"generate-final-report: duplicate re-fetch missing for idempotency_key %q",
			llmReq.IdempotencyKey)
	}
	if err := a.indexLinkedReportCorpus(ctx, existing, subReportIDs); err != nil {
		return FinalReportWorkflowResult{}, mapActivityError(err, "generate-final-report index duplicate corpus")
	}
	return FinalReportWorkflowResult{FinalReportID: int64(existing.ID)}, nil
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

	service, err := reportnotification.NewService(
		a.uowFactory,
		reportnotification.WithIMProvider(a.imProvider),
		reportnotification.WithNotificationChannelProviderResolver(a.notificationProviderResolver),
	)
	if err != nil {
		return ReportNotificationResult{}, mapActivityError(err, "send-report-notification service")
	}
	result, err := service.Send(ctx, reportnotification.Request{
		FinalReportID:                      domain.FinalReportID(req.FinalReportID),
		ReportNotificationChannelProfileID: domain.NotificationChannelProfileID(req.ReportNotificationChannelProfileID),
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvariantViolation) {
			return ReportNotificationResult{}, mapActivityError(err, "send-report-notification")
		}
		return ReportNotificationResult{}, mapNotificationError(err)
	}
	return ReportNotificationResult{
		FinalReportID:              int64(result.FinalReportID),
		NotificationIdempotencyKey: result.NotificationIdempotencyKey,
		ProviderMessageID:          result.ProviderMessageID,
		Status:                     result.Status,
	}, nil
}

func (a *Activities) buildSubReportRequest(ctx context.Context, snapshotID domain.EvidenceSnapshotID, scenario reportprompt.Scenario, groupIndex int) (domain.EvidenceSnapshot, ports.LLMRequest, []string, error) {
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
		return domain.EvidenceSnapshot{}, ports.LLMRequest{}, nil, err
	}
	historicalReports := make([]reportprompt.HistoricalReport, 0)
	retrievalRefs := make([]string, 0)
	if a.embeddingProvider != nil {
		queryText, err := retrieval.EvidenceSnapshotQuery(snapshot)
		if err != nil {
			return domain.EvidenceSnapshot{}, ports.LLMRequest{}, nil, err
		}
		items, err := retrieval.Retrieve(ctx, a.uowFactory, a.embeddingProvider, retrieval.Query{
			Text:           queryText,
			IdempotencyKey: "retrieval-query:" + retrieval.QueryDigest(queryText),
		})
		if err != nil {
			if isContextCancellation(err) {
				return domain.EvidenceSnapshot{}, ports.LLMRequest{}, nil, err
			}
			logOptionalRetrievalFailure(ctx, "query historical reports", err)
			items = nil
		}
		historicalReports = make([]reportprompt.HistoricalReport, len(items))
		retrievalRefs = make([]string, len(items))
		for i, item := range items {
			historicalReports[i] = reportprompt.HistoricalReport{
				SourceRef:      item.SourceRef,
				SourceKind:     item.SourceKind,
				Content:        item.Content,
				CosineDistance: item.CosineDistance,
			}
			retrievalRefs[i] = item.SourceRef
		}
	}
	llmReq, err := reportprompt.BuildSubReportRequest(reportprompt.SubReportInput{
		Snapshot:          snapshot,
		Scenario:          scenario,
		GroupIndex:        groupIndex,
		HistoricalReports: historicalReports,
	})
	if err != nil {
		return domain.EvidenceSnapshot{}, ports.LLMRequest{}, nil, err
	}
	return snapshot, llmReq, retrievalRefs, nil
}

func (a *Activities) lookupSubReport(ctx context.Context, snapshotID domain.EvidenceSnapshotID, idempotencyKey string) (domain.SubReport, bool, error) {
	var (
		report domain.SubReport
		found  bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindSubReportBySnapshotAndIdempotencyKey(ctx, snapshotID, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		report = existing
		found = true
		return nil
	})
	return report, found, err
}

func (a *Activities) saveSubReport(ctx context.Context, report domain.SubReport) (domain.SubReport, error) {
	var saved domain.SubReport
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Reports().SaveSubReport(ctx, report)
		if err != nil {
			return err
		}
		return nil
	})
	return saved, err
}

func (a *Activities) lookupFinalReport(ctx context.Context, idempotencyKey string) (domain.FinalReport, bool, error) {
	var (
		report domain.FinalReport
		found  bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Reports().FindFinalReportByIdempotencyKey(ctx, idempotencyKey)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		report = existing
		found = true
		return nil
	})
	return report, found, err
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

func (a *Activities) saveFinalReport(ctx context.Context, report domain.FinalReport, subReportIDs []domain.SubReportID) (domain.FinalReport, error) {
	var saved domain.FinalReport
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		saved, err = uow.Reports().SaveFinalReport(ctx, report, subReportIDs)
		if err != nil {
			return err
		}
		return nil
	})
	return saved, err
}

func (a *Activities) indexSubReport(ctx context.Context, report domain.SubReport) error {
	if a.embeddingProvider == nil {
		return nil
	}
	_, err := retrieval.IndexSubReport(ctx, a.uowFactory, a.embeddingProvider, report)
	return err
}

func (a *Activities) indexFinalReport(ctx context.Context, report domain.FinalReport) error {
	if a.embeddingProvider == nil {
		return nil
	}
	_, err := retrieval.IndexFinalReport(ctx, a.uowFactory, a.embeddingProvider, report)
	return err
}

func (a *Activities) indexLinkedReportCorpus(ctx context.Context, finalReport domain.FinalReport, expectedSubReportIDs []domain.SubReportID) error {
	if a.embeddingProvider == nil {
		return nil
	}
	linked, err := a.loadLinkedSubReportsForIndex(ctx, finalReport.ID, expectedSubReportIDs)
	if err != nil {
		return err
	}
	indexErrors := make([]error, 0, len(linked)+1)
	for _, report := range linked {
		if err := a.indexSubReport(ctx, report); err != nil {
			indexErrors = append(indexErrors, fmt.Errorf("sub_report:%d: %w", report.ID, err))
		}
	}
	if err := a.indexFinalReport(ctx, finalReport); err != nil {
		indexErrors = append(indexErrors, fmt.Errorf("final_report:%d: %w", finalReport.ID, err))
	}
	if err := errors.Join(indexErrors...); err != nil {
		if isContextCancellation(err) {
			return err
		}
		logOptionalRetrievalFailure(ctx, "index linked report corpus", err)
	}
	return nil
}

func (a *Activities) loadLinkedSubReportsForIndex(ctx context.Context, finalReportID domain.FinalReportID, expectedIDs []domain.SubReportID) ([]domain.SubReport, error) {
	if finalReportID == 0 || len(expectedIDs) == 0 {
		return nil, fmt.Errorf("linked report corpus identity is incomplete: %w", domain.ErrInvariantViolation)
	}
	expected := make(map[domain.SubReportID]struct{}, len(expectedIDs))
	for _, id := range expectedIDs {
		if id == 0 {
			return nil, fmt.Errorf("linked report corpus contains a zero SubReport ID: %w", domain.ErrInvariantViolation)
		}
		if _, duplicate := expected[id]; duplicate {
			return nil, fmt.Errorf("linked report corpus duplicates SubReport %d: %w", id, domain.ErrInvariantViolation)
		}
		expected[id] = struct{}{}
	}
	var linked []domain.SubReport
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		linked, err = uow.Reports().ListSubReportsForFinalReport(ctx, finalReportID, len(expectedIDs)+1)
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(linked) != len(expected) {
		return nil, fmt.Errorf("final report %d links %d SubReports, want %d: %w", finalReportID, len(linked), len(expected), domain.ErrInvariantViolation)
	}
	for _, report := range linked {
		if _, ok := expected[report.ID]; !ok {
			return nil, fmt.Errorf("final report %d links unexpected SubReport %d: %w", finalReportID, report.ID, domain.ErrInvariantViolation)
		}
		delete(expected, report.ID)
	}
	if len(expected) != 0 {
		return nil, fmt.Errorf("final report %d is missing requested SubReport links: %w", finalReportID, domain.ErrInvariantViolation)
	}
	return linked, nil
}

func isContextCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func logOptionalRetrievalFailure(ctx context.Context, operation string, err error) {
	const message = "optional historical report retrieval failed; continuing without it"
	if temporalactivity.IsActivity(ctx) {
		temporalactivity.GetLogger(ctx).Warn(message, "operation", operation, "error", err)
		return
	}
	slog.Default().WarnContext(ctx, message, "operation", operation, "error", err)
}

func subReportDomainFromDraft(snapshotID domain.EvidenceSnapshotID, idempotencyKey string, scenario reportprompt.Scenario, draft reportdraft.SubReport, retrievalRefs []string, result llmretry.Result) (domain.SubReport, error) {
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
		RetrievalRefs:      retrievalRefs,
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
