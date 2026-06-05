package reportworkflowimpact

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestPreviewReturnsReadyImpactFromPersistedBindings(t *testing.T) {
	base := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	policy := mustReportWorkflowPolicy(t, 13, 1, 2, 3)
	factory := &fakeFactory{
		config: &fakeConfig{
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				1: {ID: 1, Kind: domain.AlertSourceKindPrometheus, AuthMode: domain.AlertSourceAuthModeBearer, Enabled: true},
			},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				2: mustGroupingPolicy(t, 2, true),
			},
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				3: {
					ID:             3,
					Enabled:        true,
					DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
				},
			},
			policies: map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{13: policy},
		},
		alerts: &fakeAlerts{events: []domain.AlertEvent{
			alertEvent(101, "prometheus", "checkout", "warning", base),
			alertEvent(102, "prometheus", "checkout", "critical", base.Add(time.Minute)),
			alertEvent(103, "alertmanager", "payments", "info", base.Add(2*time.Minute)),
		}},
	}
	svc, err := NewService(factory, WithClock(func() time.Time { return base.Add(time.Hour) }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Preview(context.Background(), Request{PolicyID: 13, Limit: 100})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if factory.alerts.lastLimit != 100 {
		t.Fatalf("last limit = %d, want 100", factory.alerts.lastLimit)
	}
	if result.Status != StatusReady || !sameReasons(result.ReasonCodes, []ReasonCode{ReasonOK}) {
		t.Fatalf("status/reasons = %s/%v", result.Status, result.ReasonCodes)
	}
	if result.EventsScanned != 3 || result.EventsMatched != 2 || len(result.Groups) != 1 {
		t.Fatalf("impact counts = scanned %d matched %d groups %d", result.EventsScanned, result.EventsMatched, len(result.Groups))
	}
	if !result.ReportNotificationChannelBound ||
		!result.ReportNotificationChannelEnabled ||
		!result.ReportNotificationChannelReportScope {
		t.Fatalf("channel readiness = bound %t enabled %t scope %t",
			result.ReportNotificationChannelBound,
			result.ReportNotificationChannelEnabled,
			result.ReportNotificationChannelReportScope,
		)
	}
	if result.Message == "" || result.CheckedAt.IsZero() {
		t.Fatalf("message/checked_at missing: %+v", result)
	}
}

func TestPreviewReturnsReviewWhenNoRecentEventsMatch(t *testing.T) {
	base := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	factory := readyFakeFactory(t, []domain.AlertEvent{
		alertEvent(101, "alertmanager", "checkout", "warning", base),
	})
	svc, err := NewService(factory, WithClock(func() time.Time { return base }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Preview(context.Background(), Request{PolicyID: 13, Limit: 50})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if result.Status != StatusReview || !sameReasons(result.ReasonCodes, []ReasonCode{ReasonNoMatchingEvents}) {
		t.Fatalf("status/reasons = %s/%v", result.Status, result.ReasonCodes)
	}
	if result.EventsScanned != 1 || result.EventsMatched != 0 || len(result.Groups) != 0 {
		t.Fatalf("impact counts = %+v", result)
	}
}

func TestPreviewReturnsBlockedReasonsWithoutStartingAnything(t *testing.T) {
	base := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	policy := mustReportWorkflowPolicy(t, 13, 1, 2, 3)
	factory := readyFakeFactory(t, []domain.AlertEvent{
		alertEvent(101, "prometheus", "checkout", "warning", base),
	})
	factory.config.policies[13] = policy
	factory.config.sources[1] = domain.AlertSourceProfile{
		ID:       1,
		Kind:     domain.AlertSourceKindPrometheus,
		AuthMode: domain.AlertSourceAuthModeNone,
		Enabled:  false,
	}
	grouping := factory.config.groupings[2]
	grouping.Enabled = false
	factory.config.groupings[2] = grouping
	factory.config.channels[3] = domain.NotificationChannelProfile{
		ID:             3,
		Enabled:        false,
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose},
	}
	svc, err := NewService(factory, WithClock(func() time.Time { return base }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Preview(context.Background(), Request{PolicyID: 13, Limit: 25})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := []ReasonCode{
		ReasonAlertSourceDisabled,
		ReasonGroupingPolicyDisabled,
		ReasonNotificationChannelDisabled,
		ReasonNotificationChannelMissingReportScope,
	}
	if result.Status != StatusBlocked || !sameReasons(result.ReasonCodes, want) {
		t.Fatalf("status/reasons = %s/%v, want blocked/%v", result.Status, result.ReasonCodes, want)
	}
	if result.EventsScanned != 1 || result.EventsMatched != 1 || len(result.Groups) != 1 {
		t.Fatalf("blocked preview should still show bounded grouping impact: %+v", result)
	}
}

func TestPreviewRejectsInvalidRequest(t *testing.T) {
	svc, err := NewService(&fakeFactory{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = svc.Preview(context.Background(), Request{PolicyID: 0, Limit: 100})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("zero policy err = %v, want ErrInvariantViolation", err)
	}
	_, err = svc.Preview(context.Background(), Request{PolicyID: 13, Limit: 0})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("zero limit err = %v, want ErrInvariantViolation", err)
	}
}

func readyFakeFactory(t *testing.T, events []domain.AlertEvent) *fakeFactory {
	t.Helper()
	policy := mustReportWorkflowPolicy(t, 13, 1, 2, 0)
	return &fakeFactory{
		config: &fakeConfig{
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				1: {ID: 1, Kind: domain.AlertSourceKindPrometheus, AuthMode: domain.AlertSourceAuthModeNone, Enabled: true},
			},
			groupings: map[domain.GroupingPolicyID]domain.GroupingPolicy{
				2: mustGroupingPolicy(t, 2, true),
			},
			channels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{},
			policies: map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy{13: policy},
		},
		alerts: &fakeAlerts{events: events},
	}
}

func mustReportWorkflowPolicy(
	t *testing.T,
	id domain.ReportWorkflowPolicyID,
	sourceID domain.AlertSourceProfileID,
	groupingID domain.GroupingPolicyID,
	channelID domain.NotificationChannelProfileID,
) domain.ReportWorkflowPolicy {
	t.Helper()
	policy, err := domain.NewReportWorkflowPolicy(
		"Default report workflow",
		sourceID,
		groupingID,
		channelID,
		domain.ReportWorkflowTriggerModeManualReplay,
		domain.ReportWorkflowScenarioSingleAlert,
		domain.DiagnosisFollowUpModeSuggestRoom,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewReportWorkflowPolicy: %v", err)
	}
	policy.ID = id
	return policy
}

func mustGroupingPolicy(t *testing.T, id domain.GroupingPolicyID, enabled bool) domain.GroupingPolicy {
	t.Helper()
	policy, err := domain.NewGroupingPolicy(
		"Default grouping",
		[]string{"alertname"},
		"severity",
		[]string{"prometheus"},
		enabled,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	policy.ID = id
	return policy
}

func alertEvent(id int64, source, alertName, severity string, startsAt time.Time) domain.AlertEvent {
	return domain.AlertEvent{
		ID:       domain.AlertEventID(id),
		Source:   source,
		Labels:   map[string]string{"alertname": alertName, "severity": severity},
		StartsAt: startsAt,
	}
}

func sameReasons(got, want []ReasonCode) bool {
	return slices.Equal(got, want)
}

type fakeFactory struct {
	config *fakeConfig
	alerts *fakeAlerts
}

func (f *fakeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, &fakeUOW{config: f.config, alerts: f.alerts})
}

type fakeUOW struct {
	ports.UnitOfWork
	config ports.ConfigurationRepository
	alerts ports.AlertRepository
}

func (u *fakeUOW) Config() ports.ConfigurationRepository { return u.config }
func (u *fakeUOW) Alerts() ports.AlertRepository         { return u.alerts }

type fakeConfig struct {
	ports.ConfigurationRepository
	sources   map[domain.AlertSourceProfileID]domain.AlertSourceProfile
	groupings map[domain.GroupingPolicyID]domain.GroupingPolicy
	channels  map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
	policies  map[domain.ReportWorkflowPolicyID]domain.ReportWorkflowPolicy
}

func (c *fakeConfig) FindReportWorkflowPolicyByID(_ context.Context, id domain.ReportWorkflowPolicyID) (domain.ReportWorkflowPolicy, error) {
	policy, ok := c.policies[id]
	if !ok {
		return domain.ReportWorkflowPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (c *fakeConfig) FindAlertSourceProfileByID(_ context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	source, ok := c.sources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return source, nil
}

func (c *fakeConfig) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	policy, ok := c.groupings[id]
	if !ok {
		return domain.GroupingPolicy{}, domain.ErrNotFound
	}
	return policy, nil
}

func (c *fakeConfig) FindNotificationChannelProfileByID(_ context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	channel, ok := c.channels[id]
	if !ok {
		return domain.NotificationChannelProfile{}, domain.ErrNotFound
	}
	return channel, nil
}

type fakeAlerts struct {
	ports.AlertRepository
	events    []domain.AlertEvent
	lastLimit int
}

func (a *fakeAlerts) ListEvents(_ context.Context, limit int) ([]domain.AlertEvent, error) {
	a.lastLimit = limit
	if limit > len(a.events) {
		limit = len(a.events)
	}
	return a.events[:limit], nil
}
