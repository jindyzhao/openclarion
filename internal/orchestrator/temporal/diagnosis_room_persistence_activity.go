package temporal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscompression"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisRoomEventOpened                     = "diagnosis_room.opened"
	diagnosisRoomEventTurnPersisted              = "diagnosis_room.turn_persisted"
	diagnosisRoomEventEvidenceCollected          = "diagnosis_room.evidence_collected"
	diagnosisRoomEventSupplementalEvidence       = "diagnosis_room.supplemental_evidence_provided"
	diagnosisRoomEventFinalConclusionReady       = "diagnosis_room.final_conclusion_ready"
	diagnosisRoomEventFailed                     = "diagnosis_room.failed"
	diagnosisRoomEventClosed                     = "diagnosis_room.closed"
	diagnosisRoomEventCloseNotificationSent      = "diagnosis_room.close_notification_sent"
	diagnosisRoomEventFinalReadyNotificationSent = "diagnosis_room.final_ready_notification_sent"
	diagnosisRoomEventAssistantTurnNotification  = "diagnosis_room.assistant_turn_notification_sent"

	diagnosisRoomFinalConclusionMaxRunes          = 4096
	diagnosisRoomNotificationEventLookupLimit     = 100
	diagnosisRoomNotificationRetryDedupeComponent = "retry"
	diagnosisRoomNotificationDeliveryErrType      = "NotificationDeliveryFailed"
)

// EnsureDiagnosisChatSessionInput creates or reuses the persisted room session
// before Updates can persist transcript turns.
type EnsureDiagnosisChatSessionInput struct {
	SessionID       string
	DiagnosisTaskID int64
	OwnerSubject    string
	StartedAt       time.Time
}

// EnsureDiagnosisChatSessionResult returns the persisted ChatSession identity.
type EnsureDiagnosisChatSessionResult struct {
	ChatSessionID    int64
	LifecycleEventID int64
	Status           string
	TurnCount        int
	StartedAt        time.Time
	LastActivityAt   time.Time
}

// EnsureDiagnosisRoomSessionInput creates the DiagnosisTask and ChatSession
// for a workflow-started room using the workflow execution identity.
type EnsureDiagnosisRoomSessionInput struct {
	SessionID          string
	EvidenceSnapshotID int64
	WorkflowID         string
	RunID              string
	OwnerSubject       string
	StartedAt          time.Time
}

// EnsureDiagnosisRoomSessionResult returns both task and chat-session
// identities after startup persistence succeeds.
type EnsureDiagnosisRoomSessionResult struct {
	DiagnosisTaskID  int64
	ChatSessionID    int64
	LifecycleEventID int64
	Status           string
	TurnCount        int
	StartedAt        time.Time
	LastActivityAt   time.Time
}

// PersistDiagnosisTurnInput persists the accepted user turn and the accepted
// sandbox assistant response as one idempotent transcript update.
type PersistDiagnosisTurnInput struct {
	SessionID            string
	DiagnosisTaskID      int64
	OwnerSubject         string
	UserMessageID        string
	AssistantMessageID   string
	UserSequence         int
	AssistantSequence    int
	TurnCount            int
	ActorSubject         string
	UserMessage          string
	AssistantMessage     string
	UserOccurredAt       time.Time
	AssistantOccurredAt  time.Time
	ContextBytes         int
	InvocationID         string
	RuntimeID            string
	ContainerStartedAt   time.Time
	ContainerFinishedAt  time.Time
	RawOutput            json.RawMessage
	SupplementalEvidence *DiagnosisRoomSupplementalEvidence
}

// PersistDiagnosisTurnResult returns the persisted row identities for the
// user/assistant transcript pair.
type PersistDiagnosisTurnResult struct {
	ChatSessionID       int64
	LifecycleEventID    int64
	UserTurnID          int64
	AssistantTurnID     int64
	AssistantMessageID  string
	AssistantSequence   int
	AssistantOccurredAt time.Time
	TurnCount           int
	LastActivityAt      time.Time
	AssistantMessage    string
	Confidence          string
	RequiresHumanReview bool
	Findings            []string
	RecommendedActions  []string
	EvidenceRequests    []diagnosisroom.EvidenceRequest
	Insight             diagnosisroom.ConsultationInsight
	FinalConclusion     *DiagnosisRoomFinalConclusion
}

// RecordDiagnosisEvidenceCollectedInput persists the provider-backed collection
// summary for one accepted assistant evidence plan.
type RecordDiagnosisEvidenceCollectedInput struct {
	SessionID          string
	DiagnosisTaskID    int64
	ChatSessionID      int64
	OwnerSubject       string
	ActorSubject       string
	UserMessageID      string
	AssistantMessageID string
	UserTurnID         int64
	AssistantTurnID    int64
	UserSequence       int
	AssistantSequence  int
	TurnCount          int
	Items              []diagnosisevidence.Item
	OccurredAt         time.Time
}

// RecordDiagnosisEvidenceCollectedResult returns the idempotent audit event
// identity for one evidence collection summary.
type RecordDiagnosisEvidenceCollectedResult struct {
	LifecycleEventID int64
}

// CloseDiagnosisChatSessionInput closes the persisted room session and records
// the terminal lifecycle audit event.
type CloseDiagnosisChatSessionInput struct {
	SessionID                         string
	DiagnosisTaskID                   int64
	OwnerSubject                      string
	ConfirmedBy                       string
	TurnCount                         int
	ClosedAt                          time.Time
	Reason                            string
	CloseNotificationChannelProfileID int64
	DiagnosisTaskStatus               string
	DiagnosisTaskFailureReason        string
	GenerateConversationSummary       bool
}

// DiagnosisRoomConversationSummary is the workflow/read representation of one
// immutable lifecycle-end compression checkpoint.
type DiagnosisRoomConversationSummary struct {
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

// CloseDiagnosisChatSessionResult returns the persisted terminal room state.
type CloseDiagnosisChatSessionResult struct {
	ChatSessionID       int64
	LifecycleEventID    int64
	Status              string
	TurnCount           int
	ClosedAt            time.Time
	CloseReason         string
	LastActivityAt      time.Time
	FinalConclusion     DiagnosisRoomFinalConclusion
	ConversationSummary *DiagnosisRoomConversationSummary
}

// SendDiagnosisRoomAssistantTurnNotificationInput sends an operator-facing AI
// diagnosis update after a non-final assistant turn is durably persisted.
type SendDiagnosisRoomAssistantTurnNotificationInput struct {
	SessionID                         string
	DiagnosisTaskID                   int64
	OwnerSubject                      string
	AssistantTurnID                   int64
	AssistantMessageID                string
	AssistantSequence                 int
	TurnCount                         int
	OccurredAt                        time.Time
	CloseNotificationChannelProfileID int64
	AssistantMessage                  string
	Confidence                        string
	RequiresHumanReview               bool
	Findings                          []string
	RecommendedActions                []string
	EvidenceRequests                  []diagnosisroom.EvidenceRequest
	Insight                           diagnosisroom.ConsultationInsight
}

// SendDiagnosisRoomAssistantTurnNotificationResult returns outbound
// notification metadata and the idempotent audit event identity.
type SendDiagnosisRoomAssistantTurnNotificationResult struct {
	ChatSessionID      int64
	LifecycleEventID   int64
	IdempotencyKey     string
	ProviderMessageID  string
	NotificationStatus string
}

// SendDiagnosisRoomFinalReadyNotificationInput sends an operator-facing AI
// diagnosis notice after a final-ready assistant turn is durably persisted.
type SendDiagnosisRoomFinalReadyNotificationInput struct {
	SessionID                         string
	DiagnosisTaskID                   int64
	OwnerSubject                      string
	AssistantTurnID                   int64
	AssistantMessageID                string
	AssistantSequence                 int
	TurnCount                         int
	OccurredAt                        time.Time
	CloseNotificationChannelProfileID int64
	FinalConclusion                   DiagnosisRoomFinalConclusion
}

// SendDiagnosisRoomFinalReadyNotificationResult returns outbound notification
// metadata and the idempotent audit event identity.
type SendDiagnosisRoomFinalReadyNotificationResult struct {
	ChatSessionID      int64
	LifecycleEventID   int64
	IdempotencyKey     string
	ProviderMessageID  string
	NotificationStatus string
}

// SendDiagnosisRoomCloseNotificationResult returns the outbound close
// notification delivery metadata and the idempotent audit event identity.
type SendDiagnosisRoomCloseNotificationResult struct {
	ChatSessionID      int64
	LifecycleEventID   int64
	IdempotencyKey     string
	ProviderMessageID  string
	NotificationStatus string
}

// EnsureDiagnosisChatSession creates the ChatSession row exactly once. Activity
// retries are idempotent through the unique session_key / diagnosis_task_id
// constraints plus a re-fetch path after duplicate inserts.
func (a *Activities) EnsureDiagnosisChatSession(ctx context.Context, req EnsureDiagnosisChatSessionInput) (EnsureDiagnosisChatSessionResult, error) {
	if a.uowFactory == nil {
		return EnsureDiagnosisChatSessionResult{}, temporalsdk.NewNonRetryableApplicationError(
			"ensure-diagnosis-chat-session: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	session, err := a.ensureDiagnosisChatSession(ctx, req)
	if err != nil {
		return EnsureDiagnosisChatSessionResult{}, mapActivityError(err, "ensure-diagnosis-chat-session")
	}
	event, err := a.recordDiagnosisRoomOpened(ctx, req, session)
	if err != nil {
		return EnsureDiagnosisChatSessionResult{}, mapActivityError(err, "ensure-diagnosis-chat-session lifecycle event")
	}
	return ensureDiagnosisChatSessionResult(session, event), nil
}

// EnsureDiagnosisRoomSession creates or reuses the workflow-bound
// DiagnosisTask and ChatSession rows before Updates can persist transcript
// turns. This is the startup path used by HTTP-created diagnosis rooms.
func (a *Activities) EnsureDiagnosisRoomSession(ctx context.Context, req EnsureDiagnosisRoomSessionInput) (EnsureDiagnosisRoomSessionResult, error) {
	if a.uowFactory == nil {
		return EnsureDiagnosisRoomSessionResult{}, temporalsdk.NewNonRetryableApplicationError(
			"ensure-diagnosis-room-session: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	session, task, err := a.ensureDiagnosisRoomSession(ctx, req)
	if err != nil {
		return EnsureDiagnosisRoomSessionResult{}, mapActivityError(err, "ensure-diagnosis-room-session")
	}
	eventReq := EnsureDiagnosisChatSessionInput{
		SessionID:       req.SessionID,
		DiagnosisTaskID: int64(task.ID),
		OwnerSubject:    req.OwnerSubject,
		StartedAt:       req.StartedAt,
	}
	event, err := a.recordDiagnosisRoomOpened(ctx, eventReq, session)
	if err != nil {
		return EnsureDiagnosisRoomSessionResult{}, mapActivityError(err, "ensure-diagnosis-room-session lifecycle event")
	}
	return ensureDiagnosisRoomSessionResult(task, session, event), nil
}

// PersistDiagnosisTurn appends the two transcript rows for one accepted turn
// and advances ChatSession.TurnCount in one transaction. Activity retries return
// the original row IDs when both message IDs already exist.
func (a *Activities) PersistDiagnosisTurn(ctx context.Context, req PersistDiagnosisTurnInput) (PersistDiagnosisTurnResult, error) {
	if a.uowFactory == nil {
		return PersistDiagnosisTurnResult{}, temporalsdk.NewNonRetryableApplicationError(
			"persist-diagnosis-turn: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if _, err := validatePersistDiagnosisTurnInput(req); err != nil {
		return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn input")
	}

	result, err := a.persistDiagnosisTurnOnce(ctx, req)
	if err != nil {
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn")
		}

		var found bool
		var lookupErr error
		result, found, lookupErr = a.lookupPersistedDiagnosisTurn(ctx, req)
		if lookupErr != nil {
			return PersistDiagnosisTurnResult{}, mapActivityError(lookupErr, "persist-diagnosis-turn duplicate lookup")
		}
		if !found {
			return PersistDiagnosisTurnResult{}, mapActivityError(
				fmt.Errorf("duplicate turn insert did not produce both transcript rows: %w", domain.ErrInvariantViolation),
				"persist-diagnosis-turn duplicate lookup",
			)
		}
	}
	event, err := a.recordDiagnosisRoomTurnPersisted(ctx, req, result)
	if err != nil {
		return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn lifecycle event")
	}
	result.LifecycleEventID = int64(event.ID)
	if req.SupplementalEvidence != nil {
		if _, err := a.recordDiagnosisRoomSupplementalEvidenceProvided(ctx, req, result); err != nil {
			return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn supplemental evidence event")
		}
	}
	if diagnosisRoomFinalConclusionReadyStatus(result.Insight.ConclusionStatus) {
		_, finalConclusion, err := a.recordDiagnosisRoomFinalConclusionReady(ctx, req, result)
		if err != nil {
			return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn final conclusion event")
		}
		result.FinalConclusion = copyDiagnosisRoomFinalConclusion(finalConclusion)
	}
	return result, nil
}

// RecordDiagnosisEvidenceCollected appends a compact, sanitized collection audit
// event after provider-backed evidence collection completes for a turn.
func (a *Activities) RecordDiagnosisEvidenceCollected(
	ctx context.Context,
	req RecordDiagnosisEvidenceCollectedInput,
) (RecordDiagnosisEvidenceCollectedResult, error) {
	if a.uowFactory == nil {
		return RecordDiagnosisEvidenceCollectedResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-diagnosis-evidence-collected: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if err := validateRecordDiagnosisEvidenceCollectedInput(req); err != nil {
		return RecordDiagnosisEvidenceCollectedResult{}, mapActivityError(err, "record-diagnosis-evidence-collected input")
	}
	event, err := a.recordDiagnosisRoomEvidenceCollected(ctx, req)
	if err != nil {
		return RecordDiagnosisEvidenceCollectedResult{}, mapActivityError(err, "record-diagnosis-evidence-collected lifecycle event")
	}
	return RecordDiagnosisEvidenceCollectedResult{LifecycleEventID: int64(event.ID)}, nil
}

// CloseDiagnosisChatSession persists terminal room lifecycle metadata and an
// idempotent close audit event.
func (a *Activities) CloseDiagnosisChatSession(ctx context.Context, req CloseDiagnosisChatSessionInput) (CloseDiagnosisChatSessionResult, error) {
	if a.uowFactory == nil {
		return CloseDiagnosisChatSessionResult{}, temporalsdk.NewNonRetryableApplicationError(
			"close-diagnosis-chat-session: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	session, task, summary, err := a.closeDiagnosisChatSession(ctx, req)
	if err != nil {
		return CloseDiagnosisChatSessionResult{}, mapActivityError(err, "close-diagnosis-chat-session")
	}
	if task.Status == domain.DiagnosisStatusFailed {
		if _, err := a.recordDiagnosisRoomFailed(ctx, req, session, task); err != nil {
			return CloseDiagnosisChatSessionResult{}, mapActivityError(err, "close-diagnosis-chat-session failure event")
		}
	}
	event, finalConclusion, err := a.recordDiagnosisRoomClosed(ctx, req, session, summary)
	if err != nil {
		return CloseDiagnosisChatSessionResult{}, mapActivityError(err, "close-diagnosis-chat-session lifecycle event")
	}
	return closeDiagnosisChatSessionResult(session, event, finalConclusion, summary), nil
}

// SendDiagnosisRoomAssistantTurnNotification delivers an AI diagnosis update
// for a persisted non-final assistant turn and records an idempotent audit
// event. It is separate from close notification because operators may still
// need to collect evidence and continue the consultation.
func (a *Activities) SendDiagnosisRoomAssistantTurnNotification(
	ctx context.Context,
	req SendDiagnosisRoomAssistantTurnNotificationInput,
) (SendDiagnosisRoomAssistantTurnNotificationResult, error) {
	if a.uowFactory == nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-assistant-turn-notification: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if err := validateSendDiagnosisRoomAssistantTurnNotificationInput(req); err != nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, mapActivityError(err, "send-diagnosis-room-assistant-turn-notification input")
	}
	session, task, snapshot, err := a.loadDiagnosisRoomForAssistantTurnNotification(ctx, req)
	if err != nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, mapActivityError(err, "send-diagnosis-room-assistant-turn-notification load room")
	}
	idempotencyKey := diagnosisRoomAssistantTurnNotificationIdempotencyKey(req)
	existing, found, dedupeComponent, err := a.findCompletedDiagnosisRoomNotificationEvent(ctx, req.DiagnosisTaskID, diagnosisRoomEventAssistantTurnNotification, idempotencyKey)
	if err != nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, mapActivityError(err, "send-diagnosis-room-assistant-turn-notification lookup delivery event")
	}
	if found {
		return diagnosisRoomAssistantTurnNotificationResultFromEvent(existing), nil
	}

	imProvider, err := a.diagnosisRoomConsultationNotificationProvider(ctx, domain.NotificationChannelProfileID(req.CloseNotificationChannelProfileID))
	if err != nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, err
	}
	roomURL := a.diagnosisRoomPublicURL(req.SessionID)
	delivery, err := imProvider.SendNotification(ctx, diagnosisRoomAssistantTurnNotification(req, session, task, snapshot, idempotencyKey, roomURL))
	if err != nil {
		event, recordErr := a.recordDiagnosisRoomAssistantTurnNotificationSent(ctx, req, session, task, snapshot, idempotencyKey, dedupeComponent, roomURL, diagnosisRoomFailedNotificationDelivery(err))
		if recordErr != nil {
			return SendDiagnosisRoomAssistantTurnNotificationResult{}, mapActivityError(recordErr, "send-diagnosis-room-assistant-turn-notification failed lifecycle event")
		}
		if retryErr := diagnosisRoomRetryableNotificationActivityError(err); retryErr != nil {
			return SendDiagnosisRoomAssistantTurnNotificationResult{}, retryErr
		}
		return diagnosisRoomAssistantTurnNotificationResultFromEvent(event), nil
	}
	event, err := a.recordDiagnosisRoomAssistantTurnNotificationSent(ctx, req, session, task, snapshot, idempotencyKey, dedupeComponent, roomURL, delivery)
	if err != nil {
		return SendDiagnosisRoomAssistantTurnNotificationResult{}, mapActivityError(err, "send-diagnosis-room-assistant-turn-notification lifecycle event")
	}
	return diagnosisRoomAssistantTurnNotificationResultFromEvent(event), nil
}

// SendDiagnosisRoomFinalReadyNotification delivers an AI diagnosis notice for a
// persisted final-ready assistant turn and records an idempotent audit event. It
// is separate from close notification because the product flow asks operators
// to review and provide more evidence before confirming closure.
func (a *Activities) SendDiagnosisRoomFinalReadyNotification(
	ctx context.Context,
	req SendDiagnosisRoomFinalReadyNotificationInput,
) (SendDiagnosisRoomFinalReadyNotificationResult, error) {
	if a.uowFactory == nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-final-ready-notification: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if err := validateSendDiagnosisRoomFinalReadyNotificationInput(req); err != nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, mapActivityError(err, "send-diagnosis-room-final-ready-notification input")
	}
	session, task, snapshot, err := a.loadDiagnosisRoomForFinalReadyNotification(ctx, req)
	if err != nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, mapActivityError(err, "send-diagnosis-room-final-ready-notification load room")
	}
	idempotencyKey := diagnosisRoomFinalReadyNotificationIdempotencyKey(req)
	existing, found, dedupeComponent, err := a.findCompletedDiagnosisRoomNotificationEvent(ctx, req.DiagnosisTaskID, diagnosisRoomEventFinalReadyNotificationSent, idempotencyKey)
	if err != nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, mapActivityError(err, "send-diagnosis-room-final-ready-notification lookup delivery event")
	}
	if found {
		return diagnosisRoomFinalReadyNotificationResultFromEvent(existing), nil
	}

	imProvider, err := a.diagnosisRoomConsultationNotificationProvider(ctx, domain.NotificationChannelProfileID(req.CloseNotificationChannelProfileID))
	if err != nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, err
	}
	roomURL := a.diagnosisRoomPublicURL(req.SessionID)
	delivery, err := imProvider.SendNotification(ctx, diagnosisRoomFinalReadyNotification(req, session, task, snapshot, idempotencyKey, roomURL))
	if err != nil {
		event, recordErr := a.recordDiagnosisRoomFinalReadyNotificationSent(ctx, req, session, task, snapshot, idempotencyKey, dedupeComponent, roomURL, diagnosisRoomFailedNotificationDelivery(err))
		if recordErr != nil {
			return SendDiagnosisRoomFinalReadyNotificationResult{}, mapActivityError(recordErr, "send-diagnosis-room-final-ready-notification failed lifecycle event")
		}
		if retryErr := diagnosisRoomRetryableNotificationActivityError(err); retryErr != nil {
			return SendDiagnosisRoomFinalReadyNotificationResult{}, retryErr
		}
		return diagnosisRoomFinalReadyNotificationResultFromEvent(event), nil
	}
	event, err := a.recordDiagnosisRoomFinalReadyNotificationSent(ctx, req, session, task, snapshot, idempotencyKey, dedupeComponent, roomURL, delivery)
	if err != nil {
		return SendDiagnosisRoomFinalReadyNotificationResult{}, mapActivityError(err, "send-diagnosis-room-final-ready-notification lifecycle event")
	}
	return diagnosisRoomFinalReadyNotificationResultFromEvent(event), nil
}

// SendDiagnosisRoomCloseNotification delivers the final operator notification
// for a closed diagnosis room and records an idempotent delivery audit event.
func (a *Activities) SendDiagnosisRoomCloseNotification(ctx context.Context, req CloseDiagnosisChatSessionInput) (SendDiagnosisRoomCloseNotificationResult, error) {
	if a.uowFactory == nil {
		return SendDiagnosisRoomCloseNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-close-notification: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	session, task, snapshot, finalConclusion, err := a.loadClosedDiagnosisRoomForNotification(ctx, req)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification load room")
	}
	idempotencyKey := diagnosisRoomCloseNotificationIdempotencyKey(req, session)
	existing, found, dedupeComponent, err := a.findCompletedDiagnosisRoomNotificationEvent(ctx, req.DiagnosisTaskID, diagnosisRoomEventCloseNotificationSent, idempotencyKey)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification lookup delivery event")
	}
	if found {
		return diagnosisRoomCloseNotificationResultFromEvent(existing), nil
	}

	notificationContext, err := a.diagnosisRoomCloseNotificationContext(ctx, session, finalConclusion)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification load context")
	}
	imProvider, err := a.diagnosisRoomCloseNotificationProvider(ctx, domain.NotificationChannelProfileID(req.CloseNotificationChannelProfileID))
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, err
	}
	roomURL := a.diagnosisRoomPublicURL(req.SessionID)
	delivery, err := imProvider.SendNotification(ctx, diagnosisRoomCloseNotification(req, session, task, snapshot, finalConclusion, notificationContext, idempotencyKey, roomURL))
	if err != nil {
		event, recordErr := a.recordDiagnosisRoomCloseNotificationSent(ctx, req, session, task, snapshot, finalConclusion, idempotencyKey, dedupeComponent, roomURL, diagnosisRoomFailedNotificationDelivery(err))
		if recordErr != nil {
			return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(recordErr, "send-diagnosis-room-close-notification failed lifecycle event")
		}
		if retryErr := diagnosisRoomRetryableNotificationActivityError(err); retryErr != nil {
			return SendDiagnosisRoomCloseNotificationResult{}, retryErr
		}
		return diagnosisRoomCloseNotificationResultFromEvent(event), nil
	}
	event, err := a.recordDiagnosisRoomCloseNotificationSent(ctx, req, session, task, snapshot, finalConclusion, idempotencyKey, dedupeComponent, roomURL, delivery)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification lifecycle event")
	}
	return diagnosisRoomCloseNotificationResultFromEvent(event), nil
}

func (a *Activities) diagnosisRoomCloseNotificationProvider(
	ctx context.Context,
	channelProfileID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	if channelProfileID == 0 {
		if a.imProvider == nil {
			return nil, temporalsdk.NewNonRetryableApplicationError(
				"send-diagnosis-room-close-notification: im provider is not configured", errTypeInvalidInput, nil)
		}
		return a.imProvider, nil
	}
	if a.notificationProviderResolver == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-close-notification: notification channel provider resolver is not configured", errTypeInvalidInput, nil)
	}
	provider, err := a.notificationProviderResolver.ResolveDiagnosisCloseNotificationProvider(ctx, channelProfileID)
	if err != nil {
		return nil, mapActivityError(err, "send-diagnosis-room-close-notification resolve notification channel")
	}
	if provider == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-close-notification: notification channel provider resolver returned nil provider", errTypeInvalidInput, nil)
	}
	return provider, nil
}

func (a *Activities) diagnosisRoomConsultationNotificationProvider(
	ctx context.Context,
	channelProfileID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	if channelProfileID == 0 {
		if a.imProvider == nil {
			return nil, temporalsdk.NewNonRetryableApplicationError(
				"send-diagnosis-room-consultation-notification: im provider is not configured", errTypeInvalidInput, nil)
		}
		return a.imProvider, nil
	}
	if a.notificationProviderResolver == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-consultation-notification: notification channel provider resolver is not configured", errTypeInvalidInput, nil)
	}
	provider, err := a.notificationProviderResolver.ResolveDiagnosisConsultationNotificationProvider(ctx, channelProfileID)
	if err != nil {
		return nil, mapActivityError(err, "send-diagnosis-room-consultation-notification resolve notification channel")
	}
	if provider == nil {
		return nil, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-consultation-notification: notification channel provider resolver returned nil provider", errTypeInvalidInput, nil)
	}
	return provider, nil
}

func (a *Activities) ensureDiagnosisChatSession(ctx context.Context, req EnsureDiagnosisChatSessionInput) (domain.ChatSession, error) {
	if err := validateEnsureDiagnosisChatSessionInput(req); err != nil {
		return domain.ChatSession{}, err
	}
	if session, found, err := a.lookupChatSessionByKey(ctx, req.SessionID); err != nil {
		return domain.ChatSession{}, err
	} else if found {
		if err := validateChatSessionBinding(session, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return domain.ChatSession{}, err
		}
		return session, nil
	}

	candidate, err := domain.NewChatSession(
		domain.DiagnosisTaskID(req.DiagnosisTaskID),
		req.SessionID,
		req.OwnerSubject,
		req.StartedAt,
	)
	if err != nil {
		return domain.ChatSession{}, err
	}
	var saved domain.ChatSession
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().SaveChatSession(ctx, candidate)
		if err != nil {
			return err
		}
		saved = out
		return nil
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.ChatSession{}, err
	}

	session, found, lookupErr := a.lookupChatSessionByKey(ctx, req.SessionID)
	if lookupErr != nil {
		return domain.ChatSession{}, lookupErr
	}
	if !found {
		return domain.ChatSession{}, fmt.Errorf("duplicate chat session insert for task %d but session_key %q was not found: %w",
			req.DiagnosisTaskID, req.SessionID, domain.ErrInvariantViolation)
	}
	if err := validateChatSessionBinding(session, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
		return domain.ChatSession{}, err
	}
	return session, nil
}

func (a *Activities) ensureDiagnosisRoomSession(ctx context.Context, req EnsureDiagnosisRoomSessionInput) (domain.ChatSession, domain.DiagnosisTask, error) {
	if err := validateEnsureDiagnosisRoomSessionInput(req); err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, err
	}
	if session, found, err := a.lookupChatSessionByKey(ctx, req.SessionID); err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, err
	} else if found {
		task, err := a.lookupDiagnosisTaskByID(ctx, session.DiagnosisTaskID)
		if err != nil {
			return domain.ChatSession{}, domain.DiagnosisTask{}, err
		}
		if err := validateDiagnosisRoomSessionTaskBinding(req, session, task); err != nil {
			return domain.ChatSession{}, domain.DiagnosisTask{}, err
		}
		return session, task, nil
	}

	task, err := a.ensureDiagnosisRoomTask(ctx, req)
	if err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, err
	}
	session, err := a.ensureDiagnosisRoomChatSession(ctx, req, task)
	if err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, err
	}
	return session, task, nil
}

func (a *Activities) ensureDiagnosisRoomTask(ctx context.Context, req EnsureDiagnosisRoomSessionInput) (domain.DiagnosisTask, error) {
	if task, found, err := a.lookupDiagnosisTaskByExecution(ctx, req.WorkflowID, req.RunID); err != nil {
		return domain.DiagnosisTask{}, err
	} else if found {
		if err := validateDiagnosisRoomTaskBinding(req, task); err != nil {
			return domain.DiagnosisTask{}, err
		}
		return task, nil
	}

	candidate, err := domain.NewDiagnosisTask(
		domain.EvidenceSnapshotID(req.EvidenceSnapshotID),
		strings.TrimSpace(req.WorkflowID),
		strings.TrimSpace(req.RunID),
	)
	if err != nil {
		return domain.DiagnosisTask{}, err
	}
	started, err := candidate.Start(req.StartedAt)
	if err != nil {
		return domain.DiagnosisTask{}, err
	}

	var saved domain.DiagnosisTask
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		if _, err := uow.Evidence().FindByID(ctx, domain.EvidenceSnapshotID(req.EvidenceSnapshotID)); err != nil {
			return err
		}
		out, err := uow.Diagnosis().SaveTask(ctx, started)
		if err != nil {
			return err
		}
		saved = out
		return nil
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.DiagnosisTask{}, err
	}
	task, found, lookupErr := a.lookupDiagnosisTaskByExecution(ctx, req.WorkflowID, req.RunID)
	if lookupErr != nil {
		return domain.DiagnosisTask{}, lookupErr
	}
	if !found {
		return domain.DiagnosisTask{}, fmt.Errorf("duplicate diagnosis task insert for workflow %q run %q but task was not found: %w",
			req.WorkflowID, req.RunID, domain.ErrInvariantViolation)
	}
	if err := validateDiagnosisRoomTaskBinding(req, task); err != nil {
		return domain.DiagnosisTask{}, err
	}
	return task, nil
}

func (a *Activities) ensureDiagnosisRoomChatSession(ctx context.Context, req EnsureDiagnosisRoomSessionInput, task domain.DiagnosisTask) (domain.ChatSession, error) {
	candidate, err := domain.NewChatSession(
		task.ID,
		req.SessionID,
		req.OwnerSubject,
		req.StartedAt,
	)
	if err != nil {
		return domain.ChatSession{}, err
	}
	var saved domain.ChatSession
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().SaveChatSession(ctx, candidate)
		if err != nil {
			return err
		}
		saved = out
		return nil
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.ChatSession{}, err
	}
	session, found, lookupErr := a.lookupChatSessionByKey(ctx, req.SessionID)
	if lookupErr != nil {
		return domain.ChatSession{}, lookupErr
	}
	if !found {
		return domain.ChatSession{}, fmt.Errorf("duplicate chat session insert for task %d but session_key %q was not found: %w",
			task.ID, req.SessionID, domain.ErrInvariantViolation)
	}
	if err := validateDiagnosisRoomSessionTaskBinding(req, session, task); err != nil {
		return domain.ChatSession{}, err
	}
	return session, nil
}

func (a *Activities) lookupChatSessionByKey(ctx context.Context, sessionID string) (domain.ChatSession, bool, error) {
	var out domain.ChatSession
	found := false
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, sessionID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		out = session
		found = true
		return nil
	})
	return out, found, err
}

func (a *Activities) lookupDiagnosisTaskByID(ctx context.Context, taskID domain.DiagnosisTaskID) (domain.DiagnosisTask, error) {
	var task domain.DiagnosisTask
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		task, err = uow.Diagnosis().FindTaskByID(ctx, taskID)
		return err
	})
	return task, err
}

func (a *Activities) lookupDiagnosisTaskByExecution(ctx context.Context, workflowID, runID string) (domain.DiagnosisTask, bool, error) {
	var task domain.DiagnosisTask
	found := false
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Diagnosis().FindTaskByExecution(ctx, strings.TrimSpace(workflowID), strings.TrimSpace(runID))
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil
			}
			return err
		}
		task = got
		found = true
		return nil
	})
	return task, found, err
}

func (a *Activities) closeDiagnosisChatSession(ctx context.Context, req CloseDiagnosisChatSessionInput) (domain.ChatSession, domain.DiagnosisTask, *domain.ChatSessionSummary, error) {
	if err := validateCloseDiagnosisChatSessionInput(req); err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, nil, err
	}
	taskStatus, failureReason, err := diagnosisTaskTerminalStateForRoomClose(req)
	if err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, nil, err
	}
	var saved domain.ChatSession
	var savedTask domain.DiagnosisTask
	var savedSummary *domain.ChatSessionSummary
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionIdentity(session, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}
		if session.TurnCount != req.TurnCount {
			return fmt.Errorf("close diagnosis chat session: turn_count %d does not match workflow turn_count %d: %w",
				session.TurnCount, req.TurnCount, domain.ErrInvariantViolation)
		}
		if req.GenerateConversationSummary {
			summary, err := ensureDiagnosisConversationSummary(ctx, uow.Diagnosis(), session, req)
			if err != nil {
				return err
			}
			savedSummary = &summary
		}
		task, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
		if err != nil {
			return err
		}
		finishedTask, err := task.Finish(taskStatus, req.ClosedAt, failureReason)
		if err != nil {
			return err
		}
		savedTask, err = uow.Diagnosis().UpdateTask(ctx, finishedTask)
		if err != nil {
			return err
		}
		closed, err := session.Close(req.ClosedAt, req.Reason)
		if err != nil {
			return err
		}
		saved, err = uow.Diagnosis().UpdateChatSession(ctx, closed)
		return err
	})
	return saved, savedTask, savedSummary, err
}

func ensureDiagnosisConversationSummary(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	session domain.ChatSession,
	req CloseDiagnosisChatSessionInput,
) (domain.ChatSessionSummary, error) {
	turns, err := repo.ListChatTurnsBySession(ctx, session.ID, diagnosiscompression.MaxSourceTurns+1)
	if err != nil {
		return domain.ChatSessionSummary{}, err
	}
	wantSourceTurns := req.TurnCount * 2
	if len(turns) != wantSourceTurns {
		return domain.ChatSessionSummary{}, fmt.Errorf(
			"close diagnosis chat session: persisted transcript has %d turns, want %d for workflow turn_count %d: %w",
			len(turns), wantSourceTurns, req.TurnCount, domain.ErrInvariantViolation,
		)
	}
	candidate, err := diagnosiscompression.Summarize(session.ID, turns, req.ClosedAt)
	if err != nil {
		return domain.ChatSessionSummary{}, err
	}
	existing, err := repo.FindChatSessionSummaryBySessionAndVersion(ctx, session.ID, diagnosiscompression.SummaryVersion)
	if err == nil {
		if !diagnosiscompression.Equivalent(existing, candidate) {
			return domain.ChatSessionSummary{}, fmt.Errorf(
				"close diagnosis chat session: existing conversation summary version %d does not match persisted transcript: %w",
				diagnosiscompression.SummaryVersion, domain.ErrInvariantViolation,
			)
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.ChatSessionSummary{}, err
	}
	return repo.SaveChatSessionSummary(ctx, candidate)
}

func diagnosisTaskTerminalStateForRoomClose(req CloseDiagnosisChatSessionInput) (domain.DiagnosisStatus, string, error) {
	statusText := strings.TrimSpace(req.DiagnosisTaskStatus)
	failureReason := strings.TrimSpace(req.DiagnosisTaskFailureReason)
	if statusText == "" && failureReason != "" {
		statusText = string(domain.DiagnosisStatusFailed)
	}
	if statusText == "" {
		switch strings.TrimSpace(req.Reason) {
		case diagnosisRoomCloseCancelled, diagnosisRoomCloseContextCanceled, diagnosisRoomCloseSessionTimeout, diagnosisRoomCloseIdleTimeout:
			return domain.DiagnosisStatusCancelled, "", nil
		default:
			return domain.DiagnosisStatusSucceeded, "", nil
		}
	}
	status := domain.DiagnosisStatus(statusText)
	if !status.Valid() || !status.IsTerminal() {
		return "", "", fmt.Errorf("close diagnosis chat session: diagnosis_task_status must be terminal, got %q: %w", statusText, domain.ErrInvariantViolation)
	}
	if status == domain.DiagnosisStatusFailed {
		if failureReason == "" {
			return "", "", fmt.Errorf("close diagnosis chat session: diagnosis_task_failure_reason must be set when status is failed: %w", domain.ErrInvariantViolation)
		}
		return status, truncateString(failureReason, 1024), nil
	}
	if failureReason != "" {
		return "", "", fmt.Errorf("close diagnosis chat session: diagnosis_task_failure_reason must be empty when status is %q: %w", status, domain.ErrInvariantViolation)
	}
	return status, "", nil
}

func (a *Activities) loadClosedDiagnosisRoomForNotification(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
) (domain.ChatSession, domain.DiagnosisTask, domain.EvidenceSnapshot, DiagnosisRoomFinalConclusion, error) {
	if err := validateCloseDiagnosisChatSessionInput(req); err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, domain.EvidenceSnapshot{}, DiagnosisRoomFinalConclusion{}, err
	}
	var session domain.ChatSession
	var task domain.DiagnosisTask
	var snapshot domain.EvidenceSnapshot
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		gotSession, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionIdentity(gotSession, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}
		if !gotSession.Status.IsTerminal() || gotSession.ClosedAt == nil {
			return fmt.Errorf("diagnosis room close notification: session %q is not closed: %w", req.SessionID, domain.ErrInvariantViolation)
		}
		if gotSession.TurnCount != req.TurnCount {
			return fmt.Errorf("diagnosis room close notification: turn_count %d does not match session turn_count %d: %w",
				req.TurnCount, gotSession.TurnCount, domain.ErrInvariantViolation)
		}
		gotTask, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
		if err != nil {
			return err
		}
		gotSnapshot, err := uow.Evidence().FindByID(ctx, gotTask.EvidenceSnapshotID)
		if err != nil {
			return err
		}
		session = gotSession
		task = gotTask
		snapshot = gotSnapshot
		return nil
	})
	if err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, domain.EvidenceSnapshot{}, DiagnosisRoomFinalConclusion{}, err
	}
	finalConclusion, err := a.diagnosisRoomFinalConclusion(ctx, req, session, task)
	if err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, domain.EvidenceSnapshot{}, DiagnosisRoomFinalConclusion{}, err
	}
	return session, task, snapshot, finalConclusion, nil
}

func (a *Activities) loadDiagnosisRoomForAssistantTurnNotification(
	ctx context.Context,
	req SendDiagnosisRoomAssistantTurnNotificationInput,
) (domain.ChatSession, domain.DiagnosisTask, domain.EvidenceSnapshot, error) {
	var session domain.ChatSession
	var task domain.DiagnosisTask
	var snapshot domain.EvidenceSnapshot
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		gotSession, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionIdentity(gotSession, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}
		if gotSession.TurnCount < req.TurnCount {
			return fmt.Errorf("diagnosis room assistant-turn notification: turn_count %d is ahead of session turn_count %d: %w",
				req.TurnCount, gotSession.TurnCount, domain.ErrInvariantViolation)
		}
		gotTask, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
		if err != nil {
			return err
		}
		gotSnapshot, err := uow.Evidence().FindByID(ctx, gotTask.EvidenceSnapshotID)
		if err != nil {
			return err
		}
		session = gotSession
		task = gotTask
		snapshot = gotSnapshot
		return nil
	})
	return session, task, snapshot, err
}

func (a *Activities) loadDiagnosisRoomForFinalReadyNotification(
	ctx context.Context,
	req SendDiagnosisRoomFinalReadyNotificationInput,
) (domain.ChatSession, domain.DiagnosisTask, domain.EvidenceSnapshot, error) {
	var session domain.ChatSession
	var task domain.DiagnosisTask
	var snapshot domain.EvidenceSnapshot
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		gotSession, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionIdentity(gotSession, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}
		if gotSession.TurnCount < req.TurnCount {
			return fmt.Errorf("diagnosis room final-ready notification: turn_count %d is ahead of session turn_count %d: %w",
				req.TurnCount, gotSession.TurnCount, domain.ErrInvariantViolation)
		}
		gotTask, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
		if err != nil {
			return err
		}
		gotSnapshot, err := uow.Evidence().FindByID(ctx, gotTask.EvidenceSnapshotID)
		if err != nil {
			return err
		}
		session = gotSession
		task = gotTask
		snapshot = gotSnapshot
		return nil
	})
	return session, task, snapshot, err
}

func (a *Activities) persistDiagnosisTurnOnce(ctx context.Context, req PersistDiagnosisTurnInput) (PersistDiagnosisTurnResult, error) {
	parsedOutput, err := validatePersistDiagnosisTurnInput(req)
	if err != nil {
		return PersistDiagnosisTurnResult{}, err
	}
	userMetadata, err := diagnosisUserTurnMetadata(req)
	if err != nil {
		return PersistDiagnosisTurnResult{}, err
	}
	assistantMetadata, err := diagnosisAssistantTurnMetadata(req, parsedOutput)
	if err != nil {
		return PersistDiagnosisTurnResult{}, err
	}

	var result PersistDiagnosisTurnResult
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionBinding(session, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}

		userTurn, userFound, err := findChatTurnByMessageID(ctx, uow.Diagnosis(), session.ID, req.UserMessageID)
		if err != nil {
			return err
		}
		assistantTurn, assistantFound, err := findChatTurnByMessageID(ctx, uow.Diagnosis(), session.ID, req.AssistantMessageID)
		if err != nil {
			return err
		}
		if userFound || assistantFound {
			if !userFound || !assistantFound {
				return fmt.Errorf("partial persisted diagnosis turn for user_message_id %q assistant_message_id %q: %w",
					req.UserMessageID, req.AssistantMessageID, domain.ErrInvariantViolation)
			}
			result = persistDiagnosisTurnResult(session, userTurn, assistantTurn, parsedOutput)
			return nil
		}

		userTurn, err = domain.NewChatTurn(domain.ChatTurn{
			SessionID:    session.ID,
			MessageID:    req.UserMessageID,
			Sequence:     req.UserSequence,
			Role:         domain.ChatRoleUser,
			ActorSubject: req.ActorSubject,
			Content:      req.UserMessage,
			Metadata:     userMetadata,
			OccurredAt:   req.UserOccurredAt,
		})
		if err != nil {
			return err
		}
		assistantTurn, err = domain.NewChatTurn(domain.ChatTurn{
			SessionID:    session.ID,
			MessageID:    req.AssistantMessageID,
			Sequence:     req.AssistantSequence,
			Role:         domain.ChatRoleAssistant,
			ActorSubject: diagnosisRoomAgentName,
			Content:      parsedOutput.Message,
			Metadata:     assistantMetadata,
			OccurredAt:   req.AssistantOccurredAt,
		})
		if err != nil {
			return err
		}
		savedUser, err := uow.Diagnosis().SaveChatTurn(ctx, userTurn)
		if err != nil {
			return err
		}
		savedAssistant, err := uow.Diagnosis().SaveChatTurn(ctx, assistantTurn)
		if err != nil {
			return err
		}
		advanced, err := session.RecordTurn(req.AssistantOccurredAt)
		if err != nil {
			return err
		}
		savedSession, err := uow.Diagnosis().UpdateChatSession(ctx, advanced)
		if err != nil {
			return err
		}
		result = persistDiagnosisTurnResult(savedSession, savedUser, savedAssistant, parsedOutput)
		return nil
	})
	return result, err
}

func (a *Activities) lookupPersistedDiagnosisTurn(ctx context.Context, req PersistDiagnosisTurnInput) (PersistDiagnosisTurnResult, bool, error) {
	parsedOutput, err := validatePersistDiagnosisTurnInput(req)
	if err != nil {
		return PersistDiagnosisTurnResult{}, false, err
	}
	var result PersistDiagnosisTurnResult
	found := false
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		session, err := uow.Diagnosis().FindChatSessionByKey(ctx, req.SessionID)
		if err != nil {
			return err
		}
		if err := validateChatSessionBinding(session, req.SessionID, req.DiagnosisTaskID, req.OwnerSubject); err != nil {
			return err
		}
		userTurn, userFound, err := findChatTurnByMessageID(ctx, uow.Diagnosis(), session.ID, req.UserMessageID)
		if err != nil {
			return err
		}
		assistantTurn, assistantFound, err := findChatTurnByMessageID(ctx, uow.Diagnosis(), session.ID, req.AssistantMessageID)
		if err != nil {
			return err
		}
		if !userFound && !assistantFound {
			return nil
		}
		if !userFound || !assistantFound {
			return fmt.Errorf("partial persisted diagnosis turn for user_message_id %q assistant_message_id %q: %w",
				req.UserMessageID, req.AssistantMessageID, domain.ErrInvariantViolation)
		}
		result = persistDiagnosisTurnResult(session, userTurn, assistantTurn, parsedOutput)
		found = true
		return nil
	})
	return result, found, err
}

func findChatTurnByMessageID(ctx context.Context, repo ports.DiagnosisRepository, sessionID domain.ChatSessionID, messageID string) (domain.ChatTurn, bool, error) {
	turn, err := repo.FindChatTurnBySessionAndMessageID(ctx, sessionID, messageID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ChatTurn{}, false, nil
		}
		return domain.ChatTurn{}, false, err
	}
	return turn, true, nil
}

func validateEnsureDiagnosisChatSessionInput(req EnsureDiagnosisChatSessionInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("ensure diagnosis chat session: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("ensure diagnosis chat session: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("ensure diagnosis chat session: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return fmt.Errorf("ensure diagnosis chat session: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.StartedAt.IsZero() {
		return fmt.Errorf("ensure diagnosis chat session: started_at must be set: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateEnsureDiagnosisRoomSessionInput(req EnsureDiagnosisRoomSessionInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("ensure diagnosis room session: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("ensure diagnosis room session: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.EvidenceSnapshotID == 0 {
		return fmt.Errorf("ensure diagnosis room session: evidence_snapshot_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		return fmt.Errorf("ensure diagnosis room session: workflow_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.RunID) == "" {
		return fmt.Errorf("ensure diagnosis room session: run_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return fmt.Errorf("ensure diagnosis room session: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.StartedAt.IsZero() {
		return fmt.Errorf("ensure diagnosis room session: started_at must be set: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validatePersistDiagnosisTurnInput(req PersistDiagnosisTurnInput) (diagnosisroom.TurnOutput, error) {
	if strings.TrimSpace(req.SessionID) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.UserMessageID) == "" || strings.TrimSpace(req.AssistantMessageID) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: message IDs must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.UserMessageID) != req.UserMessageID || strings.TrimSpace(req.AssistantMessageID) != req.AssistantMessageID {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: message IDs must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.UserMessageID == req.AssistantMessageID {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: user and assistant message IDs must differ: %w", domain.ErrInvariantViolation)
	}
	if req.UserSequence <= 0 || req.AssistantSequence != req.UserSequence+1 {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: assistant_sequence must equal user_sequence + 1: %w", domain.ErrInvariantViolation)
	}
	if req.TurnCount <= 0 {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: turn_count must be > 0: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.ActorSubject) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: actor_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.UserMessage) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: user_message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessage) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: assistant_message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.UserOccurredAt.IsZero() || req.AssistantOccurredAt.IsZero() {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: occurred_at timestamps must be set: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantOccurredAt.Before(req.UserOccurredAt) {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: assistant_occurred_at precedes user_occurred_at: %w", domain.ErrInvariantViolation)
	}
	if req.ContextBytes <= 0 {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: context_bytes must be > 0: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.InvocationID) == "" {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: invocation_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if err := validateDiagnosisRoomSupplementalEvidence(req.SupplementalEvidence); err != nil {
		return diagnosisroom.TurnOutput{}, err
	}
	parsed, err := diagnosisroom.ParseTurnOutput(req.RawOutput)
	if err != nil {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err)
	}
	if parsed.Message != strings.TrimSpace(req.AssistantMessage) {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: assistant_message does not match validated output message: %w", domain.ErrInvariantViolation)
	}
	return parsed, nil
}

func validateRecordDiagnosisEvidenceCollectedInput(req RecordDiagnosisEvidenceCollectedInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("record diagnosis evidence collected: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("record diagnosis evidence collected: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 || req.ChatSessionID == 0 {
		return fmt.Errorf("record diagnosis evidence collected: persisted task and chat session IDs must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" || strings.TrimSpace(req.ActorSubject) == "" {
		return fmt.Errorf("record diagnosis evidence collected: owner_subject and actor_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.UserMessageID) == "" {
		return fmt.Errorf("record diagnosis evidence collected: user_message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.UserMessageID) != req.UserMessageID {
		return fmt.Errorf("record diagnosis evidence collected: user_message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if recordDiagnosisEvidenceCollectedHasTurnPair(req) {
		if strings.TrimSpace(req.AssistantMessageID) == "" {
			return fmt.Errorf("record diagnosis evidence collected: assistant_message_id must be non-empty for turn-bound collection: %w", domain.ErrInvariantViolation)
		}
		if strings.TrimSpace(req.AssistantMessageID) != req.AssistantMessageID {
			return fmt.Errorf("record diagnosis evidence collected: assistant_message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
		}
		if req.UserMessageID == req.AssistantMessageID {
			return fmt.Errorf("record diagnosis evidence collected: user and assistant message IDs must differ: %w", domain.ErrInvariantViolation)
		}
		if req.UserTurnID == 0 || req.AssistantTurnID == 0 {
			return fmt.Errorf("record diagnosis evidence collected: persisted turn IDs must be non-zero for turn-bound collection: %w", domain.ErrInvariantViolation)
		}
		if req.UserSequence <= 0 || req.AssistantSequence != req.UserSequence+1 {
			return fmt.Errorf("record diagnosis evidence collected: assistant_sequence must equal user_sequence + 1 for turn-bound collection: %w", domain.ErrInvariantViolation)
		}
	}
	if req.TurnCount <= 0 {
		return fmt.Errorf("record diagnosis evidence collected: turn_count must be > 0: %w", domain.ErrInvariantViolation)
	}
	if len(req.Items) == 0 || len(req.Items) > 5 {
		return fmt.Errorf("record diagnosis evidence collected: items must contain between 1 and 5 entries: %w", domain.ErrInvariantViolation)
	}
	if req.OccurredAt.IsZero() {
		return fmt.Errorf("record diagnosis evidence collected: occurred_at must be set: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func recordDiagnosisEvidenceCollectedHasTurnPair(req RecordDiagnosisEvidenceCollectedInput) bool {
	return strings.TrimSpace(req.AssistantMessageID) != "" ||
		req.UserTurnID != 0 ||
		req.AssistantTurnID != 0 ||
		req.UserSequence != 0 ||
		req.AssistantSequence != 0
}

func validateDiagnosisRoomSupplementalEvidence(in *DiagnosisRoomSupplementalEvidence) error {
	if in == nil {
		return nil
	}
	if strings.TrimSpace(in.Label) == "" {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence label must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Label) != in.Label {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence label must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Detail) == "" {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence detail must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Detail) != in.Detail {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence detail must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Priority) == "" {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence priority must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Priority) != in.Priority {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence priority must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	switch in.Priority {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("persist diagnosis turn: supplemental evidence priority is unsupported: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(in.Evidence) == "" {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence text must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len([]byte(in.Evidence)) > diagnosisroom.HardMaxMessageBytes {
		return fmt.Errorf("persist diagnosis turn: supplemental evidence text is %d bytes, max %d: %w",
			len([]byte(in.Evidence)), diagnosisroom.HardMaxMessageBytes, domain.ErrInvariantViolation)
	}
	return nil
}

func validateCloseDiagnosisChatSessionInput(req CloseDiagnosisChatSessionInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("close diagnosis chat session: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("close diagnosis chat session: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("close diagnosis chat session: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return fmt.Errorf("close diagnosis chat session: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.ConfirmedBy) != req.ConfirmedBy {
		return fmt.Errorf("close diagnosis chat session: confirmed_by must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.TurnCount < 0 {
		return fmt.Errorf("close diagnosis chat session: turn_count must be >= 0: %w", domain.ErrInvariantViolation)
	}
	if req.ClosedAt.IsZero() {
		return fmt.Errorf("close diagnosis chat session: closed_at must be set: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("close diagnosis chat session: reason must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.CloseNotificationChannelProfileID < 0 {
		return fmt.Errorf("close diagnosis chat session: close_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateSendDiagnosisRoomAssistantTurnNotificationInput(req SendDiagnosisRoomAssistantTurnNotificationInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("send diagnosis room assistant-turn notification: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("send diagnosis room assistant-turn notification: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("send diagnosis room assistant-turn notification: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return fmt.Errorf("send diagnosis room assistant-turn notification: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantTurnID == 0 {
		return fmt.Errorf("send diagnosis room assistant-turn notification: assistant_turn_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessageID) == "" {
		return fmt.Errorf("send diagnosis room assistant-turn notification: assistant_message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessageID) != req.AssistantMessageID {
		return fmt.Errorf("send diagnosis room assistant-turn notification: assistant_message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantSequence <= 0 {
		return fmt.Errorf("send diagnosis room assistant-turn notification: assistant_sequence must be positive: %w", domain.ErrInvariantViolation)
	}
	if req.TurnCount <= 0 {
		return fmt.Errorf("send diagnosis room assistant-turn notification: turn_count must be positive: %w", domain.ErrInvariantViolation)
	}
	if req.OccurredAt.IsZero() {
		return fmt.Errorf("send diagnosis room assistant-turn notification: occurred_at must be set: %w", domain.ErrInvariantViolation)
	}
	if req.CloseNotificationChannelProfileID < 0 {
		return fmt.Errorf("send diagnosis room assistant-turn notification: close_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessage) == "" {
		return fmt.Errorf("send diagnosis room assistant-turn notification: assistant_message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateSendDiagnosisRoomFinalReadyNotificationInput(req SendDiagnosisRoomFinalReadyNotificationInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("send diagnosis room final-ready notification: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.SessionID) != req.SessionID {
		return fmt.Errorf("send diagnosis room final-ready notification: session_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("send diagnosis room final-ready notification: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.OwnerSubject) == "" {
		return fmt.Errorf("send diagnosis room final-ready notification: owner_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantTurnID == 0 {
		return fmt.Errorf("send diagnosis room final-ready notification: assistant_turn_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessageID) == "" {
		return fmt.Errorf("send diagnosis room final-ready notification: assistant_message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.AssistantMessageID) != req.AssistantMessageID {
		return fmt.Errorf("send diagnosis room final-ready notification: assistant_message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantSequence <= 0 {
		return fmt.Errorf("send diagnosis room final-ready notification: assistant_sequence must be positive: %w", domain.ErrInvariantViolation)
	}
	if req.TurnCount <= 0 {
		return fmt.Errorf("send diagnosis room final-ready notification: turn_count must be positive: %w", domain.ErrInvariantViolation)
	}
	if req.OccurredAt.IsZero() {
		return fmt.Errorf("send diagnosis room final-ready notification: occurred_at must be set: %w", domain.ErrInvariantViolation)
	}
	if req.CloseNotificationChannelProfileID < 0 {
		return fmt.Errorf("send diagnosis room final-ready notification: close_notification_channel_profile_id must be non-negative: %w", domain.ErrInvariantViolation)
	}
	if req.FinalConclusion.Status != "available" {
		return fmt.Errorf("send diagnosis room final-ready notification: final_conclusion.status must be available: %w", domain.ErrInvariantViolation)
	}
	if req.FinalConclusion.AssistantTurnID != req.AssistantTurnID {
		return fmt.Errorf("send diagnosis room final-ready notification: final_conclusion.assistant_turn_id mismatch: %w", domain.ErrInvariantViolation)
	}
	if req.FinalConclusion.AssistantMessageID != req.AssistantMessageID {
		return fmt.Errorf("send diagnosis room final-ready notification: final_conclusion.assistant_message_id mismatch: %w", domain.ErrInvariantViolation)
	}
	if req.FinalConclusion.AssistantSequence != req.AssistantSequence {
		return fmt.Errorf("send diagnosis room final-ready notification: final_conclusion.assistant_sequence mismatch: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.FinalConclusion.Content) == "" {
		return fmt.Errorf("send diagnosis room final-ready notification: final_conclusion.content must be non-empty: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateChatSessionBinding(session domain.ChatSession, sessionID string, taskID int64, ownerSubject string) error {
	if err := validateChatSessionIdentity(session, sessionID, taskID, ownerSubject); err != nil {
		return err
	}
	if session.Status.IsTerminal() {
		return fmt.Errorf("chat session binding: session %q is terminal: %w", session.SessionKey, domain.ErrInvariantViolation)
	}
	return nil
}

func validateChatSessionIdentity(session domain.ChatSession, sessionID string, taskID int64, ownerSubject string) error {
	if session.SessionKey != sessionID {
		return fmt.Errorf("chat session binding: session_key %q does not match %q: %w", session.SessionKey, sessionID, domain.ErrInvariantViolation)
	}
	if session.DiagnosisTaskID != domain.DiagnosisTaskID(taskID) {
		return fmt.Errorf("chat session binding: diagnosis_task_id %d does not match %d: %w", session.DiagnosisTaskID, taskID, domain.ErrInvariantViolation)
	}
	if session.OwnerSubject != strings.TrimSpace(ownerSubject) {
		return fmt.Errorf("chat session binding: owner_subject mismatch: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func validateDiagnosisRoomSessionTaskBinding(req EnsureDiagnosisRoomSessionInput, session domain.ChatSession, task domain.DiagnosisTask) error {
	if err := validateChatSessionBinding(session, req.SessionID, int64(task.ID), req.OwnerSubject); err != nil {
		return err
	}
	return validateDiagnosisRoomTaskBinding(req, task)
}

func validateDiagnosisRoomTaskBinding(req EnsureDiagnosisRoomSessionInput, task domain.DiagnosisTask) error {
	if task.ID == 0 {
		return fmt.Errorf("diagnosis room task binding: task id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if task.EvidenceSnapshotID != domain.EvidenceSnapshotID(req.EvidenceSnapshotID) {
		return fmt.Errorf("diagnosis room task binding: evidence_snapshot_id %d does not match %d: %w",
			task.EvidenceSnapshotID, req.EvidenceSnapshotID, domain.ErrInvariantViolation)
	}
	if task.WorkflowID != strings.TrimSpace(req.WorkflowID) {
		return fmt.Errorf("diagnosis room task binding: workflow_id %q does not match %q: %w",
			task.WorkflowID, strings.TrimSpace(req.WorkflowID), domain.ErrInvariantViolation)
	}
	if task.RunID != strings.TrimSpace(req.RunID) {
		return fmt.Errorf("diagnosis room task binding: run_id mismatch: %w", domain.ErrInvariantViolation)
	}
	if task.Status.IsTerminal() {
		return fmt.Errorf("diagnosis room task binding: task %d is terminal: %w", task.ID, domain.ErrInvariantViolation)
	}
	return nil
}

func ensureDiagnosisChatSessionResult(session domain.ChatSession, event domain.DiagnosisTaskEvent) EnsureDiagnosisChatSessionResult {
	return EnsureDiagnosisChatSessionResult{
		ChatSessionID:    int64(session.ID),
		LifecycleEventID: int64(event.ID),
		Status:           string(session.Status),
		TurnCount:        session.TurnCount,
		StartedAt:        session.StartedAt,
		LastActivityAt:   session.LastActivityAt,
	}
}

func ensureDiagnosisRoomSessionResult(
	task domain.DiagnosisTask,
	session domain.ChatSession,
	event domain.DiagnosisTaskEvent,
) EnsureDiagnosisRoomSessionResult {
	return EnsureDiagnosisRoomSessionResult{
		DiagnosisTaskID:  int64(task.ID),
		ChatSessionID:    int64(session.ID),
		LifecycleEventID: int64(event.ID),
		Status:           string(session.Status),
		TurnCount:        session.TurnCount,
		StartedAt:        session.StartedAt,
		LastActivityAt:   session.LastActivityAt,
	}
}

func persistDiagnosisTurnResult(
	session domain.ChatSession,
	userTurn domain.ChatTurn,
	assistantTurn domain.ChatTurn,
	output diagnosisroom.TurnOutput,
) PersistDiagnosisTurnResult {
	return PersistDiagnosisTurnResult{
		ChatSessionID:       int64(session.ID),
		UserTurnID:          int64(userTurn.ID),
		AssistantTurnID:     int64(assistantTurn.ID),
		AssistantMessageID:  assistantTurn.MessageID,
		AssistantSequence:   assistantTurn.Sequence,
		AssistantOccurredAt: assistantTurn.OccurredAt,
		TurnCount:           session.TurnCount,
		LastActivityAt:      session.LastActivityAt,
		AssistantMessage:    output.Message,
		Confidence:          output.Confidence,
		RequiresHumanReview: output.RequiresHumanReview,
		Findings:            cloneStrings(output.Findings),
		RecommendedActions:  cloneStrings(output.RecommendedActions),
		EvidenceRequests:    cloneEvidenceRequests(output.EvidenceRequests),
		Insight:             output.Insight(),
	}
}

func cloneEvidenceRequests(in []diagnosisroom.EvidenceRequest) []diagnosisroom.EvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisroom.EvidenceRequest, len(in))
	copy(out, in)
	return out
}

func cloneConsultationEvidenceRequests(in []diagnosisroom.ConsultationEvidenceRequest) []diagnosisroom.ConsultationEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]diagnosisroom.ConsultationEvidenceRequest, len(in))
	copy(out, in)
	return out
}

func closeDiagnosisChatSessionResult(
	session domain.ChatSession,
	event domain.DiagnosisTaskEvent,
	finalConclusion DiagnosisRoomFinalConclusion,
	summary *domain.ChatSessionSummary,
) CloseDiagnosisChatSessionResult {
	var closedAt time.Time
	if session.ClosedAt != nil {
		closedAt = *session.ClosedAt
	}
	return CloseDiagnosisChatSessionResult{
		ChatSessionID:       int64(session.ID),
		LifecycleEventID:    int64(event.ID),
		Status:              string(session.Status),
		TurnCount:           session.TurnCount,
		ClosedAt:            closedAt,
		CloseReason:         session.CloseReason,
		LastActivityAt:      session.LastActivityAt,
		FinalConclusion:     finalConclusion,
		ConversationSummary: diagnosisRoomConversationSummary(summary),
	}
}

func diagnosisRoomConversationSummary(in *domain.ChatSessionSummary) *DiagnosisRoomConversationSummary {
	if in == nil {
		return nil
	}
	return &DiagnosisRoomConversationSummary{
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

func diagnosisUserTurnMetadata(req PersistDiagnosisTurnInput) (json.RawMessage, error) {
	return marshalDiagnosisTurnMetadata(map[string]any{
		"source":               "DiagnosisRoomWorkflow",
		"kind":                 "user",
		"diagnosis_task_id":    req.DiagnosisTaskID,
		"actor_subject":        req.ActorSubject,
		"context_bytes":        req.ContextBytes,
		"assistant_message_id": req.AssistantMessageID,
	})
}

func diagnosisAssistantTurnMetadata(req PersistDiagnosisTurnInput, output diagnosisroom.TurnOutput) (json.RawMessage, error) {
	return marshalDiagnosisTurnMetadata(map[string]any{
		"source":                "DiagnosisRoomWorkflow",
		"kind":                  "assistant",
		"diagnosis_task_id":     req.DiagnosisTaskID,
		"invocation_id":         req.InvocationID,
		"runtime_id":            req.RuntimeID,
		"container_started_at":  req.ContainerStartedAt,
		"container_finished_at": req.ContainerFinishedAt,
		"context_bytes":         req.ContextBytes,
		"schema_version":        output.SchemaVersion,
		"confidence":            output.Confidence,
		"requires_human_review": output.RequiresHumanReview,
		"findings":              output.Findings,
		"recommended_actions":   output.RecommendedActions,
		"evidence_requests":     output.EvidenceRequests,
		"consultation_insight":  output.Insight(),
		"raw_output":            req.RawOutput,
	})
}

func (a *Activities) recordDiagnosisRoomOpened(ctx context.Context, req EnsureDiagnosisChatSessionInput, session domain.ChatSession) (domain.DiagnosisTaskEvent, error) {
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":              diagnosisRoomEventOpened,
		"session_id":        req.SessionID,
		"chat_session_id":   int64(session.ID),
		"diagnosis_task_id": req.DiagnosisTaskID,
		"owner_subject":     req.OwnerSubject,
		"status":            string(session.Status),
		"turn_count":        session.TurnCount,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventOpened,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventOpened, req.SessionID, "opened"),
		Payload:    payload,
		OccurredAt: req.StartedAt,
	})
}

func (a *Activities) recordDiagnosisRoomTurnPersisted(ctx context.Context, req PersistDiagnosisTurnInput, result PersistDiagnosisTurnResult) (domain.DiagnosisTaskEvent, error) {
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                  diagnosisRoomEventTurnPersisted,
		"session_id":            req.SessionID,
		"chat_session_id":       result.ChatSessionID,
		"diagnosis_task_id":     req.DiagnosisTaskID,
		"owner_subject":         req.OwnerSubject,
		"actor_subject":         req.ActorSubject,
		"user_message_id":       req.UserMessageID,
		"assistant_message_id":  req.AssistantMessageID,
		"user_turn_id":          result.UserTurnID,
		"assistant_turn_id":     result.AssistantTurnID,
		"user_sequence":         req.UserSequence,
		"assistant_sequence":    req.AssistantSequence,
		"turn_count":            result.TurnCount,
		"context_bytes":         req.ContextBytes,
		"invocation_id":         req.InvocationID,
		"confidence":            result.Confidence,
		"requires_human_review": result.RequiresHumanReview,
		"evidence_requests":     result.EvidenceRequests,
		"consultation_insight":  result.Insight,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventTurnPersisted,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventTurnPersisted, req.SessionID, req.UserMessageID),
		Payload:    payload,
		OccurredAt: req.AssistantOccurredAt,
	})
}

func (a *Activities) recordDiagnosisRoomEvidenceCollected(
	ctx context.Context,
	req RecordDiagnosisEvidenceCollectedInput,
) (domain.DiagnosisTaskEvent, error) {
	results, err := diagnosisRoomEvidenceCollectionResultPayloads(req.Items, req.OccurredAt)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	lifecyclePayload := map[string]any{
		"kind":                        diagnosisRoomEventEvidenceCollected,
		"session_id":                  req.SessionID,
		"chat_session_id":             req.ChatSessionID,
		"diagnosis_task_id":           req.DiagnosisTaskID,
		"owner_subject":               req.OwnerSubject,
		"actor_subject":               req.ActorSubject,
		"user_message_id":             req.UserMessageID,
		"turn_count":                  req.TurnCount,
		"evidence_collection_results": results,
	}
	if recordDiagnosisEvidenceCollectedHasTurnPair(req) {
		lifecyclePayload["assistant_message_id"] = req.AssistantMessageID
		lifecyclePayload["user_turn_id"] = req.UserTurnID
		lifecyclePayload["assistant_turn_id"] = req.AssistantTurnID
		lifecyclePayload["user_sequence"] = req.UserSequence
		lifecyclePayload["assistant_sequence"] = req.AssistantSequence
		lifecyclePayload["context_refs"] = diagnosisRoomTurnRefs(req.ChatSessionID, req.UserTurnID, req.AssistantTurnID)
	}
	payload, err := diagnosisRoomLifecyclePayload(lifecyclePayload)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventEvidenceCollected,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventEvidenceCollected, req.SessionID, diagnosisRoomEvidenceCollectedDedupeComponent(req)),
		Payload:    payload,
		OccurredAt: req.OccurredAt,
	})
}

func diagnosisRoomEvidenceCollectedDedupeComponent(req RecordDiagnosisEvidenceCollectedInput) string {
	if strings.TrimSpace(req.AssistantMessageID) != "" {
		return req.AssistantMessageID
	}
	return "collect:" + req.UserMessageID
}

type diagnosisRoomEvidenceCollectionResultPayload struct {
	TemplateID           int64      `json:"template_id,omitempty"`
	AlertSourceProfileID int64      `json:"alert_source_profile_id,omitempty"`
	AlertSourceKind      string     `json:"alert_source_kind,omitempty"`
	Tool                 string     `json:"tool"`
	Status               string     `json:"status"`
	ReasonCode           string     `json:"reason_code,omitempty"`
	Message              string     `json:"message,omitempty"`
	RequestReason        string     `json:"request_reason,omitempty"`
	Query                string     `json:"query,omitempty"`
	WindowSeconds        int        `json:"window_seconds,omitempty"`
	StepSeconds          int        `json:"step_seconds,omitempty"`
	Limit                int        `json:"limit,omitempty"`
	ObservedAlerts       *int       `json:"observed_alerts,omitempty"`
	ObservedMetricSeries *int       `json:"observed_metric_series,omitempty"`
	CollectedAt          *time.Time `json:"collected_at"`
}

func diagnosisRoomEvidenceCollectionResultPayloads(
	items []diagnosisevidence.Item,
	fallbackCollectedAt time.Time,
) ([]diagnosisRoomEvidenceCollectionResultPayload, error) {
	out := make([]diagnosisRoomEvidenceCollectionResultPayload, 0, len(items))
	for i, item := range items {
		tool := item.Tool
		if strings.TrimSpace(string(tool)) == "" {
			tool = item.Request.Tool
		}
		status := strings.TrimSpace(string(item.Status))
		if strings.TrimSpace(string(tool)) == "" || status == "" {
			return nil, fmt.Errorf("diagnosis room evidence collected event: items[%d] must include tool and status: %w", i, domain.ErrInvariantViolation)
		}
		collectedAt := item.CollectedAt
		if collectedAt.IsZero() {
			collectedAt = fallbackCollectedAt
		}
		if collectedAt.IsZero() {
			return nil, fmt.Errorf("diagnosis room evidence collected event: items[%d].collected_at must be set: %w", i, domain.ErrInvariantViolation)
		}
		query := strings.TrimSpace(item.Query)
		if query == "" {
			query = strings.TrimSpace(item.Request.Query)
		}
		limit := item.Limit
		if limit == 0 {
			limit = item.Request.Limit
		}
		windowSeconds := item.WindowSeconds
		if windowSeconds == 0 {
			windowSeconds = item.Request.WindowSeconds
		}
		stepSeconds := item.StepSeconds
		if stepSeconds == 0 {
			stepSeconds = item.Request.StepSeconds
		}
		payload := diagnosisRoomEvidenceCollectionResultPayload{
			TemplateID:           int64(item.TemplateID),
			AlertSourceProfileID: int64(item.AlertSourceProfileID),
			AlertSourceKind:      strings.TrimSpace(string(item.AlertSourceKind)),
			Tool:                 strings.TrimSpace(string(tool)),
			Status:               status,
			ReasonCode:           strings.TrimSpace(string(item.ReasonCode)),
			Message:              strings.TrimSpace(item.Message),
			RequestReason:        strings.TrimSpace(item.Request.Reason),
			Query:                query,
			WindowSeconds:        windowSeconds,
			StepSeconds:          stepSeconds,
			Limit:                limit,
			CollectedAt:          &collectedAt,
		}
		switch tool {
		case domain.DiagnosisToolKindActiveAlerts:
			payload.ObservedAlerts = intPtr(item.ObservedAlerts)
		case domain.DiagnosisToolKindMetricQuery, domain.DiagnosisToolKindMetricRangeQuery:
			payload.ObservedMetricSeries = intPtr(item.ObservedMetricSeries)
		}
		out = append(out, payload)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("diagnosis room evidence collected event: no collection results: %w", domain.ErrInvariantViolation)
	}
	return out, nil
}

func intPtr(value int) *int {
	return &value
}

func (a *Activities) recordDiagnosisRoomSupplementalEvidenceProvided(
	ctx context.Context,
	req PersistDiagnosisTurnInput,
	result PersistDiagnosisTurnResult,
) (domain.DiagnosisTaskEvent, error) {
	if req.SupplementalEvidence == nil {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room supplemental evidence event: supplemental_evidence must be set: %w", domain.ErrInvariantViolation)
	}
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                  diagnosisRoomEventSupplementalEvidence,
		"session_id":            req.SessionID,
		"chat_session_id":       result.ChatSessionID,
		"diagnosis_task_id":     req.DiagnosisTaskID,
		"owner_subject":         req.OwnerSubject,
		"actor_subject":         req.ActorSubject,
		"user_message_id":       req.UserMessageID,
		"assistant_message_id":  req.AssistantMessageID,
		"user_turn_id":          result.UserTurnID,
		"assistant_turn_id":     result.AssistantTurnID,
		"user_sequence":         req.UserSequence,
		"assistant_sequence":    req.AssistantSequence,
		"turn_count":            result.TurnCount,
		"context_refs":          diagnosisRoomTurnRefs(result.ChatSessionID, result.UserTurnID, result.AssistantTurnID),
		"supplemental_evidence": req.SupplementalEvidence,
		"confidence":            result.Confidence,
		"requires_human_review": result.RequiresHumanReview,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventSupplementalEvidence,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventSupplementalEvidence, req.SessionID, req.UserMessageID),
		Payload:    payload,
		OccurredAt: req.UserOccurredAt,
	})
}

func (a *Activities) recordDiagnosisRoomFinalConclusionReady(
	ctx context.Context,
	req PersistDiagnosisTurnInput,
	result PersistDiagnosisTurnResult,
) (domain.DiagnosisTaskEvent, DiagnosisRoomFinalConclusion, error) {
	task, err := a.lookupDiagnosisTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	turns, err := a.listDiagnosisRoomTurns(ctx, domain.ChatSessionID(result.ChatSessionID), result.TurnCount)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	conclusionContext, err := diagnosisRoomCloseNotificationContextFromTurns(turns, result.AssistantTurnID)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	finalConclusion := diagnosisRoomFinalConclusionFromPersistedTurn(
		req,
		result,
		task.EvidenceSnapshotID,
		diagnosisRoomTurnRefsFromTurns(domain.ChatSessionID(result.ChatSessionID), turns),
		conclusionContext,
	)
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                 diagnosisRoomEventFinalConclusionReady,
		"session_id":           req.SessionID,
		"chat_session_id":      result.ChatSessionID,
		"diagnosis_task_id":    req.DiagnosisTaskID,
		"evidence_snapshot_id": int64(task.EvidenceSnapshotID),
		"owner_subject":        req.OwnerSubject,
		"turn_count":           result.TurnCount,
		"final_conclusion":     finalConclusion,
		"conclusion_version":   "diagnosis-room-final-ready.v1",
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	event, err := a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventFinalConclusionReady,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventFinalConclusionReady, req.SessionID, req.AssistantMessageID),
		Payload:    payload,
		OccurredAt: req.AssistantOccurredAt,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	return event, finalConclusion, nil
}

func (a *Activities) recordDiagnosisRoomFailed(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
) (domain.DiagnosisTaskEvent, error) {
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                 diagnosisRoomEventFailed,
		"session_id":           req.SessionID,
		"chat_session_id":      int64(session.ID),
		"diagnosis_task_id":    req.DiagnosisTaskID,
		"evidence_snapshot_id": int64(task.EvidenceSnapshotID),
		"owner_subject":        req.OwnerSubject,
		"status":               string(task.Status),
		"failure_reason":       strings.TrimSpace(task.FailureReason),
		"close_reason":         req.Reason,
		"closed_at":            req.ClosedAt,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventFailed,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventFailed, req.SessionID, req.Reason),
		Payload:    payload,
		OccurredAt: req.ClosedAt,
	})
}

func (a *Activities) recordDiagnosisRoomClosed(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	summary *domain.ChatSessionSummary,
) (domain.DiagnosisTaskEvent, DiagnosisRoomFinalConclusion, error) {
	task, err := a.lookupDiagnosisTaskByID(ctx, domain.DiagnosisTaskID(req.DiagnosisTaskID))
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	finalConclusion, err := a.diagnosisRoomFinalConclusion(ctx, req, session, task)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                 diagnosisRoomEventClosed,
		"session_id":           req.SessionID,
		"chat_session_id":      int64(session.ID),
		"diagnosis_task_id":    req.DiagnosisTaskID,
		"evidence_snapshot_id": int64(task.EvidenceSnapshotID),
		"owner_subject":        req.OwnerSubject,
		"status":               string(session.Status),
		"turn_count":           session.TurnCount,
		"close_reason":         session.CloseReason,
		"closed_at":            session.ClosedAt,
		"final_conclusion":     finalConclusion,
		"conversation_summary": diagnosisRoomConversationSummary(summary),
		"conclusion_version":   "diagnosis-room-close.v1",
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	event, err := a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventClosed,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventClosed, req.SessionID, "closed"),
		Payload:    payload,
		OccurredAt: req.ClosedAt,
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	return event, finalConclusion, nil
}

// DiagnosisRoomFinalConclusion is the persisted close-time diagnosis summary
// derived from the latest assistant turn, when one exists.
type DiagnosisRoomFinalConclusion struct {
	Status                        string                                      `json:"status"`
	Source                        string                                      `json:"source"`
	Reason                        string                                      `json:"reason,omitempty"`
	EvidenceSnapshotID            int64                                       `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion             string                                      `json:"conclusion_version,omitempty"`
	RecordedAt                    *time.Time                                  `json:"recorded_at,omitempty"`
	ConfirmedBy                   string                                      `json:"confirmed_by,omitempty"`
	SupplementalContextRefs       []string                                    `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID               int64                                       `json:"assistant_turn_id,omitempty"`
	AssistantMessageID            string                                      `json:"assistant_message_id,omitempty"`
	AssistantSequence             int                                         `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt           *time.Time                                  `json:"assistant_occurred_at,omitempty"`
	Content                       string                                      `json:"content,omitempty"`
	Confidence                    string                                      `json:"confidence,omitempty"`
	RequiresHumanReview           *bool                                       `json:"requires_human_review,omitempty"`
	ConfidenceRationale           string                                      `json:"confidence_rationale,omitempty"`
	Findings                      []string                                    `json:"findings,omitempty"`
	RecommendedActions            []string                                    `json:"recommended_actions,omitempty"`
	EvidenceRequests              []diagnosisroom.EvidenceRequest             `json:"evidence_requests,omitempty"`
	MissingEvidenceRequests       []diagnosisroom.ConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisroom.ConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
}

type diagnosisRoomAssistantTurnMetadata struct {
	Confidence          string                            `json:"confidence"`
	RequiresHumanReview bool                              `json:"requires_human_review"`
	Findings            []string                          `json:"findings,omitempty"`
	RecommendedActions  []string                          `json:"recommended_actions,omitempty"`
	EvidenceRequests    []diagnosisroom.EvidenceRequest   `json:"evidence_requests,omitempty"`
	ConsultationInsight diagnosisroom.ConsultationInsight `json:"consultation_insight,omitempty"`
}

type diagnosisRoomCloseNotificationContext struct {
	Findings                      []string
	RecommendedActions            []string
	EvidenceRequests              []diagnosisroom.EvidenceRequest
	MissingEvidenceRequests       []diagnosisroom.ConsultationEvidenceRequest
	EvidenceCollectionSuggestions []diagnosisroom.ConsultationEvidenceRequest
	ConfidenceRationale           string
	ConclusionStatus              string
}

func diagnosisRoomFinalConclusionFromPersistedTurn(
	req PersistDiagnosisTurnInput,
	result PersistDiagnosisTurnResult,
	evidenceSnapshotID domain.EvidenceSnapshotID,
	contextRefs []string,
	conclusionContext diagnosisRoomCloseNotificationContext,
) DiagnosisRoomFinalConclusion {
	requiresHumanReview := result.RequiresHumanReview
	occurredAt := result.AssistantOccurredAt
	if occurredAt.IsZero() {
		occurredAt = req.AssistantOccurredAt
	}
	assistantMessageID := strings.TrimSpace(result.AssistantMessageID)
	if assistantMessageID == "" {
		assistantMessageID = strings.TrimSpace(req.AssistantMessageID)
	}
	assistantSequence := result.AssistantSequence
	if assistantSequence == 0 {
		assistantSequence = req.AssistantSequence
	}
	content := strings.TrimSpace(result.AssistantMessage)
	if content == "" {
		content = strings.TrimSpace(req.AssistantMessage)
	}
	return diagnosisRoomFinalConclusionWithContext(DiagnosisRoomFinalConclusion{
		Status:             "available",
		Source:             "latest_assistant_turn",
		Reason:             diagnosisRoomFinalConclusionReadyReason(result.Insight.ConclusionStatus),
		EvidenceSnapshotID: int64(evidenceSnapshotID),
		ConclusionVersion:  "diagnosis-room-final-ready.v1",
		RecordedAt:         &occurredAt,
		SupplementalContextRefs: nonEmptyDiagnosisRoomTurnRefs(
			contextRefs,
			diagnosisRoomTurnRefs(result.ChatSessionID, result.UserTurnID, result.AssistantTurnID),
		),
		AssistantTurnID:     result.AssistantTurnID,
		AssistantMessageID:  assistantMessageID,
		AssistantSequence:   assistantSequence,
		AssistantOccurredAt: &occurredAt,
		Content:             truncateString(content, diagnosisRoomFinalConclusionMaxRunes),
		Confidence:          strings.TrimSpace(result.Confidence),
		RequiresHumanReview: &requiresHumanReview,
	}, conclusionContext)
}

func diagnosisRoomFinalConclusionReadyStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "final", "ready_for_review":
		return true
	default:
		return false
	}
}

func diagnosisRoomFinalConclusionReadyReason(status string) string {
	if strings.TrimSpace(status) == "ready_for_review" {
		return "assistant_marked_ready_for_review"
	}
	return "assistant_marked_final"
}

func (a *Activities) diagnosisRoomFinalConclusion(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
) (DiagnosisRoomFinalConclusion, error) {
	recordedAt := req.ClosedAt
	if recordedAt.IsZero() {
		recordedAt = session.LastActivityAt
	}
	confirmedBy := confirmedByForActor(req.ConfirmedBy)
	if session.TurnCount == 0 {
		return DiagnosisRoomFinalConclusion{
			Status:             "not_available",
			Source:             "none",
			Reason:             "room_closed_without_assistant_turn",
			EvidenceSnapshotID: int64(task.EvidenceSnapshotID),
			ConclusionVersion:  "diagnosis-room-close.v1",
			RecordedAt:         &recordedAt,
			ConfirmedBy:        confirmedBy,
		}, nil
	}

	turns, err := a.listDiagnosisRoomTurns(ctx, session.ID, session.TurnCount)
	if err != nil {
		return DiagnosisRoomFinalConclusion{}, err
	}
	for i := len(turns) - 1; i >= 0; i-- {
		turn := turns[i]
		if turn.Role != domain.ChatRoleAssistant {
			continue
		}
		var metadata diagnosisRoomAssistantTurnMetadata
		if err := json.Unmarshal(turn.Metadata, &metadata); err != nil {
			return DiagnosisRoomFinalConclusion{}, fmt.Errorf("diagnosis room final conclusion: parse assistant metadata for turn %d: %w", turn.ID, err)
		}
		requiresHumanReview := metadata.RequiresHumanReview
		occurredAt := turn.OccurredAt
		return diagnosisRoomFinalConclusionWithContext(DiagnosisRoomFinalConclusion{
			Status:                  "available",
			Source:                  "latest_assistant_turn",
			EvidenceSnapshotID:      int64(task.EvidenceSnapshotID),
			ConclusionVersion:       "diagnosis-room-close.v1",
			RecordedAt:              &recordedAt,
			ConfirmedBy:             confirmedBy,
			SupplementalContextRefs: diagnosisRoomTurnRefsFromTurns(session.ID, turns),
			AssistantTurnID:         int64(turn.ID),
			AssistantMessageID:      turn.MessageID,
			AssistantSequence:       turn.Sequence,
			AssistantOccurredAt:     &occurredAt,
			Content:                 truncateString(turn.Content, diagnosisRoomFinalConclusionMaxRunes),
			Confidence:              strings.TrimSpace(metadata.Confidence),
			RequiresHumanReview:     &requiresHumanReview,
		}, diagnosisRoomCloseNotificationContextFromMetadata(metadata)), nil
	}
	return DiagnosisRoomFinalConclusion{}, fmt.Errorf("diagnosis room final conclusion: no assistant turn found for non-empty session %q: %w",
		req.SessionID, domain.ErrInvariantViolation)
}

func diagnosisRoomFinalConclusionWithContext(
	finalConclusion DiagnosisRoomFinalConclusion,
	ctx diagnosisRoomCloseNotificationContext,
) DiagnosisRoomFinalConclusion {
	finalConclusion.ConfidenceRationale = strings.TrimSpace(ctx.ConfidenceRationale)
	finalConclusion.Findings = cloneStrings(ctx.Findings)
	finalConclusion.RecommendedActions = cloneStrings(ctx.RecommendedActions)
	finalConclusion.EvidenceRequests = cloneEvidenceRequests(ctx.EvidenceRequests)
	finalConclusion.MissingEvidenceRequests = cloneConsultationEvidenceRequests(ctx.MissingEvidenceRequests)
	finalConclusion.EvidenceCollectionSuggestions = cloneConsultationEvidenceRequests(ctx.EvidenceCollectionSuggestions)
	return finalConclusion
}

func (a *Activities) diagnosisRoomCloseNotificationContext(
	ctx context.Context,
	session domain.ChatSession,
	finalConclusion DiagnosisRoomFinalConclusion,
) (diagnosisRoomCloseNotificationContext, error) {
	if finalConclusion.Status != "available" || finalConclusion.AssistantTurnID == 0 {
		return diagnosisRoomCloseNotificationContext{}, nil
	}
	turns, err := a.listDiagnosisRoomTurns(ctx, session.ID, session.TurnCount)
	if err != nil {
		return diagnosisRoomCloseNotificationContext{}, err
	}
	return diagnosisRoomCloseNotificationContextFromTurns(turns, finalConclusion.AssistantTurnID)
}

func diagnosisRoomCloseNotificationContextFromTurns(
	turns []domain.ChatTurn,
	assistantTurnID int64,
) (diagnosisRoomCloseNotificationContext, error) {
	for i := len(turns) - 1; i >= 0; i-- {
		turn := turns[i]
		if int64(turn.ID) != assistantTurnID {
			continue
		}
		var metadata diagnosisRoomAssistantTurnMetadata
		if err := json.Unmarshal(turn.Metadata, &metadata); err != nil {
			return diagnosisRoomCloseNotificationContext{}, fmt.Errorf("diagnosis room close notification: parse assistant metadata for turn %d: %w", turn.ID, err)
		}
		return diagnosisRoomCloseNotificationContextFromMetadata(metadata), nil
	}
	return diagnosisRoomCloseNotificationContext{}, fmt.Errorf("diagnosis room close notification: assistant turn %d was not found: %w",
		assistantTurnID, domain.ErrInvariantViolation)
}

func diagnosisRoomCloseNotificationContextFromMetadata(metadata diagnosisRoomAssistantTurnMetadata) diagnosisRoomCloseNotificationContext {
	return diagnosisRoomCloseNotificationContext{
		Findings:                      cloneStrings(metadata.Findings),
		RecommendedActions:            cloneStrings(metadata.RecommendedActions),
		EvidenceRequests:              cloneEvidenceRequests(metadata.EvidenceRequests),
		MissingEvidenceRequests:       cloneConsultationEvidenceRequests(metadata.ConsultationInsight.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: cloneConsultationEvidenceRequests(metadata.ConsultationInsight.EvidenceCollectionSuggestions),
		ConfidenceRationale:           strings.TrimSpace(metadata.ConsultationInsight.ConfidenceRationale),
		ConclusionStatus:              strings.TrimSpace(metadata.ConsultationInsight.ConclusionStatus),
	}
}

func confirmedByForActor(actorSubject string) string {
	actorSubject = strings.TrimSpace(actorSubject)
	if actorSubject == "" || actorSubject == diagnosisRoomAutoActorSubject {
		return ""
	}
	return actorSubject
}

func diagnosisRoomTurnRefs(chatSessionID int64, turnIDs ...int64) []string {
	if chatSessionID <= 0 {
		return nil
	}
	refs := make([]string, 0, len(turnIDs))
	seen := make(map[int64]struct{}, len(turnIDs))
	for _, turnID := range turnIDs {
		if turnID <= 0 {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		refs = append(refs, fmt.Sprintf("chat_session:%d/turn:%d", chatSessionID, turnID))
	}
	return refs
}

func diagnosisRoomTurnRefsFromTurns(chatSessionID domain.ChatSessionID, turns []domain.ChatTurn) []string {
	ids := make([]int64, 0, len(turns))
	for _, turn := range turns {
		if turn.ID > 0 {
			ids = append(ids, int64(turn.ID))
		}
	}
	return diagnosisRoomTurnRefs(int64(chatSessionID), ids...)
}

func nonEmptyDiagnosisRoomTurnRefs(primary []string, fallback []string) []string {
	if len(primary) > 0 {
		return append([]string(nil), primary...)
	}
	return append([]string(nil), fallback...)
}

func (a *Activities) listDiagnosisRoomTurns(ctx context.Context, sessionID domain.ChatSessionID, turnCount int) ([]domain.ChatTurn, error) {
	if turnCount < 0 {
		return nil, fmt.Errorf("diagnosis room final conclusion: turn_count must be >= 0: %w", domain.ErrInvariantViolation)
	}
	limit := turnCount*2 + 2
	if limit <= 0 {
		limit = 2
	}
	var turns []domain.ChatTurn
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().ListChatTurnsBySession(ctx, sessionID, limit)
		if err != nil {
			return err
		}
		turns = out
		return nil
	})
	return turns, err
}

func (a *Activities) recordDiagnosisRoomCloseNotificationSent(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	finalConclusion DiagnosisRoomFinalConclusion,
	idempotencyKey string,
	dedupeComponent string,
	roomURL string,
	delivery ports.IMDelivery,
) (domain.DiagnosisTaskEvent, error) {
	payloadMap := map[string]any{
		"kind":                            diagnosisRoomEventCloseNotificationSent,
		"session_id":                      req.SessionID,
		"chat_session_id":                 int64(session.ID),
		"diagnosis_task_id":               req.DiagnosisTaskID,
		"evidence_snapshot_id":            int64(task.EvidenceSnapshotID),
		"alert_group_id":                  int64(snapshot.AlertGroupID),
		"owner_subject":                   req.OwnerSubject,
		"turn_count":                      session.TurnCount,
		"close_reason":                    session.CloseReason,
		"idempotency_key":                 idempotencyKey,
		"notification_channel_profile_id": req.CloseNotificationChannelProfileID,
		"provider_message_id":             truncateString(delivery.ProviderMessageID, 256),
		"provider_status":                 truncateString(delivery.Status, 64),
		"provider_raw":                    defaultJSONObject(delivery.Raw),
		"final_conclusion":                finalConclusion,
	}
	setDiagnosisRoomNotificationURL(payloadMap, roomURL)
	payload, err := diagnosisRoomLifecyclePayload(payloadMap)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventCloseNotificationSent,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventCloseNotificationSent, req.SessionID, dedupeComponent),
		Payload:    payload,
		OccurredAt: req.ClosedAt.Add(time.Microsecond),
	})
}

func (a *Activities) recordDiagnosisRoomAssistantTurnNotificationSent(
	ctx context.Context,
	req SendDiagnosisRoomAssistantTurnNotificationInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	idempotencyKey string,
	dedupeComponent string,
	roomURL string,
	delivery ports.IMDelivery,
) (domain.DiagnosisTaskEvent, error) {
	payloadMap := map[string]any{
		"kind":                            diagnosisRoomEventAssistantTurnNotification,
		"session_id":                      req.SessionID,
		"chat_session_id":                 int64(session.ID),
		"diagnosis_task_id":               req.DiagnosisTaskID,
		"evidence_snapshot_id":            int64(task.EvidenceSnapshotID),
		"alert_group_id":                  int64(snapshot.AlertGroupID),
		"owner_subject":                   req.OwnerSubject,
		"assistant_message_id":            req.AssistantMessageID,
		"assistant_turn_id":               req.AssistantTurnID,
		"assistant_sequence":              req.AssistantSequence,
		"turn_count":                      req.TurnCount,
		"idempotency_key":                 idempotencyKey,
		"notification_channel_profile_id": req.CloseNotificationChannelProfileID,
		"provider_message_id":             truncateString(delivery.ProviderMessageID, 256),
		"provider_status":                 truncateString(delivery.Status, 64),
		"provider_raw":                    defaultJSONObject(delivery.Raw),
		"assistant_message":               truncateString(strings.TrimSpace(req.AssistantMessage), diagnosisRoomFinalConclusionMaxRunes),
		"confidence":                      strings.TrimSpace(req.Confidence),
		"requires_human_review":           req.RequiresHumanReview,
		"findings":                        cloneStrings(req.Findings),
		"recommended_actions":             cloneStrings(req.RecommendedActions),
		"evidence_requests":               cloneEvidenceRequests(req.EvidenceRequests),
		"consultation_insight":            req.Insight,
	}
	setDiagnosisRoomNotificationURL(payloadMap, roomURL)
	payload, err := diagnosisRoomLifecyclePayload(payloadMap)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventAssistantTurnNotification,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventAssistantTurnNotification, req.SessionID, dedupeComponent),
		Payload:    payload,
		OccurredAt: req.OccurredAt.Add(time.Microsecond),
	})
}

func (a *Activities) recordDiagnosisRoomFinalReadyNotificationSent(
	ctx context.Context,
	req SendDiagnosisRoomFinalReadyNotificationInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	idempotencyKey string,
	dedupeComponent string,
	roomURL string,
	delivery ports.IMDelivery,
) (domain.DiagnosisTaskEvent, error) {
	payloadMap := map[string]any{
		"kind":                            diagnosisRoomEventFinalReadyNotificationSent,
		"session_id":                      req.SessionID,
		"chat_session_id":                 int64(session.ID),
		"diagnosis_task_id":               req.DiagnosisTaskID,
		"evidence_snapshot_id":            int64(task.EvidenceSnapshotID),
		"alert_group_id":                  int64(snapshot.AlertGroupID),
		"owner_subject":                   req.OwnerSubject,
		"assistant_message_id":            req.AssistantMessageID,
		"assistant_turn_id":               req.AssistantTurnID,
		"assistant_sequence":              req.AssistantSequence,
		"turn_count":                      req.TurnCount,
		"idempotency_key":                 idempotencyKey,
		"notification_channel_profile_id": req.CloseNotificationChannelProfileID,
		"provider_message_id":             truncateString(delivery.ProviderMessageID, 256),
		"provider_status":                 truncateString(delivery.Status, 64),
		"provider_raw":                    defaultJSONObject(delivery.Raw),
		"final_conclusion":                req.FinalConclusion,
	}
	setDiagnosisRoomNotificationURL(payloadMap, roomURL)
	payload, err := diagnosisRoomLifecyclePayload(payloadMap)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventFinalReadyNotificationSent,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventFinalReadyNotificationSent, req.SessionID, dedupeComponent),
		Payload:    payload,
		OccurredAt: req.OccurredAt.Add(time.Microsecond),
	})
}

type diagnosisRoomLifecycleEventInput struct {
	TaskID     int64
	Kind       string
	DedupeKey  string
	Payload    json.RawMessage
	OccurredAt time.Time
}

func (a *Activities) appendDiagnosisRoomLifecycleEvent(ctx context.Context, req diagnosisRoomLifecycleEventInput) (domain.DiagnosisTaskEvent, error) {
	if req.TaskID == 0 {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room lifecycle event: task id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room lifecycle event: kind must be non-empty: %w", domain.ErrInvariantViolation)
	}
	dedupeKey := strings.TrimSpace(req.DedupeKey)
	if dedupeKey == "" {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room lifecycle event: dedupe key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.OccurredAt.IsZero() {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room lifecycle event: occurred_at must be set: %w", domain.ErrInvariantViolation)
	}
	if len(req.Payload) > 0 {
		if err := strictjson.RejectDuplicateObjectKeys(req.Payload); err != nil {
			return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis room lifecycle event: payload must be valid duplicate-key-free JSON: %w: %w", err, domain.ErrInvariantViolation)
		}
	}
	event, err := domain.NewDiagnosisTaskEvent(
		domain.DiagnosisTaskID(req.TaskID),
		kind,
		req.Payload,
		&dedupeKey,
		req.OccurredAt,
	)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	var saved domain.DiagnosisTaskEvent
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().AppendEvent(ctx, event)
		if err != nil {
			return err
		}
		saved = out
		return nil
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.DiagnosisTaskEvent{}, err
	}
	err = a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, domain.DiagnosisTaskID(req.TaskID), dedupeKey)
		if err != nil {
			return err
		}
		if out.Kind != kind {
			return fmt.Errorf("diagnosis room lifecycle event: existing dedupe key %q has kind %q, want %q: %w",
				dedupeKey, out.Kind, kind, domain.ErrInvariantViolation)
		}
		saved = out
		return nil
	})
	return saved, err
}

func diagnosisRoomLifecyclePayload(payload map[string]any) (json.RawMessage, error) {
	out := make(map[string]any, len(payload)+1)
	out["source"] = "DiagnosisRoomWorkflow"
	for key, value := range payload {
		out[key] = value
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnosis room lifecycle payload: %w", err)
	}
	return raw, nil
}

func setDiagnosisRoomNotificationURL(payload map[string]any, roomURL string) {
	roomURL = strings.TrimSpace(roomURL)
	if roomURL != "" {
		payload["room_url"] = roomURL
	}
}

func diagnosisRoomEventDedupeKey(kind, sessionID, component string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + sessionID + "\x00" + component))
	return "dr:" + diagnosisRoomEventDedupePrefix(kind) + ":" + hex.EncodeToString(sum[:])[:24]
}

func diagnosisRoomEventDedupePrefix(kind string) string {
	switch kind {
	case diagnosisRoomEventOpened:
		return "open"
	case diagnosisRoomEventTurnPersisted:
		return "turn"
	case diagnosisRoomEventFinalConclusionReady:
		return "final"
	case diagnosisRoomEventFailed:
		return "failed"
	case diagnosisRoomEventClosed:
		return "close"
	case diagnosisRoomEventCloseNotificationSent:
		return "notify"
	case diagnosisRoomEventFinalReadyNotificationSent:
		return "finalnotify"
	case diagnosisRoomEventAssistantTurnNotification:
		return "turnnotify"
	default:
		return "event"
	}
}

func (a *Activities) findCompletedDiagnosisRoomNotificationEvent(
	ctx context.Context,
	taskID int64,
	kind string,
	idempotencyKey string,
) (domain.DiagnosisTaskEvent, bool, string, error) {
	events, err := a.listDiagnosisRoomNotificationEvents(ctx, taskID, kind)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, false, "", err
	}
	attempts := 0
	for _, event := range events {
		eventIDKey, providerStatus, ok, err := diagnosisRoomNotificationEventState(event, kind)
		if err != nil {
			return domain.DiagnosisTaskEvent{}, false, "", err
		}
		if !ok || eventIDKey != idempotencyKey {
			continue
		}
		attempts++
		if !diagnosisRoomNotificationDeliveryFailed(providerStatus) {
			return event, true, "", nil
		}
	}
	return domain.DiagnosisTaskEvent{}, false, diagnosisRoomNotificationDedupeComponent(idempotencyKey, attempts), nil
}

func (a *Activities) listDiagnosisRoomNotificationEvents(ctx context.Context, taskID int64, kind string) ([]domain.DiagnosisTaskEvent, error) {
	var events []domain.DiagnosisTaskEvent
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().ListEventsByTaskAndKind(ctx, domain.DiagnosisTaskID(taskID), kind, diagnosisRoomNotificationEventLookupLimit)
		if err != nil {
			return err
		}
		events = out
		return nil
	})
	return events, err
}

func diagnosisRoomNotificationEventState(event domain.DiagnosisTaskEvent, kind string) (string, string, bool, error) {
	if len(event.Payload) == 0 {
		return "", "", false, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return "", "", false, fmt.Errorf("diagnosis room notification event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload struct {
		Kind           string `json:"kind"`
		IdempotencyKey string `json:"idempotency_key"`
		ProviderStatus string `json:"provider_status"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", "", false, fmt.Errorf("diagnosis room notification event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != kind {
		return "", "", false, fmt.Errorf("diagnosis room notification event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	idempotencyKey := strings.TrimSpace(payload.IdempotencyKey)
	if idempotencyKey == "" {
		return "", "", false, nil
	}
	return idempotencyKey, strings.TrimSpace(payload.ProviderStatus), true, nil
}

func diagnosisRoomNotificationDedupeComponent(idempotencyKey string, priorAttempts int) string {
	if priorAttempts <= 0 {
		return idempotencyKey
	}
	return fmt.Sprintf("%s/%s-%d", idempotencyKey, diagnosisRoomNotificationRetryDedupeComponent, priorAttempts)
}

func diagnosisRoomCloseNotificationIdempotencyKey(req CloseDiagnosisChatSessionInput, session domain.ChatSession) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", req.SessionID, session.ID, session.CloseReason)))
	return fmt.Sprintf("diagnosis_room:%d:%s/close_notification", req.DiagnosisTaskID, hex.EncodeToString(sum[:])[:24])
}

func diagnosisRoomFinalReadyNotificationIdempotencyKey(req SendDiagnosisRoomFinalReadyNotificationInput) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", req.SessionID, req.DiagnosisTaskID, req.AssistantMessageID)))
	return fmt.Sprintf("diagnosis_room:%d:%s/final_ready_notification", req.DiagnosisTaskID, hex.EncodeToString(sum[:])[:24])
}

func diagnosisRoomAssistantTurnNotificationIdempotencyKey(req SendDiagnosisRoomAssistantTurnNotificationInput) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", req.SessionID, req.DiagnosisTaskID, req.AssistantMessageID)))
	return fmt.Sprintf("diagnosis_room:%d:%s/assistant_turn_notification", req.DiagnosisTaskID, hex.EncodeToString(sum[:])[:24])
}

func diagnosisRoomAssistantTurnNotification(
	req SendDiagnosisRoomAssistantTurnNotificationInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	idempotencyKey string,
	roomURL string,
) ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey:        idempotencyKey,
		DiagnosisTaskID:       int64(task.ID),
		NotificationChannelID: req.CloseNotificationChannelProfileID,
		CorrelationKey:        fmt.Sprintf("alert_group:%d", snapshot.AlertGroupID),
		Title:                 fmt.Sprintf("%s: %s", diagnosisRoomAssistantTurnNotificationTitlePrefix(req), req.SessionID),
		Body:                  diagnosisRoomAssistantTurnNotificationBody(req, session, snapshot, roomURL),
		Severity:              diagnosisRoomAssistantTurnNotificationSeverity(req),
	}
}

func diagnosisRoomAssistantTurnNotificationBody(
	req SendDiagnosisRoomAssistantTurnNotificationInput,
	session domain.ChatSession,
	snapshot domain.EvidenceSnapshot,
	roomURL string,
) string {
	notificationContext := diagnosisRoomNotificationContextFromAssistantTurn(req)
	lines := []string{
		diagnosisRoomAssistantTurnNotificationOpeningLine(req, session, snapshot),
	}
	lines = append(lines, diagnosisRoomNotificationReviewLines(roomURL)...)
	if confidence := strings.TrimSpace(req.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if req.RequiresHumanReview {
		lines = append(lines, "Human review: required")
	} else {
		lines = append(lines, "Human review: not required")
	}
	if conclusionStatus := strings.TrimSpace(req.Insight.ConclusionStatus); conclusionStatus != "" {
		lines = append(lines, "Conclusion status: "+conclusionStatus)
	}
	lines = append(lines, diagnosisRoomNotificationNextActionLine(notificationContext, false))
	lines = append(lines, diagnosisRoomEvidenceCollectionPriorityLines(notificationContext)...)
	content := strings.TrimSpace(req.AssistantMessage)
	if content == "" {
		content = "Assistant diagnosis is empty."
	}
	lines = append(lines, "AI diagnosis: "+truncateString(content, 1600))
	if rationale := strings.TrimSpace(notificationContext.ConfidenceRationale); rationale != "" {
		lines = append(lines, "Confidence rationale: "+truncateString(rationale, 500))
	}
	lines = append(lines, diagnosisRoomCloseNotificationItems("Findings", notificationContext.Findings, 500)...)
	lines = append(lines, diagnosisRoomCloseNotificationItems("Recommended actions", notificationContext.RecommendedActions, 700)...)
	return strings.Join(lines, "\n")
}

func diagnosisRoomAssistantTurnNotificationTitlePrefix(req SendDiagnosisRoomAssistantTurnNotificationInput) string {
	if req.TurnCount == 1 {
		return "Initial AI diagnosis report"
	}
	return "AI diagnosis update"
}

func diagnosisRoomAssistantTurnNotificationOpeningLine(
	req SendDiagnosisRoomAssistantTurnNotificationInput,
	session domain.ChatSession,
	snapshot domain.EvidenceSnapshot,
) string {
	if req.TurnCount == 1 {
		return fmt.Sprintf(
			"Initial AI diagnosis report is ready for room %s on alert group %d after %d turn(s). Review the report, collect missing evidence, and provide supplemental context before confidence is raised or closure is confirmed.",
			req.SessionID,
			snapshot.AlertGroupID,
			session.TurnCount,
		)
	}
	return fmt.Sprintf(
		"AI diagnosis update is ready for room %s on alert group %d after %d turn(s). Review the diagnosis, collect missing evidence, and provide supplemental context before confirming closure.",
		req.SessionID,
		snapshot.AlertGroupID,
		session.TurnCount,
	)
}

func diagnosisRoomFinalReadyNotification(
	req SendDiagnosisRoomFinalReadyNotificationInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	idempotencyKey string,
	roomURL string,
) ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey:        idempotencyKey,
		DiagnosisTaskID:       int64(task.ID),
		NotificationChannelID: req.CloseNotificationChannelProfileID,
		CorrelationKey:        fmt.Sprintf("alert_group:%d", snapshot.AlertGroupID),
		Title:                 fmt.Sprintf("AI diagnosis ready: %s", req.SessionID),
		Body:                  diagnosisRoomFinalReadyNotificationBody(req, session, snapshot, roomURL),
		Severity:              diagnosisRoomCloseNotificationSeverity(req.FinalConclusion),
	}
}

func diagnosisRoomFinalReadyNotificationBody(
	req SendDiagnosisRoomFinalReadyNotificationInput,
	session domain.ChatSession,
	snapshot domain.EvidenceSnapshot,
	roomURL string,
) string {
	finalConclusion := req.FinalConclusion
	notificationContext := diagnosisRoomNotificationContextFromFinalConclusion(finalConclusion)
	lines := []string{
		fmt.Sprintf(
			"AI diagnosis is ready for room %s on alert group %d after %d turn(s). Review the conclusion, provide missing evidence if needed, then confirm closure when ready.",
			req.SessionID,
			snapshot.AlertGroupID,
			session.TurnCount,
		),
	}
	lines = append(lines, diagnosisRoomNotificationReviewLines(roomURL)...)
	if confidence := strings.TrimSpace(finalConclusion.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if finalConclusion.RequiresHumanReview != nil {
		if *finalConclusion.RequiresHumanReview {
			lines = append(lines, "Human review: required")
		} else {
			lines = append(lines, "Human review: not required")
		}
	}
	lines = append(lines, diagnosisRoomNotificationNextActionLine(notificationContext, true))
	lines = append(lines, diagnosisRoomEvidenceCollectionPriorityLines(notificationContext)...)
	content := strings.TrimSpace(finalConclusion.Content)
	if content == "" {
		content = "Assistant conclusion is empty."
	}
	lines = append(lines, "AI conclusion: "+truncateString(content, 1600))
	if rationale := strings.TrimSpace(notificationContext.ConfidenceRationale); rationale != "" {
		lines = append(lines, "Confidence rationale: "+truncateString(rationale, 500))
	}
	lines = append(lines, diagnosisRoomCloseNotificationItems("Findings", notificationContext.Findings, 500)...)
	lines = append(lines, diagnosisRoomCloseNotificationItems("Recommended actions", notificationContext.RecommendedActions, 700)...)
	if len(finalConclusion.SupplementalContextRefs) > 0 {
		lines = append(lines, fmt.Sprintf("Evidence context refs: %d", len(finalConclusion.SupplementalContextRefs)))
	}
	return strings.Join(lines, "\n")
}

func diagnosisRoomCloseNotification(
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	finalConclusion DiagnosisRoomFinalConclusion,
	notificationContext diagnosisRoomCloseNotificationContext,
	idempotencyKey string,
	roomURL string,
) ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey:        idempotencyKey,
		DiagnosisTaskID:       int64(task.ID),
		NotificationChannelID: req.CloseNotificationChannelProfileID,
		CorrelationKey:        fmt.Sprintf("alert_group:%d", snapshot.AlertGroupID),
		Title:                 fmt.Sprintf("Diagnosis room closed: %s", req.SessionID),
		Body:                  diagnosisRoomCloseNotificationBody(req, session, snapshot, finalConclusion, notificationContext, roomURL),
		Severity:              diagnosisRoomCloseNotificationSeverity(finalConclusion),
	}
}

func diagnosisRoomCloseNotificationBody(
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	snapshot domain.EvidenceSnapshot,
	finalConclusion DiagnosisRoomFinalConclusion,
	notificationContext diagnosisRoomCloseNotificationContext,
	roomURL string,
) string {
	reason := strings.TrimSpace(session.CloseReason)
	if reason == "" {
		reason = strings.TrimSpace(req.Reason)
	}
	lines := []string{
		fmt.Sprintf(
			"Diagnosis room %s for alert group %d closed with reason %q after %d turn(s).",
			req.SessionID,
			snapshot.AlertGroupID,
			reason,
			session.TurnCount,
		),
	}
	lines = append(lines, diagnosisRoomNotificationReviewLines(roomURL)...)
	if finalConclusion.Status != "available" {
		detail := strings.TrimSpace(finalConclusion.Reason)
		if detail == "" {
			detail = "no final assistant conclusion was available"
		}
		lines = append(lines, "AI conclusion: unavailable - "+detail+".")
		return strings.Join(lines, "\n")
	}
	if confidence := strings.TrimSpace(finalConclusion.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if finalConclusion.RequiresHumanReview != nil {
		if *finalConclusion.RequiresHumanReview {
			lines = append(lines, "Human review: required")
		} else {
			lines = append(lines, "Human review: not required")
		}
	}
	lines = append(lines, diagnosisRoomNotificationNextActionLine(notificationContext, true))
	lines = append(lines, diagnosisRoomEvidenceCollectionPriorityLines(notificationContext)...)
	content := strings.TrimSpace(finalConclusion.Content)
	if content == "" {
		content = "Final assistant conclusion is empty."
	}
	lines = append(lines, "AI conclusion: "+truncateString(content, 1600))
	if rationale := strings.TrimSpace(notificationContext.ConfidenceRationale); rationale != "" {
		lines = append(lines, "Confidence rationale: "+truncateString(rationale, 500))
	}
	lines = append(lines, diagnosisRoomCloseNotificationItems("Findings", notificationContext.Findings, 500)...)
	lines = append(lines, diagnosisRoomCloseNotificationItems("Recommended actions", notificationContext.RecommendedActions, 700)...)
	if len(finalConclusion.SupplementalContextRefs) > 0 {
		lines = append(lines, fmt.Sprintf("Evidence context refs: %d", len(finalConclusion.SupplementalContextRefs)))
	}
	return strings.Join(lines, "\n")
}

func diagnosisRoomEvidenceCollectionPriorityLines(ctx diagnosisRoomCloseNotificationContext) []string {
	return diagnosisRoomCloseNotificationEvidenceRequests(ctx)
}

func diagnosisRoomNotificationNextActionLine(ctx diagnosisRoomCloseNotificationContext, finalReady bool) string {
	executableCount := len(ctx.EvidenceRequests)
	missingCount := len(ctx.MissingEvidenceRequests)
	switch {
	case executableCount > 0 && missingCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s) and provide %d operator-supplied evidence item(s).", executableCount, missingCount)
	case executableCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s) in the diagnosis room.", executableCount)
	case missingCount > 0:
		return fmt.Sprintf("Next action: provide %d operator-supplied evidence item(s) before confidence can be raised.", missingCount)
	case len(ctx.EvidenceCollectionSuggestions) > 0:
		return fmt.Sprintf("Next action: review %d evidence collection suggestion(s) and decide what to collect next.", len(ctx.EvidenceCollectionSuggestions))
	case finalReady:
		return "Next action: review the final conclusion and confirm closure when operationally accepted."
	default:
		return "Next action: review the AI diagnosis and continue the consultation if more evidence is needed."
	}
}

func diagnosisRoomCloseNotificationItems(title string, items []string, maxRunes int) []string {
	const limit = 3
	out := make([]string, 0, smallerInt(len(items), limit)+1)
	count := 0
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if count == 0 {
			out = append(out, title+":")
		}
		count++
		out = append(out, fmt.Sprintf("%d. %s", count, truncateString(item, maxRunes)))
		if count == limit {
			break
		}
	}
	return out
}

func smallerInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func diagnosisRoomCloseNotificationEvidenceRequests(ctx diagnosisRoomCloseNotificationContext) []string {
	var out []string
	if len(ctx.MissingEvidenceRequests) > 0 {
		out = append(out, "Missing evidence:")
		for i, item := range ctx.MissingEvidenceRequests {
			if i == 3 {
				break
			}
			out = append(out, fmt.Sprintf("%d. [%s] %s - %s", i+1, strings.TrimSpace(item.Priority), truncateString(strings.TrimSpace(item.Label), 120), truncateString(strings.TrimSpace(item.Detail), 500)))
		}
	}
	if len(ctx.EvidenceCollectionSuggestions) > 0 {
		out = append(out, "Evidence collection suggestions:")
		for i, item := range ctx.EvidenceCollectionSuggestions {
			if i == 3 {
				break
			}
			out = append(out, fmt.Sprintf("%d. [%s] %s - %s", i+1, strings.TrimSpace(item.Priority), truncateString(strings.TrimSpace(item.Label), 120), truncateString(strings.TrimSpace(item.Detail), 500)))
		}
	}
	if len(ctx.EvidenceRequests) > 0 {
		out = append(out, fmt.Sprintf("Executable evidence requests: %d", len(ctx.EvidenceRequests)))
		for i, item := range ctx.EvidenceRequests {
			if i == 3 {
				break
			}
			out = append(out, diagnosisRoomExecutableEvidenceRequestLine(i+1, item))
		}
	}
	return out
}

func diagnosisRoomNotificationReviewLines(roomURL string) []string {
	roomURL = strings.TrimSpace(roomURL)
	if roomURL == "" {
		return nil
	}
	return []string{"Review room: " + roomURL}
}

func diagnosisRoomExecutableEvidenceRequestLine(index int, req diagnosisroom.EvidenceRequest) string {
	tool := strings.TrimSpace(string(req.Tool))
	if tool == "" {
		tool = "unknown_tool"
	}
	main := tool
	if reason := strings.TrimSpace(req.Reason); reason != "" {
		main += " - " + truncateString(reason, 300)
	}
	details := make([]string, 0, 6)
	if query := strings.TrimSpace(req.Query); query != "" {
		details = append(details, "query="+truncateString(query, 240))
	}
	if req.WindowSeconds > 0 {
		details = append(details, fmt.Sprintf("window=%ds", req.WindowSeconds))
	}
	if req.StepSeconds > 0 {
		details = append(details, fmt.Sprintf("step=%ds", req.StepSeconds))
	}
	if req.Limit > 0 {
		details = append(details, fmt.Sprintf("limit=%d", req.Limit))
	}
	if req.TemplateID > 0 {
		details = append(details, fmt.Sprintf("template_id=%d", req.TemplateID))
	}
	if req.AlertSourceProfileID > 0 {
		details = append(details, fmt.Sprintf("source_profile_id=%d", req.AlertSourceProfileID))
	}
	line := fmt.Sprintf("%d. %s", index, main)
	if len(details) > 0 {
		line += " (" + strings.Join(details, ", ") + ")"
	}
	return line
}

func (a *Activities) diagnosisRoomPublicURL(sessionID string) string {
	if a == nil || a.publicBaseURL == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	u := *a.publicBaseURL
	u.Path = diagnosisRoomPublicPath(u.Path)
	values := u.Query()
	values.Set("auth_mode", "session")
	values.Set("session_id", sessionID)
	values.Set("wecom_auto_login", "1")
	values.Set("wecom_launch_context", "app_conversation")
	u.RawQuery = values.Encode()
	u.Fragment = ""
	return u.String()
}

func diagnosisRoomPublicPath(basePath string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(basePath))
	if strings.HasSuffix(strings.TrimRight(cleaned, "/"), "/diagnosis-room") {
		return cleaned
	}
	return path.Join(cleaned, "diagnosis-room")
}

func diagnosisRoomNotificationContextFromFinalConclusion(
	finalConclusion DiagnosisRoomFinalConclusion,
) diagnosisRoomCloseNotificationContext {
	return diagnosisRoomCloseNotificationContext{
		ConfidenceRationale:           strings.TrimSpace(finalConclusion.ConfidenceRationale),
		Findings:                      cloneStrings(finalConclusion.Findings),
		RecommendedActions:            cloneStrings(finalConclusion.RecommendedActions),
		EvidenceRequests:              cloneEvidenceRequests(finalConclusion.EvidenceRequests),
		MissingEvidenceRequests:       cloneConsultationEvidenceRequests(finalConclusion.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: cloneConsultationEvidenceRequests(finalConclusion.EvidenceCollectionSuggestions),
	}
}

func diagnosisRoomNotificationContextFromAssistantTurn(
	req SendDiagnosisRoomAssistantTurnNotificationInput,
) diagnosisRoomCloseNotificationContext {
	return diagnosisRoomCloseNotificationContext{
		ConfidenceRationale:           strings.TrimSpace(req.Insight.ConfidenceRationale),
		Findings:                      cloneStrings(req.Findings),
		RecommendedActions:            cloneStrings(req.RecommendedActions),
		EvidenceRequests:              cloneEvidenceRequests(req.EvidenceRequests),
		MissingEvidenceRequests:       cloneConsultationEvidenceRequests(req.Insight.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: cloneConsultationEvidenceRequests(req.Insight.EvidenceCollectionSuggestions),
		ConclusionStatus:              strings.TrimSpace(req.Insight.ConclusionStatus),
	}
}

func diagnosisRoomAssistantTurnNotificationSeverity(req SendDiagnosisRoomAssistantTurnNotificationInput) string {
	if req.RequiresHumanReview {
		return "warning"
	}
	switch strings.TrimSpace(req.Confidence) {
	case "low":
		return "warning"
	default:
		return "info"
	}
}

func diagnosisRoomCloseNotificationSeverity(finalConclusion DiagnosisRoomFinalConclusion) string {
	if finalConclusion.Status != "available" {
		return "info"
	}
	if finalConclusion.RequiresHumanReview != nil && *finalConclusion.RequiresHumanReview {
		return "warning"
	}
	switch strings.TrimSpace(finalConclusion.Confidence) {
	case "low":
		return "warning"
	default:
		return "info"
	}
}

func diagnosisRoomAssistantTurnNotificationResultFromEvent(event domain.DiagnosisTaskEvent) SendDiagnosisRoomAssistantTurnNotificationResult {
	var payload struct {
		ChatSessionID     int64  `json:"chat_session_id"`
		IdempotencyKey    string `json:"idempotency_key"`
		ProviderMessageID string `json:"provider_message_id"`
		ProviderStatus    string `json:"provider_status"`
	}
	_ = json.Unmarshal(event.Payload, &payload)
	return SendDiagnosisRoomAssistantTurnNotificationResult{
		ChatSessionID:      payload.ChatSessionID,
		LifecycleEventID:   int64(event.ID),
		IdempotencyKey:     payload.IdempotencyKey,
		ProviderMessageID:  payload.ProviderMessageID,
		NotificationStatus: payload.ProviderStatus,
	}
}

func diagnosisRoomCloseNotificationResultFromEvent(event domain.DiagnosisTaskEvent) SendDiagnosisRoomCloseNotificationResult {
	var payload struct {
		ChatSessionID     int64  `json:"chat_session_id"`
		IdempotencyKey    string `json:"idempotency_key"`
		ProviderMessageID string `json:"provider_message_id"`
		ProviderStatus    string `json:"provider_status"`
	}
	_ = json.Unmarshal(event.Payload, &payload)
	return SendDiagnosisRoomCloseNotificationResult{
		ChatSessionID:      payload.ChatSessionID,
		LifecycleEventID:   int64(event.ID),
		IdempotencyKey:     payload.IdempotencyKey,
		ProviderMessageID:  payload.ProviderMessageID,
		NotificationStatus: payload.ProviderStatus,
	}
}

func diagnosisRoomFinalReadyNotificationResultFromEvent(event domain.DiagnosisTaskEvent) SendDiagnosisRoomFinalReadyNotificationResult {
	var payload struct {
		ChatSessionID     int64  `json:"chat_session_id"`
		IdempotencyKey    string `json:"idempotency_key"`
		ProviderMessageID string `json:"provider_message_id"`
		ProviderStatus    string `json:"provider_status"`
	}
	_ = json.Unmarshal(event.Payload, &payload)
	return SendDiagnosisRoomFinalReadyNotificationResult{
		ChatSessionID:      payload.ChatSessionID,
		LifecycleEventID:   int64(event.ID),
		IdempotencyKey:     payload.IdempotencyKey,
		ProviderMessageID:  payload.ProviderMessageID,
		NotificationStatus: payload.ProviderStatus,
	}
}

func diagnosisRoomFailedNotificationDelivery(err error) ports.IMDelivery {
	rawPayload := map[string]any{
		"status":    "failed",
		"retryable": false,
	}
	var imErr *ports.IMError
	if errors.As(err, &imErr) {
		rawPayload["retryable"] = imErr.Retryable
		if imErr.StatusCode > 0 {
			rawPayload["status_code"] = imErr.StatusCode
		}
	}
	raw, marshalErr := json.Marshal(rawPayload)
	if marshalErr != nil {
		raw = []byte(`{"status":"failed","retryable":false}`)
	}
	return ports.IMDelivery{
		Status: "failed",
		Raw:    raw,
	}
}

func diagnosisRoomRetryableNotificationActivityError(err error) error {
	var imErr *ports.IMError
	if !errors.As(err, &imErr) || !imErr.Retryable {
		return nil
	}
	return temporalsdk.NewApplicationError(
		"diagnosis room notification delivery failed; retrying provider request",
		diagnosisRoomNotificationDeliveryErrType,
	)
}

func marshalDiagnosisTurnMetadata(payload map[string]any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnosis turn metadata: %w", err)
	}
	return raw, nil
}
