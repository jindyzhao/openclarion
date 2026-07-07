package notificationchannelprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestResolverResolveReportNotificationProvider(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		ID:             3,
		Name:           "Report webhook",
		Kind:           domain.NotificationChannelKindWebhook,
		SecretRef:      "secret/openclarion/report-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		Enabled:        true,
	}
	repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
		3: profile,
	}}
	var gotProfile domain.NotificationChannelProfile
	var gotCredentials WebhookCredentials
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: "https://example.invalid/report-hook",
	}}, func(profile domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
		gotProfile = profile
		gotCredentials = credentials
		return fakeIMProvider{}, nil
	})
	resolver := mustResolver(t, repo, builder)

	provider, err := resolver.ResolveReportNotificationProvider(context.Background(), 3)
	if err != nil {
		t.Fatalf("ResolveReportNotificationProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotProfile.ID != 3 || gotCredentials.URL != "https://example.invalid/report-hook" {
		t.Fatalf("factory input profile=%+v credentials=%+v", gotProfile, gotCredentials)
	}
	if gotCredentials.Format != "" {
		t.Fatalf("credentials.Format = %q, want empty generic default", gotCredentials.Format)
	}
}

func TestBuilderInfersWeComWebhookFormatFromResolvedURL(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindWebhook,
		SecretRef: "secret/openclarion/wecom-webhook-url",
	}
	var gotCredentials WebhookCredentials
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: fixtureWeComWebhookURL(),
	}}, func(_ domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
		gotCredentials = credentials
		return fakeIMProvider{}, nil
	})

	provider, err := builder.Build(context.Background(), profile)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotCredentials.Format != "wecom" {
		t.Fatalf("credentials.Format = %q, want wecom", gotCredentials.Format)
	}
	if gotCredentials.URL != fixtureWeComWebhookURL() {
		t.Fatalf("credentials.URL = %q", gotCredentials.URL)
	}
}

func TestBuilderUsesWeComFormatForWeComChannelKind(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindWeCom,
		SecretRef: "secret/openclarion/wecom-webhook-url",
	}
	var gotProfile domain.NotificationChannelProfile
	var gotCredentials WebhookCredentials
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: fixtureWeComWebhookURL(),
	}}, func(profile domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
		gotProfile = profile
		gotCredentials = credentials
		return fakeIMProvider{}, nil
	})

	provider, err := builder.Build(context.Background(), profile)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotProfile.Kind != domain.NotificationChannelKindWeCom {
		t.Fatalf("profile.Kind = %q, want wecom", gotProfile.Kind)
	}
	if gotCredentials.Format != "wecom" {
		t.Fatalf("credentials.Format = %q, want wecom", gotCredentials.Format)
	}
	if gotCredentials.URL != fixtureWeComWebhookURL() {
		t.Fatalf("credentials.URL = %q", gotCredentials.URL)
	}
}

func TestBuilderUsesExplicitRobotFormatForChannelKind(t *testing.T) {
	tests := []struct {
		name string
		kind domain.NotificationChannelKind
		want string
	}{
		{name: "dingtalk", kind: domain.NotificationChannelKindDingTalk, want: "dingtalk"},
		{name: "feishu", kind: domain.NotificationChannelKindFeishu, want: "feishu"},
		{name: "slack", kind: domain.NotificationChannelKindSlack, want: "slack"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := domain.NotificationChannelProfile{
				Kind:      tc.kind,
				SecretRef: "secret/openclarion/robot-webhook-url",
			}
			var gotProfile domain.NotificationChannelProfile
			var gotCredentials WebhookCredentials
			builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
				profile.SecretRef: "https://example.invalid/robot-hook",
			}}, func(profile domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
				gotProfile = profile
				gotCredentials = credentials
				return fakeIMProvider{}, nil
			})

			provider, err := builder.Build(context.Background(), profile)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if provider == nil {
				t.Fatal("provider is nil")
			}
			if gotProfile.Kind != tc.kind {
				t.Fatalf("profile.Kind = %q, want %q", gotProfile.Kind, tc.kind)
			}
			if gotCredentials.Format != tc.want {
				t.Fatalf("credentials.Format = %q, want %q", gotCredentials.Format, tc.want)
			}
		})
	}
}

func TestBuilderUsesEmailFactoryForEmailChannelKind(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindEmail,
		SecretRef: "secret/openclarion/report-email-url",
	}
	var gotProfile domain.NotificationChannelProfile
	var gotCredentials EmailCredentials
	builder := mustBuilderWithEmailFactory(
		t,
		fakeSecretResolver{values: map[string]string{
			profile.SecretRef: "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&starttls=disabled",
		}},
		nil,
		func(profile domain.NotificationChannelProfile, credentials EmailCredentials) (ports.IMProvider, error) {
			gotProfile = profile
			gotCredentials = credentials
			return fakeIMProvider{}, nil
		},
	)

	provider, err := builder.Build(context.Background(), profile)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotProfile.Kind != domain.NotificationChannelKindEmail {
		t.Fatalf("profile.Kind = %q, want email", gotProfile.Kind)
	}
	if gotCredentials.URL != "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&starttls=disabled" {
		t.Fatalf("credentials.URL = %q", gotCredentials.URL)
	}
}

func TestBuilderRejectsEmailWithoutFactory(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindEmail,
		SecretRef: "secret/openclarion/report-email-url",
	}
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&starttls=disabled",
	}}, nil)

	_, err := builder.Build(context.Background(), profile)
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("err = %v, want ErrUnsupportedKind", err)
	}
}

func TestBuilderRejectsInvalidWeComWebhookEndpoint(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindWeCom,
		SecretRef: "secret/openclarion/wecom-webhook-url",
	}
	tests := []struct {
		name string
		url  string
	}{
		{name: "generic endpoint", url: "https://example.invalid/wecom-compatible-hook"},
		{name: "missing key", url: fixtureWeComWebhookURLWithoutQuery()},
		{name: "http scheme", url: fixtureWeComWebhookURLWithScheme("http")},
		{name: "userinfo", url: fixtureWeComWebhookURLWithAuthority("user@")},
		{name: "fragment", url: fixtureWeComWebhookURL() + "#frag"},
		{name: "wrong path", url: fixtureWeComWebhookURLWithPath("/cgi-bin/webhook/other")},
		{name: "extra query", url: fixtureWeComWebhookURL() + "&debug=true"},
		{name: "whitespace key", url: fixtureWeComWebhookURLWithKey("fixture+key")},
		{name: "bad query", url: fixtureWeComWebhookURLWithoutQuery() + "?%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
				profile.SecretRef: tc.url,
			}}, func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error) {
				t.Fatal("factory should not be called for unusable WeCom endpoint")
				return nil, nil
			})

			_, err := builder.Build(context.Background(), profile)
			if !errors.Is(err, ErrCredentialUnusable) {
				t.Fatalf("err = %v, want %v", err, ErrCredentialUnusable)
			}
		})
	}
}

func TestResolverResolveDiagnosisCloseNotificationProvider(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		ID:             4,
		Name:           "Diagnosis close webhook",
		Kind:           domain.NotificationChannelKindWeCom,
		SecretRef:      "secret/openclarion/diagnosis-close-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose},
		Enabled:        true,
	}
	repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
		4: profile,
	}}
	var gotProfile domain.NotificationChannelProfile
	var gotCredentials WebhookCredentials
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: fixtureWeComWebhookURL(),
	}}, func(profile domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
		gotProfile = profile
		gotCredentials = credentials
		return fakeIMProvider{}, nil
	})
	resolver := mustResolver(t, repo, builder)

	provider, err := resolver.ResolveDiagnosisCloseNotificationProvider(context.Background(), 4)
	if err != nil {
		t.Fatalf("ResolveDiagnosisCloseNotificationProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotProfile.ID != 4 || gotCredentials.URL != fixtureWeComWebhookURL() || gotCredentials.Format != "wecom" {
		t.Fatalf("factory input profile=%+v credentials=%+v", gotProfile, gotCredentials)
	}
}

func TestResolverResolveDiagnosisConsultationNotificationProvider(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		ID:             5,
		Name:           "Diagnosis consultation webhook",
		Kind:           domain.NotificationChannelKindWeCom,
		SecretRef:      "secret/openclarion/diagnosis-consultation-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisConsultation},
		Enabled:        true,
	}
	repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
		5: profile,
	}}
	var gotProfile domain.NotificationChannelProfile
	var gotCredentials WebhookCredentials
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: fixtureWeComWebhookURL(),
	}}, func(profile domain.NotificationChannelProfile, credentials WebhookCredentials) (ports.IMProvider, error) {
		gotProfile = profile
		gotCredentials = credentials
		return fakeIMProvider{}, nil
	})
	resolver := mustResolver(t, repo, builder)

	provider, err := resolver.ResolveDiagnosisConsultationNotificationProvider(context.Background(), 5)
	if err != nil {
		t.Fatalf("ResolveDiagnosisConsultationNotificationProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
	if gotProfile.ID != 5 || gotCredentials.URL != fixtureWeComWebhookURL() || gotCredentials.Format != "wecom" {
		t.Fatalf("factory input profile=%+v credentials=%+v", gotProfile, gotCredentials)
	}
}

func TestResolverResolveReportNotificationProviderValidatesProfileReadiness(t *testing.T) {
	tests := []struct {
		name    string
		profile domain.NotificationChannelProfile
	}{
		{
			name: "disabled",
			profile: domain.NotificationChannelProfile{
				ID:             3,
				Kind:           domain.NotificationChannelKindWebhook,
				SecretRef:      "secret/openclarion/report-webhook-url",
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
				Enabled:        false,
			},
		},
		{
			name: "missing_report_scope",
			profile: domain.NotificationChannelProfile{
				ID:             3,
				Kind:           domain.NotificationChannelKindWebhook,
				SecretRef:      "secret/openclarion/report-webhook-url",
				DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeDiagnosisClose},
				Enabled:        true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				3: tc.profile,
			}}
			resolver := mustResolver(t, repo, mustBuilder(t, fakeSecretResolver{}, nil))

			_, err := resolver.ResolveReportNotificationProvider(context.Background(), 3)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestResolverResolveDiagnosisCloseNotificationProviderValidatesScope(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		ID:             4,
		Kind:           domain.NotificationChannelKindWeCom,
		SecretRef:      "secret/openclarion/report-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		Enabled:        true,
	}
	repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
		4: profile,
	}}
	resolver := mustResolver(t, repo, mustBuilder(t, fakeSecretResolver{}, nil))

	_, err := resolver.ResolveDiagnosisCloseNotificationProvider(context.Background(), 4)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
}

func TestResolverResolveDiagnosisConsultationNotificationProviderValidatesScope(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		ID:             5,
		Kind:           domain.NotificationChannelKindWeCom,
		SecretRef:      "secret/openclarion/report-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		Enabled:        true,
	}
	repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
		5: profile,
	}}
	resolver := mustResolver(t, repo, mustBuilder(t, fakeSecretResolver{}, nil))

	_, err := resolver.ResolveDiagnosisConsultationNotificationProvider(context.Background(), 5)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
}

func TestResolverRejectsGenericWebhookForDiagnosisDelivery(t *testing.T) {
	tests := []struct {
		name  string
		scope domain.NotificationDeliveryScope
		run   func(*Resolver) (ports.IMProvider, error)
	}{
		{
			name:  "diagnosis_consultation",
			scope: domain.NotificationDeliveryScopeDiagnosisConsultation,
			run: func(resolver *Resolver) (ports.IMProvider, error) {
				return resolver.ResolveDiagnosisConsultationNotificationProvider(context.Background(), 6)
			},
		},
		{
			name:  "diagnosis_close",
			scope: domain.NotificationDeliveryScopeDiagnosisClose,
			run: func(resolver *Resolver) (ports.IMProvider, error) {
				return resolver.ResolveDiagnosisCloseNotificationProvider(context.Background(), 6)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := domain.NotificationChannelProfile{
				ID:             6,
				Kind:           domain.NotificationChannelKindWebhook,
				SecretRef:      "secret/openclarion/diagnosis-webhook-url",
				DeliveryScopes: []domain.NotificationDeliveryScope{tc.scope},
				Enabled:        true,
			}
			repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				6: profile,
			}}
			builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
				profile.SecretRef: "https://example.invalid/diagnosis-hook",
			}}, func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error) {
				t.Fatal("factory should not be called for non-WeCom diagnosis delivery")
				return nil, nil
			})
			resolver := mustResolver(t, repo, builder)

			_, err := tc.run(resolver)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
			if !strings.Contains(err.Error(), "Enterprise WeChat") {
				t.Fatalf("err = %q, want Enterprise WeChat guidance", err.Error())
			}
		})
	}
}

func TestResolverMapsProviderBuildFailuresToInvariantViolation(t *testing.T) {
	base := domain.NotificationChannelProfile{
		ID:             3,
		Kind:           domain.NotificationChannelKindWebhook,
		SecretRef:      "secret/openclarion/report-webhook-url",
		DeliveryScopes: []domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		Enabled:        true,
	}
	tests := []struct {
		name    string
		profile domain.NotificationChannelProfile
		builder *Builder
	}{
		{
			name: "unsupported_kind",
			profile: func() domain.NotificationChannelProfile {
				profile := base
				profile.Kind = domain.NotificationChannelKind("pagerduty")
				return profile
			}(),
			builder: mustBuilder(t, fakeSecretResolver{}, nil),
		},
		{
			name:    "missing_resolver",
			profile: base,
			builder: mustBuilder(t, nil, nil),
		},
		{
			name:    "missing_secret",
			profile: base,
			builder: mustBuilder(t, fakeSecretResolver{}, nil),
		},
		{
			name:    "resolver_failure",
			profile: base,
			builder: mustBuilder(t, fakeSecretResolver{err: errors.New("backend detail")}, nil),
		},
		{
			name:    "bad_credential",
			profile: base,
			builder: mustBuilder(t, fakeSecretResolver{values: map[string]string{
				base.SecretRef: "bad url",
			}}, nil),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeConfigRepo{notificationChannels: map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile{
				3: tc.profile,
			}}
			resolver := mustResolver(t, repo, tc.builder)

			_, err := resolver.ResolveReportNotificationProvider(context.Background(), 3)
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestBuilderBuildMapsSecretFailures(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindWebhook,
		SecretRef: "secret/openclarion/report-webhook-url",
	}
	tests := []struct {
		name    string
		builder *Builder
		wantErr error
	}{
		{
			name:    "missing_resolver",
			builder: mustBuilder(t, nil, nil),
			wantErr: ErrSecretResolverUnavailable,
		},
		{
			name:    "missing_secret",
			builder: mustBuilder(t, fakeSecretResolver{}, nil),
			wantErr: ErrSecretNotFound,
		},
		{
			name:    "resolver_failure",
			builder: mustBuilder(t, fakeSecretResolver{err: errors.New("backend detail")}, nil),
			wantErr: ErrSecretResolveFailed,
		},
		{
			name: "empty_secret",
			builder: mustBuilder(t, fakeSecretResolver{values: map[string]string{
				profile.SecretRef: "",
			}}, nil),
			wantErr: ErrCredentialUnusable,
		},
		{
			name: "space_secret",
			builder: mustBuilder(t, fakeSecretResolver{values: map[string]string{
				profile.SecretRef: "bad url",
			}}, nil),
			wantErr: ErrCredentialUnusable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.builder.Build(context.Background(), profile)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestBuilderBuildMapsFactoryFailures(t *testing.T) {
	profile := domain.NotificationChannelProfile{
		Kind:      domain.NotificationChannelKindWebhook,
		SecretRef: "secret/openclarion/report-webhook-url",
	}
	builder := mustBuilder(t, fakeSecretResolver{values: map[string]string{
		profile.SecretRef: "https://example.invalid/report-hook",
	}}, func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error) {
		return nil, errors.New("raw endpoint detail")
	})

	_, err := builder.Build(context.Background(), profile)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
	if errors.Is(err, ErrSecretResolveFailed) {
		t.Fatalf("factory error should not be mapped as resolver failure: %v", err)
	}
}

func TestResolverRejectsMissingProfile(t *testing.T) {
	resolver := mustResolver(t, &fakeConfigRepo{}, mustBuilder(t, fakeSecretResolver{}, nil))

	_, err := resolver.ResolveReportNotificationProvider(context.Background(), 99)
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
}

func mustBuilder(t *testing.T, resolver ports.SecretResolver, factory WebhookFactory) *Builder {
	t.Helper()
	return mustBuilderWithEmailFactory(t, resolver, factory, nil)
}

func mustBuilderWithEmailFactory(t *testing.T, resolver ports.SecretResolver, factory WebhookFactory, emailFactory EmailFactory) *Builder {
	t.Helper()
	if factory == nil {
		factory = func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error) {
			return fakeIMProvider{}, nil
		}
	}
	opts := []Option{}
	if resolver != nil {
		opts = append(opts, WithSecretResolver(resolver))
	}
	if emailFactory != nil {
		opts = append(opts, WithEmailFactory(emailFactory))
	}
	builder, err := NewBuilder(factory, opts...)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	return builder
}

func mustResolver(t *testing.T, repo *fakeConfigRepo, builder *Builder) *Resolver {
	t.Helper()
	resolver, err := NewResolver(fakeUOWFactory{repo: repo}, builder)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return resolver
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

type fakeIMProvider struct{}

func (fakeIMProvider) SendNotification(context.Context, ports.IMNotification) (ports.IMDelivery, error) {
	return ports.IMDelivery{}, nil
}

func fixtureWeComWebhookURL() string {
	return fixtureWeComWebhookURLWithKey("fixture-key")
}

func fixtureWeComWebhookURLWithKey(key string) string {
	return fixtureWeComWebhookURLWithoutQuery() + "?key=" + key
}

func fixtureWeComWebhookURLWithoutQuery() string {
	return fixtureWeComWebhookURLWithScheme("https")
}

func fixtureWeComWebhookURLWithScheme(scheme string) string {
	return scheme + "://" + weComWebhookHost + weComWebhookPath
}

func fixtureWeComWebhookURLWithAuthority(authorityPrefix string) string {
	return "https://" + authorityPrefix + weComWebhookHost + weComWebhookPath + "?key=fixture-key"
}

func fixtureWeComWebhookURLWithPath(webhookPath string) string {
	return "https://" + weComWebhookHost + webhookPath + "?key=fixture-key"
}

type fakeUOWFactory struct {
	repo *fakeConfigRepo
}

func (f fakeUOWFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return fakeUOW{config: f.repo}, nil
}

func (f fakeUOWFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{config: f.repo})
}

type fakeUOW struct {
	ports.UnitOfWork
	config *fakeConfigRepo
}

func (u fakeUOW) Config() ports.ConfigurationRepository {
	if u.config == nil {
		return &fakeConfigRepo{}
	}
	return u.config
}

type fakeConfigRepo struct {
	ports.ConfigurationRepository
	notificationChannels map[domain.NotificationChannelProfileID]domain.NotificationChannelProfile
}

func (r *fakeConfigRepo) FindNotificationChannelProfileByID(_ context.Context, id domain.NotificationChannelProfileID) (domain.NotificationChannelProfile, error) {
	profile, ok := r.notificationChannels[id]
	if !ok {
		return domain.NotificationChannelProfile{}, domain.ErrNotFound
	}
	return profile, nil
}
