package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
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
	return api.AlertmanagerWebhookIngestResponse{
		SourceID:        int64(result.ProfileID),
		Received:        int64(result.Received),
		SkippedResolved: int64(result.SkippedResolved),
		TruncatedAlerts: int64(result.TruncatedAlerts),
		Ingested: api.AlertmanagerWebhookIngestResponseIngested{
			Total:     int64(result.Ingested.Total),
			Saved:     int64(result.Ingested.Saved),
			Duplicate: int64(result.Ingested.Duplicate),
			Failed:    int64(result.Ingested.Failed),
		},
	}
}
