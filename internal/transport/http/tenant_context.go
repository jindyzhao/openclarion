package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/tenantops"
)

const tenantSelectionHeader = "X-OpenClarion-Tenant"

// WithTenantOperations enables authenticated tenant selection and tenant
// administration handlers.
func WithTenantOperations(service *tenantops.Service) ServerOption {
	return func(server *Server) {
		server.tenantOperations = service
	}
}

func (s *Server) bindAuthenticatedTenant(
	ctx context.Context,
	header http.Header,
	principal ports.AuthPrincipal,
) (ports.AuthPrincipal, context.Context, error) {
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		return ports.AuthPrincipal{}, ctx, err
	}
	requestedKey := strings.TrimSpace(header.Get(tenantSelectionHeader))
	if requestedKey != header.Get(tenantSelectionHeader) {
		return ports.AuthPrincipal{}, ctx, fmt.Errorf("tenant selection header must not contain surrounding whitespace: %w", domain.ErrInvariantViolation)
	}
	bound := principal.TenantID > 0 || principal.TenantKey != ""
	if bound && (principal.TenantID <= 0 || principal.TenantKey == "") {
		return ports.AuthPrincipal{}, ctx, fmt.Errorf("authenticated tenant binding is incomplete: %w", diagnosisauth.ErrUnauthenticated)
	}
	if bound && requestedKey != "" && requestedKey != principal.TenantKey {
		return ports.AuthPrincipal{}, ctx, fmt.Errorf("tenant selection cannot override a signed session: %w", tenantops.ErrAccessDenied)
	}
	if bound {
		requestedKey = principal.TenantKey
	}

	var identity tenancy.Identity
	if s.tenantOperations == nil {
		if requestedKey != "" && requestedKey != domain.DefaultTenantKey {
			return ports.AuthPrincipal{}, ctx, tenantops.ErrAccessDenied
		}
		if bound {
			identity, err = tenancy.NewIdentity(principal.TenantID, principal.TenantKey)
		} else {
			identity = tenancy.DefaultIdentity()
		}
	} else {
		identity, err = s.tenantOperations.ResolveAccess(
			ctx,
			subject,
			requestedKey,
			s.localRBACBootstrapAdminSubject(subject),
		)
	}
	if err != nil {
		return ports.AuthPrincipal{}, ctx, err
	}
	if bound && (identity.ID != principal.TenantID || identity.Key != principal.TenantKey) {
		return ports.AuthPrincipal{}, ctx, fmt.Errorf("signed tenant binding no longer resolves to the same tenant: %w", tenantops.ErrAccessDenied)
	}
	tenantCtx, err := tenancy.WithTenant(ctx, identity)
	if err != nil {
		return ports.AuthPrincipal{}, ctx, err
	}
	principal.Subject = subject
	principal.TenantID = identity.ID
	principal.TenantKey = identity.Key
	return principal, tenantCtx, nil
}

func (s *Server) bindSessionIssuanceTenant(
	ctx context.Context,
	header http.Header,
	principal ports.AuthPrincipal,
) (ports.AuthPrincipal, context.Context, error) {
	requestedKey := strings.TrimSpace(header.Get(tenantSelectionHeader))
	if requestedKey == "" || requestedKey == principal.TenantKey {
		return s.bindAuthenticatedTenant(ctx, header, principal)
	}
	// A valid signed session may exchange itself for a session in another
	// accessible tenant. Resolve membership again instead of allowing a normal
	// request to override its cryptographic tenant binding.
	principal.TenantID = 0
	principal.TenantKey = ""
	return s.bindAuthenticatedTenant(ctx, header, principal)
}

func (s *Server) writeTenantBindingError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tenantops.ErrAccessDenied),
		errors.Is(err, tenantops.ErrTenantDisabled),
		errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, s.logger, http.StatusForbidden, "tenant access denied", nil)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, s.logger, http.StatusBadRequest, err.Error(), nil)
	default:
		writeError(ctx, w, s.logger, http.StatusInternalServerError, "resolve tenant access failed", err)
	}
}
