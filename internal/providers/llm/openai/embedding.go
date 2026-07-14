package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const maxEmbeddingInputs = 32

const maxEmbeddingIdempotencyKeyBytes = 512

// EmbeddingConfig configures the OpenAI-compatible embeddings endpoint.
type EmbeddingConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	HTTPClient *http.Client
}

// EmbeddingProvider implements the provider-neutral semantic embedding port.
type EmbeddingProvider struct {
	endpoint   string
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
}

var _ ports.EmbeddingProvider = (*EmbeddingProvider)(nil)

// Model returns the configured embedding-space identity used by persistence
// prechecks and model-scoped nearest-neighbor queries.
func (p *EmbeddingProvider) Model() string {
	if p == nil {
		return ""
	}
	return p.model
}

// NewEmbeddingProvider constructs a fixed-dimension embedding client.
func NewEmbeddingProvider(cfg EmbeddingConfig) (*EmbeddingProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" || len(model) > 128 {
		return nil, fmt.Errorf("openai embedding: model must contain 1-128 bytes")
	}
	if cfg.Dimensions != domain.RetrievalEmbeddingDimensions {
		return nil, fmt.Errorf("openai embedding: dimensions must equal %d", domain.RetrievalEmbeddingDimensions)
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	endpoint, err := embeddingsEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	return &EmbeddingProvider{
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: cfg.Dimensions,
		httpClient: client,
	}, nil
}

// Embed creates vectors and restores provider results to request input order.
func (p *EmbeddingProvider) Embed(ctx context.Context, req ports.EmbeddingRequest) (ports.EmbeddingResponse, error) {
	if p == nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: provider is nil")
	}
	if !validEmbeddingIdempotencyKey(req.IdempotencyKey) {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: idempotency key must contain 1-%d visible ASCII bytes", maxEmbeddingIdempotencyKeyBytes)
	}
	if len(req.Inputs) == 0 || len(req.Inputs) > maxEmbeddingInputs {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: inputs must contain 1-%d values", maxEmbeddingInputs)
	}
	inputs := make([]string, len(req.Inputs))
	for i, input := range req.Inputs {
		input = strings.TrimSpace(input)
		if input == "" || len([]byte(input)) > domain.RetrievalChunkMaxBytes {
			return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: input[%d] must contain 1-%d bytes", i, domain.RetrievalChunkMaxBytes)
		}
		inputs[i] = input
	}
	body := embeddingRequest{
		Model:          p.model,
		Input:          inputs,
		Dimensions:     p.dimensions,
		EncodingFormat: "float",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Client-Request-Id", req.IdempotencyKey)
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: post embeddings: %w", redactHTTPClientError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: %w", apiStatusError(resp))
	}
	responseRaw, err := readLimited(resp.Body, maxResponseBody, "embedding response")
	if err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: read response: %w", err)
	}
	if err := strictjson.RejectDuplicateObjectKeys(responseRaw); err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: ambiguous response: %w", err)
	}
	var out embeddingResponse
	if err := json.Unmarshal(responseRaw, &out); err != nil {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: decode response: %w", err)
	}
	model := strings.TrimSpace(out.Model)
	if model != p.model || len(out.Data) != len(inputs) {
		return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: response model and vector count must match request")
	}
	vectors := make([][]float32, len(inputs))
	for _, item := range out.Data {
		if item.Index < 0 || item.Index >= len(vectors) || vectors[item.Index] != nil {
			return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: response index %d is invalid or duplicated", item.Index)
		}
		if len(item.Embedding) != p.dimensions || !finiteNonZeroEmbedding(item.Embedding) {
			return ports.EmbeddingResponse{}, fmt.Errorf("openai embedding: response index %d must contain %d finite non-zero values", item.Index, p.dimensions)
		}
		vectors[item.Index] = append([]float32(nil), item.Embedding...)
	}
	return ports.EmbeddingResponse{Vectors: vectors, Model: model}, nil
}

func validEmbeddingIdempotencyKey(value string) bool {
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

func embeddingsEndpoint(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("openai embedding: parse base url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("openai embedding: base url must be absolute HTTP(S)")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("openai embedding: base url must not include userinfo")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/embeddings"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func finiteNonZeroEmbedding(vector []float32) bool {
	var normSquared float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		normSquared += float64(value) * float64(value)
	}
	return normSquared > 0
}

type embeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	Dimensions     int      `json:"dimensions"`
	EncodingFormat string   `json:"encoding_format"`
}

type embeddingResponse struct {
	Model string                  `json:"model"`
	Data  []embeddingResponseItem `json:"data"`
}

type embeddingResponseItem struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}
