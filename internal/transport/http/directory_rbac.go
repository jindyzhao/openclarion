package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/directorysync"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
)

// DirectorySyncer is the transport-facing directory projection sync usecase.
type DirectorySyncer interface {
	Sync(ctx context.Context, req directorysync.SyncRequest) (directorysync.Result, error)
}

// RBACAuthorizer is the transport-facing local authorization usecase.
type RBACAuthorizer interface {
	Authorize(ctx context.Context, req rbacusecase.AuthorizeRequest) (rbacusecase.AuthorizeDecision, error)
}

const maxRBACCurrentAuthorizationRequests = 50

// WithDirectorySyncer enables local directory projection sync actions.
func WithDirectorySyncer(syncer DirectorySyncer, provider ...string) ServerOption {
	return func(s *Server) {
		s.directorySyncer = syncer
		s.directorySyncProvider = firstNonEmptyString(provider...)
	}
}

// WithRBACAuthorizer enables local RBAC authorization preview actions.
func WithRBACAuthorizer(authorizer RBACAuthorizer) ServerOption {
	return func(s *Server) {
		s.rbacAuthorizer = authorizer
	}
}

// WithLocalRBACBootstrapAdminSubjects grants authenticated subjects a
// break-glass global admin authorization while the local RBAC table is being
// bootstrapped.
func WithLocalRBACBootstrapAdminSubjects(subjects []string) ServerOption {
	return func(s *Server) {
		if len(subjects) == 0 {
			return
		}
		if s.rbacBootstrapAdminSubjects == nil {
			s.rbacBootstrapAdminSubjects = make(map[string]bool, len(subjects))
		}
		for _, subject := range subjects {
			subject = strings.TrimSpace(subject)
			if subject != "" {
				s.rbacBootstrapAdminSubjects[subject] = true
			}
		}
	}
}

// SyncDirectory implements api.ServerInterface.
func (s *Server) SyncDirectory(w http.ResponseWriter, r *http.Request) {
	if s.directorySyncer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "directory sync is not configured", nil)
		return
	}
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDirectoryManage) {
		return
	}
	body, err := decodeDirectorySyncRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	result, err := s.directorySyncer.Sync(r.Context(), directorySyncRequest(body, s.directorySyncProvider))
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "directory sync failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DirectorySyncResponse{
		DepartmentPages:     result.DepartmentPages,
		UserPages:           result.UserPages,
		DepartmentsUpserted: result.DepartmentsUpserted,
		UsersUpserted:       result.UsersUpserted,
		UsersDeactivated:    result.UsersDeactivated,
		SyncedAt:            result.Run.SyncedAt,
	})
}

// ListDirectoryDepartments implements api.ServerInterface.
func (s *Server) ListDirectoryDepartments(w http.ResponseWriter, r *http.Request, params api.ListDirectoryDepartmentsParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDirectoryRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	provider := ""
	if params.Provider != nil {
		provider = strings.TrimSpace(*params.Provider)
	}

	var departments []domain.DirectoryDepartment
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		departments, lerr = uow.Config().ListDirectoryDepartments(ctx, provider, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list directory departments failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DirectoryDepartmentListResponse{
		Items: directoryDepartmentResponses(departments),
	})
}

// ListDirectoryUsers implements api.ServerInterface.
func (s *Server) ListDirectoryUsers(w http.ResponseWriter, r *http.Request, params api.ListDirectoryUsersParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDirectoryRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	provider := ""
	if params.Provider != nil {
		provider = strings.TrimSpace(*params.Provider)
	}

	var users []domain.DirectoryUser
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		users, lerr = uow.Config().ListDirectoryUsers(ctx, provider, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list directory users failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DirectoryUserListResponse{
		Items: directoryUserResponses(users),
	})
}

// ListDirectorySyncRuns implements api.ServerInterface.
func (s *Server) ListDirectorySyncRuns(w http.ResponseWriter, r *http.Request, params api.ListDirectorySyncRunsParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDirectoryRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	provider := ""
	if params.Provider != nil {
		provider = strings.TrimSpace(*params.Provider)
	}

	var runs []domain.DirectorySyncRun
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		runs, lerr = uow.Config().ListDirectorySyncRuns(ctx, provider, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list directory sync runs failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DirectorySyncRunListResponse{
		Items: directorySyncRunResponses(runs),
	})
}

// ListRBACAssignments implements api.ServerInterface.
func (s *Server) ListRBACAssignments(w http.ResponseWriter, r *http.Request, params api.ListRBACAssignmentsParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionRBACManage) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var assignments []domain.RBACAssignment
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		assignments, lerr = uow.Config().ListRBACAssignments(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list rbac assignments failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.RBACAssignmentListResponse{
		Items: rbacAssignmentResponses(assignments),
	})
}

// UpsertRBACAssignment implements api.ServerInterface.
func (s *Server) UpsertRBACAssignment(w http.ResponseWriter, r *http.Request) {
	principal, ok := s.authorizeLocalRBACPrincipal(w, r, domain.RBACPermissionRBACManage)
	if !ok {
		return
	}
	body, err := decodeRBACAssignmentWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	assignment, err := rbacAssignmentFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	assignment.CreatedBy = principal.Subject
	assignment.UpdatedBy = principal.Subject

	var saved domain.RBACAssignment
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var serr error
		saved, serr = uow.Config().UpsertRBACAssignment(ctx, assignment)
		return serr
	})
	writeRBACAssignmentMutationResult(r.Context(), w, s.logger, err, saved)
}

// AuthorizeRBAC implements api.ServerInterface.
func (s *Server) AuthorizeRBAC(w http.ResponseWriter, r *http.Request) {
	if s.rbacAuthorizer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "rbac authorizer is not configured", nil)
		return
	}
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionRBACManage) {
		return
	}
	body, err := decodeRBACAuthorizeRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	scopeKey := ""
	if body.ScopeKey != nil {
		scopeKey = *body.ScopeKey
	}
	decision, err := s.rbacAuthorizer.Authorize(r.Context(), rbacusecase.AuthorizeRequest{
		Principal: domain.RBACPrincipal{
			Subject:        body.Subject,
			DepartmentKeys: nonNilStringSlice(body.DepartmentKeys),
		},
		Permission: domain.RBACPermission(body.Permission),
		ScopeKind:  domain.RBACScopeKind(body.ScopeKind),
		ScopeKey:   scopeKey,
	})
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "rbac authorization failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.RBACAuthorizeResponse{
		Allowed:   decision.Allowed,
		CheckedAt: decision.CheckedAt,
	})
}

// AuthorizeCurrentRBAC implements api.ServerInterface.
func (s *Server) AuthorizeCurrentRBAC(w http.ResponseWriter, r *http.Request) {
	if s.rbacAuthorizer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "rbac authorizer is not configured", nil)
		return
	}
	if s.diagnosis.authProvider == nil && s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	body, err := decodeRBACCurrentAuthorizationRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	principal, err := s.authenticateLocalRBACPrincipal(r.Context(), r.Header)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	principal.Subject = subject
	principal, tenantCtx, err := s.bindAuthenticatedTenant(r.Context(), r.Header, principal)
	if err != nil {
		s.writeTenantBindingError(r.Context(), w, err)
		return
	}
	*r = *r.WithContext(tenantCtx)
	directoryUsers, err := s.localRBACDirectoryUsersBySubject(r.Context(), subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "resolve local rbac principal failed", err)
		return
	}
	departmentKeys := directoryUserDepartmentKeys(directoryUsers)

	response := api.RBACCurrentAuthorizationResponse{
		Subject:        subject,
		DepartmentKeys: nonNilStringSlice(departmentKeys),
		DirectoryUsers: directoryUserResponses(directoryUsers),
		Decisions:      make([]api.RBACCurrentAuthorizationDecision, 0, len(body.Requests)),
	}
	if s.localRBACBootstrapAdminSubject(subject) {
		checkedAt := domain.NormalizeUTCMicro(time.Now())
		bootstrapAssignment := domain.RBACAssignment{
			SubjectKind: domain.RBACSubjectKindUser,
			SubjectKey:  subject,
			Role:        domain.RBACRoleAdmin,
			ScopeKind:   domain.RBACScopeKindGlobal,
			Enabled:     true,
		}
		for _, check := range body.Requests {
			scopeKey := ""
			if check.ScopeKey != nil {
				scopeKey = *check.ScopeKey
			}
			allowed, err := domain.RBACAuthorize(
				domain.RBACPrincipal{Subject: subject},
				domain.RBACRequest{
					Permission: domain.RBACPermission(check.Permission),
					ScopeKind:  domain.RBACScopeKind(check.ScopeKind),
					ScopeKey:   scopeKey,
				},
				[]domain.RBACAssignment{bootstrapAssignment},
			)
			if errors.Is(err, domain.ErrInvariantViolation) {
				writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
				return
			}
			if err != nil {
				writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "current rbac authorization failed", err)
				return
			}
			response.Decisions = append(response.Decisions, api.RBACCurrentAuthorizationDecision{
				Permission: check.Permission,
				ScopeKind:  check.ScopeKind,
				ScopeKey:   scopeKey,
				Allowed:    allowed,
				CheckedAt:  checkedAt,
			})
		}
		writeJSON(r.Context(), w, s.logger, http.StatusOK, response)
		return
	}
	for _, check := range body.Requests {
		scopeKey := ""
		if check.ScopeKey != nil {
			scopeKey = *check.ScopeKey
		}
		ownerAllowed, err := s.diagnosisRoomOwnerAuthorizes(
			r.Context(),
			domain.RBACPrincipal{Subject: subject, DepartmentKeys: departmentKeys},
			domain.RBACPermission(check.Permission),
			domain.RBACScopeKind(check.ScopeKind),
			scopeKey,
		)
		if err != nil {
			writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "current rbac authorization failed", err)
			return
		}
		if ownerAllowed {
			response.Decisions = append(response.Decisions, api.RBACCurrentAuthorizationDecision{
				Permission: check.Permission,
				ScopeKind:  check.ScopeKind,
				ScopeKey:   scopeKey,
				Allowed:    true,
				CheckedAt:  domain.NormalizeUTCMicro(time.Now()),
			})
			continue
		}
		decision, err := s.rbacAuthorizer.Authorize(r.Context(), rbacusecase.AuthorizeRequest{
			Principal: domain.RBACPrincipal{
				Subject:        subject,
				DepartmentKeys: departmentKeys,
			},
			Permission: domain.RBACPermission(check.Permission),
			ScopeKind:  domain.RBACScopeKind(check.ScopeKind),
			ScopeKey:   scopeKey,
		})
		if errors.Is(err, domain.ErrInvariantViolation) {
			writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
			return
		}
		if err != nil {
			writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "current rbac authorization failed", err)
			return
		}
		response.Decisions = append(response.Decisions, api.RBACCurrentAuthorizationDecision{
			Permission: check.Permission,
			ScopeKind:  check.ScopeKind,
			ScopeKey:   scopeKey,
			Allowed:    decision.Allowed,
			CheckedAt:  decision.CheckedAt,
		})
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, response)
}

func decodeDirectorySyncRequest(w http.ResponseWriter, r *http.Request) (api.DirectorySyncRequest, error) {
	var body api.DirectorySyncRequest
	raw, err := readJSONRequestBody(w, r)
	if err != nil {
		return body, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		body.ApplyDefaults()
		return body, nil
	}
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return body, fmt.Errorf("invalid JSON request body: %w", err)
	}
	if len(body.AdditionalProperties) != 0 {
		return body, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func directorySyncRequest(body api.DirectorySyncRequest, provider string) directorysync.SyncRequest {
	req := directorysync.SyncRequest{
		Provider:     strings.TrimSpace(provider),
		UpdatedAfter: body.UpdatedAfter,
	}
	if body.PageSize != nil {
		req.PageSize = *body.PageSize
	}
	return req
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decodeRBACAssignmentWriteRequest(w http.ResponseWriter, r *http.Request) (api.RBACAssignmentWriteRequest, error) {
	var body api.RBACAssignmentWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
	}
	if len(body.AdditionalProperties) != 0 {
		return body, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func rbacAssignmentFromWriteRequest(body api.RBACAssignmentWriteRequest) (domain.RBACAssignment, error) {
	scopeKey := ""
	if body.ScopeKey != nil {
		scopeKey = *body.ScopeKey
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	return domain.NewRBACAssignment(
		domain.RBACSubjectKind(body.SubjectKind),
		body.SubjectKey,
		domain.RBACRole(body.Role),
		domain.RBACScopeKind(body.ScopeKind),
		scopeKey,
		enabled,
	)
}

func decodeRBACAuthorizeRequest(w http.ResponseWriter, r *http.Request) (api.RBACAuthorizeRequest, error) {
	var body api.RBACAuthorizeRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
	}
	if len(body.AdditionalProperties) != 0 {
		return body, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func decodeRBACCurrentAuthorizationRequest(w http.ResponseWriter, r *http.Request) (api.RBACCurrentAuthorizationRequest, error) {
	var body api.RBACCurrentAuthorizationRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
	}
	if len(body.AdditionalProperties) != 0 {
		return body, errors.New("request body contains unknown fields")
	}
	if len(body.Requests) == 0 {
		return body, errors.New("requests must be non-empty")
	}
	if len(body.Requests) > maxRBACCurrentAuthorizationRequests {
		return body, fmt.Errorf("requests must not exceed %d checks", maxRBACCurrentAuthorizationRequests)
	}
	for i := range body.Requests {
		if len(body.Requests[i].AdditionalProperties) != 0 {
			return body, fmt.Errorf("request %d contains unknown fields", i)
		}
		body.Requests[i].ApplyDefaults()
	}
	body.ApplyDefaults()
	return body, nil
}

func writeRBACAssignmentMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	assignment domain.RBACAssignment,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "rbac assignment already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(ctx, w, logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(ctx, w, logger, http.StatusInternalServerError, "upsert rbac assignment failed", err)
		return
	}
	writeJSON(ctx, w, logger, http.StatusOK, rbacAssignmentResponse(assignment))
}

func directoryDepartmentResponses(departments []domain.DirectoryDepartment) []api.DirectoryDepartment {
	out := make([]api.DirectoryDepartment, len(departments))
	for i, department := range departments {
		out[i] = directoryDepartmentResponse(department)
	}
	return out
}

func directoryDepartmentResponse(department domain.DirectoryDepartment) api.DirectoryDepartment {
	return api.DirectoryDepartment{
		ID:               int64(department.ID),
		Provider:         department.Provider,
		ExternalID:       department.ExternalID,
		ParentExternalID: department.ParentExternalID,
		Name:             department.Name,
		DisplayName:      department.DisplayName,
		Path:             department.Path,
		ParentPath:       department.ParentPath,
		Level:            department.Level,
		Source:           department.Source,
		MemberCount:      department.MemberCount,
		SourceUpdatedAt:  cloneTimePtr(department.SourceUpdatedAt),
		SyncedAt:         department.SyncedAt,
		CreatedAt:        department.CreatedAt,
		UpdatedAt:        department.UpdatedAt,
	}
}

func directoryUserResponses(users []domain.DirectoryUser) []api.DirectoryUser {
	out := make([]api.DirectoryUser, len(users))
	for i, user := range users {
		out[i] = directoryUserResponse(user)
	}
	return out
}

func directoryUserResponse(user domain.DirectoryUser) api.DirectoryUser {
	return api.DirectoryUser{
		ID:                    int64(user.ID),
		Provider:              user.Provider,
		Subject:               user.Subject,
		ExternalID:            user.ExternalID,
		Username:              user.Username,
		DisplayName:           user.DisplayName,
		Email:                 user.Email,
		JobTitle:              user.JobTitle,
		Department:            user.Department,
		Section:               user.Section,
		DepartmentPath:        user.DepartmentPath,
		DepartmentPaths:       nonNilStringSlice(user.DepartmentPaths),
		DepartmentExternalIds: nonNilStringSlice(user.DepartmentExternalIDs),
		Active:                user.Active,
		SourceUpdatedAt:       cloneTimePtr(user.SourceUpdatedAt),
		SyncedAt:              user.SyncedAt,
		CreatedAt:             user.CreatedAt,
		UpdatedAt:             user.UpdatedAt,
	}
}

func directorySyncRunResponses(runs []domain.DirectorySyncRun) []api.DirectorySyncRun {
	out := make([]api.DirectorySyncRun, len(runs))
	for i, run := range runs {
		out[i] = directorySyncRunResponse(run)
	}
	return out
}

func directorySyncRunResponse(run domain.DirectorySyncRun) api.DirectorySyncRun {
	return api.DirectorySyncRun{
		ID:                  int64(run.ID),
		Provider:            run.Provider,
		PageSize:            run.PageSize,
		UpdatedAfter:        cloneTimePtr(run.UpdatedAfter),
		Status:              string(run.Status),
		FailureCode:         run.FailureCode,
		FailureMessage:      run.FailureMessage,
		DepartmentPages:     run.DepartmentPages,
		UserPages:           run.UserPages,
		DepartmentsUpserted: run.DepartmentsUpserted,
		UsersUpserted:       run.UsersUpserted,
		SyncedAt:            run.SyncedAt,
		CreatedAt:           run.CreatedAt,
	}
}

func rbacAssignmentResponses(assignments []domain.RBACAssignment) []api.RBACAssignment {
	out := make([]api.RBACAssignment, len(assignments))
	for i, assignment := range assignments {
		out[i] = rbacAssignmentResponse(assignment)
	}
	return out
}

func rbacAssignmentResponse(assignment domain.RBACAssignment) api.RBACAssignment {
	return api.RBACAssignment{
		ID:          int64(assignment.ID),
		SubjectKind: api.RBACSubjectKind(assignment.SubjectKind),
		SubjectKey:  assignment.SubjectKey,
		Role:        api.RBACRole(assignment.Role),
		ScopeKind:   api.RBACScopeKind(assignment.ScopeKind),
		ScopeKey:    assignment.ScopeKey,
		Enabled:     assignment.Enabled,
		CreatedBy:   assignment.CreatedBy,
		UpdatedBy:   assignment.UpdatedBy,
		CreatedAt:   assignment.CreatedAt,
		UpdatedAt:   assignment.UpdatedAt,
	}
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
