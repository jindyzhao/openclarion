package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertsourcecheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// ListAlertSourceProfiles implements api.ServerInterface.
func (s *Server) ListAlertSourceProfiles(w http.ResponseWriter, r *http.Request, params api.ListAlertSourceProfilesParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var profiles []domain.AlertSourceProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		profiles, lerr = uow.Config().ListAlertSourceProfiles(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list alert source profiles failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.AlertSourceProfileListResponse{
		Items: alertSourceProfileResponses(profiles),
	})
}

// CreateAlertSourceProfile implements api.ServerInterface.
func (s *Server) CreateAlertSourceProfile(w http.ResponseWriter, r *http.Request) {
	body, err := decodeAlertSourceProfileWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile, err := alertSourceProfileFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var saved domain.AlertSourceProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var serr error
		saved, serr = uow.Config().SaveAlertSourceProfile(ctx, profile)
		return serr
	})
	writeAlertSourceProfileMutationResult(r.Context(), w, s.logger, err, "create alert source profile failed", saved, http.StatusCreated)
}

// GetAlertSourceProfile implements api.ServerInterface.
func (s *Server) GetAlertSourceProfile(w http.ResponseWriter, r *http.Request, sourceID int64) {
	if sourceID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "source_id must be positive", nil)
		return
	}

	var profile domain.AlertSourceProfile
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		profile, ferr = uow.Config().FindAlertSourceProfileByID(ctx, domain.AlertSourceProfileID(sourceID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "alert source profile not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get alert source profile failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, alertSourceProfileResponse(profile))
}

// ReplaceAlertSourceProfile implements api.ServerInterface.
func (s *Server) ReplaceAlertSourceProfile(w http.ResponseWriter, r *http.Request, sourceID int64) {
	if sourceID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "source_id must be positive", nil)
		return
	}
	body, err := decodeAlertSourceProfileWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile, err := alertSourceProfileFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile.ID = domain.AlertSourceProfileID(sourceID)

	var saved domain.AlertSourceProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var uerr error
		saved, uerr = uow.Config().UpdateAlertSourceProfile(ctx, profile)
		return uerr
	})
	writeAlertSourceProfileMutationResult(r.Context(), w, s.logger, err, "replace alert source profile failed", saved, http.StatusOK)
}

// TestAlertSourceProfileConnection implements api.ServerInterface.
func (s *Server) TestAlertSourceProfileConnection(w http.ResponseWriter, r *http.Request, sourceID int64) {
	if sourceID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "source_id must be positive", nil)
		return
	}
	if s.alertSourceTester == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "alert source connection tester is not configured", nil)
		return
	}

	var profile domain.AlertSourceProfile
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		profile, ferr = uow.Config().FindAlertSourceProfileByID(ctx, domain.AlertSourceProfileID(sourceID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "alert source profile not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get alert source profile failed", err)
		return
	}

	result, err := s.alertSourceTester.TestAlertSourceConnection(r.Context(), profile)
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "test alert source connection failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, alertSourceConnectionTestResponse(result))
}

func decodeAlertSourceProfileWriteRequest(w http.ResponseWriter, r *http.Request) (api.AlertSourceProfileWriteRequest, error) {
	var body api.AlertSourceProfileWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.AlertSourceProfileWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.AlertSourceProfileWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func alertSourceProfileFromWriteRequest(body api.AlertSourceProfileWriteRequest) (domain.AlertSourceProfile, error) {
	secretRef := ""
	if body.SecretRef != nil {
		secretRef = *body.SecretRef
	}
	enabled := false
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	labels := map[string]string{}
	if body.Labels != nil {
		labels = *body.Labels
	}
	return domain.NewAlertSourceProfile(
		body.Name,
		domain.AlertSourceKind(body.Kind),
		body.BaseURL,
		domain.AlertSourceAuthMode(body.AuthMode),
		secretRef,
		enabled,
		labels,
	)
}

func writeAlertSourceProfileMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	profile domain.AlertSourceProfile,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "alert source profile already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, "alert source profile not found", nil)
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
	writeJSON(ctx, w, logger, successStatus, alertSourceProfileResponse(profile))
}

func alertSourceProfileResponses(profiles []domain.AlertSourceProfile) []api.AlertSourceProfile {
	out := make([]api.AlertSourceProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = alertSourceProfileResponse(profile)
	}
	return out
}

func alertSourceProfileResponse(profile domain.AlertSourceProfile) api.AlertSourceProfile {
	labels := profile.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return api.AlertSourceProfile{
		ID:        int64(profile.ID),
		Name:      profile.Name,
		Kind:      api.AlertSourceKind(profile.Kind),
		BaseURL:   profile.BaseURL,
		AuthMode:  api.AlertSourceAuthMode(profile.AuthMode),
		SecretRef: profile.SecretRef,
		Enabled:   profile.Enabled,
		Labels:    labels,
		CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt,
	}
}

func alertSourceConnectionTestResponse(result alertsourcecheck.Result) api.AlertSourceConnectionTestResult {
	return api.AlertSourceConnectionTestResult{
		SourceID:       int64(result.SourceID),
		Kind:           api.AlertSourceKind(result.Kind),
		AuthMode:       api.AlertSourceAuthMode(result.AuthMode),
		Status:         api.AlertSourceConnectionTestStatus(result.Status),
		ReasonCode:     api.AlertSourceConnectionTestReasonCode(result.ReasonCode),
		Message:        result.Message,
		CheckedAt:      result.CheckedAt,
		ObservedAlerts: int64(result.ObservedAlerts),
	}
}
