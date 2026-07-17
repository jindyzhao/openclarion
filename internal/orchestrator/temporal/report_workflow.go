package temporal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/domain"
)

// ReportFanOutWorkflowInput identifies one EvidenceSnapshot-backed
// SubReport generation unit.
type ReportFanOutWorkflowInput struct {
	EvidenceSnapshotID int64
	Scenario           string
	GroupIndex         int
}

// ReportFanOutWorkflowResult returns the persisted SubReport identity.
type ReportFanOutWorkflowResult struct {
	SubReportID int64
}

// ReportBatchWorkflowInput identifies one operator-facing report batch.
type ReportBatchWorkflowInput struct {
	CorrelationKey                     string
	ReportNotificationChannelProfileID int64
	MaxFailedSubReports                int
	Items                              []ReportBatchItem
}

// ReportBatchItem identifies one EvidenceSnapshot-backed fan-out unit.
type ReportBatchItem struct {
	EvidenceSnapshotID int64
	Scenario           string
	GroupIndex         int
}

// ReportBatchWorkflowResult returns the full batch output.
type ReportBatchWorkflowResult struct {
	SubReportIDs               []int64
	ExpectedSubReportCount     int
	SuccessfulSubReportCount   int
	FailedSubReportCount       int
	FailedItems                []ReportBatchItemFailure
	FinalReportID              int64
	NotificationIdempotencyKey string
	ProviderMessageID          string
	NotificationStatus         string
}

// MarshalJSON preserves the pre-coverage workflow result payload for replayed
// histories while keeping every coverage count explicit for new executions.
func (r ReportBatchWorkflowResult) MarshalJSON() ([]byte, error) {
	type legacyResult struct {
		SubReportIDs               []int64
		FinalReportID              int64
		NotificationIdempotencyKey string
		ProviderMessageID          string
		NotificationStatus         string
	}
	legacy := legacyResult{
		SubReportIDs:               r.SubReportIDs,
		FinalReportID:              r.FinalReportID,
		NotificationIdempotencyKey: r.NotificationIdempotencyKey,
		ProviderMessageID:          r.ProviderMessageID,
		NotificationStatus:         r.NotificationStatus,
	}
	if r.ExpectedSubReportCount == 0 && r.SuccessfulSubReportCount == 0 &&
		r.FailedSubReportCount == 0 && len(r.FailedItems) == 0 {
		return json.Marshal(legacy)
	}
	type currentResult struct {
		SubReportIDs               []int64
		ExpectedSubReportCount     int
		SuccessfulSubReportCount   int
		FailedSubReportCount       int
		FailedItems                []ReportBatchItemFailure
		FinalReportID              int64
		NotificationIdempotencyKey string
		ProviderMessageID          string
		NotificationStatus         string
	}
	return json.Marshal(currentResult{
		SubReportIDs:               r.SubReportIDs,
		ExpectedSubReportCount:     r.ExpectedSubReportCount,
		SuccessfulSubReportCount:   r.SuccessfulSubReportCount,
		FailedSubReportCount:       r.FailedSubReportCount,
		FailedItems:                r.FailedItems,
		FinalReportID:              r.FinalReportID,
		NotificationIdempotencyKey: r.NotificationIdempotencyKey,
		ProviderMessageID:          r.ProviderMessageID,
		NotificationStatus:         r.NotificationStatus,
	})
}

// ReportBatchItemFailure is a sanitized child-workflow failure projection.
// Provider error text stays in Temporal history and is never returned here.
type ReportBatchItemFailure struct {
	ItemIndex          int
	EvidenceSnapshotID int64
	Scenario           string
	GroupIndex         int
	Reason             string
}

// FinalReportWorkflowInput identifies the validated SubReports to reduce
// into one persisted FinalReport.
type FinalReportWorkflowInput struct {
	CorrelationKey                     string
	ReportNotificationChannelProfileID int64
	SubReportIDs                       []int64
	ExpectedSubReportCount             int `json:",omitempty"`
	FailedSubReportCount               int `json:",omitempty"`
}

// finalReportWorkflowInputLegacy preserves the exact payload shape written by
// histories created before FinalReport coverage fields were introduced.
type finalReportWorkflowInputLegacy struct {
	CorrelationKey                     string
	ReportNotificationChannelProfileID int64
	SubReportIDs                       []int64
}

// FinalReportWorkflowResult returns the persisted FinalReport identity.
type FinalReportWorkflowResult struct {
	FinalReportID              int64
	NotificationIdempotencyKey string
	ProviderMessageID          string
	NotificationStatus         string
}

// ReportNotificationActivityInput identifies the persisted FinalReport
// to notify about.
type ReportNotificationActivityInput struct {
	FinalReportID                      int64
	ReportNotificationChannelProfileID int64
}

// ReportNotificationResult returns provider delivery metadata.
type ReportNotificationResult struct {
	FinalReportID              int64
	NotificationIdempotencyKey string
	ProviderMessageID          string
	Status                     string
}

// ReportFanOutWorkflow generates and persists one SubReport for a
// snapshot/group prompt variant.
func ReportFanOutWorkflow(ctx workflow.Context, input ReportFanOutWorkflowInput) (ReportFanOutWorkflowResult, error) {
	if input.EvidenceSnapshotID == 0 {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-fan-out: input.evidence_snapshot_id must be non-zero",
			errTypeInvalidInput, nil)
	}
	if strings.TrimSpace(input.Scenario) == "" {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-fan-out: input.scenario must be non-empty",
			errTypeInvalidInput, nil)
	}
	if input.GroupIndex < 0 {
		return ReportFanOutWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-fan-out: input.group_index must be >= 0",
			errTypeInvalidInput, nil)
	}

	version := workflow.GetVersion(ctx, reportFanOutTaskLifecycleChangeID, workflow.DefaultVersion, reportFanOutTaskLifecycleVersion)
	if version == workflow.DefaultVersion {
		return reportFanOutWorkflowLegacy(ctx, input)
	}
	return reportFanOutWorkflowWithTaskLifecycle(ctx, input)
}

func reportFanOutWorkflowLegacy(ctx workflow.Context, input ReportFanOutWorkflowInput) (ReportFanOutWorkflowResult, error) {
	actCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	var result ReportFanOutWorkflowResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).GenerateSubReport, input).Get(ctx, &result); err != nil {
		return ReportFanOutWorkflowResult{}, err
	}
	return result, nil
}

func reportFanOutWorkflowWithTaskLifecycle(ctx workflow.Context, input ReportFanOutWorkflowInput) (ReportFanOutWorkflowResult, error) {
	lifecycleCtx := workflow.WithActivityOptions(ctx, reportTaskActivityOptions())
	generationCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	info := workflow.GetInfo(ctx)
	startedAt := workflow.Now(ctx)
	var ensured EnsureReportTaskResult
	if err := workflow.ExecuteActivity(lifecycleCtx, (*Activities).EnsureReportTask, EnsureReportTaskInput{
		EvidenceSnapshotID: input.EvidenceSnapshotID,
		WorkflowID:         info.WorkflowExecution.ID,
		RunID:              info.WorkflowExecution.RunID,
		Scenario:           input.Scenario,
		GroupIndex:         input.GroupIndex,
		StartedAt:          startedAt,
	}).Get(ctx, &ensured); err != nil {
		return ReportFanOutWorkflowResult{}, err
	}

	var result ReportFanOutWorkflowResult
	generationErr := workflow.ExecuteActivity(generationCtx, (*Activities).GenerateSubReport, input).Get(ctx, &result)
	disconnectedCtx, _ := workflow.NewDisconnectedContext(ctx)
	finishCtx := workflow.WithActivityOptions(disconnectedCtx, reportTaskActivityOptions())
	if generationErr != nil {
		status := domainDiagnosisStatusFailed
		failureReason := reportTaskFailureGeneration
		if temporalsdk.IsCanceledError(generationErr) {
			status = domainDiagnosisStatusCancelled
			failureReason = ""
		}
		var finished FinishReportTaskResult
		finishErr := workflow.ExecuteActivity(finishCtx, (*Activities).FinishReportTask, FinishReportTaskInput{
			DiagnosisTaskID:    ensured.DiagnosisTaskID,
			EvidenceSnapshotID: input.EvidenceSnapshotID,
			Scenario:           input.Scenario,
			GroupIndex:         input.GroupIndex,
			Status:             status,
			FailureReason:      failureReason,
			FinishedAt:         workflow.Now(disconnectedCtx),
		}).Get(disconnectedCtx, &finished)
		if finishErr != nil {
			return ReportFanOutWorkflowResult{}, fmt.Errorf(
				"report-fan-out: persist terminal task state: %w; generation failed: %w",
				finishErr, generationErr,
			)
		}
		if cancellationErr := ctx.Err(); cancellationErr != nil {
			return ReportFanOutWorkflowResult{}, cancellationErr
		}
		return ReportFanOutWorkflowResult{}, generationErr
	}

	var finished FinishReportTaskResult
	if err := workflow.ExecuteActivity(finishCtx, (*Activities).FinishReportTask, FinishReportTaskInput{
		DiagnosisTaskID:    ensured.DiagnosisTaskID,
		EvidenceSnapshotID: input.EvidenceSnapshotID,
		Scenario:           input.Scenario,
		GroupIndex:         input.GroupIndex,
		SubReportID:        result.SubReportID,
		Status:             domainDiagnosisStatusSucceeded,
		FinishedAt:         workflow.Now(disconnectedCtx),
	}).Get(disconnectedCtx, &finished); err != nil {
		return ReportFanOutWorkflowResult{}, err
	}
	if cancellationErr := ctx.Err(); cancellationErr != nil {
		return ReportFanOutWorkflowResult{}, cancellationErr
	}
	return result, nil
}

// ReportBatchWorkflow fans out SubReport generation for a batch, then
// reduces the persisted SubReports into a FinalReport and sends the
// notification through FinalReportWorkflow.
func ReportBatchWorkflow(ctx workflow.Context, input ReportBatchWorkflowInput) (ReportBatchWorkflowResult, error) {
	if strings.TrimSpace(input.CorrelationKey) == "" {
		return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-batch: input.correlation_key must be non-empty",
			errTypeInvalidInput, nil)
	}
	if len(input.Items) == 0 {
		return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-batch: input.items must be non-empty",
			errTypeInvalidInput, nil)
	}
	if input.ReportNotificationChannelProfileID < 0 {
		return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-batch: input.report_notification_channel_profile_id must be >= 0",
			errTypeInvalidInput, nil)
	}
	if input.MaxFailedSubReports < 0 || input.MaxFailedSubReports > domain.ReportWorkflowMaxFailedSubReports {
		return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf(
				"report-batch: input.max_failed_sub_reports must be between 0 and %d",
				domain.ReportWorkflowMaxFailedSubReports,
			),
			errTypeInvalidInput, nil)
	}
	for i, item := range input.Items {
		if item.EvidenceSnapshotID == 0 {
			return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
				fmt.Sprintf("report-batch: input.items[%d].evidence_snapshot_id must be non-zero", i),
				errTypeInvalidInput, nil)
		}
		if strings.TrimSpace(item.Scenario) == "" {
			return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
				fmt.Sprintf("report-batch: input.items[%d].scenario must be non-empty", i),
				errTypeInvalidInput, nil)
		}
		if item.GroupIndex < 0 {
			return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
				fmt.Sprintf("report-batch: input.items[%d].group_index must be >= 0", i),
				errTypeInvalidInput, nil)
		}
	}

	version := workflow.GetVersion(ctx, reportBatchPartialFanInChangeID, workflow.DefaultVersion, reportBatchPartialFanInVersion)
	if version == workflow.DefaultVersion {
		return reportBatchWorkflowLegacy(ctx, input)
	}
	return reportBatchWorkflowWithPartialFanIn(ctx, input)
}

func reportBatchWorkflowLegacy(ctx workflow.Context, input ReportBatchWorkflowInput) (ReportBatchWorkflowResult, error) {
	childCtx := workflow.WithChildOptions(ctx, reportChildWorkflowOptions(workflow.GetInfo(ctx).TaskQueueName))
	futures := make([]workflow.ChildWorkflowFuture, len(input.Items))
	for i, item := range input.Items {
		futures[i] = workflow.ExecuteChildWorkflow(childCtx, ReportFanOutWorkflow, ReportFanOutWorkflowInput{
			EvidenceSnapshotID: item.EvidenceSnapshotID,
			Scenario:           item.Scenario,
			GroupIndex:         item.GroupIndex,
		})
	}

	subReportIDs := make([]int64, len(futures))
	for i, future := range futures {
		var result ReportFanOutWorkflowResult
		if err := future.Get(ctx, &result); err != nil {
			return ReportBatchWorkflowResult{}, err
		}
		subReportIDs[i] = result.SubReportID
	}

	var final FinalReportWorkflowResult
	if err := workflow.ExecuteChildWorkflow(childCtx, FinalReportWorkflow, finalReportWorkflowInputLegacy{
		CorrelationKey:                     strings.TrimSpace(input.CorrelationKey),
		ReportNotificationChannelProfileID: input.ReportNotificationChannelProfileID,
		SubReportIDs:                       subReportIDs,
	}).Get(ctx, &final); err != nil {
		return ReportBatchWorkflowResult{}, err
	}

	return ReportBatchWorkflowResult{
		SubReportIDs:               subReportIDs,
		FinalReportID:              final.FinalReportID,
		NotificationIdempotencyKey: final.NotificationIdempotencyKey,
		ProviderMessageID:          final.ProviderMessageID,
		NotificationStatus:         final.NotificationStatus,
	}, nil
}

func reportBatchWorkflowWithPartialFanIn(ctx workflow.Context, input ReportBatchWorkflowInput) (ReportBatchWorkflowResult, error) {
	childCtx := workflow.WithChildOptions(ctx, reportChildWorkflowOptions(workflow.GetInfo(ctx).TaskQueueName))
	futures := make([]workflow.ChildWorkflowFuture, len(input.Items))
	for i, item := range input.Items {
		futures[i] = workflow.ExecuteChildWorkflow(childCtx, ReportFanOutWorkflow, ReportFanOutWorkflowInput{
			EvidenceSnapshotID: item.EvidenceSnapshotID,
			Scenario:           item.Scenario,
			GroupIndex:         item.GroupIndex,
		})
	}

	subReportIDs := make([]int64, 0, len(futures))
	failedItems := make([]ReportBatchItemFailure, 0)
	var cancellationErr error
	for i, future := range futures {
		var result ReportFanOutWorkflowResult
		if err := future.Get(ctx, &result); err != nil {
			if cancellationErr == nil && temporalsdk.IsCanceledError(err) {
				cancellationErr = err
			}
			failedItems = append(failedItems, ReportBatchItemFailure{
				ItemIndex:          i,
				EvidenceSnapshotID: input.Items[i].EvidenceSnapshotID,
				Scenario:           input.Items[i].Scenario,
				GroupIndex:         input.Items[i].GroupIndex,
				Reason:             reportBatchChildFailureReason(err),
			})
			continue
		}
		subReportIDs = append(subReportIDs, result.SubReportID)
	}
	if cancellationErr != nil {
		return ReportBatchWorkflowResult{}, cancellationErr
	}
	if len(subReportIDs) == 0 || len(failedItems) > input.MaxFailedSubReports {
		return ReportBatchWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf(
				"report-batch: %d of %d SubReports failed; policy allows at most %d failures and requires one success",
				len(failedItems), len(input.Items), input.MaxFailedSubReports,
			),
			errTypeReportPartialFailureThreshold, nil,
			len(input.Items), len(subReportIDs), len(failedItems), input.MaxFailedSubReports,
		)
	}

	var final FinalReportWorkflowResult
	if err := workflow.ExecuteChildWorkflow(childCtx, FinalReportWorkflow, FinalReportWorkflowInput{
		CorrelationKey:                     strings.TrimSpace(input.CorrelationKey),
		ReportNotificationChannelProfileID: input.ReportNotificationChannelProfileID,
		SubReportIDs:                       subReportIDs,
		ExpectedSubReportCount:             len(input.Items),
		FailedSubReportCount:               len(failedItems),
	}).Get(ctx, &final); err != nil {
		return ReportBatchWorkflowResult{}, err
	}

	return ReportBatchWorkflowResult{
		SubReportIDs:               subReportIDs,
		ExpectedSubReportCount:     len(input.Items),
		SuccessfulSubReportCount:   len(subReportIDs),
		FailedSubReportCount:       len(failedItems),
		FailedItems:                failedItems,
		FinalReportID:              final.FinalReportID,
		NotificationIdempotencyKey: final.NotificationIdempotencyKey,
		ProviderMessageID:          final.ProviderMessageID,
		NotificationStatus:         final.NotificationStatus,
	}, nil
}

func reportBatchChildFailureReason(err error) string {
	var childErr *temporalsdk.ChildWorkflowExecutionError
	if !errors.As(err, &childErr) {
		return "subreport_workflow_failed"
	}
	if temporalsdk.IsCanceledError(err) {
		return "subreport_workflow_cancelled"
	}
	var activityErr *temporalsdk.ActivityError
	if errors.As(err, &activityErr) && activityErr.ActivityType().GetName() == "GenerateSubReport" {
		return reportTaskFailureGeneration
	}
	return "subreport_workflow_failed"
}

// FinalReportWorkflow reduces persisted SubReports, persists the
// FinalReport, and only then sends the operator notification.
func FinalReportWorkflow(ctx workflow.Context, input FinalReportWorkflowInput) (FinalReportWorkflowResult, error) {
	if strings.TrimSpace(input.CorrelationKey) == "" {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"final-report: input.correlation_key must be non-empty",
			errTypeInvalidInput, nil)
	}
	if len(input.SubReportIDs) == 0 {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"final-report: input.sub_report_ids must be non-empty",
			errTypeInvalidInput, nil)
	}
	if input.ReportNotificationChannelProfileID < 0 {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"final-report: input.report_notification_channel_profile_id must be >= 0",
			errTypeInvalidInput, nil)
	}
	for i, id := range input.SubReportIDs {
		if id == 0 {
			return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
				fmt.Sprintf("final-report: input.sub_report_ids[%d] must be non-zero", i),
				errTypeInvalidInput, nil)
		}
	}
	if _, _, err := normalizedFinalReportCoverage(input); err != nil {
		return FinalReportWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"final-report: "+err.Error(), errTypeInvalidInput, nil)
	}

	version := workflow.GetVersion(ctx, finalReportCoverageChangeID, workflow.DefaultVersion, finalReportCoverageVersion)
	if version == workflow.DefaultVersion {
		return executeFinalReportWorkflow(ctx, input, finalReportWorkflowInputLegacy{
			CorrelationKey:                     input.CorrelationKey,
			ReportNotificationChannelProfileID: input.ReportNotificationChannelProfileID,
			SubReportIDs:                       input.SubReportIDs,
		})
	}
	return executeFinalReportWorkflow(ctx, input, input)
}

func executeFinalReportWorkflow(
	ctx workflow.Context,
	input FinalReportWorkflowInput,
	generationInput any,
) (FinalReportWorkflowResult, error) {
	actCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	var result FinalReportWorkflowResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).GenerateFinalReport, generationInput).Get(ctx, &result); err != nil {
		return FinalReportWorkflowResult{}, err
	}
	var notification ReportNotificationResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).SendReportNotification, ReportNotificationActivityInput{
		FinalReportID:                      result.FinalReportID,
		ReportNotificationChannelProfileID: input.ReportNotificationChannelProfileID,
	}).Get(ctx, &notification); err != nil {
		return FinalReportWorkflowResult{}, err
	}
	result.NotificationIdempotencyKey = notification.NotificationIdempotencyKey
	result.ProviderMessageID = notification.ProviderMessageID
	result.NotificationStatus = notification.Status
	return result, nil
}

func normalizedFinalReportCoverage(input FinalReportWorkflowInput) (expected, failed int, err error) {
	expected = input.ExpectedSubReportCount
	failed = input.FailedSubReportCount
	if expected == 0 && failed == 0 {
		return len(input.SubReportIDs), 0, nil
	}
	if expected <= 0 || failed < 0 || len(input.SubReportIDs) == 0 || expected != len(input.SubReportIDs)+failed {
		return 0, 0, fmt.Errorf(
			"coverage must satisfy expected_sub_report_count = successful inputs + failed_sub_report_count with at least one success",
		)
	}
	return expected, failed, nil
}

func reportChildWorkflowOptions(taskQueue string) workflow.ChildWorkflowOptions {
	return workflow.ChildWorkflowOptions{
		TaskQueue:                strings.TrimSpace(taskQueue),
		WorkflowExecutionTimeout: reportChildWorkflowExecutionTimeout,
		WorkflowTaskTimeout:      10 * time.Second,
	}
}

func reportActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		ScheduleToCloseTimeout: reportActivityScheduleToCloseTimeout,
		StartToCloseTimeout:    reportActivityStartToCloseTimeout,
		RetryPolicy: &temporalsdk.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
			NonRetryableErrorTypes: []string{
				errTypeInvalidInput,
				errTypeInvariantViolation,
			},
		},
	}
}

func reportTaskActivityOptions() workflow.ActivityOptions {
	options := reportActivityOptions()
	options.ScheduleToCloseTimeout = reportTaskActivityScheduleToCloseTimeout
	options.StartToCloseTimeout = reportTaskActivityStartToCloseTimeout
	return options
}

const (
	reportActivityStartToCloseTimeout        = 10 * time.Minute
	reportActivityScheduleToCloseTimeout     = 35 * time.Minute
	reportTaskActivityStartToCloseTimeout    = time.Minute
	reportTaskActivityScheduleToCloseTimeout = 5 * time.Minute
	reportChildWorkflowExecutionTimeout      = 45 * time.Minute

	reportFanOutTaskLifecycleChangeID    = "report-fan-out-task-lifecycle"
	reportFanOutTaskLifecycleVersion     = 1
	reportBatchPartialFanInChangeID      = "report-batch-partial-fan-in"
	reportBatchPartialFanInVersion       = 1
	finalReportCoverageChangeID          = "final-report-coverage-input"
	finalReportCoverageVersion           = 1
	errTypeReportPartialFailureThreshold = "ReportPartialFailureThresholdExceeded"
	domainDiagnosisStatusSucceeded       = "succeeded"
	domainDiagnosisStatusFailed          = "failed"
	domainDiagnosisStatusCancelled       = "cancelled"
)
