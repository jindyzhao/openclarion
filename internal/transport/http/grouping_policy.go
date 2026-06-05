package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/groupingpreview"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// ListGroupingPolicies implements api.ServerInterface.
func (s *Server) ListGroupingPolicies(w http.ResponseWriter, r *http.Request, params api.ListGroupingPoliciesParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var policies []domain.GroupingPolicy
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		policies, lerr = uow.Config().ListGroupingPolicies(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list grouping policies failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.GroupingPolicyListResponse{
		Items: groupingPolicyResponses(policies),
	})
}

// CreateGroupingPolicy implements api.ServerInterface.
func (s *Server) CreateGroupingPolicy(w http.ResponseWriter, r *http.Request) {
	body, err := decodeGroupingPolicyWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	policy, err := groupingPolicyFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var saved domain.GroupingPolicy
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var serr error
		saved, serr = uow.Config().SaveGroupingPolicy(ctx, policy)
		return serr
	})
	writeGroupingPolicyMutationResult(r.Context(), w, s.logger, err, "create grouping policy failed", saved, http.StatusCreated)
}

// GetGroupingPolicy implements api.ServerInterface.
func (s *Server) GetGroupingPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}

	var policy domain.GroupingPolicy
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		policy, ferr = uow.Config().FindGroupingPolicyByID(ctx, domain.GroupingPolicyID(policyID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "grouping policy not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get grouping policy failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, groupingPolicyResponse(policy))
}

// ReplaceGroupingPolicy implements api.ServerInterface.
func (s *Server) ReplaceGroupingPolicy(w http.ResponseWriter, r *http.Request, policyID int64) {
	if policyID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "policy_id must be positive", nil)
		return
	}
	body, err := decodeGroupingPolicyWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	policy, err := groupingPolicyFromWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	policy.ID = domain.GroupingPolicyID(policyID)

	var saved domain.GroupingPolicy
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var uerr error
		saved, uerr = uow.Config().UpdateGroupingPolicy(ctx, policy)
		return uerr
	})
	writeGroupingPolicyMutationResult(r.Context(), w, s.logger, err, "replace grouping policy failed", saved, http.StatusOK)
}

// PreviewGroupingPolicy implements api.ServerInterface.
func (s *Server) PreviewGroupingPolicy(
	w http.ResponseWriter,
	r *http.Request,
	policyID int64,
	params api.PreviewGroupingPolicyParams,
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

	result, err := groupingpreview.NewService(s.uowFactory).Preview(r.Context(), groupingpreview.Request{
		PolicyID: domain.GroupingPolicyID(policyID),
		Limit:    limit,
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "grouping policy not found", nil)
		return
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "preview grouping policy failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, groupingPolicyPreviewResponse(result))
}

func decodeGroupingPolicyWriteRequest(w http.ResponseWriter, r *http.Request) (api.GroupingPolicyWriteRequest, error) {
	var body api.GroupingPolicyWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.GroupingPolicyWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.GroupingPolicyWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func groupingPolicyFromWriteRequest(body api.GroupingPolicyWriteRequest) (domain.GroupingPolicy, error) {
	enabled := false
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	sourceFilter := []string{}
	if body.SourceFilter != nil {
		sourceFilter = body.SourceFilter
	}
	return domain.NewGroupingPolicy(
		body.Name,
		body.DimensionKeys,
		body.SeverityKey,
		sourceFilter,
		enabled,
	)
}

func writeGroupingPolicyMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	policy domain.GroupingPolicy,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "grouping policy already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, "grouping policy not found", nil)
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
	writeJSON(ctx, w, logger, successStatus, groupingPolicyResponse(policy))
}

func groupingPolicyResponses(policies []domain.GroupingPolicy) []api.GroupingPolicy {
	out := make([]api.GroupingPolicy, len(policies))
	for i, policy := range policies {
		out[i] = groupingPolicyResponse(policy)
	}
	return out
}

func groupingPolicyResponse(policy domain.GroupingPolicy) api.GroupingPolicy {
	return api.GroupingPolicy{
		ID:            int64(policy.ID),
		Name:          policy.Name,
		DimensionKeys: policy.DimensionKeys,
		SeverityKey:   policy.SeverityKey,
		SourceFilter:  policy.SourceFilter,
		Enabled:       policy.Enabled,
		CreatedAt:     policy.CreatedAt,
		UpdatedAt:     policy.UpdatedAt,
	}
}

func groupingPolicyPreviewResponse(result groupingpreview.Result) api.GroupingPolicyPreviewResult {
	return api.GroupingPolicyPreviewResult{
		PolicyID:      int64(result.Policy.ID),
		EventsScanned: int64(result.EventsScanned),
		EventsMatched: int64(result.EventsMatched),
		Groups:        groupingPolicyPreviewGroups(result.Groups),
	}
}

func groupingPolicyPreviewGroups(groups []groupingpreview.Group) []api.GroupingPolicyPreviewGroup {
	out := make([]api.GroupingPolicyPreviewGroup, len(groups))
	for i, group := range groups {
		out[i] = api.GroupingPolicyPreviewGroup{
			GroupKey:    group.GroupKey,
			Dimensions:  group.Dimensions,
			Severity:    groupingPolicyPreviewSeverity(group.Severity),
			EventCount:  int64(group.EventCount),
			FirstSeenAt: group.FirstSeenAt,
			LastSeenAt:  group.LastSeenAt,
			EventIds:    alertEventIDsToInt64(group.EventIDs),
		}
	}
	return out
}

func groupingPolicyPreviewSeverity(severity domain.GroupSeverity) api.GroupingPolicyPreviewSeverity {
	switch severity {
	case domain.GroupSeverityCritical:
		return api.GroupingPolicyPreviewSeverityCritical
	case domain.GroupSeverityWarning:
		return api.GroupingPolicyPreviewSeverityWarning
	case domain.GroupSeverityInfo:
		return api.GroupingPolicyPreviewSeverityInfo
	default:
		return api.GroupingPolicyPreviewSeverityUnknown
	}
}

func alertEventIDsToInt64(ids []domain.AlertEventID) []int64 {
	out := make([]int64, len(ids))
	for i, id := range ids {
		out[i] = int64(id)
	}
	return out
}
