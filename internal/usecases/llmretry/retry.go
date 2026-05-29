// Package llmretry runs bounded LLM generation attempts and feeds
// validation failures back into the next prompt turn.
package llmretry

import (
	"context"
	"errors"
	"fmt"

	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const defaultMaxAttempts = 3

// Request configures one validated generation run.
type Request struct {
	Provider    ports.LLMProvider
	LLMRequest  ports.LLMRequest
	MaxAttempts int
}

// Attempt records one provider call and its validation outcome.
type Attempt struct {
	Number   int
	Response ports.LLMResponse
	Err      error
	Reason   llmoutput.Reason
}

// Result is returned after a successful validated generation.
type Result struct {
	Accepted ports.LLMResponse
	Output   llmoutput.Accepted
	Attempts []Attempt
}

// Error is returned when generation never produced accepted output.
type Error struct {
	Attempts []Attempt
	Err      error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return "llm retry failed"
	}
	return fmt.Sprintf("llm retry failed: %v", e.Err)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// GenerateValidated calls the provider until output passes
// llmoutput.Validate, a non-retryable error occurs, context is
// canceled, or MaxAttempts is exhausted. Retry feedback is appended as
// a user message so provider implementations do not need to know about
// the validation package.
func GenerateValidated(ctx context.Context, req Request) (Result, error) {
	if req.Provider == nil {
		return Result{}, fmt.Errorf("llm retry: provider must be non-nil")
	}
	if req.LLMRequest.IdempotencyKey == "" {
		return Result{}, fmt.Errorf("llm retry: idempotency key must be non-empty")
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = defaultMaxAttempts
	}
	if maxAttempts < 0 {
		return Result{}, fmt.Errorf("llm retry: max_attempts must be >= 0 (got %d)", req.MaxAttempts)
	}
	if maxAttempts == 0 {
		return Result{}, fmt.Errorf("llm retry: max_attempts must be > 0")
	}

	attemptReq := cloneRequest(req.LLMRequest)
	attempts := make([]Attempt, 0, maxAttempts)
	var lastErr error

	for i := 1; i <= maxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return Result{}, &Error{Attempts: attempts, Err: err}
		}

		resp, err := req.Provider.GenerateJSON(ctx, attemptReq)
		if err != nil {
			attempts = append(attempts, Attempt{Number: i, Err: err})
			return Result{}, &Error{Attempts: attempts, Err: err}
		}

		output, err := llmoutput.Validate(attemptReq, resp)
		if err == nil {
			attempts = append(attempts, Attempt{Number: i, Response: cloneResponse(resp)})
			return Result{
				Accepted: cloneResponse(resp),
				Output:   output,
				Attempts: cloneAttempts(attempts),
			}, nil
		}

		attempt := Attempt{Number: i, Response: cloneResponse(resp), Err: err}
		var validationErr *llmoutput.Error
		if errors.As(err, &validationErr) {
			attempt.Reason = validationErr.Reason
		}
		attempts = append(attempts, attempt)
		lastErr = err
		if !llmoutput.IsRetryable(err) || i == maxAttempts {
			return Result{}, &Error{Attempts: cloneAttempts(attempts), Err: err}
		}
		attemptReq.Messages = append(attemptReq.Messages, validationFeedbackMessage(err))
	}

	return Result{}, &Error{Attempts: cloneAttempts(attempts), Err: lastErr}
}

func validationFeedbackMessage(err error) ports.LLMMessage {
	return ports.LLMMessage{
		Role: ports.LLMRoleUser,
		Content: "The previous assistant output failed validation. " +
			"Return only corrected JSON that satisfies the requested schema. " +
			"Validation error: " + err.Error(),
	}
}

func cloneRequest(in ports.LLMRequest) ports.LLMRequest {
	out := ports.LLMRequest{
		Messages:       cloneMessages(in.Messages),
		OutputSchema:   cloneRawMessage(in.OutputSchema),
		OutputSchemaID: in.OutputSchemaID,
		IdempotencyKey: in.IdempotencyKey,
	}
	return out
}

func cloneMessages(in []ports.LLMMessage) []ports.LLMMessage {
	if in == nil {
		return nil
	}
	out := make([]ports.LLMMessage, len(in))
	copy(out, in)
	return out
}

func cloneResponse(in ports.LLMResponse) ports.LLMResponse {
	return ports.LLMResponse{
		Content:      cloneRawMessage(in.Content),
		FinishReason: in.FinishReason,
		Refusal:      cloneStringPtr(in.Refusal),
		OutputMode:   in.OutputMode,
		Model:        in.Model,
	}
}

func cloneAttempts(in []Attempt) []Attempt {
	if in == nil {
		return nil
	}
	out := make([]Attempt, len(in))
	for i, attempt := range in {
		out[i] = Attempt{
			Number:   attempt.Number,
			Response: cloneResponse(attempt.Response),
			Err:      attempt.Err,
			Reason:   attempt.Reason,
		}
	}
	return out
}

func cloneRawMessage(in []byte) []byte {
	if in == nil {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
