package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// ListNotificationChannelProfiles implements api.ServerInterface.
func (s *Server) ListNotificationChannelProfiles(w http.ResponseWriter, r *http.Request, params api.ListNotificationChannelProfilesParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionNotificationChannelRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var profiles []domain.NotificationChannelProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		profiles, lerr = uow.Config().ListNotificationChannelProfiles(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list notification channel profiles failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.NotificationChannelProfileListResponse{
		Items: notificationChannelProfileResponses(profiles),
	})
}

// CreateNotificationChannelProfile implements api.ServerInterface.
func (s *Server) CreateNotificationChannelProfile(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionNotificationChannelManage) {
		return
	}
	body, err := decodeNotificationChannelProfileWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile, err := notificationChannelProfileFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var saved domain.NotificationChannelProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var serr error
		saved, serr = uow.Config().SaveNotificationChannelProfile(ctx, profile)
		return serr
	})
	writeNotificationChannelProfileMutationResult(r.Context(), w, s.logger, err, "create notification channel profile failed", saved, http.StatusCreated)
}

// GetNotificationChannelProfile implements api.ServerInterface.
func (s *Server) GetNotificationChannelProfile(w http.ResponseWriter, r *http.Request, channelID int64) {
	if channelID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "channel_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionNotificationChannelRead, domain.RBACScopeKindNotificationChannel, rbacResourceScopeKey(channelID)) {
		return
	}

	var profile domain.NotificationChannelProfile
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		profile, ferr = uow.Config().FindNotificationChannelProfileByID(ctx, domain.NotificationChannelProfileID(channelID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "notification channel profile not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get notification channel profile failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, notificationChannelProfileResponse(profile))
}

// ReplaceNotificationChannelProfile implements api.ServerInterface.
func (s *Server) ReplaceNotificationChannelProfile(w http.ResponseWriter, r *http.Request, channelID int64) {
	if channelID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "channel_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionNotificationChannelManage, domain.RBACScopeKindNotificationChannel, rbacResourceScopeKey(channelID)) {
		return
	}
	body, err := decodeNotificationChannelProfileWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile, err := notificationChannelProfileFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	profile.ID = domain.NotificationChannelProfileID(channelID)

	var saved domain.NotificationChannelProfile
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var uerr error
		saved, uerr = uow.Config().UpdateNotificationChannelProfile(ctx, profile)
		return uerr
	})
	writeNotificationChannelProfileMutationResult(r.Context(), w, s.logger, err, "replace notification channel profile failed", saved, http.StatusOK)
}

// TestNotificationChannelProfile implements api.ServerInterface.
func (s *Server) TestNotificationChannelProfile(w http.ResponseWriter, r *http.Request, channelID int64, params api.TestNotificationChannelProfileParams) {
	if channelID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "channel_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionNotificationChannelTest, domain.RBACScopeKindNotificationChannel, rbacResourceScopeKey(channelID)) {
		return
	}
	if s.channelTester == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "notification channel tester is not configured", nil)
		return
	}

	var profile domain.NotificationChannelProfile
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		profile, ferr = uow.Config().FindNotificationChannelProfileByID(ctx, domain.NotificationChannelProfileID(channelID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "notification channel profile not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get notification channel profile failed", err)
		return
	}

	req := notificationchannelcheck.Request{}
	if params.ContentKind != nil {
		req.ContentKind = *params.ContentKind
	}
	result, err := s.channelTester.TestNotificationChannel(r.Context(), profile, req)
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "test notification channel failed", err)
		return
	}
	proof, err := notificationChannelTestProofFromResult(result)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "notification channel test result is invalid", err)
		return
	}
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		_, serr := uow.Config().SaveNotificationChannelTestProof(ctx, proof)
		return serr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "record notification channel test proof failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, notificationChannelTestResponse(result))
}

func decodeNotificationChannelProfileWriteRequest(w http.ResponseWriter, r *http.Request) (api.NotificationChannelProfileWriteRequest, error) {
	var body api.NotificationChannelProfileWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.NotificationChannelProfileWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.NotificationChannelProfileWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func notificationChannelProfileFromWriteRequest(body api.NotificationChannelProfileWriteRequest) (domain.NotificationChannelProfile, error) {
	enabled := false
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	labels := map[string]string{}
	if body.Labels != nil {
		labels = *body.Labels
	}
	return domain.NewNotificationChannelProfile(
		body.Name,
		domain.NotificationChannelKind(body.Kind),
		body.SecretRef,
		notificationDeliveryScopesFromAPI(body.DeliveryScopes),
		enabled,
		labels,
	)
}

func writeNotificationChannelProfileMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	profile domain.NotificationChannelProfile,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "notification channel profile already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, "notification channel profile not found", nil)
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
	writeJSON(ctx, w, logger, successStatus, notificationChannelProfileResponse(profile))
}

func notificationChannelProfileResponses(profiles []domain.NotificationChannelProfile) []api.NotificationChannelProfile {
	out := make([]api.NotificationChannelProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = notificationChannelProfileResponse(profile)
	}
	return out
}

func notificationChannelProfileResponse(profile domain.NotificationChannelProfile) api.NotificationChannelProfile {
	labels := profile.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return api.NotificationChannelProfile{
		ID:                int64(profile.ID),
		Name:              profile.Name,
		Kind:              api.NotificationChannelKind(profile.Kind),
		SecretRef:         profile.SecretRef,
		DeliveryScopes:    notificationDeliveryScopesToAPI(profile.DeliveryScopes),
		Enabled:           profile.Enabled,
		Labels:            labels,
		LatestTestResults: notificationChannelTestProofResponses(profile.LatestTestProofs),
		CreatedAt:         profile.CreatedAt,
		UpdatedAt:         profile.UpdatedAt,
	}
}

func notificationChannelTestProofFromResult(result notificationchannelcheck.Result) (domain.NotificationChannelTestProof, error) {
	return domain.NewNotificationChannelTestProof(
		result.ChannelID,
		result.Kind,
		domain.NotificationChannelTestStatus(result.Status),
		domain.NotificationChannelTestReasonCode(result.ReasonCode),
		result.Message,
		domain.NotificationChannelTestContentKind(result.ContentKind),
		result.ContentSHA256,
		result.CheckedAt,
		result.ProviderMessageID,
		result.ProviderStatus,
	)
}

func notificationChannelTestProofResponses(proofs []domain.NotificationChannelTestProof) []api.NotificationChannelTestResult {
	out := make([]api.NotificationChannelTestResult, len(proofs))
	for i, proof := range proofs {
		out[i] = notificationChannelTestProofResponse(proof)
	}
	return out
}

func notificationChannelTestProofResponse(proof domain.NotificationChannelTestProof) api.NotificationChannelTestResult {
	return api.NotificationChannelTestResult{
		ChannelID:         int64(proof.NotificationChannelProfileID),
		Kind:              api.NotificationChannelKind(proof.Kind),
		Status:            api.NotificationChannelTestStatus(proof.Status),
		ReasonCode:        api.NotificationChannelTestReasonCode(proof.ReasonCode),
		Message:           proof.Message,
		ContentKind:       nonEmptyStringPtr(string(proof.ContentKind)),
		ContentSha256:     nonEmptyStringPtr(proof.ContentSHA256),
		CheckedAt:         proof.CheckedAt,
		ProviderMessageID: proof.ProviderMessageID,
		ProviderStatus:    proof.ProviderStatus,
	}
}

func notificationChannelTestResponse(result notificationchannelcheck.Result) api.NotificationChannelTestResult {
	return api.NotificationChannelTestResult{
		ChannelID:         int64(result.ChannelID),
		Kind:              api.NotificationChannelKind(result.Kind),
		Status:            api.NotificationChannelTestStatus(result.Status),
		ReasonCode:        api.NotificationChannelTestReasonCode(result.ReasonCode),
		Message:           result.Message,
		ContentKind:       nonEmptyStringPtr(result.ContentKind),
		ContentSha256:     nonEmptyStringPtr(result.ContentSHA256),
		CheckedAt:         result.CheckedAt,
		ProviderMessageID: result.ProviderMessageID,
		ProviderStatus:    result.ProviderStatus,
	}
}

func notificationDeliveryScopesFromAPI(scopes []api.NotificationDeliveryScope) []domain.NotificationDeliveryScope {
	out := make([]domain.NotificationDeliveryScope, len(scopes))
	for i, scope := range scopes {
		out[i] = domain.NotificationDeliveryScope(scope)
	}
	return out
}

func notificationDeliveryScopesToAPI(scopes []domain.NotificationDeliveryScope) []api.NotificationDeliveryScope {
	out := make([]api.NotificationDeliveryScope, len(scopes))
	for i, scope := range scopes {
		out[i] = api.NotificationDeliveryScope(scope)
	}
	return out
}
