package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestAppendDiagnosisRoomLifecycleEventRejectsAmbiguousPayload(t *testing.T) {
	cases := []struct {
		name       string
		payload    json.RawMessage
		wantSubstr string
	}{
		{
			name:       "invalid payload",
			payload:    json.RawMessage(`not-json`),
			wantSubstr: "decode JSON token",
		},
		{
			name:       "duplicate payload key",
			payload:    json.RawMessage(`{"kind":"diagnosis_room.opened","kind":"diagnosis_room.closed"}`),
			wantSubstr: `duplicate object key "kind"`,
		},
		{
			name:       "trailing payload value",
			payload:    json.RawMessage(`{"kind":"diagnosis_room.opened"} {"kind":"diagnosis_room.closed"}`),
			wantSubstr: "trailing JSON values",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			activities := &Activities{}
			_, err := activities.appendDiagnosisRoomLifecycleEvent(context.Background(), diagnosisRoomLifecycleEventInput{
				TaskID:     1,
				Kind:       diagnosisRoomEventOpened,
				DedupeKey:  "diagnosis-room-test",
				Payload:    tc.payload,
				OccurredAt: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("appendDiagnosisRoomLifecycleEvent error = %v, want ErrInvariantViolation", err)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("appendDiagnosisRoomLifecycleEvent error = %v, want substring %q", err, tc.wantSubstr)
			}
		})
	}
}
