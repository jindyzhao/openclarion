package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestGenerateJSONStreamingAccumulatesOrderedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("X-Client-Request-Id") != "stream-1" {
			t.Errorf("headers = %#v", r.Header)
		}
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !request.Stream {
			t.Error("stream = false, want true")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"model":"test-model","choices":[{"index":0,"delta":{"content":"{\"message\":\"CPU "},"finish_reason":""}]}`,
			`{"model":"test-model","choices":[{"index":0,"delta":{"content":"high\"}"},"finish_reason":""}]}`,
			`{"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`[DONE]`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(Config{
		BaseURL:    server.URL,
		Model:      "test-model",
		OutputMode: ports.LLMOutputModeJSONObject,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []ports.LLMStreamDelta
	response, err := provider.GenerateJSONStreaming(context.Background(), ports.LLMRequest{
		Messages:       []ports.LLMMessage{{Role: ports.LLMRoleUser, Content: "diagnose"}},
		OutputSchemaID: "diagnosis_turn_v1",
		OutputSchema:   json.RawMessage(`{"type":"object"}`),
		IdempotencyKey: "stream-1",
	}, func(delta ports.LLMStreamDelta) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("GenerateJSONStreaming: %v", err)
	}
	if string(response.Content) != `{"message":"CPU high"}` || response.FinishReason != "stop" || response.Model != "test-model" {
		t.Fatalf("response = %+v", response)
	}
	if len(deltas) != 2 || deltas[0].Sequence != 1 || deltas[1].Sequence != 2 {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestGenerateJSONStreamingPropagatesCallbackAndProtocolErrors(t *testing.T) {
	tests := []struct {
		name        string
		events      []string
		callbackErr error
		want        string
	}{
		{
			name:        "callback",
			events:      []string{`{"model":"m","choices":[{"index":0,"delta":{"content":"x"},"finish_reason":""}]}`},
			callbackErr: errors.New("stop preview"),
			want:        "stop preview",
		},
		{
			name:   "duplicate",
			events: []string{`{"model":"m","model":"other","choices":[]}`},
			want:   "duplicate object key",
		},
		{
			name: "missing done",
			events: []string{
				`{"model":"m","choices":[{"index":0,"delta":{"content":"x"},"finish_reason":""}]}`,
				`{"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			},
			want: "before [DONE]",
		},
		{
			name: "data after done",
			events: []string{
				`[DONE]`,
				`{"model":"m","choices":[]}`,
			},
			want: "after [DONE]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, event := range tt.events {
					fmt.Fprintf(w, "data: %s\n\n", event)
				}
			}))
			defer server.Close()
			provider, err := NewProvider(Config{BaseURL: server.URL, Model: "m", OutputMode: ports.LLMOutputModeJSONObject, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.GenerateJSONStreaming(context.Background(), ports.LLMRequest{
				Messages: []ports.LLMMessage{{Role: ports.LLMRoleUser, Content: "x"}}, OutputSchemaID: "s", OutputSchema: json.RawMessage(`{"type":"object"}`), IdempotencyKey: "id",
			}, func(ports.LLMStreamDelta) error { return tt.callbackErr })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
