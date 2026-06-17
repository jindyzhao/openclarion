// Package diagnosisroom owns pure M5 short-conversation policy checks.
// Transport, Temporal, persistence, and sandbox adapters call these helpers
// instead of reimplementing turn limits or unsafe-message checks.
package diagnosisroom

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	// HardMaxTurns is the maximum accepted M5 V1 turn count.
	HardMaxTurns = 20
	// HardMaxSessionTTL is the maximum accepted M5 V1 session lifetime.
	HardMaxSessionTTL = 30 * time.Minute
	// HardMaxIdleTimeout is the maximum accepted M5 V1 idle timeout.
	HardMaxIdleTimeout = 30 * time.Minute
	// HardMaxTurnTimeout is the maximum accepted M5 V1 per-turn timeout.
	HardMaxTurnTimeout = 3 * time.Minute
	// HardMaxContextBytes is the maximum accepted mounted context size.
	HardMaxContextBytes = 2 * 1024 * 1024
	// HardMaxMessageBytes is the maximum accepted user message size.
	HardMaxMessageBytes = 64 * 1024
	// HardMaxAutoEvidenceFollowUps is the maximum workflow-triggered follow-up
	// turns allowed after a user turn collects new evidence.
	HardMaxAutoEvidenceFollowUps = 3

	// DefaultMaxTurns is the default V1 short-conversation turn cap.
	DefaultMaxTurns = 20
	// DefaultSessionTTL is the default V1 session lifetime.
	DefaultSessionTTL = 30 * time.Minute
	// DefaultIdleTimeout is the default V1 idle timeout.
	DefaultIdleTimeout = 10 * time.Minute
	// DefaultTurnTimeout is the default V1 per-turn timeout.
	DefaultTurnTimeout = 2 * time.Minute
	// DefaultContextBytes is the default mounted context budget.
	DefaultContextBytes = 256 * 1024
	// DefaultMaxMessageBytes is the default user message byte cap.
	DefaultMaxMessageBytes = 8 * 1024
	// DefaultMaxAutoEvidenceFollowUps is the default automatic follow-up cap.
	DefaultMaxAutoEvidenceFollowUps = 1
)

// Policy is the M5 V1 blast-radius boundary for one diagnosis room.
// It is intentionally small and serialisable so the same values can be
// stored with a workflow/session and checked before every turn.
type Policy struct {
	MaxTurns                 int
	MaxAutoEvidenceFollowUps int
	SessionTTL               time.Duration
	IdleTimeout              time.Duration
	TurnTimeout              time.Duration
	ContextBytes             int
	MaxMessageBytes          int
	UnsafeDenylist           []string
}

// SessionState is the workflow-visible state needed to validate an Update.
// SeenMessageIDs is copied by callers from durable workflow/session state.
type SessionState struct {
	StartedAt      time.Time
	LastActivityAt time.Time
	TurnCount      int
	InFlight       bool
	SeenMessageIDs map[string]struct{}
}

// ConversationTurn is the bounded transcript shape mounted as
// /workspace/conversation.json.
type ConversationTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SubmitTurnRequest is the pre-sandbox validation input for one user turn.
type SubmitTurnRequest struct {
	MessageID    string
	Message      string
	Now          time.Time
	Evidence     json.RawMessage
	Conversation []ConversationTurn
}

// Decision captures the deterministic output of a pre-submit validation.
type Decision struct {
	ContextBytes int
}

// DefaultPolicy returns the conservative V1 short-conversation policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxTurns:                 DefaultMaxTurns,
		MaxAutoEvidenceFollowUps: DefaultMaxAutoEvidenceFollowUps,
		SessionTTL:               DefaultSessionTTL,
		IdleTimeout:              DefaultIdleTimeout,
		TurnTimeout:              DefaultTurnTimeout,
		ContextBytes:             DefaultContextBytes,
		MaxMessageBytes:          DefaultMaxMessageBytes,
		UnsafeDenylist: []string{
			"ignore previous instructions",
			"reveal system prompt",
			"print system prompt",
			"show hidden instructions",
			"exfiltrate",
			"dump secrets",
			"disable safety",
			"/var/run/docker.sock",
		},
	}
}

// ValidatePolicy rejects configurations that would silently widen the V1
// short-conversation blast radius.
func ValidatePolicy(policy Policy) error {
	if policy.MaxTurns <= 0 || policy.MaxTurns > HardMaxTurns {
		return fmt.Errorf("diagnosis room policy: max turns %d must be in [1,%d]: %w", policy.MaxTurns, HardMaxTurns, domain.ErrInvariantViolation)
	}
	if policy.MaxAutoEvidenceFollowUps < 0 || policy.MaxAutoEvidenceFollowUps > HardMaxAutoEvidenceFollowUps {
		return fmt.Errorf("diagnosis room policy: max auto evidence follow-ups %d must be in [0,%d]: %w", policy.MaxAutoEvidenceFollowUps, HardMaxAutoEvidenceFollowUps, domain.ErrInvariantViolation)
	}
	if policy.SessionTTL <= 0 || policy.SessionTTL > HardMaxSessionTTL {
		return fmt.Errorf("diagnosis room policy: session ttl %s must be in (0,%s]: %w", policy.SessionTTL, HardMaxSessionTTL, domain.ErrInvariantViolation)
	}
	if policy.IdleTimeout <= 0 || policy.IdleTimeout > HardMaxIdleTimeout {
		return fmt.Errorf("diagnosis room policy: idle timeout %s must be in (0,%s]: %w", policy.IdleTimeout, HardMaxIdleTimeout, domain.ErrInvariantViolation)
	}
	if policy.IdleTimeout > policy.SessionTTL {
		return fmt.Errorf("diagnosis room policy: idle timeout %s exceeds session ttl %s: %w", policy.IdleTimeout, policy.SessionTTL, domain.ErrInvariantViolation)
	}
	if policy.TurnTimeout <= 0 || policy.TurnTimeout > HardMaxTurnTimeout {
		return fmt.Errorf("diagnosis room policy: turn timeout %s must be in (0,%s]: %w", policy.TurnTimeout, HardMaxTurnTimeout, domain.ErrInvariantViolation)
	}
	if policy.TurnTimeout > policy.SessionTTL {
		return fmt.Errorf("diagnosis room policy: turn timeout %s exceeds session ttl %s: %w", policy.TurnTimeout, policy.SessionTTL, domain.ErrInvariantViolation)
	}
	if policy.ContextBytes <= 0 || policy.ContextBytes > HardMaxContextBytes {
		return fmt.Errorf("diagnosis room policy: context bytes %d must be in [1,%d]: %w", policy.ContextBytes, HardMaxContextBytes, domain.ErrInvariantViolation)
	}
	if policy.MaxMessageBytes <= 0 || policy.MaxMessageBytes > HardMaxMessageBytes {
		return fmt.Errorf("diagnosis room policy: max message bytes %d must be in [1,%d]: %w", policy.MaxMessageBytes, HardMaxMessageBytes, domain.ErrInvariantViolation)
	}
	for i, term := range policy.UnsafeDenylist {
		if strings.TrimSpace(term) == "" {
			return fmt.Errorf("diagnosis room policy: unsafe denylist[%d] must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

// ValidateSubmitTurn returns nil only when a user message is safe to hand to a
// Temporal Update handler and then to the per-turn sandbox Activity.
func ValidateSubmitTurn(policy Policy, state SessionState, req SubmitTurnRequest) (Decision, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Decision{}, err
	}
	if err := validateState(policy, state, req.Now); err != nil {
		return Decision{}, err
	}
	messageID := strings.TrimSpace(req.MessageID)
	if messageID == "" {
		return Decision{}, fmt.Errorf("diagnosis room turn: message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if messageID != req.MessageID {
		return Decision{}, fmt.Errorf("diagnosis room turn: message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if _, exists := state.SeenMessageIDs[messageID]; exists {
		return Decision{}, fmt.Errorf("diagnosis room turn: duplicate message_id %q: %w", messageID, domain.ErrAlreadyExists)
	}
	if strings.TrimSpace(req.Message) == "" {
		return Decision{}, fmt.Errorf("diagnosis room turn: message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len([]byte(req.Message)) > policy.MaxMessageBytes {
		return Decision{}, fmt.Errorf("diagnosis room turn: message is %d bytes, max %d: %w", len([]byte(req.Message)), policy.MaxMessageBytes, domain.ErrInvariantViolation)
	}
	if match, blocked := MatchUnsafeInstruction(policy, req.Message); blocked {
		return Decision{}, fmt.Errorf("diagnosis room turn: message matches unsafe denylist term %q: %w", match, domain.ErrInvariantViolation)
	}
	contextBytes, err := MountContextBytes(req.Evidence, req.Conversation, req.Message)
	if err != nil {
		return Decision{}, err
	}
	if contextBytes > policy.ContextBytes {
		return Decision{}, fmt.Errorf("diagnosis room turn: mounted context is %d bytes, max %d: %w", contextBytes, policy.ContextBytes, domain.ErrInvariantViolation)
	}
	return Decision{ContextBytes: contextBytes}, nil
}

func validateState(policy Policy, state SessionState, now time.Time) error {
	if now.IsZero() {
		return fmt.Errorf("diagnosis room turn: now must be set: %w", domain.ErrInvariantViolation)
	}
	if state.StartedAt.IsZero() {
		return fmt.Errorf("diagnosis room state: started_at must be set: %w", domain.ErrInvariantViolation)
	}
	if state.LastActivityAt.IsZero() {
		return fmt.Errorf("diagnosis room state: last_activity_at must be set: %w", domain.ErrInvariantViolation)
	}
	if now.Before(state.StartedAt) {
		return fmt.Errorf("diagnosis room state: now %s precedes started_at %s: %w", now, state.StartedAt, domain.ErrInvariantViolation)
	}
	if state.LastActivityAt.Before(state.StartedAt) {
		return fmt.Errorf("diagnosis room state: last_activity_at %s precedes started_at %s: %w", state.LastActivityAt, state.StartedAt, domain.ErrInvariantViolation)
	}
	if now.Sub(state.StartedAt) >= policy.SessionTTL {
		return fmt.Errorf("diagnosis room state: session ttl %s reached: %w", policy.SessionTTL, domain.ErrInvariantViolation)
	}
	if now.Sub(state.LastActivityAt) >= policy.IdleTimeout {
		return fmt.Errorf("diagnosis room state: idle timeout %s reached: %w", policy.IdleTimeout, domain.ErrInvariantViolation)
	}
	if state.TurnCount < 0 {
		return fmt.Errorf("diagnosis room state: turn count %d must be >= 0: %w", state.TurnCount, domain.ErrInvariantViolation)
	}
	if state.TurnCount >= policy.MaxTurns {
		return fmt.Errorf("diagnosis room state: max turns %d reached: %w", policy.MaxTurns, domain.ErrInvariantViolation)
	}
	if state.InFlight {
		return fmt.Errorf("diagnosis room state: turn already in progress: %w", domain.ErrAlreadyExists)
	}
	return nil
}

// MatchUnsafeInstruction performs a deterministic, case-insensitive substring
// match against the configured denylist.
func MatchUnsafeInstruction(policy Policy, message string) (string, bool) {
	lower := strings.ToLower(message)
	for _, term := range policy.UnsafeDenylist {
		normalised := strings.ToLower(strings.TrimSpace(term))
		if normalised != "" && strings.Contains(lower, normalised) {
			return term, true
		}
	}
	return "", false
}

// MountContextBytes returns the total bytes for the three ADR-0013 M5 input
// files the sandbox would receive: evidence.json, conversation.json, and
// message.json.
func MountContextBytes(evidence json.RawMessage, conversation []ConversationTurn, latestMessage string) (int, error) {
	if len(evidence) == 0 {
		return 0, fmt.Errorf("diagnosis room context: evidence must be non-empty JSON: %w", domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(evidence); err != nil {
		return 0, fmt.Errorf("diagnosis room context: evidence must be duplicate-key-free JSON: %w: %w", err, domain.ErrInvariantViolation)
	}
	for i, turn := range conversation {
		if !validRole(turn.Role) {
			return 0, fmt.Errorf("diagnosis room context: conversation[%d].role %q is invalid: %w", i, turn.Role, domain.ErrInvariantViolation)
		}
		if strings.TrimSpace(turn.Content) == "" {
			return 0, fmt.Errorf("diagnosis room context: conversation[%d].content must be non-empty: %w", i, domain.ErrInvariantViolation)
		}
	}
	conversationRaw, err := json.Marshal(conversation)
	if err != nil {
		return 0, fmt.Errorf("diagnosis room context: marshal conversation: %w", err)
	}
	messageRaw, err := json.Marshal(ConversationTurn{Role: "user", Content: latestMessage})
	if err != nil {
		return 0, fmt.Errorf("diagnosis room context: marshal message: %w", err)
	}
	return len(evidence) + len(conversationRaw) + len(messageRaw), nil
}

func validRole(role string) bool {
	switch role {
	case "user", "assistant", "system", "tool":
		return true
	default:
		return false
	}
}
