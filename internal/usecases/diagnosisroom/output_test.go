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
		"requires_human_review": true
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

func TestTurnOutputSchemaReturnsCopy(t *testing.T) {
	first := TurnOutputSchema()
	first[0] = 'x'

	second := TurnOutputSchema()
	if len(second) == 0 || second[0] == 'x' {
		t.Fatalf("TurnOutputSchema returned shared backing storage: %s", second)
	}
}
