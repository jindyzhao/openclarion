package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisWSClientSubmitTurn                 = "submit_turn"
	diagnosisWSClientSubmitSupplementalEvidence = "submit_supplemental_evidence"
	diagnosisWSClientCollectEvidence            = "collect_evidence"
	diagnosisWSClientQueryState                 = "query_state"
	diagnosisWSClientConfirm                    = "confirm_conclusion"

	diagnosisWSServerReady      = "ready"
	diagnosisWSServerTurnStream = "turn_stream"
	diagnosisWSServerTurnResult = "turn_result"
	diagnosisWSServerState      = "state"
	diagnosisWSServerError      = "error"

	defaultDiagnosisWSUpdateTimeout = diagnosisroom.HardMaxTurnTimeout * (diagnosisroom.HardMaxAutoEvidenceFollowUps + 1)
	defaultDiagnosisWSQueryTimeout  = 10 * time.Second
	defaultDiagnosisWSWriteTimeout  = 10 * time.Second
	diagnosisWSReadLimitSlop        = 4096
)

// DiagnosisWebSocketRelayOption customises the default WebSocket relay.
type DiagnosisWebSocketRelayOption func(*DiagnosisWebSocketRelay)

type diagnosisWebSocketFrameAuthorizer interface {
	AuthorizeDiagnosisWebSocketFrame(ctx context.Context, ticket diagnosisauth.Ticket, frameType string) error
}

type diagnosisWebSocketFrameAuthorizerFunc func(ctx context.Context, ticket diagnosisauth.Ticket, frameType string) error

func (f diagnosisWebSocketFrameAuthorizerFunc) AuthorizeDiagnosisWebSocketFrame(ctx context.Context, ticket diagnosisauth.Ticket, frameType string) error {
	return f(ctx, ticket, frameType)
}

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

func withDiagnosisWebSocketFrameAuthorizer(authorizer diagnosisWebSocketFrameAuthorizer) DiagnosisWebSocketRelayOption {
	return func(r *DiagnosisWebSocketRelay) {
		if authorizer != nil {
			r.frameAuthorizer = authorizer
		}
	}
}

// WithDiagnosisTurnStreamSource enables best-effort transient assistant
// previews while a submit-turn Workflow Update is still running.
func WithDiagnosisTurnStreamSource(source ports.DiagnosisTurnStreamSource) DiagnosisWebSocketRelayOption {
	return func(r *DiagnosisWebSocketRelay) {
		if source != nil {
			r.streamSource = source
		}
	}
}

// DiagnosisWebSocketRelay forwards authenticated WebSocket frames to the
// diagnosis-room workflow Update/Query boundary.
type DiagnosisWebSocketRelay struct {
	workflows        ports.DiagnosisRoomWorkflowClient
	frameAuthorizer  diagnosisWebSocketFrameAuthorizer
	streamSource     ports.DiagnosisTurnStreamSource
	writeTurnPreview func(*websocket.Conn, diagnosisWSTurnStreamFrame) error
	updateTimeout    time.Duration
	queryTimeout     time.Duration
}

func newDiagnosisWebSocketRelay(workflows ports.DiagnosisRoomWorkflowClient, opts ...DiagnosisWebSocketRelayOption) *DiagnosisWebSocketRelay {
	relay := &DiagnosisWebSocketRelay{
		workflows:        workflows,
		writeTurnPreview: writeDiagnosisWSTurnStream,
		updateTimeout:    defaultDiagnosisWSUpdateTimeout,
		queryTimeout:     defaultDiagnosisWSQueryTimeout,
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
	if err := r.authorizeFrame(ctx, ticket, frame.Type); err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	switch frame.Type {
	case diagnosisWSClientSubmitTurn, diagnosisWSClientSubmitSupplementalEvidence:
		return r.handleSubmitTurn(ctx, conn, ticket, frame)
	case diagnosisWSClientCollectEvidence:
		return r.handleCollectEvidence(ctx, conn, ticket, frame)
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

func (r *DiagnosisWebSocketRelay) authorizeFrame(ctx context.Context, ticket diagnosisauth.Ticket, frameType string) error {
	if r.frameAuthorizer == nil {
		return nil
	}
	if _, ok := diagnosisWSFramePermission(frameType); !ok {
		return nil
	}
	return r.frameAuthorizer.AuthorizeDiagnosisWebSocketFrame(ctx, ticket, frameType)
}

func diagnosisWSFramePermission(frameType string) (domain.RBACPermission, bool) {
	switch frameType {
	case diagnosisWSClientQueryState:
		return domain.RBACPermissionDiagnosisRoomRead, true
	case diagnosisWSClientSubmitTurn,
		diagnosisWSClientSubmitSupplementalEvidence,
		diagnosisWSClientCollectEvidence:
		return domain.RBACPermissionDiagnosisRoomParticipate, true
	case diagnosisWSClientConfirm:
		return domain.RBACPermissionDiagnosisRoomApprove, true
	default:
		return "", false
	}
}

func (r *DiagnosisWebSocketRelay) handleSubmitTurn(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket, frame diagnosisWSClientFrame) error {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.updateTimeout)
	var updateGroup errgroup.Group
	// The bounded Update intentionally outlives a disconnected browser. Do not
	// wait for this group on a preview write failure; its buffered outcome lets
	// the detached call finish without retaining the WebSocket relay.

	var stream <-chan ports.DiagnosisTurnStreamEvent
	cancelStream := func() {}
	if r.streamSource != nil {
		stream, cancelStream = r.streamSource.SubscribeDiagnosisTurnStream(ticket.SessionID, frame.MessageID)
	}
	defer cancelStream()

	type updateOutcome struct {
		result ports.DiagnosisRoomSubmitTurnResult
		err    error
	}
	outcomes := make(chan updateOutcome, 1)
	workflows := r.workflows
	updateGroup.Go(func() error {
		defer cancel()
		result, err := workflows.SubmitDiagnosisTurn(updateCtx, ports.DiagnosisRoomSubmitTurnRequest{
			SessionID:            ticket.SessionID,
			MessageID:            frame.MessageID,
			ActorSubject:         ticket.Subject,
			Message:              frame.Message,
			SupplementalEvidence: diagnosisWSSupplementalEvidencePort(frame.SupplementalEvidence),
		})
		outcomes <- updateOutcome{result: result, err: err}
		return nil
	})

	for {
		select {
		case event, ok := <-stream:
			if !ok {
				stream = nil
				continue
			}
			preview, ok := diagnosisWSTurnStreamFrameFromEvent(ticket.SessionID, frame.MessageID, event)
			if ok {
				if err := r.writeTurnPreview(conn, preview); err != nil {
					return err
				}
			}
		case outcome := <-outcomes:
			if outcome.err != nil {
				return writeDiagnosisWSError(conn, outcome.err)
			}
			return writeDiagnosisWSTurnResult(conn, outcome.result, ticket.Subject)
		}
	}
}

func writeDiagnosisWSTurnStream(conn *websocket.Conn, frame diagnosisWSTurnStreamFrame) error {
	return writeDiagnosisWSJSON(conn, frame)
}

func writeDiagnosisWSTurnResult(conn *websocket.Conn, result ports.DiagnosisRoomSubmitTurnResult, actorSubject string) error {
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
		EvidenceTimeline:    diagnosisWSEvidenceTimelineFromTurnResult(result, actorSubject),
		ConfidenceTimeline:  diagnosisWSConfidenceTimeline(result.ConfidenceTimeline),
		ConsultationInsight: diagnosisWSConsultationInsightFrame(result.ConsultationInsight),
		FollowUpTurns:       diagnosisWSFollowUpTurns(result.FollowUpTurns),
		LatestError:         diagnosisWSLatestErrorFrame(result.LatestError),
	})
}

func diagnosisWSTurnStreamFrameFromEvent(
	sessionID string,
	messageID string,
	event ports.DiagnosisTurnStreamEvent,
) (diagnosisWSTurnStreamFrame, bool) {
	if event.SessionID != sessionID || event.MessageID != messageID || event.ActivityAttempt <= 0 {
		return diagnosisWSTurnStreamFrame{}, false
	}
	if strings.TrimSpace(event.AssistantMessageID) == "" ||
		strings.TrimSpace(event.AssistantMessageID) != event.AssistantMessageID ||
		!utf8.ValidString(event.AssistantMessageID) ||
		len([]byte(event.AssistantMessageID)) > diagnosisroom.HardMaxMessageBytes {
		return diagnosisWSTurnStreamFrame{}, false
	}
	if !utf8.ValidString(event.AssistantMessage) || len([]byte(event.AssistantMessage)) > ports.MaxContainerStreamTextBytes {
		return diagnosisWSTurnStreamFrame{}, false
	}
	switch event.Phase {
	case ports.DiagnosisTurnStreamStarted:
		if event.GenerationAttempt != 0 || event.Sequence != 0 || event.AssistantMessage != "" {
			return diagnosisWSTurnStreamFrame{}, false
		}
	case ports.DiagnosisTurnStreamReset:
		if event.GenerationAttempt <= 0 || event.Sequence != 0 || event.AssistantMessage != "" {
			return diagnosisWSTurnStreamFrame{}, false
		}
	case ports.DiagnosisTurnStreamDelta:
		if event.GenerationAttempt <= 0 || event.Sequence <= 0 || event.AssistantMessage == "" {
			return diagnosisWSTurnStreamFrame{}, false
		}
	default:
		return diagnosisWSTurnStreamFrame{}, false
	}
	return diagnosisWSTurnStreamFrame{
		Type:               diagnosisWSServerTurnStream,
		Phase:              event.Phase,
		SessionID:          event.SessionID,
		MessageID:          event.MessageID,
		AssistantMessageID: event.AssistantMessageID,
		ActivityAttempt:    event.ActivityAttempt,
		GenerationAttempt:  event.GenerationAttempt,
		Sequence:           event.Sequence,
		AssistantMessage:   event.AssistantMessage,
	}, true
}

func (r *DiagnosisWebSocketRelay) handleCollectEvidence(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket, frame diagnosisWSClientFrame) error {
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.updateTimeout)
	defer cancel()
	result, err := r.workflows.CollectDiagnosisEvidence(updateCtx, ports.DiagnosisRoomCollectEvidenceRequest{
		SessionID:    ticket.SessionID,
		MessageID:    frame.MessageID,
		ActorSubject: ticket.Subject,
		Message:      frame.Message,
		Requests:     diagnosisWSEvidenceRequestsPort(frame.EvidenceRequests),
	})
	if err != nil {
		return writeDiagnosisWSError(conn, err)
	}
	return writeDiagnosisWSJSON(conn, diagnosisWSStateFrameFromCollectResult(result))
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
		return writeDiagnosisWSConfirmError(conn, err)
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

func validateDiagnosisWSSupplementalEvidence(in *diagnosisWSSupplementalEvidence) error {
	if in == nil {
		return fmt.Errorf("supplemental_evidence must be set")
	}
	if err := validateDiagnosisWSSupplementalEvidenceLine("supplemental_evidence.label", in.Label); err != nil {
		return err
	}
	if err := validateDiagnosisWSSupplementalEvidenceLine("supplemental_evidence.detail", in.Detail); err != nil {
		return err
	}
	if err := validateDiagnosisWSSupplementalEvidenceLine("supplemental_evidence.priority", in.Priority); err != nil {
		return err
	}
	switch in.Priority {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("supplemental_evidence.priority is unsupported")
	}
	if strings.TrimSpace(in.Evidence) == "" {
		return fmt.Errorf("supplemental_evidence.evidence must be non-empty")
	}
	if len([]byte(in.Evidence)) > diagnosisroom.HardMaxMessageBytes {
		return fmt.Errorf("supplemental_evidence.evidence exceeds %d bytes", diagnosisroom.HardMaxMessageBytes)
	}
	return nil
}

func validateDiagnosisWSSupplementalEvidenceLine(label string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-empty", label)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", label)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s must be single-line", label)
	}
	return nil
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
		if frame.SupplementalEvidence != nil {
			return frame, fmt.Errorf("submit_turn frame must not include supplemental_evidence")
		}
		if len(frame.EvidenceRequests) > 0 {
			return frame, fmt.Errorf("submit_turn frame must not include evidence_requests")
		}
	case diagnosisWSClientSubmitSupplementalEvidence:
		if strings.TrimSpace(frame.MessageID) == "" {
			return frame, fmt.Errorf("message_id must be non-empty")
		}
		if strings.TrimSpace(frame.MessageID) != frame.MessageID {
			return frame, fmt.Errorf("message_id must not contain leading or trailing whitespace")
		}
		if strings.TrimSpace(frame.Message) == "" {
			return frame, fmt.Errorf("message must be non-empty")
		}
		if err := validateDiagnosisWSSupplementalEvidence(frame.SupplementalEvidence); err != nil {
			return frame, err
		}
		if len(frame.EvidenceRequests) > 0 {
			return frame, fmt.Errorf("submit_supplemental_evidence frame must not include evidence_requests")
		}
	case diagnosisWSClientCollectEvidence:
		if strings.TrimSpace(frame.MessageID) == "" {
			return frame, fmt.Errorf("message_id must be non-empty")
		}
		if strings.TrimSpace(frame.MessageID) != frame.MessageID {
			return frame, fmt.Errorf("message_id must not contain leading or trailing whitespace")
		}
		if strings.TrimSpace(frame.Message) == "" {
			return frame, fmt.Errorf("message must be non-empty")
		}
		if frame.SupplementalEvidence != nil {
			return frame, fmt.Errorf("collect_evidence frame must not include supplemental_evidence")
		}
		if len(frame.EvidenceRequests) == 0 {
			return frame, fmt.Errorf("collect_evidence frame must include evidence_requests")
		}
	case diagnosisWSClientQueryState:
		if frame.MessageID != "" || frame.Message != "" {
			return frame, fmt.Errorf("query_state frame must not include message_id or message")
		}
		if frame.Reason != "" {
			return frame, fmt.Errorf("query_state frame must not include reason")
		}
		if frame.SupplementalEvidence != nil {
			return frame, fmt.Errorf("query_state frame must not include supplemental_evidence")
		}
		if len(frame.EvidenceRequests) > 0 {
			return frame, fmt.Errorf("query_state frame must not include evidence_requests")
		}
	case diagnosisWSClientConfirm:
		if frame.MessageID != "" || frame.Message != "" {
			return frame, fmt.Errorf("confirm_conclusion frame must not include message_id or message")
		}
		if strings.TrimSpace(frame.Reason) != frame.Reason {
			return frame, fmt.Errorf("reason must not contain leading or trailing whitespace")
		}
		if frame.SupplementalEvidence != nil {
			return frame, fmt.Errorf("confirm_conclusion frame must not include supplemental_evidence")
		}
		if len(frame.EvidenceRequests) > 0 {
			return frame, fmt.Errorf("confirm_conclusion frame must not include evidence_requests")
		}
	case "":
		return frame, fmt.Errorf("type must be non-empty")
	default:
		return frame, fmt.Errorf("unsupported frame type %q", frame.Type)
	}
	return frame, nil
}

func diagnosisWSSupplementalEvidencePort(
	in *diagnosisWSSupplementalEvidence,
) *ports.DiagnosisRoomSupplementalEvidence {
	if in == nil {
		return nil
	}
	return &ports.DiagnosisRoomSupplementalEvidence{
		Label:    strings.TrimSpace(in.Label),
		Detail:   strings.TrimSpace(in.Detail),
		Priority: strings.TrimSpace(in.Priority),
		Evidence: strings.TrimSpace(in.Evidence),
	}
}

func diagnosisWSEvidenceRequestsPort(in []diagnosisWSEvidenceRequest) []ports.DiagnosisRoomEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]ports.DiagnosisRoomEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = ports.DiagnosisRoomEvidenceRequest{
			TemplateID:           domain.DiagnosisToolTemplateID(request.TemplateID),
			AlertSourceProfileID: domain.AlertSourceProfileID(request.AlertSourceProfileID),
			Tool:                 domain.DiagnosisToolKind(strings.TrimSpace(request.Tool)),
			Reason:               strings.TrimSpace(request.Reason),
			Query:                strings.TrimSpace(request.Query),
			WindowSeconds:        request.WindowSeconds,
			StepSeconds:          request.StepSeconds,
			Limit:                request.Limit,
		}
	}
	return out
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
		ConversationSummary: diagnosisWSConversationSummaryFrame(state.ConversationSummary),
		ApprovalMode:        state.ApprovalMode,
		ConclusionDigest:    state.ConclusionDigest,
		Approvals:           diagnosisWSConclusionApprovals(state.Approvals),
		PendingApprovalAuthorities: append(
			[]domain.DiagnosisApprovalAuthority(nil),
			state.PendingApprovalAuthorities...,
		),
		ApprovalInFlight:    state.ApprovalInFlight,
		Confidence:          state.LatestConfidence,
		RequiresHumanReview: state.LatestRequiresHumanReview,
		EvidenceRequests:    diagnosisWSEvidenceRequests(state.LatestEvidenceRequests),
		CollectionResults:   diagnosisWSEvidenceCollectionResults(state.LatestCollectionResults),
		EvidenceTimeline:    diagnosisWSEvidenceTimeline(state.EvidenceTimeline),
		ConfidenceTimeline:  diagnosisWSConfidenceTimeline(state.ConfidenceTimeline),
		SupplementalEvidence: diagnosisWSSupplementalEvidenceRecords(
			state.SupplementalEvidence,
		),
		ConsultationInsight: diagnosisWSConsultationInsightFramePtr(state.LatestConsultationInsight),
		LatestError:         diagnosisWSLatestErrorFrame(state.LatestError),
		InFlight:            state.InFlight,
		SeenMessageIDs:      append([]string(nil), state.SeenMessageIDs...),
		Conversation:        diagnosisWSConversation(state.Conversation),
	}
}

func diagnosisWSConclusionApprovals(in []ports.DiagnosisRoomConclusionApproval) []diagnosisWSConclusionApproval {
	out := make([]diagnosisWSConclusionApproval, len(in))
	for i, approval := range in {
		out[i] = diagnosisWSConclusionApproval{
			ID:               int64(approval.ID),
			ConclusionDigest: approval.ConclusionDigest,
			ActorSubject:     approval.ActorSubject,
			Authority:        approval.Authority,
			Reason:           approval.Reason,
			ApprovedAt:       approval.ApprovedAt,
		}
	}
	return out
}

func diagnosisWSStateFrameFromCollectResult(result ports.DiagnosisRoomCollectEvidenceResult) diagnosisWSStateFrame {
	frame := diagnosisWSStateFrameFromState(result.State)
	frame.FollowUpTurns = diagnosisWSFollowUpTurns(result.FollowUpTurns)
	return frame
}

func writeDiagnosisWSError(conn *websocket.Conn, err error) error {
	frame := diagnosisWSErrorFrame{
		Type:    diagnosisWSServerError,
		Code:    diagnosisWSErrorCode(err),
		Message: diagnosisWSErrorMessage(err),
	}
	return writeDiagnosisWSJSON(conn, frame)
}

func writeDiagnosisWSConfirmError(conn *websocket.Conn, err error) error {
	frame := diagnosisWSErrorFrame{
		Type:    diagnosisWSServerError,
		Code:    diagnosisWSConfirmErrorCode(err),
		Message: diagnosisWSErrorMessage(err),
	}
	return writeDiagnosisWSJSON(conn, frame)
}

func diagnosisWSConfirmErrorCode(err error) string {
	if errors.Is(err, domain.ErrPreconditionFailed) {
		return "confirm_rejected"
	}
	return diagnosisWSErrorCode(err)
}

func diagnosisWSErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "turn_still_processing"
	case errors.Is(err, diagnosisauth.ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, diagnosisauth.ErrUnauthorized):
		return "unauthorized"
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
	if errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		return "unauthenticated"
	}
	if errors.Is(err, diagnosisauth.ErrUnauthorized) {
		return "unauthorized"
	}
	return err.Error()
}

func diagnosisWSConversation(in []ports.DiagnosisRoomConversationTurn) []diagnosisWSConversationTurn {
	out := make([]diagnosisWSConversationTurn, len(in))
	for i, turn := range in {
		out[i] = diagnosisWSConversationTurn{
			Role:         turn.Role,
			ActorSubject: turn.ActorSubject,
			Content:      turn.Content,
		}
	}
	return out
}

func diagnosisWSLatestErrorFrame(in *ports.DiagnosisRoomLatestError) *diagnosisWSLatestError {
	if in == nil {
		return nil
	}
	return &diagnosisWSLatestError{
		Code:       in.Code,
		Message:    in.Message,
		MessageID:  in.MessageID,
		OccurredAt: in.OccurredAt,
	}
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
		ConfidenceRationale:     in.ConfidenceRationale,
		Findings:                append([]string(nil), in.Findings...),
		RecommendedActions:      append([]string(nil), in.RecommendedActions...),
		EvidenceRequests:        diagnosisWSEvidenceRequests(in.EvidenceRequests),
		MissingEvidenceRequests: diagnosisWSConsultationEvidenceRequests(in.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisWSConsultationEvidenceRequests(
			in.EvidenceCollectionSuggestions,
		),
	}
}

func diagnosisWSConversationSummaryFrame(in *ports.DiagnosisRoomConversationSummary) *diagnosisWSConversationSummary {
	if in == nil {
		return nil
	}
	return &diagnosisWSConversationSummary{
		ID:                  int64(in.ID),
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

func diagnosisWSEvidenceRequests(in []ports.DiagnosisRoomEvidenceRequest) []diagnosisWSEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSEvidenceRequest, len(in))
	for i, request := range in {
		out[i] = diagnosisWSEvidenceRequest{
			TemplateID:           int64(request.TemplateID),
			AlertSourceProfileID: int64(request.AlertSourceProfileID),
			Tool:                 string(request.Tool),
			Reason:               request.Reason,
			Query:                request.Query,
			WindowSeconds:        request.WindowSeconds,
			StepSeconds:          request.StepSeconds,
			Limit:                request.Limit,
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

func diagnosisWSEvidenceTimelineFromTurnResult(
	in ports.DiagnosisRoomSubmitTurnResult,
	actorSubject string,
) []diagnosisWSEvidenceTimelineEntry {
	if len(in.EvidenceTimeline) > 0 {
		return diagnosisWSEvidenceTimeline(in.EvidenceTimeline)
	}
	entries := make([]ports.DiagnosisRoomEvidenceTimelineEntry, 0, 1+len(in.FollowUpTurns))
	if len(in.EvidenceRequests) > 0 || len(in.CollectionResults) > 0 {
		entries = append(entries, ports.DiagnosisRoomEvidenceTimelineEntry{
			TurnCount:          in.TurnCount,
			MessageID:          in.MessageID,
			AssistantMessageID: in.AssistantMessageID,
			ActorSubject:       actorSubject,
			Trigger:            "operator_turn",
			EvidenceRequests:   in.EvidenceRequests,
			CollectionResults:  in.CollectionResults,
		})
	}
	for _, turn := range in.FollowUpTurns {
		if len(turn.EvidenceRequests) == 0 && len(turn.CollectionResults) == 0 {
			continue
		}
		entries = append(entries, ports.DiagnosisRoomEvidenceTimelineEntry{
			TurnCount:          turn.TurnCount,
			MessageID:          turn.MessageID,
			AssistantMessageID: turn.AssistantMessageID,
			ActorSubject:       actorSubject,
			Trigger:            turn.Trigger,
			EvidenceRequests:   turn.EvidenceRequests,
			CollectionResults:  turn.CollectionResults,
		})
	}
	return diagnosisWSEvidenceTimeline(entries)
}

func diagnosisWSEvidenceTimeline(
	in []ports.DiagnosisRoomEvidenceTimelineEntry,
) []diagnosisWSEvidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSEvidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = diagnosisWSEvidenceTimelineEntry{
			TurnCount:          item.TurnCount,
			MessageID:          item.MessageID,
			AssistantMessageID: item.AssistantMessageID,
			ActorSubject:       item.ActorSubject,
			Trigger:            item.Trigger,
			EvidenceRequests:   diagnosisWSEvidenceRequests(item.EvidenceRequests),
			CollectionResults:  diagnosisWSEvidenceCollectionResults(item.CollectionResults),
		}
	}
	return out
}

func diagnosisWSConfidenceTimeline(
	in []ports.DiagnosisRoomConfidenceTimelineEntry,
) []diagnosisWSConfidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSConfidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = diagnosisWSConfidenceTimelineEntry{
			TurnCount:                     item.TurnCount,
			MessageID:                     item.MessageID,
			AssistantMessageID:            item.AssistantMessageID,
			AssistantTurnID:               int64(item.AssistantTurnID),
			AssistantSequence:             item.AssistantSequence,
			OccurredAt:                    item.OccurredAt,
			Trigger:                       item.Trigger,
			Confidence:                    item.Confidence,
			RequiresHumanReview:           item.RequiresHumanReview,
			ConclusionStatus:              item.ConclusionStatus,
			ConfidenceRationale:           item.ConfidenceRationale,
			EvidenceRequests:              diagnosisWSEvidenceRequests(item.EvidenceRequests),
			CollectionResults:             diagnosisWSEvidenceCollectionResults(item.CollectionResults),
			MissingEvidenceRequests:       diagnosisWSConsultationEvidenceRequests(item.MissingEvidenceRequests),
			EvidenceCollectionSuggestions: diagnosisWSConsultationEvidenceRequests(item.EvidenceCollectionSuggestions),
		}
	}
	return out
}

func diagnosisWSEvidenceRequestFrame(request ports.DiagnosisRoomEvidenceRequest) diagnosisWSEvidenceRequest {
	return diagnosisWSEvidenceRequest{
		TemplateID:           int64(request.TemplateID),
		AlertSourceProfileID: int64(request.AlertSourceProfileID),
		Tool:                 string(request.Tool),
		Reason:               request.Reason,
		Query:                request.Query,
		WindowSeconds:        request.WindowSeconds,
		StepSeconds:          request.StepSeconds,
		Limit:                request.Limit,
	}
}

func diagnosisWSActiveAlerts(in []ports.DiagnosisRoomActiveAlert) []diagnosisWSActiveAlert {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSActiveAlert, len(in))
	for i, alert := range in {
		out[i] = diagnosisWSActiveAlert{
			Source:               alert.Source,
			AlertSourceProfileID: int64(alert.AlertSourceProfileID),
			Labels:               cloneDiagnosisWSStringMap(alert.Labels),
			Annotations:          cloneDiagnosisWSStringMap(alert.Annotations),
			StartsAt:             alert.StartsAt,
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

func diagnosisWSSupplementalEvidenceRecords(
	in []ports.DiagnosisRoomSupplementalEvidenceRecord,
) []diagnosisWSSupplementalEvidenceRecord {
	if in == nil {
		return nil
	}
	out := make([]diagnosisWSSupplementalEvidenceRecord, len(in))
	for i, item := range in {
		out[i] = diagnosisWSSupplementalEvidenceRecord{
			Label:              item.Label,
			Detail:             item.Detail,
			Priority:           item.Priority,
			Evidence:           item.Evidence,
			ActorSubject:       item.ActorSubject,
			UserMessageID:      item.UserMessageID,
			AssistantMessageID: item.AssistantMessageID,
			UserTurnID:         int64(item.UserTurnID),
			AssistantTurnID:    int64(item.AssistantTurnID),
			UserSequence:       item.UserSequence,
			AssistantSequence:  item.AssistantSequence,
			ProvidedAt:         item.ProvidedAt,
		}
	}
	return out
}

func writeDiagnosisWSJSON(conn *websocket.Conn, value interface{}) error {
	if err := conn.SetWriteDeadline(time.Now().Add(defaultDiagnosisWSWriteTimeout)); err != nil {
		return err
	}
	if err := conn.WriteJSON(value); err != nil {
		return err
	}
	return nil
}

type diagnosisWSClientFrame struct {
	Type                 string                           `json:"type"`
	MessageID            string                           `json:"message_id,omitempty"`
	Message              string                           `json:"message,omitempty"`
	Reason               string                           `json:"reason,omitempty"`
	SupplementalEvidence *diagnosisWSSupplementalEvidence `json:"supplemental_evidence,omitempty"`
	EvidenceRequests     []diagnosisWSEvidenceRequest     `json:"evidence_requests,omitempty"`
}

type diagnosisWSSupplementalEvidence struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
	Evidence string `json:"evidence"`
}

type diagnosisWSSupplementalEvidenceRecord struct {
	Label              string    `json:"label"`
	Detail             string    `json:"detail"`
	Priority           string    `json:"priority"`
	Evidence           string    `json:"evidence"`
	ActorSubject       string    `json:"actor_subject,omitempty"`
	UserMessageID      string    `json:"user_message_id"`
	AssistantMessageID string    `json:"assistant_message_id"`
	UserTurnID         int64     `json:"user_turn_id"`
	AssistantTurnID    int64     `json:"assistant_turn_id"`
	UserSequence       int       `json:"user_sequence"`
	AssistantSequence  int       `json:"assistant_sequence"`
	ProvidedAt         time.Time `json:"provided_at"`
}

type diagnosisWSReadyFrame struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Subject   string `json:"subject"`
}

type diagnosisWSTurnStreamFrame struct {
	Type               string                         `json:"type"`
	Phase              ports.DiagnosisTurnStreamPhase `json:"phase"`
	SessionID          string                         `json:"session_id"`
	MessageID          string                         `json:"message_id"`
	AssistantMessageID string                         `json:"assistant_message_id"`
	ActivityAttempt    int                            `json:"activity_attempt"`
	GenerationAttempt  int                            `json:"generation_attempt"`
	Sequence           int                            `json:"sequence"`
	AssistantMessage   string                         `json:"assistant_message"`
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
	EvidenceTimeline    []diagnosisWSEvidenceTimelineEntry    `json:"evidence_timeline,omitempty"`
	ConfidenceTimeline  []diagnosisWSConfidenceTimelineEntry  `json:"confidence_timeline,omitempty"`
	ConsultationInsight diagnosisWSConsultationInsight        `json:"consultation_insight"`
	FollowUpTurns       []diagnosisWSFollowUpTurn             `json:"follow_up_turns,omitempty"`
	LatestError         *diagnosisWSLatestError               `json:"latest_error,omitempty"`
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
	Type                       string                                  `json:"type"`
	SessionID                  string                                  `json:"session_id"`
	ChatSessionID              int64                                   `json:"chat_session_id"`
	DiagnosisTaskID            int64                                   `json:"diagnosis_task_id"`
	OwnerSubject               string                                  `json:"owner_subject"`
	Status                     string                                  `json:"status"`
	TurnCount                  int                                     `json:"turn_count"`
	StartedAt                  time.Time                               `json:"started_at"`
	LastActivityAt             time.Time                               `json:"last_activity_at"`
	ClosedAt                   *time.Time                              `json:"closed_at,omitempty"`
	CloseReason                string                                  `json:"close_reason,omitempty"`
	FinalConclusion            *diagnosisWSFinalConclusion             `json:"final_conclusion,omitempty"`
	ConversationSummary        *diagnosisWSConversationSummary         `json:"conversation_summary,omitempty"`
	ApprovalMode               domain.DiagnosisApprovalMode            `json:"approval_mode"`
	ConclusionDigest           string                                  `json:"conclusion_digest,omitempty"`
	Approvals                  []diagnosisWSConclusionApproval         `json:"approvals,omitempty"`
	PendingApprovalAuthorities []domain.DiagnosisApprovalAuthority     `json:"pending_approval_authorities,omitempty"`
	ApprovalInFlight           bool                                    `json:"approval_in_flight"`
	Confidence                 string                                  `json:"confidence,omitempty"`
	RequiresHumanReview        *bool                                   `json:"requires_human_review,omitempty"`
	EvidenceRequests           []diagnosisWSEvidenceRequest            `json:"evidence_requests,omitempty"`
	CollectionResults          []diagnosisWSEvidenceCollectionResult   `json:"evidence_collection_results,omitempty"`
	EvidenceTimeline           []diagnosisWSEvidenceTimelineEntry      `json:"evidence_timeline,omitempty"`
	ConfidenceTimeline         []diagnosisWSConfidenceTimelineEntry    `json:"confidence_timeline,omitempty"`
	SupplementalEvidence       []diagnosisWSSupplementalEvidenceRecord `json:"supplemental_evidence,omitempty"`
	ConsultationInsight        *diagnosisWSConsultationInsight         `json:"consultation_insight,omitempty"`
	FollowUpTurns              []diagnosisWSFollowUpTurn               `json:"follow_up_turns,omitempty"`
	LatestError                *diagnosisWSLatestError                 `json:"latest_error,omitempty"`
	InFlight                   bool                                    `json:"in_flight"`
	SeenMessageIDs             []string                                `json:"seen_message_ids"`
	Conversation               []diagnosisWSConversationTurn           `json:"conversation"`
}

type diagnosisWSConclusionApproval struct {
	ID               int64                             `json:"id"`
	ConclusionDigest string                            `json:"conclusion_digest"`
	ActorSubject     string                            `json:"actor_subject"`
	Authority        domain.DiagnosisApprovalAuthority `json:"authority"`
	Reason           string                            `json:"reason"`
	ApprovedAt       time.Time                         `json:"approved_at"`
}

type diagnosisWSConversationSummary struct {
	ID                  int64           `json:"id"`
	Version             int             `json:"version"`
	SchemaVersion       string          `json:"schema_version"`
	SourceFirstSequence int             `json:"source_first_sequence"`
	SourceLastSequence  int             `json:"source_last_sequence"`
	SourceTurnCount     int             `json:"source_turn_count"`
	SourceDigest        string          `json:"source_digest"`
	Content             json.RawMessage `json:"content"`
	GeneratedAt         time.Time       `json:"generated_at"`
}

type diagnosisWSLatestError struct {
	Code       string    `json:"code"`
	Message    string    `json:"message"`
	MessageID  string    `json:"message_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type diagnosisWSConversationTurn struct {
	Role         string `json:"role"`
	ActorSubject string `json:"actor_subject,omitempty"`
	Content      string `json:"content"`
}

type diagnosisWSFinalConclusion struct {
	Status                        string                                   `json:"status"`
	Source                        string                                   `json:"source"`
	Reason                        string                                   `json:"reason,omitempty"`
	EvidenceSnapshotID            int64                                    `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion             string                                   `json:"conclusion_version,omitempty"`
	RecordedAt                    *time.Time                               `json:"recorded_at,omitempty"`
	ConfirmedBy                   string                                   `json:"confirmed_by,omitempty"`
	SupplementalContextRefs       []string                                 `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID               int64                                    `json:"assistant_turn_id,omitempty"`
	AssistantMessageID            string                                   `json:"assistant_message_id,omitempty"`
	AssistantSequence             int                                      `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt           *time.Time                               `json:"assistant_occurred_at,omitempty"`
	Content                       string                                   `json:"content,omitempty"`
	Confidence                    string                                   `json:"confidence,omitempty"`
	RequiresHumanReview           *bool                                    `json:"requires_human_review,omitempty"`
	ConfidenceRationale           string                                   `json:"confidence_rationale,omitempty"`
	Findings                      []string                                 `json:"findings,omitempty"`
	RecommendedActions            []string                                 `json:"recommended_actions,omitempty"`
	EvidenceRequests              []diagnosisWSEvidenceRequest             `json:"evidence_requests,omitempty"`
	MissingEvidenceRequests       []diagnosisWSConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisWSConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
}

type diagnosisWSEvidenceRequest struct {
	TemplateID           int64  `json:"template_id,omitempty"`
	AlertSourceProfileID int64  `json:"alert_source_profile_id,omitempty"`
	Tool                 string `json:"tool"`
	Reason               string `json:"reason"`
	Query                string `json:"query,omitempty"`
	WindowSeconds        int    `json:"window_seconds,omitempty"`
	StepSeconds          int    `json:"step_seconds,omitempty"`
	Limit                int    `json:"limit,omitempty"`
}

type diagnosisWSEvidenceTimelineEntry struct {
	TurnCount          int                                   `json:"turn_count"`
	MessageID          string                                `json:"message_id,omitempty"`
	AssistantMessageID string                                `json:"assistant_message_id,omitempty"`
	ActorSubject       string                                `json:"actor_subject,omitempty"`
	Trigger            string                                `json:"trigger,omitempty"`
	EvidenceRequests   []diagnosisWSEvidenceRequest          `json:"evidence_requests,omitempty"`
	CollectionResults  []diagnosisWSEvidenceCollectionResult `json:"evidence_collection_results,omitempty"`
}

type diagnosisWSConfidenceTimelineEntry struct {
	TurnCount                     int                                      `json:"turn_count"`
	MessageID                     string                                   `json:"message_id,omitempty"`
	AssistantMessageID            string                                   `json:"assistant_message_id,omitempty"`
	AssistantTurnID               int64                                    `json:"assistant_turn_id,omitempty"`
	AssistantSequence             int                                      `json:"assistant_sequence,omitempty"`
	OccurredAt                    time.Time                                `json:"occurred_at"`
	Trigger                       string                                   `json:"trigger,omitempty"`
	Confidence                    string                                   `json:"confidence"`
	RequiresHumanReview           bool                                     `json:"requires_human_review"`
	ConclusionStatus              string                                   `json:"conclusion_status,omitempty"`
	ConfidenceRationale           string                                   `json:"confidence_rationale,omitempty"`
	EvidenceRequests              []diagnosisWSEvidenceRequest             `json:"evidence_requests,omitempty"`
	CollectionResults             []diagnosisWSEvidenceCollectionResult    `json:"evidence_collection_results,omitempty"`
	MissingEvidenceRequests       []diagnosisWSConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisWSConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
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
	Source               string            `json:"source"`
	AlertSourceProfileID int64             `json:"alert_source_profile_id,omitempty"`
	Labels               map[string]string `json:"labels"`
	Annotations          map[string]string `json:"annotations"`
	StartsAt             time.Time         `json:"starts_at"`
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
