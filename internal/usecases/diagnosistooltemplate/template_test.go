package diagnosistooltemplate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceCreateStoresDisabledDraftAndValidatesSource(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindPrometheus, Enabled: false}
	svc := mustService(t, repo)

	saved, err := svc.Create(context.Background(), defaultWriteRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Enabled || repo.savedTemplate.Enabled {
		t.Fatalf("saved template should be disabled: saved=%+v repo=%+v", saved, repo.savedTemplate)
	}
	if repo.savedTemplate.AlertSourceProfileID != 1 || repo.savedTemplate.Tool != domain.DiagnosisToolKindMetricRangeQuery {
		t.Fatalf("saved template = %+v", repo.savedTemplate)
	}
}

func TestServiceCreateRejectsMissingOrIncompatibleSource(t *testing.T) {
	tests := []struct {
		name   string
		source domain.AlertSourceProfile
		want   error
	}{
		{name: "missing", want: domain.ErrNotFound},
		{name: "wrong_kind", source: domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindAlertmanager, Enabled: true}, want: domain.ErrInvariantViolation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeConfigRepo()
			if tc.source.ID != 0 {
				repo.alertSources[tc.source.ID] = tc.source
			}
			svc := mustService(t, repo)

			_, err := svc.Create(context.Background(), defaultWriteRequest())
			if !errors.Is(err, tc.want) {
				t.Fatalf("Create err = %v, want %v", err, tc.want)
			}
			if repo.saveTemplateCalls != 0 {
				t.Fatalf("save calls = %d, want 0", repo.saveTemplateCalls)
			}
		})
	}
}

func TestServiceEnableRequiresEnabledCompatibleSource(t *testing.T) {
	tests := []struct {
		name   string
		source domain.AlertSourceProfile
	}{
		{name: "source_disabled", source: domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindPrometheus, Enabled: false}},
		{name: "wrong_kind", source: domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindAlertmanager, Enabled: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeConfigRepo()
			repo.templates[7] = defaultTemplate()
			repo.alertSources[1] = tc.source
			svc := mustService(t, repo).WithClock(func() time.Time {
				return time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
			})

			_, err := svc.Enable(context.Background(), ActionRequest{TemplateID: 7})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Enable err = %v, want ErrInvariantViolation", err)
			}
			if repo.updateTemplateCalls != 0 {
				t.Fatalf("update calls = %d, want 0", repo.updateTemplateCalls)
			}
		})
	}
}

func TestServiceEnableAndDisableToggleExplicitState(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.templates[7] = defaultTemplate()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindPrometheus, Enabled: true}
	now := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	svc := mustService(t, repo).WithClock(func() time.Time { return now })

	enabled, err := svc.Enable(context.Background(), ActionRequest{TemplateID: 7})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt == nil || enabled.DisabledAt != nil {
		t.Fatalf("enabled = %+v", enabled)
	}

	repo.templates[7] = enabled
	disabled, err := svc.Disable(context.Background(), ActionRequest{TemplateID: 7})
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func TestServiceReplacePreservesEnablement(t *testing.T) {
	enabledAt := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	existing := defaultTemplate()
	existing.Enabled = true
	existing.EnabledAt = &enabledAt
	repo := newFakeConfigRepo()
	repo.templates[7] = existing
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindPrometheus, Enabled: true}
	svc := mustService(t, repo)

	replaced, err := svc.Replace(context.Background(), 7, WriteRequest{
		Name:                 "CPU instant",
		AlertSourceProfileID: 1,
		Tool:                 domain.DiagnosisToolKindMetricQuery,
		QueryTemplate:        "up",
		DefaultLimit:         5,
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !replaced.Enabled || replaced.EnabledAt == nil || replaced.Tool != domain.DiagnosisToolKindMetricQuery || replaced.QueryTemplate != "up" {
		t.Fatalf("replaced = %+v", replaced)
	}
}

func TestServiceReplaceEnabledTemplateRequiresEnabledCompatibleSource(t *testing.T) {
	enabledAt := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		source domain.AlertSourceProfile
	}{
		{name: "source_disabled", source: domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindPrometheus, Enabled: false}},
		{name: "wrong_kind", source: domain.AlertSourceProfile{ID: 1, Kind: domain.AlertSourceKindAlertmanager, Enabled: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			existing := defaultTemplate()
			existing.Enabled = true
			existing.EnabledAt = &enabledAt
			repo := newFakeConfigRepo()
			repo.templates[7] = existing
			repo.alertSources[1] = tc.source
			svc := mustService(t, repo)

			_, err := svc.Replace(context.Background(), 7, defaultWriteRequest())
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Replace err = %v, want ErrInvariantViolation", err)
			}
			if repo.updateTemplateCalls != 0 {
				t.Fatalf("update calls = %d, want 0", repo.updateTemplateCalls)
			}
		})
	}
}

func mustService(t *testing.T, repo *fakeConfigRepo) *Service {
	t.Helper()
	svc, err := NewService(fakeUOWFactory{repo: repo})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func defaultWriteRequest() WriteRequest {
	return WriteRequest{
		Name:                 "CPU saturation range",
		AlertSourceProfileID: 1,
		Tool:                 domain.DiagnosisToolKindMetricRangeQuery,
		QueryTemplate:        `rate(container_cpu_usage_seconds_total[5m])`,
		DefaultLimit:         5,
		DefaultWindow:        time.Hour,
		MaxWindow:            2 * time.Hour,
		DefaultStep:          time.Minute,
	}
}

func defaultTemplate() domain.DiagnosisToolTemplate {
	template, err := domain.NewDiagnosisToolTemplate(
		"CPU saturation range",
		1,
		domain.DiagnosisToolKindMetricRangeQuery,
		`rate(container_cpu_usage_seconds_total[5m])`,
		5,
		time.Hour,
		2*time.Hour,
		time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	template.ID = 7
	return template
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
	alertSources        map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	templates           map[domain.DiagnosisToolTemplateID]domain.DiagnosisToolTemplate
	savedTemplate       domain.DiagnosisToolTemplate
	saveTemplateCalls   int
	updateTemplateCalls int
}

func newFakeConfigRepo() *fakeConfigRepo {
	return &fakeConfigRepo{
		alertSources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{},
		templates:    map[domain.DiagnosisToolTemplateID]domain.DiagnosisToolTemplate{},
	}
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	profile, ok := r.alertSources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return profile, nil
}

func (r *fakeConfigRepo) SaveDiagnosisToolTemplate(_ context.Context, template domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	r.saveTemplateCalls++
	r.savedTemplate = template
	template.ID = 7
	r.templates[7] = template
	return template, nil
}

func (r *fakeConfigRepo) UpdateDiagnosisToolTemplate(_ context.Context, template domain.DiagnosisToolTemplate) (domain.DiagnosisToolTemplate, error) {
	r.updateTemplateCalls++
	if _, ok := r.templates[template.ID]; !ok {
		return domain.DiagnosisToolTemplate{}, domain.ErrNotFound
	}
	r.templates[template.ID] = template
	return template, nil
}

func (r *fakeConfigRepo) FindDiagnosisToolTemplateByID(_ context.Context, id domain.DiagnosisToolTemplateID) (domain.DiagnosisToolTemplate, error) {
	template, ok := r.templates[id]
	if !ok {
		return domain.DiagnosisToolTemplate{}, domain.ErrNotFound
	}
	return template, nil
}
