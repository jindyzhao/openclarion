package temporal

import (
	"context"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultReportStartWorkflowExecutionTimeout = 90 * time.Minute
	defaultReportStartWorkflowTaskTimeout      = 10 * time.Second
)

type workflowExecutor interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
}

// ReportStarter starts report workflows through a Temporal client.
type ReportStarter struct {
	executor                 workflowExecutor
	taskQueue                string
	workflowExecutionTimeout time.Duration
	workflowTaskTimeout      time.Duration
}

// ReportStarterOption customises ReportStarter runtime options.
type ReportStarterOption func(*ReportStarter)

// WithReportStarterTaskQueue overrides the Temporal task queue used
// when starting ReportBatchWorkflow.
func WithReportStarterTaskQueue(taskQueue string) ReportStarterOption {
	return func(s *ReportStarter) {
		s.taskQueue = taskQueue
	}
}

// WithReportStarterWorkflowExecutionTimeout overrides the execution
// timeout used for ReportBatchWorkflow starts.
func WithReportStarterWorkflowExecutionTimeout(timeout time.Duration) ReportStarterOption {
	return func(s *ReportStarter) {
		s.workflowExecutionTimeout = timeout
	}
}

// NewReportStarter builds a Temporal-backed ReportWorkflowStarter.
func NewReportStarter(c client.Client, opts ...ReportStarterOption) (*ReportStarter, error) {
	if c == nil {
		return nil, fmt.Errorf("report starter: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return newReportStarter(c, opts...), nil
}

func newReportStarter(executor workflowExecutor, opts ...ReportStarterOption) *ReportStarter {
	starter := &ReportStarter{
		executor:                 executor,
		taskQueue:                TaskQueue,
		workflowExecutionTimeout: defaultReportStartWorkflowExecutionTimeout,
		workflowTaskTimeout:      defaultReportStartWorkflowTaskTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(starter)
		}
	}
	return starter
}

// StartReportBatch starts ReportBatchWorkflow and returns its workflow
// handle. It does not block for the workflow result.
func (s *ReportStarter) StartReportBatch(ctx context.Context, req ports.ReportBatchStartRequest) (ports.WorkflowHandle, error) {
	if s == nil || s.executor == nil {
		return ports.WorkflowHandle{}, fmt.Errorf("report starter: executor must be non-nil: %w", domain.ErrInvariantViolation)
	}
	input, err := reportBatchInputFromStartRequest(req)
	if err != nil {
		return ports.WorkflowHandle{}, err
	}

	taskQueue := strings.TrimSpace(s.taskQueue)
	if taskQueue == "" {
		return ports.WorkflowHandle{}, fmt.Errorf("report starter: task queue must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if s.workflowExecutionTimeout <= 0 {
		return ports.WorkflowHandle{}, fmt.Errorf("report starter: workflow execution timeout must be positive: %w", domain.ErrInvariantViolation)
	}
	if s.workflowTaskTimeout <= 0 {
		return ports.WorkflowHandle{}, fmt.Errorf("report starter: workflow task timeout must be positive: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := tenantScopedWorkflowID(ctx, strings.TrimSpace(req.WorkflowID))
	if err != nil {
		return ports.WorkflowHandle{}, err
	}

	run, err := s.executor.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                                       workflowID,
		TaskQueue:                                taskQueue,
		WorkflowExecutionTimeout:                 s.workflowExecutionTimeout,
		WorkflowTaskTimeout:                      s.workflowTaskTimeout,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionErrorWhenAlreadyStarted: false,
	}, ReportBatchWorkflow, input)
	if err != nil {
		return ports.WorkflowHandle{}, fmt.Errorf("report starter: start report batch workflow: %w", err)
	}
	return ports.WorkflowHandle{
		WorkflowID: run.GetID(),
		RunID:      run.GetRunID(),
	}, nil
}

func reportBatchInputFromStartRequest(req ports.ReportBatchStartRequest) (ReportBatchWorkflowInput, error) {
	workflowID := strings.TrimSpace(req.WorkflowID)
	if workflowID == "" {
		return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: workflow ID must be non-empty: %w", domain.ErrInvariantViolation)
	}
	correlationKey := strings.TrimSpace(req.CorrelationKey)
	if correlationKey == "" {
		return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: correlation key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len(req.Items) == 0 {
		return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: items must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.ReportNotificationChannelProfileID < 0 {
		return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: report_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}

	items := make([]ReportBatchItem, len(req.Items))
	for i, item := range req.Items {
		if item.EvidenceSnapshotID == 0 {
			return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: items[%d].evidence_snapshot_id must be non-zero: %w", i, domain.ErrInvariantViolation)
		}
		scenario := strings.TrimSpace(item.Scenario)
		if scenario == "" {
			return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: items[%d].scenario must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
		if item.GroupIndex < 0 {
			return ReportBatchWorkflowInput{}, fmt.Errorf("report starter: items[%d].group_index must be >= 0: %w", i, domain.ErrInvariantViolation)
		}
		items[i] = ReportBatchItem{
			EvidenceSnapshotID: int64(item.EvidenceSnapshotID),
			Scenario:           scenario,
			GroupIndex:         item.GroupIndex,
		}
	}

	return ReportBatchWorkflowInput{
		CorrelationKey:                     correlationKey,
		ReportNotificationChannelProfileID: int64(req.ReportNotificationChannelProfileID),
		Items:                              items,
	}, nil
}
