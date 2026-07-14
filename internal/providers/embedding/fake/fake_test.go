package fake

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderScriptAndDefensiveCopies(t *testing.T) {
	key := "embed:test"
	vector := make([]float32, domain.RetrievalEmbeddingDimensions)
	vector[0] = 1
	scripts := map[string][]Result{key: {{Response: ports.EmbeddingResponse{
		Model:   "embed-model",
		Vectors: [][]float32{vector},
	}}}}
	provider := New("embed-model", scripts)
	vector[0] = 9

	first, err := provider.Embed(context.Background(), ports.EmbeddingRequest{Inputs: []string{"one"}, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("Embed first: %v", err)
	}
	first.Vectors[0][0] = 8
	second, err := provider.Embed(context.Background(), ports.EmbeddingRequest{Inputs: []string{"one"}, IdempotencyKey: key})
	if err != nil {
		t.Fatalf("Embed second: %v", err)
	}
	if second.Vectors[0][0] != 1 || provider.Calls(key) != 2 {
		t.Fatalf("second vector/calls = %v/%d, want 1/2", second.Vectors[0][0], provider.Calls(key))
	}
}

func TestProviderDeterministicFallback(t *testing.T) {
	provider := NewDeterministic("embed-model")
	req := ports.EmbeddingRequest{Inputs: []string{"same", "same"}, IdempotencyKey: "embed:dynamic"}
	got, err := provider.Embed(context.Background(), req)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got.Vectors) != 2 || len(got.Vectors[0]) != domain.RetrievalEmbeddingDimensions {
		t.Fatalf("vector shape = %d/%d", len(got.Vectors), len(got.Vectors[0]))
	}
	if got.Vectors[0][17] != got.Vectors[1][17] || got.Vectors[0][17] == 0 {
		t.Fatalf("deterministic values = %v/%v", got.Vectors[0][17], got.Vectors[1][17])
	}
}

func TestProviderRejectsInvalidAndHonorsCancellation(t *testing.T) {
	provider := NewDeterministic("embed-model")
	for _, req := range []ports.EmbeddingRequest{
		{},
		{Inputs: []string{"one"}, IdempotencyKey: "embed: bad"},
		{Inputs: []string{"one"}, IdempotencyKey: strings.Repeat("k", maxEmbeddingIdempotencyKeyBytes+1)},
	} {
		if _, err := provider.Embed(context.Background(), req); err == nil {
			t.Fatalf("Embed(%+v) error = nil", req)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider.Embed(ctx, ports.EmbeddingRequest{Inputs: []string{"one"}, IdempotencyKey: "key"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v, want context.Canceled", err)
	}
}
