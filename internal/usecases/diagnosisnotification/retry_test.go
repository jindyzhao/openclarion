package diagnosisnotification

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceRetriesLatestFailedFinalReadyNotification(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(t, failedNotificationEvent(41, EventFinalReadyNotification, "failed", now.Add(-time.Minute)))
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-1",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{consultationProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventFinalReadyNotification,
		Principal: ports.AuthPrincipal{
			Subject: "notification-admin-1",
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent ||
		result.Event.ID == 0 ||
		result.Event.Kind != EventFinalReadyNotification {
		t.Fatalf("result = %+v", result)
	}
	if provider.called != 1 ||
		provider.req.IdempotencyKey != "diagnosis_room:31:abc/final_ready_notification" ||
		provider.req.NotificationChannelID != 9 ||
		provider.req.DiagnosisTaskID != 31 ||
		!strings.Contains(provider.req.Body, "AI diagnosis is ready") ||
		!strings.Contains(provider.req.Body, "Review room: https://openclarion.example.test/diagnosis-room?auth_mode=session&session_id=session-1&wecom_auto_login=1&wecom_launch_context=app_conversation") {
		t.Fatalf("provider called=%d req=%+v", provider.called, provider.req)
	}
	var payload notificationPayload
	if err := json.Unmarshal(result.Event.Payload, &payload); err != nil {
		t.Fatalf("retry event payload: %v", err)
	}
	if payload.ProviderStatus != "delivered" ||
		payload.ProviderMessageID != "wecom-retry-1" ||
		payload.IdempotencyKey != provider.req.IdempotencyKey ||
		payload.RoomURL != "https://openclarion.example.test/diagnosis-room?auth_mode=session&session_id=session-1&wecom_auto_login=1&wecom_launch_context=app_conversation" ||
		payload.ActorSubject != "notification-admin-1" ||
		payload.RetryRequestedBy != "notification-admin-1" ||
		payload.Source != "DiagnosisNotificationRetry" {
		t.Fatalf("retry payload = %+v", payload)
	}
}

func TestServiceCountsEarlierAttemptsBeforeAppendingRetry(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 2, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(
		t,
		failedNotificationEvent(42, EventFinalReadyNotification, "failed", now.Add(-30*time.Second)),
		failedNotificationEvent(41, EventFinalReadyNotification, "failed", now.Add(-time.Minute)),
	)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-2",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{consultationProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventFinalReadyNotification,
		Principal: ports.AuthPrincipal{
			Subject: "notification-admin-1",
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent || provider.called != 1 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
	wantDedupeKey := notificationEventDedupeKey(
		EventFinalReadyNotification,
		"session-1",
		notificationDedupeComponent("diagnosis_room:31:abc/final_ready_notification", 2),
	)
	if result.Event.DedupeKey == nil || *result.Event.DedupeKey != wantDedupeKey {
		t.Fatalf("retry dedupe key = %v, want %q", result.Event.DedupeKey, wantDedupeKey)
	}
}

func TestServiceDoesNotRetryWhenLaterDeliveryExists(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 10, 0, 0, time.UTC)
	delivered := failedNotificationEvent(42, EventAssistantTurnNotification, "delivered", now)
	failed := failedNotificationEvent(41, EventAssistantTurnNotification, "failed", now.Add(-time.Minute))
	repo := notificationRetryRepoFixture(t, delivered, failed)
	provider := &recordingIMProvider{}
	service := mustRetryService(t, repo, &recordingResolver{consultationProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventAssistantTurnNotification,
		Principal: ports.AuthPrincipal{
			Subject: "admin-1",
			Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateAlreadyDelivered ||
		result.Event.ID != delivered.ID ||
		provider.called != 0 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
}

func TestServiceRetriesDeliveredAssistantNotificationMissingContentProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 12, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(t, notificationEventFromPayload(
		41,
		EventAssistantTurnNotification,
		map[string]any{
			"source":                          "DiagnosisRoomWorkflow",
			"kind":                            EventAssistantTurnNotification,
			"session_id":                      "session-1",
			"chat_session_id":                 21,
			"diagnosis_task_id":               31,
			"evidence_snapshot_id":            7,
			"alert_group_id":                  3,
			"owner_subject":                   "owner-1",
			"assistant_message_id":            "msg-proof/assistant",
			"assistant_turn_id":               32,
			"assistant_sequence":              2,
			"turn_count":                      1,
			"idempotency_key":                 "diagnosis_room:31:proof/assistant_notification",
			"notification_channel_profile_id": 9,
			"provider_status":                 "delivered",
		},
		now.Add(-time.Minute),
	))
	repo.turns = []domain.ChatTurn{assistantTurnFixture(t, "msg-proof/assistant", "Hydrated assistant diagnosis from the retained transcript.")}
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-proof-1",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{consultationProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventAssistantTurnNotification,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent || provider.called != 1 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
	if provider.req.Title != "Initial AI diagnosis report: session-1" ||
		!strings.Contains(provider.req.Body, "Initial AI diagnosis report is ready") ||
		!strings.Contains(provider.req.Body, "before confidence is raised or closure is confirmed") {
		t.Fatalf("provider notification = %+v, want initial report retry wording", provider.req)
	}
	if !strings.Contains(provider.req.Body, "Hydrated assistant diagnosis from the retained transcript.") {
		t.Fatalf("provider body = %q, want hydrated assistant content", provider.req.Body)
	}
	if !strings.Contains(provider.req.Body, "Missing evidence:") ||
		!strings.Contains(provider.req.Body, "[high] CPU trend - Confirm post-scale CPU trend.") ||
		!strings.Contains(provider.req.Body, "Evidence collection suggestions:") ||
		!strings.Contains(provider.req.Body, "[medium] Collect JVM heap usage - Attach the latest JVM heap usage for the affected pod.") ||
		!strings.Contains(provider.req.Body, "Confidence rationale: CPU evidence is present but post-scale recovery is not yet proven.") ||
		!strings.Contains(provider.req.Body, "Next action: collect 1 executable evidence request(s), provide 1 operator-supplied evidence item(s), and review 1 evidence collection suggestion(s).") ||
		!strings.Contains(provider.req.Body, "Executable evidence requests: 1") {
		t.Fatalf("provider body = %q, want missing evidence and collection guidance", provider.req.Body)
	}
	assertRetryBodyOrder(t, provider.req.Body, "Next action:", "AI diagnosis:")
	assertRetryBodyOrder(t, provider.req.Body, "Confidence rationale:", "AI diagnosis:")
	assertRetryBodyOrder(t, provider.req.Body, "Missing evidence:", "AI diagnosis:")
	assertRetryBodyOrder(t, provider.req.Body, "Evidence collection suggestions:", "AI diagnosis:")
	assertRetryBodyOrder(t, provider.req.Body, "Executable evidence requests: 1", "AI diagnosis:")
	var payload notificationPayload
	if err := json.Unmarshal(result.Event.Payload, &payload); err != nil {
		t.Fatalf("retry event payload: %v", err)
	}
	if payload.AssistantMessage != "Hydrated assistant diagnosis from the retained transcript." ||
		payload.ProviderMessageID != "wecom-retry-proof-1" {
		t.Fatalf("retry payload = %+v", payload)
	}
}

func TestServiceRetriesDeliveredFinalReadyNotificationMissingContentProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 14, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(
		t,
		notificationEventFromPayload(
			41,
			EventFinalReadyNotification,
			map[string]any{
				"source":                          "DiagnosisRoomWorkflow",
				"kind":                            EventFinalReadyNotification,
				"session_id":                      "session-1",
				"chat_session_id":                 21,
				"diagnosis_task_id":               31,
				"evidence_snapshot_id":            7,
				"alert_group_id":                  3,
				"owner_subject":                   "owner-1",
				"assistant_message_id":            "msg-final-proof/assistant",
				"assistant_turn_id":               32,
				"assistant_sequence":              2,
				"turn_count":                      1,
				"idempotency_key":                 "diagnosis_room:31:proof/final_ready_notification",
				"notification_channel_profile_id": 9,
				"provider_status":                 "delivered",
			},
			now.Add(-time.Minute),
		),
		notificationEventFromPayload(
			42,
			eventFinalConclusionReady,
			map[string]any{
				"kind":                 eventFinalConclusionReady,
				"assistant_message_id": "msg-final-proof/assistant",
				"assistant_turn_id":    32,
				"assistant_sequence":   2,
				"final_conclusion": map[string]any{
					"status":                "available",
					"content":               "Hydrated final conclusion from the retained final-ready event.",
					"confidence":            "high",
					"confidence_rationale":  "Final-ready evidence is strong, but deployment events remain useful for review.",
					"requires_human_review": true,
					"assistant_message_id":  "msg-final-proof/assistant",
					"assistant_turn_id":     32,
					"assistant_sequence":    2,
					"findings":              []string{"The failing dependency recovered after scaling."},
					"recommended_actions":   []string{"Confirm the deployment scale target."},
					"evidence_requests":     []map[string]any{{"tool": "active_alerts", "reason": "Confirm current state."}},
					"missing_evidence_requests": []map[string]any{{
						"label":    "Deployment event timeline",
						"detail":   "Attach rollout and scale events around the incident window.",
						"priority": "high",
					}},
					"evidence_collection_suggestions": []map[string]any{{
						"label":    "Dependency latency trend",
						"detail":   "Collect the dependency p95 latency trend for the same window.",
						"priority": "medium",
					}},
				},
			},
			now.Add(-2*time.Minute),
		),
	)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-final-proof-1",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{consultationProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventFinalReadyNotification,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent || provider.called != 1 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
	if !strings.Contains(provider.req.Body, "Hydrated final conclusion from the retained final-ready event.") {
		t.Fatalf("provider body = %q, want hydrated final conclusion", provider.req.Body)
	}
	if !strings.Contains(provider.req.Body, "Missing evidence:") ||
		!strings.Contains(provider.req.Body, "[high] Deployment event timeline - Attach rollout and scale events around the incident window.") ||
		!strings.Contains(provider.req.Body, "Evidence collection suggestions:") ||
		!strings.Contains(provider.req.Body, "[medium] Dependency latency trend - Collect the dependency p95 latency trend for the same window.") ||
		!strings.Contains(provider.req.Body, "Confidence rationale: Final-ready evidence is strong, but deployment events remain useful for review.") ||
		!strings.Contains(provider.req.Body, "Next action: collect 1 executable evidence request(s), provide 1 operator-supplied evidence item(s), and review 1 evidence collection suggestion(s).") ||
		!strings.Contains(provider.req.Body, "Executable evidence requests: 1") {
		t.Fatalf("provider body = %q, want final-ready evidence guidance", provider.req.Body)
	}
	assertRetryBodyOrder(t, provider.req.Body, "Next action:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Confidence rationale:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Missing evidence:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Evidence collection suggestions:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Executable evidence requests: 1", "AI conclusion:")
	var payload notificationPayload
	if err := json.Unmarshal(result.Event.Payload, &payload); err != nil {
		t.Fatalf("retry event payload: %v", err)
	}
	if payload.FinalConclusion.Content != "Hydrated final conclusion from the retained final-ready event." ||
		payload.ProviderMessageID != "wecom-retry-final-proof-1" {
		t.Fatalf("retry payload = %+v", payload)
	}
}

func TestServiceRetriesCloseNotificationWithEvidenceGuidanceBeforeConclusion(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 18, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(
		t,
		notificationEventFromPayload(
			43,
			EventCloseNotification,
			map[string]any{
				"source":                          "DiagnosisRoomWorkflow",
				"kind":                            EventCloseNotification,
				"session_id":                      "session-1",
				"chat_session_id":                 21,
				"diagnosis_task_id":               31,
				"evidence_snapshot_id":            7,
				"alert_group_id":                  3,
				"owner_subject":                   "owner-1",
				"turn_count":                      1,
				"close_reason":                    "human_confirmed",
				"room_url":                        "https://openclarion.example.test/diagnosis-room?session_id=session-1",
				"idempotency_key":                 "diagnosis_room:31:proof/close_notification",
				"notification_channel_profile_id": 9,
				"provider_status":                 "failed",
				"final_conclusion": map[string]any{
					"status":               "available",
					"content":              "Close retry should preserve the final AI diagnosis.",
					"confidence":           "medium",
					"confidence_rationale": "The final evidence is acceptable, but the owner remediation note is still part of the audit trail.",
					"findings":             []string{"CPU saturation recovered after scaling."},
					"recommended_actions":  []string{"Keep the owner remediation note attached."},
					"evidence_requests": []map[string]any{{
						"tool":   "metric_query",
						"reason": "Confirm post-scale CPU trend.",
						"query":  `sum(rate(container_cpu_usage_seconds_total{namespace="prod"}[5m]))`,
						"limit":  3,
					}},
					"missing_evidence_requests": []map[string]any{{
						"label":    "Owner remediation note",
						"detail":   "Attach the final owner mitigation note before archiving.",
						"priority": "high",
					}},
					"evidence_collection_suggestions": []map[string]any{{
						"label":    "Post-scale CPU trend",
						"detail":   "Collect a bounded post-scale CPU trend for audit.",
						"priority": "medium",
					}},
				},
			},
			now.Add(-time.Minute),
		),
	)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-close-proof-1",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{closeProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventCloseNotification,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent || provider.called != 1 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
	if !strings.Contains(provider.req.Body, "Close reason: human_confirmed") ||
		!strings.Contains(provider.req.Body, "Missing evidence:") ||
		!strings.Contains(provider.req.Body, "[high] Owner remediation note - Attach the final owner mitigation note before archiving.") ||
		!strings.Contains(provider.req.Body, "Evidence collection suggestions:") ||
		!strings.Contains(provider.req.Body, "[medium] Post-scale CPU trend - Collect a bounded post-scale CPU trend for audit.") ||
		!strings.Contains(provider.req.Body, "Confidence rationale: The final evidence is acceptable, but the owner remediation note is still part of the audit trail.") ||
		!strings.Contains(provider.req.Body, "Next action: collect 1 executable evidence request(s), provide 1 operator-supplied evidence item(s), and review 1 evidence collection suggestion(s).") ||
		!strings.Contains(provider.req.Body, "Executable evidence requests: 1") ||
		!strings.Contains(provider.req.Body, "AI conclusion: Close retry should preserve the final AI diagnosis.") {
		t.Fatalf("provider body = %q, want close retry evidence guidance", provider.req.Body)
	}
	assertRetryBodyOrder(t, provider.req.Body, "Next action:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Confidence rationale:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Missing evidence:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Evidence collection suggestions:", "AI conclusion:")
	assertRetryBodyOrder(t, provider.req.Body, "Executable evidence requests: 1", "AI conclusion:")
}

func TestServiceRetriesDeliveredCloseNotificationMissingContentProof(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 19, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(
		t,
		notificationEventFromPayload(
			43,
			EventCloseNotification,
			map[string]any{
				"source":                          "DiagnosisRoomWorkflow",
				"kind":                            EventCloseNotification,
				"session_id":                      "session-1",
				"chat_session_id":                 21,
				"diagnosis_task_id":               31,
				"evidence_snapshot_id":            7,
				"alert_group_id":                  3,
				"owner_subject":                   "owner-1",
				"turn_count":                      1,
				"close_reason":                    "human_confirmed",
				"room_url":                        "https://openclarion.example.test/diagnosis-room?session_id=session-1",
				"idempotency_key":                 "diagnosis_room:31:proof/close_notification",
				"notification_channel_profile_id": 9,
				"provider_status":                 "delivered",
			},
			now.Add(-time.Minute),
		),
		notificationEventFromPayload(
			44,
			eventClosed,
			map[string]any{
				"source":       "DiagnosisRoomWorkflow",
				"kind":         eventClosed,
				"session_id":   "session-1",
				"turn_count":   1,
				"close_reason": "human_confirmed",
				"final_conclusion": map[string]any{
					"status":              "available",
					"content":             "Hydrated close conclusion from the retained close event.",
					"confidence":          "high",
					"recommended_actions": []string{"Retain the owner close note."},
					"evidence_requests": []map[string]any{{
						"tool":   "active_alerts",
						"reason": "Confirm related alerts are resolved.",
					}},
					"missing_evidence_requests": []map[string]any{{
						"label":    "Owner close note",
						"detail":   "Attach the owner note that confirms service recovery.",
						"priority": "high",
					}},
				},
			},
			now.Add(-2*time.Minute),
		),
	)
	provider := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "wecom-retry-close-proof-2",
		Status:            "delivered",
		Raw:               json.RawMessage(`{"errcode":0}`),
	}}
	service := mustRetryService(t, repo, &recordingResolver{closeProvider: provider}, now)

	result, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventCloseNotification,
		Principal: ports.AuthPrincipal{
			Subject: "owner-1",
			Roles:   []ports.AuthRole{ports.AuthRoleOwner},
		},
	})
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if result.RetryState != RetryStateSent || provider.called != 1 {
		t.Fatalf("result=%+v provider called=%d", result, provider.called)
	}
	if !strings.Contains(provider.req.Body, "Hydrated close conclusion from the retained close event.") ||
		!strings.Contains(provider.req.Body, "Missing evidence:") ||
		!strings.Contains(provider.req.Body, "[high] Owner close note - Attach the owner note that confirms service recovery.") ||
		!strings.Contains(provider.req.Body, "Next action: collect 1 executable evidence request(s) and provide 1 operator-supplied evidence item(s).") ||
		!strings.Contains(provider.req.Body, "Executable evidence requests: 1") {
		t.Fatalf("provider body = %q, want hydrated close proof content", provider.req.Body)
	}
	var payload notificationPayload
	if err := json.Unmarshal(result.Event.Payload, &payload); err != nil {
		t.Fatalf("retry event payload: %v", err)
	}
	if payload.FinalConclusion.Content != "Hydrated close conclusion from the retained close event." ||
		payload.ProviderMessageID != "wecom-retry-close-proof-2" ||
		payload.ProviderStatus != "delivered" {
		t.Fatalf("retry payload = %+v", payload)
	}
}

func TestServiceRejectsUnauthenticatedPrincipal(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 20, 0, 0, time.UTC)
	repo := notificationRetryRepoFixture(t, failedNotificationEvent(41, EventCloseNotification, "failed", now))
	provider := &recordingIMProvider{}
	service := mustRetryService(t, repo, &recordingResolver{closeProvider: provider}, now)

	_, err := service.Retry(context.Background(), Request{
		SessionID: "session-1",
		EventKind: EventCloseNotification,
		Principal: ports.AuthPrincipal{
			Subject: " ",
		},
	})
	if err == nil {
		t.Fatal("Retry error = nil, want authentication error")
	}
	if !errors.Is(err, diagnosisauth.ErrUnauthenticated) {
		t.Fatalf("Retry error = %v, want ErrUnauthenticated", err)
	}
	if provider.called != 0 {
		t.Fatalf("provider called = %d, want 0", provider.called)
	}
}

func TestNotificationPayloadFillFinalConclusionMetadataCopiesConfidenceRationale(t *testing.T) {
	payload := notificationPayload{}
	payload.fillFinalConclusionMetadata(assistantTurnMetadata{
		Confidence: "medium",
		ConsultationInsight: diagnosisroom.ConsultationInsight{
			ConfidenceRationale: "Collected metrics confirm saturation, but owner remediation is still pending.",
		},
	})

	if payload.FinalConclusion.Confidence != "medium" {
		t.Fatalf("FinalConclusion.Confidence = %q, want medium", payload.FinalConclusion.Confidence)
	}
	if payload.FinalConclusion.ConfidenceRationale != "Collected metrics confirm saturation, but owner remediation is still pending." {
		t.Fatalf("FinalConclusion.ConfidenceRationale = %q, want copied consultation rationale", payload.FinalConclusion.ConfidenceRationale)
	}
}

func TestNotificationNextActionLineIncludesSuggestionsWithOperatorEvidence(t *testing.T) {
	line := notificationNextActionLine(
		nil,
		[]diagnosisroom.ConsultationEvidenceRequest{{
			Label:    "Owner remediation note",
			Detail:   "Attach the mitigation note.",
			Priority: "high",
		}},
		[]diagnosisroom.ConsultationEvidenceRequest{{
			Label:    "Recovery metric",
			Detail:   "Collect post-recovery latency.",
			Priority: "medium",
		}},
		false,
	)

	if line != "Next action: provide 1 operator-supplied evidence item(s) and review 1 evidence collection suggestion(s) before confidence can be raised." {
		t.Fatalf("notificationNextActionLine = %q", line)
	}
}

func mustRetryService(t *testing.T, repo *retryTestRepo, resolver *recordingResolver, now time.Time) *Service {
	t.Helper()
	service, err := NewService(retryFactory{repo: repo}, resolver, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func notificationRetryRepoFixture(t *testing.T, events ...domain.DiagnosisTaskEvent) *retryTestRepo {
	t.Helper()
	return &retryTestRepo{
		t: t,
		session: domain.ChatSession{
			ID:              21,
			DiagnosisTaskID: 31,
			SessionKey:      "session-1",
			OwnerSubject:    "owner-1",
			Status:          domain.ChatSessionStatusOpen,
			TurnCount:       1,
		},
		task: domain.DiagnosisTask{
			ID:                 31,
			EvidenceSnapshotID: 7,
		},
		events: append([]domain.DiagnosisTaskEvent(nil), events...),
	}
}

func failedNotificationEvent(id int64, kind string, status string, occurredAt time.Time) domain.DiagnosisTaskEvent {
	payload := map[string]any{
		"source":                          "DiagnosisRoomWorkflow",
		"kind":                            kind,
		"session_id":                      "session-1",
		"chat_session_id":                 21,
		"diagnosis_task_id":               31,
		"evidence_snapshot_id":            7,
		"alert_group_id":                  3,
		"owner_subject":                   "owner-1",
		"assistant_message_id":            "msg-1/assistant",
		"assistant_turn_id":               32,
		"assistant_sequence":              2,
		"turn_count":                      1,
		"room_url":                        "https://openclarion.example.test/diagnosis-room?session_id=session-1",
		"idempotency_key":                 "diagnosis_room:31:abc/final_ready_notification",
		"notification_channel_profile_id": 9,
		"provider_status":                 status,
		"assistant_message":               "CPU pressure still needs evidence.",
		"confidence":                      "high",
		"requires_human_review":           true,
		"findings":                        []string{"CPU saturation is active"},
		"recommended_actions":             []string{"Collect current pod metrics"},
		"final_conclusion": map[string]any{
			"status":                "available",
			"content":               "CPU pressure is the likely cause.",
			"confidence":            "high",
			"requires_human_review": true,
			"findings":              []string{"CPU saturation is active"},
			"recommended_actions":   []string{"Collect current pod metrics"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return domain.DiagnosisTaskEvent{
		ID:         domain.DiagnosisTaskEventID(id),
		TaskID:     31,
		Kind:       kind,
		Payload:    raw,
		OccurredAt: occurredAt,
		RecordedAt: occurredAt,
	}
}

func notificationEventFromPayload(id int64, kind string, payload map[string]any, occurredAt time.Time) domain.DiagnosisTaskEvent {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return domain.DiagnosisTaskEvent{
		ID:         domain.DiagnosisTaskEventID(id),
		TaskID:     31,
		Kind:       kind,
		Payload:    raw,
		OccurredAt: occurredAt,
		RecordedAt: occurredAt,
	}
}

func assistantTurnFixture(t *testing.T, messageID string, content string) domain.ChatTurn {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{
		"confidence":            "medium",
		"confidence_rationale":  "CPU evidence is present but post-scale recovery is not yet proven.",
		"requires_human_review": true,
		"findings":              []string{"CPU saturation is active"},
		"recommended_actions":   []string{"Collect current pod metrics"},
		"evidence_requests":     []map[string]any{{"tool": "active_alerts", "reason": "Confirm current state."}},
		"consultation_insight": map[string]any{
			"conclusion_status": "needs_evidence",
			"missing_evidence_requests": []map[string]any{{
				"label":    "CPU trend",
				"detail":   "Confirm post-scale CPU trend.",
				"priority": "high",
			}},
			"evidence_collection_suggestions": []map[string]any{{
				"label":    "Collect JVM heap usage",
				"detail":   "Attach the latest JVM heap usage for the affected pod.",
				"priority": "medium",
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	return domain.ChatTurn{
		ID:           32,
		SessionID:    21,
		MessageID:    messageID,
		Sequence:     2,
		Role:         domain.ChatRoleAssistant,
		ActorSubject: "assistant",
		Content:      content,
		Metadata:     metadata,
		OccurredAt:   time.Date(2026, 6, 21, 12, 5, 0, 0, time.UTC),
	}
}

type retryFactory struct {
	repo *retryTestRepo
}

func (f retryFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("unexpected Begin call")
}

func (f retryFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, retryUOW{repo: f.repo})
}

type retryUOW struct {
	repo *retryTestRepo
}

func (u retryUOW) Alerts() ports.AlertRepository {
	u.repo.failNow("unexpected Alerts call")
	return nil
}
func (u retryUOW) Evidence() ports.EvidenceRepository {
	u.repo.failNow("unexpected Evidence call")
	return nil
}
func (u retryUOW) Diagnosis() ports.DiagnosisRepository { return u.repo }
func (u retryUOW) Reports() ports.ReportRepository {
	u.repo.failNow("unexpected Reports call")
	return nil
}
func (u retryUOW) Config() ports.ConfigurationRepository {
	u.repo.failNow("unexpected Config call")
	return nil
}
func (u retryUOW) Directory() ports.DirectoryRepository {
	u.repo.failNow("unexpected Directory call")
	return nil
}
func (u retryUOW) RBAC() ports.RBACRepository {
	u.repo.failNow("unexpected RBAC call")
	return nil
}
func (u retryUOW) Commit(context.Context) error   { return errors.New("unexpected Commit call") }
func (u retryUOW) Rollback(context.Context) error { return errors.New("unexpected Rollback call") }

type retryTestRepo struct {
	t       *testing.T
	session domain.ChatSession
	task    domain.DiagnosisTask
	events  []domain.DiagnosisTaskEvent
	turns   []domain.ChatTurn
}

func (r *retryTestRepo) SaveTask(context.Context, domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	return domain.DiagnosisTask{}, errors.New("unexpected SaveTask call")
}
func (r *retryTestRepo) UpdateTask(context.Context, domain.DiagnosisTask) (domain.DiagnosisTask, error) {
	return domain.DiagnosisTask{}, errors.New("unexpected UpdateTask call")
}
func (r *retryTestRepo) FindTaskByID(_ context.Context, id domain.DiagnosisTaskID) (domain.DiagnosisTask, error) {
	if r.task.ID != id {
		return domain.DiagnosisTask{}, domain.ErrNotFound
	}
	return r.task, nil
}
func (r *retryTestRepo) FindTaskByExecution(context.Context, string, string) (domain.DiagnosisTask, error) {
	return domain.DiagnosisTask{}, errors.New("unexpected FindTaskByExecution call")
}
func (r *retryTestRepo) ListTasksByEvidenceSnapshot(context.Context, domain.EvidenceSnapshotID, int) ([]domain.DiagnosisTask, error) {
	return nil, errors.New("unexpected ListTasksByEvidenceSnapshot call")
}
func (r *retryTestRepo) AppendEvent(_ context.Context, event domain.DiagnosisTaskEvent) (domain.DiagnosisTaskEvent, error) {
	event.ID = domain.DiagnosisTaskEventID(100 + len(r.events))
	r.events = append([]domain.DiagnosisTaskEvent{event}, r.events...)
	return event, nil
}
func (r *retryTestRepo) FindEventByTaskAndDedupeKey(context.Context, domain.DiagnosisTaskID, string) (domain.DiagnosisTaskEvent, error) {
	return domain.DiagnosisTaskEvent{}, errors.New("unexpected FindEventByTaskAndDedupeKey call")
}
func (r *retryTestRepo) ListEvents(context.Context, domain.DiagnosisTaskID, int) ([]domain.DiagnosisTaskEvent, error) {
	return nil, errors.New("unexpected ListEvents call")
}
func (r *retryTestRepo) ListEventsByTaskAndKind(_ context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error) {
	if taskID != r.task.ID {
		return nil, domain.ErrNotFound
	}
	out := make([]domain.DiagnosisTaskEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.Kind == kind {
			out = append(out, event)
		}
	}
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}
func (r *retryTestRepo) SaveChatSession(context.Context, domain.ChatSession) (domain.ChatSession, error) {
	return domain.ChatSession{}, errors.New("unexpected SaveChatSession call")
}
func (r *retryTestRepo) UpdateChatSession(context.Context, domain.ChatSession) (domain.ChatSession, error) {
	return domain.ChatSession{}, errors.New("unexpected UpdateChatSession call")
}
func (r *retryTestRepo) FindChatSessionByID(context.Context, domain.ChatSessionID) (domain.ChatSession, error) {
	return domain.ChatSession{}, errors.New("unexpected FindChatSessionByID call")
}
func (r *retryTestRepo) FindChatSessionByKey(_ context.Context, sessionKey string) (domain.ChatSession, error) {
	if r.session.SessionKey != sessionKey {
		return domain.ChatSession{}, domain.ErrNotFound
	}
	return r.session, nil
}
func (r *retryTestRepo) ListChatSessions(context.Context, int) ([]domain.ChatSessionWithTask, error) {
	return nil, errors.New("unexpected ListChatSessions call")
}
func (r *retryTestRepo) SaveChatTurn(context.Context, domain.ChatTurn) (domain.ChatTurn, error) {
	return domain.ChatTurn{}, errors.New("unexpected SaveChatTurn call")
}
func (r *retryTestRepo) FindChatTurnBySessionAndMessageID(_ context.Context, sessionID domain.ChatSessionID, messageID string) (domain.ChatTurn, error) {
	for _, turn := range r.turns {
		if turn.SessionID == sessionID && turn.MessageID == messageID {
			return turn, nil
		}
	}
	return domain.ChatTurn{}, domain.ErrNotFound
}
func (r *retryTestRepo) ListChatTurnsBySession(_ context.Context, sessionID domain.ChatSessionID, limit int) ([]domain.ChatTurn, error) {
	out := make([]domain.ChatTurn, 0, len(r.turns))
	for _, turn := range r.turns {
		if turn.SessionID == sessionID {
			out = append(out, turn)
		}
	}
	if limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}
func (r *retryTestRepo) failNow(message string) {
	r.t.Helper()
	r.t.Fatal(message)
}

func assertRetryBodyOrder(t *testing.T, body, first, second string) {
	t.Helper()
	firstIndex := strings.Index(body, first)
	if firstIndex < 0 {
		t.Fatalf("body = %q, missing %q", body, first)
	}
	secondIndex := strings.Index(body, second)
	if secondIndex < 0 {
		t.Fatalf("body = %q, missing %q", body, second)
	}
	if firstIndex >= secondIndex {
		t.Fatalf("body = %q, want %q before %q", body, first, second)
	}
}

type recordingResolver struct {
	consultationProvider ports.IMProvider
	closeProvider        ports.IMProvider
	consultationCalls    int
	closeCalls           int
}

func (r *recordingResolver) ResolveReportNotificationProvider(context.Context, domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return nil, errors.New("unexpected ResolveReportNotificationProvider call")
}

func (r *recordingResolver) ResolveDiagnosisConsultationNotificationProvider(context.Context, domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	r.consultationCalls++
	return r.consultationProvider, nil
}

func (r *recordingResolver) ResolveDiagnosisCloseNotificationProvider(context.Context, domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	r.closeCalls++
	return r.closeProvider, nil
}

type recordingIMProvider struct {
	called   int
	req      ports.IMNotification
	delivery ports.IMDelivery
	err      error
}

func (p *recordingIMProvider) SendNotification(_ context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	p.called++
	p.req = req
	if p.err != nil {
		return ports.IMDelivery{}, p.err
	}
	return p.delivery, nil
}
