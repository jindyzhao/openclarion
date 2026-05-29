package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
)

// CreateDiagnosisRoom implements api.ServerInterface.
func (s *Server) CreateDiagnosisRoom(w http.ResponseWriter, r *http.Request) {
	if s.roomStarter == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis room starter is not configured", nil)
		return
	}
	if s.diagnosis.authProvider == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	body, err := decodeDiagnosisRoomCreateRequest(r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	bearer, err := authorizationBearerHeader(r.Header.Get("Authorization"))
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	principal, err := s.diagnosis.authProvider.AuthenticateBearer(r.Context(), bearer)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	result, err := s.roomStarter.Start(r.Context(), diagnosisroomstart.Request{
		EvidenceSnapshotID: domain.EvidenceSnapshotID(body.EvidenceSnapshotID),
		Principal:          principal,
	})
	if err != nil {
		writeDiagnosisRoomCreateError(r.Context(), w, s.logger, err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusCreated, diagnosisRoomCreateResponse(result))
}

func decodeDiagnosisRoomCreateRequest(r *http.Request) (api.DiagnosisRoomCreateRequest, error) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var body api.DiagnosisRoomCreateRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		return body, fmt.Errorf("invalid JSON request body: %w", err)
	}
	var extra struct{}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return body, fmt.Errorf("request body must contain exactly one JSON object")
	}
	if len(body.AdditionalProperties) != 0 {
		return body, fmt.Errorf("request body contains unknown fields")
	}
	if body.EvidenceSnapshotID <= 0 {
		return body, fmt.Errorf("evidence_snapshot_id must be positive")
	}
	return body, nil
}

func diagnosisRoomCreateResponse(result diagnosisroomstart.Result) api.DiagnosisRoomCreateResponse {
	return api.DiagnosisRoomCreateResponse{
		SessionID:          result.SessionID,
		EvidenceSnapshotID: int64(result.EvidenceSnapshotID),
		DiagnosisTaskID:    int64(result.DiagnosisTaskID),
		ChatSessionID:      int64(result.ChatSessionID),
		WorkflowID:         result.Workflow.WorkflowID,
		RunID:              result.Workflow.RunID,
	}
}

func writeDiagnosisRoomCreateError(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, diagnosisauth.ErrUnauthenticated):
		writeError(ctx, w, logger, http.StatusUnauthorized, "authentication failed", err)
	case errors.Is(err, diagnosisauth.ErrUnauthorized):
		writeError(ctx, w, logger, http.StatusForbidden, "unauthorized", err)
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, logger, http.StatusNotFound, "evidence snapshot not found", err)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, logger, http.StatusBadRequest, err.Error(), nil)
	default:
		writeError(ctx, w, logger, http.StatusInternalServerError, "create diagnosis room failed", err)
	}
}
