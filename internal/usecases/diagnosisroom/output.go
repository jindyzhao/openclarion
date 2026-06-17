package diagnosisroom

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	ConclusionStatus              string                        `json:"conclusion_status,omitempty"`
}

// EvidenceRequest is a bounded, assistant-suggested evidence collection plan.
// It is planning metadata only; parsing a request never calls an upstream
// provider or starts a workflow.
type EvidenceRequest struct {
	TemplateID    int64                    `json:"template_id,omitempty"`
	Tool          domain.DiagnosisToolKind `json:"tool"`
	Reason        string                   `json:"reason"`
	Query         string                   `json:"query,omitempty"`
	WindowSeconds int                      `json:"window_seconds,omitempty"`
	StepSeconds   int                      `json:"step_seconds,omitempty"`
	Limit         int                      `json:"limit,omitempty"`
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
        "type": "object",
        "additionalProperties": false,
        "required": ["tool", "reason"],
        "properties": {
          "template_id": {
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
	out.EvidenceRequests = evidenceRequests
	out.Confidence = strings.TrimSpace(out.Confidence)
	out.ConfidenceRationale = strings.TrimSpace(out.ConfidenceRationale)
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
	out.ConclusionStatus = strings.TrimSpace(out.ConclusionStatus)
	return nil
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

func normalizeEvidenceRequest(index int, req EvidenceRequest) (EvidenceRequest, error) {
	req.Reason = strings.TrimSpace(req.Reason)
	req.Query = strings.TrimSpace(req.Query)
	if req.TemplateID < 0 {
		return EvidenceRequest{}, fmt.Errorf("diagnosis turn output: evidence_requests[%d].template_id must be positive when set", index)
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
