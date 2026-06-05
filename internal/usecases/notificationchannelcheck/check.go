// Package notificationchannelcheck owns sanitized delivery tests for configured
// notification channel profiles.
package notificationchannelcheck

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// DefaultTimeout bounds one notification channel test send.
const DefaultTimeout = 5 * time.Second

const (
	maxProviderMessageIDLength = 128
	maxProviderStatusLength    = 64
)

// Status is the coarse sanitized outcome category returned to operators.
type Status string

// Notification channel test status values.
const (
	StatusSuccess     Status = "success"
	StatusFailed      Status = "failed"
	StatusUnsupported Status = "unsupported"
	StatusBlocked     Status = "blocked"
)

// ReasonCode is the stable machine-readable reason for a channel test result.
type ReasonCode string

// Notification channel test reason codes.
const (
	ReasonOK                     ReasonCode = "ok"
	ReasonUnsupportedKind        ReasonCode = "unsupported_kind"
	ReasonCredentialsUnavailable ReasonCode = "credentials_unavailable"
	ReasonProviderUnreachable    ReasonCode = "provider_unreachable"
	ReasonProviderError          ReasonCode = "provider_error"
	ReasonInvalidProfile         ReasonCode = "invalid_profile"
)

// Clock supplies the check timestamp. It is injected so usecase code never
// reads wall-clock time directly.
type Clock func() time.Time

// Result is the sanitized output of one notification channel test.
type Result struct {
	ChannelID         domain.NotificationChannelProfileID
	Kind              domain.NotificationChannelKind
	Status            Status
	ReasonCode        ReasonCode
	Message           string
	CheckedAt         time.Time
	ProviderMessageID string
	ProviderStatus    string
}

// Service coordinates provider construction and sanitized test delivery.
type Service struct {
	builder *notificationchannelprovider.Builder
	clock   Clock
	timeout time.Duration
}

// Option customizes Service construction.
type Option func(*Service)

// WithClock injects the clock used to stamp test results.
func WithClock(clock Clock) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

// WithTimeout overrides the provider call timeout for channel tests.
func WithTimeout(timeout time.Duration) Option {
	return func(s *Service) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// NewService builds a notification channel test service.
func NewService(builder *notificationchannelprovider.Builder, opts ...Option) (*Service, error) {
	if builder == nil {
		return nil, fmt.Errorf("notification channel check: provider builder is required: %w", domain.ErrInvariantViolation)
	}
	service := &Service{
		builder: builder,
		timeout: DefaultTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.clock == nil {
		return nil, fmt.Errorf("notification channel check: clock is required: %w", domain.ErrInvariantViolation)
	}
	return service, nil
}

// TestNotificationChannel performs one sanitized test send for profile. Disabled
// profiles may be tested so operators can verify draft channels before
// enablement; workflow delivery still requires an enabled report-scoped profile.
func (s *Service) TestNotificationChannel(ctx context.Context, profile domain.NotificationChannelProfile) (Result, error) {
	if s == nil || s.builder == nil || s.clock == nil {
		return Result{}, fmt.Errorf("notification channel check: service is not configured: %w", domain.ErrInvariantViolation)
	}
	result := Result{
		ChannelID:  profile.ID,
		Kind:       profile.Kind,
		Status:     StatusFailed,
		ReasonCode: ReasonInvalidProfile,
		Message:    "Stored notification channel profile is invalid.",
		CheckedAt:  s.clock().UTC(),
	}
	if profile.ID <= 0 || !profile.Kind.Valid() {
		return result, nil
	}

	provider, err := s.builder.Build(ctx, profile)
	if err != nil {
		return resultFromBuildError(result, err), nil
	}
	return s.testProvider(ctx, profile, result, provider), nil
}

func resultFromBuildError(result Result, err error) Result {
	switch {
	case errors.Is(err, notificationchannelprovider.ErrUnsupportedKind):
		result.Status = StatusUnsupported
		result.ReasonCode = ReasonUnsupportedKind
		result.Message = "Notification channel kind is not supported by tests."
	case errors.Is(err, notificationchannelprovider.ErrSecretResolverUnavailable):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret-backed notification channel tests require a server-side secret resolver."
	case errors.Is(err, notificationchannelprovider.ErrSecretNotFound):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference is not available to the server-side resolver."
	case errors.Is(err, notificationchannelprovider.ErrSecretResolveFailed):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference could not be resolved by the server-side resolver."
	case errors.Is(err, notificationchannelprovider.ErrCredentialUnusable):
		result.Status = StatusBlocked
		result.ReasonCode = ReasonCredentialsUnavailable
		result.Message = "Secret reference resolved to an unusable notification credential."
	default:
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = "Notification channel provider could not be constructed from the stored profile."
	}
	return result
}

func (s *Service) testProvider(
	ctx context.Context,
	profile domain.NotificationChannelProfile,
	result Result,
	provider ports.IMProvider,
) Result {
	if provider == nil {
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = "Notification channel provider is not configured."
		return result
	}

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	delivery, err := provider.SendNotification(checkCtx, testNotification(profile.ID))
	if err != nil {
		result.Status = StatusFailed
		if checkCtx.Err() != nil {
			result.ReasonCode = ReasonProviderUnreachable
			result.Message = "Notification channel test delivery timed out."
			return result
		}
		var imErr *ports.IMError
		if errors.As(err, &imErr) && imErr.Retryable {
			result.ReasonCode = ReasonProviderUnreachable
			result.Message = "Notification channel test delivery reached a retryable provider failure."
			return result
		}
		result.ReasonCode = ReasonProviderError
		result.Message = "Notification channel test delivery failed."
		return result
	}

	result.Status = StatusSuccess
	result.ReasonCode = ReasonOK
	result.Message = "Notification channel test delivery succeeded."
	result.ProviderMessageID = truncate(delivery.ProviderMessageID, maxProviderMessageIDLength)
	result.ProviderStatus = truncate(delivery.Status, maxProviderStatusLength)
	return result
}

func testNotification(channelID domain.NotificationChannelProfileID) ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey:        fmt.Sprintf("notification_channel:%d/test", channelID),
		NotificationChannelID: int64(channelID),
		CorrelationKey:        "notification-channel-test",
		Title:                 "OpenClarion notification channel test",
		Body:                  "This is a test notification from OpenClarion.",
		Severity:              "info",
	}
}

func truncate(value string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	if len(value) <= maxLength {
		return value
	}
	count := 0
	for index := range value {
		if count == maxLength {
			return value[:index]
		}
		count++
	}
	return value
}
