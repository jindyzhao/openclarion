// Package alertsourcecheck owns sanitized connectivity checks for configured
// alert source profiles.
package alertsourcecheck

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// DefaultTimeout bounds one alert source connection-test provider call.
const DefaultTimeout = 5 * time.Second

// Status is the coarse sanitized outcome category returned to operators.
type Status string

// Connection-test status values.
const (
	StatusSuccess     Status = "success"
	StatusFailed      Status = "failed"
	StatusUnsupported Status = "unsupported"
	StatusBlocked     Status = "blocked"
)

// ReasonCode is the stable machine-readable reason for a connection-test result.
type ReasonCode string

// Connection-test reason codes.
const (
	ReasonOK                     ReasonCode = "ok"
	ReasonUnsupportedKind        ReasonCode = "unsupported_kind"
	ReasonCredentialsUnavailable ReasonCode = "credentials_unavailable"
	ReasonUpstreamUnreachable    ReasonCode = "upstream_unreachable"
	ReasonUpstreamError          ReasonCode = "upstream_error"
	ReasonInvalidProfile         ReasonCode = "invalid_profile"
)

// ProviderCredentials contains resolved credentials for one provider call.
type ProviderCredentials = alertsourceprovider.Credentials

// MetricsProviderFactory builds a provider from a stored alert source profile
// plus backend-resolved credentials. Implementations must not return providers
// that expose credential values in error text returned to this package.
type MetricsProviderFactory = alertsourceprovider.MetricsProviderFactory

// Clock supplies the check timestamp. It is injected so usecase code never
// reads wall-clock time directly.
type Clock func() time.Time

// Result is the sanitized output of one alert source connection test.
type Result struct {
	SourceID       domain.AlertSourceProfileID
	Kind           domain.AlertSourceKind
	AuthMode       domain.AlertSourceAuthMode
	Status         Status
	ReasonCode     ReasonCode
	Message        string
	CheckedAt      time.Time
	ObservedAlerts int
}

// Service coordinates provider construction and sanitized connectivity checks.
type Service struct {
	prometheusFactory   MetricsProviderFactory
	alertmanagerFactory MetricsProviderFactory
	secretResolver      ports.SecretResolver
	clock               Clock
	timeout             time.Duration
}

// Option customizes Service construction.
type Option func(*Service)

// WithClock injects the clock used to stamp connection-test results.
func WithClock(clock Clock) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithTimeout overrides the provider call timeout for connection tests.
func WithTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// WithAlertmanagerFactory enables Alertmanager connection tests.
func WithAlertmanagerFactory(factory MetricsProviderFactory) Option {
	return func(s *Service) {
		if factory != nil {
			s.alertmanagerFactory = factory
		}
	}
}

// WithSecretResolver enables bearer-backed connection tests.
func WithSecretResolver(resolver ports.SecretResolver) Option {
	return func(s *Service) {
		if resolver != nil {
			s.secretResolver = resolver
		}
	}
}

// NewService builds an alert source connection-test service.
func NewService(prometheusFactory MetricsProviderFactory, opts ...Option) (*Service, error) {
	if prometheusFactory == nil {
		return nil, fmt.Errorf("alert source check: prometheus factory is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		prometheusFactory: prometheusFactory,
		timeout:           DefaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.clock == nil {
		return nil, fmt.Errorf("alert source check: clock is required: %w", domain.ErrInvariantViolation)
	}
	return service, nil
}

// TestAlertSourceConnection performs one sanitized connection test for profile.
func (s *Service) TestAlertSourceConnection(ctx context.Context, profile domain.AlertSourceProfile) (Result, error) {
	if s == nil || s.prometheusFactory == nil || s.clock == nil {
		return Result{}, fmt.Errorf("alert source check: service is not configured: %w", domain.ErrInvariantViolation)
	}
	result := Result{
		SourceID:       profile.ID,
		Kind:           profile.Kind,
		AuthMode:       profile.AuthMode,
		Status:         StatusFailed,
		ReasonCode:     ReasonInvalidProfile,
		Message:        "Stored alert source profile is invalid.",
		CheckedAt:      s.clock().UTC(),
		ObservedAlerts: 0,
	}
	if profile.ID <= 0 || !profile.Kind.Valid() || !profile.AuthMode.Valid() {
		return result, nil
	}
	switch profile.Kind {
	case domain.AlertSourceKindPrometheus:
		credentials, credentialResult, ok := s.resolveCredentials(ctx, profile, result)
		if !ok {
			return credentialResult, nil
		}
		return s.testProvider(ctx, profile, result, s.prometheusFactory, credentials, "Prometheus"), nil
	case domain.AlertSourceKindAlertmanager:
		if s.alertmanagerFactory == nil {
			result.Status = StatusUnsupported
			result.ReasonCode = ReasonUnsupportedKind
			result.Message = "Alertmanager connection tests require the Alertmanager adapter."
			return result, nil
		}
		credentials, credentialResult, ok := s.resolveCredentials(ctx, profile, result)
		if !ok {
			return credentialResult, nil
		}
		return s.testProvider(ctx, profile, result, s.alertmanagerFactory, credentials, "Alertmanager"), nil
	default:
		result.Status = StatusUnsupported
		result.ReasonCode = ReasonUnsupportedKind
		result.Message = "Alert source kind is not supported by connection tests."
		return result, nil
	}
}

func (s *Service) resolveCredentials(
	ctx context.Context,
	profile domain.AlertSourceProfile,
	result Result,
) (ProviderCredentials, Result, bool) {
	credentials, err := alertsourceprovider.ResolveCredentials(ctx, s.secretResolver, profile)
	if err == nil {
		return credentials, result, true
	}
	result.Status = StatusBlocked
	result.ReasonCode = ReasonCredentialsUnavailable
	switch {
	case errors.Is(err, alertsourceprovider.ErrSecretResolverUnavailable):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret-backed connection tests require a server-side secret resolver."
	case errors.Is(err, alertsourceprovider.ErrSecretNotFound):
		result.Message = "Secret reference is not available to the server-side resolver."
	case errors.Is(err, alertsourceprovider.ErrSecretResolveFailed):
		result.Message = "Secret reference could not be resolved by the server-side resolver."
	case errors.Is(err, alertsourceprovider.ErrCredentialUnusable):
		result.Message = "Secret reference resolved to an unusable credential."
	default:
		result.Message = "Secret reference could not be resolved by the server-side resolver."
	}
	return ProviderCredentials{}, result, false
}

func (s *Service) testProvider(
	ctx context.Context,
	profile domain.AlertSourceProfile,
	result Result,
	factory MetricsProviderFactory,
	credentials ProviderCredentials,
	displayName string,
) Result {
	provider, err := factory(profile, credentials)
	if err != nil {
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = displayName + " provider could not be constructed from the stored profile."
		return result
	}
	if provider == nil {
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = displayName + " provider is not configured."
		return result
	}

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	alerts, err := provider.ListActiveAlerts(checkCtx)
	if err != nil {
		result.Status = StatusFailed
		if checkCtx.Err() != nil {
			result.ReasonCode = ReasonUpstreamUnreachable
			result.Message = displayName + " alert listing timed out."
			return result
		}
		result.ReasonCode = ReasonUpstreamError
		result.Message = displayName + " alert listing failed."
		return result
	}

	result.Status = StatusSuccess
	result.ReasonCode = ReasonOK
	result.Message = displayName + " alert listing succeeded."
	result.ObservedAlerts = len(alerts)
	return result
}
