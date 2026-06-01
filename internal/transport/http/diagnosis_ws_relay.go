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
	})
}

func (r *DiagnosisWebSocketRelay) handleQueryState(ctx context.Context, conn *websocket.Conn, sessionID string) error {
	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()
	state, err := r.workflows.QueryDiagnosisRoom(queryCtx, sessionID)
	if err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	return writeDiagnosisWSJSON(conn, diagnosisWSStateFrame{
		Type:            diagnosisWSServerState,
		SessionID:       state.SessionID,
		ChatSessionID:   int64(state.ChatSessionID),
		DiagnosisTaskID: int64(state.DiagnosisTaskID),
		OwnerSubject:    state.OwnerSubject,
		Status:          state.Status,
		TurnCount:       state.TurnCount,
		StartedAt:       state.StartedAt,
		LastActivityAt:  state.LastActivityAt,
		ClosedAt:        state.ClosedAt,
		CloseReason:     state.CloseReason,
		InFlight:        state.InFlight,
		SeenMessageIDs:  append([]string(nil), state.SeenMessageIDs...),
		Conversation:    diagnosisWSConversation(state.Conversation),
	})
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
	case "":
		return frame, fmt.Errorf("type must be non-empty")
	default:
		return frame, fmt.Errorf("unsupported frame type %q", frame.Type)
	}
	return frame, nil
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
}

type diagnosisWSReadyFrame struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Subject   string `json:"subject"`
}

type diagnosisWSTurnResultFrame struct {
	Type                string `json:"type"`
	SessionID           string `json:"session_id"`
	ChatSessionID       int64  `json:"chat_session_id"`
	MessageID           string `json:"message_id"`
	AssistantMessageID  string `json:"assistant_message_id"`
	UserTurnID          int64  `json:"user_turn_id"`
	AssistantTurnID     int64  `json:"assistant_turn_id"`
	UserSequence        int    `json:"user_sequence"`
	AssistantSequence   int    `json:"assistant_sequence"`
	TurnCount           int    `json:"turn_count"`
	ContextBytes        int    `json:"context_bytes"`
	Status              string `json:"status"`
	AssistantMessage    string `json:"assistant_message"`
	RequiresHumanReview bool   `json:"requires_human_review"`
	Confidence          string `json:"confidence"`
}

type diagnosisWSStateFrame struct {
	Type            string                        `json:"type"`
	SessionID       string                        `json:"session_id"`
	ChatSessionID   int64                         `json:"chat_session_id"`
	DiagnosisTaskID int64                         `json:"diagnosis_task_id"`
	OwnerSubject    string                        `json:"owner_subject"`
	Status          string                        `json:"status"`
	TurnCount       int                           `json:"turn_count"`
	StartedAt       time.Time                     `json:"started_at"`
	LastActivityAt  time.Time                     `json:"last_activity_at"`
	ClosedAt        *time.Time                    `json:"closed_at,omitempty"`
	CloseReason     string                        `json:"close_reason,omitempty"`
	InFlight        bool                          `json:"in_flight"`
	SeenMessageIDs  []string                      `json:"seen_message_ids"`
	Conversation    []diagnosisWSConversationTurn `json:"conversation"`
}

type diagnosisWSConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type diagnosisWSErrorFrame struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var _ DiagnosisWebSocketHandler = (*DiagnosisWebSocketRelay)(nil)
