// Package diagnosiswecomcallback routes Enterprise WeChat app messages into
// diagnosis-room conversations.
package diagnosiswecomcallback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/providers/im/wecomcallback"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultSubmitTimeout = 20 * time.Second
	maxMessageRunes      = 4000

	// StatusSubmitted means the message was submitted to the diagnosis workflow.
	StatusSubmitted Status = "submitted"
	// StatusSkippedNoSession means no diagnosis session id was found in the message.
	StatusSkippedNoSession Status = "skipped_no_session"
	// StatusSkippedNonText means the callback message type is not supported.
	StatusSkippedNonText Status = "skipped_non_text"
	// StatusSkippedEmptyText means the callback text body was empty after trimming.
	StatusSkippedEmptyText Status = "skipped_empty_text"
	// StatusSkippedInvalidMsg means the callback lacked required actor metadata.
	StatusSkippedInvalidMsg Status = "skipped_invalid_message"
	// StatusSkippedUnauthorized means the sender is known but cannot participate in the referenced room.
	StatusSkippedUnauthorized Status = "skipped_unauthorized"
)

var diagnosisSessionIDPattern = regexp.MustCompile(`diagnosis-session-[A-Za-z0-9._~-]+`)

// Status reports how an app callback message was handled.
type Status string

// Request carries one verified and decrypted Enterprise WeChat app callback.
type Request struct {
	Message wecomcallback.Message
}

// Result reports the routed room turn or the reason the message was ignored.
type Result struct {
	Status       Status
	SessionID    string
	MessageID    string
	ActorSubject string
}

// RoomAuthorizer authorizes a verified Enterprise WeChat sender against the
// referenced diagnosis room before the message is submitted.
type RoomAuthorizer interface {
	AuthorizeDiagnosisRoomParticipation(ctx context.Context, subject, sessionID string) (bool, error)
}

// SenderResolver maps an Enterprise WeChat sender id to the OpenClarion IAM
// subject used for RBAC and diagnosis-room transcript attribution.
type SenderResolver interface {
	ResolveWeComSenderSubject(ctx context.Context, wecomUserID string) (string, error)
}

// Service submits supported Enterprise WeChat app messages to diagnosis rooms.
type Service struct {
	workflows      ports.DiagnosisRoomWorkflowClient
	authorizer     RoomAuthorizer
	senderResolver SenderResolver
	submitTimeout  time.Duration
}

// Option customizes Service construction.
type Option func(*Service)

// WithSubmitTimeout overrides the maximum synchronous workflow update wait.
func WithSubmitTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.submitTimeout = timeout
		}
	}
}

// WithRoomAuthorizer requires local authorization before app-message turns are
// submitted to the diagnosis-room workflow.
func WithRoomAuthorizer(authorizer RoomAuthorizer) Option {
	return func(s *Service) {
		s.authorizer = authorizer
	}
}

// WithSenderResolver maps verified Enterprise WeChat sender ids to local IAM
// subjects before authorization and workflow submission.
func WithSenderResolver(resolver SenderResolver) Option {
	return func(s *Service) {
		s.senderResolver = resolver
	}
}

// NewService constructs a WeCom app callback router.
func NewService(workflows ports.DiagnosisRoomWorkflowClient, opts ...Option) (*Service, error) {
	if workflows == nil {
		return nil, fmt.Errorf("diagnosis wecom callback: workflow client is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		workflows:     workflows,
		submitTimeout: defaultSubmitTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// HandleMessage routes supported app messages into the referenced diagnosis
// room. Messages without an explicit session reference are acknowledged but not
// submitted so the app callback cannot guess an operator's active room.
func (s *Service) HandleMessage(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.workflows == nil {
		return Result{}, fmt.Errorf("diagnosis wecom callback: service is not configured: %w", domain.ErrInvariantViolation)
	}
	message := req.Message
	if !strings.EqualFold(strings.TrimSpace(message.MsgType), "text") {
		return Result{Status: StatusSkippedNonText}, nil
	}
	content := truncateMessage(strings.TrimSpace(message.Content))
	if content == "" {
		return Result{Status: StatusSkippedEmptyText}, nil
	}
	sessionID := ExtractDiagnosisSessionID(content)
	if sessionID == "" {
		sessionID = ExtractDiagnosisSessionID(message.EventKey)
	}
	if sessionID == "" {
		return Result{Status: StatusSkippedNoSession}, nil
	}
	wecomUserID := strings.TrimSpace(message.FromUserName)
	if wecomUserID == "" {
		return Result{Status: StatusSkippedInvalidMsg, SessionID: sessionID}, nil
	}
	actorSubject := wecomUserID
	if s.senderResolver != nil {
		resolved, err := s.senderResolver.ResolveWeComSenderSubject(ctx, wecomUserID)
		if errors.Is(err, domain.ErrNotFound) {
			return Result{
				Status:       StatusSkippedUnauthorized,
				SessionID:    sessionID,
				ActorSubject: wecomUserID,
			}, nil
		}
		if err != nil {
			return Result{}, fmt.Errorf("diagnosis wecom callback: resolve sender subject: %w", err)
		}
		actorSubject = strings.TrimSpace(resolved)
		if actorSubject == "" {
			return Result{}, fmt.Errorf("diagnosis wecom callback: sender resolver returned blank subject: %w", domain.ErrInvariantViolation)
		}
	}
	if s.authorizer == nil {
		result := Result{
			Status:       StatusSkippedUnauthorized,
			SessionID:    sessionID,
			ActorSubject: actorSubject,
		}
		err := fmt.Errorf("diagnosis wecom callback: room authorizer is required: %w", domain.ErrInvariantViolation)
		return result, err
	}
	allowed, err := s.authorizer.AuthorizeDiagnosisRoomParticipation(ctx, actorSubject, sessionID)
	if err != nil {
		return Result{}, fmt.Errorf("diagnosis wecom callback: authorize room participation: %w", err)
	}
	if !allowed {
		return Result{
			Status:       StatusSkippedUnauthorized,
			SessionID:    sessionID,
			ActorSubject: actorSubject,
		}, nil
	}
	messageID := callbackMessageID(message, sessionID, content)
	submitCtx, cancel := context.WithTimeout(ctx, s.submitTimeout)
	defer cancel()
	if _, err := s.workflows.SubmitDiagnosisTurn(submitCtx, ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    sessionID,
		MessageID:    messageID,
		ActorSubject: actorSubject,
		Message:      content,
	}); err != nil {
		if errors.Is(err, diagnosisroom.ErrDuplicateMessageID) {
			return Result{
				Status:       StatusSubmitted,
				SessionID:    sessionID,
				MessageID:    messageID,
				ActorSubject: actorSubject,
			}, nil
		}
		return Result{}, fmt.Errorf("diagnosis wecom callback: submit turn: %w", err)
	}
	return Result{
		Status:       StatusSubmitted,
		SessionID:    sessionID,
		MessageID:    messageID,
		ActorSubject: actorSubject,
	}, nil
}

// ExtractDiagnosisSessionID returns the first diagnosis room session id found
// in a text message, URL, query string, or event key.
func ExtractDiagnosisSessionID(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if sessionID := sessionIDFromURL(text); sessionID != "" {
		return sessionID
	}
	if values, err := url.ParseQuery(text); err == nil {
		if sessionID := normalizeSessionID(values.Get("session_id")); sessionID != "" {
			return sessionID
		}
	}
	match := diagnosisSessionIDPattern.FindString(text)
	return normalizeSessionID(match)
}

func sessionIDFromURL(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		fields = []string{raw}
	}
	for _, field := range fields {
		candidate := strings.Trim(field, "<>()[]{}\"'")
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.RawQuery == "" {
			continue
		}
		if sessionID := normalizeSessionID(parsed.Query().Get("session_id")); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func normalizeSessionID(raw string) string {
	sessionID := strings.TrimSpace(raw)
	sessionID = strings.TrimRight(sessionID, ".,;:)]}>")
	if !strings.HasPrefix(sessionID, "diagnosis-session-") {
		return ""
	}
	if sessionID != strings.TrimSpace(sessionID) || strings.ContainsAny(sessionID, " \t\r\n") {
		return ""
	}
	return sessionID
}

func callbackMessageID(message wecomcallback.Message, sessionID, content string) string {
	sourceID := strings.TrimSpace(message.MsgID)
	if sourceID == "" {
		sourceID = fmt.Sprintf("%s/%d/%s/%s", message.FromUserName, message.CreateTime, sessionID, content)
	}
	sum := sha256.Sum256([]byte(sourceID))
	return "wecom-app:" + hex.EncodeToString(sum[:])[:32]
}

func truncateMessage(text string) string {
	runes := []rune(text)
	if len(runes) <= maxMessageRunes {
		return text
	}
	return string(runes[:maxMessageRunes])
}
