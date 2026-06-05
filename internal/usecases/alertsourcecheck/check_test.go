package alertsourcecheck

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

var fixedCheckedAt = time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)

func TestServicePrometheusNoAuthSuccess(t *testing.T) {
	service := newTestService(t, func(profile domain.AlertSourceProfile) (ports.MetricsProvider, error) {
		if profile.BaseURL != "https://prometheus.example.test" {
			t.Fatalf("profile base URL = %q", profile.BaseURL)
		}
		return fakeMetricsProvider{
			alerts: []ports.ActiveAlert{
				{Source: "prometheus"},
				{Source: "prometheus"},
			},
		}, nil
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 1, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusSuccess || result.ReasonCode != ReasonOK || result.ObservedAlerts != 2 {
		t.Fatalf("result = %+v", result)
	}
	if !result.CheckedAt.Equal(fixedCheckedAt) {
		t.Fatalf("checked_at = %s, want %s", result.CheckedAt, fixedCheckedAt)
	}
}

func TestServiceBlocksBearerWithoutSecretResolver(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
		t.Fatal("factory should not be called for bearer profiles without secret resolver")
		return nil, nil
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 2, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeBearer))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusBlocked || result.ReasonCode != ReasonCredentialsUnavailable || result.ObservedAlerts != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceReturnsUnsupportedForAlertmanager(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
		t.Fatal("factory should not be called for unsupported profile kinds")
		return nil, nil
	})

	for _, authMode := range []domain.AlertSourceAuthMode{
		domain.AlertSourceAuthModeNone,
		domain.AlertSourceAuthModeBearer,
	} {
		result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 3, domain.AlertSourceKindAlertmanager, authMode))
		if err != nil {
			t.Fatalf("TestAlertSourceConnection: %v", err)
		}
		if result.Status != StatusUnsupported || result.ReasonCode != ReasonUnsupportedKind {
			t.Fatalf("authMode %q result = %+v", authMode, result)
		}
	}
}

func TestServiceSanitizesFactoryAndProviderErrors(t *testing.T) {
	// #nosec G101 -- test-only credential-bearing URL fixture verifies sanitization.
	rawEndpoint := "https://operator:token@prometheus.example.test"
	tests := []struct {
		name    string
		factory MetricsProviderFactory
	}{
		{
			name: "factory",
			factory: func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
				return nil, errors.New(rawEndpoint)
			},
		},
		{
			name: "provider",
			factory: func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
				return fakeMetricsProvider{err: errors.New(rawEndpoint)}, nil
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestService(t, tc.factory)
			result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 4, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone))
			if err != nil {
				t.Fatalf("TestAlertSourceConnection: %v", err)
			}
			if result.Status != StatusFailed {
				t.Fatalf("status = %q, want failed", result.Status)
			}
			if strings.Contains(result.Message, rawEndpoint) || strings.Contains(result.Message, "token") {
				t.Fatalf("message leaked raw provider data: %q", result.Message)
			}
		})
	}
}

func TestServiceTimeoutIsSanitized(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
		return blockingMetricsProvider{}, nil
	}, WithTimeout(time.Millisecond))

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 5, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusFailed || result.ReasonCode != ReasonUpstreamUnreachable {
		t.Fatalf("result = %+v", result)
	}
}

func TestNewServiceValidation(t *testing.T) {
	if _, err := NewService(nil, WithClock(func() time.Time { return fixedCheckedAt })); err == nil {
		t.Fatal("NewService with nil factory: want error")
	}
	if _, err := NewService(func(domain.AlertSourceProfile) (ports.MetricsProvider, error) {
		return fakeMetricsProvider{}, nil
	}); err == nil {
		t.Fatal("NewService without clock: want error")
	}
}

func newTestService(t *testing.T, factory MetricsProviderFactory, opts ...Option) *Service {
	t.Helper()
	opts = append([]Option{WithClock(func() time.Time { return fixedCheckedAt })}, opts...)
	service, err := NewService(factory, opts...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func mustProfile(t *testing.T, id domain.AlertSourceProfileID, kind domain.AlertSourceKind, authMode domain.AlertSourceAuthMode) domain.AlertSourceProfile {
	t.Helper()
	secretRef := ""
	if authMode == domain.AlertSourceAuthModeBearer {
		secretRef = "secret/openclarion/prometheus-bearer"
	}
	profile, err := domain.NewAlertSourceProfile(
		"Primary Prometheus",
		kind,
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

type fakeMetricsProvider struct {
	alerts []ports.ActiveAlert
	err    error
}

func (p fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return p.alerts, p.err
}

type blockingMetricsProvider struct{}

func (blockingMetricsProvider) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
