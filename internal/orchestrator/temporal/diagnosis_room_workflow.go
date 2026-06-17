package temporal

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
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

	diagnosisRoomEvidenceCollectionChangeID = "diagnosis-room-evidence-collection"
	diagnosisRoomEvidenceCollectionVersion  = 1
	diagnosisRoomEvidenceContextChangeID    = "diagnosis-room-evidence-context"
	diagnosisRoomEvidenceContextVersion     = 1
	diagnosisRoomFinalConclusionChangeID    = "diagnosis-room-final-conclusion"
	diagnosisRoomFinalConclusionVersion     = 1
	diagnosisRoomAutoEvidenceChangeID       = "diagnosis-room-auto-evidence-followup"
	diagnosisRoomAutoEvidenceVersion        = 1

	diagnosisRoomAutoActorSubject = "openclarion:auto-diagnosis"
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
	MessageID            string
	ActorSubject         string
	Message              string
	SupplementalEvidence *DiagnosisRoomSupplementalEvidence
}

// DiagnosisRoomSupplementalEvidence captures operator-provided context that
// should be retained as structured diagnosis provenance.
type DiagnosisRoomSupplementalEvidence struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
	Evidence string `json:"evidence"`
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
	EvidenceRequests    []diagnosisroom.EvidenceRequest
	CollectionResults   []diagnosisevidence.Item
	Insight             diagnosisroom.ConsultationInsight
	FollowUpTurns       []DiagnosisRoomFollowUpTurnResult
}

// DiagnosisRoomFollowUpTurnResult describes one workflow-triggered diagnosis
// turn that ran after collecting evidence for the operator-submitted turn.
type DiagnosisRoomFollowUpTurnResult struct {
	MessageID           string
	UserMessage         string
	AssistantMessageID  string
	UserTurnID          int64
	AssistantTurnID     int64
	UserSequence        int
	AssistantSequence   int
	TurnCount           int
	ContextBytes        int
	AssistantMessage    string
	RequiresHumanReview bool
	Confidence          string
	EvidenceRequests    []diagnosisroom.EvidenceRequest
	CollectionResults   []diagnosisevidence.Item
	Insight             diagnosisroom.ConsultationInsight
	Trigger             string
}

// DiagnosisRoomCloseRequest carries the close/cancel signal reason.
type DiagnosisRoomCloseRequest struct {
	Reason       string
	ActorSubject string
}

// DiagnosisRoomWorkflowState is the read model returned by the state query
// and by workflow completion.
type DiagnosisRoomWorkflowState struct {
	SessionID                 string
	ChatSessionID             int64
	DiagnosisTaskID           int64
	OwnerSubject              string
	Status                    string
	TurnCount                 int
	StartedAt                 time.Time
	LastActivityAt            time.Time
	ClosedAt                  *time.Time
	CloseReason               string
	FinalConclusion           *DiagnosisRoomFinalConclusion
	LatestInsight             *diagnosisroom.ConsultationInsight
	LatestConfidence          string
	LatestRequiresHumanReview *bool
	InFlight                  bool
	SeenMessageIDs            []string
	Conversation              []diagnosisroom.ConversationTurn
}

// DiagnosisRoomWorkflowResult is the terminal room state.
type DiagnosisRoomWorkflowResult = DiagnosisRoomWorkflowState

type diagnosisRoomState struct {
	input                     DiagnosisRoomWorkflowInput
	policy                    diagnosisroom.Policy
	status                    string
	startedAt                 time.Time
	lastActivityAt            time.Time
	closedAt                  *time.Time
	closeReason               string
	closeActorSubject         string
	finalConclusion           *DiagnosisRoomFinalConclusion
	latestInsight             *diagnosisroom.ConsultationInsight
	latestConfidence          string
	latestRequiresHumanReview *bool
	turnCount                 int
	diagnosisTaskID           int64
	chatSessionID             int64
	inFlight                  bool
	seen                      map[string]struct{}
	conversation              []diagnosisroom.ConversationTurn
	evidenceBatches           []diagnosisRoomEvidenceContextBatch
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
	evidenceContextVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomEvidenceContextChangeID,
		workflow.DefaultVersion,
		diagnosisRoomEvidenceContextVersion,
	)
	finalConclusionVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomFinalConclusionChangeID,
		workflow.DefaultVersion,
		diagnosisRoomFinalConclusionVersion,
	)
	autoEvidenceVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomAutoEvidenceChangeID,
		workflow.DefaultVersion,
		diagnosisRoomAutoEvidenceVersion,
	)
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
			decision, turnEvidence, err := state.validateSubmit(ctx, req, evidenceContextVersion)
			if err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}
			state.inFlight = true
			defer func() { state.inFlight = false }()

			result, collectionVersion, err := state.runDiagnosisRoomTurn(
				ctx,
				req,
				decision,
				turnEvidence,
				workflow.DefaultVersion,
				true,
				evidenceContextVersion,
				finalConclusionVersion,
			)
			if err != nil {
				return SubmitDiagnosisTurnResult{}, err
			}

			if autoEvidenceVersion >= diagnosisRoomAutoEvidenceVersion {
				followUps, err := state.runAutoEvidenceFollowUps(
					ctx,
					result,
					evidenceContextVersion,
					collectionVersion,
					finalConclusionVersion,
				)
				if err != nil {
					return SubmitDiagnosisTurnResult{}, err
				}
				result.FollowUpTurns = followUps
			}
			return result, nil
		},
		workflow.UpdateHandlerOptions{
			Validator: func(ctx workflow.Context, req SubmitDiagnosisTurnRequest) error {
				_, _, err := state.validateSubmit(ctx, req, evidenceContextVersion)
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
			state.close(workflow.Now(ctx), reasonOrDefault(req.Reason, diagnosisRoomCloseUserRequested), req.ActorSubject)
		})
		selector.AddReceive(cancelCh, func(c workflow.ReceiveChannel, _ bool) {
			var req DiagnosisRoomCloseRequest
			c.Receive(ctx, &req)
			state.close(workflow.Now(ctx), reasonOrDefault(req.Reason, diagnosisRoomCloseCancelled), req.ActorSubject)
		})

		selector.Select(ctx)
		cancelTimer()
		if timerErr != nil {
			state.close(workflow.Now(ctx), diagnosisRoomCloseContextCanceled, "")
			break
		}
	}

	if err := workflow.Await(ctx, func() bool { return workflow.AllHandlersFinished(ctx) }); err != nil {
		return DiagnosisRoomWorkflowResult{}, err
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
			ConfirmedBy:     state.closeActorSubject,
			TurnCount:       state.turnCount,
			ClosedAt:        closedAt,
			Reason:          state.closeReason,
		}).Get(closeCtx, &closeResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
		state.chatSessionID = closeResult.ChatSessionID
		state.finalConclusion = copyDiagnosisRoomFinalConclusion(closeResult.FinalConclusion)
		var notificationResult SendDiagnosisRoomCloseNotificationResult
		if err := workflow.ExecuteActivity(closeCtx, (*Activities).SendDiagnosisRoomCloseNotification, CloseDiagnosisChatSessionInput{
			SessionID:       state.input.SessionID,
			DiagnosisTaskID: state.diagnosisTaskID,
			OwnerSubject:    state.input.OwnerSubject,
			ConfirmedBy:     state.closeActorSubject,
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

func (s *diagnosisRoomState) validateSubmit(
	ctx workflow.Context,
	req SubmitDiagnosisTurnRequest,
	evidenceContextVersion workflow.Version,
) (diagnosisroom.Decision, json.RawMessage, error) {
	if strings.TrimSpace(req.ActorSubject) == "" {
		return diagnosisroom.Decision{}, nil, fmt.Errorf("diagnosis room turn: actor_subject must be non-empty")
	}
	evidence, err := s.turnEvidence(evidenceContextVersion)
	if err != nil {
		return diagnosisroom.Decision{}, nil, err
	}
	decision, err := diagnosisroom.ValidateSubmitTurn(
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
			Evidence:     evidence,
			Conversation: s.conversation,
		},
	)
	if err != nil {
		return diagnosisroom.Decision{}, nil, err
	}
	return decision, evidence, nil
}

func (s *diagnosisRoomState) turnEvidence(evidenceContextVersion workflow.Version) (json.RawMessage, error) {
	if evidenceContextVersion < diagnosisRoomEvidenceContextVersion {
		return cloneRawMessage(s.input.Evidence), nil
	}
	evidence, err := diagnosisRoomEvidenceContext(s.input.Evidence, s.evidenceBatches)
	if err != nil {
		return nil, diagnosisRoomEvidenceContextError(err)
	}
	return evidence, nil
}

func (s *diagnosisRoomState) snapshot() DiagnosisRoomWorkflowState {
	seen := make([]string, 0, len(s.seen))
	for id := range s.seen {
		seen = append(seen, id)
	}
	sort.Strings(seen)
	return DiagnosisRoomWorkflowState{
		SessionID:                 s.input.SessionID,
		ChatSessionID:             s.chatSessionID,
		DiagnosisTaskID:           s.diagnosisTaskID,
		OwnerSubject:              s.input.OwnerSubject,
		Status:                    s.status,
		TurnCount:                 s.turnCount,
		StartedAt:                 s.startedAt,
		LastActivityAt:            s.lastActivityAt,
		ClosedAt:                  s.closedAt,
		CloseReason:               s.closeReason,
		FinalConclusion:           s.finalConclusion,
		LatestInsight:             copyDiagnosisRoomConsultationInsightPtr(s.latestInsight),
		LatestConfidence:          s.latestConfidence,
		LatestRequiresHumanReview: copyBoolPtr(s.latestRequiresHumanReview),
		InFlight:                  s.inFlight,
		SeenMessageIDs:            seen,
		Conversation:              s.conversationCopy(),
	}
}

func (s *diagnosisRoomState) runDiagnosisRoomTurn(
	ctx workflow.Context,
	req SubmitDiagnosisTurnRequest,
	decision diagnosisroom.Decision,
	turnEvidence json.RawMessage,
	collectionVersion workflow.Version,
	resolveCollectionVersion bool,
	evidenceContextVersion workflow.Version,
	finalConclusionVersion workflow.Version,
) (SubmitDiagnosisTurnResult, workflow.Version, error) {
	userOccurredAt := workflow.Now(ctx)
	priorConversation := s.conversationCopy()
	userSequence := len(s.conversation) + 1
	assistantSequence := userSequence + 1
	messageID := strings.TrimSpace(req.MessageID)
	actorSubject := strings.TrimSpace(req.ActorSubject)
	userMessage := strings.TrimSpace(req.Message)
	activityReq := DiagnosisTurnActivityInput{
		SessionID:         s.input.SessionID,
		DiagnosisTaskID:   s.diagnosisTaskID,
		MessageID:         messageID,
		UserSequence:      userSequence,
		AssistantSequence: assistantSequence,
		ActorSubject:      actorSubject,
		Evidence:          turnEvidence,
		Conversation:      priorConversation,
		Message:           req.Message,
		Policy:            s.policy,
	}
	actCtx := workflow.WithActivityOptions(ctx, diagnosisRoomTurnActivityOptions(s.policy))
	var activityResult DiagnosisTurnActivityResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).RunDiagnosisTurn, activityReq).Get(ctx, &activityResult); err != nil {
		return SubmitDiagnosisTurnResult{}, collectionVersion, err
	}
	assistantOccurredAt := workflow.Now(ctx)
	persistReq := PersistDiagnosisTurnInput{
		SessionID:            s.input.SessionID,
		DiagnosisTaskID:      s.diagnosisTaskID,
		OwnerSubject:         s.input.OwnerSubject,
		UserMessageID:        messageID,
		AssistantMessageID:   activityResult.AssistantMessageID,
		UserSequence:         userSequence,
		AssistantSequence:    activityResult.AssistantSequence,
		TurnCount:            s.turnCount + 1,
		ActorSubject:         actorSubject,
		UserMessage:          req.Message,
		AssistantMessage:     activityResult.AssistantMessage,
		UserOccurredAt:       userOccurredAt,
		AssistantOccurredAt:  assistantOccurredAt,
		ContextBytes:         decision.ContextBytes,
		InvocationID:         activityResult.InvocationID,
		RuntimeID:            activityResult.RuntimeID,
		ContainerStartedAt:   activityResult.StartedAt,
		ContainerFinishedAt:  activityResult.FinishedAt,
		RawOutput:            activityResult.RawOutput,
		SupplementalEvidence: copyDiagnosisRoomSupplementalEvidence(req.SupplementalEvidence),
	}
	var persistResult PersistDiagnosisTurnResult
	persistCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
	if err := workflow.ExecuteActivity(persistCtx, (*Activities).PersistDiagnosisTurn, persistReq).Get(ctx, &persistResult); err != nil {
		return SubmitDiagnosisTurnResult{}, collectionVersion, err
	}
	if resolveCollectionVersion {
		collectionVersion = workflow.GetVersion(
			ctx,
			diagnosisRoomEvidenceCollectionChangeID,
			workflow.DefaultVersion,
			diagnosisRoomEvidenceCollectionVersion,
		)
	}
	var collectionResult CollectDiagnosisEvidenceResult
	if collectionVersion >= diagnosisRoomEvidenceCollectionVersion && len(persistResult.EvidenceRequests) > 0 {
		collectCtx := workflow.WithActivityOptions(ctx, diagnosisRoomEvidenceActivityOptions())
		if err := workflow.ExecuteActivity(collectCtx, (*Activities).CollectDiagnosisEvidence, CollectDiagnosisEvidenceInput{
			SessionID:       s.input.SessionID,
			DiagnosisTaskID: s.diagnosisTaskID,
			Requests:        cloneEvidenceRequests(persistResult.EvidenceRequests),
		}).Get(ctx, &collectionResult); err != nil {
			return SubmitDiagnosisTurnResult{}, collectionVersion, err
		}
	}
	if evidenceContextVersion >= diagnosisRoomEvidenceContextVersion {
		if batch, ok := diagnosisRoomEvidenceContextBatchFromItems(
			persistResult.TurnCount,
			activityResult.AssistantMessageID,
			collectionResult.Items,
		); ok {
			s.evidenceBatches = appendDiagnosisRoomEvidenceContextBatch(s.evidenceBatches, batch)
		}
	}

	s.chatSessionID = persistResult.ChatSessionID
	s.turnCount++
	s.lastActivityAt = persistResult.LastActivityAt
	if finalConclusionVersion >= diagnosisRoomFinalConclusionVersion && persistResult.FinalConclusion != nil {
		s.finalConclusion = copyDiagnosisRoomFinalConclusion(*persistResult.FinalConclusion)
	}
	s.latestInsight = copyDiagnosisRoomConsultationInsight(persistResult.Insight)
	s.latestConfidence = activityResult.Confidence
	s.latestRequiresHumanReview = boolPtr(activityResult.RequiresHumanReview)
	s.seen[messageID] = struct{}{}
	s.conversation = append(s.conversation, diagnosisroom.ConversationTurn{
		Role:    string(diagnosisroomRoleUser),
		Content: userMessage,
	})
	s.conversation = append(s.conversation, diagnosisroom.ConversationTurn{
		Role:    string(diagnosisroomRoleAssistant),
		Content: activityResult.AssistantMessage,
	})
	return SubmitDiagnosisTurnResult{
		SessionID:           s.input.SessionID,
		ChatSessionID:       persistResult.ChatSessionID,
		MessageID:           messageID,
		AssistantMessageID:  activityResult.AssistantMessageID,
		UserTurnID:          persistResult.UserTurnID,
		AssistantTurnID:     persistResult.AssistantTurnID,
		UserSequence:        userSequence,
		AssistantSequence:   activityResult.AssistantSequence,
		TurnCount:           s.turnCount,
		ContextBytes:        decision.ContextBytes,
		Status:              s.status,
		AssistantMessage:    activityResult.AssistantMessage,
		RequiresHumanReview: activityResult.RequiresHumanReview,
		Confidence:          activityResult.Confidence,
		EvidenceRequests:    cloneEvidenceRequests(persistResult.EvidenceRequests),
		CollectionResults:   diagnosisevidence.CloneItems(collectionResult.Items),
		Insight:             persistResult.Insight,
	}, collectionVersion, nil
}

func (s *diagnosisRoomState) runAutoEvidenceFollowUps(
	ctx workflow.Context,
	primary SubmitDiagnosisTurnResult,
	evidenceContextVersion workflow.Version,
	collectionVersion workflow.Version,
	finalConclusionVersion workflow.Version,
) ([]DiagnosisRoomFollowUpTurnResult, error) {
	maxFollowUps := s.policy.MaxAutoEvidenceFollowUps
	if maxFollowUps <= 0 {
		return nil, nil
	}
	followUps := make([]DiagnosisRoomFollowUpTurnResult, 0, maxFollowUps)
	previous := primary
	for i := 1; i <= maxFollowUps; i++ {
		if !s.shouldRunAutoEvidenceFollowUp(previous) {
			break
		}
		req := SubmitDiagnosisTurnRequest{
			MessageID:    autoEvidenceFollowUpMessageID(primary.MessageID, i),
			ActorSubject: diagnosisRoomAutoActorSubject,
			Message:      autoEvidenceFollowUpMessage(previous),
		}
		decision, turnEvidence, ok := s.tryValidateSubmitForAutoFollowUp(ctx, req, evidenceContextVersion)
		if !ok {
			break
		}
		result, _, err := s.runDiagnosisRoomTurn(
			ctx,
			req,
			decision,
			turnEvidence,
			collectionVersion,
			false,
			evidenceContextVersion,
			finalConclusionVersion,
		)
		if err != nil {
			return nil, err
		}
		followUps = append(followUps, diagnosisRoomFollowUpTurnResult(result, req.Message))
		previous = result
	}
	return followUps, nil
}

func (s *diagnosisRoomState) tryValidateSubmitForAutoFollowUp(
	ctx workflow.Context,
	req SubmitDiagnosisTurnRequest,
	evidenceContextVersion workflow.Version,
) (diagnosisroom.Decision, json.RawMessage, bool) {
	decision, evidence, err := s.validateSubmitForAutoFollowUp(ctx, req, evidenceContextVersion)
	if err != nil {
		return diagnosisroom.Decision{}, nil, false
	}
	return decision, evidence, true
}

func (s *diagnosisRoomState) shouldRunAutoEvidenceFollowUp(result SubmitDiagnosisTurnResult) bool {
	if s.status != diagnosisRoomStatusOpen || s.policy.MaxAutoEvidenceFollowUps <= 0 || s.turnCount >= s.policy.MaxTurns {
		return false
	}
	switch strings.TrimSpace(result.Insight.ConclusionStatus) {
	case "needs_evidence":
	case "final":
		return false
	default:
		return false
	}
	if len(result.EvidenceRequests) == 0 {
		return false
	}
	for _, item := range result.CollectionResults {
		if item.Status == diagnosisevidence.StatusCollected {
			return true
		}
	}
	return false
}

func (s *diagnosisRoomState) validateSubmitForAutoFollowUp(
	ctx workflow.Context,
	req SubmitDiagnosisTurnRequest,
	evidenceContextVersion workflow.Version,
) (diagnosisroom.Decision, json.RawMessage, error) {
	evidence, err := s.turnEvidence(evidenceContextVersion)
	if err != nil {
		return diagnosisroom.Decision{}, nil, err
	}
	decision, err := diagnosisroom.ValidateSubmitTurn(
		s.policy,
		diagnosisroom.SessionState{
			StartedAt:      s.startedAt,
			LastActivityAt: s.lastActivityAt,
			TurnCount:      s.turnCount,
			InFlight:       false,
			SeenMessageIDs: s.seen,
		},
		diagnosisroom.SubmitTurnRequest{
			MessageID:    req.MessageID,
			Message:      req.Message,
			Now:          workflow.Now(ctx),
			Evidence:     evidence,
			Conversation: s.conversation,
		},
	)
	if err != nil {
		return diagnosisroom.Decision{}, nil, err
	}
	return decision, evidence, nil
}

func diagnosisRoomFollowUpTurnResult(
	result SubmitDiagnosisTurnResult,
	userMessage string,
) DiagnosisRoomFollowUpTurnResult {
	return DiagnosisRoomFollowUpTurnResult{
		MessageID:           result.MessageID,
		UserMessage:         strings.TrimSpace(userMessage),
		AssistantMessageID:  result.AssistantMessageID,
		UserTurnID:          result.UserTurnID,
		AssistantTurnID:     result.AssistantTurnID,
		UserSequence:        result.UserSequence,
		AssistantSequence:   result.AssistantSequence,
		TurnCount:           result.TurnCount,
		ContextBytes:        result.ContextBytes,
		AssistantMessage:    result.AssistantMessage,
		RequiresHumanReview: result.RequiresHumanReview,
		Confidence:          result.Confidence,
		EvidenceRequests:    cloneEvidenceRequests(result.EvidenceRequests),
		CollectionResults:   diagnosisevidence.CloneItems(result.CollectionResults),
		Insight:             result.Insight,
		Trigger:             "collected_evidence",
	}
}

func autoEvidenceFollowUpMessageID(rootMessageID string, index int) string {
	return fmt.Sprintf("%s/auto-evidence-%d", strings.TrimSpace(rootMessageID), index)
}

func autoEvidenceFollowUpMessage(previous SubmitDiagnosisTurnResult) string {
	return fmt.Sprintf(
		"OpenClarion automatic evidence follow-up for %s: use the newly collected evidence in evidence.json to reassess confidence and update the diagnosis. If the evidence is sufficient, set conclusion_status to final or ready_for_review; otherwise explain the remaining missing evidence.",
		strings.TrimSpace(previous.AssistantMessageID),
	)
}

func (s *diagnosisRoomState) conversationCopy() []diagnosisroom.ConversationTurn {
	conversation := make([]diagnosisroom.ConversationTurn, len(s.conversation))
	copy(conversation, s.conversation)
	return conversation
}

func copyDiagnosisRoomFinalConclusion(in DiagnosisRoomFinalConclusion) *DiagnosisRoomFinalConclusion {
	if strings.TrimSpace(in.Status) == "" {
		return nil
	}
	out := in
	out.SupplementalContextRefs = append([]string(nil), in.SupplementalContextRefs...)
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
	return &out
}

func copyDiagnosisRoomConsultationInsight(in diagnosisroom.ConsultationInsight) *diagnosisroom.ConsultationInsight {
	if !diagnosisRoomConsultationInsightHasValue(in) {
		return nil
	}
	out := diagnosisroom.ConsultationInsight{
		ConfidenceRationale:           in.ConfidenceRationale,
		MissingEvidenceRequests:       diagnosisroom.CloneConsultationEvidenceRequests(in.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisroom.CloneConsultationEvidenceRequests(in.EvidenceCollectionSuggestions),
		ConclusionStatus:              in.ConclusionStatus,
	}
	return &out
}

func copyDiagnosisRoomSupplementalEvidence(in *DiagnosisRoomSupplementalEvidence) *DiagnosisRoomSupplementalEvidence {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func copyDiagnosisRoomConsultationInsightPtr(
	in *diagnosisroom.ConsultationInsight,
) *diagnosisroom.ConsultationInsight {
	if in == nil {
		return nil
	}
	return copyDiagnosisRoomConsultationInsight(*in)
}

func diagnosisRoomConsultationInsightHasValue(in diagnosisroom.ConsultationInsight) bool {
	return strings.TrimSpace(in.ConfidenceRationale) != "" ||
		len(in.MissingEvidenceRequests) > 0 ||
		len(in.EvidenceCollectionSuggestions) > 0 ||
		strings.TrimSpace(in.ConclusionStatus) != ""
}

func boolPtr(value bool) *bool {
	out := value
	return &out
}

func copyBoolPtr(in *bool) *bool {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (s *diagnosisRoomState) closeIfExpired(now time.Time) bool {
	if !now.Before(s.startedAt.Add(s.policy.SessionTTL)) {
		s.close(now, diagnosisRoomCloseSessionTimeout, "")
		return true
	}
	if !now.Before(s.lastActivityAt.Add(s.policy.IdleTimeout)) {
		s.close(now, diagnosisRoomCloseIdleTimeout, "")
		return true
	}
	return false
}

func (s *diagnosisRoomState) close(now time.Time, reason string, actorSubject string) {
	if s.status == diagnosisRoomStatusClosed {
		return
	}
	s.status = diagnosisRoomStatusClosed
	closedAt := now
	s.closedAt = &closedAt
	s.closeReason = reasonOrDefault(reason, diagnosisRoomCloseUserRequested)
	s.closeActorSubject = strings.TrimSpace(actorSubject)
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
		policy.MaxAutoEvidenceFollowUps == 0 &&
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

func diagnosisRoomEvidenceActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Second,
		RetryPolicy: &temporalsdk.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    2,
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
