package temporal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type recordingWorkflowExecutor struct {
	called   int
	options  client.StartWorkflowOptions
	workflow interface{}
	args     []interface{}
	run      client.WorkflowRun
	err      error
}

func (e *recordingWorkflowExecutor) ExecuteWorkflow(_ context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error) {
	e.called++
	e.options = options
	e.workflow = workflow
	e.args = append([]interface{}(nil), args...)
	if e.err != nil {
		return nil, e.err
	}
	return e.run, nil
}

type staticWorkflowRun struct {
	workflowID string
	runID      string
}

func (r staticWorkflowRun) GetID() string {
	return r.workflowID
}

func (r staticWorkflowRun) GetRunID() string {
	return r.runID
}

func (r staticWorkflowRun) Get(context.Context, interface{}) error {
	return nil
}

func (r staticWorkflowRun) GetWithOptions(context.Context, interface{}, client.WorkflowRunGetOptions) error {
	return nil
}

func TestReportStarter_StartReportBatchMapsRequestAndOptions(t *testing.T) {
	executor := &recordingWorkflowExecutor{
		run: staticWorkflowRun{workflowID: "report-batch-1", runID: "run-1"},
	}
	starter := newReportStarter(
		executor,
		WithReportStarterTaskQueue("reports"),
		WithReportStarterWorkflowExecutionTimeout(20*time.Minute),
	)

	handle, err := starter.StartReportBatch(context.Background(), ports.ReportBatchStartRequest{
		WorkflowID:                         " report-batch-1 ",
		CorrelationKey:                     " window-1 ",
		ReportNotificationChannelProfileID: 9,
		Items: []ports.ReportBatchStartItem{
			{EvidenceSnapshotID: domain.EvidenceSnapshotID(101), Scenario: " single_alert ", GroupIndex: 0},
			{EvidenceSnapshotID: domain.EvidenceSnapshotID(102), Scenario: "cascade", GroupIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("StartReportBatch: %v", err)
	}
	if handle.WorkflowID != "report-batch-1" || handle.RunID != "run-1" {
		t.Fatalf("handle = %+v, want workflowID/report-batch-1 runID/run-1", handle)
	}
	if executor.called != 1 {
		t.Fatalf("ExecuteWorkflow calls = %d, want 1", executor.called)
	}
	if executor.options.ID != "report-batch-1" {
		t.Fatalf("options.ID = %q, want report-batch-1", executor.options.ID)
	}
	if executor.options.TaskQueue != "reports" {
		t.Fatalf("options.TaskQueue = %q, want reports", executor.options.TaskQueue)
	}
	if executor.options.WorkflowExecutionTimeout != 20*time.Minute {
		t.Fatalf("WorkflowExecutionTimeout = %s, want 20m", executor.options.WorkflowExecutionTimeout)
	}
	if executor.options.WorkflowTaskTimeout != defaultReportStartWorkflowTaskTimeout {
		t.Fatalf("WorkflowTaskTimeout = %s, want %s", executor.options.WorkflowTaskTimeout, defaultReportStartWorkflowTaskTimeout)
	}
	if executor.options.WorkflowIDReusePolicy != enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE {
		t.Fatalf("WorkflowIDReusePolicy = %s, want REJECT_DUPLICATE", executor.options.WorkflowIDReusePolicy)
	}
	if executor.options.WorkflowIDConflictPolicy != enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING {
		t.Fatalf("WorkflowIDConflictPolicy = %s, want USE_EXISTING", executor.options.WorkflowIDConflictPolicy)
	}
	if executor.options.WorkflowExecutionErrorWhenAlreadyStarted {
		t.Fatal("WorkflowExecutionErrorWhenAlreadyStarted = true, want false for idempotent handle return")
	}
	if reflect.ValueOf(executor.workflow).Pointer() != reflect.ValueOf(ReportBatchWorkflow).Pointer() {
		t.Fatalf("workflow = %T, want ReportBatchWorkflow", executor.workflow)
	}
	if len(executor.args) != 1 {
		t.Fatalf("args len = %d, want 1", len(executor.args))
	}
	input, ok := executor.args[0].(ReportBatchWorkflowInput)
	if !ok {
		t.Fatalf("arg[0] type = %T, want ReportBatchWorkflowInput", executor.args[0])
	}
	if input.CorrelationKey != "window-1" {
		t.Fatalf("input.CorrelationKey = %q, want window-1", input.CorrelationKey)
	}
	if input.ReportNotificationChannelProfileID != 9 {
		t.Fatalf("input.ReportNotificationChannelProfileID = %d, want 9", input.ReportNotificationChannelProfileID)
	}
	wantItems := []ReportBatchItem{
		{EvidenceSnapshotID: 101, Scenario: "single_alert", GroupIndex: 0},
		{EvidenceSnapshotID: 102, Scenario: "cascade", GroupIndex: 1},
	}
	if !reflect.DeepEqual(input.Items, wantItems) {
		t.Fatalf("input.Items = %+v, want %+v", input.Items, wantItems)
	}
}

func TestReportStarter_StartReportBatchScopesWorkflowIDByTenant(t *testing.T) {
	t.Parallel()

	const scopedID = "openclarion-tenant-7-team-seven--report-batch-1"
	executor := &recordingWorkflowExecutor{
		run: staticWorkflowRun{workflowID: scopedID, runID: "run-1"},
	}
	ctx, err := tenancy.WithTenant(context.Background(), tenancy.Identity{ID: 7, Key: "team-seven"})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	starter := newReportStarter(executor)
	handle, err := starter.StartReportBatch(ctx, ports.ReportBatchStartRequest{
		WorkflowID:     "report-batch-1",
		CorrelationKey: "window-1",
		Items: []ports.ReportBatchStartItem{{
			EvidenceSnapshotID: 101,
			Scenario:           "single_alert",
		}},
	})
	if err != nil {
		t.Fatalf("StartReportBatch: %v", err)
	}
	if executor.options.ID != scopedID || handle.WorkflowID != scopedID {
		t.Fatalf("workflow IDs = option:%q handle:%q, want %q", executor.options.ID, handle.WorkflowID, scopedID)
	}
}

func TestReportStarter_DefaultWorkflowExecutionTimeoutAllowsSlowReportLLM(t *testing.T) {
	executor := &recordingWorkflowExecutor{
		run: staticWorkflowRun{workflowID: "report-batch-1", runID: "run-1"},
	}
	starter := newReportStarter(executor)

	_, err := starter.StartReportBatch(context.Background(), ports.ReportBatchStartRequest{
		WorkflowID:     "report-batch-1",
		CorrelationKey: "window-1",
		Items: []ports.ReportBatchStartItem{
			{EvidenceSnapshotID: domain.EvidenceSnapshotID(101), Scenario: "single_alert", GroupIndex: 0},
		},
	})
	if err != nil {
		t.Fatalf("StartReportBatch: %v", err)
	}
	if executor.options.WorkflowExecutionTimeout != defaultReportStartWorkflowExecutionTimeout {
		t.Fatalf(
			"WorkflowExecutionTimeout = %s, want %s",
			executor.options.WorkflowExecutionTimeout,
			defaultReportStartWorkflowExecutionTimeout,
		)
	}
}

func TestReportStarter_StartReportBatchValidation(t *testing.T) {
	good := ports.ReportBatchStartRequest{
		WorkflowID:     "report-batch-1",
		CorrelationKey: "window-1",
		Items: []ports.ReportBatchStartItem{
			{EvidenceSnapshotID: 1, Scenario: "single_alert", GroupIndex: 0},
		},
	}
	cases := []struct {
		name    string
		starter *ReportStarter
		req     ports.ReportBatchStartRequest
	}{
		{
			name:    "nil_starter",
			starter: nil,
			req:     good,
		},
		{
			name:    "nil_executor",
			starter: &ReportStarter{},
			req:     good,
		},
		{
			name:    "empty_workflow_id",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.WorkflowID = " "
				return req
			}(),
		},
		{
			name:    "empty_correlation_key",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.CorrelationKey = ""
				return req
			}(),
		},
		{
			name:    "empty_items",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.Items = nil
				return req
			}(),
		},
		{
			name:    "negative_notification_channel",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.ReportNotificationChannelProfileID = -1
				return req
			}(),
		},
		{
			name:    "zero_snapshot_id",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.Items = []ports.ReportBatchStartItem{{EvidenceSnapshotID: 0, Scenario: "single_alert", GroupIndex: 0}}
				return req
			}(),
		},
		{
			name:    "empty_scenario",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.Items = []ports.ReportBatchStartItem{{EvidenceSnapshotID: 1, Scenario: " ", GroupIndex: 0}}
				return req
			}(),
		},
		{
			name:    "negative_group_index",
			starter: newReportStarter(&recordingWorkflowExecutor{}),
			req: func() ports.ReportBatchStartRequest {
				req := good
				req.Items = []ports.ReportBatchStartItem{{EvidenceSnapshotID: 1, Scenario: "single_alert", GroupIndex: -1}}
				return req
			}(),
		},
		{
			name: "empty_task_queue",
			starter: newReportStarter(
				&recordingWorkflowExecutor{},
				WithReportStarterTaskQueue(" "),
			),
			req: good,
		},
		{
			name: "bad_timeout",
			starter: newReportStarter(
				&recordingWorkflowExecutor{},
				WithReportStarterWorkflowExecutionTimeout(0),
			),
			req: good,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.starter.StartReportBatch(context.Background(), tc.req)
			if err == nil {
				t.Fatal("StartReportBatch: want error, got nil")
			}
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestReportStarter_StartReportBatchPropagatesExecuteWorkflowError(t *testing.T) {
	wantErr := errors.New("temporal unavailable")
	starter := newReportStarter(&recordingWorkflowExecutor{err: wantErr})

	_, err := starter.StartReportBatch(context.Background(), ports.ReportBatchStartRequest{
		WorkflowID:     "report-batch-1",
		CorrelationKey: "window-1",
		Items: []ports.ReportBatchStartItem{
			{EvidenceSnapshotID: 1, Scenario: "single_alert", GroupIndex: 0},
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}
