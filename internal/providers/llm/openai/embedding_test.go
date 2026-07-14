package openai

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestEmbeddingProviderRestoresResponseIndexOrder(t *testing.T) {
	first := embeddingVector(1)
	second := embeddingVector(2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Client-Request-Id") != "embed:test" {
			t.Errorf("request path/auth/id = %q/%q/%q", r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Client-Request-Id"))
		}
		var body embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "embed-model" || body.Dimensions != domain.RetrievalEmbeddingDimensions || body.EncodingFormat != "float" || len(body.Input) != 2 {
			t.Errorf("request body = %+v", body)
		}
		_ = json.NewEncoder(w).Encode(embeddingResponse{
			Model: "embed-model",
			Data: []embeddingResponseItem{
				{Index: 1, Embedding: second},
				{Index: 0, Embedding: first},
			},
		})
	}))
	defer server.Close()

	provider, err := NewEmbeddingProvider(EmbeddingConfig{
		BaseURL: server.URL + "/v1", APIKey: "secret", Model: "embed-model",
		Dimensions: domain.RetrievalEmbeddingDimensions, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider: %v", err)
	}
	got, err := provider.Embed(context.Background(), ports.EmbeddingRequest{
		Inputs: []string{"first", "second"}, IdempotencyKey: "embed:test",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got.Model != "embed-model" || got.Vectors[0][0] != 1 || got.Vectors[1][0] != 2 {
		t.Fatalf("response = model %q vectors %v/%v", got.Model, got.Vectors[0][0], got.Vectors[1][0])
	}
}

func TestEmbeddingProviderRejectsMalformedResponse(t *testing.T) {
	tests := []struct {
		name     string
		response embeddingResponse
	}{
		{name: "wrong model", response: embeddingResponse{Model: "other", Data: []embeddingResponseItem{{Index: 0, Embedding: embeddingVector(1)}}}},
		{name: "duplicate index", response: embeddingResponse{Model: "embed-model", Data: []embeddingResponseItem{{Index: 0, Embedding: embeddingVector(1)}, {Index: 0, Embedding: embeddingVector(2)}}}},
		{name: "wrong dimensions", response: embeddingResponse{Model: "embed-model", Data: []embeddingResponseItem{{Index: 0, Embedding: []float32{1}}}}},
		{name: "zero vector", response: embeddingResponse{Model: "embed-model", Data: []embeddingResponseItem{{Index: 0, Embedding: embeddingVector(0)}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			defer server.Close()
			provider, err := NewEmbeddingProvider(EmbeddingConfig{
				BaseURL: server.URL, Model: "embed-model", Dimensions: domain.RetrievalEmbeddingDimensions, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("NewEmbeddingProvider: %v", err)
			}
			inputs := []string{"one"}
			if tc.name == "duplicate index" {
				inputs = []string{"one", "two"}
			}
			if _, err := provider.Embed(context.Background(), ports.EmbeddingRequest{Inputs: inputs, IdempotencyKey: "embed:test"}); err == nil {
				t.Fatal("Embed error = nil")
			}
		})
	}
}

func TestEmbeddingProviderRejectsAmbiguousResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"model":"embed-model","model":"other","data":[]}`))
	}))
	defer server.Close()
	provider, err := NewEmbeddingProvider(EmbeddingConfig{
		BaseURL: server.URL, Model: "embed-model", Dimensions: domain.RetrievalEmbeddingDimensions, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider: %v", err)
	}
	if _, err := provider.Embed(context.Background(), ports.EmbeddingRequest{Inputs: []string{"one"}, IdempotencyKey: "embed:test"}); err == nil {
		t.Fatal("Embed error = nil")
	}
}

func TestEmbeddingProviderRejectsInvalidRequest(t *testing.T) {
	provider, err := NewEmbeddingProvider(EmbeddingConfig{
		BaseURL: "https://example.test/v1", Model: "embed-model", Dimensions: domain.RetrievalEmbeddingDimensions,
	})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider: %v", err)
	}
	tooMany := make([]string, maxEmbeddingInputs+1)
	for i := range tooMany {
		tooMany[i] = "input"
	}
	for _, req := range []ports.EmbeddingRequest{
		{},
		{Inputs: []string{"one"}, IdempotencyKey: " embed:test "},
		{Inputs: []string{"one"}, IdempotencyKey: "embed:\nmalformed"},
		{Inputs: []string{"one"}, IdempotencyKey: strings.Repeat("k", maxEmbeddingIdempotencyKeyBytes+1)},
		{Inputs: []string{strings.Repeat("x", domain.RetrievalChunkMaxBytes+1)}, IdempotencyKey: "embed:test"},
		{Inputs: tooMany, IdempotencyKey: "embed:test"},
	} {
		if _, err := provider.Embed(context.Background(), req); err == nil {
			t.Fatalf("Embed(%+v) error = nil", req)
		}
	}
}

func TestNewEmbeddingProviderRejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []EmbeddingConfig{
		{},
		{Model: strings.Repeat("m", 129), Dimensions: domain.RetrievalEmbeddingDimensions},
		{Model: "model", Dimensions: 3},
		{Model: "model", Dimensions: domain.RetrievalEmbeddingDimensions, BaseURL: "ftp://example.test"},
		{Model: "model", Dimensions: domain.RetrievalEmbeddingDimensions, BaseURL: "https://user:secret@example.test/v1"}, // #nosec G101 -- verifies rejection of credential-bearing URLs.
	} {
		if _, err := NewEmbeddingProvider(cfg); err == nil {
			t.Fatalf("NewEmbeddingProvider(%+v) error = nil", cfg)
		}
	}
}

func TestFiniteNonZeroEmbedding(t *testing.T) {
	for _, vector := range [][]float32{
		make([]float32, domain.RetrievalEmbeddingDimensions),
		{float32(math.NaN())},
		{float32(math.Inf(1))},
	} {
		if finiteNonZeroEmbedding(vector) {
			t.Fatalf("finiteNonZeroEmbedding(%v) = true", vector[:1])
		}
	}
}

func embeddingVector(first float32) []float32 {
	vector := make([]float32, domain.RetrievalEmbeddingDimensions)
	vector[0] = first
	return vector
}
