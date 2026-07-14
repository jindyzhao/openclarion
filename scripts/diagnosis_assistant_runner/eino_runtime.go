package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const diagnosisAgentMaxIterations = 1

// einoDiagnosisProvider adapts OpenClarion's strict structured-output provider
// to Eino's proven ChatModelAgent lifecycle. Durable state remains outside the
// process, and no in-container tools are registered in V1.
type einoDiagnosisProvider struct {
	provider ports.StreamingLLMProvider
}

var _ ports.StreamingLLMProvider = (*einoDiagnosisProvider)(nil)

func newEinoDiagnosisProvider(provider ports.StreamingLLMProvider) (*einoDiagnosisProvider, error) {
	if provider == nil {
		return nil, errors.New("diagnosis agent model provider is required")
	}
	return &einoDiagnosisProvider{provider: provider}, nil
}

func (p *einoDiagnosisProvider) GenerateJSON(ctx context.Context, req ports.LLMRequest) (ports.LLMResponse, error) {
	return p.run(ctx, req, nil)
}

func (p *einoDiagnosisProvider) GenerateJSONStreaming(
	ctx context.Context,
	req ports.LLMRequest,
	onDelta ports.LLMStreamHandler,
) (ports.LLMResponse, error) {
	if onDelta == nil {
		return ports.LLMResponse{}, errors.New("diagnosis agent stream callback is required")
	}
	return p.run(ctx, req, onDelta)
}

func (p *einoDiagnosisProvider) run(
	ctx context.Context,
	req ports.LLMRequest,
	onDelta ports.LLMStreamHandler,
) (ports.LLMResponse, error) {
	if p == nil || p.provider == nil {
		return ports.LLMResponse{}, errors.New("diagnosis agent provider is not configured")
	}
	messages, err := einoMessages(req.Messages)
	if err != nil {
		return ports.LLMResponse{}, err
	}
	chatModel := &openClarionEinoModel{
		provider: p.provider,
		request:  cloneLLMRequest(req),
		onDelta:  onDelta,
	}
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "openclarion_diagnosis_assistant",
		Description:   "Analyze bounded alert evidence and produce one diagnosis turn.",
		Model:         chatModel,
		MaxIterations: diagnosisAgentMaxIterations,
	})
	if err != nil {
		return ports.LLMResponse{}, fmt.Errorf("configure diagnosis agent: %w", err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: onDelta != nil,
	})

	iterator := runner.Run(ctx, messages)
	var final *schema.Message
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			return ports.LLMResponse{}, errors.New("diagnosis agent emitted a nil event")
		}
		if event.Err != nil {
			return ports.LLMResponse{}, fmt.Errorf("run diagnosis agent: %w", event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		variant := event.Output.MessageOutput
		if variant.Role != schema.Assistant {
			return ports.LLMResponse{}, fmt.Errorf("diagnosis agent emitted unsupported %q event", variant.Role)
		}
		message, err := variant.GetMessage()
		if err != nil {
			return ports.LLMResponse{}, fmt.Errorf("read diagnosis agent output: %w", err)
		}
		if message == nil {
			return ports.LLMResponse{}, errors.New("diagnosis agent emitted an empty assistant message")
		}
		if final != nil {
			return ports.LLMResponse{}, errors.New("diagnosis agent emitted multiple assistant responses without an approved tool")
		}
		final = message
	}
	if final == nil {
		return ports.LLMResponse{}, errors.New("diagnosis agent completed without an assistant response")
	}
	if final.Role != schema.Assistant || len(final.ToolCalls) > 0 || final.ToolCallID != "" || final.ToolName != "" {
		return ports.LLMResponse{}, errors.New("diagnosis agent attempted an unapproved in-container tool call")
	}
	response, err := chatModel.responseValue()
	if err != nil {
		return ports.LLMResponse{}, err
	}
	if !bytes.Equal(bytes.TrimSpace(response.Content), bytes.TrimSpace([]byte(final.Content))) {
		return ports.LLMResponse{}, errors.New("diagnosis agent output diverged from the validated model response")
	}
	response.Content = json.RawMessage(final.Content)
	return response, nil
}

func einoMessages(messages []ports.LLMMessage) ([]*schema.Message, error) {
	if len(messages) == 0 {
		return nil, errors.New("diagnosis agent messages are required")
	}
	out := make([]*schema.Message, 0, len(messages))
	for index, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			return nil, fmt.Errorf("diagnosis agent message[%d] content is required", index)
		}
		switch message.Role {
		case ports.LLMRoleSystem:
			out = append(out, schema.SystemMessage(message.Content))
		case ports.LLMRoleUser:
			out = append(out, schema.UserMessage(message.Content))
		case ports.LLMRoleAssistant:
			out = append(out, schema.AssistantMessage(message.Content, nil))
		default:
			return nil, fmt.Errorf("diagnosis agent message[%d] role %q is unsupported", index, message.Role)
		}
	}
	return out, nil
}

type openClarionEinoModel struct {
	provider ports.StreamingLLMProvider
	request  ports.LLMRequest
	onDelta  ports.LLMStreamHandler

	mu       sync.Mutex
	response *ports.LLMResponse
}

var _ model.BaseChatModel = (*openClarionEinoModel)(nil)

func (m *openClarionEinoModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	req, err := m.requestForMessages(input)
	if err != nil {
		return nil, err
	}
	response, err := m.provider.GenerateJSON(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := m.recordResponse(response); err != nil {
		return nil, err
	}
	return schema.AssistantMessage(string(response.Content), nil), nil
}

func (m *openClarionEinoModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	if m.onDelta == nil {
		return nil, errors.New("diagnosis agent model stream callback is not configured")
	}
	req, err := m.requestForMessages(input)
	if err != nil {
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](8)
	go func() {
		defer writer.Close()
		response, runErr := m.provider.GenerateJSONStreaming(ctx, req, func(delta ports.LLMStreamDelta) error {
			if err := m.onDelta(delta); err != nil {
				return err
			}
			if writer.Send(schema.AssistantMessage(delta.Delta, nil), nil) {
				return errors.New("diagnosis agent model stream was closed")
			}
			return nil
		})
		if runErr != nil {
			writer.Send(nil, runErr)
			return
		}
		if err := m.recordResponse(response); err != nil {
			writer.Send(nil, err)
		}
	}()
	return reader, nil
}

func (m *openClarionEinoModel) requestForMessages(input []*schema.Message) (ports.LLMRequest, error) {
	if m == nil || m.provider == nil {
		return ports.LLMRequest{}, errors.New("diagnosis agent model is not configured")
	}
	messages := make([]ports.LLMMessage, 0, len(input))
	for index, message := range input {
		if message == nil {
			return ports.LLMRequest{}, fmt.Errorf("diagnosis agent model message[%d] is nil", index)
		}
		if len(message.ToolCalls) > 0 || message.ToolCallID != "" || message.ToolName != "" ||
			len(message.MultiContent) > 0 || len(message.UserInputMultiContent) > 0 || len(message.AssistantGenMultiContent) > 0 {
			return ports.LLMRequest{}, fmt.Errorf("diagnosis agent model message[%d] contains unsupported tool or multimodal content", index)
		}
		var role ports.LLMMessageRole
		switch message.Role {
		case schema.System:
			role = ports.LLMRoleSystem
		case schema.User:
			role = ports.LLMRoleUser
		case schema.Assistant:
			role = ports.LLMRoleAssistant
		default:
			return ports.LLMRequest{}, fmt.Errorf("diagnosis agent model message[%d] role %q is unsupported", index, message.Role)
		}
		messages = append(messages, ports.LLMMessage{Role: role, Content: message.Content})
	}
	req := cloneLLMRequest(m.request)
	req.Messages = messages
	return req, nil
}

func (m *openClarionEinoModel) recordResponse(response ports.LLMResponse) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.response != nil {
		return errors.New("diagnosis agent model was invoked more than once without an approved tool")
	}
	cloned := cloneLLMResponse(response)
	m.response = &cloned
	return nil
}

func (m *openClarionEinoModel) responseValue() (ports.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.response == nil {
		return ports.LLMResponse{}, errors.New("diagnosis agent model response metadata is missing")
	}
	return cloneLLMResponse(*m.response), nil
}

func cloneLLMRequest(req ports.LLMRequest) ports.LLMRequest {
	cloned := req
	cloned.Messages = append([]ports.LLMMessage(nil), req.Messages...)
	cloned.OutputSchema = bytes.Clone(req.OutputSchema)
	return cloned
}

func cloneLLMResponse(response ports.LLMResponse) ports.LLMResponse {
	cloned := response
	cloned.Content = bytes.Clone(response.Content)
	if response.Refusal != nil {
		value := *response.Refusal
		cloned.Refusal = &value
	}
	return cloned
}
