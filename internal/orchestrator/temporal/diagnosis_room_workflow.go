package temporal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
)

const (
	// DiagnosisRoomSubmitTurnUpdate is the primary M5 user-message path.
	DiagnosisRoomSubmitTurnUpdate = "submit-turn"
	// DiagnosisRoomStateQuery returns the current workflow-visible room state.
	DiagnosisRoomStateQuery = "state"
	// DiagnosisRoomCloseSignal closes a room by user/system request.
	DiagnosisRoomCloseSignal = "close"
	// DiagnosisRoomCancelSignal cancels a room by operator/system request.
	DiagnosisRoomCancelSignal = "cancel"
)

const (
	diagnosisRoomStatusOpen   = "open"
	diagnosisRoomStatusClosed = "closed"

	diagnosisRoomCloseUserRequested   = "user_requested"
	diagnosisRoomCloseCancelled       = "cancelled"
	diagnosisRoomCloseSessionTimeout  = "session_timeout"
	diagnosisRoomCloseIdleTimeout     = "idle_timeout"
	diagnosisRoomCloseContextCanceled = "context_cancelled"
)

// DiagnosisRoomWorkflowInput configures one M5 short-conversation diagnosis
// room. The workflow can start from an existing DiagnosisTask or create the
// task from a frozen EvidenceSnapshot. SessionID is the external room id used
// by WebSocket auth/reconnect flows.
type DiagnosisRoomWorkflowInput struct {
	SessionID          string
	DiagnosisTaskID    int64
	EvidenceSnapshotID int64
	OwnerSubject       string
	Evidence           json.RawMessage
	Policy             diagnosisroom.Policy
}

// SubmitDiagnosisTurnRequest is the Update payload for one user turn.
type SubmitDiagnosisTurnRequest struct {
	MessageID    string
	ActorSubject string
	Message      string
}

// SubmitDiagnosisTurnResult is returned after the workflow accepts a user
// message into its durable conversation state. The sandbox activity and
// assistant response are still separate M5 work.
type SubmitDiagnosisTurnResult struct {
	SessionID           string
	ChatSessionID       int64
	MessageID           string
	AssistantMessageID  string
	UserTurnID          int64
	AssistantTurnID     int64
	UserSequence        int
	AssistantSequence   int
	TurnCount           int
	ContextBytes        int
	Status              string
	AssistantMessage    string
	RequiresHumanReview bool
	Confidence          string
}

// DiagnosisRoomCloseRequest carries the close/cancel signal reason.
type DiagnosisRoomCloseRequest struct {
	Reason string
}

// DiagnosisRoomWorkflowState is the read model returned by the state query
// and by workflow completion.
type DiagnosisRoomWorkflowState struct {
	SessionID       string
	ChatSessionID   int64
	DiagnosisTaskID int64
	OwnerSubject    string
	Status          string
	TurnCount       int
	StartedAt       time.Time
	LastActivityAt  time.Time
	ClosedAt        *time.Time
	CloseReason     string
	InFlight        bool
	SeenMessageIDs  []string
	Conversation    []diagnosisroom.ConversationTurn
}

// DiagnosisRoomWorkflowResult is the terminal room state.
type DiagnosisRoomWorkflowResult = DiagnosisRoomWorkflowState

type diagnosisRoomState struct {
	input           DiagnosisRoomWorkflowInput
	policy          diagnosisroom.Policy
	status          string
	startedAt       time.Time
	lastActivityAt  time.Time
	closedAt        *time.Time
	closeReason     string
	turnCount       int
	diagnosisTaskID int64
	chatSessionID   int64
	inFlight        bool
	seen            map[string]struct{}
	conversation    []diagnosisroom.ConversationTurn
}

// DiagnosisRoomWorkflow owns the M5 room lifecycle: Update for user messages,
// Query for reconnect state, Signals for close/cancel, durable timers for fixed
// lifetime and idle timeout, per-turn sandbox execution, and transcript
// persistence.
func DiagnosisRoomWorkflow(ctx workflow.Context, input DiagnosisRoomWorkflowInput) (DiagnosisRoomWorkflowResult, error) {
	policy := diagnosisRoomPolicyOrDefault(input.Policy)
	if err := validateDiagnosisRoomWorkflowInput(input, policy); err != nil {
		return DiagnosisRoomWorkflowResult{}, temporalsdk.NewNonRetryableApplicationError(
			err.Error(), errTypeInvalidInput, nil)
	}

	now := workflow.Now(ctx)
	state := &diagnosisRoomState{
		input:           input,
		policy:          policy,
		status:          diagnosisRoomStatusOpen,
		startedAt:       now,
		lastActivityAt:  now,
		diagnosisTaskID: input.DiagnosisTaskID,
		seen:            map[string]struct{}{},
		conversation:    []diagnosisroom.ConversationTurn{},
	}
	startupComplete := false

	if err := workflow.SetQueryHandler(ctx, DiagnosisRoomStateQuery, func() (DiagnosisRoomWorkflowState, error) {
		return state.snapshot(), nil
	}); err != nil {
		return DiagnosisRoomWorkflowResult{}, err
	}

	if err := workflow.SetUpdateHandlerWithOptions(
		ctx,
		DiagnosisRoomSubmitTurnUpdate,
		func(ctx workflow.Context, req SubmitDiagnosisTurnRequest) (SubmitDiagnosisTurnResult, error) {
			if err := workflow.Await(ctx, func() bool { return startupComplete }); err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}
			decision, err := state.validateSubmit(ctx, req)
			if err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}
			state.inFlight = true
			defer func() { state.inFlight = false }()

			userOccurredAt := workflow.Now(ctx)
			priorConversation := state.conversationCopy()
			userSequence := len(state.conversation) + 1
			assistantSequence := userSequence + 1
			messageID := strings.TrimSpace(req.MessageID)
			activityReq := DiagnosisTurnActivityInput{
				SessionID:         state.input.SessionID,
				DiagnosisTaskID:   state.diagnosisTaskID,
				MessageID:         messageID,
				UserSequence:      userSequence,
				AssistantSequence: assistantSequence,
				ActorSubject:      strings.TrimSpace(req.ActorSubject),
				Evidence:          state.input.Evidence,
				Conversation:      priorConversation,
				Message:           req.Message,
				Policy:            state.policy,
			}
			actCtx := workflow.WithActivityOptions(ctx, diagnosisRoomTurnActivityOptions(state.policy))
			var activityResult DiagnosisTurnActivityResult
			if err := workflow.ExecuteActivity(actCtx, (*Activities).RunDiagnosisTurn, activityReq).Get(ctx, &activityResult); err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}
			assistantOccurredAt := workflow.Now(ctx)
			persistReq := PersistDiagnosisTurnInput{
				SessionID:           state.input.SessionID,
				DiagnosisTaskID:     state.diagnosisTaskID,
				OwnerSubject:        state.input.OwnerSubject,
				UserMessageID:       messageID,
				AssistantMessageID:  activityResult.AssistantMessageID,
				UserSequence:        userSequence,
				AssistantSequence:   activityResult.AssistantSequence,
				TurnCount:           state.turnCount + 1,
				ActorSubject:        strings.TrimSpace(req.ActorSubject),
				UserMessage:         req.Message,
				AssistantMessage:    activityResult.AssistantMessage,
				UserOccurredAt:      userOccurredAt,
				AssistantOccurredAt: assistantOccurredAt,
				ContextBytes:        decision.ContextBytes,
				InvocationID:        activityResult.InvocationID,
				RuntimeID:           activityResult.RuntimeID,
				ContainerStartedAt:  activityResult.StartedAt,
				ContainerFinishedAt: activityResult.FinishedAt,
				RawOutput:           activityResult.RawOutput,
			}
			var persistResult PersistDiagnosisTurnResult
			persistCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
			if err := workflow.ExecuteActivity(persistCtx, (*Activities).PersistDiagnosisTurn, persistReq).Get(ctx, &persistResult); err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}

			state.chatSessionID = persistResult.ChatSessionID
			state.turnCount++
			state.lastActivityAt = persistResult.LastActivityAt
			state.seen[messageID] = struct{}{}
			state.conversation = append(state.conversation, diagnosisroom.ConversationTurn{
				Role:    string(diagnosisroomRoleUser),
				Content: strings.TrimSpace(req.Message),
			})
			state.conversation = append(state.conversation, diagnosisroom.ConversationTurn{
				Role:    string(diagnosisroomRoleAssistant),
				Content: activityResult.AssistantMessage,
			})
			return SubmitDiagnosisTurnResult{
				SessionID:           state.input.SessionID,
				ChatSessionID:       persistResult.ChatSessionID,
				MessageID:           messageID,
				AssistantMessageID:  activityResult.AssistantMessageID,
				UserTurnID:          persistResult.UserTurnID,
				AssistantTurnID:     persistResult.AssistantTurnID,
				UserSequence:        userSequence,
				AssistantSequence:   activityResult.AssistantSequence,
				TurnCount:           state.turnCount,
				ContextBytes:        decision.ContextBytes,
				Status:              state.status,
				AssistantMessage:    activityResult.AssistantMessage,
				RequiresHumanReview: activityResult.RequiresHumanReview,
				Confidence:          activityResult.Confidence,
			}, nil
		},
		workflow.UpdateHandlerOptions{
			Validator: func(ctx workflow.Context, req SubmitDiagnosisTurnRequest) error {
				_, err := state.validateSubmit(ctx, req)
				return err
			},
		},
	); err != nil {
		return DiagnosisRoomWorkflowResult{}, err
	}

	ensureCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
	if state.diagnosisTaskID == 0 {
		info := workflow.GetInfo(ctx)
		var ensureResult EnsureDiagnosisRoomSessionResult
		if err := workflow.ExecuteActivity(ensureCtx, (*Activities).EnsureDiagnosisRoomSession, EnsureDiagnosisRoomSessionInput{
			SessionID:          state.input.SessionID,
			EvidenceSnapshotID: state.input.EvidenceSnapshotID,
			WorkflowID:         info.WorkflowExecution.ID,
			RunID:              info.WorkflowExecution.RunID,
			OwnerSubject:       state.input.OwnerSubject,
			StartedAt:          state.startedAt,
		}).Get(ctx, &ensureResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
		state.diagnosisTaskID = ensureResult.DiagnosisTaskID
		state.chatSessionID = ensureResult.ChatSessionID
	} else {
		var ensureResult EnsureDiagnosisChatSessionResult
		if err := workflow.ExecuteActivity(ensureCtx, (*Activities).EnsureDiagnosisChatSession, EnsureDiagnosisChatSessionInput{
			SessionID:       state.input.SessionID,
			DiagnosisTaskID: state.diagnosisTaskID,
			OwnerSubject:    state.input.OwnerSubject,
			StartedAt:       state.startedAt,
		}).Get(ctx, &ensureResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
		state.chatSessionID = ensureResult.ChatSessionID
	}
	startupComplete = true

	closeCh := workflow.GetSignalChannel(ctx, DiagnosisRoomCloseSignal)
	cancelCh := workflow.GetSignalChannel(ctx, DiagnosisRoomCancelSignal)

	for state.status == diagnosisRoomStatusOpen {
		if state.closeIfExpired(workflow.Now(ctx)) {
			break
		}
		timerDelay := state.nextTimerDelay(workflow.Now(ctx))
		timerCtx, cancelTimer := workflow.WithCancel(ctx)
		timer := workflow.NewTimer(timerCtx, timerDelay)

		selector := workflow.NewSelector(ctx)
		var timerErr error
		selector.AddFuture(timer, func(f workflow.Future) {
			if err := f.Get(ctx, nil); err != nil {
				timerErr = err
				return
			}
			state.closeIfExpired(workflow.Now(ctx))
		})
		selector.AddReceive(closeCh, func(c workflow.ReceiveChannel, _ bool) {
			var req DiagnosisRoomCloseRequest
			c.Receive(ctx, &req)
			state.close(workflow.Now(ctx), reasonOrDefault(req.Reason, diagnosisRoomCloseUserRequested))
		})
		selector.AddReceive(cancelCh, func(c workflow.ReceiveChannel, _ bool) {
			var req DiagnosisRoomCloseRequest
			c.Receive(ctx, &req)
			state.close(workflow.Now(ctx), reasonOrDefault(req.Reason, diagnosisRoomCloseCancelled))
		})

		selector.Select(ctx)
		cancelTimer()
		if timerErr != nil {
			state.close(workflow.Now(ctx), diagnosisRoomCloseContextCanceled)
			break
		}
	}

	if state.status == diagnosisRoomStatusClosed {
		closeCtxBase := ctx
		if state.closeReason == diagnosisRoomCloseContextCanceled {
			disconnectedCtx, _ := workflow.NewDisconnectedContext(ctx)
			closeCtxBase = disconnectedCtx
		}
		closeCtx := workflow.WithActivityOptions(closeCtxBase, diagnosisRoomPersistenceActivityOptions())
		var closeResult CloseDiagnosisChatSessionResult
		closedAt := state.lastActivityAt
		if state.closedAt != nil {
			closedAt = *state.closedAt
		}
		if err := workflow.ExecuteActivity(closeCtx, (*Activities).CloseDiagnosisChatSession, CloseDiagnosisChatSessionInput{
			SessionID:       state.input.SessionID,
			DiagnosisTaskID: state.diagnosisTaskID,
			OwnerSubject:    state.input.OwnerSubject,
			TurnCount:       state.turnCount,
			ClosedAt:        closedAt,
			Reason:          state.closeReason,
		}).Get(closeCtx, &closeResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
		state.chatSessionID = closeResult.ChatSessionID
		var notificationResult SendDiagnosisRoomCloseNotificationResult
		if err := workflow.ExecuteActivity(closeCtx, (*Activities).SendDiagnosisRoomCloseNotification, CloseDiagnosisChatSessionInput{
			SessionID:       state.input.SessionID,
			DiagnosisTaskID: state.diagnosisTaskID,
			OwnerSubject:    state.input.OwnerSubject,
			TurnCount:       state.turnCount,
			ClosedAt:        closeResult.ClosedAt,
			Reason:          state.closeReason,
		}).Get(closeCtx, &notificationResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
	}

	return state.snapshot(), nil
}

type diagnosisroomRole string

const (
	diagnosisroomRoleUser      diagnosisroomRole = "user"
	diagnosisroomRoleAssistant diagnosisroomRole = "assistant"
)

func (s *diagnosisRoomState) validateSubmit(ctx workflow.Context, req SubmitDiagnosisTurnRequest) (diagnosisroom.Decision, error) {
	if strings.TrimSpace(req.ActorSubject) == "" {
		return diagnosisroom.Decision{}, fmt.Errorf("diagnosis room turn: actor_subject must be non-empty")
	}
	return diagnosisroom.ValidateSubmitTurn(
		s.policy,
		diagnosisroom.SessionState{
			StartedAt:      s.startedAt,
			LastActivityAt: s.lastActivityAt,
			TurnCount:      s.turnCount,
			InFlight:       s.inFlight,
			SeenMessageIDs: s.seen,
		},
		diagnosisroom.SubmitTurnRequest{
			MessageID:    req.MessageID,
			Message:      req.Message,
			Now:          workflow.Now(ctx),
			Evidence:     s.input.Evidence,
			Conversation: s.conversation,
		},
	)
}

func (s *diagnosisRoomState) snapshot() DiagnosisRoomWorkflowState {
	seen := make([]string, 0, len(s.seen))
	for id := range s.seen {
		seen = append(seen, id)
	}
	sort.Strings(seen)
	return DiagnosisRoomWorkflowState{
		SessionID:       s.input.SessionID,
		ChatSessionID:   s.chatSessionID,
		DiagnosisTaskID: s.diagnosisTaskID,
		OwnerSubject:    s.input.OwnerSubject,
		Status:          s.status,
		TurnCount:       s.turnCount,
		StartedAt:       s.startedAt,
		LastActivityAt:  s.lastActivityAt,
		ClosedAt:        s.closedAt,
		CloseReason:     s.closeReason,
		InFlight:        s.inFlight,
		SeenMessageIDs:  seen,
		Conversation:    s.conversationCopy(),
	}
}

func (s *diagnosisRoomState) conversationCopy() []diagnosisroom.ConversationTurn {
	conversation := make([]diagnosisroom.ConversationTurn, len(s.conversation))
	copy(conversation, s.conversation)
	return conversation
}

func (s *diagnosisRoomState) closeIfExpired(now time.Time) bool {
	if !now.Before(s.startedAt.Add(s.policy.SessionTTL)) {
		s.close(now, diagnosisRoomCloseSessionTimeout)
		return true
	}
	if !now.Before(s.lastActivityAt.Add(s.policy.IdleTimeout)) {
		s.close(now, diagnosisRoomCloseIdleTimeout)
		return true
	}
	return false
}

func (s *diagnosisRoomState) close(now time.Time, reason string) {
	if s.status == diagnosisRoomStatusClosed {
		return
	}
	s.status = diagnosisRoomStatusClosed
	closedAt := now
	s.closedAt = &closedAt
	s.closeReason = reasonOrDefault(reason, diagnosisRoomCloseUserRequested)
	s.lastActivityAt = now
}

func (s *diagnosisRoomState) nextTimerDelay(now time.Time) time.Duration {
	sessionDelay := s.startedAt.Add(s.policy.SessionTTL).Sub(now)
	idleDelay := s.lastActivityAt.Add(s.policy.IdleTimeout).Sub(now)
	if sessionDelay <= 0 || idleDelay <= 0 {
		return time.Nanosecond
	}
	if sessionDelay < idleDelay {
		return sessionDelay
	}
	return idleDelay
}

func diagnosisRoomPolicyOrDefault(policy diagnosisroom.Policy) diagnosisroom.Policy {
	if policy.MaxTurns == 0 &&
		policy.SessionTTL == 0 &&
		policy.IdleTimeout == 0 &&
		policy.TurnTimeout == 0 &&
		policy.ContextBytes == 0 &&
		policy.MaxMessageBytes == 0 &&
		len(policy.UnsafeDenylist) == 0 {
		return diagnosisroom.DefaultPolicy()
	}
	return policy
}

func diagnosisRoomTurnActivityOptions(policy diagnosisroom.Policy) workflow.ActivityOptions {
	timeout := policy.TurnTimeout
	if timeout <= 0 {
		timeout = diagnosisroom.DefaultTurnTimeout
	}
	return workflow.ActivityOptions{
		StartToCloseTimeout: timeout,
		RetryPolicy: &temporalsdk.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
			NonRetryableErrorTypes: []string{
				errTypeInvalidInput,
				errTypeInvariantViolation,
			},
		},
	}
}

func diagnosisRoomPersistenceActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporalsdk.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
			NonRetryableErrorTypes: []string{
				errTypeInvalidInput,
				errTypeInvariantViolation,
			},
		},
	}
}

func validateDiagnosisRoomWorkflowInput(input DiagnosisRoomWorkflowInput, policy diagnosisroom.Policy) error {
	if strings.TrimSpace(input.SessionID) == "" {
		return fmt.Errorf("diagnosis-room: input.session_id must be non-empty")
	}
	if input.SessionID != strings.TrimSpace(input.SessionID) {
		return fmt.Errorf("diagnosis-room: input.session_id must not contain leading or trailing whitespace")
	}
	if input.DiagnosisTaskID == 0 {
		if input.EvidenceSnapshotID == 0 {
			return fmt.Errorf("diagnosis-room: input.evidence_snapshot_id must be non-zero when diagnosis_task_id is absent")
		}
	} else if input.EvidenceSnapshotID < 0 {
		return fmt.Errorf("diagnosis-room: input.evidence_snapshot_id must be non-negative")
	}
	if strings.TrimSpace(input.OwnerSubject) == "" {
		return fmt.Errorf("diagnosis-room: input.owner_subject must be non-empty")
	}
	if err := validateDiagnosisRoomEvidenceJSON("diagnosis-room: input.evidence", input.Evidence); err != nil {
		return err
	}
	if err := diagnosisroom.ValidatePolicy(policy); err != nil {
		return err
	}
	return nil
}

func reasonOrDefault(reason, fallback string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fallback
	}
	return reason
}
