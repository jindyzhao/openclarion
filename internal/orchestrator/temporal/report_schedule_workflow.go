package temporal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const scheduleLauncherCreatedByWorkflow = "ReportPolicyScheduleLauncherWorkflow"

// maxWorkflowDurationSeconds is floor(max int64 nanoseconds / one second).
const maxWorkflowDurationSeconds = int64(9223372036)

// ReportPolicyScheduleLauncherWorkflowInput is the immutable payload used by a
// Temporal Schedule action to replay one policy-owned alert window.
type ReportPolicyScheduleLauncherWorkflowInput struct {
	ScheduleID             int64
	ReportWorkflowPolicyID int64
	TemporalScheduleID     string
	ReplayWindowSeconds    int64
	ReplayDelaySeconds     int64
	ReplayLimit            int

	// FireTime is optional and primarily used by tests or explicit backfills.
	// Normal Temporal Schedule actions leave it zero because action args are
	// static; the workflow then uses WorkflowStartTime.
	FireTime time.Time
}

// ReportPolicyScheduleLauncherWorkflowResult summarizes the replay window and
// any downstream report batch workflow started by the Activity.
type ReportPolicyScheduleLauncherWorkflowResult struct {
	ScheduleID                 int64
	ReportWorkflowPolicyID     int64
	TemporalScheduleID         string
	FireTime                   time.Time
	WindowStart                time.Time
	WindowEnd                  time.Time
	CorrelationKey             string
	WorkflowID                 string
	EventsLoaded               int
	Snapshots                  int
	ReportBatchWorkflowStarted bool
	ReportBatchWorkflowID      string
	ReportBatchRunID           string
}

// ScheduledReportPolicyReplayActivityInput is the deterministic activity
// boundary produced by ReportPolicyScheduleLauncherWorkflow.
type ScheduledReportPolicyReplayActivityInput struct {
	ScheduleID             int64
	ReportWorkflowPolicyID int64
	TemporalScheduleID     string
	FireTime               time.Time
	WindowStart            time.Time
	WindowEnd              time.Time
	ReplayLimit            int
	CorrelationKey         string
	WorkflowID             string
}

// ScheduledReportPolicyReplayActivityResult returns replay counters and the
// report batch workflow handle when replay produced snapshots.
type ScheduledReportPolicyReplayActivityResult struct {
	ScheduleID                 int64
	ReportWorkflowPolicyID     int64
	TemporalScheduleID         string
	FireTime                   time.Time
	WindowStart                time.Time
	WindowEnd                  time.Time
	CorrelationKey             string
	WorkflowID                 string
	EventsLoaded               int
	Snapshots                  int
	ReportBatchWorkflowStarted bool
	ReportBatchWorkflowID      string
	ReportBatchRunID           string
}

// ReportPolicyScheduleLauncherWorkflow computes the scheduled replay window
// and delegates provider I/O plus report workflow starting to an Activity.
func ReportPolicyScheduleLauncherWorkflow(
	ctx workflow.Context,
	input ReportPolicyScheduleLauncherWorkflowInput,
) (ReportPolicyScheduleLauncherWorkflowResult, error) {
	if err := validateScheduleLauncherInput(input); err != nil {
		return ReportPolicyScheduleLauncherWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			"report-policy-schedule-launcher: "+err.Error(), errTypeInvalidInput, nil)
	}

	fireTime := scheduledFireTime(ctx, input.FireTime)
	replayWindow := time.Duration(input.ReplayWindowSeconds) * time.Second
	replayDelay := time.Duration(input.ReplayDelaySeconds) * time.Second
	windowEnd := normalizeWorkflowTime(fireTime.Add(-replayDelay))
	windowStart := normalizeWorkflowTime(windowEnd.Add(-replayWindow))
	correlationKey := scheduleReplayCorrelationKey(
		input.ScheduleID,
		input.ReportWorkflowPolicyID,
		windowStart,
		windowEnd,
	)
	workflowID := scheduleReplayWorkflowID(correlationKey)

	actCtx := workflow.WithActivityOptions(ctx, reportActivityOptions())
	var result ScheduledReportPolicyReplayActivityResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).RunScheduledReportPolicyReplay, ScheduledReportPolicyReplayActivityInput{
		ScheduleID:             input.ScheduleID,
		ReportWorkflowPolicyID: input.ReportWorkflowPolicyID,
		TemporalScheduleID:     strings.TrimSpace(input.TemporalScheduleID),
		FireTime:               fireTime,
		WindowStart:            windowStart,
		WindowEnd:              windowEnd,
		ReplayLimit:            input.ReplayLimit,
		CorrelationKey:         correlationKey,
		WorkflowID:             workflowID,
	}).Get(ctx, &result); err != nil {
		return ReportPolicyScheduleLauncherWorkflowResult{}, err
	}

	return ReportPolicyScheduleLauncherWorkflowResult{
		ScheduleID:                 result.ScheduleID,
		ReportWorkflowPolicyID:     result.ReportWorkflowPolicyID,
		TemporalScheduleID:         result.TemporalScheduleID,
		FireTime:                   result.FireTime,
		WindowStart:                result.WindowStart,
		WindowEnd:                  result.WindowEnd,
		CorrelationKey:             result.CorrelationKey,
		WorkflowID:                 result.WorkflowID,
		EventsLoaded:               result.EventsLoaded,
		Snapshots:                  result.Snapshots,
		ReportBatchWorkflowStarted: result.ReportBatchWorkflowStarted,
		ReportBatchWorkflowID:      result.ReportBatchWorkflowID,
		ReportBatchRunID:           result.ReportBatchRunID,
	}, nil
}

func validateScheduleLauncherInput(input ReportPolicyScheduleLauncherWorkflowInput) error {
	if input.ScheduleID <= 0 {
		return fmt.Errorf("schedule_id must be positive")
	}
	if input.ReportWorkflowPolicyID <= 0 {
		return fmt.Errorf("report_workflow_policy_id must be positive")
	}
	if strings.TrimSpace(input.TemporalScheduleID) == "" {
		return fmt.Errorf("temporal_schedule_id must be non-empty")
	}
	if input.ReplayWindowSeconds <= 0 {
		return fmt.Errorf("replay_window_seconds must be positive")
	}
	if input.ReplayWindowSeconds > maxWorkflowDurationSeconds {
		return fmt.Errorf("replay_window_seconds exceeds maximum workflow duration")
	}
	if input.ReplayDelaySeconds < 0 {
		return fmt.Errorf("replay_delay_seconds must be non-negative")
	}
	if input.ReplayDelaySeconds > maxWorkflowDurationSeconds {
		return fmt.Errorf("replay_delay_seconds exceeds maximum workflow duration")
	}
	if input.ReplayLimit <= 0 {
		return fmt.Errorf("replay_limit must be positive")
	}
	return nil
}

func scheduledFireTime(ctx workflow.Context, override time.Time) time.Time {
	if !override.IsZero() {
		return normalizeWorkflowTime(override)
	}
	info := workflow.GetInfo(ctx)
	if !info.WorkflowStartTime.IsZero() {
		return normalizeWorkflowTime(info.WorkflowStartTime)
	}
	return normalizeWorkflowTime(workflow.Now(ctx))
}

func normalizeWorkflowTime(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

func scheduleReplayCorrelationKey(scheduleID, policyID int64, windowStart, windowEnd time.Time) string {
	return fmt.Sprintf(
		"report-workflow-schedule:%d:policy:%d:%s:%s",
		scheduleID,
		policyID,
		normalizeWorkflowTime(windowStart).Format(time.RFC3339Nano),
		normalizeWorkflowTime(windowEnd).Format(time.RFC3339Nano),
	)
}

func scheduleReplayWorkflowID(correlationKey string) string {
	sum := sha256.Sum256([]byte(correlationKey))
	return "report-schedule-" + hex.EncodeToString(sum[:16])
}
