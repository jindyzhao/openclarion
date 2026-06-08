package temporal_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

func TestReportPolicyScheduleLauncherWorkflow_ComputesReplayWindowAndRunsActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	fireTime := time.Date(2026, 6, 6, 10, 0, 0, 123456789, time.FixedZone("HKT", 8*60*60))
	tw.SetStartTime(fireTime)

	var captured temporalpkg.ScheduledReportPolicyReplayActivityInput
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ScheduledReportPolicyReplayActivityInput) (temporalpkg.ScheduledReportPolicyReplayActivityResult, error) {
			captured = got
			return temporalpkg.ScheduledReportPolicyReplayActivityResult{
				ScheduleID:                 got.ScheduleID,
				ReportWorkflowPolicyID:     got.ReportWorkflowPolicyID,
				TemporalScheduleID:         got.TemporalScheduleID,
				FireTime:                   got.FireTime,
				WindowStart:                got.WindowStart,
				WindowEnd:                  got.WindowEnd,
				CorrelationKey:             got.CorrelationKey,
				WorkflowID:                 got.WorkflowID,
				EventsLoaded:               8,
				Snapshots:                  2,
				ReportBatchWorkflowStarted: true,
				ReportBatchWorkflowID:      "report-batch-1",
				ReportBatchRunID:           "run-1",
			}, nil
		},
		activity.RegisterOptions{Name: "RunScheduledReportPolicyReplay"},
	)

	tw.ExecuteWorkflow(temporalpkg.ReportPolicyScheduleLauncherWorkflow, temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
		ScheduleID:             7,
		ReportWorkflowPolicyID: 13,
		TemporalScheduleID:     "report-schedule-primary",
		ReplayWindowSeconds:    3600,
		ReplayDelaySeconds:     300,
		ReplayLimit:            250,
	})
	if !tw.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := tw.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	wantFire := time.Date(2026, 6, 6, 2, 0, 0, 123456000, time.UTC)
	wantEnd := time.Date(2026, 6, 6, 1, 55, 0, 123456000, time.UTC)
	wantStart := time.Date(2026, 6, 6, 0, 55, 0, 123456000, time.UTC)
	if captured.ScheduleID != 7 || captured.ReportWorkflowPolicyID != 13 ||
		captured.TemporalScheduleID != "report-schedule-primary" ||
		!captured.FireTime.Equal(wantFire) ||
		!captured.WindowStart.Equal(wantStart) ||
		!captured.WindowEnd.Equal(wantEnd) ||
		captured.ReplayLimit != 250 {
		t.Fatalf("captured activity input = %+v", captured)
	}
	if !strings.Contains(captured.CorrelationKey, "report-workflow-schedule:7:policy:13") {
		t.Fatalf("CorrelationKey = %q", captured.CorrelationKey)
	}
	if !strings.HasPrefix(captured.WorkflowID, "report-schedule-") {
		t.Fatalf("WorkflowID = %q", captured.WorkflowID)
	}

	var result temporalpkg.ReportPolicyScheduleLauncherWorkflowResult
	if err := tw.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult: %v", err)
	}
	if result.EventsLoaded != 8 || result.Snapshots != 2 ||
		!result.ReportBatchWorkflowStarted ||
		result.ReportBatchWorkflowID != "report-batch-1" ||
		result.ReportBatchRunID != "run-1" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReportPolicyScheduleLauncherWorkflow_RejectsInvalidInputBeforeActivity(t *testing.T) {
	tests := []struct {
		name       string
		input      temporalpkg.ReportPolicyScheduleLauncherWorkflowInput
		wantSubstr string
	}{
		{
			name: "missing schedule",
			input: temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
				ReportWorkflowPolicyID: 1,
				TemporalScheduleID:     "report-schedule",
				ReplayWindowSeconds:    60,
				ReplayLimit:            1,
			},
			wantSubstr: "schedule_id must be positive",
		},
		{
			name: "missing policy",
			input: temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
				ScheduleID:          1,
				TemporalScheduleID:  "report-schedule",
				ReplayWindowSeconds: 60,
				ReplayLimit:         1,
			},
			wantSubstr: "report_workflow_policy_id must be positive",
		},
		{
			name: "missing temporal id",
			input: temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
				ScheduleID:             1,
				ReportWorkflowPolicyID: 2,
				ReplayWindowSeconds:    60,
				ReplayLimit:            1,
			},
			wantSubstr: "temporal_schedule_id must be non-empty",
		},
		{
			name: "bad window",
			input: temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
				ScheduleID:             1,
				ReportWorkflowPolicyID: 2,
				TemporalScheduleID:     "report-schedule",
				ReplayLimit:            1,
			},
			wantSubstr: "replay_window_seconds must be positive",
		},
		{
			name: "overflowing delay",
			input: temporalpkg.ReportPolicyScheduleLauncherWorkflowInput{
				ScheduleID:             1,
				ReportWorkflowPolicyID: 2,
				TemporalScheduleID:     "report-schedule",
				ReplayWindowSeconds:    60,
				ReplayDelaySeconds:     9223372037,
				ReplayLimit:            1,
			},
			wantSubstr: "replay_delay_seconds exceeds maximum workflow duration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			tw := suite.NewTestWorkflowEnvironment()
			tw.RegisterActivityWithOptions(
				func(context.Context, temporalpkg.ScheduledReportPolicyReplayActivityInput) (temporalpkg.ScheduledReportPolicyReplayActivityResult, error) {
					t.Fatal("activity should not run")
					return temporalpkg.ScheduledReportPolicyReplayActivityResult{}, nil
				},
				activity.RegisterOptions{Name: "RunScheduledReportPolicyReplay"},
			)

			tw.ExecuteWorkflow(temporalpkg.ReportPolicyScheduleLauncherWorkflow, tc.input)
			if !tw.IsWorkflowCompleted() {
				t.Fatal("workflow did not complete")
			}
			err := tw.GetWorkflowError()
			if err == nil {
				t.Fatalf("expected workflow error containing %q, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("workflow error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestActivities_RunScheduledReportPolicyReplayUsesPolicyReplayer(t *testing.T) {
	replayer := &recordingPolicyReplayer{
		result: reporttrigger.Result{
			Replay: alertreplay.Result{
				Stats: alertreplay.Stats{EventsLoaded: 9},
				Snapshots: []alertreplay.SnapshotRef{
					{ID: 101, GroupIndex: 0, EventCount: 3},
					{ID: 102, GroupIndex: 1, EventCount: 2},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-2", RunID: "run-2"},
			Started:  true,
		},
	}
	activities := temporalpkg.NewActivities(nil, temporalpkg.WithReportPolicyReplayer(replayer))
	windowStart := time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)

	result, err := activities.RunScheduledReportPolicyReplay(context.Background(), temporalpkg.ScheduledReportPolicyReplayActivityInput{
		ScheduleID:             5,
		ReportWorkflowPolicyID: 8,
		TemporalScheduleID:     "report-schedule",
		FireTime:               windowEnd.Add(5 * time.Minute),
		WindowStart:            windowStart,
		WindowEnd:              windowEnd,
		ReplayLimit:            500,
		CorrelationKey:         "report-workflow-schedule:5",
		WorkflowID:             "report-schedule-hash",
	})
	if err != nil {
		t.Fatalf("RunScheduledReportPolicyReplay: %v", err)
	}
	if replayer.calls != 1 {
		t.Fatalf("replayer calls = %d, want 1", replayer.calls)
	}
	if replayer.req.PolicyID != 8 ||
		!replayer.req.WindowStart.Equal(windowStart) ||
		!replayer.req.WindowEnd.Equal(windowEnd) ||
		replayer.req.Limit != 500 ||
		replayer.req.CorrelationKey != "report-workflow-schedule:5" ||
		replayer.req.WorkflowID != "report-schedule-hash" ||
		replayer.req.CreatedByWorkflow != "ReportPolicyScheduleLauncherWorkflow" {
		t.Fatalf("replayer request = %+v", replayer.req)
	}
	if result.EventsLoaded != 9 || result.Snapshots != 2 ||
		!result.ReportBatchWorkflowStarted ||
		result.ReportBatchWorkflowID != "report-batch-2" ||
		result.ReportBatchRunID != "run-2" {
		t.Fatalf("result = %+v", result)
	}
}

func TestActivities_RunScheduledReportPolicyReplayRequiresReplayer(t *testing.T) {
	activities := temporalpkg.NewActivities(nil)
	_, err := activities.RunScheduledReportPolicyReplay(context.Background(), temporalpkg.ScheduledReportPolicyReplayActivityInput{
		ScheduleID:             5,
		ReportWorkflowPolicyID: 8,
		TemporalScheduleID:     "report-schedule",
		WindowStart:            time.Date(2026, 6, 6, 9, 0, 0, 0, time.UTC),
		WindowEnd:              time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
		ReplayLimit:            500,
		CorrelationKey:         "report-workflow-schedule:5",
		WorkflowID:             "report-schedule-hash",
	})
	if err == nil {
		t.Fatal("expected missing replayer error, got nil")
	}
	if !strings.Contains(err.Error(), "report policy replayer is not configured") {
		t.Fatalf("error = %q", err.Error())
	}
}

type recordingPolicyReplayer struct {
	calls  int
	req    reportpolicytrigger.Request
	result reporttrigger.Result
	err    error
}

func (r *recordingPolicyReplayer) ReplayAndStart(
	_ context.Context,
	req reportpolicytrigger.Request,
) (reporttrigger.Result, error) {
	r.calls++
	r.req = req
	if r.err != nil {
		return reporttrigger.Result{}, r.err
	}
	if req.PolicyID == 0 {
		return reporttrigger.Result{}, errors.New("policy id was not forwarded")
	}
	return r.result, nil
}
