package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosistooltemplate"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const maxDiagnosisToolTemplateAPILimit = 20

// ListDiagnosisToolTemplates implements api.ServerInterface.
func (s *Server) ListDiagnosisToolTemplates(w http.ResponseWriter, r *http.Request, params api.ListDiagnosisToolTemplatesParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDiagnosisToolTemplateRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var templates []domain.DiagnosisToolTemplate
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		templates, lerr = uow.Config().ListDiagnosisToolTemplates(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list diagnosis tool templates failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DiagnosisToolTemplateListResponse{
		Items: diagnosisToolTemplateResponses(templates),
	})
}

// CreateDiagnosisToolTemplate implements api.ServerInterface.
func (s *Server) CreateDiagnosisToolTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionDiagnosisToolTemplateManage) {
		return
	}
	body, err := decodeDiagnosisToolTemplateWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req := diagnosisToolTemplateWriteRequest(body)
	svc, err := s.newDiagnosisToolTemplateService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "create diagnosis tool template failed", err)
		return
	}

	saved, err := svc.Create(r.Context(), req)
	writeDiagnosisToolTemplateMutationResult(r.Context(), w, s.logger, err, "create diagnosis tool template failed", saved, http.StatusCreated)
}

// GetDiagnosisToolTemplate implements api.ServerInterface.
func (s *Server) GetDiagnosisToolTemplate(w http.ResponseWriter, r *http.Request, templateID int64) {
	if templateID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "template_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionDiagnosisToolTemplateRead, domain.RBACScopeKindDiagnosisToolTemplate, rbacResourceScopeKey(templateID)) {
		return
	}

	var template domain.DiagnosisToolTemplate
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		template, ferr = uow.Config().FindDiagnosisToolTemplateByID(ctx, domain.DiagnosisToolTemplateID(templateID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "diagnosis tool template not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get diagnosis tool template failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, diagnosisToolTemplateResponse(template))
}

// ReplaceDiagnosisToolTemplate implements api.ServerInterface.
func (s *Server) ReplaceDiagnosisToolTemplate(w http.ResponseWriter, r *http.Request, templateID int64) {
	if templateID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "template_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionDiagnosisToolTemplateManage, domain.RBACScopeKindDiagnosisToolTemplate, rbacResourceScopeKey(templateID)) {
		return
	}
	body, err := decodeDiagnosisToolTemplateWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	svc, err := s.newDiagnosisToolTemplateService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "replace diagnosis tool template failed", err)
		return
	}

	saved, err := svc.Replace(r.Context(), domain.DiagnosisToolTemplateID(templateID), diagnosisToolTemplateWriteRequest(body))
	writeDiagnosisToolTemplateMutationResult(r.Context(), w, s.logger, err, "replace diagnosis tool template failed", saved, http.StatusOK)
}

// EnableDiagnosisToolTemplate implements api.ServerInterface.
func (s *Server) EnableDiagnosisToolTemplate(w http.ResponseWriter, r *http.Request, templateID int64) {
	s.runDiagnosisToolTemplateAction(w, r, templateID, true)
}

// DisableDiagnosisToolTemplate implements api.ServerInterface.
func (s *Server) DisableDiagnosisToolTemplate(w http.ResponseWriter, r *http.Request, templateID int64) {
	s.runDiagnosisToolTemplateAction(w, r, templateID, false)
}

func (s *Server) runDiagnosisToolTemplateAction(w http.ResponseWriter, r *http.Request, templateID int64, enabled bool) {
	if templateID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "template_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionDiagnosisToolTemplateManage, domain.RBACScopeKindDiagnosisToolTemplate, rbacResourceScopeKey(templateID)) {
		return
	}
	svc, err := s.newDiagnosisToolTemplateService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "diagnosis tool template action failed", err)
		return
	}
	req := diagnosistooltemplate.ActionRequest{TemplateID: domain.DiagnosisToolTemplateID(templateID)}
	var template domain.DiagnosisToolTemplate
	if enabled {
		template, err = svc.Enable(r.Context(), req)
		writeDiagnosisToolTemplateMutationResult(r.Context(), w, s.logger, err, "enable diagnosis tool template failed", template, http.StatusOK)
		return
	}
	template, err = svc.Disable(r.Context(), req)
	writeDiagnosisToolTemplateMutationResult(r.Context(), w, s.logger, err, "disable diagnosis tool template failed", template, http.StatusOK)
}

func (s *Server) newDiagnosisToolTemplateService() (*diagnosistooltemplate.Service, error) {
	return diagnosistooltemplate.NewService(
		s.uowFactory,
		diagnosistooltemplate.WithClock(func() time.Time { return time.Now().UTC() }),
	)
}

func decodeDiagnosisToolTemplateWriteRequest(w http.ResponseWriter, r *http.Request) (api.DiagnosisToolTemplateWriteRequest, error) {
	var body api.DiagnosisToolTemplateWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.DiagnosisToolTemplateWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.DiagnosisToolTemplateWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func diagnosisToolTemplateWriteRequest(body api.DiagnosisToolTemplateWriteRequest) diagnosistooltemplate.WriteRequest {
	return diagnosistooltemplate.WriteRequest{
		Name:                 body.Name,
		AlertSourceProfileID: domain.AlertSourceProfileID(body.AlertSourceProfileID),
		Tool:                 domain.DiagnosisToolKind(body.Tool),
		QueryTemplate:        body.QueryTemplate,
		DefaultLimit:         int(body.DefaultLimit),
		DefaultWindow:        time.Duration(body.DefaultWindowSeconds) * time.Second,
		MaxWindow:            time.Duration(body.MaxWindowSeconds) * time.Second,
		DefaultStep:          time.Duration(body.DefaultStepSeconds) * time.Second,
	}
}

func writeDiagnosisToolTemplateMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	template domain.DiagnosisToolTemplate,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "diagnosis tool template already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, diagnosisToolTemplateNotFoundMessage(err), nil)
		return
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(ctx, w, logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(ctx, w, logger, http.StatusInternalServerError, fallback, err)
		return
	}
	writeJSON(ctx, w, logger, successStatus, diagnosisToolTemplateResponse(template))
}

func diagnosisToolTemplateNotFoundMessage(err error) string {
	message := err.Error()
	switch {
	case message == domain.ErrNotFound.Error():
		return "diagnosis tool template not found"
	case errors.Is(err, domain.ErrNotFound):
		return "diagnosis tool template binding not found"
	default:
		return "diagnosis tool template not found"
	}
}

func diagnosisToolTemplateResponses(templates []domain.DiagnosisToolTemplate) []api.DiagnosisToolTemplate {
	out := make([]api.DiagnosisToolTemplate, len(templates))
	for i, template := range templates {
		out[i] = diagnosisToolTemplateResponse(template)
	}
	return out
}

func diagnosisToolTemplateResponse(template domain.DiagnosisToolTemplate) api.DiagnosisToolTemplate {
	return api.DiagnosisToolTemplate{
		ID:                   int64(template.ID),
		Name:                 template.Name,
		AlertSourceProfileID: int64(template.AlertSourceProfileID),
		Tool:                 api.DiagnosisToolKind(template.Tool),
		QueryTemplate:        template.QueryTemplate,
		DefaultLimit:         diagnosisToolTemplateLimitResponse(template.DefaultLimit),
		DefaultWindowSeconds: int64(template.DefaultWindow / time.Second),
		MaxWindowSeconds:     int64(template.MaxWindow / time.Second),
		DefaultStepSeconds:   int64(template.DefaultStep / time.Second),
		Enabled:              template.Enabled,
		EnabledAt:            nullableTime(template.EnabledAt),
		DisabledAt:           nullableTime(template.DisabledAt),
		CreatedAt:            template.CreatedAt,
		UpdatedAt:            template.UpdatedAt,
	}
}

func diagnosisToolTemplateLimitResponse(limit int) int32 {
	if limit <= 0 {
		return 0
	}
	if limit > maxDiagnosisToolTemplateAPILimit {
		return maxDiagnosisToolTemplateAPILimit
	}
	return int32(limit)
}
