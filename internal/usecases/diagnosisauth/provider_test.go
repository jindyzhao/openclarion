package diagnosisauth

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestAnyAuthProviderReturnsFirstSuccessfulProvider(t *testing.T) {
	first := &fakeAuthProvider{err: ErrUnauthenticated}
	second := &fakeAuthProvider{principal: ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}}
	provider, err := NewAnyAuthProvider(first, nil, second)
	if err != nil {
		t.Fatalf("NewAnyAuthProvider: %v", err)
	}

	principal, err := provider.AuthenticateAuthorization(context.Background(), "Bearer token-1")
	if err != nil {
		t.Fatalf("AuthenticateAuthorization: %v", err)
	}
	if principal.Subject != "operator-1" {
		t.Fatalf("subject = %q, want operator-1", principal.Subject)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("calls first=%d second=%d, want 1 each", first.calls, second.calls)
	}
}

func TestAnyAuthProviderPassesAuxiliaryCredentialsOnlyToSupportingProviders(t *testing.T) {
	first := &fakeAuthProvider{principal: ports.AuthPrincipal{
		Subject: "unexpected",
		Roles:   []ports.AuthRole{ports.AuthRoleAdmin},
	}}
	second := &fakeAuxAuthProvider{principal: ports.AuthPrincipal{
		Subject: "operator-1",
		Roles:   []ports.AuthRole{ports.AuthRoleOwner},
	}}
	provider, err := NewAnyAuthProvider(first, second)
	if err != nil {
		t.Fatalf("NewAnyAuthProvider: %v", err)
	}

	principal, err := provider.AuthenticateAuthorizationWithAuxiliaryCredentials(
		context.Background(),
		"Bearer id-token",
		ports.AuthAuxiliaryCredentials{OIDCAccessToken: "access-token-1"},
	)
	if err != nil {
		t.Fatalf("AuthenticateAuthorizationWithAuxiliaryCredentials: %v", err)
	}
	if principal.Subject != "operator-1" {
		t.Fatalf("Subject = %q, want operator-1", principal.Subject)
	}
	if first.calls != 0 {
		t.Fatalf("first plain provider calls = %d, want 0", first.calls)
	}
	if second.calls != 1 || second.credentials.OIDCAccessToken != "access-token-1" {
		t.Fatalf("second calls = %d credentials = %#v", second.calls, second.credentials)
	}
}

func TestAnyAuthProviderReturnsLastError(t *testing.T) {
	wantErr := errors.New("second failed")
	provider, err := NewAnyAuthProvider(
		&fakeAuthProvider{err: ErrUnauthenticated},
		&fakeAuthProvider{err: wantErr},
	)
	if err != nil {
		t.Fatalf("NewAnyAuthProvider: %v", err)
	}

	_, err = provider.AuthenticateAuthorization(context.Background(), "Bearer token-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("AuthenticateAuthorization err = %v, want second error", err)
	}
}

func TestAnyAuthProviderCombinesRoleMappingStatus(t *testing.T) {
	provider, err := NewAnyAuthProvider(
		&fakeAuthProvider{
			status: ports.AuthRoleMappingStatus{
				OwnerMappingCount: 1,
				DefaultRoles:      []ports.AuthRole{ports.AuthRoleOwner},
			},
		},
		&fakeAuthProvider{},
		&fakeAuthProvider{
			status: ports.AuthRoleMappingStatus{
				AdminMappingCount: 2,
				DefaultRoles:      []ports.AuthRole{ports.AuthRoleOwner, ports.AuthRoleAdmin},
			},
		},
	)
	if err != nil {
		t.Fatalf("NewAnyAuthProvider: %v", err)
	}

	status := provider.RoleMappingStatus()
	if status.OwnerMappingCount != 1 || status.AdminMappingCount != 2 {
		t.Fatalf("role mapping status = %+v", status)
	}
	if !slices.Equal(status.DefaultRoles, []ports.AuthRole{ports.AuthRoleOwner, ports.AuthRoleAdmin}) {
		t.Fatalf("default roles = %+v", status.DefaultRoles)
	}
}

func TestAnyAuthProviderReportsLeastSafeTransportPolicyStatus(t *testing.T) {
	provider, err := NewAnyAuthProvider(
		&fakeAuthProvider{transportStatus: ports.AuthTransportPolicyStatus{
			Security: ports.AuthTransportSecurityTLS,
		}},
		&fakeAuthProvider{transportStatus: ports.AuthTransportPolicyStatus{
			Security: ports.AuthTransportSecurityStartTLS,
		}},
		&fakeAuthProvider{transportStatus: ports.AuthTransportPolicyStatus{
			Security: ports.AuthTransportSecurityInsecurePlaintext,
		}},
	)
	if err != nil {
		t.Fatalf("NewAnyAuthProvider: %v", err)
	}

	status := provider.TransportPolicyStatus()
	if status.Security != ports.AuthTransportSecurityInsecurePlaintext {
		t.Fatalf("transport security = %q, want insecure_plaintext", status.Security)
	}
}

type fakeAuthProvider struct {
	calls           int
	principal       ports.AuthPrincipal
	err             error
	status          ports.AuthRoleMappingStatus
	transportStatus ports.AuthTransportPolicyStatus
}

func (p *fakeAuthProvider) AuthenticateAuthorization(context.Context, string) (ports.AuthPrincipal, error) {
	p.calls++
	if p.err != nil {
		return ports.AuthPrincipal{}, p.err
	}
	return p.principal, nil
}

func (p *fakeAuthProvider) RoleMappingStatus() ports.AuthRoleMappingStatus {
	return ports.AuthRoleMappingStatus{
		OwnerMappingCount: p.status.OwnerMappingCount,
		AdminMappingCount: p.status.AdminMappingCount,
		DefaultRoles:      append([]ports.AuthRole(nil), p.status.DefaultRoles...),
	}
}

func (p *fakeAuthProvider) TransportPolicyStatus() ports.AuthTransportPolicyStatus {
	return p.transportStatus
}

type fakeAuxAuthProvider struct {
	calls       int
	credentials ports.AuthAuxiliaryCredentials
	principal   ports.AuthPrincipal
	err         error
}

func (p *fakeAuxAuthProvider) AuthenticateAuthorization(context.Context, string) (ports.AuthPrincipal, error) {
	return ports.AuthPrincipal{}, ErrUnauthenticated
}

func (p *fakeAuxAuthProvider) AuthenticateAuthorizationWithAuxiliaryCredentials(_ context.Context, _ string, credentials ports.AuthAuxiliaryCredentials) (ports.AuthPrincipal, error) {
	p.calls++
	p.credentials = credentials
	if p.err != nil {
		return ports.AuthPrincipal{}, p.err
	}
	return p.principal, nil
}
