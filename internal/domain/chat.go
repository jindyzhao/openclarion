package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
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

// DiagnosisApprovalMode defines the human approval quorum required before a
// retained conclusion can close a diagnosis room.
type DiagnosisApprovalMode string

const (
	// DiagnosisApprovalModeSingle preserves the existing one-authorized-operator
	// confirmation behavior.
	DiagnosisApprovalModeSingle DiagnosisApprovalMode = "single"
	// DiagnosisApprovalModeOwnerAndLeader requires distinct owner and leader
	// subjects to approve the same conclusion digest.
	DiagnosisApprovalModeOwnerAndLeader DiagnosisApprovalMode = "owner_and_leader"
)

// Valid reports whether m is a supported approval policy.
func (m DiagnosisApprovalMode) Valid() bool {
	switch m {
	case DiagnosisApprovalModeSingle, DiagnosisApprovalModeOwnerAndLeader:
		return true
	default:
		return false
	}
}

// DiagnosisApprovalAuthority identifies the stakeholder capacity in which an
// authenticated subject approved a conclusion.
type DiagnosisApprovalAuthority string

const (
	// DiagnosisApprovalAuthorityOwner records approval by the room owner.
	DiagnosisApprovalAuthorityOwner DiagnosisApprovalAuthority = "owner"
	// DiagnosisApprovalAuthorityLeader records approval by an authorized leader.
	DiagnosisApprovalAuthorityLeader DiagnosisApprovalAuthority = "leader"
)

// Valid reports whether a is a supported approval authority.
func (a DiagnosisApprovalAuthority) Valid() bool {
	return a == DiagnosisApprovalAuthorityOwner || a == DiagnosisApprovalAuthorityLeader
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
	ApprovalMode    DiagnosisApprovalMode
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ChatSessionWithTask is a read model for operator-facing room lists. It keeps
// the ChatSession lifecycle fields together with the DiagnosisTask execution
// that anchors the room to one EvidenceSnapshot.
type ChatSessionWithTask struct {
	Session ChatSession
	Task    DiagnosisTask
}

// NewChatSession constructs an open diagnosis-room chat session.
// Repository insert paths fill ID / CreatedAt / UpdatedAt.
func NewChatSession(taskID DiagnosisTaskID, sessionKey, ownerSubject string, startedAt time.Time, approvalModes ...DiagnosisApprovalMode) (ChatSession, error) {
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
	if len(approvalModes) > 1 {
		return ChatSession{}, fmt.Errorf("chat session: at most one approval mode may be provided: %w", ErrInvariantViolation)
	}
	approvalMode := DiagnosisApprovalModeSingle
	if len(approvalModes) == 1 {
		approvalMode = approvalModes[0]
	}
	if !approvalMode.Valid() {
		return ChatSession{}, fmt.Errorf("chat session: approval_mode %q is unsupported: %w", approvalMode, ErrInvariantViolation)
	}
	normalised := NormalizeUTCMicro(startedAt)
	return ChatSession{
		DiagnosisTaskID: taskID,
		SessionKey:      sessionKey,
		OwnerSubject:    ownerSubject,
		Status:          ChatSessionStatusOpen,
		StartedAt:       normalised,
		LastActivityAt:  normalised,
		ApprovalMode:    approvalMode,
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

// ChatSessionSummary is one immutable compression checkpoint for a diagnosis
// room transcript. SourceDigest binds the summary to the exact ordered turn
// contents while Content retains the versioned, provider-neutral summary JSON.
// Original ChatTurn rows remain the audit source of truth.
type ChatSessionSummary struct {
	ID                  ChatSessionSummaryID
	SessionID           ChatSessionID
	Version             int
	SchemaVersion       string
	SourceFirstSequence int
	SourceLastSequence  int
	SourceTurnCount     int
	SourceDigest        string
	Content             json.RawMessage
	GeneratedAt         time.Time
	CreatedAt           time.Time
}

// ChatSessionApproval is one immutable stakeholder approval bound to an exact
// retained conclusion digest. A later conclusion creates new approvals rather
// than mutating or deleting prior audit rows.
type ChatSessionApproval struct {
	ID               ChatSessionApprovalID
	SessionID        ChatSessionID
	ConclusionDigest string
	ActorSubject     string
	Authority        DiagnosisApprovalAuthority
	Reason           string
	ApprovedAt       time.Time
	CreatedAt        time.Time
}

// NewChatSessionApproval validates one append-only conclusion approval.
func NewChatSessionApproval(in ChatSessionApproval) (ChatSessionApproval, error) {
	if in.SessionID == 0 {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: session_id must be non-zero: %w", ErrInvariantViolation)
	}
	in.ConclusionDigest = strings.TrimSpace(in.ConclusionDigest)
	if len(in.ConclusionDigest) != 64 || !isLowerHex(in.ConclusionDigest) {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: conclusion_digest must be a lowercase SHA-256 hex digest: %w", ErrInvariantViolation)
	}
	in.ActorSubject = strings.TrimSpace(in.ActorSubject)
	if in.ActorSubject == "" || len(in.ActorSubject) > 256 {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: actor_subject must contain 1-256 bytes: %w", ErrInvariantViolation)
	}
	if !in.Authority.Valid() {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: authority %q is unsupported: %w", in.Authority, ErrInvariantViolation)
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || len(in.Reason) > 512 || strings.ContainsFunc(in.Reason, unicode.IsControl) {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: reason must be a single-line value containing 1-512 bytes: %w", ErrInvariantViolation)
	}
	if in.ApprovedAt.IsZero() {
		return ChatSessionApproval{}, fmt.Errorf("chat session approval: approved_at must be set: %w", ErrInvariantViolation)
	}
	in.ApprovedAt = NormalizeUTCMicro(in.ApprovedAt)
	return in, nil
}

// NewChatSessionSummary validates an append-only conversation summary before
// persistence. Empty transcripts use zero source bounds and the SHA-256 digest
// of the canonical empty source document.
func NewChatSessionSummary(in ChatSessionSummary) (ChatSessionSummary, error) {
	if in.SessionID == 0 {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: session_id must be non-zero: %w", ErrInvariantViolation)
	}
	if in.Version <= 0 {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: version must be > 0 (got %d): %w", in.Version, ErrInvariantViolation)
	}
	in.SchemaVersion = strings.TrimSpace(in.SchemaVersion)
	if in.SchemaVersion == "" || len(in.SchemaVersion) > 64 {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: schema_version must contain 1-64 bytes: %w", ErrInvariantViolation)
	}
	if in.SourceTurnCount < 0 {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: source_turn_count must be >= 0 (got %d): %w", in.SourceTurnCount, ErrInvariantViolation)
	}
	if in.SourceTurnCount == 0 {
		if in.SourceFirstSequence != 0 || in.SourceLastSequence != 0 {
			return ChatSessionSummary{}, fmt.Errorf("chat session summary: empty source must use zero sequence bounds: %w", ErrInvariantViolation)
		}
	} else {
		if in.SourceFirstSequence <= 0 || in.SourceLastSequence < in.SourceFirstSequence {
			return ChatSessionSummary{}, fmt.Errorf("chat session summary: invalid source sequence range [%d,%d]: %w", in.SourceFirstSequence, in.SourceLastSequence, ErrInvariantViolation)
		}
		if in.SourceLastSequence-in.SourceFirstSequence+1 != in.SourceTurnCount {
			return ChatSessionSummary{}, fmt.Errorf("chat session summary: source sequence range [%d,%d] does not contain %d turns: %w", in.SourceFirstSequence, in.SourceLastSequence, in.SourceTurnCount, ErrInvariantViolation)
		}
	}
	in.SourceDigest = strings.TrimSpace(in.SourceDigest)
	if len(in.SourceDigest) != 64 || !isLowerHex(in.SourceDigest) {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: source_digest must be a lowercase SHA-256 hex digest: %w", ErrInvariantViolation)
	}
	if len(in.Content) == 0 {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: content must be set: %w", ErrInvariantViolation)
	}
	if err := requireValidJSON("chat session summary: content", in.Content); err != nil {
		return ChatSessionSummary{}, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(in.Content, &object); err != nil || object == nil {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: content must be a JSON object: %w", ErrInvariantViolation)
	}
	if in.GeneratedAt.IsZero() {
		return ChatSessionSummary{}, fmt.Errorf("chat session summary: generated_at must be set: %w", ErrInvariantViolation)
	}
	in.GeneratedAt = NormalizeUTCMicro(in.GeneratedAt)
	return in, nil
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
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
