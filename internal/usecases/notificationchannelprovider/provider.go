// Package notificationchannelprovider resolves persisted notification channel
// profiles into IM providers at backend runtime boundaries.
package notificationchannelprovider

import (
	"context"
	"errors"
	"fmt"
	"unicode"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

var (
	// ErrUnsupportedKind is returned when no provider factory exists for the
	// persisted notification channel kind.
	ErrUnsupportedKind = errors.New("notification channel kind is unsupported")
	// ErrSecretResolverUnavailable is returned when a profile requires a
	// secret_ref but no resolver was configured at the backend boundary.
	ErrSecretResolverUnavailable = errors.New("notification channel secret resolver is unavailable")
	// ErrSecretNotFound is returned when the configured resolver cannot resolve
	// the profile secret_ref.
	ErrSecretNotFound = errors.New("notification channel secret reference was not found")
	// ErrSecretResolveFailed is returned for unexpected resolver failures.
	ErrSecretResolveFailed = errors.New("notification channel secret reference could not be resolved")
	// ErrCredentialUnusable is returned when a resolved secret is empty or
	// malformed for the selected provider factory.
	ErrCredentialUnusable = errors.New("notification channel resolved credential is unusable")
)

// WebhookCredentials carries the resolved webhook endpoint URL. It intentionally
// stays outside persisted profile rows and OpenAPI responses.
type WebhookCredentials struct {
	URL string
}

// WebhookFactory constructs a webhook-backed IMProvider from a stored profile
// plus resolved credentials.
type WebhookFactory func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error)

// Builder maps a notification channel profile to a concrete IMProvider.
type Builder struct {
	webhookFactory WebhookFactory
	secretResolver ports.SecretResolver
}

// Option customizes Builder behavior.
type Option func(*Builder)

// WithSecretResolver enables secret_ref resolution for notification channels.
func WithSecretResolver(resolver ports.SecretResolver) Option {
	return func(b *Builder) {
		b.secretResolver = resolver
	}
}

// NewBuilder constructs a Builder.
func NewBuilder(webhookFactory WebhookFactory, opts ...Option) (*Builder, error) {
	if webhookFactory == nil {
		return nil, fmt.Errorf("notification channel provider: webhook factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	builder := &Builder{webhookFactory: webhookFactory}
	for _, opt := range opts {
		if opt != nil {
			opt(builder)
		}
	}
	return builder, nil
}

// Build resolves a provider for the supplied profile. The profile must already
// be selected and authorization-checked by the caller.
func (b *Builder) Build(ctx context.Context, profile domain.NotificationChannelProfile) (ports.IMProvider, error) {
	if b == nil {
		return nil, fmt.Errorf("notification channel provider: builder must be non-nil: %w", domain.ErrInvariantViolation)
	}
	switch profile.Kind {
	case domain.NotificationChannelKindWebhook:
		url, err := b.resolveWebhookURL(ctx, profile.SecretRef)
		if err != nil {
			return nil, err
		}
		provider, err := b.webhookFactory(profile, WebhookCredentials{URL: url})
		if err != nil {
			return nil, fmt.Errorf("notification channel provider: webhook provider could not be constructed from stored profile: %w", domain.ErrInvariantViolation)
		}
		if provider == nil {
			return nil, fmt.Errorf("notification channel provider: webhook factory returned nil provider: %w", domain.ErrInvariantViolation)
		}
		return provider, nil
	default:
		return nil, ErrUnsupportedKind
	}
}

func (b *Builder) resolveWebhookURL(ctx context.Context, secretRef string) (string, error) {
	if b.secretResolver == nil {
		return "", ErrSecretResolverUnavailable
	}
	secret, err := b.secretResolver.ResolveSecret(ctx, secretRef)
	if err != nil {
		if errors.Is(err, ports.ErrSecretNotFound) {
			return "", ErrSecretNotFound
		}
		return "", ErrSecretResolveFailed
	}
	if secret.Value == "" || containsControlOrSpace(secret.Value) {
		return "", ErrCredentialUnusable
	}
	return secret.Value, nil
}

// Resolver loads persisted notification channel profiles and resolves them into
// providers. It is intended for Activity/runtime code, not workflow code.
type Resolver struct {
	uowFactory ports.UnitOfWorkFactory
	builder    *Builder
}

// NewResolver constructs a profile-backed provider resolver.
func NewResolver(uowFactory ports.UnitOfWorkFactory, builder *Builder) (*Resolver, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("notification channel provider: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if builder == nil {
		return nil, fmt.Errorf("notification channel provider: builder must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return &Resolver{uowFactory: uowFactory, builder: builder}, nil
}

// ResolveReportNotificationProvider implements ports.NotificationChannelProviderResolver.
func (r *Resolver) ResolveReportNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	if r == nil {
		return nil, fmt.Errorf("notification channel provider: resolver must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if channelProfileID <= 0 {
		return nil, fmt.Errorf("notification channel provider: channel profile id must be positive: %w", domain.ErrInvariantViolation)
	}

	profile, err := r.loadProfile(ctx, channelProfileID)
	if err != nil {
		return nil, err
	}
	if !profile.Enabled {
		return nil, fmt.Errorf("notification channel provider: channel profile must be enabled before report notification delivery: %w", domain.ErrInvariantViolation)
	}
	if !supportsReport(profile) {
		return nil, fmt.Errorf("notification channel provider: channel profile must include report delivery scope: %w", domain.ErrInvariantViolation)
	}
	provider, err := r.builder.Build(ctx, profile)
	if err != nil {
		return nil, mapBuildError(err)
	}
	return provider, nil
}

func (r *Resolver) loadProfile(ctx context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	var profile domain.NotificationChannelProfile
	err := r.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Config().FindNotificationChannelProfileByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("notification channel provider: channel profile not found: %w", domain.ErrInvariantViolation)
			}
			return err
		}
		profile = got
		return nil
	})
	return profile, err
}

func supportsReport(profile domain.NotificationChannelProfile) bool {
	for _, scope := range profile.DeliveryScopes {
		if scope == domain.NotificationDeliveryScopeReport {
			return true
		}
	}
	return false
}

func mapBuildError(err error) error {
	if err == nil || errors.Is(err, domain.ErrInvariantViolation) {
		return err
	}
	switch {
	case errors.Is(err, ErrUnsupportedKind):
		return fmt.Errorf("notification channel provider: channel profile kind is unsupported for report delivery: %w", domain.ErrInvariantViolation)
	case errors.Is(err, ErrSecretResolverUnavailable):
		return fmt.Errorf("notification channel provider: secret resolver is required for profile-backed report delivery: %w", domain.ErrInvariantViolation)
	case errors.Is(err, ErrSecretNotFound):
		return fmt.Errorf("notification channel provider: channel profile secret reference is unavailable: %w", domain.ErrInvariantViolation)
	case errors.Is(err, ErrSecretResolveFailed):
		return fmt.Errorf("notification channel provider: channel profile secret reference could not be resolved: %w", domain.ErrInvariantViolation)
	case errors.Is(err, ErrCredentialUnusable):
		return fmt.Errorf("notification channel provider: resolved channel profile credential is unusable: %w", domain.ErrInvariantViolation)
	default:
		return err
	}
}

func containsControlOrSpace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
