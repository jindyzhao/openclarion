package reportdraft

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func llmResponse(content string) ports.LLMResponse {
	return ports.LLMResponse{
		Content:      json.RawMessage(content),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-llm",
	}
}

func validSubReportJSON() string {
	return `{
		"title": "CPU saturation on payments",
		"summary": "The payments service is above CPU threshold.",
		"severity": "warning",
		"confidence": "high",
		"findings": [
			{"label": "CPU", "detail": "CPU remained above threshold for 10 minutes.", "evidence_id": "alert:1"}
		],
		"recommended_actions": [
			{"label": "Scale service", "detail": "Add one payments replica and monitor queue latency.", "priority": "medium"}
		],
		"evidence_refs": ["alert:1", "snapshot:11"]
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

func TestParseSubReport_AcceptsValidDraft(t *testing.T) {
	got, err := ParseSubReport(llmResponse(validSubReportJSON()))
	if err != nil {
		t.Fatalf("ParseSubReport: %v", err)
	}
	if got.Title != "CPU saturation on payments" {
		t.Fatalf("Title = %q", got.Title)
	}
	if got.Severity != SeverityWarning {
		t.Fatalf("Severity = %q", got.Severity)
	}
	if len(got.Findings) != 1 || got.Findings[0].EvidenceID != "alert:1" {
		t.Fatalf("Findings = %+v", got.Findings)
	}
}

func TestParseFinalReport_AcceptsValidDraft(t *testing.T) {
	got, err := ParseFinalReport(llmResponse(validFinalReportJSON()))
	if err != nil {
		t.Fatalf("ParseFinalReport: %v", err)
	}
	if got.Title != "Payments degradation" {
		t.Fatalf("Title = %q", got.Title)
	}
	if len(got.SubReports) != 1 || got.SubReports[0].Severity != SeverityWarning {
		t.Fatalf("SubReports = %+v", got.SubReports)
	}
}

func TestParseSubReport_RejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "additional property",
			content: `{
				"title":"CPU",
				"summary":"CPU high",
				"severity":"warning",
				"confidence":"high",
				"findings":[{"label":"CPU","detail":"high","evidence_id":"alert:1"}],
				"recommended_actions":[],
				"evidence_refs":[],
				"extra":"not allowed"
			}`,
		},
		{
			name: "invalid enum",
			content: `{
				"title":"CPU",
				"summary":"CPU high",
				"severity":"page",
				"confidence":"high",
				"findings":[{"label":"CPU","detail":"high","evidence_id":"alert:1"}],
				"recommended_actions":[],
				"evidence_refs":[]
			}`,
		},
		{
			name: "blank title",
			content: `{
				"title":"   ",
				"summary":"CPU high",
				"severity":"warning",
				"confidence":"high",
				"findings":[{"label":"CPU","detail":"high","evidence_id":"alert:1"}],
				"recommended_actions":[],
				"evidence_refs":[]
			}`,
		},
		{
			name: "missing required",
			content: `{
				"title":"CPU",
				"summary":"CPU high",
				"severity":"warning",
				"confidence":"high",
				"recommended_actions":[],
				"evidence_refs":[]
			}`,
		},
		{
			name:    "semantic length limit",
			content: strings.Replace(validSubReportJSON(), "CPU saturation on payments", strings.Repeat("x", maxTitleRunes+1), 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSubReport(llmResponse(tc.content))
			assertReason(t, err, llmoutput.ReasonSchemaViolation)
		})
	}
}

func TestParseFinalReport_RejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "additional property",
			content: `{
				"title":"Payments degradation",
				"executive_summary":"Payments is degraded.",
				"severity":"warning",
				"confidence":"high",
				"sub_reports":[{"title":"CPU","severity":"warning","summary":"CPU high"}],
				"recommended_actions":[],
				"notification_text":"Scale payments.",
				"extra":"not allowed"
			}`,
		},
		{
			name: "invalid enum",
			content: `{
				"title":"Payments degradation",
				"executive_summary":"Payments is degraded.",
				"severity":"page",
				"confidence":"high",
				"sub_reports":[{"title":"CPU","severity":"warning","summary":"CPU high"}],
				"recommended_actions":[],
				"notification_text":"Scale payments."
			}`,
		},
		{
			name: "missing required",
			content: `{
				"title":"Payments degradation",
				"executive_summary":"Payments is degraded.",
				"severity":"warning",
				"confidence":"high",
				"recommended_actions":[],
				"notification_text":"Scale payments."
			}`,
		},
		{
			name:    "semantic length limit",
			content: strings.Replace(validFinalReportJSON(), "Payments degradation", strings.Repeat("x", maxTitleRunes+1), 1),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFinalReport(llmResponse(tc.content))
			assertReason(t, err, llmoutput.ReasonSchemaViolation)
		})
	}
}

func TestSchemasAreStrictStructuredOutputCompatible(t *testing.T) {
	for _, item := range []struct {
		name   string
		schema json.RawMessage
	}{
		{"subreport", SubReportSchema()},
		{"finalreport", FinalReportSchema()},
	} {
		t.Run(item.name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(item.schema, &root); err != nil {
				t.Fatalf("schema unmarshal: %v", err)
			}
			assertOpenAIStrictSchema(t, "#", root, true)
		})
	}
}

func TestSchemaAccessorsReturnCopies(t *testing.T) {
	first := SubReportSchema()
	first[0] = 'X'
	second := SubReportSchema()
	if !json.Valid(second) {
		t.Fatalf("SubReportSchema returned shared bytes: %s", second)
	}

	first = FinalReportSchema()
	first[0] = 'X'
	second = FinalReportSchema()
	if !json.Valid(second) {
		t.Fatalf("FinalReportSchema returned shared bytes: %s", second)
	}
}

func assertReason(t *testing.T, err error, reason llmoutput.Reason) {
	t.Helper()
	var validationErr *llmoutput.Error
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %T %v, want *llmoutput.Error", err, err)
	}
	if validationErr.Reason != reason {
		t.Fatalf("Reason = %q, want %q", validationErr.Reason, reason)
	}
}

var openAIStrictSchemaKeywords = map[string]struct{}{
	"$schema":              {},
	"additionalProperties": {},
	"enum":                 {},
	"items":                {},
	"maxItems":             {},
	"minItems":             {},
	"pattern":              {},
	"properties":           {},
	"required":             {},
	"type":                 {},
}

func assertOpenAIStrictSchema(t *testing.T, path string, node map[string]any, root bool) {
	t.Helper()
	for key := range node {
		if _, ok := openAIStrictSchemaKeywords[key]; !ok {
			t.Fatalf("%s uses unsupported strict structured output keyword %q", path, key)
		}
	}

	typ, ok := node["type"].(string)
	if !ok {
		t.Fatalf("%s missing string type", path)
	}
	if root && typ != "object" {
		t.Fatalf("%s root type = %q, want object", path, typ)
	}
	if typ == "object" {
		if got, ok := node["additionalProperties"].(bool); !ok || got {
			t.Fatalf("%s additionalProperties = %v, want false", path, node["additionalProperties"])
		}
		props, _ := node["properties"].(map[string]any)
		requiredValues, _ := node["required"].([]any)
		if len(props) == 0 {
			t.Fatalf("%s object schema has no properties", path)
		}
		required := make([]string, 0, len(requiredValues))
		for _, value := range requiredValues {
			requiredValue, ok := value.(string)
			if !ok {
				t.Fatalf("%s required contains %T", path, value)
			}
			required = append(required, requiredValue)
		}
		sort.Strings(required)
		keys := make([]string, 0, len(props))
		for key := range props {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(required) != len(keys) {
			t.Fatalf("%s required = %v, want property keys %v", path, required, keys)
		}
		for i := range keys {
			if required[i] != keys[i] {
				t.Fatalf("%s required = %v, want property keys %v", path, required, keys)
			}
		}
		for key, value := range props {
			child, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("%s/properties/%s schema is %T", path, key, value)
			}
			assertOpenAIStrictSchema(t, path+"/properties/"+key, child, false)
		}
	}
	if typ == "array" {
		item, ok := node["items"].(map[string]any)
		if !ok {
			t.Fatalf("%s/items missing object schema", path)
		}
		assertOpenAIStrictSchema(t, path+"/items", item, false)
	}
	if enumValues, ok := node["enum"].([]any); ok {
		if len(enumValues) == 0 {
			t.Fatalf("%s enum is empty", path)
		}
		for _, value := range enumValues {
			if _, ok := value.(string); !ok {
				t.Fatalf("%s enum contains %T", path, value)
			}
		}
	}
}
