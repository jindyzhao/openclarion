package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestEinoDiagnosisProviderPreservesMultiTurnRequest(t *testing.T) {
	upstream := &recordingProvider{response: ports.LLMResponse{
		Content:      json.RawMessage(`{"message":"继续检查 api-1。"}`),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "test-model",
	}}
	provider, err := newEinoDiagnosisProvider(upstream)
	if err != nil {
		t.Fatal(err)
	}
	req := ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: ports.LLMRoleSystem, Content: "Return strict JSON."},
			{Role: ports.LLMRoleUser, Content: "Evidence: CPU is saturated."},
			{Role: ports.LLMRoleUser, Content: "What happened?"},
			{Role: ports.LLMRoleAssistant, Content: "I am checking api-1."},
			{Role: ports.LLMRoleUser, Content: "请继续。"},
		},
		OutputSchema:   json.RawMessage(`{"type":"object"}`),
		OutputSchemaID: "diagnosis_turn_v1",
		IdempotencyKey: "diagnosis-turn:test",
	}
	response, err := provider.GenerateJSON(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	if response.Model != "test-model" || response.FinishReason != "stop" || response.OutputMode != ports.LLMOutputModeJSONSchema {
		t.Fatalf("response metadata = %+v", response)
	}
	requests := upstream.requestValues()
	if len(requests) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(requests))
	}
	if !reflect.DeepEqual(requests[0].Messages, req.Messages) {
		t.Fatalf("upstream messages = %#v, want %#v", requests[0].Messages, req.Messages)
	}
	if requests[0].OutputSchemaID != req.OutputSchemaID ||
		requests[0].IdempotencyKey != req.IdempotencyKey ||
		string(requests[0].OutputSchema) != string(req.OutputSchema) {
		t.Fatalf("upstream request contract = %+v", requests[0])
	}
}

func TestEinoDiagnosisProviderRejectsUnsupportedRoleBeforeModel(t *testing.T) {
	upstream := &recordingProvider{}
	provider, err := newEinoDiagnosisProvider(upstream)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateJSON(context.Background(), ports.LLMRequest{
		Messages: []ports.LLMMessage{{Role: "tool", Content: "unapproved"}},
	})
	if err == nil || !strings.Contains(err.Error(), `role "tool" is unsupported`) {
		t.Fatalf("GenerateJSON error = %v", err)
	}
	if len(upstream.requestValues()) != 0 {
		t.Fatal("unsupported input reached the upstream model")
	}
}

func TestEinoDiagnosisProviderPropagatesContextCancellation(t *testing.T) {
	provider, err := newEinoDiagnosisProvider(cancelingProvider{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.GenerateJSON(ctx, validAgentRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GenerateJSON error = %v, want context cancellation", err)
	}
}

func TestOpenClarionEinoModelStreamProvidesOneCompleteMessage(t *testing.T) {
	upstream := &recordingProvider{response: ports.LLMResponse{
		Content:      json.RawMessage(`{"message":"complete"}`),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
	}}
	model := &openClarionEinoModel{provider: upstream, request: validAgentRequest()}
	reader, err := model.Stream(context.Background(), []*schema.Message{schema.UserMessage("Diagnose.")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer reader.Close()
	message, err := reader.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if message.Content != `{"message":"complete"}` {
		t.Fatalf("message = %+v", message)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("second Recv error = %v, want EOF", err)
	}
}

func validAgentRequest() ports.LLMRequest {
	return ports.LLMRequest{
		Messages:       []ports.LLMMessage{{Role: ports.LLMRoleUser, Content: "Diagnose."}},
		OutputSchema:   json.RawMessage(`{"type":"object"}`),
		OutputSchemaID: "diagnosis_turn_v1",
		IdempotencyKey: "diagnosis-turn:test",
	}
}

type recordingProvider struct {
	mu       sync.Mutex
	requests []ports.LLMRequest
	response ports.LLMResponse
}

func (p *recordingProvider) GenerateJSON(_ context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, cloneLLMRequest(req))
	return cloneLLMResponse(p.response), nil
}

func (p *recordingProvider) requestValues() []ports.LLMRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ports.LLMRequest, len(p.requests))
	for index := range p.requests {
		out[index] = cloneLLMRequest(p.requests[index])
	}
	return out
}

type cancelingProvider struct{}

func (cancelingProvider) GenerateJSON(ctx context.Context, _ ports.LLMRequest) (ports.LLMResponse, error) {
	<-ctx.Done()
	return ports.LLMResponse{}, ctx.Err()
}
