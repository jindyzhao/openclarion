package temporal

import (
	"context"
	"fmt"
	"strings"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const diagnosisRoomWorkflowIDPrefix = "diagnosis-room-"

type diagnosisRoomWorkflowClient interface {
	UpdateWorkflow(ctx context.Context, options client.UpdateWorkflowOptions) (client.WorkflowUpdateHandle, error)
	QueryWorkflow(ctx context.Context, workflowID string, runID string, queryType string, args ...interface{}) (converter.EncodedValue, error)
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

// SubmitDiagnosisTurn sends a synchronous Workflow Update and waits for the
// workflow handler result.
func (c *DiagnosisRoomClient) SubmitDiagnosisTurn(ctx context.Context, req ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := DiagnosisRoomWorkflowID(req.SessionID)
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
		return ports.DiagnosisRoomSubmitTurnResult{}, fmt.Errorf("diagnosis-room client: submit turn update: %w", err)
	}
	var result SubmitDiagnosisTurnResult
	if err := handle.Get(ctx, &result); err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, fmt.Errorf("diagnosis-room client: get submit turn result: %w", err)
	}
	return diagnosisRoomSubmitTurnResult(result), nil
}

// QueryDiagnosisRoom returns the current room state for reconnect/read flows.
func (c *DiagnosisRoomClient) QueryDiagnosisRoom(ctx context.Context, sessionID string) (ports.DiagnosisRoomState, error) {
	if c == nil || c.client == nil {
		return ports.DiagnosisRoomState{}, fmt.Errorf("diagnosis-room client: Temporal client must be non-nil: %w", domain.ErrInvariantViolation)
	}
	workflowID, err := DiagnosisRoomWorkflowID(sessionID)
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
	return SubmitDiagnosisTurnRequest{
		MessageID:    messageID,
		ActorSubject: actorSubject,
		Message:      req.Message,
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
	}
}

func diagnosisRoomWorkflowState(state DiagnosisRoomWorkflowState) ports.DiagnosisRoomState {
	conversation := make([]ports.DiagnosisRoomConversationTurn, len(state.Conversation))
	for i, turn := range state.Conversation {
		conversation[i] = ports.DiagnosisRoomConversationTurn{
			Role:    turn.Role,
			Content: turn.Content,
		}
	}
	seen := append([]string(nil), state.SeenMessageIDs...)
	return ports.DiagnosisRoomState{
		SessionID:       state.SessionID,
		ChatSessionID:   domain.ChatSessionID(state.ChatSessionID),
		DiagnosisTaskID: domain.DiagnosisTaskID(state.DiagnosisTaskID),
		OwnerSubject:    state.OwnerSubject,
		Status:          state.Status,
		TurnCount:       state.TurnCount,
		StartedAt:       state.StartedAt,
		LastActivityAt:  state.LastActivityAt,
		ClosedAt:        state.ClosedAt,
		CloseReason:     state.CloseReason,
		FinalConclusion: diagnosisRoomFinalConclusionPort(state.FinalConclusion),
		InFlight:        state.InFlight,
		SeenMessageIDs:  seen,
		Conversation:    conversation,
	}
}

func diagnosisRoomFinalConclusionPort(in *DiagnosisRoomFinalConclusion) *ports.DiagnosisRoomFinalConclusion {
	if in == nil {
		return nil
	}
	out := &ports.DiagnosisRoomFinalConclusion{
		Status:              in.Status,
		Source:              in.Source,
		Reason:              in.Reason,
		AssistantTurnID:     domain.ChatTurnID(in.AssistantTurnID),
		AssistantMessageID:  in.AssistantMessageID,
		AssistantSequence:   in.AssistantSequence,
		Content:             in.Content,
		Confidence:          in.Confidence,
		RequiresHumanReview: in.RequiresHumanReview,
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
