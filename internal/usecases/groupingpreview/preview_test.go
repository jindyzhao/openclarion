package groupingpreview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestPreviewGroupsBoundedPersistedEventsWithoutExternalCalls(t *testing.T) {
	base := time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)
	policy := mustPolicy(t)
	policy.ID = 7
	factory := &fakeFactory{
		config: &fakeConfig{policy: policy},
		alerts: &fakeAlerts{events: []domain.AlertEvent{
			event(1, "prometheus", "checkout", "warning", base),
			event(2, "prometheus", "checkout", "critical", base.Add(time.Minute)),
			event(3, "alertmanager", "payments", "info", base.Add(2*time.Minute)),
		}},
	}

	result, err := NewService(factory).Preview(context.Background(), Request{PolicyID: 7, Limit: 100})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if factory.alerts.lastLimit != 100 {
		t.Fatalf("last limit = %d, want 100", factory.alerts.lastLimit)
	}
	if result.Policy.ID != 7 || result.EventsScanned != 3 || result.EventsMatched != 2 {
		t.Fatalf("result counts = %+v", result)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups len = %d, want 1", len(result.Groups))
	}
	group := result.Groups[0]
	if group.Dimensions["alertname"] != "checkout" || group.Severity != domain.GroupSeverityCritical {
		t.Fatalf("group = %+v", group)
	}
	if len(group.EventIDs) != 2 || group.EventIDs[0] != 1 || group.EventIDs[1] != 2 {
		t.Fatalf("event IDs = %v", group.EventIDs)
	}
}

func TestPreviewReturnsEmptyGroupsWhenSourceFilterMatchesNothing(t *testing.T) {
	policy := mustPolicy(t)
	policy.ID = 7
	policy.SourceFilter = []string{"alertmanager"}
	factory := &fakeFactory{
		config: &fakeConfig{policy: policy},
		alerts: &fakeAlerts{events: []domain.AlertEvent{
			event(1, "prometheus", "checkout", "warning", time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)),
		}},
	}

	result, err := NewService(factory).Preview(context.Background(), Request{PolicyID: 7, Limit: 100})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if result.EventsScanned != 1 || result.EventsMatched != 0 || len(result.Groups) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestPreviewRejectsInvalidRequest(t *testing.T) {
	_, err := NewService(&fakeFactory{}).Preview(context.Background(), Request{PolicyID: 0, Limit: 100})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("zero policy err = %v, want ErrInvariantViolation", err)
	}
	_, err = NewService(&fakeFactory{}).Preview(context.Background(), Request{PolicyID: 7, Limit: 0})
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("zero limit err = %v, want ErrInvariantViolation", err)
	}
}

func mustPolicy(t *testing.T) domain.GroupingPolicy {
	t.Helper()
	policy, err := domain.NewGroupingPolicy(
		"Default grouping",
		[]string{"alertname"},
		"severity",
		[]string{"prometheus"},
		true,
	)
	if err != nil {
		t.Fatalf("NewGroupingPolicy: %v", err)
	}
	return policy
}

func event(id int64, source, alertName, severity string, startsAt time.Time) domain.AlertEvent {
	return domain.AlertEvent{
		ID:       domain.AlertEventID(id),
		Source:   source,
		Labels:   map[string]string{"alertname": alertName, "severity": severity},
		StartsAt: startsAt,
	}
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
	policy domain.GroupingPolicy
}

func (c *fakeConfig) FindGroupingPolicyByID(_ context.Context, id domain.GroupingPolicyID) (domain.GroupingPolicy, error) {
	if c.policy.ID == id {
		return c.policy, nil
	}
	return domain.GroupingPolicy{}, domain.ErrNotFound
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
