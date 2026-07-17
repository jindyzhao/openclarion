package temporal_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	embeddingfake "github.com/openclarion/openclarion/internal/providers/embedding/fake"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
)

type reportLLMProvider struct {
	mu      sync.Mutex
	calls   map[string]int
	prompts map[string]string
}

type recordingIMProvider struct {
	mu       sync.Mutex
	requests []ports.IMNotification
	delivery ports.IMDelivery
	err      error
}

type recordingNotificationProviderResolver struct {
	mu       sync.Mutex
	calls    int
	profile  domain.NotificationChannelProfileID
	scope    domain.NotificationDeliveryScope
	provider ports.IMProvider
	err      error
}

func newReportLLMProvider() *reportLLMProvider {
	return &reportLLMProvider{calls: map[string]int{}, prompts: map[string]string{}}
}

func (p *reportLLMProvider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	if err := ctx.Err(); err != nil {
		return ports.LLMResponse{}, err
	}
	p.mu.Lock()
	p.calls[req.IdempotencyKey]++
	if len(req.Messages) > 0 {
		p.prompts[req.IdempotencyKey] = req.Messages[len(req.Messages)-1].Content
	}
	p.mu.Unlock()

	switch req.OutputSchemaID {
	case reportdraft.SubReportSchemaID:
		return llmResponse(`{
			"title":"CPU saturation",
			"summary":"CPU usage is above threshold.",
			"severity":"warning",
			"confidence":"high",
			"findings":[{"label":"CPU","detail":"CPU is saturated.","evidence_id":"alert:1"}],
			"recommended_actions":[{"label":"Scale","detail":"Add one replica.","priority":"medium"}],
			"evidence_refs":["alert:1"]
		}`), nil
	case reportdraft.FinalReportSchemaID:
		return llmResponse(`{
			"title":"Payments degradation",
			"executive_summary":"Payments is degraded by CPU saturation.",
			"severity":"warning",
			"confidence":"high",
			"sub_reports":[{"title":"CPU saturation","severity":"warning","summary":"CPU usage is above threshold."}],
			"recommended_actions":[{"label":"Scale","detail":"Add one replica.","priority":"medium"}],
			"notification_text":"Payments is degraded. Scale the payments deployment."
		}`), nil
	default:
		return ports.LLMResponse{}, nil
	}
}

func (p *reportLLMProvider) Calls(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[key]
}

func (p *reportLLMProvider) Prompt(key string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prompts[key]
}

func (p *recordingIMProvider) SendNotification(_ context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, req)
	if p.err != nil {
		return ports.IMDelivery{}, p.err
	}
	return p.delivery, nil
}

func (p *recordingIMProvider) Requests() []ports.IMNotification {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ports.IMNotification, len(p.requests))
	copy(out, p.requests)
	return out
}

func (r *recordingNotificationProviderResolver) ResolveReportNotificationProvider(ctx context.Context, profileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeReport)
}

func (r *recordingNotificationProviderResolver) ResolveDiagnosisConsultationNotificationProvider(ctx context.Context, profileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeDiagnosisConsultation)
}

func (r *recordingNotificationProviderResolver) ResolveDiagnosisCloseNotificationProvider(ctx context.Context, profileID domain.NotificationChannelProfileID) (ports.IMProvider, error) {
	return r.resolve(ctx, profileID, domain.NotificationDeliveryScopeDiagnosisClose)
}

func (r *recordingNotificationProviderResolver) resolve(ctx context.Context, profileID domain.NotificationChannelProfileID, scope domain.NotificationDeliveryScope) (ports.IMProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.profile = profileID
	r.scope = scope
	if r.err != nil {
		return nil, r.err
	}
	return r.provider, nil
}

func (r *recordingNotificationProviderResolver) LastCall() (int, domain.NotificationChannelProfileID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.profile
}

func (r *recordingNotificationProviderResolver) LastScope() domain.NotificationDeliveryScope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.scope
}

func llmResponse(content string) ports.LLMResponse {
	var compacted json.RawMessage
	if err := json.Unmarshal([]byte(content), &compacted); err != nil {
		panic(err)
	}
	return ports.LLMResponse{
		Content:      compacted,
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-report-model",
	}
}

func loadReportTaskLifecycle(
	t *testing.T,
	taskID domain.DiagnosisTaskID,
	eventKinds ...string,
) (domain.DiagnosisTask, map[string]domain.DiagnosisTaskEvent) {
	t.Helper()
	ctx := workflowTestContext(t)
	events := make(map[string]domain.DiagnosisTaskEvent, len(eventKinds))
	var task domain.DiagnosisTask
	err := env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		task, err = uow.Diagnosis().FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}
		for _, kind := range eventKinds {
			event, err := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskID, kind)
			if err != nil {
				return err
			}
			events[kind] = event
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load report task lifecycle: %v", err)
	}
	return task, events
}

func TestReportActivities_ReportTaskLifecycleIsDurableAndIdempotent(t *testing.T) {
	t.Run("succeeded", func(t *testing.T) {
		seed := seedDiagnosisTask(t, "report-task-succeeded")
		provider := newReportLLMProvider()
		activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
		startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
		ensureReq := temporalpkg.EnsureReportTaskInput{
			EvidenceSnapshotID: int64(seed.SnapshotID),
			WorkflowID:         "report-task-succeeded-workflow",
			RunID:              "report-task-succeeded-run",
			Scenario:           "cascade",
			GroupIndex:         2,
			StartedAt:          startedAt,
		}
		ctx := workflowTestContext(t)
		first, err := activities.EnsureReportTask(ctx, ensureReq)
		if err != nil {
			t.Fatalf("EnsureReportTask first: %v", err)
		}
		second, err := activities.EnsureReportTask(ctx, ensureReq)
		if err != nil {
			t.Fatalf("EnsureReportTask second: %v", err)
		}
		if first.DiagnosisTaskID == 0 || second.DiagnosisTaskID != first.DiagnosisTaskID {
			t.Fatalf("idempotent task IDs = first %d, second %d", first.DiagnosisTaskID, second.DiagnosisTaskID)
		}

		subReport, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
			EvidenceSnapshotID: int64(seed.SnapshotID),
			Scenario:           "cascade",
			GroupIndex:         2,
		})
		if err != nil {
			t.Fatalf("GenerateSubReport: %v", err)
		}
		finishedAt := startedAt.Add(30 * time.Second)
		finishReq := temporalpkg.FinishReportTaskInput{
			DiagnosisTaskID:    first.DiagnosisTaskID,
			EvidenceSnapshotID: int64(seed.SnapshotID),
			Scenario:           "cascade",
			GroupIndex:         2,
			SubReportID:        subReport.SubReportID,
			Status:             "succeeded",
			FinishedAt:         finishedAt,
		}
		finished, err := activities.FinishReportTask(ctx, finishReq)
		if err != nil {
			t.Fatalf("FinishReportTask first: %v", err)
		}
		finishedAgain, err := activities.FinishReportTask(ctx, finishReq)
		if err != nil {
			t.Fatalf("FinishReportTask second: %v", err)
		}
		if finished.LifecycleEventID == 0 || finishedAgain.LifecycleEventID != finished.LifecycleEventID {
			t.Fatalf("idempotent lifecycle event IDs = first %d, second %d", finished.LifecycleEventID, finishedAgain.LifecycleEventID)
		}

		task, events := loadReportTaskLifecycle(t, domain.DiagnosisTaskID(first.DiagnosisTaskID), "subreport.started", "subreport.succeeded")
		if task.Status != domain.DiagnosisStatusSucceeded || task.FailureReason != "" || task.StartedAt == nil ||
			task.FinishedAt == nil || !task.StartedAt.Equal(startedAt) || !task.FinishedAt.Equal(finishedAt) {
			t.Fatalf("persisted succeeded task = %+v", task)
		}
		if !events["subreport.started"].OccurredAt.Equal(*task.StartedAt) ||
			!events["subreport.succeeded"].OccurredAt.Equal(*task.FinishedAt) ||
			!strings.Contains(string(events["subreport.succeeded"].Payload), `"sub_report_id":`) {
			t.Fatalf("persisted succeeded events = %+v", events)
		}
	})

	t.Run("failed with stable reason", func(t *testing.T) {
		seed := seedDiagnosisTask(t, "report-task-failed")
		activities := temporalpkg.NewActivities(env.factory)
		startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
		ctx := workflowTestContext(t)
		ensured, err := activities.EnsureReportTask(ctx, temporalpkg.EnsureReportTaskInput{
			EvidenceSnapshotID: int64(seed.SnapshotID),
			WorkflowID:         "report-task-failed-workflow",
			RunID:              "report-task-failed-run",
			Scenario:           "single_alert",
			GroupIndex:         0,
			StartedAt:          startedAt,
		})
		if err != nil {
			t.Fatalf("EnsureReportTask: %v", err)
		}
		finishReq := temporalpkg.FinishReportTaskInput{
			DiagnosisTaskID:    ensured.DiagnosisTaskID,
			EvidenceSnapshotID: int64(seed.SnapshotID),
			Scenario:           "single_alert",
			GroupIndex:         0,
			Status:             "failed",
			FailureReason:      "provider credential leaked",
			FinishedAt:         startedAt.Add(20 * time.Second),
		}
		if _, err := activities.FinishReportTask(ctx, finishReq); err == nil {
			t.Fatal("FinishReportTask accepted an arbitrary provider failure message")
		}
		finishReq.FailureReason = "subreport_generation_failed"
		first, err := activities.FinishReportTask(ctx, finishReq)
		if err != nil {
			t.Fatalf("FinishReportTask first: %v", err)
		}
		second, err := activities.FinishReportTask(ctx, finishReq)
		if err != nil {
			t.Fatalf("FinishReportTask second: %v", err)
		}
		if first.LifecycleEventID == 0 || second.LifecycleEventID != first.LifecycleEventID {
			t.Fatalf("idempotent failure event IDs = first %d, second %d", first.LifecycleEventID, second.LifecycleEventID)
		}

		task, events := loadReportTaskLifecycle(t, domain.DiagnosisTaskID(ensured.DiagnosisTaskID), "subreport.started", "subreport.failed")
		if task.Status != domain.DiagnosisStatusFailed || task.FailureReason != "subreport_generation_failed" {
			t.Fatalf("persisted failed task = %+v", task)
		}
		failedPayload := string(events["subreport.failed"].Payload)
		if !strings.Contains(failedPayload, "subreport_generation_failed") ||
			strings.Contains(failedPayload, "provider credential") {
			t.Fatalf("persisted failure event payload = %s", failedPayload)
		}
	})
}

func TestReportActivities_FinishReportTaskRejectsPromptIdentityDrift(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-task-identity")
	other := seedDiagnosisTask(t, "report-task-other-snapshot")
	activities := temporalpkg.NewActivities(env.factory)
	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	ctx := workflowTestContext(t)
	ensured, err := activities.EnsureReportTask(ctx, temporalpkg.EnsureReportTaskInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		WorkflowID:         "report-task-identity-workflow",
		RunID:              "report-task-identity-run",
		Scenario:           "alert_storm",
		GroupIndex:         3,
		StartedAt:          startedAt,
	})
	if err != nil {
		t.Fatalf("EnsureReportTask: %v", err)
	}
	base := temporalpkg.FinishReportTaskInput{
		DiagnosisTaskID:    ensured.DiagnosisTaskID,
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "alert_storm",
		GroupIndex:         3,
		Status:             "failed",
		FailureReason:      "subreport_generation_failed",
		FinishedAt:         startedAt.Add(15 * time.Second),
	}

	wrongGroup := base
	wrongGroup.GroupIndex++
	if _, err := activities.FinishReportTask(ctx, wrongGroup); err == nil {
		t.Fatal("FinishReportTask accepted a different group index")
	}
	wrongSnapshot := base
	wrongSnapshot.EvidenceSnapshotID = int64(other.SnapshotID)
	if _, err := activities.FinishReportTask(ctx, wrongSnapshot); err == nil {
		t.Fatal("FinishReportTask accepted a different evidence snapshot")
	}
	task, _ := loadReportTaskLifecycle(t, domain.DiagnosisTaskID(ensured.DiagnosisTaskID), "subreport.started")
	if task.Status != domain.DiagnosisStatusRunning || task.FinishedAt != nil {
		t.Fatalf("identity mismatch mutated task = %+v", task)
	}
}

func TestReportActivities_GenerateSubReportPersistsAndIsIdempotent(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-sub-activity")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))

	req := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	}
	ctx := context.Background()
	first, err := activities.GenerateSubReport(ctx, req)
	if err != nil {
		t.Fatalf("GenerateSubReport first: %v", err)
	}
	if first.SubReportID == 0 {
		t.Fatal("GenerateSubReport first returned zero ID")
	}
	second, err := activities.GenerateSubReport(ctx, req)
	if err != nil {
		t.Fatalf("GenerateSubReport second: %v", err)
	}
	if second.SubReportID != first.SubReportID {
		t.Fatalf("idempotent SubReportID mismatch: first=%d second=%d", first.SubReportID, second.SubReportID)
	}
	key := "snapshot:" + int64String(seed.SnapshotID) + "/group:0/scenario:single_alert/sub_report"
	if provider.Calls(key) != 1 {
		t.Fatalf("provider calls for %s = %d, want 1", key, provider.Calls(key))
	}

	var stored domain.SubReport
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		got, err := uow.Reports().FindSubReportByID(ctx, domain.SubReportID(first.SubReportID))
		if err != nil {
			return err
		}
		stored = got
		return nil
	})
	if err != nil {
		t.Fatalf("load stored subreport: %v", err)
	}
	if stored.CreatedByWorkflow != "ReportFanOutWorkflow" {
		t.Fatalf("CreatedByWorkflow = %q, want ReportFanOutWorkflow", stored.CreatedByWorkflow)
	}
	if stored.IdempotencyKey != key {
		t.Fatalf("IdempotencyKey = %q, want %q", stored.IdempotencyKey, key)
	}
}

func TestReportActivities_GenerateSubReportSeparatesScenarios(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-sub-scenario-identity")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := context.Background()

	single, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport single_alert: %v", err)
	}
	cascade, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "cascade",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport cascade: %v", err)
	}
	if single.SubReportID == cascade.SubReportID {
		t.Fatalf("different scenarios reused SubReportID %d", single.SubReportID)
	}
	for _, scenario := range []string{"single_alert", "cascade"} {
		key := "snapshot:" + int64String(seed.SnapshotID) + "/group:0/scenario:" + scenario + "/sub_report"
		if provider.Calls(key) != 1 {
			t.Fatalf("provider calls for %s = %d, want 1", key, provider.Calls(key))
		}
	}
}

func TestReportActivities_IndexesAndRetrievesHistoricalReports(t *testing.T) {
	ctx := context.Background()
	llm := newReportLLMProvider()
	embeddings := embeddingfake.NewDeterministic("report-rag-test-model")
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithLLMProvider(llm),
		temporalpkg.WithEmbeddingProvider(embeddings),
	)

	firstSeed := seedDiagnosisTask(t, "report-rag-first")
	firstSub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(firstSeed.SnapshotID), Scenario: "single_alert", GroupIndex: 0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport first incident: %v", err)
	}
	firstIndexKey := "retrieval-index:sub_report:" + strconv.FormatInt(firstSub.SubReportID, 10)
	if embeddings.Calls(firstIndexKey) != 0 {
		t.Fatal("unlinked first SubReport was indexed")
	}

	siblingSeed := seedDiagnosisTask(t, "report-rag-sibling")
	siblingReq := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(siblingSeed.SnapshotID), Scenario: "cascade", GroupIndex: 1,
	}
	siblingSub, err := activities.GenerateSubReport(ctx, siblingReq)
	if err != nil {
		t.Fatalf("GenerateSubReport sibling incident: %v", err)
	}
	siblingKey := "snapshot:" + int64String(siblingSeed.SnapshotID) + "/group:1/scenario:cascade/sub_report"
	if strings.Contains(llm.Prompt(siblingKey), "sub_report:"+strconv.FormatInt(firstSub.SubReportID, 10)) {
		t.Fatal("unlinked sibling SubReport leaked into current-batch historical context")
	}
	siblingIndexKey := "retrieval-index:sub_report:" + strconv.FormatInt(siblingSub.SubReportID, 10)
	if embeddings.Calls(siblingIndexKey) != 0 {
		t.Fatal("unlinked sibling SubReport was indexed")
	}

	firstFinal, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "report-rag-first", SubReportIDs: []int64{firstSub.SubReportID, siblingSub.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport first incident: %v", err)
	}
	for _, key := range []string{
		firstIndexKey,
		siblingIndexKey,
		"retrieval-index:final_report:" + strconv.FormatInt(firstFinal.FinalReportID, 10),
	} {
		if embeddings.Calls(key) != 1 {
			t.Fatalf("linked report index calls for %s = %d, want 1", key, embeddings.Calls(key))
		}
	}

	secondSeed := seedDiagnosisTask(t, "report-rag-second")
	secondReq := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(secondSeed.SnapshotID), Scenario: "cascade", GroupIndex: 0,
	}
	secondSub, err := activities.GenerateSubReport(ctx, secondReq)
	if err != nil {
		t.Fatalf("GenerateSubReport second incident: %v", err)
	}
	duplicate, err := activities.GenerateSubReport(ctx, secondReq)
	if err != nil {
		t.Fatalf("GenerateSubReport duplicate: %v", err)
	}
	if duplicate.SubReportID != secondSub.SubReportID {
		t.Fatalf("duplicate subreport ID = %d, want %d", duplicate.SubReportID, secondSub.SubReportID)
	}

	key := "snapshot:" + int64String(secondSeed.SnapshotID) + "/group:0/scenario:cascade/sub_report"
	if llm.Calls(key) != 1 {
		t.Fatalf("LLM calls = %d, want 1", llm.Calls(key))
	}
	prompt := llm.Prompt(key)
	for _, want := range []string{
		"Historical accepted reports",
		"sub_report:" + strconv.FormatInt(firstSub.SubReportID, 10),
		"sub_report:" + strconv.FormatInt(siblingSub.SubReportID, 10),
		"final_report:" + strconv.FormatInt(firstFinal.FinalReportID, 10),
		"Do not treat them as current evidence",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("second report prompt missing %q: %s", want, prompt)
		}
	}

	var stored domain.SubReport
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		stored, err = uow.Reports().FindSubReportByID(ctx, domain.SubReportID(secondSub.SubReportID))
		return err
	})
	if err != nil {
		t.Fatalf("load second SubReport: %v", err)
	}
	if len(stored.RetrievalRefs) < 3 {
		t.Fatalf("RetrievalRefs = %v, want linked prior subreports and final report", stored.RetrievalRefs)
	}
	secondIndexKey := "retrieval-index:sub_report:" + strconv.FormatInt(secondSub.SubReportID, 10)
	if embeddings.Calls(secondIndexKey) != 0 {
		t.Fatal("second SubReport was indexed before final linkage")
	}
	if _, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "report-rag-second", SubReportIDs: []int64{secondSub.SubReportID},
	}); err != nil {
		t.Fatalf("GenerateFinalReport second incident: %v", err)
	}
	if embeddings.Calls(secondIndexKey) != 1 {
		t.Fatal("second SubReport was not indexed after final linkage")
	}
}

func TestReportActivities_HistoricalRetrievalFailureDoesNotBlockSubReport(t *testing.T) {
	ctx := context.Background()
	llm := newReportLLMProvider()
	embeddings := embeddingfake.New("report-rag-fail-open-model", nil)
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithLLMProvider(llm),
		temporalpkg.WithEmbeddingProvider(embeddings),
	)
	seed := seedDiagnosisTask(t, "report-rag-fail-open")
	req := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID), Scenario: "single_alert", GroupIndex: 0,
	}

	result, err := activities.GenerateSubReport(ctx, req)
	if err != nil {
		t.Fatalf("GenerateSubReport with unavailable retrieval: %v", err)
	}
	key := "snapshot:" + int64String(seed.SnapshotID) + "/group:0/scenario:single_alert/sub_report"
	if llm.Calls(key) != 1 || strings.Contains(llm.Prompt(key), "Historical accepted reports") {
		t.Fatalf("LLM fallback call/prompt = %d/%q", llm.Calls(key), llm.Prompt(key))
	}
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		stored, err := uow.Reports().FindSubReportByID(ctx, domain.SubReportID(result.SubReportID))
		if err != nil {
			return err
		}
		if len(stored.RetrievalRefs) != 0 {
			t.Fatalf("RetrievalRefs = %v, want empty fallback", stored.RetrievalRefs)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify fallback SubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "report-rag-fail-open", SubReportIDs: []int64{result.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport with unavailable indexing: %v", err)
	}
	if final.FinalReportID == 0 {
		t.Fatal("GenerateFinalReport with unavailable indexing returned zero ID")
	}
}

func TestReportActivities_GenerateFinalReportPersistsAndLinksSubReports(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-final-activity")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := context.Background()

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}

	req := temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-final-activity",
		SubReportIDs:   []int64{sub.SubReportID},
	}
	first, err := activities.GenerateFinalReport(ctx, req)
	if err != nil {
		t.Fatalf("GenerateFinalReport first: %v", err)
	}
	if first.FinalReportID == 0 {
		t.Fatal("GenerateFinalReport returned zero ID")
	}
	second, err := activities.GenerateFinalReport(ctx, req)
	if err != nil {
		t.Fatalf("GenerateFinalReport second: %v", err)
	}
	if second.FinalReportID != first.FinalReportID {
		t.Fatalf("idempotent FinalReportID mismatch: first=%d second=%d", first.FinalReportID, second.FinalReportID)
	}
	if provider.Calls("final_report:window-report-final-activity") != 1 {
		t.Fatalf("final report provider calls = %d, want 1", provider.Calls("final_report:window-report-final-activity"))
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		report, err := uow.Reports().FindFinalReportByID(ctx, domain.FinalReportID(first.FinalReportID))
		if err != nil {
			return err
		}
		if report.NotificationText == "" || report.GenerationStatus != domain.FinalReportGenerationStatusComplete ||
			report.ExpectedSubReportCount != 1 || report.SuccessfulSubReportCount != 1 || report.FailedSubReportCount != 0 {
			t.Fatalf("persisted final report = %+v", report)
		}
		linked, err := uow.Reports().ListSubReportsForFinalReport(ctx, report.ID, 10)
		if err != nil {
			return err
		}
		if len(linked) != 1 || linked[0].ID != domain.SubReportID(sub.SubReportID) {
			t.Fatalf("linked subreports = %+v, want only %d", linked, sub.SubReportID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify final report: %v", err)
	}
}

func TestReportActivities_GenerateFinalReportPersistsPartialCoverage(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-final-partial")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := workflowTestContext(t)

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey:         "window-report-final-partial",
		SubReportIDs:           []int64{sub.SubReportID},
		ExpectedSubReportCount: 2,
		FailedSubReportCount:   1,
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport: %v", err)
	}
	if !strings.Contains(provider.Prompt("final_report:window-report-final-partial"), "Coverage: 1 of 2 expected SubReports succeeded; 1 failed.") {
		t.Fatalf("partial report prompt = %s", provider.Prompt("final_report:window-report-final-partial"))
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		report, err := uow.Reports().FindFinalReportByID(ctx, domain.FinalReportID(final.FinalReportID))
		if err != nil {
			return err
		}
		if report.GenerationStatus != domain.FinalReportGenerationStatusPartial ||
			report.ExpectedSubReportCount != 2 || report.SuccessfulSubReportCount != 1 || report.FailedSubReportCount != 1 {
			t.Fatalf("persisted partial final report = %+v", report)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify partial final report: %v", err)
	}
}

func TestReportActivities_GenerateFinalReportRejectsIdempotencyIdentityDrift(t *testing.T) {
	firstSeed := seedDiagnosisTask(t, "report-final-identity-first")
	secondSeed := seedDiagnosisTask(t, "report-final-identity-second")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := workflowTestContext(t)

	generateSubReport := func(seed seededDiagnosisTask) int64 {
		t.Helper()
		result, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
			EvidenceSnapshotID: int64(seed.SnapshotID),
			Scenario:           "single_alert",
			GroupIndex:         0,
		})
		if err != nil {
			t.Fatalf("GenerateSubReport: %v", err)
		}
		return result.SubReportID
	}
	firstSubReportID := generateSubReport(firstSeed)
	secondSubReportID := generateSubReport(secondSeed)
	first, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-final-identity",
		SubReportIDs:   []int64{firstSubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport first: %v", err)
	}
	if _, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-final-identity",
		SubReportIDs:   []int64{secondSubReportID},
	}); err == nil {
		t.Fatal("GenerateFinalReport reused an idempotency key with different SubReport links")
	}
	if provider.Calls("final_report:window-report-final-identity") != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.Calls("final_report:window-report-final-identity"))
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		linked, err := uow.Reports().ListSubReportsForFinalReport(ctx, domain.FinalReportID(first.FinalReportID), 2)
		if err != nil {
			return err
		}
		if len(linked) != 1 || int64(linked[0].ID) != firstSubReportID {
			t.Fatalf("persisted links after drift attempt = %+v", linked)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify idempotent final report links: %v", err)
	}
}

func TestReportActivities_SendReportNotificationLoadsPersistedReport(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-notify-activity")
	provider := newReportLLMProvider()
	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "msg-1",
		Status:            "accepted",
		Raw:               json.RawMessage(`{"message_id":"msg-1","status":"accepted"}`),
	}}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithLLMProvider(provider),
		temporalpkg.WithIMProvider(im),
	)
	ctx := context.Background()

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-notify-activity",
		SubReportIDs:   []int64{sub.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport: %v", err)
	}

	notification, err := activities.SendReportNotification(ctx, temporalpkg.ReportNotificationActivityInput{
		FinalReportID: final.FinalReportID,
	})
	if err != nil {
		t.Fatalf("SendReportNotification: %v", err)
	}
	wantKey := "final_report:" + strconv.FormatInt(final.FinalReportID, 10) + "/notification/handoff"
	if notification.FinalReportID != final.FinalReportID ||
		notification.NotificationIdempotencyKey != wantKey ||
		notification.ProviderMessageID != "msg-1" ||
		notification.Status != "accepted" {
		t.Fatalf("notification result = %+v", notification)
	}
	requests := im.Requests()
	if len(requests) != 1 {
		t.Fatalf("notification requests len = %d, want 1", len(requests))
	}
	if requests[0].IdempotencyKey != wantKey {
		t.Fatalf("IdempotencyKey = %q, want %q", requests[0].IdempotencyKey, wantKey)
	}
	if requests[0].FinalReportID != final.FinalReportID || requests[0].Body == "" || requests[0].Title == "" {
		t.Fatalf("notification request = %+v", requests[0])
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		delivery, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, wantKey)
		if err != nil {
			return err
		}
		if delivery.FinalReportID != domain.FinalReportID(final.FinalReportID) ||
			delivery.Status != domain.ReportNotificationDeliveryStatusDelivered ||
			delivery.ProviderMessageID != "msg-1" ||
			delivery.ProviderStatus != "accepted" ||
			delivery.DeliveredAt == nil {
			t.Fatalf("delivery log = %+v", delivery)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify delivery log: %v", err)
	}

	again, err := activities.SendReportNotification(ctx, temporalpkg.ReportNotificationActivityInput{
		FinalReportID: final.FinalReportID,
	})
	if err != nil {
		t.Fatalf("SendReportNotification repeat: %v", err)
	}
	if again.NotificationIdempotencyKey != wantKey ||
		again.ProviderMessageID != "msg-1" ||
		again.Status != "accepted" {
		t.Fatalf("repeat notification result = %+v", again)
	}
	if got := len(im.Requests()); got != 1 {
		t.Fatalf("repeat provider calls = %d, want 1", got)
	}
}

func TestReportActivities_SendReportNotificationUsesProfileResolver(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-notify-profile-resolver")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := context.Background()
	profile, err := domain.NewNotificationChannelProfile(
		"Report profile resolver",
		domain.NotificationChannelKindWebhook,
		"secret/report-profile-resolver",
		[]domain.NotificationDeliveryScope{domain.NotificationDeliveryScopeReport},
		true,
		nil,
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelProfile: %v", err)
	}
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var saveErr error
		profile, saveErr = uow.Config().SaveNotificationChannelProfile(ctx, profile)
		return saveErr
	})
	if err != nil {
		t.Fatalf("SaveNotificationChannelProfile: %v", err)
	}

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-notify-profile-resolver",
		SubReportIDs:   []int64{sub.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport: %v", err)
	}

	im := &recordingIMProvider{delivery: ports.IMDelivery{
		ProviderMessageID: "profile-msg-1",
		Status:            "accepted",
		Raw:               json.RawMessage(`{"message_id":"profile-msg-1","status":"accepted"}`),
	}}
	resolver := &recordingNotificationProviderResolver{provider: im}
	notifyActivities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithNotificationChannelProviderResolver(resolver),
	)

	notification, err := notifyActivities.SendReportNotification(ctx, temporalpkg.ReportNotificationActivityInput{
		FinalReportID:                      final.FinalReportID,
		ReportNotificationChannelProfileID: int64(profile.ID),
	})
	if err != nil {
		t.Fatalf("SendReportNotification: %v", err)
	}
	calls, profileID := resolver.LastCall()
	if calls != 1 || profileID != profile.ID {
		t.Fatalf("resolver calls=%d profileID=%d, want 1/%d", calls, profileID, profile.ID)
	}
	if scope := resolver.LastScope(); scope != domain.NotificationDeliveryScopeReport {
		t.Fatalf("resolver scope = %s, want %s", scope, domain.NotificationDeliveryScopeReport)
	}
	if notification.ProviderMessageID != "profile-msg-1" || notification.Status != "accepted" {
		t.Fatalf("notification = %+v", notification)
	}
	if len(im.Requests()) != 1 {
		t.Fatalf("provider request count = %d, want 1", len(im.Requests()))
	}
}

func TestReportActivities_SendReportNotificationPersistsFailure(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-notify-failure")
	provider := newReportLLMProvider()
	im := &recordingIMProvider{err: &ports.IMError{Message: "webhook unavailable", StatusCode: 503, Retryable: true}}
	activities := temporalpkg.NewActivities(
		env.factory,
		temporalpkg.WithLLMProvider(provider),
		temporalpkg.WithIMProvider(im),
	)
	ctx := context.Background()

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-notify-failure",
		SubReportIDs:   []int64{sub.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport: %v", err)
	}

	_, err = activities.SendReportNotification(ctx, temporalpkg.ReportNotificationActivityInput{
		FinalReportID: final.FinalReportID,
	})
	if err == nil {
		t.Fatalf("SendReportNotification: want provider error")
	}
	wantKey := "final_report:" + strconv.FormatInt(final.FinalReportID, 10) + "/notification/handoff"
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		delivery, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, wantKey)
		if err != nil {
			return err
		}
		var failurePayload struct {
			StatusCode int `json:"status_code"`
		}
		if err := json.Unmarshal(delivery.Raw, &failurePayload); err != nil {
			t.Fatalf("decode failure raw: %v", err)
		}
		if delivery.Status != domain.ReportNotificationDeliveryStatusFailed ||
			delivery.FailureReason == "" ||
			delivery.DeliveredAt != nil ||
			failurePayload.StatusCode != 503 {
			t.Fatalf("failed delivery log = %+v raw=%s", delivery, delivery.Raw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify failed delivery log: %v", err)
	}
}

func TestReportActivities_SendReportNotificationPersistsProviderResolutionFailure(t *testing.T) {
	seed := seedDiagnosisTask(t, "report-notify-provider-resolution-failure")
	provider := newReportLLMProvider()
	activities := temporalpkg.NewActivities(env.factory, temporalpkg.WithLLMProvider(provider))
	ctx := context.Background()

	sub, err := activities.GenerateSubReport(ctx, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(seed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("GenerateSubReport: %v", err)
	}
	final, err := activities.GenerateFinalReport(ctx, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-report-notify-provider-resolution-failure",
		SubReportIDs:   []int64{sub.SubReportID},
	})
	if err != nil {
		t.Fatalf("GenerateFinalReport: %v", err)
	}

	_, err = activities.SendReportNotification(ctx, temporalpkg.ReportNotificationActivityInput{
		FinalReportID: final.FinalReportID,
	})
	if err == nil || !strings.Contains(err.Error(), "im provider is not configured") {
		t.Fatalf("SendReportNotification err = %v, want provider configuration error", err)
	}
	wantKey := "final_report:" + strconv.FormatInt(final.FinalReportID, 10) + "/notification/handoff"
	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		delivery, err := uow.Reports().FindNotificationDeliveryByIdempotencyKey(ctx, wantKey)
		if err != nil {
			return err
		}
		var failurePayload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(delivery.Raw, &failurePayload); err != nil {
			t.Fatalf("decode failure raw: %v", err)
		}
		if delivery.Status != domain.ReportNotificationDeliveryStatusFailed ||
			!strings.Contains(delivery.FailureReason, "im provider is not configured") ||
			!strings.Contains(failurePayload.Error, "im provider is not configured") ||
			delivery.DeliveredAt != nil {
			t.Fatalf("failed delivery log = %+v raw=%s", delivery, delivery.Raw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify failed delivery log: %v", err)
	}
}

func TestReportWorkflows_ReplayCompletedHistories(t *testing.T) {
	ctx := context.Background()

	fanoutSeed := seedDiagnosisTask(t, "report-replay-fanout")
	fanoutWorkflowID := "test-report-fanout-" + strconv.FormatInt(int64(fanoutSeed.SnapshotID), 10)
	fanoutRun, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        fanoutWorkflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.ReportFanOutWorkflow, temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: int64(fanoutSeed.SnapshotID),
		Scenario:           "single_alert",
		GroupIndex:         0,
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow fanout: %v", err)
	}
	var fanoutResult temporalpkg.ReportFanOutWorkflowResult
	if err := fanoutRun.Get(ctx, &fanoutResult); err != nil {
		t.Fatalf("fanout workflow result: %v", err)
	}
	if fanoutResult.SubReportID == 0 {
		t.Fatal("fanout workflow returned zero SubReportID")
	}
	replayWorkflowHistoryWithRegistrations(ctx, t, fanoutWorkflowID, fanoutRun.GetRunID(), temporalpkg.ReportFanOutWorkflow)

	finalWorkflowID := "test-report-final-" + strconv.FormatInt(fanoutResult.SubReportID, 10)
	finalRun, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        finalWorkflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.FinalReportWorkflow, temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: finalWorkflowID,
		SubReportIDs:   []int64{fanoutResult.SubReportID},
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow final: %v", err)
	}
	var finalResult temporalpkg.FinalReportWorkflowResult
	if err := finalRun.Get(ctx, &finalResult); err != nil {
		t.Fatalf("final workflow result: %v", err)
	}
	if finalResult.FinalReportID == 0 ||
		finalResult.NotificationIdempotencyKey == "" ||
		finalResult.NotificationStatus == "" {
		t.Fatalf("final workflow returned incomplete result: %+v", finalResult)
	}
	replayWorkflowHistoryWithRegistrations(ctx, t, finalWorkflowID, finalRun.GetRunID(), temporalpkg.FinalReportWorkflow)

	firstBatchSeed := seedDiagnosisTask(t, "report-replay-batch-a")
	secondBatchSeed := seedDiagnosisTask(t, "report-replay-batch-b")
	batchWorkflowID := "test-report-batch-" +
		strconv.FormatInt(int64(firstBatchSeed.SnapshotID), 10) + "-" +
		strconv.FormatInt(int64(secondBatchSeed.SnapshotID), 10)
	batchRun, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        batchWorkflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.ReportBatchWorkflow, temporalpkg.ReportBatchWorkflowInput{
		CorrelationKey: batchWorkflowID,
		Items: []temporalpkg.ReportBatchItem{
			{EvidenceSnapshotID: int64(firstBatchSeed.SnapshotID), Scenario: "single_alert", GroupIndex: 0},
			{EvidenceSnapshotID: int64(secondBatchSeed.SnapshotID), Scenario: "cascade", GroupIndex: 1},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow batch: %v", err)
	}
	var batchResult temporalpkg.ReportBatchWorkflowResult
	if err := batchRun.Get(ctx, &batchResult); err != nil {
		t.Fatalf("batch workflow result: %v", err)
	}
	if len(batchResult.SubReportIDs) != 2 ||
		batchResult.FinalReportID == 0 ||
		batchResult.NotificationIdempotencyKey == "" ||
		batchResult.NotificationStatus == "" {
		t.Fatalf("batch workflow returned incomplete result: %+v", batchResult)
	}
	replayWorkflowHistoryWithRegistrations(
		ctx,
		t,
		batchWorkflowID,
		batchRun.GetRunID(),
		temporalpkg.ReportBatchWorkflow,
		temporalpkg.ReportFanOutWorkflow,
		temporalpkg.FinalReportWorkflow,
	)
}

func TestReportFanOutWorkflow_ExecutesGenerateSubReportActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	input := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: 42,
		Scenario:           "single_alert",
		GroupIndex:         0,
	}
	calls := make([]string, 0, 3)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.EnsureReportTaskInput) (temporalpkg.EnsureReportTaskResult, error) {
			calls = append(calls, "ensure")
			if got.EvidenceSnapshotID != input.EvidenceSnapshotID || got.Scenario != input.Scenario ||
				got.GroupIndex != input.GroupIndex || got.WorkflowID == "" || got.RunID == "" || got.StartedAt.IsZero() {
				t.Fatalf("ensure input = %+v", got)
			}
			return temporalpkg.EnsureReportTaskResult{DiagnosisTaskID: 55}, nil
		},
		activity.RegisterOptions{Name: "EnsureReportTask"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportFanOutWorkflowInput) (temporalpkg.ReportFanOutWorkflowResult, error) {
			calls = append(calls, "generate")
			if got != input {
				t.Fatalf("activity input = %+v, want %+v", got, input)
			}
			return temporalpkg.ReportFanOutWorkflowResult{SubReportID: 99}, nil
		},
		activity.RegisterOptions{Name: "GenerateSubReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinishReportTaskInput) (temporalpkg.FinishReportTaskResult, error) {
			calls = append(calls, "finish")
			if got.DiagnosisTaskID != 55 || got.EvidenceSnapshotID != input.EvidenceSnapshotID ||
				got.Scenario != input.Scenario || got.GroupIndex != input.GroupIndex ||
				got.SubReportID != 99 || got.Status != "succeeded" || got.FailureReason != "" || got.FinishedAt.IsZero() {
				t.Fatalf("finish input = %+v", got)
			}
			return temporalpkg.FinishReportTaskResult{LifecycleEventID: 77}, nil
		},
		activity.RegisterOptions{Name: "FinishReportTask"},
	)

	tw.ExecuteWorkflow(temporalpkg.ReportFanOutWorkflow, input)
	if !tw.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := tw.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result temporalpkg.ReportFanOutWorkflowResult
	if err := tw.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult: %v", err)
	}
	if result.SubReportID != 99 {
		t.Fatalf("SubReportID = %d, want 99", result.SubReportID)
	}
	if strings.Join(calls, ",") != "ensure,generate,finish" {
		t.Fatalf("activity calls = %v", calls)
	}
}

func TestFinalReportWorkflow_ExecutesGenerateFinalReportActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	input := temporalpkg.FinalReportWorkflowInput{
		CorrelationKey:                     "window-1",
		ReportNotificationChannelProfileID: 5,
		SubReportIDs:                       []int64{10, 11},
	}
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
			if got.CorrelationKey != input.CorrelationKey || len(got.SubReportIDs) != len(input.SubReportIDs) ||
				got.SubReportIDs[0] != input.SubReportIDs[0] || got.SubReportIDs[1] != input.SubReportIDs[1] ||
				got.ReportNotificationChannelProfileID != input.ReportNotificationChannelProfileID {
				t.Fatalf("activity input = %+v, want %+v", got, input)
			}
			return temporalpkg.FinalReportWorkflowResult{FinalReportID: 77}, nil
		},
		activity.RegisterOptions{Name: "GenerateFinalReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
			if got.FinalReportID != 77 || got.ReportNotificationChannelProfileID != 5 {
				t.Fatalf("notification input = %+v, want final report 77 channel profile 5", got)
			}
			return temporalpkg.ReportNotificationResult{
				FinalReportID:              got.FinalReportID,
				NotificationIdempotencyKey: "final_report:77/notification/handoff",
				ProviderMessageID:          "msg-77",
				Status:                     "delivered",
			}, nil
		},
		activity.RegisterOptions{Name: "SendReportNotification"},
	)

	tw.ExecuteWorkflow(temporalpkg.FinalReportWorkflow, input)
	if !tw.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := tw.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result temporalpkg.FinalReportWorkflowResult
	if err := tw.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult: %v", err)
	}
	if result.FinalReportID != 77 {
		t.Fatalf("FinalReportID = %d, want 77", result.FinalReportID)
	}
	if result.NotificationIdempotencyKey != "final_report:77/notification/handoff" ||
		result.ProviderMessageID != "msg-77" ||
		result.NotificationStatus != "delivered" {
		t.Fatalf("notification fields = %+v", result)
	}
}

func TestReportBatchWorkflow_LegacyVersionPreservesResultShape(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	tw.RegisterWorkflow(temporalpkg.ReportFanOutWorkflow)
	tw.RegisterWorkflow(temporalpkg.FinalReportWorkflow)
	tw.OnGetVersion("report-batch-partial-fan-in", workflow.DefaultVersion, workflow.Version(1)).
		Return(workflow.DefaultVersion)
	tw.OnGetVersion("report-fan-out-task-lifecycle", workflow.DefaultVersion, workflow.Version(1)).
		Return(workflow.DefaultVersion)
	tw.OnGetVersion("final-report-coverage-input", workflow.DefaultVersion, workflow.Version(1)).
		Return(workflow.DefaultVersion)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportFanOutWorkflowInput) (temporalpkg.ReportFanOutWorkflowResult, error) {
			return temporalpkg.ReportFanOutWorkflowResult{SubReportID: got.EvidenceSnapshotID * 10}, nil
		},
		activity.RegisterOptions{Name: "GenerateSubReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
			if got.ExpectedSubReportCount != 0 || got.FailedSubReportCount != 0 {
				t.Fatalf("legacy final-report input gained coverage fields: %+v", got)
			}
			return temporalpkg.FinalReportWorkflowResult{FinalReportID: 90}, nil
		},
		activity.RegisterOptions{Name: "GenerateFinalReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
			return temporalpkg.ReportNotificationResult{
				FinalReportID:              got.FinalReportID,
				NotificationIdempotencyKey: "final_report:90/notification",
				Status:                     "delivered",
			}, nil
		},
		activity.RegisterOptions{Name: "SendReportNotification"},
	)

	tw.ExecuteWorkflow(temporalpkg.ReportBatchWorkflow, temporalpkg.ReportBatchWorkflowInput{
		CorrelationKey: "legacy-window",
		Items: []temporalpkg.ReportBatchItem{{
			EvidenceSnapshotID: 9,
			Scenario:           "single_alert",
		}},
	})
	if err := tw.GetWorkflowError(); err != nil {
		t.Fatalf("legacy workflow error: %v", err)
	}
	var result temporalpkg.ReportBatchWorkflowResult
	if err := tw.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult: %v", err)
	}
	if result.ExpectedSubReportCount != 0 || result.SuccessfulSubReportCount != 0 ||
		result.FailedSubReportCount != 0 || result.FailedItems != nil {
		t.Fatalf("legacy result shape gained coverage fields: %+v", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal legacy result: %v", err)
	}
	for _, field := range []string{"ExpectedSubReportCount", "SuccessfulSubReportCount", "FailedSubReportCount", "FailedItems"} {
		if strings.Contains(string(payload), field) {
			t.Fatalf("legacy result payload %s contains new field %q", payload, field)
		}
	}
}

func TestReportBatchWorkflow_FansOutThenFinalizes(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	tw.RegisterWorkflow(temporalpkg.ReportFanOutWorkflow)
	tw.RegisterWorkflow(temporalpkg.FinalReportWorkflow)

	input := temporalpkg.ReportBatchWorkflowInput{
		CorrelationKey:                     "window-batch",
		ReportNotificationChannelProfileID: 5,
		Items: []temporalpkg.ReportBatchItem{
			{EvidenceSnapshotID: 7, Scenario: "single_alert", GroupIndex: 0},
			{EvidenceSnapshotID: 8, Scenario: "cascade", GroupIndex: 1},
		},
	}
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.EnsureReportTaskInput) (temporalpkg.EnsureReportTaskResult, error) {
			if got.EvidenceSnapshotID == 0 || got.WorkflowID == "" || got.RunID == "" || got.StartedAt.IsZero() {
				t.Fatalf("ensure input = %+v", got)
			}
			return temporalpkg.EnsureReportTaskResult{DiagnosisTaskID: got.EvidenceSnapshotID + 1000}, nil
		},
		activity.RegisterOptions{Name: "EnsureReportTask"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportFanOutWorkflowInput) (temporalpkg.ReportFanOutWorkflowResult, error) {
			for _, item := range input.Items {
				if got.EvidenceSnapshotID == item.EvidenceSnapshotID && got.Scenario == item.Scenario && got.GroupIndex == item.GroupIndex {
					return temporalpkg.ReportFanOutWorkflowResult{
						SubReportID: got.EvidenceSnapshotID*10 + int64(got.GroupIndex),
					}, nil
				}
			}
			t.Fatalf("unexpected fanout input: %+v", got)
			return temporalpkg.ReportFanOutWorkflowResult{}, nil
		},
		activity.RegisterOptions{Name: "GenerateSubReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinishReportTaskInput) (temporalpkg.FinishReportTaskResult, error) {
			if got.DiagnosisTaskID != got.EvidenceSnapshotID+1000 || got.SubReportID == 0 || got.Status != "succeeded" {
				t.Fatalf("finish input = %+v", got)
			}
			return temporalpkg.FinishReportTaskResult{LifecycleEventID: got.DiagnosisTaskID + 2000}, nil
		},
		activity.RegisterOptions{Name: "FinishReportTask"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
			if got.CorrelationKey != "window-batch" {
				t.Fatalf("final correlation = %q, want window-batch", got.CorrelationKey)
			}
			if got.ReportNotificationChannelProfileID != 5 {
				t.Fatalf("final channel profile = %d, want 5", got.ReportNotificationChannelProfileID)
			}
			want := []int64{70, 81}
			if len(got.SubReportIDs) != len(want) || got.SubReportIDs[0] != want[0] || got.SubReportIDs[1] != want[1] {
				t.Fatalf("final SubReportIDs = %+v, want %+v", got.SubReportIDs, want)
			}
			if got.ExpectedSubReportCount != 2 || got.FailedSubReportCount != 0 {
				t.Fatalf("final coverage = %+v", got)
			}
			return temporalpkg.FinalReportWorkflowResult{FinalReportID: 500}, nil
		},
		activity.RegisterOptions{Name: "GenerateFinalReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
			if got.FinalReportID != 500 || got.ReportNotificationChannelProfileID != 5 {
				t.Fatalf("notification input = %+v, want final report 500 channel profile 5", got)
			}
			return temporalpkg.ReportNotificationResult{
				FinalReportID:              got.FinalReportID,
				NotificationIdempotencyKey: "final_report:500/notification/handoff",
				ProviderMessageID:          "msg-500",
				Status:                     "delivered",
			}, nil
		},
		activity.RegisterOptions{Name: "SendReportNotification"},
	)

	tw.ExecuteWorkflow(temporalpkg.ReportBatchWorkflow, input)
	if !tw.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := tw.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result temporalpkg.ReportBatchWorkflowResult
	if err := tw.GetWorkflowResult(&result); err != nil {
		t.Fatalf("GetWorkflowResult: %v", err)
	}
	if result.FinalReportID != 500 ||
		result.NotificationIdempotencyKey != "final_report:500/notification/handoff" ||
		result.ProviderMessageID != "msg-500" ||
		result.NotificationStatus != "delivered" {
		t.Fatalf("batch result = %+v", result)
	}
	if result.ExpectedSubReportCount != 2 || result.SuccessfulSubReportCount != 2 ||
		result.FailedSubReportCount != 0 || len(result.FailedItems) != 0 {
		t.Fatalf("batch coverage = %+v", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal batch result: %v", err)
	}
	for _, field := range []string{"ExpectedSubReportCount", "SuccessfulSubReportCount", "FailedSubReportCount", "FailedItems"} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("new batch result payload %s omits coverage field %q", payload, field)
		}
	}
	wantSubReports := []int64{70, 81}
	if len(result.SubReportIDs) != len(wantSubReports) || result.SubReportIDs[0] != wantSubReports[0] || result.SubReportIDs[1] != wantSubReports[1] {
		t.Fatalf("batch SubReportIDs = %+v, want %+v", result.SubReportIDs, wantSubReports)
	}
}

func TestReportBatchWorkflow_EnforcesPartialFailurePolicy(t *testing.T) {
	tests := []struct {
		name          string
		maxFailures   int
		wantSucceeded bool
	}{
		{name: "within threshold produces partial report", maxFailures: 1, wantSucceeded: true},
		{name: "above threshold rejects fan in", maxFailures: 0, wantSucceeded: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			tw := suite.NewTestWorkflowEnvironment()
			tw.RegisterWorkflow(temporalpkg.ReportFanOutWorkflow)
			tw.RegisterWorkflow(temporalpkg.FinalReportWorkflow)

			input := temporalpkg.ReportBatchWorkflowInput{
				CorrelationKey:      "window-partial",
				MaxFailedSubReports: tc.maxFailures,
				Items: []temporalpkg.ReportBatchItem{
					{EvidenceSnapshotID: 7, Scenario: "single_alert", GroupIndex: 0},
					{EvidenceSnapshotID: 8, Scenario: "cascade", GroupIndex: 1},
				},
			}
			var mu sync.Mutex
			generated := make(map[int64]int, len(input.Items))
			finished := make(map[int64]temporalpkg.FinishReportTaskInput, len(input.Items))
			finalCalled := false

			tw.RegisterActivityWithOptions(
				func(_ context.Context, got temporalpkg.EnsureReportTaskInput) (temporalpkg.EnsureReportTaskResult, error) {
					return temporalpkg.EnsureReportTaskResult{DiagnosisTaskID: got.EvidenceSnapshotID + 1000}, nil
				},
				activity.RegisterOptions{Name: "EnsureReportTask"},
			)
			tw.RegisterActivityWithOptions(
				func(_ context.Context, got temporalpkg.ReportFanOutWorkflowInput) (temporalpkg.ReportFanOutWorkflowResult, error) {
					mu.Lock()
					generated[got.EvidenceSnapshotID]++
					mu.Unlock()
					if got.EvidenceSnapshotID == 8 {
						return temporalpkg.ReportFanOutWorkflowResult{}, errors.New("provider credential secret")
					}
					return temporalpkg.ReportFanOutWorkflowResult{SubReportID: 70}, nil
				},
				activity.RegisterOptions{Name: "GenerateSubReport"},
			)
			tw.RegisterActivityWithOptions(
				func(_ context.Context, got temporalpkg.FinishReportTaskInput) (temporalpkg.FinishReportTaskResult, error) {
					mu.Lock()
					finished[got.EvidenceSnapshotID] = got
					mu.Unlock()
					if got.EvidenceSnapshotID == 7 && (got.Status != "succeeded" || got.SubReportID != 70 || got.FailureReason != "") {
						t.Fatalf("successful finish input = %+v", got)
					}
					if got.EvidenceSnapshotID == 8 && (got.Status != "failed" || got.SubReportID != 0 || got.FailureReason != "subreport_generation_failed") {
						t.Fatalf("failed finish input = %+v", got)
					}
					return temporalpkg.FinishReportTaskResult{LifecycleEventID: got.DiagnosisTaskID + 2000}, nil
				},
				activity.RegisterOptions{Name: "FinishReportTask"},
			)
			tw.RegisterActivityWithOptions(
				func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
					mu.Lock()
					finalCalled = true
					mu.Unlock()
					if got.CorrelationKey != input.CorrelationKey || len(got.SubReportIDs) != 1 || got.SubReportIDs[0] != 70 ||
						got.ExpectedSubReportCount != 2 || got.FailedSubReportCount != 1 {
						t.Fatalf("partial final input = %+v", got)
					}
					return temporalpkg.FinalReportWorkflowResult{FinalReportID: 501}, nil
				},
				activity.RegisterOptions{Name: "GenerateFinalReport"},
			)
			tw.RegisterActivityWithOptions(
				func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
					return temporalpkg.ReportNotificationResult{
						FinalReportID:              got.FinalReportID,
						NotificationIdempotencyKey: "final_report:501/notification/handoff",
						Status:                     "delivered",
					}, nil
				},
				activity.RegisterOptions{Name: "SendReportNotification"},
			)

			tw.ExecuteWorkflow(temporalpkg.ReportBatchWorkflow, input)
			if !tw.IsWorkflowCompleted() {
				t.Fatal("workflow did not complete")
			}
			workflowErr := tw.GetWorkflowError()
			if tc.wantSucceeded {
				if workflowErr != nil {
					t.Fatalf("workflow error: %v", workflowErr)
				}
				var result temporalpkg.ReportBatchWorkflowResult
				if err := tw.GetWorkflowResult(&result); err != nil {
					t.Fatalf("GetWorkflowResult: %v", err)
				}
				if result.FinalReportID != 501 || len(result.SubReportIDs) != 1 || result.SubReportIDs[0] != 70 ||
					result.ExpectedSubReportCount != 2 || result.SuccessfulSubReportCount != 1 || result.FailedSubReportCount != 1 ||
					len(result.FailedItems) != 1 || result.FailedItems[0].ItemIndex != 1 ||
					result.FailedItems[0].Reason != "subreport_generation_failed" {
					t.Fatalf("partial batch result = %+v", result)
				}
				if strings.Contains(result.FailedItems[0].Reason, "provider credential") {
					t.Fatalf("partial result leaked provider error: %+v", result.FailedItems[0])
				}
			} else {
				if workflowErr == nil {
					t.Fatal("workflow succeeded above the failure threshold")
				}
				if !strings.Contains(workflowErr.Error(), "policy allows at most 0 failures") ||
					strings.Contains(workflowErr.Error(), "provider credential secret") {
					t.Fatalf("threshold workflow error = %v", workflowErr)
				}
			}

			mu.Lock()
			defer mu.Unlock()
			if generated[7] == 0 || generated[8] == 0 || len(finished) != 2 {
				t.Fatalf("fan-out activity coverage: generated=%v finished=%v", generated, finished)
			}
			if finalCalled != tc.wantSucceeded {
				t.Fatalf("finalCalled = %t, want %t", finalCalled, tc.wantSucceeded)
			}
		})
	}
}

func TestReportWorkflows_RejectInvalidInputBeforeActivity(t *testing.T) {
	tests := []struct {
		name       string
		workflow   any
		input      any
		wantSubstr string
	}{
		{
			name:       "fanout zero snapshot",
			workflow:   temporalpkg.ReportFanOutWorkflow,
			input:      temporalpkg.ReportFanOutWorkflowInput{Scenario: "single_alert"},
			wantSubstr: "evidence_snapshot_id must be non-zero",
		},
		{
			name:       "fanout empty scenario",
			workflow:   temporalpkg.ReportFanOutWorkflow,
			input:      temporalpkg.ReportFanOutWorkflowInput{EvidenceSnapshotID: 1},
			wantSubstr: "scenario must be non-empty",
		},
		{
			name:       "batch empty correlation",
			workflow:   temporalpkg.ReportBatchWorkflow,
			input:      temporalpkg.ReportBatchWorkflowInput{Items: []temporalpkg.ReportBatchItem{{EvidenceSnapshotID: 1, Scenario: "single_alert"}}},
			wantSubstr: "correlation_key must be non-empty",
		},
		{
			name:       "batch empty items",
			workflow:   temporalpkg.ReportBatchWorkflow,
			input:      temporalpkg.ReportBatchWorkflowInput{CorrelationKey: "window"},
			wantSubstr: "items must be non-empty",
		},
		{
			name:       "batch invalid item",
			workflow:   temporalpkg.ReportBatchWorkflow,
			input:      temporalpkg.ReportBatchWorkflowInput{CorrelationKey: "window", Items: []temporalpkg.ReportBatchItem{{Scenario: "single_alert"}}},
			wantSubstr: "items[0].evidence_snapshot_id must be non-zero",
		},
		{
			name:       "batch negative notification channel",
			workflow:   temporalpkg.ReportBatchWorkflow,
			input:      temporalpkg.ReportBatchWorkflowInput{CorrelationKey: "window", ReportNotificationChannelProfileID: -1, Items: []temporalpkg.ReportBatchItem{{EvidenceSnapshotID: 1, Scenario: "single_alert"}}},
			wantSubstr: "report_notification_channel_profile_id must be >= 0",
		},
		{
			name:     "batch excessive failure tolerance",
			workflow: temporalpkg.ReportBatchWorkflow,
			input: temporalpkg.ReportBatchWorkflowInput{
				CorrelationKey:      "window",
				MaxFailedSubReports: domain.ReportWorkflowMaxFailedSubReports + 1,
				Items:               []temporalpkg.ReportBatchItem{{EvidenceSnapshotID: 1, Scenario: "single_alert"}},
			},
			wantSubstr: "max_failed_sub_reports must be between 0 and 100000",
		},
		{
			name:       "final empty correlation",
			workflow:   temporalpkg.FinalReportWorkflow,
			input:      temporalpkg.FinalReportWorkflowInput{SubReportIDs: []int64{1}},
			wantSubstr: "correlation_key must be non-empty",
		},
		{
			name:       "final empty subreports",
			workflow:   temporalpkg.FinalReportWorkflow,
			input:      temporalpkg.FinalReportWorkflowInput{CorrelationKey: "window"},
			wantSubstr: "sub_report_ids must be non-empty",
		},
		{
			name:       "final negative notification channel",
			workflow:   temporalpkg.FinalReportWorkflow,
			input:      temporalpkg.FinalReportWorkflowInput{CorrelationKey: "window", ReportNotificationChannelProfileID: -1, SubReportIDs: []int64{1}},
			wantSubstr: "report_notification_channel_profile_id must be >= 0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			tw := suite.NewTestWorkflowEnvironment()
			tw.ExecuteWorkflow(tc.workflow, tc.input)
			if !tw.IsWorkflowCompleted() {
				t.Fatal("workflow did not complete")
			}
			err := tw.GetWorkflowError()
			if err == nil {
				t.Fatalf("expected workflow error containing %q, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("workflow error = %q, want substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func int64String(id domain.EvidenceSnapshotID) string {
	return strconv.FormatInt(int64(id), 10)
}
