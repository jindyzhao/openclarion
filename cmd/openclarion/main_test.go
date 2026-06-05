package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

func TestReportActivityOptionsFromEnv_ConfiguresProviders(t *testing.T) {
	var gotAuth string
	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("LLM path = %q, want /v1/chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
	}))
	defer llmServer.Close()

	// #nosec G101 -- test-only env fixture uses non-secret placeholder values.
	env := map[string]string{
		"OPENCLARION_LLM_MODEL":               "gpt-test",
		"OPENCLARION_LLM_BASE_URL":            llmServer.URL + "/v1",
		"OPENCLARION_LLM_API_KEY":             "test-api-value",
		"OPENCLARION_IM_WEBHOOK_URL":          "https://example.invalid/report-hook",
		"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN": "webhook-bearer-value",
	}
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(env), nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}
	if gotAuth != "Bearer test-api-value" {
		t.Fatalf("Authorization = %q, want Bearer test-api-value", gotAuth)
	}
}

func TestReportActivityOptionsFromEnv_AllowsUnconfiguredProviders(t *testing.T) {
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(nil), nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("len(opts) = %d, want 0", len(opts))
	}
}

func TestTemporalTaskQueueFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		want       string
		wantSubstr string
	}{
		{
			name: "default",
			want: temporalpkg.TaskQueue,
		},
		{
			name: "custom",
			env: map[string]string{
				"OPENCLARION_TEMPORAL_TASK_QUEUE": "openclarion-local-rehearsal",
			},
			want: "openclarion-local-rehearsal",
		},
		{
			name: "leading whitespace",
			env: map[string]string{
				"OPENCLARION_TEMPORAL_TASK_QUEUE": " openclarion-local-rehearsal",
			},
			wantSubstr: "leading or trailing whitespace",
		},
		{
			name: "tab",
			env: map[string]string{
				"OPENCLARION_TEMPORAL_TASK_QUEUE": "openclarion\tlocal",
			},
			wantSubstr: "control whitespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := temporalTaskQueueFromEnv(mapGetenv(tc.env))
			if tc.wantSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("temporalTaskQueueFromEnv: %v", err)
			}
			if got != tc.want {
				t.Fatalf("task queue = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReportActivityOptionsFromEnv_RejectsPartialConfig(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantSubstr string
	}{
		{
			name: "llm base without model",
			env: map[string]string{
				"OPENCLARION_LLM_BASE_URL": "https://example.invalid/v1",
			},
			wantSubstr: "OPENCLARION_LLM_MODEL",
		},
		{
			name: "webhook token without url",
			// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
			env: map[string]string{
				"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN": "test-bearer-value",
			},
			wantSubstr: "OPENCLARION_IM_WEBHOOK_URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(tc.env), nil)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresReportTrigger(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_PROMETHEUS_URL":          "http://prometheus.example",
		"OPENCLARION_PROMETHEUS_BEARER_TOKEN": "test-bearer-value",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_AllowsUnconfiguredTrigger(t *testing.T) {
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(nil), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("len(opts) = %d, want 0", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_RejectsPartialConfig(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_PROMETHEUS_BEARER_TOKEN": "test-bearer-value",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected OPENCLARION_PROMETHEUS_URL error, got nil")
	}
	if !strings.Contains(err.Error(), "OPENCLARION_PROMETHEUS_URL") {
		t.Fatalf("error = %q, want OPENCLARION_PROMETHEUS_URL", err.Error())
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresDiagnosisRoom(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	opts, originPolicy, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": " " + oidcServer.URL + " ",
		"OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID":  "openclarion-web",
		"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS": "http://127.0.0.1:32101",
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv diagnosis: %v", err)
	}
	if len(opts) != 4 {
		t.Fatalf("len(opts) = %d, want 4", len(opts))
	}
	if originPolicy == nil {
		t.Fatal("originPolicy is nil")
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:8080/ws/diagnosis", nil)
	req.Header.Set("Origin", "http://127.0.0.1:32101")
	if !originPolicy.CheckWebSocketOrigin(req) {
		t.Fatal("expected configured origin to be allowed")
	}
	req.Header.Set("Origin", "http://127.0.0.1:9999")
	if originPolicy.CheckWebSocketOrigin(req) {
		t.Fatal("expected unconfigured origin to be rejected")
	}
}

func TestHTTPServerOptionsFromEnv_RejectsCredentialedDiagnosisAllowedOrigin(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	rawOriginMarker := "raw-marker"
	tests := []struct {
		name       string
		origin     string
		wantDetail string
		wantNot    string
	}{
		{name: "username", origin: "https://operator@example.test", wantDetail: "userinfo", wantNot: "operator@example.test"},
		{name: "password", origin: credentialedDiagnosisOrigin(), wantDetail: "userinfo", wantNot: "opaque"},
		{name: "escaped userinfo", origin: "https://operator%40team@example.test", wantDetail: "userinfo", wantNot: "operator%40team"},
		{name: "malformed credentialed origin does not leak raw input", origin: "https://operator:%" + rawOriginMarker + "@example.test", wantDetail: "parse origin", wantNot: rawOriginMarker},
		{name: "credentialed unsupported scheme does not leak raw input", origin: "ftp://operator:" + rawOriginMarker + "@example.test", wantDetail: "userinfo", wantNot: rawOriginMarker},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
				"OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL": " " + oidcServer.URL + " ",
				"OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID":  "openclarion-web",
				"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS": tc.origin,
			}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil)
			if err == nil {
				t.Fatal("expected credentialed allowed origin error, got nil")
			}
			if !strings.Contains(err.Error(), "OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS") || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("error = %q, want allowed origins %q rejection", err.Error(), tc.wantDetail)
			}
			if tc.wantNot != "" && strings.Contains(err.Error(), tc.wantNot) {
				t.Fatalf("error = %q, must not contain %q", err.Error(), tc.wantNot)
			}
		})
	}
}

func credentialedDiagnosisOrigin() string {
	return (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "opaque"),
		Host:   "example.test",
	}).String()
}

func TestHTTPServerOptionsFromEnv_RejectsIncompleteDiagnosisConfig(t *testing.T) {
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID": "openclarion-web",
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil)
	if err == nil {
		t.Fatal("expected diagnosis OIDC issuer error, got nil")
	}
	if !strings.Contains(err.Error(), "issuer url") {
		t.Fatalf("error = %q, want issuer url", err.Error())
	}
}

func TestDiagnosisActivityOptionsFromEnv_ConfiguresDockerProvider(t *testing.T) {
	opts, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": t.TempDir(),
		"OPENCLARION_SANDBOX_COMMAND_JSON":      `["/runner"]`,
	}))
	if err != nil {
		t.Fatalf("diagnosisActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestDiagnosisActivityOptionsFromEnv_RejectsPartialConfig(t *testing.T) {
	_, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_COMMAND_JSON": `["/runner"]`,
	}))
	if err == nil {
		t.Fatal("expected sandbox image error, got nil")
	}
	if !strings.Contains(err.Error(), "OPENCLARION_SANDBOX_IMAGE_REF") {
		t.Fatalf("error = %q, want OPENCLARION_SANDBOX_IMAGE_REF", err.Error())
	}
}

func TestParseReportReplayCLIArgs(t *testing.T) {
	cfg, err := parseReportReplayCLIArgs([]string{
		"--window-start", "2026-05-28T10:00:00Z",
		"--window-end", "2026-05-28T11:00:00Z",
		"--limit", "25",
		"--correlation-key", "incident-1",
		"--workflow-id", "report-batch-incident-1",
		"--scenario", "cascade",
		"--wait",
		"--wait-timeout", "5m",
	})
	if err != nil {
		t.Fatalf("parseReportReplayCLIArgs: %v", err)
	}
	if !cfg.WindowStart.Equal(time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)) ||
		!cfg.WindowEnd.Equal(time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)) ||
		cfg.Limit != 25 ||
		cfg.CorrelationKey != "incident-1" ||
		cfg.WorkflowID != "report-batch-incident-1" ||
		cfg.Scenario != reportprompt.ScenarioCascade ||
		!cfg.Wait ||
		cfg.WaitTimeout != 5*time.Minute {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseReportReplayCLIArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing start",
			args: []string{"--window-end", "2026-05-28T11:00:00Z"},
			want: "--window-start",
		},
		{
			name: "invalid end",
			args: []string{"--window-start", "2026-05-28T10:00:00Z", "--window-end", "not-time"},
			want: "parse --window-end",
		},
		{
			name: "bad limit",
			args: []string{"--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z", "--limit", "0"},
			want: "--limit",
		},
		{
			name: "bad scenario",
			args: []string{"--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z", "--scenario", "freeform"},
			want: "--scenario",
		},
		{
			name: "bad wait timeout",
			args: []string{"--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z", "--wait", "--wait-timeout", "0s"},
			want: "--wait-timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReportReplayCLIArgs(tc.args)
			if err == nil {
				t.Fatalf("parseReportReplayCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRunReportReplayCLITriggerMapsRequestAndWritesJSON(t *testing.T) {
	windowStart := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	checkedAt := time.Date(2026, 5, 29, 1, 2, 3, 456000000, time.UTC)
	previousNow := reportReplayCLINowUTC
	reportReplayCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { reportReplayCLINowUTC = previousNow })
	trigger := &recordingReportReplayCLITrigger{
		result: reporttrigger.Result{
			Replay: alertreplay.Result{
				Stats: alertreplay.Stats{
					Ingested:       alertingest.Stats{Total: 1, Saved: 1},
					EventsLoaded:   1,
					GroupsBuilt:    1,
					GroupsSaved:    1,
					SnapshotsSaved: 1,
					GroupsClosed:   1,
				},
				Snapshots: []alertreplay.SnapshotRef{
					{ID: domain.EvidenceSnapshotID(7), GroupIndex: 0, EventCount: 1},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-1", RunID: "run-1"},
			Started:  true,
		},
	}
	var out bytes.Buffer
	err := runReportReplayCLITrigger(context.Background(), trigger, nil, reportReplayCLIConfig{
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		Limit:          5,
		CorrelationKey: "incident-1",
		WorkflowID:     "report-batch-1",
		Scenario:       reportprompt.ScenarioSingleAlert,
		WaitTimeout:    defaultReportReplayCLIWait,
	}, &out)
	if err != nil {
		t.Fatalf("runReportReplayCLITrigger: %v", err)
	}
	if trigger.req.Replay.WindowStart != windowStart ||
		trigger.req.Replay.WindowEnd != windowEnd ||
		trigger.req.Replay.Limit != 5 ||
		trigger.req.Replay.CreatedByWorkflow != reportReplayCLICreatedByWorkflow ||
		trigger.req.CorrelationKey != "incident-1" ||
		trigger.req.WorkflowID != "report-batch-1" ||
		trigger.req.Scenario != reportprompt.ScenarioSingleAlert {
		t.Fatalf("trigger req = %+v", trigger.req)
	}

	var got reportReplayCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if !got.Started || got.WorkflowID != "report-batch-1" || got.RunID != "run-1" {
		t.Fatalf("output workflow = %+v", got)
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) {
		t.Fatalf("output checked_at = %q, want %q", got.CheckedAt, checkedAt.Format(time.RFC3339Nano))
	}
	if got.Request.WindowStart != windowStart.Format(time.RFC3339Nano) ||
		got.Request.WindowEnd != windowEnd.Format(time.RFC3339Nano) ||
		got.Request.Limit != 5 ||
		got.Request.CorrelationKey != "incident-1" ||
		got.Request.WorkflowID != "report-batch-1" ||
		got.Request.Scenario != string(reportprompt.ScenarioSingleAlert) ||
		got.Request.Wait ||
		got.Request.WaitTimeout != defaultReportReplayCLIWait.String() {
		t.Fatalf("output request = %+v", got.Request)
	}
	if got.Waited || got.WorkflowResult != nil {
		t.Fatalf("output wait = waited %v result %+v, want no wait result", got.Waited, got.WorkflowResult)
	}
	if got.Stats.Ingested.Saved != 1 || got.Stats.SnapshotsSaved != 1 {
		t.Fatalf("output stats = %+v", got.Stats)
	}
	if len(got.Snapshots) != 1 || got.Snapshots[0].ID != 7 {
		t.Fatalf("output snapshots = %+v", got.Snapshots)
	}
}

func TestRunReportReplayCLITriggerWaitsForCompletion(t *testing.T) {
	windowStart := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	checkedAt := time.Date(2026, 5, 29, 1, 2, 3, 456000000, time.UTC)
	previousNow := reportReplayCLINowUTC
	reportReplayCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { reportReplayCLINowUTC = previousNow })
	trigger := &recordingReportReplayCLITrigger{
		result: reporttrigger.Result{
			Replay: alertreplay.Result{
				Snapshots: []alertreplay.SnapshotRef{
					{ID: domain.EvidenceSnapshotID(7), GroupIndex: 0, EventCount: 1},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-1", RunID: "run-1"},
			Started:  true,
		},
	}
	waiter := &recordingReportReplayCLIWaiter{
		result: reportReplayCLIWorkflowResult{
			SubReportIDs:               []int64{11, 12},
			FinalReportID:              99,
			NotificationIdempotencyKey: "final_report:99/notification",
			ProviderMessageID:          "message-1",
			NotificationStatus:         "delivered",
		},
	}

	var out bytes.Buffer
	err := runReportReplayCLITrigger(context.Background(), trigger, waiter, reportReplayCLIConfig{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Limit:       5,
		Scenario:    reportprompt.ScenarioSingleAlert,
		Wait:        true,
		WaitTimeout: time.Minute,
	}, &out)
	if err != nil {
		t.Fatalf("runReportReplayCLITrigger: %v", err)
	}
	if waiter.handle.WorkflowID != "report-batch-1" || waiter.handle.RunID != "run-1" {
		t.Fatalf("waiter handle = %+v", waiter.handle)
	}
	var got reportReplayCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if !got.Waited || got.WorkflowResult == nil {
		t.Fatalf("output wait = waited %v result %+v", got.Waited, got.WorkflowResult)
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) {
		t.Fatalf("output checked_at = %q, want %q", got.CheckedAt, checkedAt.Format(time.RFC3339Nano))
	}
	if got.Request.WindowStart != windowStart.Format(time.RFC3339Nano) ||
		got.Request.WindowEnd != windowEnd.Format(time.RFC3339Nano) ||
		got.Request.Limit != 5 ||
		got.Request.Scenario != string(reportprompt.ScenarioSingleAlert) ||
		!got.Request.Wait ||
		got.Request.WaitTimeout != time.Minute.String() {
		t.Fatalf("output request = %+v", got.Request)
	}
	if got.WorkflowResult.FinalReportID != 99 ||
		got.WorkflowResult.NotificationIdempotencyKey != "final_report:99/notification" ||
		got.WorkflowResult.ProviderMessageID != "message-1" ||
		got.WorkflowResult.NotificationStatus != "delivered" ||
		len(got.WorkflowResult.SubReportIDs) != 2 {
		t.Fatalf("workflow result = %+v", got.WorkflowResult)
	}
}

func TestParseDiagnosisRoomCloseCLIArgs(t *testing.T) {
	cfg, err := parseDiagnosisRoomCloseCLIArgs([]string{
		"--session-id", "diagnosis-session-abc",
		"--run-id", "run-1",
		"--reason", "live_smoke_completed",
		"--wait-timeout", "3m",
	})
	if err != nil {
		t.Fatalf("parseDiagnosisRoomCloseCLIArgs: %v", err)
	}
	if cfg.SessionID != "diagnosis-session-abc" ||
		cfg.RunID != "run-1" ||
		cfg.Reason != "live_smoke_completed" ||
		cfg.WaitTimeout != 3*time.Minute {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseDiagnosisRoomCloseCLIArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing session",
			args: []string{},
			want: "--session-id",
		},
		{
			name: "session whitespace",
			args: []string{"--session-id", " diagnosis-session-abc "},
			want: "session-id must not contain leading or trailing whitespace",
		},
		{
			name: "empty reason",
			args: []string{"--session-id", "diagnosis-session-abc", "--reason", " "},
			want: "--reason must be non-empty",
		},
		{
			name: "reason whitespace",
			args: []string{"--session-id", "diagnosis-session-abc", "--reason", " live_smoke_completed "},
			want: "--reason must not contain leading or trailing whitespace",
		},
		{
			name: "bad timeout",
			args: []string{"--session-id", "diagnosis-session-abc", "--wait-timeout", "0s"},
			want: "--wait-timeout",
		},
		{
			name: "positional",
			args: []string{"--session-id", "diagnosis-session-abc", "extra"},
			want: "unexpected positional arguments",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDiagnosisRoomCloseCLIArgs(tc.args)
			if err == nil {
				t.Fatal("parseDiagnosisRoomCloseCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRunDiagnosisRoomCloseCLIWithDependenciesWritesProof(t *testing.T) {
	checkedAt := time.Date(2026, 6, 4, 8, 0, 0, 123000000, time.UTC)
	previousNow := diagnosisRoomCloseCLINowUTC
	diagnosisRoomCloseCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { diagnosisRoomCloseCLINowUTC = previousNow })

	closedAt := time.Date(2026, 6, 4, 7, 59, 0, 0, time.UTC)
	assistantOccurredAt := closedAt.Add(-time.Second)
	requiresHumanReview := true
	finalConclusion := temporalpkg.DiagnosisRoomFinalConclusion{
		Status:              "available",
		Source:              "latest_assistant_turn",
		AssistantTurnID:     303,
		AssistantMessageID:  "msg-1/assistant",
		AssistantSequence:   2,
		AssistantOccurredAt: &assistantOccurredAt,
		Content:             "CPU alert is still firing.",
		Confidence:          "medium",
		RequiresHumanReview: &requiresHumanReview,
	}
	waiter := &recordingDiagnosisRoomCloseWaiter{
		result: temporalpkg.DiagnosisRoomWorkflowResult{
			SessionID:       "diagnosis-session-abc",
			ChatSessionID:   202,
			DiagnosisTaskID: 101,
			Status:          "closed",
			TurnCount:       1,
			ClosedAt:        &closedAt,
			CloseReason:     "live_smoke_completed",
			FinalConclusion: &finalConclusion,
		},
	}
	loader := &recordingDiagnosisRoomCloseEventsLoader{
		events: diagnosisRoomCloseEvents{
			CloseEvent: domain.DiagnosisTaskEvent{
				ID:         domain.DiagnosisTaskEventID(11),
				TaskID:     domain.DiagnosisTaskID(101),
				Kind:       diagnosisRoomCloseEventClosedKind,
				OccurredAt: closedAt,
			},
			ClosePayload: testDiagnosisRoomCloseEventPayload(closedAt, 1, finalConclusion),
			NotificationEvent: domain.DiagnosisTaskEvent{
				ID:         domain.DiagnosisTaskEventID(12),
				TaskID:     domain.DiagnosisTaskID(101),
				Kind:       diagnosisRoomCloseEventNotificationSentKind,
				OccurredAt: closedAt.Add(time.Microsecond),
			},
			Notification: diagnosisRoomCloseNotificationPayload{
				IdempotencyKey:    "diagnosis_room:101:close-notification",
				ProviderMessageID: "webhook-message-1",
				ProviderStatus:    "accepted",
			},
		},
	}
	cfg := diagnosisRoomCloseCLIConfig{
		SessionID:   "diagnosis-session-abc",
		RunID:       "run-1",
		Reason:      "live_smoke_completed",
		WaitTimeout: 3 * time.Second,
	}
	var out bytes.Buffer
	if err := runDiagnosisRoomCloseCLIWithDependencies(context.Background(), waiter, loader, cfg, &out); err != nil {
		t.Fatalf("runDiagnosisRoomCloseCLIWithDependencies: %v", err)
	}
	if waiter.cfg != cfg {
		t.Fatalf("waiter cfg = %+v, want %+v", waiter.cfg, cfg)
	}
	if loader.taskID != domain.DiagnosisTaskID(101) {
		t.Fatalf("loader taskID = %d, want 101", loader.taskID)
	}
	var got diagnosisRoomCloseCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) ||
		!got.Signaled ||
		got.Request.WorkflowID != "diagnosis-room-diagnosis-session-abc" ||
		got.Request.RunID != "run-1" ||
		got.Request.WaitTimeout != "3s" {
		t.Fatalf("output request/signaled = %+v checked_at=%q", got.Request, got.CheckedAt)
	}
	if got.Workflow.Status != "closed" ||
		got.Workflow.SessionID != "diagnosis-session-abc" ||
		got.Workflow.DiagnosisTaskID != 101 ||
		got.Workflow.ChatSessionID != 202 ||
		got.Workflow.TurnCount != 1 ||
		got.Workflow.CloseReason != "live_smoke_completed" ||
		got.Workflow.FinalConclusion.Status != "available" ||
		got.Workflow.FinalConclusion.AssistantTurnID != 303 ||
		got.Workflow.FinalConclusion.AssistantMessageID != "msg-1/assistant" ||
		got.Workflow.FinalConclusion.AssistantSequence != 2 ||
		got.Workflow.FinalConclusion.AssistantOccurredAt != assistantOccurredAt.Format(time.RFC3339Nano) ||
		got.Workflow.FinalConclusion.Content != "CPU alert is still firing." ||
		got.Workflow.FinalConclusion.Confidence != "medium" ||
		got.Workflow.FinalConclusion.RequiresHumanReview == nil ||
		!*got.Workflow.FinalConclusion.RequiresHumanReview {
		t.Fatalf("workflow output = %+v", got.Workflow)
	}
	if got.CloseEvent.Kind != diagnosisRoomCloseEventClosedKind ||
		got.CloseEvent.ConclusionVersion != "diagnosis-room-close.v1" ||
		got.CloseEvent.FinalConclusion.Status != "available" ||
		got.CloseEvent.FinalConclusion.AssistantTurnID != 303 ||
		got.NotificationEvent.Kind != diagnosisRoomCloseEventNotificationSentKind ||
		got.NotificationEvent.IdempotencyKey != "diagnosis_room:101:close-notification" ||
		got.NotificationEvent.ProviderMessageID != "webhook-message-1" ||
		got.NotificationEvent.ProviderStatus != "accepted" {
		t.Fatalf("event output = close %+v notification %+v", got.CloseEvent, got.NotificationEvent)
	}
}

func TestRunDiagnosisRoomCloseCLIWithDependenciesRequiresNotificationEvent(t *testing.T) {
	closedAt := time.Date(2026, 6, 4, 7, 59, 0, 0, time.UTC)
	finalConclusion := temporalpkg.DiagnosisRoomFinalConclusion{
		Status: "not_available",
		Source: "none",
		Reason: "room_closed_without_assistant_turn",
	}
	waiter := &recordingDiagnosisRoomCloseWaiter{
		result: temporalpkg.DiagnosisRoomWorkflowResult{
			SessionID:       "diagnosis-session-abc",
			ChatSessionID:   202,
			DiagnosisTaskID: 101,
			Status:          "closed",
			TurnCount:       0,
			ClosedAt:        &closedAt,
			CloseReason:     "live_smoke_completed",
			FinalConclusion: &finalConclusion,
		},
	}
	loader := &recordingDiagnosisRoomCloseEventsLoader{
		events: diagnosisRoomCloseEvents{
			CloseEvent: domain.DiagnosisTaskEvent{
				ID:         domain.DiagnosisTaskEventID(11),
				TaskID:     domain.DiagnosisTaskID(101),
				Kind:       diagnosisRoomCloseEventClosedKind,
				OccurredAt: closedAt,
			},
			ClosePayload: testDiagnosisRoomCloseEventPayload(closedAt, 0, finalConclusion),
		},
	}
	var out bytes.Buffer
	err := runDiagnosisRoomCloseCLIWithDependencies(context.Background(), waiter, loader, diagnosisRoomCloseCLIConfig{
		SessionID:   "diagnosis-session-abc",
		Reason:      "live_smoke_completed",
		WaitTimeout: time.Second,
	}, &out)
	if err == nil {
		t.Fatal("runDiagnosisRoomCloseCLIWithDependencies: want missing notification event error")
	}
	if !strings.Contains(err.Error(), "close notification event is missing") {
		t.Fatalf("err = %q, want notification event error", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
}

func TestDiagnosisRoomClosePayloadsDecodeLiveEventShape(t *testing.T) {
	closePayload := []byte(`{
		"kind": "diagnosis_room.closed",
		"source": "DiagnosisRoomWorkflow",
		"status": "closed",
		"closed_at": "2026-06-05T07:45:20.308144+08:00",
		"session_id": "diagnosis-session-abc",
		"turn_count": 1,
		"close_reason": "local_rehearsal_completed",
		"owner_subject": "operator-1",
		"chat_session_id": 202,
		"final_conclusion": {
			"source": "latest_assistant_turn",
			"status": "available",
			"content": "Local diagnosis conclusion.",
			"confidence": "medium",
			"assistant_turn_id": 303,
			"assistant_sequence": 2,
			"assistant_message_id": "msg-1/assistant",
			"assistant_occurred_at": "2026-06-05T07:45:18.961702+08:00",
			"requires_human_review": true
		},
		"diagnosis_task_id": 101,
		"conclusion_version": "diagnosis-room-close.v1"
	}`)
	var closeOut diagnosisRoomCloseEventPayload
	if err := strictjson.Unmarshal(closePayload, &closeOut); err != nil {
		t.Fatalf("decode close payload: %v", err)
	}
	if closeOut.Source != "DiagnosisRoomWorkflow" ||
		closeOut.FinalConclusion.Source != "latest_assistant_turn" ||
		closeOut.FinalConclusion.AssistantTurnID != 303 {
		t.Fatalf("close payload = %+v", closeOut)
	}

	notificationPayload := []byte(`{
		"kind": "diagnosis_room.close_notification_sent",
		"source": "DiagnosisRoomWorkflow",
		"session_id": "diagnosis-session-abc",
		"turn_count": 1,
		"close_reason": "local_rehearsal_completed",
		"provider_raw": {"status": "accepted", "message_id": "m5-local-close-1"},
		"owner_subject": "operator-1",
		"alert_group_id": 1,
		"chat_session_id": 202,
		"idempotency_key": "diagnosis_room:101:close-notification",
		"provider_status": "accepted",
		"diagnosis_task_id": 101,
		"provider_message_id": "m5-local-close-1",
		"evidence_snapshot_id": 1
	}`)
	var notificationOut diagnosisRoomCloseNotificationPayload
	if err := strictjson.Unmarshal(notificationPayload, &notificationOut); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if notificationOut.Source != "DiagnosisRoomWorkflow" ||
		notificationOut.ProviderStatus != "accepted" ||
		notificationOut.ProviderMessageID != "m5-local-close-1" ||
		len(notificationOut.ProviderRaw) == 0 {
		t.Fatalf("notification payload = %+v", notificationOut)
	}
}

func TestValidateDiagnosisRoomClosePayloadAcceptsPersistedMicrosecondPrecision(t *testing.T) {
	closedAt := time.Date(2026, 6, 4, 23, 49, 59, 340646365, time.UTC)
	assistantOccurredAt := closedAt.Add(-time.Second)
	payloadClosedAt := closedAt.Truncate(time.Microsecond)
	payloadAssistantOccurredAt := assistantOccurredAt.Truncate(time.Microsecond)
	requiresHumanReview := true
	result := temporalpkg.DiagnosisRoomWorkflowResult{
		SessionID:       "diagnosis-session-abc",
		ChatSessionID:   202,
		DiagnosisTaskID: 101,
		Status:          "closed",
		TurnCount:       1,
		ClosedAt:        &closedAt,
		CloseReason:     "local_rehearsal_completed",
		FinalConclusion: &temporalpkg.DiagnosisRoomFinalConclusion{
			Status:              "available",
			Source:              "latest_assistant_turn",
			AssistantTurnID:     303,
			AssistantMessageID:  "msg-1/assistant",
			AssistantSequence:   2,
			AssistantOccurredAt: &assistantOccurredAt,
			Content:             "CPU alert is still firing.",
			Confidence:          "medium",
			RequiresHumanReview: &requiresHumanReview,
		},
	}
	payload := diagnosisRoomCloseEventPayload{
		Kind:            diagnosisRoomCloseEventClosedKind,
		Source:          "DiagnosisRoomWorkflow",
		SessionID:       result.SessionID,
		ChatSessionID:   result.ChatSessionID,
		DiagnosisTaskID: result.DiagnosisTaskID,
		OwnerSubject:    "operator-1",
		Status:          result.Status,
		TurnCount:       result.TurnCount,
		CloseReason:     result.CloseReason,
		ClosedAt:        payloadClosedAt,
		FinalConclusion: temporalpkg.DiagnosisRoomFinalConclusion{
			Status:              result.FinalConclusion.Status,
			Source:              result.FinalConclusion.Source,
			AssistantTurnID:     result.FinalConclusion.AssistantTurnID,
			AssistantMessageID:  result.FinalConclusion.AssistantMessageID,
			AssistantSequence:   result.FinalConclusion.AssistantSequence,
			AssistantOccurredAt: &payloadAssistantOccurredAt,
			Content:             result.FinalConclusion.Content,
			Confidence:          result.FinalConclusion.Confidence,
			RequiresHumanReview: result.FinalConclusion.RequiresHumanReview,
		},
		ConclusionVersion: "diagnosis-room-close.v1",
	}
	if err := validateDiagnosisRoomClosePayload(result, payload); err != nil {
		t.Fatalf("validateDiagnosisRoomClosePayload: %v", err)
	}
}

func testDiagnosisRoomCloseEventPayload(
	closedAt time.Time,
	turnCount int,
	finalConclusion temporalpkg.DiagnosisRoomFinalConclusion,
) diagnosisRoomCloseEventPayload {
	return diagnosisRoomCloseEventPayload{
		Kind:              diagnosisRoomCloseEventClosedKind,
		SessionID:         "diagnosis-session-abc",
		ChatSessionID:     202,
		DiagnosisTaskID:   101,
		OwnerSubject:      "oidc|user-1",
		Status:            "closed",
		TurnCount:         turnCount,
		CloseReason:       "live_smoke_completed",
		ClosedAt:          closedAt,
		FinalConclusion:   finalConclusion,
		ConclusionVersion: "diagnosis-room-close.v1",
	}
}

func mapGetenv(env map[string]string) getenvFunc {
	return func(key string) string {
		return env[key]
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newOIDCDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 issuer,
				"jwks_uri":               issuer + "/keys",
				"authorization_endpoint": issuer + "/auth",
				"token_endpoint":         issuer + "/token",
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = server.URL
	t.Cleanup(server.Close)
	return server
}

type emptyFactory struct{}

func (emptyFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, nil
}

func (emptyFactory) WithinTx(context.Context, func(context.Context, ports.UnitOfWork) error) error {
	return nil
}

type emptyStarter struct{}

func (emptyStarter) StartReportBatch(context.Context, ports.ReportBatchStartRequest) (ports.WorkflowHandle, error) {
	return ports.WorkflowHandle{}, nil
}

type noopDiagnosisRoomWorkflowClient struct{}

func (noopDiagnosisRoomWorkflowClient) SubmitDiagnosisTurn(context.Context, ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	return ports.DiagnosisRoomSubmitTurnResult{}, nil
}

func (noopDiagnosisRoomWorkflowClient) QueryDiagnosisRoom(context.Context, string) (ports.DiagnosisRoomState, error) {
	return ports.DiagnosisRoomState{}, nil
}

type noopDiagnosisRoomStarter struct{}

func (noopDiagnosisRoomStarter) StartDiagnosisRoom(context.Context, ports.DiagnosisRoomStartRequest) (ports.DiagnosisRoomStartResult, error) {
	return ports.DiagnosisRoomStartResult{}, nil
}

type recordingReportReplayCLITrigger struct {
	req    reporttrigger.Request
	result reporttrigger.Result
}

func (t *recordingReportReplayCLITrigger) ReplayAndStart(_ context.Context, req reporttrigger.Request) (reporttrigger.Result, error) {
	t.req = req
	return t.result, nil
}

type recordingReportReplayCLIWaiter struct {
	handle ports.WorkflowHandle
	result reportReplayCLIWorkflowResult
}

func (w *recordingReportReplayCLIWaiter) WaitReportBatch(_ context.Context, handle ports.WorkflowHandle) (reportReplayCLIWorkflowResult, error) {
	w.handle = handle
	return w.result, nil
}

type recordingDiagnosisRoomCloseWaiter struct {
	cfg    diagnosisRoomCloseCLIConfig
	result temporalpkg.DiagnosisRoomWorkflowResult
}

func (w *recordingDiagnosisRoomCloseWaiter) SignalAndWaitDiagnosisRoomClose(_ context.Context, cfg diagnosisRoomCloseCLIConfig) (temporalpkg.DiagnosisRoomWorkflowResult, error) {
	w.cfg = cfg
	return w.result, nil
}

type recordingDiagnosisRoomCloseEventsLoader struct {
	taskID domain.DiagnosisTaskID
	events diagnosisRoomCloseEvents
}

func (l *recordingDiagnosisRoomCloseEventsLoader) LoadDiagnosisRoomCloseEvents(_ context.Context, taskID domain.DiagnosisTaskID) (diagnosisRoomCloseEvents, error) {
	l.taskID = taskID
	return l.events, nil
}
