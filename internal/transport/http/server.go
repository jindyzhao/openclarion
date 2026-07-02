// Package http implements the transport layer for OpenClarion's HTTP API.
// Handlers satisfy the generated ServerInterface from api/openapi.gen.go.
package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
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
	"github.com/openclarion/openclarion/internal/usecases/diagnosisnotification"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomclose"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/notificationchannelcheck"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportnotification"
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
	diagnosisNotificationTimelineLimit = 20
	diagnosisParticipantTurnLimit      = 100
	reportNotificationDeliveryLimit    = 20

	diagnosisConclusionEventTurnPersisted             = "diagnosis_room.turn_persisted"
	diagnosisConclusionEventEvidenceCollected         = "diagnosis_room.evidence_collected"
	diagnosisConclusionEventFinalReady                = "diagnosis_room.final_conclusion_ready"
	diagnosisConclusionEventFailed                    = "diagnosis_room.failed"
	diagnosisConclusionEventClosed                    = "diagnosis_room.closed"
	diagnosisConclusionEventSupplementalEvidence      = "diagnosis_room.supplemental_evidence_provided"
	diagnosisConclusionEventCloseNotification         = "diagnosis_room.close_notification_sent"
	diagnosisConclusionEventFinalReadyNotification    = "diagnosis_room.final_ready_notification_sent"
	diagnosisConclusionEventAssistantTurnNotification = "diagnosis_room.assistant_turn_notification_sent"

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
	logger                     *slog.Logger
	uowFactory                 ports.UnitOfWorkFactory
	reportTrigger              ReportReplayTrigger
	reportNotifier             ReportNotificationSender
	policyTrigger              ReportWorkflowPolicyReplayTrigger
	scheduleSyncer             ReportWorkflowScheduleSynchronizer
	webhookIngestor            AlertmanagerWebhookIngestor
	roomStarter                DiagnosisRoomStarter
	roomCloser                 DiagnosisRoomCloser
	roomNotifier               DiagnosisRoomNotificationRetrier
	alertSourceTester          AlertSourceConnectionTester
	channelTester              NotificationChannelTester
	directorySyncer            DirectorySyncer
	rbacAuthorizer             RBACAuthorizer
	rbacBootstrapAdminSubjects map[string]bool
	roomVisibility             ports.DiagnosisRoomWorkflowVisibilityLookup
	weComAppCallback           DiagnosisWeComAppCallbackVerifier
	weComAppMessages           DiagnosisWeComAppCallbackMessageHandler
	diagnosis                  diagnosisConfig
}

// ReportReplayTrigger is the transport-facing report trigger usecase.
type ReportReplayTrigger interface {
	ReplayAndStart(ctx context.Context, req reporttrigger.Request) (reporttrigger.Result, error)
}

// ReportNotificationSender is the transport-facing final report notification
// retry usecase.
type ReportNotificationSender interface {
	Send(ctx context.Context, req reportnotification.Request) (reportnotification.Result, error)
}

// ReportWorkflowPolicyReplayTrigger is the transport-facing policy-driven
// report replay usecase.
type ReportWorkflowPolicyReplayTrigger interface {
	ReplayAndStart(ctx context.Context, req reportpolicytrigger.Request) (reporttrigger.Result, error)
}

// ReportWorkflowPolicyReplayDetailedTrigger is implemented by policy replay
// services that can return policy-only side effects such as auto diagnosis.
type ReportWorkflowPolicyReplayDetailedTrigger interface {
	ReplayAndStartDetailed(ctx context.Context, req reportpolicytrigger.Request) (reportpolicytrigger.Result, error)
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

// DiagnosisRoomCloser is the transport-facing room local-close usecase.
type DiagnosisRoomCloser interface {
	CloseUnavailable(ctx context.Context, req diagnosisroomclose.Request) (diagnosisroomclose.Result, error)
}

// DiagnosisRoomNotificationRetrier is the transport-facing diagnosis-room
// notification retry usecase.
type DiagnosisRoomNotificationRetrier interface {
	Retry(ctx context.Context, req diagnosisnotification.Request) (diagnosisnotification.Result, error)
}

// AlertSourceConnectionTester is the transport-facing alert source
// connectivity check usecase.
type AlertSourceConnectionTester interface {
	TestAlertSourceConnection(ctx context.Context, profile domain.AlertSourceProfile) (alertsourcecheck.Result, error)
}

// NotificationChannelTester is the transport-facing notification channel
// delivery-test usecase.
type NotificationChannelTester interface {
	TestNotificationChannel(ctx context.Context, profile domain.NotificationChannelProfile, req ...notificationchannelcheck.Request) (notificationchannelcheck.Result, error)
}

// ServerOption customises optional HTTP handlers.
type ServerOption func(*Server)

// WithReportReplayTrigger enables POST /api/v1/report-triggers/replay-window.
func WithReportReplayTrigger(trigger ReportReplayTrigger) ServerOption {
	return func(s *Server) {
		s.reportTrigger = trigger
	}
}

// WithReportNotificationSender enables final report notification retries.
func WithReportNotificationSender(sender ReportNotificationSender) ServerOption {
	return func(s *Server) {
		s.reportNotifier = sender
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

// WithDiagnosisRoomCloser enables local closure for unavailable diagnosis rooms.
func WithDiagnosisRoomCloser(closer DiagnosisRoomCloser) ServerOption {
	return func(s *Server) {
		s.roomCloser = closer
	}
}

// WithDiagnosisRoomNotificationRetrier enables diagnosis-room notification
// retries.
func WithDiagnosisRoomNotificationRetrier(retrier DiagnosisRoomNotificationRetrier) ServerOption {
	return func(s *Server) {
		s.roomNotifier = retrier
	}
}

// WithDiagnosisRoomWorkflowVisibilityLookup enriches room lists with sanitized
// workflow execution metadata when the workflow engine is reachable.
func WithDiagnosisRoomWorkflowVisibilityLookup(lookup ports.DiagnosisRoomWorkflowVisibilityLookup) ServerOption {
	return func(s *Server) {
		s.roomVisibility = lookup
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
	rbacPrincipal, ok := s.authorizeLocalRBACPrincipalForGlobalPermission(w, r, domain.RBACPermissionOperationsRead)
	if !ok {
		return
	}

	var events []domain.AlertEvent
	var snapshots []domain.EvidenceSnapshot
	var rooms []api.DiagnosisRoomSummary
	var reports []domain.FinalReport
	latestDeliveries := map[domain.FinalReportID][]domain.ReportNotificationDelivery{}
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		events, lerr = uow.Alerts().ListEvents(ctx, defaultListLimit)
		if lerr != nil {
			return lerr
		}
		snapshots, lerr = uow.Evidence().List(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		roomRows, lerr := uow.Diagnosis().ListChatSessions(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		rooms = diagnosisRoomSummaries(roomRows)
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
	rooms, ok = s.filterDiagnosisRoomSummariesByReadAccess(w, r, rbacPrincipal, rooms)
	if !ok {
		return
	}
	links, err := alertEvidenceLinks(events, snapshots, rooms)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "map dashboard alert links failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, dashboardSummary(time.Now().UTC(), events, links, reports, latestDeliveries))
}

// ListAlerts implements api.ServerInterface.
func (s *Server) ListAlerts(w http.ResponseWriter, r *http.Request, params api.ListAlertsParams) {
	rbacPrincipal, ok := s.authorizeLocalRBACPrincipalForGlobalPermission(w, r, domain.RBACPermissionOperationsRead)
	if !ok {
		return
	}

	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var events []domain.AlertEvent
	var snapshots []domain.EvidenceSnapshot
	var rooms []api.DiagnosisRoomSummary
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		events, lerr = uow.Alerts().ListEvents(ctx, limit)
		if lerr != nil {
			return lerr
		}
		snapshots, lerr = uow.Evidence().List(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		roomRows, lerr := uow.Diagnosis().ListChatSessions(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		rooms, lerr = diagnosisRoomSummariesWithConclusions(ctx, uow.Diagnosis(), roomRows)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list alerts failed", err)
		return
	}
	rooms, ok = s.filterDiagnosisRoomSummariesByReadAccess(w, r, rbacPrincipal, rooms)
	if !ok {
		return
	}
	links, err := alertEvidenceLinks(events, snapshots, rooms)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "map alert links failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.AlertListResponse{
		Items: alertEventSummaries(events, links),
	})
}

// ListEvidenceSnapshots implements api.ServerInterface.
func (s *Server) ListEvidenceSnapshots(w http.ResponseWriter, r *http.Request, params api.ListEvidenceSnapshotsParams) {
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionOperationsRead) {
		return
	}

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
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionOperationsRead) {
		return
	}

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

// ListDiagnosisRooms implements api.ServerInterface.
func (s *Server) ListDiagnosisRooms(w http.ResponseWriter, r *http.Request, params api.ListDiagnosisRoomsParams) {
	_, rbacPrincipal, ok := s.resolveLocalRBACPrincipal(w, r)
	if !ok {
		return
	}
	hasGlobalRead, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		rbacPrincipal,
		domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACScopeKindGlobal,
		"",
		false,
	)
	if !ok {
		return
	}
	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var items []api.DiagnosisRoomSummary
	queryLimit := limit
	if !hasGlobalRead {
		queryLimit = maxListLimit
	}
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		rooms, lerr := uow.Diagnosis().ListChatSessions(ctx, queryLimit)
		if lerr != nil {
			return lerr
		}
		items, lerr = diagnosisRoomSummariesWithConclusionsAndNotifications(ctx, uow.Diagnosis(), rooms)
		return lerr
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list diagnosis rooms failed", err)
		return
	}
	if !hasGlobalRead {
		items, ok = s.filterDiagnosisRoomSummariesByLocalRBAC(w, r, rbacPrincipal, items)
		if !ok {
			return
		}
		if len(items) > limit {
			items = items[:limit]
		}
	}
	items, err = s.withDiagnosisRoomParticipantDirectoryUsers(r.Context(), items)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "load diagnosis room participant directory users failed", err)
		return
	}
	items = s.withDiagnosisRoomWorkflowVisibility(r.Context(), items)

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DiagnosisRoomListResponse{
		Items: items,
	})
}

func (s *Server) filterDiagnosisRoomSummariesByLocalRBAC(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	items []api.DiagnosisRoomSummary,
) ([]api.DiagnosisRoomSummary, bool) {
	if len(items) == 0 {
		return items, true
	}
	out := make([]api.DiagnosisRoomSummary, 0, len(items))
	allowedBySession := map[string]bool{}
	for _, item := range items {
		allowed, ok := s.diagnosisRoomReadAllowedBySession(w, r, principal, item.SessionID, allowedBySession)
		if !ok {
			return nil, false
		}
		if allowed {
			out = append(out, item)
		}
	}
	return out, true
}

func (s *Server) filterDiagnosisRoomSummariesByReadAccess(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	items []api.DiagnosisRoomSummary,
) ([]api.DiagnosisRoomSummary, bool) {
	if len(items) == 0 {
		return items, true
	}
	hasGlobalRead, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		principal,
		domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACScopeKindGlobal,
		"",
		false,
	)
	if !ok {
		return nil, false
	}
	if hasGlobalRead {
		return items, true
	}
	return s.filterDiagnosisRoomSummariesByLocalRBAC(w, r, principal, items)
}

func (s *Server) diagnosisRoomReadAllowedBySession(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	sessionID string,
	allowedBySession map[string]bool,
) (bool, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, true
	}
	if allowed, ok := allowedBySession[sessionID]; ok {
		return allowed, true
	}
	allowed, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		principal,
		domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACScopeKindDiagnosisRoom,
		sessionID,
		false,
	)
	if !ok {
		return false, false
	}
	allowedBySession[sessionID] = allowed
	return allowed, true
}

// GetDiagnosisRoom implements api.ServerInterface.
func (s *Server) GetDiagnosisRoom(w http.ResponseWriter, r *http.Request, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "session_id is required", nil)
		return
	}
	if !s.authorizeLocalRBACRequestForScope(w, r, domain.RBACPermissionDiagnosisRoomRead, domain.RBACScopeKindDiagnosisRoom, sessionID) {
		return
	}

	var item api.DiagnosisRoomSummary
	err := s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		room, gerr := diagnosisRoomSummaryBySession(ctx, uow.Diagnosis(), sessionID)
		if gerr != nil {
			return gerr
		}
		item = room
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		writeError(r.Context(), w, s.logger, http.StatusNotFound, "diagnosis room not found", nil)
		return
	}
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "get diagnosis room failed", err)
		return
	}
	enrichedItems, err := s.withDiagnosisRoomParticipantDirectoryUsers(r.Context(), []api.DiagnosisRoomSummary{item})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "load diagnosis room participant directory users failed", err)
		return
	}
	if len(enrichedItems) > 0 {
		item = enrichedItems[0]
	}
	items := s.withDiagnosisRoomWorkflowVisibility(r.Context(), []api.DiagnosisRoomSummary{item})
	if len(items) > 0 {
		item = items[0]
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, item)
}

// ListDiagnosisHandoffs implements api.ServerInterface.
func (s *Server) ListDiagnosisHandoffs(w http.ResponseWriter, r *http.Request, params api.ListDiagnosisHandoffsParams) {
	_, rbacPrincipal, ok := s.resolveLocalRBACPrincipal(w, r)
	if !ok {
		return
	}
	for _, permission := range []domain.RBACPermission{
		domain.RBACPermissionOperationsRead,
		domain.RBACPermissionDiagnosisRoomParticipate,
	} {
		allowed, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
			w,
			r,
			rbacPrincipal,
			permission,
			domain.RBACScopeKindGlobal,
			"",
			true,
		)
		if !ok || !allowed {
			return
		}
	}

	limit, err := parseListLimit(params.Limit)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var events []domain.AlertEvent
	var snapshots []domain.EvidenceSnapshot
	var rooms []api.DiagnosisRoomSummary
	err = s.uowFactory.WithinTx(r.Context(), func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		events, lerr = uow.Alerts().ListEvents(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		snapshots, lerr = uow.Evidence().List(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		roomRows, lerr := uow.Diagnosis().ListChatSessions(ctx, maxListLimit)
		if lerr != nil {
			return lerr
		}
		rooms = diagnosisRoomSummaries(roomRows)
		return nil
	})
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "list diagnosis handoffs failed", err)
		return
	}
	links, err := alertEvidenceLinks(events, snapshots, rooms)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusInternalServerError, "map diagnosis handoffs failed", err)
		return
	}

	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DiagnosisHandoffListResponse{
		Items: diagnosisHandoffBacklogItems(events, links, limit),
	})
}

// TriggerReportReplay implements api.ServerInterface.
func (s *Server) TriggerReportReplay(w http.ResponseWriter, r *http.Request) {
	if s.reportTrigger == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "report trigger is not configured", nil)
		return
	}
	if !s.authorizeLocalRBACRequest(w, r, domain.RBACPermissionReportWorkflowManage) {
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
	rbacPrincipal, ok := s.authorizeLocalRBACPrincipalForGlobalPermission(w, r, domain.RBACPermissionOperationsRead)
	if !ok {
		return
	}

	var report domain.FinalReport
	var subReports []domain.SubReport
	var notificationDeliveries []domain.ReportNotificationDelivery
	var diagnosisConclusions diagnosisConclusionBySnapshot
	var diagnosisProgress diagnosisProgressBySnapshot
	var diagnosisRooms diagnosisRoomBySnapshot
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
		notificationDeliveries, ferr = repo.ListNotificationDeliveriesByFinalReport(ctx, report.ID, reportNotificationDeliveryLimit)
		if ferr != nil {
			return ferr
		}
		diagnosisConclusions, diagnosisProgress, diagnosisRooms, ferr = diagnosisStatesForSubReports(ctx, uow.Diagnosis(), subReports)
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
	diagnosisConclusions, diagnosisProgress, diagnosisRooms, ok = s.filterDiagnosisReportStatesByReadAccess(
		w,
		r,
		rbacPrincipal,
		diagnosisConclusions,
		diagnosisProgress,
		diagnosisRooms,
	)
	if !ok {
		return
	}

	detail, err := finalReportDetail(report, subReports, notificationDeliveries, diagnosisConclusions, diagnosisProgress, diagnosisRooms)
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
		Started:        result.Started,
		CorrelationKey: result.CorrelationKey,
		WorkflowID:     result.Workflow.WorkflowID,
		RunID:          result.Workflow.RunID,
		Stats:          reportReplayStats(result.Replay.Stats),
		Snapshots:      reportReplaySnapshotRefs(result.Replay.Snapshots),
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
	links map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink,
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
		Diagnosis:   dashboardDiagnosisStats(links),
		Reports:     reportStats,
	}
}

func dashboardDiagnosisStats(
	links map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink,
) api.DashboardSummaryDiagnosis {
	stats := api.DashboardSummaryDiagnosis{}
	affectedAlerts := make(map[domain.AlertEventID]struct{})
	seenRooms := make(map[int64]struct{})
	seenSnapshots := make(map[int64]struct{})
	seenSnapshotsNeedingRoom := make(map[int64]struct{})
	for alertID, snapshots := range links {
		for _, snapshot := range snapshots {
			if _, ok := seenSnapshots[snapshot.ID]; !ok {
				seenSnapshots[snapshot.ID] = struct{}{}
				stats.LinkedSnapshots++
			}
			if len(snapshot.DiagnosisRooms) == 0 {
				if _, ok := seenSnapshotsNeedingRoom[snapshot.ID]; !ok {
					seenSnapshotsNeedingRoom[snapshot.ID] = struct{}{}
					stats.SnapshotsNeedingRoom++
				}
				affectedAlerts[alertID] = struct{}{}
				continue
			}
			for _, room := range snapshot.DiagnosisRooms {
				if _, ok := seenRooms[room.ChatSessionID]; ok {
					continue
				}
				seenRooms[room.ChatSessionID] = struct{}{}
				stats.RoomsStarted++
			}
		}
	}
	stats.AffectedAlertsNeedingRoom = int64(len(affectedAlerts))
	return stats
}

type diagnosisHandoffBacklogAccumulator struct {
	snapshot api.AlertEvidenceSnapshotLink
	alerts   []domain.AlertEvent
	seen     map[domain.AlertEventID]struct{}
}

func diagnosisHandoffBacklogItems(
	events []domain.AlertEvent,
	links map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink,
	limit int,
) []api.DiagnosisHandoffBacklogItem {
	if limit <= 0 {
		return []api.DiagnosisHandoffBacklogItem{}
	}
	bySnapshot := make(map[int64]*diagnosisHandoffBacklogAccumulator)
	snapshotIDs := make([]int64, 0)
	for _, event := range events {
		for _, snapshot := range links[event.ID] {
			if len(snapshot.DiagnosisRooms) > 0 {
				continue
			}
			acc, ok := bySnapshot[snapshot.ID]
			if !ok {
				snapshot.DiagnosisRooms = nonNilDiagnosisRoomSummaries(snapshot.DiagnosisRooms)
				acc = &diagnosisHandoffBacklogAccumulator{
					snapshot: snapshot,
					seen:     make(map[domain.AlertEventID]struct{}),
				}
				bySnapshot[snapshot.ID] = acc
				snapshotIDs = append(snapshotIDs, snapshot.ID)
			}
			if _, ok := acc.seen[event.ID]; ok {
				continue
			}
			acc.seen[event.ID] = struct{}{}
			acc.alerts = append(acc.alerts, event)
		}
	}
	sort.SliceStable(snapshotIDs, func(i, j int) bool {
		left := bySnapshot[snapshotIDs[i]].snapshot
		right := bySnapshot[snapshotIDs[j]].snapshot
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.After(right.CreatedAt)
		}
		return left.ID > right.ID
	})
	if limit > len(snapshotIDs) {
		limit = len(snapshotIDs)
	}
	items := make([]api.DiagnosisHandoffBacklogItem, 0, limit)
	for _, snapshotID := range snapshotIDs[:limit] {
		acc := bySnapshot[snapshotID]
		items = append(items, api.DiagnosisHandoffBacklogItem{
			Reason:           api.MissingDiagnosisRoom,
			EvidenceSnapshot: acc.snapshot,
			Alerts:           alertEventSummaries(acc.alerts, links),
		})
	}
	return items
}

func alertEventSummaries(
	events []domain.AlertEvent,
	links map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink,
) []api.AlertEventSummary {
	items := make([]api.AlertEventSummary, len(events))
	for i, event := range events {
		endsAt := nullableTime(event.EndsAt)
		items[i] = api.AlertEventSummary{
			ID:                      int64(event.ID),
			Source:                  event.Source,
			AlertSourceProfileID:    int64(event.AlertSourceProfileID),
			SourceFingerprint:       event.SourceFingerprint,
			CanonicalFingerprint:    event.CanonicalFingerprint,
			Labels:                  nonNilStringMap(event.Labels),
			Annotations:             nonNilStringMap(event.Annotations),
			Status:                  string(event.Status),
			StartsAt:                event.StartsAt,
			EndsAt:                  endsAt,
			LinkedEvidenceSnapshots: nonNilAlertEvidenceSnapshotLinks(links[event.ID]),
			CreatedAt:               event.CreatedAt,
		}
	}
	return items
}

type snapshotLinkPayload struct {
	Events []snapshotLinkEvent `json:"events"`
}

type snapshotLinkEvent struct {
	SourceFingerprint    string `json:"source_fingerprint"`
	CanonicalFingerprint string `json:"canonical_fingerprint"`
}

func alertEvidenceLinks(
	events []domain.AlertEvent,
	snapshots []domain.EvidenceSnapshot,
	rooms []api.DiagnosisRoomSummary,
) (map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink, error) {
	eventsByFingerprint := make(map[string][]domain.AlertEventID)
	for _, event := range events {
		if event.CanonicalFingerprint != "" {
			eventsByFingerprint[fingerprintKey("canonical", event.CanonicalFingerprint)] = append(
				eventsByFingerprint[fingerprintKey("canonical", event.CanonicalFingerprint)],
				event.ID,
			)
		}
		if event.SourceFingerprint != "" {
			eventsByFingerprint[fingerprintKey("source", event.SourceFingerprint)] = append(
				eventsByFingerprint[fingerprintKey("source", event.SourceFingerprint)],
				event.ID,
			)
		}
	}

	roomsBySnapshot := make(map[domain.EvidenceSnapshotID][]api.DiagnosisRoomSummary)
	for _, room := range rooms {
		snapshotID := domain.EvidenceSnapshotID(room.EvidenceSnapshotID)
		roomsBySnapshot[snapshotID] = append(roomsBySnapshot[snapshotID], room)
	}

	links := make(map[domain.AlertEventID][]api.AlertEvidenceSnapshotLink)
	seen := make(map[domain.AlertEventID]map[domain.EvidenceSnapshotID]struct{})
	for _, snapshot := range snapshots {
		payloadEvents, err := snapshotLinkEvents(snapshot)
		if err != nil {
			return nil, err
		}
		for _, payloadEvent := range payloadEvents {
			for _, key := range []string{
				fingerprintKey("canonical", payloadEvent.CanonicalFingerprint),
				fingerprintKey("source", payloadEvent.SourceFingerprint),
			} {
				if key == "" {
					continue
				}
				for _, alertID := range eventsByFingerprint[key] {
					if seen[alertID] == nil {
						seen[alertID] = make(map[domain.EvidenceSnapshotID]struct{})
					}
					if _, ok := seen[alertID][snapshot.ID]; ok {
						continue
					}
					seen[alertID][snapshot.ID] = struct{}{}
					links[alertID] = append(links[alertID], api.AlertEvidenceSnapshotLink{
						ID:                int64(snapshot.ID),
						AlertGroupID:      int64(snapshot.AlertGroupID),
						Digest:            snapshot.Digest,
						Status:            string(snapshot.Status),
						CreatedByWorkflow: snapshot.CreatedByWorkflow,
						CreatedAt:         snapshot.CreatedAt,
						DiagnosisRooms:    nonNilDiagnosisRoomSummaries(roomsBySnapshot[snapshot.ID]),
					})
				}
			}
		}
	}
	return links, nil
}

func snapshotLinkEvents(snapshot domain.EvidenceSnapshot) ([]snapshotLinkEvent, error) {
	var payload snapshotLinkPayload
	if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode evidence snapshot %d payload links: %w", snapshot.ID, err)
	}
	return payload.Events, nil
}

func fingerprintKey(kind, value string) string {
	if value == "" {
		return ""
	}
	return kind + ":" + value
}

func nonNilAlertEvidenceSnapshotLinks(in []api.AlertEvidenceSnapshotLink) []api.AlertEvidenceSnapshotLink {
	if in == nil {
		return []api.AlertEvidenceSnapshotLink{}
	}
	return in
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

func diagnosisRoomSummaries(rooms []domain.ChatSessionWithTask) []api.DiagnosisRoomSummary {
	items := make([]api.DiagnosisRoomSummary, len(rooms))
	for i, room := range rooms {
		items[i] = diagnosisRoomSummary(room)
	}
	return items
}

func diagnosisRoomSummariesWithConclusions(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	rooms []domain.ChatSessionWithTask,
) ([]api.DiagnosisRoomSummary, error) {
	return diagnosisRoomSummariesWithOptions(ctx, repo, rooms, false)
}

func diagnosisRoomSummariesWithConclusionsAndNotifications(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	rooms []domain.ChatSessionWithTask,
) ([]api.DiagnosisRoomSummary, error) {
	return diagnosisRoomSummariesWithOptions(ctx, repo, rooms, true)
}

func diagnosisRoomSummaryBySession(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	sessionID string,
) (api.DiagnosisRoomSummary, error) {
	session, err := repo.FindChatSessionByKey(ctx, sessionID)
	if err != nil {
		return api.DiagnosisRoomSummary{}, err
	}
	task, err := repo.FindTaskByID(ctx, session.DiagnosisTaskID)
	if err != nil {
		return api.DiagnosisRoomSummary{}, err
	}
	rooms, err := diagnosisRoomSummariesWithConclusionsAndNotifications(ctx, repo, []domain.ChatSessionWithTask{{
		Session: session,
		Task:    task,
	}})
	if err != nil {
		return api.DiagnosisRoomSummary{}, err
	}
	if len(rooms) == 0 {
		return api.DiagnosisRoomSummary{}, domain.ErrNotFound
	}
	return rooms[0], nil
}

func diagnosisRoomSummariesWithOptions(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	rooms []domain.ChatSessionWithTask,
	includeNotifications bool,
) ([]api.DiagnosisRoomSummary, error) {
	items := diagnosisRoomSummaries(rooms)
	for i, room := range rooms {
		conclusion, _, ok, err := latestDiagnosisConclusionForTask(ctx, repo, room.Task.ID)
		if err != nil {
			return nil, err
		}
		if ok {
			items[i].LatestConclusion = &conclusion
		}
		if !ok {
			progress, ok, err := latestDiagnosisProgressForTask(ctx, repo, room.Task)
			if err != nil {
				return nil, err
			}
			if ok {
				items[i].LatestProgress = &progress
			}
		}
		turns, err := diagnosisRoomParticipantTurns(ctx, repo, room.Session.ID)
		if err != nil {
			return nil, err
		}
		evidenceActors, err := diagnosisRoomEvidenceCollectionActors(ctx, repo, room.Task.ID)
		if err != nil {
			return nil, err
		}
		items[i].Participants = diagnosisRoomParticipantSummaries(
			room.Session.OwnerSubject,
			turns,
			evidenceActors,
			items[i].LatestProgress,
			items[i].LatestConclusion,
		)
		if includeNotifications {
			timeline, err := notificationTimelineForDiagnosisTask(ctx, repo, room.Task.ID)
			if err != nil {
				return nil, err
			}
			if len(timeline) > 0 {
				items[i].NotificationTimeline = timeline
			}
		}
	}
	return items, nil
}

func diagnosisRoomSummary(room domain.ChatSessionWithTask) api.DiagnosisRoomSummary {
	session := room.Session
	task := room.Task
	return api.DiagnosisRoomSummary{
		SessionID:          session.SessionKey,
		ChatSessionID:      int64(session.ID),
		DiagnosisTaskID:    int64(task.ID),
		EvidenceSnapshotID: int64(task.EvidenceSnapshotID),
		WorkflowID:         task.WorkflowID,
		RunID:              task.RunID,
		TaskStatus:         api.DiagnosisTaskStatus(task.Status),
		RoomStatus:         api.DiagnosisRoomStatus(session.Status),
		TurnCount:          session.TurnCount,
		StartedAt:          session.StartedAt,
		LastActivityAt:     session.LastActivityAt,
		ClosedAt:           nullableTime(session.ClosedAt),
		CloseReason:        session.CloseReason,
		Participants:       diagnosisRoomParticipantSummaries(session.OwnerSubject, nil, nil, nil, nil),
		CreatedAt:          session.CreatedAt,
		UpdatedAt:          session.UpdatedAt,
	}
}

func (s *Server) withDiagnosisRoomParticipantDirectoryUsers(
	ctx context.Context,
	items []api.DiagnosisRoomSummary,
) ([]api.DiagnosisRoomSummary, error) {
	if len(items) == 0 {
		return items, nil
	}
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var lerr error
		items, lerr = diagnosisRoomSummariesWithParticipantDirectoryUsers(ctx, uow.Config(), items)
		return lerr
	})
	return items, err
}

func diagnosisRoomSummariesWithParticipantDirectoryUsers(
	ctx context.Context,
	config ports.ConfigurationRepository,
	items []api.DiagnosisRoomSummary,
) ([]api.DiagnosisRoomSummary, error) {
	subjectUsers := map[string]api.DiagnosisRoomParticipantDirectoryUser{}
	for _, subject := range diagnosisRoomParticipantDirectorySubjects(items) {
		users, err := config.ListDirectoryUsersBySubject(ctx, subject, maxListLimit)
		if err != nil {
			return nil, err
		}
		user, ok := diagnosisRoomParticipantDirectoryUserForSubject(users)
		if ok {
			subjectUsers[subject] = user
		}
	}
	for i := range items {
		items[i].ParticipantDirectoryUsers = diagnosisRoomParticipantDirectoryUsers(items[i].Participants, subjectUsers)
	}
	return items, nil
}

func diagnosisRoomParticipantDirectorySubjects(items []api.DiagnosisRoomSummary) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range items {
		for _, participant := range item.Participants {
			subject := strings.TrimSpace(participant.Subject)
			if subject == "" || participant.IsSystem || diagnosisRoomSystemSubject(subject) {
				continue
			}
			if _, ok := seen[subject]; ok {
				continue
			}
			seen[subject] = struct{}{}
			out = append(out, subject)
		}
	}
	sort.Strings(out)
	return out
}

func diagnosisRoomParticipantDirectoryUsers(
	participants []api.DiagnosisRoomParticipantSummary,
	subjectUsers map[string]api.DiagnosisRoomParticipantDirectoryUser,
) []api.DiagnosisRoomParticipantDirectoryUser {
	if len(participants) == 0 || len(subjectUsers) == 0 {
		return nil
	}
	out := make([]api.DiagnosisRoomParticipantDirectoryUser, 0)
	for _, participant := range participants {
		subject := strings.TrimSpace(participant.Subject)
		if subject == "" || participant.IsSystem || diagnosisRoomSystemSubject(subject) {
			continue
		}
		user, ok := subjectUsers[subject]
		if ok {
			out = append(out, user)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func diagnosisRoomParticipantDirectoryUserForSubject(
	users []domain.DirectoryUser,
) (api.DiagnosisRoomParticipantDirectoryUser, bool) {
	if len(users) == 0 {
		return api.DiagnosisRoomParticipantDirectoryUser{}, false
	}
	selected := users[0]
	for _, user := range users {
		if user.Active {
			selected = user
			break
		}
	}
	return api.DiagnosisRoomParticipantDirectoryUser{
		Subject:         selected.Subject,
		Username:        selected.Username,
		DisplayName:     selected.DisplayName,
		JobTitle:        selected.JobTitle,
		Department:      selected.Department,
		Section:         selected.Section,
		DepartmentPath:  selected.DepartmentPath,
		DepartmentPaths: nonNilStringSlice(selected.DepartmentPaths),
		Active:          selected.Active,
	}, true
}

type diagnosisRoomParticipantAccumulator struct {
	subject                   string
	roles                     map[api.DiagnosisRoomParticipantSummaryRolesItem]struct{}
	isSystem                  bool
	messageCount              int
	evidenceCollectionCount   int
	supplementalEvidenceCount int
	confirmedConclusion       bool
}

func diagnosisRoomParticipantTurns(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	sessionID domain.ChatSessionID,
) ([]domain.ChatTurn, error) {
	if sessionID == 0 {
		return nil, nil
	}
	return repo.ListChatTurnsBySession(ctx, sessionID, diagnosisParticipantTurnLimit)
}

func diagnosisRoomEvidenceCollectionActors(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) ([]string, error) {
	if taskID == 0 {
		return nil, nil
	}
	events, err := repo.ListEventsByTaskAndKind(ctx, taskID, diagnosisConclusionEventEvidenceCollected, diagnosisConfidenceTimelineLimit)
	if err != nil {
		return nil, err
	}
	actors := make([]string, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		actor, ok, err := evidenceCollectionActorFromDiagnosisEvent(events[i])
		if err != nil {
			return nil, err
		}
		if ok {
			actors = append(actors, actor)
		}
	}
	return actors, nil
}

func diagnosisRoomParticipantSummaries(
	ownerSubject string,
	turns []domain.ChatTurn,
	evidenceActors []string,
	progress *api.DiagnosisRoomProgressSummary,
	conclusion *api.DiagnosisRoomConclusionSummary,
) []api.DiagnosisRoomParticipantSummary {
	participants := map[string]*diagnosisRoomParticipantAccumulator{}
	if owner := strings.TrimSpace(ownerSubject); owner != "" {
		diagnosisRoomParticipantWithRole(participants, owner, api.DiagnosisRoomParticipantSummaryRolesItemOwner)
	}
	for _, turn := range turns {
		subject := strings.TrimSpace(turn.ActorSubject)
		if subject == "" {
			continue
		}
		role := api.DiagnosisRoomParticipantSummaryRolesItemMessage
		if turn.Role == domain.ChatRoleAssistant {
			role = api.DiagnosisRoomParticipantSummaryRolesItemAssistant
		}
		participant := diagnosisRoomParticipantWithRole(participants, subject, role)
		participant.messageCount++
	}
	for _, actor := range evidenceActors {
		subject := strings.TrimSpace(actor)
		if subject == "" {
			continue
		}
		participant := diagnosisRoomParticipantWithRole(participants, subject, api.DiagnosisRoomParticipantSummaryRolesItemEvidence)
		participant.evidenceCollectionCount++
	}
	if progress != nil {
		diagnosisRoomAddSupplementalEvidenceParticipants(participants, progress.SupplementalEvidence)
	}
	if conclusion != nil {
		diagnosisRoomAddSupplementalEvidenceParticipants(participants, conclusion.SupplementalEvidence)
		if conclusion.ConfirmedBy != nil {
			subject := strings.TrimSpace(*conclusion.ConfirmedBy)
			if subject != "" {
				participant := diagnosisRoomParticipantWithRole(participants, subject, api.DiagnosisRoomParticipantSummaryRolesItemConfirmation)
				participant.confirmedConclusion = true
			}
		}
	}
	if len(participants) == 0 {
		return nil
	}
	out := make([]api.DiagnosisRoomParticipantSummary, 0, len(participants))
	for _, participant := range participants {
		out = append(out, api.DiagnosisRoomParticipantSummary{
			Subject:                   participant.subject,
			Roles:                     diagnosisRoomParticipantRoles(participant.roles),
			IsSystem:                  participant.isSystem,
			MessageCount:              participant.messageCount,
			EvidenceCollectionCount:   participant.evidenceCollectionCount,
			SupplementalEvidenceCount: participant.supplementalEvidenceCount,
			ConfirmedConclusion:       participant.confirmedConclusion,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsSystem != out[j].IsSystem {
			return !out[i].IsSystem
		}
		leftActivity := diagnosisRoomParticipantActivity(out[i])
		rightActivity := diagnosisRoomParticipantActivity(out[j])
		if leftActivity != rightActivity {
			return leftActivity > rightActivity
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}

func diagnosisRoomParticipantWithRole(
	participants map[string]*diagnosisRoomParticipantAccumulator,
	subject string,
	role api.DiagnosisRoomParticipantSummaryRolesItem,
) *diagnosisRoomParticipantAccumulator {
	participant := participants[subject]
	if participant == nil {
		participant = &diagnosisRoomParticipantAccumulator{
			subject:  subject,
			roles:    map[api.DiagnosisRoomParticipantSummaryRolesItem]struct{}{},
			isSystem: diagnosisRoomSystemSubject(subject),
		}
		participants[subject] = participant
	}
	participant.roles[role] = struct{}{}
	return participant
}

func diagnosisRoomAddSupplementalEvidenceParticipants(
	participants map[string]*diagnosisRoomParticipantAccumulator,
	items []api.DiagnosisRoomSupplementalEvidenceSummary,
) {
	for _, item := range items {
		if item.ActorSubject == nil {
			continue
		}
		subject := strings.TrimSpace(*item.ActorSubject)
		if subject == "" {
			continue
		}
		participant := diagnosisRoomParticipantWithRole(participants, subject, api.DiagnosisRoomParticipantSummaryRolesItemSupplementalEvidence)
		participant.supplementalEvidenceCount++
	}
}

func diagnosisRoomParticipantRoles(
	roles map[api.DiagnosisRoomParticipantSummaryRolesItem]struct{},
) []api.DiagnosisRoomParticipantSummaryRolesItem {
	out := make([]api.DiagnosisRoomParticipantSummaryRolesItem, 0, len(roles))
	for role := range roles {
		out = append(out, role)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return diagnosisRoomParticipantRoleRank(out[i]) < diagnosisRoomParticipantRoleRank(out[j])
	})
	return out
}

func diagnosisRoomParticipantRoleRank(role api.DiagnosisRoomParticipantSummaryRolesItem) int {
	switch role {
	case api.DiagnosisRoomParticipantSummaryRolesItemOwner:
		return 0
	case api.DiagnosisRoomParticipantSummaryRolesItemMessage:
		return 1
	case api.DiagnosisRoomParticipantSummaryRolesItemEvidence:
		return 2
	case api.DiagnosisRoomParticipantSummaryRolesItemSupplementalEvidence:
		return 3
	case api.DiagnosisRoomParticipantSummaryRolesItemConfirmation:
		return 4
	case api.DiagnosisRoomParticipantSummaryRolesItemAssistant:
		return 5
	default:
		return 100
	}
}

func diagnosisRoomParticipantActivity(participant api.DiagnosisRoomParticipantSummary) int {
	activity := participant.MessageCount + participant.EvidenceCollectionCount + participant.SupplementalEvidenceCount
	if participant.ConfirmedConclusion {
		activity++
	}
	return activity
}

func diagnosisRoomSystemSubject(subject string) bool {
	return strings.HasPrefix(subject, "openclarion:") || strings.HasPrefix(subject, "openclarion.")
}

func (s *Server) withDiagnosisRoomWorkflowVisibility(
	ctx context.Context,
	items []api.DiagnosisRoomSummary,
) []api.DiagnosisRoomSummary {
	if s.roomVisibility == nil || len(items) == 0 {
		return items
	}
	requests := diagnosisRoomWorkflowVisibilityRequests(items)
	if len(requests) == 0 {
		return items
	}
	visibility, err := s.roomVisibility.ListDiagnosisRoomWorkflowVisibility(ctx, requests)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("diagnosis room workflow visibility lookup failed", "error", err)
		}
		return items
	}
	for i := range items {
		key := ports.DiagnosisRoomWorkflowVisibilityRequest{
			WorkflowID: strings.TrimSpace(items[i].WorkflowID),
			RunID:      strings.TrimSpace(items[i].RunID),
		}
		if value, ok := visibility[key]; ok {
			items[i].WorkflowVisibility = diagnosisRoomWorkflowVisibilitySummary(value)
		}
	}
	return items
}

func diagnosisRoomWorkflowVisibilityRequests(items []api.DiagnosisRoomSummary) []ports.DiagnosisRoomWorkflowVisibilityRequest {
	requests := make([]ports.DiagnosisRoomWorkflowVisibilityRequest, 0, len(items))
	seen := map[ports.DiagnosisRoomWorkflowVisibilityRequest]struct{}{}
	for _, item := range items {
		if !diagnosisRoomNeedsWorkflowVisibility(item) {
			continue
		}
		req := ports.DiagnosisRoomWorkflowVisibilityRequest{
			WorkflowID: strings.TrimSpace(item.WorkflowID),
			RunID:      strings.TrimSpace(item.RunID),
		}
		if req.WorkflowID == "" {
			continue
		}
		if _, ok := seen[req]; ok {
			continue
		}
		seen[req] = struct{}{}
		requests = append(requests, req)
	}
	return requests
}

func diagnosisRoomNeedsWorkflowVisibility(item api.DiagnosisRoomSummary) bool {
	return item.RoomStatus == api.Open || item.TaskStatus == api.DiagnosisTaskStatusRunning
}

func diagnosisRoomWorkflowVisibilitySummary(
	value ports.DiagnosisRoomWorkflowVisibility,
) *api.DiagnosisRoomWorkflowVisibility {
	return &api.DiagnosisRoomWorkflowVisibility{
		Status:           strings.TrimSpace(value.Status),
		TaskQueue:        optionalStringPtr(value.TaskQueue),
		StartTime:        value.StartTime,
		ExecutionTime:    value.ExecutionTime,
		CloseTime:        value.CloseTime,
		HistoryLength:    optionalPositiveInt64Ptr(value.HistoryLength),
		HistorySizeBytes: optionalPositiveInt64Ptr(value.HistorySizeBytes),
	}
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func optionalPositiveInt64Ptr(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func nonNilDiagnosisRoomSummaries(in []api.DiagnosisRoomSummary) []api.DiagnosisRoomSummary {
	if in == nil {
		return []api.DiagnosisRoomSummary{}
	}
	return in
}

type diagnosisConclusionBySnapshot map[domain.EvidenceSnapshotID]api.DiagnosisRoomConclusionSummary
type diagnosisProgressBySnapshot map[domain.EvidenceSnapshotID]api.DiagnosisRoomProgressSummary
type diagnosisRoomBySnapshot map[domain.EvidenceSnapshotID]api.DiagnosisRoomSummary

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
	Status                        string                                            `json:"status"`
	Source                        string                                            `json:"source"`
	Reason                        string                                            `json:"reason,omitempty"`
	EvidenceSnapshotID            int64                                             `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion             string                                            `json:"conclusion_version,omitempty"`
	ConfirmedBy                   string                                            `json:"confirmed_by,omitempty"`
	SupplementalContextRefs       []string                                          `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID               int64                                             `json:"assistant_turn_id,omitempty"`
	AssistantMessageID            string                                            `json:"assistant_message_id,omitempty"`
	AssistantSequence             int                                               `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt           *time.Time                                        `json:"assistant_occurred_at,omitempty"`
	Content                       string                                            `json:"content,omitempty"`
	Confidence                    string                                            `json:"confidence,omitempty"`
	RequiresHumanReview           *bool                                             `json:"requires_human_review,omitempty"`
	ConfidenceRationale           string                                            `json:"confidence_rationale,omitempty"`
	Findings                      []string                                          `json:"findings,omitempty"`
	RecommendedActions            []string                                          `json:"recommended_actions,omitempty"`
	EvidenceRequests              []diagnosisRoomEvidenceRequestPayload             `json:"evidence_requests,omitempty"`
	MissingEvidenceRequests       []diagnosisRoomConsultationEvidenceRequestPayload `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisRoomConsultationEvidenceRequestPayload `json:"evidence_collection_suggestions,omitempty"`
}

type diagnosisRoomSupplementalEvidenceEventPayload struct {
	Kind                 string                                   `json:"kind"`
	SessionID            string                                   `json:"session_id,omitempty"`
	ChatSessionID        int64                                    `json:"chat_session_id,omitempty"`
	DiagnosisTaskID      int64                                    `json:"diagnosis_task_id,omitempty"`
	ActorSubject         string                                   `json:"actor_subject,omitempty"`
	UserMessageID        string                                   `json:"user_message_id,omitempty"`
	AssistantMessageID   string                                   `json:"assistant_message_id,omitempty"`
	UserTurnID           int64                                    `json:"user_turn_id,omitempty"`
	AssistantTurnID      int64                                    `json:"assistant_turn_id,omitempty"`
	UserSequence         int                                      `json:"user_sequence,omitempty"`
	AssistantSequence    int                                      `json:"assistant_sequence,omitempty"`
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

type diagnosisRoomFailedEventPayload struct {
	Kind               string     `json:"kind"`
	SessionID          string     `json:"session_id,omitempty"`
	ChatSessionID      int64      `json:"chat_session_id,omitempty"`
	DiagnosisTaskID    int64      `json:"diagnosis_task_id,omitempty"`
	EvidenceSnapshotID int64      `json:"evidence_snapshot_id,omitempty"`
	Status             string     `json:"status,omitempty"`
	FailureReason      string     `json:"failure_reason,omitempty"`
	CloseReason        string     `json:"close_reason,omitempty"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
}

type diagnosisRoomEvidenceCollectedEventPayload struct {
	Kind                      string                                         `json:"kind"`
	SessionID                 string                                         `json:"session_id,omitempty"`
	ChatSessionID             int64                                          `json:"chat_session_id,omitempty"`
	DiagnosisTaskID           int64                                          `json:"diagnosis_task_id,omitempty"`
	UserMessageID             string                                         `json:"user_message_id,omitempty"`
	AssistantMessageID        string                                         `json:"assistant_message_id,omitempty"`
	UserTurnID                int64                                          `json:"user_turn_id,omitempty"`
	AssistantTurnID           int64                                          `json:"assistant_turn_id,omitempty"`
	UserSequence              int                                            `json:"user_sequence,omitempty"`
	AssistantSequence         int                                            `json:"assistant_sequence,omitempty"`
	ActorSubject              string                                         `json:"actor_subject,omitempty"`
	TurnCount                 int                                            `json:"turn_count,omitempty"`
	ContextRefs               []string                                       `json:"context_refs,omitempty"`
	EvidenceCollectionResults []diagnosisRoomEvidenceCollectionResultPayload `json:"evidence_collection_results,omitempty"`
}

type diagnosisRoomEvidenceCollectionResultPayload struct {
	TemplateID           int64      `json:"template_id,omitempty"`
	AlertSourceProfileID int64      `json:"alert_source_profile_id,omitempty"`
	AlertSourceKind      string     `json:"alert_source_kind,omitempty"`
	Tool                 string     `json:"tool"`
	Status               string     `json:"status"`
	ReasonCode           string     `json:"reason_code,omitempty"`
	Message              string     `json:"message,omitempty"`
	RequestReason        string     `json:"request_reason,omitempty"`
	Query                string     `json:"query,omitempty"`
	WindowSeconds        int        `json:"window_seconds,omitempty"`
	StepSeconds          int        `json:"step_seconds,omitempty"`
	Limit                int        `json:"limit,omitempty"`
	ObservedAlerts       *int       `json:"observed_alerts,omitempty"`
	ObservedMetricSeries *int       `json:"observed_metric_series,omitempty"`
	CollectedAt          *time.Time `json:"collected_at,omitempty"`
}

type diagnosisRoomConsultationInsightPayload struct {
	ConfidenceRationale           string                                            `json:"confidence_rationale,omitempty"`
	MissingEvidenceRequests       []diagnosisRoomConsultationEvidenceRequestPayload `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []diagnosisRoomConsultationEvidenceRequestPayload `json:"evidence_collection_suggestions,omitempty"`
	ConclusionStatus              string                                            `json:"conclusion_status,omitempty"`
}

type diagnosisRoomEvidenceRequestPayload struct {
	TemplateID           int64  `json:"template_id,omitempty"`
	AlertSourceProfileID int64  `json:"alert_source_profile_id,omitempty"`
	Tool                 string `json:"tool"`
	Reason               string `json:"reason"`
	Query                string `json:"query,omitempty"`
	WindowSeconds        int    `json:"window_seconds,omitempty"`
	StepSeconds          int    `json:"step_seconds,omitempty"`
	Limit                int    `json:"limit,omitempty"`
}

type diagnosisRoomConsultationEvidenceRequestPayload struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
}

type diagnosisRoomNotificationEventPayload struct {
	Kind                         string                                  `json:"kind"`
	SessionID                    string                                  `json:"session_id,omitempty"`
	ChatSessionID                int64                                   `json:"chat_session_id,omitempty"`
	DiagnosisTaskID              int64                                   `json:"diagnosis_task_id,omitempty"`
	OwnerSubject                 string                                  `json:"owner_subject,omitempty"`
	AssistantMessageID           string                                  `json:"assistant_message_id,omitempty"`
	AssistantTurnID              int64                                   `json:"assistant_turn_id,omitempty"`
	AssistantSequence            int                                     `json:"assistant_sequence,omitempty"`
	TurnCount                    int                                     `json:"turn_count,omitempty"`
	NotificationChannelProfileID int64                                   `json:"notification_channel_profile_id,omitempty"`
	ProviderMessageID            string                                  `json:"provider_message_id,omitempty"`
	ProviderStatus               string                                  `json:"provider_status,omitempty"`
	AssistantMessage             string                                  `json:"assistant_message,omitempty"`
	Confidence                   string                                  `json:"confidence,omitempty"`
	RequiresHumanReview          *bool                                   `json:"requires_human_review,omitempty"`
	RecommendedActions           []string                                `json:"recommended_actions,omitempty"`
	EvidenceRequests             []diagnosisRoomEvidenceRequestPayload   `json:"evidence_requests,omitempty"`
	ConsultationInsight          diagnosisRoomConsultationInsightPayload `json:"consultation_insight,omitempty"`
	FinalConclusion              diagnosisRoomConclusionPayload          `json:"final_conclusion,omitempty"`
}

func diagnosisStatesForSubReports(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	subReports []domain.SubReport,
) (diagnosisConclusionBySnapshot, diagnosisProgressBySnapshot, diagnosisRoomBySnapshot, error) {
	conclusions := diagnosisConclusionBySnapshot{}
	progress := diagnosisProgressBySnapshot{}
	rooms := diagnosisRoomBySnapshot{}
	if repo == nil || len(subReports) == 0 {
		return conclusions, progress, rooms, nil
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
			return nil, nil, nil, err
		}
		conclusion, ok, err := latestDiagnosisConclusion(ctx, repo, tasks)
		if err != nil {
			return nil, nil, nil, err
		}
		hasConclusion := ok
		if ok {
			conclusions[snapshotID] = conclusion
		}
		latestProgress, ok, err := latestDiagnosisProgress(ctx, repo, tasks)
		if err != nil {
			return nil, nil, nil, err
		}
		hasProgress := ok
		if ok {
			progress[snapshotID] = latestProgress
		}
		room, ok, err := exactDiagnosisRoomForReportState(ctx, repo, tasks, conclusion, hasConclusion, latestProgress, hasProgress)
		if err != nil {
			return nil, nil, nil, err
		}
		if ok {
			rooms[snapshotID] = room
		}
	}
	return conclusions, progress, rooms, nil
}

func (s *Server) filterDiagnosisReportStatesByReadAccess(
	w http.ResponseWriter,
	r *http.Request,
	principal domain.RBACPrincipal,
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
	rooms diagnosisRoomBySnapshot,
) (diagnosisConclusionBySnapshot, diagnosisProgressBySnapshot, diagnosisRoomBySnapshot, bool) {
	if len(conclusions) == 0 && len(progress) == 0 && len(rooms) == 0 {
		return conclusions, progress, rooms, true
	}
	hasGlobalRead, ok := s.authorizeResolvedLocalRBACPrincipalForScope(
		w,
		r,
		principal,
		domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACScopeKindGlobal,
		"",
		false,
	)
	if !ok {
		return nil, nil, nil, false
	}
	if hasGlobalRead {
		return conclusions, progress, rooms, true
	}

	filteredConclusions := diagnosisConclusionBySnapshot{}
	filteredProgress := diagnosisProgressBySnapshot{}
	filteredRooms := diagnosisRoomBySnapshot{}
	allowedBySession := map[string]bool{}
	for _, snapshotID := range diagnosisReportStateSnapshotIDs(conclusions, progress, rooms) {
		conclusion, hasConclusion := conclusions[snapshotID]
		latestProgress, hasProgress := progress[snapshotID]
		room, hasRoom := rooms[snapshotID]
		sessionID := diagnosisReportStateSessionID(conclusion, hasConclusion, latestProgress, hasProgress, room, hasRoom)
		allowed, ok := s.diagnosisRoomReadAllowedBySession(w, r, principal, sessionID, allowedBySession)
		if !ok {
			return nil, nil, nil, false
		}
		if !allowed {
			continue
		}
		if hasConclusion {
			filteredConclusions[snapshotID] = conclusion
		}
		if hasProgress {
			filteredProgress[snapshotID] = latestProgress
		}
		if hasRoom {
			filteredRooms[snapshotID] = room
		}
	}
	return filteredConclusions, filteredProgress, filteredRooms, true
}

func diagnosisReportStateSnapshotIDs(
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
	rooms diagnosisRoomBySnapshot,
) []domain.EvidenceSnapshotID {
	seen := map[domain.EvidenceSnapshotID]struct{}{}
	for snapshotID := range conclusions {
		seen[snapshotID] = struct{}{}
	}
	for snapshotID := range progress {
		seen[snapshotID] = struct{}{}
	}
	for snapshotID := range rooms {
		seen[snapshotID] = struct{}{}
	}
	ids := make([]domain.EvidenceSnapshotID, 0, len(seen))
	for snapshotID := range seen {
		ids = append(ids, snapshotID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

func diagnosisReportStateSessionID(
	conclusion api.DiagnosisRoomConclusionSummary,
	hasConclusion bool,
	progress api.DiagnosisRoomProgressSummary,
	hasProgress bool,
	room api.DiagnosisRoomSummary,
	hasRoom bool,
) string {
	if hasRoom && strings.TrimSpace(room.SessionID) != "" {
		return room.SessionID
	}
	sessionID, _ := reportStateRoomIdentity(conclusion, hasConclusion, progress, hasProgress)
	if sessionID != "" {
		return sessionID
	}
	if hasConclusion {
		return conclusion.SessionID
	}
	if hasProgress && progress.SessionID != nil {
		return *progress.SessionID
	}
	return ""
}

func exactDiagnosisRoomForReportState(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	tasks []domain.DiagnosisTask,
	conclusion api.DiagnosisRoomConclusionSummary,
	hasConclusion bool,
	progress api.DiagnosisRoomProgressSummary,
	hasProgress bool,
) (api.DiagnosisRoomSummary, bool, error) {
	sessionID, taskID := reportStateRoomIdentity(conclusion, hasConclusion, progress, hasProgress)
	if sessionID == "" || taskID == 0 {
		return api.DiagnosisRoomSummary{}, false, nil
	}
	task, ok := diagnosisTaskByID(tasks, taskID)
	if !ok {
		return api.DiagnosisRoomSummary{}, false, nil
	}
	session, err := repo.FindChatSessionByKey(ctx, sessionID)
	if errors.Is(err, domain.ErrNotFound) {
		return api.DiagnosisRoomSummary{}, false, nil
	}
	if err != nil {
		return api.DiagnosisRoomSummary{}, false, err
	}
	if session.DiagnosisTaskID != task.ID {
		return api.DiagnosisRoomSummary{}, false, nil
	}
	rooms, err := diagnosisRoomSummariesWithConclusionsAndNotifications(ctx, repo, []domain.ChatSessionWithTask{{
		Session: session,
		Task:    task,
	}})
	if err != nil {
		return api.DiagnosisRoomSummary{}, false, err
	}
	if len(rooms) == 0 {
		return api.DiagnosisRoomSummary{}, false, nil
	}
	return rooms[0], true, nil
}

func reportStateRoomIdentity(
	conclusion api.DiagnosisRoomConclusionSummary,
	hasConclusion bool,
	progress api.DiagnosisRoomProgressSummary,
	hasProgress bool,
) (string, domain.DiagnosisTaskID) {
	if reportProgressIsNewerThanConclusion(conclusion, hasConclusion, progress, hasProgress) {
		return progressRoomIdentity(progress)
	}
	return conclusionRoomIdentity(conclusion, hasConclusion)
}

func reportProgressIsNewerThanConclusion(
	conclusion api.DiagnosisRoomConclusionSummary,
	hasConclusion bool,
	progress api.DiagnosisRoomProgressSummary,
	hasProgress bool,
) bool {
	if !hasProgress {
		return false
	}
	if !hasConclusion {
		return true
	}
	progressRecordedAt := reportProgressRecordedAt(progress)
	if progressRecordedAt.IsZero() {
		return false
	}
	if conclusion.RecordedAt.IsZero() {
		return true
	}
	return progressRecordedAt.After(conclusion.RecordedAt)
}

func reportProgressRecordedAt(progress api.DiagnosisRoomProgressSummary) time.Time {
	if !progress.RecordedAt.IsZero() {
		return progress.RecordedAt
	}
	return progress.OccurredAt
}

func conclusionRoomIdentity(
	conclusion api.DiagnosisRoomConclusionSummary,
	hasConclusion bool,
) (string, domain.DiagnosisTaskID) {
	if hasConclusion && conclusion.SessionID != "" && conclusion.DiagnosisTaskID > 0 {
		return conclusion.SessionID, domain.DiagnosisTaskID(conclusion.DiagnosisTaskID)
	}
	return "", 0
}

func progressRoomIdentity(progress api.DiagnosisRoomProgressSummary) (string, domain.DiagnosisTaskID) {
	if progress.SessionID != nil && *progress.SessionID != "" && progress.DiagnosisTaskID > 0 {
		return *progress.SessionID, domain.DiagnosisTaskID(progress.DiagnosisTaskID)
	}
	return "", 0
}

func diagnosisTaskByID(tasks []domain.DiagnosisTask, taskID domain.DiagnosisTaskID) (domain.DiagnosisTask, bool) {
	for _, task := range tasks {
		if task.ID == taskID {
			return task, true
		}
	}
	return domain.DiagnosisTask{}, false
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

func latestDiagnosisProgress(ctx context.Context, repo ports.DiagnosisRepository, tasks []domain.DiagnosisTask) (api.DiagnosisRoomProgressSummary, bool, error) {
	var best api.DiagnosisRoomProgressSummary
	bestSet := false
	for _, task := range tasks {
		progress, ok, err := latestDiagnosisProgressForTask(ctx, repo, task)
		if err != nil {
			return api.DiagnosisRoomProgressSummary{}, false, err
		}
		if !ok {
			continue
		}
		if !bestSet ||
			progress.OccurredAt.After(best.OccurredAt) ||
			(progress.OccurredAt.Equal(best.OccurredAt) && progress.RecordedAt.After(best.RecordedAt)) {
			best = progress
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
	best, bestOccurred, bestSet, err := latestDiagnosisConclusionEventForTask(ctx, repo, taskID)
	if err != nil {
		return api.DiagnosisRoomConclusionSummary{}, time.Time{}, false, err
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

func latestDiagnosisProgressForTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	task domain.DiagnosisTask,
) (api.DiagnosisRoomProgressSummary, bool, error) {
	if task.Status == domain.DiagnosisStatusFailed {
		progress, ok, err := failedDiagnosisProgressForTask(ctx, repo, task)
		if err != nil {
			return api.DiagnosisRoomProgressSummary{}, false, err
		}
		if ok {
			return progress, true, nil
		}
	}

	events, err := repo.ListEventsByTaskAndKind(ctx, task.ID, diagnosisConclusionEventTurnPersisted, diagnosisConfidenceTimelineLimit)
	if err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, err
	}
	if len(events) == 0 {
		return api.DiagnosisRoomProgressSummary{}, false, nil
	}
	collectionResults, err := evidenceCollectionResultsForDiagnosisTask(ctx, repo, task.ID)
	if err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, err
	}

	items := make([]api.DiagnosisRoomConfidenceTimelineEntry, 0, len(events))
	var latestEvent domain.DiagnosisTaskEvent
	var latestItem api.DiagnosisRoomConfidenceTimelineEntry
	latestSet := false
	for i := len(events) - 1; i >= 0; i-- {
		item, ok, err := confidenceTimelineEntryFromDiagnosisEvent(events[i])
		if err != nil {
			return api.DiagnosisRoomProgressSummary{}, false, err
		}
		if !ok {
			continue
		}
		item.EvidenceCollectionResults = evidenceCollectionResultsForTimelineEntry(collectionResults, item)
		items = append(items, item)
		event := events[i]
		if !latestSet ||
			item.OccurredAt.After(latestItem.OccurredAt) ||
			(item.OccurredAt.Equal(latestItem.OccurredAt) && event.RecordedAt.After(latestEvent.RecordedAt)) {
			latestEvent = event
			latestItem = item
			latestSet = true
		}
	}
	if !latestSet {
		return api.DiagnosisRoomProgressSummary{}, false, nil
	}

	supplementalEvidence, err := supplementalEvidenceForDiagnosisTask(ctx, repo, task.ID)
	if err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, err
	}
	sessionID, chatSessionID, err := diagnosisRoomTurnIdentityFromEvent(latestEvent)
	if err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, err
	}
	progress := api.DiagnosisRoomProgressSummary{
		DiagnosisTaskID:               int64(task.ID),
		SessionID:                     nonEmptyStringPtr(sessionID),
		ChatSessionID:                 nonZeroInt64Ptr(chatSessionID),
		EventKind:                     latestItem.EventKind,
		Status:                        string(api.DiagnosisRoomProgressSummaryStatusInProgress),
		EvidenceSnapshotID:            int64(task.EvidenceSnapshotID),
		Confidence:                    latestItem.Confidence,
		RequiresHumanReview:           latestItem.RequiresHumanReview,
		ConclusionStatus:              latestItem.ConclusionStatus,
		ConfidenceRationale:           latestItem.ConfidenceRationale,
		EvidenceRequestCount:          latestItem.EvidenceRequestCount,
		EvidenceRequests:              latestItem.EvidenceRequests,
		EvidenceCollectionResults:     latestItem.EvidenceCollectionResults,
		MissingEvidenceRequests:       latestItem.MissingEvidenceRequests,
		EvidenceCollectionSuggestions: latestItem.EvidenceCollectionSuggestions,
		SupplementalEvidence:          supplementalEvidence,
		ConfidenceTimeline:            items,
		AssistantMessageID:            latestItem.AssistantMessageID,
		AssistantTurnID:               latestItem.AssistantTurnID,
		AssistantSequence:             latestItem.AssistantSequence,
		TurnCount:                     latestItem.TurnCount,
		OccurredAt:                    latestItem.OccurredAt,
		RecordedAt:                    latestEvent.RecordedAt,
	}
	return progress, true, nil
}

func failedDiagnosisProgressForTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	task domain.DiagnosisTask,
) (api.DiagnosisRoomProgressSummary, bool, error) {
	events, err := repo.ListEventsByTaskAndKind(ctx, task.ID, diagnosisConclusionEventFailed, 1)
	if err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, err
	}
	if len(events) > 0 {
		progress, ok, err := failedDiagnosisProgressFromEvent(task, events[0])
		if err != nil {
			return api.DiagnosisRoomProgressSummary{}, false, err
		}
		if ok {
			return progress, true, nil
		}
	}
	return failedDiagnosisProgressFromTask(task)
}

func failedDiagnosisProgressFromEvent(
	task domain.DiagnosisTask,
	event domain.DiagnosisTaskEvent,
) (api.DiagnosisRoomProgressSummary, bool, error) {
	if len(event.Payload) == 0 {
		return api.DiagnosisRoomProgressSummary{}, false, fmt.Errorf("diagnosis failed event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, fmt.Errorf("diagnosis failed event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomFailedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return api.DiagnosisRoomProgressSummary{}, false, fmt.Errorf("diagnosis failed event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return api.DiagnosisRoomProgressSummary{}, false, fmt.Errorf("diagnosis failed event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return api.DiagnosisRoomProgressSummary{}, false, fmt.Errorf("diagnosis failed event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.Status != "" && payload.Status != string(domain.DiagnosisStatusFailed) {
		return api.DiagnosisRoomProgressSummary{}, false, nil
	}
	failureReason := strings.TrimSpace(payload.FailureReason)
	if failureReason == "" {
		failureReason = strings.TrimSpace(task.FailureReason)
	}
	if failureReason == "" {
		return api.DiagnosisRoomProgressSummary{}, false, nil
	}
	occurredAt := event.OccurredAt
	if payload.ClosedAt != nil {
		occurredAt = *payload.ClosedAt
	}
	if occurredAt.IsZero() {
		occurredAt = event.RecordedAt
	}
	recordedAt := event.RecordedAt
	if recordedAt.IsZero() {
		recordedAt = occurredAt
	}
	evidenceSnapshotID := int64(task.EvidenceSnapshotID)
	if payload.EvidenceSnapshotID > 0 {
		evidenceSnapshotID = payload.EvidenceSnapshotID
	}
	return api.DiagnosisRoomProgressSummary{
		DiagnosisTaskID:      int64(task.ID),
		SessionID:            nonEmptyStringPtr(payload.SessionID),
		ChatSessionID:        nonZeroInt64Ptr(payload.ChatSessionID),
		EventKind:            event.Kind,
		Status:               string(api.DiagnosisRoomProgressSummaryStatusFailed),
		EvidenceSnapshotID:   evidenceSnapshotID,
		Confidence:           api.ReportConfidenceLow,
		RequiresHumanReview:  true,
		FailureReason:        nonEmptyStringPtr(failureReason),
		EvidenceRequestCount: 0,
		OccurredAt:           occurredAt,
		RecordedAt:           recordedAt,
	}, true, nil
}

func failedDiagnosisProgressFromTask(task domain.DiagnosisTask) (api.DiagnosisRoomProgressSummary, bool, error) {
	failureReason := strings.TrimSpace(task.FailureReason)
	if task.Status != domain.DiagnosisStatusFailed || task.FinishedAt == nil || failureReason == "" {
		return api.DiagnosisRoomProgressSummary{}, false, nil
	}
	recordedAt := task.UpdatedAt
	if recordedAt.IsZero() {
		recordedAt = *task.FinishedAt
	}
	return api.DiagnosisRoomProgressSummary{
		DiagnosisTaskID:      int64(task.ID),
		EventKind:            diagnosisConclusionEventFailed,
		Status:               string(api.DiagnosisRoomProgressSummaryStatusFailed),
		EvidenceSnapshotID:   int64(task.EvidenceSnapshotID),
		Confidence:           api.ReportConfidenceLow,
		RequiresHumanReview:  true,
		FailureReason:        nonEmptyStringPtr(failureReason),
		EvidenceRequestCount: 0,
		OccurredAt:           *task.FinishedAt,
		RecordedAt:           recordedAt,
	}, true, nil
}

func latestDiagnosisConclusionEventForTask(
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
		ConfidenceRationale:     nonEmptyStringPtr(conclusion.ConfidenceRationale),
		Findings:                nonEmptyStringSlice(conclusion.Findings),
		RecommendedActions:      nonEmptyStringSlice(conclusion.RecommendedActions),
		EvidenceRequests:        diagnosisRoomEvidenceRequestSummaries(conclusion.EvidenceRequests),
		MissingEvidenceRequests: diagnosisRoomConsultationEvidenceRequestSummaries(
			conclusion.MissingEvidenceRequests,
		),
		EvidenceCollectionSuggestions: diagnosisRoomConsultationEvidenceRequestSummaries(
			conclusion.EvidenceCollectionSuggestions,
		),
		RecordedAt: event.RecordedAt,
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

func notificationTimelineForDiagnosisTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) ([]api.DiagnosisRoomNotificationTimelineEntry, error) {
	kinds := []string{
		diagnosisConclusionEventAssistantTurnNotification,
		diagnosisConclusionEventFinalReadyNotification,
		diagnosisConclusionEventCloseNotification,
	}
	items := make([]api.DiagnosisRoomNotificationTimelineEntry, 0, len(kinds))
	for _, kind := range kinds {
		events, err := repo.ListEventsByTaskAndKind(ctx, taskID, kind, diagnosisNotificationTimelineLimit)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			item, ok, err := notificationTimelineEntryFromDiagnosisEvent(event)
			if err != nil {
				return nil, err
			}
			if ok {
				items = append(items, item)
			}
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OccurredAt.Before(items[j].OccurredAt)
	})
	if len(items) > diagnosisNotificationTimelineLimit {
		items = items[len(items)-diagnosisNotificationTimelineLimit:]
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
	collectionResults, err := evidenceCollectionResultsForDiagnosisTask(ctx, repo, taskID)
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
			item.EvidenceCollectionResults = evidenceCollectionResultsForTimelineEntry(collectionResults, item)
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

func evidenceCollectionResultsForDiagnosisTask(
	ctx context.Context,
	repo ports.DiagnosisRepository,
	taskID domain.DiagnosisTaskID,
) (map[string][]api.DiagnosisRoomEvidenceCollectionResultSummary, error) {
	events, err := repo.ListEventsByTaskAndKind(ctx, taskID, diagnosisConclusionEventEvidenceCollected, diagnosisConfidenceTimelineLimit)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]api.DiagnosisRoomEvidenceCollectionResultSummary)
	for i := len(events) - 1; i >= 0; i-- {
		keys, results, ok, err := evidenceCollectionResultsFromDiagnosisEvent(events[i])
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, key := range keys {
			if key != "" {
				out[key] = append(out[key], results...)
			}
		}
	}
	return out, nil
}

func evidenceCollectionResultsForTimelineEntry(
	collectionResults map[string][]api.DiagnosisRoomEvidenceCollectionResultSummary,
	item api.DiagnosisRoomConfidenceTimelineEntry,
) []api.DiagnosisRoomEvidenceCollectionResultSummary {
	if len(collectionResults) == 0 {
		return nil
	}
	if item.AssistantMessageID != nil {
		if results := collectionResults[evidenceCollectionAssistantMessageKey(*item.AssistantMessageID)]; len(results) > 0 {
			return results
		}
	}
	if item.TurnCount != nil {
		if results := collectionResults[evidenceCollectionTurnKey(*item.TurnCount)]; len(results) > 0 {
			return results
		}
	}
	return nil
}

func notificationTimelineEntryFromDiagnosisEvent(event domain.DiagnosisTaskEvent) (api.DiagnosisRoomNotificationTimelineEntry, bool, error) {
	if len(event.Payload) == 0 {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, fmt.Errorf("diagnosis notification timeline event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, fmt.Errorf("diagnosis notification timeline event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomNotificationEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, fmt.Errorf("diagnosis notification timeline event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, fmt.Errorf("diagnosis notification timeline event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, fmt.Errorf("diagnosis notification timeline event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	providerStatus := strings.TrimSpace(payload.ProviderStatus)
	if providerStatus == "" {
		return api.DiagnosisRoomNotificationTimelineEntry{}, false, nil
	}
	confidence := reportConfidencePtr(payload.Confidence)
	if confidence == nil {
		confidence = reportConfidencePtr(payload.FinalConclusion.Confidence)
	}
	requiresHumanReview := copyBoolPtr(payload.RequiresHumanReview)
	if requiresHumanReview == nil {
		requiresHumanReview = copyBoolPtr(payload.FinalConclusion.RequiresHumanReview)
	}
	contentProof := diagnosisRoomNotificationContentProof(payload)
	occurredAt := event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = event.RecordedAt
	}
	return api.DiagnosisRoomNotificationTimelineEntry{
		EventKind:                    event.Kind,
		NotificationChannelProfileID: nonZeroInt64Ptr(payload.NotificationChannelProfileID),
		ProviderStatus:               providerStatus,
		ProviderMessageID:            nonEmptyStringPtr(payload.ProviderMessageID),
		AssistantMessageID:           nonEmptyStringPtr(payload.AssistantMessageID),
		AssistantTurnID:              nonZeroInt64Ptr(payload.AssistantTurnID),
		AssistantSequence:            nonZeroIntPtr(payload.AssistantSequence),
		TurnCount:                    nonZeroIntPtr(payload.TurnCount),
		Confidence:                   confidence,
		RequiresHumanReview:          requiresHumanReview,
		ContentKind:                  nonEmptyStringPtr(contentProof.kind),
		ContentSha256:                nonEmptyStringPtr(contentProof.sha256),
		RecommendedActionCount:       nonZeroIntPtr(contentProof.recommendedActionCount),
		EvidenceRequestCount:         nonZeroIntPtr(contentProof.evidenceRequestCount),
		OccurredAt:                   occurredAt,
	}, true, nil
}

type diagnosisRoomNotificationContentProofResult struct {
	kind                   string
	sha256                 string
	recommendedActionCount int
	evidenceRequestCount   int
}

func diagnosisRoomNotificationContentProof(payload diagnosisRoomNotificationEventPayload) diagnosisRoomNotificationContentProofResult {
	switch payload.Kind {
	case diagnosisConclusionEventAssistantTurnNotification:
		return newDiagnosisRoomNotificationContentProof(
			"assistant_message",
			payload.AssistantMessage,
			len(payload.RecommendedActions),
			len(payload.EvidenceRequests)+len(payload.ConsultationInsight.MissingEvidenceRequests),
		)
	case diagnosisConclusionEventFinalReadyNotification, diagnosisConclusionEventCloseNotification:
		conclusion := payload.FinalConclusion
		return newDiagnosisRoomNotificationContentProof(
			"final_conclusion",
			conclusion.Content,
			len(conclusion.RecommendedActions),
			len(conclusion.EvidenceRequests)+len(conclusion.MissingEvidenceRequests),
		)
	default:
		return diagnosisRoomNotificationContentProofResult{}
	}
}

func newDiagnosisRoomNotificationContentProof(kind, content string, recommendedActionCount, evidenceRequestCount int) diagnosisRoomNotificationContentProofResult {
	content = strings.TrimSpace(content)
	if content == "" {
		return diagnosisRoomNotificationContentProofResult{}
	}
	sum := sha256.Sum256([]byte(content))
	return diagnosisRoomNotificationContentProofResult{
		kind:                   kind,
		sha256:                 hex.EncodeToString(sum[:]),
		recommendedActionCount: recommendedActionCount,
		evidenceRequestCount:   evidenceRequestCount,
	}
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

func diagnosisRoomTurnIdentityFromEvent(event domain.DiagnosisTaskEvent) (string, int64, error) {
	if len(event.Payload) == 0 {
		return "", 0, fmt.Errorf("diagnosis progress event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return "", 0, fmt.Errorf("diagnosis progress event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomTurnPersistedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", 0, fmt.Errorf("diagnosis progress event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return "", 0, fmt.Errorf("diagnosis progress event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return "", 0, fmt.Errorf("diagnosis progress event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	return strings.TrimSpace(payload.SessionID), payload.ChatSessionID, nil
}

func evidenceCollectionResultsFromDiagnosisEvent(
	event domain.DiagnosisTaskEvent,
) ([]string, []api.DiagnosisRoomEvidenceCollectionResultSummary, bool, error) {
	if len(event.Payload) == 0 {
		return nil, nil, false, fmt.Errorf("diagnosis evidence collected event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return nil, nil, false, fmt.Errorf("diagnosis evidence collected event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomEvidenceCollectedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, nil, false, fmt.Errorf("diagnosis evidence collected event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return nil, nil, false, fmt.Errorf("diagnosis evidence collected event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return nil, nil, false, fmt.Errorf("diagnosis evidence collected event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	results := diagnosisRoomEvidenceCollectionResultSummaries(payload.EvidenceCollectionResults, event.OccurredAt)
	if len(results) == 0 {
		return nil, nil, false, nil
	}
	keys := evidenceCollectionResultKeys(payload.AssistantMessageID, payload.TurnCount)
	if len(keys) == 0 {
		return nil, nil, false, nil
	}
	return keys, results, true, nil
}

func evidenceCollectionActorFromDiagnosisEvent(event domain.DiagnosisTaskEvent) (string, bool, error) {
	if len(event.Payload) == 0 {
		return "", false, fmt.Errorf("diagnosis evidence collected event %d has empty payload: %w", event.ID, domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(event.Payload); err != nil {
		return "", false, fmt.Errorf("diagnosis evidence collected event %d payload is ambiguous: %w", event.ID, err)
	}
	var payload diagnosisRoomEvidenceCollectedEventPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false, fmt.Errorf("diagnosis evidence collected event %d payload: %w", event.ID, err)
	}
	if payload.Kind != "" && payload.Kind != event.Kind {
		return "", false, fmt.Errorf("diagnosis evidence collected event %d kind mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	if payload.DiagnosisTaskID != 0 && domain.DiagnosisTaskID(payload.DiagnosisTaskID) != event.TaskID {
		return "", false, fmt.Errorf("diagnosis evidence collected event %d task mismatch: %w", event.ID, domain.ErrInvariantViolation)
	}
	actor := strings.TrimSpace(payload.ActorSubject)
	if actor == "" {
		return "", false, nil
	}
	return actor, true, nil
}

func diagnosisRoomEvidenceCollectionResultSummaries(
	in []diagnosisRoomEvidenceCollectionResultPayload,
	fallbackCollectedAt time.Time,
) []api.DiagnosisRoomEvidenceCollectionResultSummary {
	if len(in) == 0 {
		return nil
	}
	out := make([]api.DiagnosisRoomEvidenceCollectionResultSummary, 0, len(in))
	for _, item := range in {
		tool := strings.TrimSpace(item.Tool)
		status := strings.TrimSpace(item.Status)
		collectedAt := item.CollectedAt
		if collectedAt == nil && !fallbackCollectedAt.IsZero() {
			collectedAt = &fallbackCollectedAt
		}
		if tool == "" || status == "" || collectedAt == nil || collectedAt.IsZero() {
			continue
		}
		out = append(out, api.DiagnosisRoomEvidenceCollectionResultSummary{
			Tool:                 tool,
			Status:               status,
			ReasonCode:           nonEmptyStringPtr(item.ReasonCode),
			Message:              nonEmptyStringPtr(item.Message),
			RequestReason:        nonEmptyStringPtr(item.RequestReason),
			Query:                nonEmptyStringPtr(item.Query),
			TemplateID:           nonZeroInt64Ptr(item.TemplateID),
			AlertSourceProfileID: nonZeroInt64Ptr(item.AlertSourceProfileID),
			AlertSourceKind:      nonEmptyStringPtr(item.AlertSourceKind),
			WindowSeconds:        nonZeroIntPtr(item.WindowSeconds),
			StepSeconds:          nonZeroIntPtr(item.StepSeconds),
			Limit:                nonZeroIntPtr(item.Limit),
			ObservedAlerts:       copyIntPtr(item.ObservedAlerts),
			ObservedMetricSeries: copyIntPtr(item.ObservedMetricSeries),
			CollectedAt:          *collectedAt,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func evidenceCollectionResultKeys(assistantMessageID string, turnCount int) []string {
	keys := make([]string, 0, 2)
	if key := evidenceCollectionAssistantMessageKey(assistantMessageID); key != "" {
		keys = append(keys, key)
	}
	if key := evidenceCollectionTurnKey(turnCount); key != "" {
		keys = append(keys, key)
	}
	return keys
}

func evidenceCollectionAssistantMessageKey(assistantMessageID string) string {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if assistantMessageID == "" {
		return ""
	}
	return "assistant:" + assistantMessageID
}

func evidenceCollectionTurnKey(turnCount int) string {
	if turnCount <= 0 {
		return ""
	}
	return fmt.Sprintf("turn:%d", turnCount)
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
			Tool:                 tool,
			Reason:               reason,
			Query:                nonEmptyStringPtr(req.Query),
			TemplateID:           nonZeroInt64Ptr(req.TemplateID),
			AlertSourceProfileID: nonZeroInt64Ptr(req.AlertSourceProfileID),
			WindowSeconds:        nonZeroIntPtr(req.WindowSeconds),
			StepSeconds:          nonZeroIntPtr(req.StepSeconds),
			Limit:                nonZeroIntPtr(req.Limit),
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
		ActorSubject:       nonEmptyStringPtr(payload.ActorSubject),
		ContextRefs:        nonEmptyStringSlice(payload.ContextRefs),
		UserMessageID:      nonEmptyStringPtr(payload.UserMessageID),
		AssistantMessageID: nonEmptyStringPtr(payload.AssistantMessageID),
		UserTurnID:         nonZeroInt64Ptr(payload.UserTurnID),
		AssistantTurnID:    nonZeroInt64Ptr(payload.AssistantTurnID),
		UserSequence:       nonZeroIntPtr(payload.UserSequence),
		AssistantSequence:  nonZeroIntPtr(payload.AssistantSequence),
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

func copyIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
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

func finalReportDetail(
	report domain.FinalReport,
	subReports []domain.SubReport,
	notificationDeliveries []domain.ReportNotificationDelivery,
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
	rooms diagnosisRoomBySnapshot,
) (api.FinalReportDetail, error) {
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
	linked, err := subReportDetails(subReports, conclusions, progress, rooms)
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
		NotificationDeliveries: reportNotificationDeliveryProofs(
			notificationDeliveries,
		),
		FinalNotificationReadiness: reportFinalNotificationReadiness(
			report.ID,
			subReports,
			conclusions,
			progress,
		),
		Content:           content,
		Model:             report.Model,
		OutputMode:        report.OutputMode,
		CreatedByWorkflow: report.CreatedByWorkflow,
		CreatedAt:         report.CreatedAt,
		LinkedSubReports:  linked,
	}, nil
}

func reportNotificationDeliveryProofs(deliveries []domain.ReportNotificationDelivery) []api.ReportNotificationDeliveryProof {
	items := make([]api.ReportNotificationDeliveryProof, len(deliveries))
	for i, delivery := range deliveries {
		items[i] = api.ReportNotificationDeliveryProof{
			ID:                                 int64(delivery.ID),
			IdempotencyKey:                     delivery.IdempotencyKey,
			NotificationPurpose:                reportNotificationDeliveryPurpose(delivery),
			ReportNotificationChannelProfileID: nullableReportNotificationChannelProfileID(delivery.ReportNotificationChannelProfileID),
			Status:                             api.ReportNotificationDeliveryStatus(delivery.Status),
			CreatedAt:                          delivery.CreatedAt,
			UpdatedAt:                          delivery.UpdatedAt,
		}
		if delivery.ProviderMessageID != "" {
			items[i].ProviderMessageID = &delivery.ProviderMessageID
		}
		if delivery.ProviderStatus != "" {
			items[i].ProviderStatus = &delivery.ProviderStatus
		}
		if delivery.FailureReason != "" {
			items[i].FailureReason = &delivery.FailureReason
		}
		if delivery.DeliveredAt != nil {
			items[i].DeliveredAt = delivery.DeliveredAt
		}
	}
	return items
}

func reportNotificationDeliveryPurpose(delivery domain.ReportNotificationDelivery) api.ReportNotificationPurpose {
	switch {
	case strings.HasSuffix(delivery.IdempotencyKey, "/notification/final"):
		return api.Final
	case strings.HasSuffix(delivery.IdempotencyKey, "/notification/handoff"):
		return api.Handoff
	default:
		return api.Handoff
	}
}

func subReportDetails(
	reports []domain.SubReport,
	conclusions diagnosisConclusionBySnapshot,
	progress diagnosisProgressBySnapshot,
	rooms diagnosisRoomBySnapshot,
) ([]api.SubReportDetail, error) {
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
		if diagnosisProgress, ok := progress[report.EvidenceSnapshotID]; ok {
			item.DiagnosisProgress = &diagnosisProgress
		}
		if room, ok := rooms[report.EvidenceSnapshotID]; ok {
			item.DiagnosisRoom = &room
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
