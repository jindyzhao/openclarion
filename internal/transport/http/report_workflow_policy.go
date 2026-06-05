package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportworkflowimpact"
	"github.com/openclarion/openclarion/internal/usecases/reportworkflowpolicy"
)

// ListReportWorkflowPolicies implements api.ServerInterface.
func (s *Server) ListReportWorkflowPolicies(w http.ResponseWriter, r *http.Request, params api.ListReportWorkflowPoliciesParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var policies []domain.ReportWorkflowPolicy
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		policies, lerr = uow.Config().ListReportWorkflowPolicies(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list report workflow policies failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.ReportWorkflowPolicyListResponse{
		Items: reportWorkflowPolicyResponses(policies),
	})
}

// CreateReportWorkflowPolicy implements api.ServerInterface.
func (s *Server) CreateReportWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	body, err := decodeReportWorkflowPolicyWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportWorkflowPolicyWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	svc, err := s.newReportWorkflowPolicyService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "create report workflow policy failed", err)
		return
	}

	saved, err := svc.Create(r.Context(), req)
	writeReportWorkflowPolicyMutationResult(r.Context(), w, s.logger, err, "create report workflow policy failed", saved, http.StatusCreated)
}

// GetReportWorkflowPolicy implements api.ServerInterface.
func (s *Server) GetReportWorkflowPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}

	var policy domain.ReportWorkflowPolicy
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		policy, ferr = uow.Config().FindReportWorkflowPolicyByID(ctx, domain.ReportWorkflowPolicyID(policyID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "report workflow policy not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get report workflow policy failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, reportWorkflowPolicyResponse(policy))
}

// ReplaceReportWorkflowPolicy implements api.ServerInterface.
func (s *Server) ReplaceReportWorkflowPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}
	body, err := decodeReportWorkflowPolicyWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	svc, err := s.newReportWorkflowPolicyService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "replace report workflow policy failed", err)
		return
	}

	req, err := reportWorkflowPolicyWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	saved, err := svc.Replace(r.Context(), domain.ReportWorkflowPolicyID(policyID), req)
	writeReportWorkflowPolicyMutationResult(r.Context(), w, s.logger, err, "replace report workflow policy failed", saved, http.StatusOK)
}

// EnableReportWorkflowPolicy implements api.ServerInterface.
func (s *Server) EnableReportWorkflowPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	s.runReportWorkflowPolicyAction(w, r, policyID, true)
}

// DisableReportWorkflowPolicy implements api.ServerInterface.
func (s *Server) DisableReportWorkflowPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	s.runReportWorkflowPolicyAction(w, r, policyID, false)
}

// PreviewReportWorkflowPolicyImpact implements api.ServerInterface.
func (s *Server) PreviewReportWorkflowPolicyImpact(
	w http.ResponseWriter,
	r *http.Request,
	policyID int64,
	params api.PreviewReportWorkflowPolicyImpactParams,
) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	svc, err := reportworkflowimpact.NewService(
		s.uowFactory,
		reportworkflowimpact.WithClock(func() time.Time { return time.Now().UTC() }),
	)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "preview report workflow policy impact failed", err)
		return
	}

	result, err := svc.Preview(r.Context(), reportworkflowimpact.Request{
		PolicyID: domain.ReportWorkflowPolicyID(policyID),
		Limit:    limit,
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, reportWorkflowPolicyNotFoundMessage(err), nil)
		return
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "preview report workflow policy impact failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, reportWorkflowImpactPreviewResponse(result))
}

// TriggerReportWorkflowPolicyReplay implements api.ServerInterface.
func (s *Server) TriggerReportWorkflowPolicyReplay(w http.ResponseWriter, r *http.Request, policyID int64) {
	if s.policyTrigger == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "report workflow policy replay trigger is not configured", nil)
		return
	}
	body, err := decodeReportWorkflowPolicyReplayRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportWorkflowPolicyReplayRequest(policyID, body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	result, err := s.policyTrigger.ReplayAndStart(r.Context(), req)
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, reportWorkflowPolicyNotFoundMessage(err), nil)
		return
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "trigger report workflow policy replay failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusAccepted, reportReplayTriggerResponse(result))
}

func (s *Server) runReportWorkflowPolicyAction(w http.ResponseWriter, r *http.Request, policyID int64, enabled bool) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}
	svc, err := s.newReportWorkflowPolicyService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "report workflow policy action failed", err)
		return
	}
	req := reportworkflowpolicy.ActionRequest{PolicyID: domain.ReportWorkflowPolicyID(policyID)}
	var policy domain.ReportWorkflowPolicy
	if enabled {
		policy, err = svc.Enable(r.Context(), req)
		writeReportWorkflowPolicyMutationResult(r.Context(), w, s.logger, err, "enable report workflow policy failed", policy, http.StatusOK)
		return
	}
	policy, err = svc.Disable(r.Context(), req)
	writeReportWorkflowPolicyMutationResult(r.Context(), w, s.logger, err, "disable report workflow policy failed", policy, http.StatusOK)
}

func (s *Server) newReportWorkflowPolicyService() (*reportworkflowpolicy.Service, error) {
	return reportworkflowpolicy.NewService(
		s.uowFactory,
		reportworkflowpolicy.WithClock(func() time.Time { return time.Now().UTC() }),
	)
}

func decodeReportWorkflowPolicyWriteRequest(w http.ResponseWriter, r *http.Request) (api.ReportWorkflowPolicyWriteRequest, error) {
	var body api.ReportWorkflowPolicyWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.ReportWorkflowPolicyWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.ReportWorkflowPolicyWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func decodeReportWorkflowPolicyReplayRequest(w http.ResponseWriter, r *http.Request) (api.ReportWorkflowPolicyReplayRequest, error) {
	var body api.ReportWorkflowPolicyReplayRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.ReportWorkflowPolicyReplayRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.ReportWorkflowPolicyReplayRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func reportWorkflowPolicyWriteRequest(body api.ReportWorkflowPolicyWriteRequest) (reportworkflowpolicy.WriteRequest, error) {
	reportNotificationChannelProfileID, err := optionalNotificationChannelProfileID(body.ReportNotificationChannelProfileID)
	if err != nil {
		return reportworkflowpolicy.WriteRequest{}, err
	}
	return reportworkflowpolicy.WriteRequest{
		Name:                               body.Name,
		AlertSourceProfileID:               domain.AlertSourceProfileID(body.AlertSourceProfileID),
		GroupingPolicyID:                   domain.GroupingPolicyID(body.GroupingPolicyID),
		ReportNotificationChannelProfileID: reportNotificationChannelProfileID,
		TriggerMode:                        domain.ReportWorkflowTriggerMode(derefString(body.TriggerMode)),
		ReportScenario:                     domain.ReportWorkflowScenario(derefString(body.ReportScenario)),
		DiagnosisFollowUp:                  domain.DiagnosisFollowUpMode(derefString(body.DiagnosisFollowUp)),
	}, nil
}

func reportWorkflowPolicyReplayRequest(policyID int64, body api.ReportWorkflowPolicyReplayRequest) (reportpolicytrigger.Request, error) {
	if policyID <= 0 {
		return reportpolicytrigger.Request{}, errors.New("policy_id must be positive")
	}
	limit := defaultReportReplayLimit
	if body.Limit != nil {
		limit = int(*body.Limit)
	}
	if limit < 1 || limit > maxReportReplayLimit {
		return reportpolicytrigger.Request{}, errors.New("limit must be between 1 and 100000")
	}
	return reportpolicytrigger.Request{
		PolicyID:       domain.ReportWorkflowPolicyID(policyID),
		WindowStart:    body.WindowStart,
		WindowEnd:      body.WindowEnd,
		Limit:          limit,
		CorrelationKey: derefString(body.CorrelationKey),
		WorkflowID:     derefString(body.WorkflowID),
	}, nil
}

func writeReportWorkflowPolicyMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	policy domain.ReportWorkflowPolicy,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "report workflow policy already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, reportWorkflowPolicyNotFoundMessage(err), nil)
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
	writeJSON(ctx, w, logger, successStatus, reportWorkflowPolicyResponse(policy))
}

func reportWorkflowPolicyNotFoundMessage(err error) string {
	message := err.Error()
	switch {
	case message == domain.ErrNotFound.Error():
		return "report workflow policy not found"
	case errors.Is(err, domain.ErrNotFound):
		return "report workflow policy binding not found"
	default:
		return "report workflow policy not found"
	}
}

func reportWorkflowPolicyResponses(policies []domain.ReportWorkflowPolicy) []api.ReportWorkflowPolicy {
	out := make([]api.ReportWorkflowPolicy, len(policies))
	for i, policy := range policies {
		out[i] = reportWorkflowPolicyResponse(policy)
	}
	return out
}

func reportWorkflowPolicyResponse(policy domain.ReportWorkflowPolicy) api.ReportWorkflowPolicy {
	return api.ReportWorkflowPolicy{
		ID:                                 int64(policy.ID),
		Name:                               policy.Name,
		AlertSourceProfileID:               int64(policy.AlertSourceProfileID),
		GroupingPolicyID:                   int64(policy.GroupingPolicyID),
		ReportNotificationChannelProfileID: nullableReportNotificationChannelProfileID(policy.ReportNotificationChannelProfileID),
		TriggerMode:                        api.ReportWorkflowTriggerMode(policy.TriggerMode),
		ReportScenario:                     api.ReportWorkflowScenario(policy.ReportScenario),
		DiagnosisFollowUp:                  api.DiagnosisFollowUpMode(policy.DiagnosisFollowUp),
		Enabled:                            policy.Enabled,
		EnabledAt:                          nullableTime(policy.EnabledAt),
		DisabledAt:                         nullableTime(policy.DisabledAt),
		CreatedAt:                          policy.CreatedAt,
		UpdatedAt:                          policy.UpdatedAt,
	}
}

func reportWorkflowImpactPreviewResponse(result reportworkflowimpact.Result) api.ReportWorkflowPolicyImpactPreviewResult {
	return api.ReportWorkflowPolicyImpactPreviewResult{
		PolicyID:                                int64(result.Policy.ID),
		Status:                                  reportWorkflowImpactPreviewStatus(result.Status),
		ReasonCodes:                             reportWorkflowImpactPreviewReasonCodes(result.ReasonCodes),
		Message:                                 result.Message,
		CheckedAt:                               result.CheckedAt,
		TriggerMode:                             api.ReportWorkflowTriggerMode(result.Policy.TriggerMode),
		ReportScenario:                          api.ReportWorkflowScenario(result.Policy.ReportScenario),
		DiagnosisFollowUp:                       api.DiagnosisFollowUpMode(result.Policy.DiagnosisFollowUp),
		AlertSourceProfileID:                    int64(result.AlertSourceID),
		AlertSourceKind:                         api.AlertSourceKind(result.AlertSourceKind),
		AlertSourceAuthMode:                     api.AlertSourceAuthMode(result.AlertSourceAuthMode),
		AlertSourceEnabled:                      result.AlertSourceEnabled,
		GroupingPolicyID:                        int64(result.GroupingPolicy.ID),
		GroupingPolicyEnabled:                   result.GroupingPolicy.Enabled,
		GroupingDimensionKeys:                   result.GroupingPolicy.DimensionKeys,
		GroupingSeverityKey:                     result.GroupingPolicy.SeverityKey,
		GroupingSourceFilter:                    result.GroupingPolicy.SourceFilter,
		ReportNotificationChannelProfileID:      nullableReportNotificationChannelProfileID(result.Policy.ReportNotificationChannelProfileID),
		ReportNotificationChannelBound:          result.ReportNotificationChannelBound,
		ReportNotificationChannelEnabled:        result.ReportNotificationChannelEnabled,
		ReportNotificationChannelHasReportScope: result.ReportNotificationChannelReportScope,
		EventsScanned:                           int64(result.EventsScanned),
		EventsMatched:                           int64(result.EventsMatched),
		GroupsEstimated:                         int64(len(result.Groups)),
		Groups:                                  groupingPolicyPreviewGroups(result.Groups),
	}
}

func reportWorkflowImpactPreviewStatus(status reportworkflowimpact.Status) api.ReportWorkflowPolicyImpactPreviewStatus {
	switch status {
	case reportworkflowimpact.StatusReady:
		return api.ReportWorkflowPolicyImpactPreviewStatusReady
	case reportworkflowimpact.StatusReview:
		return api.ReportWorkflowPolicyImpactPreviewStatusReview
	case reportworkflowimpact.StatusBlocked:
		return api.ReportWorkflowPolicyImpactPreviewStatusBlocked
	default:
		return api.ReportWorkflowPolicyImpactPreviewStatusReview
	}
}

func reportWorkflowImpactPreviewReasonCodes(reasonCodes []reportworkflowimpact.ReasonCode) []api.ReportWorkflowPolicyImpactPreviewReasonCode {
	out := make([]api.ReportWorkflowPolicyImpactPreviewReasonCode, len(reasonCodes))
	for i, reason := range reasonCodes {
		out[i] = reportWorkflowImpactPreviewReasonCode(reason)
	}
	return out
}

func reportWorkflowImpactPreviewReasonCode(reason reportworkflowimpact.ReasonCode) api.ReportWorkflowPolicyImpactPreviewReasonCode {
	switch reason {
	case reportworkflowimpact.ReasonOK:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeOk
	case reportworkflowimpact.ReasonAlertSourceDisabled:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeAlertSourceDisabled
	case reportworkflowimpact.ReasonGroupingPolicyDisabled:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeGroupingPolicyDisabled
	case reportworkflowimpact.ReasonNotificationChannelDisabled:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeNotificationChannelDisabled
	case reportworkflowimpact.ReasonNotificationChannelMissingReportScope:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeNotificationChannelMissingReportScope
	case reportworkflowimpact.ReasonUnsupportedTriggerMode:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeUnsupportedTriggerMode
	case reportworkflowimpact.ReasonNoMatchingEvents:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeNoMatchingEvents
	default:
		return api.ReportWorkflowPolicyImpactPreviewReasonCodeNoMatchingEvents
	}
}

func optionalNotificationChannelProfileID(value api.Nullable[int64]) (domain.NotificationChannelProfileID, error) {
	if !value.IsSpecified() || value.IsNull() {
		return 0, nil
	}
	id, err := value.Get()
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, errors.New("report_notification_channel_profile_id must be positive when provided")
	}
	return domain.NotificationChannelProfileID(id), nil
}

func nullableReportNotificationChannelProfileID(id domain.NotificationChannelProfileID) api.Nullable[int64] {
	var out api.Nullable[int64]
	if id == 0 {
		out.SetNull()
		return out
	}
	out.Set(int64(id))
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
