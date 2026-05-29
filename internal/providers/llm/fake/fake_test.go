package fake

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const testKey = "snapshot-11/group-0"

func responseFor(content string) ports.LLMResponse {
	return ports.LLMResponse{
		Content:      json.RawMessage(content),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "fake-llm",
	}
}

func requestFor(key string) ports.LLMRequest {
	return ports.LLMRequest{
		IdempotencyKey: key,
		Messages: []ports.LLMMessage{{
			Role:    ports.LLMRoleUser,
			Content: "summarise",
		}},
		OutputSchemaID: "sub_report",
		OutputSchema:   json.RawMessage(`{"type":"object"}`),
	}
}

func TestNew_DeepCopiesScript_PreventsPostConstructPollution(t *testing.T) {
	refusal := "refused"
	scripts := map[string][]Result{
		testKey: {{
			Response: ports.LLMResponse{
				Content:      json.RawMessage(`{"summary":"original"}`),
				FinishReason: "stop",
				Refusal:      &refusal,
				OutputMode:   ports.LLMOutputModeJSONSchema,
				Model:        "fake-llm",
			},
		}},
	}
	p := New(scripts)

	scripts[testKey][0].Response.Content[12] = 'X'
	*scripts[testKey][0].Response.Refusal = "mutated"
	scripts[testKey] = append(scripts[testKey], Result{Response: responseFor(`{"summary":"extra"}`)})

	got, err := p.GenerateJSON(context.Background(), requestFor(testKey))
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if string(got.Content) != `{"summary":"original"}` {
		t.Fatalf("Content = %s, want original", got.Content)
	}
	if got.Refusal == nil || *got.Refusal != "refused" {
		t.Fatalf("Refusal = %v, want refused", got.Refusal)
	}
}

func TestGenerateJSON_DeepCopiesReturn_PreventsConsumerPollution(t *testing.T) {
	refusal := "initial"
	p := New(map[string][]Result{
		testKey: {{
			Response: ports.LLMResponse{
				Content:      json.RawMessage(`{"summary":"stable"}`),
				FinishReason: "stop",
				Refusal:      &refusal,
				OutputMode:   ports.LLMOutputModeJSONSchema,
				Model:        "fake-llm",
			},
		}},
	})

	first, err := p.GenerateJSON(context.Background(), requestFor(testKey))
	if err != nil {
		t.Fatalf("first GenerateJSON: %v", err)
	}
	first.Content[12] = 'X'
	*first.Refusal = "mutated"

	second, err := p.GenerateJSON(context.Background(), requestFor(testKey))
	if err != nil {
		t.Fatalf("second GenerateJSON: %v", err)
	}
	if string(second.Content) != `{"summary":"stable"}` {
		t.Fatalf("second Content = %s, want stable", second.Content)
	}
	if second.Refusal == nil || *second.Refusal != "initial" {
		t.Fatalf("second Refusal = %v, want initial", second.Refusal)
	}
}

func TestGenerateJSON_ScriptedByIdempotencyKey_RepeatsLastResult(t *testing.T) {
	p := New(map[string][]Result{
		testKey: {
			{Response: responseFor(`{"summary":"first"}`)},
			{Response: responseFor(`{"summary":"second"}`)},
		},
		"snapshot-11/group-1": {
			{Response: responseFor(`{"summary":"other"}`)},
		},
	})

	ctx := context.Background()
	first, err := p.GenerateJSON(ctx, requestFor(testKey))
	if err != nil {
		t.Fatalf("first GenerateJSON: %v", err)
	}
	second, err := p.GenerateJSON(ctx, requestFor(testKey))
	if err != nil {
		t.Fatalf("second GenerateJSON: %v", err)
	}
	third, err := p.GenerateJSON(ctx, requestFor(testKey))
	if err != nil {
		t.Fatalf("third GenerateJSON: %v", err)
	}
	other, err := p.GenerateJSON(ctx, requestFor("snapshot-11/group-1"))
	if err != nil {
		t.Fatalf("other GenerateJSON: %v", err)
	}

	if string(first.Content) != `{"summary":"first"}` {
		t.Fatalf("first Content = %s", first.Content)
	}
	if string(second.Content) != `{"summary":"second"}` {
		t.Fatalf("second Content = %s", second.Content)
	}
	if string(third.Content) != `{"summary":"second"}` {
		t.Fatalf("third Content = %s, want repeated last result", third.Content)
	}
	if string(other.Content) != `{"summary":"other"}` {
		t.Fatalf("other Content = %s", other.Content)
	}
	if p.Calls(testKey) != 3 {
		t.Fatalf("Calls(%q) = %d, want 3", testKey, p.Calls(testKey))
	}
}

func TestGenerateJSON_ReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("provider unavailable")
	p := New(map[string][]Result{
		testKey: {{Err: wantErr}},
	})

	_, err := p.GenerateJSON(context.Background(), requestFor(testKey))
	if !errors.Is(err, wantErr) {
		t.Fatalf("GenerateJSON err = %v, want %v", err, wantErr)
	}
}

func TestGenerateJSON_RejectsMissingOrUnknownIdempotencyKey(t *testing.T) {
	p := New(map[string][]Result{
		testKey: {{Response: responseFor(`{"summary":"ok"}`)}},
	})

	for _, key := range []string{"", "unknown"} {
		t.Run(key, func(t *testing.T) {
			_, err := p.GenerateJSON(context.Background(), requestFor(key))
			if err == nil {
				t.Fatal("GenerateJSON err = nil, want error")
			}
		})
	}
}

func TestGenerateJSON_HonoursCancelledContext(t *testing.T) {
	p := New(map[string][]Result{
		testKey: {{Response: responseFor(`{"summary":"ok"}`)}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.GenerateJSON(ctx, requestFor(testKey))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateJSON err = %v, want context.Canceled", err)
	}
}
