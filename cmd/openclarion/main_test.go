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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/alertdiagnosis"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/directorysync"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	temporalclient "go.temporal.io/sdk/client"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(env), nil, nil, nil, nil)
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

func TestReportActivityOptionsFromEnv_ConfiguresWeComWebhookProvider(t *testing.T) {
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":    "https://example.invalid/report-hook",
		"OPENCLARION_IM_WEBHOOK_FORMAT": "wecom",
	}), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestReportActivityOptionsFromEnv_AllowsUnconfiguredProviders(t *testing.T) {
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(nil), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("len(opts) = %d, want 0", len(opts))
	}
}

func TestReportActivityOptionsFromEnv_ConfiguresScheduledPolicyReplayer(t *testing.T) {
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(nil), emptyFactory{}, emptyStarter{}, nil, nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestCMDBProviderFromEnv_ConfiguresHTTPProvider(t *testing.T) {
	type capturedRequest struct {
		path string
		auth string
	}
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- capturedRequest{path: r.URL.Path, auth: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":false}`))
	}))
	defer server.Close()

	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	provider, configured, err := cmdbProviderFromEnv(mapGetenv(map[string]string{
		cmdbHTTPURLEnv:         server.URL + "/lookup",
		cmdbHTTPBearerTokenEnv: "test-cmdb-token",
		cmdbHTTPTimeoutEnv:     "2",
	}), nil)
	if err != nil {
		t.Fatalf("cmdbProviderFromEnv: %v", err)
	}
	if !configured || provider == nil {
		t.Fatalf("configured = %v, provider = %T, want configured provider", configured, provider)
	}
	result, err := provider.LookupResource(context.Background(), ports.CMDBLookupRequest{
		Labels: map[string]string{"service": "payments"},
	})
	if err != nil {
		t.Fatalf("LookupResource: %v", err)
	}
	if result.Found {
		t.Fatalf("result = %+v, want normal no-match", result)
	}
	got := <-requests
	if got.path != "/lookup" || got.auth != "Bearer test-cmdb-token" {
		t.Fatalf("request = %+v, want lookup path and bearer auth", got)
	}
}

func TestCMDBProviderFromEnv_OptionalAndPartialConfiguration(t *testing.T) {
	provider, configured, err := cmdbProviderFromEnv(mapGetenv(nil), nil)
	if err != nil || configured || provider != nil {
		t.Fatalf("unconfigured result = (%T, %v, %v), want nil, false, nil", provider, configured, err)
	}

	tests := []struct {
		name       string
		env        map[string]string
		wantSubstr string
	}{
		{
			name: "bearer token without URL",
			env: map[string]string{
				// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
				cmdbHTTPBearerTokenEnv: "test-cmdb-token",
			},
			wantSubstr: cmdbHTTPURLEnv,
		},
		{
			name: "timeout without URL",
			env: map[string]string{
				cmdbHTTPTimeoutEnv: "5",
			},
			wantSubstr: cmdbHTTPURLEnv,
		},
		{
			name: "invalid timeout",
			env: map[string]string{
				cmdbHTTPURLEnv:     "https://cmdb.example.invalid/lookup",
				cmdbHTTPTimeoutEnv: "0",
			},
			wantSubstr: cmdbHTTPTimeoutEnv,
		},
		{
			name: "invalid URL",
			env: map[string]string{
				cmdbHTTPURLEnv: "file:///tmp/cmdb.json",
			},
			wantSubstr: "URL scheme must be http or https",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider, configured, err := cmdbProviderFromEnv(mapGetenv(tc.env), nil)
			if !configured || provider != nil || err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("result = (%T, %v, %v), want configured error containing %q", provider, configured, err, tc.wantSubstr)
			}
		})
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

func TestPositiveDurationSecondsFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		fallback   time.Duration
		want       time.Duration
		wantSubstr string
	}{
		{
			name:     "default",
			fallback: 30 * time.Second,
			want:     30 * time.Second,
		},
		{
			name: "custom seconds",
			env: map[string]string{
				reportLLMHTTPTimeoutSecondsEnv: "120",
			},
			fallback: 30 * time.Second,
			want:     120 * time.Second,
		},
		{
			name: "zero",
			env: map[string]string{
				reportLLMHTTPTimeoutSecondsEnv: "0",
			},
			fallback:   30 * time.Second,
			wantSubstr: reportLLMHTTPTimeoutSecondsEnv,
		},
		{
			name: "non integer",
			env: map[string]string{
				reportLLMHTTPTimeoutSecondsEnv: "soon",
			},
			fallback:   30 * time.Second,
			wantSubstr: reportLLMHTTPTimeoutSecondsEnv,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := positiveDurationSecondsFromEnv(mapGetenv(tc.env), reportLLMHTTPTimeoutSecondsEnv, tc.fallback)
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
				t.Fatalf("positiveDurationSecondsFromEnv: %v", err)
			}
			if got != tc.want {
				t.Fatalf("duration = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestReportLLMDefaultHTTPTimeoutAllowsSlowGateway(t *testing.T) {
	got, err := positiveDurationSecondsFromEnv(mapGetenv(nil), reportLLMHTTPTimeoutSecondsEnv, defaultReportLLMHTTPTimeout)
	if err != nil {
		t.Fatalf("positiveDurationSecondsFromEnv: %v", err)
	}
	if got != 260*time.Second {
		t.Fatalf("default report LLM HTTP timeout = %s, want 260s", got)
	}
}

func TestOutboundHTTPClientPreservesTimeoutWithoutTracing(t *testing.T) {
	client := outboundHTTPClient(nil, 45*time.Second)
	if client == nil {
		t.Fatal("client is nil")
	}
	if client.Timeout != 45*time.Second {
		t.Fatalf("Timeout = %s, want 45s", client.Timeout)
	}
}

func TestPositiveIntFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{name: "default", want: 3},
		{name: "configured", env: map[string]string{"OPENCLARION_TEST_INT": "7"}, want: 7},
		{name: "zero", env: map[string]string{"OPENCLARION_TEST_INT": "0"}, wantErr: true},
		{name: "negative", env: map[string]string{"OPENCLARION_TEST_INT": "-1"}, wantErr: true},
		{name: "not_integer", env: map[string]string{"OPENCLARION_TEST_INT": "1.5"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := positiveIntFromEnv(mapGetenv(tc.env), "OPENCLARION_TEST_INT", 3)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("positiveIntFromEnv error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("positiveIntFromEnv: %v", err)
			}
			if got != tc.want {
				t.Fatalf("positiveIntFromEnv = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAutoDiagnosisOptionsFromEnvRejectsOutOfRange(t *testing.T) {
	_, err := autoDiagnosisOptionsFromEnv(mapGetenv(map[string]string{
		autoDiagnosisMaxRoomsPerTriggerEnv: strconv.Itoa(alertdiagnosis.MaxRoomsPerTriggerLimit + 1),
	}))
	if err == nil || !strings.Contains(err.Error(), autoDiagnosisMaxRoomsPerTriggerEnv) {
		t.Fatalf("autoDiagnosisOptionsFromEnv error = %v, want env name", err)
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
		{
			name: "webhook format without url",
			env: map[string]string{
				"OPENCLARION_IM_WEBHOOK_FORMAT": "wecom",
			},
			wantSubstr: "OPENCLARION_IM_WEBHOOK_URL",
		},
		{
			name: "unsupported webhook format",
			env: map[string]string{
				"OPENCLARION_IM_WEBHOOK_URL":    "https://example.invalid/report-hook",
				"OPENCLARION_IM_WEBHOOK_FORMAT": "msteams",
			},
			wantSubstr: "unsupported format",
		},
		{
			name: "wecom webhook bearer token",
			// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
			env: map[string]string{
				"OPENCLARION_IM_WEBHOOK_URL":          "https://example.invalid/report-hook",
				"OPENCLARION_IM_WEBHOOK_FORMAT":       "wecom",
				"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN": "test-bearer-value",
			},
			wantSubstr: "bearer token is unsupported",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(tc.env), nil, nil, nil, nil)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestReportActivityOptionsFromEnv_ConfiguresNotificationChannelProviderResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	opts, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(map[string]string{
		notificationChannelSecretRefsEnv: `{"secret/openclarion/report-webhook-url":"https://example.invalid/report-hook"}`,
	}), emptyFactory{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("reportActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestReportActivityOptionsFromEnv_RejectsInvalidNotificationChannelSecretResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(map[string]string{
		notificationChannelSecretRefsEnv: `{"secret/openclarion/report-webhook-url":"bad webhook url"}`,
	}), emptyFactory{}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected notification channel secret resolver error, got nil")
	}
	if !strings.Contains(err.Error(), notificationChannelSecretRefsEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), notificationChannelSecretRefsEnv)
	}
	if strings.Contains(err.Error(), "bad webhook url") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestReportActivityOptionsFromEnv_RejectsNotificationResolverWithoutUnitOfWork(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, err := reportActivityOptionsFromEnv(context.Background(), discardLogger(), mapGetenv(map[string]string{
		notificationChannelSecretRefsEnv: `{"secret/openclarion/report-webhook-url":"https://example.invalid/report-hook"}`,
	}), nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected notification channel unit of work error, got nil")
	}
	if !strings.Contains(err.Error(), notificationChannelSecretRefsEnv) ||
		!strings.Contains(err.Error(), "unit of work factory") {
		t.Fatalf("error = %q, want notification channel unit of work rejection", err.Error())
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresReportTrigger(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_PROMETHEUS_URL":          "http://prometheus.example",
		"OPENCLARION_PROMETHEUS_BEARER_TOKEN": "test-bearer-value",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) = %d, want 6", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_AllowsUnconfiguredTrigger(t *testing.T) {
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(nil), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 5 {
		t.Fatalf("len(opts) = %d, want 5", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresRBACBootstrapAdmins(t *testing.T) {
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		rbacBootstrapAdminSubjectsEnv: "iam-admin-1, iam-admin-2",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) = %d, want 6", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresReportNotificationRetryWebhook(t *testing.T) {
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_IM_WEBHOOK_URL":    "https://example.invalid/report-hook",
		"OPENCLARION_IM_WEBHOOK_FORMAT": "wecom",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) = %d, want 6", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresReportNotificationRetryResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		notificationChannelSecretRefsEnv: `{"secret/openclarion/report-webhook-url":"https://example.invalid/report-hook"}`,
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 7 {
		t.Fatalf("len(opts) = %d, want 7", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresScheduleSynchronizer(t *testing.T) {
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(nil), emptyFactory{}, emptyStarter{}, nil, nil, nil, noopScheduleSyncer{}, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) = %d, want 6", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresAlertSourceSecretResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		alertSourceSecretRefsEnv: `{"secret/openclarion/prometheus-bearer":"test-bearer-value"}`,
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 5 {
		t.Fatalf("len(opts) = %d, want 5", len(opts))
	}
}

func TestDirectorySyncerFromEnv_ConfiguresFromIAMOIDCFallback(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	syncer, configured, err := directorySyncerFromEnv(mapGetenv(map[string]string{
		iamDirectoryProviderNameEnv: "ops_iam",
		iamOIDCIssuerEnv:            oidcServer.URL,
		iamOIDCClientIDEnv:          "openclarion-directory",
		iamOIDCClientSecretEnv:      "test-client-secret",
	}), emptyFactory{}, nil)
	if err != nil {
		t.Fatalf("directorySyncerFromEnv: %v", err)
	}
	if !configured {
		t.Fatal("configured = false, want true")
	}
	if syncer == nil {
		t.Fatal("syncer = nil, want configured service")
	}
}

func TestDirectorySyncerFromEnv_ConfiguresFromStandardOIDCAndDirectoryScopes(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	syncer, configured, err := directorySyncerFromEnv(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:       oidcServer.URL,
		standardOIDCClientIDEnv:     "openclarion-console",
		standardOIDCClientSecretEnv: "test-client-secret",
		standardDirectoryScopesEnv:  "directory:read",
	}), emptyFactory{}, nil)
	if err != nil {
		t.Fatalf("directorySyncerFromEnv: %v", err)
	}
	if !configured {
		t.Fatal("configured = false, want true")
	}
	if syncer == nil {
		t.Fatal("syncer = nil, want configured service")
	}
}

func TestDirectorySyncerFromEnv_UnconfiguredByDefault(t *testing.T) {
	syncer, configured, err := directorySyncerFromEnv(mapGetenv(nil), emptyFactory{}, nil)
	if err != nil {
		t.Fatalf("directorySyncerFromEnv: %v", err)
	}
	if configured {
		t.Fatal("configured = true, want false")
	}
	if syncer != nil {
		t.Fatalf("syncer = %#v, want nil", syncer)
	}
}

func TestIAMDirectorySyncConfigured(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "empty",
			env:  nil,
			want: false,
		},
		{
			name: "standard oidc alone does not enable directory sync",
			env: map[string]string{
				standardOIDCIssuerEnv:       "https://iam.example.test",
				standardOIDCClientIDEnv:     "openclarion",
				standardOIDCClientSecretEnv: "test-client-secret",
			},
			want: false,
		},
		{
			name: "standard directory scopes enable directory sync",
			env: map[string]string{
				standardDirectoryScopesEnv: "directory:read",
			},
			want: true,
		},
		{
			name: "directory-specific base url enables directory sync",
			env: map[string]string{
				iamDirectoryBaseURLEnv: "https://iam.example.test",
			},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := iamDirectorySyncConfigured(mapGetenv(tt.env)); got != tt.want {
				t.Fatalf("iamDirectorySyncConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDirectorySyncIntervalFromEnv(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		interval, enabled, err := directorySyncIntervalFromEnv(mapGetenv(nil))
		if err != nil {
			t.Fatalf("directorySyncIntervalFromEnv: %v", err)
		}
		if enabled || interval != 0 {
			t.Fatalf("enabled=%t interval=%s, want disabled", enabled, interval)
		}
	})

	t.Run("accepts production interval", func(t *testing.T) {
		interval, enabled, err := directorySyncIntervalFromEnv(mapGetenv(map[string]string{
			iamDirectorySyncIntervalEnv: "3600",
		}))
		if err != nil {
			t.Fatalf("directorySyncIntervalFromEnv: %v", err)
		}
		if !enabled || interval != time.Hour {
			t.Fatalf("enabled=%t interval=%s, want 1h", enabled, interval)
		}
	})

	t.Run("rejects short interval", func(t *testing.T) {
		_, _, err := directorySyncIntervalFromEnv(mapGetenv(map[string]string{
			iamDirectorySyncIntervalEnv: "59",
		}))
		if err == nil || !strings.Contains(err.Error(), "at least 1m0s") {
			t.Fatalf("error = %v, want minimum interval rejection", err)
		}
	})
}

func TestPeriodicDirectorySyncerFromEnvRequiresDirectoryConfig(t *testing.T) {
	_, _, _, err := periodicDirectorySyncerFromEnv(mapGetenv(map[string]string{
		iamDirectorySyncIntervalEnv: "3600",
	}), emptyFactory{}, nil)
	if err == nil || !strings.Contains(err.Error(), iamDirectorySyncIntervalEnv) {
		t.Fatalf("error = %v, want interval requiring IAM directory config", err)
	}
}

func TestRunPeriodicDirectorySyncRunsStartupAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	syncer := &recordingPeriodicDirectorySyncer{
		requests: make(chan directorysync.SyncRequest, 1),
		result: directorysync.Result{
			Run: domain.DirectorySyncRun{
				SyncedAt: time.Date(2026, 6, 28, 8, 0, 0, 0, time.UTC),
			},
			DepartmentPages:     1,
			UserPages:           1,
			DepartmentsUpserted: 2,
			UsersUpserted:       3,
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- runPeriodicDirectorySync(ctx, discardLogger(), syncer, time.Hour)
	}()

	select {
	case req := <-syncer.requests:
		if req.PageSize != 0 || req.UpdatedAfter != nil {
			t.Fatalf("startup request = %+v, want full default sync", req)
		}
	case <-time.After(time.Second):
		t.Fatal("periodic directory sync did not run startup sync")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runPeriodicDirectorySync: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("periodic directory sync did not stop after cancellation")
	}
}

func TestHTTPServerOptionsFromEnv_RejectsInvalidAlertSourceSecretResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		alertSourceSecretRefsEnv: `{"secret/openclarion/prometheus-bearer":"test bearer value"}`,
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected alert source secret resolver error, got nil")
	}
	if !strings.Contains(err.Error(), alertSourceSecretRefsEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), alertSourceSecretRefsEnv)
	}
	if strings.Contains(err.Error(), "test bearer value") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestHTTPServerOptionsFromEnv_RejectsInvalidNotificationChannelSecretResolver(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		notificationChannelSecretRefsEnv: `{"secret/openclarion/ops-webhook":"bad webhook url"}`,
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected notification channel secret resolver error, got nil")
	}
	if !strings.Contains(err.Error(), notificationChannelSecretRefsEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), notificationChannelSecretRefsEnv)
	}
	if strings.Contains(err.Error(), "bad webhook url") {
		t.Fatalf("error leaked secret value: %v", err)
	}
}

func TestHTTPServerOptionsFromEnv_RejectsPartialConfig(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_PROMETHEUS_BEARER_TOKEN": "test-bearer-value",
	}), emptyFactory{}, emptyStarter{}, nil, nil, nil, nil, nil)
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
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv diagnosis: %v", err)
	}
	if len(opts) != 9 {
		t.Fatalf("len(opts) = %d, want 9", len(opts))
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

func TestHTTPServerOptionsFromEnv_ConfiguresDiagnosisRoomFromStandardOIDCEnv(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OIDC_ISSUER":                           " " + oidcServer.URL + " ",
		"OIDC_CLIENT_ID":                        "openclarion-web",
		"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS": "http://127.0.0.1:32101",
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv diagnosis: %v", err)
	}
	if len(opts) != 9 {
		t.Fatalf("len(opts) = %d, want 9", len(opts))
	}
}

func TestHTTPServerOptionsFromEnv_PrefersStandardOIDCOverLegacyDiagnosisOIDCEnv(t *testing.T) {
	oidcServer := newOIDCDiscoveryServer(t)
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "iam aliases",
			env: map[string]string{
				iamOIDCIssuerEnv:   " " + oidcServer.URL + " ",
				iamOIDCClientIDEnv: "openclarion-web",
			},
		},
		{
			name: "standard aliases",
			env: map[string]string{
				standardOIDCIssuerEnv:   " " + oidcServer.URL + " ",
				standardOIDCClientIDEnv: "openclarion-web",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{
				diagnosisOIDCIssuerURLEnv:               "https://legacy-issuer.example.test",
				diagnosisOIDCClientIDEnv:                "legacy-openclarion-web",
				"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS": "http://127.0.0.1:32101",
			}
			for key, value := range tc.env {
				env[key] = value
			}
			opts, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(env), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
			if err != nil {
				t.Fatalf("httpServerOptionsFromEnv diagnosis: %v", err)
			}
			if len(opts) != 9 {
				t.Fatalf("len(opts) = %d, want 9", len(opts))
			}
		})
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresDiagnosisRoomWithLDAP(t *testing.T) {
	opts, originPolicy, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		diagnosisAuthModeEnv:                    "ldap",
		diagnosisLDAPURLEnv:                     "ldaps://ldap.example.test:636",
		diagnosisLDAPBaseDNEnv:                  "dc=example,dc=test",
		diagnosisLDAPSubjectAttributeEnv:        "mail",
		diagnosisLDAPOwnerRoleValuesEnv:         "cn=openclarion-operators,ou=groups,dc=example,dc=test",
		"OPENCLARION_DIAGNOSIS_ALLOWED_ORIGINS": "http://127.0.0.1:32101",
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv diagnosis ldap: %v", err)
	}
	if len(opts) != 9 {
		t.Fatalf("len(opts) = %d, want 9", len(opts))
	}
	if originPolicy == nil {
		t.Fatal("originPolicy is nil")
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:8080/ws/diagnosis", nil)
	req.Header.Set("Origin", "http://127.0.0.1:32101")
	if !originPolicy.CheckWebSocketOrigin(req) {
		t.Fatal("expected configured origin to be allowed")
	}
}

func TestHTTPServerOptionsFromEnv_RejectsDirectWeComBrowserLoginMode(t *testing.T) {
	_, _, err := httpServerOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		diagnosisAuthModeEnv:          "wecom",
		diagnosisSessionSigningKeyEnv: "unit-test-state-signing-key-32-bytes",
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
	if err == nil {
		t.Fatal("httpServerOptionsFromEnv error = nil, want direct WeCom browser login rejection")
	}
	for _, want := range []string{
		diagnosisAuthModeEnv,
		"IAM OIDC",
		"application messages",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want guidance for %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "0123456789abcdef") ||
		strings.Contains(err.Error(), "owner") {
		t.Fatalf("error leaked configured values: %v", err)
	}
}

func TestDiagnosisAuthProviderFromEnv_ConfiguresStaticBearer(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	provider, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:              "static",
		diagnosisStaticBearerTokenEnv:     "test-diagnosis-token",
		diagnosisStaticSubjectEnv:         "operator-1",
		diagnosisStaticRolesEnv:           "admin,owner",
		"OPENCLARION_DIAGNOSIS_OIDC_URL":  "ignored",
		"OPENCLARION_DIAGNOSIS_OIDC_JWKS": "ignored",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "static" {
		t.Fatalf("provider name = %q, want static", name)
	}
	principal, err := provider.AuthenticateAuthorization(context.Background(), "Bearer test-diagnosis-token")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "operator-1" || len(principal.Roles) != 2 {
		t.Fatalf("principal = %+v", principal)
	}
	if strings.Contains(string(principal.Claims), "test-diagnosis-token") {
		t.Fatalf("claims leaked bearer token: %s", string(principal.Claims))
	}
}

func TestDiagnosisAuthProviderFromEnv_ConfiguresLDAP(t *testing.T) {
	_, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:             "ldap",
		diagnosisLDAPURLEnv:              "ldaps://ldap.example.test:636",
		diagnosisLDAPBaseDNEnv:           "dc=example,dc=test",
		diagnosisLDAPUserFilterEnv:       "(uid={username})",
		diagnosisLDAPSubjectAttributeEnv: "mail",
		diagnosisLDAPOwnerRoleValuesEnv:  "cn=openclarion-operators,ou=groups,dc=example,dc=test",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "ldap" {
		t.Fatalf("provider name = %q, want ldap", name)
	}
}

func TestDiagnosisWeComAppCallbackFromEnv_ConfiguresVerifier(t *testing.T) {
	verifier, err := diagnosisWeComAppCallbackFromEnv(mapGetenv(map[string]string{
		diagnosisWeComCorpIDEnv:                 "ww-openclarion",
		diagnosisWeComCallbackTokenEnv:          "callback-token-1",
		diagnosisWeComCallbackEncodingAESKeyEnv: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
	}))
	if err != nil {
		t.Fatalf("diagnosisWeComAppCallbackFromEnv: %v", err)
	}
	if verifier == nil {
		t.Fatal("verifier = nil, want configured verifier")
	}
}

func TestHTTPServerOptionsFromEnv_ConfiguresWeComAppCallbackMessageRouter(t *testing.T) {
	env := map[string]string{
		diagnosisWeComCorpIDEnv:                 "ww-openclarion",
		diagnosisWeComCallbackTokenEnv:          "callback-token-1",
		diagnosisWeComCallbackEncodingAESKeyEnv: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
	}
	opts, _, err := httpServerOptionsFromEnv(
		discardLogger(),
		mapGetenv(env),
		emptyFactory{},
		emptyStarter{},
		noopDiagnosisRoomWorkflowClient{},
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv: %v", err)
	}
	if len(opts) != 7 {
		t.Fatalf("len(opts) = %d, want 7", len(opts))
	}

	opts, _, err = httpServerOptionsFromEnv(
		discardLogger(),
		mapGetenv(env),
		emptyFactory{},
		emptyStarter{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("httpServerOptionsFromEnv without workflows: %v", err)
	}
	if len(opts) != 6 {
		t.Fatalf("len(opts) without workflows = %d, want 6", len(opts))
	}
}

func TestDiagnosisWeComAppCallbackFromEnv_RejectsPartialConfigWithoutLeakingValues(t *testing.T) {
	_, err := diagnosisWeComAppCallbackFromEnv(mapGetenv(map[string]string{
		diagnosisWeComCallbackTokenEnv: "callback-token-1",
	}))
	if err == nil {
		t.Fatal("diagnosisWeComAppCallbackFromEnv error = nil, want partial config error")
	}
	for _, want := range []string{
		diagnosisWeComCallbackTokenEnv,
		diagnosisWeComCallbackEncodingAESKeyEnv,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want guidance for %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "callback-token-1") {
		t.Fatalf("error leaked configured token: %v", err)
	}
}

func TestDiagnosisWeComAppCallbackFromEnv_RequiresReceiveID(t *testing.T) {
	_, err := diagnosisWeComAppCallbackFromEnv(mapGetenv(map[string]string{
		diagnosisWeComCallbackTokenEnv:          "callback-token-1",
		diagnosisWeComCallbackEncodingAESKeyEnv: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
	}))
	if err == nil {
		t.Fatal("diagnosisWeComAppCallbackFromEnv error = nil, want receive id guidance")
	}
	if !strings.Contains(err.Error(), diagnosisWeComCallbackReceiveIDEnv) ||
		!strings.Contains(err.Error(), diagnosisWeComCorpIDEnv) {
		t.Fatalf("error = %v, want receive id guidance", err)
	}
}

func TestDiagnosisAuthProviderFromEnv_IgnoresWeComCallbackWhenDetectingMode(t *testing.T) {
	provider, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisWeComCorpIDEnv:                 "ww-openclarion",
		diagnosisWeComCallbackTokenEnv:          "callback-token-1",
		diagnosisWeComCallbackEncodingAESKeyEnv: "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if provider != nil || name != "" {
		t.Fatalf("provider=%T name=%q, want no detected auth provider", provider, name)
	}
}

func TestDiagnosisAuthProviderFromEnv_DoesNotDetectLDAPWithoutExplicitMode(t *testing.T) {
	provider, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
		diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv: "owner",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if provider != nil || name != "" {
		t.Fatalf("provider=%T name=%q, want no detected auth provider", provider, name)
	}
}

func TestDiagnosisAuthProviderFromEnv_IgnoresLDAPOptionalEnvWhenDetectingMode(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	provider, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisLDAPUserFilterEnv:       "(uid={username})",
		diagnosisLDAPSubjectAttributeEnv: "mail",
		diagnosisLDAPRoleAttributeEnv:    "memberOf",
		diagnosisLDAPStartTLSEnv:         "sometimes",
		diagnosisLDAPAllowPlaintextEnv:   "sometimes",
		diagnosisStaticBearerTokenEnv:    "test-diagnosis-token",
		diagnosisStaticSubjectEnv:        "operator-1",
		diagnosisStaticRolesEnv:          "admin",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "static" {
		t.Fatalf("provider name = %q, want static", name)
	}
	principal, err := provider.AuthenticateAuthorization(context.Background(), "Bearer test-diagnosis-token")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "operator-1" {
		t.Fatalf("principal.Subject = %q, want operator-1", principal.Subject)
	}
}

func TestDiagnosisAuthProviderFromEnv_ConfiguresLDAPStartTLS(t *testing.T) {
	_, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:         "ldap",
		diagnosisLDAPURLEnv:          "ldap://ldap.example.test:389",
		diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv: "owner",
		diagnosisLDAPStartTLSEnv:     "true",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "ldap" {
		t.Fatalf("provider name = %q, want ldap", name)
	}
}

func TestDiagnosisAuthProviderFromEnv_AllowsExplicitInsecureLDAP(t *testing.T) {
	_, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:           "ldap",
		diagnosisLDAPURLEnv:            "ldap://ldap.example.test:389",
		diagnosisLDAPBaseDNEnv:         "dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv:   "owner",
		diagnosisLDAPAllowPlaintextEnv: "true",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "ldap" {
		t.Fatalf("provider name = %q, want ldap", name)
	}
}

func TestDiagnosisAuthProviderFromEnv_RejectsInsecureLDAPByDefault(t *testing.T) {
	_, _, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:         "ldap",
		diagnosisLDAPURLEnv:          "ldap://ldap.example.test:389",
		diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv: "owner",
	}), nil)
	if err == nil {
		t.Fatal("diagnosisAuthProviderFromEnv error = nil, want error")
	}
	if !strings.Contains(err.Error(), diagnosisLDAPStartTLSEnv) &&
		!strings.Contains(err.Error(), "start tls") {
		t.Fatalf("error = %q, want start tls transport error", err.Error())
	}
	if strings.Contains(err.Error(), "ldap.example.test") {
		t.Fatalf("error leaked ldap url: %v", err)
	}
}

func TestDiagnosisAuthProviderFromEnv_DetectsStaticBearerWithoutExplicitMode(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
	_, name, err := diagnosisAuthProviderFromEnv(mapGetenv(map[string]string{
		diagnosisStaticBearerTokenEnv: "test-diagnosis-token",
		diagnosisStaticSubjectEnv:     "operator-1",
		diagnosisStaticRolesEnv:       "admin",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisAuthProviderFromEnv: %v", err)
	}
	if name != "static" {
		t.Fatalf("provider name = %q, want static", name)
	}
}

func TestDiagnosisAuthProviderFromEnv_RejectsInvalidStaticConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "missing roles",
			// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
			env: map[string]string{
				diagnosisAuthModeEnv:          "static",
				diagnosisStaticBearerTokenEnv: "test-diagnosis-token",
				diagnosisStaticSubjectEnv:     "operator-1",
			},
			want: diagnosisStaticRolesEnv,
		},
		{
			name: "unsupported role",
			// #nosec G101 -- test-only env fixture uses a non-secret placeholder value.
			env: map[string]string{
				diagnosisAuthModeEnv:          "static",
				diagnosisStaticBearerTokenEnv: "test-diagnosis-token",
				diagnosisStaticSubjectEnv:     "operator-1",
				diagnosisStaticRolesEnv:       "viewer",
			},
			want: "viewer",
		},
		{
			name: "unsupported mode",
			env: map[string]string{
				diagnosisAuthModeEnv: "basic",
			},
			want: diagnosisAuthModeEnv,
		},
		{
			name: "invalid ldap role",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv: "viewer",
				diagnosisLDAPBindPasswordEnv: "fixture-password",
			},
			want: "viewer",
		},
		{
			name: "invalid ldap bind pair",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv: "owner",
				diagnosisLDAPBindDNEnv:       "cn=openclarion,dc=example,dc=test",
			},
			want: "bind",
		},
		{
			name: "invalid ldap bind password newline",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv: "owner",
				diagnosisLDAPBindDNEnv:       "cn=openclarion,dc=example,dc=test",
				diagnosisLDAPBindPasswordEnv: "fixture-password\n",
			},
			want: "bind password",
		},
		{
			name: "invalid ldap base dn whitespace",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       " dc=example,dc=test ",
				diagnosisLDAPDefaultRolesEnv: "owner",
			},
			want: "base dn",
		},
		{
			name: "invalid ldap bind dn whitespace",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv: "owner",
				diagnosisLDAPBindDNEnv:       " cn=openclarion,dc=example,dc=test ",
				diagnosisLDAPBindPasswordEnv: "fixture-password",
			},
			want: "bind dn",
		},
		{
			name: "invalid ldap subject attribute whitespace",
			env: map[string]string{
				diagnosisAuthModeEnv:             "ldap",
				diagnosisLDAPURLEnv:              "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:           "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv:     "owner",
				diagnosisLDAPSubjectAttributeEnv: " mail ",
			},
			want: "subject attribute",
		},
		{
			name: "invalid ldap start tls bool",
			env: map[string]string{
				diagnosisAuthModeEnv:         "ldap",
				diagnosisLDAPURLEnv:          "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv: "owner",
				diagnosisLDAPStartTLSEnv:     "sometimes",
			},
			want: diagnosisLDAPStartTLSEnv,
		},
		{
			name: "invalid ldap plaintext bool",
			env: map[string]string{
				diagnosisAuthModeEnv:           "ldap",
				diagnosisLDAPURLEnv:            "ldaps://ldap.example.test:636",
				diagnosisLDAPBaseDNEnv:         "dc=example,dc=test",
				diagnosisLDAPDefaultRolesEnv:   "owner",
				diagnosisLDAPAllowPlaintextEnv: "sometimes",
			},
			want: diagnosisLDAPAllowPlaintextEnv,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := diagnosisAuthProviderFromEnv(mapGetenv(tc.env), nil)
			if err == nil {
				t.Fatal("diagnosisAuthProviderFromEnv error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "test-diagnosis-token") {
				t.Fatalf("error leaked bearer token: %v", err)
			}
		})
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
			}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
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
	}), emptyFactory{}, emptyStarter{}, noopDiagnosisRoomWorkflowClient{}, noopDiagnosisRoomStarter{}, diagnosisauth.NewMemoryStore(), nil, nil)
	if err == nil {
		t.Fatal("expected diagnosis OIDC issuer error, got nil")
	}
	if !strings.Contains(err.Error(), "issuer url") {
		t.Fatalf("error = %q, want issuer url", err.Error())
	}
}

func TestDiagnosisActivityOptionsFromEnv_ConfiguresEvidenceProviderWithoutSandbox(t *testing.T) {
	opts, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{}), nil)
	if err != nil {
		t.Fatalf("diagnosisActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

func TestDiagnosisActivityOptionsFromEnv_ConfiguresPublicBaseURL(t *testing.T) {
	opts, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		publicBaseURLEnv: "https://openclarion.example.test/ops",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 2 {
		t.Fatalf("len(opts) = %d, want 2", len(opts))
	}
}

func TestDiagnosisActivityOptionsFromEnv_RejectsInvalidPublicBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		wantSubstr string
	}{
		{name: "relative", value: "openclarion.local/ops", wantSubstr: "scheme"},
		// #nosec G101 -- test-only credential-bearing URL fixture verifies sanitization.
		{name: "userinfo", value: "https://operator:secret@example.test", wantSubstr: "userinfo"},
		{name: "query", value: "https://example.test/ops?token=secret", wantSubstr: "query or fragment"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
				publicBaseURLEnv: tc.value,
			}), nil)
			if err == nil {
				t.Fatal("expected public base URL error, got nil")
			}
			if !strings.Contains(err.Error(), publicBaseURLEnv) || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %q, want %s and %s", err.Error(), publicBaseURLEnv, tc.wantSubstr)
			}
			if strings.Contains(err.Error(), tc.value) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked public base URL value: %q", err.Error())
			}
		})
	}
}

func TestDiagnosisActivityOptionsFromEnv_ConfiguresDockerProvider(t *testing.T) {
	opts, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": t.TempDir(),
		"OPENCLARION_SANDBOX_COMMAND_JSON":      `["/runner"]`,
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":    "llm.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_NETWORK":    "openclarion-sandbox-egress-prod",
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":    "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":     "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":       "test-model",
	}), nil)
	if err != nil {
		t.Fatalf("diagnosisActivityOptionsFromEnv: %v", err)
	}
	if len(opts) != 4 {
		t.Fatalf("len(opts) = %d, want 4", len(opts))
	}
}

func TestDiagnosisActivityOptionsFromEnv_RejectsUnsafeSandboxEgressNetwork(t *testing.T) {
	_, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": t.TempDir(),
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":    "llm.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_NETWORK":    "host",
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":    "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":     "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":       "test-model",
	}), nil)
	if err == nil {
		t.Fatal("diagnosisActivityOptionsFromEnv err = nil, want unsafe egress network error")
	}
	if !strings.Contains(err.Error(), "OPENCLARION_SANDBOX_EGRESS_NETWORK") &&
		!strings.Contains(err.Error(), "dedicated Docker network") {
		t.Fatalf("err = %v, want sandbox egress network rejection", err)
	}
}

func TestDiagnosisActivityOptionsFromEnv_RejectsMissingDiagnosisLLMConfig(t *testing.T) {
	_, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_IMAGE_REF":         "registry.example/openclarion/diagnosis@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"OPENCLARION_SANDBOX_AGENT_CONFIG_ROOT": t.TempDir(),
		"OPENCLARION_SANDBOX_COMMAND_JSON":      `["/runner"]`,
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":    "https://llm.example.invalid/v1",
	}), nil)
	if err == nil {
		t.Fatal("expected diagnosis LLM config error, got nil")
	}
	if !strings.Contains(err.Error(), "OPENCLARION_DIAGNOSIS_LLM_API_KEY") {
		t.Fatalf("error = %q, want OPENCLARION_DIAGNOSIS_LLM_API_KEY", err.Error())
	}
	if strings.Contains(err.Error(), "test-api-key") {
		t.Fatalf("error leaked credential value: %q", err.Error())
	}
}

func TestDiagnosisContainerCredentialsFromEnv_IncludesOptionalRunnerConfig(t *testing.T) {
	got, err := diagnosisContainerCredentialsFromEnv(mapGetenv(map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":             "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":              "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":                "test-model",
		"OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS": "170",
		"OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE":          "json_schema",
	}))
	if err != nil {
		t.Fatalf("diagnosisContainerCredentialsFromEnv: %v", err)
	}
	want := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":             "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":              "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":                "test-model",
		"OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS": "170",
		"OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE":          "json_schema",
	}
	if len(got) != len(want) {
		t.Fatalf("credentials len = %d, want %d: %+v", len(got), len(want), got)
	}
	for _, credential := range got {
		if want[credential.Name] != credential.Value {
			t.Fatalf("credential %q = %q", credential.Name, credential.Value)
		}
		delete(want, credential.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing credentials: %+v", want)
	}
}

func TestDiagnosisContainerCredentialsFromEnv_RejectsInvalidOptionalRunnerConfig(t *testing.T) {
	base := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	tests := []struct {
		name       string
		override   map[string]string
		wantSubstr string
	}{
		{
			name: "invalid timeout",
			override: map[string]string{
				"OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS": "soon",
			},
			wantSubstr: "OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS",
		},
		{
			name: "invalid output mode",
			override: map[string]string{
				"OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE": "markdown",
			},
			wantSubstr: "OPENCLARION_DIAGNOSIS_LLM_OUTPUT_MODE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{}
			for key, value := range base {
				env[key] = value
			}
			for key, value := range tc.override {
				env[key] = value
			}
			_, err := diagnosisContainerCredentialsFromEnv(mapGetenv(env))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
			if strings.Contains(err.Error(), "test-api-key") {
				t.Fatalf("error leaked credential value: %q", err.Error())
			}
		})
	}
}

func TestDiagnosisActivityOptionsFromEnv_RejectsPartialConfig(t *testing.T) {
	_, err := diagnosisActivityOptionsFromEnv(discardLogger(), mapGetenv(map[string]string{
		"OPENCLARION_SANDBOX_COMMAND_JSON": `["/runner"]`,
	}), nil)
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

func TestParseReportPolicyReplayCLIArgs(t *testing.T) {
	cfg, err := parseReportPolicyReplayCLIArgs([]string{
		"--policy-id", "42",
		"--window-start", "2026-05-28T10:00:00Z",
		"--window-end", "2026-05-28T11:00:00Z",
		"--limit", "25",
		"--correlation-key", "incident-1",
		"--workflow-id", "report-batch-incident-1",
		"--wait",
		"--wait-timeout", "5m",
	})
	if err != nil {
		t.Fatalf("parseReportPolicyReplayCLIArgs: %v", err)
	}
	if cfg.PolicyID != 42 ||
		!cfg.WindowStart.Equal(time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)) ||
		!cfg.WindowEnd.Equal(time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC)) ||
		cfg.Limit != 25 ||
		cfg.CorrelationKey != "incident-1" ||
		cfg.WorkflowID != "report-batch-incident-1" ||
		!cfg.Wait ||
		cfg.WaitTimeout != 5*time.Minute {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseReportPolicyReplayCLIArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing policy",
			args: []string{"--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z"},
			want: "--policy-id",
		},
		{
			name: "invalid start",
			args: []string{"--policy-id", "42", "--window-start", "bad", "--window-end", "2026-05-28T11:00:00Z"},
			want: "parse --window-start",
		},
		{
			name: "invalid end",
			args: []string{"--policy-id", "42", "--window-start", "2026-05-28T10:00:00Z", "--window-end", "not-time"},
			want: "parse --window-end",
		},
		{
			name: "bad limit",
			args: []string{"--policy-id", "42", "--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z", "--limit", "0"},
			want: "--limit",
		},
		{
			name: "bad wait timeout",
			args: []string{"--policy-id", "42", "--window-start", "2026-05-28T10:00:00Z", "--window-end", "2026-05-28T11:00:00Z", "--wait", "--wait-timeout", "0s"},
			want: "--wait-timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReportPolicyReplayCLIArgs(tc.args)
			if err == nil {
				t.Fatalf("parseReportPolicyReplayCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRunReportPolicyReplayCLITriggerMapsRequestAndWritesJSON(t *testing.T) {
	windowStart := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	checkedAt := time.Date(2026, 5, 29, 1, 2, 3, 456000000, time.UTC)
	previousNow := reportReplayCLINowUTC
	reportReplayCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { reportReplayCLINowUTC = previousNow })
	trigger := &recordingReportPolicyReplayCLITrigger{
		result: reportpolicytrigger.Result{
			Policy: domain.ReportWorkflowPolicy{
				ID:             42,
				ReportScenario: domain.ReportWorkflowScenarioAlertStorm,
			},
			Trigger: reporttrigger.Result{
				Replay: alertreplay.Result{
					Stats: alertreplay.Stats{
						Ingested:       alertingest.Stats{Total: 1, Saved: 1},
						EventsLoaded:   1,
						GroupsBuilt:    1,
						GroupsSaved:    1,
						SnapshotsSaved: 1,
					},
					Snapshots: []alertreplay.SnapshotRef{
						{ID: domain.EvidenceSnapshotID(7), GroupIndex: 0, EventCount: 1},
					},
				},
				Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-1", RunID: "run-1"},
				Started:  true,
			},
		},
	}
	waiter := &recordingReportReplayCLIWaiter{
		result: reportReplayCLIWorkflowResult{
			SubReportIDs:               []int64{11},
			FinalReportID:              99,
			NotificationIdempotencyKey: "final_report:99/notification",
			ProviderMessageID:          "message-1",
			NotificationStatus:         "accepted",
		},
	}

	var out bytes.Buffer
	err := runReportPolicyReplayCLITrigger(context.Background(), trigger, waiter, reportPolicyReplayCLIConfig{
		PolicyID:       42,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		Limit:          5,
		CorrelationKey: "incident-1",
		WorkflowID:     "report-batch-1",
		Wait:           true,
		WaitTimeout:    time.Minute,
	}, &out)
	if err != nil {
		t.Fatalf("runReportPolicyReplayCLITrigger: %v", err)
	}
	if trigger.req.PolicyID != 42 ||
		trigger.req.WindowStart != windowStart ||
		trigger.req.WindowEnd != windowEnd ||
		trigger.req.Limit != 5 ||
		trigger.req.CorrelationKey != "incident-1" ||
		trigger.req.WorkflowID != "report-batch-1" {
		t.Fatalf("trigger req = %+v", trigger.req)
	}
	if waiter.handle.WorkflowID != "report-batch-1" || waiter.handle.RunID != "run-1" {
		t.Fatalf("waiter handle = %+v", waiter.handle)
	}
	var got reportReplayCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if got.Request.PolicyID != 42 ||
		got.Request.Scenario != string(reportprompt.ScenarioAlertStorm) ||
		got.Request.CorrelationKey != "incident-1" ||
		got.Request.WorkflowID != "report-batch-1" ||
		!got.Request.Wait ||
		got.Request.WaitTimeout != time.Minute.String() {
		t.Fatalf("output request = %+v", got.Request)
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) {
		t.Fatalf("checked_at = %q, want %q", got.CheckedAt, checkedAt.Format(time.RFC3339Nano))
	}
	if !got.Waited || got.WorkflowResult == nil || got.WorkflowResult.NotificationStatus != "accepted" {
		t.Fatalf("workflow result = %+v waited=%v", got.WorkflowResult, got.Waited)
	}
}

func TestParseReportScheduleLiveSmokeCLIArgs(t *testing.T) {
	cfg, err := parseReportScheduleLiveSmokeCLIArgs([]string{
		"--schedule-id", "9",
		"--policy-id", "42",
		"--temporal-schedule-id", "openclarion-report-policy-42-hourly",
		"--observed-after", "2026-06-06T00:00:00Z",
		"--wait-timeout", "10m",
	})
	if err != nil {
		t.Fatalf("parseReportScheduleLiveSmokeCLIArgs: %v", err)
	}
	if cfg.ScheduleID != 9 ||
		cfg.PolicyID != 42 ||
		cfg.TemporalScheduleID != "openclarion-report-policy-42-hourly" ||
		!cfg.ObservedAfter.Equal(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)) ||
		cfg.WaitTimeout != 10*time.Minute {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseReportScheduleLiveSmokeCLIArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing schedule",
			args: []string{"--policy-id", "42"},
			want: "--schedule-id",
		},
		{
			name: "missing policy",
			args: []string{"--schedule-id", "9"},
			want: "--policy-id",
		},
		{
			name: "bad temporal id whitespace",
			args: []string{"--schedule-id", "9", "--policy-id", "42", "--temporal-schedule-id", "bad id"},
			want: "--temporal-schedule-id",
		},
		{
			name: "bad observed after",
			args: []string{"--schedule-id", "9", "--policy-id", "42", "--observed-after", "soon"},
			want: "parse --observed-after",
		},
		{
			name: "bad wait timeout",
			args: []string{"--schedule-id", "9", "--policy-id", "42", "--wait-timeout", "0s"},
			want: "--wait-timeout",
		},
		{
			name: "positional",
			args: []string{"--schedule-id", "9", "--policy-id", "42", "extra"},
			want: "unexpected positional arguments",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseReportScheduleLiveSmokeCLIArgs(tc.args)
			if err == nil {
				t.Fatal("parseReportScheduleLiveSmokeCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestNewestScheduleActionAtOrAfterSelectsLatestActualTime(t *testing.T) {
	observedAfter := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	newerActual := time.Date(2026, 6, 6, 0, 20, 0, 0, time.UTC)
	olderActual := time.Date(2026, 6, 6, 0, 10, 0, 0, time.UTC)
	actions := []temporalclient.ScheduleActionResult{
		{
			ScheduleTime: time.Date(2026, 6, 6, 0, 19, 0, 0, time.UTC),
			ActualTime:   newerActual,
			StartWorkflowResult: &temporalclient.ScheduleWorkflowExecution{
				WorkflowID:          "launcher-newer",
				FirstExecutionRunID: "run-newer",
			},
		},
		{
			ScheduleTime: time.Date(2026, 6, 6, 0, 9, 0, 0, time.UTC),
			ActualTime:   olderActual,
			StartWorkflowResult: &temporalclient.ScheduleWorkflowExecution{
				WorkflowID:          "launcher-older",
				FirstExecutionRunID: "run-older",
			},
		},
		{
			ScheduleTime:        time.Date(2026, 6, 6, 0, 29, 0, 0, time.UTC),
			ActualTime:          time.Date(2026, 6, 6, 0, 30, 0, 0, time.UTC),
			StartWorkflowResult: nil,
		},
		{
			ScheduleTime: time.Date(2026, 6, 5, 23, 59, 0, 0, time.UTC),
			ActualTime:   observedAfter.Add(-time.Second),
			StartWorkflowResult: &temporalclient.ScheduleWorkflowExecution{
				WorkflowID:          "before-window",
				FirstExecutionRunID: "run-before",
			},
		},
	}

	got, ok := newestScheduleActionAtOrAfter(actions, observedAfter)
	if !ok {
		t.Fatal("newestScheduleActionAtOrAfter: want action")
	}
	if got.StartWorkflowResult.WorkflowID != "launcher-newer" || !got.ActualTime.Equal(newerActual) {
		t.Fatalf("action = %+v", got)
	}
}

func TestRunReportScheduleLiveSmokeCLIWithDependenciesWritesProof(t *testing.T) {
	checkedAt := time.Date(2026, 6, 6, 1, 2, 3, 456000000, time.UTC)
	previousNow := reportScheduleLiveSmokeCLINowUTC
	reportScheduleLiveSmokeCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { reportScheduleLiveSmokeCLINowUTC = previousNow })

	observedAfter := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 6, 5, 23, 45, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 6, 6, 0, 45, 0, 0, time.UTC)
	waiter := &recordingReportScheduleLiveSmokeWaiter{
		result: reportScheduleLiveSmokeWaitResult{
			ScheduleAction: reportScheduleLiveSmokeCLIAction{
				ScheduleTime: "2026-06-06T00:45:00Z",
				ActualTime:   "2026-06-06T00:45:01Z",
				WorkflowID:   "report-policy-schedule-9",
				RunID:        "launcher-run-1",
			},
			LauncherWorkflow: temporalpkg.ReportPolicyScheduleLauncherWorkflowResult{
				ScheduleID:                 9,
				ReportWorkflowPolicyID:     42,
				TemporalScheduleID:         "openclarion-report-policy-42-hourly",
				FireTime:                   time.Date(2026, 6, 6, 0, 45, 0, 0, time.UTC),
				WindowStart:                windowStart,
				WindowEnd:                  windowEnd,
				CorrelationKey:             "report-workflow-schedule:9:policy:42:2026-06-05T23:45:00Z:2026-06-06T00:45:00Z",
				WorkflowID:                 "report-schedule-abc",
				EventsLoaded:               2,
				Snapshots:                  1,
				ReportBatchWorkflowStarted: true,
				ReportBatchWorkflowID:      "report-batch-1",
				ReportBatchRunID:           "report-run-1",
			},
			ReportWorkflowResult: &reportReplayCLIWorkflowResult{
				SubReportIDs:               []int64{11},
				FinalReportID:              99,
				NotificationIdempotencyKey: "final_report:99/notification",
				ProviderMessageID:          "message-1",
				NotificationStatus:         "accepted",
			},
		},
	}
	schedule := testReportWorkflowSchedule(t)
	var out bytes.Buffer
	err := runReportScheduleLiveSmokeCLIWithDependencies(context.Background(), waiter, schedule, reportScheduleLiveSmokeCLIConfig{
		ScheduleID:         9,
		PolicyID:           42,
		TemporalScheduleID: "openclarion-report-policy-42-hourly",
		ObservedAfter:      observedAfter,
		WaitTimeout:        10 * time.Minute,
	}, &out)
	if err != nil {
		t.Fatalf("runReportScheduleLiveSmokeCLIWithDependencies: %v", err)
	}
	if waiter.schedule.ID != schedule.ID || waiter.cfg.ObservedAfter != observedAfter {
		t.Fatalf("waiter schedule/cfg = %+v %+v", waiter.schedule, waiter.cfg)
	}
	var got reportScheduleLiveSmokeCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) ||
		got.Request.ScheduleID != 9 ||
		got.Request.PolicyID != 42 ||
		got.Request.TemporalScheduleID != "openclarion-report-policy-42-hourly" ||
		got.Request.ObservedAfter != observedAfter.Format(time.RFC3339Nano) ||
		got.Request.WaitTimeout != "10m0s" ||
		!got.PersistedSchedule.Enabled ||
		got.PersistedSchedule.TemporalScheduleID != "openclarion-report-policy-42-hourly" ||
		!got.Waited {
		t.Fatalf("output request/schedule = %+v %+v checked_at=%q waited=%v", got.Request, got.PersistedSchedule, got.CheckedAt, got.Waited)
	}
	if got.ScheduleAction.WorkflowID != "report-policy-schedule-9" ||
		got.LauncherWorkflow.ReportBatchWorkflowID != "report-batch-1" ||
		got.ReportWorkflowResult == nil ||
		got.ReportWorkflowResult.FinalReportID != 99 ||
		got.ReportWorkflowResult.NotificationStatus != "accepted" {
		t.Fatalf("output action/launcher/report = %+v %+v %+v", got.ScheduleAction, got.LauncherWorkflow, got.ReportWorkflowResult)
	}
}

func TestRunReportScheduleLiveSmokeCLIWithDependenciesRejectsInvalidSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule domain.ReportWorkflowSchedule
		cfg      reportScheduleLiveSmokeCLIConfig
		want     string
	}{
		{
			name:     "policy mismatch",
			schedule: testReportWorkflowSchedule(t),
			cfg:      reportScheduleLiveSmokeCLIConfig{ScheduleID: 9, PolicyID: 7, WaitTimeout: time.Minute},
			want:     "schedule policy id",
		},
		{
			name: "disabled",
			schedule: func() domain.ReportWorkflowSchedule {
				s := testReportWorkflowSchedule(t)
				s.Enabled = false
				return s
			}(),
			cfg:  reportScheduleLiveSmokeCLIConfig{ScheduleID: 9, PolicyID: 42, WaitTimeout: time.Minute},
			want: "must be enabled",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runReportScheduleLiveSmokeCLIWithDependencies(context.Background(), &recordingReportScheduleLiveSmokeWaiter{}, tc.schedule, tc.cfg, &out)
			if err == nil {
				t.Fatal("runReportScheduleLiveSmokeCLIWithDependencies: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
		})
	}
}

func TestParseWorkflowBacklogCLIArgsDefaultsToOpenClarionRunningWorkflows(t *testing.T) {
	cfg, err := parseWorkflowBacklogCLIArgs(nil)
	if err != nil {
		t.Fatalf("parseWorkflowBacklogCLIArgs: %v", err)
	}
	if cfg.Limit != defaultWorkflowBacklogCLILimit || cfg.Status != "running" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if !sameStrings(cfg.WorkflowTypes, defaultWorkflowBacklogCLIWorkflowTypes) {
		t.Fatalf("workflow types = %#v, want %#v", cfg.WorkflowTypes, defaultWorkflowBacklogCLIWorkflowTypes)
	}
	gotQuery := workflowBacklogCLIVisibilityQuery(cfg)
	wantQuery := `ExecutionStatus = "Running" AND (WorkflowType = "ReportBatchWorkflow" OR WorkflowType = "ReportFanOutWorkflow" OR WorkflowType = "FinalReportWorkflow" OR WorkflowType = "ReportPolicyScheduleLauncherWorkflow" OR WorkflowType = "DiagnosisRoomWorkflow")`
	if gotQuery != wantQuery {
		t.Fatalf("query = %q, want %q", gotQuery, wantQuery)
	}
}

func TestParseWorkflowBacklogCLIArgsAllowsRawQuery(t *testing.T) {
	rawQuery := `WorkflowId = "report-batch-1" AND ExecutionStatus = "Running"`
	cfg, err := parseWorkflowBacklogCLIArgs([]string{"--query", rawQuery, "--limit", "5"})
	if err != nil {
		t.Fatalf("parseWorkflowBacklogCLIArgs: %v", err)
	}
	if cfg.Query != rawQuery || cfg.Limit != 5 || cfg.Status != "" || len(cfg.WorkflowTypes) != 0 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if got := workflowBacklogCLIVisibilityQuery(cfg); got != rawQuery {
		t.Fatalf("query = %q, want %q", got, rawQuery)
	}
}

func TestParseWorkflowBacklogCLIArgsRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "bad limit",
			args: []string{"--limit", "0"},
			want: "--limit",
		},
		{
			name: "workflow type injection",
			args: []string{"--workflow-types", `ReportBatchWorkflow" OR WorkflowType = "Other`},
			want: "--workflow-types",
		},
		{
			name: "raw query newline",
			args: []string{"--query", "ExecutionStatus = \"Running\"\nWorkflowType = \"Other\""},
			want: "--query must be a single-line",
		},
		{
			name: "namespace wide all statuses",
			args: []string{"--status", "all", "--all-types"},
			want: "too broad",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseWorkflowBacklogCLIArgs(tc.args)
			if err == nil {
				t.Fatal("parseWorkflowBacklogCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRunWorkflowBacklogCLIWithDependenciesWritesMetadataOnly(t *testing.T) {
	checkedAt := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	previousNow := workflowBacklogCLINowUTC
	workflowBacklogCLINowUTC = func() time.Time { return checkedAt }
	t.Cleanup(func() { workflowBacklogCLINowUTC = previousNow })

	lister := &recordingWorkflowBacklogLister{
		responses: []*workflowservicepb.ListWorkflowExecutionsResponse{
			{
				Executions: []*workflowpb.WorkflowExecutionInfo{
					testWorkflowExecutionInfo("report-batch-1", "run-1", "ReportBatchWorkflow", enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING, checkedAt.Add(-10*time.Minute)),
				},
				NextPageToken: []byte("next"),
			},
			{
				Executions: []*workflowpb.WorkflowExecutionInfo{
					testWorkflowExecutionInfo("diagnosis-session-1", "run-2", "DiagnosisRoomWorkflow", enumspb.WORKFLOW_EXECUTION_STATUS_FAILED, checkedAt.Add(-time.Hour)),
				},
			},
		},
	}
	var out bytes.Buffer
	cfg := workflowBacklogCLIConfig{
		Limit:         2,
		Status:        "running",
		WorkflowTypes: []string{"ReportBatchWorkflow", "DiagnosisRoomWorkflow"},
	}
	if err := runWorkflowBacklogCLIWithDependencies(context.Background(), lister, cfg, &out); err != nil {
		t.Fatalf("runWorkflowBacklogCLIWithDependencies: %v", err)
	}
	if len(lister.requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(lister.requests))
	}
	wantQuery := `ExecutionStatus = "Running" AND (WorkflowType = "ReportBatchWorkflow" OR WorkflowType = "DiagnosisRoomWorkflow")`
	if lister.requests[0].GetQuery() != wantQuery || lister.requests[1].GetQuery() != wantQuery {
		t.Fatalf("queries = %q %q, want %q", lister.requests[0].GetQuery(), lister.requests[1].GetQuery(), wantQuery)
	}
	if lister.requests[0].GetPageSize() != 2 || lister.requests[1].GetPageSize() != 1 {
		t.Fatalf("page sizes = %d %d, want 2 1", lister.requests[0].GetPageSize(), lister.requests[1].GetPageSize())
	}
	if string(lister.requests[1].GetNextPageToken()) != "next" {
		t.Fatalf("next page token = %q, want next", string(lister.requests[1].GetNextPageToken()))
	}
	if strings.Contains(out.String(), "should-not-leak") {
		t.Fatalf("workflow backlog output leaked memo payload: %s", out.String())
	}
	var got workflowBacklogCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if got.CheckedAt != checkedAt.Format(time.RFC3339Nano) ||
		got.Returned != 2 ||
		got.Truncated ||
		got.CountsByType["ReportBatchWorkflow"] != 1 ||
		got.CountsByStatus["running"] != 1 ||
		got.CountsByStatus["failed"] != 1 {
		t.Fatalf("output = %+v", got)
	}
	if len(got.Workflows) != 2 {
		t.Fatalf("workflow count = %d, want 2", len(got.Workflows))
	}
	first := got.Workflows[0]
	if first.WorkflowID != "report-batch-1" ||
		first.RunID != "run-1" ||
		first.WorkflowType != "ReportBatchWorkflow" ||
		first.Status != "running" ||
		first.TaskQueue != "openclarion" ||
		first.AgeSeconds != int64(10*time.Minute/time.Second) ||
		first.HistoryLength != 42 ||
		first.HistorySizeBytes != 4096 ||
		first.ParentWorkflowID != "parent-report-batch-1" ||
		first.RootWorkflowID != "root-report-batch-1" {
		t.Fatalf("first workflow = %+v", first)
	}
}

func TestParseDiagnosisRoomListCLIArgs(t *testing.T) {
	cfg, err := parseDiagnosisRoomListCLIArgs([]string{"--limit", "25", "--queue", "attention"})
	if err != nil {
		t.Fatalf("parseDiagnosisRoomListCLIArgs: %v", err)
	}
	if cfg.Limit != 25 || cfg.Queue != "attention" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseDiagnosisRoomListCLIArgsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "bad limit", args: []string{"--limit", "0"}, want: "--limit"},
		{name: "bad queue", args: []string{"--queue", "stale"}, want: "--queue"},
		{name: "positional", args: []string{"extra"}, want: "unexpected positional"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseDiagnosisRoomListCLIArgs(tc.args)
			if err == nil {
				t.Fatal("parseDiagnosisRoomListCLIArgs: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func TestRunDiagnosisRoomListCLIWithDependenciesFiltersAndWritesMetadataOnly(t *testing.T) {
	occurredAt := time.Date(2026, 6, 20, 8, 1, 0, 0, time.UTC)
	requiresHumanReview := true
	conclusion, err := diagnosisRoomListConclusionFromEvent(domain.DiagnosisTaskEvent{
		ID:         1,
		Kind:       "diagnosis_room.final_conclusion_ready",
		TaskID:     101,
		OccurredAt: occurredAt,
		Payload: json.RawMessage(`{
			"kind":"diagnosis_room.final_conclusion_ready",
			"final_conclusion":{
				"status":"available",
				"confidence":"high",
				"requires_human_review":true,
				"content":"should-not-leak"
			}
		}`),
	})
	if err != nil {
		t.Fatalf("diagnosisRoomListConclusionFromEvent: %v", err)
	}
	conclusion.RequiresHumanReview = &requiresHumanReview
	notification, err := diagnosisRoomListNotificationFromEvent(domain.DiagnosisTaskEvent{
		ID:         2,
		Kind:       "diagnosis_room.final_ready_notification_sent",
		TaskID:     101,
		OccurredAt: occurredAt.Add(time.Minute),
		Payload: json.RawMessage(`{
			"kind":"diagnosis_room.final_ready_notification_sent",
			"provider_status":"failed",
			"provider_raw":{"text":"should-not-leak"}
		}`),
	})
	if err != nil {
		t.Fatalf("diagnosisRoomListNotificationFromEvent: %v", err)
	}

	loader := &recordingDiagnosisRoomListLoader{
		rooms: []diagnosisRoomListRoom{
			testDiagnosisRoomListRoom("room-attention", domain.DiagnosisStatusRunning, domain.ChatSessionStatusOpen, 1, &conclusion, &notification),
			testDiagnosisRoomListRoom("room-ready", domain.DiagnosisStatusRunning, domain.ChatSessionStatusOpen, 2, &diagnosisRoomListCLIConclusion{
				EventKind:           "diagnosis_room.final_conclusion_ready",
				Status:              "available",
				Confidence:          "high",
				RequiresHumanReview: boolPtr(false),
				OccurredAt:          occurredAt.Format(time.RFC3339Nano),
			}, nil),
			testDiagnosisRoomListRoom("room-active", domain.DiagnosisStatusRunning, domain.ChatSessionStatusOpen, 1, nil, nil),
			testDiagnosisRoomListRoom("room-closed", domain.DiagnosisStatusSucceeded, domain.ChatSessionStatusClosed, 3, nil, nil),
		},
	}
	var out bytes.Buffer
	err = runDiagnosisRoomListCLIWithDependencies(context.Background(), loader, diagnosisRoomListCLIConfig{
		Limit: 10,
		Queue: "attention",
	}, &out)
	if err != nil {
		t.Fatalf("runDiagnosisRoomListCLIWithDependencies: %v", err)
	}
	if loader.limit != 10 {
		t.Fatalf("loader limit = %d, want 10", loader.limit)
	}
	if strings.Contains(out.String(), "should-not-leak") {
		t.Fatalf("diagnosis room list output leaked payload content: %s", out.String())
	}
	var got diagnosisRoomListCLIOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; raw=%s", err, out.String())
	}
	if got.Returned != 1 ||
		got.CountsByQueue["attention"] != 1 ||
		got.CountsByQueue["ready"] != 1 ||
		got.CountsByQueue["active"] != 1 ||
		got.CountsByQueue["closed"] != 1 {
		t.Fatalf("output counts = returned %d counts %+v", got.Returned, got.CountsByQueue)
	}
	if len(got.Rooms) != 1 {
		t.Fatalf("rooms len = %d, want 1", len(got.Rooms))
	}
	room := got.Rooms[0]
	if room.SessionID != "room-attention" ||
		room.NextStep.Queue != "attention" ||
		room.NextStep.Label != "Notification failed" ||
		room.LatestNotification == nil ||
		room.LatestNotification.ProviderStatus != "failed" ||
		room.LatestConclusion == nil ||
		room.LatestConclusion.Confidence != "high" {
		t.Fatalf("room = %+v", room)
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
		"evidence_snapshot_id": 1,
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
		closeOut.FinalConclusion.AssistantTurnID != 303 ||
		closeOut.EvidenceSnapshotID != 1 {
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
			"notification_channel_profile_id": 9,
			"idempotency_key": "diagnosis_room:101:close-notification",
			"provider_status": "accepted",
			"final_conclusion": {
				"source": "latest_assistant_turn",
				"status": "available",
				"content": "Local diagnosis conclusion.",
				"confidence": "medium",
				"assistant_turn_id": 303,
				"assistant_sequence": 2,
				"assistant_message_id": "msg-1/assistant",
				"assistant_occurred_at": "2026-06-05T07:45:18.961702+08:00",
				"evidence_snapshot_id": 1,
				"requires_human_review": true
			},
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
		notificationOut.NotificationChannelProfileID != 9 ||
		notificationOut.FinalConclusion.AssistantTurnID != 303 ||
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

func testReportWorkflowSchedule(t *testing.T) domain.ReportWorkflowSchedule {
	t.Helper()
	enabledAt := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	schedule, err := domain.NewReportWorkflowSchedule(
		"Hourly report",
		42,
		"openclarion-report-policy-42-hourly",
		time.Hour,
		15*time.Minute,
		time.Hour,
		0,
		100,
		10*time.Minute,
		true,
		&enabledAt,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowSchedule: %v", err)
	}
	schedule.ID = 9
	return schedule
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

type noopScheduleSyncer struct{}

func (noopScheduleSyncer) SyncReportWorkflowSchedule(context.Context, domain.ReportWorkflowSchedule) error {
	return nil
}

type recordingPeriodicDirectorySyncer struct {
	requests chan directorysync.SyncRequest
	result   directorysync.Result
	err      error
}

func (s *recordingPeriodicDirectorySyncer) Sync(_ context.Context, req directorysync.SyncRequest) (directorysync.Result, error) {
	s.requests <- req
	if s.err != nil {
		return directorysync.Result{}, s.err
	}
	return s.result, nil
}

type noopDiagnosisRoomWorkflowClient struct{}

func (noopDiagnosisRoomWorkflowClient) SubmitDiagnosisTurn(context.Context, ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	return ports.DiagnosisRoomSubmitTurnResult{}, nil
}

func (noopDiagnosisRoomWorkflowClient) CollectDiagnosisEvidence(context.Context, ports.DiagnosisRoomCollectEvidenceRequest) (ports.DiagnosisRoomCollectEvidenceResult, error) {
	return ports.DiagnosisRoomCollectEvidenceResult{}, nil
}

func (noopDiagnosisRoomWorkflowClient) ConfirmDiagnosisConclusion(context.Context, ports.DiagnosisRoomConfirmConclusionRequest) (ports.DiagnosisRoomState, error) {
	return ports.DiagnosisRoomState{}, nil
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

type recordingReportPolicyReplayCLITrigger struct {
	req    reportpolicytrigger.Request
	result reportpolicytrigger.Result
}

func (t *recordingReportPolicyReplayCLITrigger) ReplayAndStartDetailed(_ context.Context, req reportpolicytrigger.Request) (reportpolicytrigger.Result, error) {
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

type recordingReportScheduleLiveSmokeWaiter struct {
	schedule domain.ReportWorkflowSchedule
	cfg      reportScheduleLiveSmokeCLIConfig
	result   reportScheduleLiveSmokeWaitResult
}

func (w *recordingReportScheduleLiveSmokeWaiter) WaitReportSchedule(
	_ context.Context,
	schedule domain.ReportWorkflowSchedule,
	cfg reportScheduleLiveSmokeCLIConfig,
) (reportScheduleLiveSmokeWaitResult, error) {
	w.schedule = schedule
	w.cfg = cfg
	return w.result, nil
}

type recordingWorkflowBacklogLister struct {
	requests  []*workflowservicepb.ListWorkflowExecutionsRequest
	responses []*workflowservicepb.ListWorkflowExecutionsResponse
	err       error
}

func (l *recordingWorkflowBacklogLister) ListWorkflow(
	_ context.Context,
	req *workflowservicepb.ListWorkflowExecutionsRequest,
) (*workflowservicepb.ListWorkflowExecutionsResponse, error) {
	l.requests = append(l.requests, req)
	if l.err != nil {
		return nil, l.err
	}
	if len(l.responses) == 0 {
		return &workflowservicepb.ListWorkflowExecutionsResponse{}, nil
	}
	resp := l.responses[0]
	l.responses = l.responses[1:]
	return resp, nil
}

func testWorkflowExecutionInfo(
	workflowID string,
	runID string,
	workflowType string,
	status enumspb.WorkflowExecutionStatus,
	startedAt time.Time,
) *workflowpb.WorkflowExecutionInfo {
	return &workflowpb.WorkflowExecutionInfo{
		Execution: &commonpb.WorkflowExecution{
			WorkflowId: workflowID,
			RunId:      runID,
		},
		Type: &commonpb.WorkflowType{
			Name: workflowType,
		},
		StartTime:        timestamppb.New(startedAt),
		ExecutionTime:    timestamppb.New(startedAt.Add(2 * time.Second)),
		Status:           status,
		HistoryLength:    42,
		HistorySizeBytes: 4096,
		TaskQueue:        "openclarion",
		ParentExecution: &commonpb.WorkflowExecution{
			WorkflowId: "parent-" + workflowID,
			RunId:      "parent-" + runID,
		},
		RootExecution: &commonpb.WorkflowExecution{
			WorkflowId: "root-" + workflowID,
			RunId:      "root-" + runID,
		},
		Memo: &commonpb.Memo{
			Fields: map[string]*commonpb.Payload{
				"secret": {Data: []byte("should-not-leak")},
			},
		},
	}
}

type recordingDiagnosisRoomListLoader struct {
	limit int
	rooms []diagnosisRoomListRoom
	err   error
}

func (l *recordingDiagnosisRoomListLoader) ListDiagnosisRooms(_ context.Context, limit int) ([]diagnosisRoomListRoom, error) {
	l.limit = limit
	if l.err != nil {
		return nil, l.err
	}
	return l.rooms, nil
}

func testDiagnosisRoomListRoom(
	sessionKey string,
	taskStatus domain.DiagnosisStatus,
	roomStatus domain.ChatSessionStatus,
	turnCount int,
	conclusion *diagnosisRoomListCLIConclusion,
	notification *diagnosisRoomListCLINotification,
) diagnosisRoomListRoom {
	startedAt := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	var closedAt *time.Time
	closeReason := ""
	if roomStatus == domain.ChatSessionStatusClosed {
		closed := startedAt.Add(10 * time.Minute)
		closedAt = &closed
		closeReason = "operator confirmed"
	}
	return diagnosisRoomListRoom{
		Room: domain.ChatSessionWithTask{
			Session: domain.ChatSession{
				ID:              domain.ChatSessionID(202),
				DiagnosisTaskID: domain.DiagnosisTaskID(101),
				SessionKey:      sessionKey,
				Status:          roomStatus,
				TurnCount:       turnCount,
				StartedAt:       startedAt,
				LastActivityAt:  startedAt.Add(time.Minute),
				ClosedAt:        closedAt,
				CloseReason:     closeReason,
				CreatedAt:       startedAt,
				UpdatedAt:       startedAt.Add(time.Minute),
			},
			Task: domain.DiagnosisTask{
				ID:                 domain.DiagnosisTaskID(101),
				EvidenceSnapshotID: domain.EvidenceSnapshotID(7),
				WorkflowID:         "diagnosis-room-" + sessionKey,
				RunID:              "run-" + sessionKey,
				Status:             taskStatus,
				CreatedAt:          startedAt,
				UpdatedAt:          startedAt.Add(time.Minute),
			},
		},
		LatestConclusion:   conclusion,
		LatestNotification: notification,
	}
}

func boolPtr(value bool) *bool {
	return &value
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
