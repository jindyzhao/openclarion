// Package diagnosiscompression creates bounded, auditable conversation
// summaries from persisted diagnosis-room turns.
package diagnosiscompression

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	// SummaryVersion is the append-only summary revision used for the first
	// lifecycle-end compression checkpoint in a room.
	SummaryVersion = 1
	// SchemaVersion identifies the JSON document contract stored in Content.
	SchemaVersion = "diagnosis-conversation-summary.v1"

	// MaxSourceTurns bounds one compression checkpoint before allocation and
	// digest generation. Callers should query at most MaxSourceTurns+1 rows so
	// truncation is detected rather than silently summarized.
	MaxSourceTurns              = 10_000
	maxOpeningRequestRunes      = 1_024
	maxLatestRequestRunes       = 1_024
	maxLatestAssistantRunes     = 4_096
	maxAssistantHighlightRunes  = 1_024
	maxAssistantHighlightCount  = 3
	compressionMethodExtractive = "deterministic-extractive"
)

// Content is the bounded, provider-neutral JSON document stored for one
// summary. Every text field is an exact prefix extracted from an immutable
// source turn; TruncatedFields records when a bound was applied.
type Content struct {
	SchemaVersion           string   `json:"schema_version"`
	CompressionMethod       string   `json:"compression_method"`
	SourceTurnCount         int      `json:"source_turn_count"`
	OpeningRequest          string   `json:"opening_request,omitempty"`
	LatestRequest           string   `json:"latest_request,omitempty"`
	LatestAssistantResponse string   `json:"latest_assistant_response,omitempty"`
	AssistantHighlights     []string `json:"assistant_highlights,omitempty"`
	TruncatedFields         []string `json:"truncated_fields,omitempty"`
}

type digestTurn struct {
	Sequence     int             `json:"sequence"`
	MessageID    string          `json:"message_id"`
	Role         domain.ChatRole `json:"role"`
	ActorSubject string          `json:"actor_subject"`
	Content      string          `json:"content"`
}

// Summarize creates a deterministic summary from the complete ordered
// persisted transcript for one session.
func Summarize(sessionID domain.ChatSessionID, turns []domain.ChatTurn, generatedAt time.Time) (domain.ChatSessionSummary, error) {
	if sessionID == 0 {
		return domain.ChatSessionSummary{}, fmt.Errorf("compress diagnosis conversation: session_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if len(turns) > MaxSourceTurns {
		return domain.ChatSessionSummary{}, fmt.Errorf("compress diagnosis conversation: source has %d turns, max %d: %w", len(turns), MaxSourceTurns, domain.ErrInvariantViolation)
	}
	if err := validateTurns(sessionID, turns); err != nil {
		return domain.ChatSessionSummary{}, err
	}

	content := summarizeContent(turns)
	raw, err := json.Marshal(content)
	if err != nil {
		return domain.ChatSessionSummary{}, fmt.Errorf("compress diagnosis conversation: marshal summary: %w", err)
	}
	digest, err := sourceDigest(turns)
	if err != nil {
		return domain.ChatSessionSummary{}, err
	}
	firstSequence, lastSequence := 0, 0
	if len(turns) > 0 {
		firstSequence = turns[0].Sequence
		lastSequence = turns[len(turns)-1].Sequence
	}
	return domain.NewChatSessionSummary(domain.ChatSessionSummary{
		SessionID:           sessionID,
		Version:             SummaryVersion,
		SchemaVersion:       SchemaVersion,
		SourceFirstSequence: firstSequence,
		SourceLastSequence:  lastSequence,
		SourceTurnCount:     len(turns),
		SourceDigest:        digest,
		Content:             raw,
		GeneratedAt:         generatedAt,
	})
}

// ParseContent strictly decodes and validates a persisted summary document for
// API and UI read models.
func ParseContent(raw json.RawMessage) (Content, error) {
	var content Content
	if err := strictjson.Unmarshal(raw, &content); err != nil {
		return Content{}, fmt.Errorf("parse diagnosis conversation summary: %w", err)
	}
	if err := validateContent(content); err != nil {
		return Content{}, err
	}
	return content, nil
}

func validateContent(content Content) error {
	if content.SchemaVersion != SchemaVersion {
		return fmt.Errorf("parse diagnosis conversation summary: unsupported schema_version %q: %w", content.SchemaVersion, domain.ErrInvariantViolation)
	}
	if content.CompressionMethod != compressionMethodExtractive {
		return fmt.Errorf("parse diagnosis conversation summary: unsupported compression_method %q: %w", content.CompressionMethod, domain.ErrInvariantViolation)
	}
	if content.SourceTurnCount < 0 || content.SourceTurnCount > MaxSourceTurns {
		return fmt.Errorf("parse diagnosis conversation summary: source_turn_count must be in [0,%d]: %w", MaxSourceTurns, domain.ErrInvariantViolation)
	}
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{name: "opening_request", value: content.OpeningRequest, limit: maxOpeningRequestRunes},
		{name: "latest_request", value: content.LatestRequest, limit: maxLatestRequestRunes},
		{name: "latest_assistant_response", value: content.LatestAssistantResponse, limit: maxLatestAssistantRunes},
	}
	if len(content.AssistantHighlights) > maxAssistantHighlightCount {
		return fmt.Errorf("parse diagnosis conversation summary: assistant_highlights has %d items, max %d: %w", len(content.AssistantHighlights), maxAssistantHighlightCount, domain.ErrInvariantViolation)
	}
	for i, value := range content.AssistantHighlights {
		fields = append(fields, struct {
			name  string
			value string
			limit int
		}{name: fmt.Sprintf("assistant_highlights[%d]", i), value: value, limit: maxAssistantHighlightRunes})
	}
	allowedTruncations := make(map[string]int, len(fields))
	for _, field := range fields {
		if utf8.RuneCountInString(field.value) > field.limit {
			return fmt.Errorf("parse diagnosis conversation summary: %s exceeds %d characters: %w", field.name, field.limit, domain.ErrInvariantViolation)
		}
		allowedTruncations[field.name] = field.limit
	}
	seen := make(map[string]struct{}, len(content.TruncatedFields))
	for _, name := range content.TruncatedFields {
		limit, ok := allowedTruncations[name]
		if !ok {
			return fmt.Errorf("parse diagnosis conversation summary: unsupported truncated field %q: %w", name, domain.ErrInvariantViolation)
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("parse diagnosis conversation summary: duplicate truncated field %q: %w", name, domain.ErrInvariantViolation)
		}
		seen[name] = struct{}{}
		for _, field := range fields {
			if field.name == name && utf8.RuneCountInString(field.value) != limit {
				return fmt.Errorf("parse diagnosis conversation summary: truncated field %q is not at its %d-character bound: %w", name, limit, domain.ErrInvariantViolation)
			}
		}
	}
	return nil
}

func validateTurns(sessionID domain.ChatSessionID, turns []domain.ChatTurn) error {
	for i, turn := range turns {
		wantSequence := i + 1
		if turn.SessionID != sessionID {
			return fmt.Errorf("compress diagnosis conversation: turn %d belongs to session %d, want %d: %w", i, turn.SessionID, sessionID, domain.ErrInvariantViolation)
		}
		if turn.Sequence != wantSequence {
			return fmt.Errorf("compress diagnosis conversation: turn %d sequence is %d, want %d: %w", i, turn.Sequence, wantSequence, domain.ErrInvariantViolation)
		}
		if !turn.Role.Valid() {
			return fmt.Errorf("compress diagnosis conversation: turn %d has invalid role %q: %w", i, turn.Role, domain.ErrInvariantViolation)
		}
		if strings.TrimSpace(turn.MessageID) == "" || strings.TrimSpace(turn.ActorSubject) == "" || strings.TrimSpace(turn.Content) == "" {
			return fmt.Errorf("compress diagnosis conversation: turn %d has incomplete source identity/content: %w", i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

func summarizeContent(turns []domain.ChatTurn) Content {
	content := Content{
		SchemaVersion:     SchemaVersion,
		CompressionMethod: compressionMethodExtractive,
		SourceTurnCount:   len(turns),
	}
	var openingRequest, latestRequest string
	assistantResponses := make([]string, 0)
	for _, turn := range turns {
		switch turn.Role {
		case domain.ChatRoleUser:
			if openingRequest == "" {
				openingRequest = turn.Content
			}
			latestRequest = turn.Content
		case domain.ChatRoleAssistant:
			assistantResponses = append(assistantResponses, turn.Content)
		}
	}
	content.OpeningRequest, content.TruncatedFields = boundedField(openingRequest, maxOpeningRequestRunes, "opening_request", content.TruncatedFields)
	content.LatestRequest, content.TruncatedFields = boundedField(latestRequest, maxLatestRequestRunes, "latest_request", content.TruncatedFields)
	if len(assistantResponses) == 0 {
		return content
	}
	content.LatestAssistantResponse, content.TruncatedFields = boundedField(
		assistantResponses[len(assistantResponses)-1],
		maxLatestAssistantRunes,
		"latest_assistant_response",
		content.TruncatedFields,
	)
	start := max(0, len(assistantResponses)-1-maxAssistantHighlightCount)
	for i, response := range assistantResponses[start : len(assistantResponses)-1] {
		field := fmt.Sprintf("assistant_highlights[%d]", i)
		highlight, truncated := boundedField(response, maxAssistantHighlightRunes, field, content.TruncatedFields)
		content.TruncatedFields = truncated
		content.AssistantHighlights = append(content.AssistantHighlights, highlight)
	}
	return content
}

func boundedField(value string, maxRunes int, field string, truncated []string) (string, []string) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maxRunes {
		return value, truncated
	}
	runes := []rune(value)
	return string(runes[:maxRunes]), append(truncated, field)
}

func sourceDigest(turns []domain.ChatTurn) (string, error) {
	source := make([]digestTurn, len(turns))
	for i, turn := range turns {
		source[i] = digestTurn{
			Sequence:     turn.Sequence,
			MessageID:    turn.MessageID,
			Role:         turn.Role,
			ActorSubject: turn.ActorSubject,
			Content:      turn.Content,
		}
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return "", fmt.Errorf("compress diagnosis conversation: marshal digest source: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Equivalent reports whether two summaries bind the same immutable source and
// versioned content. Generated/created timestamps do not affect idempotency.
func Equivalent(left, right domain.ChatSessionSummary) bool {
	if left.SessionID != right.SessionID ||
		left.Version != right.Version ||
		left.SchemaVersion != right.SchemaVersion ||
		left.SourceFirstSequence != right.SourceFirstSequence ||
		left.SourceLastSequence != right.SourceLastSequence ||
		left.SourceTurnCount != right.SourceTurnCount ||
		left.SourceDigest != right.SourceDigest {
		return false
	}
	leftContent, leftErr := ParseContent(left.Content)
	rightContent, rightErr := ParseContent(right.Content)
	if leftErr != nil || rightErr != nil ||
		left.SchemaVersion != leftContent.SchemaVersion ||
		right.SchemaVersion != rightContent.SchemaVersion ||
		left.SourceTurnCount != leftContent.SourceTurnCount ||
		right.SourceTurnCount != rightContent.SourceTurnCount {
		return false
	}
	return equivalentContent(leftContent, rightContent)
}

func equivalentContent(left, right Content) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.CompressionMethod == right.CompressionMethod &&
		left.SourceTurnCount == right.SourceTurnCount &&
		left.OpeningRequest == right.OpeningRequest &&
		left.LatestRequest == right.LatestRequest &&
		left.LatestAssistantResponse == right.LatestAssistantResponse &&
		slices.Equal(left.AssistantHighlights, right.AssistantHighlights) &&
		slices.Equal(left.TruncatedFields, right.TruncatedFields)
}
