package temporal

import (
	"fmt"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
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
	FinalReportID              int64
	NotificationIdempotencyKey string
	ProviderMessageID          string
	NotificationStatus         string
}

// FinalReportWorkflowInput identifies the validated SubReports to reduce
// into one persisted FinalReport.
type FinalReportWorkflowInput struct {
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

	actCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	var result ReportFanOutWorkflowResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).GenerateSubReport, input).Get(ctx, &result); err != nil {
		return ReportFanOutWorkflowResult{}, err
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
	if err := workflow.ExecuteChildWorkflow(childCtx, FinalReportWorkflow, FinalReportWorkflowInput{
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

	actCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	var result FinalReportWorkflowResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).GenerateFinalReport, input).Get(ctx, &result); err != nil {
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

const (
	reportActivityStartToCloseTimeout    = 10 * time.Minute
	reportActivityScheduleToCloseTimeout = 35 * time.Minute
	reportChildWorkflowExecutionTimeout  = 45 * time.Minute
)
