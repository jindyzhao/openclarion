package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomclose"
)

// CloseUnavailableDiagnosisRoom implements api.ServerInterface.
func (s *Server) CloseUnavailableDiagnosisRoom(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.roomCloser == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis room recovery is not configured", nil)
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "session_id is required", nil)
		return
	}
	principal, ok := s.authorizeLocalRBACPrincipalForScope(
		w,
		r,
		domain.RBACPermissionDiagnosisRoomAdminister,
		domain.RBACScopeKindDiagnosisRoom,
		sessionID,
	)
	if !ok {
		return
	}
	body, err := decodeDiagnosisRoomCloseUnavailableRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	result, err := s.roomCloser.CloseUnavailable(r.Context(), diagnosisroomclose.Request{
		SessionID: sessionID,
		Principal: principal,
		Reason:    diagnosisRoomCloseReason(body.Reason),
		Now:       s.diagnosis.now(),
	})
	if err != nil {
		writeDiagnosisRoomCloseUnavailableError(r.Context(), w, s.logger, err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, diagnosisRoomSummary(domain.ChatSessionWithTask{
		Session: result.Session,
		Task:    result.Task,
	}))
}

func decodeDiagnosisRoomCloseUnavailableRequest(w http.ResponseWriter, r *http.Request) (api.DiagnosisRoomCloseUnavailableRequest, error) {
	var body api.DiagnosisRoomCloseUnavailableRequest
	raw, err := readJSONRequestBody(w, r)
	if err != nil {
		return body, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return body, nil
	}
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return body, fmt.Errorf("invalid JSON request body: %w", err)
	}
	if len(body.AdditionalProperties) != 0 {
		return body, fmt.Errorf("request body contains unknown fields")
	}
	return body, nil
}

func diagnosisRoomCloseReason(reason *string) string {
	if reason == nil {
		return ""
	}
	return *reason
}

func writeDiagnosisRoomCloseUnavailableError(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, diagnosisauth.ErrUnauthenticated):
		writeError(ctx, w, logger, http.StatusUnauthorized, "authentication failed", err)
	case errors.Is(err, diagnosisauth.ErrUnauthorized):
		writeError(ctx, w, logger, http.StatusForbidden, "unauthorized", err)
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, logger, http.StatusNotFound, "diagnosis room not found", err)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, logger, http.StatusBadRequest, err.Error(), nil)
	case errors.Is(err, domain.ErrPreconditionFailed):
		writeError(ctx, w, logger, http.StatusBadRequest, "diagnosis room workflow is still running", err)
	default:
		writeError(ctx, w, logger, http.StatusInternalServerError, "close unavailable diagnosis room failed", err)
	}
}
