package reportworkflowpolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceCreateStoresDisabledDraftAndValidatesBindings(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: false}
	repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: false}
	svc := mustService(t, repo)

	saved, err := svc.Create(context.Background(), defaultWriteRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Enabled || repo.savedReportPolicy.Enabled {
		t.Fatalf("saved policy should be disabled: saved=%+v repo=%+v", saved, repo.savedReportPolicy)
	}
	if repo.savedReportPolicy.AlertSourceProfileID != 1 || repo.savedReportPolicy.GroupingPolicyID != 2 {
		t.Fatalf("saved bindings = %+v", repo.savedReportPolicy)
	}
}

func TestServiceCreateRejectsMissingBindings(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: true}
	svc := mustService(t, repo)

	_, err := svc.Create(context.Background(), defaultWriteRequest())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create err = %v, want ErrNotFound", err)
	}
	if repo.saveReportPolicyCalls != 0 {
		t.Fatalf("save calls = %d, want 0", repo.saveReportPolicyCalls)
	}
}

func TestServiceCreateValidatesOptionalNotificationChannelBinding(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: false}
	repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: false}
	req := defaultWriteRequest()
	req.ReportNotificationChannelProfileID = 3
	svc := mustService(t, repo)

	_, err := svc.Create(context.Background(), req)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create err = %v, want ErrNotFound", err)
	}
	if repo.saveReportPolicyCalls != 0 {
		t.Fatalf("save calls = %d, want 0", repo.saveReportPolicyCalls)
	}

	repo.notificationChannels[3] = domain.NotificationChannelProfile{
		ID:             3,
		Enabled:        false,
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
	}
	saved, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create with channel: %v", err)
	}
	if saved.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("ReportNotificationChannelProfileID = %d, want 3", saved.ReportNotificationChannelProfileID)
	}
}

func TestServiceEnableRequiresEnabledBindings(t *testing.T) {
	tests := []struct {
		name     string
		sourceOn bool
		groupOn  bool
	}{
		{name: "source_disabled", sourceOn: false, groupOn: true},
		{name: "grouping_disabled", sourceOn: true, groupOn: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeConfigRepo()
			repo.reportPolicies[7] = defaultPolicy()
			repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: tc.sourceOn}
			repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: tc.groupOn}
			svc := mustService(t, repo)

			_, err := svc.Enable(context.Background(), ActionRequest{PolicyID: 7})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Enable err = %v, want ErrInvariantViolation", err)
			}
			if repo.updateReportPolicyCalls != 0 {
				t.Fatalf("update calls = %d, want 0", repo.updateReportPolicyCalls)
			}
		})
	}
}

func TestServiceEnableRequiresReportNotificationChannelReady(t *testing.T) {
	tests := []struct {
		name    string
		channel domain.NotificationChannelProfile
	}{
		{
			name: "channel_disabled",
			channel: domain.NotificationChannelProfile{
				ID:             3,
				Enabled:        false,
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
			},
		},
		{
			name: "missing_report_scope",
			channel: domain.NotificationChannelProfile{
				ID:             3,
				Enabled:        true,
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := defaultPolicy()
			policy.ReportNotificationChannelProfileID = 3
			repo := newFakeConfigRepo()
			repo.reportPolicies[7] = policy
			repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: true}
			repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: true}
			repo.notificationChannels[3] = tc.channel
			svc := mustService(t, repo)

			_, err := svc.Enable(context.Background(), ActionRequest{PolicyID: 7})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("Enable err = %v, want ErrInvariantViolation", err)
			}
			if repo.updateReportPolicyCalls != 0 {
				t.Fatalf("update calls = %d, want 0", repo.updateReportPolicyCalls)
			}
		})
	}
}

func TestServiceEnableAndDisableToggleExplicitState(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.reportPolicies[7] = defaultPolicy()
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: true}
	repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: true}
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	svc := mustService(t, repo).WithClock(func() time.Time { return now })

	enabled, err := svc.Enable(context.Background(), ActionRequest{PolicyID: 7})
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !enabled.Enabled || enabled.EnabledAt == nil || enabled.DisabledAt != nil {
		t.Fatalf("enabled = %+v", enabled)
	}

	repo.reportPolicies[7] = enabled
	disabled, err := svc.Disable(context.Background(), ActionRequest{PolicyID: 7})
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func TestServiceReplacePreservesEnablementAndValidatesEnabledBindings(t *testing.T) {
	enabledAt := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	existing := defaultPolicy()
	existing.Enabled = true
	existing.EnabledAt = &enabledAt
	repo := newFakeConfigRepo()
	repo.reportPolicies[7] = existing
	repo.alertSources[1] = domain.AlertSourceProfile{ID: 1, Enabled: true}
	repo.groupingPolicies[2] = domain.GroupingPolicy{ID: 2, Enabled: true}
	repo.notificationChannels[3] = domain.NotificationChannelProfile{
		ID:             3,
		Enabled:        true,
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
	}
	svc := mustService(t, repo)

	replaced, err := svc.Replace(context.Background(), 7, WriteRequest{
		Name:                               "Cascade workflow",
		AlertSourceProfileID:               1,
		GroupingPolicyID:                   2,
		ReportNotificationChannelProfileID: 3,
		TriggerMode:                        domain.ReportWorkflowTriggerModeManualReplay,
		ReportScenario:                     domain.ReportWorkflowScenarioCascade,
		DiagnosisFollowUp:                  domain.DiagnosisFollowUpModeSuggestRoom,
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !replaced.Enabled ||
		replaced.EnabledAt == nil ||
		replaced.ReportScenario != domain.ReportWorkflowScenarioCascade ||
		replaced.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("replaced = %+v", replaced)
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
		Name:                 "Default report workflow",
		AlertSourceProfileID: 1,
		GroupingPolicyID:     2,
		TriggerMode:          domain.ReportWorkflowTriggerModeManualReplay,
		ReportScenario:       domain.ReportWorkflowScenarioSingleAlert,
		DiagnosisFollowUp:    domain.DiagnosisFollowUpModeDisabled,
	}
}

func defaultPolicy() domain.ReportWorkflowPolicy {
	policy, err := domain.NewReportWorkflowPolicy(
		"Default report workflow",
		1,
		2,
		0,
		domain.ReportWorkflowTriggerModeManualReplay,
		domain.ReportWorkflowScenarioSingleAlert,
		domain.DiagnosisFollowUpModeDisabled,
		false,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	policy.ID = 7
	return policy
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
	alertSources            map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	groupingPolicies        map[domain.GroupingPolicyID]domain.GroupingPolicy
	notificationChannels    map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
	reportPolicies          map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy
	savedReportPolicy       domain.ReportWorkflowPolicy
	saveReportPolicyCalls   int
	updateReportPolicyCalls int
}

func newFakeConfigRepo() *fakeConfigRepo {
	return &fakeConfigRepo{
		alertSources:         map[domain.AlertSourceProfileID]domain.AlertSourceProfile{},
		groupingPolicies:     map[domain.GroupingPolicyID]domain.GroupingPolicy{},
		notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{},
		reportPolicies:       map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{},
	}
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	profile, ok := r.alertSources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return profile, nil
}

func (r *fakeConfigRepo) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	policy, ok := r.groupingPolicies[id]
	if !ok {
		return domain.GroupingPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (r *fakeConfigRepo) FindNotificationChannelProfileByID(_ context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	channel, ok := r.notificationChannels[id]
	if !ok {
		return domain.NotificationChannelProfile{}, domain.ErrNotFound
	}
	return channel, nil
}

func (r *fakeConfigRepo) SaveReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.saveReportPolicyCalls++
	r.savedReportPolicy = policy
	policy.ID = 7
	r.reportPolicies[7] = policy
	return policy, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowPolicy(_ context.Context, policy domain.ReportWorkflowPolicy) (domain.ReportWorkflowPolicy, error) {
	r.updateReportPolicyCalls++
	if _, ok := r.reportPolicies[policy.ID]; !ok {
		return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
	}
	r.reportPolicies[policy.ID] = policy
	return policy, nil
}

func (r *fakeConfigRepo) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	policy, ok := r.reportPolicies[id]
	if !ok {
		return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}
