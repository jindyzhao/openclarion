package diagnosisevidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceCollectActiveAlertsWithTemplateID(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{alerts: []ports.ActiveAlert{
		{
			Source:      "alertmanager",
			Labels:      map[string]string{"alertname": "CPUHigh", "namespace": "prod"},
			Annotations: map[string]string{"summary": "CPU is high"},
			StartsAt:    now.Add(-time.Minute),
			RawPayload:  json.RawMessage(`{"receiver":"private"}`),
		},
		{
			Source:   "alertmanager",
			Labels:   map[string]string{"alertname": "MemoryHigh"},
			StartsAt: now.Add(-2 * time.Minute),
		},
	}}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 7,
		Tool:       domain.DiagnosisToolKindActiveAlerts,
		Reason:     "Need current sibling alerts.",
		Limit:      1,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(got.Items))
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.ReasonCode != ReasonOK ||
		item.TemplateID != 7 ||
		item.AlertSourceProfileID != 1 ||
		item.AlertSourceKind != domain.AlertSourceKindAlertmanager ||
		item.ObservedAlerts != 2 ||
		len(item.ActiveAlerts) != 1 ||
		item.ActiveAlerts[0].Labels["alertname"] != "CPUHigh" ||
		item.ActiveAlerts[0].RawPayload != nil ||
		item.CollectedAt != now {
		t.Fatalf("item = %+v", item)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestServiceCollectResolvesSingleEnabledTemplate(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.templates[8] = activeAlertsTemplate(8, false)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		Tool:   domain.DiagnosisToolKindActiveAlerts,
		Reason: "Need active alerts.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.TemplateID != 7 ||
		item.Limit != 5 ||
		item.AlertSourceKind != domain.AlertSourceKindPrometheus {
		t.Fatalf("item = %+v", item)
	}
}

func TestServiceCollectReportsAmbiguousTemplateWithoutTemplateID(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.templates[8] = activeAlertsTemplate(8, true)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		Tool:   domain.DiagnosisToolKindActiveAlerts,
		Reason: "Need active alerts.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped || item.ReasonCode != ReasonTemplateAmbiguous {
		t.Fatalf("item = %+v", item)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestServiceCollectResolvesTemplateByAlertSourceProfileID(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplateForSource(7, 1, true)
	repo.templates[8] = activeAlertsTemplateForSource(8, 2, true)
	repo.alertSources[1] = alertSourceProfileWithID(1, domain.AlertSourceKindAlertmanager)
	repo.alertSources[2] = alertSourceProfileWithID(2, domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		AlertSourceProfileID: 2,
		Tool:                 domain.DiagnosisToolKindActiveAlerts,
		Reason:               "Need active alerts from secondary Alertmanager.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.ReasonCode != ReasonOK ||
		item.TemplateID != 8 ||
		item.AlertSourceProfileID != 2 {
		t.Fatalf("item = %+v", item)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestServiceCollectReportsTemplateSourceMismatch(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplateForSource(7, 1, true)
	repo.alertSources[1] = alertSourceProfileWithID(1, domain.AlertSourceKindAlertmanager)
	repo.alertSources[2] = alertSourceProfileWithID(2, domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID:           7,
		AlertSourceProfileID: 2,
		Tool:                 domain.DiagnosisToolKindActiveAlerts,
		Reason:               "Need active alerts from secondary Alertmanager.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped || item.ReasonCode != ReasonTemplateSourceMismatch {
		t.Fatalf("item = %+v", item)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestServiceCollectReportsUnsupportedTools(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		Tool:   domain.DiagnosisToolKind("logs"),
		Reason: "Need current logs.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusUnsupported || item.ReasonCode != ReasonUnsupportedTool {
		t.Fatalf("item = %+v", item)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestServiceCollectMetricQueryUsesTemplateQuery(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricQueryTemplate("up")
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{metricResult: ports.MetricQueryResult{
		ResultType: "vector",
		Series: []ports.MetricSeries{
			{
				Metric: map[string]string{"job": "prometheus"},
				Points: []ports.MetricPoint{{Timestamp: now, Value: "1"}},
			},
			{
				Metric: map[string]string{"job": "node"},
				Points: []ports.MetricPoint{{Timestamp: now, Value: "0"}},
			},
		},
		Warnings: []string{"partial response"},
	}}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current health.",
		Query:      "up",
		Limit:      1,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.ReasonCode != ReasonOK ||
		item.Query != "up" ||
		item.ObservedMetricSeries != 2 ||
		len(item.MetricResult.Series) != 1 ||
		item.MetricResult.Series[0].Metric["job"] != "prometheus" ||
		item.MetricResult.Warnings[0] != "partial response" {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 1 || provider.lastMetricReq.Query != "up" || provider.lastMetricReq.Time != now || provider.lastMetricReq.Limit != 1 {
		t.Fatalf("metric request calls=%d req=%+v", provider.metricCalls, provider.lastMetricReq)
	}
}

func TestServiceCollectMetricQueryReportsMissingProviderCapability(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricQueryTemplate("up")
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	svc := mustServiceWithProvider(t, repo, alertOnlyProvider{}, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current health.",
		Query:      "up",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusUnsupported || item.ReasonCode != ReasonProviderCapability {
		t.Fatalf("item = %+v", item)
	}
}

func TestServiceCollectMetricQueryAllowsParameterizedTemplateQuery(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	queryTemplate := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	concreteQuery := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR"}`
	repo.templates[9] = metricQueryTemplate(queryTemplate)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{metricResult: ports.MetricQueryResult{
		ResultType: "vector",
		Series: []ports.MetricSeries{{
			Metric: map[string]string{"ORACLE_SID": "sapprd1", "TABLESPACE": "PSAPSR3USR"},
			Points: []ports.MetricPoint{{Timestamp: now, Value: "95.2"}},
		}},
	}}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current tablespace saturation.",
		Query:      concreteQuery,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.ReasonCode != ReasonOK ||
		item.Query != concreteQuery ||
		item.ObservedMetricSeries != 1 {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 1 || provider.lastMetricReq.Query != concreteQuery {
		t.Fatalf("metric request calls=%d req=%+v", provider.metricCalls, provider.lastMetricReq)
	}
}

func TestServiceCollectMetricQueryRejectsTemplateQueryMismatch(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricQueryTemplate("up")
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current health.",
		Query:      "process_start_time_seconds",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped || item.ReasonCode != ReasonTemplateQueryMismatch {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 0 {
		t.Fatalf("metric calls = %d, want 0", provider.metricCalls)
	}
}

func TestServiceCollectMetricQueryRejectsParameterizedTemplateMismatch(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	queryTemplate := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	repo.templates[9] = metricQueryTemplate(queryTemplate)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current tablespace saturation.",
		Query:      `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR",pod="api-1"}`,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped || item.ReasonCode != ReasonTemplateQueryMismatch {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 0 {
		t.Fatalf("metric calls = %d, want 0", provider.metricCalls)
	}
}

func TestServiceCollectMetricQueryRejectsParameterizedTemplateWithoutQuery(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricQueryTemplate(`up{job="{{label.job}}"}`)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current target health.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped || item.ReasonCode != ReasonTemplateQueryMismatch {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 0 {
		t.Fatalf("metric calls = %d, want 0", provider.metricCalls)
	}
}

func TestServiceCollectMetricQueryRejectsThanosRuleProfile(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricQueryTemplate("up")
	repo.alertSources[1] = alertSourceProfileWithLabels(
		1,
		domain.AlertSourceKindPrometheus,
		map[string]string{"source": "thanos-rule"},
	)
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 9,
		Tool:       domain.DiagnosisToolKindMetricQuery,
		Reason:     "Need current health.",
		Query:      "up",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusSkipped ||
		item.ReasonCode != ReasonSourceKindMismatch ||
		item.AlertSourceKind != domain.AlertSourceKindPrometheus ||
		item.Message != "Thanos Rule alert source supports active_alerts only; use a Thanos Query or Prometheus alert source profile for metric evidence." {
		t.Fatalf("item = %+v", item)
	}
	if provider.metricCalls != 0 {
		t.Fatalf("metric calls = %d, want 0", provider.metricCalls)
	}
}

func TestServiceCollectMetricRangeUsesWindowAndCapsPoints(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[9] = metricRangeTemplate(9, true, `rate(http_requests_total[5m])`)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindPrometheus)
	provider := &fakeMetricsProvider{rangeResult: ports.MetricQueryResult{
		ResultType: "matrix",
		Series: []ports.MetricSeries{{
			Metric: map[string]string{"job": "api"},
			Points: metricPoints(now.Add(-70*time.Minute), 70),
		}},
	}}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID:    9,
		Tool:          domain.DiagnosisToolKindMetricRangeQuery,
		Reason:        "Need recent request rate.",
		Query:         `rate(http_requests_total[5m])`,
		WindowSeconds: 1800,
		StepSeconds:   60,
		Limit:         5,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusCollected ||
		item.Query != `rate(http_requests_total[5m])` ||
		item.WindowSeconds != 1800 ||
		item.StepSeconds != 60 ||
		item.ObservedMetricSeries != 1 ||
		len(item.MetricResult.Series) != 1 ||
		len(item.MetricResult.Series[0].Points) != 60 {
		t.Fatalf("item = %+v", item)
	}
	if provider.rangeCalls != 1 ||
		provider.lastRangeReq.Start != now.Add(-30*time.Minute) ||
		provider.lastRangeReq.End != now ||
		provider.lastRangeReq.Step != time.Minute ||
		provider.lastRangeReq.Limit != 5 {
		t.Fatalf("range request calls=%d req=%+v", provider.rangeCalls, provider.lastRangeReq)
	}
	if gotFirst := item.MetricResult.Series[0].Points[0].Timestamp; gotFirst != now.Add(-60*time.Minute) {
		t.Fatalf("first retained point = %s, want %s", gotFirst, now.Add(-60*time.Minute))
	}
}

func TestServiceCollectSanitizesProviderFailures(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{err: errors.New("upstream leaked http://secret.example.invalid/token")}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 7,
		Tool:       domain.DiagnosisToolKindActiveAlerts,
		Reason:     "Need active alerts.",
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusFailed || item.ReasonCode != ReasonProviderFailed {
		t.Fatalf("item = %+v", item)
	}
	if item.Message == provider.err.Error() {
		t.Fatalf("provider error leaked through message: %q", item.Message)
	}
}

func TestServiceCollectPreservesPartialActiveAlertsOnProviderFailure(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{
		alerts: []ports.ActiveAlert{
			{Source: "alertmanager", Labels: map[string]string{"alertname": "CPUHigh"}, RawPayload: json.RawMessage(`{"secret":"redacted"}`)},
			{Source: "alertmanager", Labels: map[string]string{"alertname": "MemoryHigh"}},
		},
		err: errors.New("second upstream alert was malformed"),
	}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 7,
		Tool:       domain.DiagnosisToolKindActiveAlerts,
		Reason:     "Need active alerts.",
		Limit:      1,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusFailed || item.ReasonCode != ReasonProviderFailed ||
		item.ObservedAlerts != 2 || len(item.ActiveAlerts) != 1 ||
		item.ActiveAlerts[0].Labels["alertname"] != "CPUHigh" || item.ActiveAlerts[0].RawPayload != nil {
		t.Fatalf("partial item = %+v", item)
	}
}

func TestServiceCollectDiscardsPartialAlertsOnProviderDeadline(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	repo.templates[7] = activeAlertsTemplate(7, true)
	repo.alertSources[1] = alertSourceProfile(domain.AlertSourceKindAlertmanager)
	provider := &fakeMetricsProvider{
		alerts: []ports.ActiveAlert{{Source: "alertmanager"}},
		err:    fmt.Errorf("provider timeout: %w", context.DeadlineExceeded),
	}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		TemplateID: 7,
		Tool:       domain.DiagnosisToolKindActiveAlerts,
		Reason:     "Need active alerts.",
		Limit:      1,
	}}})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	item := got.Items[0]
	if item.Status != StatusFailed || item.ReasonCode != ReasonCollectionTimedOut ||
		item.ObservedAlerts != 0 || len(item.ActiveAlerts) != 0 {
		t.Fatalf("deadline item = %+v, want failed without partial alerts", item)
	}
}

func mustService(t *testing.T, repo *fakeConfigRepo, provider *fakeMetricsProvider, now time.Time) *Service {
	t.Helper()
	return mustServiceWithProvider(t, repo, provider, now)
}

func mustServiceWithProvider(t *testing.T, repo *fakeConfigRepo, provider ports.ActiveAlertProvider, now time.Time) *Service {
	t.Helper()
	builder, err := alertsourceprovider.NewBuilder(
		alertsourceprovider.ProviderFactories{
			domain.AlertSourceKindPrometheus: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return provider, nil
			},
			domain.AlertSourceKindAlertmanager: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return provider, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	svc, err := NewService(fakeUOWFactory{repo: repo}, builder, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type alertOnlyProvider struct{}

func (alertOnlyProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

func activeAlertsTemplate(
	id domain.DiagnosisToolTemplateID,
	enabled bool,
) domain.DiagnosisToolTemplate {
	return activeAlertsTemplateForSource(id, 1, enabled)
}

func activeAlertsTemplateForSource(
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
	enabled bool,
) domain.DiagnosisToolTemplate {
	template, err := domain.NewDiagnosisToolTemplate(
		"Active alerts",
		sourceID,
		domain.DiagnosisToolKindActiveAlerts,
		"",
		5,
		0,
		0,
		0,
		enabled,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	template.ID = id
	return template
}

func metricQueryTemplate(
	query string,
) domain.DiagnosisToolTemplate {
	template, err := domain.NewDiagnosisToolTemplate(
		"Metric query",
		1,
		domain.DiagnosisToolKindMetricQuery,
		query,
		5,
		0,
		0,
		0,
		true,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	template.ID = 9
	return template
}

func metricRangeTemplate(
	id domain.DiagnosisToolTemplateID,
	enabled bool,
	query string,
) domain.DiagnosisToolTemplate {
	template, err := domain.NewDiagnosisToolTemplate(
		"Metric range",
		1,
		domain.DiagnosisToolKindMetricRangeQuery,
		query,
		5,
		time.Hour,
		2*time.Hour,
		time.Minute,
		enabled,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	template.ID = id
	return template
}

func metricPoints(start time.Time, count int) []ports.MetricPoint {
	points := make([]ports.MetricPoint, count)
	for i := range points {
		points[i] = ports.MetricPoint{
			Timestamp: start.Add(time.Duration(i) * time.Minute),
			Value:     "1",
		}
	}
	return points
}

func alertSourceProfile(
	kind domain.AlertSourceKind,
) domain.AlertSourceProfile {
	return alertSourceProfileWithID(1, kind)
}

func alertSourceProfileWithID(
	id domain.AlertSourceProfileID,
	kind domain.AlertSourceKind,
) domain.AlertSourceProfile {
	return alertSourceProfileWithLabels(id, kind, nil)
}

func alertSourceProfileWithLabels(
	id domain.AlertSourceProfileID,
	kind domain.AlertSourceKind,
	labels map[string]string,
) domain.AlertSourceProfile {
	profile, err := domain.NewAlertSourceProfile(
		"Primary alert source",
		kind,
		"https://alerts.example.invalid",
		domain.AlertSourceAuthModeNone,
		"",
		true,
		labels,
	)
	if err != nil {
		panic(err)
	}
	profile.ID = id
	return profile
}

type fakeMetricsProvider struct {
	alerts        []ports.ActiveAlert
	metricResult  ports.MetricQueryResult
	rangeResult   ports.MetricQueryResult
	err           error
	metricErr     error
	rangeErr      error
	calls         int
	metricCalls   int
	rangeCalls    int
	lastMetricReq ports.MetricQueryRequest
	lastRangeReq  ports.MetricRangeQueryRequest
}

func (p *fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	p.calls++
	return p.alerts, p.err
}

func (p *fakeMetricsProvider) QueryMetric(_ context.Context, req ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	p.metricCalls++
	p.lastMetricReq = req
	return p.metricResult, p.metricErr
}

func (p *fakeMetricsProvider) QueryMetricRange(_ context.Context, req ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	p.rangeCalls++
	p.lastRangeReq = req
	return p.rangeResult, p.rangeErr
}

type fakeUOWFactory struct {
	repo *fakeConfigRepo
}

func (f fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return fakeUOW{repo: f.repo}, nil
}

func (f fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{repo: f.repo})
}

type fakeUOW struct {
	ports.UnitOfWork
	repo *fakeConfigRepo
}

func (u fakeUOW) Config() ports.ConfigurationRepository {
	return u.repo
}

type fakeConfigRepo struct {
	ports.ConfigurationRepository
	alertSources map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	templates    map[domain.DiagnosisToolTemplateID]domain.DiagnosisToolTemplate
}

func newFakeConfigRepo() *fakeConfigRepo {
	return &fakeConfigRepo{
		alertSources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{},
		templates:    map[domain.DiagnosisToolTemplateID]domain.DiagnosisToolTemplate{},
	}
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(
	_ context.Context,
	id domain.AlertSourceProfileID,
) (domain.AlertSourceProfile, error) {
	profile, ok := r.alertSources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return profile, nil
}

func (r *fakeConfigRepo) FindDiagnosisToolTemplateByID(
	_ context.Context,
	id domain.DiagnosisToolTemplateID,
) (domain.DiagnosisToolTemplate, error) {
	template, ok := r.templates[id]
	if !ok {
		return domain.DiagnosisToolTemplate{}, domain.ErrNotFound
	}
	return template, nil
}

func (r *fakeConfigRepo) ListDiagnosisToolTemplates(
	context.Context,
	int,
) ([]domain.DiagnosisToolTemplate, error) {
	out := make([]domain.DiagnosisToolTemplate, 0, len(r.templates))
	for _, template := range r.templates {
		out = append(out, template)
	}
	return out, nil
}
