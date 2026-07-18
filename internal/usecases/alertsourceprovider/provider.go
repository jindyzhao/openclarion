// Package alertsourceprovider builds runtime metrics providers from
// operator-managed alert source profiles.
package alertsourceprovider

import (
	"context"
	"errors"
	"fmt"
	"unicode"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

var (
	// ErrUnsupportedKind is returned when no runtime provider factory exists
	// for the stored alert source kind.
	ErrUnsupportedKind = errors.New("alert source provider kind is unsupported")
	// ErrSecretResolverUnavailable is returned when a bearer profile is used
	// without a server-side secret resolver.
	ErrSecretResolverUnavailable = errors.New("alert source secret resolver is unavailable")
	// ErrSecretNotFound is returned when the configured secret reference is not
	// available to the server-side resolver.
	ErrSecretNotFound = errors.New("alert source secret reference is unavailable")
	// ErrSecretResolveFailed is returned for resolver failures other than a
	// missing secret reference. The underlying resolver error is intentionally
	// not wrapped so callers cannot leak secret refs or provider details.
	ErrSecretResolveFailed = errors.New("alert source secret reference could not be resolved")
	// ErrCredentialUnusable is returned when a resolved credential is empty or
	// contains control/space characters.
	ErrCredentialUnusable = errors.New("alert source credential is unusable")
)

// Credentials contains resolved credentials for one provider call.
type Credentials struct {
	BearerToken string
}

// ProviderFactory builds the minimum active-alert capability from a stored
// source profile plus backend-resolved credentials. Factories may return a
// provider that also implements ports.MetricQueryProvider.
type ProviderFactory func(domain.AlertSourceProfile, Credentials) (ports.ActiveAlertProvider, error)

// ProviderFactories maps persisted source kinds to their runtime adapters.
// New source kinds extend this registry without changing Builder.Build.
type ProviderFactories map[domain.AlertSourceKind]ProviderFactory

// Builder coordinates profile-kind routing and server-side credential
// resolution for runtime provider construction.
type Builder struct {
	factories      ProviderFactories
	secretResolver ports.SecretResolver
}

// Option customizes Builder construction.
type Option func(*Builder)

// WithSecretResolver enables bearer-backed profile construction.
func WithSecretResolver(resolver ports.SecretResolver) Option {
	return func(b *Builder) {
		if resolver != nil {
			b.secretResolver = resolver
		}
	}
}

// NewBuilder constructs an alert source provider builder from a cloned,
// validated factory registry.
func NewBuilder(factories ProviderFactories, opts ...Option) (*Builder, error) {
	if len(factories) == 0 {
		return nil, fmt.Errorf("alert source provider: at least one factory is required: %w", domain.ErrInvariantViolation)
	}
	cloned := make(ProviderFactories, len(factories))
	for kind, factory := range factories {
		if !kind.Valid() {
			return nil, fmt.Errorf("alert source provider: factory kind %q is unsupported: %w", kind, domain.ErrInvariantViolation)
		}
		if factory == nil {
			return nil, fmt.Errorf("alert source provider: factory for kind %q is nil: %w", kind, domain.ErrInvariantViolation)
		}
		cloned[kind] = factory
	}
	builder := &Builder{factories: cloned}
	for _, opt := range opts {
		if opt != nil {
			opt(builder)
		}
	}
	return builder, nil
}

// Build resolves credentials and constructs a runtime provider for profile.
func (b *Builder) Build(ctx context.Context, profile domain.AlertSourceProfile) (ports.ActiveAlertProvider, error) {
	if b == nil || len(b.factories) == 0 {
		return nil, fmt.Errorf("alert source provider: builder is not configured: %w", domain.ErrInvariantViolation)
	}
	factory, ok := b.factories[profile.Kind]
	if !ok {
		return nil, ErrUnsupportedKind
	}
	credentials, err := ResolveCredentials(ctx, b.secretResolver, profile)
	if err != nil {
		return nil, err
	}
	provider, err := factory(profile, credentials)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("alert source provider: factory returned nil provider: %w", domain.ErrInvariantViolation)
	}
	activeProvider := sourceProfileProvider{
		profileID: profile.ID,
		inner:     provider,
	}
	metricProvider, ok := provider.(ports.MetricQueryProvider)
	if !ok {
		return activeProvider, nil
	}
	return sourceProfileMetricsProvider{
		ActiveAlertProvider: activeProvider,
		metricProvider:      metricProvider,
	}, nil
}

type sourceProfileProvider struct {
	profileID domain.AlertSourceProfileID
	inner     ports.ActiveAlertProvider
}

var _ ports.ActiveAlertProvider = sourceProfileProvider{}

func (p sourceProfileProvider) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	alerts, err := p.inner.ListActiveAlerts(ctx)
	if p.profileID == 0 || len(alerts) == 0 {
		return alerts, err
	}
	out := append([]ports.ActiveAlert(nil), alerts...)
	for i := range out {
		out[i].AlertSourceProfileID = p.profileID
	}
	return out, err
}

type sourceProfileMetricsProvider struct {
	ports.ActiveAlertProvider
	metricProvider ports.MetricQueryProvider
}

var _ ports.MetricsProvider = sourceProfileMetricsProvider{}

func (p sourceProfileMetricsProvider) QueryMetric(ctx context.Context, req ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	return p.metricProvider.QueryMetric(ctx, req)
}

func (p sourceProfileMetricsProvider) QueryMetricRange(ctx context.Context, req ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return p.metricProvider.QueryMetricRange(ctx, req)
}

// ResolveCredentials resolves server-side credentials for a stored alert
// source profile without returning raw resolver errors or credential values.
func ResolveCredentials(
	ctx context.Context,
	resolver ports.SecretResolver,
	profile domain.AlertSourceProfile,
) (Credentials, error) {
	if profile.AuthMode == domain.AlertSourceAuthModeNone {
		return Credentials{}, nil
	}
	if resolver == nil {
		return Credentials{}, ErrSecretResolverUnavailable
	}
	secret, err := resolver.ResolveSecret(ctx, profile.SecretRef)
	if err != nil {
		if errors.Is(err, ports.ErrSecretNotFound) {
			return Credentials{}, ErrSecretNotFound
		}
		return Credentials{}, ErrSecretResolveFailed
	}
	if secret.Value == "" || ContainsControlOrSpace(secret.Value) {
		return Credentials{}, ErrCredentialUnusable
	}
	return Credentials{BearerToken: secret.Value}, nil
}

// ContainsControlOrSpace reports whether value contains whitespace or control
// runes that make it unsafe as an opaque bearer token.
func ContainsControlOrSpace(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
