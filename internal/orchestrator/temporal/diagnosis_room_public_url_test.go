package temporal

import (
	"net/url"
	"testing"
)

func TestActivitiesDiagnosisRoomPublicURLIsIdempotentForDiagnosisRoomBasePath(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseURL   string
		sessionID string
		want      string
	}{
		{
			name:      "base path prefix",
			baseURL:   "https://console.example.test/ops?tenant=prod#ignored",
			sessionID: "session-1",
			want:      "https://console.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-1&tenant=prod&wecom_auto_login=1&wecom_launch_context=app_conversation",
		},
		{
			name:      "already points at diagnosis room",
			baseURL:   "https://console.example.test/ops/diagnosis-room?tenant=prod",
			sessionID: "session-1",
			want:      "https://console.example.test/ops/diagnosis-room?auth_mode=session&session_id=session-1&tenant=prod&wecom_auto_login=1&wecom_launch_context=app_conversation",
		},
		{
			name:      "blank session",
			baseURL:   "https://console.example.test/ops",
			sessionID: " ",
			want:      "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL, err := url.Parse(tc.baseURL)
			if err != nil {
				t.Fatalf("parse base URL: %v", err)
			}
			activities := NewActivities(nil, WithPublicBaseURL(baseURL))

			if got := activities.diagnosisRoomPublicURL(tc.sessionID); got != tc.want {
				t.Fatalf("diagnosisRoomPublicURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
