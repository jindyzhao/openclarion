package diagnosisevidence

import (
	"context"
	"encoding/json"
	"errors"
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

func TestServiceCollectReportsUnsupportedTools(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	repo := newFakeConfigRepo()
	provider := &fakeMetricsProvider{}
	svc := mustService(t, repo, provider, now)

	got, err := svc.Collect(context.Background(), Request{Requests: []diagnosisroom.EvidenceRequest{{
		Tool:   domain.DiagnosisToolKindMetricQuery,
		Reason: "Need current CPU.",
		Query:  "up",
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

func mustService(t *testing.T, repo *fakeConfigRepo, provider *fakeMetricsProvider, now time.Time) *Service {
	t.Helper()
	builder, err := alertsourceprovider.NewBuilder(
		func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.MetricsProvider, error) {
			return provider, nil
		},
		alertsourceprovider.WithAlertmanagerFactory(func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.MetricsProvider, error) {
			return provider, nil
		}),
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

func activeAlertsTemplate(
	id domain.DiagnosisToolTemplateID,
	enabled bool,
) domain.DiagnosisToolTemplate {
	template, err := domain.NewDiagnosisToolTemplate(
		"Active alerts",
		1,
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

func alertSourceProfile(
	kind domain.AlertSourceKind,
) domain.AlertSourceProfile {
	profile, err := domain.NewAlertSourceProfile(
		"Primary alert source",
		kind,
		"https://alerts.example.invalid",
		domain.AlertSourceAuthModeNone,
		"",
		true,
		nil,
	)
	if err != nil {
		panic(err)
	}
	profile.ID = 1
	return profile
}

type fakeMetricsProvider struct {
	alerts []ports.ActiveAlert
	err    error
	calls  int
}

func (p *fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	p.calls++
	return p.alerts, p.err
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
