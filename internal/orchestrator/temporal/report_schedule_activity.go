package temporal

import (
	"context"
	"fmt"
	"strings"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

type reportPolicyReplayer interface {
	ReplayAndStart(ctx context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error)
}

// WithReportPolicyReplayer injects the policy replay usecase used by scheduled
// report launcher Activities.
func WithReportPolicyReplayer(replayer reportPolicyReplayer) ActivityOption {
	return func(a *Activities) {
		a.reportPolicyReplayer = replayer
	}
}

// RunScheduledReportPolicyReplay replays the launcher-computed alert window
// through the policy replay service and starts a report batch when snapshots
// are available.
func (a *Activities) RunScheduledReportPolicyReplay(
	ctx context.Context,
	req ScheduledReportPolicyReplayActivityInput,
) (ScheduledReportPolicyReplayActivityResult, error) {
	if a.reportPolicyReplayer == nil {
		return ScheduledReportPolicyReplayActivityResult{}, temporalsdk.NewNonRetryableApplicationError(
			"run-scheduled-report-policy-replay: report policy replayer is not configured", errTypeInvalidInput, nil)
	}
	if err := validateScheduledReplayActivityInput(req); err != nil {
		return ScheduledReportPolicyReplayActivityResult{}, temporalsdk.NewNonRetryableApplicationError(
			"run-scheduled-report-policy-replay: "+err.Error(), errTypeInvalidInput, nil)
	}

	result, err := a.reportPolicyReplayer.ReplayAndStart(ctx, reportpolicytrigger.Request{
		PolicyID:          domain.ReportWorkflowPolicyID(req.ReportWorkflowPolicyID),
		WindowStart:       req.WindowStart,
		WindowEnd:         req.WindowEnd,
		Limit:             req.ReplayLimit,
		CorrelationKey:    strings.TrimSpace(req.CorrelationKey),
		WorkflowID:        strings.TrimSpace(req.WorkflowID),
		CreatedByWorkflow: scheduleLauncherCreatedByWorkflow,
	})
	if err != nil {
		return ScheduledReportPolicyReplayActivityResult{}, mapActivityError(err, "run-scheduled-report-policy-replay")
	}

	return ScheduledReportPolicyReplayActivityResult{
		ScheduleID:                 req.ScheduleID,
		ReportWorkflowPolicyID:     req.ReportWorkflowPolicyID,
		TemporalScheduleID:         strings.TrimSpace(req.TemporalScheduleID),
		FireTime:                   req.FireTime,
		WindowStart:                req.WindowStart,
		WindowEnd:                  req.WindowEnd,
		CorrelationKey:             strings.TrimSpace(req.CorrelationKey),
		WorkflowID:                 strings.TrimSpace(req.WorkflowID),
		EventsLoaded:               result.Replay.Stats.EventsLoaded,
		Snapshots:                  len(result.Replay.Snapshots),
		ReportBatchWorkflowStarted: result.Started,
		ReportBatchWorkflowID:      result.Workflow.WorkflowID,
		ReportBatchRunID:           result.Workflow.RunID,
	}, nil
}

func validateScheduledReplayActivityInput(req ScheduledReportPolicyReplayActivityInput) error {
	if req.ScheduleID <= 0 {
		return fmt.Errorf("schedule_id must be positive")
	}
	if req.ReportWorkflowPolicyID <= 0 {
		return fmt.Errorf("report_workflow_policy_id must be positive")
	}
	if strings.TrimSpace(req.TemporalScheduleID) == "" {
		return fmt.Errorf("temporal_schedule_id must be non-empty")
	}
	if req.WindowStart.IsZero() {
		return fmt.Errorf("window_start must be set")
	}
	if req.WindowEnd.IsZero() {
		return fmt.Errorf("window_end must be set")
	}
	if !req.WindowEnd.After(req.WindowStart) {
		return fmt.Errorf("window_end must be after window_start")
	}
	if req.ReplayLimit <= 0 {
		return fmt.Errorf("replay_limit must be positive")
	}
	if strings.TrimSpace(req.CorrelationKey) == "" {
		return fmt.Errorf("correlation_key must be non-empty")
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		return fmt.Errorf("workflow_id must be non-empty")
	}
	return nil
}
