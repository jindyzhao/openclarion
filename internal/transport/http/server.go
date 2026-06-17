// Package http implements the transport layer for OpenClarion's HTTP API.
// Handlers satisfy the generated ServerInterface from api/openapi.gen.go.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/alertgrouping"
	"github.com/openclarion/openclarion/internal/usecases/alertmanagerwebhook"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/alertsourcecheck"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500

	diagnosisConclusionTaskLimit       = 5
	diagnosisSupplementalEvidenceLimit = 20
	diagnosisConfidenceTimelineLimit   = 20

	diagnosisConclusionEventTurnPersisted        = "diagnosis_room.turn_persisted"
	diagnosisConclusionEventFinalReady           = "diagnosis_room.final_conclusion_ready"
	diagnosisConclusionEventClosed               = "diagnosis_room.closed"
	diagnosisConclusionEventSupplementalEvidence = "diagnosis_room.supplemental_evidence_provided"

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
	logger            *slog.Logger
	uowFactory        ports.UnitOfWorkFactory
	reportTrigger     ReportReplayTrigger
	policyTrigger     ReportWorkflowPolicyReplayTrigger
	scheduleSyncer    ReportWorkflowScheduleSynchronizer
	webhookIngestor   AlertmanagerWebhookIngestor
	roomStarter       DiagnosisRoomStarter
	alertSourceTester AlertSourceConnectionTester
	channelTester     NotificationChannelTester
	diagnosis         diagnosisConfig
}

// ReportReplayTrigger is the transport-facing report trigger usecase.
type ReportReplayTrigger interface {
	ReplayAndStart(ctx context.Context, req reporttrigger.Request) (reporttrigger.Result, error)
}

// ReportWorkflowPolicyReplayTrigger is the transport-facing policy-driven
// report replay usecase.
type ReportWorkflowPolicyReplayTrigger interface {
	ReplayAndStart(ctx context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error)
}

// ReportWorkflowScheduleSynchronizer synchronizes persisted schedules into the
// server-owned scheduler after successful configuration mutations.
type ReportWorkflowScheduleSynchronizer interface {
	SyncReportWorkflowSchedule(ctx context.Context, schedule domain.ReportWorkflowSchedule) error
}

// AlertmanagerWebhookIngestor is the transport-facing Alertmanager webhook
// ingest usecase.
type AlertmanagerWebhookIngestor interface {
	Ingest(ctx context.Context, req alertmanagerwebhook.Request) (alertmanagerwebhook.Result, error)
}

// DiagnosisRoomStarter is the transport-facing room creation usecase.
type DiagnosisRoomStarter interface {
	Start(ctx context.Context, req diagnosisroomstart.Request) (diagnosisroomstart.Result, error)
}

// AlertSourceConnectionTester is the transport-facing alert source
// connectivity check usecase.
type AlertSourceConnectionTester interface {
	TestAlertSourceConnection(ctx context.Context, profile domain.AlertSourceProfile) (alertsourcecheck.Result, error)
}

// NotificationChannelTester is the transport-facing notification channel
// delivery-test usecase.
type NotificationChannelTester interface {
	TestNotificationChannel(ctx context.Context, profile domain.NotificationChannelProfile) (notificationchannelcheck.Result, error)
}

// ServerOption customises optional HTTP handlers.
type ServerOption func(*Server)

// WithReportReplayTrigger enables POST /api/v1/report-triggers/replay-window.
func WithReportReplayTrigger(trigger ReportReplayTrigger) ServerOption {
	return func(s *Server) {
		s.reportTrigger = trigger
	}
}

// WithReportWorkflowPolicyReplayTrigger enables policy-driven report replay.
func WithReportWorkflowPolicyReplayTrigger(trigger ReportWorkflowPolicyReplayTrigger) ServerOption {
	return func(s *Server) {
		s.policyTrigger = trigger
	}
}

// WithReportWorkflowScheduleSynchronizer enables server-side schedule sync.
func WithReportWorkflowScheduleSynchronizer(syncer ReportWorkflowScheduleSynchronizer) ServerOption {
	return func(s *Server) {
		s.scheduleSyncer = syncer
	}
}

// WithAlertmanagerWebhookIngestor enables profile-bound Alertmanager webhook
// ingestion.
func WithAlertmanagerWebhookIngestor(ingestor AlertmanagerWebhookIngestor) ServerOption {
	return func(s *Server) {
		s.webhookIngestor = ingestor
	}
}

// WithDiagnosisRoomStarter enables POST /api/v1/diagnosis/rooms.
func WithDiagnosisRoomStarter(starter DiagnosisRoomStarter) ServerOption {
	return func(s *Server) {
		s.roomStarter = starter
	}
}

// WithAlertSourceConnectionTester enables alert source connection-test actions.
func WithAlertSourceConnectionTester(tester AlertSourceConnectionTester) ServerOption {
	return func(s *Server) {
		s.alertSourceTester = tester
	}
}

// WithNotificationChannelTester enables notification channel test actions.
func WithNotificationChannelTester(tester NotificationChannelTester) ServerOption {
	return func(s *Server) {
		s.channelTester = tester
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

	body, err := decodeReportReplayTriggerRequest(w, r)
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
	var diagnosisConclusions diagnosisConclusionBySnapshot
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var ferr error
		repo := uow.Reports()
		report, ferr = repo.FindFinalReportByID(ctx, domain.FinalReportID(reportID))
		if ferr != nil {
			return ferr
		}
		subReports, ferr = repo.ListSubReportsForFinalReport(ctx, report.ID, maxListLimit)
		if ferr != nil {
			return ferr
		}
		diagnosisConclusions, ferr = diagnosisConclusionsForSubReports(ctx, uow.Diagnosis(), subReports)
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

	detail, err := finalReportDetail(report, subReports, diagnosisConclusions)
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

func decodeReportReplayTriggerRequest(w http.ResponseWriter, r *http.Request) (api.ReportReplayTriggerRequest, error) {
	var body api.ReportReplayTriggerRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
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

type diagnosisConclusionBySnapshot map[domain.EvidenceSnapshotID]api.DiagnosisRoomConclusionSummary

type diagnosisRoomConclusionEventPayload struct {
	Kind              string                         `json:"kind"`
	SessionID         string                         `json:"session_id"`
	ChatSessionID     int64                          `json:"chat_session_id"`
	DiagnosisTaskID   int64                          `json:"diagnosis_task_id"`
	FinalConclusion   diagnosisRoomConclusionPayload `json:"final_conclusion"`
	OwnerSubject      string                         `json:"owner_subject,omitempty"`
	TurnCount         int                            `json:"turn_count,omitempty"`
	Status            string                         `json:"status,omitempty"`
	CloseReason       string                         `json:"close_reason,omitempty"`
	ConclusionVersion string                         `json:"conclusion_version,omitempty"`
	ClosedAt          *time.Time                     `json:"closed_at,omitempty"`
}

type diagnosisRoomConclusionPayload struct {
	Status                  string     `json:"status"`
	Source                  string     `json:"source"`
	Reason                  string     `json:"reason,omitempty"`
	EvidenceSnapshotID      int64      `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion       string     `json:"conclusion_version,omitempty"`
	ConfirmedBy             string     `json:"confirmed_by,omitempty"`
	SupplementalContextRefs []string   `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID         int64      `json:"assistant_turn_id,omitempty"`
	AssistantMessageID      string     `json:"assistant_message_id,omitempty"`
	AssistantSequence       int        `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt     *time.Time `json:"assistant_occurred_at,omitempty"`
	Content                 string     `json:"content,omitempty"`
	Confidence              string     `json:"confidence,omitempty"`
	RequiresHumanReview     *bool      `json:"requires_human_review,omitempty"`
}

type diagnosisRoomSupplementalEvidenceEventPayload struct {
	Kind                 string                                   `json:"kind"`
	SessionID            string                                   `json:"session_id,omitempty"`
	ChatSessionID        int64                                    `json:"chat_session_id,omitempty"`
	DiagnosisTaskID      int64                                    `json:"diagnosis_task_id,omitempty"`
	UserMessageID        string                                   `json:"user_message_id,omitempty"`
	AssistantMessageID   string                                   `json:"assistant_message_id,omitempty"`
	UserTurnID           int64                                    `json:"user_turn_id,omitempty"`
	AssistantTurnID      int64                                    `json:"assistant_turn_id,omitempty"`
	ContextRefs          []string                                 `json:"context_refs,omitempty"`
	SupplementalEvidence diagnosisRoomSupplementalEvidencePayload `json:"supplemental_evidence"`
}

type diagnosisRoomSupplementalEvidencePayload struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
	Evidence string `json:"evidence"`
}

type diagnosisRoomTurnPersistedEventPayload struct {
	Kind                string                                  `json:"kind"`
	SessionID           string                                  `json:"session_id,omitempty"`
	ChatSessionID       int64                                   `json:"chat_session_id,omitempty"`
	DiagnosisTaskID     int64                                   `json:"diagnosis_task_id,omitempty"`
	AssistantMessageID  string                                  `json:"assistant_message_id,omitempty"`
	AssistantTurnID     int64                                   `json:"assistant_turn_id,omitempty"`
	AssistantSequence   int                                     `json:"assistant_sequence,omitempty"`
	TurnCount           int                                     `json:"turn_count,omitempty"`
	Confidence          string                                  `json:"confidence,omitempty"`
	RequiresHumanReview bool                                    `json:"requires_human_review,omitempty"`
	EvidenceRequests    []diagnosisRoomEvidenceRequestPayload   `json:"evidence_requests,omitempty"`
	ConsultationInsight diagnosisRoomConsultationInsightPayload `json:"consultation_insight,omitempty"`
}

type diagnosisRoomConsultationInsightPayload struct {
	ConfidenceRationale           string                                            `json:"confidence_rationale,omitempty"`
	MissingEvidenceRequests       []diagnosisRoomConsultationEvidenceRequestPayload `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisRoomConsultationEvidenceRequestPayload `json:"evidence_collection_suggestions,omitempty"`
	ConclusionStatus              string                                            `json:"conclusion_status,omitempty"`
}

type diagnosisRoomEvidenceRequestPayload struct {
	TemplateID    int64  `json:"template_id,omitempty"`
	Tool          string `json:"tool"`
	Reason        string `json:"reason"`
	Query         string `json:"query,omitempty"`
	WindowSeconds int    `json:"window_seconds,omitempty"`
	StepSeconds   int    `json:"step_seconds,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type diagnosisRoomConsultationEvidenceRequestPayload struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
}

func diagnosisConclusionsForSubReports(ctx context.Context, repo ports.DiagnosisRepository, subReports []domain.SubReport) (diagnosisConclusionBySnapshot, error) {
	out := diagnosisConclusionBySnapshot{}
	if repo == nil || len(subReports) == 0 {
		return out, nil
	}
	seen := map[domain.EvidenceSnapshotID]struct{}{}
	for _, subReport := range subReports {
		snapshotID := subReport.EvidenceSnapshotID
		if snapshotID == 0 {
			continue
		}
		if _, ok := seen[snapshotID]; ok {
			continue
		}
		seen[snapshotID] = struct{}{}

		tasks, err := repo.ListTasksByEvidenceSnapshot(ctx, snapshotID, diagnosisConclusionTaskLimit)
		if err != nil {
			return nil, err
		}
		conclusion, ok, err := latestDiagnosisConclusion(ctx, repo, tasks)
		if err != nil {
			return nil, err
		}
		if ok {
			out[snapshotID] = conclusion
		}
	}
	return out, nil
}

func latestDiagnosisConclusion(ctx context.Context, repo ports.DiagnosisRepository, tasks []domain.DiagnosisTask) (api.DiagnosisRoomConclusionSummary, bool, error) {
	var best api.DiagnosisRoomConclusionSummary
	var bestOccurred time.Time
	bestSet := false
	for _, task := range tasks {
		conclusion, occurredAt, ok, err := latestDiagnosisConclusionForTask(ctx, repo, task.ID)
		if err != nil {
			return api.DiagnosisRoomConclusionSummary{}, false, err
		}
		if !ok {
			continue
		}
		if !bestSet ||
			occurredAt.After(bestOccurred) ||
			(occurredAt.Equal(bestOccurred) && conclusion.RecordedAt.After(best.RecordedAt)) {
			best = conclusion
			bestOccurred = occurredAt
			bestSet = true
		}
	}
	return best, bestSet, nil
}

func latestDiagnosisConclusionForTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) (api.DiagnosisRoomConclusionSummary, time.Time, bool, error) {
	var best api.DiagnosisRoomConclusionSummary
	var bestOccurred time.Time
	bestSet := false
	for _, kind := range []string{diagnosisConclusionEventFinalReady, diagnosisConclusionEventClosed} {
		events, err := repo.ListEventsByTaskAndKind(ctx, taskID, kind, 1)
		if err != nil {
			return api.DiagnosisRoomConclusionSummary{}, time.Time{}, false, err
		}
		if len(events) == 0 {
			continue
		}
		conclusion, ok, err := diagnosisConclusionFromEvent(events[0])
		if err != nil {
			return api.DiagnosisRoomConclusionSummary{}, time.Time{}, false, err
		}
		if !ok {
			continue
		}
		event := events[0]
		if !bestSet ||
			event.OccurredAt.After(bestOccurred) ||
			(event.OccurredAt.Equal(bestOccurred) && conclusion.RecordedAt.After(best.RecordedAt)) {
			best = conclusion
			bestOccurred = event.OccurredAt
			bestSet = true
		}
	}
	if bestSet {
		evidence, err := supplementalEvidenceForDiagnosisTask(ctx, repo, taskID)
		if err != nil {
			return api.DiagnosisRoomConclusionSummary{}, time.Time{}, false, err
		}
		best.SupplementalEvidence = evidence
		timeline, err := confidenceTimelineForDiagnosisTask(ctx, repo, taskID)
		if err != nil {
			return api.DiagnosisRoomConclusionSummary{}, time.Time{}, false, err
		}
		best.ConfidenceTimeline = timeline
	}
	return best, bestOccurred, bestSet, nil
}

func diagnosisConclusionFromEvent(event domain.DiagnosisTaskEvent) (api.DiagnosisRoomConclusionSummary, bool, error) {
	if len(event.Payload) == 0 {
		return api.DiagnosisRoomConclusionSummary{}, false, fmt.Errorf("diagnosis conclusion event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return api.DiagnosisRoomConclusionSummary{}, false, fmt.Errorf("diagnosis conclusion event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomConclusionEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return api.DiagnosisRoomConclusionSummary{}, false, fmt.Errorf("diagnosis conclusion event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return api.DiagnosisRoomConclusionSummary{}, false, fmt.Errorf("diagnosis conclusion event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return api.DiagnosisRoomConclusionSummary{}, false, fmt.Errorf("diagnosis conclusion event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	conclusion := payload.FinalConclusion
	if conclusion.Status != "available" {
		return api.DiagnosisRoomConclusionSummary{}, false, nil
	}
	content := strings.TrimSpace(conclusion.Content)
	if content == "" || payload.SessionID == "" || payload.ChatSessionID == 0 {
		return api.DiagnosisRoomConclusionSummary{}, false, nil
	}
	summary := api.DiagnosisRoomConclusionSummary{
		DiagnosisTaskID:         int64(event.TaskID),
		SessionID:               payload.SessionID,
		ChatSessionID:           payload.ChatSessionID,
		EventKind:               event.Kind,
		Status:                  conclusion.Status,
		Source:                  conclusion.Source,
		Reason:                  nonEmptyStringPtr(conclusion.Reason),
		EvidenceSnapshotID:      nonZeroInt64Ptr(conclusion.EvidenceSnapshotID),
		ConclusionVersion:       nonEmptyStringPtr(conclusion.ConclusionVersion),
		ConfirmedBy:             nonEmptyStringPtr(conclusion.ConfirmedBy),
		SupplementalContextRefs: nonEmptyStringSlice(conclusion.SupplementalContextRefs),
		AssistantTurnID:         nonZeroInt64Ptr(conclusion.AssistantTurnID),
		AssistantMessageID:      nonEmptyStringPtr(conclusion.AssistantMessageID),
		AssistantSequence:       nonZeroIntPtr(conclusion.AssistantSequence),
		AssistantOccurredAt:     copyTimePtr(conclusion.AssistantOccurredAt),
		Content:                 content,
		Confidence:              reportConfidencePtr(conclusion.Confidence),
		RequiresHumanReview:     copyBoolPtr(conclusion.RequiresHumanReview),
		RecordedAt:              event.RecordedAt,
	}
	return summary, true, nil
}

func supplementalEvidenceForDiagnosisTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) ([]api.DiagnosisRoomSupplementalEvidenceSummary, error) {
	events, err := repo.ListEventsByTaskAndKind(ctx, taskID, diagnosisConclusionEventSupplementalEvidence, diagnosisSupplementalEvidenceLimit)
	if err != nil {
		return nil, err
	}
	items := make([]api.DiagnosisRoomSupplementalEvidenceSummary, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		item, ok, err := supplementalEvidenceFromDiagnosisEvent(events[i])
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func confidenceTimelineForDiagnosisTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) ([]api.DiagnosisRoomConfidenceTimelineEntry, error) {
	events, err := repo.ListEventsByTaskAndKind(ctx, taskID, diagnosisConclusionEventTurnPersisted, diagnosisConfidenceTimelineLimit)
	if err != nil {
		return nil, err
	}
	items := make([]api.DiagnosisRoomConfidenceTimelineEntry, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		item, ok, err := confidenceTimelineEntryFromDiagnosisEvent(events[i])
		if err != nil {
			return nil, err
		}
		if ok {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func confidenceTimelineEntryFromDiagnosisEvent(event domain.DiagnosisTaskEvent) (api.DiagnosisRoomConfidenceTimelineEntry, bool, error) {
	if len(event.Payload) == 0 {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, fmt.Errorf("diagnosis confidence timeline event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, fmt.Errorf("diagnosis confidence timeline event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomTurnPersistedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, fmt.Errorf("diagnosis confidence timeline event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, fmt.Errorf("diagnosis confidence timeline event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, fmt.Errorf("diagnosis confidence timeline event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	confidence := reportConfidencePtr(payload.Confidence)
	if confidence == nil {
		return api.DiagnosisRoomConfidenceTimelineEntry{}, false, nil
	}
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = event.RecordedAt
	}
	evidenceRequests := diagnosisRoomEvidenceRequestSummaries(payload.EvidenceRequests)
	return api.DiagnosisRoomConfidenceTimelineEntry{
		EventKind:                     event.Kind,
		Confidence:                    *confidence,
		RequiresHumanReview:           payload.RequiresHumanReview,
		ConclusionStatus:              nonEmptyStringPtr(payload.ConsultationInsight.ConclusionStatus),
		ConfidenceRationale:           nonEmptyStringPtr(payload.ConsultationInsight.ConfidenceRationale),
		EvidenceRequestCount:          len(evidenceRequests),
		EvidenceRequests:              evidenceRequests,
		MissingEvidenceRequests:       diagnosisRoomConsultationEvidenceRequestSummaries(payload.ConsultationInsight.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: diagnosisRoomConsultationEvidenceRequestSummaries(payload.ConsultationInsight.EvidenceCollectionSuggestions),
		AssistantMessageID:            nonEmptyStringPtr(payload.AssistantMessageID),
		AssistantTurnID:               nonZeroInt64Ptr(payload.AssistantTurnID),
		AssistantSequence:             nonZeroIntPtr(payload.AssistantSequence),
		TurnCount:                     nonZeroIntPtr(payload.TurnCount),
		OccurredAt:                    occurredAt,
	}, true, nil
}

func diagnosisRoomEvidenceRequestSummaries(in []diagnosisRoomEvidenceRequestPayload) []api.DiagnosisRoomEvidenceRequestSummary {
	if len(in) == 0 {
		return nil
	}
	out := make([]api.DiagnosisRoomEvidenceRequestSummary, 0, len(in))
	for _, req := range in {
		tool := strings.TrimSpace(req.Tool)
		reason := strings.TrimSpace(req.Reason)
		if tool == "" || reason == "" {
			continue
		}
		out = append(out, api.DiagnosisRoomEvidenceRequestSummary{
			Tool:          tool,
			Reason:        reason,
			Query:         nonEmptyStringPtr(req.Query),
			TemplateID:    nonZeroInt64Ptr(req.TemplateID),
			WindowSeconds: nonZeroIntPtr(req.WindowSeconds),
			StepSeconds:   nonZeroIntPtr(req.StepSeconds),
			Limit:         nonZeroIntPtr(req.Limit),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func diagnosisRoomConsultationEvidenceRequestSummaries(
	in []diagnosisRoomConsultationEvidenceRequestPayload,
) []api.DiagnosisRoomConsultationEvidenceRequestSummary {
	if len(in) == 0 {
		return nil
	}
	out := make([]api.DiagnosisRoomConsultationEvidenceRequestSummary, 0, len(in))
	for _, req := range in {
		label := strings.TrimSpace(req.Label)
		detail := strings.TrimSpace(req.Detail)
		priority := strings.TrimSpace(req.Priority)
		if label == "" || detail == "" || priority == "" {
			continue
		}
		out = append(out, api.DiagnosisRoomConsultationEvidenceRequestSummary{
			Label:    label,
			Detail:   detail,
			Priority: priority,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func supplementalEvidenceFromDiagnosisEvent(event domain.DiagnosisTaskEvent) (api.DiagnosisRoomSupplementalEvidenceSummary, bool, error) {
	if len(event.Payload) == 0 {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, fmt.Errorf("diagnosis supplemental evidence event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, fmt.Errorf("diagnosis supplemental evidence event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomSupplementalEvidenceEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, fmt.Errorf("diagnosis supplemental evidence event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, fmt.Errorf("diagnosis supplemental evidence event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, fmt.Errorf("diagnosis supplemental evidence event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	evidence := payload.SupplementalEvidence
	label := strings.TrimSpace(evidence.Label)
	detail := strings.TrimSpace(evidence.Detail)
	priority := strings.TrimSpace(evidence.Priority)
	value := strings.TrimSpace(evidence.Evidence)
	if label == "" || detail == "" || priority == "" || value == "" {
		return api.DiagnosisRoomSupplementalEvidenceSummary{}, false, nil
	}
	providedAt := event.OccurredAt
	if providedAt.IsZero() {
		providedAt = event.RecordedAt
	}
	return api.DiagnosisRoomSupplementalEvidenceSummary{
		Label:              label,
		Detail:             detail,
		Priority:           priority,
		Evidence:           value,
		ContextRefs:        nonEmptyStringSlice(payload.ContextRefs),
		UserMessageID:      nonEmptyStringPtr(payload.UserMessageID),
		AssistantMessageID: nonEmptyStringPtr(payload.AssistantMessageID),
		UserTurnID:         nonZeroInt64Ptr(payload.UserTurnID),
		AssistantTurnID:    nonZeroInt64Ptr(payload.AssistantTurnID),
		ProvidedAt:         providedAt,
	}, true, nil
}

func nonEmptyStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nonEmptyStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func nonZeroInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func nonZeroIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func reportConfidencePtr(value string) *api.ReportConfidence {
	confidence := api.ReportConfidence(strings.TrimSpace(value))
	switch confidence {
	case api.ReportConfidenceLow, api.ReportConfidenceMedium, api.ReportConfidenceHigh:
		return &confidence
	default:
		return nil
	}
}

func finalReportDetail(report domain.FinalReport, subReports []domain.SubReport, conclusions diagnosisConclusionBySnapshot) (api.FinalReportDetail, error) {
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
	linked, err := subReportDetails(subReports, conclusions)
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

func subReportDetails(reports []domain.SubReport, conclusions diagnosisConclusionBySnapshot) ([]api.SubReportDetail, error) {
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
		item := api.SubReportDetail{
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
		if conclusion, ok := conclusions[report.EvidenceSnapshotID]; ok {
			item.DiagnosisConclusion = &conclusion
		}
		items[i] = item
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
