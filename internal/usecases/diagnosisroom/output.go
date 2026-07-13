package diagnosisroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	// TurnOutputSchemaID identifies the V1 per-turn sandbox output contract.
	TurnOutputSchemaID = "https://openclarion.dev/schemas/diagnosis-turn-output.v1.json"
	// TurnOutputSchemaVersion is embedded in every accepted sandbox response.
	TurnOutputSchemaVersion = "diagnosis_turn.v1"

	maxEvidenceRequests            = 5
	maxEvidenceRequestReasonBytes  = 500
	maxEvidenceRequestQueryBytes   = 500
	minEvidenceRequestRangeSeconds = 15
	maxEvidenceRequestRangeSeconds = 6 * 60 * 60
	maxEvidenceRequestAlertLimit   = 10
	maxEvidenceRequestMetricLimit  = 20

	maxConsultationEvidenceRequests = 10
	maxToolRequestSuggestions       = 10
)

// TurnOutput is the schema-validated response written by the sandboxed
// diagnosis assistant to /workspace/out/output.json.
type TurnOutput struct {
	SchemaVersion                 string                        `json:"schema_version"`
	Message                       string                        `json:"message"`
	Findings                      []string                      `json:"findings,omitempty"`
	RecommendedActions            []string                      `json:"recommended_actions,omitempty"`
	EvidenceRequests              []EvidenceRequest             `json:"evidence_requests,omitempty"`
	Confidence                    string                        `json:"confidence"`
	RequiresHumanReview           bool                          `json:"requires_human_review"`
	ConfidenceRationale           string                        `json:"confidence_rationale,omitempty"`
	MissingEvidenceRequests       []ConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []ConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
	ToolRequestSuggestions        []ToolRequestSuggestion       `json:"tool_request_suggestions,omitempty"`
	ConclusionStatus              string                        `json:"conclusion_status,omitempty"`
}

// EvidenceRequest is a bounded, assistant-suggested evidence collection plan.
// It is planning metadata only; parsing a request never calls an upstream
// provider or starts a workflow.
type EvidenceRequest struct {
	TemplateID           int64                    `json:"template_id,omitempty"`
	AlertSourceProfileID int64                    `json:"alert_source_profile_id,omitempty"`
	Tool                 domain.DiagnosisToolKind `json:"tool"`
	Reason               string                   `json:"reason"`
	Query                string                   `json:"query,omitempty"`
	WindowSeconds        int                      `json:"window_seconds,omitempty"`
	StepSeconds          int                      `json:"step_seconds,omitempty"`
	Limit                int                      `json:"limit,omitempty"`
}

// ConsultationEvidenceRequest captures a human-readable evidence gap or
// collection hint. It complements EvidenceRequest: the latter is a bounded
// executable tool plan, while this shape is safe to display directly to an
// operator during confidence-lift review.
type ConsultationEvidenceRequest struct {
	Label    string `json:"label"`
	Detail   string `json:"detail"`
	Priority string `json:"priority"`
}

// ToolRequestSuggestion is a runner compatibility shape that combines a
// displayable evidence collection suggestion with optional bounded tool
// metadata. Normalization projects it into the stable public fields.
type ToolRequestSuggestion struct {
	Label                string                   `json:"label"`
	Detail               string                   `json:"detail"`
	Priority             string                   `json:"priority"`
	TemplateID           int64                    `json:"template_id,omitempty"`
	AlertSourceProfileID int64                    `json:"alert_source_profile_id,omitempty"`
	Tool                 domain.DiagnosisToolKind `json:"tool"`
	Query                string                   `json:"query,omitempty"`
	WindowSeconds        int                      `json:"window_seconds,omitempty"`
	WindowMinutes        int                      `json:"window_minutes,omitempty"`
	StepSeconds          int                      `json:"step_seconds,omitempty"`
	Limit                int                      `json:"limit,omitempty"`
}

// ConsultationInsight is the structured diagnosis state that can be surfaced
// to reconnecting clients without reparsing assistant message text.
type ConsultationInsight struct {
	ConfidenceRationale           string                        `json:"confidence_rationale,omitempty"`
	MissingEvidenceRequests       []ConsultationEvidenceRequest `json:"missing_evidence_requests,omitempty"`
	EvidenceCollectionSuggestions []ConsultationEvidenceRequest `json:"evidence_collection_suggestions,omitempty"`
	ConclusionStatus              string                        `json:"conclusion_status,omitempty"`
}

const turnOutputSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://openclarion.dev/schemas/diagnosis-turn-output.v1.json",
  "type": "object",
  "additionalProperties": false,
  "$defs": {
    "consultation_evidence_request": {
      "type": "object",
      "additionalProperties": false,
      "required": ["label", "detail", "priority"],
      "properties": {
        "label": {
          "type": "string",
          "minLength": 1,
          "maxLength": 120,
          "pattern": "\\S"
        },
        "detail": {
          "type": "string",
          "minLength": 1,
          "maxLength": 1000,
          "pattern": "\\S"
        },
        "priority": {
          "type": "string",
          "enum": ["low", "medium", "high"]
        }
      }
    },
    "evidence_request": {
      "type": "object",
      "additionalProperties": false,
      "required": ["tool", "reason"],
      "properties": {
        "template_id": {
          "type": "integer",
          "minimum": 1
        },
        "alert_source_profile_id": {
          "type": "integer",
          "minimum": 1
        },
        "tool": {
          "type": "string",
          "enum": ["active_alerts", "metric_query", "metric_range_query"]
        },
        "reason": {
          "type": "string",
          "minLength": 1,
          "maxLength": 500
        },
        "query": {
          "type": "string",
          "maxLength": 500
        },
        "window_seconds": {
          "type": "integer",
          "minimum": 0,
          "maximum": 21600
        },
        "step_seconds": {
          "type": "integer",
          "minimum": 0,
          "maximum": 21600
        },
        "limit": {
          "type": "integer",
          "minimum": 1,
          "maximum": 20
        }
      }
    },
    "tool_request_suggestion": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "label": {
          "type": "string",
          "maxLength": 120
        },
        "detail": {
          "type": "string",
          "maxLength": 1000
        },
        "priority": {
          "type": "string",
          "maxLength": 20
        },
        "template_id": {
          "type": "integer",
          "minimum": 0
        },
        "alert_source_profile_id": {
          "type": "integer",
          "minimum": 0
        },
        "tool": {
          "type": "string",
          "maxLength": 80
        },
        "query": {
          "type": "string",
          "maxLength": 500
        },
        "window_seconds": {
          "type": "integer",
          "minimum": 0,
          "maximum": 21600
        },
        "window_minutes": {
          "type": "integer",
          "minimum": 0,
          "maximum": 360
        },
        "step_seconds": {
          "type": "integer",
          "minimum": 0,
          "maximum": 21600
        },
        "limit": {
          "type": "integer",
          "minimum": 1,
          "maximum": 20
        }
      }
    }
  },
  "required": [
    "schema_version",
    "message",
    "confidence",
    "requires_human_review"
  ],
  "properties": {
    "schema_version": {
      "type": "string",
      "const": "diagnosis_turn.v1"
    },
    "message": {
      "type": "string",
      "minLength": 1,
      "maxLength": 20000
    },
    "findings": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "type": "string",
        "minLength": 1,
        "maxLength": 1000
      }
    },
    "recommended_actions": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "type": "string",
        "minLength": 1,
        "maxLength": 1000
      }
    },
    "evidence_requests": {
      "type": "array",
      "maxItems": 5,
      "items": {
        "$ref": "#/$defs/evidence_request"
      }
    },
    "confidence": {
      "type": "string",
      "enum": ["low", "medium", "high"]
    },
    "requires_human_review": {
      "type": "boolean"
    },
    "confidence_rationale": {
      "type": "string",
      "minLength": 1,
      "maxLength": 2000,
      "pattern": "\\S"
    },
    "missing_evidence_requests": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "$ref": "#/$defs/consultation_evidence_request"
      }
    },
    "evidence_collection_suggestions": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "$ref": "#/$defs/consultation_evidence_request"
      }
    },
    "tool_request_suggestions": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "$ref": "#/$defs/tool_request_suggestion"
      }
    },
    "conclusion_status": {
      "type": "string",
      "enum": ["investigating", "needs_evidence", "ready_for_review", "final"]
    }
  }
}`

// TurnOutputSchema returns a defensive copy of the V1 sandbox output schema.
func TurnOutputSchema() json.RawMessage {
	return cloneRawMessage(json.RawMessage(turnOutputSchemaJSON))
}

// TurnOutputStructuredSchema returns a strict-mode-compatible projection of
// the V1 schema for LLM providers. Every object property is required; fields
// that remain optional in the persisted V1 contract accept null.
func TurnOutputStructuredSchema() (json.RawMessage, error) {
	var schema any
	if err := strictjson.Unmarshal(json.RawMessage(turnOutputSchemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: decode structured-output schema: %w", err)
	}
	if err := requireStructuredOutputProperties(schema); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: build structured-output schema: %w", err)
	}
	if err := projectStructuredOutputProviderSubset(schema); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: project provider schema: %w", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("diagnosis turn output: encode structured-output schema: %w", err)
	}
	return json.RawMessage(raw), nil
}

// NormalizeTurnOutputStructuredResponse fills only omitted V1-optional object
// properties with null. This compensates for OpenAI-compatible gateways that
// accept a strict schema but do not enforce required nullable properties; V1
// required properties remain absent so the structured validator rejects them.
func NormalizeTurnOutputStructuredResponse(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := strictjson.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: decode structured response: %w", err)
	}
	var schema any
	if err := strictjson.Unmarshal(json.RawMessage(turnOutputSchemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: decode response-normalization schema: %w", err)
	}
	root, ok := schema.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("diagnosis turn output: response-normalization schema must be an object")
	}
	if err := fillOptionalSchemaProperties(value, root, root); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: normalize structured response: %w", err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("diagnosis turn output: encode structured response: %w", err)
	}
	return json.RawMessage(normalized), nil
}

func fillOptionalSchemaProperties(instance any, schema, root map[string]any) error {
	if ref, ok := schema["$ref"].(string); ok {
		resolved, err := resolveLocalSchemaRef(ref, root)
		if err != nil {
			return err
		}
		return fillOptionalSchemaProperties(instance, resolved, root)
	}
	if propertiesRaw, exists := schema["properties"]; exists {
		properties, ok := propertiesRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("properties must be an object")
		}
		required, err := requiredSchemaPropertySet(schema["required"])
		if err != nil {
			return err
		}
		object, ok := instance.(map[string]any)
		if !ok {
			return nil
		}
		for name, propertyRaw := range properties {
			property, ok := propertyRaw.(map[string]any)
			if !ok {
				return fmt.Errorf("property %q schema must be an object", name)
			}
			child, exists := object[name]
			if !exists {
				if _, mandatory := required[name]; !mandatory {
					object[name] = nil
				}
				continue
			}
			if child != nil {
				if err := fillOptionalSchemaProperties(child, property, root); err != nil {
					return fmt.Errorf("property %q: %w", name, err)
				}
			}
		}
	}
	itemsRaw, exists := schema["items"]
	if !exists {
		return nil
	}
	items, ok := itemsRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("items schema must be an object")
	}
	array, ok := instance.([]any)
	if !ok {
		return nil
	}
	for i, child := range array {
		if child == nil {
			continue
		}
		if err := fillOptionalSchemaProperties(child, items, root); err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
	}
	return nil
}

func resolveLocalSchemaRef(ref string, root map[string]any) (map[string]any, error) {
	const prefix = "#/$defs/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported schema reference %q", ref)
	}
	name := strings.TrimPrefix(ref, prefix)
	name = strings.ReplaceAll(strings.ReplaceAll(name, "~1", "/"), "~0", "~")
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema definitions must be an object")
	}
	resolved, ok := definitions[name].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema reference %q was not found", ref)
	}
	return resolved, nil
}

func requireStructuredOutputProperties(node any) error {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "properties" {
				continue
			}
			if err := requireStructuredOutputProperties(child); err != nil {
				return err
			}
		}
		propertiesRaw, exists := value["properties"]
		if !exists {
			return nil
		}
		properties, ok := propertiesRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("properties must be an object")
		}
		required, err := requiredSchemaPropertySet(value["required"])
		if err != nil {
			return err
		}
		names := make([]string, 0, len(properties))
		for name, property := range properties {
			if err := requireStructuredOutputProperties(property); err != nil {
				return fmt.Errorf("property %q: %w", name, err)
			}
			if _, alreadyRequired := required[name]; !alreadyRequired {
				properties[name] = map[string]any{
					"anyOf": []any{property, map[string]any{"type": "null"}},
				}
			}
			names = append(names, name)
		}
		for name := range required {
			if _, exists := properties[name]; !exists {
				return fmt.Errorf("required property %q is not declared", name)
			}
		}
		sort.Strings(names)
		value["required"] = names
		value["additionalProperties"] = false
	case []any:
		for i, child := range value {
			if err := requireStructuredOutputProperties(child); err != nil {
				return fmt.Errorf("schema item %d: %w", i, err)
			}
		}
	}
	return nil
}

func projectStructuredOutputProviderSubset(node any) error {
	schema, ok := node.(map[string]any)
	if !ok {
		return fmt.Errorf("schema node must be an object")
	}
	if constant, exists := schema["const"]; exists {
		if _, alreadyConstrained := schema["enum"]; alreadyConstrained {
			return fmt.Errorf("const and enum cannot both be projected")
		}
		schema["enum"] = []any{constant}
		delete(schema, "const")
	}
	for _, keyword := range []string{
		"$schema",
		"$id",
		"default",
		"examples",
		"minLength",
		"maxLength",
		"pattern",
		"format",
		"minimum",
		"maximum",
		"exclusiveMinimum",
		"exclusiveMaximum",
		"multipleOf",
		"minItems",
		"maxItems",
		"uniqueItems",
		"minProperties",
		"maxProperties",
	} {
		delete(schema, keyword)
	}

	for keyword, child := range schema {
		switch keyword {
		case "$ref", "type", "description", "enum", "required", "additionalProperties":
			continue
		case "properties", "$defs":
			children, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("%s must be an object", keyword)
			}
			for name, rawChild := range children {
				childSchema, ok := rawChild.(map[string]any)
				if !ok {
					return fmt.Errorf("%s %q must be a schema object", keyword, name)
				}
				if err := projectStructuredOutputProviderSubset(childSchema); err != nil {
					return fmt.Errorf("%s %q: %w", keyword, name, err)
				}
			}
		case "items":
			childSchema, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("items must be a schema object")
			}
			if err := projectStructuredOutputProviderSubset(childSchema); err != nil {
				return fmt.Errorf("items: %w", err)
			}
		case "anyOf":
			options, ok := child.([]any)
			if !ok {
				return fmt.Errorf("anyOf must be an array")
			}
			for i, rawOption := range options {
				option, ok := rawOption.(map[string]any)
				if !ok {
					return fmt.Errorf("anyOf item %d must be a schema object", i)
				}
				if err := projectStructuredOutputProviderSubset(option); err != nil {
					return fmt.Errorf("anyOf item %d: %w", i, err)
				}
			}
		default:
			return fmt.Errorf("schema keyword %q is not in the provider subset", keyword)
		}
	}
	return nil
}

func requiredSchemaPropertySet(raw any) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if raw == nil {
		return out, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("required must be an array")
	}
	for i, item := range items {
		name, ok := item.(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("required item %d must be a non-empty string", i)
		}
		if _, exists := out[name]; exists {
			return nil, fmt.Errorf("required property %q is duplicated", name)
		}
		out[name] = struct{}{}
	}
	return out, nil
}

// ParseTurnOutput validates raw sandbox output.json bytes and returns the
// normalized typed response used by the workflow/persistence layers.
func ParseTurnOutput(raw json.RawMessage) (TurnOutput, error) {
	if len(raw) == 0 {
		return TurnOutput{}, fmt.Errorf("diagnosis turn output: raw output must be non-empty JSON")
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return TurnOutput{}, fmt.Errorf("diagnosis turn output: output must be strict JSON: %w", err)
	}
	schema, err := compileTurnOutputSchema()
	if err != nil {
		return TurnOutput{}, err
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return TurnOutput{}, fmt.Errorf("diagnosis turn output: invalid JSON: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return TurnOutput{}, fmt.Errorf("diagnosis turn output: schema violation: %w", err)
	}
	var out TurnOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return TurnOutput{}, fmt.Errorf("diagnosis turn output: unmarshal: %w", err)
	}
	if err := normalizeTurnOutput(&out); err != nil {
		return TurnOutput{}, err
	}
	return out, nil
}

func compileTurnOutputSchema() (*jsonschema.Schema, error) {
	schemaJSON := json.RawMessage(turnOutputSchemaJSON)
	if err := strictjson.RejectDuplicateObjectKeys(schemaJSON); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: schema must be strict JSON: %w", err)
	}
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("diagnosis turn output: parse schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(TurnOutputSchemaID, parsed); err != nil {
		return nil, fmt.Errorf("diagnosis turn output: add schema: %w", err)
	}
	compiled, err := compiler.Compile(TurnOutputSchemaID)
	if err != nil {
		return nil, fmt.Errorf("diagnosis turn output: compile schema: %w", err)
	}
	return compiled, nil
}

func normalizeTurnOutput(out *TurnOutput) error {
	if out == nil {
		return fmt.Errorf("diagnosis turn output: output must be non-nil")
	}
	out.Message = strings.TrimSpace(out.Message)
	if out.Message == "" {
		return fmt.Errorf("diagnosis turn output: message must be non-empty after trimming")
	}
	out.Findings = normalizeOutputStrings(out.Findings)
	for i, finding := range out.Findings {
		if finding == "" {
			return fmt.Errorf("diagnosis turn output: findings[%d] must be non-empty after trimming", i)
		}
	}
	out.RecommendedActions = normalizeOutputStrings(out.RecommendedActions)
	for i, action := range out.RecommendedActions {
		if action == "" {
			return fmt.Errorf("diagnosis turn output: recommended_actions[%d] must be non-empty after trimming", i)
		}
	}
	evidenceRequests, err := normalizeEvidenceRequests(out.EvidenceRequests)
	if err != nil {
		return err
	}
	toolSuggestions, executableToolRequests, err := normalizeToolRequestSuggestions(out.ToolRequestSuggestions)
	if err != nil {
		return err
	}
	out.ToolRequestSuggestions = nil
	for _, req := range executableToolRequests {
		if len(evidenceRequests) >= maxEvidenceRequests {
			break
		}
		evidenceRequests = append(evidenceRequests, req)
	}
	out.EvidenceRequests = evidenceRequests
	out.Confidence = strings.TrimSpace(out.Confidence)
	out.ConfidenceRationale = strings.TrimSpace(out.ConfidenceRationale)
	if out.ConfidenceRationale == "" && turnOutputNeedsConfidenceRationale(*out) {
		out.ConfidenceRationale = "The assistant did not provide a confidence rationale; treat this confidence as unverified until a human reviewer checks the evidence."
	}
	out.MissingEvidenceRequests, err = normalizeConsultationEvidenceRequests(
		"missing_evidence_requests",
		out.MissingEvidenceRequests,
	)
	if err != nil {
		return err
	}
	out.EvidenceCollectionSuggestions, err = normalizeConsultationEvidenceRequests(
		"evidence_collection_suggestions",
		out.EvidenceCollectionSuggestions,
	)
	if err != nil {
		return err
	}
	if len(toolSuggestions) > 0 {
		if len(out.EvidenceCollectionSuggestions)+len(toolSuggestions) > maxConsultationEvidenceRequests {
			return fmt.Errorf("diagnosis turn output: evidence_collection_suggestions plus tool_request_suggestions exceeds %d items", maxConsultationEvidenceRequests)
		}
		out.EvidenceCollectionSuggestions = append(out.EvidenceCollectionSuggestions, toolSuggestions...)
	}
	out.ConclusionStatus = strings.TrimSpace(out.ConclusionStatus)
	if err := validateConsultationInsightCompleteness(*out); err != nil {
		return err
	}
	return nil
}

func validateConsultationInsightCompleteness(out TurnOutput) error {
	needsImprovementPath := out.Confidence == "low" || out.ConclusionStatus == "needs_evidence"
	if needsImprovementPath &&
		len(out.EvidenceRequests) == 0 &&
		len(out.MissingEvidenceRequests) == 0 &&
		len(out.EvidenceCollectionSuggestions) == 0 {
		return fmt.Errorf("diagnosis turn output: low-confidence or evidence-seeking output must include evidence_requests, missing_evidence_requests, or evidence_collection_suggestions")
	}
	return nil
}

func turnOutputNeedsConfidenceRationale(out TurnOutput) bool {
	return out.Confidence != "high" || out.RequiresHumanReview
}

// Insight returns the optional structured consultation fields from the
// accepted output, using defensive copies for slice-backed values.
func (out TurnOutput) Insight() ConsultationInsight {
	return ConsultationInsight{
		ConfidenceRationale:           out.ConfidenceRationale,
		MissingEvidenceRequests:       CloneConsultationEvidenceRequests(out.MissingEvidenceRequests),
		EvidenceCollectionSuggestions: CloneConsultationEvidenceRequests(out.EvidenceCollectionSuggestions),
		ConclusionStatus:              out.ConclusionStatus,
	}
}

func normalizeOutputStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, value := range in {
		out[i] = strings.TrimSpace(value)
	}
	return out
}

func normalizeEvidenceRequests(in []EvidenceRequest) ([]EvidenceRequest, error) {
	if in == nil {
		return nil, nil
	}
	if len(in) > maxEvidenceRequests {
		return nil, fmt.Errorf("diagnosis turn output: evidence_requests exceeds %d items", maxEvidenceRequests)
	}
	out := make([]EvidenceRequest, len(in))
	for i, req := range in {
		normalized, err := normalizeEvidenceRequest(i, req)
		if err != nil {
			return nil, err
		}
		out[i] = normalized
	}
	return out, nil
}

func normalizeConsultationEvidenceRequests(
	field string,
	in []ConsultationEvidenceRequest,
) ([]ConsultationEvidenceRequest, error) {
	if in == nil {
		return nil, nil
	}
	if len(in) > maxConsultationEvidenceRequests {
		return nil, fmt.Errorf("diagnosis turn output: %s exceeds %d items", field, maxConsultationEvidenceRequests)
	}
	out := make([]ConsultationEvidenceRequest, len(in))
	for i, req := range in {
		normalized := ConsultationEvidenceRequest{
			Label:    strings.TrimSpace(req.Label),
			Detail:   strings.TrimSpace(req.Detail),
			Priority: strings.TrimSpace(req.Priority),
		}
		if normalized.Label == "" {
			return nil, fmt.Errorf("diagnosis turn output: %s[%d].label must be non-empty after trimming", field, i)
		}
		if normalized.Detail == "" {
			return nil, fmt.Errorf("diagnosis turn output: %s[%d].detail must be non-empty after trimming", field, i)
		}
		switch normalized.Priority {
		case "low", "medium", "high":
		default:
			return nil, fmt.Errorf("diagnosis turn output: %s[%d].priority is unsupported", field, i)
		}
		out[i] = normalized
	}
	return out, nil
}

func normalizeToolRequestSuggestions(
	in []ToolRequestSuggestion,
) ([]ConsultationEvidenceRequest, []EvidenceRequest, error) {
	if in == nil {
		return nil, nil, nil
	}
	if len(in) > maxToolRequestSuggestions {
		return nil, nil, fmt.Errorf("diagnosis turn output: tool_request_suggestions exceeds %d items", maxToolRequestSuggestions)
	}
	consultation := make([]ConsultationEvidenceRequest, 0, len(in))
	executable := make([]EvidenceRequest, 0, len(in))
	for i, suggestion := range in {
		if incompleteToolRequestSuggestion(suggestion) {
			continue
		}
		normalized, err := normalizeToolRequestSuggestion(i, suggestion)
		if err != nil {
			return nil, nil, err
		}
		if req, ok := executableEvidenceRequestFromToolSuggestion(normalized); ok {
			executable = append(executable, req)
			continue
		}
		consultation = append(consultation, ConsultationEvidenceRequest{
			Label:    normalized.Label,
			Detail:   normalized.Detail,
			Priority: normalized.Priority,
		})
	}
	return consultation, executable, nil
}

func incompleteToolRequestSuggestion(suggestion ToolRequestSuggestion) bool {
	return strings.TrimSpace(suggestion.Label) == "" ||
		strings.TrimSpace(suggestion.Detail) == "" ||
		strings.TrimSpace(suggestion.Priority) == ""
}

func normalizeToolRequestSuggestion(index int, suggestion ToolRequestSuggestion) (ToolRequestSuggestion, error) {
	suggestion.Label = strings.TrimSpace(suggestion.Label)
	suggestion.Detail = strings.TrimSpace(suggestion.Detail)
	suggestion.Priority = strings.TrimSpace(suggestion.Priority)
	suggestion.Query = strings.TrimSpace(suggestion.Query)
	suggestion.Tool = domain.DiagnosisToolKind(strings.TrimSpace(string(suggestion.Tool)))
	if suggestion.Label == "" {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].label must be non-empty after trimming", index)
	}
	if suggestion.Detail == "" {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].detail must be non-empty after trimming", index)
	}
	switch suggestion.Priority {
	case "low", "medium", "high":
	default:
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].priority is unsupported", index)
	}
	if suggestion.TemplateID < 0 {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].template_id must be positive when set", index)
	}
	if suggestion.AlertSourceProfileID < 0 {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].alert_source_profile_id must be positive when set", index)
	}
	if suggestion.Tool != "" && !suggestion.Tool.Valid() {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].tool is unsupported", index)
	}
	if suggestion.Tool == domain.DiagnosisToolKindActiveAlerts {
		suggestion.Query = ""
		suggestion.WindowSeconds = 0
		suggestion.WindowMinutes = 0
		suggestion.StepSeconds = 0
	}
	if suggestion.Query != "" {
		if len([]byte(suggestion.Query)) > maxEvidenceRequestQueryBytes {
			return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].query exceeds %d bytes", index, maxEvidenceRequestQueryBytes)
		}
		if containsControlRune(suggestion.Query) {
			return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d].query must be single-line", index)
		}
	}
	if suggestion.WindowSeconds > 0 && suggestion.WindowMinutes > 0 {
		return ToolRequestSuggestion{}, fmt.Errorf("diagnosis turn output: tool_request_suggestions[%d] must not include both window_seconds and window_minutes", index)
	}
	if suggestion.WindowMinutes > 0 {
		suggestion.WindowSeconds = suggestion.WindowMinutes * 60
		suggestion.WindowMinutes = 0
	}
	return suggestion, nil
}

func executableEvidenceRequestFromToolSuggestion(suggestion ToolRequestSuggestion) (EvidenceRequest, bool) {
	if suggestion.Tool == "" {
		return EvidenceRequest{}, false
	}
	req := EvidenceRequest{
		TemplateID:           suggestion.TemplateID,
		AlertSourceProfileID: suggestion.AlertSourceProfileID,
		Tool:                 suggestion.Tool,
		Reason:               suggestion.Detail,
		Query:                suggestion.Query,
		WindowSeconds:        suggestion.WindowSeconds,
		StepSeconds:          suggestion.StepSeconds,
		Limit:                suggestion.Limit,
	}
	normalized, err := normalizeEvidenceRequest(0, req)
	if err != nil {
		return EvidenceRequest{}, false
	}
	return normalized, true
}

func normalizeEvidenceRequest(index int, req EvidenceRequest) (EvidenceRequest, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.Query = strings.TrimSpace(req.Query)
	if req.TemplateID < 0 {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].template_id must be positive when set", index)
	}
	if req.AlertSourceProfileID < 0 {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].alert_source_profile_id must be positive when set", index)
	}
	if !req.Tool.Valid() {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].tool is unsupported", index)
	}
	if req.Reason == "" {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].reason must be non-empty after trimming", index)
	}
	if len([]byte(req.Reason)) > maxEvidenceRequestReasonBytes {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].reason exceeds %d bytes", index, maxEvidenceRequestReasonBytes)
	}
	if containsControlRune(req.Reason) {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].reason must be single-line", index)
	}
	if req.Query != "" {
		if len([]byte(req.Query)) > maxEvidenceRequestQueryBytes {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].query exceeds %d bytes", index, maxEvidenceRequestQueryBytes)
		}
		if containsControlRune(req.Query) {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].query must be single-line", index)
		}
	}

	switch req.Tool {
	case domain.DiagnosisToolKindActiveAlerts:
		if req.Query != "" || req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d] active_alerts must not include query, window_seconds, or step_seconds", index)
		}
		if err := validateEvidenceRequestLimit(index, req.Limit, maxEvidenceRequestAlertLimit); err != nil {
			return EvidenceRequest{}, err
		}
	case domain.DiagnosisToolKindMetricQuery:
		if req.TemplateID == 0 && req.Query == "" {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d] metric_query requires query or template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d] metric_query must not include window_seconds or step_seconds", index)
		}
		if err := validateEvidenceRequestLimit(index, req.Limit, maxEvidenceRequestMetricLimit); err != nil {
			return EvidenceRequest{}, err
		}
	case domain.DiagnosisToolKindMetricRangeQuery:
		if req.TemplateID == 0 && req.Query == "" {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d] metric_range_query requires query or template_id", index)
		}
		if req.TemplateID == 0 && (req.WindowSeconds == 0 || req.StepSeconds == 0) {
			return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d] metric_range_query requires window_seconds and step_seconds without template_id", index)
		}
		if req.WindowSeconds != 0 || req.StepSeconds != 0 {
			if err := validateEvidenceRequestRange(index, req.WindowSeconds, req.StepSeconds); err != nil {
				return EvidenceRequest{}, err
			}
		}
		if err := validateEvidenceRequestLimit(index, req.Limit, maxEvidenceRequestMetricLimit); err != nil {
			return EvidenceRequest{}, err
		}
	}
	return req, nil
}

func validateEvidenceRequestLimit(index int, limit int, maximum int) error {
	if limit == 0 {
		return nil
	}
	if limit < 1 || limit > maximum {
		return fmt.Errorf("diagnosis turn output: evidence_requests[%d].limit must be between 1 and %d", index, maximum)
	}
	return nil
}

// EvidenceRequestLimitMaximum returns the parser-enforced positive limit cap
// for one executable evidence request tool.
func EvidenceRequestLimitMaximum(tool domain.DiagnosisToolKind) (int, bool) {
	switch tool {
	case domain.DiagnosisToolKindActiveAlerts:
		return maxEvidenceRequestAlertLimit, true
	case domain.DiagnosisToolKindMetricQuery, domain.DiagnosisToolKindMetricRangeQuery:
		return maxEvidenceRequestMetricLimit, true
	default:
		return 0, false
	}
}

// EvidenceRequestRangeSecondsBounds returns the parser-enforced range window
// and step bounds for metric_range_query requests.
func EvidenceRequestRangeSecondsBounds() (minSeconds int, maxSeconds int) {
	return minEvidenceRequestRangeSeconds, maxEvidenceRequestRangeSeconds
}

// EvidenceRequestReasonBytesMaximum returns the parser-enforced reason byte cap.
func EvidenceRequestReasonBytesMaximum() int {
	return maxEvidenceRequestReasonBytes
}

// EvidenceRequestQueryBytesMaximum returns the parser-enforced query byte cap.
func EvidenceRequestQueryBytesMaximum() int {
	return maxEvidenceRequestQueryBytes
}

// EvidenceRequestTextHasControlRune reports whether text would violate the
// parser's single-line evidence request text contract.
func EvidenceRequestTextHasControlRune(value string) bool {
	return containsControlRune(value)
}

func validateEvidenceRequestRange(index int, windowSeconds int, stepSeconds int) error {
	if windowSeconds < minEvidenceRequestRangeSeconds || windowSeconds > maxEvidenceRequestRangeSeconds {
		return fmt.Errorf("diagnosis turn output: evidence_requests[%d].window_seconds must be between %d and %d", index, minEvidenceRequestRangeSeconds, maxEvidenceRequestRangeSeconds)
	}
	if stepSeconds < minEvidenceRequestRangeSeconds || stepSeconds > maxEvidenceRequestRangeSeconds {
		return fmt.Errorf("diagnosis turn output: evidence_requests[%d].step_seconds must be between %d and %d", index, minEvidenceRequestRangeSeconds, maxEvidenceRequestRangeSeconds)
	}
	if stepSeconds > windowSeconds {
		return fmt.Errorf("diagnosis turn output: evidence_requests[%d].step_seconds must not exceed window_seconds", index)
	}
	return nil
}

// CloneConsultationEvidenceRequests returns a defensive copy of consultation
// evidence request items.
func CloneConsultationEvidenceRequests(in []ConsultationEvidenceRequest) []ConsultationEvidenceRequest {
	if in == nil {
		return nil
	}
	out := make([]ConsultationEvidenceRequest, len(in))
	copy(out, in)
	return out
}

func containsControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
