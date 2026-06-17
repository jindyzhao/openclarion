package temporal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisRoomEventOpened                = "diagnosis_room.opened"
	diagnosisRoomEventTurnPersisted         = "diagnosis_room.turn_persisted"
	diagnosisRoomEventFinalConclusionReady  = "diagnosis_room.final_conclusion_ready"
	diagnosisRoomEventClosed                = "diagnosis_room.closed"
	diagnosisRoomEventCloseNotificationSent = "diagnosis_room.close_notification_sent"

	diagnosisRoomFinalConclusionMaxRunes = 4096
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
	SessionID           string
	DiagnosisTaskID     int64
	OwnerSubject        string
	UserMessageID       string
	AssistantMessageID  string
	UserSequence        int
	AssistantSequence   int
	TurnCount           int
	ActorSubject        string
	UserMessage         string
	AssistantMessage    string
	UserOccurredAt      time.Time
	AssistantOccurredAt time.Time
	ContextBytes        int
	InvocationID        string
	RuntimeID           string
	ContainerStartedAt  time.Time
	ContainerFinishedAt time.Time
	RawOutput           json.RawMessage
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
	EvidenceRequests    []diagnosisroom.EvidenceRequest
	Insight             diagnosisroom.ConsultationInsight
	FinalConclusion     *DiagnosisRoomFinalConclusion
}

// CloseDiagnosisChatSessionInput closes the persisted room session and records
// the terminal lifecycle audit event.
type CloseDiagnosisChatSessionInput struct {
	SessionID       string
	DiagnosisTaskID int64
	OwnerSubject    string
	TurnCount       int
	ClosedAt        time.Time
	Reason          string
}

// CloseDiagnosisChatSessionResult returns the persisted terminal room state.
type CloseDiagnosisChatSessionResult struct {
	ChatSessionID    int64
	LifecycleEventID int64
	Status           string
	TurnCount        int
	ClosedAt         time.Time
	CloseReason      string
	LastActivityAt   time.Time
	FinalConclusion  DiagnosisRoomFinalConclusion
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
	if result.Insight.ConclusionStatus == "final" {
		_, finalConclusion, err := a.recordDiagnosisRoomFinalConclusionReady(ctx, req, result)
		if err != nil {
			return PersistDiagnosisTurnResult{}, mapActivityError(err, "persist-diagnosis-turn final conclusion event")
		}
		result.FinalConclusion = copyDiagnosisRoomFinalConclusion(finalConclusion)
	}
	return result, nil
}

// CloseDiagnosisChatSession persists terminal room lifecycle metadata and an
// idempotent close audit event.
func (a *Activities) CloseDiagnosisChatSession(ctx context.Context, req CloseDiagnosisChatSessionInput) (CloseDiagnosisChatSessionResult, error) {
	if a.uowFactory == nil {
		return CloseDiagnosisChatSessionResult{}, temporalsdk.NewNonRetryableApplicationError(
			"close-diagnosis-chat-session: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	session, err := a.closeDiagnosisChatSession(ctx, req)
	if err != nil {
		return CloseDiagnosisChatSessionResult{}, mapActivityError(err, "close-diagnosis-chat-session")
	}
	event, finalConclusion, err := a.recordDiagnosisRoomClosed(ctx, req, session)
	if err != nil {
		return CloseDiagnosisChatSessionResult{}, mapActivityError(err, "close-diagnosis-chat-session lifecycle event")
	}
	return closeDiagnosisChatSessionResult(session, event, finalConclusion), nil
}

// SendDiagnosisRoomCloseNotification delivers the final operator notification
// for a closed diagnosis room and records an idempotent delivery audit event.
func (a *Activities) SendDiagnosisRoomCloseNotification(ctx context.Context, req CloseDiagnosisChatSessionInput) (SendDiagnosisRoomCloseNotificationResult, error) {
	if a.uowFactory == nil {
		return SendDiagnosisRoomCloseNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-close-notification: unit of work factory is not configured", errTypeInvalidInput, nil)
	}
	if a.imProvider == nil {
		return SendDiagnosisRoomCloseNotificationResult{}, temporalsdk.NewNonRetryableApplicationError(
			"send-diagnosis-room-close-notification: im provider is not configured", errTypeInvalidInput, nil)
	}
	session, task, snapshot, err := a.loadClosedDiagnosisRoomForNotification(ctx, req)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification load room")
	}
	idempotencyKey := diagnosisRoomCloseNotificationIdempotencyKey(req, session)
	dedupeKey := diagnosisRoomEventDedupeKey(diagnosisRoomEventCloseNotificationSent, req.SessionID, idempotencyKey)
	existing, found, err := a.findDiagnosisRoomLifecycleEvent(ctx, req.DiagnosisTaskID, dedupeKey, diagnosisRoomEventCloseNotificationSent)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification lookup delivery event")
	}
	if found {
		return diagnosisRoomCloseNotificationResultFromEvent(existing), nil
	}

	delivery, err := a.imProvider.SendNotification(ctx, diagnosisRoomCloseNotification(req, session, task, snapshot, idempotencyKey))
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapDiagnosisRoomNotificationError(err)
	}
	event, err := a.recordDiagnosisRoomCloseNotificationSent(ctx, req, session, task, snapshot, idempotencyKey, delivery)
	if err != nil {
		return SendDiagnosisRoomCloseNotificationResult{}, mapActivityError(err, "send-diagnosis-room-close-notification lifecycle event")
	}
	return diagnosisRoomCloseNotificationResultFromEvent(event), nil
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

func (a *Activities) closeDiagnosisChatSession(ctx context.Context, req CloseDiagnosisChatSessionInput) (domain.ChatSession, error) {
	if err := validateCloseDiagnosisChatSessionInput(req); err != nil {
		return domain.ChatSession{}, err
	}
	var saved domain.ChatSession
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
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
		closed, err := session.Close(req.ClosedAt, req.Reason)
		if err != nil {
			return err
		}
		saved, err = uow.Diagnosis().UpdateChatSession(ctx, closed)
		return err
	})
	return saved, err
}

func (a *Activities) loadClosedDiagnosisRoomForNotification(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
) (domain.ChatSession, domain.DiagnosisTask, domain.EvidenceSnapshot, error) {
	if err := validateCloseDiagnosisChatSessionInput(req); err != nil {
		return domain.ChatSession{}, domain.DiagnosisTask{}, domain.EvidenceSnapshot{}, err
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
	parsed, err := diagnosisroom.ParseTurnOutput(req.RawOutput)
	if err != nil {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err)
	}
	if parsed.Message != strings.TrimSpace(req.AssistantMessage) {
		return diagnosisroom.TurnOutput{}, fmt.Errorf("persist diagnosis turn: assistant_message does not match validated output message: %w", domain.ErrInvariantViolation)
	}
	return parsed, nil
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
	if req.TurnCount < 0 {
		return fmt.Errorf("close diagnosis chat session: turn_count must be >= 0: %w", domain.ErrInvariantViolation)
	}
	if req.ClosedAt.IsZero() {
		return fmt.Errorf("close diagnosis chat session: closed_at must be set: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return fmt.Errorf("close diagnosis chat session: reason must be non-empty: %w", domain.ErrInvariantViolation)
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

func closeDiagnosisChatSessionResult(
	session domain.ChatSession,
	event domain.DiagnosisTaskEvent,
	finalConclusion DiagnosisRoomFinalConclusion,
) CloseDiagnosisChatSessionResult {
	var closedAt time.Time
	if session.ClosedAt != nil {
		closedAt = *session.ClosedAt
	}
	return CloseDiagnosisChatSessionResult{
		ChatSessionID:    int64(session.ID),
		LifecycleEventID: int64(event.ID),
		Status:           string(session.Status),
		TurnCount:        session.TurnCount,
		ClosedAt:         closedAt,
		CloseReason:      session.CloseReason,
		LastActivityAt:   session.LastActivityAt,
		FinalConclusion:  finalConclusion,
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

func (a *Activities) recordDiagnosisRoomFinalConclusionReady(
	ctx context.Context,
	req PersistDiagnosisTurnInput,
	result PersistDiagnosisTurnResult,
) (domain.DiagnosisTaskEvent, DiagnosisRoomFinalConclusion, error) {
	finalConclusion := diagnosisRoomFinalConclusionFromPersistedTurn(req, result)
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":               diagnosisRoomEventFinalConclusionReady,
		"session_id":         req.SessionID,
		"chat_session_id":    result.ChatSessionID,
		"diagnosis_task_id":  req.DiagnosisTaskID,
		"owner_subject":      req.OwnerSubject,
		"turn_count":         result.TurnCount,
		"final_conclusion":   finalConclusion,
		"conclusion_version": "diagnosis-room-final-ready.v1",
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

func (a *Activities) recordDiagnosisRoomClosed(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
) (domain.DiagnosisTaskEvent, DiagnosisRoomFinalConclusion, error) {
	finalConclusion, err := a.diagnosisRoomFinalConclusion(ctx, req, session)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, DiagnosisRoomFinalConclusion{}, err
	}
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":               diagnosisRoomEventClosed,
		"session_id":         req.SessionID,
		"chat_session_id":    int64(session.ID),
		"diagnosis_task_id":  req.DiagnosisTaskID,
		"owner_subject":      req.OwnerSubject,
		"status":             string(session.Status),
		"turn_count":         session.TurnCount,
		"close_reason":       session.CloseReason,
		"closed_at":          session.ClosedAt,
		"final_conclusion":   finalConclusion,
		"conclusion_version": "diagnosis-room-close.v1",
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
	Status              string     `json:"status"`
	Source              string     `json:"source"`
	Reason              string     `json:"reason,omitempty"`
	AssistantTurnID     int64      `json:"assistant_turn_id,omitempty"`
	AssistantMessageID  string     `json:"assistant_message_id,omitempty"`
	AssistantSequence   int        `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt *time.Time `json:"assistant_occurred_at,omitempty"`
	Content             string     `json:"content,omitempty"`
	Confidence          string     `json:"confidence,omitempty"`
	RequiresHumanReview *bool      `json:"requires_human_review,omitempty"`
}

type diagnosisRoomAssistantTurnMetadata struct {
	Confidence          string `json:"confidence"`
	RequiresHumanReview bool   `json:"requires_human_review"`
}

func diagnosisRoomFinalConclusionFromPersistedTurn(
	req PersistDiagnosisTurnInput,
	result PersistDiagnosisTurnResult,
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
	return DiagnosisRoomFinalConclusion{
		Status:              "available",
		Source:              "latest_assistant_turn",
		Reason:              "assistant_marked_final",
		AssistantTurnID:     result.AssistantTurnID,
		AssistantMessageID:  assistantMessageID,
		AssistantSequence:   assistantSequence,
		AssistantOccurredAt: &occurredAt,
		Content:             truncateString(content, diagnosisRoomFinalConclusionMaxRunes),
		Confidence:          strings.TrimSpace(result.Confidence),
		RequiresHumanReview: &requiresHumanReview,
	}
}

func (a *Activities) diagnosisRoomFinalConclusion(
	ctx context.Context,
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
) (DiagnosisRoomFinalConclusion, error) {
	if session.TurnCount == 0 {
		return DiagnosisRoomFinalConclusion{
			Status: "not_available",
			Source: "none",
			Reason: "room_closed_without_assistant_turn",
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
		return DiagnosisRoomFinalConclusion{
			Status:              "available",
			Source:              "latest_assistant_turn",
			AssistantTurnID:     int64(turn.ID),
			AssistantMessageID:  turn.MessageID,
			AssistantSequence:   turn.Sequence,
			AssistantOccurredAt: &occurredAt,
			Content:             truncateString(turn.Content, diagnosisRoomFinalConclusionMaxRunes),
			Confidence:          strings.TrimSpace(metadata.Confidence),
			RequiresHumanReview: &requiresHumanReview,
		}, nil
	}
	return DiagnosisRoomFinalConclusion{}, fmt.Errorf("diagnosis room final conclusion: no assistant turn found for non-empty session %q: %w",
		req.SessionID, domain.ErrInvariantViolation)
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
	idempotencyKey string,
	delivery ports.IMDelivery,
) (domain.DiagnosisTaskEvent, error) {
	payload, err := diagnosisRoomLifecyclePayload(map[string]any{
		"kind":                 diagnosisRoomEventCloseNotificationSent,
		"session_id":           req.SessionID,
		"chat_session_id":      int64(session.ID),
		"diagnosis_task_id":    req.DiagnosisTaskID,
		"evidence_snapshot_id": int64(task.EvidenceSnapshotID),
		"alert_group_id":       int64(snapshot.AlertGroupID),
		"owner_subject":        req.OwnerSubject,
		"turn_count":           session.TurnCount,
		"close_reason":         session.CloseReason,
		"idempotency_key":      idempotencyKey,
		"provider_message_id":  truncateString(delivery.ProviderMessageID, 256),
		"provider_status":      truncateString(delivery.Status, 64),
		"provider_raw":         defaultJSONObject(delivery.Raw),
	})
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	return a.appendDiagnosisRoomLifecycleEvent(ctx, diagnosisRoomLifecycleEventInput{
		TaskID:     req.DiagnosisTaskID,
		Kind:       diagnosisRoomEventCloseNotificationSent,
		DedupeKey:  diagnosisRoomEventDedupeKey(diagnosisRoomEventCloseNotificationSent, req.SessionID, idempotencyKey),
		Payload:    payload,
		OccurredAt: req.ClosedAt.Add(time.Microsecond),
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

func (a *Activities) findDiagnosisRoomLifecycleEvent(
	ctx context.Context,
	taskID int64,
	dedupeKey string,
	kind string,
) (domain.DiagnosisTaskEvent, bool, error) {
	var event domain.DiagnosisTaskEvent
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		out, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, domain.DiagnosisTaskID(taskID), dedupeKey)
		if err != nil {
			return err
		}
		if out.Kind != kind {
			return fmt.Errorf("diagnosis room lifecycle event: existing dedupe key %q has kind %q, want %q: %w",
				dedupeKey, out.Kind, kind, domain.ErrInvariantViolation)
		}
		event = out
		return nil
	})
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.DiagnosisTaskEvent{}, false, nil
		}
		return domain.DiagnosisTaskEvent{}, false, err
	}
	return event, true, nil
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
	case diagnosisRoomEventClosed:
		return "close"
	case diagnosisRoomEventCloseNotificationSent:
		return "notify"
	default:
		return "event"
	}
}

func diagnosisRoomCloseNotificationIdempotencyKey(req CloseDiagnosisChatSessionInput, session domain.ChatSession) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", req.SessionID, session.ID, session.CloseReason)))
	return fmt.Sprintf("diagnosis_room:%d:%s/close_notification", req.DiagnosisTaskID, hex.EncodeToString(sum[:])[:24])
}

func diagnosisRoomCloseNotification(
	req CloseDiagnosisChatSessionInput,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	snapshot domain.EvidenceSnapshot,
	idempotencyKey string,
) ports.IMNotification {
	reason := strings.TrimSpace(session.CloseReason)
	if reason == "" {
		reason = strings.TrimSpace(req.Reason)
	}
	return ports.IMNotification{
		IdempotencyKey:  idempotencyKey,
		DiagnosisTaskID: int64(task.ID),
		CorrelationKey:  fmt.Sprintf("alert_group:%d", snapshot.AlertGroupID),
		Title:           fmt.Sprintf("Diagnosis room closed: %s", req.SessionID),
		Body: fmt.Sprintf(
			"Diagnosis room %s for alert group %d closed with reason %q after %d turn(s).",
			req.SessionID,
			snapshot.AlertGroupID,
			reason,
			session.TurnCount,
		),
		Severity: "info",
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

func mapDiagnosisRoomNotificationError(err error) error {
	if err == nil {
		return nil
	}
	var imErr *ports.IMError
	if errors.As(err, &imErr) && !imErr.Retryable {
		return temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf("send-diagnosis-room-close-notification: %v", err), errTypeInvalidInput, err)
	}
	return fmt.Errorf("send-diagnosis-room-close-notification: %w", err)
}

func marshalDiagnosisTurnMetadata(payload map[string]any) (json.RawMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnosis turn metadata: %w", err)
	}
	return raw, nil
}
