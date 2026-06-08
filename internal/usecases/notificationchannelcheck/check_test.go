package notificationchannelcheck

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelprovider"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestServiceDeliversSanitizedTestNotification(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	provider := &recordingIMProvider{
		delivery: ports.IMDelivery{
			ProviderMessageID: strings.Repeat("m", maxProviderMessageIDLength+8),
			Status:            strings.Repeat("accepted", 16),
		},
	}
	service := mustService(t, fakeSecretResolver{values: map[string]string{
		testNotificationChannelProfile().SecretRef: "https://example.invalid/hook",
	}}, provider, now)

	result, err := service.TestNotificationChannel(context.Background(), testNotificationChannelProfile())
	if err != nil {
		t.Fatalf("TestNotificationChannel: %v", err)
	}
	if provider.called != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.called)
	}
	if provider.req.NotificationChannelID != 7 ||
		provider.req.FinalReportID != 0 ||
		provider.req.DiagnosisTaskID != 0 ||
		provider.req.IdempotencyKey != "notification_channel:7/test" {
		t.Fatalf("test notification = %+v", provider.req)
	}
	if provider.req.Title == "" || provider.req.Body == "" || provider.req.Severity != "info" {
		t.Fatalf("test notification content = %+v", provider.req)
	}
	if result.Status != StatusSuccess ||
		result.ReasonCode != ReasonOK ||
		result.ChannelID != 7 ||
		result.Kind != domain.NotificationChannelKindWebhook ||
		!result.CheckedAt.Equal(now) {
		t.Fatalf("result = %+v", result)
	}
	if len(result.ProviderMessageID) != maxProviderMessageIDLength ||
		len(result.ProviderStatus) != maxProviderStatusLength {
		t.Fatalf("provider metadata was not bounded: %+v", result)
	}
}

func TestServiceAllowsDisabledProfileTests(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 5, 0, 0, time.UTC)
	profile := testNotificationChannelProfile()
	profile.Enabled = false
	provider := &recordingIMProvider{}
	service := mustService(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: "https://example.invalid/hook",
	}}, provider, now)

	result, err := service.TestNotificationChannel(context.Background(), profile)
	if err != nil {
		t.Fatalf("TestNotificationChannel: %v", err)
	}
	if result.Status != StatusSuccess || provider.called != 1 {
		t.Fatalf("result=%+v provider calls=%d, want success despite disabled profile", result, provider.called)
	}
}

func TestServiceMapsCredentialFailuresWithoutLeakingDetails(t *testing.T) {
	base := testNotificationChannelProfile()
	// #nosec G101 -- test-only placeholder URL proves sanitized messages do not leak raw secret values.
	rawSecretValue := "https://private.example.test/hook"
	rawSecretRef := base.SecretRef
	rawErr := "backend detail with " + rawSecretValue
	tests := []struct {
		name     string
		resolver ports.SecretResolver
		wantMsg  string
	}{
		{
			name:    "missing resolver",
			wantMsg: "Secret-backed notification channel tests require a server-side secret resolver.",
		},
		{
			name:     "missing secret",
			resolver: fakeSecretResolver{},
			wantMsg:  "Secret reference is not available to the server-side resolver.",
		},
		{
			name:     "resolver failure",
			resolver: fakeSecretResolver{err: errors.New(rawErr)},
			wantMsg:  "Secret reference could not be resolved by the server-side resolver.",
		},
		{
			name:     "unusable secret",
			resolver: fakeSecretResolver{values: map[string]string{rawSecretRef: "bad url"}},
			wantMsg:  "Secret reference resolved to an unusable notification credential.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := mustService(t, tc.resolver, &recordingIMProvider{}, time.Date(2026, 6, 5, 10, 10, 0, 0, time.UTC))

			result, err := service.TestNotificationChannel(context.Background(), base)
			if err != nil {
				t.Fatalf("TestNotificationChannel: %v", err)
			}
			if result.Status != StatusBlocked ||
				result.ReasonCode != ReasonCredentialsUnavailable ||
				result.Message != tc.wantMsg {
				t.Fatalf("result = %+v", result)
			}
			if strings.Contains(result.Message, rawSecretRef) ||
				strings.Contains(result.Message, rawSecretValue) ||
				strings.Contains(result.Message, rawErr) {
				t.Fatalf("message leaked secret material: %q", result.Message)
			}
		})
	}
}

func TestServiceMapsProviderFailuresWithoutRawProviderText(t *testing.T) {
	base := testNotificationChannelProfile()
	tests := []struct {
		name       string
		provider   *recordingIMProvider
		wantReason ReasonCode
		wantMsg    string
	}{
		{
			name:       "retryable",
			provider:   &recordingIMProvider{err: &ports.IMError{Message: "raw endpoint https://private.example.test failed", Retryable: true}},
			wantReason: ReasonProviderUnreachable,
			wantMsg:    "Notification channel test delivery reached a retryable provider failure.",
		},
		{
			name:       "nonretryable",
			provider:   &recordingIMProvider{err: &ports.IMError{Message: "raw provider rejected bearer token", Retryable: false}},
			wantReason: ReasonProviderError,
			wantMsg:    "Notification channel test delivery failed.",
		},
		{
			name:       "generic",
			provider:   &recordingIMProvider{err: errors.New("raw provider detail")},
			wantReason: ReasonProviderError,
			wantMsg:    "Notification channel test delivery failed.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := mustService(t, fakeSecretResolver{values: map[string]string{
				base.SecretRef: "https://example.invalid/hook",
			}}, tc.provider, time.Date(2026, 6, 5, 10, 15, 0, 0, time.UTC))

			result, err := service.TestNotificationChannel(context.Background(), base)
			if err != nil {
				t.Fatalf("TestNotificationChannel: %v", err)
			}
			if result.Status != StatusFailed || result.ReasonCode != tc.wantReason || result.Message != tc.wantMsg {
				t.Fatalf("result = %+v", result)
			}
			if strings.Contains(result.Message, "private.example") ||
				strings.Contains(result.Message, "bearer token") ||
				strings.Contains(result.Message, "raw provider detail") {
				t.Fatalf("message leaked provider detail: %q", result.Message)
			}
		})
	}
}

func TestServiceMapsTimeout(t *testing.T) {
	base := testNotificationChannelProfile()
	provider := &recordingIMProvider{waitForContext: true}
	service := mustService(t, fakeSecretResolver{values: map[string]string{
		base.SecretRef: "https://example.invalid/hook",
	}}, provider, time.Date(2026, 6, 5, 10, 20, 0, 0, time.UTC), WithTimeout(time.Nanosecond))

	result, err := service.TestNotificationChannel(context.Background(), base)
	if err != nil {
		t.Fatalf("TestNotificationChannel: %v", err)
	}
	if result.Status != StatusFailed ||
		result.ReasonCode != ReasonProviderUnreachable ||
		result.Message != "Notification channel test delivery timed out." {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceRejectsInvalidProfileAndBuilder(t *testing.T) {
	now := time.Date(2026, 6, 5, 10, 25, 0, 0, time.UTC)
	service := mustService(t, fakeSecretResolver{values: map[string]string{
		testNotificationChannelProfile().SecretRef: "https://example.invalid/hook",
	}}, &recordingIMProvider{}, now)

	invalid := testNotificationChannelProfile()
	invalid.ID = 0
	result, err := service.TestNotificationChannel(context.Background(), invalid)
	if err != nil {
		t.Fatalf("TestNotificationChannel invalid: %v", err)
	}
	if result.Status != StatusFailed || result.ReasonCode != ReasonInvalidProfile {
		t.Fatalf("invalid profile result = %+v", result)
	}

	builder, err := notificationchannelprovider.NewBuilder(func(domain.NotificationChannelProfile, notificationchannelprovider.WebhookCredentials) (ports.IMProvider, error) {
		return nil, errors.New("raw construction failure")
	}, notificationchannelprovider.WithSecretResolver(fakeSecretResolver{values: map[string]string{
		testNotificationChannelProfile().SecretRef: "https://example.invalid/hook",
	}}))
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	service, err = NewService(builder, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	result, err = service.TestNotificationChannel(context.Background(), testNotificationChannelProfile())
	if err != nil {
		t.Fatalf("TestNotificationChannel factory failure: %v", err)
	}
	if result.Status != StatusFailed ||
		result.ReasonCode != ReasonInvalidProfile ||
		result.Message != "Notification channel provider could not be constructed from the stored profile." ||
		strings.Contains(result.Message, "raw construction failure") {
		t.Fatalf("factory failure result = %+v", result)
	}
}

func TestTruncateKeepsUTF8Boundary(t *testing.T) {
	value := strings.Repeat("\u2713", maxProviderStatusLength+1)

	got := truncate(value, maxProviderStatusLength)

	if !utf8.ValidString(got) {
		t.Fatalf("truncate returned invalid UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) != maxProviderStatusLength {
		t.Fatalf("rune count = %d, want %d", utf8.RuneCountInString(got), maxProviderStatusLength)
	}
}

func mustService(
	t *testing.T,
	resolver ports.SecretResolver,
	provider ports.IMProvider,
	now time.Time,
	opts ...Option,
) *Service {
	t.Helper()
	builderOpts := []notificationchannelprovider.Option{}
	if resolver != nil {
		builderOpts = append(builderOpts, notificationchannelprovider.WithSecretResolver(resolver))
	}
	builder, err := notificationchannelprovider.NewBuilder(
		func(domain.NotificationChannelProfile, notificationchannelprovider.WebhookCredentials) (ports.IMProvider, error) {
			return provider, nil
		},
		builderOpts...,
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	serviceOpts := []Option{WithClock(func() time.Time { return now })}
	serviceOpts = append(serviceOpts, opts...)
	service, err := NewService(builder, serviceOpts...)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func testNotificationChannelProfile() domain.NotificationChannelProfile {
	return domain.NotificationChannelProfile{
		ID:             7,
		Name:           "Operations webhook",
		Kind:           domain.NotificationChannelKindWebhook,
		SecretRef:      "secret/openclarion/ops-webhook",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		Enabled:        true,
	}
}

type recordingIMProvider struct {
	called         int
	req            ports.IMNotification
	delivery       ports.IMDelivery
	err            error
	waitForContext bool
}

func (p *recordingIMProvider) SendNotification(ctx context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	p.called++
	p.req = req
	if p.waitForContext {
		<-ctx.Done()
		return ports.IMDelivery{}, ctx.Err()
	}
	if p.err != nil {
		return ports.IMDelivery{}, p.err
	}
	return p.delivery, nil
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
