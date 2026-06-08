package reportpolicytrigger

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

var replayWindowStart = time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)

func TestReplayAndStartResolvesPolicyBindings(t *testing.T) {
	source := mustAlertSourceProfile(t, 11, domain.AlertSourceAuthModeBearer)
	grouping := mustGroupingPolicy(t, 12, []string{"prometheus"})
	policy := mustReportWorkflowPolicy(t, 13, source.ID, grouping.ID, domain.ReportWorkflowScenarioCascade)
	policy.ReportNotificationChannelProfileID = 14
	factory := &fakePolicyUOWFactory{configRepo: &fakePolicyConfigRepo{
		sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: source},
		groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{grouping.ID: grouping},
		policies:  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{policy.ID: policy},
	}}
	providers, err := alertsourceprovider.NewBuilder(
		func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.MetricsProvider, error) {
			if profile.ID != source.ID {
				t.Fatalf("profile ID = %d, want %d", profile.ID, source.ID)
			}
			if credentials.BearerToken != "resolved-token" {
				t.Fatalf("BearerToken = %q, want resolved token", credentials.BearerToken)
			}
			return fakeMetricsProvider{}, nil
		},
		alertsourceprovider.WithSecretResolver(fakeSecretResolver{
			values: map[string]string{source.SecretRef: "resolved-token"},
		}),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	var captured reporttrigger.Request
	service, err := NewService(
		factory,
		fakeReportStarter{},
		providers,
		WithReplayAndStart(func(
			_ context.Context,
			provider ports.MetricsProvider,
			gotFactory ports.UnitOfWorkFactory,
			starter ports.ReportWorkflowStarter,
			req reporttrigger.Request,
		) (reporttrigger.Result, error) {
			if provider == nil || gotFactory != factory || starter == nil {
				t.Fatalf("bad replay dependencies provider=%v factory=%v starter=%v", provider, gotFactory, starter)
			}
			captured = req
			return reporttrigger.Result{Started: true, Workflow: ports.WorkflowHandle{WorkflowID: "wf-1", RunID: "run-1"}}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := service.ReplayAndStart(context.Background(), Request{
		PolicyID:    policy.ID,
		WindowStart: replayWindowStart,
		WindowEnd:   replayWindowStart.Add(time.Hour),
		Limit:       25,
	})
	if err != nil {
		t.Fatalf("ReplayAndStart: %v", err)
	}
	if !result.Started || result.Workflow.WorkflowID != "wf-1" {
		t.Fatalf("result = %+v", result)
	}
	if captured.Replay.CreatedByWorkflow != CreatedByWorkflow {
		t.Fatalf("CreatedByWorkflow = %q, want %q", captured.Replay.CreatedByWorkflow, CreatedByWorkflow)
	}
	if captured.Replay.Limit != 25 ||
		!captured.Replay.WindowStart.Equal(replayWindowStart) ||
		!captured.Replay.WindowEnd.Equal(replayWindowStart.Add(time.Hour)) {
		t.Fatalf("replay request = %+v", captured.Replay)
	}
	if captured.Replay.Grouping.SeverityKey != "severity" ||
		len(captured.Replay.Grouping.DimensionKeys) != 1 ||
		captured.Replay.Grouping.DimensionKeys[0] != "alertname" {
		t.Fatalf("grouping = %+v", captured.Replay.Grouping)
	}
	if len(captured.Replay.SourceFilter) != 1 || captured.Replay.SourceFilter[0] != "prometheus" {
		t.Fatalf("source filter = %+v", captured.Replay.SourceFilter)
	}
	wantCorrelation := "report-workflow-policy:13:2026-06-05T08:00:00Z:2026-06-05T09:00:00Z"
	if captured.CorrelationKey != wantCorrelation {
		t.Fatalf("CorrelationKey = %q, want %q", captured.CorrelationKey, wantCorrelation)
	}
	if captured.Scenario != reportprompt.ScenarioCascade {
		t.Fatalf("Scenario = %q, want cascade", captured.Scenario)
	}
	if captured.ReportNotificationChannelProfileID != 14 {
		t.Fatalf("ReportNotificationChannelProfileID = %d, want 14", captured.ReportNotificationChannelProfileID)
	}
}

func TestReplayAndStartAllowsCreatedByWorkflowOverride(t *testing.T) {
	source := mustAlertSourceProfile(t, 15, domain.AlertSourceAuthModeNone)
	grouping := mustGroupingPolicy(t, 16, nil)
	policy := mustReportWorkflowPolicy(t, 17, source.ID, grouping.ID, domain.ReportWorkflowScenarioSingleAlert)
	var captured reporttrigger.Request
	service := newTestPolicyTriggerService(t, &fakePolicyConfigRepo{
		sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: source},
		groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{grouping.ID: grouping},
		policies:  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{policy.ID: policy},
	}, func(_ *testing.T, req reporttrigger.Request) {
		captured = req
	})

	_, err := service.ReplayAndStart(context.Background(), Request{
		PolicyID:          policy.ID,
		WindowStart:       replayWindowStart,
		WindowEnd:         replayWindowStart.Add(time.Hour),
		Limit:             10,
		CreatedByWorkflow: "ReportPolicyScheduleLauncherWorkflow",
	})
	if err != nil {
		t.Fatalf("ReplayAndStart: %v", err)
	}
	if captured.Replay.CreatedByWorkflow != "ReportPolicyScheduleLauncherWorkflow" {
		t.Fatalf("CreatedByWorkflow = %q", captured.Replay.CreatedByWorkflow)
	}
}

func TestReplayAndStartRejectsDisabledBindings(t *testing.T) {
	source := mustAlertSourceProfile(t, 21, domain.AlertSourceAuthModeNone)
	grouping := mustGroupingPolicy(t, 22, nil)
	policy := mustReportWorkflowPolicy(t, 23, source.ID, grouping.ID, domain.ReportWorkflowScenarioSingleAlert)

	tests := []struct {
		name     string
		source   domain.AlertSourceProfile
		grouping domain.GroupingPolicy
		policy   domain.ReportWorkflowPolicy
	}{
		{name: "policy", source: source, grouping: grouping, policy: withPolicyEnabled(policy, false)},
		{name: "source", source: withSourceEnabled(source, false), grouping: grouping, policy: policy},
		{name: "grouping", source: source, grouping: withGroupingEnabled(grouping, false), policy: policy},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestPolicyTriggerService(t, &fakePolicyConfigRepo{
				sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: tc.source},
				groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{grouping.ID: tc.grouping},
				policies:  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{policy.ID: tc.policy},
			}, nil)

			_, err := service.ReplayAndStart(context.Background(), Request{
				PolicyID:    policy.ID,
				WindowStart: replayWindowStart,
				WindowEnd:   replayWindowStart.Add(time.Hour),
				Limit:       1,
			})
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestReplayAndStartRejectsMissingCredentialsWithoutLeakingSecretRef(t *testing.T) {
	source := mustAlertSourceProfile(t, 31, domain.AlertSourceAuthModeBearer)
	grouping := mustGroupingPolicy(t, 32, nil)
	policy := mustReportWorkflowPolicy(t, 33, source.ID, grouping.ID, domain.ReportWorkflowScenarioSingleAlert)
	service := newTestPolicyTriggerService(t, &fakePolicyConfigRepo{
		sources:   map[domain.AlertSourceProfileID]domain.AlertSourceProfile{source.ID: source},
		groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{grouping.ID: grouping},
		policies:  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{policy.ID: policy},
	}, nil)

	_, err := service.ReplayAndStart(context.Background(), Request{
		PolicyID:    policy.ID,
		WindowStart: replayWindowStart,
		WindowEnd:   replayWindowStart.Add(time.Hour),
		Limit:       1,
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
	if strings.Contains(err.Error(), source.SecretRef) {
		t.Fatalf("error leaked secret ref: %q", err.Error())
	}
}

func TestReplayAndStartRejectsBadRequestBeforeReplay(t *testing.T) {
	service := newTestPolicyTriggerService(t, &fakePolicyConfigRepo{}, func(t *testing.T, _ reporttrigger.Request) {
		t.Fatal("replay should not be called for invalid requests")
	})
	_, err := service.ReplayAndStart(context.Background(), Request{
		PolicyID:    0,
		WindowStart: replayWindowStart,
		WindowEnd:   replayWindowStart.Add(time.Hour),
		Limit:       1,
	})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
}

func newTestPolicyTriggerService(
	t *testing.T,
	repo *fakePolicyConfigRepo,
	onReplay func(*testing.T, reporttrigger.Request),
) *Service {
	t.Helper()
	providers, err := alertsourceprovider.NewBuilder(func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.MetricsProvider, error) {
		return fakeMetricsProvider{}, nil
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	service, err := NewService(
		&fakePolicyUOWFactory{configRepo: repo},
		fakeReportStarter{},
		providers,
		WithReplayAndStart(func(
			_ context.Context,
			_ ports.MetricsProvider,
			_ ports.UnitOfWorkFactory,
			_ ports.ReportWorkflowStarter,
			req reporttrigger.Request,
		) (reporttrigger.Result, error) {
			if onReplay != nil {
				onReplay(t, req)
			}
			return reporttrigger.Result{}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func mustAlertSourceProfile(
	t *testing.T,
	id domain.AlertSourceProfileID,
	authMode domain.AlertSourceAuthMode,
) domain.AlertSourceProfile {
	t.Helper()
	secretRef := ""
	if authMode == domain.AlertSourceAuthModeBearer {
		secretRef = "secret/openclarion/prometheus-bearer"
	}
	profile, err := domain.NewAlertSourceProfile(
		"Primary Prometheus",
		domain.AlertSourceKindPrometheus,
		"https://prometheus.example.test",
		authMode,
		secretRef,
		true,
		map[string]string{"env": "test"},
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	profile.ID = id
	return profile
}

func mustGroupingPolicy(t *testing.T, id domain.GroupingPolicyID, sourceFilter []string) domain.GroupingPolicy {
	t.Helper()
	policy, err := domain.NewGroupingPolicy(
		"Primary grouping",
		[]string{"alertname"},
		"severity",
		sourceFilter,
		true,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	policy.ID = id
	return policy
}

func mustReportWorkflowPolicy(
	t *testing.T,
	id domain.ReportWorkflowPolicyID,
	sourceID domain.AlertSourceProfileID,
	groupingID domain.GroupingPolicyID,
	scenario domain.ReportWorkflowScenario,
) domain.ReportWorkflowPolicy {
	t.Helper()
	enabledAt := replayWindowStart.Add(-time.Hour)
	policy, err := domain.NewReportWorkflowPolicy(
		"Manual replay",
		sourceID,
		groupingID,
		0,
		domain.ReportWorkflowTriggerModeManualReplay,
		scenario,
		domain.DiagnosisFollowUpModeDisabled,
		true,
		&enabledAt,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	policy.ID = id
	return policy
}

func withPolicyEnabled(policy domain.ReportWorkflowPolicy, enabled bool) domain.ReportWorkflowPolicy {
	policy.Enabled = enabled
	if !enabled {
		policy.EnabledAt = nil
	}
	return policy
}

func withSourceEnabled(profile domain.AlertSourceProfile, enabled bool) domain.AlertSourceProfile {
	profile.Enabled = enabled
	return profile
}

func withGroupingEnabled(policy domain.GroupingPolicy, enabled bool) domain.GroupingPolicy {
	policy.Enabled = enabled
	return policy
}

type fakePolicyUOWFactory struct {
	configRepo ports.ConfigurationRepository
}

func (f *fakePolicyUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return &fakePolicyUOW{configRepo: f.configRepo}, nil
}

func (f *fakePolicyUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, &fakePolicyUOW{configRepo: f.configRepo})
}

type fakePolicyUOW struct {
	ports.UnitOfWork
	configRepo ports.ConfigurationRepository
}

func (u *fakePolicyUOW) Config() ports.ConfigurationRepository {
	return u.configRepo
}

type fakePolicyConfigRepo struct {
	ports.ConfigurationRepository
	sources   map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	groupings map[domain.GroupingPolicyID]domain.GroupingPolicy
	policies  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy
}

func (r *fakePolicyConfigRepo) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	source, ok := r.sources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return source, nil
}

func (r *fakePolicyConfigRepo) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	grouping, ok := r.groupings[id]
	if !ok {
		return domain.GroupingPolicy{}, domain.ErrNotFound
	}
	return grouping, nil
}

func (r *fakePolicyConfigRepo) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	policy, ok := r.policies[id]
	if !ok {
		return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

type fakeMetricsProvider struct{}

func (fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

type fakeReportStarter struct{}

func (fakeReportStarter) StartReportBatch(context.Context, ports.ReportBatchStartRequest) (ports.WorkflowHandle, error) {
	return ports.WorkflowHandle{}, nil
}

type fakeSecretResolver struct {
	values map[string]string
	err    error
}

func (r fakeSecretResolver) ResolveSecret(_ context.Context, ref string) (ports.Secret, error) {
	if r.err != nil {
		return ports.Secret{}, r.err
	}
	value, ok := r.values[ref]
	if !ok {
		return ports.Secret{}, ports.ErrSecretNotFound
	}
	return ports.Secret{Value: value}, nil
}
