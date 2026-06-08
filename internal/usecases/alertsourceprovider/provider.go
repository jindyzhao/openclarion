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

// MetricsProviderFactory builds a provider from a stored alert source profile
// plus backend-resolved credentials.
type MetricsProviderFactory func(domain.AlertSourceProfile, Credentials) (ports.MetricsProvider, error)

// Builder coordinates profile-kind routing and server-side credential
// resolution for runtime provider construction.
type Builder struct {
	prometheusFactory   MetricsProviderFactory
	alertmanagerFactory MetricsProviderFactory
	secretResolver      ports.SecretResolver
}

// Option customizes Builder construction.
type Option func(*Builder)

// WithAlertmanagerFactory enables Alertmanager provider construction.
func WithAlertmanagerFactory(factory MetricsProviderFactory) Option {
	return func(b *Builder) {
		if factory != nil {
			b.alertmanagerFactory = factory
		}
	}
}

// WithSecretResolver enables bearer-backed profile construction.
func WithSecretResolver(resolver ports.SecretResolver) Option {
	return func(b *Builder) {
		if resolver != nil {
			b.secretResolver = resolver
		}
	}
}

// NewBuilder constructs an alert source provider builder.
func NewBuilder(prometheusFactory MetricsProviderFactory, opts ...Option) (*Builder, error) {
	if prometheusFactory == nil {
		return nil, fmt.Errorf("alert source provider: prometheus factory is required: %w", domain.ErrInvariantViolation)
	}
	builder := &Builder{prometheusFactory: prometheusFactory}
	for _, opt := range opts {
		if opt != nil {
			opt(builder)
		}
	}
	return builder, nil
}

// Build resolves credentials and constructs a runtime provider for profile.
func (b *Builder) Build(ctx context.Context, profile domain.AlertSourceProfile) (ports.MetricsProvider, error) {
	if b == nil || b.prometheusFactory == nil {
		return nil, fmt.Errorf("alert source provider: builder is not configured: %w", domain.ErrInvariantViolation)
	}
	credentials, err := ResolveCredentials(ctx, b.secretResolver, profile)
	if err != nil {
		return nil, err
	}

	var factory MetricsProviderFactory
	switch profile.Kind {
	case domain.AlertSourceKindPrometheus:
		factory = b.prometheusFactory
	case domain.AlertSourceKindAlertmanager:
		factory = b.alertmanagerFactory
	default:
		return nil, ErrUnsupportedKind
	}
	if factory == nil {
		return nil, ErrUnsupportedKind
	}
	provider, err := factory(profile, credentials)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("alert source provider: factory returned nil provider: %w", domain.ErrInvariantViolation)
	}
	return provider, nil
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
