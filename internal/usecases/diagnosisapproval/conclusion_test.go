package diagnosisapproval

import (
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestConclusionDigestIsStableAndContentBound(t *testing.T) {
	t.Parallel()
	first, err := ConclusionDigest("message-1/assistant", 2, "Database saturation is causal.")
	if err != nil {
		t.Fatalf("ConclusionDigest: %v", err)
	}
	again, err := ConclusionDigest("message-1/assistant", 2, "Database saturation is causal.")
	if err != nil {
		t.Fatalf("ConclusionDigest again: %v", err)
	}
	changed, err := ConclusionDigest("message-1/assistant", 2, "Deployment overlap is causal.")
	if err != nil {
		t.Fatalf("ConclusionDigest changed: %v", err)
	}
	if len(first) != 64 || first != again || first == changed {
		t.Fatalf("digests first=%q again=%q changed=%q", first, again, changed)
	}
}

func TestValidateConclusionDigest(t *testing.T) {
	t.Parallel()
	valid := strings.Repeat("a", 64)
	if err := ValidateConclusionDigest(valid); err != nil {
		t.Fatalf("ValidateConclusionDigest valid: %v", err)
	}
	for _, value := range []string{
		strings.Repeat("A", 64),
		" " + valid,
		strings.Repeat("a", 63),
		strings.Repeat("z", 64),
	} {
		if err := ValidateConclusionDigest(value); !errors.Is(err, domain.ErrInvariantViolation) {
			t.Fatalf("ValidateConclusionDigest(%q) error = %v", value, err)
		}
	}
}

func TestLatestPersistedConclusionBindsLatestAssistantTurn(t *testing.T) {
	t.Parallel()
	session := domain.ChatSession{ID: 7, TurnCount: 2}
	turns := []domain.ChatTurn{
		{SessionID: 7, MessageID: "msg-1", Sequence: 1, Role: domain.ChatRoleUser, Content: "Investigate."},
		{SessionID: 7, MessageID: "msg-1/assistant", Sequence: 2, Role: domain.ChatRoleAssistant, Content: "Initial conclusion."},
		{SessionID: 7, MessageID: "msg-2", Sequence: 3, Role: domain.ChatRoleUser, Content: "Use the new evidence."},
		{SessionID: 7, MessageID: "msg-2/assistant", Sequence: 4, Role: domain.ChatRoleAssistant, Content: "Revised conclusion."},
	}

	turn, digest, err := LatestPersistedConclusion(session, turns)
	if err != nil {
		t.Fatalf("LatestPersistedConclusion: %v", err)
	}
	wantDigest, err := ConclusionDigest("msg-2/assistant", 4, "Revised conclusion.")
	if err != nil {
		t.Fatalf("ConclusionDigest: %v", err)
	}
	if turn.MessageID != "msg-2/assistant" || digest != wantDigest {
		t.Fatalf("latest turn=%+v digest=%q, want digest=%q", turn, digest, wantDigest)
	}

	invalid := append([]domain.ChatTurn(nil), turns[:3]...)
	if _, _, err := LatestPersistedConclusion(session, invalid); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("incomplete transcript error = %v", err)
	}
	invalid = append([]domain.ChatTurn(nil), turns...)
	invalid[1].Role = domain.ChatRoleUser
	if _, _, err := LatestPersistedConclusion(session, invalid); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("non-canonical transcript error = %v", err)
	}
	if _, _, err := LatestPersistedConclusion(
		domain.ChatSession{ID: 7, TurnCount: int(^uint(0) >> 1)},
		turns,
	); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("overflowing turn count error = %v", err)
	}
}

func TestPendingAuthorities(t *testing.T) {
	t.Parallel()
	owner := domain.ChatSessionApproval{ActorSubject: "owner-1", Authority: domain.DiagnosisApprovalAuthorityOwner}
	leader := domain.ChatSessionApproval{ActorSubject: "leader-1", Authority: domain.DiagnosisApprovalAuthorityLeader}
	pending, err := PendingAuthorities(domain.DiagnosisApprovalModeOwnerAndLeader, []domain.ChatSessionApproval{owner})
	if err != nil {
		t.Fatalf("PendingAuthorities owner: %v", err)
	}
	if len(pending) != 1 || pending[0] != domain.DiagnosisApprovalAuthorityLeader {
		t.Fatalf("pending = %v, want leader", pending)
	}
	pending, err = PendingAuthorities(domain.DiagnosisApprovalModeOwnerAndLeader, []domain.ChatSessionApproval{owner, leader})
	if err != nil {
		t.Fatalf("PendingAuthorities complete: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want none", pending)
	}
	_, err = PendingAuthorities(domain.DiagnosisApprovalModeOwnerAndLeader, []domain.ChatSessionApproval{
		owner,
		{ActorSubject: "owner-1", Authority: domain.DiagnosisApprovalAuthorityLeader},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("same subject err = %v, want ErrInvariantViolation", err)
	}
	_, err = PendingAuthorities(domain.DiagnosisApprovalModeOwnerAndLeader, []domain.ChatSessionApproval{
		leader,
		{ActorSubject: "leader-2", Authority: domain.DiagnosisApprovalAuthorityLeader},
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("duplicate authority err = %v, want ErrInvariantViolation", err)
	}
	_, err = PendingAuthorities(domain.DiagnosisApprovalModeSingle, []domain.ChatSessionApproval{{
		ActorSubject: "owner-1",
		Authority:    "invalid",
	}})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("invalid single approval err = %v, want ErrInvariantViolation", err)
	}
}
