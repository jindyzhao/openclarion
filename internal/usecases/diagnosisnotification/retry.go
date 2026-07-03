// Package diagnosisnotification owns operator-triggered retries for
// diagnosis-room notification lifecycle events.
package diagnosisnotification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Diagnosis-room notification event kinds retained for operator-triggered
// retry decisions.
const (
	EventAssistantTurnNotification = "diagnosis_room.assistant_turn_notification_sent"
	EventFinalReadyNotification    = "diagnosis_room.final_ready_notification_sent"
	EventCloseNotification         = "diagnosis_room.close_notification_sent"
	eventFinalConclusionReady      = "diagnosis_room.final_conclusion_ready"
	eventClosed                    = "diagnosis_room.closed"

	// RetryState values returned by the notification retry service.
	RetryStateSent             RetryState = "sent"
	RetryStateAlreadyDelivered RetryState = "already_delivered"

	notificationEventLookupLimit   = 100
	notificationTurnLookupLimit    = 200
	maxProviderMessageIDLength     = 256
	maxProviderStatusLength        = 64
	maxNotificationBodyRunes       = 1600
	maxFinalConclusionContentRunes = 12000
)

// RetryState reports whether Retry sent a provider request or found that a
// later successful attempt already covers the failed event.
type RetryState string

// Request identifies one failed diagnosis-room notification kind to retry.
// Callers are responsible for authorizing the principal through
// OpenClarion-local RBAC before invoking this use case.
type Request struct {
	SessionID  string
	EventKind  string
	Principal  ports.AuthPrincipal
	OccurredAt time.Time
}

// Result returns the retained retry event.
type Result struct {
	RetryState RetryState
	Event      domain.DiagnosisTaskEvent
}

// Clock supplies retry event timestamps. It is injected in tests.
type Clock func() time.Time

// Service retries failed diagnosis-room notification events using the
// deployment-managed notification-channel resolver.
type Service struct {
	uowFactory ports.UnitOfWorkFactory
	resolver   ports.NotificationChannelProviderResolver
	clock      Clock
}

// Option customizes Service construction.
type Option func(*Service)

// WithClock overrides the timestamp source.
func WithClock(clock Clock) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// NewService constructs a diagnosis notification retry service.
func NewService(
	uowFactory ports.UnitOfWorkFactory,
	resolver ports.NotificationChannelProviderResolver,
	opts ...Option,
) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis notification retry: unit of work factory is required: %w", domain.ErrInvariantViolation)
	}
	if resolver == nil {
		return nil, fmt.Errorf("diagnosis notification retry: notification channel provider resolver is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		uowFactory: uowFactory,
		resolver:   resolver,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.clock == nil {
		return nil, fmt.Errorf("diagnosis notification retry: clock is required: %w", domain.ErrInvariantViolation)
	}
	return service, nil
}

// Retry resends the latest still-failed notification event for a room/kind and
// appends a new immutable lifecycle event with the same provider idempotency
// key.
func (s *Service) Retry(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.uowFactory == nil || s.resolver == nil || s.clock == nil {
		return Result{}, fmt.Errorf("diagnosis notification retry: service is not configured: %w", domain.ErrInvariantViolation)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return Result{}, fmt.Errorf("diagnosis notification retry: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	eventKind, err := normalizeEventKind(req.EventKind)
	if err != nil {
		return Result{}, err
	}
	principalSubject := strings.TrimSpace(req.Principal.Subject)
	if principalSubject == "" {
		return Result{}, fmt.Errorf("diagnosis notification retry: principal subject is required: %w", diagnosisauth.ErrUnauthenticated)
	}

	session, task, events, err := s.loadRoomAndEvents(ctx, sessionID, eventKind)
	if err != nil {
		return Result{}, err
	}
	failed, found, attempts, err := latestRetryableNotificationEvent(events, eventKind)
	if err != nil {
		return Result{}, err
	}
	if !found {
		completed, ok, err := latestCompletedNotificationEvent(events, eventKind)
		if err != nil {
			return Result{}, err
		}
		if ok {
			return Result{RetryState: RetryStateAlreadyDelivered, Event: completed.Event}, nil
		}
		return Result{}, fmt.Errorf("diagnosis notification retry: no failed notification event found for %s: %w", eventKind, domain.ErrPreconditionFailed)
	}
	if failed.Payload.NotificationChannelProfileID <= 0 {
		return Result{}, fmt.Errorf("diagnosis notification retry: failed event has no notification channel profile: %w", domain.ErrPreconditionFailed)
	}

	provider, err := s.provider(ctx, eventKind, domain.NotificationChannelProfileID(failed.Payload.NotificationChannelProfileID))
	if err != nil {
		return Result{}, err
	}
	payload, err := s.payloadForRetry(ctx, session, task, failed.Payload)
	if err != nil {
		return Result{}, err
	}
	notification, err := notificationFromFailedEvent(payload, task)
	if err != nil {
		return Result{}, err
	}
	delivery, err := provider.SendNotification(ctx, notification)
	if err != nil {
		if _, recordErr := s.appendRetryEvent(ctx, payload, eventKind, attempts, failedDelivery(err), req.OccurredAt, principalSubject); recordErr != nil {
			return Result{}, fmt.Errorf("diagnosis notification retry: persist failed retry event: %w", recordErr)
		}
		return Result{}, err
	}
	event, err := s.appendRetryEvent(ctx, payload, eventKind, attempts, delivery, req.OccurredAt, principalSubject)
	if err != nil {
		return Result{}, err
	}
	return Result{RetryState: RetryStateSent, Event: event}, nil
}

func (s *Service) loadRoomAndEvents(
	ctx context.Context,
	sessionID string,
	eventKind string,
) (domain.ChatSession, domain.DiagnosisTask, []domain.DiagnosisTaskEvent, error) {
	var session domain.ChatSession
	var task domain.DiagnosisTask
	var events []domain.DiagnosisTaskEvent
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		session, err = uow.Diagnosis().FindChatSessionByKey(ctx, sessionID)
		if err != nil {
			return err
		}
		task, err = uow.Diagnosis().FindTaskByID(ctx, session.DiagnosisTaskID)
		if err != nil {
			return err
		}
		events, err = uow.Diagnosis().ListEventsByTaskAndKind(ctx, task.ID, eventKind, notificationEventLookupLimit)
		return err
	})
	return session, task, events, err
}

func (s *Service) provider(
	ctx context.Context,
	eventKind string,
	channelID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	var (
		provider ports.IMProvider
		err      error
	)
	switch eventKind {
	case EventAssistantTurnNotification, EventFinalReadyNotification:
		provider, err = s.resolver.ResolveDiagnosisConsultationNotificationProvider(ctx, channelID)
	case EventCloseNotification:
		provider, err = s.resolver.ResolveDiagnosisCloseNotificationProvider(ctx, channelID)
	default:
		return nil, fmt.Errorf("diagnosis notification retry: event_kind %q is unsupported: %w", eventKind, domain.ErrInvariantViolation)
	}
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("diagnosis notification retry: notification channel provider resolver returned nil provider: %w", domain.ErrInvariantViolation)
	}
	return provider, nil
}

func (s *Service) appendRetryEvent(
	ctx context.Context,
	payload notificationPayload,
	eventKind string,
	priorAttempts int,
	delivery ports.IMDelivery,
	occurredAt time.Time,
	actorSubject string,
) (domain.DiagnosisTaskEvent, error) {
	now := occurredAt.UTC()
	if now.IsZero() {
		now = s.clock().UTC()
	}
	nextPayload := payload
	nextPayload.Source = "DiagnosisNotificationRetry"
	nextPayload.Kind = eventKind
	nextPayload.ActorSubject = actorSubject
	nextPayload.RetryRequestedBy = actorSubject
	nextPayload.ProviderMessageID = truncateString(delivery.ProviderMessageID, maxProviderMessageIDLength)
	nextPayload.ProviderStatus = truncateString(delivery.Status, maxProviderStatusLength)
	nextPayload.ProviderRaw = defaultJSONObject(delivery.Raw)
	raw, err := json.Marshal(nextPayload)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, fmt.Errorf("diagnosis notification retry: marshal event payload: %w", err)
	}
	dedupeKey := notificationEventDedupeKey(eventKind, payload.SessionID, notificationDedupeComponent(payload.IdempotencyKey, priorAttempts))
	event, err := domain.NewDiagnosisTaskEvent(
		domain.DiagnosisTaskID(payload.DiagnosisTaskID),
		eventKind,
		raw,
		&dedupeKey,
		now,
	)
	if err != nil {
		return domain.DiagnosisTaskEvent{}, err
	}
	var saved domain.DiagnosisTaskEvent
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		inserted, err := uow.Diagnosis().AppendEvent(ctx, event)
		if err != nil {
			return err
		}
		saved = inserted
		return nil
	})
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.DiagnosisTaskEvent{}, err
	}
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, domain.DiagnosisTaskID(payload.DiagnosisTaskID), dedupeKey)
		if err != nil {
			return err
		}
		saved = existing
		return nil
	})
	return saved, err
}

type notificationAttempt struct {
	Event   domain.DiagnosisTaskEvent
	Payload notificationPayload
}

func latestRetryableNotificationEvent(
	events []domain.DiagnosisTaskEvent,
	eventKind string,
) (notificationAttempt, bool, int, error) {
	completedKeys := map[string]struct{}{}
	attemptsByKey := map[string]int{}
	var (
		selected    notificationAttempt
		selectedKey string
		found       bool
	)
	for _, event := range events {
		payload, ok, err := notificationPayloadFromEvent(event, eventKind)
		if err != nil {
			return notificationAttempt{}, false, 0, err
		}
		if !ok {
			continue
		}
		attemptsByKey[payload.IdempotencyKey]++
		if notificationDelivered(payload.ProviderStatus) {
			if notificationAIContentProofMissing(payload, eventKind) {
				if _, completed := completedKeys[payload.IdempotencyKey]; completed {
					continue
				}
				if !found {
					selected = notificationAttempt{Event: event, Payload: payload}
					selectedKey = payload.IdempotencyKey
					found = true
				}
				continue
			}
			completedKeys[payload.IdempotencyKey] = struct{}{}
			continue
		}
		if !notificationFailed(payload.ProviderStatus) {
			continue
		}
		if _, completed := completedKeys[payload.IdempotencyKey]; completed {
			continue
		}
		if !found {
			selected = notificationAttempt{Event: event, Payload: payload}
			selectedKey = payload.IdempotencyKey
			found = true
		}
	}
	if found {
		return selected, true, attemptsByKey[selectedKey], nil
	}
	return notificationAttempt{}, false, 0, nil
}

func latestCompletedNotificationEvent(
	events []domain.DiagnosisTaskEvent,
	eventKind string,
) (notificationAttempt, bool, error) {
	for _, event := range events {
		payload, ok, err := notificationPayloadFromEvent(event, eventKind)
		if err != nil {
			return notificationAttempt{}, false, err
		}
		if ok && notificationDelivered(payload.ProviderStatus) && !notificationAIContentProofMissing(payload, eventKind) {
			return notificationAttempt{Event: event, Payload: payload}, true, nil
		}
	}
	return notificationAttempt{}, false, nil
}

type notificationPayload struct {
	Source                       string                              `json:"source,omitempty"`
	Kind                         string                              `json:"kind"`
	SessionID                    string                              `json:"session_id"`
	ChatSessionID                int64                               `json:"chat_session_id,omitempty"`
	DiagnosisTaskID              int64                               `json:"diagnosis_task_id"`
	EvidenceSnapshotID           int64                               `json:"evidence_snapshot_id,omitempty"`
	AlertGroupID                 int64                               `json:"alert_group_id,omitempty"`
	OwnerSubject                 string                              `json:"owner_subject,omitempty"`
	ActorSubject                 string                              `json:"actor_subject,omitempty"`
	RetryRequestedBy             string                              `json:"retry_requested_by,omitempty"`
	AssistantMessageID           string                              `json:"assistant_message_id,omitempty"`
	AssistantTurnID              int64                               `json:"assistant_turn_id,omitempty"`
	AssistantSequence            int                                 `json:"assistant_sequence,omitempty"`
	TurnCount                    int                                 `json:"turn_count,omitempty"`
	CloseReason                  string                              `json:"close_reason,omitempty"`
	RoomURL                      string                              `json:"room_url,omitempty"`
	IdempotencyKey               string                              `json:"idempotency_key"`
	NotificationChannelProfileID int64                               `json:"notification_channel_profile_id"`
	ProviderMessageID            string                              `json:"provider_message_id,omitempty"`
	ProviderStatus               string                              `json:"provider_status"`
	ProviderRaw                  json.RawMessage                     `json:"provider_raw,omitempty"`
	AssistantMessage             string                              `json:"assistant_message,omitempty"`
	Confidence                   string                              `json:"confidence,omitempty"`
	RequiresHumanReview          bool                                `json:"requires_human_review,omitempty"`
	Findings                     []string                            `json:"findings,omitempty"`
	RecommendedActions           []string                            `json:"recommended_actions,omitempty"`
	EvidenceRequests             []diagnosisroom.EvidenceRequest     `json:"evidence_requests,omitempty"`
	ConsultationInsight          diagnosisroom.ConsultationInsight   `json:"consultation_insight,omitempty"`
	FinalConclusion              diagnosisRoomFinalConclusionPayload `json:"final_conclusion,omitempty"`
}

type diagnosisRoomFinalConclusionPayload struct {
	Status                        string                                      `json:"status,omitempty"`
	Content                       string                                      `json:"content,omitempty"`
	Confidence                    string                                      `json:"confidence,omitempty"`
	ConfidenceRationale           string                                      `json:"confidence_rationale,omitempty"`
	AssistantMessageID            string                                      `json:"assistant_message_id,omitempty"`
	AssistantTurnID               int64                                       `json:"assistant_turn_id,omitempty"`
	AssistantSequence             int                                         `json:"assistant_sequence,omitempty"`
	Findings                      []string                                    `json:"findings,omitempty"`
	RecommendedActions            []string                                    `json:"recommended_actions,omitempty"`
	EvidenceRequests              []diagnosisroom.EvidenceRequest             `json:"evidence_requests,omitempty"`
	MissingEvidenceRequests       []diagnosisroom.ConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisroom.ConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
	RequiresHumanReview           *bool                                       `json:"requires_human_review,omitempty"`
}

func notificationPayloadFromEvent(event domain.DiagnosisTaskEvent, eventKind string) (notificationPayload, bool, error) {
	if len(event.Payload) == 0 {
		return notificationPayload{}, false, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return notificationPayload{}, false, fmt.Errorf("diagnosis notification retry: event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload notificationPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return notificationPayload{}, false, fmt.Errorf("diagnosis notification retry: event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != eventKind {
		return notificationPayload{}, false, fmt.Errorf("diagnosis notification retry: event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return notificationPayload{}, false, fmt.Errorf("diagnosis notification retry: event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	payload.Kind = eventKind
	payload.IdempotencyKey = strings.TrimSpace(payload.IdempotencyKey)
	payload.ProviderStatus = strings.TrimSpace(payload.ProviderStatus)
	payload.SessionID = strings.TrimSpace(payload.SessionID)
	payload.RoomURL = notificationRoomURLWithWeComContext(payload.RoomURL, payload.SessionID)
	if payload.IdempotencyKey == "" || payload.ProviderStatus == "" || payload.SessionID == "" {
		return notificationPayload{}, false, nil
	}
	return payload, true, nil
}

func notificationRoomURLWithWeComContext(raw string, sessionID string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return value
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/diagnosis-room") {
		return value
	}
	values := parsed.Query()
	if strings.TrimSpace(sessionID) != "" && values.Get("session_id") == "" {
		values.Set("session_id", strings.TrimSpace(sessionID))
	}
	values.Set("auth_mode", "session")
	values.Set("wecom_auto_login", "1")
	values.Set("wecom_launch_context", "app_conversation")
	parsed.RawQuery = values.Encode()
	parsed.Fragment = ""
	return parsed.String()
}

func notificationFromFailedEvent(payload notificationPayload, task domain.DiagnosisTask) (ports.IMNotification, error) {
	title, body, severity, err := notificationContent(payload)
	if err != nil {
		return ports.IMNotification{}, err
	}
	return ports.IMNotification{
		IdempotencyKey:        payload.IdempotencyKey,
		DiagnosisTaskID:       int64(task.ID),
		NotificationChannelID: payload.NotificationChannelProfileID,
		CorrelationKey:        fmt.Sprintf("alert_group:%d", payload.AlertGroupID),
		Title:                 title,
		Body:                  body,
		Severity:              severity,
	}, nil
}

func (s *Service) payloadForRetry(
	ctx context.Context,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	payload notificationPayload,
) (notificationPayload, error) {
	switch payload.Kind {
	case EventAssistantTurnNotification:
		return s.hydrateAssistantTurnPayload(ctx, session, payload)
	case EventFinalReadyNotification:
		return s.hydrateFinalReadyPayload(ctx, session, task, payload)
	case EventCloseNotification:
		return s.hydrateClosePayload(ctx, task, payload)
	default:
		return payload, nil
	}
}

func (s *Service) hydrateAssistantTurnPayload(
	ctx context.Context,
	session domain.ChatSession,
	payload notificationPayload,
) (notificationPayload, error) {
	if strings.TrimSpace(payload.AssistantMessage) != "" {
		return payload, nil
	}
	turn, found, err := s.findAssistantTurnForPayload(ctx, session.ID, payload)
	if err != nil {
		return notificationPayload{}, err
	}
	if !found {
		return notificationPayload{}, fmt.Errorf("diagnosis notification retry: assistant notification is missing AI content and no matching assistant turn was found: %w", domain.ErrPreconditionFailed)
	}
	payload.AssistantMessage = truncateString(strings.TrimSpace(turn.Content), maxFinalConclusionContentRunes)
	if payload.AssistantMessageID == "" {
		payload.AssistantMessageID = turn.MessageID
	}
	if payload.AssistantTurnID == 0 {
		payload.AssistantTurnID = int64(turn.ID)
	}
	if payload.AssistantSequence == 0 {
		payload.AssistantSequence = turn.Sequence
	}
	metadata, err := assistantTurnMetadataFromChatTurn(turn)
	if err != nil {
		return notificationPayload{}, err
	}
	payload.fillAssistantMetadata(metadata)
	return payload, nil
}

func (s *Service) hydrateFinalReadyPayload(
	ctx context.Context,
	session domain.ChatSession,
	task domain.DiagnosisTask,
	payload notificationPayload,
) (notificationPayload, error) {
	if strings.TrimSpace(payload.FinalConclusion.Content) != "" {
		return payload, nil
	}
	eventPayload, found, err := s.findFinalReadyPayload(ctx, task.ID, payload)
	if err != nil {
		return notificationPayload{}, err
	}
	if found {
		payload.fillFinalReadyPayload(eventPayload)
		return payload, nil
	}
	turn, found, err := s.findAssistantTurnForPayload(ctx, session.ID, payload)
	if err != nil {
		return notificationPayload{}, err
	}
	if !found {
		return notificationPayload{}, fmt.Errorf("diagnosis notification retry: final-ready notification is missing AI content and no matching assistant turn was found: %w", domain.ErrPreconditionFailed)
	}
	payload.fillFinalConclusionFromAssistantTurn(turn)
	metadata, err := assistantTurnMetadataFromChatTurn(turn)
	if err != nil {
		return notificationPayload{}, err
	}
	payload.fillFinalConclusionMetadata(metadata)
	return payload, nil
}

func (s *Service) hydrateClosePayload(
	ctx context.Context,
	task domain.DiagnosisTask,
	payload notificationPayload,
) (notificationPayload, error) {
	if strings.TrimSpace(payload.FinalConclusion.Content) != "" {
		return payload, nil
	}
	eventPayload, found, err := s.findClosedPayload(ctx, task.ID, payload)
	if err != nil {
		return notificationPayload{}, err
	}
	if found {
		payload.fillClosePayload(eventPayload)
		return payload, nil
	}
	return notificationPayload{}, fmt.Errorf("diagnosis notification retry: close notification is missing AI content and no matching close event was found: %w", domain.ErrPreconditionFailed)
}

func (s *Service) findAssistantTurnForPayload(
	ctx context.Context,
	sessionID domain.ChatSessionID,
	payload notificationPayload,
) (domain.ChatTurn, bool, error) {
	if messageID := strings.TrimSpace(payload.AssistantMessageID); messageID != "" {
		var turn domain.ChatTurn
		err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
			got, err := uow.Diagnosis().FindChatTurnBySessionAndMessageID(ctx, sessionID, messageID)
			if err != nil {
				return err
			}
			turn = got
			return nil
		})
		if err == nil {
			if turn.Role != domain.ChatRoleAssistant {
				return domain.ChatTurn{}, false, fmt.Errorf("diagnosis notification retry: message %q is not an assistant turn: %w", messageID, domain.ErrPreconditionFailed)
			}
			return turn, true, nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.ChatTurn{}, false, err
		}
	}

	var turns []domain.ChatTurn
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		turns, err = uow.Diagnosis().ListChatTurnsBySession(ctx, sessionID, notificationTurnLookupLimit)
		return err
	})
	if err != nil {
		return domain.ChatTurn{}, false, err
	}
	for _, turn := range turns {
		if turn.Role != domain.ChatRoleAssistant {
			continue
		}
		if payload.AssistantTurnID > 0 && int64(turn.ID) == payload.AssistantTurnID {
			return turn, true, nil
		}
		if payload.AssistantSequence > 0 && turn.Sequence == payload.AssistantSequence {
			return turn, true, nil
		}
	}
	return domain.ChatTurn{}, false, nil
}

func (s *Service) findFinalReadyPayload(
	ctx context.Context,
	taskID domain.DiagnosisTaskID,
	payload notificationPayload,
) (diagnosisRoomFinalReadyPayload, bool, error) {
	var events []domain.DiagnosisTaskEvent
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		events, err = uow.Diagnosis().ListEventsByTaskAndKind(ctx, taskID, eventFinalConclusionReady, notificationEventLookupLimit)
		return err
	})
	if err != nil {
		return diagnosisRoomFinalReadyPayload{}, false, err
	}
	for _, event := range events {
		eventPayload, ok, err := finalReadyPayloadFromEvent(event)
		if err != nil {
			return diagnosisRoomFinalReadyPayload{}, false, err
		}
		if !ok || !finalReadyPayloadMatchesNotification(eventPayload, payload) {
			continue
		}
		return eventPayload, true, nil
	}
	return diagnosisRoomFinalReadyPayload{}, false, nil
}

func (s *Service) findClosedPayload(
	ctx context.Context,
	taskID domain.DiagnosisTaskID,
	payload notificationPayload,
) (diagnosisRoomClosedPayload, bool, error) {
	var events []domain.DiagnosisTaskEvent
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		events, err = uow.Diagnosis().ListEventsByTaskAndKind(ctx, taskID, eventClosed, notificationEventLookupLimit)
		return err
	})
	if err != nil {
		return diagnosisRoomClosedPayload{}, false, err
	}
	for _, event := range events {
		eventPayload, ok, err := closedPayloadFromEvent(event)
		if err != nil {
			return diagnosisRoomClosedPayload{}, false, err
		}
		if !ok || !closedPayloadMatchesNotification(eventPayload, payload) {
			continue
		}
		return eventPayload, true, nil
	}
	return diagnosisRoomClosedPayload{}, false, nil
}

type assistantTurnMetadata struct {
	Confidence          string                            `json:"confidence,omitempty"`
	ConfidenceRationale string                            `json:"confidence_rationale,omitempty"`
	RequiresHumanReview bool                              `json:"requires_human_review,omitempty"`
	Findings            []string                          `json:"findings,omitempty"`
	RecommendedActions  []string                          `json:"recommended_actions,omitempty"`
	EvidenceRequests    []diagnosisroom.EvidenceRequest   `json:"evidence_requests,omitempty"`
	ConsultationInsight diagnosisroom.ConsultationInsight `json:"consultation_insight,omitempty"`
}

func assistantTurnMetadataFromChatTurn(turn domain.ChatTurn) (assistantTurnMetadata, error) {
	if len(turn.Metadata) == 0 {
		return assistantTurnMetadata{}, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(turn.Metadata); err != nil {
		return assistantTurnMetadata{}, fmt.Errorf("diagnosis notification retry: assistant turn %d metadata is ambiguous: %w", turn.ID, err)
	}
	var metadata assistantTurnMetadata
	if err := json.Unmarshal(turn.Metadata, &metadata); err != nil {
		return assistantTurnMetadata{}, fmt.Errorf("diagnosis notification retry: assistant turn %d metadata: %w", turn.ID, err)
	}
	return metadata, nil
}

func (metadata assistantTurnMetadata) confidenceRationale() string {
	if rationale := strings.TrimSpace(metadata.ConsultationInsight.ConfidenceRationale); rationale != "" {
		return rationale
	}
	return strings.TrimSpace(metadata.ConfidenceRationale)
}

func (payload *notificationPayload) fillAssistantMetadata(metadata assistantTurnMetadata) {
	if strings.TrimSpace(payload.Confidence) == "" {
		payload.Confidence = strings.TrimSpace(metadata.Confidence)
	}
	if !payload.RequiresHumanReview {
		payload.RequiresHumanReview = metadata.RequiresHumanReview
	}
	if len(payload.Findings) == 0 {
		payload.Findings = cloneStrings(metadata.Findings)
	}
	if len(payload.RecommendedActions) == 0 {
		payload.RecommendedActions = cloneStrings(metadata.RecommendedActions)
	}
	if len(payload.EvidenceRequests) == 0 {
		payload.EvidenceRequests = cloneEvidenceRequests(metadata.EvidenceRequests)
	}
	if payload.ConsultationInsight.ConclusionStatus == "" {
		payload.ConsultationInsight = metadata.ConsultationInsight
	}
	if strings.TrimSpace(payload.ConsultationInsight.ConfidenceRationale) == "" {
		payload.ConsultationInsight.ConfidenceRationale = metadata.confidenceRationale()
	}
}

func (payload *notificationPayload) fillFinalReadyPayload(eventPayload diagnosisRoomFinalReadyPayload) {
	payload.FinalConclusion = eventPayload.FinalConclusion
	if payload.AssistantMessageID == "" {
		payload.AssistantMessageID = eventPayload.AssistantMessageID
	}
	if payload.AssistantTurnID == 0 {
		payload.AssistantTurnID = eventPayload.AssistantTurnID
	}
	if payload.AssistantSequence == 0 {
		payload.AssistantSequence = eventPayload.AssistantSequence
	}
	if payload.Confidence == "" {
		payload.Confidence = eventPayload.FinalConclusion.Confidence
	}
	if !payload.RequiresHumanReview && eventPayload.FinalConclusion.RequiresHumanReview != nil {
		payload.RequiresHumanReview = *eventPayload.FinalConclusion.RequiresHumanReview
	}
}

func (payload *notificationPayload) fillFinalConclusionFromAssistantTurn(turn domain.ChatTurn) {
	content := truncateString(strings.TrimSpace(turn.Content), maxFinalConclusionContentRunes)
	payload.FinalConclusion.Content = content
	if payload.FinalConclusion.Status == "" {
		payload.FinalConclusion.Status = "available"
	}
	if payload.FinalConclusion.AssistantMessageID == "" {
		payload.FinalConclusion.AssistantMessageID = turn.MessageID
	}
	if payload.FinalConclusion.AssistantTurnID == 0 {
		payload.FinalConclusion.AssistantTurnID = int64(turn.ID)
	}
	if payload.FinalConclusion.AssistantSequence == 0 {
		payload.FinalConclusion.AssistantSequence = turn.Sequence
	}
	if payload.AssistantMessageID == "" {
		payload.AssistantMessageID = turn.MessageID
	}
	if payload.AssistantTurnID == 0 {
		payload.AssistantTurnID = int64(turn.ID)
	}
	if payload.AssistantSequence == 0 {
		payload.AssistantSequence = turn.Sequence
	}
}

func (payload *notificationPayload) fillFinalConclusionMetadata(metadata assistantTurnMetadata) {
	if strings.TrimSpace(payload.FinalConclusion.Confidence) == "" {
		payload.FinalConclusion.Confidence = strings.TrimSpace(metadata.Confidence)
	}
	if strings.TrimSpace(payload.Confidence) == "" {
		payload.Confidence = strings.TrimSpace(metadata.Confidence)
	}
	if payload.FinalConclusion.RequiresHumanReview == nil {
		value := metadata.RequiresHumanReview
		payload.FinalConclusion.RequiresHumanReview = &value
	}
	if !payload.RequiresHumanReview {
		payload.RequiresHumanReview = metadata.RequiresHumanReview
	}
	if len(payload.FinalConclusion.Findings) == 0 {
		payload.FinalConclusion.Findings = cloneStrings(metadata.Findings)
	}
	if len(payload.FinalConclusion.RecommendedActions) == 0 {
		payload.FinalConclusion.RecommendedActions = cloneStrings(metadata.RecommendedActions)
	}
	if len(payload.FinalConclusion.EvidenceRequests) == 0 {
		payload.FinalConclusion.EvidenceRequests = cloneEvidenceRequests(metadata.EvidenceRequests)
	}
	if len(payload.FinalConclusion.MissingEvidenceRequests) == 0 {
		payload.FinalConclusion.MissingEvidenceRequests = cloneConsultationEvidenceRequests(metadata.ConsultationInsight.MissingEvidenceRequests)
	}
	if len(payload.FinalConclusion.EvidenceCollectionSuggestions) == 0 {
		payload.FinalConclusion.EvidenceCollectionSuggestions = cloneConsultationEvidenceRequests(metadata.ConsultationInsight.EvidenceCollectionSuggestions)
	}
	if strings.TrimSpace(payload.FinalConclusion.ConfidenceRationale) == "" {
		payload.FinalConclusion.ConfidenceRationale = metadata.confidenceRationale()
	}
}

func (payload *notificationPayload) fillClosePayload(eventPayload diagnosisRoomClosedPayload) {
	payload.FinalConclusion = eventPayload.FinalConclusion
	if payload.CloseReason == "" {
		payload.CloseReason = strings.TrimSpace(eventPayload.CloseReason)
	}
	if payload.TurnCount == 0 {
		payload.TurnCount = eventPayload.TurnCount
	}
	if payload.AssistantMessageID == "" {
		payload.AssistantMessageID = eventPayload.FinalConclusion.AssistantMessageID
	}
	if payload.AssistantTurnID == 0 {
		payload.AssistantTurnID = eventPayload.FinalConclusion.AssistantTurnID
	}
	if payload.AssistantSequence == 0 {
		payload.AssistantSequence = eventPayload.FinalConclusion.AssistantSequence
	}
}

type diagnosisRoomFinalReadyPayload struct {
	Kind               string                              `json:"kind"`
	AssistantMessageID string                              `json:"assistant_message_id,omitempty"`
	AssistantTurnID    int64                               `json:"assistant_turn_id,omitempty"`
	AssistantSequence  int                                 `json:"assistant_sequence,omitempty"`
	FinalConclusion    diagnosisRoomFinalConclusionPayload `json:"final_conclusion,omitempty"`
}

type diagnosisRoomClosedPayload struct {
	Kind            string                              `json:"kind"`
	SessionID       string                              `json:"session_id,omitempty"`
	TurnCount       int                                 `json:"turn_count,omitempty"`
	CloseReason     string                              `json:"close_reason,omitempty"`
	FinalConclusion diagnosisRoomFinalConclusionPayload `json:"final_conclusion,omitempty"`
}

func finalReadyPayloadFromEvent(event domain.DiagnosisTaskEvent) (diagnosisRoomFinalReadyPayload, bool, error) {
	if len(event.Payload) == 0 {
		return diagnosisRoomFinalReadyPayload{}, false, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return diagnosisRoomFinalReadyPayload{}, false, fmt.Errorf("diagnosis notification retry: final-ready event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomFinalReadyPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return diagnosisRoomFinalReadyPayload{}, false, fmt.Errorf("diagnosis notification retry: final-ready event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != eventFinalConclusionReady {
		return diagnosisRoomFinalReadyPayload{}, false, fmt.Errorf("diagnosis notification retry: final-ready event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(payload.FinalConclusion.Content) == "" {
		return diagnosisRoomFinalReadyPayload{}, false, nil
	}
	return payload, true, nil
}

func closedPayloadFromEvent(event domain.DiagnosisTaskEvent) (diagnosisRoomClosedPayload, bool, error) {
	if len(event.Payload) == 0 {
		return diagnosisRoomClosedPayload{}, false, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return diagnosisRoomClosedPayload{}, false, fmt.Errorf("diagnosis notification retry: close event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomClosedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return diagnosisRoomClosedPayload{}, false, fmt.Errorf("diagnosis notification retry: close event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != eventClosed {
		return diagnosisRoomClosedPayload{}, false, fmt.Errorf("diagnosis notification retry: close event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(payload.FinalConclusion.Content) == "" {
		return diagnosisRoomClosedPayload{}, false, nil
	}
	return payload, true, nil
}

func finalReadyPayloadMatchesNotification(eventPayload diagnosisRoomFinalReadyPayload, notification notificationPayload) bool {
	if notification.AssistantMessageID != "" {
		return eventPayload.AssistantMessageID == notification.AssistantMessageID ||
			eventPayload.FinalConclusion.AssistantMessageID == notification.AssistantMessageID
	}
	if notification.AssistantTurnID > 0 {
		return eventPayload.AssistantTurnID == notification.AssistantTurnID ||
			eventPayload.FinalConclusion.AssistantTurnID == notification.AssistantTurnID
	}
	if notification.AssistantSequence > 0 {
		return eventPayload.AssistantSequence == notification.AssistantSequence ||
			eventPayload.FinalConclusion.AssistantSequence == notification.AssistantSequence
	}
	return true
}

func closedPayloadMatchesNotification(eventPayload diagnosisRoomClosedPayload, notification notificationPayload) bool {
	if notification.SessionID != "" {
		return strings.TrimSpace(eventPayload.SessionID) == notification.SessionID
	}
	return true
}

func notificationAIContentProofMissing(payload notificationPayload, eventKind string) bool {
	switch eventKind {
	case EventAssistantTurnNotification:
		return strings.TrimSpace(payload.AssistantMessage) == ""
	case EventFinalReadyNotification, EventCloseNotification:
		return strings.TrimSpace(payload.FinalConclusion.Content) == ""
	default:
		return false
	}
}

func notificationContent(payload notificationPayload) (string, string, string, error) {
	switch payload.Kind {
	case EventAssistantTurnNotification:
		return assistantTurnNotificationContent(payload)
	case EventFinalReadyNotification:
		return finalReadyNotificationContent(payload)
	case EventCloseNotification:
		return closeNotificationContent(payload)
	default:
		return "", "", "", fmt.Errorf("diagnosis notification retry: event_kind %q is unsupported: %w", payload.Kind, domain.ErrInvariantViolation)
	}
}

func assistantTurnNotificationContent(payload notificationPayload) (string, string, string, error) {
	lines := []string{
		assistantTurnNotificationOpeningLine(payload),
	}
	lines = append(lines, notificationReviewLines(payload.RoomURL)...)
	if confidence := strings.TrimSpace(payload.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	lines = append(lines, confidenceRationaleLine(payload.ConsultationInsight.ConfidenceRationale)...)
	if payload.RequiresHumanReview {
		lines = append(lines, "Human review: required")
	} else {
		lines = append(lines, "Human review: not required")
	}
	if status := strings.TrimSpace(payload.ConsultationInsight.ConclusionStatus); status != "" {
		lines = append(lines, "Conclusion status: "+status)
	}
	lines = append(lines, notificationNextActionLine(
		payload.EvidenceRequests,
		payload.ConsultationInsight.MissingEvidenceRequests,
		payload.ConsultationInsight.EvidenceCollectionSuggestions,
		false,
	))
	lines = append(lines, consultationEvidenceRequestLines("Missing evidence", payload.ConsultationInsight.MissingEvidenceRequests)...)
	lines = append(lines, consultationEvidenceRequestLines("Evidence collection suggestions", payload.ConsultationInsight.EvidenceCollectionSuggestions)...)
	lines = append(lines, evidenceRequestLines(payload.EvidenceRequests)...)
	content := strings.TrimSpace(payload.AssistantMessage)
	if content == "" {
		content = "Assistant diagnosis is empty."
	}
	lines = append(lines, "AI diagnosis: "+truncateString(content, maxNotificationBodyRunes))
	lines = append(lines, notificationList("Findings", payload.Findings)...)
	lines = append(lines, notificationList("Recommended actions", payload.RecommendedActions)...)
	return assistantTurnNotificationTitlePrefix(payload) + ": " + payload.SessionID, strings.Join(lines, "\n"), severityFromConfidence(payload.Confidence), nil
}

func assistantTurnNotificationTitlePrefix(payload notificationPayload) string {
	if payload.TurnCount == 1 {
		return "Initial AI diagnosis report"
	}
	return "AI diagnosis update"
}

func assistantTurnNotificationOpeningLine(payload notificationPayload) string {
	if payload.TurnCount == 1 {
		return fmt.Sprintf(
			"Initial AI diagnosis report is ready for room %s on alert group %d after %d turn(s). Review the report, collect missing evidence, and provide supplemental context before confidence is raised or closure is confirmed.",
			payload.SessionID,
			payload.AlertGroupID,
			payload.TurnCount,
		)
	}
	return fmt.Sprintf(
		"AI diagnosis update is ready for room %s on alert group %d after %d turn(s). Review the diagnosis, collect missing evidence, and provide supplemental context before confirming closure.",
		payload.SessionID,
		payload.AlertGroupID,
		payload.TurnCount,
	)
}

func finalReadyNotificationContent(payload notificationPayload) (string, string, string, error) {
	conclusion := payload.FinalConclusion
	lines := []string{
		fmt.Sprintf("AI diagnosis is ready for room %s on alert group %d after %d turn(s). Review the conclusion, provide missing evidence if needed, then confirm closure when ready.", payload.SessionID, payload.AlertGroupID, payload.TurnCount),
	}
	lines = append(lines, notificationReviewLines(payload.RoomURL)...)
	if confidence := strings.TrimSpace(conclusion.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	lines = append(lines, confidenceRationaleLine(conclusion.ConfidenceRationale)...)
	if conclusion.RequiresHumanReview != nil && *conclusion.RequiresHumanReview {
		lines = append(lines, "Human review: required")
	} else if conclusion.RequiresHumanReview != nil {
		lines = append(lines, "Human review: not required")
	}
	lines = append(lines, notificationNextActionLine(
		conclusion.EvidenceRequests,
		conclusion.MissingEvidenceRequests,
		conclusion.EvidenceCollectionSuggestions,
		true,
	))
	lines = append(lines, consultationEvidenceRequestLines("Missing evidence", conclusion.MissingEvidenceRequests)...)
	lines = append(lines, consultationEvidenceRequestLines("Evidence collection suggestions", conclusion.EvidenceCollectionSuggestions)...)
	lines = append(lines, evidenceRequestLines(conclusion.EvidenceRequests)...)
	content := strings.TrimSpace(conclusion.Content)
	if content == "" {
		content = "Assistant conclusion is empty."
	}
	lines = append(lines, "AI conclusion: "+truncateString(content, maxNotificationBodyRunes))
	lines = append(lines, notificationList("Findings", conclusion.Findings)...)
	lines = append(lines, notificationList("Recommended actions", conclusion.RecommendedActions)...)
	return "AI diagnosis ready: " + payload.SessionID, strings.Join(lines, "\n"), severityFromConfidence(conclusion.Confidence), nil
}

func closeNotificationContent(payload notificationPayload) (string, string, string, error) {
	conclusion := payload.FinalConclusion
	lines := []string{
		fmt.Sprintf("Diagnosis room %s closed for alert group %d after %d turn(s).", payload.SessionID, payload.AlertGroupID, payload.TurnCount),
	}
	lines = append(lines, notificationReviewLines(payload.RoomURL)...)
	if reason := strings.TrimSpace(payload.CloseReason); reason != "" {
		lines = append(lines, "Close reason: "+reason)
	}
	if confidence := strings.TrimSpace(conclusion.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	lines = append(lines, confidenceRationaleLine(conclusion.ConfidenceRationale)...)
	lines = append(lines, notificationNextActionLine(
		conclusion.EvidenceRequests,
		conclusion.MissingEvidenceRequests,
		conclusion.EvidenceCollectionSuggestions,
		true,
	))
	lines = append(lines, consultationEvidenceRequestLines("Missing evidence", conclusion.MissingEvidenceRequests)...)
	lines = append(lines, consultationEvidenceRequestLines("Evidence collection suggestions", conclusion.EvidenceCollectionSuggestions)...)
	lines = append(lines, evidenceRequestLines(conclusion.EvidenceRequests)...)
	content := strings.TrimSpace(conclusion.Content)
	if content == "" {
		content = "Final conclusion is unavailable."
	}
	lines = append(lines, "AI conclusion: "+truncateString(content, maxNotificationBodyRunes))
	lines = append(lines, notificationList("Findings", conclusion.Findings)...)
	lines = append(lines, notificationList("Recommended actions", conclusion.RecommendedActions)...)
	return "Diagnosis room closed: " + payload.SessionID, strings.Join(lines, "\n"), severityFromConfidence(conclusion.Confidence), nil
}

func notificationReviewLines(roomURL string) []string {
	roomURL = strings.TrimSpace(roomURL)
	if roomURL == "" {
		return nil
	}
	return []string{"Review room: " + roomURL}
}

func confidenceRationaleLine(rationale string) []string {
	rationale = strings.TrimSpace(rationale)
	if rationale == "" {
		return nil
	}
	return []string{"Confidence rationale: " + truncateString(rationale, 500)}
}

func notificationList(label string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := []string{label + ":"}
	for i, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, fmt.Sprintf("%d. %s", i+1, truncateString(trimmed, 500)))
		}
	}
	if len(out) == 1 {
		return nil
	}
	return out
}

func notificationNextActionLine(
	evidenceRequests []diagnosisroom.EvidenceRequest,
	missingEvidence []diagnosisroom.ConsultationEvidenceRequest,
	collectionSuggestions []diagnosisroom.ConsultationEvidenceRequest,
	finalReady bool,
) string {
	executableCount := len(evidenceRequests)
	missingCount := len(missingEvidence)
	suggestionCount := len(collectionSuggestions)
	switch {
	case executableCount > 0 && missingCount > 0 && suggestionCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s), provide %d operator-supplied evidence item(s), and review %d evidence collection suggestion(s).", executableCount, missingCount, suggestionCount)
	case executableCount > 0 && missingCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s) and provide %d operator-supplied evidence item(s).", executableCount, missingCount)
	case executableCount > 0 && suggestionCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s) and review %d evidence collection suggestion(s).", executableCount, suggestionCount)
	case executableCount > 0:
		return fmt.Sprintf("Next action: collect %d executable evidence request(s) in the diagnosis room.", executableCount)
	case missingCount > 0 && suggestionCount > 0:
		return fmt.Sprintf("Next action: provide %d operator-supplied evidence item(s) and review %d evidence collection suggestion(s) before confidence can be raised.", missingCount, suggestionCount)
	case missingCount > 0:
		return fmt.Sprintf("Next action: provide %d operator-supplied evidence item(s) before confidence can be raised.", missingCount)
	case suggestionCount > 0:
		return fmt.Sprintf("Next action: review %d evidence collection suggestion(s) and decide what to collect next.", suggestionCount)
	case finalReady:
		return "Next action: review the final conclusion and confirm closure when operationally accepted."
	default:
		return "Next action: review the AI diagnosis and continue the consultation if more evidence is needed."
	}
}

func evidenceRequestLines(requests []diagnosisroom.EvidenceRequest) []string {
	if len(requests) == 0 {
		return nil
	}
	out := []string{fmt.Sprintf("Executable evidence requests: %d", len(requests))}
	for i, item := range requests {
		tool := strings.TrimSpace(string(item.Tool))
		if tool == "" {
			tool = "evidence"
		}
		reason := strings.TrimSpace(item.Reason)
		line := fmt.Sprintf("%d. %s", i+1, tool)
		if reason != "" {
			line += " - " + truncateString(reason, 300)
		}
		if item.Limit > 0 {
			line += fmt.Sprintf(" (limit=%d)", item.Limit)
		}
		out = append(out, line)
	}
	return out
}

func consultationEvidenceRequestLines(label string, requests []diagnosisroom.ConsultationEvidenceRequest) []string {
	if len(requests) == 0 {
		return nil
	}
	out := []string{label + ":"}
	for i, item := range requests {
		itemLabel := strings.TrimSpace(item.Label)
		if itemLabel == "" {
			itemLabel = "Evidence"
		}
		line := fmt.Sprintf("%d. %s", i+1, truncateString(itemLabel, 160))
		if priority := strings.TrimSpace(item.Priority); priority != "" {
			line = fmt.Sprintf("%d. [%s] %s", i+1, truncateString(priority, 40), truncateString(itemLabel, 160))
		}
		if detail := strings.TrimSpace(item.Detail); detail != "" {
			line += " - " + truncateString(detail, 300)
		}
		out = append(out, line)
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
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

func normalizeEventKind(raw string) (string, error) {
	eventKind := strings.TrimSpace(raw)
	switch eventKind {
	case EventAssistantTurnNotification, EventFinalReadyNotification, EventCloseNotification:
		return eventKind, nil
	default:
		return "", fmt.Errorf("diagnosis notification retry: event_kind must be a diagnosis notification event kind: %w", domain.ErrInvariantViolation)
	}
}

func notificationFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return true
	default:
		return false
	}
}

func notificationDelivered(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status != "" && !notificationFailed(status)
}

func severityFromConfidence(confidence string) string {
	switch strings.ToLower(strings.TrimSpace(confidence)) {
	case "low":
		return "critical"
	default:
		return "warning"
	}
}

func failedDelivery(err error) ports.IMDelivery {
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
	return ports.IMDelivery{Status: "failed", Raw: raw}
}

func defaultJSONObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	copyRaw := make(json.RawMessage, len(raw))
	copy(copyRaw, raw)
	return copyRaw
}

func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func notificationDedupeComponent(idempotencyKey string, priorAttempts int) string {
	if priorAttempts <= 0 {
		return idempotencyKey
	}
	return fmt.Sprintf("%s/retry-%d", idempotencyKey, priorAttempts)
}

func notificationEventDedupeKey(kind, sessionID, component string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + sessionID + "\x00" + component))
	return "dr:" + notificationEventDedupePrefix(kind) + ":" + hex.EncodeToString(sum[:])[:24]
}

func notificationEventDedupePrefix(kind string) string {
	switch kind {
	case EventAssistantTurnNotification:
		return "turnnotify"
	case EventFinalReadyNotification:
		return "finalnotify"
	case EventCloseNotification:
		return "notify"
	default:
		return "event"
	}
}
