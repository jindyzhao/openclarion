package notificationchannelprovider

import (
	"context"
	"errors"
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
				profile.Kind = domain.NotificationChannelKind("email")
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
	if factory == nil {
		factory = func(domain.NotificationChannelProfile, WebhookCredentials) (ports.IMProvider, error) {
			return fakeIMProvider{}, nil
		}
	}
	opts := []Option{}
	if resolver != nil {
		opts = append(opts, WithSecretResolver(resolver))
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
