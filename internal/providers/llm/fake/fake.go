// Package fake provides a deterministic in-memory LLMProvider for
// usecase and workflow tests. It is intentionally scriptable by
// idempotency key so retry tests can model "invalid first output,
// valid second output" without calling a real model.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Result is one scripted provider outcome.
type Result struct {
	Response ports.LLMResponse
	Err      error
}

// Provider is a deterministic, concurrency-safe LLMProvider
// implementation. Each idempotency key owns an independent script;
// after a script is exhausted, the provider repeats its last result so
// tests remain deterministic under extra retries.
type Provider struct {
	mu      sync.Mutex
	scripts map[string][]Result
	calls   map[string]int
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.LLMProvider = (*Provider)(nil)

// New constructs a Provider from scripts keyed by
// ports.LLMRequest.IdempotencyKey. The scripts are deep-copied so
// caller-side mutations after construction cannot change provider
// behavior.
func New(scripts map[string][]Result) *Provider {
	return &Provider{
		scripts: cloneScripts(scripts),
		calls:   map[string]int{},
	}
}

// GenerateJSON returns the next scripted Result for req.IdempotencyKey.
// Unknown or empty keys are rejected because the M2 activity contract
// requires idempotency for retry-safe report generation.
func (p *Provider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	if err := ctx.Err(); err != nil {
		return ports.LLMResponse{}, err
	}
	if req.IdempotencyKey == "" {
		return ports.LLMResponse{}, fmt.Errorf("fake llm: idempotency key must be non-empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	script, ok := p.scripts[req.IdempotencyKey]
	if !ok || len(script) == 0 {
		return ports.LLMResponse{}, fmt.Errorf("fake llm: no script for idempotency key %q", req.IdempotencyKey)
	}
	call := p.calls[req.IdempotencyKey]
	p.calls[req.IdempotencyKey] = call + 1
	if call >= len(script) {
		call = len(script) - 1
	}

	result := script[call]
	if result.Err != nil {
		return ports.LLMResponse{}, result.Err
	}
	return cloneResponse(result.Response), nil
}

// Calls returns how many GenerateJSON calls have been made for the
// given idempotency key.
func (p *Provider) Calls(idempotencyKey string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[idempotencyKey]
}

func cloneScripts(in map[string][]Result) map[string][]Result {
	if in == nil {
		return nil
	}
	out := make(map[string][]Result, len(in))
	for key, script := range in {
		if script == nil {
			out[key] = nil
			continue
		}
		copied := make([]Result, len(script))
		for i, result := range script {
			copied[i] = Result{
				Response: cloneResponse(result.Response),
				Err:      result.Err,
			}
		}
		out[key] = copied
	}
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

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
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
