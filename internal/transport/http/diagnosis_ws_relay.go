package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisWSClientSubmitTurn = "submit_turn"
	diagnosisWSClientQueryState = "query_state"
	diagnosisWSClientConfirm    = "confirm_conclusion"

	diagnosisWSServerReady      = "ready"
	diagnosisWSServerTurnResult = "turn_result"
	diagnosisWSServerState      = "state"
	diagnosisWSServerError      = "error"

	defaultDiagnosisWSUpdateTimeout = diagnosisroom.HardMaxTurnTimeout
	defaultDiagnosisWSQueryTimeout  = 10 * time.Second
	diagnosisWSReadLimitSlop        = 4096
)

// DiagnosisWebSocketRelayOption customises the default WebSocket relay.
type DiagnosisWebSocketRelayOption func(*DiagnosisWebSocketRelay)

// WithDiagnosisWebSocketUpdateTimeout sets the maximum time the WebSocket
// relay waits for a submit-turn Update result before returning an error frame.
func WithDiagnosisWebSocketUpdateTimeout(timeout time.Duration) DiagnosisWebSocketRelayOption {
	return func(r *DiagnosisWebSocketRelay) {
		if timeout > 0 {
			r.updateTimeout = timeout
		}
	}
}

// WithDiagnosisWebSocketQueryTimeout sets the maximum time the WebSocket
// relay waits for a state Query result.
func WithDiagnosisWebSocketQueryTimeout(timeout time.Duration) DiagnosisWebSocketRelayOption {
	return func(r *DiagnosisWebSocketRelay) {
		if timeout > 0 {
			r.queryTimeout = timeout
		}
	}
}

// DiagnosisWebSocketRelay forwards authenticated WebSocket frames to the
// diagnosis-room workflow Update/Query boundary.
type DiagnosisWebSocketRelay struct {
	workflows     ports.DiagnosisRoomWorkflowClient
	updateTimeout time.Duration
	queryTimeout  time.Duration
}

func newDiagnosisWebSocketRelay(workflows ports.DiagnosisRoomWorkflowClient, opts ...DiagnosisWebSocketRelayOption) *DiagnosisWebSocketRelay {
	relay := &DiagnosisWebSocketRelay{
		workflows:     workflows,
		updateTimeout: defaultDiagnosisWSUpdateTimeout,
		queryTimeout:  defaultDiagnosisWSQueryTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(relay)
		}
	}
	return relay
}

// ServeDiagnosisWebSocket owns the connection after ticket authentication.
func (r *DiagnosisWebSocketRelay) ServeDiagnosisWebSocket(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket) {
	if r == nil || r.workflows == nil {
		_ = writeDiagnosisWSJSON(conn, diagnosisWSErrorFrame{
			Type:    diagnosisWSServerError,
			Code:    "not_configured",
			Message: "diagnosis WebSocket relay is not configured",
		})
		return
	}
	conn.SetReadLimit(int64(diagnosisroom.HardMaxMessageBytes + diagnosisWSReadLimitSlop))
	if err := writeDiagnosisWSJSON(conn, diagnosisWSReadyFrame{
		Type:      diagnosisWSServerReady,
		SessionID: ticket.SessionID,
		Subject:   ticket.Subject,
	}); err != nil {
		return
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		frame, err := decodeDiagnosisWSClientFrame(raw)
		if err != nil {
			if writeDiagnosisWSJSON(conn, diagnosisWSErrorFrame{
				Type:    diagnosisWSServerError,
				Code:    "bad_frame",
				Message: err.Error(),
			}) != nil {
				return
			}
			continue
		}
		if err := r.handleFrame(ctx, conn, ticket, frame); err != nil {
			return
		}
	}
}

func (r *DiagnosisWebSocketRelay) handleFrame(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket, frame diagnosisWSClientFrame) error {
	switch frame.Type {
	case diagnosisWSClientSubmitTurn:
		return r.handleSubmitTurn(ctx, conn, ticket, frame)
	case diagnosisWSClientQueryState:
		return r.handleQueryState(ctx, conn, ticket.SessionID)
	case diagnosisWSClientConfirm:
		return r.handleConfirmConclusion(ctx, conn, ticket, frame)
	default:
		return writeDiagnosisWSJSON(conn, diagnosisWSErrorFrame{
			Type:    diagnosisWSServerError,
			Code:    "bad_frame",
			Message: "unsupported frame type",
		})
	}
}

func (r *DiagnosisWebSocketRelay) handleSubmitTurn(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket, frame diagnosisWSClientFrame) error {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.updateTimeout)
	defer cancel()
	result, err := r.workflows.SubmitDiagnosisTurn(updateCtx, ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    ticket.SessionID,
		MessageID:    frame.MessageID,
		ActorSubject: ticket.Subject,
		Message:      frame.Message,
	})
	if err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	return writeDiagnosisWSJSON(conn, diagnosisWSTurnResultFrame{
		Type:                diagnosisWSServerTurnResult,
		SessionID:           result.SessionID,
		ChatSessionID:       int64(result.ChatSessionID),
		MessageID:           result.MessageID,
		AssistantMessageID:  result.AssistantMessageID,
		UserTurnID:          int64(result.UserTurnID),
		AssistantTurnID:     int64(result.AssistantTurnID),
		UserSequence:        result.UserSequence,
		AssistantSequence:   result.AssistantSequence,
		TurnCount:           result.TurnCount,
		ContextBytes:        result.ContextBytes,
		Status:              result.Status,
		AssistantMessage:    result.AssistantMessage,
		RequiresHumanReview: result.RequiresHumanReview,
		Confidence:          result.Confidence,
		EvidenceRequests:    diagnosisWSEvidenceRequests(result.EvidenceRequests),
		CollectionResults:   diagnosisWSEvidenceCollectionResults(result.CollectionResults),
		ConsultationInsight: diagnosisWSConsultationInsightFrame(result.ConsultationInsight),
		FollowUpTurns:       diagnosisWSFollowUpTurns(result.FollowUpTurns),
	})
}

func (r *DiagnosisWebSocketRelay) handleQueryState(ctx context.Context, conn *websocket.Conn, sessionID string) error {
	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	state, err := r.workflows.QueryDiagnosisRoom(queryCtx, sessionID)
	if err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	return writeDiagnosisWSJSON(conn, diagnosisWSStateFrameFromState(state))
}

func (r *DiagnosisWebSocketRelay) handleConfirmConclusion(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket, frame diagnosisWSClientFrame) error {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.updateTimeout)
	defer cancel()
	state, err := r.workflows.ConfirmDiagnosisConclusion(updateCtx, ports.DiagnosisRoomConfirmConclusionRequest{
		SessionID:    ticket.SessionID,
		ActorSubject: ticket.Subject,
		Reason:       diagnosisWSReasonOrDefault(frame.Reason, "human_confirmed"),
	})
	if err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	return writeDiagnosisWSJSON(conn, diagnosisWSStateFrameFromState(state))
}

func diagnosisWSReasonOrDefault(reason, fallback string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fallback
	}
	return reason
}

func decodeDiagnosisWSClientFrame(raw []byte) (diagnosisWSClientFrame, error) {
	var frame diagnosisWSClientFrame
	if err := strictjson.Unmarshal(raw, &frame); err != nil {
		return frame, fmt.Errorf("invalid JSON frame: %w", err)
	}
	frame.Type = strings.TrimSpace(frame.Type)
	switch frame.Type {
	case diagnosisWSClientSubmitTurn:
		if strings.TrimSpace(frame.MessageID) == "" {
			return frame, fmt.Errorf("message_id must be non-empty")
		}
		if strings.TrimSpace(frame.MessageID) != frame.MessageID {
			return frame, fmt.Errorf("message_id must not contain leading or trailing whitespace")
		}
		if strings.TrimSpace(frame.Message) == "" {
			return frame, fmt.Errorf("message must be non-empty")
		}
	case diagnosisWSClientQueryState:
		if frame.MessageID != "" || frame.Message != "" {
			return frame, fmt.Errorf("query_state frame must not include message_id or message")
		}
		if frame.Reason != "" {
			return frame, fmt.Errorf("query_state frame must not include reason")
		}
	case diagnosisWSClientConfirm:
		if frame.MessageID != "" || frame.Message != "" {
			return frame, fmt.Errorf("confirm_conclusion frame must not include message_id or message")
		}
		if strings.TrimSpace(frame.Reason) != frame.Reason {
			return frame, fmt.Errorf("reason must not contain leading or trailing whitespace")
		}
	case "":
		return frame, fmt.Errorf("type must be non-empty")
	default:
		return frame, fmt.Errorf("unsupported frame type %q", frame.Type)
	}
	return frame, nil
}

func diagnosisWSStateFrameFromState(state ports.DiagnosisRoomState) diagnosisWSStateFrame {
	return diagnosisWSStateFrame{
		Type:                diagnosisWSServerState,
		SessionID:           state.SessionID,
		ChatSessionID:       int64(state.ChatSessionID),
		DiagnosisTaskID:     int64(state.DiagnosisTaskID),
		OwnerSubject:        state.OwnerSubject,
		Status:              state.Status,
		TurnCount:           state.TurnCount,
		StartedAt:           state.StartedAt,
		LastActivityAt:      state.LastActivityAt,
		ClosedAt:            state.ClosedAt,
		CloseReason:         state.CloseReason,
		FinalConclusion:     diagnosisWSFinalConclusionFrame(state.FinalConclusion),
		Confidence:          state.LatestConfidence,
		RequiresHumanReview: state.LatestRequiresHumanReview,
		ConsultationInsight: diagnosisWSConsultationInsightFramePtr(state.LatestConsultationInsight),
		InFlight:            state.InFlight,
		SeenMessageIDs:      append([]string(nil), state.SeenMessageIDs...),
		Conversation:        diagnosisWSConversation(state.Conversation),
	}
}

func writeDiagnosisWSError(conn *websocket.Conn, err error) error {
	frame := diagnosisWSErrorFrame{
		Type:    diagnosisWSServerError,
		Code:    diagnosisWSErrorCode(err),
		Message: diagnosisWSErrorMessage(err),
	}
	return writeDiagnosisWSJSON(conn, frame)
}

func diagnosisWSErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "turn_still_processing"
	case errors.Is(err, domain.ErrAlreadyExists):
		return "conflict"
	case errors.Is(err, domain.ErrInvariantViolation):
		return "invalid_request"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	default:
		return "workflow_error"
	}
}

func diagnosisWSErrorMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "turn is still processing"
	}
	return err.Error()
}

func diagnosisWSConversation(in []ports.DiagnosisRoomConversationTurn) []diagnosisWSConversationTurn {
	out := make([]diagnosisWSConversationTurn, len(in))
	for i, turn := range in {
		out[i] = diagnosisWSConversationTurn{
			Role:    turn.Role,
			Content: turn.Content,
		}
	}
	return out
}

func diagnosisWSFinalConclusionFrame(in *ports.DiagnosisRoomFinalConclusion) *diagnosisWSFinalConclusion {
	if in == nil {
		return nil
	}
	return &diagnosisWSFinalConclusion{
		Status:                  in.Status,
		Source:                  in.Source,
		Reason:                  in.Reason,
		EvidenceSnapshotID:      int64(in.EvidenceSnapshotID),
		ConclusionVersion:       in.ConclusionVersion,
		RecordedAt:              in.RecordedAt,
		ConfirmedBy:             in.ConfirmedBy,
		SupplementalContextRefs: append([]string(nil), in.SupplementalContextRefs...),
		AssistantTurnID:         int64(in.AssistantTurnID),
		AssistantMessageID:      in.AssistantMessageID,
		AssistantSequence:       in.AssistantSequence,
		AssistantOccurredAt:     in.AssistantOccurredAt,
		Content:                 in.Content,
		Confidence:              in.Confidence,
		RequiresHumanReview:     in.RequiresHumanReview,
	}
}

func diagnosisWSEvidenceRequests(in []ports.DiagnosisRoomEvidenceRequest) []diagnosisWSEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = diagnosisWSEvidenceRequest{
			TemplateID:    int64(request.TemplateID),
			Tool:          string(request.Tool),
			Reason:        request.Reason,
			Query:         request.Query,
			WindowSeconds: request.WindowSeconds,
			StepSeconds:   request.StepSeconds,
			Limit:         request.Limit,
		}
	}
	return out
}

func diagnosisWSEvidenceCollectionResults(
	in []ports.DiagnosisRoomEvidenceCollectionResult,
) []diagnosisWSEvidenceCollectionResult {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSEvidenceCollectionResult, len(in))
	for i, item := range in {
		out[i] = diagnosisWSEvidenceCollectionResult{
			Request:              diagnosisWSEvidenceRequestFrame(item.Request),
			TemplateID:           int64(item.TemplateID),
			AlertSourceProfileID: int64(item.AlertSourceProfileID),
			AlertSourceKind:      string(item.AlertSourceKind),
			Tool:                 string(item.Tool),
			Status:               item.Status,
			ReasonCode:           item.ReasonCode,
			Message:              item.Message,
			Limit:                item.Limit,
			ObservedAlerts:       item.ObservedAlerts,
			ActiveAlerts:         diagnosisWSActiveAlerts(item.ActiveAlerts),
			Query:                item.Query,
			WindowSeconds:        item.WindowSeconds,
			StepSeconds:          item.StepSeconds,
			ObservedMetricSeries: item.ObservedMetricSeries,
			MetricResult:         diagnosisWSMetricResult(item.MetricResult),
			CollectedAt:          item.CollectedAt,
		}
	}
	return out
}

func diagnosisWSFollowUpTurns(in []ports.DiagnosisRoomFollowUpTurnResult) []diagnosisWSFollowUpTurn {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSFollowUpTurn, len(in))
	for i, turn := range in {
		out[i] = diagnosisWSFollowUpTurn{
			MessageID:           turn.MessageID,
			UserMessage:         turn.UserMessage,
			AssistantMessageID:  turn.AssistantMessageID,
			UserTurnID:          int64(turn.UserTurnID),
			AssistantTurnID:     int64(turn.AssistantTurnID),
			UserSequence:        turn.UserSequence,
			AssistantSequence:   turn.AssistantSequence,
			TurnCount:           turn.TurnCount,
			ContextBytes:        turn.ContextBytes,
			AssistantMessage:    turn.AssistantMessage,
			RequiresHumanReview: turn.RequiresHumanReview,
			Confidence:          turn.Confidence,
			EvidenceRequests:    diagnosisWSEvidenceRequests(turn.EvidenceRequests),
			CollectionResults:   diagnosisWSEvidenceCollectionResults(turn.CollectionResults),
			ConsultationInsight: diagnosisWSConsultationInsightFrame(turn.ConsultationInsight),
			Trigger:             turn.Trigger,
		}
	}
	return out
}

func diagnosisWSEvidenceRequestFrame(request ports.DiagnosisRoomEvidenceRequest) diagnosisWSEvidenceRequest {
	return diagnosisWSEvidenceRequest{
		TemplateID:    int64(request.TemplateID),
		Tool:          string(request.Tool),
		Reason:        request.Reason,
		Query:         request.Query,
		WindowSeconds: request.WindowSeconds,
		StepSeconds:   request.StepSeconds,
		Limit:         request.Limit,
	}
}

func diagnosisWSActiveAlerts(in []ports.DiagnosisRoomActiveAlert) []diagnosisWSActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSActiveAlert, len(in))
	for i, alert := range in {
		out[i] = diagnosisWSActiveAlert{
			Source:      alert.Source,
			Labels:      cloneDiagnosisWSStringMap(alert.Labels),
			Annotations: cloneDiagnosisWSStringMap(alert.Annotations),
			StartsAt:    alert.StartsAt,
		}
	}
	return out
}

func diagnosisWSMetricResult(in ports.DiagnosisRoomMetricQueryResult) diagnosisWSMetricQueryResult {
	out := diagnosisWSMetricQueryResult{
		ResultType: in.ResultType,
		Warnings:   append([]string(nil), in.Warnings...),
	}
	if in.Scalar != nil {
		scalar := diagnosisWSMetricPointFrame(*in.Scalar)
		out.Scalar = &scalar
	}
	if in.String != nil {
		value := diagnosisWSMetricPointFrame(*in.String)
		out.String = &value
	}
	if in.Series != nil {
		out.Series = make([]diagnosisWSMetricSeries, len(in.Series))
		for i, series := range in.Series {
			out.Series[i] = diagnosisWSMetricSeries{
				Metric: cloneDiagnosisWSStringMap(series.Metric),
				Points: diagnosisWSMetricPoints(series.Points),
			}
		}
	}
	return out
}

func diagnosisWSMetricPoints(in []ports.DiagnosisRoomMetricPoint) []diagnosisWSMetricPoint {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSMetricPoint, len(in))
	for i, point := range in {
		out[i] = diagnosisWSMetricPointFrame(point)
	}
	return out
}

func diagnosisWSMetricPointFrame(point ports.DiagnosisRoomMetricPoint) diagnosisWSMetricPoint {
	return diagnosisWSMetricPoint{
		Timestamp: point.Timestamp,
		Value:     point.Value,
	}
}

func cloneDiagnosisWSStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func diagnosisWSConsultationInsightFrame(
	in ports.DiagnosisRoomConsultationInsight,
) diagnosisWSConsultationInsight {
	return diagnosisWSConsultationInsight{
		ConfidenceRationale:           in.ConfidenceRationale,
		MissingEvidenceRequests:       diagnosisWSConsultationEvidenceRequests(in.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisWSConsultationEvidenceRequests(in.EvidenceCollectionSuggestions),
		ConclusionStatus:              in.ConclusionStatus,
	}
}

func diagnosisWSConsultationInsightFramePtr(
	in *ports.DiagnosisRoomConsultationInsight,
) *diagnosisWSConsultationInsight {
	if in == nil {
		return nil
	}
	out := diagnosisWSConsultationInsightFrame(*in)
	return &out
}

func diagnosisWSConsultationEvidenceRequests(
	in []ports.DiagnosisRoomConsultationEvidenceRequest,
) []diagnosisWSConsultationEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSConsultationEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = diagnosisWSConsultationEvidenceRequest{
			Label:    request.Label,
			Detail:   request.Detail,
			Priority: request.Priority,
		}
	}
	return out
}

func writeDiagnosisWSJSON(conn *websocket.Conn, value interface{}) error {
	if err := conn.WriteJSON(value); err != nil {
		return err
	}
	return nil
}

type diagnosisWSClientFrame struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id,omitempty"`
	Message   string `json:"message,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type diagnosisWSReadyFrame struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Subject   string `json:"subject"`
}

type diagnosisWSTurnResultFrame struct {
	Type                string                                `json:"type"`
	SessionID           string                                `json:"session_id"`
	ChatSessionID       int64                                 `json:"chat_session_id"`
	MessageID           string                                `json:"message_id"`
	AssistantMessageID  string                                `json:"assistant_message_id"`
	UserTurnID          int64                                 `json:"user_turn_id"`
	AssistantTurnID     int64                                 `json:"assistant_turn_id"`
	UserSequence        int                                   `json:"user_sequence"`
	AssistantSequence   int                                   `json:"assistant_sequence"`
	TurnCount           int                                   `json:"turn_count"`
	ContextBytes        int                                   `json:"context_bytes"`
	Status              string                                `json:"status"`
	AssistantMessage    string                                `json:"assistant_message"`
	RequiresHumanReview bool                                  `json:"requires_human_review"`
	Confidence          string                                `json:"confidence"`
	EvidenceRequests    []diagnosisWSEvidenceRequest          `json:"evidence_requests,omitempty"`
	CollectionResults   []diagnosisWSEvidenceCollectionResult `json:"evidence_collection_results,omitempty"`
	ConsultationInsight diagnosisWSConsultationInsight        `json:"consultation_insight"`
	FollowUpTurns       []diagnosisWSFollowUpTurn             `json:"follow_up_turns,omitempty"`
}

type diagnosisWSFollowUpTurn struct {
	MessageID           string                                `json:"message_id"`
	UserMessage         string                                `json:"user_message"`
	AssistantMessageID  string                                `json:"assistant_message_id"`
	UserTurnID          int64                                 `json:"user_turn_id"`
	AssistantTurnID     int64                                 `json:"assistant_turn_id"`
	UserSequence        int                                   `json:"user_sequence"`
	AssistantSequence   int                                   `json:"assistant_sequence"`
	TurnCount           int                                   `json:"turn_count"`
	ContextBytes        int                                   `json:"context_bytes"`
	AssistantMessage    string                                `json:"assistant_message"`
	RequiresHumanReview bool                                  `json:"requires_human_review"`
	Confidence          string                                `json:"confidence"`
	EvidenceRequests    []diagnosisWSEvidenceRequest          `json:"evidence_requests,omitempty"`
	CollectionResults   []diagnosisWSEvidenceCollectionResult `json:"evidence_collection_results,omitempty"`
	ConsultationInsight diagnosisWSConsultationInsight        `json:"consultation_insight"`
	Trigger             string                                `json:"trigger"`
}

type diagnosisWSStateFrame struct {
	Type                string                          `json:"type"`
	SessionID           string                          `json:"session_id"`
	ChatSessionID       int64                           `json:"chat_session_id"`
	DiagnosisTaskID     int64                           `json:"diagnosis_task_id"`
	OwnerSubject        string                          `json:"owner_subject"`
	Status              string                          `json:"status"`
	TurnCount           int                             `json:"turn_count"`
	StartedAt           time.Time                       `json:"started_at"`
	LastActivityAt      time.Time                       `json:"last_activity_at"`
	ClosedAt            *time.Time                      `json:"closed_at,omitempty"`
	CloseReason         string                          `json:"close_reason,omitempty"`
	FinalConclusion     *diagnosisWSFinalConclusion     `json:"final_conclusion,omitempty"`
	Confidence          string                          `json:"confidence,omitempty"`
	RequiresHumanReview *bool                           `json:"requires_human_review,omitempty"`
	ConsultationInsight *diagnosisWSConsultationInsight `json:"consultation_insight,omitempty"`
	InFlight            bool                            `json:"in_flight"`
	SeenMessageIDs      []string                        `json:"seen_message_ids"`
	Conversation        []diagnosisWSConversationTurn   `json:"conversation"`
}

type diagnosisWSConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type diagnosisWSFinalConclusion struct {
	Status                  string     `json:"status"`
	Source                  string     `json:"source"`
	Reason                  string     `json:"reason,omitempty"`
	EvidenceSnapshotID      int64      `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion       string     `json:"conclusion_version,omitempty"`
	RecordedAt              *time.Time `json:"recorded_at,omitempty"`
	ConfirmedBy             string     `json:"confirmed_by,omitempty"`
	SupplementalContextRefs []string   `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID         int64      `json:"assistant_turn_id,omitempty"`
	AssistantMessageID      string     `json:"assistant_message_id,omitempty"`
	AssistantSequence       int        `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt     *time.Time `json:"assistant_occurred_at,omitempty"`
	Content                 string     `json:"content,omitempty"`
	Confidence              string     `json:"confidence,omitempty"`
	RequiresHumanReview     *bool      `json:"requires_human_review,omitempty"`
}

type diagnosisWSEvidenceRequest struct {
	TemplateID    int64  `json:"template_id,omitempty"`
	Tool          string `json:"tool"`
	Reason        string `json:"reason"`
	Query         string `json:"query,omitempty"`
	WindowSeconds int    `json:"window_seconds,omitempty"`
	StepSeconds   int    `json:"step_seconds,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type diagnosisWSEvidenceCollectionResult struct {
	Request              diagnosisWSEvidenceRequest   `json:"request"`
	TemplateID           int64                        `json:"template_id,omitempty"`
	AlertSourceProfileID int64                        `json:"alert_source_profile_id,omitempty"`
	AlertSourceKind      string                       `json:"alert_source_kind,omitempty"`
	Tool                 string                       `json:"tool"`
	Status               string                       `json:"status"`
	ReasonCode           string                       `json:"reason_code"`
	Message              string                       `json:"message"`
	Limit                int                          `json:"limit,omitempty"`
	ObservedAlerts       int                          `json:"observed_alerts"`
	ActiveAlerts         []diagnosisWSActiveAlert     `json:"active_alerts,omitempty"`
	Query                string                       `json:"query,omitempty"`
	WindowSeconds        int                          `json:"window_seconds,omitempty"`
	StepSeconds          int                          `json:"step_seconds,omitempty"`
	ObservedMetricSeries int                          `json:"observed_metric_series"`
	MetricResult         diagnosisWSMetricQueryResult `json:"metric_result,omitempty"`
	CollectedAt          time.Time                    `json:"collected_at"`
}

type diagnosisWSActiveAlert struct {
	Source      string            `json:"source"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"starts_at"`
}

type diagnosisWSMetricQueryResult struct {
	ResultType string                    `json:"result_type,omitempty"`
	Series     []diagnosisWSMetricSeries `json:"series,omitempty"`
	Scalar     *diagnosisWSMetricPoint   `json:"scalar,omitempty"`
	String     *diagnosisWSMetricPoint   `json:"string,omitempty"`
	Warnings   []string                  `json:"warnings,omitempty"`
}

type diagnosisWSMetricSeries struct {
	Metric map[string]string        `json:"metric,omitempty"`
	Points []diagnosisWSMetricPoint `json:"points,omitempty"`
}

type diagnosisWSMetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     string    `json:"value"`
}

type diagnosisWSConsultationInsight struct {
	ConfidenceRationale           string                                   `json:"confidence_rationale,omitempty"`
	MissingEvidenceRequests       []diagnosisWSConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisWSConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
	ConclusionStatus              string                                   `json:"conclusion_status,omitempty"`
}

type diagnosisWSConsultationEvidenceRequest struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
}

type diagnosisWSErrorFrame struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var _ DiagnosisWebSocketHandler = (*DiagnosisWebSocketRelay)(nil)
