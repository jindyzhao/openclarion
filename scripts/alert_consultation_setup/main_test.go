package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

func TestRunCreatesWeComAutoRoomConfiguration(t *testing.T) {
	now := time.Date(2026, 6, 22, 2, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	secretRef := "secret/openclarion/ops-wecom-live"
	alertmanagerURL := "https://alertmanager.example.test/alerts"
	token := "api-token"
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path]++
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("authorization header = %q", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /openclarion/api/v1/config/alert-sources":
			var req api.AlertSourceProfileWriteRequest
			decodeJSON(t, r, &req)
			if req.Kind != api.Alertmanager ||
				req.AuthMode != api.AlertSourceAuthModeNone ||
				req.BaseURL != alertmanagerURL ||
				req.SecretRef != nil ||
				req.Enabled == nil ||
				!*req.Enabled {
				t.Fatalf("alert source request = %+v", req)
			}
			writeJSON(t, w, http.StatusCreated, api.AlertSourceProfile{
				ID:        11,
				Name:      req.Name,
				Kind:      req.Kind,
				BaseURL:   req.BaseURL,
				AuthMode:  req.AuthMode,
				SecretRef: "",
				Enabled:   true,
				Labels:    derefAlertLabels(req.Labels),
				CreatedAt: now,
				UpdatedAt: now,
			})
		case "POST /openclarion/api/v1/config/notification-channels":
			var req api.NotificationChannelProfileWriteRequest
			decodeJSON(t, r, &req)
			if req.Kind != api.Wecom ||
				req.SecretRef != secretRef ||
				req.Enabled == nil ||
				!*req.Enabled ||
				!hasScope(req.DeliveryScopes, api.Report) ||
				!hasScope(req.DeliveryScopes, api.DiagnosisConsultation) ||
				!hasScope(req.DeliveryScopes, api.DiagnosisClose) {
				t.Fatalf("notification channel request = %+v", req)
			}
			writeJSON(t, w, http.StatusCreated, api.NotificationChannelProfile{
				ID:             22,
				Name:           req.Name,
				Kind:           req.Kind,
				SecretRef:      req.SecretRef,
				DeliveryScopes: req.DeliveryScopes,
				Enabled:        true,
				Labels:         derefNotificationLabels(req.Labels),
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		case "POST /openclarion/api/v1/config/notification-channels/22/test":
			contentKind := r.URL.Query().Get("content_kind")
			switch contentKind {
			case string(api.NotificationChannelTestResultContentKindAiDiagnosisSample),
				string(api.NotificationChannelTestResultContentKindDiagnosisCloseSample):
			default:
				t.Fatalf("notification test content_kind = %q", contentKind)
			}
			writeJSON(t, w, http.StatusOK, notificationChannelTestSuccessResponse(now, 22, contentKind, "delivered"))
		case "POST /openclarion/api/v1/config/grouping-policies":
			var req api.GroupingPolicyWriteRequest
			decodeJSON(t, r, &req)
			if req.Name != defaultGroupingPolicyName ||
				req.SeverityKey != "severity" ||
				req.Enabled == nil ||
				!*req.Enabled ||
				!slices.Contains(req.DimensionKeys, "alertname") ||
				!slices.Contains(req.SourceFilter, "alertmanager") {
				t.Fatalf("grouping policy request = %+v", req)
			}
			writeJSON(t, w, http.StatusCreated, api.GroupingPolicy{
				ID:            33,
				Name:          req.Name,
				DimensionKeys: req.DimensionKeys,
				SeverityKey:   req.SeverityKey,
				SourceFilter:  req.SourceFilter,
				Enabled:       true,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		case "POST /openclarion/api/v1/config/report-workflow-policies":
			var req api.ReportWorkflowPolicyWriteRequest
			decodeJSON(t, r, &req)
			channelID, err := req.ReportNotificationChannelProfileID.Get()
			if err != nil {
				t.Fatalf("notification channel id missing: %v", err)
			}
			if req.AlertSourceProfileID != 11 ||
				req.GroupingPolicyID != 33 ||
				channelID != 22 ||
				req.TriggerMode == nil ||
				*req.TriggerMode != string(api.ReportWorkflowTriggerModeManualReplay) ||
				req.ReportScenario == nil ||
				*req.ReportScenario != string(api.ReportWorkflowScenarioCascade) ||
				req.DiagnosisFollowUp == nil ||
				*req.DiagnosisFollowUp != string(api.DiagnosisFollowUpModeAutoRoom) {
				t.Fatalf("report workflow policy request = %+v", req)
			}
			writeJSON(t, w, http.StatusCreated, reportWorkflowPolicyResponse(now, req, 44))
		case "POST /openclarion/api/v1/config/report-workflow-policies/44/enable":
			writeJSON(t, w, http.StatusOK, api.ReportWorkflowPolicy{
				ID:                                 44,
				Name:                               defaultReportWorkflowName,
				AlertSourceProfileID:               11,
				GroupingPolicyID:                   33,
				ReportNotificationChannelProfileID: nullableInt64(22),
				TriggerMode:                        api.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     api.ReportWorkflowScenarioCascade,
				DiagnosisFollowUp:                  api.DiagnosisFollowUpModeAutoRoom,
				Enabled:                            true,
				CreatedAt:                          now,
				UpdatedAt:                          now,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	envOutput := filepath.Join(t.TempDir(), "ids.env")
	t.Setenv("OPENCLARION_TEST_ALERT_CONSULTATION_BEARER_TOKEN", "Bearer "+token)
	err := run([]string{
		"--api-base-url", server.URL + "/openclarion",
		"--alertmanager-base-url", alertmanagerURL,
		"--notification-secret-ref", secretRef,
		"--bearer-token-env", "OPENCLARION_TEST_ALERT_CONSULTATION_BEARER_TOKEN",
		"--output", output,
		"--env-output", envOutput,
		"--timeout", "5s",
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := seen["POST /openclarion/api/v1/config/report-workflow-policies/44/enable"]; got != 1 {
		t.Fatalf("enable calls = %d, want 1", got)
	}
	if got := seen["POST /openclarion/api/v1/config/notification-channels/22/test"]; got != 2 {
		t.Fatalf("notification test calls = %d, want 2", got)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.CheckedAt != "2026-06-22T02:00:00Z" ||
		out.Result.AlertSourceProfileID != 11 ||
		out.Result.NotificationChannelID != 22 ||
		out.Result.GroupingPolicyID != 33 ||
		out.Result.ReportWorkflowPolicyID != 44 ||
		!out.Result.ReportWorkflowEnabled ||
		out.Result.NotificationKind != "wecom" ||
		out.Result.DiagnosisFollowUp != "auto_room" ||
		!out.Request.NotificationAIProofRequired ||
		len(out.Result.NotificationAIProofs) != 2 ||
		out.Evidence != proofEvidence {
		t.Fatalf("proof = %+v", out)
	}
	if out.Result.NotificationAIProofs[0].ContentKind != string(api.NotificationChannelTestResultContentKindAiDiagnosisSample) ||
		out.Result.NotificationAIProofs[1].ContentKind != string(api.NotificationChannelTestResultContentKindDiagnosisCloseSample) {
		t.Fatalf("notification AI proofs = %+v", out.Result.NotificationAIProofs)
	}
	for _, item := range out.Result.NotificationAIProofs {
		if item.Status != string(api.NotificationChannelTestStatusSuccess) ||
			item.ReasonCode != string(api.NotificationChannelTestReasonCodeOk) ||
			item.ProviderStatus != "delivered" ||
			!item.ContentSHA256Present ||
			item.CheckedAt != "2026-06-22T02:00:00Z" {
			t.Fatalf("notification AI proof = %+v", item)
		}
	}
	assertNotContains(t, string(raw), token, alertmanagerURL, secretRef, "provider-message-id-not-written-to-setup-proof")

	envRaw, err := os.ReadFile(envOutput) // #nosec G304 -- test reads the env path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	envText := string(envRaw)
	for _, want := range []string{
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID='11'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID='22'",
		"OPENCLARION_LIVE_GROUPING_POLICY_ID='33'",
		"OPENCLARION_LIVE_REPORT_WORKFLOW_POLICY_ID='44'",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID='22'",
		"NOTIFICATION_CHANNEL_EXPECTED_KIND='wecom'",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF='true'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF='true'",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND='final_conclusion'",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf("env output missing %q:\n%s", want, envText)
		}
	}
	for _, unwanted := range []string{
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND='ai_diagnosis_sample'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND='ai_diagnosis_sample'",
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS='ai_diagnosis_sample,diagnosis_close_sample'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS='ai_diagnosis_sample,diagnosis_close_sample'",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND='assistant_message'",
	} {
		if strings.Contains(envText, unwanted) {
			t.Fatalf("env output contains obsolete single content kind %q:\n%s", unwanted, envText)
		}
	}
	assertNotContains(t, envText, token, alertmanagerURL, secretRef)
}

func TestRunReplacesExistingGroupingPolicyOnCreateConflict(t *testing.T) {
	now := time.Date(2026, 6, 22, 2, 30, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path]++
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/config/alert-sources":
			var req api.AlertSourceProfileWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, api.AlertSourceProfile{
				ID:        11,
				Name:      req.Name,
				Kind:      req.Kind,
				BaseURL:   req.BaseURL,
				AuthMode:  req.AuthMode,
				Enabled:   true,
				CreatedAt: now,
				UpdatedAt: now,
			})
		case "POST /api/v1/config/notification-channels":
			var req api.NotificationChannelProfileWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, api.NotificationChannelProfile{
				ID:             22,
				Name:           req.Name,
				Kind:           api.Wecom,
				SecretRef:      req.SecretRef,
				DeliveryScopes: req.DeliveryScopes,
				Enabled:        true,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		case "POST /api/v1/config/grouping-policies":
			writeJSON(t, w, http.StatusConflict, api.ErrorResponse{Error: "grouping policy already exists"})
		case "GET /api/v1/config/grouping-policies":
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Fatalf("grouping list limit = %q, want 500", got)
			}
			writeJSON(t, w, http.StatusOK, api.GroupingPolicyListResponse{
				Items: []api.GroupingPolicy{{
					ID:            33,
					Name:          defaultGroupingPolicyName,
					DimensionKeys: []string{"old"},
					SeverityKey:   "severity",
					SourceFilter:  []string{"alertmanager"},
					Enabled:       false,
					CreatedAt:     now.Add(-time.Hour),
					UpdatedAt:     now.Add(-time.Hour),
				}},
			})
		case "PUT /api/v1/config/grouping-policies/33":
			var req api.GroupingPolicyWriteRequest
			decodeJSON(t, r, &req)
			if !slices.Contains(req.DimensionKeys, "namespace") ||
				req.Enabled == nil ||
				!*req.Enabled {
				t.Fatalf("grouping replacement request = %+v", req)
			}
			writeJSON(t, w, http.StatusOK, api.GroupingPolicy{
				ID:            33,
				Name:          req.Name,
				DimensionKeys: req.DimensionKeys,
				SeverityKey:   req.SeverityKey,
				SourceFilter:  req.SourceFilter,
				Enabled:       true,
				CreatedAt:     now.Add(-time.Hour),
				UpdatedAt:     now,
			})
		case "POST /api/v1/config/report-workflow-policies":
			var req api.ReportWorkflowPolicyWriteRequest
			decodeJSON(t, r, &req)
			if req.GroupingPolicyID != 33 {
				t.Fatalf("report workflow grouping id = %d, want 33", req.GroupingPolicyID)
			}
			writeJSON(t, w, http.StatusCreated, reportWorkflowPolicyResponse(now, req, 44))
		case "POST /api/v1/config/notification-channels/22/test":
			contentKind := r.URL.Query().Get("content_kind")
			writeJSON(t, w, http.StatusOK, notificationChannelTestSuccessResponse(now, 22, contentKind, "delivered"))
		case "POST /api/v1/config/report-workflow-policies/44/enable":
			writeJSON(t, w, http.StatusOK, api.ReportWorkflowPolicy{
				ID:                                 44,
				Name:                               defaultReportWorkflowName,
				AlertSourceProfileID:               11,
				GroupingPolicyID:                   33,
				ReportNotificationChannelProfileID: nullableInt64(22),
				TriggerMode:                        api.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     api.ReportWorkflowScenarioCascade,
				DiagnosisFollowUp:                  api.DiagnosisFollowUpModeAutoRoom,
				Enabled:                            true,
				CreatedAt:                          now,
				UpdatedAt:                          now,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--alertmanager-base-url", "https://alertmanager.example.test/api/v2",
		"--notification-secret-ref", "secret/openclarion/ops-wecom",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := seen["GET /api/v1/config/grouping-policies"]; got != 1 {
		t.Fatalf("grouping list calls = %d, want 1", got)
	}
	if got := seen["PUT /api/v1/config/grouping-policies/33"]; got != 1 {
		t.Fatalf("grouping replace calls = %d, want 1", got)
	}
}

func TestRunReplacesExistingConfigurationWithoutEnable(t *testing.T) {
	now := time.Date(2026, 6, 22, 3, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	sourceSecret := "secret/openclarion/am-bearer"
	seenEnable := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/config/report-workflow-policies/404/enable" {
			seenEnable = true
		}
		switch r.Method + " " + r.URL.Path {
		case "PUT /api/v1/config/alert-sources/101":
			var req api.AlertSourceProfileWriteRequest
			decodeJSON(t, r, &req)
			if req.AuthMode != api.AlertSourceAuthModeBearer ||
				req.SecretRef == nil ||
				*req.SecretRef != sourceSecret {
				t.Fatalf("alert source request = %+v", req)
			}
			writeJSON(t, w, http.StatusOK, api.AlertSourceProfile{
				ID:        101,
				Name:      req.Name,
				Kind:      api.Alertmanager,
				BaseURL:   req.BaseURL,
				AuthMode:  req.AuthMode,
				SecretRef: sourceSecret,
				Enabled:   true,
				CreatedAt: now,
				UpdatedAt: now,
			})
		case "PUT /api/v1/config/notification-channels/202":
			var req api.NotificationChannelProfileWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusOK, api.NotificationChannelProfile{
				ID:             202,
				Name:           req.Name,
				Kind:           api.Wecom,
				SecretRef:      req.SecretRef,
				DeliveryScopes: req.DeliveryScopes,
				Enabled:        true,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		case "PUT /api/v1/config/grouping-policies/303":
			var req api.GroupingPolicyWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusOK, api.GroupingPolicy{
				ID:            303,
				Name:          req.Name,
				DimensionKeys: req.DimensionKeys,
				SeverityKey:   req.SeverityKey,
				SourceFilter:  req.SourceFilter,
				Enabled:       true,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		case "PUT /api/v1/config/report-workflow-policies/404":
			var req api.ReportWorkflowPolicyWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusOK, reportWorkflowPolicyResponse(now, req, 404))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--alertmanager-base-url", "https://alertmanager.example.test/api/v2",
		"--alertmanager-auth-mode", "bearer",
		"--alertmanager-secret-ref", sourceSecret,
		"--notification-secret-ref", "secret/openclarion/ops-wecom",
		"--source-id", "101",
		"--channel-id", "202",
		"--grouping-policy-id", "303",
		"--report-workflow-policy-id", "404",
		"--enable-policy=false",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if seenEnable {
		t.Fatal("enable endpoint was called")
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.SourceID != 101 ||
		out.Request.ChannelID != 202 ||
		out.Request.GroupingPolicyID != 303 ||
		out.Request.ReportWorkflowPolicyID != 404 ||
		out.Request.AlertmanagerAuthMode != "bearer" ||
		out.Request.NotificationAIProofRequired ||
		len(out.Result.NotificationAIProofs) != 0 ||
		out.Result.ReportWorkflowEnabled {
		t.Fatalf("proof = %+v", out)
	}
	assertNotContains(t, string(raw), sourceSecret, "alertmanager.example.test")
}

func TestRunDoesNotEnablePolicyWhenNotificationAIProofFails(t *testing.T) {
	now := time.Date(2026, 6, 22, 4, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	enableCalled := false
	workflowCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/config/report-workflow-policies/44/enable" {
			enableCalled = true
		}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/config/alert-sources":
			var req api.AlertSourceProfileWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, api.AlertSourceProfile{
				ID:        11,
				Name:      req.Name,
				Kind:      req.Kind,
				BaseURL:   req.BaseURL,
				AuthMode:  req.AuthMode,
				Enabled:   true,
				CreatedAt: now,
				UpdatedAt: now,
			})
		case "POST /api/v1/config/notification-channels":
			var req api.NotificationChannelProfileWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, api.NotificationChannelProfile{
				ID:             22,
				Name:           req.Name,
				Kind:           api.Wecom,
				SecretRef:      req.SecretRef,
				DeliveryScopes: req.DeliveryScopes,
				Enabled:        true,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		case "POST /api/v1/config/grouping-policies":
			var req api.GroupingPolicyWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, api.GroupingPolicy{
				ID:            33,
				Name:          req.Name,
				DimensionKeys: req.DimensionKeys,
				SeverityKey:   req.SeverityKey,
				SourceFilter:  req.SourceFilter,
				Enabled:       true,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		case "POST /api/v1/config/report-workflow-policies":
			workflowCalled = true
			var req api.ReportWorkflowPolicyWriteRequest
			decodeJSON(t, r, &req)
			writeJSON(t, w, http.StatusCreated, reportWorkflowPolicyResponse(now, req, 44))
		case "POST /api/v1/config/notification-channels/22/test":
			contentKind := r.URL.Query().Get("content_kind")
			if contentKind != string(api.NotificationChannelTestResultContentKindAiDiagnosisSample) {
				t.Fatalf("notification test content_kind = %q", contentKind)
			}
			writeJSON(t, w, http.StatusOK, api.NotificationChannelTestResult{
				ChannelID:      22,
				Kind:           api.Wecom,
				Status:         api.NotificationChannelTestStatusFailed,
				ReasonCode:     api.NotificationChannelTestReasonCodeProviderError,
				Message:        "Notification provider rejected the test delivery.",
				ContentKind:    &contentKind,
				CheckedAt:      now,
				ProviderStatus: "failed",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--alertmanager-base-url", "https://alertmanager.example.test/api/v2",
		"--notification-secret-ref", "secret/openclarion/ops-wecom",
		"--output", output,
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded")
	}
	if !strings.Contains(err.Error(), "ai_diagnosis_sample") ||
		!strings.Contains(err.Error(), "result.status") {
		t.Fatalf("error = %q", err.Error())
	}
	if enableCalled {
		t.Fatal("enable endpoint was called after notification proof failed")
	}
	if workflowCalled {
		t.Fatal("report workflow policy was written after notification proof failed")
	}
}

func TestParseArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing alertmanager URL",
			args: []string{"--api-base-url", "https://api.example.test", "--output", "proof.json"},
			want: "--alertmanager-base-url is required",
		},
		{
			name: "webhook URL as notification secret ref",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--notification-secret-ref", "https://example.invalid/webhook?token=secret", "--output", "proof.json"},
			want: "--notification-secret-ref must be a secret reference",
		},
		{
			name: "bearer without secret ref",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--alertmanager-auth-mode", "bearer", "--output", "proof.json"},
			want: "--alertmanager-secret-ref is required",
		},
		{
			name: "secret ref without bearer",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--alertmanager-secret-ref", "secret/ref", "--output", "proof.json"},
			want: "--alertmanager-secret-ref requires",
		},
		{
			name: "userinfo API URL",
			args: []string{"--api-base-url", "https://user@api.example.test", "--alertmanager-base-url", "https://am.example.test", "--output", "proof.json"},
			want: "--api-base-url must not contain userinfo",
		},
		{
			name: "bad token",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--bearer-token", "bad token", "--output", "proof.json"},
			want: "--bearer-token must be a single token",
		},
		{
			name: "bad token env name",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--bearer-token-env", "OPENCLARION-TOKEN", "--output", "proof.json"},
			want: "--bearer-token-env must be a valid environment variable name",
		},
		{
			name: "missing token env",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--bearer-token-env", "OPENCLARION_TEST_MISSING_ALERT_CONSULTATION_TOKEN", "--output", "proof.json"},
			want: "--bearer-token-env references an unset environment variable",
		},
		{
			name: "direct token and token env",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--bearer-token", "test-token", "--bearer-token-env", "OPENCLARION_TEST_ALERT_CONSULTATION_BEARER_TOKEN", "--output", "proof.json"},
			want: "--bearer-token and --bearer-token-env are mutually exclusive",
		},
		{
			name: "negative id",
			args: []string{"--api-base-url", "https://api.example.test", "--alertmanager-base-url", "https://am.example.test", "--source-id", "-1", "--output", "proof.json"},
			want: "existing IDs must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.args)
			if err == nil {
				t.Fatal("parseArgs succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func decodeJSON(t *testing.T, r *http.Request, out any) {
	t.Helper()
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode request: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func notificationChannelTestSuccessResponse(
	now time.Time,
	channelID int64,
	contentKind string,
	providerStatus string,
) api.NotificationChannelTestResult {
	contentSHA256 := strings.Repeat("a", 64)
	return api.NotificationChannelTestResult{
		ChannelID:         channelID,
		Kind:              api.Wecom,
		Status:            api.NotificationChannelTestStatusSuccess,
		ReasonCode:        api.NotificationChannelTestReasonCodeOk,
		Message:           "Notification channel test delivery succeeded.",
		ContentKind:       &contentKind,
		ContentSha256:     &contentSHA256,
		CheckedAt:         now,
		ProviderMessageID: "provider-message-id-not-written-to-setup-proof",
		ProviderStatus:    providerStatus,
	}
}

func reportWorkflowPolicyResponse(now time.Time, req api.ReportWorkflowPolicyWriteRequest, id int64) api.ReportWorkflowPolicy {
	triggerMode := api.ReportWorkflowTriggerModeManualReplay
	if req.TriggerMode != nil {
		triggerMode = api.ReportWorkflowTriggerMode(*req.TriggerMode)
	}
	scenario := api.ReportWorkflowScenarioCascade
	if req.ReportScenario != nil {
		scenario = api.ReportWorkflowScenario(*req.ReportScenario)
	}
	followUp := api.DiagnosisFollowUpModeAutoRoom
	if req.DiagnosisFollowUp != nil {
		followUp = api.DiagnosisFollowUpMode(*req.DiagnosisFollowUp)
	}
	return api.ReportWorkflowPolicy{
		ID:                                 id,
		Name:                               req.Name,
		AlertSourceProfileID:               req.AlertSourceProfileID,
		GroupingPolicyID:                   req.GroupingPolicyID,
		ReportNotificationChannelProfileID: req.ReportNotificationChannelProfileID,
		TriggerMode:                        triggerMode,
		ReportScenario:                     scenario,
		DiagnosisFollowUp:                  followUp,
		Enabled:                            false,
		CreatedAt:                          now,
		UpdatedAt:                          now,
	}
}

func nullableInt64(value int64) api.Nullable[int64] {
	out := api.Nullable[int64]{}
	out.Set(value)
	return out
}

func derefAlertLabels(labels *api.AlertSourceLabels) api.AlertSourceLabels {
	if labels == nil {
		return api.AlertSourceLabels{}
	}
	return *labels
}

func derefNotificationLabels(labels *api.NotificationChannelLabels) api.NotificationChannelLabels {
	if labels == nil {
		return api.NotificationChannelLabels{}
	}
	return *labels
}

func hasScope(scopes []api.NotificationDeliveryScope, want api.NotificationDeliveryScope) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func assertNotContains(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if value == "" {
			continue
		}
		if strings.Contains(text, value) {
			t.Fatalf("text leaked %q:\n%s", value, text)
		}
	}
}
