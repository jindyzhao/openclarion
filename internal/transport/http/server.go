// Package http implements the transport layer for OpenClarion's HTTP API.
// Handlers satisfy the generated ServerInterface from api/openapi.gen.go.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500

	defaultReportReplayLimit = 10000
	maxReportReplayLimit     = 100000

	reportTriggerHTTPWorkflow = "ReportTriggerHTTP"
)

// Server implements api.ServerInterface.
//
// uowFactory is held for use by future ingestion / diagnosis
// endpoints (M1-PR3 onward). The current /healthz handler does not
// touch the database; intentionally not used yet so that the DI
// graph is stable before workflow code lands.
type Server struct {
	logger        *slog.Logger
	uowFactory    ports.UnitOfWorkFactory
	reportTrigger ReportReplayTrigger
	roomStarter   DiagnosisRoomStarter
	diagnosis     diagnosisConfig
}

// ReportReplayTrigger is the transport-facing report trigger usecase.
type ReportReplayTrigger interface {
	ReplayAndStart(ctx context.Context, req reporttrigger.Request) (reporttrigger.Result, error)
}

// DiagnosisRoomStarter is the transport-facing room creation usecase.
type DiagnosisRoomStarter interface {
	Start(ctx context.Context, req diagnosisroomstart.Request) (diagnosisroomstart.Result, error)
}

// ServerOption customises optional HTTP handlers.
type ServerOption func(*Server)

// WithReportReplayTrigger enables POST /api/v1/report-triggers/replay-window.
func WithReportReplayTrigger(trigger ReportReplayTrigger) ServerOption {
	return func(s *Server) {
		s.reportTrigger = trigger
	}
}

// WithDiagnosisRoomStarter enables POST /api/v1/diagnosis/rooms.
func WithDiagnosisRoomStarter(starter DiagnosisRoomStarter) ServerOption {
	return func(s *Server) {
		s.roomStarter = starter
	}
}

// NewServer creates a new Server with the given dependencies. The
// UnitOfWorkFactory MUST be non-nil; transports that legitimately do
// not need persistence (e.g. trivial smoke binaries) should construct
// their own narrower struct rather than pass nil here.
func NewServer(logger *slog.Logger, uowFactory ports.UnitOfWorkFactory, opts ...ServerOption) *Server {
	server := &Server{logger: logger, uowFactory: uowFactory, diagnosis: newDiagnosisConfig()}
	for _, opt := range opts {
		if opt != nil {
			opt(server)
		}
	}
	return server
}

// GetHealthz implements api.ServerInterface.
func (s *Server) GetHealthz(w http.ResponseWriter, r *http.Request) {
	resp := api.HealthResponse{
		Status: "ok",
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, resp)
}

// GetDashboard implements api.ServerInterface.
func (s *Server) GetDashboard(w http.ResponseWriter, r *http.Request) {
	var events []domain.AlertEvent
	var reports []domain.FinalReport
	latestDeliveries := map[domain.FinalReportID][]domain.ReportNotificationDelivery{}
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		events, lerr = uow.Alerts().ListEvents(ctx, defaultListLimit)
		if lerr != nil {
			return lerr
		}
		repo := uow.Reports()
		reports, lerr = repo.ListFinalReports(ctx, defaultListLimit)
		if lerr != nil {
			return lerr
		}
		for _, report := range reports {
			deliveries, derr := repo.ListNotificationDeliveriesByFinalReport(ctx, report.ID, 1)
			if derr != nil {
				return derr
			}
			latestDeliveries[report.ID] = deliveries
		}
		return nil
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "dashboard summary failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, dashboardSummary(time.Now().UTC(), events, reports, latestDeliveries))
}

// ListAlerts implements api.ServerInterface.
func (s *Server) ListAlerts(w http.ResponseWriter, r *http.Request, params api.ListAlertsParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var events []domain.AlertEvent
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		events, lerr = uow.Alerts().ListEvents(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list alerts failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.AlertListResponse{
		Items: alertEventSummaries(events),
	})
}

// ListEvidenceSnapshots implements api.ServerInterface.
func (s *Server) ListEvidenceSnapshots(w http.ResponseWriter, r *http.Request, params api.ListEvidenceSnapshotsParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var snapshots []domain.EvidenceSnapshot
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		snapshots, lerr = uow.Evidence().List(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list evidence snapshots failed", err)
		return
	}

	items, err := evidenceSnapshotSummaries(snapshots)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "map evidence snapshots failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.EvidenceSnapshotListResponse{
		Items: items,
	})
}

// ListReports implements api.ServerInterface.
func (s *Server) ListReports(w http.ResponseWriter, r *http.Request, params api.ListReportsParams) {
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var reports []domain.FinalReport
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		reports, lerr = uow.Reports().ListFinalReports(ctx, limit)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list reports failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.ReportListResponse{
		Items: finalReportSummaries(reports),
	})
}

// TriggerReportReplay implements api.ServerInterface.
func (s *Server) TriggerReportReplay(w http.ResponseWriter, r *http.Request) {
	if s.reportTrigger == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "report trigger is not configured", nil)
		return
	}

	body, err := decodeReportReplayTriggerRequest(r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	req, err := reportReplayTriggerRequest(body)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	result, err := s.reportTrigger.ReplayAndStart(r.Context(), req)
	if errors.Is(err, domain.ErrInvariantViolation) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "trigger report replay failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusAccepted, reportReplayTriggerResponse(result))
}

// GetReport implements api.ServerInterface.
func (s *Server) GetReport(w http.ResponseWriter, r *http.Request, reportID int64) {
	if reportID <= 0 {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "report_id must be positive", nil)
		return
	}

	var report domain.FinalReport
	var subReports []domain.SubReport
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		repo := uow.Reports()
		report, ferr = repo.FindFinalReportByID(ctx, domain.FinalReportID(reportID))
		if ferr != nil {
			return ferr
		}
		subReports, ferr = repo.ListSubReportsForFinalReport(ctx, report.ID, maxListLimit)
		return ferr
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "report not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get report failed", err)
		return
	}

	detail, err := finalReportDetail(report, subReports)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "map report detail failed", err)
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusOK, detail)
}

// OpenAPIErrorHandler returns the generated router's parameter
// binding errors as the API's JSON error shape.
func OpenAPIErrorHandler(logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		writeError(r.Context(), w, logger, http.StatusBadRequest, err.Error(), nil)
	}
}

// Compile-time check that Server implements ServerInterface.
var _ api.ServerInterface = (*Server)(nil)

func parseListLimit(value *int32) (int, error) {
	if value == nil {
		return defaultListLimit, nil
	}
	limit := int(*value)
	if limit < 1 || limit > maxListLimit {
		return 0, fmt.Errorf("limit must be between 1 and 500 (got %d)", limit)
	}
	return limit, nil
}

func decodeReportReplayTriggerRequest(r *http.Request) (api.ReportReplayTriggerRequest, error) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var body api.ReportReplayTriggerRequest
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
	return body, nil
}

func reportReplayTriggerRequest(body api.ReportReplayTriggerRequest) (reporttrigger.Request, error) {
	limit := defaultReportReplayLimit
	if body.Limit != nil {
		limit = int(*body.Limit)
		if limit < 1 || limit > maxReportReplayLimit {
			return reporttrigger.Request{}, fmt.Errorf("limit must be between 1 and %d (got %d): %w", maxReportReplayLimit, limit, domain.ErrInvariantViolation)
		}
	}

	var correlationKey string
	if body.CorrelationKey != nil {
		correlationKey = *body.CorrelationKey
	}
	var workflowID string
	if body.WorkflowID != nil {
		workflowID = *body.WorkflowID
	}
	var scenario reportprompt.Scenario
	if body.Scenario != nil {
		scenario = reportprompt.Scenario(*body.Scenario)
	}

	return reporttrigger.Request{
		Replay: alertreplay.Request{
			WindowStart:       body.WindowStart,
			WindowEnd:         body.WindowEnd,
			Grouping:          alertgrouping.DefaultConfig(),
			CreatedByWorkflow: reportTriggerHTTPWorkflow,
			Limit:             limit,
		},
		CorrelationKey: correlationKey,
		WorkflowID:     workflowID,
		Scenario:       scenario,
	}, nil
}

func reportReplayTriggerResponse(result reporttrigger.Result) api.ReportReplayTriggerResponse {
	return api.ReportReplayTriggerResponse{
		Started:    result.Started,
		WorkflowID: result.Workflow.WorkflowID,
		RunID:      result.Workflow.RunID,
		Stats:      reportReplayStats(result.Replay.Stats),
		Snapshots:  reportReplaySnapshotRefs(result.Replay.Snapshots),
	}
}

func reportReplayStats(stats alertreplay.Stats) api.ReportReplayStats {
	return api.ReportReplayStats{
		Ingested: api.ReportReplayIngestStats{
			Total:     int64(stats.Ingested.Total),
			Saved:     int64(stats.Ingested.Saved),
			Duplicate: int64(stats.Ingested.Duplicate),
			Failed:    int64(stats.Ingested.Failed),
		},
		EventsLoaded:       int64(stats.EventsLoaded),
		GroupsBuilt:        int64(stats.GroupsBuilt),
		GroupsSaved:        int64(stats.GroupsSaved),
		GroupsRefreshed:    int64(stats.GroupsRefreshed),
		GroupsExisting:     int64(stats.GroupsExisting),
		SnapshotsSaved:     int64(stats.SnapshotsSaved),
		SnapshotsDuplicate: int64(stats.SnapshotsDuplicate),
		GroupsClosed:       int64(stats.GroupsClosed),
		Failed:             int64(stats.Failed),
	}
}

func reportReplaySnapshotRefs(refs []alertreplay.SnapshotRef) []api.ReportReplaySnapshotRef {
	out := make([]api.ReportReplaySnapshotRef, len(refs))
	for i, ref := range refs {
		out[i] = api.ReportReplaySnapshotRef{
			ID:         int64(ref.ID),
			GroupIndex: int64(ref.GroupIndex),
			EventCount: int64(ref.EventCount),
		}
	}
	return out
}

func dashboardSummary(
	generatedAt time.Time,
	events []domain.AlertEvent,
	reports []domain.FinalReport,
	latestDeliveries map[domain.FinalReportID][]domain.ReportNotificationDelivery,
) api.DashboardSummary {
	reportStats := api.DashboardSummaryReports{
		TotalRecent: int64(len(reports)),
	}
	reportStats.SuccessRate.SetNull()
	alertStats := api.DashboardSummaryAlerts{
		TotalRecent: int64(len(events)),
	}
	for _, event := range events {
		switch event.Status {
		case domain.AlertStatusFiring:
			alertStats.Firing++
		case domain.AlertStatusResolved:
			alertStats.Resolved++
		}
	}
	for _, report := range reports {
		switch report.Severity {
		case domain.ReportSeverityInfo:
			reportStats.Severity.Info++
		case domain.ReportSeverityWarning:
			reportStats.Severity.Warning++
		case domain.ReportSeverityCritical:
			reportStats.Severity.Critical++
		}
		deliveries := latestDeliveries[report.ID]
		if len(deliveries) == 0 {
			reportStats.MissingDelivery++
			continue
		}
		switch deliveries[0].Status {
		case domain.ReportNotificationDeliveryStatusDelivered:
			reportStats.Delivered++
		case domain.ReportNotificationDeliveryStatusFailed:
			reportStats.Failed++
		case domain.ReportNotificationDeliveryStatusPending:
			reportStats.Pending++
		default:
			reportStats.MissingDelivery++
		}
	}
	if len(reports) > 0 {
		reportStats.SuccessRate.Set(float64(reportStats.Delivered) / float64(len(reports)))
	}
	return api.DashboardSummary{
		GeneratedAt: generatedAt,
		Alerts:      alertStats,
		Reports:     reportStats,
	}
}

func alertEventSummaries(events []domain.AlertEvent) []api.AlertEventSummary {
	items := make([]api.AlertEventSummary, len(events))
	for i, event := range events {
		endsAt := nullableTime(event.EndsAt)
		items[i] = api.AlertEventSummary{
			ID:                   int64(event.ID),
			Source:               event.Source,
			SourceFingerprint:    event.SourceFingerprint,
			CanonicalFingerprint: event.CanonicalFingerprint,
			Labels:               nonNilStringMap(event.Labels),
			Annotations:          nonNilStringMap(event.Annotations),
			Status:               string(event.Status),
			StartsAt:             event.StartsAt,
			EndsAt:               endsAt,
			CreatedAt:            event.CreatedAt,
		}
	}
	return items
}

func evidenceSnapshotSummaries(snapshots []domain.EvidenceSnapshot) ([]api.EvidenceSnapshotSummary, error) {
	items := make([]api.EvidenceSnapshotSummary, len(snapshots))
	for i, snapshot := range snapshots {
		payload, err := jsonObject(snapshot.Payload)
		if err != nil {
			return nil, err
		}
		provenance, err := jsonObject(snapshot.Provenance)
		if err != nil {
			return nil, err
		}
		items[i] = api.EvidenceSnapshotSummary{
			ID:                int64(snapshot.ID),
			AlertGroupID:      int64(snapshot.AlertGroupID),
			Digest:            snapshot.Digest,
			Payload:           payload,
			Provenance:        provenance,
			Status:            string(snapshot.Status),
			MissingFields:     nonNilStringSlice(snapshot.MissingFields),
			CreatedByWorkflow: snapshot.CreatedByWorkflow,
			CreatedAt:         snapshot.CreatedAt,
		}
	}
	return items, nil
}

func finalReportSummaries(reports []domain.FinalReport) []api.FinalReportSummary {
	items := make([]api.FinalReportSummary, len(reports))
	for i, report := range reports {
		items[i] = api.FinalReportSummary{
			ID:                int64(report.ID),
			CorrelationKey:    report.CorrelationKey,
			Title:             report.Title,
			ExecutiveSummary:  report.ExecutiveSummary,
			Severity:          api.ReportSeverity(report.Severity),
			Confidence:        api.ReportConfidence(report.Confidence),
			NotificationText:  report.NotificationText,
			CreatedByWorkflow: report.CreatedByWorkflow,
			CreatedAt:         report.CreatedAt,
		}
	}
	return items
}

func finalReportDetail(report domain.FinalReport, subReports []domain.SubReport) (api.FinalReportDetail, error) {
	reportSummaries, err := jsonArray[api.FinalReportSubReportSummary]("final report sub_reports", report.SubReports)
	if err != nil {
		return api.FinalReportDetail{}, err
	}
	actions, err := jsonArray[api.ReportAction]("final report recommended_actions", report.RecommendedActions)
	if err != nil {
		return api.FinalReportDetail{}, err
	}
	content, err := jsonObject(report.Content)
	if err != nil {
		return api.FinalReportDetail{}, fmt.Errorf("final report content: %w", err)
	}
	linked, err := subReportDetails(subReports)
	if err != nil {
		return api.FinalReportDetail{}, err
	}
	return api.FinalReportDetail{
		ID:                 int64(report.ID),
		CorrelationKey:     report.CorrelationKey,
		Title:              report.Title,
		ExecutiveSummary:   report.ExecutiveSummary,
		Severity:           api.ReportSeverity(report.Severity),
		Confidence:         api.ReportConfidence(report.Confidence),
		SubReports:         reportSummaries,
		RecommendedActions: actions,
		NotificationText:   report.NotificationText,
		Content:            content,
		Model:              report.Model,
		OutputMode:         report.OutputMode,
		CreatedByWorkflow:  report.CreatedByWorkflow,
		CreatedAt:          report.CreatedAt,
		LinkedSubReports:   linked,
	}, nil
}

func subReportDetails(reports []domain.SubReport) ([]api.SubReportDetail, error) {
	items := make([]api.SubReportDetail, len(reports))
	for i, report := range reports {
		findings, err := jsonArray[api.ReportFinding]("subreport findings", report.Findings)
		if err != nil {
			return nil, err
		}
		actions, err := jsonArray[api.ReportAction]("subreport recommended_actions", report.RecommendedActions)
		if err != nil {
			return nil, err
		}
		content, err := jsonObject(report.Content)
		if err != nil {
			return nil, fmt.Errorf("subreport content: %w", err)
		}
		items[i] = api.SubReportDetail{
			ID:                 int64(report.ID),
			EvidenceSnapshotID: int64(report.EvidenceSnapshotID),
			Scenario:           report.Scenario,
			Title:              report.Title,
			Summary:            report.Summary,
			Severity:           api.ReportSeverity(report.Severity),
			Confidence:         api.ReportConfidence(report.Confidence),
			Findings:           findings,
			RecommendedActions: actions,
			EvidenceRefs:       nonNilStringSlice(report.EvidenceRefs),
			Content:            content,
			Model:              report.Model,
			OutputMode:         report.OutputMode,
			CreatedByWorkflow:  report.CreatedByWorkflow,
			CreatedAt:          report.CreatedAt,
		}
	}
	return items, nil
}

func nullableTime(value *time.Time) api.Nullable[time.Time] {
	var out api.Nullable[time.Time]
	if value == nil {
		out.SetNull()
		return out
	}
	out.Set(*value)
	return out
}

func jsonObject(raw json.RawMessage) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]any{}, nil
	}
	return out, nil
}

func jsonArray[T any](label string, raw json.RawMessage) ([]T, error) {
	out := []T{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	if out == nil {
		return []T{}, nil
	}
	return out, nil
}

func nonNilStringMap(in map[string]string) map[string]string {
	if in != nil {
		return in
	}
	return map[string]string{}
}

func nonNilStringSlice(in []string) []string {
	if in != nil {
		return in
	}
	return []string{}
}

func writeJSON(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logError(ctx, logger, "failed to encode response", slog.Int("status", status), slog.Any("error", err))
	}
}

func writeError(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, status int, message string, cause error) {
	if cause != nil {
		logError(ctx, logger, "request failed", slog.Int("status", status), slog.Any("error", cause))
	}
	writeJSON(ctx, w, logger, status, api.ErrorResponse{Error: message})
}

func logError(ctx context.Context, logger *slog.Logger, msg string, attrs ...slog.Attr) {
	if logger == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logAttrs := correlation.LogAttrs(ctx)
	logAttrs = append(logAttrs, attrs...)
	logger.LogAttrs(ctx, slog.LevelError, msg, logAttrs...)
}
