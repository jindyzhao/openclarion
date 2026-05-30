package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultDiagnosisRoomStartWorkflowExecutionTimeout = 35 * time.Minute
	defaultDiagnosisRoomStartWorkflowTaskTimeout      = 10 * time.Second
	defaultDiagnosisRoomStartReadyTimeout             = 5 * time.Second
	defaultDiagnosisRoomStartReadyPollInterval        = 50 * time.Millisecond
)

type diagnosisRoomStarterClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
	QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...interface{}) (converter.EncodedValue, error)
}

// DiagnosisRoomStarter starts DiagnosisRoomWorkflow and waits until the
// workflow has materialized the persistent task/session boundary.
type DiagnosisRoomStarter struct {
	client                   diagnosisRoomStarterClient
	taskQueue                string
	workflowExecutionTimeout time.Duration
	workflowTaskTimeout      time.Duration
	readyTimeout             time.Duration
	readyPollInterval        time.Duration
}

// DiagnosisRoomStarterOption customizes DiagnosisRoomStarter runtime options.
type DiagnosisRoomStarterOption func(*DiagnosisRoomStarter)

// WithDiagnosisRoomStarterTaskQueue overrides the Temporal task queue used for
// DiagnosisRoomWorkflow starts.
func WithDiagnosisRoomStarterTaskQueue(taskQueue string) DiagnosisRoomStarterOption {
	return func(s *DiagnosisRoomStarter) {
		s.taskQueue = taskQueue
	}
}

// WithDiagnosisRoomStarterReadyTimeout overrides the maximum time to wait for
// startup persistence before returning the room handle.
func WithDiagnosisRoomStarterReadyTimeout(timeout time.Duration) DiagnosisRoomStarterOption {
	return func(s *DiagnosisRoomStarter) {
		s.readyTimeout = timeout
	}
}

// NewDiagnosisRoomStarter builds a Temporal-backed room workflow starter.
func NewDiagnosisRoomStarter(c client.Client, opts ...DiagnosisRoomStarterOption) (*DiagnosisRoomStarter, error) {
	if c == nil {
		return nil, fmt.Errorf("diagnosis-room starter: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return newDiagnosisRoomStarter(c, opts...), nil
}

func newDiagnosisRoomStarter(c diagnosisRoomStarterClient, opts ...DiagnosisRoomStarterOption) *DiagnosisRoomStarter {
	starter := &DiagnosisRoomStarter{
		client:                   c,
		taskQueue:                TaskQueue,
		workflowExecutionTimeout: defaultDiagnosisRoomStartWorkflowExecutionTimeout,
		workflowTaskTimeout:      defaultDiagnosisRoomStartWorkflowTaskTimeout,
		readyTimeout:             defaultDiagnosisRoomStartReadyTimeout,
		readyPollInterval:        defaultDiagnosisRoomStartReadyPollInterval,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(starter)
		}
	}
	return starter
}

// StartDiagnosisRoom starts the workflow and waits until state query proves the
// ChatSession exists. It does not wait for workflow completion.
func (s *DiagnosisRoomStarter) StartDiagnosisRoom(ctx context.Context, req ports.DiagnosisRoomStartRequest) (ports.DiagnosisRoomStartResult, error) {
	if s == nil || s.client == nil {
		return ports.DiagnosisRoomStartResult{}, fmt.Errorf("diagnosis-room starter: client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	input, workflowID, err := diagnosisRoomWorkflowInputFromStartRequest(req)
	if err != nil {
		return ports.DiagnosisRoomStartResult{}, err
	}
	taskQueue := strings.TrimSpace(s.taskQueue)
	if taskQueue == "" {
		return ports.DiagnosisRoomStartResult{}, fmt.Errorf("diagnosis-room starter: task queue must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if s.workflowExecutionTimeout <= 0 || s.workflowTaskTimeout <= 0 || s.readyTimeout <= 0 || s.readyPollInterval <= 0 {
		return ports.DiagnosisRoomStartResult{}, fmt.Errorf("diagnosis-room starter: timeouts must be positive: %w", domain.ErrInvariantViolation)
	}

	run, err := s.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                                       workflowID,
		TaskQueue:                                taskQueue,
		WorkflowExecutionTimeout:                 s.workflowExecutionTimeout,
		WorkflowTaskTimeout:                      s.workflowTaskTimeout,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
		WorkflowExecutionErrorWhenAlreadyStarted: false,
	}, DiagnosisRoomWorkflow, input)
	if err != nil {
		return ports.DiagnosisRoomStartResult{}, fmt.Errorf("diagnosis-room starter: start workflow: %w", err)
	}

	state, err := s.waitReady(ctx, run.GetID(), run.GetRunID())
	if err != nil {
		return ports.DiagnosisRoomStartResult{}, err
	}
	return ports.DiagnosisRoomStartResult{
		SessionID:          state.SessionID,
		EvidenceSnapshotID: req.EvidenceSnapshotID,
		DiagnosisTaskID:    domain.DiagnosisTaskID(state.DiagnosisTaskID),
		ChatSessionID:      domain.ChatSessionID(state.ChatSessionID),
		Workflow:           ports.WorkflowHandle{WorkflowID: run.GetID(), RunID: run.GetRunID()},
	}, nil
}

func diagnosisRoomWorkflowInputFromStartRequest(req ports.DiagnosisRoomStartRequest) (DiagnosisRoomWorkflowInput, string, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: session id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if sessionID != req.SessionID {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: session id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.EvidenceSnapshotID == 0 {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: evidence_snapshot_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	ownerSubject := strings.TrimSpace(req.OwnerSubject)
	if ownerSubject == "" {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: owner subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len(req.Evidence) == 0 {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: evidence must be non-empty JSON: %w", domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(req.Evidence); err != nil {
		return DiagnosisRoomWorkflowInput{}, "", fmt.Errorf("diagnosis-room starter: evidence must be duplicate-key-free JSON: %w: %w", err, domain.ErrInvariantViolation)
	}
	workflowID, err := DiagnosisRoomWorkflowID(sessionID)
	if err != nil {
		return DiagnosisRoomWorkflowInput{}, "", err
	}
	return DiagnosisRoomWorkflowInput{
		SessionID:          sessionID,
		EvidenceSnapshotID: int64(req.EvidenceSnapshotID),
		OwnerSubject:       ownerSubject,
		Evidence:           append(json.RawMessage(nil), req.Evidence...),
	}, workflowID, nil
}

func (s *DiagnosisRoomStarter) waitReady(ctx context.Context, workflowID, runID string) (DiagnosisRoomWorkflowState, error) {
	readyCtx, cancel := context.WithTimeout(ctx, s.readyTimeout)
	defer cancel()

	var lastErr error
	for {
		state, err := s.queryState(readyCtx, workflowID, runID)
		if err == nil {
			if state.DiagnosisTaskID != 0 && state.ChatSessionID != 0 {
				return state, nil
			}
			lastErr = fmt.Errorf("diagnosis-room starter: room %q is not ready yet", workflowID)
		} else {
			lastErr = err
		}

		timer := time.NewTimer(s.readyPollInterval)
		select {
		case <-readyCtx.Done():
			timer.Stop()
			if lastErr != nil {
				return DiagnosisRoomWorkflowState{}, fmt.Errorf("diagnosis-room starter: wait for room readiness: %w: last error: %w", readyCtx.Err(), lastErr)
			}
			return DiagnosisRoomWorkflowState{}, fmt.Errorf("diagnosis-room starter: wait for room readiness: %w", readyCtx.Err())
		case <-timer.C:
		}
	}
}

func (s *DiagnosisRoomStarter) queryState(ctx context.Context, workflowID, runID string) (DiagnosisRoomWorkflowState, error) {
	response, err := s.client.QueryWorkflow(ctx, workflowID, runID, DiagnosisRoomStateQuery)
	if err != nil {
		return DiagnosisRoomWorkflowState{}, err
	}
	var state DiagnosisRoomWorkflowState
	if err := response.Get(&state); err != nil {
		return DiagnosisRoomWorkflowState{}, fmt.Errorf("diagnosis-room starter: decode state query: %w", err)
	}
	return state, nil
}

var _ ports.DiagnosisRoomWorkflowStarter = (*DiagnosisRoomStarter)(nil)
