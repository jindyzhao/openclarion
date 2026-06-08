package temporal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"

	"github.com/openclarion/openclarion/internal/domain"
)

const (
	defaultScheduleLauncherWorkflowExecutionTimeout = 15 * time.Minute
	defaultScheduleLauncherWorkflowTaskTimeout      = 10 * time.Second
)

// DefaultReportWorkflowScheduleReconcileLimit bounds startup schedule
// reconciliation so boot fails instead of silently skipping excess rows.
const DefaultReportWorkflowScheduleReconcileLimit = 1000

type scheduleManager interface {
	Create(ctx context.Context, options client.ScheduleOptions) (client.ScheduleHandle, error)
	GetHandle(ctx context.Context, scheduleID string) client.ScheduleHandle
}

// ReportWorkflowScheduleRegistrar creates Temporal Schedules from persisted
// ReportWorkflowSchedule configuration.
type ReportWorkflowScheduleRegistrar struct {
	schedules                scheduleManager
	taskQueue                string
	workflowExecutionTimeout time.Duration
	workflowTaskTimeout      time.Duration
}

// ReportWorkflowScheduleSyncAction describes how one persisted schedule was
// synchronized with Temporal.
type ReportWorkflowScheduleSyncAction string

const (
	// ReportWorkflowScheduleSyncActionCreated means Temporal accepted a new
	// schedule during synchronization.
	ReportWorkflowScheduleSyncActionCreated ReportWorkflowScheduleSyncAction = "created"

	// ReportWorkflowScheduleSyncActionUpdated means an existing Temporal
	// schedule was updated during synchronization.
	ReportWorkflowScheduleSyncActionUpdated ReportWorkflowScheduleSyncAction = "updated"
)

// ReportWorkflowScheduleSyncResult describes one schedule synchronization.
type ReportWorkflowScheduleSyncResult struct {
	ScheduleID string
	Action     ReportWorkflowScheduleSyncAction
	Paused     bool
}

// ReportWorkflowScheduleReconcileResult summarizes a batch reconciliation.
type ReportWorkflowScheduleReconcileResult struct {
	Total   int
	Created int
	Updated int
}

// ReportWorkflowScheduleRegistrarOption customizes schedule registration.
type ReportWorkflowScheduleRegistrarOption func(*ReportWorkflowScheduleRegistrar)

// WithReportWorkflowScheduleRegistrarTaskQueue overrides the launcher workflow
// task queue.
func WithReportWorkflowScheduleRegistrarTaskQueue(taskQueue string) ReportWorkflowScheduleRegistrarOption {
	return func(r *ReportWorkflowScheduleRegistrar) {
		r.taskQueue = taskQueue
	}
}

// WithReportWorkflowScheduleRegistrarWorkflowExecutionTimeout overrides the
// launcher workflow execution timeout.
func WithReportWorkflowScheduleRegistrarWorkflowExecutionTimeout(timeout time.Duration) ReportWorkflowScheduleRegistrarOption {
	return func(r *ReportWorkflowScheduleRegistrar) {
		r.workflowExecutionTimeout = timeout
	}
}

// NewReportWorkflowScheduleRegistrar constructs a Temporal-backed schedule
// registrar.
func NewReportWorkflowScheduleRegistrar(
	c client.Client,
	opts ...ReportWorkflowScheduleRegistrarOption,
) (*ReportWorkflowScheduleRegistrar, error) {
	if c == nil {
		return nil, fmt.Errorf("report workflow schedule registrar: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return newReportWorkflowScheduleRegistrar(c.ScheduleClient(), opts...), nil
}

func newReportWorkflowScheduleRegistrar(
	schedules scheduleManager,
	opts ...ReportWorkflowScheduleRegistrarOption,
) *ReportWorkflowScheduleRegistrar {
	registrar := &ReportWorkflowScheduleRegistrar{
		schedules:                schedules,
		taskQueue:                TaskQueue,
		workflowExecutionTimeout: defaultScheduleLauncherWorkflowExecutionTimeout,
		workflowTaskTimeout:      defaultScheduleLauncherWorkflowTaskTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(registrar)
		}
	}
	return registrar
}

// Create registers a persisted report workflow schedule with Temporal and
// returns the Temporal schedule ID accepted by the server.
func (r *ReportWorkflowScheduleRegistrar) Create(
	ctx context.Context,
	schedule domain.ReportWorkflowSchedule,
) (string, error) {
	if r == nil || r.schedules == nil {
		return "", fmt.Errorf("report workflow schedule registrar: schedule manager must be non-nil: %w", domain.ErrInvariantViolation)
	}
	options, err := r.BuildCreateOptions(schedule)
	if err != nil {
		return "", err
	}
	handle, err := r.schedules.Create(ctx, options)
	if err != nil {
		return "", fmt.Errorf("report workflow schedule registrar: create Temporal schedule: %w", err)
	}
	if handle == nil {
		return "", fmt.Errorf("report workflow schedule registrar: Temporal schedule create returned nil handle: %w", domain.ErrInvariantViolation)
	}
	return handle.GetID(), nil
}

// Sync creates a missing Temporal Schedule or updates an existing one so
// Temporal matches the persisted schedule metadata and enablement state.
func (r *ReportWorkflowScheduleRegistrar) Sync(
	ctx context.Context,
	schedule domain.ReportWorkflowSchedule,
) (ReportWorkflowScheduleSyncResult, error) {
	if r == nil || r.schedules == nil {
		return ReportWorkflowScheduleSyncResult{}, fmt.Errorf("report workflow schedule registrar: schedule manager must be non-nil: %w", domain.ErrInvariantViolation)
	}
	options, err := r.BuildCreateOptions(schedule)
	if err != nil {
		return ReportWorkflowScheduleSyncResult{}, err
	}
	handle, err := r.schedules.Create(ctx, options)
	if err == nil {
		if handle == nil {
			return ReportWorkflowScheduleSyncResult{}, fmt.Errorf("report workflow schedule registrar: Temporal schedule create returned nil handle: %w", domain.ErrInvariantViolation)
		}
		return ReportWorkflowScheduleSyncResult{
			ScheduleID: handle.GetID(),
			Action:     ReportWorkflowScheduleSyncActionCreated,
			Paused:     options.Paused,
		}, nil
	}
	if !isTemporalAlreadyExists(err) {
		return ReportWorkflowScheduleSyncResult{}, fmt.Errorf("report workflow schedule registrar: create Temporal schedule: %w", err)
	}

	scheduleID := strings.TrimSpace(schedule.TemporalScheduleID)
	handle = r.schedules.GetHandle(ctx, scheduleID)
	if handle == nil {
		return ReportWorkflowScheduleSyncResult{}, fmt.Errorf("report workflow schedule registrar: Temporal schedule handle is nil for %q: %w", scheduleID, domain.ErrInvariantViolation)
	}
	if err := handle.Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			return &client.ScheduleUpdate{Schedule: scheduleFromCreateOptions(options)}, nil
		},
	}); err != nil {
		return ReportWorkflowScheduleSyncResult{}, fmt.Errorf("report workflow schedule registrar: update Temporal schedule %q: %w", scheduleID, err)
	}
	return ReportWorkflowScheduleSyncResult{
		ScheduleID: scheduleID,
		Action:     ReportWorkflowScheduleSyncActionUpdated,
		Paused:     options.Paused,
	}, nil
}

// SyncReportWorkflowSchedule adapts the registrar to HTTP transport code that
// only needs to know whether synchronization succeeded.
func (r *ReportWorkflowScheduleRegistrar) SyncReportWorkflowSchedule(
	ctx context.Context,
	schedule domain.ReportWorkflowSchedule,
) error {
	_, err := r.Sync(ctx, schedule)
	return err
}

// Reconcile synchronizes a bounded batch of persisted schedules with Temporal.
func (r *ReportWorkflowScheduleRegistrar) Reconcile(
	ctx context.Context,
	schedules []domain.ReportWorkflowSchedule,
) (ReportWorkflowScheduleReconcileResult, error) {
	var result ReportWorkflowScheduleReconcileResult
	for _, schedule := range schedules {
		syncResult, err := r.Sync(ctx, schedule)
		if err != nil {
			return result, fmt.Errorf("report workflow schedule registrar: reconcile schedule %d (%q): %w", schedule.ID, strings.TrimSpace(schedule.TemporalScheduleID), err)
		}
		result.Total++
		switch syncResult.Action {
		case ReportWorkflowScheduleSyncActionCreated:
			result.Created++
		case ReportWorkflowScheduleSyncActionUpdated:
			result.Updated++
		}
	}
	return result, nil
}

// BuildCreateOptions maps persisted schedule configuration into a Temporal
// ScheduleOptions value without contacting Temporal.
func (r *ReportWorkflowScheduleRegistrar) BuildCreateOptions(
	schedule domain.ReportWorkflowSchedule,
) (client.ScheduleOptions, error) {
	if r == nil {
		return client.ScheduleOptions{}, fmt.Errorf("report workflow schedule registrar: registrar must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if err := r.validate(schedule); err != nil {
		return client.ScheduleOptions{}, err
	}
	replayWindowSeconds, err := wholeSeconds("replay_window", schedule.ReplayWindow, false)
	if err != nil {
		return client.ScheduleOptions{}, err
	}
	replayDelaySeconds, err := wholeSeconds("replay_delay", schedule.ReplayDelay, true)
	if err != nil {
		return client.ScheduleOptions{}, err
	}

	scheduleID := strings.TrimSpace(schedule.TemporalScheduleID)
	return client.ScheduleOptions{
		ID: scheduleID,
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{{
				Every:  schedule.Interval,
				Offset: schedule.Offset,
			}},
		},
		Action: &client.ScheduleWorkflowAction{
			ID:        scheduleLauncherWorkflowID(schedule.ID),
			Workflow:  ReportPolicyScheduleLauncherWorkflow,
			TaskQueue: strings.TrimSpace(r.taskQueue),
			Args: []interface{}{ReportPolicyScheduleLauncherWorkflowInput{
				ScheduleID:             int64(schedule.ID),
				ReportWorkflowPolicyID: int64(schedule.ReportWorkflowPolicyID),
				TemporalScheduleID:     scheduleID,
				ReplayWindowSeconds:    replayWindowSeconds,
				ReplayDelaySeconds:     replayDelaySeconds,
				ReplayLimit:            schedule.ReplayLimit,
			}},
			WorkflowExecutionTimeout: r.workflowExecutionTimeout,
			WorkflowTaskTimeout:      r.workflowTaskTimeout,
		},
		Overlap:       enumspb.SCHEDULE_OVERLAP_POLICY_SKIP,
		CatchupWindow: schedule.CatchupWindow,
		Paused:        !schedule.Enabled,
		Note:          fmt.Sprintf("OpenClarion report workflow schedule %d", schedule.ID),
	}, nil
}

func (r *ReportWorkflowScheduleRegistrar) validate(schedule domain.ReportWorkflowSchedule) error {
	if schedule.ID <= 0 {
		return fmt.Errorf("report workflow schedule registrar: schedule id must be positive: %w", domain.ErrInvariantViolation)
	}
	if schedule.ReportWorkflowPolicyID <= 0 {
		return fmt.Errorf("report workflow schedule registrar: report workflow policy id must be positive: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(schedule.TemporalScheduleID) == "" {
		return fmt.Errorf("report workflow schedule registrar: temporal schedule id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if schedule.Interval <= 0 {
		return fmt.Errorf("report workflow schedule registrar: interval must be positive: %w", domain.ErrInvariantViolation)
	}
	if schedule.Offset < 0 || schedule.Offset >= schedule.Interval {
		return fmt.Errorf("report workflow schedule registrar: offset must be non-negative and less than interval: %w", domain.ErrInvariantViolation)
	}
	if schedule.ReplayLimit <= 0 {
		return fmt.Errorf("report workflow schedule registrar: replay limit must be positive: %w", domain.ErrInvariantViolation)
	}
	if schedule.CatchupWindow <= 0 {
		return fmt.Errorf("report workflow schedule registrar: catchup window must be positive: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(r.taskQueue) == "" {
		return fmt.Errorf("report workflow schedule registrar: task queue must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if r.workflowExecutionTimeout <= 0 {
		return fmt.Errorf("report workflow schedule registrar: workflow execution timeout must be positive: %w", domain.ErrInvariantViolation)
	}
	if r.workflowTaskTimeout <= 0 {
		return fmt.Errorf("report workflow schedule registrar: workflow task timeout must be positive: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func wholeSeconds(name string, value time.Duration, allowZero bool) (int64, error) {
	if allowZero {
		if value < 0 {
			return 0, fmt.Errorf("report workflow schedule registrar: %s must be non-negative: %w", name, domain.ErrInvariantViolation)
		}
	} else if value <= 0 {
		return 0, fmt.Errorf("report workflow schedule registrar: %s must be positive: %w", name, domain.ErrInvariantViolation)
	}
	if value%time.Second != 0 {
		return 0, fmt.Errorf("report workflow schedule registrar: %s must use whole-second precision: %w", name, domain.ErrInvariantViolation)
	}
	return int64(value / time.Second), nil
}

func scheduleFromCreateOptions(options client.ScheduleOptions) *client.Schedule {
	spec := options.Spec
	state := &client.ScheduleState{
		Note:   options.Note,
		Paused: options.Paused,
	}
	if options.RemainingActions > 0 {
		state.LimitedActions = true
		state.RemainingActions = options.RemainingActions
	}
	return &client.Schedule{
		Action: options.Action,
		Spec:   &spec,
		Policy: &client.SchedulePolicies{
			Overlap:        options.Overlap,
			CatchupWindow:  options.CatchupWindow,
			PauseOnFailure: options.PauseOnFailure,
		},
		State: state,
	}
}

func isTemporalAlreadyExists(err error) bool {
	var alreadyExists *serviceerror.AlreadyExists
	return errors.As(err, &alreadyExists)
}

func scheduleLauncherWorkflowID(id domain.ReportWorkflowScheduleID) string {
	return fmt.Sprintf("report-policy-schedule-%d", id)
}
