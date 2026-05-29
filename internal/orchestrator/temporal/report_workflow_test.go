package temporal_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
)

type reportLLMProvider struct {
	mu    sync.Mutex
	calls map[string]int
}

type recordingIMProvider struct {
	mu       sync.Mutex
	requests []ports.IMNotification
	delivery ports.IMDelivery
	err      error
}

func newReportLLMProvider() *reportLLMProvider {
	return &reportLLMProvider{calls: map[string]int{}}
}

func (p *reportLLMProvider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	if err := ctx.Err(); err != nil {
		return ports.LLMResponse{}, err
	}
	p.mu.Lock()
	p.calls[req.IdempotencyKey]++
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
	key := "snapshot:" + int64String(seed.SnapshotID) + "/group:0/sub_report"
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
		if report.NotificationText == "" {
			t.Fatalf("NotificationText is empty")
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
	wantKey := "final_report:" + strconv.FormatInt(final.FinalReportID, 10) + "/notification"
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
	wantKey := "final_report:" + strconv.FormatInt(final.FinalReportID, 10) + "/notification"
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

func TestReportFanOutWorkflow_ExecutesGenerateSubReportActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	input := temporalpkg.ReportFanOutWorkflowInput{
		EvidenceSnapshotID: 42,
		Scenario:           "single_alert",
		GroupIndex:         0,
	}
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportFanOutWorkflowInput) (temporalpkg.ReportFanOutWorkflowResult, error) {
			if got != input {
				t.Fatalf("activity input = %+v, want %+v", got, input)
			}
			return temporalpkg.ReportFanOutWorkflowResult{SubReportID: 99}, nil
		},
		activity.RegisterOptions{Name: "GenerateSubReport"},
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
}

func TestFinalReportWorkflow_ExecutesGenerateFinalReportActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	input := temporalpkg.FinalReportWorkflowInput{
		CorrelationKey: "window-1",
		SubReportIDs:   []int64{10, 11},
	}
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
			if got.CorrelationKey != input.CorrelationKey || len(got.SubReportIDs) != len(input.SubReportIDs) ||
				got.SubReportIDs[0] != input.SubReportIDs[0] || got.SubReportIDs[1] != input.SubReportIDs[1] {
				t.Fatalf("activity input = %+v, want %+v", got, input)
			}
			return temporalpkg.FinalReportWorkflowResult{FinalReportID: 77}, nil
		},
		activity.RegisterOptions{Name: "GenerateFinalReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
			if got.FinalReportID != 77 {
				t.Fatalf("notification input FinalReportID = %d, want 77", got.FinalReportID)
			}
			return temporalpkg.ReportNotificationResult{
				FinalReportID:              got.FinalReportID,
				NotificationIdempotencyKey: "final_report:77/notification",
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
	if result.NotificationIdempotencyKey != "final_report:77/notification" ||
		result.ProviderMessageID != "msg-77" ||
		result.NotificationStatus != "delivered" {
		t.Fatalf("notification fields = %+v", result)
	}
}

func TestReportBatchWorkflow_FansOutThenFinalizes(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	tw := suite.NewTestWorkflowEnvironment()
	tw.RegisterWorkflow(temporalpkg.ReportFanOutWorkflow)
	tw.RegisterWorkflow(temporalpkg.FinalReportWorkflow)

	input := temporalpkg.ReportBatchWorkflowInput{
		CorrelationKey: "window-batch",
		Items: []temporalpkg.ReportBatchItem{
			{EvidenceSnapshotID: 7, Scenario: "single_alert", GroupIndex: 0},
			{EvidenceSnapshotID: 8, Scenario: "cascade", GroupIndex: 1},
		},
	}
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
		func(_ context.Context, got temporalpkg.FinalReportWorkflowInput) (temporalpkg.FinalReportWorkflowResult, error) {
			if got.CorrelationKey != "window-batch" {
				t.Fatalf("final correlation = %q, want window-batch", got.CorrelationKey)
			}
			want := []int64{70, 81}
			if len(got.SubReportIDs) != len(want) || got.SubReportIDs[0] != want[0] || got.SubReportIDs[1] != want[1] {
				t.Fatalf("final SubReportIDs = %+v, want %+v", got.SubReportIDs, want)
			}
			return temporalpkg.FinalReportWorkflowResult{FinalReportID: 500}, nil
		},
		activity.RegisterOptions{Name: "GenerateFinalReport"},
	)
	tw.RegisterActivityWithOptions(
		func(_ context.Context, got temporalpkg.ReportNotificationActivityInput) (temporalpkg.ReportNotificationResult, error) {
			if got.FinalReportID != 500 {
				t.Fatalf("notification FinalReportID = %d, want 500", got.FinalReportID)
			}
			return temporalpkg.ReportNotificationResult{
				FinalReportID:              got.FinalReportID,
				NotificationIdempotencyKey: "final_report:500/notification",
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
		result.NotificationIdempotencyKey != "final_report:500/notification" ||
		result.ProviderMessageID != "msg-500" ||
		result.NotificationStatus != "delivered" {
		t.Fatalf("batch result = %+v", result)
	}
	wantSubReports := []int64{70, 81}
	if len(result.SubReportIDs) != len(wantSubReports) || result.SubReportIDs[0] != wantSubReports[0] || result.SubReportIDs[1] != wantSubReports[1] {
		t.Fatalf("batch SubReportIDs = %+v, want %+v", result.SubReportIDs, wantSubReports)
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
