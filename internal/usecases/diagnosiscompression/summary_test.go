package diagnosiscompression

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestSummarize(t *testing.T) {
	t.Parallel()
	generatedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	turns := []domain.ChatTurn{
		turn(1, domain.ChatRoleUser, "owner", "Why is checkout latency high?"),
		turn(2, domain.ChatRoleAssistant, "assistant", "The first sample points to database saturation."),
		turn(3, domain.ChatRoleUser, "owner", "Check the newest metrics."),
		turn(4, domain.ChatRoleAssistant, "assistant", "The newest metrics confirm connection pool exhaustion."),
	}

	got, err := Summarize(9, turns, generatedAt)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.SourceFirstSequence != 1 || got.SourceLastSequence != 4 || got.SourceTurnCount != 4 || len(got.SourceDigest) != 64 {
		t.Fatalf("summary source = %+v", got)
	}
	content, err := ParseContent(got.Content)
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if content.OpeningRequest != "Why is checkout latency high?" ||
		content.LatestRequest != "Check the newest metrics." ||
		content.LatestAssistantResponse != "The newest metrics confirm connection pool exhaustion." ||
		len(content.AssistantHighlights) != 1 {
		t.Fatalf("content = %+v", content)
	}

	again, err := Summarize(9, turns, generatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("Summarize again: %v", err)
	}
	if !Equivalent(got, again) {
		t.Fatalf("summaries should be equivalent:\nfirst=%+v\nagain=%+v", got, again)
	}
}

func TestSummarizeEmptyAndBounds(t *testing.T) {
	t.Parallel()
	empty, err := Summarize(4, nil, time.Now())
	if err != nil {
		t.Fatalf("Summarize empty: %v", err)
	}
	if empty.SourceTurnCount != 0 || empty.SourceFirstSequence != 0 || empty.SourceLastSequence != 0 {
		t.Fatalf("empty source = %+v", empty)
	}

	long := strings.Repeat("x", maxLatestAssistantRunes+10)
	bounded, err := Summarize(9, []domain.ChatTurn{
		turn(1, domain.ChatRoleUser, "owner", "question"),
		turn(2, domain.ChatRoleAssistant, "assistant", long),
	}, time.Now())
	if err != nil {
		t.Fatalf("Summarize bounded: %v", err)
	}
	content, err := ParseContent(bounded.Content)
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if len([]rune(content.LatestAssistantResponse)) != maxLatestAssistantRunes ||
		!slices.Equal(content.TruncatedFields, []string{"latest_assistant_response"}) {
		t.Fatalf("bounded content = %+v", content)
	}
}

func TestSummarizeTracksOnlyFinalFieldTruncation(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", maxLatestRequestRunes+1)
	summary, err := Summarize(9, []domain.ChatTurn{
		turn(1, domain.ChatRoleUser, "owner", long),
		turn(2, domain.ChatRoleAssistant, "assistant", "first answer"),
		turn(3, domain.ChatRoleUser, "owner", "short follow-up"),
		turn(4, domain.ChatRoleAssistant, "assistant", "latest answer"),
	}, time.Now())
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	content, err := ParseContent(summary.Content)
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if content.LatestRequest != "short follow-up" ||
		!slices.Equal(content.TruncatedFields, []string{"opening_request"}) {
		t.Fatalf("content = %+v", content)
	}
}

func TestParseContentRejectsContractViolations(t *testing.T) {
	t.Parallel()
	base := Content{
		SchemaVersion:     SchemaVersion,
		CompressionMethod: compressionMethodExtractive,
		SourceTurnCount:   1,
	}
	tests := []struct {
		name   string
		mutate func(*Content)
	}{
		{name: "too many turns", mutate: func(in *Content) { in.SourceTurnCount = MaxSourceTurns + 1 }},
		{name: "oversized request", mutate: func(in *Content) { in.LatestRequest = strings.Repeat("x", maxLatestRequestRunes+1) }},
		{name: "too many highlights", mutate: func(in *Content) { in.AssistantHighlights = []string{"a", "b", "c", "d"} }},
		{name: "unknown truncation", mutate: func(in *Content) { in.TruncatedFields = []string{"unknown"} }},
		{name: "duplicate truncation", mutate: func(in *Content) {
			in.LatestRequest = strings.Repeat("x", maxLatestRequestRunes)
			in.TruncatedFields = []string{"latest_request", "latest_request"}
		}},
		{name: "short truncated field", mutate: func(in *Content) {
			in.LatestRequest = "short"
			in.TruncatedFields = []string{"latest_request"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content := base
			tc.mutate(&content)
			raw, err := json.Marshal(content)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if _, err := ParseContent(raw); !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("want ErrInvariantViolation, got %v", err)
			}
		})
	}
}

func TestSummarizeRejectsInvalidSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		turns []domain.ChatTurn
	}{
		{name: "wrong session", turns: []domain.ChatTurn{turnForSession(2, 1, domain.ChatRoleUser, "owner", "question")}},
		{name: "sequence gap", turns: []domain.ChatTurn{turn(2, domain.ChatRoleUser, "owner", "question")}},
		{name: "invalid role", turns: []domain.ChatTurn{turn(1, domain.ChatRole("bad"), "owner", "question")}},
		{name: "empty content", turns: []domain.ChatTurn{turn(1, domain.ChatRoleUser, "owner", " ")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Summarize(9, tc.turns, time.Now())
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("want ErrInvariantViolation, got %v", err)
			}
		})
	}
}

func turn(sequence int, role domain.ChatRole, actor, content string) domain.ChatTurn {
	return turnForSession(9, sequence, role, actor, content)
}

func turnForSession(sessionID domain.ChatSessionID, sequence int, role domain.ChatRole, actor, content string) domain.ChatTurn {
	return domain.ChatTurn{
		SessionID:    sessionID,
		MessageID:    fmt.Sprintf("message-%d", sequence),
		Sequence:     sequence,
		Role:         role,
		ActorSubject: actor,
		Content:      content,
	}
}
