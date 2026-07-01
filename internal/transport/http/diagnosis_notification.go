package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"strings"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisnotification"
)

// RetryDiagnosisRoomNotification implements api.ServerInterface.
func (s *Server) RetryDiagnosisRoomNotification(w stdhttp.ResponseWriter, r *stdhttp.Request, sessionID string) {
	if s.roomNotifier == nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusServiceUnavailable, "diagnosis notification retry is not configured", nil)
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		writeError(r.Context(), w, s.logger, stdhttp.StatusBadRequest, "session_id is required", nil)
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
	body, err := decodeDiagnosisNotificationRetryRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusBadRequest, err.Error(), nil)
		return
	}
	result, err := s.roomNotifier.Retry(r.Context(), diagnosisnotification.Request{
		SessionID:  sessionID,
		EventKind:  body.EventKind,
		Principal:  principal,
		OccurredAt: s.diagnosis.now(),
	})
	if err != nil {
		writeDiagnosisNotificationRetryError(r.Context(), w, s.logger, err)
		return
	}
	notification, ok, err := notificationTimelineEntryFromDiagnosisEvent(result.Event)
	if err != nil {
		writeError(r.Context(), w, s.logger, stdhttp.StatusInternalServerError, "retry diagnosis room notification failed", err)
		return
	}
	if !ok {
		writeError(r.Context(), w, s.logger, stdhttp.StatusInternalServerError, "retry diagnosis room notification failed", fmt.Errorf("retry event has no notification status"))
		return
	}
	writeJSON(r.Context(), w, s.logger, stdhttp.StatusOK, api.DiagnosisNotificationRetryResponse{
		RetryState:   diagnosisNotificationRetryState(result.RetryState),
		Notification: notification,
	})
}

func decodeDiagnosisNotificationRetryRequest(w stdhttp.ResponseWriter, r *stdhttp.Request) (api.DiagnosisNotificationRetryRequest, error) {
	var body api.DiagnosisNotificationRetryRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
	}
	if len(body.AdditionalProperties) != 0 {
		return body, fmt.Errorf("request body contains unknown fields")
	}
	if body.EventKind == "" {
		return body, fmt.Errorf("event_kind is required")
	}
	return body, nil
}

func diagnosisNotificationRetryState(state diagnosisnotification.RetryState) api.DiagnosisNotificationRetryState {
	switch state {
	case diagnosisnotification.RetryStateAlreadyDelivered:
		return api.DiagnosisNotificationRetryStateAlreadyDelivered
	default:
		return api.DiagnosisNotificationRetryStateSent
	}
}

func writeDiagnosisNotificationRetryError(ctx context.Context, w stdhttp.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, diagnosisauth.ErrUnauthenticated):
		writeError(ctx, w, logger, stdhttp.StatusUnauthorized, "authentication failed", err)
	case errors.Is(err, diagnosisauth.ErrUnauthorized):
		writeError(ctx, w, logger, stdhttp.StatusForbidden, "unauthorized", err)
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, logger, stdhttp.StatusNotFound, "diagnosis room not found", err)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, logger, stdhttp.StatusBadRequest, err.Error(), nil)
	case errors.Is(err, domain.ErrPreconditionFailed):
		writeError(ctx, w, logger, stdhttp.StatusBadRequest, err.Error(), nil)
	default:
		writeError(ctx, w, logger, stdhttp.StatusInternalServerError, "retry diagnosis room notification failed", err)
	}
}
