// Package diagnosisapproval owns deterministic conclusion identity and quorum
// helpers for diagnosis-room human approvals.
package diagnosisapproval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
)

type conclusionSource struct {
	AssistantMessageID string `json:"assistant_message_id"`
	AssistantSequence  int    `json:"assistant_sequence"`
	Content            string `json:"content"`
}

// ConclusionDigest returns the stable SHA-256 identity shared by workflow
// approval state and persisted ChatSessionApproval rows.
func ConclusionDigest(assistantMessageID string, assistantSequence int, content string) (string, error) {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	content = strings.TrimSpace(content)
	if assistantMessageID == "" {
		return "", fmt.Errorf("diagnosis approval: assistant_message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if assistantSequence <= 0 {
		return "", fmt.Errorf("diagnosis approval: assistant_sequence must be positive: %w", domain.ErrInvariantViolation)
	}
	if content == "" {
		return "", fmt.Errorf("diagnosis approval: content must be non-empty: %w", domain.ErrInvariantViolation)
	}
	raw, err := json.Marshal(conclusionSource{
		AssistantMessageID: assistantMessageID,
		AssistantSequence:  assistantSequence,
		Content:            content,
	})
	if err != nil {
		return "", fmt.Errorf("diagnosis approval: marshal conclusion identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateConclusionDigest rejects ambiguous or malformed persisted conclusion
// identities before they are used as approval lookup keys.
func ValidateConclusionDigest(value string) error {
	if value != strings.TrimSpace(value) || len(value) != sha256.Size*2 {
		return fmt.Errorf("diagnosis approval: conclusion_digest must be a lowercase SHA-256 hex digest: %w", domain.ErrInvariantViolation)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(value) != value {
		return fmt.Errorf("diagnosis approval: conclusion_digest must be a lowercase SHA-256 hex digest: %w", domain.ErrInvariantViolation)
	}
	return nil
}

// LatestPersistedConclusion verifies the canonical user/assistant transcript
// shape recorded by ChatSession.turn_count and returns the latest assistant
// turn with its deterministic conclusion digest.
func LatestPersistedConclusion(session domain.ChatSession, turns []domain.ChatTurn) (domain.ChatTurn, string, error) {
	if session.ID <= 0 || session.TurnCount <= 0 {
		return domain.ChatTurn{}, "", fmt.Errorf("diagnosis approval: chat session identity and turn_count must be positive: %w", domain.ErrInvariantViolation)
	}
	if len(turns)%2 != 0 || len(turns)/2 != session.TurnCount {
		return domain.ChatTurn{}, "", fmt.Errorf(
			"diagnosis approval: persisted transcript has %d messages, want two per completed turn: %w",
			len(turns),
			domain.ErrInvariantViolation,
		)
	}
	for index, turn := range turns {
		wantSequence := index + 1
		wantRole := domain.ChatRoleUser
		if wantSequence%2 == 0 {
			wantRole = domain.ChatRoleAssistant
		}
		if turn.SessionID != session.ID || turn.Sequence != wantSequence || turn.Role != wantRole {
			return domain.ChatTurn{}, "", fmt.Errorf(
				"diagnosis approval: persisted message %d is outside the canonical transcript: %w",
				wantSequence,
				domain.ErrInvariantViolation,
			)
		}
	}
	turn := turns[len(turns)-1]
	digest, err := ConclusionDigest(turn.MessageID, turn.Sequence, turn.Content)
	if err != nil {
		return domain.ChatTurn{}, "", err
	}
	return turn, digest, nil
}

// PendingAuthorities returns the quorum entries still required for one mode.
// Approvals for a different conclusion digest must be filtered by the caller.
func PendingAuthorities(mode domain.DiagnosisApprovalMode, approvals []domain.ChatSessionApproval) ([]domain.DiagnosisApprovalAuthority, error) {
	if !mode.Valid() {
		return nil, fmt.Errorf("diagnosis approval: mode %q is unsupported: %w", mode, domain.ErrInvariantViolation)
	}
	seen := map[domain.DiagnosisApprovalAuthority]struct{}{}
	subjects := map[string]struct{}{}
	for _, approval := range approvals {
		if !approval.Authority.Valid() || strings.TrimSpace(approval.ActorSubject) == "" {
			return nil, fmt.Errorf("diagnosis approval: invalid approval state: %w", domain.ErrInvariantViolation)
		}
		if _, duplicate := subjects[approval.ActorSubject]; duplicate {
			return nil, fmt.Errorf("diagnosis approval: one subject cannot satisfy multiple authorities: %w", domain.ErrInvariantViolation)
		}
		if _, duplicate := seen[approval.Authority]; duplicate {
			return nil, fmt.Errorf("diagnosis approval: one authority cannot be satisfied by multiple subjects: %w", domain.ErrInvariantViolation)
		}
		subjects[approval.ActorSubject] = struct{}{}
		seen[approval.Authority] = struct{}{}
	}
	if mode == domain.DiagnosisApprovalModeSingle {
		if len(approvals) > 1 {
			return nil, fmt.Errorf("diagnosis approval: single mode accepts exactly one approval: %w", domain.ErrInvariantViolation)
		}
		if len(approvals) > 0 {
			return []domain.DiagnosisApprovalAuthority{}, nil
		}
		return []domain.DiagnosisApprovalAuthority{domain.DiagnosisApprovalAuthorityOwner}, nil
	}
	pending := make([]domain.DiagnosisApprovalAuthority, 0, 2)
	for _, authority := range []domain.DiagnosisApprovalAuthority{
		domain.DiagnosisApprovalAuthorityOwner,
		domain.DiagnosisApprovalAuthorityLeader,
	} {
		if _, ok := seen[authority]; !ok {
			pending = append(pending, authority)
		}
	}
	return pending, nil
}
