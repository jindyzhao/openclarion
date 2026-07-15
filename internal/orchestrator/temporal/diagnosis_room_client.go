package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	temporalsdk "go.temporal.io/sdk/temporal"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const diagnosisRoomWorkflowIDPrefix = "diagnosis-room-"

type diagnosisRoomWorkflowClient interface {
	UpdateWorkflow(ctx context.Context, options client.UpdateWorkflowOptions) (client.WorkflowUpdateHandle, error)
	QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...interface{}) (converter.EncodedValue, error)
	SignalWorkflow(ctx context.Context, workflowID string, runID string, signalName string, arg interface{}) error
	GetWorkflow(ctx context.Context, workflowID string, runID string) client.WorkflowRun
	DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservicepb.DescribeWorkflowExecutionResponse, error)
}

// DiagnosisRoomClient adapts Temporal workflow Update/Query calls to the
// provider-neutral diagnosis-room orchestration port.
type DiagnosisRoomClient struct {
	client diagnosisRoomWorkflowClient
}

// NewDiagnosisRoomClient builds a Temporal-backed DiagnosisRoomWorkflowClient.
func NewDiagnosisRoomClient(c client.Client) (*DiagnosisRoomClient, error) {
	if c == nil {
		return nil, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return newDiagnosisRoomClient(c), nil
}

func newDiagnosisRoomClient(c diagnosisRoomWorkflowClient) *DiagnosisRoomClient {
	return &DiagnosisRoomClient{client: c}
}

// ListDiagnosisRoomWorkflowVisibility returns sanitized workflow execution
// metadata for diagnosis-room list surfaces.
func (c *DiagnosisRoomClient) ListDiagnosisRoomWorkflowVisibility(
	ctx context.Context,
	requests []ports.DiagnosisRoomWorkflowVisibilityRequest,
) (map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("diagnosis-room visibility: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	out := make(map[ports.DiagnosisRoomWorkflowVisibilityRequest]ports.DiagnosisRoomWorkflowVisibility, len(requests))
	for _, req := range requests {
		normalized, err := normalizeDiagnosisRoomVisibilityRequest(req)
		if err != nil {
			return nil, err
		}
		if _, exists := out[normalized]; exists {
			continue
		}
		resp, err := c.client.DescribeWorkflowExecution(ctx, normalized.WorkflowID, normalized.RunID)
		if err != nil {
			var notFound *serviceerror.NotFound
			if errors.As(err, &notFound) {
				out[normalized] = ports.DiagnosisRoomWorkflowVisibility{
					WorkflowID: normalized.WorkflowID,
					RunID:      normalized.RunID,
					Status:     "not_found",
				}
				continue
			}
			return nil, fmt.Errorf("describe diagnosis-room workflow %q: %w", normalized.WorkflowID, err)
		}
		out[normalized] = diagnosisRoomWorkflowVisibilityFromDescribe(normalized, resp)
	}
	return out, nil
}

func normalizeDiagnosisRoomVisibilityRequest(
	req ports.DiagnosisRoomWorkflowVisibilityRequest,
) (ports.DiagnosisRoomWorkflowVisibilityRequest, error) {
	req.WorkflowID = strings.TrimSpace(req.WorkflowID)
	req.RunID = strings.TrimSpace(req.RunID)
	if req.WorkflowID == "" {
		return ports.DiagnosisRoomWorkflowVisibilityRequest{}, fmt.Errorf("diagnosis-room visibility: workflow_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return req, nil
}

func diagnosisRoomWorkflowVisibilityFromDescribe(
	req ports.DiagnosisRoomWorkflowVisibilityRequest,
	resp *workflowservicepb.DescribeWorkflowExecutionResponse,
) ports.DiagnosisRoomWorkflowVisibility {
	out := ports.DiagnosisRoomWorkflowVisibility{
		WorkflowID: req.WorkflowID,
		RunID:      req.RunID,
		Status:     "unknown",
	}
	if resp == nil || resp.GetWorkflowExecutionInfo() == nil {
		return out
	}
	info := resp.GetWorkflowExecutionInfo()
	if execution := info.GetExecution(); execution != nil {
		if workflowID := strings.TrimSpace(execution.GetWorkflowId()); workflowID != "" {
			out.WorkflowID = workflowID
		}
		if runID := strings.TrimSpace(execution.GetRunId()); runID != "" {
			out.RunID = runID
		}
	}
	out.Status = diagnosisRoomWorkflowExecutionStatusString(info.GetStatus())
	out.TaskQueue = strings.TrimSpace(info.GetTaskQueue())
	out.StartTime = protoTimePtr(info.GetStartTime())
	out.ExecutionTime = protoTimePtr(info.GetExecutionTime())
	out.CloseTime = protoTimePtr(info.GetCloseTime())
	out.HistoryLength = info.GetHistoryLength()
	out.HistorySizeBytes = info.GetHistorySizeBytes()
	return out
}

func diagnosisRoomWorkflowExecutionStatusString(status enumspb.WorkflowExecutionStatus) string {
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return "running"
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return "completed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED:
		return "failed"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return "canceled"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return "terminated"
	case enumspb.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return "continued_as_new"
	case enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return "timed_out"
	default:
		return "unknown"
	}
}

func protoTimePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime().UTC()
	if t.IsZero() {
		return nil
	}
	return &t
}

// DiagnosisRoomWorkflowID returns the stable Temporal workflow id for an
// external diagnosis-room session id.
func DiagnosisRoomWorkflowID(sessionID string) (string, error) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return "", fmt.Errorf("diagnosis-room client: session id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if trimmed != sessionID {
		return "", fmt.Errorf("diagnosis-room client: session id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	return diagnosisRoomWorkflowIDPrefix + sessionID, nil
}

func diagnosisRoomWorkflowIDForTenant(ctx context.Context, sessionID string) (string, error) {
	workflowID, err := DiagnosisRoomWorkflowID(sessionID)
	if err != nil {
		return "", err
	}
	return tenantScopedWorkflowID(ctx, workflowID)
}

// SubmitDiagnosisTurn sends a synchronous Workflow Update and waits for the
// workflow handler result.
func (c *DiagnosisRoomClient) SubmitDiagnosisTurn(ctx context.Context, req ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := diagnosisRoomWorkflowIDForTenant(ctx, req.SessionID)
	if err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, err
	}
	updateReq, err := diagnosisRoomSubmitTurnRequest(req)
	if err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, err
	}
	handle, err := c.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   DiagnosisRoomSubmitTurnUpdate,
		Args:         []interface{}{updateReq},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, mapDiagnosisRoomSubmitTurnError("diagnosis-room client: submit turn update", err)
	}
	var result SubmitDiagnosisTurnResult
	if err := handle.Get(ctx, &result); err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, mapDiagnosisRoomSubmitTurnError("diagnosis-room client: get submit turn result", err)
	}
	return diagnosisRoomSubmitTurnResult(result), nil
}

// CollectDiagnosisEvidence sends a Workflow Update that executes a bounded
// operator-selected evidence plan and returns the updated room state plus any
// automatic reassessment turns triggered by that collection.
func (c *DiagnosisRoomClient) CollectDiagnosisEvidence(ctx context.Context, req ports.DiagnosisRoomCollectEvidenceRequest) (ports.DiagnosisRoomCollectEvidenceResult, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := diagnosisRoomWorkflowIDForTenant(ctx, req.SessionID)
	if err != nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, err
	}
	updateReq, err := diagnosisRoomCollectEvidenceRequest(req)
	if err != nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, err
	}
	handle, err := c.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   DiagnosisRoomCollectEvidenceUpdate,
		Args:         []interface{}{updateReq},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, fmt.Errorf("diagnosis-room client: collect evidence update: %w", err)
	}
	var result CollectDiagnosisEvidenceUpdateResult
	if err := handle.Get(ctx, &result); err != nil {
		return ports.DiagnosisRoomCollectEvidenceResult{}, fmt.Errorf("diagnosis-room client: get collect evidence result: %w", err)
	}
	return diagnosisRoomCollectEvidenceResult(result), nil
}

// QueryDiagnosisRoom returns the current room state for reconnect/read flows.
func (c *DiagnosisRoomClient) QueryDiagnosisRoom(ctx context.Context, sessionID string) (ports.DiagnosisRoomState, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := diagnosisRoomWorkflowIDForTenant(ctx, sessionID)
	if err != nil {
		return ports.DiagnosisRoomState{}, err
	}
	response, err := c.client.QueryWorkflow(ctx, workflowID, "", DiagnosisRoomStateQuery)
	if err != nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: query state: %w", err)
	}
	var state DiagnosisRoomWorkflowState
	if err := response.Get(&state); err != nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: decode state query: %w", err)
	}
	return diagnosisRoomWorkflowState(state), nil
}

// ConfirmDiagnosisConclusion records one approval and returns immediately when
// the configured quorum still has missing authorities. The final approval waits
// for the terminal workflow state that contains the retained conclusion.
func (c *DiagnosisRoomClient) ConfirmDiagnosisConclusion(
	ctx context.Context,
	req ports.DiagnosisRoomConfirmConclusionRequest,
) (ports.DiagnosisRoomState, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := diagnosisRoomWorkflowIDForTenant(ctx, req.SessionID)
	if err != nil {
		return ports.DiagnosisRoomState{}, err
	}
	closeReq, err := diagnosisRoomConfirmConclusionRequest(req)
	if err != nil {
		return ports.DiagnosisRoomState{}, err
	}
	handle, err := c.client.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   DiagnosisRoomConfirmConclusionUpdate,
		Args:         []interface{}{closeReq},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: confirm conclusion update: %w", err)
	}
	var accepted DiagnosisRoomWorkflowState
	if err := handle.Get(ctx, &accepted); err != nil {
		return ports.DiagnosisRoomState{}, mapDiagnosisRoomConfirmError(
			"diagnosis-room client: get confirm conclusion result",
			err,
		)
	}
	acceptedState := diagnosisRoomWorkflowState(accepted)
	if accepted.Status != diagnosisRoomStatusClosed && len(accepted.PendingApprovalAuthorities) > 0 {
		return acceptedState, nil
	}
	var result DiagnosisRoomWorkflowResult
	if err := c.client.GetWorkflow(ctx, workflowID, "").Get(ctx, &result); err != nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: wait confirm conclusion: %w", err)
	}
	return diagnosisRoomWorkflowState(result), nil
}

type diagnosisRoomClientClassifiedError struct {
	prefix string
	err    error
	class  error
}

func (e diagnosisRoomClientClassifiedError) Error() string {
	return e.prefix + ": " + e.err.Error()
}

func (e diagnosisRoomClientClassifiedError) Unwrap() []error {
	return []error{e.err, e.class}
}

func mapDiagnosisRoomSubmitTurnError(prefix string, err error) error {
	var appErr *temporalsdk.ApplicationError
	if errors.As(err, &appErr) {
		switch appErr.Type() {
		case errTypeSubmitTurnDuplicateMessage:
			return diagnosisRoomClientClassifiedError{
				prefix: prefix,
				err:    err,
				class:  errors.Join(diagnosisroom.ErrDuplicateMessageID, domain.ErrAlreadyExists),
			}
		case errTypeSubmitTurnInFlight:
			return diagnosisRoomClientClassifiedError{
				prefix: prefix,
				err:    err,
				class:  errors.Join(diagnosisroom.ErrTurnInFlight, domain.ErrAlreadyExists),
			}
		case errTypeInvariantViolation:
			return diagnosisRoomClientClassifiedError{
				prefix: prefix,
				err:    err,
				class:  domain.ErrInvariantViolation,
			}
		}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func mapDiagnosisRoomConfirmError(prefix string, err error) error {
	var appErr *temporalsdk.ApplicationError
	if errors.As(err, &appErr) && appErr.Type() == errTypeConfirmRejected {
		return diagnosisRoomClientClassifiedError{
			prefix: prefix,
			err:    err,
			class:  domain.ErrPreconditionFailed,
		}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func diagnosisRoomSubmitTurnRequest(req ports.DiagnosisRoomSubmitTurnRequest) (SubmitDiagnosisTurnRequest, error) {
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return SubmitDiagnosisTurnRequest{}, fmt.Errorf("diagnosis-room client: message id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if messageID != req.MessageID {
		return SubmitDiagnosisTurnRequest{}, fmt.Errorf("diagnosis-room client: message id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	actorSubject := strings.TrimSpace(req.ActorSubject)
	if actorSubject == "" {
		return SubmitDiagnosisTurnRequest{}, fmt.Errorf("diagnosis-room client: actor subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.Message) == "" {
		return SubmitDiagnosisTurnRequest{}, fmt.Errorf("diagnosis-room client: message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	supplementalEvidence, err := diagnosisRoomSupplementalEvidenceRequest(req.SupplementalEvidence)
	if err != nil {
		return SubmitDiagnosisTurnRequest{}, err
	}
	return SubmitDiagnosisTurnRequest{
		MessageID:            messageID,
		ActorSubject:         actorSubject,
		Message:              req.Message,
		SupplementalEvidence: supplementalEvidence,
	}, nil
}

func diagnosisRoomSupplementalEvidenceRequest(
	in *ports.DiagnosisRoomSupplementalEvidence,
) (*DiagnosisRoomSupplementalEvidence, error) {
	if in == nil {
		return nil, nil
	}
	label := strings.TrimSpace(in.Label)
	detail := strings.TrimSpace(in.Detail)
	priority := strings.TrimSpace(in.Priority)
	evidence := strings.TrimSpace(in.Evidence)
	if label == "" {
		return nil, fmt.Errorf("diagnosis-room client: supplemental evidence label must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if detail == "" {
		return nil, fmt.Errorf("diagnosis-room client: supplemental evidence detail must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if priority == "" {
		return nil, fmt.Errorf("diagnosis-room client: supplemental evidence priority must be non-empty: %w", domain.ErrInvariantViolation)
	}
	switch priority {
	case "low", "medium", "high":
	default:
		return nil, fmt.Errorf("diagnosis-room client: supplemental evidence priority is unsupported: %w", domain.ErrInvariantViolation)
	}
	if evidence == "" {
		return nil, fmt.Errorf("diagnosis-room client: supplemental evidence text must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return &DiagnosisRoomSupplementalEvidence{
		Label:    label,
		Detail:   detail,
		Priority: priority,
		Evidence: evidence,
	}, nil
}

func diagnosisRoomCollectEvidenceRequest(
	req ports.DiagnosisRoomCollectEvidenceRequest,
) (CollectDiagnosisEvidenceRequest, error) {
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: message id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if messageID != req.MessageID {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: message id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	actorSubject := strings.TrimSpace(req.ActorSubject)
	if actorSubject == "" {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: actor subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if actorSubject != req.ActorSubject {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: actor subject must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	requests := diagnosisRoomEvidenceRequestsFromPort(req.Requests)
	if len(requests) == 0 {
		return CollectDiagnosisEvidenceRequest{}, fmt.Errorf("diagnosis-room client: evidence requests must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return CollectDiagnosisEvidenceRequest{
		MessageID:    messageID,
		ActorSubject: actorSubject,
		Message:      req.Message,
		Requests:     requests,
	}, nil
}

func diagnosisRoomConfirmConclusionRequest(
	req ports.DiagnosisRoomConfirmConclusionRequest,
) (DiagnosisRoomCloseRequest, error) {
	actorSubject := strings.TrimSpace(req.ActorSubject)
	if actorSubject == "" {
		return DiagnosisRoomCloseRequest{}, fmt.Errorf("diagnosis-room client: actor subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if actorSubject != req.ActorSubject {
		return DiagnosisRoomCloseRequest{}, fmt.Errorf("diagnosis-room client: actor subject must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "human_confirmed"
	}
	if reason != req.Reason && req.Reason != "" {
		return DiagnosisRoomCloseRequest{}, fmt.Errorf("diagnosis-room client: reason must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	return DiagnosisRoomCloseRequest{
		Reason:       reason,
		ActorSubject: actorSubject,
	}, nil
}

func diagnosisRoomSubmitTurnResult(result SubmitDiagnosisTurnResult) ports.DiagnosisRoomSubmitTurnResult {
	return ports.DiagnosisRoomSubmitTurnResult{
		SessionID:           result.SessionID,
		ChatSessionID:       domain.ChatSessionID(result.ChatSessionID),
		MessageID:           result.MessageID,
		AssistantMessageID:  result.AssistantMessageID,
		UserTurnID:          domain.ChatTurnID(result.UserTurnID),
		AssistantTurnID:     domain.ChatTurnID(result.AssistantTurnID),
		UserSequence:        result.UserSequence,
		AssistantSequence:   result.AssistantSequence,
		TurnCount:           result.TurnCount,
		ContextBytes:        result.ContextBytes,
		Status:              result.Status,
		AssistantMessage:    result.AssistantMessage,
		RequiresHumanReview: result.RequiresHumanReview,
		Confidence:          result.Confidence,
		RetrievalRefs:       cloneStrings(result.RetrievalRefs),
		EvidenceRequests:    diagnosisRoomEvidenceRequestsPort(result.EvidenceRequests),
		CollectionResults:   diagnosisRoomEvidenceCollectionResultsPort(result.CollectionResults),
		EvidenceTimeline:    diagnosisRoomEvidenceTimelinePort(result.EvidenceTimeline),
		ConfidenceTimeline:  diagnosisRoomConfidenceTimelinePort(result.ConfidenceTimeline),
		ConsultationInsight: diagnosisRoomConsultationInsightPort(result.Insight),
		FollowUpTurns:       diagnosisRoomFollowUpTurnsPort(result.FollowUpTurns),
		LatestError:         diagnosisRoomLatestErrorPort(result.LatestError),
	}
}

func diagnosisRoomCollectEvidenceResult(
	result CollectDiagnosisEvidenceUpdateResult,
) ports.DiagnosisRoomCollectEvidenceResult {
	return ports.DiagnosisRoomCollectEvidenceResult{
		State:         diagnosisRoomWorkflowState(result.State),
		FollowUpTurns: diagnosisRoomFollowUpTurnsPort(result.FollowUpTurns),
	}
}

func diagnosisRoomFollowUpTurnsPort(
	in []DiagnosisRoomFollowUpTurnResult,
) []ports.DiagnosisRoomFollowUpTurnResult {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomFollowUpTurnResult, len(in))
	for i, turn := range in {
		out[i] = ports.DiagnosisRoomFollowUpTurnResult{
			MessageID:           turn.MessageID,
			UserMessage:         turn.UserMessage,
			AssistantMessageID:  turn.AssistantMessageID,
			UserTurnID:          domain.ChatTurnID(turn.UserTurnID),
			AssistantTurnID:     domain.ChatTurnID(turn.AssistantTurnID),
			UserSequence:        turn.UserSequence,
			AssistantSequence:   turn.AssistantSequence,
			TurnCount:           turn.TurnCount,
			ContextBytes:        turn.ContextBytes,
			AssistantMessage:    turn.AssistantMessage,
			RequiresHumanReview: turn.RequiresHumanReview,
			Confidence:          turn.Confidence,
			RetrievalRefs:       cloneStrings(turn.RetrievalRefs),
			EvidenceRequests:    diagnosisRoomEvidenceRequestsPort(turn.EvidenceRequests),
			CollectionResults:   diagnosisRoomEvidenceCollectionResultsPort(turn.CollectionResults),
			ConsultationInsight: diagnosisRoomConsultationInsightPort(turn.Insight),
			Trigger:             turn.Trigger,
		}
	}
	return out
}

func diagnosisRoomEvidenceTimelinePort(
	in []DiagnosisRoomEvidenceTimelineEntry,
) []ports.DiagnosisRoomEvidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomEvidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = ports.DiagnosisRoomEvidenceTimelineEntry{
			TurnCount:          item.TurnCount,
			MessageID:          item.MessageID,
			AssistantMessageID: item.AssistantMessageID,
			ActorSubject:       item.ActorSubject,
			Trigger:            item.Trigger,
			EvidenceRequests:   diagnosisRoomEvidenceRequestsPort(item.EvidenceRequests),
			CollectionResults:  diagnosisRoomEvidenceCollectionResultsPort(item.CollectionResults),
		}
	}
	return out
}

func diagnosisRoomEvidenceRequestsPort(in []diagnosisroom.EvidenceRequest) []ports.DiagnosisRoomEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = ports.DiagnosisRoomEvidenceRequest{
			TemplateID:           domain.DiagnosisToolTemplateID(request.TemplateID),
			AlertSourceProfileID: domain.AlertSourceProfileID(request.AlertSourceProfileID),
			Tool:                 request.Tool,
			Reason:               request.Reason,
			Query:                request.Query,
			WindowSeconds:        request.WindowSeconds,
			StepSeconds:          request.StepSeconds,
			Limit:                request.Limit,
		}
	}
	return out
}

func diagnosisRoomEvidenceRequestsFromPort(in []ports.DiagnosisRoomEvidenceRequest) []diagnosisroom.EvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisroom.EvidenceRequest, len(in))
	for i, request := range in {
		out[i] = diagnosisroom.EvidenceRequest{
			TemplateID:           int64(request.TemplateID),
			AlertSourceProfileID: int64(request.AlertSourceProfileID),
			Tool:                 request.Tool,
			Reason:               request.Reason,
			Query:                request.Query,
			WindowSeconds:        request.WindowSeconds,
			StepSeconds:          request.StepSeconds,
			Limit:                request.Limit,
		}
	}
	return out
}

func diagnosisRoomEvidenceCollectionResultsPort(
	in []diagnosisevidence.Item,
) []ports.DiagnosisRoomEvidenceCollectionResult {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomEvidenceCollectionResult, len(in))
	for i, item := range in {
		out[i] = ports.DiagnosisRoomEvidenceCollectionResult{
			Request:              diagnosisRoomEvidenceRequestPort(item.Request),
			TemplateID:           item.TemplateID,
			AlertSourceProfileID: item.AlertSourceProfileID,
			AlertSourceKind:      item.AlertSourceKind,
			Tool:                 item.Tool,
			Status:               string(item.Status),
			ReasonCode:           string(item.ReasonCode),
			Message:              item.Message,
			Limit:                item.Limit,
			ObservedAlerts:       item.ObservedAlerts,
			ActiveAlerts:         diagnosisRoomActiveAlertsPort(item.ActiveAlerts),
			Query:                item.Query,
			WindowSeconds:        item.WindowSeconds,
			StepSeconds:          item.StepSeconds,
			ObservedMetricSeries: item.ObservedMetricSeries,
			MetricResult:         diagnosisRoomMetricResultPort(item.MetricResult),
			CollectedAt:          item.CollectedAt,
		}
	}
	return out
}

func diagnosisRoomEvidenceRequestPort(request diagnosisroom.EvidenceRequest) ports.DiagnosisRoomEvidenceRequest {
	return ports.DiagnosisRoomEvidenceRequest{
		TemplateID:           domain.DiagnosisToolTemplateID(request.TemplateID),
		AlertSourceProfileID: domain.AlertSourceProfileID(request.AlertSourceProfileID),
		Tool:                 request.Tool,
		Reason:               request.Reason,
		Query:                request.Query,
		WindowSeconds:        request.WindowSeconds,
		StepSeconds:          request.StepSeconds,
		Limit:                request.Limit,
	}
}

func diagnosisRoomActiveAlertsPort(in []ports.ActiveAlert) []ports.DiagnosisRoomActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomActiveAlert, len(in))
	for i, alert := range in {
		out[i] = ports.DiagnosisRoomActiveAlert{
			Source:               alert.Source,
			AlertSourceProfileID: alert.AlertSourceProfileID,
			Labels:               cloneStringMap(alert.Labels),
			Annotations:          cloneStringMap(alert.Annotations),
			StartsAt:             alert.StartsAt,
		}
	}
	return out
}

func diagnosisRoomConsultationInsightPort(in diagnosisroom.ConsultationInsight) ports.DiagnosisRoomConsultationInsight {
	return ports.DiagnosisRoomConsultationInsight{
		ConfidenceRationale:           in.ConfidenceRationale,
		MissingEvidenceRequests:       diagnosisRoomConsultationEvidenceRequestsPort(in.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisRoomConsultationEvidenceRequestsPort(in.EvidenceCollectionSuggestions),
		ConclusionStatus:              in.ConclusionStatus,
	}
}

func diagnosisRoomConsultationInsightPortPtr(
	in *diagnosisroom.ConsultationInsight,
) *ports.DiagnosisRoomConsultationInsight {
	if in == nil {
		return nil
	}
	out := diagnosisRoomConsultationInsightPort(*in)
	return &out
}

func diagnosisRoomConsultationEvidenceRequestsPort(
	in []diagnosisroom.ConsultationEvidenceRequest,
) []ports.DiagnosisRoomConsultationEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomConsultationEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = ports.DiagnosisRoomConsultationEvidenceRequest{
			Label:    request.Label,
			Detail:   request.Detail,
			Priority: request.Priority,
		}
	}
	return out
}

func diagnosisRoomMetricResultPort(in ports.MetricQueryResult) ports.DiagnosisRoomMetricQueryResult {
	out := ports.DiagnosisRoomMetricQueryResult{
		ResultType: in.ResultType,
		Warnings:   append([]string(nil), in.Warnings...),
	}
	if in.Scalar != nil {
		scalar := diagnosisRoomMetricPointPort(*in.Scalar)
		out.Scalar = &scalar
	}
	if in.String != nil {
		value := diagnosisRoomMetricPointPort(*in.String)
		out.String = &value
	}
	if in.Series != nil {
		out.Series = make([]ports.DiagnosisRoomMetricSeries, len(in.Series))
		for i, series := range in.Series {
			out.Series[i] = ports.DiagnosisRoomMetricSeries{
				Metric: cloneStringMap(series.Metric),
				Points: diagnosisRoomMetricPointsPort(series.Points),
			}
		}
	}
	return out
}

func diagnosisRoomMetricPointsPort(in []ports.MetricPoint) []ports.DiagnosisRoomMetricPoint {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomMetricPoint, len(in))
	for i, point := range in {
		out[i] = diagnosisRoomMetricPointPort(point)
	}
	return out
}

func diagnosisRoomMetricPointPort(point ports.MetricPoint) ports.DiagnosisRoomMetricPoint {
	return ports.DiagnosisRoomMetricPoint{
		Timestamp: point.Timestamp,
		Value:     point.Value,
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func diagnosisRoomConfidenceTimelinePort(
	in []DiagnosisRoomConfidenceTimelineEntry,
) []ports.DiagnosisRoomConfidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomConfidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = ports.DiagnosisRoomConfidenceTimelineEntry{
			TurnCount:                     item.TurnCount,
			MessageID:                     item.MessageID,
			AssistantMessageID:            item.AssistantMessageID,
			AssistantTurnID:               domain.ChatTurnID(item.AssistantTurnID),
			AssistantSequence:             item.AssistantSequence,
			OccurredAt:                    item.OccurredAt,
			Trigger:                       item.Trigger,
			Confidence:                    item.Confidence,
			RequiresHumanReview:           item.RequiresHumanReview,
			ConclusionStatus:              item.ConclusionStatus,
			ConfidenceRationale:           item.ConfidenceRationale,
			ContextBytes:                  item.ContextBytes,
			RetrievalRefs:                 cloneStrings(item.RetrievalRefs),
			EvidenceRequests:              diagnosisRoomEvidenceRequestsPort(item.EvidenceRequests),
			CollectionResults:             diagnosisRoomEvidenceCollectionResultsPort(item.CollectionResults),
			MissingEvidenceRequests:       diagnosisRoomConsultationEvidenceRequestsPort(item.MissingEvidenceRequests),
			EvidenceCollectionSuggestions: diagnosisRoomConsultationEvidenceRequestsPort(item.EvidenceCollectionSuggestions),
		}
	}
	return out
}

func diagnosisRoomWorkflowState(state DiagnosisRoomWorkflowState) ports.DiagnosisRoomState {
	conversation := make([]ports.DiagnosisRoomConversationTurn, len(state.Conversation))
	for i, turn := range state.Conversation {
		conversation[i] = ports.DiagnosisRoomConversationTurn{
			Role:         turn.Role,
			ActorSubject: turn.ActorSubject,
			Content:      turn.Content,
		}
	}
	seen := append([]string(nil), state.SeenMessageIDs...)
	return ports.DiagnosisRoomState{
		SessionID:                  state.SessionID,
		ChatSessionID:              domain.ChatSessionID(state.ChatSessionID),
		DiagnosisTaskID:            domain.DiagnosisTaskID(state.DiagnosisTaskID),
		OwnerSubject:               state.OwnerSubject,
		Status:                     state.Status,
		TurnCount:                  state.TurnCount,
		StartedAt:                  state.StartedAt,
		LastActivityAt:             state.LastActivityAt,
		ClosedAt:                   state.ClosedAt,
		CloseReason:                state.CloseReason,
		FinalConclusion:            diagnosisRoomFinalConclusionPort(state.FinalConclusion),
		ConversationSummary:        diagnosisRoomConversationSummaryPort(state.ConversationSummary),
		ApprovalMode:               state.ApprovalMode,
		ConclusionDigest:           state.ConclusionDigest,
		Approvals:                  diagnosisRoomConclusionApprovalsPort(state.Approvals),
		PendingApprovalAuthorities: append([]domain.DiagnosisApprovalAuthority(nil), state.PendingApprovalAuthorities...),
		ApprovalInFlight:           state.ApprovalInFlight,
		LatestConsultationInsight:  diagnosisRoomConsultationInsightPortPtr(state.LatestInsight),
		LatestConfidence:           state.LatestConfidence,
		LatestRequiresHumanReview:  copyBoolPtr(state.LatestRequiresHumanReview),
		LatestEvidenceRequests:     diagnosisRoomEvidenceRequestsPort(state.LatestEvidenceRequests),
		LatestCollectionResults:    diagnosisRoomEvidenceCollectionResultsPort(state.LatestCollectionResults),
		EvidenceTimeline:           diagnosisRoomEvidenceTimelinePort(state.EvidenceTimeline),
		ConfidenceTimeline:         diagnosisRoomConfidenceTimelinePort(state.ConfidenceTimeline),
		SupplementalEvidence:       diagnosisRoomSupplementalEvidenceRecordsPort(state.SupplementalEvidence),
		LatestError:                diagnosisRoomLatestErrorPort(state.LatestError),
		InFlight:                   state.InFlight,
		SeenMessageIDs:             seen,
		Conversation:               conversation,
	}
}

func diagnosisRoomConclusionApprovalsPort(in []domain.ChatSessionApproval) []ports.DiagnosisRoomConclusionApproval {
	out := make([]ports.DiagnosisRoomConclusionApproval, len(in))
	for i, approval := range in {
		out[i] = ports.DiagnosisRoomConclusionApproval{
			ID:               approval.ID,
			ConclusionDigest: approval.ConclusionDigest,
			ActorSubject:     approval.ActorSubject,
			Authority:        approval.Authority,
			Reason:           approval.Reason,
			ApprovedAt:       approval.ApprovedAt,
		}
	}
	return out
}

func diagnosisRoomConversationSummaryPort(in *DiagnosisRoomConversationSummary) *ports.DiagnosisRoomConversationSummary {
	if in == nil {
		return nil
	}
	return &ports.DiagnosisRoomConversationSummary{
		ID:                  domain.ChatSessionSummaryID(in.ID),
		Version:             in.Version,
		SchemaVersion:       in.SchemaVersion,
		SourceFirstSequence: in.SourceFirstSequence,
		SourceLastSequence:  in.SourceLastSequence,
		SourceTurnCount:     in.SourceTurnCount,
		SourceDigest:        in.SourceDigest,
		Content:             append(json.RawMessage(nil), in.Content...),
		GeneratedAt:         in.GeneratedAt,
	}
}

func diagnosisRoomLatestErrorPort(in *DiagnosisRoomLatestError) *ports.DiagnosisRoomLatestError {
	if in == nil {
		return nil
	}
	return &ports.DiagnosisRoomLatestError{
		Code:       in.Code,
		Message:    in.Message,
		MessageID:  in.MessageID,
		OccurredAt: in.OccurredAt,
	}
}

func diagnosisRoomSupplementalEvidenceRecordsPort(
	in []DiagnosisRoomSupplementalEvidenceRecord,
) []ports.DiagnosisRoomSupplementalEvidenceRecord {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomSupplementalEvidenceRecord, len(in))
	for i, item := range in {
		out[i] = ports.DiagnosisRoomSupplementalEvidenceRecord{
			Label:              item.Label,
			Detail:             item.Detail,
			Priority:           item.Priority,
			Evidence:           item.Evidence,
			ActorSubject:       item.ActorSubject,
			UserMessageID:      item.UserMessageID,
			AssistantMessageID: item.AssistantMessageID,
			UserTurnID:         domain.ChatTurnID(item.UserTurnID),
			AssistantTurnID:    domain.ChatTurnID(item.AssistantTurnID),
			UserSequence:       item.UserSequence,
			AssistantSequence:  item.AssistantSequence,
			ProvidedAt:         item.ProvidedAt,
		}
	}
	return out
}

func diagnosisRoomFinalConclusionPort(in *DiagnosisRoomFinalConclusion) *ports.DiagnosisRoomFinalConclusion {
	if in == nil {
		return nil
	}
	out := &ports.DiagnosisRoomFinalConclusion{
		Status:                  in.Status,
		Source:                  in.Source,
		Reason:                  in.Reason,
		EvidenceSnapshotID:      domain.EvidenceSnapshotID(in.EvidenceSnapshotID),
		ConclusionVersion:       in.ConclusionVersion,
		ConfirmedBy:             in.ConfirmedBy,
		SupplementalContextRefs: append([]string(nil), in.SupplementalContextRefs...),
		AssistantTurnID:         domain.ChatTurnID(in.AssistantTurnID),
		AssistantMessageID:      in.AssistantMessageID,
		AssistantSequence:       in.AssistantSequence,
		Content:                 in.Content,
		Confidence:              in.Confidence,
		RequiresHumanReview:     in.RequiresHumanReview,
		ConfidenceRationale:     in.ConfidenceRationale,
		Findings:                append([]string(nil), in.Findings...),
		RecommendedActions:      append([]string(nil), in.RecommendedActions...),
		EvidenceRequests:        diagnosisRoomEvidenceRequestsPort(in.EvidenceRequests),
		MissingEvidenceRequests: diagnosisRoomConsultationEvidenceRequestsPort(in.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisRoomConsultationEvidenceRequestsPort(
			in.EvidenceCollectionSuggestions,
		),
	}
	if in.RecordedAt != nil {
		recordedAt := *in.RecordedAt
		out.RecordedAt = &recordedAt
	}
	if in.AssistantOccurredAt != nil {
		occurredAt := *in.AssistantOccurredAt
		out.AssistantOccurredAt = &occurredAt
	}
	if in.RequiresHumanReview != nil {
		requiresHumanReview := *in.RequiresHumanReview
		out.RequiresHumanReview = &requiresHumanReview
	}
	return out
}

var _ ports.DiagnosisRoomWorkflowClient = (*DiagnosisRoomClient)(nil)
var _ ports.DiagnosisRoomWorkflowVisibilityLookup = (*DiagnosisRoomClient)(nil)
