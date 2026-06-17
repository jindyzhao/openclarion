package temporal

import (
	"context"
	"fmt"
	"strings"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
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
		EvidenceRequests:    diagnosisRoomEvidenceRequestsPort(result.EvidenceRequests),
		CollectionResults:   diagnosisRoomEvidenceCollectionResultsPort(result.CollectionResults),
		ConsultationInsight: diagnosisRoomConsultationInsightPort(result.Insight),
		FollowUpTurns:       diagnosisRoomFollowUpTurnsPort(result.FollowUpTurns),
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
			EvidenceRequests:    diagnosisRoomEvidenceRequestsPort(turn.EvidenceRequests),
			CollectionResults:   diagnosisRoomEvidenceCollectionResultsPort(turn.CollectionResults),
			ConsultationInsight: diagnosisRoomConsultationInsightPort(turn.Insight),
			Trigger:             turn.Trigger,
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
			TemplateID:    domain.DiagnosisToolTemplateID(request.TemplateID),
			Tool:          request.Tool,
			Reason:        request.Reason,
			Query:         request.Query,
			WindowSeconds: request.WindowSeconds,
			StepSeconds:   request.StepSeconds,
			Limit:         request.Limit,
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
		TemplateID:    domain.DiagnosisToolTemplateID(request.TemplateID),
		Tool:          request.Tool,
		Reason:        request.Reason,
		Query:         request.Query,
		WindowSeconds: request.WindowSeconds,
		StepSeconds:   request.StepSeconds,
		Limit:         request.Limit,
	}
}

func diagnosisRoomActiveAlertsPort(in []ports.ActiveAlert) []ports.DiagnosisRoomActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomActiveAlert, len(in))
	for i, alert := range in {
		out[i] = ports.DiagnosisRoomActiveAlert{
			Source:      alert.Source,
			Labels:      cloneStringMap(alert.Labels),
			Annotations: cloneStringMap(alert.Annotations),
			StartsAt:    alert.StartsAt,
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
