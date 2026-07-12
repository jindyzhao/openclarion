// Package alertsourcecheck owns sanitized connectivity checks for configured
// alert source profiles.
package alertsourcecheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// DefaultTimeout bounds one alert source connection-test provider call.
const DefaultTimeout = 5 * time.Second

const prometheusMetricProbeQuery = "vector(1)"

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
	ReasonCapabilityUnavailable  ReasonCode = "capability_unavailable"
	ReasonCredentialsUnavailable ReasonCode = "credentials_unavailable"
	ReasonUpstreamUnreachable    ReasonCode = "upstream_unreachable"
	ReasonUpstreamError          ReasonCode = "upstream_error"
	ReasonInvalidProfile         ReasonCode = "invalid_profile"
)

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
	providers *alertsourceprovider.Builder
	clock     Clock
	timeout   time.Duration
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

// NewService builds an alert source connection-test service.
func NewService(providers *alertsourceprovider.Builder, opts ...Option) (*Service, error) {
	if providers == nil {
		return nil, fmt.Errorf("alert source check: provider builder is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		providers: providers,
		timeout:   DefaultTimeout,
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
	if s == nil || s.providers == nil || s.clock == nil {
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
	provider, err := s.providers.Build(ctx, profile)
	if err != nil {
		return providerBuildFailure(result, err), nil
	}
	displayName := alertSourceDisplayName(profile)
	result = s.testActiveAlertListing(ctx, result, provider, displayName)
	if result.Status != StatusSuccess {
		return result, nil
	}
	if !prometheusMetricProbeRequired(profile) {
		result.Message = displayName + " alert listing succeeded."
		return result, nil
	}
	metricProvider, ok := provider.(ports.MetricQueryProvider)
	if !ok {
		result.Status = StatusUnsupported
		result.ReasonCode = ReasonCapabilityUnavailable
		result.Message = displayName + " adapter does not provide metric query capability."
		return result, nil
	}
	return s.testMetricQuery(ctx, result, metricProvider, displayName), nil
}

func providerBuildFailure(result Result, err error) Result {
	switch {
	case errors.Is(err, alertsourceprovider.ErrUnsupportedKind):
		result.Status = StatusUnsupported
		result.ReasonCode = ReasonUnsupportedKind
		result.Message = "Alert source kind is not supported by the configured adapters."
	case errors.Is(err, alertsourceprovider.ErrSecretResolverUnavailable):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret-backed connection tests require a server-side secret resolver."
	case errors.Is(err, alertsourceprovider.ErrSecretNotFound):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference is not available to the server-side resolver."
	case errors.Is(err, alertsourceprovider.ErrSecretResolveFailed):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference could not be resolved by the server-side resolver."
	case errors.Is(err, alertsourceprovider.ErrCredentialUnusable):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference resolved to an unusable credential."
	default:
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = "Alert source provider could not be constructed from the stored profile."
	}
	return result
}

func (s *Service) testMetricQuery(
	ctx context.Context,
	result Result,
	provider ports.MetricQueryProvider,
	displayName string,
) Result {
	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	_, err := provider.QueryMetric(checkCtx, ports.MetricQueryRequest{
		Query:   prometheusMetricProbeQuery,
		Time:    result.CheckedAt,
		Timeout: s.timeout,
		Limit:   1,
	})
	if err != nil {
		result.Status = StatusFailed
		if checkCtx.Err() != nil {
			result.ReasonCode = ReasonUpstreamUnreachable
			result.Message = displayName + " metric query timed out."
			return result
		}
		result.ReasonCode = ReasonUpstreamError
		result.Message = displayName + " metric query failed."
		return result
	}
	result.Message = displayName + " alert listing and metric query succeeded."
	return result
}

func prometheusMetricProbeRequired(profile domain.AlertSourceProfile) bool {
	return profile.Kind == domain.AlertSourceKindPrometheus &&
		!prometheusProfileSourceLabelIs(profile, "thanos-rule")
}

func alertSourceDisplayName(profile domain.AlertSourceProfile) string {
	if prometheusProfileSourceLabelIs(profile, "thanos-rule") {
		return "Thanos Rule"
	}
	switch profile.Kind {
	case domain.AlertSourceKindPrometheus:
		return "Prometheus"
	case domain.AlertSourceKindAlertmanager:
		return "Alertmanager"
	default:
		return "Alert source"
	}
}

func prometheusProfileSourceLabelIs(profile domain.AlertSourceProfile, want string) bool {
	if profile.Labels == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(profile.Labels["source"]), want)
}

func (s *Service) testActiveAlertListing(
	ctx context.Context,
	result Result,
	provider ports.ActiveAlertProvider,
	displayName string,
) Result {
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
