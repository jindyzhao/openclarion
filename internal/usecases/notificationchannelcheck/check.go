// Package notificationchannelcheck owns sanitized delivery tests for configured
// notification channel profiles.
package notificationchannelcheck

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	ContentKind       string
	ContentSHA256     string
}

// Request selects optional controls for one notification channel test.
type Request struct {
	ContentKind string
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

// TestNotificationChannel performs one sanitized test send for profile.
// Disabled profiles may be tested so operators can verify draft channels before
// enablement; workflow delivery still requires an enabled report-scoped profile.
func (s *Service) TestNotificationChannel(ctx context.Context, profile domain.NotificationChannelProfile, requests ...Request) (Result, error) {
	if s == nil || s.builder == nil || s.clock == nil {
		return Result{}, fmt.Errorf("notification channel check: service is not configured: %w", domain.ErrInvariantViolation)
	}
	if len(requests) > 1 {
		return Result{}, fmt.Errorf("notification channel check: at most one request is supported: %w", domain.ErrInvariantViolation)
	}
	req := Request{}
	if len(requests) == 1 {
		req = requests[0]
	}
	contentKind := strings.TrimSpace(req.ContentKind)
	if contentKind != "" && !validTestContentKind(contentKind) {
		return Result{}, fmt.Errorf("notification channel check: content_kind is unsupported: %w", domain.ErrInvariantViolation)
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
	if profile.Kind != domain.NotificationChannelKindWeCom && profileHasDiagnosisDeliveryScope(profile) {
		result.Message = "Diagnosis notification channel tests require an Enterprise WeChat profile."
		return result, nil
	}

	provider, err := s.builder.Build(ctx, profile)
	if err != nil {
		return resultFromBuildError(result, err), nil
	}
	return s.testProvider(ctx, profile, result, provider, contentKind), nil
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
	contentKind string,
) Result {
	if provider == nil {
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = "Notification channel provider is not configured."
		return result
	}

	checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	testMessage, err := testNotification(profile, contentKind)
	if err != nil {
		result.Status = StatusFailed
		result.ReasonCode = ReasonInvalidProfile
		result.Message = "Requested notification channel test content is not compatible with the profile delivery scopes."
		return result
	}
	result.ContentKind = testMessage.ContentKind
	result.ContentSHA256 = testMessage.ContentSHA256
	delivery, err := provider.SendNotification(checkCtx, testMessage.Notification)
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

type testNotificationMessage struct {
	Notification  ports.IMNotification
	ContentKind   string
	ContentSHA256 string
}

func testNotification(profile domain.NotificationChannelProfile, contentKind string) (testNotificationMessage, error) {
	title, body, contentKind, err := testNotificationContent(profile, contentKind)
	if err != nil {
		return testNotificationMessage{}, err
	}
	return testNotificationMessage{
		Notification: ports.IMNotification{
			IdempotencyKey:        fmt.Sprintf("notification_channel:%d/test", profile.ID),
			NotificationChannelID: int64(profile.ID),
			CorrelationKey:        "notification-channel-test",
			Title:                 title,
			Body:                  body,
			Severity:              "info",
		},
		ContentKind:   contentKind,
		ContentSHA256: sha256Hex(body),
	}, nil
}

func testNotificationContent(profile domain.NotificationChannelProfile, contentKind string) (string, string, string, error) {
	if contentKind == "" {
		switch {
		case hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisConsultation):
			contentKind = "ai_diagnosis_sample"
		case hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisClose):
			contentKind = "diagnosis_close_sample"
		default:
			contentKind = "transport_sample"
		}
	}

	switch contentKind {
	case "ai_diagnosis_sample":
		if !hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisConsultation) {
			return "", "", "", fmt.Errorf("ai diagnosis sample requires diagnosis_consultation delivery scope: %w", domain.ErrInvariantViolation)
		}
		return "OpenClarion AI diagnosis channel test", strings.Join([]string{
			"This test validates that the channel can receive OpenClarion AI diagnosis updates, not raw Alertmanager alerts.",
			"Confidence: medium",
			"Human review: required",
			"Missing evidence:",
			"1. [high] Owner rollout context - Confirm whether the service owner has already mitigated the rollout risk.",
			"Evidence collection suggestions:",
			"1. [medium] Current saturation trend - Collect a bounded CPU or JVM memory range query before raising confidence.",
			"AI diagnosis: Synthetic CPU saturation is likely related to a recent rollout.",
			"Recommended actions:",
			"1. Review the diagnosis room before closing the alert.",
			"2. Provide supplemental evidence if confidence needs improvement.",
			"Executable evidence requests: 1",
			"1. active_alerts - Confirm related alerts are still firing. (limit=5)",
		}, "\n"), "ai_diagnosis_sample", nil
	case "diagnosis_close_sample":
		if !hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisClose) {
			return "", "", "", fmt.Errorf("diagnosis close sample requires diagnosis_close delivery scope: %w", domain.ErrInvariantViolation)
		}
		return "OpenClarion diagnosis close channel test", strings.Join([]string{
			"This test validates that the channel can receive OpenClarion diagnosis room close notifications.",
			"Confidence: medium",
			"AI conclusion: Synthetic alert impact has been reviewed and is ready for operator closure.",
			"Recommended actions:",
			"1. Confirm the close reason is recorded before archiving the room.",
		}, "\n"), "diagnosis_close_sample", nil
	case "transport_sample":
		return "OpenClarion notification channel test", "This is a test notification from OpenClarion.", "transport_sample", nil
	default:
		return "", "", "", fmt.Errorf("unsupported notification channel test content kind: %w", domain.ErrInvariantViolation)
	}
}

func validTestContentKind(value string) bool {
	switch value {
	case "transport_sample", "ai_diagnosis_sample", "diagnosis_close_sample":
		return true
	default:
		return false
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hasDeliveryScope(profile domain.NotificationChannelProfile, want domain.NotificationDeliveryScope) bool {
	for _, scope := range profile.DeliveryScopes {
		if scope == want {
			return true
		}
	}
	return false
}

func profileHasDiagnosisDeliveryScope(profile domain.NotificationChannelProfile) bool {
	return hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisConsultation) ||
		hasDeliveryScope(profile, domain.NotificationDeliveryScopeDiagnosisClose)
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
