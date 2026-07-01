package temporal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestDiagnosisRoomEvidenceContextIncludesAgentFacingSummary(t *testing.T) {
	collectedAt := time.Date(2026, 6, 18, 9, 30, 0, 0, time.UTC)
	items := []diagnosisevidence.Item{
		{
			Request: diagnosisroom.EvidenceRequest{
				Tool:   domain.DiagnosisToolKindActiveAlerts,
				Reason: "Need sibling alerts.",
				Limit:  5,
			},
			Tool:           domain.DiagnosisToolKindActiveAlerts,
			Status:         diagnosisevidence.StatusCollected,
			ReasonCode:     diagnosisevidence.ReasonOK,
			Message:        "Active alert collection succeeded.",
			ObservedAlerts: 2,
			ActiveAlerts: []ports.ActiveAlert{
				{
					Source:               "alertmanager",
					AlertSourceProfileID: 7,
					Labels:               map[string]string{"alertname": "HighCPU"},
					StartsAt:             collectedAt.Add(-15 * time.Minute),
				},
			},
			CollectedAt: collectedAt,
		},
		{
			Request: diagnosisroom.EvidenceRequest{
				Tool:   domain.DiagnosisToolKindMetricQuery,
				Query:  "up",
				Reason: "Need current scrape health.",
				Limit:  10,
			},
			Tool:                 domain.DiagnosisToolKindMetricQuery,
			Status:               diagnosisevidence.StatusCollected,
			ReasonCode:           diagnosisevidence.ReasonOK,
			Message:              "Metric query collection succeeded.",
			Query:                "up",
			ObservedMetricSeries: 3,
			MetricResult: ports.MetricQueryResult{
				ResultType: "vector",
				Series: []ports.MetricSeries{{
					Metric: map[string]string{"job": "api"},
					Points: []ports.MetricPoint{{Timestamp: collectedAt, Value: "1"}},
				}},
			},
			CollectedAt: collectedAt.Add(time.Second),
		},
		{
			Request: diagnosisroom.EvidenceRequest{
				Tool:   domain.DiagnosisToolKindMetricRangeQuery,
				Query:  "rate(container_cpu_usage_seconds_total[5m])",
				Reason: "Need CPU trend.",
				Limit:  10,
			},
			Tool:        domain.DiagnosisToolKindMetricRangeQuery,
			Status:      diagnosisevidence.StatusFailed,
			ReasonCode:  diagnosisevidence.ReasonProviderFailed,
			Message:     "Metric range collection failed.",
			Query:       "rate(container_cpu_usage_seconds_total[5m])",
			CollectedAt: collectedAt.Add(2 * time.Second),
		},
	}
	batch, ok := diagnosisRoomEvidenceContextBatchFromItems(2, "msg-1/assistant", items)
	if !ok {
		t.Fatal("batch was not created")
	}
	raw, err := diagnosisRoomEvidenceContext(json.RawMessage(`{"alert":"cpu_saturation"}`), []diagnosisRoomEvidenceContextBatch{batch})
	if err != nil {
		t.Fatalf("diagnosisRoomEvidenceContext: %v", err)
	}

	var got struct {
		Alert     string `json:"alert"`
		Collected []struct {
			TurnCount int `json:"turn_count"`
			Summary   struct {
				TotalRequests        int    `json:"total_requests"`
				CollectedRequests    int    `json:"collected_requests"`
				UnresolvedRequests   int    `json:"unresolved_requests"`
				FailedRequests       int    `json:"failed_requests"`
				SkippedRequests      int    `json:"skipped_requests"`
				UnsupportedRequests  int    `json:"unsupported_requests"`
				ObservedAlerts       int    `json:"observed_alerts"`
				ObservedMetricSeries int    `json:"observed_metric_series"`
				AgentInstruction     string `json:"agent_instruction"`
			} `json:"summary"`
			Items []struct {
				Tool                 string `json:"tool"`
				Status               string `json:"status"`
				ReasonCode           string `json:"reason_code"`
				ObservedAlerts       int    `json:"observed_alerts"`
				ObservedMetricSeries int    `json:"observed_metric_series"`
			} `json:"items"`
		} `json:"openclarion_collected_evidence"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	if got.Alert != "cpu_saturation" {
		t.Fatalf("base alert = %q, want cpu_saturation", got.Alert)
	}
	if len(got.Collected) != 1 {
		t.Fatalf("collected batches = %d, want 1", len(got.Collected))
	}
	summary := got.Collected[0].Summary
	if got.Collected[0].TurnCount != 2 ||
		summary.TotalRequests != 3 ||
		summary.CollectedRequests != 2 ||
		summary.UnresolvedRequests != 1 ||
		summary.FailedRequests != 1 ||
		summary.SkippedRequests != 0 ||
		summary.UnsupportedRequests != 0 ||
		summary.ObservedAlerts != 2 ||
		summary.ObservedMetricSeries != 3 {
		t.Fatalf("summary = %+v", summary)
	}
	if !strings.Contains(summary.AgentInstruction, "reassess confidence") ||
		!strings.Contains(summary.AgentInstruction, "remaining evidence gaps") {
		t.Fatalf("agent instruction = %q, want confidence and evidence-gap guidance", summary.AgentInstruction)
	}
	if len(got.Collected[0].Items) != 3 ||
		got.Collected[0].Items[0].Tool != "active_alerts" ||
		got.Collected[0].Items[0].Status != "collected" ||
		got.Collected[0].Items[1].ObservedMetricSeries != 3 ||
		got.Collected[0].Items[2].Status != "failed" ||
		got.Collected[0].Items[2].ReasonCode != "provider_failed" {
		t.Fatalf("items = %+v", got.Collected[0].Items)
	}
}
