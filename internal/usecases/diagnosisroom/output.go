package diagnosisroom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	// TurnOutputSchemaID identifies the V1 per-turn sandbox output contract.
	TurnOutputSchemaID = "https://openclarion.dev/schemas/diagnosis-turn-output.v1.json"
	// TurnOutputSchemaVersion is embedded in every accepted sandbox response.
	TurnOutputSchemaVersion = "diagnosis_turn.v1"
)

// TurnOutput is the schema-validated response written by the sandboxed
// diagnosis assistant to /workspace/out/output.json.
type TurnOutput struct {
	SchemaVersion       string   `json:"schema_version"`
	Message             string   `json:"message"`
	Findings            []string `json:"findings,omitempty"`
	RecommendedActions  []string `json:"recommended_actions,omitempty"`
	Confidence          string   `json:"confidence"`
	RequiresHumanReview bool     `json:"requires_human_review"`
}

const turnOutputSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://openclarion.dev/schemas/diagnosis-turn-output.v1.json",
  "type": "object",
  "additionalProperties": false,
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
    "confidence": {
      "type": "string",
      "enum": ["low", "medium", "high"]
    },
    "requires_human_review": {
      "type": "boolean"
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
	out.Confidence = strings.TrimSpace(out.Confidence)
	return nil
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

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
