package reportprompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
)

func TestBuildSubReportRequest_StructuralGoldenScenarios(t *testing.T) {
	scenarios := []Scenario{
		ScenarioSingleAlert,
		ScenarioCascade,
		ScenarioAlertStorm,
	}
	for i, scenario := range scenarios {
		t.Run(string(scenario), func(t *testing.T) {
			snapshot := validSnapshot()
			req, err := BuildSubReportRequest(SubReportInput{
				Snapshot:   snapshot,
				Scenario:   scenario,
				GroupIndex: i,
			})
			if err != nil {
				t.Fatalf("BuildSubReportRequest: %v", err)
			}

			assertSubReportRequestShape(t, req, snapshot.ID, i, scenario)
			if _, err := llmoutput.Validate(req, response(validSubReportJSON())); err != nil {
				t.Fatalf("Validate with request schema: %v", err)
			}
			if _, err := reportdraft.ParseSubReport(response(validSubReportJSON())); err != nil {
				t.Fatalf("ParseSubReport fixture: %v", err)
			}
		})
	}
}

func TestBuildFinalReportRequest_StructuralGolden(t *testing.T) {
	req, err := BuildFinalReportRequest(FinalReportInput{
		CorrelationKey: "window-2026-05-28T00:00Z",
		SubReports:     []reportdraft.SubReport{validSubReport()},
	})
	if err != nil {
		t.Fatalf("BuildFinalReportRequest: %v", err)
	}

	if req.OutputSchemaID != reportdraft.FinalReportSchemaID {
		t.Fatalf("OutputSchemaID = %q", req.OutputSchemaID)
	}
	if req.IdempotencyKey != "final_report:window-2026-05-28T00:00Z" {
		t.Fatalf("IdempotencyKey = %q", req.IdempotencyKey)
	}
	assertMessagesRequireJSON(t, req.Messages)
	assertActionShapePrompt(t, req.Messages[0].Content)
	if !strings.Contains(req.Messages[1].Content, `"recommended_actions"`) {
		t.Fatalf("final user prompt does not include serialized subreports: %s", req.Messages[1].Content)
	}
	if _, err := llmoutput.Validate(req, response(validFinalReportJSON())); err != nil {
		t.Fatalf("Validate with request schema: %v", err)
	}
	if _, err := reportdraft.ParseFinalReport(response(validFinalReportJSON())); err != nil {
		t.Fatalf("ParseFinalReport fixture: %v", err)
	}
}

func TestBuildSubReportRequest_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input SubReportInput
	}{
		{
			name: "zero snapshot id",
			input: SubReportInput{
				Snapshot: domain.EvidenceSnapshot{Digest: "digest", Payload: json.RawMessage(`{"ok":true}`)},
				Scenario: ScenarioSingleAlert,
			},
		},
		{
			name: "missing digest",
			input: SubReportInput{
				Snapshot: domain.EvidenceSnapshot{ID: 11, Payload: json.RawMessage(`{"ok":true}`)},
				Scenario: ScenarioSingleAlert,
			},
		},
		{
			name: "negative group index",
			input: SubReportInput{
				Snapshot:   validSnapshot(),
				Scenario:   ScenarioSingleAlert,
				GroupIndex: -1,
			},
		},
		{
			name: "unsupported scenario",
			input: SubReportInput{
				Snapshot: validSnapshot(),
				Scenario: Scenario("unknown"),
			},
		},
		{
			name: "empty payload",
			input: SubReportInput{
				Snapshot: domain.EvidenceSnapshot{ID: 11, Digest: "digest"},
				Scenario: ScenarioSingleAlert,
			},
		},
		{
			name: "invalid payload json",
			input: SubReportInput{
				Snapshot: domain.EvidenceSnapshot{ID: 11, Digest: "digest", Payload: json.RawMessage(`{`)},
				Scenario: ScenarioSingleAlert,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildSubReportRequest(tc.input)
			assertInvariant(t, err)
		})
	}
}

func TestBuildFinalReportRequest_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input FinalReportInput
	}{
		{
			name:  "missing correlation key",
			input: FinalReportInput{SubReports: []reportdraft.SubReport{validSubReport()}},
		},
		{
			name:  "empty subreports",
			input: FinalReportInput{CorrelationKey: "window"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildFinalReportRequest(tc.input)
			assertInvariant(t, err)
		})
	}
}

func TestBuildSubReportRequest_ReturnsSchemaCopies(t *testing.T) {
	req, err := BuildSubReportRequest(SubReportInput{
		Snapshot: validSnapshot(),
		Scenario: ScenarioSingleAlert,
	})
	if err != nil {
		t.Fatalf("BuildSubReportRequest: %v", err)
	}
	req.OutputSchema[0] = 'X'

	req2, err := BuildSubReportRequest(SubReportInput{
		Snapshot: validSnapshot(),
		Scenario: ScenarioSingleAlert,
	})
	if err != nil {
		t.Fatalf("BuildSubReportRequest second: %v", err)
	}
	if !json.Valid(req2.OutputSchema) {
		t.Fatalf("schema was shared between requests: %s", req2.OutputSchema)
	}
}

func assertSubReportRequestShape(t *testing.T, req ports.LLMRequest, snapshotID domain.EvidenceSnapshotID, groupIndex int, scenario Scenario) {
	t.Helper()
	if req.OutputSchemaID != reportdraft.SubReportSchemaID {
		t.Fatalf("OutputSchemaID = %q", req.OutputSchemaID)
	}
	wantKey := fmt.Sprintf("snapshot:%d/group:%d/scenario:%s/sub_report", snapshotID, groupIndex, scenario)
	if req.IdempotencyKey != wantKey {
		t.Fatalf("IdempotencyKey = %q, want %q", req.IdempotencyKey, wantKey)
	}
	assertMessagesRequireJSON(t, req.Messages)
	assertActionShapePrompt(t, req.Messages[0].Content)
	if !strings.Contains(req.Messages[1].Content, string(scenario)) {
		t.Fatalf("user prompt missing scenario %q: %s", scenario, req.Messages[1].Content)
	}
	if !strings.Contains(req.Messages[1].Content, fmt.Sprintf("Evidence snapshot ref: snapshot:%d", snapshotID)) {
		t.Fatalf("user prompt missing snapshot ref: %s", req.Messages[1].Content)
	}
	if !strings.Contains(req.Messages[1].Content, "Include the evidence snapshot ref in evidence_refs") {
		t.Fatalf("user prompt does not require snapshot ref evidence_refs: %s", req.Messages[1].Content)
	}
	if !strings.Contains(req.Messages[1].Content, `"schema_version":"evidence.snapshot.v1"`) {
		t.Fatalf("user prompt does not include compact snapshot payload: %s", req.Messages[1].Content)
	}
}

func assertMessagesRequireJSON(t *testing.T, messages []ports.LLMMessage) {
	t.Helper()
	if len(messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(messages))
	}
	if messages[0].Role != ports.LLMRoleSystem || messages[1].Role != ports.LLMRoleUser {
		t.Fatalf("messages roles = %+v", messages)
	}
	for i, msg := range messages {
		if !strings.Contains(msg.Content, "JSON") {
			t.Fatalf("message[%d] does not explicitly require JSON: %s", i, msg.Content)
		}
	}
}

func assertActionShapePrompt(t *testing.T, content string) {
	t.Helper()
	for _, want := range []string{
		"recommended_actions",
		"label",
		"detail",
		"priority",
		"do not use an action field",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("system prompt missing %q: %s", want, content)
		}
	}
}

func assertInvariant(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("err = %v, want ErrInvariantViolation", err)
	}
}

func validSnapshot() domain.EvidenceSnapshot {
	return domain.EvidenceSnapshot{
		ID:           11,
		AlertGroupID: 7,
		Digest:       "digest-abc",
		Payload: json.RawMessage(`{
			"schema_version":"evidence.snapshot.v1",
			"group":{"id":7,"group_key":"payments","severity":"warning"},
			"events":[{"id":101,"source":"prometheus","labels":{"alertname":"HighCPU"}}]
		}`),
		Status: domain.SnapshotStatusComplete,
	}
}

func validSubReport() reportdraft.SubReport {
	return reportdraft.SubReport{
		Title:      "CPU saturation on payments",
		Summary:    "The payments service is above CPU threshold.",
		Severity:   reportdraft.SeverityWarning,
		Confidence: reportdraft.ConfidenceHigh,
		Findings: []reportdraft.Finding{{
			Label:      "CPU",
			Detail:     "CPU remained above threshold for 10 minutes.",
			EvidenceID: "alert:101",
		}},
		RecommendedActions: []reportdraft.Action{{
			Label:    "Scale service",
			Detail:   "Add one payments replica and monitor queue latency.",
			Priority: reportdraft.PriorityMedium,
		}},
		EvidenceRefs: []string{"alert:101", "snapshot:11"},
	}
}

func validSubReportJSON() string {
	return `{
		"title": "CPU saturation on payments",
		"summary": "The payments service is above CPU threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:101"}
		],
		"recommended_actions": [
			{"label": "Scale service", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["alert:101", "snapshot:11"]
	}`
}

func validFinalReportJSON() string {
	return `{
		"title": "Payments degradation",
		"executive_summary": "Payments is degraded due to CPU saturation.",
		"severity": "warning",
		"confidence": "high",
		"sub_reports": [
			{"title": "CPU saturation on payments", "severity": "warning", "summary": "CPU is above threshold."}
		],
		"recommended_actions": [
			{"label": "Scale service", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"notification_text": "Payments degradation detected; scale the service and monitor."
	}`
}

func response(content string) ports.LLMResponse {
	return ports.LLMResponse{
		Content:      json.RawMessage(content),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-llm",
	}
}
