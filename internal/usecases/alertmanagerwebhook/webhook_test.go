package alertmanagerwebhook

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestIngestPersistsFiringAlertsAndSkipsResolved(t *testing.T) {
	factory := newWebhookTestFactory(mustAlertmanagerProfile(t, domain.AlertSourceAuthModeNone, true))
	service, err := NewService(factory)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.Ingest(context.Background(), Request{
		ProfileID: 7,
		Body:      json.RawMessage(validWebhookPayload()),
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.ProfileID != 7 || result.Received != 2 || result.SkippedResolved != 1 || result.TruncatedAlerts != 1 {
		t.Fatalf("result counters = %+v", result)
	}
	if result.Ingested.Total != 1 || result.Ingested.Saved != 1 {
		t.Fatalf("ingested stats = %+v", result.Ingested)
	}
	if len(factory.alerts.saved) != 1 {
		t.Fatalf("saved alerts = %d, want 1", len(factory.alerts.saved))
	}
	saved := factory.alerts.saved[0]
	if saved.Source != sourceName || saved.Labels["alertname"] != "HighCPU" || saved.Annotations["summary"] != "CPU high" {
		t.Fatalf("saved alert = %+v", saved)
	}
	if !json.Valid(saved.RawPayload) {
		t.Fatalf("raw payload is not valid JSON: %s", saved.RawPayload)
	}
}

func TestIngestChecksBearerAuthorization(t *testing.T) {
	factory := newWebhookTestFactory(mustAlertmanagerProfile(t, domain.AlertSourceAuthModeBearer, true))
	service, err := NewService(factory, WithSecretResolver(fakeSecretResolver{
		"secret/openclarion/alertmanager-webhook": "expected-token",
	}))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.Ingest(context.Background(), Request{
		ProfileID:     7,
		Authorization: "Bearer wrong-token",
		Body:          json.RawMessage(validWebhookPayload()),
	})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Ingest err = %v, want ErrUnauthorized", err)
	}
	if len(factory.alerts.saved) != 0 {
		t.Fatalf("saved alerts after failed auth = %d, want 0", len(factory.alerts.saved))
	}

	result, err := service.Ingest(context.Background(), Request{
		ProfileID:     7,
		Authorization: "Bearer expected-token",
		Body:          json.RawMessage(validWebhookPayload()),
	})
	if err != nil {
		t.Fatalf("authorized Ingest: %v", err)
	}
	if result.Ingested.Saved != 1 {
		t.Fatalf("authorized stats = %+v", result.Ingested)
	}
}

func TestIngestRejectsInvalidProfileState(t *testing.T) {
	tests := []struct {
		name    string
		profile domain.AlertSourceProfile
	}{
		{
			name:    "wrong_kind",
			profile: mustProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone, true),
		},
		{
			name:    "disabled",
			profile: mustAlertmanagerProfile(t, domain.AlertSourceAuthModeNone, false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			factory := newWebhookTestFactory(tc.profile)
			service, err := NewService(factory)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}

			_, err = service.Ingest(context.Background(), Request{
				ProfileID: 7,
				Body:      json.RawMessage(validWebhookPayload()),
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Ingest err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestDecodePayloadRejectsInvalidJSONShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "duplicate_key",
			body: `{"version":"4","status":"firing","alerts":[],"alerts":[]}`,
		},
		{
			name: "unknown_top_level_field",
			body: `{"version":"4","status":"firing","alerts":[],"unexpected":true}`,
		},
		{
			name: "unsupported_version",
			body: `{"version":"3","status":"firing","alerts":[]}`,
		},
		{
			name: "invalid_alert_status",
			body: `{"version":"4","status":"firing","alerts":[{"status":"pending","labels":{},"annotations":{},"startsAt":"2026-06-06T01:00:00Z"}]}`,
		},
		{
			name: "missing_alert_labels",
			body: `{"version":"4","status":"firing","alerts":[{"status":"firing","annotations":{},"startsAt":"2026-06-06T01:00:00Z"}]}`,
		},
		{
			name: "missing_alert_annotations",
			body: `{"version":"4","status":"firing","alerts":[{"status":"firing","labels":{},"startsAt":"2026-06-06T01:00:00Z"}]}`,
		},
		{
			name: "missing_firing_start",
			body: `{"version":"4","status":"firing","alerts":[{"status":"firing","labels":{},"annotations":{}}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodePayload(json.RawMessage(tc.body))
			if err == nil {
				t.Fatal("decodePayload err = nil, want error")
			}
		})
	}
}

func validWebhookPayload() string {
	return `{
		"version":"4",
		"groupKey":"{}:{alertname=\"HighCPU\"}",
		"truncatedAlerts":1,
		"status":"firing",
		"receiver":"openclarion",
		"groupLabels":{"alertname":"HighCPU"},
		"commonLabels":{"severity":"warning"},
		"commonAnnotations":{"runbook":"https://runbooks.example/high-cpu"},
		"externalURL":"https://alertmanager.example",
		"alerts":[
			{
				"status":"firing",
				"labels":{"alertname":"HighCPU","instance":"api-1","severity":"warning"},
				"annotations":{"summary":"CPU high"},
				"startsAt":"2026-06-06T01:00:00Z",
				"endsAt":"0001-01-01T00:00:00Z",
				"generatorURL":"https://prometheus.example/graph",
				"fingerprint":"abc123"
			},
			{
				"status":"resolved",
				"labels":{"alertname":"HighMemory","instance":"api-2"},
				"annotations":{"summary":"Memory recovered"},
				"startsAt":"2026-06-06T00:30:00Z",
				"endsAt":"2026-06-06T01:30:00Z",
				"generatorURL":"https://prometheus.example/graph",
				"fingerprint":"def456"
			}
		]
	}`
}

func mustAlertmanagerProfile(t *testing.T, authMode domain.AlertSourceAuthMode, enabled bool) domain.AlertSourceProfile {
	t.Helper()
	return mustProfile(t, domain.AlertSourceKindAlertmanager, authMode, enabled)
}

func mustProfile(
	t *testing.T,
	kind domain.AlertSourceKind,
	authMode domain.AlertSourceAuthMode,
	enabled bool,
) domain.AlertSourceProfile {
	t.Helper()
	secretRef := ""
	if authMode == domain.AlertSourceAuthModeBearer {
		secretRef = "secret/openclarion/alertmanager-webhook"
	}
	profile, err := domain.NewAlertSourceProfile(
		"Primary Alertmanager",
		kind,
		"https://alertmanager.example",
		authMode,
		secretRef,
		enabled,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	profile.ID = 7
	return profile
}

type fakeSecretResolver map[string]string

func (r fakeSecretResolver) ResolveSecret(_ context.Context, ref string) (ports.Secret, error) {
	value, ok := r[ref]
	if !ok {
		return ports.Secret{}, ports.ErrSecretNotFound
	}
	return ports.Secret{Value: value}, nil
}

type webhookTestFactory struct {
	config *fakeWebhookConfigRepo
	alerts *fakeWebhookAlertRepo
}

func newWebhookTestFactory(profile domain.AlertSourceProfile) *webhookTestFactory {
	return &webhookTestFactory{
		config: &fakeWebhookConfigRepo{profile: profile},
		alerts: &fakeWebhookAlertRepo{},
	}
}

func (f *webhookTestFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return &fakeWebhookUOW{config: f.config, alerts: f.alerts}, nil
}

func (f *webhookTestFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, &fakeWebhookUOW{config: f.config, alerts: f.alerts})
}

type fakeWebhookUOW struct {
	config *fakeWebhookConfigRepo
	alerts *fakeWebhookAlertRepo
}

func (u *fakeWebhookUOW) Alerts() ports.AlertRepository         { return u.alerts }
func (u *fakeWebhookUOW) Evidence() ports.EvidenceRepository    { return nil }
func (u *fakeWebhookUOW) Diagnosis() ports.DiagnosisRepository  { return nil }
func (u *fakeWebhookUOW) Reports() ports.ReportRepository       { return nil }
func (u *fakeWebhookUOW) Config() ports.ConfigurationRepository { return u.config }
func (u *fakeWebhookUOW) Directory() ports.DirectoryRepository  { return nil }
func (u *fakeWebhookUOW) RBAC() ports.RBACRepository            { return nil }
func (u *fakeWebhookUOW) Commit(context.Context) error          { return nil }
func (u *fakeWebhookUOW) Rollback(context.Context) error        { return nil }

type fakeWebhookAlertRepo struct {
	saved []domain.AlertEvent
}

func (r *fakeWebhookAlertRepo) SaveEvent(_ context.Context, e domain.AlertEvent) (domain.AlertEvent, error) {
	e.ID = domain.AlertEventID(len(r.saved) + 1)
	e.CreatedAt = time.Date(2026, 6, 6, 2, 0, 0, 0, time.UTC)
	r.saved = append(r.saved, e)
	return e, nil
}

func (r *fakeWebhookAlertRepo) UpdateEventResolution(context.Context, domain.AlertEvent) (domain.AlertEvent, error) {
	return domain.AlertEvent{}, domain.ErrNotFound
}

func (r *fakeWebhookAlertRepo) FindEventByID(context.Context, domain.AlertEventID) (domain.AlertEvent, error) {
	return domain.AlertEvent{}, domain.ErrNotFound
}

func (r *fakeWebhookAlertRepo) FindEventByNaturalKey(context.Context, string, string, time.Time) (domain.AlertEvent, error) {
	return domain.AlertEvent{}, domain.ErrNotFound
}

func (r *fakeWebhookAlertRepo) ListEventsByStartsAtRange(context.Context, time.Time, time.Time, int) ([]domain.AlertEvent, error) {
	return nil, nil
}

func (r *fakeWebhookAlertRepo) ListEvents(context.Context, int) ([]domain.AlertEvent, error) {
	return append([]domain.AlertEvent(nil), r.saved...), nil
}

func (r *fakeWebhookAlertRepo) SaveGroup(context.Context, domain.AlertGroup) (domain.AlertGroup, error) {
	return domain.AlertGroup{}, nil
}

func (r *fakeWebhookAlertRepo) UpdateGroup(context.Context, domain.AlertGroup) (domain.AlertGroup, error) {
	return domain.AlertGroup{}, nil
}

func (r *fakeWebhookAlertRepo) FindGroupByID(context.Context, domain.AlertGroupID) (domain.AlertGroup, error) {
	return domain.AlertGroup{}, domain.ErrNotFound
}

func (r *fakeWebhookAlertRepo) FindGroupByNaturalKey(context.Context, string, time.Time) (domain.AlertGroup, error) {
	return domain.AlertGroup{}, domain.ErrNotFound
}

func (r *fakeWebhookAlertRepo) LinkEventsToGroup(context.Context, domain.AlertGroupID, []domain.AlertEventID) error {
	return nil
}

func (r *fakeWebhookAlertRepo) ListEventIDsForGroup(context.Context, domain.AlertGroupID) ([]domain.AlertEventID, error) {
	return nil, nil
}

func (r *fakeWebhookAlertRepo) ListActiveGroups(context.Context, int) ([]domain.AlertGroup, error) {
	return nil, nil
}

type fakeWebhookConfigRepo struct {
	profile domain.AlertSourceProfile
}

func (r *fakeWebhookConfigRepo) SaveAlertSourceProfile(context.Context, domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	return domain.AlertSourceProfile{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateAlertSourceProfile(context.Context, domain.AlertSourceProfile) (domain.AlertSourceProfile, error) {
	return domain.AlertSourceProfile{}, nil
}

func (r *fakeWebhookConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	if id != r.profile.ID {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return r.profile, nil
}

func (r *fakeWebhookConfigRepo) ListAlertSourceProfiles(context.Context, int) ([]domain.AlertSourceProfile, error) {
	return []domain.AlertSourceProfile{r.profile}, nil
}

func (r *fakeWebhookConfigRepo) SaveGroupingPolicy(context.Context, domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	return domain.GroupingPolicy{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateGroupingPolicy(context.Context, domain.GroupingPolicy) (domain.GroupingPolicy, error) {
	return domain.GroupingPolicy{}, nil
}

func (r *fakeWebhookConfigRepo) FindGroupingPolicyByID(context.Context, domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	return domain.GroupingPolicy{}, domain.ErrNotFound
}

func (r *fakeWebhookConfigRepo) ListGroupingPolicies(context.Context, int) ([]domain.GroupingPolicy, error) {
	return nil, nil
}

func (r *fakeWebhookConfigRepo) SaveReportWorkflowPolicy(context.Context, domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateReportWorkflowPolicy(context.Context, domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, nil
}

func (r *fakeWebhookConfigRepo) FindReportWorkflowPolicyByID(context.Context, domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
}

func (r *fakeWebhookConfigRepo) ListReportWorkflowPolicies(context.Context, int) ([]domain.ReportWorkflowPolicy, error) {
	return nil, nil
}

func (r *fakeWebhookConfigRepo) SaveReportWorkflowSchedule(context.Context, domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateReportWorkflowSchedule(context.Context, domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, nil
}

func (r *fakeWebhookConfigRepo) FindReportWorkflowScheduleByID(context.Context, domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
}

func (r *fakeWebhookConfigRepo) ListReportWorkflowSchedules(context.Context, int) ([]domain.ReportWorkflowSchedule, error) {
	return nil, nil
}

func (r *fakeWebhookConfigRepo) SaveDiagnosisToolTemplate(context.Context, domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateDiagnosisToolTemplate(context.Context, domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, nil
}

func (r *fakeWebhookConfigRepo) FindDiagnosisToolTemplateByID(context.Context, domain.DiagnosisToolTemplateID) (domain.DiagnosisToolTemplate, error) {
	return domain.DiagnosisToolTemplate{}, domain.ErrNotFound
}

func (r *fakeWebhookConfigRepo) ListDiagnosisToolTemplates(context.Context, int) ([]domain.DiagnosisToolTemplate, error) {
	return nil, nil
}

func (r *fakeWebhookConfigRepo) SaveNotificationChannelProfile(context.Context, domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	return domain.NotificationChannelProfile{}, nil
}

func (r *fakeWebhookConfigRepo) UpdateNotificationChannelProfile(context.Context, domain.NotificationChannelProfile) (domain.NotificationChannelProfile, error) {
	return domain.NotificationChannelProfile{}, nil
}

func (r *fakeWebhookConfigRepo) FindNotificationChannelProfileByID(context.Context, domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	return domain.NotificationChannelProfile{}, domain.ErrNotFound
}

func (r *fakeWebhookConfigRepo) ListNotificationChannelProfiles(context.Context, int) ([]domain.NotificationChannelProfile, error) {
	return nil, nil
}
