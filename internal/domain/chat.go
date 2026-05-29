package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ChatSessionStatus is the lifecycle state for an M5 diagnosis-room
// chat session. The database stores this as text, not a database
// enum, so future workflow states do not require a schema migration.
type ChatSessionStatus string

const (
	// ChatSessionStatusOpen means the diagnosis room accepts more
	// turns subject to policy limits.
	ChatSessionStatusOpen ChatSessionStatus = "open"
	// ChatSessionStatusClosed means the diagnosis room is terminal and
	// no more turns may be appended.
	ChatSessionStatusClosed ChatSessionStatus = "closed"
)

// Valid reports whether s is a known ChatSessionStatus value.
func (s ChatSessionStatus) Valid() bool {
	switch s {
	case ChatSessionStatusOpen, ChatSessionStatusClosed:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the session is closed to further turns.
func (s ChatSessionStatus) IsTerminal() bool {
	return s == ChatSessionStatusClosed
}

// ChatRole is the role vocabulary persisted for diagnosis-room turns
// and later mounted into /workspace/conversation.json.
type ChatRole string

const (
	// ChatRoleUser is a human/user message submitted through the
	// diagnosis-room transport.
	ChatRoleUser ChatRole = "user"
	// ChatRoleAssistant is the sandboxed diagnosis assistant response.
	ChatRoleAssistant ChatRole = "assistant"
	// ChatRoleSystem is a control-plane/system transcript entry.
	ChatRoleSystem ChatRole = "system"
	// ChatRoleTool is a tool-observation transcript entry.
	ChatRoleTool ChatRole = "tool"
)

// Valid reports whether r is a known ChatRole value.
func (r ChatRole) Valid() bool {
	switch r {
	case ChatRoleUser, ChatRoleAssistant, ChatRoleSystem, ChatRoleTool:
		return true
	default:
		return false
	}
}

// ChatSession is the persisted lifecycle record for one M5 short
// diagnosis-room workflow execution. It remains alert-analysis-first by
// anchoring every room to a DiagnosisTask, while SessionKey is the
// stable external id used by WebSocket auth and reconnect flows.
type ChatSession struct {
	ID              ChatSessionID
	DiagnosisTaskID DiagnosisTaskID
	SessionKey      string
	OwnerSubject    string
	Status          ChatSessionStatus
	TurnCount       int
	StartedAt       time.Time
	LastActivityAt  time.Time
	ClosedAt        *time.Time
	CloseReason     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewChatSession constructs an open diagnosis-room chat session.
// Repository insert paths fill ID / CreatedAt / UpdatedAt.
func NewChatSession(taskID DiagnosisTaskID, sessionKey, ownerSubject string, startedAt time.Time) (ChatSession, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	ownerSubject = strings.TrimSpace(ownerSubject)
	if taskID == 0 {
		return ChatSession{}, fmt.Errorf("chat session: diagnosis_task_id must be non-zero: %w", ErrInvariantViolation)
	}
	if sessionKey == "" {
		return ChatSession{}, fmt.Errorf("chat session: session_key must be non-empty: %w", ErrInvariantViolation)
	}
	if ownerSubject == "" {
		return ChatSession{}, fmt.Errorf("chat session: owner_subject must be non-empty: %w", ErrInvariantViolation)
	}
	if startedAt.IsZero() {
		return ChatSession{}, fmt.Errorf("chat session: started_at must be set: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(startedAt)
	return ChatSession{
		DiagnosisTaskID: taskID,
		SessionKey:      sessionKey,
		OwnerSubject:    ownerSubject,
		Status:          ChatSessionStatusOpen,
		StartedAt:       normalised,
		LastActivityAt:  normalised,
	}, nil
}

// RecordTurn advances LastActivityAt and TurnCount after a turn has
// been accepted by the workflow policy.
func (s ChatSession) RecordTurn(occurredAt time.Time) (ChatSession, error) {
	if s.Status.IsTerminal() {
		return ChatSession{}, fmt.Errorf("chat session: cannot record turn on terminal status %q: %w", s.Status, ErrInvariantViolation)
	}
	if occurredAt.IsZero() {
		return ChatSession{}, fmt.Errorf("chat session: turn occurred_at must be set: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(occurredAt)
	if s.StartedAt.IsZero() {
		return ChatSession{}, fmt.Errorf("chat session: started_at must be set before recording turns: %w", ErrInvariantViolation)
	}
	if normalised.Before(s.StartedAt) {
		return ChatSession{}, fmt.Errorf("chat session: turn occurred_at %s precedes started_at %s: %w", normalised, s.StartedAt, ErrInvariantViolation)
	}
	if s.TurnCount < 0 {
		return ChatSession{}, fmt.Errorf("chat session: turn_count %d must be >= 0: %w", s.TurnCount, ErrInvariantViolation)
	}
	s.TurnCount++
	s.LastActivityAt = normalised
	return s, nil
}

// Close moves the session to its terminal state and records the
// workflow/user/system reason that ended the room.
func (s ChatSession) Close(closedAt time.Time, reason string) (ChatSession, error) {
	reason = strings.TrimSpace(reason)
	if closedAt.IsZero() {
		return ChatSession{}, fmt.Errorf("chat session: closed_at must be set: %w", ErrInvariantViolation)
	}
	if reason == "" {
		return ChatSession{}, fmt.Errorf("chat session: close_reason must be non-empty: %w", ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(closedAt)
	if s.StartedAt.IsZero() {
		return ChatSession{}, fmt.Errorf("chat session: started_at must be set before close: %w", ErrInvariantViolation)
	}
	if normalised.Before(s.StartedAt) {
		return ChatSession{}, fmt.Errorf("chat session: closed_at %s precedes started_at %s: %w", normalised, s.StartedAt, ErrInvariantViolation)
	}
	if s.Status.IsTerminal() {
		if s.ClosedAt != nil && s.ClosedAt.Equal(normalised) && s.CloseReason == reason {
			return s, nil
		}
		return ChatSession{}, fmt.Errorf("chat session: terminal close metadata is immutable: %w", ErrInvariantViolation)
	}
	s.Status = ChatSessionStatusClosed
	s.ClosedAt = &normalised
	s.CloseReason = reason
	s.LastActivityAt = normalised
	return s, nil
}

// ChatTurn is one immutable diagnosis-room transcript entry. User
// turns and assistant responses are both persisted so crash recovery
// can replay from PostgreSQL and Temporal workflow state without
// relying on container memory.
type ChatTurn struct {
	ID           ChatTurnID
	SessionID    ChatSessionID
	MessageID    string
	Sequence     int
	Role         ChatRole
	ActorSubject string
	Content      string
	Metadata     json.RawMessage
	OccurredAt   time.Time
	CreatedAt    time.Time
}

// NewChatTurn constructs an append-only turn ready for persistence.
// MessageID is unique per session and Sequence is the monotonically
// increasing transcript order.
func NewChatTurn(in ChatTurn) (ChatTurn, error) {
	if in.SessionID == 0 {
		return ChatTurn{}, fmt.Errorf("chat turn: session_id must be non-zero: %w", ErrInvariantViolation)
	}
	in.MessageID = strings.TrimSpace(in.MessageID)
	if in.MessageID == "" {
		return ChatTurn{}, fmt.Errorf("chat turn: message_id must be non-empty: %w", ErrInvariantViolation)
	}
	if in.Sequence <= 0 {
		return ChatTurn{}, fmt.Errorf("chat turn: sequence must be > 0 (got %d): %w", in.Sequence, ErrInvariantViolation)
	}
	if !in.Role.Valid() {
		return ChatTurn{}, fmt.Errorf("chat turn: role %q is invalid: %w", in.Role, ErrInvariantViolation)
	}
	in.ActorSubject = strings.TrimSpace(in.ActorSubject)
	if in.ActorSubject == "" {
		return ChatTurn{}, fmt.Errorf("chat turn: actor_subject must be non-empty: %w", ErrInvariantViolation)
	}
	in.Content = strings.TrimSpace(in.Content)
	if in.Content == "" {
		return ChatTurn{}, fmt.Errorf("chat turn: content must be non-empty: %w", ErrInvariantViolation)
	}
	if in.OccurredAt.IsZero() {
		return ChatTurn{}, fmt.Errorf("chat turn: occurred_at must be set: %w", ErrInvariantViolation)
	}
	in.OccurredAt = NormalizeUTCMicro(in.OccurredAt)
	in.Metadata = defaultJSONObject(in.Metadata)
	if err := requireValidJSON("chat turn: metadata", in.Metadata); err != nil {
		return ChatTurn{}, err
	}
	return in, nil
}
