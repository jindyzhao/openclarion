package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewChatSession(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 5, 28, 10, 0, 0, 1234, time.FixedZone("HKT", 8*60*60))

	t.Run("happy path defaults open and normalises time", func(t *testing.T) {
		t.Parallel()
		got, err := NewChatSession(11, " session-1 ", " owner-1 ", startedAt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.SessionKey != "session-1" || got.OwnerSubject != "owner-1" {
			t.Fatalf("trimmed identity = (%q,%q), want (session-1,owner-1)", got.SessionKey, got.OwnerSubject)
		}
		if got.Status != ChatSessionStatusOpen || got.TurnCount != 0 {
			t.Fatalf("status/turn_count = (%q,%d), want (open,0)", got.Status, got.TurnCount)
		}
		want := NormalizeUTCMicro(startedAt)
		if !got.StartedAt.Equal(want) || !got.LastActivityAt.Equal(want) {
			t.Fatalf("times = (%s,%s), want %s", got.StartedAt, got.LastActivityAt, want)
		}
	})

	cases := []struct {
		name         string
		taskID       DiagnosisTaskID
		sessionKey   string
		ownerSubject string
		startedAt    time.Time
	}{
		{name: "zero task id", sessionKey: "s", ownerSubject: "o", startedAt: startedAt},
		{name: "empty session key", taskID: 1, ownerSubject: "o", startedAt: startedAt},
		{name: "empty owner", taskID: 1, sessionKey: "s", startedAt: startedAt},
		{name: "zero started_at", taskID: 1, sessionKey: "s", ownerSubject: "o"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewChatSession(tc.taskID, tc.sessionKey, tc.ownerSubject, tc.startedAt)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestChatSession_RecordTurnAndClose(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	session, err := NewChatSession(1, "session-1", "owner-1", startedAt)
	if err != nil {
		t.Fatalf("NewChatSession: %v", err)
	}

	t.Run("record turn increments count", func(t *testing.T) {
		t.Parallel()
		got, err := session.RecordTurn(startedAt.Add(time.Minute))
		if err != nil {
			t.Fatalf("RecordTurn: %v", err)
		}
		if got.TurnCount != 1 || !got.LastActivityAt.Equal(startedAt.Add(time.Minute)) {
			t.Fatalf("post-turn state = %+v", got)
		}
	})

	t.Run("close is terminal and idempotent for same metadata", func(t *testing.T) {
		t.Parallel()
		closedAt := startedAt.Add(10 * time.Minute)
		closed, err := session.Close(closedAt, "user_requested")
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
		again, err := closed.Close(closedAt, "user_requested")
		if err != nil {
			t.Fatalf("idempotent Close: %v", err)
		}
		if again.Status != ChatSessionStatusClosed || again.ClosedAt == nil || again.CloseReason != "user_requested" {
			t.Fatalf("closed state = %+v", again)
		}
	})

	t.Run("record after close is rejected", func(t *testing.T) {
		t.Parallel()
		closed, err := session.Close(startedAt.Add(time.Minute), "limit_reached")
		if err != nil {
			t.Fatalf("Close setup: %v", err)
		}
		_, err = closed.RecordTurn(startedAt.Add(2 * time.Minute))
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("err = %v, want ErrInvariantViolation", err)
		}
	})

	t.Run("invalid close metadata rejected", func(t *testing.T) {
		t.Parallel()
		_, err := session.Close(startedAt.Add(-time.Second), "bad")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("close before start err = %v, want ErrInvariantViolation", err)
		}
		_, err = session.Close(startedAt.Add(time.Second), " ")
		if !errors.Is(err, ErrInvariantViolation) {
			t.Fatalf("empty reason err = %v, want ErrInvariantViolation", err)
		}
	})
}

func TestNewChatTurn(t *testing.T) {
	t.Parallel()

	occurredAt := time.Date(2026, 5, 28, 11, 0, 0, 999, time.UTC)

	t.Run("happy path defaults metadata object", func(t *testing.T) {
		t.Parallel()
		got, err := NewChatTurn(ChatTurn{
			SessionID:    1,
			MessageID:    " msg-1 ",
			Sequence:     1,
			Role:         ChatRoleUser,
			ActorSubject: " owner-1 ",
			Content:      " hello ",
			OccurredAt:   occurredAt,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.MessageID != "msg-1" || got.ActorSubject != "owner-1" || got.Content != "hello" {
			t.Fatalf("trimmed values = %+v", got)
		}
		if string(got.Metadata) != "{}" {
			t.Fatalf("metadata = %s, want {}", got.Metadata)
		}
	})

	t.Run("accepts valid metadata", func(t *testing.T) {
		t.Parallel()
		got, err := NewChatTurn(ChatTurn{
			SessionID:    1,
			MessageID:    "assistant-msg-1",
			Sequence:     2,
			Role:         ChatRoleAssistant,
			ActorSubject: "diagnosis-assistant",
			Content:      "summary",
			Metadata:     json.RawMessage(`{"model":"local"}`),
			OccurredAt:   occurredAt,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got.Metadata) != `{"model":"local"}` {
			t.Fatalf("metadata = %s", got.Metadata)
		}
	})

	cases := []struct {
		name string
		turn ChatTurn
	}{
		{name: "zero session", turn: ChatTurn{MessageID: "m", Sequence: 1, Role: ChatRoleUser, ActorSubject: "o", Content: "c", OccurredAt: occurredAt}},
		{name: "empty message", turn: ChatTurn{SessionID: 1, Sequence: 1, Role: ChatRoleUser, ActorSubject: "o", Content: "c", OccurredAt: occurredAt}},
		{name: "zero sequence", turn: ChatTurn{SessionID: 1, MessageID: "m", Role: ChatRoleUser, ActorSubject: "o", Content: "c", OccurredAt: occurredAt}},
		{name: "invalid role", turn: ChatTurn{SessionID: 1, MessageID: "m", Sequence: 1, Role: "owner", ActorSubject: "o", Content: "c", OccurredAt: occurredAt}},
		{name: "empty actor", turn: ChatTurn{SessionID: 1, MessageID: "m", Sequence: 1, Role: ChatRoleUser, Content: "c", OccurredAt: occurredAt}},
		{name: "empty content", turn: ChatTurn{SessionID: 1, MessageID: "m", Sequence: 1, Role: ChatRoleUser, ActorSubject: "o", OccurredAt: occurredAt}},
		{name: "zero occurred_at", turn: ChatTurn{SessionID: 1, MessageID: "m", Sequence: 1, Role: ChatRoleUser, ActorSubject: "o", Content: "c"}},
		{name: "invalid metadata", turn: ChatTurn{SessionID: 1, MessageID: "m", Sequence: 1, Role: ChatRoleUser, ActorSubject: "o", Content: "c", Metadata: json.RawMessage(`{`), OccurredAt: occurredAt}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewChatTurn(tc.turn)
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}
