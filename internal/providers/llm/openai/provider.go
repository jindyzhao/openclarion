// Package openai provides an OpenAI-compatible Chat Completions
// implementation of ports.LLMProvider.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultBaseURL  = "https://api.openai.com/v1"
	defaultTimeout  = 30 * time.Second
	maxErrorBody    = 1 << 20
	maxResponseBody = 4 << 20
)

var schemaNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Config holds OpenAI-compatible provider configuration.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	OutputMode ports.LLMOutputMode
	HTTPClient *http.Client
}

// Provider is a thin Chat Completions client. It is safe for
// concurrent use when the configured HTTPClient is safe for
// concurrent use.
type Provider struct {
	endpoint   string
	apiKey     string
	model      string
	outputMode ports.LLMOutputMode
	httpClient *http.Client
}

var (
	_ ports.LLMProvider          = (*Provider)(nil)
	_ ports.StreamingLLMProvider = (*Provider)(nil)
)

// NewProvider constructs a Provider with an explicit output mode.
// Use NewProviderWithCapabilityDetection when the caller wants to
// probe strict structured-output support at startup.
func NewProvider(cfg Config) (*Provider, error) {
	if cfg.OutputMode != ports.LLMOutputModeJSONSchema && cfg.OutputMode != ports.LLMOutputModeJSONObject {
		return nil, fmt.Errorf("openai llm: output mode %q is unsupported", cfg.OutputMode)
	}
	return newProvider(cfg)
}

// NewProviderWithCapabilityDetection probes strict JSON Schema
// support and falls back to json_object mode only when the upstream
// rejects the strict response_format as unsupported.
func NewProviderWithCapabilityDetection(ctx context.Context, cfg Config) (*Provider, error) {
	probe, err := newProvider(withOutputMode(cfg, ports.LLMOutputModeJSONSchema))
	if err != nil {
		return nil, err
	}
	if err := probeStrictJSONSchema(ctx, probe); err == nil {
		return probe, nil
	} else if !isUnsupportedResponseFormat(err) {
		return nil, err
	}
	return newProvider(withOutputMode(cfg, ports.LLMOutputModeJSONObject))
}

// OutputMode reports the provider output mode selected at
// construction time.
func (p *Provider) OutputMode() ports.LLMOutputMode {
	if p == nil {
		return ""
	}
	return p.outputMode
}

// GenerateJSON executes one Chat Completions request and maps the
// first choice into ports.LLMResponse. It deliberately does not
// validate the returned JSON; callers use llmoutput/reportdraft for
// acceptance checks.
func (p *Provider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	if p == nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: provider is nil")
	}
	if req.IdempotencyKey == "" {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: idempotency key must be non-empty")
	}
	body, err := p.buildRequest(req)
	if err != nil {
		return ports.LLMResponse{}, err
	}

	var out chatCompletionResponse
	if err := p.post(ctx, body, req.IdempotencyKey, &out); err != nil {
		return ports.LLMResponse{}, err
	}
	if len(out.Choices) == 0 {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: response has no choices")
	}
	choice := out.Choices[0]
	return ports.LLMResponse{
		Content:      json.RawMessage(choice.Message.Content),
		FinishReason: choice.FinishReason,
		Refusal:      choice.Message.Refusal,
		OutputMode:   p.outputMode,
		Model:        out.Model,
	}, nil
}

func newProvider(cfg Config) (*Provider, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("openai llm: model must be non-empty")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	endpoint, err := chatCompletionsEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	return &Provider{
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		model:      model,
		outputMode: cfg.OutputMode,
		httpClient: client,
	}, nil
}

func withOutputMode(cfg Config, mode ports.LLMOutputMode) Config {
	cfg.OutputMode = mode
	return cfg
}

func chatCompletionsEndpoint(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("openai llm: parse base url")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("openai llm: base url must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("openai llm: base url scheme must be http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("openai llm: base url must not include userinfo")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (p *Provider) buildRequest(req ports.LLMRequest) (chatCompletionRequest, error) {
	if len(req.Messages) == 0 {
		return chatCompletionRequest{}, fmt.Errorf("openai llm: messages must be non-empty")
	}
	messages := make([]chatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		if msg.Role != ports.LLMRoleSystem && msg.Role != ports.LLMRoleUser && msg.Role != ports.LLMRoleAssistant {
			return chatCompletionRequest{}, fmt.Errorf("openai llm: message[%d] role %q is unsupported", i, msg.Role)
		}
		if strings.TrimSpace(msg.Content) == "" {
			return chatCompletionRequest{}, fmt.Errorf("openai llm: message[%d] content must be non-empty", i)
		}
		messages[i] = chatMessage{Role: string(msg.Role), Content: msg.Content}
	}

	responseFormat, err := p.responseFormat(req)
	if err != nil {
		return chatCompletionRequest{}, err
	}
	return chatCompletionRequest{
		Model:          p.model,
		Messages:       messages,
		ResponseFormat: responseFormat,
	}, nil
}

func (p *Provider) responseFormat(req ports.LLMRequest) (responseFormat, error) {
	switch p.outputMode {
	case ports.LLMOutputModeJSONObject:
		return responseFormat{Type: string(ports.LLMOutputModeJSONObject)}, nil
	case ports.LLMOutputModeJSONSchema:
		if !schemaNamePattern.MatchString(req.OutputSchemaID) {
			return responseFormat{}, fmt.Errorf("openai llm: output schema id %q must match [A-Za-z0-9_-]{1,64}", req.OutputSchemaID)
		}
		if len(req.OutputSchema) == 0 {
			return responseFormat{}, fmt.Errorf("openai llm: output schema must be non-empty")
		}
		var schema any
		if err := json.Unmarshal(req.OutputSchema, &schema); err != nil {
			return responseFormat{}, fmt.Errorf("openai llm: output schema must be valid JSON: %w", err)
		}
		return responseFormat{
			Type: string(ports.LLMOutputModeJSONSchema),
			JSONSchema: &jsonSchemaFormat{
				Name:   req.OutputSchemaID,
				Strict: true,
				Schema: schema,
			},
		}, nil
	default:
		return responseFormat{}, fmt.Errorf("openai llm: output mode %q is unsupported", p.outputMode)
	}
}

func (p *Provider) post(ctx context.Context, body chatCompletionRequest, idempotencyKey string, out *chatCompletionResponse) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("openai llm: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("openai llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Client-Request-Id", idempotencyKey)
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("openai llm: post chat completion: %w", redactHTTPClientError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiStatusError(resp)
	}
	if err := decodeChatCompletionResponse(resp.Body, out); err != nil {
		return fmt.Errorf("openai llm: decode response: %w", err)
	}
	return nil
}

func redactHTTPClientError(err error) error {
	if err == nil {
		return nil
	}
	op := ""
	for {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			break
		}
		if op == "" {
			op = urlErr.Op
		}
		if urlErr.Err == nil {
			break
		}
		err = urlErr.Err
	}
	if op != "" {
		return fmt.Errorf("%s: %w", op, err)
	}
	return err
}

func apiStatusError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	trimmed := strings.TrimSpace(string(raw))
	var apiErr errorEnvelope
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
			return &statusError{
				StatusCode: resp.StatusCode,
				Message:    "invalid API error response",
			}
		}
	} else {
		return &statusError{
			StatusCode: resp.StatusCode,
			Message:    trimmed,
		}
	}
	if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Error.Message != "" {
		return &statusError{
			StatusCode: resp.StatusCode,
			Message:    apiErr.Error.Message,
			Type:       apiErr.Error.Type,
			Code:       apiErr.Error.Code,
		}
	}
	return &statusError{
		StatusCode: resp.StatusCode,
		Message:    trimmed,
	}
}

func decodeChatCompletionResponse(body io.Reader, out *chatCompletionResponse) error {
	raw, err := readLimited(body, maxResponseBody, "chat completion response")
	if err != nil {
		return err
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}
	return nil
}

func readLimited(body io.Reader, limit int64, label string) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: limit + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, limit)
	}
	return raw, nil
}

func probeStrictJSONSchema(ctx context.Context, p *Provider) error {
	req := chatCompletionRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: string(ports.LLMRoleSystem), Content: "Return only JSON."},
			{Role: string(ports.LLMRoleUser), Content: "Return {\"ok\":true}."},
		},
		ResponseFormat: responseFormat{
			Type: string(ports.LLMOutputModeJSONSchema),
			JSONSchema: &jsonSchemaFormat{
				Name:   "openclarion_probe",
				Strict: true,
				Schema: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok"},
					"properties": map[string]any{
						"ok": map[string]any{"type": "boolean"},
					},
				},
			},
		},
	}
	var out chatCompletionResponse
	if err := p.post(ctx, req, "openclarion-capability-probe", &out); err != nil {
		return fmt.Errorf("openai llm: strict capability probe: %w", err)
	}
	return nil
}

func isUnsupportedResponseFormat(err error) bool {
	var status *statusError
	if !strings.Contains(err.Error(), "strict capability probe") {
		return false
	}
	if !errors.As(err, &status) || status.StatusCode != http.StatusBadRequest {
		return false
	}
	text := strings.ToLower(status.Message + " " + status.Type + " " + status.Code)
	return strings.Contains(text, "response_format") ||
		strings.Contains(text, "json_schema") ||
		strings.Contains(text, "strict") ||
		strings.Contains(text, "unsupported")
}

type chatCompletionRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	ResponseFormat responseFormat `json:"response_format"`
	Stream         bool           `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *jsonSchemaFormat `json:"json_schema,omitempty"`
}

type jsonSchemaFormat struct {
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

type chatCompletionResponse struct {
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
}

type choice struct {
	Message      assistantMessage `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type assistantMessage struct {
	Content string  `json:"content"`
	Refusal *string `json:"refusal"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type statusError struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *statusError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		return fmt.Sprintf("openai llm: status %d", e.StatusCode)
	}
	return fmt.Sprintf("openai llm: status %d: %s", e.StatusCode, e.Message)
}
