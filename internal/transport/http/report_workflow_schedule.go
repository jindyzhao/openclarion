package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportworkflowschedule"
)

// ListReportWorkflowSchedules implements api.ServerInterface.
func (s *Server) ListReportWorkflowSchedules(w http.ResponseWriter, r *http.Request, params api.ListReportWorkflowSchedulesParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionReportWorkflowRead) {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var schedules []domain.ReportWorkflowSchedule
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		schedules, lerr = uow.Config().ListReportWorkflowSchedules(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list report workflow schedules failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.ReportWorkflowScheduleListResponse{
		Items: reportWorkflowScheduleResponses(schedules),
	})
}

// CreateReportWorkflowSchedule implements api.ServerInterface.
func (s *Server) CreateReportWorkflowSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionReportWorkflowManage) {
		return
	}
	body, err := decodeReportWorkflowScheduleWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportWorkflowScheduleWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	svc, err := s.newReportWorkflowScheduleService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "create report workflow schedule failed", err)
		return
	}

	saved, err := svc.Create(r.Context(), req)
	if err == nil {
		err = s.syncReportWorkflowSchedule(r.Context(), saved)
	}
	writeReportWorkflowScheduleMutationResult(r.Context(), w, s.logger, err, "create report workflow schedule failed", saved, http.StatusCreated)
}

// GetReportWorkflowSchedule implements api.ServerInterface.
func (s *Server) GetReportWorkflowSchedule(w http.ResponseWriter, r *http.Request, scheduleID int64) {
	if scheduleID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "schedule_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionReportWorkflowRead, domain.RBACScopeKindReportWorkflowSchedule, rbacResourceScopeKey(scheduleID)) {
		return
	}

	var schedule domain.ReportWorkflowSchedule
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		schedule, ferr = uow.Config().FindReportWorkflowScheduleByID(ctx, domain.ReportWorkflowScheduleID(scheduleID))
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "report workflow schedule not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get report workflow schedule failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, reportWorkflowScheduleResponse(schedule))
}

// ReplaceReportWorkflowSchedule implements api.ServerInterface.
func (s *Server) ReplaceReportWorkflowSchedule(w http.ResponseWriter, r *http.Request, scheduleID int64) {
	if scheduleID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "schedule_id must be positive", nil)
		return
	}
	_, rbacPrincipal, ok := s.authorizeLocalRBACPrincipalsForScope(
		w,
		r,
		domain.RBACPermissionReportWorkflowManage,
		domain.RBACScopeKindReportWorkflowSchedule,
		rbacResourceScopeKey(scheduleID),
	)
	if !ok {
		return
	}
	body, err := decodeReportWorkflowScheduleWriteRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportWorkflowScheduleWriteRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !s.authorizeReportWorkflowScheduleBinding(w, r, rbacPrincipal, req) {
		return
	}
	svc, err := s.newReportWorkflowScheduleService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "replace report workflow schedule failed", err)
		return
	}

	saved, err := svc.Replace(r.Context(), domain.ReportWorkflowScheduleID(scheduleID), req)
	if err == nil {
		err = s.syncReportWorkflowSchedule(r.Context(), saved)
	}
	writeReportWorkflowScheduleMutationResult(r.Context(), w, s.logger, err, "replace report workflow schedule failed", saved, http.StatusOK)
}

// EnableReportWorkflowSchedule implements api.ServerInterface.
func (s *Server) EnableReportWorkflowSchedule(w http.ResponseWriter, r *http.Request, scheduleID int64) {
	s.runReportWorkflowScheduleAction(w, r, scheduleID, true)
}

// DisableReportWorkflowSchedule implements api.ServerInterface.
func (s *Server) DisableReportWorkflowSchedule(w http.ResponseWriter, r *http.Request, scheduleID int64) {
	s.runReportWorkflowScheduleAction(w, r, scheduleID, false)
}

func (s *Server) runReportWorkflowScheduleAction(w http.ResponseWriter, r *http.Request, scheduleID int64, enabled bool) {
	if scheduleID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "schedule_id must be positive", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionReportWorkflowManage, domain.RBACScopeKindReportWorkflowSchedule, rbacResourceScopeKey(scheduleID)) {
		return
	}
	svc, err := s.newReportWorkflowScheduleService()
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "report workflow schedule action failed", err)
		return
	}
	req := reportworkflowschedule.ActionRequest{ScheduleID: domain.ReportWorkflowScheduleID(scheduleID)}
	var schedule domain.ReportWorkflowSchedule
	if enabled {
		schedule, err = svc.Enable(r.Context(), req)
		if err == nil {
			err = s.syncReportWorkflowSchedule(r.Context(), schedule)
		}
		writeReportWorkflowScheduleMutationResult(r.Context(), w, s.logger, err, "enable report workflow schedule failed", schedule, http.StatusOK)
		return
	}
	schedule, err = svc.Disable(r.Context(), req)
	if err == nil {
		err = s.syncReportWorkflowSchedule(r.Context(), schedule)
	}
	writeReportWorkflowScheduleMutationResult(r.Context(), w, s.logger, err, "disable report workflow schedule failed", schedule, http.StatusOK)
}

func (s *Server) syncReportWorkflowSchedule(ctx context.Context, schedule domain.ReportWorkflowSchedule) error {
	if s.scheduleSyncer == nil {
		return nil
	}
	if err := s.scheduleSyncer.SyncReportWorkflowSchedule(ctx, schedule); err != nil {
		return fmt.Errorf("synchronize report workflow schedule: %w", err)
	}
	return nil
}

func (s *Server) newReportWorkflowScheduleService() (*reportworkflowschedule.Service, error) {
	return reportworkflowschedule.NewService(
		s.uowFactory,
		reportworkflowschedule.WithClock(func() time.Time { return time.Now().UTC() }),
	)
}

func decodeReportWorkflowScheduleWriteRequest(w http.ResponseWriter, r *http.Request) (api.ReportWorkflowScheduleWriteRequest, error) {
	var body api.ReportWorkflowScheduleWriteRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return api.ReportWorkflowScheduleWriteRequest{}, err
	}
	if len(body.AdditionalProperties) != 0 {
		return api.ReportWorkflowScheduleWriteRequest{}, errors.New("request body contains unknown fields")
	}
	body.ApplyDefaults()
	return body, nil
}

func reportWorkflowScheduleWriteRequest(body api.ReportWorkflowScheduleWriteRequest) (reportworkflowschedule.WriteRequest, error) {
	if body.ReportWorkflowPolicyID <= 0 {
		return reportworkflowschedule.WriteRequest{}, errors.New("report_workflow_policy_id must be positive")
	}
	interval, err := durationSeconds("interval_seconds", body.IntervalSeconds)
	if err != nil {
		return reportworkflowschedule.WriteRequest{}, err
	}
	offset, err := nonNegativeDurationSeconds("offset_seconds", body.OffsetSeconds)
	if err != nil {
		return reportworkflowschedule.WriteRequest{}, err
	}
	replayWindow, err := durationSeconds("replay_window_seconds", body.ReplayWindowSeconds)
	if err != nil {
		return reportworkflowschedule.WriteRequest{}, err
	}
	replayDelay, err := nonNegativeDurationSeconds("replay_delay_seconds", body.ReplayDelaySeconds)
	if err != nil {
		return reportworkflowschedule.WriteRequest{}, err
	}
	catchupWindow, err := durationSeconds("catchup_window_seconds", body.CatchupWindowSeconds)
	if err != nil {
		return reportworkflowschedule.WriteRequest{}, err
	}
	if body.ReplayLimit <= 0 {
		return reportworkflowschedule.WriteRequest{}, errors.New("replay_limit must be positive")
	}
	if body.ReplayLimit > maxReportWorkflowScheduleReplayLimit {
		return reportworkflowschedule.WriteRequest{}, errors.New("replay_limit is too large")
	}
	return reportworkflowschedule.WriteRequest{
		Name:                   body.Name,
		ReportWorkflowPolicyID: domain.ReportWorkflowPolicyID(body.ReportWorkflowPolicyID),
		TemporalScheduleID:     body.TemporalScheduleID,
		Interval:               interval,
		Offset:                 offset,
		ReplayWindow:           replayWindow,
		ReplayDelay:            replayDelay,
		ReplayLimit:            int(body.ReplayLimit),
		CatchupWindow:          catchupWindow,
	}, nil
}

func (s *Server) authorizeReportWorkflowScheduleBinding(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	req reportworkflowschedule.WriteRequest,
) bool {
	allowed, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		principal,
		domain.RBACPermissionReportWorkflowManage,
		domain.RBACScopeKindReportWorkflow,
		rbacResourceScopeKey(int64(req.ReportWorkflowPolicyID)),
		true,
	)
	return ok && allowed
}

func durationSeconds(field string, seconds int64) (time.Duration, error) {
	if seconds <= 0 {
		return 0, errors.New(field + " must be positive")
	}
	return nonNegativeDurationSeconds(field, seconds)
}

func nonNegativeDurationSeconds(field string, seconds int64) (time.Duration, error) {
	if seconds < 0 {
		return 0, errors.New(field + " must be non-negative")
	}
	if seconds > maxReportWorkflowScheduleDurationSeconds {
		return 0, errors.New(field + " is too large")
	}
	return time.Duration(seconds) * time.Second, nil
}

const (
	maxReportWorkflowScheduleDurationSeconds int64 = 31536000
	maxReportWorkflowScheduleReplayLimit           = 100000
)

func writeReportWorkflowScheduleMutationResult(
	ctx context.Context,
	w http.ResponseWriter,
	logger *slog.Logger,
	err error,
	fallback string,
	schedule domain.ReportWorkflowSchedule,
	successStatus int,
) {
	if errors.Is(err, domain.ErrAlreadyExists) {
		writeError(ctx, w, logger, http.StatusConflict, "report workflow schedule already exists", nil)
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		writeError(ctx, w, logger, http.StatusNotFound, reportWorkflowScheduleNotFoundMessage(err), nil)
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
	writeJSON(ctx, w, logger, successStatus, reportWorkflowScheduleResponse(schedule))
}

func reportWorkflowScheduleNotFoundMessage(err error) string {
	message := err.Error()
	switch {
	case message == domain.ErrNotFound.Error():
		return "report workflow schedule not found"
	case errors.Is(err, domain.ErrNotFound):
		return "report workflow schedule binding not found"
	default:
		return "report workflow schedule not found"
	}
}

func reportWorkflowScheduleResponses(schedules []domain.ReportWorkflowSchedule) []api.ReportWorkflowSchedule {
	out := make([]api.ReportWorkflowSchedule, len(schedules))
	for i, schedule := range schedules {
		out[i] = reportWorkflowScheduleResponse(schedule)
	}
	return out
}

func reportWorkflowScheduleResponse(schedule domain.ReportWorkflowSchedule) api.ReportWorkflowSchedule {
	return api.ReportWorkflowSchedule{
		ID:                     int64(schedule.ID),
		Name:                   schedule.Name,
		ReportWorkflowPolicyID: int64(schedule.ReportWorkflowPolicyID),
		TemporalScheduleID:     schedule.TemporalScheduleID,
		IntervalSeconds:        durationToSeconds(schedule.Interval),
		OffsetSeconds:          durationToSeconds(schedule.Offset),
		ReplayWindowSeconds:    durationToSeconds(schedule.ReplayWindow),
		ReplayDelaySeconds:     durationToSeconds(schedule.ReplayDelay),
		ReplayLimit:            reportWorkflowScheduleReplayLimitResponse(schedule.ReplayLimit),
		CatchupWindowSeconds:   durationToSeconds(schedule.CatchupWindow),
		Enabled:                schedule.Enabled,
		EnabledAt:              nullableTime(schedule.EnabledAt),
		DisabledAt:             nullableTime(schedule.DisabledAt),
		CreatedAt:              schedule.CreatedAt,
		UpdatedAt:              schedule.UpdatedAt,
	}
}

func durationToSeconds(duration time.Duration) int64 {
	return int64(duration / time.Second)
}

func reportWorkflowScheduleReplayLimitResponse(limit int) int32 {
	if limit <= 0 {
		return 0
	}
	if limit > maxReportWorkflowScheduleReplayLimit {
		return int32(maxReportWorkflowScheduleReplayLimit)
	}
	return int32(limit)
}
