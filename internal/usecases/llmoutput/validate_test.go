package llmoutput

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func validRequest() ports.LLMRequest {
	return ports.LLMRequest{
		OutputSchemaID: "sub_report.schema.json",
		OutputSchema: json.RawMessage(`{
			"$schema":"https://json-schema.org/draft/2020-12/schema",
			"type":"object",
			"required":["title","severity","findings"],
			"additionalProperties":false,
			"properties":{
				"title":{"type":"string","minLength":1},
				"severity":{"type":"string","enum":["info","warning","critical"]},
				"findings":{
					"type":"array",
					"minItems":1,
					"items":{"type":"string","minLength":1}
				}
			}
		}`),
		IdempotencyKey: "snapshot-11/group-0",
	}
}

func validResponse() ports.LLMResponse {
	return ports.LLMResponse{
		Content:      json.RawMessage(`{"title":"CPU saturation","severity":"warning","findings":["cpu above threshold"]}`),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-llm",
	}
}

func TestValidate_AcceptsStrictSchemaOutput(t *testing.T) {
	resp := validResponse()
	accepted, err := Validate(validRequest(), resp)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if string(accepted.Content) != string(resp.Content) {
		t.Fatalf("accepted Content = %s, want %s", accepted.Content, resp.Content)
	}
	if accepted.OutputMode != ports.LLMOutputModeJSONSchema {
		t.Fatalf("OutputMode = %q", accepted.OutputMode)
	}
	if accepted.Model != "fake-llm" {
		t.Fatalf("Model = %q", accepted.Model)
	}
	if _, ok := accepted.Parsed.(map[string]any); !ok {
		t.Fatalf("Parsed type = %T, want map[string]any", accepted.Parsed)
	}

	accepted.Content[2] = 'X'
	if string(resp.Content) != `{"title":"CPU saturation","severity":"warning","findings":["cpu above threshold"]}` {
		t.Fatalf("Validate returned Content sharing provider bytes: %s", resp.Content)
	}
}

func TestValidate_AcceptsJSONObjectFallbackWhenSchemaValidates(t *testing.T) {
	resp := validResponse()
	resp.OutputMode = ports.LLMOutputModeJSONObject
	if _, err := Validate(validRequest(), resp); err != nil {
		t.Fatalf("Validate json_object fallback: %v", err)
	}
}

func TestValidate_RejectsRefusal(t *testing.T) {
	refusal := "cannot comply"
	resp := validResponse()
	resp.Refusal = &refusal

	err := validateErr(t, validRequest(), resp)
	assertReason(t, err, ReasonRefusal, false)
	if !strings.Contains(err.Error(), "cannot comply") {
		t.Fatalf("err = %q, want refusal text", err.Error())
	}
}

func TestValidate_RejectsNonStopFinishReasonAsRetryable(t *testing.T) {
	resp := validResponse()
	resp.FinishReason = "length"

	err := validateErr(t, validRequest(), resp)
	assertReason(t, err, ReasonIncomplete, true)
	if !IsRetryable(err) {
		t.Fatal("IsRetryable = false, want true")
	}
}

func TestValidate_RejectsInvalidJSONAsRetryable(t *testing.T) {
	resp := validResponse()
	resp.Content = json.RawMessage(`{"title":`)

	err := validateErr(t, validRequest(), resp)
	assertReason(t, err, ReasonInvalidJSON, true)
}

func TestValidate_RejectsAmbiguousProviderJSONAsRetryable(t *testing.T) {
	tests := []struct {
		name    string
		content json.RawMessage
		want    string
	}{
		{
			name:    "duplicate content key",
			content: json.RawMessage(`{"title":"stale","title":"CPU saturation","severity":"warning","findings":["cpu above threshold"]}`),
			want:    "duplicate object key",
		},
		{
			name:    "trailing content value",
			content: json.RawMessage(`{"title":"CPU saturation","severity":"warning","findings":["cpu above threshold"]} {"extra":true}`),
			want:    "trailing JSON",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := validResponse()
			resp.Content = tt.content

			err := validateErr(t, validRequest(), resp)
			assertReason(t, err, ReasonInvalidJSON, true)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidate_RejectsSchemaViolationAsRetryable(t *testing.T) {
	resp := validResponse()
	resp.Content = json.RawMessage(`{"title":"","severity":"page","findings":[]}`)

	err := validateErr(t, validRequest(), resp)
	assertReason(t, err, ReasonSchemaViolation, true)
}

func TestValidate_RejectsInvalidRequestAsNonRetryable(t *testing.T) {
	req := validRequest()
	req.OutputSchema = json.RawMessage(`{"type":`)

	err := validateErr(t, req, validResponse())
	assertReason(t, err, ReasonInvalidRequest, false)
}

func TestValidate_RejectsAmbiguousOutputSchemaAsNonRetryable(t *testing.T) {
	tests := []struct {
		name   string
		schema json.RawMessage
		want   string
	}{
		{
			name:   "duplicate schema key",
			schema: json.RawMessage(`{"type":"object","type":"object"}`),
			want:   "duplicate object key",
		},
		{
			name:   "trailing schema value",
			schema: json.RawMessage(`{"type":"object"} {"type":"string"}`),
			want:   "trailing JSON",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			req.OutputSchema = tt.schema

			err := validateErr(t, req, validResponse())
			assertReason(t, err, ReasonInvalidRequest, false)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestValidate_RejectsUnsupportedOutputModeAsNonRetryable(t *testing.T) {
	resp := validResponse()
	resp.OutputMode = "xml"

	err := validateErr(t, validRequest(), resp)
	assertReason(t, err, ReasonInvalidProviderMetadata, false)
}

func TestValidate_RejectsMissingSchemaMetadata(t *testing.T) {
	t.Run("missing schema id", func(t *testing.T) {
		req := validRequest()
		req.OutputSchemaID = ""
		err := validateErr(t, req, validResponse())
		assertReason(t, err, ReasonInvalidRequest, false)
	})

	t.Run("missing schema", func(t *testing.T) {
		req := validRequest()
		req.OutputSchema = nil
		err := validateErr(t, req, validResponse())
		assertReason(t, err, ReasonInvalidRequest, false)
	})
}

func validateErr(t *testing.T, req ports.LLMRequest, resp ports.LLMResponse) error {
	t.Helper()
	_, err := Validate(req, resp)
	if err == nil {
		t.Fatal("Validate returned nil error")
	}
	return err
}

func assertReason(t *testing.T, err error, reason Reason, retryable bool) {
	t.Helper()
	var validationErr *Error
	if !errors.As(err, &validationErr) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if validationErr.Reason != reason {
		t.Fatalf("Reason = %q, want %q", validationErr.Reason, reason)
	}
	if validationErr.Retryable != retryable {
		t.Fatalf("Retryable = %v, want %v", validationErr.Retryable, retryable)
	}
	if IsRetryable(err) != retryable {
		t.Fatalf("IsRetryable = %v, want %v", IsRetryable(err), retryable)
	}
}
