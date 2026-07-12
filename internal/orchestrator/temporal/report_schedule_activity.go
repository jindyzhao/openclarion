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
	window, err := validateScheduledReplayActivityInput(req)
	if err != nil {
		return ScheduledReportPolicyReplayActivityResult{}, temporalsdk.NewNonRetryableApplicationError(
			"run-scheduled-report-policy-replay: "+err.Error(), errTypeInvalidInput, nil)
	}

	result, err := a.reportPolicyReplayer.ReplayAndStart(ctx, reportpolicytrigger.Request{
		PolicyID:          domain.ReportWorkflowPolicyID(req.ReportWorkflowPolicyID),
		WindowStart:       window.StartInclusive(),
		WindowEnd:         window.EndExclusive(),
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
		WindowStart:                window.StartInclusive(),
		WindowEnd:                  window.EndExclusive(),
		CorrelationKey:             strings.TrimSpace(req.CorrelationKey),
		WorkflowID:                 strings.TrimSpace(req.WorkflowID),
		EventsLoaded:               result.Replay.Stats.EventsLoaded,
		Snapshots:                  len(result.Replay.Snapshots),
		ReportBatchWorkflowStarted: result.Started,
		ReportBatchWorkflowID:      result.Workflow.WorkflowID,
		ReportBatchRunID:           result.Workflow.RunID,
	}, nil
}

func validateScheduledReplayActivityInput(req ScheduledReportPolicyReplayActivityInput) (domain.AlertWindow, error) {
	if req.ScheduleID <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("schedule_id must be positive")
	}
	if req.ReportWorkflowPolicyID <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("report_workflow_policy_id must be positive")
	}
	if strings.TrimSpace(req.TemporalScheduleID) == "" {
		return domain.AlertWindow{}, fmt.Errorf("temporal_schedule_id must be non-empty")
	}
	window, err := domain.NewAlertWindow(req.WindowStart, req.WindowEnd)
	if err != nil {
		return domain.AlertWindow{}, fmt.Errorf("replay window: %w", err)
	}
	if req.ReplayLimit <= 0 {
		return domain.AlertWindow{}, fmt.Errorf("replay_limit must be positive")
	}
	if strings.TrimSpace(req.CorrelationKey) == "" {
		return domain.AlertWindow{}, fmt.Errorf("correlation_key must be non-empty")
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		return domain.AlertWindow{}, fmt.Errorf("workflow_id must be non-empty")
	}
	return window, nil
}
