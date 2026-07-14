package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// ListAccessibleTenants implements api.ServerInterface.
func (s *Server) ListAccessibleTenants(w http.ResponseWriter, r *http.Request) {
	if s.tenantOperations == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "tenant operations are not configured", nil)
		return
	}
	principal, ok := s.authenticateTenantRegistryPrincipal(w, r)
	if !ok {
		return
	}
	rows, err := s.tenantOperations.ListAccessible(
		r.Context(),
		principal.Subject,
		s.localRBACBootstrapAdminSubject(principal.Subject),
	)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list accessible tenants failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.TenantListResponse{Items: tenantsToAPI(rows)})
}

// CreateTenant implements api.ServerInterface.
func (s *Server) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if s.tenantOperations == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "tenant operations are not configured", nil)
		return
	}
	principal, ok := s.authenticateTenantRegistryPrincipal(w, r)
	if !ok {
		return
	}
	if !s.localRBACBootstrapAdminSubject(principal.Subject) {
		writeError(r.Context(), w, s.logger, http.StatusForbidden, "tenant creation is not authorized", nil)
		return
	}
	var body api.TenantCreateRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if len(body.AdditionalProperties) != 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "request body contains unknown fields", nil)
		return
	}
	created, _, err := s.tenantOperations.CreateTenant(r.Context(), body.Key, body.Name, principal.Subject)
	if err != nil {
		s.writeTenantOperationError(r.Context(), w, "create tenant failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusCreated, tenantToAPI(created))
}

// UpdateTenantStatus implements api.ServerInterface.
func (s *Server) UpdateTenantStatus(w http.ResponseWriter, r *http.Request, tenantID int64) {
	if s.tenantOperations == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "tenant operations are not configured", nil)
		return
	}
	id, ok := validTenantID(w, r, s, tenantID)
	if !ok {
		return
	}
	if _, ok := s.authorizeTenantManager(w, r, id); !ok {
		return
	}
	var body api.TenantStatusUpdateRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if len(body.AdditionalProperties) != 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "request body contains unknown fields", nil)
		return
	}
	status := domain.TenantStatus(body.Status)
	if err := domain.ValidateTenantStatus(status); err != nil {
		s.writeTenantOperationError(r.Context(), w, "update tenant status failed", err)
		return
	}
	if id == domain.DefaultTenantID && status != domain.TenantStatusActive {
		s.writeTenantOperationError(
			r.Context(),
			w,
			"update tenant status failed",
			fmt.Errorf("default tenant cannot be disabled: %w", domain.ErrPreconditionFailed),
		)
		return
	}
	current, err := s.tenantOperations.FindTenant(r.Context(), id)
	if err != nil {
		s.writeTenantOperationError(r.Context(), w, "find tenant before status update failed", err)
		return
	}
	if current.Status == status {
		writeJSON(r.Context(), w, s.logger, http.StatusOK, tenantToAPI(current))
		return
	}

	// A disabled tenant must never retain runnable schedules. Pause first when
	// disabling; when enabling, persist active state before restoring schedules.
	if status == domain.TenantStatusDisabled {
		paused := current
		paused.Status = status
		if err := s.syncTenantScheduleState(r.Context(), paused); err != nil {
			writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "pause tenant schedules failed", err)
			return
		}
	}
	updated, err := s.tenantOperations.UpdateStatus(r.Context(), id, status)
	if err != nil {
		s.writeTenantOperationError(r.Context(), w, "update tenant status failed", err)
		return
	}
	if status == domain.TenantStatusActive {
		if err := s.syncTenantScheduleState(r.Context(), updated); err != nil {
			writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "restore tenant schedules failed", err)
			return
		}
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, tenantToAPI(updated))
}

// The pointer preserves the distinction between a required false value and an
// omitted property, which the generated value-type model cannot represent.
type tenantMembershipWriteRequest struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
	Enabled *bool  `json:"enabled"`
}

// ListTenantMemberships implements api.ServerInterface.
func (s *Server) ListTenantMemberships(w http.ResponseWriter, r *http.Request, tenantID int64) {
	if s.tenantOperations == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "tenant operations are not configured", nil)
		return
	}
	id, ok := validTenantID(w, r, s, tenantID)
	if !ok {
		return
	}
	if _, ok := s.authorizeTenantManager(w, r, id); !ok {
		return
	}
	rows, err := s.tenantOperations.ListMemberships(r.Context(), id)
	if err != nil {
		s.writeTenantOperationError(r.Context(), w, "list tenant memberships failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.TenantMembershipListResponse{Items: tenantMembershipsToAPI(rows)})
}

// SetTenantMembership implements api.ServerInterface.
func (s *Server) SetTenantMembership(w http.ResponseWriter, r *http.Request, tenantID int64) {
	if s.tenantOperations == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "tenant operations are not configured", nil)
		return
	}
	id, ok := validTenantID(w, r, s, tenantID)
	if !ok {
		return
	}
	principal, ok := s.authorizeTenantManager(w, r, id)
	if !ok {
		return
	}
	var body tenantMembershipWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if body.Enabled == nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "enabled is required", nil)
		return
	}
	saved, err := s.tenantOperations.SetMembership(
		r.Context(),
		id,
		body.Subject,
		domain.TenantMembershipRole(body.Role),
		*body.Enabled,
		principal.Subject,
	)
	if err != nil {
		s.writeTenantOperationError(r.Context(), w, "set tenant membership failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, tenantMembershipToAPI(saved))
}

func (s *Server) authenticateTenantRegistryPrincipal(w http.ResponseWriter, r *http.Request) (ports.AuthPrincipal, bool) {
	if s.diagnosis.authProvider == nil && s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "authentication is not configured", nil)
		return ports.AuthPrincipal{}, false
	}
	principal, err := s.authenticateLocalRBACPrincipal(r.Context(), r.Header)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return ports.AuthPrincipal{}, false
	}
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return ports.AuthPrincipal{}, false
	}
	principal.Subject = subject
	return principal, true
}

func (s *Server) authorizeTenantManager(w http.ResponseWriter, r *http.Request, tenantID domain.TenantID) (ports.AuthPrincipal, bool) {
	principal, ok := s.authenticateTenantRegistryPrincipal(w, r)
	if !ok {
		return ports.AuthPrincipal{}, false
	}
	allowed, err := s.tenantOperations.CanManage(
		r.Context(),
		tenantID,
		principal.Subject,
		s.localRBACBootstrapAdminSubject(principal.Subject),
	)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "authorize tenant manager failed", err)
		return ports.AuthPrincipal{}, false
	}
	if !allowed {
		writeError(r.Context(), w, s.logger, http.StatusForbidden, "tenant administration is not authorized", nil)
		return ports.AuthPrincipal{}, false
	}
	return principal, true
}

func validTenantID(w http.ResponseWriter, r *http.Request, server *Server, raw int64) (domain.TenantID, bool) {
	if raw <= 0 {
		writeError(r.Context(), w, server.logger, http.StatusBadRequest, "tenant_id must be positive", nil)
		return 0, false
	}
	return domain.TenantID(raw), true
}

func (s *Server) writeTenantOperationError(ctx context.Context, w http.ResponseWriter, message string, err error) {
	switch {
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, s.logger, http.StatusBadRequest, err.Error(), nil)
	case errors.Is(err, domain.ErrPreconditionFailed), errors.Is(err, domain.ErrAlreadyExists):
		writeError(ctx, w, s.logger, http.StatusConflict, err.Error(), nil)
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, s.logger, http.StatusNotFound, "tenant not found", nil)
	default:
		writeError(ctx, w, s.logger, http.StatusInternalServerError, message, err)
	}
}

func (s *Server) syncTenantScheduleState(ctx context.Context, tenant domain.Tenant) error {
	if s.scheduleSyncer == nil {
		return nil
	}
	identity, err := tenancy.NewIdentity(tenant.ID, tenant.Key)
	if err != nil {
		return err
	}
	ctx, err = tenancy.WithTenant(ctx, identity)
	if err != nil {
		return err
	}
	var schedules []domain.ReportWorkflowSchedule
	err = s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var listErr error
		schedules, listErr = uow.Config().ListReportWorkflowSchedules(ctx, maxListLimit+1)
		return listErr
	})
	if err != nil {
		return err
	}
	if len(schedules) > maxListLimit {
		return fmt.Errorf("tenant %q exceeds %d report workflow schedules: %w", tenant.Key, maxListLimit, domain.ErrInvariantViolation)
	}
	for _, schedule := range schedules {
		if tenant.Status != domain.TenantStatusActive {
			schedule.Enabled = false
		}
		if err := s.scheduleSyncer.SyncReportWorkflowSchedule(ctx, schedule); err != nil {
			return fmt.Errorf("sync schedule %d: %w", schedule.ID, err)
		}
	}
	return nil
}

func tenantToAPI(tenant domain.Tenant) api.Tenant {
	return api.Tenant{
		ID:        int64(tenant.ID),
		Key:       tenant.Key,
		Name:      tenant.Name,
		Status:    string(tenant.Status),
		CreatedAt: tenant.CreatedAt,
		UpdatedAt: tenant.UpdatedAt,
	}
}

func tenantsToAPI(rows []domain.Tenant) []api.Tenant {
	out := make([]api.Tenant, len(rows))
	for i, row := range rows {
		out[i] = tenantToAPI(row)
	}
	return out
}

func tenantMembershipToAPI(membership domain.TenantMembership) api.TenantMembership {
	return api.TenantMembership{
		ID:        int64(membership.ID),
		TenantID:  int64(membership.TenantID),
		Subject:   membership.Subject,
		Role:      string(membership.Role),
		Enabled:   membership.Enabled,
		CreatedBy: membership.CreatedBy,
		CreatedAt: membership.CreatedAt,
		UpdatedAt: membership.UpdatedAt,
	}
}

func tenantMembershipsToAPI(rows []domain.TenantMembership) []api.TenantMembership {
	out := make([]api.TenantMembership, len(rows))
	for i, row := range rows {
		out[i] = tenantMembershipToAPI(row)
	}
	return out
}
