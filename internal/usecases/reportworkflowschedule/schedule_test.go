package reportworkflowschedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceCreateStoresDisabledDraftAndValidatesPolicyBinding(t *testing.T) {
	repo := newFakeConfigRepo()
	repo.reportPolicies[7] = defaultPolicy(false)
	svc := mustService(t, repo)

	saved, err := svc.Create(context.Background(), defaultWriteRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Enabled || repo.savedSchedule.Enabled {
		t.Fatalf("saved schedule should be disabled: saved=%+v repo=%+v", saved, repo.savedSchedule)
	}
	if repo.savedSchedule.ReportWorkflowPolicyID != 7 || repo.savedSchedule.TemporalScheduleID == "" {
		t.Fatalf("saved binding = %+v", repo.savedSchedule)
	}
}

func TestServiceCreateRejectsMissingPolicy(t *testing.T) {
	repo := newFakeConfigRepo()
	svc := mustService(t, repo)

	_, err := svc.Create(context.Background(), defaultWriteRequest())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Create err = %v, want ErrNotFound", err)
	}
	if repo.saveScheduleCalls != 0 {
		t.Fatalf("save calls = %d, want 0", repo.saveScheduleCalls)
	}
}

func TestServiceEnableRequiresEnabledPolicy(t *testing.T) {
	tests := []struct {
		name          string
		policyEnabled bool
	}{
		{name: "disabled_policy", policyEnabled: false},
		{name: "enabled_policy", policyEnabled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeConfigRepo()
			repo.schedules[9] = defaultSchedule()
			repo.reportPolicies[7] = defaultPolicy(tc.policyEnabled)
			svc := mustService(t, repo).WithClock(func() time.Time {
				return time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
			})

			enabled, err := svc.Enable(context.Background(), ActionRequest{ScheduleID: 9})
			if !tc.policyEnabled {
				if !errors.Is(err, domain.ErrInvariantViolation) {
					t.Fatalf("Enable err = %v, want ErrInvariantViolation", err)
				}
				if repo.updateScheduleCalls != 0 {
					t.Fatalf("update calls = %d, want 0", repo.updateScheduleCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Enable: %v", err)
			}
			if !enabled.Enabled || enabled.EnabledAt == nil || enabled.DisabledAt != nil {
				t.Fatalf("enabled = %+v", enabled)
			}
		})
	}
}

func TestServiceEnableRejectsOverlappingReplayWindow(t *testing.T) {
	schedule := defaultSchedule()
	schedule.Interval = time.Minute
	schedule.ReplayWindow = time.Hour
	repo := newFakeConfigRepo()
	repo.schedules[9] = schedule
	repo.reportPolicies[7] = defaultPolicy(true)
	svc := mustService(t, repo).WithClock(func() time.Time {
		return time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	})

	_, err := svc.Enable(context.Background(), ActionRequest{ScheduleID: 9})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("Enable err = %v, want ErrInvariantViolation", err)
	}
	if repo.updateScheduleCalls != 0 {
		t.Fatalf("update calls = %d, want 0", repo.updateScheduleCalls)
	}
}

func TestServiceDisableDoesNotRequireEnabledPolicy(t *testing.T) {
	schedule := defaultSchedule()
	schedule.Enabled = true
	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	schedule.EnabledAt = &enabledAt
	repo := newFakeConfigRepo()
	repo.schedules[9] = schedule
	repo.reportPolicies[7] = defaultPolicy(false)
	svc := mustService(t, repo).WithClock(func() time.Time {
		return time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	})

	disabled, err := svc.Disable(context.Background(), ActionRequest{ScheduleID: 9})
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if disabled.Enabled || disabled.EnabledAt != nil || disabled.DisabledAt == nil {
		t.Fatalf("disabled = %+v", disabled)
	}
}

func TestServiceReplacePreservesEnablementAndValidatesEnabledPolicy(t *testing.T) {
	enabledAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	existing := defaultSchedule()
	existing.Enabled = true
	existing.EnabledAt = &enabledAt
	repo := newFakeConfigRepo()
	repo.schedules[9] = existing
	repo.reportPolicies[7] = defaultPolicy(true)
	svc := mustService(t, repo)

	replaced, err := svc.Replace(context.Background(), 9, WriteRequest{
		Name:                   "Thirty minute reports",
		ReportWorkflowPolicyID: 7,
		TemporalScheduleID:     "openclarion-report-policy-7-30m",
		Interval:               30 * time.Minute,
		Offset:                 time.Minute,
		ReplayWindow:           15 * time.Minute,
		ReplayDelay:            time.Minute,
		ReplayLimit:            500,
		CatchupWindow:          5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !replaced.Enabled ||
		replaced.EnabledAt == nil ||
		replaced.Interval != 30*time.Minute ||
		replaced.TemporalScheduleID != "openclarion-report-policy-7-30m" {
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
		Name:                   "Hourly reports",
		ReportWorkflowPolicyID: 7,
		TemporalScheduleID:     "openclarion-report-policy-7-hourly",
		Interval:               time.Hour,
		Offset:                 0,
		ReplayWindow:           30 * time.Minute,
		ReplayDelay:            2 * time.Minute,
		ReplayLimit:            1000,
		CatchupWindow:          10 * time.Minute,
	}
}

func defaultSchedule() domain.ReportWorkflowSchedule {
	schedule, err := domain.NewReportWorkflowSchedule(
		"Hourly reports",
		7,
		"openclarion-report-policy-7-hourly",
		time.Hour,
		0,
		30*time.Minute,
		2*time.Minute,
		1000,
		10*time.Minute,
		false,
		nil,
		nil,
	)
	if err != nil {
		panic(err)
	}
	schedule.ID = 9
	return schedule
}

func defaultPolicy(enabled bool) domain.ReportWorkflowPolicy {
	policy, err := domain.NewReportWorkflowPolicy(
		"Default report workflow",
		1,
		2,
		0,
		domain.ReportWorkflowTriggerModeManualReplay,
		domain.ReportWorkflowScenarioSingleAlert,
		domain.DiagnosisFollowUpModeDisabled,
		enabled,
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
	reportPolicies      map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy
	schedules           map[domain.ReportWorkflowScheduleID]domain.ReportWorkflowSchedule
	savedSchedule       domain.ReportWorkflowSchedule
	saveScheduleCalls   int
	updateScheduleCalls int
}

func newFakeConfigRepo() *fakeConfigRepo {
	return &fakeConfigRepo{
		reportPolicies: map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{},
		schedules:      map[domain.ReportWorkflowScheduleID]domain.ReportWorkflowSchedule{},
	}
}

func (r *fakeConfigRepo) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	policy, ok := r.reportPolicies[id]
	if !ok {
		return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (r *fakeConfigRepo) SaveReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.saveScheduleCalls++
	r.savedSchedule = schedule
	schedule.ID = 9
	r.schedules[9] = schedule
	return schedule, nil
}

func (r *fakeConfigRepo) UpdateReportWorkflowSchedule(_ context.Context, schedule domain.ReportWorkflowSchedule) (domain.ReportWorkflowSchedule, error) {
	r.updateScheduleCalls++
	if _, ok := r.schedules[schedule.ID]; !ok {
		return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
	}
	r.schedules[schedule.ID] = schedule
	return schedule, nil
}

func (r *fakeConfigRepo) FindReportWorkflowScheduleByID(_ context.Context, id domain.ReportWorkflowScheduleID) (domain.ReportWorkflowSchedule, error) {
	schedule, ok := r.schedules[id]
	if !ok {
		return domain.ReportWorkflowSchedule{}, domain.ErrNotFound
	}
	return schedule, nil
}
