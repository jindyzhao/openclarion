package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	authfake "github.com/openclarion/openclarion/internal/providers/auth/fake"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/alertmanagerwebhook"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/alertsourcecheck"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

func TestListAlerts_ReturnsSummaries(t *testing.T) {
	startsAt := time.Date(2026, 5, 27, 8, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{
					ID:                   42,
					Source:               "prometheus",
					SourceFingerprint:    "source-fp",
					CanonicalFingerprint: "canon-fp",
					Labels:               map[string]string{"alertname": "HighCPU"},
					Annotations:          map[string]string{"summary": "CPU high"},
					Status:               domain.AlertStatusFiring,
					StartsAt:             startsAt,
					CreatedAt:            startsAt.Add(time.Second),
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.alertRepo.lastLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.alertRepo.lastLimit)
	}

	var body api.AlertListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 42 || got.Source != "prometheus" || got.Status != string(domain.AlertStatusFiring) {
		t.Fatalf("unexpected alert summary: %+v", got)
	}
	if !got.EndsAt.IsNull() {
		t.Fatalf("ends_at should be explicit null for firing alerts")
	}
}

func TestListEvidenceSnapshots_ReturnsSummaries(t *testing.T) {
	createdAt := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		evidenceRepo: &fakeEvidenceRepo{
			snapshots: []domain.EvidenceSnapshot{
				{
					ID:                7,
					AlertGroupID:      5,
					Digest:            "digest-1",
					Payload:           json.RawMessage(`{"metric":"cpu"}`),
					Provenance:        json.RawMessage(`{"prometheus":"ok"}`),
					Status:            domain.SnapshotStatusComplete,
					CreatedByWorkflow: "DiagnosisWorkflow",
					CreatedAt:         createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/evidence-snapshots?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.evidenceRepo.lastLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.evidenceRepo.lastLimit)
	}

	var body api.EvidenceSnapshotListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 7 || got.AlertGroupID != 5 || got.Payload["metric"] != "cpu" || got.Provenance["prometheus"] != "ok" {
		t.Fatalf("unexpected evidence summary: %+v", got)
	}
	if got.MissingFields == nil {
		t.Fatalf("missing_fields should be an empty array, not null")
	}
}

func TestListReports_ReturnsSummaries(t *testing.T) {
	createdAt := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                11,
					CorrelationKey:    "window:checkout",
					Title:             "Checkout latency incident",
					ExecutiveSummary:  "Checkout latency increased after a deployment.",
					Severity:          domain.ReportSeverityWarning,
					Confidence:        domain.ReportConfidenceHigh,
					NotificationText:  "Checkout latency incident requires review.",
					CreatedByWorkflow: "FinalReportWorkflow",
					CreatedAt:         createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.reportRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.reportRepo.lastListLimit)
	}

	var body api.ReportListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.ID != 11 || got.CorrelationKey != "window:checkout" || string(got.Severity) != string(domain.ReportSeverityWarning) {
		t.Fatalf("unexpected report summary: %+v", got)
	}
}

func TestGetDashboard_ReturnsRecentCounters(t *testing.T) {
	createdAt := time.Date(2026, 5, 28, 5, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		alertRepo: &fakeAlertRepo{
			events: []domain.AlertEvent{
				{ID: 1, Status: domain.AlertStatusFiring},
				{ID: 2, Status: domain.AlertStatusResolved},
				{ID: 3, Status: domain.AlertStatusFiring},
			},
		},
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{ID: 11, Severity: domain.ReportSeverityWarning, CreatedAt: createdAt},
				{ID: 12, Severity: domain.ReportSeverityCritical, CreatedAt: createdAt},
				{ID: 13, Severity: domain.ReportSeverityInfo, CreatedAt: createdAt},
				{ID: 14, Severity: domain.ReportSeverityWarning, CreatedAt: createdAt},
			},
			deliveriesByReport: map[domain.FinalReportID][]domain.ReportNotificationDelivery{
				11: {{Status: domain.ReportNotificationDeliveryStatusDelivered}},
				12: {{Status: domain.ReportNotificationDeliveryStatusFailed}},
				13: {{Status: domain.ReportNotificationDeliveryStatusPending}},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/dashboard", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.alertRepo.lastLimit != defaultListLimit || factory.reportRepo.lastListLimit != defaultListLimit {
		t.Fatalf("limits alert=%d report=%d, want %d", factory.alertRepo.lastLimit, factory.reportRepo.lastListLimit, defaultListLimit)
	}
	if factory.reportRepo.lastDeliveryLimit != 1 {
		t.Fatalf("delivery limit = %d, want 1", factory.reportRepo.lastDeliveryLimit)
	}

	var body api.DashboardSummary
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.GeneratedAt.IsZero() {
		t.Fatalf("generated_at should be set")
	}
	if body.Alerts.TotalRecent != 3 || body.Alerts.Firing != 2 || body.Alerts.Resolved != 1 {
		t.Fatalf("alert stats = %+v", body.Alerts)
	}
	if body.Reports.TotalRecent != 4 ||
		body.Reports.Delivered != 1 ||
		body.Reports.Failed != 1 ||
		body.Reports.Pending != 1 ||
		body.Reports.MissingDelivery != 1 {
		t.Fatalf("report delivery stats = %+v", body.Reports)
	}
	if body.Reports.Severity.Info != 1 || body.Reports.Severity.Warning != 2 || body.Reports.Severity.Critical != 1 {
		t.Fatalf("severity stats = %+v", body.Reports.Severity)
	}
	rate, err := body.Reports.SuccessRate.Get()
	if err != nil {
		t.Fatalf("success_rate should be set: %v", err)
	}
	if rate != 0.25 {
		t.Fatalf("success_rate = %v, want 0.25", rate)
	}
}

func TestListAlertSourceProfilesReturnsProfiles(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			alertSourceProfiles: []domain.AlertSourceProfile{
				{
					ID:        1,
					Name:      "Primary Prometheus",
					Kind:      domain.AlertSourceKindPrometheus,
					BaseURL:   "https://prometheus.example.test",
					AuthMode:  domain.AlertSourceAuthModeBearer,
					SecretRef: "secret/openclarion/prometheus-bearer",
					Enabled:   false,
					Labels:    map[string]string{"env": "staging"},
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/alert-sources?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.AlertSourceProfileListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(body.Items))
	}
	got := body.Items[0]
	if got.Name != "Primary Prometheus" || got.Kind != api.Prometheus || got.SecretRef != "secret/openclarion/prometheus-bearer" {
		t.Fatalf("unexpected profile: %+v", got)
	}
}

func TestCreateAlertSourceProfileSavesSanitizedProfile(t *testing.T) {
	repo := &fakeConfigRepo{
		saveResult: domain.AlertSourceProfile{
			ID:        1,
			Name:      "Primary Prometheus",
			Kind:      domain.AlertSourceKindPrometheus,
			BaseURL:   "https://prometheus.example.test",
			AuthMode:  domain.AlertSourceAuthModeBearer,
			SecretRef: "secret/openclarion/prometheus-bearer",
			Enabled:   true,
			Labels:    map[string]string{"env": "staging"},
			CreatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":" Primary Prometheus ",
		"kind":"prometheus",
		"base_url":"https://prometheus.example.test",
		"auth_mode":"bearer",
		"secret_ref":"secret/openclarion/prometheus-bearer",
		"enabled":true,
		"labels":{" env ":" staging "}
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/alert-sources", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.saved.Name != "Primary Prometheus" || repo.saved.Labels["env"] != "staging" {
		t.Fatalf("saved profile was not normalized: %+v", repo.saved)
	}
	var resp api.AlertSourceProfile
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.ID != 1 || resp.SecretRef != "secret/openclarion/prometheus-bearer" || !resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReplaceAlertSourceProfileDisablesSource(t *testing.T) {
	repo := &fakeConfigRepo{
		updateResult: domain.AlertSourceProfile{
			ID:        7,
			Name:      "Primary Alertmanager",
			Kind:      domain.AlertSourceKindAlertmanager,
			BaseURL:   "https://alertmanager.example.test",
			AuthMode:  domain.AlertSourceAuthModeNone,
			Enabled:   false,
			Labels:    map[string]string{},
			CreatedAt: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 6, 5, 3, 5, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":"Primary Alertmanager",
		"kind":"alertmanager",
		"base_url":"https://alertmanager.example.test",
		"auth_mode":"none",
		"enabled":false
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/alert-sources/7", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updated.ID != 7 || repo.updated.Enabled {
		t.Fatalf("updated = %+v, want id 7 disabled", repo.updated)
	}
}

func TestAlertSourceProfileConnectionTestReturnsSanitizedResult(t *testing.T) {
	checkedAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{
				ID:        1,
				Name:      "Primary Prometheus",
				Kind:      domain.AlertSourceKindPrometheus,
				BaseURL:   "https://prometheus.example.test",
				AuthMode:  domain.AlertSourceAuthModeBearer,
				SecretRef: "secret/openclarion/prometheus-bearer",
				Enabled:   true,
				Labels:    map[string]string{"env": "prod"},
				CreatedAt: checkedAt,
				UpdatedAt: checkedAt,
			},
		},
	}
	tester := &fakeAlertSourceConnectionTester{
		result: alertsourcecheck.Result{
			SourceID:       1,
			Kind:           domain.AlertSourceKindPrometheus,
			AuthMode:       domain.AlertSourceAuthModeBearer,
			Status:         alertsourcecheck.StatusBlocked,
			ReasonCode:     alertsourcecheck.ReasonCredentialsUnavailable,
			Message:        "Secret-backed connection tests require a server-side secret resolver.",
			CheckedAt:      checkedAt,
			ObservedAlerts: 0,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/alert-sources/1/test", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}, WithAlertSourceConnectionTester(tester)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if tester.called != 1 || tester.profile.ID != 1 || tester.profile.BaseURL == "" {
		t.Fatalf("tester called=%d profile=%+v", tester.called, tester.profile)
	}
	if body := rec.Body.String(); strings.Contains(body, "https://prometheus.example.test") || strings.Contains(body, "secret/openclarion") {
		t.Fatalf("response leaked endpoint or secret reference: %s", body)
	}
	var resp api.AlertSourceConnectionTestResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Status != api.AlertSourceConnectionTestStatusBlocked ||
		resp.ReasonCode != api.AlertSourceConnectionTestReasonCodeCredentialsUnavailable {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAlertSourceProfileConnectionTestRejectsUnconfiguredAndMissingProfiles(t *testing.T) {
	tests := []struct {
		name       string
		repo       *fakeConfigRepo
		opts       []ServerOption
		wantStatus int
	}{
		{
			name:       "unconfigured",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:       "not_found",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithAlertSourceConnectionTester(&fakeAlertSourceConnectionTester{})},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithAlertSourceConnectionTester(&fakeAlertSourceConnectionTester{})},
			wantStatus: stdhttp.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/config/alert-sources/99/test"
			if tc.name == "invalid_id" {
				path = "/api/v1/config/alert-sources/0/test"
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, path, nil)
			testHandler(&fakeUOWFactory{configRepo: tc.repo}, tc.opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestAlertSourceProfileWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		repoErr    error
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none","extra":true}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "userinfo_url",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://user@prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "bearer_missing_secret_ref",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"bearer"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "duplicate_name",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/alert-sources",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusConflict,
			repoErr:    domain.ErrAlreadyExists,
		},
		{
			name:       "replace_not_found",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/alert-sources/99",
			body:       `{"name":"Prom","kind":"prometheus","base_url":"https://prometheus.example.test","auth_mode":"none"}`,
			wantStatus: stdhttp.StatusNotFound,
			repoErr:    domain.ErrNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{saveErr: tc.repoErr, updateErr: tc.repoErr}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListGroupingPoliciesReturnsPolicies(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			groupingPolicies: []domain.GroupingPolicy{
				{
					ID:            1,
					Name:          "Default alert grouping",
					DimensionKeys: []string{"alertname", "service"},
					SeverityKey:   "severity",
					SourceFilter:  []string{"prometheus"},
					Enabled:       true,
					CreatedAt:     createdAt,
					UpdatedAt:     createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/grouping-policies?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.GroupingPolicyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Default alert grouping" {
		t.Fatalf("body = %+v", body)
	}
}

func TestCreateGroupingPolicySavesNormalizedPolicy(t *testing.T) {
	repo := &fakeConfigRepo{
		saveGroupingPolicyResult: domain.GroupingPolicy{
			ID:            1,
			Name:          "Default alert grouping",
			DimensionKeys: []string{"alertname", "service"},
			SeverityKey:   "severity",
			SourceFilter:  []string{"prometheus"},
			Enabled:       true,
			CreatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
			UpdatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":" Default alert grouping ",
		"dimension_keys":["service","alertname","service"],
		"severity_key":" severity ",
		"source_filter":["prometheus","prometheus"],
		"enabled":true
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/grouping-policies", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedGroupingPolicy.Name != "Default alert grouping" ||
		len(repo.savedGroupingPolicy.DimensionKeys) != 2 ||
		repo.savedGroupingPolicy.DimensionKeys[0] != "alertname" ||
		repo.savedGroupingPolicy.SeverityKey != "severity" {
		t.Fatalf("saved policy was not normalized: %+v", repo.savedGroupingPolicy)
	}
	var resp api.GroupingPolicy
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.ID != 1 || !resp.Enabled || len(resp.SourceFilter) != 1 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReplaceGroupingPolicyDisablesPolicy(t *testing.T) {
	repo := &fakeConfigRepo{
		updateGroupingPolicyResult: domain.GroupingPolicy{
			ID:            7,
			Name:          "Service grouping",
			DimensionKeys: []string{"service"},
			SeverityKey:   "severity",
			SourceFilter:  []string{},
			Enabled:       false,
			CreatedAt:     time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC),
			UpdatedAt:     time.Date(2026, 6, 5, 4, 5, 0, 0, time.UTC),
		},
	}
	body := `{
		"name":"Service grouping",
		"dimension_keys":["service"],
		"severity_key":"severity",
		"enabled":false
	}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPut, "/api/v1/config/grouping-policies/7", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedGroupingPolicy.ID != 7 || repo.updatedGroupingPolicy.Enabled {
		t.Fatalf("updated = %+v, want id 7 disabled", repo.updatedGroupingPolicy)
	}
}

func TestPreviewGroupingPolicyReturnsBoundedGroups(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		groupingPolicies: []domain.GroupingPolicy{
			{
				ID:            1,
				Name:          "Default alert grouping",
				DimensionKeys: []string{"alertname"},
				SeverityKey:   "severity",
				SourceFilter:  []string{"prometheus"},
				Enabled:       true,
				CreatedAt:     createdAt,
				UpdatedAt:     createdAt,
			},
		},
	}
	alerts := &fakeAlertRepo{
		events: []domain.AlertEvent{
			{
				ID:       101,
				Source:   "prometheus",
				Labels:   map[string]string{"alertname": "HighCPU", "severity": "warning"},
				StartsAt: createdAt,
			},
			{
				ID:       102,
				Source:   "prometheus",
				Labels:   map[string]string{"alertname": "HighCPU", "severity": "critical"},
				StartsAt: createdAt.Add(time.Minute),
			},
			{
				ID:       103,
				Source:   "alertmanager",
				Labels:   map[string]string{"alertname": "DiskFull", "severity": "info"},
				StartsAt: createdAt.Add(2 * time.Minute),
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/grouping-policies/1/preview?limit=3", nil)
	testHandler(&fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if alerts.lastLimit != 3 {
		t.Fatalf("alert list limit = %d, want 3", alerts.lastLimit)
	}
	var resp api.GroupingPolicyPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.PolicyID != 1 || resp.EventsScanned != 3 || resp.EventsMatched != 2 {
		t.Fatalf("response counts = %+v", resp)
	}
	if len(resp.Groups) != 1 || resp.Groups[0].Severity != api.GroupingPolicyPreviewSeverityCritical {
		t.Fatalf("groups = %+v", resp.Groups)
	}
	if resp.Groups[0].Dimensions["alertname"] != "HighCPU" || len(resp.Groups[0].EventIds) != 2 {
		t.Fatalf("group detail = %+v", resp.Groups[0])
	}
}

func TestGroupingPolicyWriteAndPreviewRejectInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		repoErr    error
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity","extra":true}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "dimension_whitespace",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alert name"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "duplicate_name",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusConflict,
			repoErr:    domain.ErrAlreadyExists,
		},
		{
			name:       "replace_not_found",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/grouping-policies/99",
			body:       `{"name":"Policy","dimension_keys":["alertname"],"severity_key":"severity"}`,
			wantStatus: stdhttp.StatusNotFound,
			repoErr:    domain.ErrNotFound,
		},
		{
			name:       "preview_invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies/0/preview",
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "preview_not_found",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/grouping-policies/99/preview",
			wantStatus: stdhttp.StatusNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{
				saveErr:   tc.repoErr,
				updateErr: tc.repoErr,
			}
			rec := httptest.NewRecorder()
			var body *strings.Reader
			if tc.body == "" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, body)
			testHandler(&fakeUOWFactory{configRepo: repo, alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestGetReport_ReturnsDetailWithLinkedSubReports(t *testing.T) {
	finalCreatedAt := time.Date(2026, 5, 27, 10, 5, 0, 0, time.UTC)
	subCreatedAt := finalCreatedAt.Add(-time.Minute)
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{
			finalReports: []domain.FinalReport{
				{
					ID:                 11,
					CorrelationKey:     "window:checkout",
					Title:              "Checkout latency incident",
					ExecutiveSummary:   "Checkout latency increased after a deployment.",
					Severity:           domain.ReportSeverityWarning,
					Confidence:         domain.ReportConfidenceHigh,
					SubReports:         json.RawMessage(`[{"title":"Checkout API latency","severity":"warning","summary":"p95 latency exceeded the warning threshold."}]`),
					RecommendedActions: json.RawMessage(`[{"label":"Inspect deployment","detail":"Compare deployment timestamps.","priority":"high"}]`),
					NotificationText:   "Checkout latency incident requires review.",
					Content:            json.RawMessage(`{"title":"Checkout latency incident","executive_summary":"Checkout latency increased after a deployment."}`),
					Model:              "gpt-4.1-mini",
					OutputMode:         "json_schema",
					CreatedByWorkflow:  "FinalReportWorkflow",
					CreatedAt:          finalCreatedAt,
				},
			},
			linkedSubReports: map[domain.FinalReportID][]domain.SubReport{
				11: {
					{
						ID:                 21,
						EvidenceSnapshotID: 7,
						Scenario:           "single_alert",
						Title:              "Checkout API latency",
						Summary:            "p95 latency exceeded the warning threshold.",
						Severity:           domain.ReportSeverityWarning,
						Confidence:         domain.ReportConfidenceHigh,
						Findings:           json.RawMessage(`[{"label":"High p95 latency","detail":"p95 latency stayed above threshold.","evidence_id":"alert:checkout-latency"}]`),
						RecommendedActions: json.RawMessage(`[{"label":"Inspect deployment","detail":"Compare deployment timestamps.","priority":"high"}]`),
						EvidenceRefs:       []string{"alert:checkout-latency"},
						Content:            json.RawMessage(`{"title":"Checkout API latency","summary":"p95 latency exceeded the warning threshold."}`),
						Model:              "gpt-4.1-mini",
						OutputMode:         "json_schema",
						CreatedByWorkflow:  "ReportFanOutWorkflow",
						CreatedAt:          subCreatedAt,
					},
				},
			},
		},
		diagnosisRepo: &fakeDiagnosisRepo{
			tasksBySnapshot: map[domain.EvidenceSnapshotID][]domain.DiagnosisTask{
				7: {
					{
						ID:                 31,
						EvidenceSnapshotID: 7,
						WorkflowID:         "diagnosis-room-31",
						RunID:              "run-31",
						Status:             domain.DiagnosisStatusSucceeded,
						CreatedAt:          finalCreatedAt.Add(2 * time.Minute),
					},
				},
			},
			eventsByTaskAndKind: map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent{
				31: {
					diagnosisConclusionEventFinalReady: {
						{
							ID:     41,
							TaskID: 31,
							Kind:   diagnosisConclusionEventFinalReady,
							Payload: json.RawMessage(`{
									"kind":"diagnosis_room.final_conclusion_ready",
									"session_id":"diagnosis-session-31",
									"chat_session_id":51,
									"diagnosis_task_id":31,
									"owner_subject":"owner-1",
									"turn_count":1,
									"final_conclusion":{
										"status":"available",
										"source":"latest_assistant_turn",
										"reason":"assistant_marked_final",
										"assistant_turn_id":61,
										"assistant_message_id":"msg-1/assistant",
										"assistant_sequence":2,
										"assistant_occurred_at":"2026-05-27T10:08:00Z",
										"content":"Checkout latency remains correlated with the deployment.",
										"confidence":"high",
										"requires_human_review":true
									},
									"conclusion_version":"diagnosis-room-final-ready.v1"
								}`),
							OccurredAt: finalCreatedAt.Add(3 * time.Minute),
							RecordedAt: finalCreatedAt.Add(3*time.Minute + time.Second),
						},
					},
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/11", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.reportRepo.lastSubReportsLimit != maxListLimit {
		t.Fatalf("linked subreport limit = %d, want %d", factory.reportRepo.lastSubReportsLimit, maxListLimit)
	}

	var body api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ID != 11 || body.Content["title"] != "Checkout latency incident" {
		t.Fatalf("unexpected report detail: %+v", body)
	}
	if len(body.SubReports) != 1 || body.SubReports[0].Title != "Checkout API latency" {
		t.Fatalf("unexpected embedded sub_reports: %+v", body.SubReports)
	}
	if len(body.LinkedSubReports) != 1 {
		t.Fatalf("len(linked_sub_reports) = %d, want 1", len(body.LinkedSubReports))
	}
	linked := body.LinkedSubReports[0]
	if linked.ID != 21 || linked.EvidenceSnapshotID != 7 || len(linked.Findings) != 1 || linked.Findings[0].EvidenceID != "alert:checkout-latency" {
		t.Fatalf("unexpected linked subreport: %+v", linked)
	}
	conclusion := linked.DiagnosisConclusion
	if conclusion == nil {
		t.Fatalf("diagnosis_conclusion is nil, want latest conclusion")
	}
	if conclusion.DiagnosisTaskID != 31 ||
		conclusion.SessionID != "diagnosis-session-31" ||
		conclusion.ChatSessionID != 51 ||
		conclusion.Content != "Checkout latency remains correlated with the deployment." ||
		conclusion.Confidence == nil ||
		*conclusion.Confidence != api.ReportConfidenceHigh ||
		conclusion.RequiresHumanReview == nil ||
		!*conclusion.RequiresHumanReview {
		t.Fatalf("unexpected diagnosis conclusion: %+v", conclusion)
	}
}

func TestGetReport_ReturnsNotFound(t *testing.T) {
	factory := &fakeUOWFactory{
		reportRepo: &fakeReportRepo{},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/reports/999", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestListReportWorkflowPoliciesReturnsPolicies(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
				{
					ID:                                 1,
					Name:                               "Default report workflow",
					AlertSourceProfileID:               1,
					GroupingPolicyID:                   2,
					ReportNotificationChannelProfileID: 3,
					TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
					ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
					DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
					Enabled:                            false,
					CreatedAt:                          createdAt,
					UpdatedAt:                          createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/report-workflow-policies?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.ReportWorkflowPolicyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Default report workflow" || body.Items[0].Enabled {
		t.Fatalf("items = %+v", body.Items)
	}
	if !body.Items[0].EnabledAt.IsNull() || !body.Items[0].DisabledAt.IsNull() {
		t.Fatalf("enablement timestamps should be explicit null: %+v", body.Items[0])
	}
	reportChannelID, err := body.Items[0].ReportNotificationChannelProfileID.Get()
	if err != nil || reportChannelID != 3 {
		t.Fatalf("report notification channel ID = %v, %v; want 3", reportChannelID, err)
	}
}

func TestCreateReportWorkflowPolicyStoresDisabledDraft(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles:         []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:            []domain.GroupingPolicy{{ID: 2, Enabled: false}},
		notificationChannelProfiles: []domain.NotificationChannelProfile{{ID: 3, Enabled: false}},
		saveReportWorkflowPolicyResult: domain.ReportWorkflowPolicy{
			ID:                                 7,
			Name:                               "Default report workflow",
			AlertSourceProfileID:               1,
			GroupingPolicyID:                   2,
			ReportNotificationChannelProfileID: 3,
			TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
			ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
			DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
			Enabled:                            false,
			CreatedAt:                          time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC),
			UpdatedAt:                          time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC),
		},
	}

	body := `{
		"name":"Default report workflow",
		"alert_source_profile_id":1,
		"grouping_policy_id":2,
		"report_notification_channel_profile_id":3,
		"trigger_mode":"manual_replay",
		"report_scenario":"single_alert",
		"diagnosis_follow_up":"suggest_room"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedReportWorkflowPolicy.Enabled {
		t.Fatalf("saved policy should be disabled: %+v", repo.savedReportWorkflowPolicy)
	}
	if repo.savedReportWorkflowPolicy.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("saved report notification channel ID = %d, want 3", repo.savedReportWorkflowPolicy.ReportNotificationChannelProfileID)
	}
	var resp api.ReportWorkflowPolicy
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 7 || resp.ReportScenario != api.ReportWorkflowScenarioSingleAlert || resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
	reportChannelID, err := resp.ReportNotificationChannelProfileID.Get()
	if err != nil || reportChannelID != 3 {
		t.Fatalf("response report notification channel ID = %v, %v; want 3", reportChannelID, err)
	}
}

func TestEnableReportWorkflowPolicyRequiresReadyBindings(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1, Enabled: false}},
		groupingPolicies:    []domain.GroupingPolicy{{ID: 2, Enabled: true}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/enable", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if repo.updatedReportWorkflowPolicy.ID != 0 {
		t.Fatalf("policy should not be updated on failed enablement: %+v", repo.updatedReportWorkflowPolicy)
	}
}

func TestEnableAndDisableReportWorkflowPolicyToggleState(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1, Enabled: true}},
		groupingPolicies:    []domain.GroupingPolicy{{ID: 2, Enabled: true}},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	enableRec := httptest.NewRecorder()
	enableReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/enable", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(enableRec, enableReq)
	if enableRec.Code != stdhttp.StatusOK {
		t.Fatalf("enable status = %d, want 200; body=%s", enableRec.Code, enableRec.Body.String())
	}
	var enabled api.ReportWorkflowPolicy
	if err := json.NewDecoder(enableRec.Body).Decode(&enabled); err != nil {
		t.Fatalf("decode enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt.IsNull() || !enabled.DisabledAt.IsNull() {
		t.Fatalf("enabled response = %+v", enabled)
	}

	disableRec := httptest.NewRecorder()
	disableReq := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/disable", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(disableRec, disableReq)
	if disableRec.Code != stdhttp.StatusOK {
		t.Fatalf("disable status = %d, want 200; body=%s", disableRec.Code, disableRec.Body.String())
	}
	var disabled api.ReportWorkflowPolicy
	if err := json.NewDecoder(disableRec.Body).Decode(&disabled); err != nil {
		t.Fatalf("decode disable: %v", err)
	}
	if disabled.Enabled || !disabled.EnabledAt.IsNull() || disabled.DisabledAt.IsNull() {
		t.Fatalf("disabled response = %+v", disabled)
	}
}

func TestPreviewReportWorkflowPolicyImpactReturnsReadinessAndGroupingImpact(t *testing.T) {
	base := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{
				ID:       1,
				Kind:     domain.AlertSourceKindPrometheus,
				AuthMode: domain.AlertSourceAuthModeBearer,
				Enabled:  true,
			},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{
				ID:            2,
				DimensionKeys: []string{"alertname"},
				SeverityKey:   "severity",
				SourceFilter:  []string{"prometheus"},
				Enabled:       true,
			},
		},
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:             3,
				Enabled:        true,
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
			},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                                 7,
				Name:                               "Default report workflow",
				AlertSourceProfileID:               1,
				GroupingPolicyID:                   2,
				ReportNotificationChannelProfileID: 3,
				TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:                     domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
			},
		},
	}
	alerts := &fakeAlertRepo{
		events: []domain.AlertEvent{
			{
				ID:       101,
				Source:   "prometheus",
				Labels:   map[string]string{"alertname": "checkout", "severity": "critical"},
				StartsAt: base,
			},
			{
				ID:       102,
				Source:   "alertmanager",
				Labels:   map[string]string{"alertname": "payments", "severity": "warning"},
				StartsAt: base.Add(time.Minute),
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview?limit=2", nil)
	testHandler(&fakeUOWFactory{configRepo: repo, alertRepo: alerts}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if alerts.lastLimit != 2 {
		t.Fatalf("alert limit = %d, want 2", alerts.lastLimit)
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.PolicyID != 7 ||
		body.Status != api.ReportWorkflowPolicyImpactPreviewStatusReady ||
		len(body.ReasonCodes) != 1 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeOk {
		t.Fatalf("readiness = %+v", body)
	}
	channelID, err := body.ReportNotificationChannelProfileID.Get()
	if err != nil || channelID != 3 {
		t.Fatalf("channel ID = %v, %v; want 3", channelID, err)
	}
	if !body.ReportNotificationChannelBound ||
		!body.ReportNotificationChannelEnabled ||
		!body.ReportNotificationChannelHasReportScope {
		t.Fatalf("channel readiness = %+v", body)
	}
	if body.EventsScanned != 2 || body.EventsMatched != 1 || body.GroupsEstimated != 1 || len(body.Groups) != 1 {
		t.Fatalf("impact counts = %+v", body)
	}
	if body.Groups[0].EventCount != 1 || body.Groups[0].Dimensions["alertname"] != "checkout" {
		t.Fatalf("group = %+v", body.Groups[0])
	}
}

func TestPreviewReportWorkflowPolicyImpactReturnsBlockedReadiness(t *testing.T) {
	repo := &fakeConfigRepo{
		alertSourceProfiles: []domain.AlertSourceProfile{
			{ID: 1, Kind: domain.AlertSourceKindPrometheus, AuthMode: domain.AlertSourceAuthModeNone, Enabled: false},
		},
		groupingPolicies: []domain.GroupingPolicy{
			{ID: 2, DimensionKeys: []string{"alertname"}, SeverityKey: "severity", SourceFilter: []string{"prometheus"}, Enabled: false},
		},
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{
			{
				ID:                   7,
				Name:                 "Default report workflow",
				AlertSourceProfileID: 1,
				GroupingPolicyID:     2,
				TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
				ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
				DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/impact-preview", nil)
	testHandler(&fakeUOWFactory{configRepo: repo, alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ReportWorkflowPolicyImpactPreviewResult
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != api.ReportWorkflowPolicyImpactPreviewStatusBlocked ||
		len(body.ReasonCodes) != 2 ||
		body.ReasonCodes[0] != api.ReportWorkflowPolicyImpactPreviewReasonCodeAlertSourceDisabled ||
		body.ReasonCodes[1] != api.ReportWorkflowPolicyImpactPreviewReasonCodeGroupingPolicyDisabled {
		t.Fatalf("readiness = %+v", body)
	}
	if !body.ReportNotificationChannelProfileID.IsNull() || body.ReportNotificationChannelBound {
		t.Fatalf("unbound channel fields = %+v", body)
	}
}

func TestReportWorkflowPolicyWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_scenario",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"report_scenario":"unknown"}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_notification_channel_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies",
			body:       `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2,"report_notification_channel_profile_id":0}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:   "missing_binding",
			method: stdhttp.MethodPost,
			path:   "/api/v1/config/report-workflow-policies",
			body:   `{"name":"Default","alert_source_profile_id":1,"grouping_policy_id":2}`,
			repo: &fakeConfigRepo{
				alertSourceProfiles: []domain.AlertSourceProfile{{ID: 1}},
			},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies/0/enable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_policy",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-policies/99/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListReportWorkflowSchedulesReturnsSchedules(t *testing.T) {
	createdAt := time.Date(2026, 6, 6, 2, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			reportWorkflowSchedules: []domain.ReportWorkflowSchedule{
				{
					ID:                     1,
					Name:                   "Daily report window",
					ReportWorkflowPolicyID: 7,
					TemporalScheduleID:     "openclarion-report-policy-7-daily",
					Interval:               24 * time.Hour,
					Offset:                 6 * time.Hour,
					ReplayWindow:           time.Hour,
					ReplayDelay:            5 * time.Minute,
					ReplayLimit:            10000,
					CatchupWindow:          time.Hour,
					Enabled:                false,
					CreatedAt:              createdAt,
					UpdatedAt:              createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/report-workflow-schedules?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.ReportWorkflowScheduleListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Daily report window" {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].IntervalSeconds != 86400 ||
		body.Items[0].OffsetSeconds != 21600 ||
		body.Items[0].ReplayWindowSeconds != 3600 ||
		body.Items[0].ReplayDelaySeconds != 300 ||
		body.Items[0].CatchupWindowSeconds != 3600 {
		t.Fatalf("duration fields = %+v", body.Items[0])
	}
}

func TestCreateReportWorkflowScheduleStoresSecondsAsDurations(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedReportWorkflowSchedule.ReportWorkflowPolicyID != 7 ||
		repo.savedReportWorkflowSchedule.Interval != 24*time.Hour ||
		repo.savedReportWorkflowSchedule.Offset != 6*time.Hour ||
		repo.savedReportWorkflowSchedule.ReplayWindow != time.Hour ||
		repo.savedReportWorkflowSchedule.ReplayDelay != 5*time.Minute ||
		repo.savedReportWorkflowSchedule.ReplayLimit != 10000 ||
		repo.savedReportWorkflowSchedule.CatchupWindow != time.Hour {
		t.Fatalf("saved schedule = %+v", repo.savedReportWorkflowSchedule)
	}
	var response api.ReportWorkflowSchedule
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if response.TemporalScheduleID != "openclarion-report-policy-7-daily" || response.Enabled {
		t.Fatalf("response = %+v", response)
	}
}

func TestReportWorkflowScheduleMutationSynchronizesSavedSchedule(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testHandler(
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if syncer.calls != 1 ||
		syncer.schedule.ID == 0 ||
		syncer.schedule.TemporalScheduleID != "openclarion-report-policy-7-daily" ||
		syncer.schedule.Enabled {
		t.Fatalf("syncer = %+v", syncer)
	}
}

func TestReportWorkflowScheduleSyncFailureReturnsServerError(t *testing.T) {
	repo := &fakeConfigRepo{
		reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{err: errors.New("temporal unavailable")}
	body := `{
		"name":"Daily report window",
		"report_workflow_policy_id":7,
		"temporal_schedule_id":"openclarion-report-policy-7-daily",
		"interval_seconds":86400,
		"offset_seconds":21600,
		"replay_window_seconds":3600,
		"replay_delay_seconds":300,
		"replay_limit":10000,
		"catchup_window_seconds":3600
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules", strings.NewReader(body))
	testHandler(
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "temporal unavailable") {
		t.Fatalf("response leaked synchronizer error: %s", rec.Body.String())
	}
	if syncer.calls != 1 || repo.savedReportWorkflowSchedule.Name != "Daily report window" {
		t.Fatalf("sync calls = %d saved = %+v", syncer.calls, repo.savedReportWorkflowSchedule)
	}
}

func TestEnableReportWorkflowScheduleRequiresEnabledPolicy(t *testing.T) {
	schedule := domain.ReportWorkflowSchedule{
		ID:                     9,
		Name:                   "Daily report window",
		ReportWorkflowPolicyID: 7,
		TemporalScheduleID:     "openclarion-report-policy-7-daily",
		Interval:               24 * time.Hour,
		Offset:                 6 * time.Hour,
		ReplayWindow:           time.Hour,
		ReplayDelay:            5 * time.Minute,
		ReplayLimit:            10000,
		CatchupWindow:          time.Hour,
	}
	repo := &fakeConfigRepo{
		reportWorkflowPolicies:  []domain.ReportWorkflowPolicy{{ID: 7, Enabled: true}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{schedule},
	}
	syncer := &recordingReportWorkflowScheduleSyncer{}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules/9/enable", nil)
	testHandler(
		&fakeUOWFactory{configRepo: repo},
		WithReportWorkflowScheduleSynchronizer(syncer),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !repo.updatedReportWorkflowSchedule.Enabled || repo.updatedReportWorkflowSchedule.EnabledAt == nil {
		t.Fatalf("updated schedule = %+v", repo.updatedReportWorkflowSchedule)
	}
	if syncer.calls != 1 || !syncer.schedule.Enabled || syncer.schedule.EnabledAt == nil {
		t.Fatalf("syncer = %+v", syncer)
	}

	repo = &fakeConfigRepo{
		reportWorkflowPolicies:  []domain.ReportWorkflowPolicy{{ID: 7, Enabled: false}},
		reportWorkflowSchedules: []domain.ReportWorkflowSchedule{schedule},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-schedules/9/enable", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("disabled policy status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportWorkflowScheduleWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600,"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_offset",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":3600,"offset_seconds":3600,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{reportWorkflowPolicies: []domain.ReportWorkflowPolicy{{ID: 7}}},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "negative_policy_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":-7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "oversized_duration",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":31536001,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "oversized_replay_limit",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":100001,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_policy",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules",
			body:       `{"name":"Daily","report_workflow_policy_id":7,"temporal_schedule_id":"schedule-1","interval_seconds":86400,"offset_seconds":0,"replay_window_seconds":3600,"replay_delay_seconds":300,"replay_limit":10000,"catchup_window_seconds":3600}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules/0/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_schedule",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/report-workflow-schedules/99/disable",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestListNotificationChannelProfilesReturnsProfiles(t *testing.T) {
	createdAt := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	factory := &fakeUOWFactory{
		configRepo: &fakeConfigRepo{
			notificationChannelProfiles: []domain.NotificationChannelProfile{
				{
					ID:             1,
					Name:           "Operations webhook",
					Kind:           domain.NotificationChannelKindWebhook,
					SecretRef:      "secret/openclarion/ops-webhook",
					DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
					Enabled:        true,
					Labels:         map[string]string{"owner": "sre"},
					CreatedAt:      createdAt,
					UpdatedAt:      createdAt,
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/config/notification-channels?limit=1", nil)
	testHandler(factory).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if factory.configRepo.lastListLimit != 1 {
		t.Fatalf("repo limit = %d, want 1", factory.configRepo.lastListLimit)
	}
	var body api.NotificationChannelProfileListResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "Operations webhook" || !body.Items[0].Enabled {
		t.Fatalf("items = %+v", body.Items)
	}
	if body.Items[0].SecretRef != "secret/openclarion/ops-webhook" || body.Items[0].DeliveryScopes[0] != api.Report {
		t.Fatalf("profile = %+v", body.Items[0])
	}
}

func TestCreateNotificationChannelProfileStoresSecretRefOnly(t *testing.T) {
	repo := &fakeConfigRepo{
		saveNotificationChannelResult: domain.NotificationChannelProfile{
			ID:             7,
			Name:           "Operations webhook",
			Kind:           domain.NotificationChannelKindWebhook,
			SecretRef:      "secret/openclarion/ops-webhook",
			DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose, domain.NotificationDeliveryScopeReport},
			Enabled:        false,
			Labels:         map[string]string{"owner": "sre"},
			CreatedAt:      time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC),
		},
	}

	body := `{
		"name":"Operations webhook",
		"kind":"webhook",
		"secret_ref":"secret/openclarion/ops-webhook",
		"delivery_scopes":["diagnosis_close","report"],
		"enabled":false,
		"labels":{"owner":"sre"}
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/notification-channels", strings.NewReader(body))
	testHandler(&fakeUOWFactory{configRepo: repo}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if repo.savedNotificationChannel.SecretRef != "secret/openclarion/ops-webhook" {
		t.Fatalf("saved notification channel = %+v", repo.savedNotificationChannel)
	}
	if len(repo.savedNotificationChannel.DeliveryScopes) != 2 ||
		repo.savedNotificationChannel.DeliveryScopes[0] != domain.NotificationDeliveryScopeDiagnosisClose ||
		repo.savedNotificationChannel.DeliveryScopes[1] != domain.NotificationDeliveryScopeReport {
		t.Fatalf("saved scopes = %+v", repo.savedNotificationChannel.DeliveryScopes)
	}
	var resp api.NotificationChannelProfile
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != 7 || resp.Kind != api.Webhook || resp.Enabled {
		t.Fatalf("response = %+v", resp)
	}
}

func TestNotificationChannelProfileWriteRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		repo       *fakeConfigRepo
		wantStatus int
	}{
		{
			name:       "unknown_field",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"],"extra":true}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_secret_ref",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "empty_delivery_scopes",
			method:     stdhttp.MethodPost,
			path:       "/api/v1/config/notification-channels",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":[]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "invalid_id",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/notification-channels/0",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "missing_channel",
			method:     stdhttp.MethodPut,
			path:       "/api/v1/config/notification-channels/99",
			body:       `{"name":"Operations webhook","kind":"webhook","secret_ref":"secret/ref","delivery_scopes":["report"]}`,
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{configRepo: tc.repo}).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestNotificationChannelProfileTestReturnsSanitizedResult(t *testing.T) {
	checkedAt := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	repo := &fakeConfigRepo{
		notificationChannelProfiles: []domain.NotificationChannelProfile{
			{
				ID:             1,
				Name:           "Operations webhook",
				Kind:           domain.NotificationChannelKindWebhook,
				SecretRef:      "secret/openclarion/ops-webhook",
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
				Enabled:        false,
				Labels:         map[string]string{"owner": "sre"},
				CreatedAt:      checkedAt,
				UpdatedAt:      checkedAt,
			},
		},
	}
	tester := &fakeNotificationChannelTester{
		result: notificationchannelcheck.Result{
			ChannelID:         1,
			Kind:              domain.NotificationChannelKindWebhook,
			Status:            notificationchannelcheck.StatusBlocked,
			ReasonCode:        notificationchannelcheck.ReasonCredentialsUnavailable,
			Message:           "Secret-backed notification channel tests require a server-side secret resolver.",
			CheckedAt:         checkedAt,
			ProviderMessageID: "",
			ProviderStatus:    "",
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/notification-channels/1/test", nil)
	testHandler(&fakeUOWFactory{configRepo: repo}, WithNotificationChannelTester(tester)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if tester.called != 1 || tester.profile.ID != 1 || tester.profile.SecretRef == "" {
		t.Fatalf("tester called=%d profile=%+v", tester.called, tester.profile)
	}
	if body := rec.Body.String(); strings.Contains(body, "secret/openclarion") || strings.Contains(body, "ops-webhook") {
		t.Fatalf("response leaked secret reference: %s", body)
	}
	var resp api.NotificationChannelTestResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Status != api.NotificationChannelTestStatusBlocked ||
		resp.ReasonCode != api.NotificationChannelTestReasonCodeCredentialsUnavailable {
		t.Fatalf("response = %+v", resp)
	}
}

func TestNotificationChannelProfileTestRejectsUnconfiguredAndMissingProfiles(t *testing.T) {
	tests := []struct {
		name       string
		repo       *fakeConfigRepo
		opts       []ServerOption
		wantStatus int
	}{
		{
			name:       "unconfigured",
			repo:       &fakeConfigRepo{},
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:       "not_found",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithNotificationChannelTester(&fakeNotificationChannelTester{})},
			wantStatus: stdhttp.StatusNotFound,
		},
		{
			name:       "invalid_id",
			repo:       &fakeConfigRepo{},
			opts:       []ServerOption{WithNotificationChannelTester(&fakeNotificationChannelTester{})},
			wantStatus: stdhttp.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/config/notification-channels/99/test"
			if tc.name == "invalid_id" {
				path = "/api/v1/config/notification-channels/0/test"
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, path, nil)
			testHandler(&fakeUOWFactory{configRepo: tc.repo}, tc.opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestTriggerReportReplay_StartsReportWorkflow(t *testing.T) {
	windowStart := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	trigger := &fakeReportReplayTrigger{
		result: reporttrigger.Result{
			Replay: alertreplay.Result{
				Stats: alertreplay.Stats{
					Ingested:           alertingest.Stats{Total: 2, Saved: 2},
					EventsLoaded:       2,
					GroupsBuilt:        1,
					GroupsSaved:        1,
					SnapshotsSaved:     1,
					SnapshotsDuplicate: 0,
					GroupsClosed:       1,
				},
				Snapshots: []alertreplay.SnapshotRef{
					{ID: 7, GroupIndex: 0, EventCount: 2},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-1", RunID: "run-1"},
			Started:  true,
		},
	}

	body := `{
		"window_start":"2026-05-27T09:00:00Z",
		"window_end":"2026-05-27T10:00:00Z",
		"limit":2,
		"correlation_key":"incident-42",
		"workflow_id":"report-batch-1",
		"scenario":"cascade"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(body))
	testHandler(&fakeUOWFactory{}, WithReportReplayTrigger(trigger)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if trigger.called != 1 {
		t.Fatalf("trigger calls = %d, want 1", trigger.called)
	}
	if !trigger.req.Replay.WindowStart.Equal(windowStart) || !trigger.req.Replay.WindowEnd.Equal(windowEnd) {
		t.Fatalf("trigger window = %s..%s", trigger.req.Replay.WindowStart, trigger.req.Replay.WindowEnd)
	}
	if trigger.req.Replay.Limit != 2 {
		t.Fatalf("trigger limit = %d, want 2", trigger.req.Replay.Limit)
	}
	if trigger.req.Replay.Grouping.SeverityKey != alertgrouping.DefaultConfig().SeverityKey {
		t.Fatalf("trigger grouping = %+v", trigger.req.Replay.Grouping)
	}
	if trigger.req.CorrelationKey != "incident-42" || trigger.req.WorkflowID != "report-batch-1" || trigger.req.Scenario != reportprompt.ScenarioCascade {
		t.Fatalf("trigger identity = %+v", trigger.req)
	}

	var resp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Started || resp.WorkflowID != "report-batch-1" || resp.RunID != "run-1" {
		t.Fatalf("response workflow = %+v", resp)
	}
	if resp.Stats.Ingested.Total != 2 || resp.Stats.SnapshotsSaved != 1 {
		t.Fatalf("response stats = %+v", resp.Stats)
	}
	if len(resp.Snapshots) != 1 || resp.Snapshots[0].ID != 7 || resp.Snapshots[0].EventCount != 2 {
		t.Fatalf("response snapshots = %+v", resp.Snapshots)
	}
}

func TestIngestAlertmanagerWebhook_AcceptsPayload(t *testing.T) {
	ingestor := &fakeAlertmanagerWebhookIngestor{
		result: alertmanagerwebhook.Result{
			ProfileID:       7,
			Received:        2,
			SkippedResolved: 1,
			TruncatedAlerts: 0,
			Ingested:        alertingest.Stats{Total: 1, Saved: 1},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/alert-sources/7/webhooks/alertmanager",
		strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
	)
	req.Header.Set("Authorization", "Bearer webhook-token")
	testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if ingestor.called != 1 {
		t.Fatalf("ingestor calls = %d, want 1", ingestor.called)
	}
	if ingestor.req.ProfileID != 7 || ingestor.req.Authorization != "Bearer webhook-token" {
		t.Fatalf("ingestor request = %+v", ingestor.req)
	}
	if string(ingestor.req.Body) != `{"version":"4","status":"firing","alerts":[]}` {
		t.Fatalf("ingestor body = %s", ingestor.req.Body)
	}

	var body api.AlertmanagerWebhookIngestResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SourceID != 7 || body.Received != 2 || body.SkippedResolved != 1 || body.Ingested.Saved != 1 {
		t.Fatalf("response = %+v", body)
	}
}

func TestIngestAlertmanagerWebhookRejectsUnconfiguredIngestor(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		stdhttp.MethodPost,
		"/api/v1/alert-sources/7/webhooks/alertmanager",
		strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
	)
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestIngestAlertmanagerWebhookMapsUsecaseErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "unauthorized", err: alertmanagerwebhook.ErrUnauthorized, wantStatus: stdhttp.StatusUnauthorized},
		{name: "not_found", err: domain.ErrNotFound, wantStatus: stdhttp.StatusNotFound},
		{name: "invariant", err: domain.ErrInvariantViolation, wantStatus: stdhttp.StatusBadRequest},
		{name: "secret_unavailable", err: alertmanagerwebhook.ErrSecretResolverUnavailable, wantStatus: stdhttp.StatusServiceUnavailable},
		{name: "unexpected", err: errors.New("database unavailable"), wantStatus: stdhttp.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ingestor := &fakeAlertmanagerWebhookIngestor{err: tc.err}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				stdhttp.MethodPost,
				"/api/v1/alert-sources/7/webhooks/alertmanager",
				strings.NewReader(`{"version":"4","status":"firing","alerts":[]}`),
			)
			testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestIngestAlertmanagerWebhookRejectsInvalidTransportRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "invalid_source_id",
			path: "/api/v1/alert-sources/0/webhooks/alertmanager",
			body: `{"version":"4","status":"firing","alerts":[]}`,
		},
		{
			name: "overlarge_body",
			path: "/api/v1/alert-sources/7/webhooks/alertmanager",
			body: strings.Repeat("{", maxJSONRequestBodyBytes+1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ingestor := &fakeAlertmanagerWebhookIngestor{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{}, WithAlertmanagerWebhookIngestor(ingestor)).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if ingestor.called != 0 {
				t.Fatalf("ingestor calls = %d, want 0", ingestor.called)
			}
		})
	}
}

func TestTriggerReportReplayRejectsUnconfiguredTrigger(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(`{}`))
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReportReplayRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "limit_out_of_range",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","limit":0}`,
		},
		{
			name: "unknown_field",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","extra":true}`,
		},
		{
			name: "duplicate_key",
			body: `{"window_start":"2026-05-27T09:00:00Z","window_end":"2026-05-27T10:00:00Z","limit":1,"limit":2}`,
		},
		{
			name: "overlarge_body",
			body: strings.Repeat("{", maxJSONRequestBodyBytes+1),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportReplayTrigger{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{}, WithReportReplayTrigger(trigger)).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if trigger.called != 0 {
				t.Fatalf("trigger calls = %d, want 0", trigger.called)
			}
		})
	}
}

func TestTriggerReportWorkflowPolicyReplay_StartsReportWorkflow(t *testing.T) {
	windowStart := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	trigger := &fakeReportWorkflowPolicyReplayTrigger{
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
					{ID: 9, GroupIndex: 0, EventCount: 1},
				},
			},
			Workflow: ports.WorkflowHandle{WorkflowID: "report-batch-policy-1", RunID: "run-policy-1"},
			Started:  true,
		},
	}

	body := `{
		"window_start":"2026-06-05T08:00:00Z",
		"window_end":"2026-06-05T09:00:00Z",
		"limit":5,
		"correlation_key":"policy-window-1",
		"workflow_id":"report-batch-policy-1"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/replay-window", strings.NewReader(body))
	testHandler(&fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	if trigger.called != 1 {
		t.Fatalf("trigger calls = %d, want 1", trigger.called)
	}
	if trigger.req.PolicyID != 7 ||
		!trigger.req.WindowStart.Equal(windowStart) ||
		!trigger.req.WindowEnd.Equal(windowEnd) ||
		trigger.req.Limit != 5 ||
		trigger.req.CorrelationKey != "policy-window-1" ||
		trigger.req.WorkflowID != "report-batch-policy-1" {
		t.Fatalf("trigger request = %+v", trigger.req)
	}

	var resp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Started || resp.WorkflowID != "report-batch-policy-1" || resp.RunID != "run-policy-1" {
		t.Fatalf("response workflow = %+v", resp)
	}
	if len(resp.Snapshots) != 1 || resp.Snapshots[0].ID != 9 {
		t.Fatalf("response snapshots = %+v", resp.Snapshots)
	}
}

func TestTriggerReportWorkflowPolicyReplayRejectsUnconfiguredTrigger(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/config/report-workflow-policies/7/replay-window", strings.NewReader(`{}`))
	testHandler(&fakeUOWFactory{}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReportWorkflowPolicyReplayRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "limit_out_of_range",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","limit":0}`,
		},
		{
			name: "unknown_field",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","extra":true}`,
		},
		{
			name: "duplicate_key",
			path: "/api/v1/config/report-workflow-policies/7/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z","limit":1,"limit":2}`,
		},
		{
			name: "invalid_policy_id",
			path: "/api/v1/config/report-workflow-policies/0/replay-window",
			body: `{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportWorkflowPolicyReplayTrigger{}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, tc.path, strings.NewReader(tc.body))
			testHandler(&fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			if trigger.called != 0 {
				t.Fatalf("trigger calls = %d, want 0", trigger.called)
			}
		})
	}
}

func TestTriggerReportWorkflowPolicyReplayMapsUsecaseErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not_found", err: domain.ErrNotFound, wantStatus: stdhttp.StatusNotFound},
		{name: "binding_not_found", err: errors.Join(errors.New("binding missing"), domain.ErrNotFound), wantStatus: stdhttp.StatusNotFound},
		{name: "invariant", err: domain.ErrInvariantViolation, wantStatus: stdhttp.StatusBadRequest},
		{name: "unexpected", err: errors.New("temporal unavailable"), wantStatus: stdhttp.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trigger := &fakeReportWorkflowPolicyReplayTrigger{err: tc.err}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(
				context.Background(),
				stdhttp.MethodPost,
				"/api/v1/config/report-workflow-policies/7/replay-window",
				strings.NewReader(`{"window_start":"2026-06-05T08:00:00Z","window_end":"2026-06-05T09:00:00Z"}`),
			)
			testHandler(&fakeUOWFactory{}, WithReportWorkflowPolicyReplayTrigger(trigger)).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestIssueDiagnosisWSTicketAuthenticatesAndIssuesTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("B", diagnosisauth.DefaultTokenBytes)))
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	resolver := &fakeDiagnosisSessionResolver{
		sessions: map[string]diagnosisauth.SessionRef{
			"session-1": {SessionID: "session-1", OwnerSubject: "owner-1"},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/ws-ticket", strings.NewReader(`{"session_id":"session-1"}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, service, resolver),
		withDiagnosisClock(func() time.Time { return now }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if resolver.called != 1 || resolver.sessionID != "session-1" {
		t.Fatalf("resolver called=%d session=%q", resolver.called, resolver.sessionID)
	}

	var body api.DiagnosisWSTicketResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Ticket == "" || strings.ContainsAny(body.Ticket, "+/=") {
		t.Fatalf("ticket = %q, want URL-safe opaque token", body.Ticket)
	}
	if body.SessionID != "session-1" || !body.ExpiresAt.Equal(now.Add(diagnosisauth.DefaultTicketTTL)) {
		t.Fatalf("response = %+v", body)
	}

	consumed, err := service.ConsumeTicket(context.Background(), body.Ticket, resolver.sessions["session-1"], now.Add(time.Second))
	if err != nil {
		t.Fatalf("issued ticket should be consumable: %v", err)
	}
	if consumed.Token != "" || consumed.Subject != "owner-1" {
		t.Fatalf("consumed ticket = %+v, want redacted owner ticket", consumed)
	}
}

func TestIssueDiagnosisWSTicketRejectsBadInputs(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name          string
		authHeader    string
		body          string
		principal     ports.AuthPrincipal
		session       diagnosisauth.SessionRef
		resolverErr   error
		wantStatus    int
		wantAuthCalls int
	}{
		{
			name:       "missing bearer",
			body:       `{"session_id":"session-1"}`,
			session:    diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus: stdhttp.StatusUnauthorized,
		},
		{
			name:          "unknown field",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1","ticket":"nope"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "duplicate session id",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1","session_id":"session-2"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "overlarge body",
			authHeader:    "Bearer oidc-token",
			body:          strings.Repeat("{", maxJSONRequestBodyBytes+1),
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:          "owner cannot issue for another subject",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"session-1"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-2", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			session:       diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"},
			wantStatus:    stdhttp.StatusForbidden,
			wantAuthCalls: 1,
		},
		{
			name:          "session not found",
			authHeader:    "Bearer oidc-token",
			body:          `{"session_id":"missing"}`,
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			resolverErr:   domain.ErrNotFound,
			wantStatus:    stdhttp.StatusNotFound,
			wantAuthCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("C", diagnosisauth.DefaultTokenBytes)))
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer oidc-token": {
					{Principal: tc.principal},
				},
			})
			resolver := &fakeDiagnosisSessionResolver{
				sessions: map[string]diagnosisauth.SessionRef{
					"session-1": tc.session,
				},
				err: tc.resolverErr,
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/ws-ticket", strings.NewReader(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			testHandler(
				&fakeUOWFactory{},
				WithDiagnosisAuth(authProvider, service, resolver),
				withDiagnosisClock(func() time.Time { return now }),
			).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if calls := authProvider.Calls("Bearer oidc-token"); calls != tc.wantAuthCalls {
				t.Fatalf("auth calls = %d, want %d", calls, tc.wantAuthCalls)
			}
		})
	}
}

func TestCreateDiagnosisRoomAuthenticatesAndStartsRoom(t *testing.T) {
	authProvider := authfake.New(map[string][]authfake.Result{
		"Bearer oidc-token": {
			{Principal: ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}}},
		},
	})
	starter := &fakeDiagnosisRoomStarter{
		result: diagnosisroomstart.Result{
			SessionID:          "diagnosis-session-1",
			EvidenceSnapshotID: 42,
			DiagnosisTaskID:    101,
			ChatSessionID:      202,
			Workflow:           ports.WorkflowHandle{WorkflowID: "diagnosis-room-diagnosis-session-1", RunID: "run-1"},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(`{"evidence_snapshot_id":42}`))
	req.Header.Set("Authorization", "Bearer oidc-token")
	testHandler(
		&fakeUOWFactory{},
		WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}),
		WithDiagnosisRoomStarter(starter),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if authProvider.Calls("Bearer oidc-token") != 1 {
		t.Fatalf("auth calls = %d, want 1", authProvider.Calls("Bearer oidc-token"))
	}
	if starter.called != 1 ||
		starter.req.EvidenceSnapshotID != 42 ||
		starter.req.Principal.Subject != "owner-1" {
		t.Fatalf("starter called=%d req=%+v", starter.called, starter.req)
	}
	var body api.DiagnosisRoomCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.SessionID != "diagnosis-session-1" ||
		body.EvidenceSnapshotID != 42 ||
		body.DiagnosisTaskID != 101 ||
		body.ChatSessionID != 202 ||
		body.WorkflowID != "diagnosis-room-diagnosis-session-1" ||
		body.RunID != "run-1" {
		t.Fatalf("response = %+v", body)
	}
}

func TestCreateDiagnosisRoomRejectsBadInputs(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		authHeader    string
		starter       *fakeDiagnosisRoomStarter
		principal     ports.AuthPrincipal
		withAuth      bool
		wantStatus    int
		wantAuthCalls int
	}{
		{
			name:       "unconfigured starter",
			body:       `{"evidence_snapshot_id":42}`,
			authHeader: "Bearer oidc-token",
			principal:  ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:   true,
			wantStatus: stdhttp.StatusServiceUnavailable,
		},
		{
			name:       "unknown field",
			body:       `{"evidence_snapshot_id":42,"session_id":"manual"}`,
			authHeader: "Bearer oidc-token",
			starter:    &fakeDiagnosisRoomStarter{},
			principal:  ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:   true,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:       "duplicate evidence snapshot id",
			body:       `{"evidence_snapshot_id":42,"evidence_snapshot_id":43}`,
			authHeader: "Bearer oidc-token",
			starter:    &fakeDiagnosisRoomStarter{},
			principal:  ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:   true,
			wantStatus: stdhttp.StatusBadRequest,
		},
		{
			name:          "overlarge body",
			body:          strings.Repeat("{", maxJSONRequestBodyBytes+1),
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusBadRequest,
			wantAuthCalls: 0,
		},
		{
			name:       "missing bearer",
			body:       `{"evidence_snapshot_id":42}`,
			starter:    &fakeDiagnosisRoomStarter{},
			withAuth:   true,
			wantStatus: stdhttp.StatusUnauthorized,
		},
		{
			name:          "snapshot not found",
			body:          `{"evidence_snapshot_id":42}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{err: domain.ErrNotFound},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusNotFound,
			wantAuthCalls: 1,
		},
		{
			name:          "unauthorized",
			body:          `{"evidence_snapshot_id":42}`,
			authHeader:    "Bearer oidc-token",
			starter:       &fakeDiagnosisRoomStarter{err: diagnosisauth.ErrUnauthorized},
			principal:     ports.AuthPrincipal{Subject: "owner-1", Roles: []ports.AuthRole{ports.AuthRoleOwner}},
			withAuth:      true,
			wantStatus:    stdhttp.StatusForbidden,
			wantAuthCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authProvider := authfake.New(map[string][]authfake.Result{
				"Bearer oidc-token": {
					{Principal: tc.principal},
				},
			})
			var opts []ServerOption
			if tc.withAuth {
				opts = append(opts, WithDiagnosisAuth(authProvider, newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("E", diagnosisauth.DefaultTokenBytes))), &fakeDiagnosisSessionResolver{}))
			}
			if tc.starter != nil {
				opts = append(opts, WithDiagnosisRoomStarter(tc.starter))
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodPost, "/api/v1/diagnosis/rooms", strings.NewReader(tc.body))
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			testHandler(&fakeUOWFactory{}, opts...).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if calls := authProvider.Calls("Bearer oidc-token"); calls != tc.wantAuthCalls {
				t.Fatalf("auth calls = %d, want %d", calls, tc.wantAuthCalls)
			}
		})
	}
}

func TestHandleDiagnosisWebSocketConsumesTicketAndHandsOffConnection(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("D", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	wsHandler := &fakeDiagnosisWebSocketHandler{done: make(chan struct{})}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(wsHandler),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready map[string]string
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	select {
	case <-wsHandler.done:
	case <-time.After(time.Second):
		t.Fatal("websocket handler did not finish")
	}
	if ready["type"] != "ready" || ready["subject"] != "owner-1" {
		t.Fatalf("ready = %+v", ready)
	}
	if wsHandler.called != 1 || wsHandler.ticket.Token != "" || wsHandler.ticket.SessionID != "session-1" {
		t.Fatalf("handler called=%d ticket=%+v", wsHandler.called, wsHandler.ticket)
	}
	_, err = service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second))
	if !errors.Is(err, diagnosisauth.ErrTicketConsumed) {
		t.Fatalf("ConsumeTicket after websocket err = %v, want consumed", err)
	}
}

func TestHandleDiagnosisWebSocketAcceptsSameHostOrigin(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("O", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	wsHandler := &fakeDiagnosisWebSocketHandler{done: make(chan struct{})}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(wsHandler),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	headers := stdhttp.Header{"Origin": []string{"http://" + host}}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready map[string]string
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	select {
	case <-wsHandler.done:
	case <-time.After(time.Second):
		t.Fatal("websocket handler did not finish")
	}
	if ready["type"] != "ready" || wsHandler.called != 1 {
		t.Fatalf("ready=%+v handler called=%d", ready, wsHandler.called)
	}
}

func TestHandleDiagnosisWebSocketRejectsBadOriginBeforeConsumingTicket(t *testing.T) {
	tests := []struct {
		name   string
		origin func(host string) string
	}{
		{
			name:   "different host",
			origin: func(string) string { return "https://evil.example" },
		},
		{
			name:   "userinfo same host",
			origin: func(host string) string { return "https://operator@" + host },
		},
		{
			name:   "escaped userinfo same host",
			origin: func(host string) string { return "https://operator%40team@" + host },
		},
		{
			name:   "malformed",
			origin: func(string) string { return "http://[::1" },
		},
		{
			name:   "missing host",
			origin: func(host string) string { return "https:" + host },
		},
		{
			name:   "path same host",
			origin: func(host string) string { return "https://" + host + "/diagnosis" },
		},
		{
			name:   "root path same host",
			origin: func(host string) string { return "https://" + host + "/" },
		},
		{
			name:   "query same host",
			origin: func(host string) string { return "https://" + host + "?ticket=redacted" },
		},
		{
			name:   "fragment same host",
			origin: func(host string) string { return "https://" + host + "#diagnosis" },
		},
		{
			name:   "scheme same host",
			origin: func(host string) string { return "ftp://" + host },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
			service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("E", diagnosisauth.DefaultTokenBytes)))
			session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
			ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
				Subject: "owner-1",
				Roles:   []ports.AuthRole{ports.AuthRoleOwner},
			}, session, now)
			if err != nil {
				t.Fatalf("IssueTicket: %v", err)
			}
			handler := testHandlerWithDiagnosisWS(
				&fakeUOWFactory{},
				WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
					sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
				}),
				WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
				withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
			)
			srv := httptest.NewServer(handler)
			defer srv.Close()

			host := strings.TrimPrefix(srv.URL, "http://")
			headers := stdhttp.Header{"Origin": []string{tc.origin(host)}}
			wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
			defer closeWebSocketDialResponse(resp)
			if err == nil {
				_ = conn.Close()
				t.Fatal("Dial with bad origin: want error")
			}
			if resp == nil || resp.StatusCode != stdhttp.StatusForbidden {
				t.Fatalf("resp status = %v, want 403", resp)
			}

			if _, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second)); err != nil {
				t.Fatalf("ticket should not be consumed by rejected origin: %v", err)
			}
		})
	}
}

func TestHandleDiagnosisWebSocketRejectsNonUpgradeBeforeConsumingTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("G", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/ws/diagnosis?session_id=session-1&ticket="+ticket.Token, nil)
	testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := service.ConsumeTicket(context.Background(), ticket.Token, session, now.Add(2*time.Second)); err != nil {
		t.Fatalf("ticket should not be consumed by non-upgrade request: %v", err)
	}
}

func TestHandleDiagnosisWebSocketRejectsMissingTicket(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("F", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisWebSocketHandler(&fakeDiagnosisWebSocketHandler{}),
		withDiagnosisClock(func() time.Time { return now }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err == nil {
		_ = conn.Close()
		t.Fatal("Dial without ticket: want error")
	}
	if resp == nil || resp.StatusCode != stdhttp.StatusUnauthorized {
		t.Fatalf("resp status = %v, want 401", resp)
	}
}

func TestDiagnosisWebSocketRelaySubmitsTurnAndQueriesState(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("H", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	requiresHumanReview := true
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	startedAt := now.Add(time.Minute)
	closedAt := startedAt.Add(2 * time.Second)
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitResult: ports.DiagnosisRoomSubmitTurnResult{
			SessionID:           "session-1",
			ChatSessionID:       domain.ChatSessionID(21),
			MessageID:           "msg-1",
			AssistantMessageID:  "msg-1-assistant",
			UserTurnID:          domain.ChatTurnID(31),
			AssistantTurnID:     domain.ChatTurnID(32),
			UserSequence:        1,
			AssistantSequence:   2,
			TurnCount:           1,
			ContextBytes:        100,
			Status:              "open",
			AssistantMessage:    "CPU alert is still firing.",
			RequiresHumanReview: true,
			Confidence:          "medium",
			EvidenceRequests: []ports.DiagnosisRoomEvidenceRequest{{
				Tool:   domain.DiagnosisToolKindActiveAlerts,
				Reason: "Need current sibling alerts.",
				Limit:  5,
			}},
			CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
				Tool:           domain.DiagnosisToolKindActiveAlerts,
				Status:         "collected",
				ReasonCode:     "ok",
				Message:        "Active alert collection succeeded.",
				ObservedAlerts: 1,
				ActiveAlerts: []ports.DiagnosisRoomActiveAlert{{
					Source:   "alertmanager",
					Labels:   map[string]string{"alertname": "CPUHigh"},
					StartsAt: now,
				}},
				CollectedAt: now.Add(time.Second),
			}, {
				Tool:                 domain.DiagnosisToolKindMetricQuery,
				Status:               "collected",
				ReasonCode:           "ok",
				Message:              "Metric query collection succeeded.",
				Query:                "up",
				ObservedMetricSeries: 1,
				MetricResult: ports.DiagnosisRoomMetricQueryResult{
					ResultType: "vector",
					Series: []ports.DiagnosisRoomMetricSeries{{
						Metric: map[string]string{"__name__": "up", "job": "prometheus"},
						Points: []ports.DiagnosisRoomMetricPoint{{
							Timestamp: now,
							Value:     "1",
						}},
					}},
					Warnings: []string{"partial response"},
				},
				CollectedAt: now.Add(time.Second),
			}},
			ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
				ConfidenceRationale: "CPU evidence is present but restart evidence is missing.",
				MissingEvidenceRequests: []ports.DiagnosisRoomConsultationEvidenceRequest{{
					Label:    "Restart cause",
					Detail:   "Inspect previous pod logs.",
					Priority: "high",
				}},
				ConclusionStatus: "needs_evidence",
			},
			FollowUpTurns: []ports.DiagnosisRoomFollowUpTurnResult{{
				MessageID:           "msg-1/auto-evidence-1",
				UserMessage:         "OpenClarion automatic evidence follow-up.",
				AssistantMessageID:  "msg-1/auto-evidence-1/assistant",
				UserTurnID:          domain.ChatTurnID(33),
				AssistantTurnID:     domain.ChatTurnID(34),
				UserSequence:        3,
				AssistantSequence:   4,
				TurnCount:           2,
				ContextBytes:        512,
				AssistantMessage:    "Collected evidence confirms CPU saturation.",
				RequiresHumanReview: false,
				Confidence:          "high",
				CollectionResults: []ports.DiagnosisRoomEvidenceCollectionResult{{
					Tool:           domain.DiagnosisToolKindActiveAlerts,
					Status:         "collected",
					ReasonCode:     "ok",
					Message:        "Active alert collection succeeded.",
					ObservedAlerts: 1,
					CollectedAt:    now.Add(2 * time.Second),
				}},
				ConsultationInsight: ports.DiagnosisRoomConsultationInsight{
					ConclusionStatus: "final",
				},
				Trigger: "collected_evidence",
			}},
		},
		queryState: ports.DiagnosisRoomState{
			SessionID:       "session-1",
			ChatSessionID:   domain.ChatSessionID(21),
			DiagnosisTaskID: domain.DiagnosisTaskID(11),
			OwnerSubject:    "owner-1",
			Status:          "closed",
			TurnCount:       1,
			StartedAt:       startedAt,
			LastActivityAt:  closedAt,
			ClosedAt:        &closedAt,
			CloseReason:     "user_done",
			FinalConclusion: &ports.DiagnosisRoomFinalConclusion{
				Status:              "available",
				Source:              "latest_assistant_turn",
				AssistantTurnID:     domain.ChatTurnID(32),
				AssistantMessageID:  "msg-1/assistant",
				AssistantSequence:   2,
				AssistantOccurredAt: &startedAt,
				Content:             "CPU alert is still firing.",
				Confidence:          "medium",
				RequiresHumanReview: &requiresHumanReview,
			},
			InFlight:       false,
			SeenMessageIDs: []string{"msg-1"},
			Conversation: []ports.DiagnosisRoomConversationTurn{
				{Role: "user", Content: "Please investigate"},
				{Role: "assistant", Content: "CPU alert is still firing."},
			},
		},
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if ready.Type != diagnosisWSServerReady || ready.SessionID != "session-1" || ready.Subject != "owner-1" {
		t.Fatalf("ready = %+v", ready)
	}

	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	var turn diagnosisWSTurnResultFrame
	if err := conn.ReadJSON(&turn); err != nil {
		t.Fatalf("ReadJSON turn: %v", err)
	}
	if turn.Type != diagnosisWSServerTurnResult || turn.MessageID != "msg-1" || turn.AssistantMessage != "CPU alert is still firing." {
		t.Fatalf("turn = %+v", turn)
	}
	if turn.ConsultationInsight.ConfidenceRationale != "CPU evidence is present but restart evidence is missing." ||
		len(turn.ConsultationInsight.MissingEvidenceRequests) != 1 ||
		turn.ConsultationInsight.MissingEvidenceRequests[0].Label != "Restart cause" ||
		turn.ConsultationInsight.ConclusionStatus != "needs_evidence" {
		t.Fatalf("turn consultation insight = %+v", turn.ConsultationInsight)
	}
	if len(turn.EvidenceRequests) != 1 ||
		turn.EvidenceRequests[0].Tool != "active_alerts" ||
		turn.EvidenceRequests[0].Reason != "Need current sibling alerts." ||
		turn.EvidenceRequests[0].Limit != 5 {
		t.Fatalf("turn evidence requests = %+v", turn.EvidenceRequests)
	}
	if len(turn.CollectionResults) != 2 ||
		turn.CollectionResults[0].Status != "collected" ||
		turn.CollectionResults[0].ReasonCode != "ok" ||
		turn.CollectionResults[0].ObservedAlerts != 1 ||
		len(turn.CollectionResults[0].ActiveAlerts) != 1 ||
		turn.CollectionResults[0].ActiveAlerts[0].Labels["alertname"] != "CPUHigh" {
		t.Fatalf("turn collection results = %+v", turn.CollectionResults)
	}
	metricResult := turn.CollectionResults[1]
	if metricResult.Query != "up" ||
		metricResult.ObservedMetricSeries != 1 ||
		metricResult.MetricResult.ResultType != "vector" ||
		metricResult.MetricResult.Series[0].Metric["job"] != "prometheus" ||
		metricResult.MetricResult.Series[0].Points[0].Value != "1" ||
		metricResult.MetricResult.Warnings[0] != "partial response" {
		t.Fatalf("turn metric collection result = %+v", metricResult)
	}
	submitReq, submitCalled := workflowClient.submitSnapshot()
	if submitCalled != 1 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 1", submitCalled)
	}
	if submitReq.SessionID != "session-1" || submitReq.MessageID != "msg-1" || submitReq.ActorSubject != "owner-1" || submitReq.Message != "Please investigate" {
		t.Fatalf("submit request = %+v", submitReq)
	}
	if len(turn.FollowUpTurns) != 1 ||
		turn.FollowUpTurns[0].MessageID != "msg-1/auto-evidence-1" ||
		turn.FollowUpTurns[0].UserMessage != "OpenClarion automatic evidence follow-up." ||
		turn.FollowUpTurns[0].AssistantMessage != "Collected evidence confirms CPU saturation." ||
		turn.FollowUpTurns[0].ConsultationInsight.ConclusionStatus != "final" ||
		turn.FollowUpTurns[0].CollectionResults[0].Status != "collected" ||
		turn.FollowUpTurns[0].Trigger != "collected_evidence" {
		t.Fatalf("turn follow-up results = %+v", turn.FollowUpTurns)
	}

	if err := conn.WriteJSON(map[string]string{"type": diagnosisWSClientQueryState}); err != nil {
		t.Fatalf("WriteJSON query: %v", err)
	}
	var state diagnosisWSStateFrame
	if err := conn.ReadJSON(&state); err != nil {
		t.Fatalf("ReadJSON state: %v", err)
	}
	if state.Type != diagnosisWSServerState || state.DiagnosisTaskID != 11 || len(state.Conversation) != 2 {
		t.Fatalf("state = %+v", state)
	}
	if state.FinalConclusion == nil ||
		state.FinalConclusion.Status != "available" ||
		state.FinalConclusion.AssistantTurnID != 32 ||
		state.FinalConclusion.AssistantMessageID != "msg-1/assistant" ||
		state.FinalConclusion.Content != "CPU alert is still firing." ||
		state.FinalConclusion.Confidence != "medium" ||
		state.FinalConclusion.RequiresHumanReview == nil ||
		!*state.FinalConclusion.RequiresHumanReview {
		t.Fatalf("state final conclusion = %+v", state.FinalConclusion)
	}
	if querySession, queryCalled := workflowClient.querySnapshot(); queryCalled != 1 || querySession != "session-1" {
		t.Fatalf("QueryDiagnosisRoom calls=%d session=%q, want 1/session-1", queryCalled, querySession)
	}
}

func TestDiagnosisWebSocketRelayRejectsAmbiguousFrame(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("K", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"submit_turn","message_id":"msg-1","message_id":"msg-2","message":"Please investigate"}`)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError || frame.Code != "bad_frame" || !strings.Contains(frame.Message, "duplicate object key") {
		t.Fatalf("error frame = %+v", frame)
	}
	if _, submitCalled := workflowClient.submitSnapshot(); submitCalled != 0 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 0", submitCalled)
	}
}

func TestDiagnosisWebSocketRelayReportsStillProcessingOnUpdateTimeout(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("I", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{blockSubmitUntilContextDone: true}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient, WithDiagnosisWebSocketUpdateTimeout(10*time.Millisecond)),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}
	defer conn.Close()

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	var frame diagnosisWSErrorFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("ReadJSON error: %v", err)
	}
	if frame.Type != diagnosisWSServerError || frame.Code != "turn_still_processing" || frame.Message != "turn is still processing" {
		t.Fatalf("error frame = %+v", frame)
	}
	submitReq, submitCalled := workflowClient.submitSnapshot()
	if submitCalled != 1 || submitReq.ActorSubject != "owner-1" {
		t.Fatalf("submit calls=%d request=%+v", submitCalled, submitReq)
	}
}

func TestDiagnosisWebSocketRelayDoesNotCancelUpdateOnDisconnect(t *testing.T) {
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	service := newHTTPTestDiagnosisAuthService(t, strings.NewReader(strings.Repeat("J", diagnosisauth.DefaultTokenBytes)))
	session := diagnosisauth.SessionRef{SessionID: "session-1", OwnerSubject: "owner-1"}
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	workflowClient := &fakeDiagnosisRoomWorkflowClient{
		submitResult: ports.DiagnosisRoomSubmitTurnResult{
			SessionID:        "session-1",
			MessageID:        "msg-1",
			Status:           "open",
			AssistantMessage: "CPU alert is still firing.",
			Confidence:       "medium",
		},
		submitStarted: make(chan struct{}),
		releaseSubmit: make(chan struct{}),
		submitDone:    make(chan struct{}),
	}
	handler := testHandlerWithDiagnosisWS(
		&fakeUOWFactory{},
		WithDiagnosisAuth(&neverAuthProvider{}, service, &fakeDiagnosisSessionResolver{
			sessions: map[string]diagnosisauth.SessionRef{"session-1": session},
		}),
		WithDiagnosisRoomWorkflowClient(workflowClient, WithDiagnosisWebSocketUpdateTimeout(time.Second)),
		withDiagnosisClock(func() time.Time { return now.Add(time.Second) }),
	)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/diagnosis?session_id=session-1&ticket=" + ticket.Token
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	defer closeWebSocketDialResponse(resp)
	if err != nil {
		t.Fatalf("Dial: %v; resp=%v", err, resp)
	}

	var ready diagnosisWSReadyFrame
	if err := conn.ReadJSON(&ready); err != nil {
		t.Fatalf("ReadJSON ready: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{
		"type":       diagnosisWSClientSubmitTurn,
		"message_id": "msg-1",
		"message":    "Please investigate",
	}); err != nil {
		t.Fatalf("WriteJSON submit: %v", err)
	}
	select {
	case <-workflowClient.submitStarted:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not start")
	}
	_ = conn.Close()
	select {
	case <-workflowClient.submitDone:
		t.Fatal("SubmitDiagnosisTurn completed before release; request context likely cancelled the update")
	case <-time.After(20 * time.Millisecond):
	}
	close(workflowClient.releaseSubmit)
	select {
	case <-workflowClient.submitDone:
	case <-time.After(time.Second):
		t.Fatal("SubmitDiagnosisTurn did not finish after release")
	}
	if err := workflowClient.submitContextErrSnapshot(); err != nil {
		t.Fatalf("submit context err = %v, want nil", err)
	}
}

func TestListEndpointRejectsOutOfRangeLimit(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), stdhttp.MethodGet, "/api/v1/alerts?limit=0", nil)
	testHandler(&fakeUOWFactory{alertRepo: &fakeAlertRepo{}}).ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error == "" {
		t.Fatalf("error response should include a message")
	}
}

func testHandler(factory *fakeUOWFactory, opts ...ServerOption) stdhttp.Handler {
	logger := slog.New(slog.NewTextHandler(testingWriter{}, nil))
	server := NewServer(logger, factory, opts...)
	return api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		ErrorHandlerFunc: OpenAPIErrorHandler(logger),
	})
}

func testHandlerWithDiagnosisWS(factory *fakeUOWFactory, opts ...ServerOption) stdhttp.Handler {
	logger := slog.New(slog.NewTextHandler(testingWriter{}, nil))
	server := NewServer(logger, factory, opts...)
	mux := stdhttp.NewServeMux()
	server.RegisterDiagnosisWebSocketRoutes(mux)
	return api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		BaseRouter:       mux,
		ErrorHandlerFunc: OpenAPIErrorHandler(logger),
	})
}

func newHTTPTestDiagnosisAuthService(t *testing.T, random *strings.Reader) diagnosisauth.Service {
	t.Helper()
	service, err := diagnosisauth.NewService(diagnosisauth.NewMemoryStore(), diagnosisauth.DefaultTicketPolicy(), random)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func closeWebSocketDialResponse(resp *stdhttp.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

type testingWriter struct{}

func (testingWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

type fakeDiagnosisSessionResolver struct {
	sessions  map[string]diagnosisauth.SessionRef
	err       error
	called    int
	sessionID string
}

func (r *fakeDiagnosisSessionResolver) ResolveDiagnosisSession(_ context.Context, sessionID string) (diagnosisauth.SessionRef, error) {
	r.called++
	r.sessionID = sessionID
	if r.err != nil {
		return diagnosisauth.SessionRef{}, r.err
	}
	session, ok := r.sessions[sessionID]
	if !ok {
		return diagnosisauth.SessionRef{}, domain.ErrNotFound
	}
	return session, nil
}

type fakeDiagnosisWebSocketHandler struct {
	called int
	ticket diagnosisauth.Ticket
	done   chan struct{}
}

func (h *fakeDiagnosisWebSocketHandler) ServeDiagnosisWebSocket(_ context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket) {
	h.called++
	h.ticket = ticket
	_ = conn.WriteJSON(map[string]string{
		"type":    "ready",
		"subject": ticket.Subject,
	})
	if h.done != nil {
		close(h.done)
	}
}

type fakeDiagnosisRoomWorkflowClient struct {
	mu                          sync.Mutex
	submitCalled                int
	submitReq                   ports.DiagnosisRoomSubmitTurnRequest
	submitResult                ports.DiagnosisRoomSubmitTurnResult
	submitErr                   error
	blockSubmitUntilContextDone bool
	submitStarted               chan struct{}
	releaseSubmit               chan struct{}
	submitDone                  chan struct{}
	submitContextErr            error
	queryCalled                 int
	querySessionID              string
	queryState                  ports.DiagnosisRoomState
	queryErr                    error
}

type fakeDiagnosisRoomStarter struct {
	called int
	req    diagnosisroomstart.Request
	result diagnosisroomstart.Result
	err    error
}

func (s *fakeDiagnosisRoomStarter) Start(_ context.Context, req diagnosisroomstart.Request) (diagnosisroomstart.Result, error) {
	s.called++
	s.req = req
	if s.err != nil {
		return diagnosisroomstart.Result{}, s.err
	}
	return s.result, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) SubmitDiagnosisTurn(ctx context.Context, req ports.DiagnosisRoomSubmitTurnRequest) (ports.DiagnosisRoomSubmitTurnResult, error) {
	c.mu.Lock()
	c.submitCalled++
	c.submitReq = req
	result := c.submitResult
	err := c.submitErr
	block := c.blockSubmitUntilContextDone
	started := c.submitStarted
	release := c.releaseSubmit
	done := c.submitDone
	c.mu.Unlock()
	if started != nil {
		close(started)
	}
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	if block {
		<-ctx.Done()
		c.recordSubmitContextErr(ctx.Err())
		return ports.DiagnosisRoomSubmitTurnResult{}, ctx.Err()
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			c.recordSubmitContextErr(ctx.Err())
			return ports.DiagnosisRoomSubmitTurnResult{}, ctx.Err()
		}
	}
	if err != nil {
		return ports.DiagnosisRoomSubmitTurnResult{}, err
	}
	return result, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) QueryDiagnosisRoom(_ context.Context, sessionID string) (ports.DiagnosisRoomState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queryCalled++
	c.querySessionID = sessionID
	if c.queryErr != nil {
		return ports.DiagnosisRoomState{}, c.queryErr
	}
	return c.queryState, nil
}

func (c *fakeDiagnosisRoomWorkflowClient) submitSnapshot() (ports.DiagnosisRoomSubmitTurnRequest, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.submitReq, c.submitCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) querySnapshot() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.querySessionID, c.queryCalled
}

func (c *fakeDiagnosisRoomWorkflowClient) recordSubmitContextErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.submitContextErr = err
}

func (c *fakeDiagnosisRoomWorkflowClient) submitContextErrSnapshot() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.submitContextErr
}

type neverAuthProvider struct{}

func (*neverAuthProvider) AuthenticateBearer(context.Context, string) (ports.AuthPrincipal, error) {
	return ports.AuthPrincipal{}, errors.New("unexpected auth provider call")
}

type fakeUOWFactory struct {
	alertRepo     *fakeAlertRepo
	evidenceRepo  *fakeEvidenceRepo
	diagnosisRepo *fakeDiagnosisRepo
	reportRepo    *fakeReportRepo
	configRepo    *fakeConfigRepo
	err           error
}

type fakeReportReplayTrigger struct {
	called int
	req    reporttrigger.Request
	result reporttrigger.Result
	err    error
}

func (t *fakeReportReplayTrigger) ReplayAndStart(_ context.Context, req reporttrigger.Request) (reporttrigger.Result, error) {
	t.called++
	t.req = req
	if t.err != nil {
		return reporttrigger.Result{}, t.err
	}
	return t.result, nil
}

type fakeReportWorkflowPolicyReplayTrigger struct {
	called int
	req    reportpolicytrigger.Request
	result reporttrigger.Result
	err    error
}

func (t *fakeReportWorkflowPolicyReplayTrigger) ReplayAndStart(_ context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error) {
	t.called++
	t.req = req
	if t.err != nil {
		return reporttrigger.Result{}, t.err
	}
	return t.result, nil
}

type fakeAlertmanagerWebhookIngestor struct {
	called int
	req    alertmanagerwebhook.Request
	result alertmanagerwebhook.Result
	err    error
}

func (i *fakeAlertmanagerWebhookIngestor) Ingest(_ context.Context, req alertmanagerwebhook.Request) (alertmanagerwebhook.Result, error) {
	i.called++
	i.req = req
	if i.err != nil {
		return alertmanagerwebhook.Result{}, i.err
	}
	return i.result, nil
}

type fakeAlertSourceConnectionTester struct {
	called  int
	profile domain.AlertSourceProfile
	result  alertsourcecheck.Result
	err     error
}

func (t *fakeAlertSourceConnectionTester) TestAlertSourceConnection(_ context.Context, profile domain.AlertSourceProfile) (alertsourcecheck.Result, error) {
	t.called++
	t.profile = profile
	if t.err != nil {
		return alertsourcecheck.Result{}, t.err
	}
	return t.result, nil
}

type fakeNotificationChannelTester struct {
	called  int
	profile domain.NotificationChannelProfile
	result  notificationchannelcheck.Result
	err     error
}

func (t *fakeNotificationChannelTester) TestNotificationChannel(_ context.Context, profile domain.NotificationChannelProfile) (notificationchannelcheck.Result, error) {
	t.called++
	t.profile = profile
	if t.err != nil {
		return notificationchannelcheck.Result{}, t.err
	}
	return t.result, nil
}

func (f *fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return &fakeUOW{alertRepo: f.alertRepo, evidenceRepo: f.evidenceRepo, diagnosisRepo: f.diagnosisRepo, reportRepo: f.reportRepo, configRepo: f.configRepo}, f.err
}

func (f *fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(ctx, &fakeUOW{alertRepo: f.alertRepo, evidenceRepo: f.evidenceRepo, diagnosisRepo: f.diagnosisRepo, reportRepo: f.reportRepo, configRepo: f.configRepo})
}

type fakeUOW struct {
	ports.UnitOfWork
	alertRepo     ports.AlertRepository
	evidenceRepo  ports.EvidenceRepository
	diagnosisRepo ports.DiagnosisRepository
	reportRepo    ports.ReportRepository
	configRepo    ports.ConfigurationRepository
}

func (u *fakeUOW) Alerts() ports.AlertRepository {
	return u.alertRepo
}

func (u *fakeUOW) Evidence() ports.EvidenceRepository {
	return u.evidenceRepo
}

func (u *fakeUOW) Diagnosis() ports.DiagnosisRepository {
	return u.diagnosisRepo
}

func (u *fakeUOW) Reports() ports.ReportRepository {
	return u.reportRepo
}

func (u *fakeUOW) Config() ports.ConfigurationRepository {
	return u.configRepo
}

func (u *fakeUOW) Commit(context.Context) error {
	return nil
}

func (u *fakeUOW) Rollback(context.Context) error {
	return nil
}

type fakeAlertRepo struct {
	ports.AlertRepository
	events    []domain.AlertEvent
	lastLimit int
}

func (r *fakeAlertRepo) ListEvents(_ context.Context, limit int) ([]domain.AlertEvent, error) {
	r.lastLimit = limit
	if limit > len(r.events) {
		limit = len(r.events)
	}
	return r.events[:limit], nil
}

type fakeEvidenceRepo struct {
	ports.EvidenceRepository
	snapshots []domain.EvidenceSnapshot
	lastLimit int
}

func (r *fakeEvidenceRepo) List(_ context.Context, limit int) ([]domain.EvidenceSnapshot, error) {
	r.lastLimit = limit
	if limit > len(r.snapshots) {
		limit = len(r.snapshots)
	}
	return r.snapshots[:limit], nil
}

type fakeDiagnosisRepo struct {
	ports.DiagnosisRepository
	tasksBySnapshot     map[domain.EvidenceSnapshotID][]domain.DiagnosisTask
	eventsByTaskAndKind map[domain.DiagnosisTaskID]map[string][]domain.DiagnosisTaskEvent
	lastSnapshotID      domain.EvidenceSnapshotID
	lastTaskLimit       int
	lastTaskID          domain.DiagnosisTaskID
	lastEventKind       string
	lastEventLimit      int
}

func (r *fakeDiagnosisRepo) ListTasksByEvidenceSnapshot(_ context.Context, snapshotID domain.EvidenceSnapshotID, limit int) ([]domain.DiagnosisTask, error) {
	r.lastSnapshotID = snapshotID
	r.lastTaskLimit = limit
	tasks := r.tasksBySnapshot[snapshotID]
	if limit > len(tasks) {
		limit = len(tasks)
	}
	return tasks[:limit], nil
}

func (r *fakeDiagnosisRepo) ListEventsByTaskAndKind(_ context.Context, taskID domain.DiagnosisTaskID, kind string, limit int) ([]domain.DiagnosisTaskEvent, error) {
	r.lastTaskID = taskID
	r.lastEventKind = kind
	r.lastEventLimit = limit
	events := r.eventsByTaskAndKind[taskID][kind]
	if limit > len(events) {
		limit = len(events)
	}
	return events[:limit], nil
}

type fakeReportRepo struct {
	ports.ReportRepository
	finalReports        []domain.FinalReport
	linkedSubReports    map[domain.FinalReportID][]domain.SubReport
	deliveriesByReport  map[domain.FinalReportID][]domain.ReportNotificationDelivery
	lastListLimit       int
	lastSubReportsLimit int
	lastDeliveryLimit   int
}

type fakeConfigRepo struct {
	ports.ConfigurationRepository
	alertSourceProfiles                []domain.AlertSourceProfile
	saveResult                         domain.AlertSourceProfile
	updateResult                       domain.AlertSourceProfile
	saved                              domain.AlertSourceProfile
	updated                            domain.AlertSourceProfile
	groupingPolicies                   []domain.GroupingPolicy
	saveGroupingPolicyResult           domain.GroupingPolicy
	updateGroupingPolicyResult         domain.GroupingPolicy
	savedGroupingPolicy                domain.GroupingPolicy
	updatedGroupingPolicy              domain.GroupingPolicy
	reportWorkflowPolicies             []domain.ReportWorkflowPolicy
	saveReportWorkflowPolicyResult     domain.ReportWorkflowPolicy
	updateReportWorkflowPolicyResult   domain.ReportWorkflowPolicy
	savedReportWorkflowPolicy          domain.ReportWorkflowPolicy
	updatedReportWorkflowPolicy        domain.ReportWorkflowPolicy
	reportWorkflowSchedules            []domain.ReportWorkflowSchedule
	saveReportWorkflowScheduleResult   domain.ReportWorkflowSchedule
	updateReportWorkflowScheduleResult domain.ReportWorkflowSchedule
	savedReportWorkflowSchedule        domain.ReportWorkflowSchedule
	updatedReportWorkflowSchedule      domain.ReportWorkflowSchedule
	notificationChannelProfiles        []domain.NotificationChannelProfile
	saveNotificationChannelResult      domain.NotificationChannelProfile
	updateNotificationChannelResult    domain.NotificationChannelProfile
	savedNotificationChannel           domain.NotificationChannelProfile
	updatedNotificationChannel         domain.NotificationChannelProfile
	saveErr                            error
	updateErr                          error
	lastListLimit                      int
}

type recordingReportWorkflowScheduleSyncer struct {
	calls    int
	schedule domain.ReportWorkflowSchedule
	err      error
}

func (r *recordingReportWorkflowScheduleSyncer) SyncReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) error {
	r.calls++
	r.schedule = schedule
	return r.err
}

func (r *fakeConfigRepo) ListAlertSourceProfiles(_ context.Context, limit int) ([]domain.AlertSourceProfile, error) {
	r.lastListLimit = limit
	if limit > len(r.alertSourceProfiles) {
		limit = len(r.alertSourceProfiles)
	}
	return r.alertSourceProfiles[:limit], nil
}

func (r *fakeConfigRepo) SaveAlertSourceProfile(_ context.Context, profile domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	r.saved = profile
	if r.saveErr != nil {
		return domain.AlertSourceProfile{}, r.saveErr
	}
	return r.saveResult, nil
}

func (r *fakeConfigRepo) UpdateAlertSourceProfile(_ context.Context, profile domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	r.updated = profile
	if r.updateErr != nil {
		return domain.AlertSourceProfile{}, r.updateErr
	}
	return r.updateResult, nil
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	for _, profile := range r.alertSourceProfiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return domain.AlertSourceProfile{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListGroupingPolicies(_ context.Context, limit int) ([]domain.GroupingPolicy, error) {
	r.lastListLimit = limit
	if limit > len(r.groupingPolicies) {
		limit = len(r.groupingPolicies)
	}
	return r.groupingPolicies[:limit], nil
}

func (r *fakeConfigRepo) SaveGroupingPolicy(_ context.Context, policy domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	r.savedGroupingPolicy = policy
	if r.saveErr != nil {
		return domain.GroupingPolicy{}, r.saveErr
	}
	return r.saveGroupingPolicyResult, nil
}

func (r *fakeConfigRepo) UpdateGroupingPolicy(_ context.Context, policy domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	r.updatedGroupingPolicy = policy
	if r.updateErr != nil {
		return domain.GroupingPolicy{}, r.updateErr
	}
	return r.updateGroupingPolicyResult, nil
}

func (r *fakeConfigRepo) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	for _, policy := range r.groupingPolicies {
		if policy.ID == id {
			return policy, nil
		}
	}
	return domain.GroupingPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListReportWorkflowPolicies(_ context.Context, limit int) ([]domain.ReportWorkflowPolicy, error) {
	r.lastListLimit = limit
	if limit > len(r.reportWorkflowPolicies) {
		limit = len(r.reportWorkflowPolicies)
	}
	return r.reportWorkflowPolicies[:limit], nil
}

func (r *fakeConfigRepo) SaveReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.savedReportWorkflowPolicy = policy
	if r.saveErr != nil {
		return domain.ReportWorkflowPolicy{}, r.saveErr
	}
	if r.saveReportWorkflowPolicyResult.ID != 0 {
		r.reportWorkflowPolicies = append(r.reportWorkflowPolicies, r.saveReportWorkflowPolicyResult)
		return r.saveReportWorkflowPolicyResult, nil
	}
	policy.ID = domain.ReportWorkflowPolicyID(len(r.reportWorkflowPolicies) + 1)
	r.reportWorkflowPolicies = append(r.reportWorkflowPolicies, policy)
	return policy, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.updatedReportWorkflowPolicy = policy
	if r.updateErr != nil {
		return domain.ReportWorkflowPolicy{}, r.updateErr
	}
	for i, existing := range r.reportWorkflowPolicies {
		if existing.ID == policy.ID {
			if r.updateReportWorkflowPolicyResult.ID != 0 {
				r.reportWorkflowPolicies[i] = r.updateReportWorkflowPolicyResult
				return r.updateReportWorkflowPolicyResult, nil
			}
			r.reportWorkflowPolicies[i] = policy
			return policy, nil
		}
	}
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	for _, policy := range r.reportWorkflowPolicies {
		if policy.ID == id {
			return policy, nil
		}
	}
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListReportWorkflowSchedules(_ context.Context, limit int) ([]domain.ReportWorkflowSchedule, error) {
	r.lastListLimit = limit
	if limit > len(r.reportWorkflowSchedules) {
		limit = len(r.reportWorkflowSchedules)
	}
	return r.reportWorkflowSchedules[:limit], nil
}

func (r *fakeConfigRepo) SaveReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.savedReportWorkflowSchedule = schedule
	if r.saveErr != nil {
		return domain.ReportWorkflowSchedule{}, r.saveErr
	}
	if r.saveReportWorkflowScheduleResult.ID != 0 {
		r.reportWorkflowSchedules = append(r.reportWorkflowSchedules, r.saveReportWorkflowScheduleResult)
		return r.saveReportWorkflowScheduleResult, nil
	}
	schedule.ID = domain.ReportWorkflowScheduleID(len(r.reportWorkflowSchedules) + 1)
	r.reportWorkflowSchedules = append(r.reportWorkflowSchedules, schedule)
	return schedule, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.updatedReportWorkflowSchedule = schedule
	if r.updateErr != nil {
		return domain.ReportWorkflowSchedule{}, r.updateErr
	}
	for i, existing := range r.reportWorkflowSchedules {
		if existing.ID == schedule.ID {
			if r.updateReportWorkflowScheduleResult.ID != 0 {
				r.reportWorkflowSchedules[i] = r.updateReportWorkflowScheduleResult
				return r.updateReportWorkflowScheduleResult, nil
			}
			r.reportWorkflowSchedules[i] = schedule
			return schedule, nil
		}
	}
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindReportWorkflowScheduleByID(_ context.Context, id domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	for _, schedule := range r.reportWorkflowSchedules {
		if schedule.ID == id {
			return schedule, nil
		}
	}
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) ListNotificationChannelProfiles(_ context.Context, limit int) ([]domain.NotificationChannelProfile, error) {
	r.lastListLimit = limit
	if limit > len(r.notificationChannelProfiles) {
		limit = len(r.notificationChannelProfiles)
	}
	return r.notificationChannelProfiles[:limit], nil
}

func (r *fakeConfigRepo) SaveNotificationChannelProfile(_ context.Context, profile domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	r.savedNotificationChannel = profile
	if r.saveErr != nil {
		return domain.NotificationChannelProfile{}, r.saveErr
	}
	if r.saveNotificationChannelResult.ID != 0 {
		r.notificationChannelProfiles = append(r.notificationChannelProfiles, r.saveNotificationChannelResult)
		return r.saveNotificationChannelResult, nil
	}
	profile.ID = domain.NotificationChannelProfileID(len(r.notificationChannelProfiles) + 1)
	r.notificationChannelProfiles = append(r.notificationChannelProfiles, profile)
	return profile, nil
}

func (r *fakeConfigRepo) UpdateNotificationChannelProfile(_ context.Context, profile domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	r.updatedNotificationChannel = profile
	if r.updateErr != nil {
		return domain.NotificationChannelProfile{}, r.updateErr
	}
	for i, existing := range r.notificationChannelProfiles {
		if existing.ID == profile.ID {
			if r.updateNotificationChannelResult.ID != 0 {
				r.notificationChannelProfiles[i] = r.updateNotificationChannelResult
				return r.updateNotificationChannelResult, nil
			}
			r.notificationChannelProfiles[i] = profile
			return profile, nil
		}
	}
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}

func (r *fakeConfigRepo) FindNotificationChannelProfileByID(_ context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	for _, profile := range r.notificationChannelProfiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}

func (r *fakeReportRepo) ListFinalReports(_ context.Context, limit int) ([]domain.FinalReport, error) {
	r.lastListLimit = limit
	if limit > len(r.finalReports) {
		limit = len(r.finalReports)
	}
	return r.finalReports[:limit], nil
}

func (r *fakeReportRepo) FindFinalReportByID(_ context.Context, id domain.FinalReportID) (domain.FinalReport, error) {
	for _, report := range r.finalReports {
		if report.ID == id {
			return report, nil
		}
	}
	return domain.FinalReport{}, domain.ErrNotFound
}

func (r *fakeReportRepo) ListSubReportsForFinalReport(_ context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.SubReport, error) {
	r.lastSubReportsLimit = limit
	if _, err := r.FindFinalReportByID(context.Background(), finalReportID); err != nil {
		return nil, err
	}
	reports := r.linkedSubReports[finalReportID]
	if limit > len(reports) {
		limit = len(reports)
	}
	return reports[:limit], nil
}

func (r *fakeReportRepo) ListNotificationDeliveriesByFinalReport(_ context.Context, finalReportID domain.FinalReportID, limit int) ([]domain.ReportNotificationDelivery, error) {
	r.lastDeliveryLimit = limit
	deliveries := r.deliveriesByReport[finalReportID]
	if limit > len(deliveries) {
		limit = len(deliveries)
	}
	return deliveries[:limit], nil
}
