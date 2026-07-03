package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

func TestRunWritesAutoDiagnosisProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 3, 30, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	var gotPostPath string
	var gotPolicyPath string
	var gotRoomPath string
	var gotAuth string
	var gotPolicyAuth string
	var payload webhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alert-sources/42/webhooks/alertmanager":
			gotPostPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			if r.Method != http.MethodPost {
				t.Fatalf("webhook method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode webhook payload: %v", err)
			}
			writeJSON(t, w, http.StatusAccepted, alertmanagerAutoDiagnosisResponse())
		case "/api/v1/config/report-workflow-policies/9":
			gotPolicyPath = r.URL.Path
			gotPolicyAuth = r.Header.Get("Authorization")
			if r.Method != http.MethodGet {
				t.Fatalf("policy method = %s, want GET", r.Method)
			}
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyWithNotificationChannel())
		case "/api/v1/diagnosis/rooms/diagnosis-session-auto-1":
			gotRoomPath = r.URL.Path
			if r.Method != http.MethodGet {
				t.Fatalf("room method = %s, want GET", r.Method)
			}
			writeJSON(t, w, http.StatusOK, diagnosisRoomSummaryWithAssistantProof(now))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("OPENCLARION_TEST_ALERTMANAGER_WEBHOOK_BEARER_TOKEN", "Bearer test-token")
	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--webhook-bearer-token-env", "OPENCLARION_TEST_ALERTMANAGER_WEBHOOK_BEARER_TOKEN",
		"--output", output,
		"--http-timeout", "5s",
		"--room-timeout", "100ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotPostPath != "/api/v1/alert-sources/42/webhooks/alertmanager" {
		t.Fatalf("webhook path = %q", gotPostPath)
	}
	if gotPolicyPath != "/api/v1/config/report-workflow-policies/9" {
		t.Fatalf("policy path = %q", gotPolicyPath)
	}
	if gotRoomPath != "/api/v1/diagnosis/rooms/diagnosis-session-auto-1" {
		t.Fatalf("room path = %q", gotRoomPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization header = %q", gotAuth)
	}
	if gotPolicyAuth != "" {
		t.Fatalf("policy authorization header = %q, want empty", gotPolicyAuth)
	}
	if payload.Version != "4" ||
		payload.Status != "firing" ||
		payload.GroupLabels["alertname"] != "OpenClarionAutoDiagnosisSmoke" ||
		len(payload.Alerts) != int(expectedSyntheticAlertsReceived) ||
		payload.Alerts[0].Labels["smoke_id"] == "" {
		t.Fatalf("unexpected webhook payload: %+v", payload)
	}
	if payload.Alerts[0].Status != "firing" ||
		payload.Alerts[0].Labels["instance"] != "auto-diagnosis-smoke-active" ||
		len(payload.Alerts[0].SilencedBy) != 0 ||
		len(payload.Alerts[0].InhibitedBy) != 0 {
		t.Fatalf("active alert = %+v, want unsuppressed firing alert", payload.Alerts[0])
	}
	if payload.Alerts[1].Status != "resolved" ||
		payload.Alerts[1].EndsAt == nil ||
		payload.Alerts[1].Fingerprint == payload.Alerts[0].Fingerprint {
		t.Fatalf("resolved alert = %+v, want resolved alert with unique fingerprint and endsAt", payload.Alerts[1])
	}
	if payload.Alerts[2].Status != "firing" ||
		len(payload.Alerts[2].SilencedBy) != 1 ||
		len(payload.Alerts[2].InhibitedBy) != 0 {
		t.Fatalf("silenced alert = %+v, want only silencedBy", payload.Alerts[2])
	}
	if payload.Alerts[3].Status != "firing" ||
		len(payload.Alerts[3].SilencedBy) != 0 ||
		len(payload.Alerts[3].InhibitedBy) != 1 {
		t.Fatalf("inhibited alert = %+v, want only inhibitedBy", payload.Alerts[3])
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Passed != true ||
		out.CheckedAt != "2026-06-21T03:30:00Z" ||
		out.Webhook.Received != expectedSyntheticAlertsReceived ||
		out.Webhook.Skipped.Resolved != expectedSyntheticResolvedSkipped ||
		out.Webhook.Skipped.Suppressed != expectedSyntheticSuppressedSkipped ||
		out.Webhook.Ingested.Total != expectedSyntheticIngestedTotal ||
		out.Webhook.AutoDiagnosis.RoomsStarted != 1 ||
		out.Room.SessionID != "diagnosis-session-auto-1" ||
		out.Room.TurnCount != 1 ||
		!out.Room.AINotificationDelivered ||
		out.Room.LatestConfidence != "medium" ||
		out.Room.LatestRequiresHumanReview == nil ||
		!*out.Room.LatestRequiresHumanReview ||
		out.Room.LatestEvidenceRequestCount != 1 ||
		!out.Room.LatestConfidenceRationalePresent ||
		!out.Room.LatestConfidenceImprovementPathPresent ||
		len(out.Room.NotificationContentProofs) != 2 ||
		out.Request.ExpectedNotificationChannelProfileID != 5 ||
		out.Request.ExpectedNotificationChannelSource != expectedNotificationChannelSourcePolicy ||
		out.Request.ExpectedContentKind != "assistant_message" ||
		strings.Join(out.Request.RequiredContentKinds, ",") != "assistant_message" ||
		out.Room.NotificationContentProofs[0].NotificationChannelProfileID != 5 ||
		out.Room.NotificationContentProofs[0].EventKind != eventAssistantTurnNotification ||
		out.Room.NotificationContentProofs[0].ContentKind != "assistant_message" ||
		out.Room.NotificationContentProofs[1].NotificationChannelProfileID != 5 ||
		out.Room.NotificationContentProofs[1].EventKind != eventFinalReadyNotification ||
		out.Room.NotificationContentProofs[1].ContentKind != "final_conclusion" ||
		out.Evidence != proofEvidence {
		t.Fatalf("proof = %+v", out)
	}
	if strings.Contains(string(raw), "test-token") {
		t.Fatalf("proof leaked bearer token: %s", raw)
	}
}

func TestRunRejectsFinalOnlyFirstTurnProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 3, 32, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alert-sources/42/webhooks/alertmanager":
			writeJSON(t, w, http.StatusAccepted, alertmanagerAutoDiagnosisResponse())
		case "/api/v1/config/report-workflow-policies/9":
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyWithNotificationChannel())
		case "/api/v1/diagnosis/rooms/diagnosis-session-auto-1":
			writeJSON(t, w, http.StatusOK, diagnosisRoomSummary(now))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
		"--http-timeout", "5s",
		"--room-timeout", "20ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want default first-turn assistant content proof failure")
	}
	if !strings.Contains(err.Error(), "timed out waiting for AI notification delivery") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunWritesAutoDiagnosisProofWithRequiredContentKinds(t *testing.T) {
	now := time.Date(2026, 6, 21, 3, 35, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alert-sources/42/webhooks/alertmanager":
			writeJSON(t, w, http.StatusAccepted, alertmanagerAutoDiagnosisResponse())
		case "/api/v1/config/report-workflow-policies/9":
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyWithNotificationChannel())
		case "/api/v1/diagnosis/rooms/diagnosis-session-auto-1":
			writeJSON(t, w, http.StatusOK, diagnosisRoomSummaryWithAssistantProof(now))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--required-content-kinds", " assistant_message,Final_Conclusion ",
		"--output", output,
		"--http-timeout", "5s",
		"--room-timeout", "100ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if got, want := strings.Join(out.Request.RequiredContentKinds, ","), "assistant_message,final_conclusion"; got != want {
		t.Fatalf("required content kinds = %q, want %q", got, want)
	}
	if len(out.Room.NotificationContentProofs) != 2 {
		t.Fatalf("notification content proofs = %+v, want assistant and final proofs", out.Room.NotificationContentProofs)
	}
}

func TestRunRejectsNotificationChannelMismatchWithPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alert-sources/42/webhooks/alertmanager":
			writeJSON(t, w, http.StatusAccepted, alertmanagerAutoDiagnosisResponse())
		case "/api/v1/config/report-workflow-policies/9":
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyWithNotificationChannel())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--expected-notification-channel-profile-id", "6",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
		"--room-timeout", "20ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want notification channel mismatch rejection")
	}
	if !strings.Contains(err.Error(), "does not match policy") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsPolicyWithoutNotificationChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/alert-sources/42/webhooks/alertmanager":
			writeJSON(t, w, http.StatusAccepted, alertmanagerAutoDiagnosisResponse())
		case "/api/v1/config/report-workflow-policies/9":
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyWithoutNotificationChannel(9))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
		"--room-timeout", "20ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want missing notification channel binding rejection")
	}
	if !strings.Contains(err.Error(), "has no report_notification_channel_profile_id") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsMissingAutoDiagnosis(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alert-sources/42/webhooks/alertmanager" {
			http.NotFound(w, r)
			return
		}
		writeJSON(t, w, http.StatusAccepted, api.AlertmanagerWebhookIngestResponse{
			SourceID:          42,
			Received:          expectedSyntheticAlertsReceived,
			SkippedResolved:   expectedSyntheticResolvedSkipped,
			SkippedSuppressed: expectedSyntheticSuppressedSkipped,
			Ingested: api.AlertmanagerWebhookIngestResponseIngested{
				Total: expectedSyntheticIngestedTotal,
				Saved: 1,
			},
		})
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--source-profile-id", "42",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
		"--room-timeout", "20ms",
		"--poll-interval", "1ms",
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want missing auto_diagnosis rejection")
	}
	if !strings.Contains(err.Error(), "missing auto_diagnosis") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateWebhookResultRejectsWeakShapes(t *testing.T) {
	cfg := config{sourceProfileID: 42}
	valid := api.AlertmanagerWebhookIngestResponse{
		SourceID:          42,
		Received:          expectedSyntheticAlertsReceived,
		SkippedResolved:   expectedSyntheticResolvedSkipped,
		SkippedSuppressed: expectedSyntheticSuppressedSkipped,
		Ingested: api.AlertmanagerWebhookIngestResponseIngested{
			Total: expectedSyntheticIngestedTotal,
			Saved: 1,
		},
		AutoDiagnosis: &api.AlertmanagerWebhookAutoDiagnosisSummary{
			PoliciesMatched: 1,
			Snapshots:       1,
			RoomsStarted:    1,
			Rooms: []api.AlertmanagerWebhookAutoDiagnosisRoom{
				{
					PolicyID:           9,
					EvidenceSnapshotID: 11,
					SessionID:          "diagnosis-session-auto-1",
					WorkflowID:         "diagnosis-room-diagnosis-session-auto-1",
					RunID:              "run-1",
				},
			},
		},
	}
	if err := validateWebhookResult(cfg, valid); err != nil {
		t.Fatalf("valid webhook result rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*api.AlertmanagerWebhookIngestResponse)
		want string
	}{
		{
			name: "source mismatch",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.SourceID = 7
			},
			want: "source_id=7",
		},
		{
			name: "wrong received count",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.Received = 0
			},
			want: "received=0",
		},
		{
			name: "wrong resolved skipped count",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.SkippedResolved = 0
			},
			want: "skipped_resolved=0",
		},
		{
			name: "wrong suppressed skipped count",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.SkippedSuppressed = 1
			},
			want: "skipped_suppressed=1",
		},
		{
			name: "wrong ingest count",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.Ingested.Total = 0
			},
			want: "ingested.total=0",
		},
		{
			name: "failed ingest",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.Ingested.Failed = 1
			},
			want: "ingested.failed=1",
		},
		{
			name: "no started rooms",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.AutoDiagnosis.RoomsStarted = 0
			},
			want: "did not start a diagnosis room",
		},
		{
			name: "missing room session",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.AutoDiagnosis.Rooms[0].SessionID = " "
			},
			want: "session_id is empty",
		},
		{
			name: "missing room snapshot",
			edit: func(result *api.AlertmanagerWebhookIngestResponse) {
				result.AutoDiagnosis.Rooms[0].EvidenceSnapshotID = 0
			},
			want: "evidence_snapshot_id must be positive",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := valid
			autoDiagnosis := *valid.AutoDiagnosis
			autoDiagnosis.Rooms = append([]api.AlertmanagerWebhookAutoDiagnosisRoom(nil), valid.AutoDiagnosis.Rooms...)
			result.AutoDiagnosis = &autoDiagnosis
			tc.edit(&result)

			err := validateWebhookResult(cfg, result)
			if err == nil {
				t.Fatal("validateWebhookResult succeeded; want rejection")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestRunRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "userinfo_base_url",
			args: []string{"--api-base-url", "https://user@example.test", "--source-profile-id", "1", "--output", "proof.json"},
		},
		{
			name: "query_base_url",
			args: []string{"--api-base-url", "https://example.test?token=value", "--source-profile-id", "1", "--output", "proof.json"},
		},
		{
			name: "bad_source_profile",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "0", "--output", "proof.json"},
		},
		{
			name: "token_whitespace",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--webhook-bearer-token", "bad token"},
		},
		{
			name: "bad_token_env_name",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--webhook-bearer-token-env", "OPENCLARION-TOKEN"},
		},
		{
			name: "missing_token_env",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--webhook-bearer-token-env", "OPENCLARION_MISSING_ALERTMANAGER_WEBHOOK_TOKEN"},
		},
		{
			name: "conflicting_token_sources",
			args: []string{
				"--api-base-url", "https://example.test",
				"--source-profile-id", "1",
				"--output", "proof.json",
				"--webhook-bearer-token", "direct-token",
				"--webhook-bearer-token-env", "PATH",
			},
		},
		{
			name: "bad_alert_name",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--alert-name", "Bad Alert"},
		},
		{
			name: "bad_expected_channel",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--expected-notification-channel-profile-id", "-1"},
		},
		{
			name: "zero_expected_channel",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--expected-notification-channel-profile-id", "0"},
		},
		{
			name: "bad_expected_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--expected-content-kind", "raw_alert"},
		},
		{
			name: "bad_required_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--required-content-kinds", "raw_alert"},
		},
		{
			name: "duplicate_required_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--required-content-kinds", "assistant_message,assistant_message"},
		},
		{
			name: "empty_required_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--source-profile-id", "1", "--output", "proof.json", "--required-content-kinds", "assistant_message,,final_conclusion"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(tc.args, unusedHTTPClient(t)); err == nil {
				t.Fatal("run succeeded; want input rejection")
			}
		})
	}
}

func TestValidateProofRejectsWeakShapes(t *testing.T) {
	now := time.Date(2026, 6, 21, 3, 40, 0, 0, time.UTC)
	valid := proof{
		Passed:    true,
		CheckedAt: now.Format(time.RFC3339Nano),
		Request: proofRequest{
			APIBaseURL:                           "https://openclarion.example.test",
			SourceProfileID:                      42,
			AlertName:                            "OpenClarionAutoDiagnosisSmoke",
			SmokeID:                              "openclarion-auto-diagnosis-smoke-20260621T034000Z",
			StartsAt:                             now.Add(-time.Minute).Format(time.RFC3339Nano),
			ExpectedNotificationChannelProfileID: 5,
			ExpectedContentKind:                  "assistant_message",
			HTTPTimeout:                          "15s",
			RoomTimeout:                          "10m0s",
			PollInterval:                         "5s",
		},
		Webhook: webhookProof{
			HTTPStatus: http.StatusAccepted,
			SourceID:   42,
			Received:   expectedSyntheticAlertsReceived,
			Skipped: webhookSkipped{
				Resolved:   expectedSyntheticResolvedSkipped,
				Suppressed: expectedSyntheticSuppressedSkipped,
			},
			Ingested: api.AlertmanagerWebhookIngestResponseIngested{
				Total: expectedSyntheticIngestedTotal,
				Saved: 1,
			},
			AutoDiagnosis: autoDiagnosisProof{
				PoliciesMatched: 1,
				Snapshots:       1,
				RoomsStarted:    1,
				Rooms: []autoDiagnosisRoomRef{
					{
						PolicyID:           9,
						EvidenceSnapshotID: 11,
						SessionID:          "diagnosis-session-auto-1",
						WorkflowID:         "diagnosis-room-diagnosis-session-auto-1",
						RunID:              "run-1",
					},
				},
			},
		},
		Room: roomProof{
			SessionID:                              "diagnosis-session-auto-1",
			ChatSessionID:                          31,
			DiagnosisTaskID:                        31,
			EvidenceSnapshotID:                     11,
			WorkflowID:                             "diagnosis-room-diagnosis-session-auto-1",
			RunID:                                  "run-1",
			TaskStatus:                             "running",
			RoomStatus:                             "open",
			TurnCount:                              1,
			LatestProgressStatus:                   "in_progress",
			LatestConfidence:                       "medium",
			LatestRequiresHumanReview:              boolPtr(true),
			LatestEvidenceRequestCount:             1,
			LatestConfidenceRationalePresent:       true,
			LatestConfidenceImprovementPathPresent: true,
			NotificationEventKinds:                 []string{eventAssistantTurnNotification},
			NotificationProviderStatuses:           []string{"delivered"},
			NotificationContentProofs: []notificationContentProof{
				{
					EventKind:                    eventAssistantTurnNotification,
					NotificationChannelProfileID: 5,
					ContentKind:                  "assistant_message",
					ContentSHA256:                strings.Repeat("a", 64),
					RecommendedActionCount:       1,
					EvidenceRequestCount:         1,
				},
			},
			AINotificationDelivered: true,
		},
		Evidence: proofEvidence,
	}
	if err := validateProof(valid); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}

	bad := valid
	bad.Room.AINotificationDelivered = false
	if err := validateProof(bad); err == nil {
		t.Fatal("missing AI notification proof accepted")
	}

	bad = valid
	bad.Webhook.AutoDiagnosis.RoomsStarted = 0
	if err := validateProof(bad); err == nil {
		t.Fatal("missing auto_room start accepted")
	}

	bad = valid
	bad.Room.NotificationContentProofs = nil
	if err := validateProof(bad); err == nil {
		t.Fatal("missing notification content proof accepted")
	}

	bad = valid
	bad.Room.LatestConfidenceRationalePresent = false
	if err := validateProof(bad); err == nil {
		t.Fatal("missing confidence rationale proof accepted")
	}

	bad = valid
	bad.Room.LatestConfidenceImprovementPathPresent = false
	if err := validateProof(bad); err == nil {
		t.Fatal("missing confidence improvement path proof accepted")
	}

	bad = valid
	bad.Room.NotificationContentProofs = append([]notificationContentProof(nil), valid.Room.NotificationContentProofs...)
	bad.Room.NotificationContentProofs[0].NotificationChannelProfileID = 6
	if err := validateProof(bad); err == nil {
		t.Fatal("mismatched notification channel proof accepted")
	}

	bad = valid
	bad.Room.NotificationContentProofs = append([]notificationContentProof(nil), valid.Room.NotificationContentProofs...)
	bad.Room.NotificationContentProofs[0].ContentKind = "final_conclusion"
	if err := validateProof(bad); err == nil {
		t.Fatal("mismatched content kind proof accepted")
	}

	withRequired := valid
	withRequired.Request.ExpectedContentKind = ""
	withRequired.Request.RequiredContentKinds = []string{"assistant_message"}
	if err := validateProof(withRequired); err != nil {
		t.Fatalf("valid required content proof rejected: %v", err)
	}

	bad = withRequired
	bad.Request.RequiredContentKinds = []string{"assistant_message", "final_conclusion"}
	if err := validateProof(bad); err == nil {
		t.Fatal("missing required content kind proof accepted")
	}

	bad = withRequired
	bad.Request.RequiredContentKinds = []string{"assistant_message", "assistant_message"}
	if err := validateProof(bad); err == nil {
		t.Fatal("duplicate required content kinds accepted")
	}
}

func alertmanagerAutoDiagnosisResponse() api.AlertmanagerWebhookIngestResponse {
	return api.AlertmanagerWebhookIngestResponse{
		SourceID:          42,
		Received:          expectedSyntheticAlertsReceived,
		SkippedResolved:   expectedSyntheticResolvedSkipped,
		SkippedSuppressed: expectedSyntheticSuppressedSkipped,
		Ingested: api.AlertmanagerWebhookIngestResponseIngested{
			Total: expectedSyntheticIngestedTotal,
			Saved: 1,
		},
		AutoDiagnosis: &api.AlertmanagerWebhookAutoDiagnosisSummary{
			PoliciesMatched: 1,
			Snapshots:       1,
			RoomsStarted:    1,
			Rooms: []api.AlertmanagerWebhookAutoDiagnosisRoom{
				{
					PolicyID:           9,
					EvidenceSnapshotID: 11,
					SessionID:          "diagnosis-session-auto-1",
					InitialMessageID:   "initial-msg-1",
					WorkflowID:         "diagnosis-room-diagnosis-session-auto-1",
					RunID:              "run-1",
				},
			},
		},
	}
}

func reportWorkflowPolicyWithNotificationChannel() api.ReportWorkflowPolicy {
	policy := reportWorkflowPolicyWithoutNotificationChannel(9)
	policy.ReportNotificationChannelProfileID.Set(5)
	return policy
}

func reportWorkflowPolicyWithoutNotificationChannel(policyID int64) api.ReportWorkflowPolicy {
	var channel api.Nullable[int64]
	channel.SetNull()
	return api.ReportWorkflowPolicy{
		ID:                                 policyID,
		Name:                               "auto-room-policy",
		AlertSourceProfileID:               42,
		GroupingPolicyID:                   7,
		ReportNotificationChannelProfileID: channel,
		TriggerMode:                        api.ReportWorkflowTriggerModeManualReplay,
		ReportScenario:                     api.ReportWorkflowScenarioSingleAlert,
		DiagnosisFollowUp:                  api.DiagnosisFollowUpModeAutoRoom,
		Enabled:                            true,
		CreatedAt:                          time.Date(2026, 6, 21, 3, 0, 0, 0, time.UTC),
		UpdatedAt:                          time.Date(2026, 6, 21, 3, 0, 0, 0, time.UTC),
	}
}

func diagnosisRoomSummary(now time.Time) api.DiagnosisRoomSummary {
	messageID := "msg-1/assistant"
	providerMessageID := "wecom-final-ready-1"
	assistantTurnID := int64(32)
	assistantSequence := 2
	turnCount := 1
	conclusionVersion := "diagnosis-session-auto-1:1"
	conclusionReason := "assistant_marked_final"
	requiresHumanReview := true
	confidence := api.ReportConfidenceHigh
	confidenceRationale := "The first automatic diagnosis has enough signal for notification, but current sibling alert evidence remains part of the confidence path."
	return api.DiagnosisRoomSummary{
		SessionID:          "diagnosis-session-auto-1",
		ChatSessionID:      31,
		DiagnosisTaskID:    31,
		EvidenceSnapshotID: 11,
		WorkflowID:         "diagnosis-room-diagnosis-session-auto-1",
		RunID:              "run-1",
		TaskStatus:         api.DiagnosisTaskStatusRunning,
		RoomStatus:         api.Open,
		TurnCount:          1,
		StartedAt:          now.Add(-10 * time.Second),
		LastActivityAt:     now,
		LatestProgress: &api.DiagnosisRoomProgressSummary{
			DiagnosisTaskID:      31,
			SessionID:            stringPtr("diagnosis-session-auto-1"),
			ChatSessionID:        int64Ptr(31),
			EventKind:            "diagnosis_room.turn_persisted",
			Status:               "in_progress",
			EvidenceSnapshotID:   11,
			Confidence:           api.ReportConfidenceMedium,
			RequiresHumanReview:  true,
			ConfidenceRationale:  &confidenceRationale,
			EvidenceRequestCount: 1,
			AssistantMessageID:   &messageID,
			AssistantTurnID:      &assistantTurnID,
			AssistantSequence:    &assistantSequence,
			TurnCount:            &turnCount,
			OccurredAt:           now,
			RecordedAt:           now,
		},
		LatestConclusion: &api.DiagnosisRoomConclusionSummary{
			DiagnosisTaskID:     31,
			SessionID:           "diagnosis-session-auto-1",
			ChatSessionID:       31,
			EventKind:           "diagnosis_room.final_conclusion_ready",
			Status:              "available",
			Source:              "latest_assistant_turn",
			Reason:              &conclusionReason,
			EvidenceSnapshotID:  int64Ptr(11),
			ConclusionVersion:   &conclusionVersion,
			AssistantTurnID:     &assistantTurnID,
			AssistantMessageID:  &messageID,
			AssistantSequence:   &assistantSequence,
			AssistantOccurredAt: &now,
			Content:             "Final diagnosis is ready for operator review.",
			Confidence:          &confidence,
			RequiresHumanReview: &requiresHumanReview,
			RecommendedActions:  []string{"Review the correlated alert evidence."},
			EvidenceRequests:    []api.DiagnosisRoomEvidenceRequestSummary{},
			RecordedAt:          now,
		},
		NotificationTimeline: []api.DiagnosisRoomNotificationTimelineEntry{
			{
				EventKind:                    eventFinalReadyNotification,
				NotificationChannelProfileID: int64Ptr(5),
				ProviderStatus:               "delivered",
				ProviderMessageID:            &providerMessageID,
				AssistantMessageID:           &messageID,
				AssistantTurnID:              &assistantTurnID,
				AssistantSequence:            &assistantSequence,
				TurnCount:                    &turnCount,
				ContentKind:                  stringPtr("final_conclusion"),
				ContentSha256:                stringPtr(strings.Repeat("a", 64)),
				RecommendedActionCount:       intPtr(1),
				EvidenceRequestCount:         intPtr(0),
				OccurredAt:                   now.Add(time.Millisecond),
			},
		},
		CreatedAt: now.Add(-10 * time.Second),
		UpdatedAt: now,
	}
}

func diagnosisRoomSummaryWithAssistantProof(now time.Time) api.DiagnosisRoomSummary {
	room := diagnosisRoomSummary(now)
	messageID := "msg-1/assistant"
	providerMessageID := "wecom-assistant-turn-1"
	assistantTurnID := int64(32)
	assistantSequence := 2
	turnCount := 1
	room.NotificationTimeline = append([]api.DiagnosisRoomNotificationTimelineEntry{
		{
			EventKind:                    eventAssistantTurnNotification,
			NotificationChannelProfileID: int64Ptr(5),
			ProviderStatus:               "delivered",
			ProviderMessageID:            &providerMessageID,
			AssistantMessageID:           &messageID,
			AssistantTurnID:              &assistantTurnID,
			AssistantSequence:            &assistantSequence,
			TurnCount:                    &turnCount,
			ContentKind:                  stringPtr("assistant_message"),
			ContentSha256:                stringPtr(strings.Repeat("b", 64)),
			RecommendedActionCount:       intPtr(1),
			EvidenceRequestCount:         intPtr(1),
			OccurredAt:                   now,
		},
	}, room.NotificationTimeline...)
	return room
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func stringPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func unusedHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be used for rejected inputs")
		return nil, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
