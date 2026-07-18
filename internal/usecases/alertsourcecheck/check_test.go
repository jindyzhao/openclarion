package alertsourcecheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

var fixedCheckedAt = time.Date(2026, 6, 5, 4, 0, 0, 0, time.UTC)

func TestServicePrometheusNoAuthSuccess(t *testing.T) {
	provider := &recordingMetricsProvider{
		alerts: []ports.ActiveAlert{
			{Source: "prometheus"},
			{Source: "prometheus"},
		},
	}
	service := newTestService(t, func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
		if profile.BaseURL != "https://prometheus.example.test" {
			t.Fatalf("profile base URL = %q", profile.BaseURL)
		}
		if credentials.BearerToken != "" {
			t.Fatalf("BearerToken = %q, want empty", credentials.BearerToken)
		}
		return provider, nil
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 1, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusSuccess || result.ReasonCode != ReasonOK || result.ObservedAlerts != 2 {
		t.Fatalf("result = %+v", result)
	}
	if result.Message != "Prometheus alert listing and metric query succeeded." {
		t.Fatalf("message = %q", result.Message)
	}
	if !result.CheckedAt.Equal(fixedCheckedAt) {
		t.Fatalf("checked_at = %s, want %s", result.CheckedAt, fixedCheckedAt)
	}
	if provider.listCalls != 1 || provider.queryCalls != 1 {
		t.Fatalf("provider calls list=%d query=%d", provider.listCalls, provider.queryCalls)
	}
	if provider.queryReq.Query != prometheusMetricProbeQuery ||
		!provider.queryReq.Time.Equal(fixedCheckedAt) ||
		provider.queryReq.Timeout != DefaultTimeout ||
		provider.queryReq.Limit != 1 {
		t.Fatalf("metric probe request = %+v", provider.queryReq)
	}
}

func TestServiceThanosRuleSkipsMetricProbe(t *testing.T) {
	provider := &recordingMetricsProvider{
		alerts: []ports.ActiveAlert{
			{Source: "prometheus"},
		},
	}
	service := newTestService(t, func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
		if profile.Kind != domain.AlertSourceKindPrometheus {
			t.Fatalf("profile kind = %q, want prometheus", profile.Kind)
		}
		if profile.Labels["source"] != "thanos-rule" {
			t.Fatalf("profile labels = %#v, want source=thanos-rule", profile.Labels)
		}
		if credentials.BearerToken != "" {
			t.Fatalf("BearerToken = %q, want empty", credentials.BearerToken)
		}
		return provider, nil
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustThanosRuleProfile(t, 7))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusSuccess || result.ReasonCode != ReasonOK || result.ObservedAlerts != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Message != "Thanos Rule alert listing succeeded." {
		t.Fatalf("message = %q", result.Message)
	}
	if provider.listCalls != 1 || provider.queryCalls != 0 {
		t.Fatalf("provider calls list=%d query=%d", provider.listCalls, provider.queryCalls)
	}
}

func TestServicePrometheusBearerUsesSecretResolver(t *testing.T) {
	service := newTestServiceWithBuilder(t, alertsourceprovider.ProviderFactories{
		domain.AlertSourceKindPrometheus: func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
			if profile.SecretRef != "secret/openclarion/prometheus-bearer" {
				t.Fatalf("SecretRef = %q", profile.SecretRef)
			}
			if credentials.BearerToken != "resolved-bearer-token" {
				t.Fatalf("BearerToken = %q", credentials.BearerToken)
			}
			return fakeMetricsProvider{
				alerts: []ports.ActiveAlert{{Source: "prometheus"}},
			}, nil
		},
	}, fakeSecretResolver{
		secrets: map[string]string{"secret/openclarion/prometheus-bearer": "resolved-bearer-token"},
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 2, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeBearer))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusSuccess || result.ReasonCode != ReasonOK || result.ObservedAlerts != 1 {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Message, "resolved-bearer-token") {
		t.Fatalf("result leaked token: %+v", result)
	}
}

func TestServiceBlocksBearerWithoutSecretResolver(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
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

func TestServiceBlocksBearerWhenSecretIsUnavailable(t *testing.T) {
	service := newTestServiceWithBuilder(t, alertsourceprovider.ProviderFactories{
		domain.AlertSourceKindPrometheus: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
			t.Fatal("factory should not be called when secret resolution fails")
			return nil, nil
		},
	}, fakeSecretResolver{})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 3, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeBearer))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusBlocked || result.ReasonCode != ReasonCredentialsUnavailable {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Message, "secret/openclarion/prometheus-bearer") {
		t.Fatalf("message leaked secret_ref: %q", result.Message)
	}
}

func TestServiceReturnsUnsupportedForAlertmanagerWithoutFactory(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
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

func TestServiceAlertmanagerNoAuthSuccess(t *testing.T) {
	provider := &recordingMetricsProvider{alerts: []ports.ActiveAlert{
		{Source: "alertmanager"},
		{Source: "alertmanager"},
		{Source: "alertmanager"},
	}}
	service := newTestServiceWithBuilder(t,
		alertsourceprovider.ProviderFactories{
			domain.AlertSourceKindAlertmanager: func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				if profile.Kind != domain.AlertSourceKindAlertmanager {
					t.Fatalf("kind = %q", profile.Kind)
				}
				if credentials.BearerToken != "" {
					t.Fatalf("BearerToken = %q, want empty", credentials.BearerToken)
				}
				return provider, nil
			},
		}, nil,
	)

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 4, domain.AlertSourceKindAlertmanager, domain.AlertSourceAuthModeNone))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusSuccess || result.ReasonCode != ReasonOK || result.ObservedAlerts != 3 {
		t.Fatalf("result = %+v", result)
	}
	if provider.listCalls != 1 || provider.queryCalls != 0 {
		t.Fatalf("provider calls list=%d query=%d", provider.listCalls, provider.queryCalls)
	}
}

func TestServiceSanitizesFactoryAndProviderErrors(t *testing.T) {
	// #nosec G101 -- test-only credential-bearing URL fixture verifies sanitization.
	rawEndpoint := "https://operator:token@prometheus.example.test"
	tests := []struct {
		name    string
		factory alertsourceprovider.ProviderFactory
	}{
		{
			name: "factory",
			factory: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return nil, errors.New(rawEndpoint)
			},
		},
		{
			name: "provider",
			factory: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
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

func TestServicePreservesPartialAlertCountOnProviderFailure(t *testing.T) {
	providerErr := errors.New("second upstream alert was malformed")
	service := newTestServiceWithBuilder(t,
		alertsourceprovider.ProviderFactories{
			domain.AlertSourceKindAlertmanager: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return fakeMetricsProvider{
					alerts: []ports.ActiveAlert{{Source: "alertmanager"}, {Source: "alertmanager"}},
					err:    providerErr,
				}, nil
			},
		}, nil,
	)

	result, err := service.TestAlertSourceConnection(
		context.Background(),
		mustProfile(t, 4, domain.AlertSourceKindAlertmanager, domain.AlertSourceAuthModeNone),
	)
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusFailed || result.ReasonCode != ReasonUpstreamError || result.ObservedAlerts != 2 {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Message, providerErr.Error()) {
		t.Fatalf("result leaked provider error: %+v", result)
	}
}

func TestServiceDiscardsPartialAlertCountOnProviderDeadline(t *testing.T) {
	service := newTestServiceWithBuilder(t,
		alertsourceprovider.ProviderFactories{
			domain.AlertSourceKindAlertmanager: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return fakeMetricsProvider{
					alerts: []ports.ActiveAlert{{Source: "alertmanager"}},
					err:    fmt.Errorf("provider timeout: %w", context.DeadlineExceeded),
				}, nil
			},
		}, nil,
	)

	result, err := service.TestAlertSourceConnection(
		context.Background(),
		mustProfile(t, 4, domain.AlertSourceKindAlertmanager, domain.AlertSourceAuthModeNone),
	)
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusFailed || result.ReasonCode != ReasonUpstreamUnreachable || result.ObservedAlerts != 0 {
		t.Fatalf("result = %+v, want timeout without partial count", result)
	}
}

func TestServicePrometheusMetricProbeFailureIsSanitized(t *testing.T) {
	// #nosec G101 -- test-only credential-bearing URL fixture verifies sanitization.
	rawEndpoint := "https://operator:token@prometheus.example.test/api/v1/query"
	service := newTestService(t, func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
		return metricProbeFailureProvider{err: errors.New(rawEndpoint)}, nil
	})

	result, err := service.TestAlertSourceConnection(context.Background(), mustProfile(t, 6, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone))
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusFailed || result.ReasonCode != ReasonUpstreamError {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Message, rawEndpoint) || strings.Contains(result.Message, "token") {
		t.Fatalf("message leaked raw provider data: %q", result.Message)
	}
}

func TestServiceReportsMissingPrometheusMetricCapability(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
		return alertOnlyProvider{}, nil
	})

	result, err := service.TestAlertSourceConnection(
		context.Background(),
		mustProfile(t, 8, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone),
	)
	if err != nil {
		t.Fatalf("TestAlertSourceConnection: %v", err)
	}
	if result.Status != StatusUnsupported || result.ReasonCode != ReasonCapabilityUnavailable {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceTimeoutIsSanitized(t *testing.T) {
	service := newTestService(t, func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
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
	builder, err := alertsourceprovider.NewBuilder(alertsourceprovider.ProviderFactories{
		domain.AlertSourceKindPrometheus: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
			return fakeMetricsProvider{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if _, err := NewService(builder); err == nil {
		t.Fatal("NewService without clock: want error")
	}
}

func newTestService(t *testing.T, factory alertsourceprovider.ProviderFactory, opts ...Option) *Service {
	t.Helper()
	return newTestServiceWithBuilder(t, alertsourceprovider.ProviderFactories{
		domain.AlertSourceKindPrometheus: factory,
	}, nil, opts...)
}

func newTestServiceWithBuilder(
	t *testing.T,
	factories alertsourceprovider.ProviderFactories,
	resolver ports.SecretResolver,
	opts ...Option,
) *Service {
	t.Helper()
	builderOpts := []alertsourceprovider.Option{}
	if resolver != nil {
		builderOpts = append(builderOpts, alertsourceprovider.WithSecretResolver(resolver))
	}
	builder, err := alertsourceprovider.NewBuilder(factories, builderOpts...)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	opts = append([]Option{WithClock(func() time.Time { return fixedCheckedAt })}, opts...)
	service, err := NewService(builder, opts...)
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

func mustThanosRuleProfile(t *testing.T, id domain.AlertSourceProfileID) domain.AlertSourceProfile {
	t.Helper()
	profile, err := domain.NewAlertSourceProfile(
		"Thanos Rule active alerts",
		domain.AlertSourceKindPrometheus,
		"https://thanos-rule.example.test",
		domain.AlertSourceAuthModeNone,
		"",
		true,
		map[string]string{"env": "test", "source": "thanos-rule"},
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

type alertOnlyProvider struct{}

func (alertOnlyProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

func (p fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return p.alerts, p.err
}

func (p fakeMetricsProvider) QueryMetric(context.Context, ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, p.err
}

func (p fakeMetricsProvider) QueryMetricRange(context.Context, ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, p.err
}

type recordingMetricsProvider struct {
	alerts     []ports.ActiveAlert
	listCalls  int
	queryCalls int
	queryReq   ports.MetricQueryRequest
}

func (p *recordingMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	p.listCalls++
	return p.alerts, nil
}

func (p *recordingMetricsProvider) QueryMetric(_ context.Context, req ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	p.queryCalls++
	p.queryReq = req
	return ports.MetricQueryResult{ResultType: "vector"}, nil
}

func (p *recordingMetricsProvider) QueryMetricRange(context.Context, ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
}

type metricProbeFailureProvider struct {
	err error
}

func (p metricProbeFailureProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

func (p metricProbeFailureProvider) QueryMetric(context.Context, ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, p.err
}

func (p metricProbeFailureProvider) QueryMetricRange(context.Context, ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
}

type blockingMetricsProvider struct{}

func (blockingMetricsProvider) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingMetricsProvider) QueryMetric(ctx context.Context, _ ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	<-ctx.Done()
	return ports.MetricQueryResult{}, ctx.Err()
}

func (blockingMetricsProvider) QueryMetricRange(ctx context.Context, _ ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	<-ctx.Done()
	return ports.MetricQueryResult{}, ctx.Err()
}

type fakeSecretResolver struct {
	secrets map[string]string
	err     error
}

func (r fakeSecretResolver) ResolveSecret(_ context.Context, ref string) (ports.Secret, error) {
	if r.err != nil {
		return ports.Secret{}, r.err
	}
	value, ok := r.secrets[ref]
	if !ok {
		return ports.Secret{}, ports.ErrSecretNotFound
	}
	return ports.Secret{Value: value}, nil
}
