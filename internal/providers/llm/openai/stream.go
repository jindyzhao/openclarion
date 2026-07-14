package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type chatCompletionChunk struct {
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Index        int                       `json:"index"`
	Delta        chatCompletionChoiceDelta `json:"delta"`
	FinishReason string                    `json:"finish_reason"`
}

type chatCompletionChoiceDelta struct {
	Content string  `json:"content"`
	Refusal *string `json:"refusal"`
}

type chatCompletionAccumulator struct {
	content      strings.Builder
	refusal      strings.Builder
	model        string
	finishReason string
	sequence     int
	seenChoice   bool
	done         bool
	onDelta      ports.LLMStreamHandler
}

// GenerateJSONStreaming streams raw structured-output content deltas and then
// returns the same provider-neutral final response shape as GenerateJSON.
func (p *Provider) GenerateJSONStreaming(
	ctx context.Context,
	req ports.LLMRequest,
	onDelta ports.LLMStreamHandler,
) (ports.LLMResponse, error) {
	if p == nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: provider is nil")
	}
	if onDelta == nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: stream callback must be non-nil")
	}
	if req.IdempotencyKey == "" {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: idempotency key must be non-empty")
	}
	body, err := p.buildRequest(req)
	if err != nil {
		return ports.LLMResponse{}, err
	}
	body.Stream = true
	return p.postStreaming(ctx, body, req.IdempotencyKey, onDelta)
}

func (p *Provider) postStreaming(
	ctx context.Context,
	body chatCompletionRequest,
	idempotencyKey string,
	onDelta ports.LLMStreamHandler,
) (ports.LLMResponse, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: marshal streaming request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: build streaming request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-Client-Request-Id", idempotencyKey)
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: post streaming chat completion: %w", redactHTTPClientError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr := apiStatusError(resp)
		if isUnsupportedStreaming(statusErr) {
			return ports.LLMResponse{}, fmt.Errorf(
				"openai llm: streaming capability rejected: %w",
				errors.Join(ports.ErrLLMStreamingUnsupported, statusErr),
			)
		}
		return ports.LLMResponse{}, statusErr
	}
	if contentType := strings.ToLower(resp.Header.Get("Content-Type")); contentType != "" && !strings.HasPrefix(contentType, "text/event-stream") {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: streaming response content type %q is not text/event-stream", contentType)
	}

	accumulator := &chatCompletionAccumulator{onDelta: onDelta}
	if err := decodeChatCompletionStream(resp.Body, accumulator.accept); err != nil {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: decode streaming response: %w", err)
	}
	if !accumulator.done {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: streaming response ended before [DONE]")
	}
	if !accumulator.seenChoice {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: streaming response has no choices")
	}
	if accumulator.finishReason == "" {
		return ports.LLMResponse{}, fmt.Errorf("openai llm: streaming response has no finish reason")
	}
	var refusal *string
	if accumulator.refusal.Len() > 0 {
		value := accumulator.refusal.String()
		refusal = &value
	}
	return ports.LLMResponse{
		Content:      json.RawMessage(accumulator.content.String()),
		FinishReason: accumulator.finishReason,
		Refusal:      refusal,
		OutputMode:   p.outputMode,
		Model:        accumulator.model,
	}, nil
}

func isUnsupportedStreaming(err error) bool {
	var status *statusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.StatusCode {
	case http.StatusBadRequest,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity,
		http.StatusNotImplemented:
	default:
		return false
	}
	if strings.EqualFold(strings.TrimSpace(status.Param), "stream") {
		return true
	}
	text := strings.ToLower(strings.Join([]string{status.Message, status.Type, status.Code}, " "))
	if !strings.Contains(text, "stream") {
		return false
	}
	return strings.Contains(text, "unsupported") ||
		strings.Contains(text, "not support") ||
		strings.Contains(text, "not available") ||
		strings.Contains(text, "not implemented") ||
		strings.Contains(text, "invalid parameter")
}

func (a *chatCompletionAccumulator) accept(raw []byte) error {
	if a.done {
		return fmt.Errorf("streaming response included data after [DONE]")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
		a.done = true
		return nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return err
	}
	var chunk chatCompletionChunk
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return err
	}
	if chunk.Model != "" {
		if a.model != "" && a.model != chunk.Model {
			return fmt.Errorf("streaming response model changed from %q to %q", a.model, chunk.Model)
		}
		a.model = chunk.Model
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			return fmt.Errorf("streaming response choice index %d is unsupported", choice.Index)
		}
		a.seenChoice = true
		if choice.Delta.Refusal != nil {
			a.refusal.WriteString(*choice.Delta.Refusal)
		}
		if choice.Delta.Content != "" {
			if a.content.Len()+len([]byte(choice.Delta.Content)) > maxResponseBody {
				return fmt.Errorf("chat completion response exceeds %d bytes", maxResponseBody)
			}
			a.content.WriteString(choice.Delta.Content)
			a.sequence++
			if err := a.onDelta(ports.LLMStreamDelta{Sequence: a.sequence, Delta: choice.Delta.Content}); err != nil {
				return fmt.Errorf("stream callback: %w", err)
			}
		}
		if choice.FinishReason != "" {
			if a.finishReason != "" && a.finishReason != choice.FinishReason {
				return fmt.Errorf("streaming response finish reason changed from %q to %q", a.finishReason, choice.FinishReason)
			}
			a.finishReason = choice.FinishReason
		}
	}
	return nil
}

func decodeChatCompletionStream(body io.Reader, accept func([]byte) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxResponseBody)
	var dataLines []string
	totalBytes := 0
	flush := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := []byte(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		return accept(data)
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		totalBytes += len(line) + 1
		if totalBytes > maxResponseBody {
			return fmt.Errorf("chat completion stream exceeds %d bytes", maxResponseBody)
		}
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		dataLines = append(dataLines, data)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}
