package temporal

import (
	"testing"

	"go.temporal.io/sdk/workflow"
)

func TestDiagnosisRoomStateCloseActorsSeparateCloseFromConfirmation(t *testing.T) {
	tests := []struct {
		name                string
		version             workflow.Version
		conclusionConfirmed bool
		wantClosedBy        string
		wantConfirmedBy     string
	}{
		{
			name:            "legacy workflow history",
			version:         workflow.DefaultVersion,
			wantConfirmedBy: "operator-1",
		},
		{
			name:         "operator close",
			version:      diagnosisRoomCloseActorSemanticsVersion,
			wantClosedBy: "operator-1",
		},
		{
			name:                "confirmed conclusion",
			version:             diagnosisRoomCloseActorSemanticsVersion,
			conclusionConfirmed: true,
			wantClosedBy:        "operator-1",
			wantConfirmedBy:     "operator-1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := diagnosisRoomState{
				closeActorSubject:   "operator-1",
				conclusionConfirmed: tc.conclusionConfirmed,
			}
			closedBy, confirmedBy := state.closeActors(tc.version)
			if closedBy != tc.wantClosedBy || confirmedBy != tc.wantConfirmedBy {
				t.Fatalf("close actors = closed_by:%q confirmed_by:%q, want %q/%q",
					closedBy, confirmedBy, tc.wantClosedBy, tc.wantConfirmedBy)
			}
		})
	}
}
