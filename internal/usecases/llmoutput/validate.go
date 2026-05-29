// Package llmoutput validates provider-neutral LLM responses before
// any report persistence step can consume them.
package llmoutput

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const acceptedFinishReason = "stop"

// Reason identifies why an LLM response was rejected.
type Reason string

const (
	// ReasonInvalidRequest means the caller supplied an invalid schema
	// or missing request metadata.
	ReasonInvalidRequest Reason = "invalid_request"
	// ReasonInvalidProviderMetadata means the provider returned an
	// unsupported output mode.
	ReasonInvalidProviderMetadata Reason = "invalid_provider_metadata"
	// ReasonRefusal means the provider surfaced a model refusal.
	ReasonRefusal Reason = "refusal"
	// ReasonIncomplete means finish_reason indicates truncation or any
	// other non-stop completion.
	ReasonIncomplete Reason = "incomplete"
	// ReasonInvalidJSON means the provider content is not valid JSON.
	ReasonInvalidJSON Reason = "invalid_json"
	// ReasonSchemaViolation means content failed JSON Schema validation.
	ReasonSchemaViolation Reason = "schema_violation"
)

// Error is returned when Validate rejects an LLM response. Retryable
// is true only for model-output failures that the M2 retry loop may
// reasonably send back to the provider as validation feedback.
type Error struct {
	Reason    Reason
	Retryable bool
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// IsRetryable reports whether err represents model output that can be
// retried with validation feedback.
func IsRetryable(err error) bool {
	var validationErr *Error
	if !errors.As(err, &validationErr) {
		return false
	}
	return validationErr.Retryable
}

// Accepted is a schema-validated LLM output. Content preserves the
// provider's original JSON bytes; Parsed carries the unmarshaled value
// already validated against the request schema.
type Accepted struct {
	Content    json.RawMessage
	Parsed     any
	OutputMode ports.LLMOutputMode
	Model      string
}

// Validate checks an LLM response against request metadata and JSON
// Schema. It intentionally performs provider-independent acceptance
// checks only: prompt construction, provider retry, and report
// persistence stay in higher-level M2 usecases.
func Validate(req ports.LLMRequest, resp ports.LLMResponse) (Accepted, error) {
	if req.OutputSchemaID == "" {
		return Accepted{}, reject(ReasonInvalidRequest, false, fmt.Errorf("output_schema_id must be non-empty"))
	}
	if len(req.OutputSchema) == 0 {
		return Accepted{}, reject(ReasonInvalidRequest, false, fmt.Errorf("output_schema must be non-empty"))
	}
	if resp.OutputMode != ports.LLMOutputModeJSONSchema && resp.OutputMode != ports.LLMOutputModeJSONObject {
		return Accepted{}, reject(ReasonInvalidProviderMetadata, false, fmt.Errorf("output mode %q is unsupported", resp.OutputMode))
	}
	if resp.Refusal != nil {
		return Accepted{}, reject(ReasonRefusal, false, fmt.Errorf("model refused: %s", *resp.Refusal))
	}
	if resp.FinishReason != acceptedFinishReason {
		return Accepted{}, reject(ReasonIncomplete, true, fmt.Errorf("finish_reason %q is not %q", resp.FinishReason, acceptedFinishReason))
	}
	if len(resp.Content) == 0 {
		return Accepted{}, reject(ReasonInvalidJSON, true, fmt.Errorf("content must be non-empty JSON"))
	}

	schema, err := compileSchema(req.OutputSchemaID, req.OutputSchema)
	if err != nil {
		return Accepted{}, reject(ReasonInvalidRequest, false, err)
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(resp.Content))
	if err != nil {
		return Accepted{}, reject(ReasonInvalidJSON, true, err)
	}
	if err := schema.Validate(instance); err != nil {
		return Accepted{}, reject(ReasonSchemaViolation, true, err)
	}
	return Accepted{
		Content:    cloneRawMessage(resp.Content),
		Parsed:     instance,
		OutputMode: resp.OutputMode,
		Model:      resp.Model,
	}, nil
}

func compileSchema(id string, raw json.RawMessage) (*jsonschema.Schema, error) {
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse output schema %q: %w", id, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(id, parsed); err != nil {
		return nil, fmt.Errorf("add output schema %q: %w", id, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("compile output schema %q: %w", id, err)
	}
	return compiled, nil
}

func reject(reason Reason, retryable bool, err error) error {
	return &Error{Reason: reason, Retryable: retryable, Err: err}
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
