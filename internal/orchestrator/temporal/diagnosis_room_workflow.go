package temporal

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
)

const (
	// DiagnosisRoomSubmitTurnUpdate is the primary M5 user-message path.
	DiagnosisRoomSubmitTurnUpdate = "submit-turn"
	// DiagnosisRoomConfirmConclusionUpdate is the synchronous human confirmation
	// path. The validator rejects confirmation until the assistant marks the
	// room final or ready for review.
	DiagnosisRoomConfirmConclusionUpdate = "confirm-conclusion"
	// DiagnosisRoomCollectEvidenceUpdate executes an operator-selected bounded
	// evidence plan and feeds the collected evidence into an automatic follow-up.
	DiagnosisRoomCollectEvidenceUpdate = "collect-evidence"
	// DiagnosisRoomStateQuery returns the current workflow-visible room state.
	DiagnosisRoomStateQuery = "state"
	// DiagnosisRoomCloseSignal closes a room by user/system request.
	DiagnosisRoomCloseSignal = "close"
	// DiagnosisRoomCancelSignal cancels a room by operator/system request.
	DiagnosisRoomCancelSignal = "cancel"

	errTypeSubmitTurnDuplicateMessage = "DiagnosisRoomSubmitTurnDuplicateMessage"
	errTypeSubmitTurnInFlight         = "DiagnosisRoomSubmitTurnInFlight"
)

const (
	diagnosisRoomStatusOpen   = "open"
	diagnosisRoomStatusClosed = "closed"

	diagnosisRoomCloseUserRequested     = "user_requested"
	diagnosisRoomCloseCancelled         = "cancelled"
	diagnosisRoomCloseSessionTimeout    = "session_timeout"
	diagnosisRoomCloseIdleTimeout       = "idle_timeout"
	diagnosisRoomCloseContextCanceled   = "context_cancelled"
	diagnosisRoomCloseInitialTurnFailed = "initial_turn_failed"

	diagnosisRoomEvidenceCollectionChangeID        = "diagnosis-room-evidence-collection"
	diagnosisRoomEvidenceCollectionVersion         = 1
	diagnosisRoomEvidenceCollectedChangeID         = "diagnosis-room-evidence-collected-event"
	diagnosisRoomEvidenceCollectedVersion          = 1
	diagnosisRoomEvidenceContextChangeID           = "diagnosis-room-evidence-context"
	diagnosisRoomEvidenceContextVersion            = 1
	diagnosisRoomFinalConclusionChangeID           = "diagnosis-room-final-conclusion"
	diagnosisRoomFinalConclusionVersion            = 1
	diagnosisRoomAutoEvidenceChangeID              = "diagnosis-room-auto-evidence-followup"
	diagnosisRoomAutoEvidenceVersion               = 1
	diagnosisRoomFinalReadyNotificationChangeID    = "diagnosis-room-final-ready-notification"
	diagnosisRoomFinalReadyNotificationVersion     = 1
	diagnosisRoomAssistantTurnNotificationChangeID = "diagnosis-room-assistant-turn-notification"
	diagnosisRoomAssistantTurnNotificationVersion  = 1
	diagnosisRoomConfirmEvidenceGuardChangeID      = "diagnosis-room-confirm-evidence-guard"
	diagnosisRoomConfirmEvidenceGuardVersion       = 1
	diagnosisRoomManualEvidenceChangeID            = "diagnosis-room-manual-evidence-collection"
	diagnosisRoomManualEvidenceVersion             = 1
	diagnosisRoomManualEvidenceCollectedChangeID   = "diagnosis-room-manual-evidence-collected-event"
	diagnosisRoomManualEvidenceCollectedVersion    = 1

	diagnosisRoomAutoActorSubject = "openclarion:auto-diagnosis"

	maxDiagnosisRoomManualEvidenceRequests       = 5
	maxDiagnosisRoomConsultationEvidenceRequests = 10
)

// DiagnosisRoomWorkflowInput configures one M5 short-conversation diagnosis
// room. The workflow can start from an existing DiagnosisTask or create the
// task from a frozen EvidenceSnapshot. SessionID is the external room id used
// by WebSocket auth/reconnect flows.
type DiagnosisRoomWorkflowInput struct {
	SessionID                         string
	DiagnosisTaskID                   int64
	EvidenceSnapshotID                int64
	OwnerSubject                      string
	Evidence                          json.RawMessage
	CloseNotificationChannelProfileID int64
	Policy                            diagnosisroom.Policy
	InitialTurn                       *SubmitDiagnosisTurnRequest
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

// CollectDiagnosisEvidenceRequest is the Update payload for an operator-selected
// evidence plan that should be collected before the next AI reassessment.
type CollectDiagnosisEvidenceRequest struct {
	MessageID    string
	ActorSubject string
	Message      string
	Requests     []diagnosisroom.EvidenceRequest
}

// DiagnosisRoomSupplementalEvidenceRecord captures one accepted supplemental
// evidence update together with the turn pair that retained it.
type DiagnosisRoomSupplementalEvidenceRecord struct {
	Label              string
	Detail             string
	Priority           string
	Evidence           string
	ActorSubject       string
	UserMessageID      string
	AssistantMessageID string
	UserTurnID         int64
	AssistantTurnID    int64
	UserSequence       int
	AssistantSequence  int
	ProvidedAt         time.Time
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
	EvidenceTimeline    []DiagnosisRoomEvidenceTimelineEntry
	ConfidenceTimeline  []DiagnosisRoomConfidenceTimelineEntry
	Insight             diagnosisroom.ConsultationInsight
	FollowUpTurns       []DiagnosisRoomFollowUpTurnResult
	LatestError         *DiagnosisRoomLatestError
}

// CollectDiagnosisEvidenceUpdateResult is returned by the manual evidence
// collection Update. It keeps the reconnect state and the immediate
// auto-reassessment turns separate so transports can show the just-finished AI
// loop without inferring it from the full conversation.
type CollectDiagnosisEvidenceUpdateResult struct {
	State         DiagnosisRoomWorkflowState
	FollowUpTurns []DiagnosisRoomFollowUpTurnResult
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

// DiagnosisRoomEvidenceTimelineEntry records one concrete evidence collection
// cycle retained in workflow state for reconnect/read flows.
type DiagnosisRoomEvidenceTimelineEntry struct {
	TurnCount          int
	MessageID          string
	AssistantMessageID string
	ActorSubject       string
	Trigger            string
	EvidenceRequests   []diagnosisroom.EvidenceRequest
	CollectionResults  []diagnosisevidence.Item
}

// DiagnosisRoomConfidenceTimelineEntry records one assistant confidence
// checkpoint for reconnect/read flows.
type DiagnosisRoomConfidenceTimelineEntry struct {
	TurnCount                     int
	MessageID                     string
	AssistantMessageID            string
	AssistantTurnID               int64
	AssistantSequence             int
	OccurredAt                    time.Time
	Trigger                       string
	Confidence                    string
	RequiresHumanReview           bool
	ConclusionStatus              string
	ConfidenceRationale           string
	EvidenceRequests              []diagnosisroom.EvidenceRequest
	CollectionResults             []diagnosisevidence.Item
	MissingEvidenceRequests       []diagnosisroom.ConsultationEvidenceRequest
	EvidenceCollectionSuggestions []diagnosisroom.ConsultationEvidenceRequest
}

// DiagnosisRoomLatestError is the last operator-visible failure retained in
// workflow query state. It intentionally avoids raw activity stderr/details.
type DiagnosisRoomLatestError struct {
	Code       string
	Message    string
	MessageID  string
	OccurredAt time.Time
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
	LatestEvidenceRequests    []diagnosisroom.EvidenceRequest
	LatestCollectionResults   []diagnosisevidence.Item
	EvidenceTimeline          []DiagnosisRoomEvidenceTimelineEntry
	ConfidenceTimeline        []DiagnosisRoomConfidenceTimelineEntry
	SupplementalEvidence      []DiagnosisRoomSupplementalEvidenceRecord
	LatestError               *DiagnosisRoomLatestError
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
	latestEvidenceRequests    []diagnosisroom.EvidenceRequest
	latestCollectionResults   []diagnosisevidence.Item
	evidenceTimeline          []DiagnosisRoomEvidenceTimelineEntry
	confidenceTimeline        []DiagnosisRoomConfidenceTimelineEntry
	supplementalEvidence      []DiagnosisRoomSupplementalEvidenceRecord
	latestError               *DiagnosisRoomLatestError
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
	evidenceCollectedVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomEvidenceCollectedChangeID,
		workflow.DefaultVersion,
		diagnosisRoomEvidenceCollectedVersion,
	)
	finalReadyNotificationVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomFinalReadyNotificationChangeID,
		workflow.DefaultVersion,
		diagnosisRoomFinalReadyNotificationVersion,
	)
	assistantTurnNotificationVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomAssistantTurnNotificationChangeID,
		workflow.DefaultVersion,
		diagnosisRoomAssistantTurnNotificationVersion,
	)
	confirmEvidenceGuardVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomConfirmEvidenceGuardChangeID,
		workflow.DefaultVersion,
		diagnosisRoomConfirmEvidenceGuardVersion,
	)
	manualEvidenceVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomManualEvidenceChangeID,
		workflow.DefaultVersion,
		diagnosisRoomManualEvidenceVersion,
	)
	manualEvidenceCollectedVersion := workflow.GetVersion(
		ctx,
		diagnosisRoomManualEvidenceCollectedChangeID,
		workflow.DefaultVersion,
		diagnosisRoomManualEvidenceCollectedVersion,
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
			return state.submitDiagnosisRoomTurn(
				ctx,
				req,
				evidenceContextVersion,
				finalConclusionVersion,
				autoEvidenceVersion,
				evidenceCollectedVersion,
				finalReadyNotificationVersion,
				assistantTurnNotificationVersion,
			)
		},
		workflow.UpdateHandlerOptions{
			Validator: func(ctx workflow.Context, req SubmitDiagnosisTurnRequest) error {
				_, _, err := state.validateSubmit(ctx, req, evidenceContextVersion)
				return diagnosisRoomSubmitTurnValidatorError(err)
			},
		},
	); err != nil {
		return DiagnosisRoomWorkflowResult{}, err
	}

	confirmCh := workflow.NewChannel(ctx)
	if err := workflow.SetUpdateHandlerWithOptions(
		ctx,
		DiagnosisRoomConfirmConclusionUpdate,
		func(ctx workflow.Context, req DiagnosisRoomCloseRequest) (DiagnosisRoomWorkflowState, error) {
			if err := workflow.Await(ctx, func() bool { return startupComplete }); err != nil {
				return DiagnosisRoomWorkflowState{}, err
			}
			if err := state.validateConfirmConclusion(req, confirmEvidenceGuardVersion); err != nil {
				return DiagnosisRoomWorkflowState{}, newDiagnosisRoomConfirmRejectedError(err)
			}
			confirmCh.Send(ctx, req)
			return state.snapshot(), nil
		},
		workflow.UpdateHandlerOptions{
			Validator: func(_ workflow.Context, req DiagnosisRoomCloseRequest) error {
				return newDiagnosisRoomConfirmRejectedError(
					state.validateConfirmConclusion(req, confirmEvidenceGuardVersion),
				)
			},
		},
	); err != nil {
		return DiagnosisRoomWorkflowResult{}, err
	}

	if manualEvidenceVersion >= diagnosisRoomManualEvidenceVersion {
		if err := workflow.SetUpdateHandlerWithOptions(
			ctx,
			DiagnosisRoomCollectEvidenceUpdate,
			func(ctx workflow.Context, req CollectDiagnosisEvidenceRequest) (CollectDiagnosisEvidenceUpdateResult, error) {
				if err := workflow.Await(ctx, func() bool { return startupComplete }); err != nil {
					return CollectDiagnosisEvidenceUpdateResult{}, err
				}
				return state.collectDiagnosisEvidence(
					ctx,
					req,
					evidenceContextVersion,
					finalConclusionVersion,
					evidenceCollectedVersion,
					manualEvidenceCollectedVersion,
					finalReadyNotificationVersion,
					assistantTurnNotificationVersion,
				)
			},
			workflow.UpdateHandlerOptions{
				Validator: func(_ workflow.Context, req CollectDiagnosisEvidenceRequest) error {
					return state.validateCollectEvidence(req)
				},
			},
		); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
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

	if state.input.InitialTurn != nil {
		if _, err := state.submitDiagnosisRoomTurn(
			ctx,
			*state.input.InitialTurn,
			evidenceContextVersion,
			finalConclusionVersion,
			autoEvidenceVersion,
			evidenceCollectedVersion,
			finalReadyNotificationVersion,
			assistantTurnNotificationVersion,
		); err != nil {
			state.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), state.input.InitialTurn.MessageID, err)
			state.close(workflow.Now(ctx), diagnosisRoomCloseInitialTurnFailed, diagnosisRoomAutoActorSubject)
		}
	}

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
		selector.AddReceive(confirmCh, func(c workflow.ReceiveChannel, _ bool) {
			var req DiagnosisRoomCloseRequest
			c.Receive(ctx, &req)
			state.close(workflow.Now(ctx), reasonOrDefault(req.Reason, "human_confirmed"), req.ActorSubject)
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
			SessionID:                         state.input.SessionID,
			DiagnosisTaskID:                   state.diagnosisTaskID,
			OwnerSubject:                      state.input.OwnerSubject,
			ConfirmedBy:                       state.closeActorSubject,
			TurnCount:                         state.turnCount,
			ClosedAt:                          closedAt,
			Reason:                            state.closeReason,
			CloseNotificationChannelProfileID: state.input.CloseNotificationChannelProfileID,
			DiagnosisTaskStatus:               state.closeDiagnosisTaskStatus(),
			DiagnosisTaskFailureReason:        state.closeDiagnosisTaskFailureReason(),
		}).Get(closeCtx, &closeResult); err != nil {
			return DiagnosisRoomWorkflowResult{}, err
		}
		state.chatSessionID = closeResult.ChatSessionID
		state.finalConclusion = copyDiagnosisRoomFinalConclusion(closeResult.FinalConclusion)
		if state.input.CloseNotificationChannelProfileID > 0 {
			var notificationResult SendDiagnosisRoomCloseNotificationResult
			if err := workflow.ExecuteActivity(closeCtx, (*Activities).SendDiagnosisRoomCloseNotification, CloseDiagnosisChatSessionInput{
				SessionID:                         state.input.SessionID,
				DiagnosisTaskID:                   state.diagnosisTaskID,
				OwnerSubject:                      state.input.OwnerSubject,
				ConfirmedBy:                       state.closeActorSubject,
				TurnCount:                         state.turnCount,
				ClosedAt:                          closeResult.ClosedAt,
				Reason:                            state.closeReason,
				CloseNotificationChannelProfileID: state.input.CloseNotificationChannelProfileID,
			}).Get(closeCtx, &notificationResult); err != nil {
				state.latestError = diagnosisRoomLatestErrorFromNotificationFailure(workflow.Now(ctx), "close_notification", err)
			} else if diagnosisRoomNotificationDeliveryFailed(notificationResult.NotificationStatus) {
				state.latestError = diagnosisRoomLatestErrorFromNotificationFailure(
					workflow.Now(ctx),
					"close_notification",
					fmt.Errorf("diagnosis room close notification delivery failed"),
				)
			}
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

func diagnosisRoomSubmitTurnValidatorError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, diagnosisroom.ErrDuplicateMessageID):
		return temporalsdk.NewNonRetryableApplicationError(
			err.Error(),
			errTypeSubmitTurnDuplicateMessage,
			err,
		)
	case errors.Is(err, diagnosisroom.ErrTurnInFlight):
		return temporalsdk.NewNonRetryableApplicationError(
			err.Error(),
			errTypeSubmitTurnInFlight,
			err,
		)
	case errors.Is(err, domain.ErrInvariantViolation):
		return temporalsdk.NewNonRetryableApplicationError(
			err.Error(),
			errTypeInvariantViolation,
			err,
		)
	default:
		return err
	}
}

func (s *diagnosisRoomState) submitDiagnosisRoomTurn(
	ctx workflow.Context,
	req SubmitDiagnosisTurnRequest,
	evidenceContextVersion workflow.Version,
	finalConclusionVersion workflow.Version,
	autoEvidenceVersion workflow.Version,
	evidenceCollectedVersion workflow.Version,
	finalReadyNotificationVersion workflow.Version,
	assistantTurnNotificationVersion workflow.Version,
) (SubmitDiagnosisTurnResult, error) {
	decision, turnEvidence, err := s.validateSubmit(ctx, req, evidenceContextVersion)
	if err != nil {
		return SubmitDiagnosisTurnResult{}, diagnosisRoomSubmitTurnValidatorError(err)
	}
	s.inFlight = true
	s.latestError = nil
	defer func() { s.inFlight = false }()

	result, collectionVersion, err := s.runDiagnosisRoomTurn(
		ctx,
		req,
		decision,
		turnEvidence,
		workflow.DefaultVersion,
		true,
		evidenceContextVersion,
		finalConclusionVersion,
		evidenceCollectedVersion,
		finalReadyNotificationVersion,
		assistantTurnNotificationVersion,
	)
	if err != nil {
		s.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), req.MessageID, err)
		return SubmitDiagnosisTurnResult{}, err
	}

	if autoEvidenceVersion >= diagnosisRoomAutoEvidenceVersion {
		followUps, err := s.runAutoEvidenceFollowUps(
			ctx,
			result,
			evidenceContextVersion,
			collectionVersion,
			finalConclusionVersion,
			evidenceCollectedVersion,
			finalReadyNotificationVersion,
			assistantTurnNotificationVersion,
		)
		if err != nil {
			s.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), result.MessageID, err)
			return SubmitDiagnosisTurnResult{}, err
		}
		result.FollowUpTurns = followUps
	}
	return result, nil
}

func (s *diagnosisRoomState) collectDiagnosisEvidence(
	ctx workflow.Context,
	req CollectDiagnosisEvidenceRequest,
	evidenceContextVersion workflow.Version,
	finalConclusionVersion workflow.Version,
	evidenceCollectedVersion workflow.Version,
	manualEvidenceCollectedVersion workflow.Version,
	finalReadyNotificationVersion workflow.Version,
	assistantTurnNotificationVersion workflow.Version,
) (CollectDiagnosisEvidenceUpdateResult, error) {
	if err := s.validateCollectEvidence(req); err != nil {
		return CollectDiagnosisEvidenceUpdateResult{}, err
	}
	s.inFlight = true
	s.latestError = nil
	defer func() { s.inFlight = false }()

	collectCtx := workflow.WithActivityOptions(ctx, diagnosisRoomEvidenceActivityOptions())
	var collectionResult CollectDiagnosisEvidenceResult
	requests := cloneEvidenceRequests(req.Requests)
	if err := workflow.ExecuteActivity(collectCtx, (*Activities).CollectDiagnosisEvidence, CollectDiagnosisEvidenceInput{
		SessionID:       s.input.SessionID,
		DiagnosisTaskID: s.diagnosisTaskID,
		Requests:        requests,
	}).Get(ctx, &collectionResult); err != nil {
		s.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), req.MessageID, err)
		return CollectDiagnosisEvidenceUpdateResult{}, err
	}
	if evidenceContextVersion >= diagnosisRoomEvidenceContextVersion {
		if batch, ok := diagnosisRoomEvidenceContextBatchFromItems(
			s.turnCount,
			"",
			collectionResult.Items,
		); ok {
			s.evidenceBatches = appendDiagnosisRoomEvidenceContextBatch(s.evidenceBatches, batch)
		}
	}

	actorSubject := strings.TrimSpace(req.ActorSubject)
	messageID := strings.TrimSpace(req.MessageID)
	if manualEvidenceCollectedVersion >= diagnosisRoomManualEvidenceCollectedVersion && len(collectionResult.Items) > 0 {
		recordCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
		var recordResult RecordDiagnosisEvidenceCollectedResult
		if err := workflow.ExecuteActivity(recordCtx, (*Activities).RecordDiagnosisEvidenceCollected, RecordDiagnosisEvidenceCollectedInput{
			SessionID:       s.input.SessionID,
			DiagnosisTaskID: s.diagnosisTaskID,
			ChatSessionID:   s.chatSessionID,
			OwnerSubject:    s.input.OwnerSubject,
			ActorSubject:    actorSubject,
			UserMessageID:   messageID,
			TurnCount:       s.turnCount,
			Items:           diagnosisRoomEvidenceCollectionAuditItems(collectionResult.Items),
			OccurredAt:      diagnosisRoomEvidenceCollectedAt(collectionResult.Items, workflow.Now(ctx)),
		}).Get(ctx, &recordResult); err != nil {
			s.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), req.MessageID, err)
			return CollectDiagnosisEvidenceUpdateResult{}, err
		}
	}
	s.lastActivityAt = workflow.Now(ctx)
	s.recordManualEvidenceCollection(requests, collectionResult.Items)
	s.recordEvidenceTimelineEntry(diagnosisRoomEvidenceTimelineEntry{
		turnCount:         s.turnCount,
		messageID:         messageID,
		actorSubject:      actorSubject,
		trigger:           "manual_evidence_collection",
		evidenceRequests:  requests,
		collectionResults: collectionResult.Items,
	})
	s.seen[messageID] = struct{}{}

	var followUps []DiagnosisRoomFollowUpTurnResult
	if diagnosisRoomManualEvidenceShouldFollowUp(collectionResult.Items) {
		primary := SubmitDiagnosisTurnResult{
			SessionID:          s.input.SessionID,
			ChatSessionID:      s.chatSessionID,
			MessageID:          messageID,
			AssistantMessageID: messageID,
			TurnCount:          s.turnCount,
			Status:             s.status,
			EvidenceRequests:   requests,
			CollectionResults:  diagnosisevidence.CloneItems(collectionResult.Items),
			Insight: diagnosisroom.ConsultationInsight{
				ConclusionStatus: "needs_evidence",
			},
		}
		var err error
		followUps, err = s.runAutoEvidenceFollowUps(
			ctx,
			primary,
			evidenceContextVersion,
			diagnosisRoomEvidenceCollectionVersion,
			finalConclusionVersion,
			evidenceCollectedVersion,
			finalReadyNotificationVersion,
			assistantTurnNotificationVersion,
		)
		if err != nil {
			s.latestError = diagnosisRoomLatestErrorFromTurnFailure(workflow.Now(ctx), req.MessageID, err)
			return CollectDiagnosisEvidenceUpdateResult{}, err
		}
	}

	s.inFlight = false
	return CollectDiagnosisEvidenceUpdateResult{
		State:         s.snapshot(),
		FollowUpTurns: followUps,
	}, nil
}

func diagnosisRoomManualEvidenceShouldFollowUp(items []diagnosisevidence.Item) bool {
	return len(items) > 0
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
		LatestEvidenceRequests:    cloneEvidenceRequests(s.latestEvidenceRequests),
		LatestCollectionResults:   diagnosisevidence.CloneItems(s.latestCollectionResults),
		EvidenceTimeline:          cloneDiagnosisRoomEvidenceTimeline(s.evidenceTimeline),
		ConfidenceTimeline:        cloneDiagnosisRoomConfidenceTimeline(s.confidenceTimeline),
		SupplementalEvidence:      cloneDiagnosisRoomSupplementalEvidenceRecords(s.supplementalEvidence),
		LatestError:               cloneDiagnosisRoomLatestErrorPtr(s.latestError),
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
	evidenceCollectedVersion workflow.Version,
	finalReadyNotificationVersion workflow.Version,
	assistantTurnNotificationVersion workflow.Version,
) (SubmitDiagnosisTurnResult, workflow.Version, error) {
	userOccurredAt := workflow.Now(ctx)
	priorConversation := s.conversationCopy()
	userSequence := len(s.conversation) + 1
	assistantSequence := userSequence + 1
	messageID := strings.TrimSpace(req.MessageID)
	actorSubject := strings.TrimSpace(req.ActorSubject)
	userMessage := strings.TrimSpace(req.Message)
	activityReq := DiagnosisTurnActivityInput{
		SessionID:            s.input.SessionID,
		DiagnosisTaskID:      s.diagnosisTaskID,
		MessageID:            messageID,
		UserSequence:         userSequence,
		AssistantSequence:    assistantSequence,
		ActorSubject:         actorSubject,
		Evidence:             turnEvidence,
		Conversation:         priorConversation,
		Message:              req.Message,
		SupplementalEvidence: copyDiagnosisRoomSupplementalEvidence(req.SupplementalEvidence),
		Policy:               s.policy,
	}
	actCtx := workflow.WithActivityOptions(ctx, diagnosisRoomTurnActivityOptions(s.policy))
	var activityResult DiagnosisTurnActivityResult
	if err := workflow.ExecuteActivity(actCtx, (*Activities).RunDiagnosisTurn, activityReq).Get(ctx, &activityResult); err != nil {
		return SubmitDiagnosisTurnResult{}, collectionVersion, err
	}
	if normalized, changed := s.preserveUnresolvedMissingEvidenceRequests(req, activityResult.Output); changed {
		rawOutput, err := json.Marshal(normalized)
		if err != nil {
			return SubmitDiagnosisTurnResult{}, collectionVersion, fmt.Errorf("marshal diagnosis turn output: %w", err)
		}
		activityResult.Output = normalized
		activityResult.RawOutput = rawOutput
		activityResult.RequiresHumanReview = normalized.RequiresHumanReview
		activityResult.Confidence = normalized.Confidence
		activityResult.Insight = normalized.Insight()
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
	if collectionVersion >= diagnosisRoomEvidenceCollectionVersion &&
		len(persistResult.EvidenceRequests) > 0 &&
		diagnosisRoomShouldAutoCollectEvidence(actorSubject) {
		collectCtx := workflow.WithActivityOptions(ctx, diagnosisRoomEvidenceActivityOptions())
		if err := workflow.ExecuteActivity(collectCtx, (*Activities).CollectDiagnosisEvidence, CollectDiagnosisEvidenceInput{
			SessionID:       s.input.SessionID,
			DiagnosisTaskID: s.diagnosisTaskID,
			Requests:        cloneEvidenceRequests(persistResult.EvidenceRequests),
		}).Get(ctx, &collectionResult); err != nil {
			return SubmitDiagnosisTurnResult{}, collectionVersion, err
		}
	}
	if evidenceCollectedVersion >= diagnosisRoomEvidenceCollectedVersion && len(collectionResult.Items) > 0 {
		recordCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
		var recordResult RecordDiagnosisEvidenceCollectedResult
		if err := workflow.ExecuteActivity(recordCtx, (*Activities).RecordDiagnosisEvidenceCollected, RecordDiagnosisEvidenceCollectedInput{
			SessionID:          s.input.SessionID,
			DiagnosisTaskID:    s.diagnosisTaskID,
			ChatSessionID:      persistResult.ChatSessionID,
			OwnerSubject:       s.input.OwnerSubject,
			ActorSubject:       actorSubject,
			UserMessageID:      messageID,
			AssistantMessageID: activityResult.AssistantMessageID,
			UserTurnID:         persistResult.UserTurnID,
			AssistantTurnID:    persistResult.AssistantTurnID,
			UserSequence:       userSequence,
			AssistantSequence:  activityResult.AssistantSequence,
			TurnCount:          persistResult.TurnCount,
			Items:              diagnosisRoomEvidenceCollectionAuditItems(collectionResult.Items),
			OccurredAt:         diagnosisRoomEvidenceCollectedAt(collectionResult.Items, workflow.Now(ctx)),
		}).Get(ctx, &recordResult); err != nil {
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
	if finalReadyNotificationVersion >= diagnosisRoomFinalReadyNotificationVersion &&
		persistResult.FinalConclusion != nil &&
		s.input.CloseNotificationChannelProfileID > 0 {
		if err := s.sendFinalReadyNotification(ctx, persistResult, s.input.CloseNotificationChannelProfileID); err != nil {
			s.latestError = diagnosisRoomLatestErrorFromNotificationFailure(workflow.Now(ctx), persistResult.AssistantMessageID, err)
		}
	}
	if assistantTurnNotificationVersion >= diagnosisRoomAssistantTurnNotificationVersion &&
		persistResult.FinalConclusion == nil &&
		s.input.CloseNotificationChannelProfileID > 0 {
		if err := s.sendAssistantTurnNotification(ctx, persistResult, s.input.CloseNotificationChannelProfileID); err != nil {
			s.latestError = diagnosisRoomLatestErrorFromNotificationFailure(workflow.Now(ctx), persistResult.AssistantMessageID, err)
		}
	}
	s.latestInsight = copyDiagnosisRoomConsultationInsight(persistResult.Insight)
	s.latestConfidence = activityResult.Confidence
	s.latestRequiresHumanReview = boolPtr(activityResult.RequiresHumanReview)
	s.recordLatestEvidenceCycle(actorSubject, persistResult.EvidenceRequests, collectionResult.Items)
	s.recordEvidenceTimelineEntry(diagnosisRoomEvidenceTimelineEntry{
		turnCount:          persistResult.TurnCount,
		messageID:          messageID,
		assistantMessageID: activityResult.AssistantMessageID,
		actorSubject:       actorSubject,
		trigger:            diagnosisRoomEvidenceTrigger(actorSubject),
		evidenceRequests:   persistResult.EvidenceRequests,
		collectionResults:  collectionResult.Items,
	})
	s.recordConfidenceTimelineEntry(diagnosisRoomConfidenceTimelineEntry{
		turnCount:                     persistResult.TurnCount,
		messageID:                     messageID,
		assistantMessageID:            activityResult.AssistantMessageID,
		assistantTurnID:               persistResult.AssistantTurnID,
		assistantSequence:             activityResult.AssistantSequence,
		occurredAt:                    persistResult.AssistantOccurredAt,
		trigger:                       diagnosisRoomEvidenceTrigger(actorSubject),
		confidence:                    activityResult.Confidence,
		requiresHumanReview:           activityResult.RequiresHumanReview,
		conclusionStatus:              persistResult.Insight.ConclusionStatus,
		confidenceRationale:           persistResult.Insight.ConfidenceRationale,
		evidenceRequests:              persistResult.EvidenceRequests,
		collectionResults:             collectionResult.Items,
		missingEvidenceRequests:       persistResult.Insight.MissingEvidenceRequests,
		evidenceCollectionSuggestions: persistResult.Insight.EvidenceCollectionSuggestions,
	})
	if req.SupplementalEvidence != nil {
		s.supplementalEvidence = append(s.supplementalEvidence, diagnosisRoomSupplementalEvidenceRecord(
			req.SupplementalEvidence,
			actorSubject,
			messageID,
			activityResult.AssistantMessageID,
			persistResult.UserTurnID,
			persistResult.AssistantTurnID,
			userSequence,
			activityResult.AssistantSequence,
			userOccurredAt,
		))
	}
	s.seen[messageID] = struct{}{}
	s.conversation = append(s.conversation, diagnosisroom.ConversationTurn{
		Role:         string(diagnosisroomRoleUser),
		ActorSubject: actorSubject,
		Content:      userMessage,
	})
	s.conversation = append(s.conversation, diagnosisroom.ConversationTurn{
		Role:         string(diagnosisroomRoleAssistant),
		ActorSubject: diagnosisRoomAutoActorSubject,
		Content:      activityResult.AssistantMessage,
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
		EvidenceTimeline:    cloneDiagnosisRoomEvidenceTimeline(s.evidenceTimeline),
		ConfidenceTimeline:  cloneDiagnosisRoomConfidenceTimeline(s.confidenceTimeline),
		Insight:             persistResult.Insight,
		LatestError:         cloneDiagnosisRoomLatestErrorPtr(s.latestError),
	}, collectionVersion, nil
}

func (s *diagnosisRoomState) sendFinalReadyNotification(
	ctx workflow.Context,
	persistResult PersistDiagnosisTurnResult,
	channelProfileID int64,
) error {
	if persistResult.FinalConclusion == nil {
		return nil
	}
	notifyCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
	var notificationResult SendDiagnosisRoomFinalReadyNotificationResult
	if err := workflow.ExecuteActivity(
		notifyCtx,
		(*Activities).SendDiagnosisRoomFinalReadyNotification,
		SendDiagnosisRoomFinalReadyNotificationInput{
			SessionID:                         s.input.SessionID,
			DiagnosisTaskID:                   s.diagnosisTaskID,
			OwnerSubject:                      s.input.OwnerSubject,
			AssistantTurnID:                   persistResult.AssistantTurnID,
			AssistantMessageID:                persistResult.AssistantMessageID,
			AssistantSequence:                 persistResult.AssistantSequence,
			TurnCount:                         persistResult.TurnCount,
			OccurredAt:                        persistResult.AssistantOccurredAt,
			CloseNotificationChannelProfileID: channelProfileID,
			FinalConclusion:                   *copyDiagnosisRoomFinalConclusion(*persistResult.FinalConclusion),
		},
	).Get(notifyCtx, &notificationResult); err != nil {
		return err
	}
	if diagnosisRoomNotificationDeliveryFailed(notificationResult.NotificationStatus) {
		return fmt.Errorf("diagnosis room final-ready notification delivery failed")
	}
	return nil
}

func (s *diagnosisRoomState) sendAssistantTurnNotification(
	ctx workflow.Context,
	persistResult PersistDiagnosisTurnResult,
	channelProfileID int64,
) error {
	notifyCtx := workflow.WithActivityOptions(ctx, diagnosisRoomPersistenceActivityOptions())
	var notificationResult SendDiagnosisRoomAssistantTurnNotificationResult
	if err := workflow.ExecuteActivity(
		notifyCtx,
		(*Activities).SendDiagnosisRoomAssistantTurnNotification,
		SendDiagnosisRoomAssistantTurnNotificationInput{
			SessionID:                         s.input.SessionID,
			DiagnosisTaskID:                   s.diagnosisTaskID,
			OwnerSubject:                      s.input.OwnerSubject,
			AssistantTurnID:                   persistResult.AssistantTurnID,
			AssistantMessageID:                persistResult.AssistantMessageID,
			AssistantSequence:                 persistResult.AssistantSequence,
			TurnCount:                         persistResult.TurnCount,
			OccurredAt:                        persistResult.AssistantOccurredAt,
			CloseNotificationChannelProfileID: channelProfileID,
			AssistantMessage:                  persistResult.AssistantMessage,
			Confidence:                        persistResult.Confidence,
			RequiresHumanReview:               persistResult.RequiresHumanReview,
			Findings:                          cloneStrings(persistResult.Findings),
			RecommendedActions:                cloneStrings(persistResult.RecommendedActions),
			EvidenceRequests:                  cloneEvidenceRequests(persistResult.EvidenceRequests),
			Insight:                           persistResult.Insight,
		},
	).Get(notifyCtx, &notificationResult); err != nil {
		return err
	}
	if diagnosisRoomNotificationDeliveryFailed(notificationResult.NotificationStatus) {
		return fmt.Errorf("diagnosis room assistant-turn notification delivery failed")
	}
	return nil
}

func diagnosisRoomNotificationDeliveryFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return true
	default:
		return false
	}
}

func (s *diagnosisRoomState) recordLatestEvidenceCycle(
	actorSubject string,
	evidenceRequests []diagnosisroom.EvidenceRequest,
	collectionResults []diagnosisevidence.Item,
) {
	if strings.TrimSpace(actorSubject) == diagnosisRoomAutoActorSubject &&
		len(evidenceRequests) == 0 &&
		len(collectionResults) == 0 &&
		(len(s.latestEvidenceRequests) > 0 || len(s.latestCollectionResults) > 0) {
		return
	}
	s.latestEvidenceRequests = cloneEvidenceRequests(evidenceRequests)
	s.latestCollectionResults = diagnosisevidence.CloneItems(collectionResults)
}

func (s *diagnosisRoomState) recordManualEvidenceCollection(
	evidenceRequests []diagnosisroom.EvidenceRequest,
	collectionResults []diagnosisevidence.Item,
) {
	if len(s.latestEvidenceRequests) == 0 && len(s.latestCollectionResults) == 0 {
		s.latestEvidenceRequests = cloneEvidenceRequests(evidenceRequests)
		s.latestCollectionResults = diagnosisevidence.CloneItems(collectionResults)
		return
	}
	s.latestEvidenceRequests = appendUniqueDiagnosisRoomEvidenceRequests(s.latestEvidenceRequests, evidenceRequests)
	s.latestCollectionResults = mergeDiagnosisRoomCollectionResults(s.latestCollectionResults, collectionResults)
}

func appendUniqueDiagnosisRoomEvidenceRequests(
	base []diagnosisroom.EvidenceRequest,
	additions []diagnosisroom.EvidenceRequest,
) []diagnosisroom.EvidenceRequest {
	out := cloneEvidenceRequests(base)
	seen := make(map[string]struct{}, len(out)+len(additions))
	for _, item := range out {
		seen[diagnosisRoomEvidenceRequestIdentity(item)] = struct{}{}
	}
	for _, item := range additions {
		key := diagnosisRoomEvidenceRequestIdentity(item)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, item)
		seen[key] = struct{}{}
	}
	return out
}

func mergeDiagnosisRoomCollectionResults(
	existing []diagnosisevidence.Item,
	updates []diagnosisevidence.Item,
) []diagnosisevidence.Item {
	if len(existing) == 0 {
		return diagnosisevidence.CloneItems(updates)
	}
	if len(updates) == 0 {
		return diagnosisevidence.CloneItems(existing)
	}
	updateKeys := make(map[string]struct{}, len(updates)*2)
	for _, item := range updates {
		for _, key := range diagnosisRoomEvidenceResultIdentities(item) {
			updateKeys[key] = struct{}{}
		}
	}
	out := make([]diagnosisevidence.Item, 0, len(existing)+len(updates))
	for _, item := range existing {
		replace := false
		for _, key := range diagnosisRoomEvidenceResultIdentities(item) {
			if _, ok := updateKeys[key]; ok {
				replace = true
				break
			}
		}
		if replace {
			continue
		}
		out = append(out, diagnosisevidence.CloneItems([]diagnosisevidence.Item{item})...)
	}
	out = append(out, diagnosisevidence.CloneItems(updates)...)
	return out
}

func diagnosisRoomEvidenceResultIdentities(item diagnosisevidence.Item) []string {
	requestKey := diagnosisRoomEvidenceRequestIdentity(item.Request)
	resolvedKey := diagnosisRoomEvidenceRequestIdentity(diagnosisRoomEvidenceRequestFromCollectionResult(item))
	if resolvedKey == requestKey {
		return []string{requestKey}
	}
	return []string{requestKey, resolvedKey}
}

type diagnosisRoomEvidenceTimelineEntry struct {
	turnCount          int
	messageID          string
	assistantMessageID string
	actorSubject       string
	trigger            string
	evidenceRequests   []diagnosisroom.EvidenceRequest
	collectionResults  []diagnosisevidence.Item
}

type diagnosisRoomConfidenceTimelineEntry struct {
	turnCount                     int
	messageID                     string
	assistantMessageID            string
	assistantTurnID               int64
	assistantSequence             int
	occurredAt                    time.Time
	trigger                       string
	confidence                    string
	requiresHumanReview           bool
	conclusionStatus              string
	confidenceRationale           string
	evidenceRequests              []diagnosisroom.EvidenceRequest
	collectionResults             []diagnosisevidence.Item
	missingEvidenceRequests       []diagnosisroom.ConsultationEvidenceRequest
	evidenceCollectionSuggestions []diagnosisroom.ConsultationEvidenceRequest
}

func (s *diagnosisRoomState) recordEvidenceTimelineEntry(entry diagnosisRoomEvidenceTimelineEntry) {
	if len(entry.evidenceRequests) == 0 && len(entry.collectionResults) == 0 {
		return
	}
	s.evidenceTimeline = append(s.evidenceTimeline, DiagnosisRoomEvidenceTimelineEntry{
		TurnCount:          entry.turnCount,
		MessageID:          strings.TrimSpace(entry.messageID),
		AssistantMessageID: strings.TrimSpace(entry.assistantMessageID),
		ActorSubject:       strings.TrimSpace(entry.actorSubject),
		Trigger:            strings.TrimSpace(entry.trigger),
		EvidenceRequests:   cloneEvidenceRequests(entry.evidenceRequests),
		CollectionResults:  diagnosisevidence.CloneItems(entry.collectionResults),
	})
}

func (s *diagnosisRoomState) recordConfidenceTimelineEntry(entry diagnosisRoomConfidenceTimelineEntry) {
	if strings.TrimSpace(entry.confidence) == "" {
		return
	}
	s.confidenceTimeline = append(s.confidenceTimeline, DiagnosisRoomConfidenceTimelineEntry{
		TurnCount:                     entry.turnCount,
		MessageID:                     strings.TrimSpace(entry.messageID),
		AssistantMessageID:            strings.TrimSpace(entry.assistantMessageID),
		AssistantTurnID:               entry.assistantTurnID,
		AssistantSequence:             entry.assistantSequence,
		OccurredAt:                    entry.occurredAt,
		Trigger:                       strings.TrimSpace(entry.trigger),
		Confidence:                    strings.TrimSpace(entry.confidence),
		RequiresHumanReview:           entry.requiresHumanReview,
		ConclusionStatus:              strings.TrimSpace(entry.conclusionStatus),
		ConfidenceRationale:           strings.TrimSpace(entry.confidenceRationale),
		EvidenceRequests:              cloneEvidenceRequests(entry.evidenceRequests),
		CollectionResults:             diagnosisevidence.CloneItems(entry.collectionResults),
		MissingEvidenceRequests:       diagnosisroom.CloneConsultationEvidenceRequests(entry.missingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisroom.CloneConsultationEvidenceRequests(entry.evidenceCollectionSuggestions),
	})
}

func diagnosisRoomEvidenceTrigger(actorSubject string) string {
	if strings.TrimSpace(actorSubject) == diagnosisRoomAutoActorSubject {
		return "collected_evidence"
	}
	return "operator_turn"
}

func diagnosisRoomShouldAutoCollectEvidence(actorSubject string) bool {
	actor := strings.TrimSpace(actorSubject)
	return actor == diagnosisRoomAutoActorSubject || strings.HasPrefix(actor, "openclarion.")
}

func (s *diagnosisRoomState) validateConfirmConclusion(
	req DiagnosisRoomCloseRequest,
	confirmEvidenceGuardVersion workflow.Version,
) error {
	if strings.TrimSpace(req.ActorSubject) == "" {
		return fmt.Errorf("diagnosis room confirm conclusion: actor_subject must be non-empty")
	}
	if strings.TrimSpace(req.ActorSubject) != req.ActorSubject {
		return fmt.Errorf("diagnosis room confirm conclusion: actor_subject must not contain leading or trailing whitespace")
	}
	if strings.TrimSpace(req.Reason) != req.Reason {
		return fmt.Errorf("diagnosis room confirm conclusion: reason must not contain leading or trailing whitespace")
	}
	if s.status != diagnosisRoomStatusOpen {
		return fmt.Errorf("diagnosis room confirm conclusion: room status %q is not open", s.status)
	}
	if s.inFlight {
		return fmt.Errorf("diagnosis room confirm conclusion: turn is still in progress")
	}
	ready := false
	if s.finalConclusion != nil && strings.TrimSpace(s.finalConclusion.Status) == "available" {
		ready = true
	}
	if s.latestInsight != nil {
		switch strings.TrimSpace(s.latestInsight.ConclusionStatus) {
		case "final", "ready_for_review":
			ready = true
		}
	}
	if !ready {
		return fmt.Errorf("diagnosis room confirm conclusion: latest diagnosis is not final or ready_for_review")
	}
	if confirmEvidenceGuardVersion >= diagnosisRoomConfirmEvidenceGuardVersion {
		if reason := s.confirmEvidenceBlockReason(); reason != "" {
			return fmt.Errorf("diagnosis room confirm conclusion: %s", reason)
		}
	}
	return nil
}

func (s *diagnosisRoomState) validateCollectEvidence(req CollectDiagnosisEvidenceRequest) error {
	if strings.TrimSpace(req.MessageID) == "" {
		return fmt.Errorf("diagnosis room collect evidence: message_id must be non-empty")
	}
	if strings.TrimSpace(req.MessageID) != req.MessageID {
		return fmt.Errorf("diagnosis room collect evidence: message_id must not contain leading or trailing whitespace")
	}
	if strings.TrimSpace(req.ActorSubject) == "" {
		return fmt.Errorf("diagnosis room collect evidence: actor_subject must be non-empty")
	}
	if strings.TrimSpace(req.ActorSubject) != req.ActorSubject {
		return fmt.Errorf("diagnosis room collect evidence: actor_subject must not contain leading or trailing whitespace")
	}
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("diagnosis room collect evidence: message must be non-empty")
	}
	if s.status != diagnosisRoomStatusOpen {
		return fmt.Errorf("diagnosis room collect evidence: room status %q is not open", s.status)
	}
	if s.inFlight {
		return fmt.Errorf("diagnosis room collect evidence: turn is still in progress")
	}
	if s.turnCount <= 0 {
		return fmt.Errorf("diagnosis room collect evidence: at least one diagnosis turn is required before evidence collection")
	}
	if _, seen := s.seen[req.MessageID]; seen {
		return fmt.Errorf("diagnosis room collect evidence: duplicate message_id %q", req.MessageID)
	}
	if len(req.Requests) == 0 || len(req.Requests) > maxDiagnosisRoomManualEvidenceRequests {
		return fmt.Errorf("diagnosis room collect evidence: requests must contain between 1 and %d items", maxDiagnosisRoomManualEvidenceRequests)
	}
	for i, request := range req.Requests {
		if err := validateManualEvidenceRequest(i, request); err != nil {
			return err
		}
	}
	if err := s.validateManualEvidenceRequestsApproved(req.Requests); err != nil {
		return err
	}
	return nil
}

func (s *diagnosisRoomState) validateManualEvidenceRequestsApproved(requests []diagnosisroom.EvidenceRequest) error {
	approved := make(map[string]struct{}, len(s.latestEvidenceRequests)+len(s.latestCollectionResults))
	addApproved := func(items []diagnosisroom.EvidenceRequest) {
		for _, item := range items {
			approved[diagnosisRoomEvidenceRequestIdentity(item)] = struct{}{}
		}
	}
	addRetryable := func(items []diagnosisevidence.Item) {
		for _, item := range items {
			if !diagnosisRoomEvidenceResultRetryable(item) {
				continue
			}
			approved[diagnosisRoomEvidenceRequestIdentity(item.Request)] = struct{}{}
			approved[diagnosisRoomEvidenceRequestIdentity(diagnosisRoomEvidenceRequestFromCollectionResult(item))] = struct{}{}
		}
	}
	addApproved(pendingDiagnosisRoomEvidenceRequests(s.latestEvidenceRequests, s.latestCollectionResults))
	addRetryable(s.latestCollectionResults)
	if s.finalConclusion != nil {
		addApproved(pendingDiagnosisRoomEvidenceRequests(s.finalConclusion.EvidenceRequests, s.latestCollectionResults))
	}
	sourceProfileIDs := s.manualEvidenceAllowedSourceProfileIDs()
	seen := make(map[string]struct{}, len(requests))
	for i, request := range requests {
		key := diagnosisRoomEvidenceRequestIdentity(request)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] duplicates another request", i)
		}
		seen[key] = struct{}{}
		if _, ok := approved[key]; ok {
			delete(approved, key)
			continue
		}
		if diagnosisRoomManualEvidenceRequestMatchesRoomScope(request, sourceProfileIDs) {
			continue
		}
		return fmt.Errorf("diagnosis room collect evidence: requests[%d] must match a pending assistant evidence request or room alert source profile", i)
	}
	return nil
}

func (s *diagnosisRoomState) manualEvidenceAllowedSourceProfileIDs() map[int64]struct{} {
	return diagnosisRoomEvidenceSourceProfileIDs(s.input.Evidence)
}

func diagnosisRoomManualEvidenceRequestMatchesRoomScope(
	request diagnosisroom.EvidenceRequest,
	sourceProfileIDs map[int64]struct{},
) bool {
	if request.AlertSourceProfileID <= 0 {
		return false
	}
	_, ok := sourceProfileIDs[request.AlertSourceProfileID]
	return ok
}

func validateManualEvidenceRequest(index int, req diagnosisroom.EvidenceRequest) error {
	if req.TemplateID < 0 || req.AlertSourceProfileID < 0 {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d] identifiers must be non-negative", index)
	}
	if !req.Tool.Valid() {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].tool is unsupported", index)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].reason must be non-empty", index)
	}
	if strings.TrimSpace(req.Reason) != req.Reason {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].reason must not contain leading or trailing whitespace", index)
	}
	if len([]byte(req.Reason)) > diagnosisroom.EvidenceRequestReasonBytesMaximum() {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].reason exceeds %d bytes", index, diagnosisroom.EvidenceRequestReasonBytesMaximum())
	}
	if diagnosisroom.EvidenceRequestTextHasControlRune(req.Reason) {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].reason must be single-line", index)
	}
	if strings.TrimSpace(req.Query) != req.Query {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].query must not contain leading or trailing whitespace", index)
	}
	if req.Query != "" {
		if len([]byte(req.Query)) > diagnosisroom.EvidenceRequestQueryBytesMaximum() {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d].query exceeds %d bytes", index, diagnosisroom.EvidenceRequestQueryBytesMaximum())
		}
		if diagnosisroom.EvidenceRequestTextHasControlRune(req.Query) {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d].query must be single-line", index)
		}
	}
	if req.Limit < 0 {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].limit must be non-negative", index)
	}
	if err := validateManualEvidenceLimit(index, req); err != nil {
		return err
	}
	switch req.Tool {
	case "active_alerts":
		if req.Query != "" || req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] active_alerts must not include query, window_seconds, or step_seconds", index)
		}
	case "metric_query":
		if req.TemplateID == 0 && strings.TrimSpace(req.Query) == "" {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] metric_query requires query or template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] metric_query must not include window_seconds or step_seconds", index)
		}
	case "metric_range_query":
		if req.TemplateID == 0 && strings.TrimSpace(req.Query) == "" {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] metric_range_query requires query or template_id", index)
		}
		if req.TemplateID == 0 && (req.WindowSeconds <= 0 || req.StepSeconds <= 0) {
			return fmt.Errorf("diagnosis room collect evidence: requests[%d] metric_range_query requires window_seconds and step_seconds without template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			if err := validateManualEvidenceRange(index, req.WindowSeconds, req.StepSeconds); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].tool is unsupported", index)
	}
	return nil
}

func validateManualEvidenceLimit(index int, req diagnosisroom.EvidenceRequest) error {
	if req.Limit == 0 {
		return nil
	}
	maximum, ok := diagnosisroom.EvidenceRequestLimitMaximum(req.Tool)
	if !ok {
		return nil
	}
	if req.Limit > maximum {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].limit must be between 1 and %d", index, maximum)
	}
	return nil
}

func validateManualEvidenceRange(index int, windowSeconds int, stepSeconds int) error {
	minimum, maximum := diagnosisroom.EvidenceRequestRangeSecondsBounds()
	if windowSeconds < minimum || windowSeconds > maximum {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].window_seconds must be between %d and %d", index, minimum, maximum)
	}
	if stepSeconds < minimum || stepSeconds > maximum {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].step_seconds must be between %d and %d", index, minimum, maximum)
	}
	if stepSeconds > windowSeconds {
		return fmt.Errorf("diagnosis room collect evidence: requests[%d].step_seconds must not exceed window_seconds", index)
	}
	return nil
}

func newDiagnosisRoomConfirmRejectedError(err error) error {
	if err == nil {
		return nil
	}
	return temporalsdk.NewNonRetryableApplicationError(err.Error(), errTypeConfirmRejected, nil)
}

func (s *diagnosisRoomState) confirmEvidenceBlockReason() string {
	for _, item := range s.latestCollectionResults {
		switch strings.TrimSpace(string(item.Status)) {
		case string(diagnosisevidence.StatusFailed), string(diagnosisevidence.StatusSkipped), string(diagnosisevidence.StatusUnsupported):
			if s.confirmAllowsReviewedCollectionEvidence(item, s.latestAssistantSequence()) {
				continue
			}
			tool := strings.TrimSpace(string(item.Tool))
			if tool == "" {
				tool = "planned"
			}
			return fmt.Sprintf("resolve %s evidence collection before confirming", tool)
		}
	}
	if s.latestInsight != nil &&
		len(s.latestInsight.MissingEvidenceRequests) > 0 &&
		!s.confirmAllowsReviewedMissingEvidence(
			s.latestInsight.MissingEvidenceRequests,
			s.latestInsight.ConclusionStatus,
			s.latestAssistantSequence(),
		) {
		return "resolve missing evidence requests before confirming"
	}
	if len(pendingDiagnosisRoomEvidenceRequests(s.latestEvidenceRequests, s.latestCollectionResults)) > 0 {
		return "collect planned executable evidence before confirming"
	}
	if s.finalConclusion != nil {
		if len(s.finalConclusion.MissingEvidenceRequests) > 0 &&
			!s.confirmAllowsReviewedMissingEvidence(
				s.finalConclusion.MissingEvidenceRequests,
				"ready_for_review",
				s.finalConclusion.AssistantSequence,
			) {
			return "resolve missing evidence requests before confirming"
		}
		if len(pendingDiagnosisRoomEvidenceRequests(s.finalConclusion.EvidenceRequests, s.latestCollectionResults)) > 0 {
			return "collect planned executable evidence before confirming"
		}
	}
	return ""
}

func (s *diagnosisRoomState) confirmAllowsReviewedMissingEvidence(
	requests []diagnosisroom.ConsultationEvidenceRequest,
	conclusionStatus string,
	assistantSequence int,
) bool {
	if s == nil || len(requests) == 0 || len(s.supplementalEvidence) == 0 {
		return false
	}
	switch strings.TrimSpace(conclusionStatus) {
	case "final", "ready_for_review":
	default:
		return false
	}
	for _, request := range requests {
		if !s.confirmHasReviewedSupplementalEvidence(request, assistantSequence) {
			return false
		}
	}
	return true
}

func (s *diagnosisRoomState) confirmAllowsReviewedCollectionEvidence(
	item diagnosisevidence.Item,
	assistantSequence int,
) bool {
	if s == nil || len(s.supplementalEvidence) == 0 {
		return false
	}
	for _, key := range diagnosisRoomCollectionEvidenceTopicKeys(item) {
		if s.confirmHasReviewedSupplementalEvidenceKey(key, assistantSequence) {
			return true
		}
	}
	return false
}

func (s *diagnosisRoomState) confirmHasReviewedSupplementalEvidence(
	request diagnosisroom.ConsultationEvidenceRequest,
	assistantSequence int,
) bool {
	requestKey := diagnosisRoomConsultationEvidenceRequestKey(
		request.Label,
		request.Detail,
	)
	return s.confirmHasReviewedSupplementalEvidenceKey(requestKey, assistantSequence)
}

func (s *diagnosisRoomState) confirmHasReviewedSupplementalEvidenceKey(
	requestKey string,
	assistantSequence int,
) bool {
	if requestKey == "" {
		return false
	}
	for _, item := range s.supplementalEvidence {
		if diagnosisRoomConsultationEvidenceRequestKey(item.Label, item.Detail) != requestKey {
			continue
		}
		if assistantSequence > 0 && item.AssistantSequence != assistantSequence {
			continue
		}
		return true
	}
	return false
}

func diagnosisRoomCollectionEvidenceTopicKeys(item diagnosisevidence.Item) []string {
	var raw []string
	tool := strings.TrimSpace(string(item.Tool))
	if tool == "" {
		tool = strings.TrimSpace(string(item.Request.Tool))
	}
	if tool != "" {
		raw = append(raw, tool+" evidence collection")
	}
	if reason := strings.TrimSpace(item.Request.Reason); reason != "" {
		raw = append(raw, reason)
	}
	if message := strings.TrimSpace(item.Message); message != "" {
		raw = append(raw, message)
	}
	keys := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		key := diagnosisRoomConsultationEvidenceTopicKey(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
		seen[key] = struct{}{}
	}
	return keys
}

func (s *diagnosisRoomState) latestAssistantSequence() int {
	for i := len(s.confidenceTimeline) - 1; i >= 0; i-- {
		if s.confidenceTimeline[i].AssistantSequence > 0 {
			return s.confidenceTimeline[i].AssistantSequence
		}
	}
	return 0
}

func pendingDiagnosisRoomEvidenceRequests(
	requests []diagnosisroom.EvidenceRequest,
	results []diagnosisevidence.Item,
) []diagnosisroom.EvidenceRequest {
	if len(requests) == 0 {
		return nil
	}
	resultKeys := make(map[string]struct{}, len(results))
	for _, result := range results {
		if !diagnosisRoomEvidenceResultCompletesRequest(result) {
			continue
		}
		resultKeys[diagnosisRoomEvidenceRequestIdentity(result.Request)] = struct{}{}
		resultKeys[diagnosisRoomEvidenceRequestIdentity(diagnosisRoomEvidenceRequestFromCollectionResult(result))] = struct{}{}
	}
	pending := make([]diagnosisroom.EvidenceRequest, 0, len(requests))
	for _, request := range requests {
		if _, ok := resultKeys[diagnosisRoomEvidenceRequestIdentity(request)]; ok {
			continue
		}
		pending = append(pending, request)
	}
	return pending
}

func diagnosisRoomEvidenceResultCompletesRequest(result diagnosisevidence.Item) bool {
	if diagnosisRoomEvidenceResultRetryable(result) {
		return false
	}
	switch result.Status {
	case diagnosisevidence.StatusCollected,
		diagnosisevidence.StatusSkipped,
		diagnosisevidence.StatusUnsupported:
		return true
	default:
		return false
	}
}

func diagnosisRoomEvidenceResultRetryable(result diagnosisevidence.Item) bool {
	switch result.Status {
	case diagnosisevidence.StatusFailed:
		return true
	case diagnosisevidence.StatusSkipped:
		switch result.ReasonCode {
		case diagnosisevidence.ReasonCollectionTimedOut,
			diagnosisevidence.ReasonProviderFailed,
			diagnosisevidence.ReasonProviderUnavailable,
			diagnosisevidence.ReasonSourceUnavailable:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func diagnosisRoomEvidenceRequestFromCollectionResult(item diagnosisevidence.Item) diagnosisroom.EvidenceRequest {
	req := item.Request
	if item.TemplateID > 0 {
		req.TemplateID = int64(item.TemplateID)
	}
	if item.AlertSourceProfileID > 0 {
		req.AlertSourceProfileID = int64(item.AlertSourceProfileID)
	}
	if item.Tool != "" {
		req.Tool = item.Tool
	}
	if item.Query != "" {
		req.Query = item.Query
	}
	if item.WindowSeconds > 0 {
		req.WindowSeconds = item.WindowSeconds
	}
	if item.StepSeconds > 0 {
		req.StepSeconds = item.StepSeconds
	}
	if item.Limit > 0 {
		req.Limit = item.Limit
	}
	return req
}

func diagnosisRoomEvidenceRequestIdentity(request diagnosisroom.EvidenceRequest) string {
	return strings.Join([]string{
		fmt.Sprintf("%d", request.TemplateID),
		fmt.Sprintf("%d", request.AlertSourceProfileID),
		strings.TrimSpace(string(request.Tool)),
		strings.TrimSpace(request.Reason),
		strings.TrimSpace(request.Query),
		fmt.Sprintf("%d", request.WindowSeconds),
		fmt.Sprintf("%d", request.StepSeconds),
		fmt.Sprintf("%d", request.Limit),
	}, "\x00")
}

func diagnosisRoomEvidenceSourceProfileIDs(raw json.RawMessage) map[int64]struct{} {
	ids := make(map[int64]struct{})
	var snapshot struct {
		AlertSourceProfileID int64 `json:"alert_source_profile_id"`
		Events               []struct {
			AlertSourceProfileID int64 `json:"alert_source_profile_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ids
	}
	if snapshot.AlertSourceProfileID > 0 {
		ids[snapshot.AlertSourceProfileID] = struct{}{}
	}
	for _, event := range snapshot.Events {
		if event.AlertSourceProfileID > 0 {
			ids[event.AlertSourceProfileID] = struct{}{}
		}
	}
	return ids
}

func (s *diagnosisRoomState) runAutoEvidenceFollowUps(
	ctx workflow.Context,
	primary SubmitDiagnosisTurnResult,
	evidenceContextVersion workflow.Version,
	collectionVersion workflow.Version,
	finalConclusionVersion workflow.Version,
	evidenceCollectedVersion workflow.Version,
	finalReadyNotificationVersion workflow.Version,
	assistantTurnNotificationVersion workflow.Version,
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
			evidenceCollectedVersion,
			finalReadyNotificationVersion,
			assistantTurnNotificationVersion,
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
	case "", "investigating", "needs_evidence":
	case "final", "ready_for_review":
		return false
	default:
		return false
	}
	if len(result.EvidenceRequests) == 0 {
		return false
	}
	for _, item := range result.CollectionResults {
		switch item.Status {
		case diagnosisevidence.StatusCollected,
			diagnosisevidence.StatusFailed,
			diagnosisevidence.StatusSkipped,
			diagnosisevidence.StatusUnsupported:
			return true
		}
	}
	return false
}

func (s *diagnosisRoomState) preserveUnresolvedMissingEvidenceRequests(
	req SubmitDiagnosisTurnRequest,
	output diagnosisroom.TurnOutput,
) (diagnosisroom.TurnOutput, bool) {
	current := diagnosisroom.CloneConsultationEvidenceRequests(output.MissingEvidenceRequests)
	merged := diagnosisroom.CloneConsultationEvidenceRequests(current)
	if len(current) < maxDiagnosisRoomConsultationEvidenceRequests && len(s.confidenceTimeline) > 0 {
		resolved := s.resolvedSupplementalEvidenceTopicKeys(req.SupplementalEvidence)
		seen := make(map[string]struct{}, len(current))
		for _, item := range current {
			if key := diagnosisRoomConsultationEvidenceRequestKey(item.Label, item.Detail); key != "" {
				seen[key] = struct{}{}
			}
		}
		for i := len(s.confidenceTimeline) - 1; i >= 0; i-- {
			for _, item := range s.confidenceTimeline[i].MissingEvidenceRequests {
				if len(merged) >= maxDiagnosisRoomConsultationEvidenceRequests {
					break
				}
				key := diagnosisRoomConsultationEvidenceRequestKey(item.Label, item.Detail)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				if _, ok := resolved[key]; ok {
					continue
				}
				merged = append(merged, item)
				seen[key] = struct{}{}
			}
		}
	}
	changed := false
	if !consultationEvidenceRequestsEqual(current, merged) {
		output.MissingEvidenceRequests = merged
		changed = true
	}
	if len(output.MissingEvidenceRequests) == 0 {
		return output, changed
	}
	if strings.TrimSpace(output.ConclusionStatus) == "final" {
		output.ConclusionStatus = "needs_evidence"
		changed = true
	}
	if !output.RequiresHumanReview {
		output.RequiresHumanReview = true
		changed = true
	}
	if strings.TrimSpace(output.ConfidenceRationale) == "" {
		output.ConfidenceRationale = "Unresolved operator evidence remains before final confirmation."
		changed = true
	}
	return output, changed
}

func (s *diagnosisRoomState) resolvedSupplementalEvidenceTopicKeys(
	current *DiagnosisRoomSupplementalEvidence,
) map[string]struct{} {
	resolved := make(map[string]struct{}, len(s.supplementalEvidence)+1)
	for _, item := range s.supplementalEvidence {
		if key := diagnosisRoomConsultationEvidenceRequestKey(item.Label, item.Detail); key != "" {
			resolved[key] = struct{}{}
		}
	}
	if current != nil {
		if key := diagnosisRoomConsultationEvidenceRequestKey(current.Label, current.Detail); key != "" {
			resolved[key] = struct{}{}
		}
	}
	return resolved
}

func consultationEvidenceRequestsEqual(
	left []diagnosisroom.ConsultationEvidenceRequest,
	right []diagnosisroom.ConsultationEvidenceRequest,
) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func diagnosisRoomConsultationEvidenceTopicKey(label string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(label))), " ")
}

func diagnosisRoomConsultationEvidenceRequestKey(label, detail string) string {
	labelKey := diagnosisRoomConsultationEvidenceTopicKey(label)
	detailKey := diagnosisRoomConsultationEvidenceTopicKey(detail)
	if labelKey == "" {
		return ""
	}
	if detailKey == "" {
		return labelKey
	}
	return labelKey + "\x00" + detailKey
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
		"OpenClarion automatic evidence follow-up for %s: use the newly collected or attempted evidence in evidence.json to reassess confidence and update the diagnosis. Treat failed, skipped, or unsupported collection results as unresolved evidence gaps unless other verified evidence resolves them. Preserve any previous missing_evidence_requests unless matching operator supplemental evidence is present; executable evidence alone must not clear human or operator evidence gaps. If unresolved missing evidence remains, keep conclusion_status out of final and explain the remaining missing evidence. When the operator states that a non-executable artifact is unavailable and accepts the residual uncertainty, stop repeating that same request and use ready_for_review with requires_human_review=true if no additional executable evidence is needed.",
		strings.TrimSpace(previous.AssistantMessageID),
	)
}

func (s *diagnosisRoomState) conversationCopy() []diagnosisroom.ConversationTurn {
	conversation := make([]diagnosisroom.ConversationTurn, len(s.conversation))
	copy(conversation, s.conversation)
	return conversation
}

func diagnosisRoomLatestErrorFromTurnFailure(now time.Time, messageID string, err error) *DiagnosisRoomLatestError {
	code := "turn_failed"
	message := "Diagnosis turn failed before an assistant response; check workflow logs for runner details."
	errText := strings.ToLower(err.Error())
	switch {
	case strings.Contains(errText, "context deadline exceeded") || strings.Contains(errText, "deadline exceeded"):
		code = "llm_timeout"
		message = "Diagnosis turn failed before an assistant response; upstream LLM request timed out."
	case strings.Contains(errText, "llm"):
		code = "llm_failed"
		message = "Diagnosis turn failed before an assistant response; upstream LLM request failed."
	}
	return &DiagnosisRoomLatestError{
		Code:       code,
		Message:    message,
		MessageID:  strings.TrimSpace(messageID),
		OccurredAt: now,
	}
}

func diagnosisRoomLatestErrorFromNotificationFailure(now time.Time, messageID string, err error) *DiagnosisRoomLatestError {
	code := "notification_failed"
	message := "AI diagnosis was saved, but downstream diagnosis notification delivery failed; review notification channel configuration."
	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "context deadline exceeded") || strings.Contains(errText, "deadline exceeded") {
		message = "AI diagnosis was saved, but downstream diagnosis notification delivery timed out; review notification channel configuration."
	}
	return &DiagnosisRoomLatestError{
		Code:       code,
		Message:    message,
		MessageID:  strings.TrimSpace(messageID),
		OccurredAt: now,
	}
}

func cloneDiagnosisRoomLatestErrorPtr(in *DiagnosisRoomLatestError) *DiagnosisRoomLatestError {
	if in == nil || strings.TrimSpace(in.Code) == "" {
		return nil
	}
	out := *in
	return &out
}

func copyDiagnosisRoomFinalConclusion(in DiagnosisRoomFinalConclusion) *DiagnosisRoomFinalConclusion {
	if strings.TrimSpace(in.Status) == "" {
		return nil
	}
	out := in
	out.SupplementalContextRefs = append([]string(nil), in.SupplementalContextRefs...)
	out.Findings = append([]string(nil), in.Findings...)
	out.RecommendedActions = append([]string(nil), in.RecommendedActions...)
	out.EvidenceRequests = cloneEvidenceRequests(in.EvidenceRequests)
	out.MissingEvidenceRequests = diagnosisroom.CloneConsultationEvidenceRequests(in.MissingEvidenceRequests)
	out.EvidenceCollectionSuggestions = diagnosisroom.CloneConsultationEvidenceRequests(in.EvidenceCollectionSuggestions)
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

func diagnosisRoomSupplementalEvidenceRecord(
	in *DiagnosisRoomSupplementalEvidence,
	actorSubject string,
	userMessageID string,
	assistantMessageID string,
	userTurnID int64,
	assistantTurnID int64,
	userSequence int,
	assistantSequence int,
	providedAt time.Time,
) DiagnosisRoomSupplementalEvidenceRecord {
	if in == nil {
		return DiagnosisRoomSupplementalEvidenceRecord{}
	}
	return DiagnosisRoomSupplementalEvidenceRecord{
		Label:              in.Label,
		Detail:             in.Detail,
		Priority:           in.Priority,
		Evidence:           in.Evidence,
		ActorSubject:       actorSubject,
		UserMessageID:      userMessageID,
		AssistantMessageID: assistantMessageID,
		UserTurnID:         userTurnID,
		AssistantTurnID:    assistantTurnID,
		UserSequence:       userSequence,
		AssistantSequence:  assistantSequence,
		ProvidedAt:         providedAt,
	}
}

func cloneDiagnosisRoomSupplementalEvidenceRecords(
	in []DiagnosisRoomSupplementalEvidenceRecord,
) []DiagnosisRoomSupplementalEvidenceRecord {
	if in == nil {
		return nil
	}
	out := make([]DiagnosisRoomSupplementalEvidenceRecord, len(in))
	copy(out, in)
	return out
}

func cloneDiagnosisRoomEvidenceTimeline(
	in []DiagnosisRoomEvidenceTimelineEntry,
) []DiagnosisRoomEvidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]DiagnosisRoomEvidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = DiagnosisRoomEvidenceTimelineEntry{
			TurnCount:          item.TurnCount,
			MessageID:          item.MessageID,
			AssistantMessageID: item.AssistantMessageID,
			ActorSubject:       item.ActorSubject,
			Trigger:            item.Trigger,
			EvidenceRequests:   cloneEvidenceRequests(item.EvidenceRequests),
			CollectionResults:  diagnosisevidence.CloneItems(item.CollectionResults),
		}
	}
	return out
}

func cloneDiagnosisRoomConfidenceTimeline(
	in []DiagnosisRoomConfidenceTimelineEntry,
) []DiagnosisRoomConfidenceTimelineEntry {
	if in == nil {
		return nil
	}
	out := make([]DiagnosisRoomConfidenceTimelineEntry, len(in))
	for i, item := range in {
		out[i] = DiagnosisRoomConfidenceTimelineEntry{
			TurnCount:                     item.TurnCount,
			MessageID:                     item.MessageID,
			AssistantMessageID:            item.AssistantMessageID,
			AssistantTurnID:               item.AssistantTurnID,
			AssistantSequence:             item.AssistantSequence,
			OccurredAt:                    item.OccurredAt,
			Trigger:                       item.Trigger,
			Confidence:                    item.Confidence,
			RequiresHumanReview:           item.RequiresHumanReview,
			ConclusionStatus:              item.ConclusionStatus,
			ConfidenceRationale:           item.ConfidenceRationale,
			EvidenceRequests:              cloneEvidenceRequests(item.EvidenceRequests),
			CollectionResults:             diagnosisevidence.CloneItems(item.CollectionResults),
			MissingEvidenceRequests:       diagnosisroom.CloneConsultationEvidenceRequests(item.MissingEvidenceRequests),
			EvidenceCollectionSuggestions: diagnosisroom.CloneConsultationEvidenceRequests(item.EvidenceCollectionSuggestions),
		}
	}
	return out
}

func diagnosisRoomEvidenceCollectedAt(items []diagnosisevidence.Item, fallback time.Time) time.Time {
	var out time.Time
	for _, item := range items {
		if item.CollectedAt.IsZero() {
			continue
		}
		if out.IsZero() || item.CollectedAt.After(out) {
			out = item.CollectedAt
		}
	}
	if out.IsZero() {
		return fallback
	}
	return out
}

func diagnosisRoomEvidenceCollectionAuditItems(items []diagnosisevidence.Item) []diagnosisevidence.Item {
	if items == nil {
		return nil
	}
	out := make([]diagnosisevidence.Item, len(items))
	for i, item := range items {
		out[i] = diagnosisevidence.Item{
			Request:              item.Request,
			TemplateID:           item.TemplateID,
			AlertSourceProfileID: item.AlertSourceProfileID,
			AlertSourceKind:      item.AlertSourceKind,
			Tool:                 item.Tool,
			Status:               item.Status,
			ReasonCode:           item.ReasonCode,
			Message:              item.Message,
			Limit:                item.Limit,
			ObservedAlerts:       item.ObservedAlerts,
			Query:                item.Query,
			WindowSeconds:        item.WindowSeconds,
			StepSeconds:          item.StepSeconds,
			ObservedMetricSeries: item.ObservedMetricSeries,
			CollectedAt:          item.CollectedAt,
		}
	}
	return out
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

func (s *diagnosisRoomState) closeDiagnosisTaskStatus() string {
	if s == nil || s.closeReason != diagnosisRoomCloseInitialTurnFailed {
		return ""
	}
	return "failed"
}

func (s *diagnosisRoomState) closeDiagnosisTaskFailureReason() string {
	if s == nil || s.closeReason != diagnosisRoomCloseInitialTurnFailed || s.latestError == nil {
		return ""
	}
	return strings.TrimSpace(s.latestError.Message)
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
				errTypeRuntimeFailure,
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
	if input.CloseNotificationChannelProfileID < 0 {
		return fmt.Errorf("diagnosis-room: input.close_notification_channel_profile_id must be non-negative")
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
