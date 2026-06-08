package temporal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestReportWorkflowScheduleRegistrarBuildsCreateOptions(t *testing.T) {
	schedule := mustRegistrarSchedule(t, true)
	registrar := newReportWorkflowScheduleRegistrar(
		&recordingScheduleCreator{},
		WithReportWorkflowScheduleRegistrarTaskQueue("report-schedules"),
		WithReportWorkflowScheduleRegistrarWorkflowExecutionTimeout(20*time.Minute),
	)

	options, err := registrar.BuildCreateOptions(schedule)
	if err != nil {
		t.Fatalf("BuildCreateOptions: %v", err)
	}
	if options.ID != "report-schedule-primary" {
		t.Fatalf("ID = %q", options.ID)
	}
	if len(options.Spec.Intervals) != 1 ||
		options.Spec.Intervals[0].Every != 24*time.Hour ||
		options.Spec.Intervals[0].Offset != 30*time.Minute {
		t.Fatalf("Intervals = %+v", options.Spec.Intervals)
	}
	if options.Overlap != enumspb.SCHEDULE_OVERLAP_POLICY_SKIP {
		t.Fatalf("Overlap = %v, want skip", options.Overlap)
	}
	if options.CatchupWindow != 10*time.Minute {
		t.Fatalf("CatchupWindow = %s", options.CatchupWindow)
	}
	if options.Paused {
		t.Fatal("Paused = true, want false for enabled schedule")
	}
	action, ok := options.Action.(*client.ScheduleWorkflowAction)
	if !ok {
		t.Fatalf("Action type = %T, want *client.ScheduleWorkflowAction", options.Action)
	}
	if action.ID != "report-policy-schedule-42" ||
		action.TaskQueue != "report-schedules" ||
		action.WorkflowExecutionTimeout != 20*time.Minute ||
		action.WorkflowTaskTimeout != defaultScheduleLauncherWorkflowTaskTimeout {
		t.Fatalf("Action = %+v", action)
	}
	if _, ok := action.Workflow.(func(workflow.Context, ReportPolicyScheduleLauncherWorkflowInput) (ReportPolicyScheduleLauncherWorkflowResult, error)); !ok {
		t.Fatalf("Workflow type = %T", action.Workflow)
	}
	if len(action.Args) != 1 {
		t.Fatalf("Args len = %d, want 1", len(action.Args))
	}
	input, ok := action.Args[0].(ReportPolicyScheduleLauncherWorkflowInput)
	if !ok {
		t.Fatalf("Args[0] type = %T", action.Args[0])
	}
	if input.ScheduleID != 42 ||
		input.ReportWorkflowPolicyID != 7 ||
		input.TemporalScheduleID != "report-schedule-primary" ||
		input.ReplayWindowSeconds != 7200 ||
		input.ReplayDelaySeconds != 300 ||
		input.ReplayLimit != 500 {
		t.Fatalf("launcher input = %+v", input)
	}
}

func TestReportWorkflowScheduleRegistrarCreateCallsTemporal(t *testing.T) {
	schedule := mustRegistrarSchedule(t, false)
	creator := &recordingScheduleCreator{handle: &fakeScheduleHandle{id: "report-schedule-primary"}}
	registrar := newReportWorkflowScheduleRegistrar(creator)

	id, err := registrar.Create(context.Background(), schedule)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id != "report-schedule-primary" {
		t.Fatalf("id = %q", id)
	}
	if creator.calls != 1 {
		t.Fatalf("calls = %d, want 1", creator.calls)
	}
	if !creator.options.Paused {
		t.Fatal("Paused = false, want true for disabled schedule")
	}
}

func TestReportWorkflowScheduleRegistrarSyncCreatesMissingSchedule(t *testing.T) {
	schedule := mustRegistrarSchedule(t, true)
	creator := &recordingScheduleCreator{handle: &fakeScheduleHandle{id: "report-schedule-primary"}}
	registrar := newReportWorkflowScheduleRegistrar(creator)

	result, err := registrar.Sync(context.Background(), schedule)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Action != ReportWorkflowScheduleSyncActionCreated ||
		result.ScheduleID != "report-schedule-primary" ||
		result.Paused {
		t.Fatalf("result = %+v", result)
	}
	if creator.calls != 1 || creator.getHandleCalls != 0 {
		t.Fatalf("create calls = %d, get handle calls = %d", creator.calls, creator.getHandleCalls)
	}
}

func TestReportWorkflowScheduleRegistrarSyncUpdatesExistingSchedule(t *testing.T) {
	schedule := mustRegistrarSchedule(t, false)
	handle := &fakeScheduleHandle{id: "report-schedule-primary"}
	creator := &recordingScheduleCreator{
		err: serviceerror.NewAlreadyExists("schedule exists"),
		handles: map[string]client.ScheduleHandle{
			"report-schedule-primary": handle,
		},
	}
	registrar := newReportWorkflowScheduleRegistrar(creator)

	result, err := registrar.Sync(context.Background(), schedule)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.Action != ReportWorkflowScheduleSyncActionUpdated ||
		result.ScheduleID != "report-schedule-primary" ||
		!result.Paused {
		t.Fatalf("result = %+v", result)
	}
	if creator.calls != 1 || creator.getHandleCalls != 1 || handle.updateCalls != 1 {
		t.Fatalf("calls: create=%d get=%d update=%d", creator.calls, creator.getHandleCalls, handle.updateCalls)
	}
	if handle.updatedSchedule == nil ||
		handle.updatedSchedule.State == nil ||
		!handle.updatedSchedule.State.Paused ||
		handle.updatedSchedule.Policy == nil ||
		handle.updatedSchedule.Policy.Overlap != enumspb.SCHEDULE_OVERLAP_POLICY_SKIP ||
		handle.updatedSchedule.Policy.CatchupWindow != 10*time.Minute {
		t.Fatalf("updated schedule = %+v", handle.updatedSchedule)
	}
	if handle.updatedSchedule.Spec == nil ||
		len(handle.updatedSchedule.Spec.Intervals) != 1 ||
		handle.updatedSchedule.Spec.Intervals[0].Every != 24*time.Hour {
		t.Fatalf("updated spec = %+v", handle.updatedSchedule.Spec)
	}
	action, ok := handle.updatedSchedule.Action.(*client.ScheduleWorkflowAction)
	if !ok {
		t.Fatalf("updated action type = %T", handle.updatedSchedule.Action)
	}
	if action.ID != "report-policy-schedule-42" || action.TaskQueue != TaskQueue {
		t.Fatalf("updated action = %+v", action)
	}
}

func TestReportWorkflowScheduleRegistrarReconcileSummarizesBatch(t *testing.T) {
	first := mustRegistrarSchedule(t, true)
	second := mustRegistrarSchedule(t, false)
	second.ID = 43
	second.TemporalScheduleID = "report-schedule-secondary"
	secondaryHandle := &fakeScheduleHandle{id: "report-schedule-secondary"}
	creator := &recordingScheduleCreator{
		handle: &fakeScheduleHandle{id: "report-schedule-primary"},
		createErrByID: map[string]error{
			"report-schedule-secondary": serviceerror.NewAlreadyExists("schedule exists"),
		},
		handles: map[string]client.ScheduleHandle{
			"report-schedule-secondary": secondaryHandle,
		},
	}
	registrar := newReportWorkflowScheduleRegistrar(creator)

	result, err := registrar.Reconcile(context.Background(), []domain.ReportWorkflowSchedule{first, second})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Total != 2 || result.Created != 1 || result.Updated != 1 {
		t.Fatalf("result = %+v", result)
	}
	if secondaryHandle.updateCalls != 1 {
		t.Fatalf("secondary update calls = %d, want 1", secondaryHandle.updateCalls)
	}
}

func TestReportWorkflowScheduleRegistrarRejectsInvalidSchedule(t *testing.T) {
	schedule := mustRegistrarSchedule(t, true)
	schedule.ReplayWindow = 1500 * time.Millisecond
	registrar := newReportWorkflowScheduleRegistrar(&recordingScheduleCreator{})

	_, err := registrar.BuildCreateOptions(schedule)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
	if err == nil || !strings.Contains(err.Error(), "whole-second precision") {
		t.Fatalf("err = %v", err)
	}
}

func mustRegistrarSchedule(t *testing.T, enabled bool) domain.ReportWorkflowSchedule {
	t.Helper()
	var enabledAt *time.Time
	var disabledAt *time.Time
	if enabled {
		at := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
		enabledAt = &at
	} else {
		at := time.Date(2026, 6, 6, 8, 0, 0, 0, time.UTC)
		disabledAt = &at
	}
	schedule, err := domain.NewReportWorkflowSchedule(
		"Primary report schedule",
		7,
		"report-schedule-primary",
		24*time.Hour,
		30*time.Minute,
		2*time.Hour,
		5*time.Minute,
		500,
		10*time.Minute,
		enabled,
		enabledAt,
		disabledAt,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowSchedule: %v", err)
	}
	schedule.ID = 42
	return schedule
}

type recordingScheduleCreator struct {
	calls          int
	getHandleCalls int
	options        client.ScheduleOptions
	handle         client.ScheduleHandle
	err            error
	createErrByID  map[string]error
	handles        map[string]client.ScheduleHandle
}

func (c *recordingScheduleCreator) Create(
	_ context.Context,
	options client.ScheduleOptions,
) (client.ScheduleHandle, error) {
	c.calls++
	c.options = options
	if c.createErrByID != nil {
		if err := c.createErrByID[options.ID]; err != nil {
			return nil, err
		}
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.handle, nil
}

func (c *recordingScheduleCreator) GetHandle(_ context.Context, scheduleID string) client.ScheduleHandle {
	c.getHandleCalls++
	if c.handles != nil {
		return c.handles[scheduleID]
	}
	return c.handle
}

type fakeScheduleHandle struct {
	id              string
	updateCalls     int
	updatedSchedule *client.Schedule
	updateErr       error
}

func (h *fakeScheduleHandle) GetID() string {
	return h.id
}

func (*fakeScheduleHandle) Delete(context.Context) error {
	return nil
}

func (*fakeScheduleHandle) Backfill(context.Context, client.ScheduleBackfillOptions) error {
	return nil
}

func (h *fakeScheduleHandle) Update(_ context.Context, options client.ScheduleUpdateOptions) error {
	h.updateCalls++
	if h.updateErr != nil {
		return h.updateErr
	}
	update, err := options.DoUpdate(client.ScheduleUpdateInput{})
	if err != nil {
		return err
	}
	if update != nil {
		h.updatedSchedule = update.Schedule
	}
	return nil
}

func (*fakeScheduleHandle) Describe(context.Context) (*client.ScheduleDescription, error) {
	return nil, nil
}

func (*fakeScheduleHandle) Trigger(context.Context, client.ScheduleTriggerOptions) error {
	return nil
}

func (*fakeScheduleHandle) Pause(context.Context, client.SchedulePauseOptions) error {
	return nil
}

func (*fakeScheduleHandle) Unpause(context.Context, client.ScheduleUnpauseOptions) error {
	return nil
}
