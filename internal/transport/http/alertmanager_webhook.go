package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertdiagnosis"
	"github.com/openclarion/openclarion/internal/usecases/alertmanagerwebhook"
)

// IngestAlertmanagerWebhook implements api.ServerInterface.
func (s *Server) IngestAlertmanagerWebhook(w http.ResponseWriter, r *http.Request, sourceID int64) {
	if s.webhookIngestor == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "alertmanager webhook ingest is not configured", nil)
		return
	}
	if sourceID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "source_id must be positive", nil)
		return
	}
	raw, err := readJSONRequestBody(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	result, err := s.webhookIngestor.Ingest(r.Context(), alertmanagerwebhook.Request{
		ProfileID:     domain.AlertSourceProfileID(sourceID),
		Authorization: r.Header.Get("Authorization"),
		Body:          append(json.RawMessage(nil), raw...),
	})
	switch {
	case errors.Is(err, alertmanagerwebhook.ErrUnauthorized):
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "alertmanager webhook authorization failed", nil)
		return
	case errors.Is(err, domain.ErrNotFound):
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "alert source profile not found", nil)
		return
	case errors.Is(err, alertmanagerwebhook.ErrSecretResolverUnavailable),
		errors.Is(err, alertmanagerwebhook.ErrSecretNotFound),
		errors.Is(err, alertmanagerwebhook.ErrSecretResolveFailed):
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "alertmanager webhook authorization is not configured", err)
		return
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	case err != nil:
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "ingest alertmanager webhook failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusAccepted, alertmanagerWebhookIngestResponse(result))
}

func alertmanagerWebhookIngestResponse(result alertmanagerwebhook.Result) api.AlertmanagerWebhookIngestResponse {
	response := api.AlertmanagerWebhookIngestResponse{
		SourceID:          int64(result.ProfileID),
		Received:          int64(result.Received),
		Resolved:          int64(result.Resolved),
		SkippedResolved:   int64(result.SkippedResolved),
		SkippedSuppressed: int64(result.SkippedSuppressed),
		TruncatedAlerts:   int64(result.TruncatedAlerts),
		Ingested: api.AlertmanagerWebhookIngestResponseIngested{
			Total:     int64(result.Ingested.Total),
			Saved:     int64(result.Ingested.Saved),
			Duplicate: int64(result.Ingested.Duplicate),
			Failed:    int64(result.Ingested.Failed),
		},
	}
	if result.AutoDiagnosis != nil {
		autoDiagnosis := alertmanagerWebhookAutoDiagnosisSummary(*result.AutoDiagnosis)
		response.AutoDiagnosis = &autoDiagnosis
	}
	return response
}

func alertmanagerWebhookAutoDiagnosisSummary(result alertdiagnosis.Result) api.AlertmanagerWebhookAutoDiagnosisSummary {
	rooms := make([]api.AlertmanagerWebhookAutoDiagnosisRoom, 0, len(result.Rooms))
	for _, room := range result.Rooms {
		rooms = append(rooms, api.AlertmanagerWebhookAutoDiagnosisRoom{
			PolicyID:           int64(room.PolicyID),
			EvidenceSnapshotID: int64(room.EvidenceSnapshotID),
			SessionID:          room.SessionID,
			InitialMessageID:   room.InitialMessageID,
			WorkflowID:         room.Workflow.WorkflowID,
			RunID:              room.Workflow.RunID,
		})
	}
	skippedSnapshotIDs := make([]int64, 0, len(result.SkippedSnapshots))
	for _, snapshot := range result.SkippedSnapshots {
		skippedSnapshotIDs = append(skippedSnapshotIDs, int64(snapshot.ID))
	}
	return api.AlertmanagerWebhookAutoDiagnosisSummary{
		PoliciesMatched:    int64(result.PoliciesMatched),
		Snapshots:          int64(len(result.Snapshots)),
		RoomsStarted:       int64(len(result.Rooms)),
		RoomsSkipped:       int64(len(result.SkippedSnapshots)),
		SkippedSnapshotIds: skippedSnapshotIDs,
		Rooms:              rooms,
	}
}
