package diagnosisauth

import (
	"context"
	"fmt"
	"slices"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// AnyAuthProvider tries configured providers in order and returns the first
// successful principal.
type AnyAuthProvider struct {
	providers []ports.AuthProvider
}

var _ ports.AuthProvider = (*AnyAuthProvider)(nil)
var _ ports.AuthProviderWithAuxiliaryCredentials = (*AnyAuthProvider)(nil)
var _ ports.AuthRoleMappingReporter = (*AnyAuthProvider)(nil)
var _ ports.AuthTransportPolicyReporter = (*AnyAuthProvider)(nil)

// NewAnyAuthProvider constructs an ordered authentication fallback chain.
func NewAnyAuthProvider(providers ...ports.AuthProvider) (*AnyAuthProvider, error) {
	out := make([]ports.AuthProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			out = append(out, provider)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("diagnosis auth: at least one auth provider is required: %w", domain.ErrInvariantViolation)
	}
	return &AnyAuthProvider{providers: out}, nil
}

// AuthenticateAuthorization returns the first provider success.
func (p *AnyAuthProvider) AuthenticateAuthorization(ctx context.Context, authorization string) (ports.AuthPrincipal, error) {
	return p.authenticateAuthorization(ctx, authorization, ports.AuthAuxiliaryCredentials{})
}

// AuthenticateAuthorizationWithAuxiliaryCredentials returns the first provider
// success while passing auxiliary credentials only to providers that explicitly
// support them.
func (p *AnyAuthProvider) AuthenticateAuthorizationWithAuxiliaryCredentials(ctx context.Context, authorization string, credentials ports.AuthAuxiliaryCredentials) (ports.AuthPrincipal, error) {
	return p.authenticateAuthorization(ctx, authorization, credentials)
}

func (p *AnyAuthProvider) authenticateAuthorization(ctx context.Context, authorization string, credentials ports.AuthAuxiliaryCredentials) (ports.AuthPrincipal, error) {
	if err := ctx.Err(); err != nil {
		return ports.AuthPrincipal{}, err
	}
	if p == nil || len(p.providers) == 0 {
		return ports.AuthPrincipal{}, fmt.Errorf("diagnosis auth: auth provider chain is not configured")
	}
	var lastErr error
	hasAuxiliaryCredentials := credentials.OIDCAccessToken != ""
	for _, provider := range p.providers {
		var principal ports.AuthPrincipal
		var err error
		if enhanced, ok := provider.(ports.AuthProviderWithAuxiliaryCredentials); ok {
			principal, err = enhanced.AuthenticateAuthorizationWithAuxiliaryCredentials(ctx, authorization, credentials)
		} else if hasAuxiliaryCredentials {
			err = ErrUnauthenticated
		} else {
			principal, err = provider.AuthenticateAuthorization(ctx, authorization)
		}
		if err == nil {
			return principal, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrUnauthenticated
	}
	return ports.AuthPrincipal{}, lastErr
}

// RoleMappingStatus combines non-sensitive role-mapping summaries exposed by
// providers in the fallback chain.
func (p *AnyAuthProvider) RoleMappingStatus() ports.AuthRoleMappingStatus {
	if p == nil {
		return ports.AuthRoleMappingStatus{}
	}
	var out ports.AuthRoleMappingStatus
	for _, provider := range p.providers {
		reporter, ok := provider.(ports.AuthRoleMappingReporter)
		if !ok {
			continue
		}
		status := reporter.RoleMappingStatus()
		out.OwnerMappingCount += status.OwnerMappingCount
		out.AdminMappingCount += status.AdminMappingCount
		for _, role := range status.DefaultRoles {
			if !slices.Contains(out.DefaultRoles, role) {
				out.DefaultRoles = append(out.DefaultRoles, role)
			}
		}
	}
	return out
}

// TransportPolicyStatus combines non-sensitive transport summaries exposed by
// providers in the fallback chain. Any explicitly allowed plaintext transport
// wins because it is the least safe accepted path.
func (p *AnyAuthProvider) TransportPolicyStatus() ports.AuthTransportPolicyStatus {
	if p == nil {
		return ports.AuthTransportPolicyStatus{}
	}
	var out ports.AuthTransportPolicyStatus
	for _, provider := range p.providers {
		reporter, ok := provider.(ports.AuthTransportPolicyReporter)
		if !ok {
			continue
		}
		status := reporter.TransportPolicyStatus()
		switch status.Security {
		case ports.AuthTransportSecurityInsecurePlaintext:
			return status
		case ports.AuthTransportSecurityStartTLS:
			out = status
		case ports.AuthTransportSecurityTLS:
			if out.Security == "" {
				out = status
			}
		}
	}
	return out
}
