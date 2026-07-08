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

func TestRunWritesSuccessfulSanitizedProof(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 0, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })
	t.Setenv("OPENCLARION_TEST_NOTIFICATION_BEARER_TOKEN", "test-token")

	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:         7,
			Kind:              api.Webhook,
			Status:            api.NotificationChannelTestStatusSuccess,
			ReasonCode:        api.NotificationChannelTestReasonCodeOk,
			Message:           "Notification channel test delivery succeeded.",
			ContentKind:       stringPtr("transport_sample"),
			ContentSha256:     stringPtr(strings.Repeat("a", 64)),
			CheckedAt:         now,
			ProviderMessageID: "msg-1",
			ProviderStatus:    "accepted",
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--bearer-token-env", "OPENCLARION_TEST_NOTIFICATION_BEARER_TOKEN",
		"--output", output,
		"--timeout", "5s",
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotPath != "/api/v1/config/notification-channels/7/test" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization header = %q", gotAuth)
	}

	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.CheckedAt != "2026-06-20T04:00:00Z" ||
		!out.Passed ||
		out.Request.ChannelID != 7 ||
		out.Request.APIBaseURL != server.URL ||
		out.HTTPStatus != http.StatusOK ||
		out.Evidence != "notification_channel_test success:transport_sample" ||
		out.Result.ProviderMessageID != "msg-1" {
		t.Fatalf("proof = %+v", out)
	}
	if strings.Contains(string(raw), "test-token") {
		t.Fatalf("proof leaked bearer token: %s", raw)
	}
	if strings.Contains(string(raw), "OPENCLARION_TEST_NOTIFICATION_BEARER_TOKEN") {
		t.Fatalf("proof leaked bearer env name: %s", raw)
	}
}

func TestRunEnforcesExpectedKind(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	var gotContentKindQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentKindQuery = r.URL.Query().Get("content_kind")
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr("ai_diagnosis_sample"),
			ContentSha256: stringPtr(strings.Repeat("b", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-kind", "WeCom",
		"--expected-content-kind", "ai_diagnosis_sample",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotContentKindQuery != "ai_diagnosis_sample" {
		t.Fatalf("content_kind query = %q, want ai_diagnosis_sample", gotContentKindQuery)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.ExpectedKind != "wecom" ||
		out.Request.ExpectedContentKind != "ai_diagnosis_sample" ||
		out.Result.Kind != api.Wecom ||
		out.Result.ContentKind == nil ||
		*out.Result.ContentKind != "ai_diagnosis_sample" {
		t.Fatalf("proof = %+v, want expected/result kind wecom", out)
	}
}

func TestRunAcceptsExpandedExpectedKinds(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 15, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	for _, kind := range []api.NotificationChannelKind{
		api.Dingtalk,
		api.Feishu,
		api.Slack,
		api.NotificationChannelEmail,
	} {
		t.Run(string(kind), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeTestResult(t, w, api.NotificationChannelTestResult{
					ChannelID:     7,
					Kind:          kind,
					Status:        api.NotificationChannelTestStatusSuccess,
					ReasonCode:    api.NotificationChannelTestReasonCodeOk,
					Message:       "Notification channel test delivery succeeded.",
					ContentKind:   stringPtr("transport_sample"),
					ContentSha256: stringPtr(strings.Repeat("a", 64)),
					CheckedAt:     now,
				})
			}))
			defer server.Close()

			output := filepath.Join(t.TempDir(), "proof.json")
			err := run([]string{
				"--api-base-url", server.URL,
				"--channel-id", "7",
				"--expected-kind", string(kind),
				"--output", output,
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
			if out.Request.ExpectedKind != string(kind) || out.Result.Kind != kind {
				t.Fatalf("proof = %+v, want kind %s", out, kind)
			}
		})
	}
}

func TestRunDefaultsExpectedKindForDiagnosisContent(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 30, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	var gotContentKindQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentKindQuery = r.URL.Query().Get("content_kind")
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr("diagnosis_close_sample"),
			ContentSha256: stringPtr(strings.Repeat("f", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-content-kind", "diagnosis_close_sample",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotContentKindQuery != "diagnosis_close_sample" {
		t.Fatalf("content_kind query = %q, want diagnosis_close_sample", gotContentKindQuery)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Request.ExpectedKind != "wecom" ||
		out.Request.ExpectedContentKind != "diagnosis_close_sample" ||
		out.Result.Kind != api.Wecom {
		t.Fatalf("proof = %+v, want default expected kind wecom", out)
	}
}

func TestRunExercisesMultipleExpectedContentKinds(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 45, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	var gotContentKindQueries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentKind := r.URL.Query().Get("content_kind")
		gotContentKindQueries = append(gotContentKindQueries, contentKind)
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr(contentKind),
			ContentSha256: stringPtr(strings.Repeat("a", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-content-kinds", "AI_Diagnosis_Sample, diagnosis_close_sample",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Join(gotContentKindQueries, ",") != "ai_diagnosis_sample,diagnosis_close_sample" {
		t.Fatalf("content_kind queries = %#v", gotContentKindQueries)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if !out.Passed ||
		out.Request.ExpectedKind != "wecom" ||
		strings.Join(out.Request.ExpectedContentKinds, ",") != "ai_diagnosis_sample,diagnosis_close_sample" ||
		out.Request.ExpectedContentKind != "" ||
		len(out.Results) != 2 ||
		out.Evidence != "notification_channel_test success:ai_diagnosis_sample,diagnosis_close_sample" {
		t.Fatalf("proof = %+v", out)
	}
	if out.Result.ContentKind == nil ||
		out.Results[0].ContentKind == nil ||
		*out.Result.ContentKind != *out.Results[0].ContentKind ||
		out.Result.ContentSha256 == nil ||
		out.Results[0].ContentSha256 == nil ||
		*out.Result.ContentSha256 != *out.Results[0].ContentSha256 {
		t.Fatalf("legacy result = %+v, want mirror of first result %+v", out.Result, out.Results[0])
	}
}

func TestRunRequireAIProofExercisesBothDiagnosisSamples(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 47, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	var gotContentKindQueries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentKind := r.URL.Query().Get("content_kind")
		gotContentKindQueries = append(gotContentKindQueries, contentKind)
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr(contentKind),
			ContentSha256: stringPtr(strings.Repeat("d", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--require-ai-proof",
		"--output", output,
	}, server.Client())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Join(gotContentKindQueries, ",") != "ai_diagnosis_sample,diagnosis_close_sample" {
		t.Fatalf("content_kind queries = %#v", gotContentKindQueries)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if !out.Passed ||
		!out.Request.RequireAIProof ||
		out.Request.ExpectedKind != "wecom" ||
		strings.Join(out.Request.ExpectedContentKinds, ",") != "ai_diagnosis_sample,diagnosis_close_sample" ||
		out.Evidence != "notification_channel_test success:ai_diagnosis_sample,diagnosis_close_sample" {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunWritesMultiContentProofAndReturnsFailureForPartialFailure(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 2, 50, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentKind := r.URL.Query().Get("content_kind")
		result := api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr(contentKind),
			ContentSha256: stringPtr(strings.Repeat("b", 64)),
			CheckedAt:     now,
		}
		if contentKind == "diagnosis_close_sample" {
			result.Status = api.NotificationChannelTestStatusFailed
			result.ReasonCode = api.NotificationChannelTestReasonCodeProviderError
			result.Message = "Notification channel provider returned an error."
			result.ContentSha256 = nil
		}
		writeTestResult(t, w, result)
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-content-kinds", "ai_diagnosis_sample,diagnosis_close_sample",
		"--output", output,
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want failure for partial content proof failure")
	}
	if !strings.Contains(err.Error(), "content_kind=diagnosis_close_sample status=failed") {
		t.Fatalf("error = %v", err)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Passed || len(out.Results) != 2 || out.Evidence != "notification_channel_test completed_without_success" {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunRejectsExpectedKindMismatch(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 3, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Webhook,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr("transport_sample"),
			ContentSha256: stringPtr(strings.Repeat("c", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-kind", "wecom",
		"--output", output,
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want expected kind mismatch")
	}
	if !strings.Contains(err.Error(), `result.kind = "webhook", want wecom`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsExpectedContentKindMismatch(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 4, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     7,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr("transport_sample"),
			ContentSha256: stringPtr(strings.Repeat("d", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-content-kind", "ai_diagnosis_sample",
		"--output", output,
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want expected content kind mismatch")
	}
	if !strings.Contains(err.Error(), `result.content_kind = "transport_sample", want ai_diagnosis_sample`) {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsChannelIDMismatch(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 4, 30, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:     8,
			Kind:          api.Wecom,
			Status:        api.NotificationChannelTestStatusSuccess,
			ReasonCode:    api.NotificationChannelTestReasonCodeOk,
			Message:       "Notification channel test delivery succeeded.",
			ContentKind:   stringPtr("ai_diagnosis_sample"),
			ContentSha256: stringPtr(strings.Repeat("a", 64)),
			CheckedAt:     now,
		})
	}))
	defer server.Close()

	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "7",
		"--expected-content-kind", "ai_diagnosis_sample",
		"--output", filepath.Join(t.TempDir(), "proof.json"),
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want channel id mismatch")
	}
	if !strings.Contains(err.Error(), "result.channel_id = 8, want 7") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunWritesBlockedProofAndReturnsFailure(t *testing.T) {
	now := time.Date(2026, 6, 20, 4, 5, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestResult(t, w, api.NotificationChannelTestResult{
			ChannelID:  8,
			Kind:       api.Webhook,
			Status:     api.NotificationChannelTestStatusBlocked,
			ReasonCode: api.NotificationChannelTestReasonCodeCredentialsUnavailable,
			Message:    "Secret reference is not available to the server-side resolver.",
			CheckedAt:  now,
		})
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "proof.json")
	err := run([]string{
		"--api-base-url", server.URL,
		"--channel-id", "8",
		"--output", output,
	}, server.Client())
	if err == nil {
		t.Fatal("run succeeded; want failure for non-success channel test")
	}
	if !strings.Contains(err.Error(), "status=blocked") {
		t.Fatalf("error = %v", err)
	}
	raw, err := os.ReadFile(output) // #nosec G304 -- test reads the proof path it created under t.TempDir.
	if err != nil {
		t.Fatalf("read proof: %v", err)
	}
	var out proof
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode proof: %v", err)
	}
	if out.Evidence != "notification_channel_test completed_without_success" ||
		out.Passed ||
		out.Result.Status != api.NotificationChannelTestStatusBlocked {
		t.Fatalf("proof = %+v", out)
	}
}

func TestRunRejectsUnsafeInputs(t *testing.T) {
	t.Setenv("OPENCLARION_TEST_NOTIFICATION_BEARER_TOKEN", "test-token")
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "userinfo_base_url",
			args: []string{"--api-base-url", "https://user@example.test", "--channel-id", "1", "--output", "proof.json"},
		},
		{
			name: "bad_channel",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "0", "--output", "proof.json"},
		},
		{
			name: "query_base_url",
			args: []string{"--api-base-url", "https://example.test?token=value", "--channel-id", "1", "--output", "proof.json"},
		},
		{
			name: "token_whitespace",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--bearer-token", "bad token"},
		},
		{
			name: "bad_bearer_token_env_name",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--bearer-token-env", "OPENCLARION-TOKEN"},
		},
		{
			name: "missing_bearer_token_env",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--bearer-token-env", "OPENCLARION_TEST_MISSING_NOTIFICATION_BEARER"},
		},
		{
			name: "duplicate_bearer_token_source",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--bearer-token", "test-token", "--bearer-token-env", "OPENCLARION_TEST_NOTIFICATION_BEARER_TOKEN"},
		},
		{
			name: "bad_expected_kind",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--expected-kind", "pager"},
		},
		{
			name: "bad_expected_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--expected-content-kind", "pager"},
		},
		{
			name: "generic_webhook_diagnosis_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--expected-kind", "webhook", "--expected-content-kind", "ai_diagnosis_sample"},
		},
		{
			name: "require_ai_proof_with_explicit_content_kind",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--require-ai-proof", "--expected-content-kind", "ai_diagnosis_sample"},
		},
		{
			name: "require_ai_proof_with_generic_webhook_kind",
			args: []string{"--api-base-url", "https://example.test", "--channel-id", "1", "--output", "proof.json", "--require-ai-proof", "--expected-kind", "webhook"},
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
	now := time.Date(2026, 6, 20, 4, 10, 0, 0, time.UTC)
	previousNow := nowUTC
	nowUTC = func() time.Time { return now }
	t.Cleanup(func() { nowUTC = previousNow })

	valid := proof{
		Passed:     true,
		CheckedAt:  now.Format(time.RFC3339Nano),
		HTTPStatus: http.StatusOK,
		Request: proofRequest{
			APIBaseURL: "https://openclarion.example.test",
			ChannelID:  1,
			Timeout:    "15s",
		},
		Result: api.NotificationChannelTestResult{
			ChannelID:         1,
			Kind:              api.Webhook,
			Status:            api.NotificationChannelTestStatusSuccess,
			ReasonCode:        api.NotificationChannelTestReasonCodeOk,
			Message:           "Notification channel test delivery succeeded.",
			ContentKind:       stringPtr("transport_sample"),
			ContentSha256:     stringPtr(strings.Repeat("e", 64)),
			CheckedAt:         now,
			ProviderMessageID: "msg-1",
			ProviderStatus:    "accepted",
		},
		Evidence: "notification_channel_test success:transport_sample",
	}
	if err := validateProof(valid, true); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}

	bad := valid
	bad.Result.ProviderMessageID = "msg\n1"
	if err := validateProof(bad, true); err == nil {
		t.Fatal("provider message with newline accepted")
	}

	bad = valid
	bad.Result.ContentSha256 = stringPtr(strings.Repeat("A", 64))
	bad.Evidence = notificationChannelSingleResultEvidence(bad.Result)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("uppercase content digest accepted")
	}

	bad = valid
	bad.Result.ContentKind = nil
	bad.Evidence = notificationChannelSingleResultEvidence(bad.Result)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("success without content kind accepted")
	}

	bad = valid
	bad.Passed = false
	if err := validateProof(bad, true); err == nil {
		t.Fatal("successful result with passed=false accepted")
	}

	bad = valid
	bad.Result.Status = api.NotificationChannelTestStatusFailed
	bad.Result.ReasonCode = api.NotificationChannelTestReasonCodeProviderError
	bad.Passed = false
	bad.Evidence = notificationChannelSingleResultEvidence(bad.Result)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("non-success accepted when success is required")
	}

	if err := validateProof(bad, false); err != nil {
		t.Fatalf("non-success proof rejected when success is not required: %v", err)
	}

	bad.Passed = true
	if err := validateProof(bad, false); err == nil {
		t.Fatal("non-success result with passed=true accepted")
	}

	bad = valid
	bad.Result.ContentKind = stringPtr("ai_diagnosis_sample")
	bad.Evidence = notificationChannelSingleResultEvidence(bad.Result)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("diagnosis notification content on generic webhook accepted")
	}

	multi := valid
	multi.Request.ExpectedKind = "wecom"
	multi.Request.ExpectedContentKind = ""
	multi.Request.ExpectedContentKinds = []string{"ai_diagnosis_sample", "diagnosis_close_sample"}
	multi.Result.Kind = api.Wecom
	multi.Result.ContentKind = stringPtr("ai_diagnosis_sample")
	multi.Results = []api.NotificationChannelTestResult{
		multi.Result,
		multi.Result,
	}
	multi.Results[1].ContentKind = stringPtr("diagnosis_close_sample")
	multi.Evidence = notificationChannelEvidence(multi.Results)
	if err := validateProof(multi, true); err != nil {
		t.Fatalf("valid multi-result proof rejected: %v", err)
	}

	multi.Request.RequireAIProof = true
	if err := validateProof(multi, true); err != nil {
		t.Fatalf("valid AI proof mode proof rejected: %v", err)
	}

	bad = multi
	bad.Result = bad.Results[1]
	if err := validateProof(bad, true); err == nil {
		t.Fatal("multi-result proof with divergent legacy result accepted")
	}

	bad = multi
	bad.Results[1].ChannelID = 2
	bad.Evidence = notificationChannelEvidence(bad.Results)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("multi-result proof with mismatched channel id accepted")
	}

	bad = multi
	bad.Request.ExpectedContentKinds = []string{"ai_diagnosis_sample"}
	bad.Results = bad.Results[:1]
	bad.Evidence = notificationChannelEvidence(bad.Results)
	if err := validateProof(bad, true); err == nil {
		t.Fatal("AI proof mode without both diagnosis samples accepted")
	}
}

func stringPtr(value string) *string {
	return &value
}

func writeTestResult(t *testing.T, w http.ResponseWriter, result api.NotificationChannelTestResult) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func unusedHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be used for rejected inputs")
		return nil, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
