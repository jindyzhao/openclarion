package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProvider_GenerateJSON_StrictSchemaRequest(t *testing.T) {
	var seen chatCompletionRequest
	var requestID string
	var authorization string
	srv := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		requestID = r.Header.Get("X-Client-Request-Id")
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeChatResponse(w, `{"ok":true}`, nil, "stop")
	})

	p, err := NewProvider(Config{
		BaseURL:    srv.URL + "/v1",
		APIKey:     "test-api-value",
		Model:      "gpt-test",
		OutputMode: ports.LLMOutputModeJSONSchema,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	resp, err := p.GenerateJSON(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	if requestID != "req-1" {
		t.Fatalf("X-Client-Request-Id = %q", requestID)
	}
	if authorization != "Bearer test-api-value" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if seen.Model != "gpt-test" {
		t.Fatalf("model = %q", seen.Model)
	}
	if seen.ResponseFormat.Type != "json_schema" || seen.ResponseFormat.JSONSchema == nil {
		t.Fatalf("response_format = %+v", seen.ResponseFormat)
	}
	if seen.ResponseFormat.JSONSchema.Name != "test_schema" || !seen.ResponseFormat.JSONSchema.Strict {
		t.Fatalf("json_schema = %+v", *seen.ResponseFormat.JSONSchema)
	}
	if len(seen.Messages) != 2 || seen.Messages[0].Role != "system" || seen.Messages[1].Role != "user" {
		t.Fatalf("messages = %+v", seen.Messages)
	}
	if resp.OutputMode != ports.LLMOutputModeJSONSchema || resp.FinishReason != "stop" || resp.Model != "gpt-test" {
		t.Fatalf("response metadata = %+v", resp)
	}
	if string(resp.Content) != `{"ok":true}` {
		t.Fatalf("content = %s", resp.Content)
	}
}

func TestProvider_GenerateJSON_JSONObjectFallbackRequest(t *testing.T) {
	var seen chatCompletionRequest
	srv := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeChatResponse(w, `{"ok":true}`, nil, "stop")
	})

	p, err := NewProvider(Config{
		BaseURL:    srv.URL,
		Model:      "gpt-test",
		OutputMode: ports.LLMOutputModeJSONObject,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	resp, err := p.GenerateJSON(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if seen.ResponseFormat.Type != "json_object" {
		t.Fatalf("response_format.type = %q", seen.ResponseFormat.Type)
	}
	if seen.ResponseFormat.JSONSchema != nil {
		t.Fatalf("json_schema = %+v, want nil", seen.ResponseFormat.JSONSchema)
	}
	if resp.OutputMode != ports.LLMOutputModeJSONObject {
		t.Fatalf("OutputMode = %q", resp.OutputMode)
	}
}

func TestProvider_GenerateJSON_PropagatesRequestID(t *testing.T) {
	var seen string
	srv := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(correlation.RequestIDHeader)
		writeChatResponse(w, `{"ok":true}`, nil, "stop")
	})

	p, err := NewProvider(Config{
		BaseURL:    srv.URL,
		Model:      "gpt-test",
		OutputMode: ports.LLMOutputModeJSONSchema,
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := correlation.ContextWithRequestID(context.Background(), "request-1")
	if _, err := p.GenerateJSON(ctx, validRequest()); err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if seen != "request-1" {
		t.Fatalf("%s = %q, want request-1", correlation.RequestIDHeader, seen)
	}
}

func TestProvider_GenerateJSON_MapsRefusal(t *testing.T) {
	refusal := "request refused"
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeChatResponse(w, "", &refusal, "stop")
	})

	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	resp, err := p.GenerateJSON(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if resp.Refusal == nil || *resp.Refusal != refusal {
		t.Fatalf("Refusal = %v", resp.Refusal)
	}
}

func TestProvider_GenerateJSON_MapsFinishReasonVerbatim(t *testing.T) {
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeChatResponse(w, `{"ok":true}`, nil, "length")
	})

	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	resp, err := p.GenerateJSON(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if resp.FinishReason != "length" {
		t.Fatalf("FinishReason = %q", resp.FinishReason)
	}
}

func TestProvider_GenerateJSON_RejectsAmbiguousResponseEnvelope(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate top-level key",
			body: `{"model":"old","model":"new","choices":[{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}]}`,
			want: `duplicate object key "model"`,
		},
		{
			name: "trailing value",
			body: `{"model":"gpt-test","choices":[{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}]} {"model":"shadow"}`,
			want: "trailing JSON values",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}

			_, err = p.GenerateJSON(context.Background(), validRequest())
			if err == nil {
				t.Fatal("GenerateJSON err = nil, want ambiguous response error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestProvider_GenerateJSON_RejectsOversizedResponseEnvelope(t *testing.T) {
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat(" ", maxResponseBody+1)))
	})

	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.GenerateJSON(context.Background(), validRequest())
	if err == nil {
		t.Fatal("GenerateJSON err = nil, want oversized response error")
	}
	if !strings.Contains(err.Error(), "chat completion response exceeds") {
		t.Fatalf("err = %v, want response size error", err)
	}
}

func TestProvider_GenerateJSON_WrapsAPIError(t *testing.T) {
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"model unavailable","type":"server_error","code":"unavailable"}}`))
	})

	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.GenerateJSON(context.Background(), validRequest())
	if err == nil {
		t.Fatalf("GenerateJSON: want error")
	}
	var status *statusError
	if !errors.As(err, &status) {
		t.Fatalf("err = %T %v, want statusError", err, err)
	}
	if status.StatusCode != http.StatusServiceUnavailable || !strings.Contains(status.Message, "model unavailable") {
		t.Fatalf("status = %+v", status)
	}
}

func TestProvider_GenerateJSON_WrapsPlainTextAPIError(t *testing.T) {
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate limited`))
	})

	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.GenerateJSON(context.Background(), validRequest())
	if err == nil {
		t.Fatalf("GenerateJSON: want error")
	}
	var status *statusError
	if !errors.As(err, &status) {
		t.Fatalf("err = %T %v, want statusError", err, err)
	}
	if status.StatusCode != http.StatusTooManyRequests || status.Message != "rate limited" {
		t.Fatalf("status = %+v", status)
	}
}

func TestProvider_RejectsInvalidRequestBeforeHTTP(t *testing.T) {
	srv := newChatServer(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("server should not be called")
	})
	p, err := NewProvider(Config{BaseURL: srv.URL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	req := validRequest()
	req.OutputSchemaID = "bad.schema"
	_, err = p.GenerateJSON(context.Background(), req)
	if err == nil {
		t.Fatalf("GenerateJSON: want error")
	}
	if !strings.Contains(err.Error(), "output schema id") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewProviderWithCapabilityDetection_StrictSuccess(t *testing.T) {
	var probe chatCompletionRequest
	srv := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&probe); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeChatResponse(w, `{"ok":true}`, nil, "stop")
	})

	p, err := NewProviderWithCapabilityDetection(context.Background(), Config{
		BaseURL: srv.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("NewProviderWithCapabilityDetection: %v", err)
	}
	if p.OutputMode() != ports.LLMOutputModeJSONSchema {
		t.Fatalf("OutputMode = %q", p.OutputMode())
	}
	if probe.ResponseFormat.Type != "json_schema" || probe.ResponseFormat.JSONSchema == nil || probe.ResponseFormat.JSONSchema.Name != "openclarion_probe" {
		t.Fatalf("probe response_format = %+v", probe.ResponseFormat)
	}
}

func TestNewProviderWithCapabilityDetection_FallsBackToJSONObject(t *testing.T) {
	calls := 0
	var generated chatCompletionRequest
	srv := newChatServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format json_schema strict is unsupported","type":"invalid_request_error","code":"unsupported_response_format"}}`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&generated); err != nil {
			t.Fatalf("decode generated request: %v", err)
		}
		writeChatResponse(w, `{"ok":true}`, nil, "stop")
	})

	p, err := NewProviderWithCapabilityDetection(context.Background(), Config{
		BaseURL: srv.URL,
		Model:   "gpt-test",
	})
	if err != nil {
		t.Fatalf("NewProviderWithCapabilityDetection: %v", err)
	}
	if p.OutputMode() != ports.LLMOutputModeJSONObject {
		t.Fatalf("OutputMode = %q", p.OutputMode())
	}
	if _, err := p.GenerateJSON(context.Background(), validRequest()); err != nil {
		t.Fatalf("GenerateJSON after fallback: %v", err)
	}
	if generated.ResponseFormat.Type != "json_object" {
		t.Fatalf("generated response_format = %+v", generated.ResponseFormat)
	}
}

func TestNewProviderWithCapabilityDetection_PropagatesNonCapabilityError(t *testing.T) {
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"authentication_error","code":"invalid_api_key"}}`))
	})

	_, err := NewProviderWithCapabilityDetection(context.Background(), Config{
		BaseURL: srv.URL,
		Model:   "gpt-test",
	})
	if err == nil {
		t.Fatalf("NewProviderWithCapabilityDetection: want error")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("err = %v", err)
	}
}

func TestNewProviderWithCapabilityDetection_RejectsAmbiguousCapabilityError(t *testing.T) {
	calls := 0
	srv := newChatServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"response_format unsupported","message":"shadowed","type":"invalid_request_error","code":"unsupported_response_format"}}`))
	})

	_, err := NewProviderWithCapabilityDetection(context.Background(), Config{
		BaseURL: srv.URL,
		Model:   "gpt-test",
	})
	if err == nil {
		t.Fatalf("NewProviderWithCapabilityDetection: want error")
	}
	if calls != 1 {
		t.Fatalf("server calls = %d, want 1", calls)
	}
	if !strings.Contains(err.Error(), "invalid API error response") {
		t.Fatalf("err = %v, want invalid API error response", err)
	}
}

func TestNewProvider_Validation(t *testing.T) {
	passwordBaseURL := (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "opaque"),
		Host:   "api.example.test",
		Path:   "/v1",
	}).String()
	rawMarker := "raw-marker"
	tests := []struct {
		name    string
		cfg     Config
		want    string
		wantNot string
	}{
		{name: "missing model", cfg: Config{BaseURL: "https://api.example.test/v1", OutputMode: ports.LLMOutputModeJSONSchema}, want: "model"},
		{name: "malformed credentialed base url does not leak raw input", cfg: Config{BaseURL: "https://operator:" + rawMarker + "@api.example.test/\nv1", Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "parse base url", wantNot: rawMarker},
		{name: "relative base url", cfg: Config{BaseURL: "/v1", Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "absolute"},
		{name: "unsupported base url scheme", cfg: Config{BaseURL: "ftp://api.example.test/v1", Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "scheme"},
		{name: "base url username userinfo", cfg: Config{BaseURL: "https://operator@api.example.test/v1", Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "userinfo"},
		{name: "base url password userinfo", cfg: Config{BaseURL: passwordBaseURL, Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "userinfo"},
		{name: "base url escaped userinfo", cfg: Config{BaseURL: "https://%6fperator@api.example.test/v1", Model: "gpt-test", OutputMode: ports.LLMOutputModeJSONSchema}, want: "userinfo"},
		{name: "unsupported output mode", cfg: Config{BaseURL: "https://api.example.test/v1", Model: "gpt-test", OutputMode: ports.LLMOutputMode("text")}, want: "output mode"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.cfg)
			if err == nil {
				t.Fatalf("NewProvider: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewProvider error = %v, want substring %q", err, tc.want)
			}
			if tc.wantNot != "" && strings.Contains(err.Error(), tc.wantNot) {
				t.Fatalf("NewProvider error = %v, must not contain %q", err, tc.wantNot)
			}
		})
	}
}

func newChatServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/chat/completions", handler)
	mux.Handle("/v1/chat/completions", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func validRequest() ports.LLMRequest {
	return ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: ports.LLMRoleSystem, Content: "Return JSON only."},
			{Role: ports.LLMRoleUser, Content: "Return an object."},
		},
		OutputSchemaID: "test_schema",
		OutputSchema: json.RawMessage(`{
			"type":"object",
			"additionalProperties":false,
			"required":["ok"],
			"properties":{"ok":{"type":"boolean"}}
		}`),
		IdempotencyKey: "req-1",
	}
}

func writeChatResponse(w http.ResponseWriter, content string, refusal *string, finishReason string) {
	w.Header().Set("Content-Type", "application/json")
	message := map[string]any{"role": "assistant", "content": content, "refusal": refusal}
	if refusal != nil {
		message["content"] = ""
	}
	out := map[string]any{
		"id":     "chatcmpl-test",
		"model":  "gpt-test",
		"object": "chat.completion",
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
		"usage": map[string]any{"total_tokens": 42},
	}
	_ = json.NewEncoder(w).Encode(out)
}
