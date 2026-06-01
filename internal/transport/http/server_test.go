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
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/ports"
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
	ticket, err := service.IssueTicket(context.Background(), ports.AuthPrincipal{
		Subject: "owner-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}, session, now)
	if err != nil {
		t.Fatalf("IssueTicket: %v", err)
	}
	startedAt := now.Add(time.Minute)
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
		},
		queryState: ports.DiagnosisRoomState{
			SessionID:       "session-1",
			ChatSessionID:   domain.ChatSessionID(21),
			DiagnosisTaskID: domain.DiagnosisTaskID(11),
			OwnerSubject:    "owner-1",
			Status:          "open",
			TurnCount:       1,
			StartedAt:       startedAt,
			LastActivityAt:  startedAt.Add(time.Second),
			InFlight:        false,
			SeenMessageIDs:  []string{"msg-1"},
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
	submitReq, submitCalled := workflowClient.submitSnapshot()
	if submitCalled != 1 {
		t.Fatalf("SubmitDiagnosisTurn calls = %d, want 1", submitCalled)
	}
	if submitReq.SessionID != "session-1" || submitReq.MessageID != "msg-1" || submitReq.ActorSubject != "owner-1" || submitReq.Message != "Please investigate" {
		t.Fatalf("submit request = %+v", submitReq)
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
	alertRepo    *fakeAlertRepo
	evidenceRepo *fakeEvidenceRepo
	reportRepo   *fakeReportRepo
	err          error
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

func (f *fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return &fakeUOW{alertRepo: f.alertRepo, evidenceRepo: f.evidenceRepo, reportRepo: f.reportRepo}, f.err
}

func (f *fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(ctx, &fakeUOW{alertRepo: f.alertRepo, evidenceRepo: f.evidenceRepo, reportRepo: f.reportRepo})
}

type fakeUOW struct {
	ports.UnitOfWork
	alertRepo    ports.AlertRepository
	evidenceRepo ports.EvidenceRepository
	reportRepo   ports.ReportRepository
}

func (u *fakeUOW) Alerts() ports.AlertRepository {
	return u.alertRepo
}

func (u *fakeUOW) Evidence() ports.EvidenceRepository {
	return u.evidenceRepo
}

func (u *fakeUOW) Diagnosis() ports.DiagnosisRepository {
	return nil
}

func (u *fakeUOW) Reports() ports.ReportRepository {
	return u.reportRepo
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

type fakeReportRepo struct {
	ports.ReportRepository
	finalReports        []domain.FinalReport
	linkedSubReports    map[domain.FinalReportID][]domain.SubReport
	deliveriesByReport  map[domain.FinalReportID][]domain.ReportNotificationDelivery
	lastListLimit       int
	lastSubReportsLimit int
	lastDeliveryLimit   int
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
