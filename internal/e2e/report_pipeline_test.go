package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.temporal.io/sdk/testsuite"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	imwebhook "github.com/openclarion/openclarion/internal/providers/im/webhook"
	openaillm "github.com/openclarion/openclarion/internal/providers/llm/openai"
	metricsprometheus "github.com/openclarion/openclarion/internal/providers/metrics/prometheus"
	transporthttp "github.com/openclarion/openclarion/internal/transport/http"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
	"github.com/openclarion/openclarion/internal/usecases/reporttrigger"
)

const (
	testPGImage    = "postgres:18-alpine"
	testDBName     = "openclarion_e2e"
	testDBUser     = "openclarion"
	testDBPassword = "openclarion"
)

func TestReportReplayHTTPTriggerEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factory, closeDB := startE2EDatabase(ctx, t)
	defer closeDB()

	prometheusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(e2eAlertsEnvelope))
	}))
	defer prometheusServer.Close()

	llmServer := newE2ELLMServer(t)
	defer llmServer.Close()

	webhook := newRecordingWebhookServer(t)
	defer webhook.Close()

	metricsProvider, err := metricsprometheus.NewProvider(prometheusServer.URL)
	if err != nil {
		t.Fatalf("New prometheus provider: %v", err)
	}
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

	devServer, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
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
		temporalpkg.WithIMProvider(imProvider),
	)
	if err := worker.Start(); err != nil {
		t.Fatalf("start temporal worker: %v", err)
	}
	defer worker.Stop()

	starter, err := temporalpkg.NewReportStarter(tc)
	if err != nil {
		t.Fatalf("NewReportStarter: %v", err)
	}
	trigger, err := reporttrigger.NewService(metricsProvider, factory, starter)
	if err != nil {
		t.Fatalf("New report trigger service: %v", err)
	}
	handler := e2eHTTPHandler(factory, trigger)

	body := `{
		"window_start":"2026-05-28T10:00:00Z",
		"window_end":"2026-05-28T11:00:00Z",
		"limit":10,
		"correlation_key":"e2e-alert-window",
		"workflow_id":"report-batch-e2e-alert-window",
		"scenario":"single_alert"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/report-triggers/replay-window", strings.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("trigger status = %d, want 202; body=%s", rec.Code, rec.Body.String())
	}
	var triggerResp api.ReportReplayTriggerResponse
	if err := json.NewDecoder(rec.Body).Decode(&triggerResp); err != nil {
		t.Fatalf("decode trigger response: %v", err)
	}
	if !triggerResp.Started || triggerResp.WorkflowID != "report-batch-e2e-alert-window" || triggerResp.RunID == "" {
		t.Fatalf("trigger response workflow = %+v", triggerResp)
	}
	if triggerResp.Stats.Ingested.Saved != 1 ||
		triggerResp.Stats.EventsLoaded != 1 ||
		triggerResp.Stats.GroupsBuilt != 1 ||
		triggerResp.Stats.SnapshotsSaved != 1 ||
		len(triggerResp.Snapshots) != 1 {
		t.Fatalf("trigger response stats/snapshots = %+v snapshots=%+v", triggerResp.Stats, triggerResp.Snapshots)
	}

	var workflowResult temporalpkg.ReportBatchWorkflowResult
	if err := tc.GetWorkflow(ctx, triggerResp.WorkflowID, triggerResp.RunID).Get(ctx, &workflowResult); err != nil {
		t.Fatalf("wait report workflow: %v", err)
	}
	if workflowResult.FinalReportID == 0 ||
		len(workflowResult.SubReportIDs) != 1 ||
		workflowResult.ProviderMessageID != "msg-e2e" ||
		workflowResult.NotificationStatus != "accepted" {
		t.Fatalf("workflow result = %+v", workflowResult)
	}

	requests := webhook.Requests()
	if len(requests) != 1 {
		t.Fatalf("webhook requests len = %d, want 1", len(requests))
	}
	if requests[0].FinalReportID != workflowResult.FinalReportID ||
		requests[0].IdempotencyKey != fmt.Sprintf("final_report:%d/notification", workflowResult.FinalReportID) ||
		requests[0].Title != "Payments degradation" ||
		requests[0].Body == "" {
		t.Fatalf("webhook request = %+v; workflow result = %+v", requests[0], workflowResult)
	}

	assertPersistedReportPipeline(ctx, t, factory, workflowResult)
}

func startE2EDatabase(ctx context.Context, t *testing.T) (ports.UnitOfWorkFactory, func()) {
	t.Helper()
	ctr, err := postgres.Run(
		ctx,
		testPGImage,
		postgres.WithDatabase(testDBName),
		postgres.WithUsername(testDBUser),
		postgres.WithPassword(testDBPassword),
		postgres.BasicWaitStrategies(),
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	cleanupContainer := func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Fatalf("terminate postgres container: %v", err)
		}
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		cleanupContainer()
		t.Fatalf("postgres connection string: %v", err)
	}
	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		cleanupContainer()
		t.Fatalf("open postgres for migration: %v", err)
	}
	migrateDrv := entsql.OpenDB(dialect.Postgres, migrateDB)
	migrateClient := ent.NewClient(ent.Driver(migrateDrv))
	if err := migrateClient.Schema.Create(ctx); err != nil {
		_ = migrateClient.Close()
		cleanupContainer()
		t.Fatalf("create ent schema: %v", err)
	}
	if err := migrateClient.Close(); err != nil {
		cleanupContainer()
		t.Fatalf("close migration client: %v", err)
	}

	client, err := repository.OpenPostgres(ctx, dsn)
	if err != nil {
		cleanupContainer()
		t.Fatalf("open ent client: %v", err)
	}
	return repository.NewFactory(client), func() {
		if err := client.Close(); err != nil {
			t.Fatalf("close ent client: %v", err)
		}
		cleanupContainer()
	}
}

func e2eHTTPHandler(factory ports.UnitOfWorkFactory, trigger *reporttrigger.Service) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := transporthttp.NewServer(logger, factory, transporthttp.WithReportReplayTrigger(trigger))
	return api.HandlerWithOptions(server, api.StdHTTPServerOptions{
		ErrorHandlerFunc: transporthttp.OpenAPIErrorHandler(logger),
	})
}

func assertPersistedReportPipeline(ctx context.Context, t *testing.T, factory ports.UnitOfWorkFactory, result temporalpkg.ReportBatchWorkflowResult) {
	t.Helper()
	err := factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		report, err := uow.Reports().FindFinalReportByID(ctx, domain.FinalReportID(result.FinalReportID))
		if err != nil {
			return err
		}
		if report.CorrelationKey != "e2e-alert-window" || report.Title != "Payments degradation" {
			t.Fatalf("final report = %+v", report)
		}

		subReports, err := uow.Reports().ListSubReportsForFinalReport(ctx, report.ID, 10)
		if err != nil {
			return err
		}
		if len(subReports) != 1 || int64(subReports[0].ID) != result.SubReportIDs[0] {
			t.Fatalf("linked subreports = %+v; workflow result = %+v", subReports, result)
		}

		deliveries, err := uow.Reports().ListNotificationDeliveriesByFinalReport(ctx, report.ID, 10)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 ||
			deliveries[0].Status != domain.ReportNotificationDeliveryStatusDelivered ||
			deliveries[0].ProviderMessageID != "msg-e2e" ||
			deliveries[0].ProviderStatus != "accepted" ||
			deliveries[0].DeliveredAt == nil {
			t.Fatalf("delivery log = %+v", deliveries)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("assert persisted report pipeline: %v", err)
	}
}

func newE2ELLMServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			ResponseFormat struct {
				JSONSchema *struct {
					Name string `json:"name"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode llm request: %v", err)
		}

		schemaName := ""
		if req.ResponseFormat.JSONSchema != nil {
			schemaName = req.ResponseFormat.JSONSchema.Name
		}
		switch schemaName {
		case "openclarion_probe":
			writeChatCompletion(t, w, `{"ok":true}`)
		case reportdraft.SubReportSchemaID:
			writeChatCompletion(t, w, e2eSubReportJSON)
		case reportdraft.FinalReportSchemaID:
			writeChatCompletion(t, w, e2eFinalReportJSON)
		default:
			t.Fatalf("unexpected llm schema name %q", schemaName)
		}
	}))
}

func writeChatCompletion(t *testing.T, w http.ResponseWriter, content string) {
	t.Helper()
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, []byte(content)); err != nil {
		t.Fatalf("compact llm content: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"model": "gpt-e2e",
		"choices": []map[string]any{
			{
				"message":       map[string]string{"content": compacted.String()},
				"finish_reason": "stop",
			},
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("write llm response: %v", err)
	}
}

type recordingWebhookServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedWebhookRequest
}

type recordedWebhookRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	FinalReportID  int64  `json:"final_report_id"`
	CorrelationKey string `json:"correlation_key"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	Severity       string `json:"severity"`
	HeaderKey      string
	HeaderReportID string
}

func newRecordingWebhookServer(t *testing.T) *recordingWebhookServer {
	t.Helper()
	rec := &recordingWebhookServer{}
	rec.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("webhook method = %s, want POST", r.Method)
		}
		var req recordedWebhookRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode webhook request: %v", err)
		}
		req.HeaderKey = r.Header.Get("X-OpenClarion-Idempotency-Key")
		req.HeaderReportID = r.Header.Get("X-OpenClarion-Final-Report-Id")
		if req.HeaderKey != req.IdempotencyKey || req.HeaderReportID != fmt.Sprintf("%d", req.FinalReportID) {
			t.Fatalf("webhook headers = key:%q report:%q body:%+v", req.HeaderKey, req.HeaderReportID, req)
		}

		rec.mu.Lock()
		rec.requests = append(rec.requests, req)
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message_id":"msg-e2e","status":"accepted"}`))
	}))
	return rec
}

func (s *recordingWebhookServer) Requests() []recordedWebhookRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedWebhookRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

const e2eAlertsEnvelope = `{
  "status": "success",
  "data": {
    "alerts": [
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
        "state": "firing",
        "activeAt": "2026-05-28T10:15:00.000000000Z",
        "value": "1e+00"
      }
    ]
  }
}`

const e2eSubReportJSON = `{
  "title": "Payments CPU saturation",
  "summary": "The payments service has one firing HighCPU alert.",
  "severity": "warning",
  "confidence": "high",
  "findings": [
    {
      "label": "HighCPU",
      "detail": "payments-1 is firing a HighCPU alert.",
      "evidence_id": "alert:HighCPU"
    }
  ],
  "recommended_actions": [
    {
      "label": "Scale payments",
      "detail": "Inspect CPU saturation and scale the payments deployment if load is expected.",
      "priority": "medium"
    }
  ],
  "evidence_refs": ["alert:HighCPU"]
}`

const e2eFinalReportJSON = `{
  "title": "Payments degradation",
  "executive_summary": "Payments is degraded by CPU saturation on payments-1.",
  "severity": "warning",
  "confidence": "high",
  "sub_reports": [
    {
      "title": "Payments CPU saturation",
      "severity": "warning",
      "summary": "The payments service has one firing HighCPU alert."
    }
  ],
  "recommended_actions": [
    {
      "label": "Scale payments",
      "detail": "Inspect CPU saturation and scale the payments deployment if load is expected.",
      "priority": "medium"
    }
  ],
  "notification_text": "Payments is degraded by CPU saturation on payments-1."
}`
