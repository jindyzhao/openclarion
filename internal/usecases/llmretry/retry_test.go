package llmretry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	fakellm "github.com/openclarion/openclarion/internal/providers/llm/fake"
	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const testKey = "snapshot-11/group-0"

type recordingProvider struct {
	provider *fakellm.Provider
	requests []ports.LLMRequest
}

func (p *recordingProvider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	p.requests = append(p.requests, cloneRequest(req))
	return p.provider.GenerateJSON(ctx, req)
}

func validRequest() ports.LLMRequest {
	return ports.LLMRequest{
		Messages: []ports.LLMMessage{{
			Role:    ports.LLMRoleSystem,
			Content: "return a sub-report as JSON",
		}},
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
		IdempotencyKey: testKey,
	}
}

func response(content string) ports.LLMResponse {
	return ports.LLMResponse{
		Content:      json.RawMessage(content),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-llm",
	}
}

func TestGenerateValidated_SucceedsFirstAttempt(t *testing.T) {
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)}},
	})

	got, err := GenerateValidated(context.Background(), Request{
		Provider:   provider,
		LLMRequest: validRequest(),
	})
	if err != nil {
		t.Fatalf("GenerateValidated: %v", err)
	}
	if len(got.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(got.Attempts))
	}
	if string(got.Accepted.Content) != `{"title":"CPU","severity":"warning","findings":["high usage"]}` {
		t.Fatalf("accepted content = %s", got.Accepted.Content)
	}
	if provider.Calls(testKey) != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.Calls(testKey))
	}

	got.Accepted.Content[2] = 'X'
	if string(got.Output.Content) != `{"title":"CPU","severity":"warning","findings":["high usage"]}` {
		t.Fatalf("accepted and output content share bytes: %s", got.Output.Content)
	}
}

func TestGenerateValidated_RetriesValidationErrorWithFeedback(t *testing.T) {
	rec := &recordingProvider{provider: fakellm.New(map[string][]fakellm.Result{
		testKey: {
			{Response: response(`{"title":"","severity":"warning","findings":[]}`)},
			{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)},
		},
	})}

	got, err := GenerateValidated(context.Background(), Request{
		Provider:    rec,
		LLMRequest:  validRequest(),
		MaxAttempts: 3,
	})
	if err != nil {
		t.Fatalf("GenerateValidated: %v", err)
	}
	if len(got.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(got.Attempts))
	}
	if got.Attempts[0].Reason != llmoutput.ReasonSchemaViolation {
		t.Fatalf("attempt 1 reason = %q", got.Attempts[0].Reason)
	}
	if len(rec.requests) != 2 {
		t.Fatalf("recorded requests = %d, want 2", len(rec.requests))
	}
	if len(rec.requests[0].Messages) != 1 {
		t.Fatalf("first request messages = %d, want original only", len(rec.requests[0].Messages))
	}
	if len(rec.requests[1].Messages) != 1 {
		t.Fatalf("second request messages = %d, want feedback merged into system instructions", len(rec.requests[1].Messages))
	}
	feedback := rec.requests[1].Messages[0]
	if feedback.Role != ports.LLMRoleSystem {
		t.Fatalf("feedback role = %q", feedback.Role)
	}
	if !strings.Contains(feedback.Content, "failed validation") ||
		!strings.Contains(feedback.Content, string(llmoutput.ReasonSchemaViolation)) {
		t.Fatalf("feedback content = %q", feedback.Content)
	}
}

func TestGenerateValidated_UsesCustomValidator(t *testing.T) {
	rec := &recordingProvider{provider: fakellm.New(map[string][]fakellm.Result{
		testKey: {
			{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)},
			{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)},
		},
	})}
	validatorCalls := 0
	validator := func(req ports.LLMRequest, resp ports.LLMResponse) (llmoutput.Accepted, error) {
		accepted, err := llmoutput.Validate(req, resp)
		if err != nil {
			return llmoutput.Accepted{}, err
		}
		validatorCalls++
		if validatorCalls == 1 {
			return llmoutput.Accepted{}, &llmoutput.Error{
				Reason:    llmoutput.ReasonSchemaViolation,
				Retryable: true,
				Err:       errors.New("semantic evidence_refs mismatch"),
			}
		}
		return accepted, nil
	}

	got, err := GenerateValidated(context.Background(), Request{
		Provider:    rec,
		LLMRequest:  validRequest(),
		MaxAttempts: 2,
		Validator:   validator,
	})
	if err != nil {
		t.Fatalf("GenerateValidated: %v", err)
	}
	if len(got.Attempts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(got.Attempts))
	}
	if validatorCalls != 2 {
		t.Fatalf("validator calls = %d, want 2", validatorCalls)
	}
	if len(rec.requests) != 2 ||
		!strings.Contains(rec.requests[1].Messages[0].Content, "semantic evidence_refs mismatch") {
		t.Fatalf("second request feedback = %+v", rec.requests)
	}
}

func TestAddValidationFeedbackPrependsSystemWithoutReplacingLatestUser(t *testing.T) {
	messages := []ports.LLMMessage{{Role: ports.LLMRoleUser, Content: "请继续分析。"}}
	got := addValidationFeedback(messages, errors.New("schema violation"))
	if len(got) != 2 || got[0].Role != ports.LLMRoleSystem ||
		!strings.Contains(got[0].Content, "schema violation") {
		t.Fatalf("feedback messages = %+v", got)
	}
	if got[1] != messages[0] {
		t.Fatalf("latest user message changed: %+v", got)
	}
}

func TestGenerateValidated_StopsOnNonRetryableRefusal(t *testing.T) {
	refusal := "cannot comply"
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {{
			Response: ports.LLMResponse{
				FinishReason: "stop",
				Refusal:      &refusal,
				OutputMode:   ports.LLMOutputModeJSONSchema,
			},
		}},
	})

	_, err := GenerateValidated(context.Background(), Request{
		Provider:    provider,
		LLMRequest:  validRequest(),
		MaxAttempts: 3,
	})
	assertRetryError(t, err, 1, llmoutput.ReasonRefusal)
	if provider.Calls(testKey) != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.Calls(testKey))
	}
}

func TestGenerateValidated_StopsAfterMaxAttempts(t *testing.T) {
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {
			{Response: response(`{"title":"","severity":"warning","findings":[]}`)},
			{Response: response(`{"title":"","severity":"warning","findings":[]}`)},
			{Response: response(`{"title":"","severity":"warning","findings":[]}`)},
		},
	})

	_, err := GenerateValidated(context.Background(), Request{
		Provider:    provider,
		LLMRequest:  validRequest(),
		MaxAttempts: 2,
	})
	assertRetryError(t, err, 2, llmoutput.ReasonSchemaViolation)
	if provider.Calls(testKey) != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.Calls(testKey))
	}
}

func TestGenerateValidated_ReturnsProviderErrorWithoutRetry(t *testing.T) {
	wantErr := errors.New("provider unavailable")
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {{Err: wantErr}},
	})

	_, err := GenerateValidated(context.Background(), Request{
		Provider:    provider,
		LLMRequest:  validRequest(),
		MaxAttempts: 3,
	})
	var retryErr *Error
	if !errors.As(err, &retryErr) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if !errors.Is(retryErr.Err, wantErr) {
		t.Fatalf("inner err = %v, want %v", retryErr.Err, wantErr)
	}
	if len(retryErr.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(retryErr.Attempts))
	}
}

func TestGenerateValidated_ValidatesRequest(t *testing.T) {
	baseReq := validRequest()
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)}},
	})

	tests := []struct {
		name string
		req  Request
	}{
		{
			name: "nil provider",
			req: Request{
				Provider:   nil,
				LLMRequest: baseReq,
			},
		},
		{
			name: "empty idempotency key",
			req: Request{
				Provider:   provider,
				LLMRequest: ports.LLMRequest{OutputSchemaID: baseReq.OutputSchemaID, OutputSchema: baseReq.OutputSchema},
			},
		},
		{
			name: "negative max attempts",
			req: Request{
				Provider:    provider,
				LLMRequest:  baseReq,
				MaxAttempts: -1,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateValidated(context.Background(), tc.req); err == nil {
				t.Fatal("GenerateValidated err = nil, want error")
			}
		})
	}
}

func TestGenerateValidated_HonoursCancelledContext(t *testing.T) {
	provider := fakellm.New(map[string][]fakellm.Result{
		testKey: {{Response: response(`{"title":"CPU","severity":"warning","findings":["high usage"]}`)}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GenerateValidated(ctx, Request{
		Provider:   provider,
		LLMRequest: validRequest(),
	})
	var retryErr *Error
	if !errors.As(err, &retryErr) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if !errors.Is(retryErr.Err, context.Canceled) {
		t.Fatalf("inner err = %v, want context.Canceled", retryErr.Err)
	}
	if len(retryErr.Attempts) != 0 {
		t.Fatalf("attempts = %d, want 0 before provider call", len(retryErr.Attempts))
	}
}

func assertRetryError(t *testing.T, err error, attempts int, reason llmoutput.Reason) {
	t.Helper()
	var retryErr *Error
	if !errors.As(err, &retryErr) {
		t.Fatalf("err type = %T, want *Error", err)
	}
	if len(retryErr.Attempts) != attempts {
		t.Fatalf("attempts = %d, want %d", len(retryErr.Attempts), attempts)
	}
	last := retryErr.Attempts[len(retryErr.Attempts)-1]
	if last.Reason != reason {
		t.Fatalf("last reason = %q, want %q", last.Reason, reason)
	}
}
