// Package notificationchannelprovider resolves persisted notification channel
// profiles into IM providers at backend runtime boundaries.
package notificationchannelprovider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	weComWebhookHost = "qyapi.weixin.qq.com"
	weComWebhookPath = "/cgi-bin/webhook/send"
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

// WebhookCredentials carries resolved webhook endpoint material. It
// intentionally stays outside persisted profile rows and OpenAPI responses.
type WebhookCredentials struct {
	URL    string
	Format string
}

// EmailCredentials carries resolved SMTP URL material. It intentionally stays
// outside persisted profile rows and OpenAPI responses.
type EmailCredentials struct {
	URL string
}

// WebhookFactory constructs a webhook-backed IMProvider from a stored profile
// plus resolved credentials.
type WebhookFactory func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error)

// EmailFactory constructs an SMTP email IMProvider from a stored profile plus
// resolved credentials.
type EmailFactory func(domain.NotificationChannelProfile, EmailCredentials) (ports.IMProvider, error)

// Builder maps a notification channel profile to a concrete IMProvider.
type Builder struct {
	webhookFactory WebhookFactory
	emailFactory   EmailFactory
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

// WithEmailFactory enables profile-backed email notification channels.
func WithEmailFactory(factory EmailFactory) Option {
	return func(b *Builder) {
		b.emailFactory = factory
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
	case domain.NotificationChannelKindWebhook, domain.NotificationChannelKindWeCom, domain.NotificationChannelKindDingTalk, domain.NotificationChannelKindFeishu, domain.NotificationChannelKindSlack:
		credentials, err := b.resolveWebhookCredentials(ctx, profile)
		if err != nil {
			return nil, err
		}
		provider, err := b.webhookFactory(profile, credentials)
		if err != nil {
			return nil, fmt.Errorf("notification channel provider: webhook provider could not be constructed from stored profile: %w", domain.ErrInvariantViolation)
		}
		if provider == nil {
			return nil, fmt.Errorf("notification channel provider: webhook factory returned nil provider: %w", domain.ErrInvariantViolation)
		}
		return provider, nil
	case domain.NotificationChannelKindEmail:
		if b.emailFactory == nil {
			return nil, ErrUnsupportedKind
		}
		credentials, err := b.resolveEmailCredentials(ctx, profile)
		if err != nil {
			return nil, err
		}
		provider, err := b.emailFactory(profile, credentials)
		if err != nil {
			return nil, fmt.Errorf("notification channel provider: email provider could not be constructed from stored profile: %w", domain.ErrInvariantViolation)
		}
		if provider == nil {
			return nil, fmt.Errorf("notification channel provider: email factory returned nil provider: %w", domain.ErrInvariantViolation)
		}
		return provider, nil
	default:
		return nil, ErrUnsupportedKind
	}
}

func (b *Builder) resolveWebhookCredentials(ctx context.Context, profile domain.NotificationChannelProfile) (WebhookCredentials, error) {
	if b.secretResolver == nil {
		return WebhookCredentials{}, ErrSecretResolverUnavailable
	}
	secret, err := b.secretResolver.ResolveSecret(ctx, profile.SecretRef)
	if err != nil {
		if errors.Is(err, ports.ErrSecretNotFound) {
			return WebhookCredentials{}, ErrSecretNotFound
		}
		return WebhookCredentials{}, ErrSecretResolveFailed
	}
	if secret.Value == "" || containsControlOrSpace(secret.Value) {
		return WebhookCredentials{}, ErrCredentialUnusable
	}
	if profile.Kind == domain.NotificationChannelKindWeCom && !validWeComWebhookEndpoint(secret.Value) {
		return WebhookCredentials{}, ErrCredentialUnusable
	}
	return WebhookCredentials{
		URL:    secret.Value,
		Format: notificationWebhookFormat(profile.Kind, secret.Value),
	}, nil
}

func (b *Builder) resolveEmailCredentials(ctx context.Context, profile domain.NotificationChannelProfile) (EmailCredentials, error) {
	if b.secretResolver == nil {
		return EmailCredentials{}, ErrSecretResolverUnavailable
	}
	secret, err := b.secretResolver.ResolveSecret(ctx, profile.SecretRef)
	if err != nil {
		if errors.Is(err, ports.ErrSecretNotFound) {
			return EmailCredentials{}, ErrSecretNotFound
		}
		return EmailCredentials{}, ErrSecretResolveFailed
	}
	if secret.Value == "" || containsControlOrSpace(secret.Value) {
		return EmailCredentials{}, ErrCredentialUnusable
	}
	return EmailCredentials{URL: secret.Value}, nil
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
	return r.resolveScopedNotificationProvider(ctx, channelProfileID, domain.NotificationDeliveryScopeReport, "report")
}

// ResolveDiagnosisConsultationNotificationProvider implements ports.NotificationChannelProviderResolver.
func (r *Resolver) ResolveDiagnosisConsultationNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return r.resolveScopedNotificationProvider(ctx, channelProfileID, domain.NotificationDeliveryScopeDiagnosisConsultation, "diagnosis consultation")
}

// ResolveDiagnosisCloseNotificationProvider implements ports.NotificationChannelProviderResolver.
func (r *Resolver) ResolveDiagnosisCloseNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return r.resolveScopedNotificationProvider(ctx, channelProfileID, domain.NotificationDeliveryScopeDiagnosisClose, "diagnosis close")
}

func (r *Resolver) resolveScopedNotificationProvider(
	ctx context.Context,
	channelProfileID domain.NotificationChannelProfileID,
	scope domain.NotificationDeliveryScope,
	deliveryName string,
) (ports.IMProvider, error) {
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
		return nil, fmt.Errorf("notification channel provider: channel profile must be enabled before %s notification delivery: %w", deliveryName, domain.ErrInvariantViolation)
	}
	if diagnosisDeliveryScope(scope) && profile.Kind != domain.NotificationChannelKindWeCom {
		return nil, fmt.Errorf("notification channel provider: channel profile must be an Enterprise WeChat channel before %s notification delivery: %w", deliveryName, domain.ErrInvariantViolation)
	}
	if !supportsDeliveryScope(profile, scope) {
		return nil, fmt.Errorf("notification channel provider: channel profile must include %s delivery scope: %w", scope, domain.ErrInvariantViolation)
	}
	provider, err := r.builder.Build(ctx, profile)
	if err != nil {
		return nil, mapBuildError(err, deliveryName)
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

func diagnosisDeliveryScope(scope domain.NotificationDeliveryScope) bool {
	switch scope {
	case domain.NotificationDeliveryScopeDiagnosisConsultation, domain.NotificationDeliveryScopeDiagnosisClose:
		return true
	default:
		return false
	}
}

func supportsDeliveryScope(profile domain.NotificationChannelProfile, want domain.NotificationDeliveryScope) bool {
	for _, scope := range profile.DeliveryScopes {
		if scope == want {
			return true
		}
	}
	return false
}

func mapBuildError(err error, deliveryName string) error {
	if err == nil || errors.Is(err, domain.ErrInvariantViolation) {
		return err
	}
	switch {
	case errors.Is(err, ErrUnsupportedKind):
		return fmt.Errorf("notification channel provider: channel profile kind is unsupported for %s delivery: %w", deliveryName, domain.ErrInvariantViolation)
	case errors.Is(err, ErrSecretResolverUnavailable):
		return fmt.Errorf("notification channel provider: secret resolver is required for profile-backed %s delivery: %w", deliveryName, domain.ErrInvariantViolation)
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

func inferredWebhookFormat(raw string) string {
	if validWeComWebhookEndpoint(raw) {
		return "wecom"
	}
	return ""
}

func notificationWebhookFormat(kind domain.NotificationChannelKind, raw string) string {
	switch kind {
	case domain.NotificationChannelKindWeCom:
		return "wecom"
	case domain.NotificationChannelKindDingTalk:
		return "dingtalk"
	case domain.NotificationChannelKindFeishu:
		return "feishu"
	case domain.NotificationChannelKindSlack:
		return "slack"
	default:
		return inferredWebhookFormat(raw)
	}
}

func validWeComWebhookEndpoint(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), weComWebhookHost) ||
		!strings.EqualFold(parsed.EscapedPath(), weComWebhookPath) {
		return false
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	keys, ok := values["key"]
	if !ok || len(values) != 1 || len(keys) != 1 {
		return false
	}
	key := keys[0]
	return key != "" && !containsControlOrSpace(key)
}
