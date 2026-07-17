package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	imwebhook "github.com/openclarion/openclarion/internal/providers/im/webhook"
	openaillm "github.com/openclarion/openclarion/internal/providers/llm/openai"
	metricsalertmanager "github.com/openclarion/openclarion/internal/providers/metrics/alertmanager"
	transporthttp "github.com/openclarion/openclarion/internal/transport/http"
	"github.com/openclarion/openclarion/internal/usecases/alertsourceprovider"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroomstart"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportpolicytrigger"
)

const (
	e2eCoreReportWorkflowID   = "report-batch-e2e-core-flow"
	e2eCoreCorrelationKey     = "e2e-core-flow"
	e2eDiagnosisAssessMessage = "e2e-diagnosis-assess"
	e2eDiagnosisReadyMessage  = "e2e-diagnosis-ready"
)

var e2eCoreWindowStart = time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

func TestReportToDiagnosisCoreFlowEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), e2eTestTimeout)
	defer cancel()

	factory, closeDB := startE2EDatabase(ctx, t)
	defer closeDB()

	alertmanagerServer := newE2EAlertmanagerServer(t)
	defer alertmanagerServer.Close()

	llmServer := newE2ELLMServer(t)
	defer llmServer.Close()

	webhook := newRecordingWebhookServer(t)
	defer webhook.Close()

	configuration := seedE2ECoreConfiguration(ctx, t, factory, alertmanagerServer.URL)
	providers := newE2EAlertSourceProviderBuilder(t)

	llmProvider, err := openaillm.NewProviderWithCapabilityDetection(ctx, openaillm.Config{
		BaseURL: llmServer.URL + "/v1",
		Model:   "gpt-e2e",
	})
	if err != nil {
		t.Fatalf("New OpenAI-compatible provider: %v", err)
	}
	imProvider, err := imwebhook.NewProvider(imwebhook.Config{URL: webhook.URL})
	if err != nil {
		t.Fatalf("New webhook provider: %v", err)
	}
	notificationResolver := newE2ENotificationProviderResolver(imProvider)
	containerProvider := &e2eDiagnosisContainerProvider{}

	devServerStartCtx, cancelDevServerStart := context.WithTimeout(ctx, e2eTemporalDevServerStartBudget)
	defer cancelDevServerStart()
	devServer, err := testsuite.StartDevServer(devServerStartCtx, testsuite.DevServerOptions{
		LogLevel: "error",
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})
	if err != nil {
		t.Fatalf("start temporal dev server: %v", err)
	}
	defer func() {
		if err := devServer.Stop(); err != nil {
			t.Fatalf("stop temporal dev server: %v", err)
		}
	}()
	tc := devServer.Client()
	defer tc.Close()

	worker := temporalpkg.NewWorker(
		tc,
		factory,
		temporalpkg.WithLLMProvider(llmProvider),
		temporalpkg.WithNotificationChannelProviderResolver(notificationResolver),
		temporalpkg.WithContainerProvider(containerProvider),
	)
	if err := worker.Start(); err != nil {
		t.Fatalf("start temporal worker: %v", err)
	}
	defer worker.Stop()

	reportStarter, err := temporalpkg.NewReportStarter(tc)
	if err != nil {
		t.Fatalf("NewReportStarter: %v", err)
	}
	policyTrigger, err := reportpolicytrigger.NewService(factory, reportStarter, providers)
	if err != nil {
		t.Fatalf("New report policy trigger: %v", err)
	}
	roomWorkflowStarter, err := temporalpkg.NewDiagnosisRoomStarter(tc)
	if err != nil {
		t.Fatalf("NewDiagnosisRoomStarter: %v", err)
	}
	roomStartService, err := diagnosisroomstart.NewService(factory, roomWorkflowStarter)
	if err != nil {
		t.Fatalf("New diagnosis room start service: %v", err)
	}
	roomClient, err := temporalpkg.NewDiagnosisRoomClient(tc)
	if err != nil {
		t.Fatalf("NewDiagnosisRoomClient: %v", err)
	}
	handler := e2eHTTPHandlerWithOptions(
		factory,
		transporthttp.WithReportWorkflowPolicyReplayTrigger(policyTrigger),
		transporthttp.WithDiagnosisRoomStarter(roomStartService),
		transporthttp.WithDiagnosisRoomWorkflowClient(roomClient),
	)

	triggerResponse := triggerE2ECoreReport(ctx, t, handler, configuration.Policy.ID)
	if triggerResponse.AutoDiagnosis != nil {
		t.Fatalf("manual report handoff unexpectedly started automatic diagnosis: %+v", triggerResponse.AutoDiagnosis)
	}
	if !triggerResponse.Started ||
		triggerResponse.WorkflowID != e2eCoreReportWorkflowID ||
		triggerResponse.RunID == "" ||
		len(triggerResponse.Snapshots) != 1 {
		t.Fatalf("policy replay response = %+v", triggerResponse)
	}

	var reportResult temporalpkg.ReportBatchWorkflowResult
	if err := tc.GetWorkflow(ctx, triggerResponse.WorkflowID, triggerResponse.RunID).Get(ctx, &reportResult); err != nil {
		t.Fatalf("wait report workflow: %v", err)
	}
	if reportResult.FinalReportID == 0 || len(reportResult.SubReportIDs) != 1 {
		t.Fatalf("report workflow result = %+v", reportResult)
	}

	reportBeforeDiagnosis := getE2EReportDetail(ctx, t, handler, reportResult.FinalReportID)
	if len(reportBeforeDiagnosis.LinkedSubReports) != 1 ||
		reportBeforeDiagnosis.LinkedSubReports[0].EvidenceSnapshotID != triggerResponse.Snapshots[0].ID ||
		reportBeforeDiagnosis.LinkedSubReports[0].DiagnosisRoom != nil ||
		reportBeforeDiagnosis.LinkedSubReports[0].DiagnosisConclusion != nil {
		t.Fatalf("report before diagnosis = %+v", reportBeforeDiagnosis.LinkedSubReports)
	}

	room := createE2EDiagnosisRoom(
		ctx,
		t,
		handler,
		triggerResponse.Snapshots[0].ID,
		int64(configuration.Channel.ID),
	)
	if room.EvidenceSnapshotID != triggerResponse.Snapshots[0].ID ||
		room.DiagnosisTaskID == 0 ||
		room.ChatSessionID == 0 ||
		room.WorkflowID == "" ||
		room.RunID == "" {
		t.Fatalf("created diagnosis room = %+v", room)
	}

	firstTurn, err := roomClient.SubmitDiagnosisTurn(ctx, ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    room.SessionID,
		MessageID:    e2eDiagnosisAssessMessage,
		ActorSubject: "e2e-operator",
		Message:      "Assess the frozen report evidence and identify the remaining review step.",
	})
	if err != nil {
		t.Fatalf("submit diagnosis assessment turn: %v", err)
	}
	if firstTurn.TurnCount != 1 ||
		firstTurn.Confidence != "low" ||
		firstTurn.ConsultationInsight.ConclusionStatus != "needs_evidence" ||
		len(firstTurn.ConsultationInsight.EvidenceCollectionSuggestions) != 1 {
		t.Fatalf("first diagnosis turn = %+v", firstTurn)
	}

	readyTurn, err := roomClient.SubmitDiagnosisTurn(ctx, ports.DiagnosisRoomSubmitTurnRequest{
		SessionID:    room.SessionID,
		MessageID:    e2eDiagnosisReadyMessage,
		ActorSubject: "e2e-operator",
		Message:      "The deployment context was reviewed. Prepare the bounded conclusion for confirmation.",
	})
	if err != nil {
		t.Fatalf("submit diagnosis ready turn: %v", err)
	}
	if readyTurn.TurnCount != 2 ||
		readyTurn.Confidence != "high" ||
		readyTurn.ConsultationInsight.ConclusionStatus != "ready_for_review" {
		t.Fatalf("ready diagnosis turn = %+v", readyTurn)
	}

	closed, err := roomClient.ConfirmDiagnosisConclusion(ctx, ports.DiagnosisRoomConfirmConclusionRequest{
		SessionID:    room.SessionID,
		ActorSubject: "e2e-operator",
	})
	if err != nil {
		t.Fatalf("confirm diagnosis conclusion: %v", err)
	}
	if closed.Status != "closed" ||
		closed.CloseReason != "human_confirmed" ||
		closed.TurnCount != 2 ||
		closed.FinalConclusion == nil ||
		closed.FinalConclusion.EvidenceSnapshotID != domain.EvidenceSnapshotID(room.EvidenceSnapshotID) ||
		closed.FinalConclusion.ConfirmedBy != "e2e-operator" ||
		closed.FinalConclusion.Content != "CPU saturation on payments-1 explains the degradation." {
		t.Fatalf("closed diagnosis room = %+v", closed)
	}

	reportAfterDiagnosis := getE2EReportDetail(ctx, t, handler, reportResult.FinalReportID)
	linked := reportAfterDiagnosis.LinkedSubReports[0]
	if linked.DiagnosisRoom == nil ||
		linked.DiagnosisRoom.SessionID != room.SessionID ||
		linked.DiagnosisConclusion == nil ||
		linked.DiagnosisConclusion.Status != "available" ||
		linked.DiagnosisConclusion.ConfirmedBy == nil ||
		*linked.DiagnosisConclusion.ConfirmedBy != "e2e-operator" ||
		linked.DiagnosisConclusion.Content != closed.FinalConclusion.Content {
		t.Fatalf("report after diagnosis = %+v", linked)
	}

	assertPersistedCoreFlow(ctx, t, factory, configuration, reportResult, room)
	assertE2ECoreNotifications(t, webhook.Requests(), reportResult.FinalReportID, room.DiagnosisTaskID, int64(configuration.Channel.ID))
	assertE2ENotificationResolution(t, notificationResolver)
	if got := containerProvider.MessageIDs(); len(got) != 2 ||
		got[0] != e2eDiagnosisAssessMessage ||
		got[1] != e2eDiagnosisReadyMessage {
		t.Fatalf("diagnosis container messages = %v", got)
	}
}

type e2eCoreConfiguration struct {
	Source  domain.AlertSourceProfile
	Channel domain.NotificationChannelProfile
	Policy  domain.ReportWorkflowPolicy
}

func seedE2ECoreConfiguration(
	ctx context.Context,
	t *testing.T,
	factory ports.UnitOfWorkFactory,
	alertmanagerURL string,
) e2eCoreConfiguration {
	t.Helper()
	var result e2eCoreConfiguration
	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		source, err := domain.NewAlertSourceProfile(
			"E2E Alertmanager",
			domain.AlertSourceKindAlertmanager,
			alertmanagerURL,
			domain.AlertSourceAuthModeNone,
			"",
			true,
			map[string]string{"environment": "e2e"},
		)
		if err != nil {
			return err
		}
		result.Source, err = uow.Config().SaveAlertSourceProfile(ctx, source)
		if err != nil {
			return err
		}

		grouping, err := domain.NewGroupingPolicy(
			"E2E service grouping",
			[]string{"alertname", "service"},
			"severity",
			[]string{"alertmanager"},
			true,
		)
		if err != nil {
			return err
		}
		grouping, err = uow.Config().SaveGroupingPolicy(ctx, grouping)
		if err != nil {
			return err
		}

		channel, err := domain.NewNotificationChannelProfile(
			"E2E operations WeCom",
			domain.NotificationChannelKindWeCom,
			"secret/e2e/operations-wecom",
			[]domain.NotificationDeliveryScope{
				domain.NotificationDeliveryScopeReport,
				domain.NotificationDeliveryScopeDiagnosisConsultation,
				domain.NotificationDeliveryScopeDiagnosisClose,
			},
			true,
			map[string]string{"environment": "e2e"},
		)
		if err != nil {
			return err
		}
		result.Channel, err = uow.Config().SaveNotificationChannelProfile(ctx, channel)
		if err != nil {
			return err
		}
		proofAt := result.Channel.UpdatedAt.Add(time.Second)
		for i, kind := range []domain.NotificationChannelTestContentKind{
			domain.NotificationChannelTestContentAIDiagnosisSample,
			domain.NotificationChannelTestContentDiagnosisCloseSample,
		} {
			proof, err := domain.NewNotificationChannelTestProof(
				result.Channel.ID,
				domain.NotificationChannelKindWeCom,
				domain.NotificationChannelTestStatusSuccess,
				domain.NotificationChannelTestReasonOK,
				"E2E notification proof succeeded.",
				kind,
				strings.Repeat(string(rune('a'+i)), 64),
				proofAt.Add(time.Duration(i)*time.Second),
				fmt.Sprintf("proof-e2e-%d", i+1),
				"accepted",
			)
			if err != nil {
				return err
			}
			if _, err := uow.Config().SaveNotificationChannelTestProof(ctx, proof); err != nil {
				return err
			}
		}

		enabledAt := e2eCoreWindowStart.Add(-time.Hour)
		policy, err := domain.NewReportWorkflowPolicy(
			"E2E report and diagnosis",
			result.Source.ID,
			grouping.ID,
			result.Channel.ID,
			0,
			domain.ReportWorkflowTriggerModeManualReplay,
			domain.ReportWorkflowScenarioSingleAlert,
			domain.DiagnosisFollowUpModeSuggestRoom,
			true,
			&enabledAt,
			nil,
		)
		if err != nil {
			return err
		}
		result.Policy, err = uow.Config().SaveReportWorkflowPolicy(ctx, policy)
		return err
	})
	if err != nil {
		t.Fatalf("seed core-flow configuration: %v", err)
	}
	return result
}

func newE2EAlertSourceProviderBuilder(t *testing.T) *alertsourceprovider.Builder {
	t.Helper()
	builder, err := alertsourceprovider.NewBuilder(
		alertsourceprovider.ProviderFactories{
			domain.AlertSourceKindPrometheus: func(domain.AlertSourceProfile, alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				return nil, fmt.Errorf("unexpected Prometheus provider construction")
			},
			domain.AlertSourceKindAlertmanager: func(profile domain.AlertSourceProfile, credentials alertsourceprovider.Credentials) (ports.ActiveAlertProvider, error) {
				opts := []metricsalertmanager.Option{}
				if credentials.BearerToken != "" {
					opts = append(opts, metricsalertmanager.WithBearer(credentials.BearerToken))
				}
				return metricsalertmanager.NewProvider(profile.BaseURL, opts...)
			},
		},
	)
	if err != nil {
		t.Fatalf("New alert source provider builder: %v", err)
	}
	return builder
}

func newE2EAlertmanagerServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(e2eAlertmanagerAlerts))
	}))
}

func triggerE2ECoreReport(
	ctx context.Context,
	t *testing.T,
	handler http.Handler,
	policyID domain.ReportWorkflowPolicyID,
) api.ReportReplayTriggerResponse {
	t.Helper()
	limit := int32(10)
	correlationKey := e2eCoreCorrelationKey
	workflowID := e2eCoreReportWorkflowID
	body, err := json.Marshal(api.ReportWorkflowPolicyReplayRequest{
		WindowStart:    e2eCoreWindowStart,
		WindowEnd:      e2eCoreWindowStart.Add(time.Hour),
		Limit:          &limit,
		CorrelationKey: &correlationKey,
		WorkflowID:     &workflowID,
	})
	if err != nil {
		t.Fatalf("encode policy replay request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("/api/v1/config/report-workflow-policies/%d/replay-window", policyID),
		bytes.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer e2e-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("policy replay status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var response api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode policy replay response: %v", err)
	}
	return response
}

func createE2EDiagnosisRoom(
	ctx context.Context,
	t *testing.T,
	handler http.Handler,
	snapshotID int64,
	channelID int64,
) api.DiagnosisRoomCreateResponse {
	t.Helper()
	body, err := json.Marshal(api.DiagnosisRoomCreateRequest{
		EvidenceSnapshotID:                snapshotID,
		CloseNotificationChannelProfileID: &channelID,
	})
	if err != nil {
		t.Fatalf("encode diagnosis room create request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/diagnosis/rooms", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer e2e-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create diagnosis room status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var response api.DiagnosisRoomCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode diagnosis room create response: %v", err)
	}
	return response
}

func getE2EReportDetail(ctx context.Context, t *testing.T, handler http.Handler, reportID int64) api.FinalReportDetail {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("/api/v1/reports/%d", reportID), nil)
	req.Header.Set("Authorization", "Bearer e2e-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get report status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response api.FinalReportDetail
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode report detail: %v", err)
	}
	return response
}

func assertPersistedCoreFlow(
	ctx context.Context,
	t *testing.T,
	factory ports.UnitOfWorkFactory,
	configuration e2eCoreConfiguration,
	reportResult temporalpkg.ReportBatchWorkflowResult,
	room api.DiagnosisRoomCreateResponse,
) {
	t.Helper()
	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		snapshot, err := uow.Evidence().FindByID(ctx, domain.EvidenceSnapshotID(room.EvidenceSnapshotID))
		if err != nil {
			return err
		}
		var payload struct {
			Events []struct {
				AlertSourceProfileID int64 `json:"alert_source_profile_id"`
			} `json:"events"`
		}
		if err := json.Unmarshal(snapshot.Payload, &payload); err != nil {
			return err
		}
		if len(payload.Events) != 1 || payload.Events[0].AlertSourceProfileID != int64(configuration.Source.ID) {
			t.Fatalf("persisted snapshot source binding = %+v", payload.Events)
		}

		report, err := uow.Reports().FindFinalReportByID(ctx, domain.FinalReportID(reportResult.FinalReportID))
		if err != nil {
			return err
		}
		if report.CorrelationKey != e2eCoreCorrelationKey {
			t.Fatalf("persisted final report = %+v", report)
		}
		subReports, err := uow.Reports().ListSubReportsForFinalReport(ctx, report.ID, 10)
		if err != nil {
			return err
		}
		if len(subReports) != 1 || subReports[0].EvidenceSnapshotID != snapshot.ID {
			t.Fatalf("persisted report snapshot link = %+v", subReports)
		}
		deliveries, err := uow.Reports().ListNotificationDeliveriesByFinalReport(ctx, report.ID, 10)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 ||
			deliveries[0].ReportNotificationChannelProfileID != configuration.Channel.ID ||
			deliveries[0].Status != domain.ReportNotificationDeliveryStatusDelivered {
			t.Fatalf("persisted report notification delivery = %+v", deliveries)
		}

		task, err := uow.Diagnosis().FindTaskByID(ctx, domain.DiagnosisTaskID(room.DiagnosisTaskID))
		if err != nil {
			return err
		}
		if task.EvidenceSnapshotID != snapshot.ID || task.Status != domain.DiagnosisStatusSucceeded || task.FinishedAt == nil {
			t.Fatalf("persisted diagnosis task = %+v", task)
		}
		session, err := uow.Diagnosis().FindChatSessionByID(ctx, domain.ChatSessionID(room.ChatSessionID))
		if err != nil {
			return err
		}
		if session.SessionKey != room.SessionID ||
			session.OwnerSubject != "e2e-operator" ||
			session.Status != domain.ChatSessionStatusClosed ||
			session.TurnCount != 2 ||
			session.CloseReason != "human_confirmed" ||
			session.ClosedAt == nil {
			t.Fatalf("persisted chat session = %+v", session)
		}
		turns, err := uow.Diagnosis().ListChatTurnsBySession(ctx, session.ID, 10)
		if err != nil {
			return err
		}
		wantMessageIDs := []string{
			e2eDiagnosisAssessMessage,
			e2eDiagnosisAssessMessage + "/assistant",
			e2eDiagnosisReadyMessage,
			e2eDiagnosisReadyMessage + "/assistant",
		}
		wantRoles := []domain.ChatRole{
			domain.ChatRoleUser,
			domain.ChatRoleAssistant,
			domain.ChatRoleUser,
			domain.ChatRoleAssistant,
		}
		if len(turns) != len(wantMessageIDs) {
			t.Fatalf("persisted chat turns = %+v", turns)
		}
		for i, turn := range turns {
			if turn.Sequence != i+1 || turn.MessageID != wantMessageIDs[i] || turn.Role != wantRoles[i] {
				t.Fatalf("persisted chat turn[%d] = %+v", i, turn)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("assert persisted core flow: %v", err)
	}
}

func assertE2ECoreNotifications(
	t *testing.T,
	requests []recordedWebhookRequest,
	finalReportID int64,
	diagnosisTaskID int64,
	channelID int64,
) {
	t.Helper()
	if len(requests) != 4 {
		t.Fatalf("core-flow webhook requests len = %d, want 4; requests=%+v", len(requests), requests)
	}
	reportNotifications := 0
	diagnosisNotifications := 0
	for _, request := range requests {
		switch {
		case request.FinalReportID != 0:
			reportNotifications++
			if request.FinalReportID != finalReportID ||
				request.DiagnosisTaskID != 0 ||
				request.NotificationChannelID != 0 {
				t.Fatalf("report webhook = %+v", request)
			}
		case request.DiagnosisTaskID != 0:
			diagnosisNotifications++
			if request.DiagnosisTaskID != diagnosisTaskID ||
				request.FinalReportID != 0 ||
				request.NotificationChannelID != channelID {
				t.Fatalf("diagnosis webhook = %+v", request)
			}
		default:
			t.Fatalf("webhook has no report or diagnosis identity: %+v", request)
		}
	}
	if reportNotifications != 1 || diagnosisNotifications != 3 {
		t.Fatalf("webhook identities = report:%d diagnosis:%d", reportNotifications, diagnosisNotifications)
	}
}

type e2eNotificationProviderResolver struct {
	mu       sync.Mutex
	provider ports.IMProvider
	calls    map[domain.NotificationDeliveryScope]int
}

func newE2ENotificationProviderResolver(provider ports.IMProvider) *e2eNotificationProviderResolver {
	return &e2eNotificationProviderResolver{
		provider: provider,
		calls:    make(map[domain.NotificationDeliveryScope]int),
	}
}

func (r *e2eNotificationProviderResolver) ResolveReportNotificationProvider(
	ctx context.Context,
	profileID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeReport)
}

func (r *e2eNotificationProviderResolver) ResolveDiagnosisConsultationNotificationProvider(
	ctx context.Context,
	profileID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeDiagnosisConsultation)
}

func (r *e2eNotificationProviderResolver) ResolveDiagnosisCloseNotificationProvider(
	ctx context.Context,
	profileID domain.NotificationChannelProfileID,
) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeDiagnosisClose)
}

func (r *e2eNotificationProviderResolver) resolve(
	ctx context.Context,
	profileID domain.NotificationChannelProfileID,
	scope domain.NotificationDeliveryScope,
) (ports.IMProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if profileID <= 0 || r == nil || r.provider == nil {
		return nil, fmt.Errorf("e2e notification resolver is not configured")
	}
	r.mu.Lock()
	r.calls[scope]++
	r.mu.Unlock()
	return r.provider, nil
}

func (r *e2eNotificationProviderResolver) Calls(scope domain.NotificationDeliveryScope) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[scope]
}

func assertE2ENotificationResolution(t *testing.T, resolver *e2eNotificationProviderResolver) {
	t.Helper()
	if got := resolver.Calls(domain.NotificationDeliveryScopeReport); got != 1 {
		t.Fatalf("report notification resolutions = %d, want 1", got)
	}
	if got := resolver.Calls(domain.NotificationDeliveryScopeDiagnosisConsultation); got != 2 {
		t.Fatalf("diagnosis consultation notification resolutions = %d, want 2", got)
	}
	if got := resolver.Calls(domain.NotificationDeliveryScopeDiagnosisClose); got != 1 {
		t.Fatalf("diagnosis close notification resolutions = %d, want 1", got)
	}
}

type e2eDiagnosisContainerProvider struct {
	mu         sync.Mutex
	messageIDs []string
}

func (p *e2eDiagnosisContainerProvider) Run(
	ctx context.Context,
	req ports.ContainerRunRequest,
) (ports.ContainerRunResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.ContainerRunResult{}, err
	}
	if err := req.Validate(); err != nil {
		return ports.ContainerRunResult{}, err
	}
	messageID := req.Metadata["message_id"]
	var output json.RawMessage
	switch messageID {
	case e2eDiagnosisAssessMessage:
		output = json.RawMessage(e2eDiagnosisNeedsEvidenceJSON)
	case e2eDiagnosisReadyMessage:
		output = json.RawMessage(e2eDiagnosisReadyJSON)
	default:
		return ports.ContainerRunResult{}, fmt.Errorf("unexpected e2e diagnosis message_id %q", messageID)
	}
	p.mu.Lock()
	p.messageIDs = append(p.messageIDs, messageID)
	p.mu.Unlock()
	started := time.Now().UTC()
	return ports.ContainerRunResult{
		InvocationID: req.InvocationID,
		AgentName:    req.AgentName,
		Output:       append(json.RawMessage(nil), output...),
		ExitCode:     0,
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		RuntimeID:    "e2e-runtime-" + messageID,
	}, nil
}

func (p *e2eDiagnosisContainerProvider) MessageIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.messageIDs...)
}

const e2eAlertmanagerAlerts = `[
  {
    "labels": {
      "alertname": "HighCPU",
      "instance": "payments-1",
      "service": "payments",
      "severity": "warning"
    },
    "annotations": {
      "summary": "CPU saturation on payments"
    },
    "startsAt": "2026-05-28T10:15:00Z",
    "status": {
      "state": "active",
      "silencedBy": [],
      "inhibitedBy": [],
      "mutedBy": []
    }
  }
]`

const e2eDiagnosisNeedsEvidenceJSON = `{
  "schema_version": "diagnosis_turn.v1",
  "message": "CPU saturation is the leading explanation, but deployment context should be reviewed.",
  "findings": ["payments-1 has a firing HighCPU alert"],
  "recommended_actions": ["Review the deployment timeline before confirmation"],
  "confidence": "low",
  "requires_human_review": true,
  "confidence_rationale": "The frozen alert is strong, but deployment context has not been reviewed.",
  "evidence_collection_suggestions": [{
    "label": "Deployment context",
    "detail": "Review whether a deployment overlapped the alert start.",
    "priority": "high"
  }],
  "conclusion_status": "needs_evidence"
}`

const e2eDiagnosisReadyJSON = `{
  "schema_version": "diagnosis_turn.v1",
  "message": "CPU saturation on payments-1 explains the degradation.",
  "findings": ["payments-1 has a firing HighCPU alert", "Deployment context was reviewed by the operator"],
  "recommended_actions": ["Inspect CPU demand and scale payments if the load is expected"],
  "confidence": "high",
  "requires_human_review": true,
  "confidence_rationale": "The frozen alert and reviewed deployment context support the conclusion.",
  "conclusion_status": "ready_for_review"
}`
