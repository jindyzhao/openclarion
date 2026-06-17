package alertsourceprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestBuilderBuildsPrometheusProviderWithoutCredentials(t *testing.T) {
	profile := mustProviderProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone)
	provider := fakeMetricsProvider{}
	builder, err := NewBuilder(func(got domain.AlertSourceProfile, credentials Credentials) (ports.MetricsProvider, error) {
		if got.ID != profile.ID {
			t.Fatalf("profile ID = %d, want %d", got.ID, profile.ID)
		}
		if credentials.BearerToken != "" {
			t.Fatalf("BearerToken = %q, want empty", credentials.BearerToken)
		}
		return provider, nil
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	got, err := builder.Build(context.Background(), profile)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got != provider {
		t.Fatalf("provider = %v, want configured fake", got)
	}
}

func TestBuilderBuildsAlertmanagerProviderWithBearerCredentials(t *testing.T) {
	profile := mustProviderProfile(t, domain.AlertSourceKindAlertmanager, domain.AlertSourceAuthModeBearer)
	builder, err := NewBuilder(
		func(domain.AlertSourceProfile, Credentials) (ports.MetricsProvider, error) {
			t.Fatal("prometheus factory should not be called for Alertmanager")
			return nil, nil
		},
		WithAlertmanagerFactory(func(got domain.AlertSourceProfile, credentials Credentials) (ports.MetricsProvider, error) {
			if got.Kind != domain.AlertSourceKindAlertmanager {
				t.Fatalf("kind = %q, want alertmanager", got.Kind)
			}
			if credentials.BearerToken != "resolved-token" {
				t.Fatalf("BearerToken = %q, want resolved-token", credentials.BearerToken)
			}
			return fakeMetricsProvider{}, nil
		}),
		WithSecretResolver(fakeSecretResolver{values: map[string]string{profile.SecretRef: "resolved-token"}}),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	if _, err := builder.Build(context.Background(), profile); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func TestBuilderRejectsUnsupportedKindAndNilProvider(t *testing.T) {
	builder, err := NewBuilder(func(domain.AlertSourceProfile, Credentials) (ports.MetricsProvider, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	unsupported := mustProviderProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone)
	unsupported.Kind = domain.AlertSourceKind("custom")
	if _, err := builder.Build(context.Background(), unsupported); !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("unsupported err = %v, want ErrUnsupportedKind", err)
	}

	prometheus := mustProviderProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeNone)
	if _, err := builder.Build(context.Background(), prometheus); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("nil provider err = %v, want ErrInvariantViolation", err)
	}
}

func TestResolveCredentialsSanitizesFailureReasons(t *testing.T) {
	profile := mustProviderProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeBearer)
	tests := []struct {
		name     string
		resolver ports.SecretResolver
		wantErr  error
	}{
		{name: "missing_resolver", wantErr: ErrSecretResolverUnavailable},
		{name: "missing_secret", resolver: fakeSecretResolver{}, wantErr: ErrSecretNotFound},
		{name: "resolver_failure", resolver: fakeSecretResolver{err: errors.New("backend leaked detail")}, wantErr: ErrSecretResolveFailed},
		{name: "empty_secret", resolver: fakeSecretResolver{values: map[string]string{profile.SecretRef: ""}}, wantErr: ErrCredentialUnusable},
		{name: "space_secret", resolver: fakeSecretResolver{values: map[string]string{profile.SecretRef: "bad token"}}, wantErr: ErrCredentialUnusable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveCredentials(context.Background(), tc.resolver, profile)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "backend leaked detail") {
				t.Fatalf("raw resolver error leaked: %v", err)
			}
		})
	}
}

func TestResolveCredentialsReturnsBearerToken(t *testing.T) {
	profile := mustProviderProfile(t, domain.AlertSourceKindPrometheus, domain.AlertSourceAuthModeBearer)
	credentials, err := ResolveCredentials(
		context.Background(),
		fakeSecretResolver{values: map[string]string{profile.SecretRef: "resolved-token"}},
		profile,
	)
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if credentials.BearerToken != "resolved-token" {
		t.Fatalf("BearerToken = %q, want resolved-token", credentials.BearerToken)
	}
}

func TestNewBuilderRequiresPrometheusFactory(t *testing.T) {
	if _, err := NewBuilder(nil); !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("NewBuilder err = %v, want ErrInvariantViolation", err)
	}
}

func mustProviderProfile(
	t *testing.T,
	kind domain.AlertSourceKind,
	authMode domain.AlertSourceAuthMode,
) domain.AlertSourceProfile {
	t.Helper()
	secretRef := ""
	if authMode == domain.AlertSourceAuthModeBearer {
		secretRef = "secret/openclarion/alert-source"
	}
	profile, err := domain.NewAlertSourceProfile(
		"Primary alert source",
		kind,
		"https://alerts.example.test",
		authMode,
		secretRef,
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	profile.ID = 1
	return profile
}

type fakeMetricsProvider struct{}

func (fakeMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

func (fakeMetricsProvider) QueryMetric(context.Context, ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
}

func (fakeMetricsProvider) QueryMetricRange(context.Context, ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
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
