// Package fake provides deterministic, concurrency-safe embedding providers
// for usecase, Activity, and repository integration tests.
package fake

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Result is one scripted embedding outcome.
type Result struct {
	Response ports.EmbeddingResponse
	Err      error
}

// Provider scripts calls by idempotency key or deterministically derives one
// vector per input when deterministic fallback is enabled.
type Provider struct {
	mu            sync.Mutex
	model         string
	scripts       map[string][]Result
	calls         map[string]int
	deterministic bool
}

var _ ports.EmbeddingProvider = (*Provider)(nil)

const (
	maxEmbeddingInputs              = 32
	maxEmbeddingIdempotencyKeyBytes = 512
)

// New constructs a scripted provider and deep-copies caller-owned values.
func New(model string, scripts map[string][]Result) *Provider {
	return &Provider{
		model:   strings.TrimSpace(model),
		scripts: cloneScripts(scripts),
		calls:   make(map[string]int),
	}
}

// NewDeterministic constructs a provider suitable for tests whose source IDs
// and therefore idempotency keys are assigned at runtime.
func NewDeterministic(model string) *Provider {
	p := New(model, nil)
	p.deterministic = true
	return p
}

// Model returns the configured embedding-space identity.
func (p *Provider) Model() string {
	if p == nil {
		return ""
	}
	return p.model
}

// Embed returns the next scripted response or a deterministic local vector.
func (p *Provider) Embed(ctx context.Context, req ports.EmbeddingRequest) (ports.EmbeddingResponse, error) {
	if err := ctx.Err(); err != nil {
		return ports.EmbeddingResponse{}, err
	}
	if p == nil || p.model == "" {
		return ports.EmbeddingResponse{}, fmt.Errorf("fake embedding: model must be configured")
	}
	if !validIdempotencyKey(req.IdempotencyKey) {
		return ports.EmbeddingResponse{}, fmt.Errorf("fake embedding: idempotency key must contain 1-%d visible ASCII bytes", maxEmbeddingIdempotencyKeyBytes)
	}
	if len(req.Inputs) == 0 || len(req.Inputs) > maxEmbeddingInputs {
		return ports.EmbeddingResponse{}, fmt.Errorf("fake embedding: inputs must contain 1-%d values", maxEmbeddingInputs)
	}
	for i, input := range req.Inputs {
		if strings.TrimSpace(input) == "" || len([]byte(input)) > domain.RetrievalChunkMaxBytes {
			return ports.EmbeddingResponse{}, fmt.Errorf("fake embedding: input[%d] must be non-empty and bounded", i)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	call := p.calls[req.IdempotencyKey]
	p.calls[req.IdempotencyKey] = call + 1
	if script := p.scripts[req.IdempotencyKey]; len(script) > 0 {
		if call >= len(script) {
			call = len(script) - 1
		}
		result := script[call]
		if result.Err != nil {
			return ports.EmbeddingResponse{}, result.Err
		}
		return cloneResponse(result.Response), nil
	}
	if !p.deterministic {
		return ports.EmbeddingResponse{}, fmt.Errorf("fake embedding: no script for idempotency key %q", req.IdempotencyKey)
	}
	vectors := make([][]float32, len(req.Inputs))
	for i, input := range req.Inputs {
		vectors[i] = deterministicVector(input)
	}
	return ports.EmbeddingResponse{Vectors: vectors, Model: p.model}, nil
}

func validIdempotencyKey(value string) bool {
	if len(value) == 0 || len(value) > maxEmbeddingIdempotencyKeyBytes {
		return false
	}
	for i := range len(value) {
		if value[i] < 0x21 || value[i] > 0x7e {
			return false
		}
	}
	return true
}

// Calls returns how many calls used one idempotency key.
func (p *Provider) Calls(idempotencyKey string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[idempotencyKey]
}

func deterministicVector(input string) []float32 {
	digest := sha256.Sum256([]byte(strings.TrimSpace(input)))
	vector := make([]float32, domain.RetrievalEmbeddingDimensions)
	for i := range vector {
		// Positive values keep independently generated fixtures within the
		// default cosine threshold while preserving deterministic ordering.
		vector[i] = float32(digest[i%len(digest)]+1) / 256
	}
	return vector
}

func cloneScripts(in map[string][]Result) map[string][]Result {
	out := make(map[string][]Result, len(in))
	for key, script := range in {
		copied := make([]Result, len(script))
		for i, result := range script {
			copied[i] = Result{Response: cloneResponse(result.Response), Err: result.Err}
		}
		out[key] = copied
	}
	return out
}

func cloneResponse(in ports.EmbeddingResponse) ports.EmbeddingResponse {
	out := ports.EmbeddingResponse{Model: in.Model, Vectors: make([][]float32, len(in.Vectors))}
	for i, vector := range in.Vectors {
		out.Vectors[i] = append([]float32(nil), vector...)
	}
	return out
}
