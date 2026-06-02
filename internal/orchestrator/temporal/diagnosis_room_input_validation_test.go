package temporal

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
)

func TestValidateDiagnosisRoomWorkflowInputRejectsAmbiguousEvidence(t *testing.T) {
	base := DiagnosisRoomWorkflowInput{
		SessionID:       "session-1",
		DiagnosisTaskID: 1001,
		OwnerSubject:    "owner-1",
		Evidence:        json.RawMessage(`{"alert":"cpu_saturation","severity":"warning"}`),
		Policy:          diagnosisroom.DefaultPolicy(),
	}

	cases := []struct {
		name       string
		evidence   json.RawMessage
		wantSubstr string
	}{
		{
			name:       "invalid evidence",
			evidence:   json.RawMessage(`not-json`),
			wantSubstr: "decode JSON token",
		},
		{
			name:       "duplicate evidence key",
			evidence:   json.RawMessage(`{"alert":"cpu","alert":"memory"}`),
			wantSubstr: `duplicate object key "alert"`,
		},
		{
			name:       "trailing evidence value",
			evidence:   json.RawMessage(`{"alert":"cpu"} {"alert":"memory"}`),
			wantSubstr: "trailing JSON values",
		},
		{
			name:       "non object evidence",
			evidence:   json.RawMessage(`["cpu"]`),
			wantSubstr: "must be a JSON object",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			input.Evidence = tc.evidence

			err := validateDiagnosisRoomWorkflowInput(input, input.Policy)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("validateDiagnosisRoomWorkflowInput error = %v, want substring %q", err, tc.wantSubstr)
			}
		})
	}
}
