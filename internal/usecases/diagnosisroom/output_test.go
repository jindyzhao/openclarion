package diagnosisroom

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
)

func TestParseTurnOutput_AcceptsValidOutput(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version": "diagnosis_turn.v1",
		"message": "  The alert is likely caused by CPU saturation on api-1.  ",
		"findings": [" CPU is above threshold ", "Error rate is stable"],
		"recommended_actions": ["Check top processes", "Review recent deploys"],
		"evidence_requests": [{
			"template_id": 3,
			"tool": "metric_range_query",
			"reason": "  Need the CPU trend around the alert onset.  ",
			"window_seconds": 3600,
			"step_seconds": 60,
			"limit": 5
		}],
		"confidence": "medium",
		"requires_human_review": true,
		"confidence_rationale": "  CPU evidence is present but memory and restart context are missing.  ",
		"missing_evidence_requests": [{
			"label": "Restart cause",
			"detail": "  Provide previous pod logs before raising confidence.  ",
			"priority": "high"
		}],
		"evidence_collection_suggestions": [{
			"label": "JVM memory trend",
			"detail": "Collect a bounded heap usage range query.",
			"priority": "medium"
		}],
		"conclusion_status": "needs_evidence"
	}`)

	got, err := ParseTurnOutput(raw)
	if err != nil {
		t.Fatalf("ParseTurnOutput: %v", err)
	}
	if got.SchemaVersion != TurnOutputSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", got.SchemaVersion, TurnOutputSchemaVersion)
	}
	if got.Message != "The alert is likely caused by CPU saturation on api-1." {
		t.Fatalf("Message = %q", got.Message)
	}
	if len(got.Findings) != 2 || got.Findings[0] != "CPU is above threshold" {
		t.Fatalf("Findings = %+v", got.Findings)
	}
	if len(got.EvidenceRequests) != 1 {
		t.Fatalf("EvidenceRequests len = %d, want 1", len(got.EvidenceRequests))
	}
	req := got.EvidenceRequests[0]
	if req.TemplateID != 3 ||
		req.Tool != domain.DiagnosisToolKindMetricRangeQuery ||
		req.Reason != "Need the CPU trend around the alert onset." ||
		req.WindowSeconds != 3600 ||
		req.StepSeconds != 60 ||
		req.Limit != 5 {
		t.Fatalf("EvidenceRequests[0] = %+v", req)
	}
	if got.Confidence != "medium" || !got.RequiresHumanReview {
		t.Fatalf("output flags = %+v", got)
	}
	if got.ConfidenceRationale != "CPU evidence is present but memory and restart context are missing." {
		t.Fatalf("ConfidenceRationale = %q", got.ConfidenceRationale)
	}
	if len(got.MissingEvidenceRequests) != 1 ||
		got.MissingEvidenceRequests[0].Detail != "Provide previous pod logs before raising confidence." ||
		got.MissingEvidenceRequests[0].Priority != "high" {
		t.Fatalf("MissingEvidenceRequests = %+v", got.MissingEvidenceRequests)
	}
	if len(got.EvidenceCollectionSuggestions) != 1 ||
		got.EvidenceCollectionSuggestions[0].Label != "JVM memory trend" ||
		got.EvidenceCollectionSuggestions[0].Priority != "medium" {
		t.Fatalf("EvidenceCollectionSuggestions = %+v", got.EvidenceCollectionSuggestions)
	}
	if got.ConclusionStatus != "needs_evidence" {
		t.Fatalf("ConclusionStatus = %q", got.ConclusionStatus)
	}
}

func TestParseTurnOutput_NormalizesToolRequestSuggestions(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version": "diagnosis_turn.v1",
		"message": "The alert needs more database and Kubernetes evidence.",
		"confidence": "low",
		"requires_human_review": true,
		"confidence_rationale": "The current alert batch is correlated, but supporting metrics are incomplete.",
		"conclusion_status": "needs_evidence",
		"tool_request_suggestions": [{
			"label": "  Check active alerts  ",
			"detail": "  Verify whether sibling database or storage alerts are firing.  ",
			"priority": "high",
			"tool": "active_alerts",
			"limit": 5
		}, {
			"label": "Current tablespace usage",
			"detail": "Query the current tablespace usage percentage.",
			"priority": "high",
			"tool": "metric_query",
			"query": "  oracle_tablespace_usage_percent{tablespace=\"OMPLATFORM\"}  ",
			"limit": 10
		}, {
			"label": "Tablespace usage trend",
			"detail": "Check the growth trend before deciding on expansion urgency.",
			"priority": "medium",
			"tool": "metric_range_query",
			"query": "oracle_tablespace_usage_percent{tablespace=\"OMPLATFORM\"}",
			"limit": 10
		}]
	}`)

	got, err := ParseTurnOutput(raw)
	if err != nil {
		t.Fatalf("ParseTurnOutput: %v", err)
	}
	if len(got.ToolRequestSuggestions) != 0 {
		t.Fatalf("ToolRequestSuggestions should be normalized away: %+v", got.ToolRequestSuggestions)
	}
	if len(got.EvidenceCollectionSuggestions) != 3 {
		t.Fatalf("EvidenceCollectionSuggestions len = %d, want 3", len(got.EvidenceCollectionSuggestions))
	}
	if got.EvidenceCollectionSuggestions[0].Label != "Check active alerts" ||
		got.EvidenceCollectionSuggestions[0].Detail != "Verify whether sibling database or storage alerts are firing." ||
		got.EvidenceCollectionSuggestions[2].Label != "Tablespace usage trend" {
		t.Fatalf("EvidenceCollectionSuggestions = %+v", got.EvidenceCollectionSuggestions)
	}
	if len(got.EvidenceRequests) != 2 {
		t.Fatalf("EvidenceRequests len = %d, want 2: %+v", len(got.EvidenceRequests), got.EvidenceRequests)
	}
	if got.EvidenceRequests[0].Tool != domain.DiagnosisToolKindActiveAlerts ||
		got.EvidenceRequests[0].Reason != "Verify whether sibling database or storage alerts are firing." ||
		got.EvidenceRequests[0].Limit != 5 {
		t.Fatalf("EvidenceRequests[0] = %+v", got.EvidenceRequests[0])
	}
	if got.EvidenceRequests[1].Tool != domain.DiagnosisToolKindMetricQuery ||
		got.EvidenceRequests[1].Query != `oracle_tablespace_usage_percent{tablespace="OMPLATFORM"}` ||
		got.EvidenceRequests[1].Limit != 10 {
		t.Fatalf("EvidenceRequests[1] = %+v", got.EvidenceRequests[1])
	}
}

func TestParseTurnOutput_ConvertsToolRequestSuggestionWindowMinutes(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version": "diagnosis_turn.v1",
		"message": "The alert needs a bounded range query.",
		"confidence": "medium",
		"requires_human_review": true,
		"confidence_rationale": "The current value is high, but the trend is still missing.",
		"conclusion_status": "needs_evidence",
		"tool_request_suggestions": [{
			"label": "CPU saturation trend",
			"detail": "Collect a bounded CPU usage range query.",
			"priority": "medium",
			"tool": "metric_range_query",
			"query": "sum(rate(container_cpu_usage_seconds_total[5m]))",
			"window_minutes": 30,
			"step_seconds": 60,
			"limit": 10
		}]
	}`)

	got, err := ParseTurnOutput(raw)
	if err != nil {
		t.Fatalf("ParseTurnOutput: %v", err)
	}
	if len(got.EvidenceRequests) != 1 {
		t.Fatalf("EvidenceRequests len = %d, want 1: %+v", len(got.EvidenceRequests), got.EvidenceRequests)
	}
	req := got.EvidenceRequests[0]
	if req.Tool != domain.DiagnosisToolKindMetricRangeQuery ||
		req.WindowSeconds != 1800 ||
		req.StepSeconds != 60 ||
		req.Query != "sum(rate(container_cpu_usage_seconds_total[5m]))" {
		t.Fatalf("EvidenceRequests[0] = %+v", req)
	}
}

func TestParseTurnOutput_RejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "invalid json",
			raw:  `{`,
			want: "strict JSON",
		},
		{
			name: "duplicate output key",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"stale","message":"ok","confidence":"medium","requires_human_review":true}`,
			want: "duplicate object key",
		},
		{
			name: "trailing output value",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true} {"extra":true}`,
			want: "trailing JSON",
		},
		{
			name: "wrong schema version",
			raw:  `{"schema_version":"v0","message":"ok","confidence":"medium","requires_human_review":true}`,
			want: "schema violation",
		},
		{
			name: "missing message",
			raw:  `{"schema_version":"diagnosis_turn.v1","confidence":"medium","requires_human_review":true}`,
			want: "schema violation",
		},
		{
			name: "bad confidence",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"certain","requires_human_review":true}`,
			want: "schema violation",
		},
		{
			name: "extra property",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"debug":"nope"}`,
			want: "schema violation",
		},
		{
			name: "whitespace message",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"   ","confidence":"medium","requires_human_review":true}`,
			want: "message must be non-empty",
		},
		{
			name: "bad evidence tool",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"shell","reason":"need logs"}],"confidence":"medium","requires_human_review":true}`,
			want: "schema violation",
		},
		{
			name: "empty evidence reason after trim",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"active_alerts","reason":"   "}],"confidence":"medium","requires_human_review":true}`,
			want: "reason must be non-empty",
		},
		{
			name: "metric query without query or template",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"metric_query","reason":"need instant CPU"}],"confidence":"medium","requires_human_review":true}`,
			want: "metric_query requires query or template_id",
		},
		{
			name: "active alerts with query",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"active_alerts","reason":"need alerts","query":"ALERTS"}],"confidence":"medium","requires_human_review":true}`,
			want: "active_alerts must not include query",
		},
		{
			name: "range query step exceeds window",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"metric_range_query","reason":"need range","query":"up","window_seconds":60,"step_seconds":120}],"confidence":"medium","requires_human_review":true}`,
			want: "step_seconds must not exceed window_seconds",
		},
		{
			name: "multiline evidence query",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","evidence_requests":[{"tool":"metric_query","reason":"need query","query":"up\nrate"}],"confidence":"medium","requires_human_review":true}`,
			want: "query must be single-line",
		},
		{
			name: "empty missing evidence label",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"missing_evidence_requests":[{"label":"   ","detail":"need logs","priority":"high"}]}`,
			want: "schema violation",
		},
		{
			name: "unsupported collection suggestion priority",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"evidence_collection_suggestions":[{"label":"logs","detail":"need logs","priority":"urgent"}]}`,
			want: "schema violation",
		},
		{
			name: "bad conclusion status",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"conclusion_status":"done"}`,
			want: "schema violation",
		},
		{
			name: "tool suggestion extra property",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"confidence_rationale":"Needs review.","tool_request_suggestions":[{"label":"alerts","detail":"need alerts","priority":"high","tool":"active_alerts","debug":true}]}`,
			want: "schema violation",
		},
		{
			name: "tool suggestion ambiguous window",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":true,"confidence_rationale":"Needs review.","tool_request_suggestions":[{"label":"trend","detail":"need trend","priority":"high","tool":"metric_range_query","query":"up","window_seconds":300,"window_minutes":5,"step_seconds":60}]}`,
			want: "must not include both window_seconds and window_minutes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTurnOutput(json.RawMessage(tc.raw))
			if err == nil {
				t.Fatal("ParseTurnOutput returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseTurnOutput_RejectsIncompleteConsultationInsight(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "medium confidence requires rationale",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"medium","requires_human_review":false}`,
			want: "confidence_rationale is required",
		},
		{
			name: "human review requires rationale",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"high","requires_human_review":true,"missing_evidence_requests":[{"label":"Owner review","detail":"Confirm the diagnosis before closing.","priority":"medium"}]}`,
			want: "confidence_rationale is required",
		},
		{
			name: "low confidence requires improvement path",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"low","requires_human_review":false,"confidence_rationale":"Alert evidence is incomplete."}`,
			want: "must include evidence_requests",
		},
		{
			name: "needs evidence requires improvement path",
			raw:  `{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"high","requires_human_review":false,"confidence_rationale":"The current evidence is strong but one source is unavailable.","conclusion_status":"needs_evidence"}`,
			want: "must include evidence_requests",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTurnOutput(json.RawMessage(tc.raw))
			if err == nil {
				t.Fatal("ParseTurnOutput returned nil error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseTurnOutput_AcceptsReadyForReviewWithoutAdditionalEvidenceRequest(t *testing.T) {
	raw := json.RawMessage(`{
		"schema_version": "diagnosis_turn.v1",
		"message": "The supplemental restart evidence is sufficient for owner review.",
		"confidence": "medium",
		"requires_human_review": true,
		"confidence_rationale": "The causal chain is supported, but the owner still needs to confirm the closeout.",
		"conclusion_status": "ready_for_review"
	}`)

	got, err := ParseTurnOutput(raw)
	if err != nil {
		t.Fatalf("ParseTurnOutput: %v", err)
	}
	if got.ConclusionStatus != "ready_for_review" || got.Confidence != "medium" || !got.RequiresHumanReview {
		t.Fatalf("output = %+v", got)
	}
}

func TestTurnOutputInsightReturnsDefensiveCopies(t *testing.T) {
	out := TurnOutput{
		ConfidenceRationale: "Needs one more metric sample.",
		MissingEvidenceRequests: []ConsultationEvidenceRequest{{
			Label:    "Metric trend",
			Detail:   "Collect a bounded range query.",
			Priority: "high",
		}},
		EvidenceCollectionSuggestions: []ConsultationEvidenceRequest{{
			Label:    "Active alerts",
			Detail:   "Collect sibling active alerts.",
			Priority: "medium",
		}},
		ConclusionStatus: "needs_evidence",
	}

	insight := out.Insight()
	insight.MissingEvidenceRequests[0].Label = "changed"
	insight.EvidenceCollectionSuggestions[0].Label = "changed"

	if out.MissingEvidenceRequests[0].Label != "Metric trend" ||
		out.EvidenceCollectionSuggestions[0].Label != "Active alerts" {
		t.Fatalf("Insight returned shared slices: out=%+v insight=%+v", out, insight)
	}
}

func TestTurnOutputSchemaReturnsCopy(t *testing.T) {
	first := TurnOutputSchema()
	first[0] = 'x'

	second := TurnOutputSchema()
	if len(second) == 0 || second[0] == 'x' {
		t.Fatalf("TurnOutputSchema returned shared backing storage: %s", second)
	}
}
