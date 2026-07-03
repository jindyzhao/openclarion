package reporttrigger

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/alertreplay"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportprompt"
)

func TestBuildStartRequest_DefaultsAndMapsSnapshots(t *testing.T) {
	windowStart := time.Date(2026, 5, 26, 12, 0, 0, 123456789, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	replay := alertreplay.Result{
		Snapshots: []alertreplay.SnapshotRef{
			{ID: 101, GroupIndex: 0, EventCount: 2},
			{ID: 102, GroupIndex: 1, EventCount: 1},
		},
	}
	req := Request{
		Replay: alertreplay.Request{
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
		},
	}

	startReq, ok, err := BuildStartRequest(replay, req)
	if err != nil {
		t.Fatalf("BuildStartRequest: %v", err)
	}
	if !ok {
		t.Fatal("BuildStartRequest ok = false, want true")
	}
	wantCorrelation := "alert-replay:2026-05-26T12:00:00.123456Z:2026-05-26T13:00:00.123456Z"
	if startReq.CorrelationKey != wantCorrelation {
		t.Fatalf("CorrelationKey = %q, want %q", startReq.CorrelationKey, wantCorrelation)
	}
	if !strings.HasPrefix(startReq.WorkflowID, "report-batch-") || len(startReq.WorkflowID) != len("report-batch-")+32 {
		t.Fatalf("WorkflowID = %q, want stable report-batch-<128-bit-hex>", startReq.WorkflowID)
	}
	again, ok, err := BuildStartRequest(replay, req)
	if err != nil || !ok {
		t.Fatalf("second BuildStartRequest ok=%v err=%v", ok, err)
	}
	if again.WorkflowID != startReq.WorkflowID {
		t.Fatalf("WorkflowID changed: first=%q second=%q", startReq.WorkflowID, again.WorkflowID)
	}
	wantItems := []ports.ReportBatchStartItem{
		{EvidenceSnapshotID: 101, Scenario: string(reportprompt.ScenarioSingleAlert), GroupIndex: 0},
		{EvidenceSnapshotID: 102, Scenario: string(reportprompt.ScenarioSingleAlert), GroupIndex: 1},
	}
	if len(startReq.Items) != len(wantItems) {
		t.Fatalf("items len = %d, want %d", len(startReq.Items), len(wantItems))
	}
	for i := range wantItems {
		if startReq.Items[i] != wantItems[i] {
			t.Fatalf("items[%d] = %+v, want %+v", i, startReq.Items[i], wantItems[i])
		}
	}
}

func TestBuildStartRequest_DefaultIdentityIncludesAlertEventFilters(t *testing.T) {
	windowStart := time.Date(2026, 5, 26, 12, 0, 0, 123456789, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	replay := alertreplay.Result{
		Snapshots: []alertreplay.SnapshotRef{
			{ID: 101, GroupIndex: 0, EventCount: 1},
		},
	}

	startReq, ok, err := BuildStartRequest(replay, Request{
		Replay: alertreplay.Request{
			WindowStart:        windowStart,
			WindowEnd:          windowEnd,
			AlertEventIDFilter: []domain.AlertEventID{42},
		},
	})
	if err != nil {
		t.Fatalf("BuildStartRequest alert filter: %v", err)
	}
	if !ok {
		t.Fatal("BuildStartRequest ok = false, want true")
	}
	wantCorrelation := "alert-replay:2026-05-26T12:00:00.123456Z:2026-05-26T13:00:00.123456Z:alert-events:42"
	if startReq.CorrelationKey != wantCorrelation {
		t.Fatalf("CorrelationKey = %q, want %q", startReq.CorrelationKey, wantCorrelation)
	}

	otherAlert, ok, err := BuildStartRequest(replay, Request{
		Replay: alertreplay.Request{
			WindowStart:        windowStart,
			WindowEnd:          windowEnd,
			AlertEventIDFilter: []domain.AlertEventID{43},
		},
	})
	if err != nil || !ok {
		t.Fatalf("BuildStartRequest other alert ok=%v err=%v", ok, err)
	}
	if otherAlert.WorkflowID == startReq.WorkflowID {
		t.Fatalf("WorkflowID = %q for different alert filters, want distinct IDs", startReq.WorkflowID)
	}

	ordered, ok, err := BuildStartRequest(replay, Request{
		Replay: alertreplay.Request{
			WindowStart:        windowStart,
			WindowEnd:          windowEnd,
			AlertEventIDFilter: []domain.AlertEventID{43, 42},
		},
	})
	if err != nil || !ok {
		t.Fatalf("BuildStartRequest ordered alert filter ok=%v err=%v", ok, err)
	}
	wantOrderedCorrelation := "alert-replay:2026-05-26T12:00:00.123456Z:2026-05-26T13:00:00.123456Z:alert-events:42,43"
	if ordered.CorrelationKey != wantOrderedCorrelation {
		t.Fatalf("CorrelationKey = %q, want %q", ordered.CorrelationKey, wantOrderedCorrelation)
	}
}

func TestBuildStartRequest_ExplicitValuesAndNoSnapshots(t *testing.T) {
	replay := alertreplay.Result{
		Snapshots: []alertreplay.SnapshotRef{
			{ID: 201, GroupIndex: 4, EventCount: 7},
		},
	}
	startReq, ok, err := BuildStartRequest(replay, Request{
		CorrelationKey:                     " incident-42 ",
		WorkflowID:                         " report-workflow-42 ",
		Scenario:                           reportprompt.ScenarioCascade,
		ReportNotificationChannelProfileID: 3,
	})
	if err != nil {
		t.Fatalf("BuildStartRequest explicit: %v", err)
	}
	if !ok {
		t.Fatal("BuildStartRequest explicit ok = false, want true")
	}
	if startReq.CorrelationKey != "incident-42" || startReq.WorkflowID != "report-workflow-42" {
		t.Fatalf("startReq identity = %+v", startReq)
	}
	if startReq.ReportNotificationChannelProfileID != 3 {
		t.Fatalf("ReportNotificationChannelProfileID = %d, want 3", startReq.ReportNotificationChannelProfileID)
	}
	if got := startReq.Items[0]; got != (ports.ReportBatchStartItem{
		EvidenceSnapshotID: 201,
		Scenario:           string(reportprompt.ScenarioCascade),
		GroupIndex:         4,
	}) {
		t.Fatalf("item = %+v", got)
	}

	empty, ok, err := BuildStartRequest(alertreplay.Result{}, Request{})
	if err != nil {
		t.Fatalf("BuildStartRequest empty: %v", err)
	}
	if ok {
		t.Fatalf("BuildStartRequest empty ok = true, want false with request %+v", empty)
	}
}

func TestBuildStartRequestValidation(t *testing.T) {
	validReplay := alertreplay.Result{
		Snapshots: []alertreplay.SnapshotRef{
			{ID: 1, GroupIndex: 0, EventCount: 1},
		},
	}
	validReq := Request{
		CorrelationKey: "window-1",
		Scenario:       reportprompt.ScenarioSingleAlert,
	}
	cases := []struct {
		name   string
		replay alertreplay.Result
		req    Request
	}{
		{
			name:   "invalid_scenario",
			replay: validReplay,
			req: Request{
				CorrelationKey: "window-1",
				Scenario:       reportprompt.Scenario("unknown"),
			},
		},
		{
			name: "zero_snapshot_id",
			replay: alertreplay.Result{
				Snapshots: []alertreplay.SnapshotRef{{ID: 0, GroupIndex: 0, EventCount: 1}},
			},
			req: validReq,
		},
		{
			name: "negative_group_index",
			replay: alertreplay.Result{
				Snapshots: []alertreplay.SnapshotRef{{ID: 1, GroupIndex: -1, EventCount: 1}},
			},
			req: validReq,
		},
		{
			name: "zero_event_count",
			replay: alertreplay.Result{
				Snapshots: []alertreplay.SnapshotRef{{ID: 1, GroupIndex: 0, EventCount: 0}},
			},
			req: validReq,
		},
		{
			name:   "missing_correlation_with_zero_window",
			replay: validReplay,
			req:    Request{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := BuildStartRequest(tc.replay, tc.req)
			if err == nil {
				t.Fatal("BuildStartRequest: want error, got nil")
			}
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestReplayAndStartRejectsNilStarterBeforeReplay(t *testing.T) {
	_, err := ReplayAndStart(context.Background(), nil, nil, nil, Request{})
	if err == nil {
		t.Fatal("ReplayAndStart: want error, got nil")
	}
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("error = %v, want ErrInvariantViolation", err)
	}
}

func TestNewServiceValidation(t *testing.T) {
	_, err := NewService(noopMetricsProvider{}, noopFactory{}, noopStarter{})
	if err != nil {
		t.Fatalf("NewService valid deps: %v", err)
	}

	cases := []struct {
		name     string
		provider ports.MetricsProvider
		factory  ports.UnitOfWorkFactory
		starter  ports.ReportWorkflowStarter
	}{
		{name: "nil_provider", provider: nil, factory: noopFactory{}, starter: noopStarter{}},
		{name: "nil_factory", provider: noopMetricsProvider{}, factory: nil, starter: noopStarter{}},
		{name: "nil_starter", provider: noopMetricsProvider{}, factory: noopFactory{}, starter: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(tc.provider, tc.factory, tc.starter)
			if err == nil {
				t.Fatal("NewService: want error, got nil")
			}
			if !errors.Is(err, domain.ErrInvariantViolation) {
				t.Fatalf("error = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

type noopMetricsProvider struct{}

func (noopMetricsProvider) ListActiveAlerts(context.Context) ([]ports.ActiveAlert, error) {
	return nil, nil
}

func (noopMetricsProvider) QueryMetric(context.Context, ports.MetricQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
}

func (noopMetricsProvider) QueryMetricRange(context.Context, ports.MetricRangeQueryRequest) (ports.MetricQueryResult, error) {
	return ports.MetricQueryResult{}, nil
}

type noopFactory struct{}

func (noopFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return nil, nil
}

func (noopFactory) WithinTx(context.Context, func(context.Context, ports.UnitOfWork) error) error {
	return nil
}

type noopStarter struct{}

func (noopStarter) StartReportBatch(context.Context, ports.ReportBatchStartRequest) (ports.WorkflowHandle, error) {
	return ports.WorkflowHandle{}, nil
}
