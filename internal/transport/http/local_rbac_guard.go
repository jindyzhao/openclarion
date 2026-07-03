package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
)

func (s *Server) authorizeLocalRBACRequest(w http.ResponseWriter, r *http.Request, permission domain.RBACPermission) bool {
	return s.authorizeLocalRBACRequestForScope(w, r, permission, domain.RBACScopeKindGlobal, "")
}

func (s *Server) authorizeLocalRBACPrincipalForGlobalPermission(
	w http.ResponseWriter,
	r *http.Request,
	permission domain.RBACPermission,
) (domain.RBACPrincipal, bool) {
	_, rbacPrincipal, ok := s.resolveLocalRBACPrincipal(w, r)
	if !ok {
		return domain.RBACPrincipal{}, false
	}
	allowed, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		rbacPrincipal,
		permission,
		domain.RBACScopeKindGlobal,
		"",
		true,
	)
	if !ok || !allowed {
		return domain.RBACPrincipal{}, false
	}
	return rbacPrincipal, true
}

func (s *Server) authorizeLocalRBACRequestForScope(
	w http.ResponseWriter,
	r *http.Request,
	permission domain.RBACPermission,
	scopeKind domain.RBACScopeKind,
	scopeKey string,
) bool {
	_, ok := s.authorizeLocalRBACPrincipalForScope(w, r, permission, scopeKind, scopeKey)
	return ok
}

func (s *Server) authorizeLocalRBACPrincipal(
	w http.ResponseWriter,
	r *http.Request,
	permission domain.RBACPermission,
) (ports.AuthPrincipal, bool) {
	return s.authorizeLocalRBACPrincipalForScope(w, r, permission, domain.RBACScopeKindGlobal, "")
}

func (s *Server) authorizeLocalRBACPrincipalForScope(
	w http.ResponseWriter,
	r *http.Request,
	permission domain.RBACPermission,
	scopeKind domain.RBACScopeKind,
	scopeKey string,
) (ports.AuthPrincipal, bool) {
	principal, _, ok := s.authorizeLocalRBACPrincipalsForScope(w, r, permission, scopeKind, scopeKey)
	return principal, ok
}

func (s *Server) authorizeLocalRBACPrincipalsForScope(
	w http.ResponseWriter,
	r *http.Request,
	permission domain.RBACPermission,
	scopeKind domain.RBACScopeKind,
	scopeKey string,
) (ports.AuthPrincipal, domain.RBACPrincipal, bool) {
	principal, rbacPrincipal, ok := s.resolveLocalRBACPrincipal(w, r)
	if !ok {
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	allowed, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		rbacPrincipal,
		permission,
		scopeKind,
		scopeKey,
		true,
	)
	if !ok || !allowed {
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	return principal, rbacPrincipal, true
}

func (s *Server) resolveLocalRBACPrincipal(
	w http.ResponseWriter,
	r *http.Request,
) (ports.AuthPrincipal, domain.RBACPrincipal, bool) {
	if s.diagnosis.authProvider == nil && s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	principal, err := s.authenticateLocalRBACPrincipal(r.Context(), r.Header)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	principal.Subject = subject
	departmentKeys, err := s.localRBACDepartmentKeys(r.Context(), subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "resolve local rbac principal failed", err)
		return ports.AuthPrincipal{}, domain.RBACPrincipal{}, false
	}
	return principal, domain.RBACPrincipal{
		Subject:        subject,
		DepartmentKeys: departmentKeys,
	}, true
}

func (s *Server) authorizeResolvedLocalRBACPrincipalForScope(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	permission domain.RBACPermission,
	scopeKind domain.RBACScopeKind,
	scopeKey string,
	writeForbidden bool,
) (bool, bool) {
	if s.localRBACBootstrapAdminSubject(principal.Subject) {
		return true, true
	}
	if s.rbacAuthorizer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "rbac authorizer is not configured", nil)
		return false, false
	}
	ownerAllowed, err := s.diagnosisRoomOwnerAuthorizes(r.Context(), principal, permission, scopeKind, scopeKey)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "diagnosis room owner authorization failed", err)
		return false, false
	}
	if ownerAllowed {
		return true, true
	}
	decision, err := s.rbacAuthorizer.Authorize(r.Context(), rbacusecase.AuthorizeRequest{
		Principal:  principal,
		Permission: permission,
		ScopeKind:  scopeKind,
		ScopeKey:   scopeKey,
	})
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return false, false
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "rbac authorization failed", err)
		return false, false
	}
	if !decision.Allowed {
		if writeForbidden {
			writeError(r.Context(), w, s.logger, http.StatusForbidden, "unauthorized", nil)
		}
		return false, true
	}
	return true, true
}

func (s *Server) diagnosisRoomOwnerAuthorizes(
	ctx context.Context,
	principal domain.RBACPrincipal,
	permission domain.RBACPermission,
	scopeKind domain.RBACScopeKind,
	scopeKey string,
) (bool, error) {
	subject := strings.TrimSpace(principal.Subject)
	sessionKey := strings.TrimSpace(scopeKey)
	if subject == "" ||
		sessionKey == "" ||
		scopeKind != domain.RBACScopeKindDiagnosisRoom ||
		!diagnosisRoomOwnerPermission(permission) {
		return false, nil
	}
	var session domain.ChatSession
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		repo := uow.Diagnosis()
		if repo == nil {
			return domain.ErrNotFound
		}
		var lerr error
		session, lerr = repo.FindChatSessionByKey(ctx, sessionKey)
		return lerr
	})
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find diagnosis room owner: %w", err)
	}
	return strings.TrimSpace(session.OwnerSubject) == subject, nil
}

func diagnosisRoomOwnerPermission(permission domain.RBACPermission) bool {
	switch permission {
	case domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACPermissionDiagnosisRoomParticipate,
		domain.RBACPermissionDiagnosisRoomAdminister:
		return true
	default:
		return false
	}
}

func (s *Server) localRBACBootstrapAdminSubject(subject string) bool {
	if s == nil || len(s.rbacBootstrapAdminSubjects) == 0 {
		return false
	}
	return s.rbacBootstrapAdminSubjects[strings.TrimSpace(subject)]
}

func rbacResourceScopeKey(id int64) string {
	return strconv.FormatInt(id, 10)
}

func (s *Server) authenticateLocalRBACPrincipal(ctx context.Context, header http.Header) (ports.AuthPrincipal, error) {
	authorization, err := authorizationCredentialsHeader(header.Get("Authorization"))
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	if s.diagnosis.sessionIssuer != nil {
		principal, sessionErr := s.diagnosis.sessionIssuer.AuthenticateAuthorization(ctx, authorization)
		if sessionErr == nil {
			return principal, nil
		}
		if s.diagnosis.authProvider == nil {
			return ports.AuthPrincipal{}, sessionErr
		}
	}
	if s.diagnosis.authProvider == nil {
		return ports.AuthPrincipal{}, diagnosisauth.ErrUnauthenticated
	}
	credentials, err := diagnosisAuthAuxiliaryCredentials(header)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	return authenticateDiagnosisAuthorization(ctx, s.diagnosis.authProvider, authorization, credentials)
}

func (s *Server) localRBACDepartmentKeys(ctx context.Context, subject string) ([]string, error) {
	users, err := s.localRBACDirectoryUsersBySubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	return directoryUserDepartmentKeys(users), nil
}

func (s *Server) localRBACDirectoryUsersBySubject(ctx context.Context, subject string) ([]domain.DirectoryUser, error) {
	var users []domain.DirectoryUser
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		users, lerr = uow.Config().ListDirectoryUsersBySubject(ctx, subject, maxListLimit)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Server) localRBACDirectoryUsersByExternalID(ctx context.Context, externalID string) ([]domain.DirectoryUser, error) {
	var users []domain.DirectoryUser
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		users, lerr = uow.Config().ListDirectoryUsersByExternalID(ctx, externalID, maxListLimit)
		return lerr
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

func directoryUserDepartmentKeys(users []domain.DirectoryUser) []string {
	out := []string{}
	for _, user := range users {
		if !user.Active {
			continue
		}
		for _, key := range user.DepartmentExternalIDs {
			key = strings.TrimSpace(key)
			if key == "" || slices.Contains(out, key) {
				continue
			}
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}
