// Package webhook provides an HTTP Webhook implementation of
// ports.IMProvider.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout = 10 * time.Second
	maxBodyBytes   = 1 << 20

	headerIdempotencyKey  = "X-OpenClarion-Idempotency-Key"
	headerReportID        = "X-OpenClarion-Final-Report-Id"
	headerDiagnosisTaskID = "X-OpenClarion-Diagnosis-Task-Id"
)

// Config holds Webhook provider configuration.
type Config struct {
	URL         string
	BearerToken string
	HTTPClient  *http.Client
}

// Provider sends IM notifications as JSON HTTP POST requests. It is
// safe for concurrent use when the configured HTTPClient is safe for
// concurrent use.
type Provider struct {
	endpoint    string
	bearerToken string
	httpClient  *http.Client
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.IMProvider = (*Provider)(nil)

// NewProvider constructs a Webhook provider.
func NewProvider(cfg Config) (*Provider, error) {
	endpoint, err := normalizeEndpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	return &Provider{
		endpoint:    endpoint,
		bearerToken: strings.TrimSpace(cfg.BearerToken),
		httpClient:  client,
	}, nil
}

// SendNotification POSTs one JSON notification to the configured
// webhook endpoint. A 2xx response is success; 5xx/429 are retryable;
// other non-2xx statuses are non-retryable provider errors.
func (p *Provider) SendNotification(ctx context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	if p == nil {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: provider is nil")
	}
	if err := validateNotification(req); err != nil {
		return ports.IMDelivery{}, err
	}
	payload := webhookPayload{
		IdempotencyKey:  req.IdempotencyKey,
		FinalReportID:   req.FinalReportID,
		DiagnosisTaskID: req.DiagnosisTaskID,
		CorrelationKey:  req.CorrelationKey,
		Title:           req.Title,
		Body:            req.Body,
		Severity:        req.Severity,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(headerIdempotencyKey, req.IdempotencyKey)
	if req.FinalReportID != 0 {
		httpReq.Header.Set(headerReportID, fmt.Sprintf("%d", req.FinalReportID))
	}
	if req.DiagnosisTaskID != 0 {
		httpReq.Header.Set(headerDiagnosisTaskID, fmt.Sprintf("%d", req.DiagnosisTaskID))
	}
	if p.bearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.IMDelivery{}, &ports.IMError{
			Message:   fmt.Sprintf("webhook im: post notification: %v", err),
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	rawBody, err := readResponseBody(resp.Body)
	if err != nil {
		retryable := true
		var tooLarge responseBodyTooLargeError
		if errors.As(err, &tooLarge) {
			retryable = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		}
		return ports.IMDelivery{}, &ports.IMError{
			Message:    fmt.Sprintf("webhook im: read response: %v", err),
			StatusCode: resp.StatusCode,
			Retryable:  retryable,
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.IMDelivery{}, statusError(resp.StatusCode, rawBody)
	}
	delivery, err := deliveryFromResponse(rawBody)
	if err != nil {
		return ports.IMDelivery{}, &ports.IMError{
			Message:    err.Error(),
			StatusCode: resp.StatusCode,
			Retryable:  false,
		}
	}
	return delivery, nil
}

type responseBodyTooLargeError struct {
	limit int
}

func (e responseBodyTooLargeError) Error() string {
	return fmt.Sprintf("response body exceeds %d bytes", e.limit)
}

func readResponseBody(body io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: maxBodyBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBodyBytes {
		return nil, responseBodyTooLargeError{limit: maxBodyBytes}
	}
	return raw, nil
}

func normalizeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("webhook im: url must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("webhook im: parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("webhook im: url scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("webhook im: url must be absolute")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func validateNotification(req ports.IMNotification) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("webhook im: idempotency key must be non-empty")
	}
	if req.FinalReportID == 0 && req.DiagnosisTaskID == 0 {
		return fmt.Errorf("webhook im: notification subject id must be non-zero")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("webhook im: title must be non-empty")
	}
	if strings.TrimSpace(req.Body) == "" {
		return fmt.Errorf("webhook im: body must be non-empty")
	}
	return nil
}

func statusError(statusCode int, raw []byte) error {
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		msg = fmt.Sprintf("webhook im: status %d", statusCode)
	} else {
		msg = fmt.Sprintf("webhook im: status %d: %s", statusCode, msg)
	}
	return &ports.IMError{
		Message:    msg,
		StatusCode: statusCode,
		Retryable:  statusCode == http.StatusTooManyRequests || statusCode >= 500,
	}
}

func deliveryFromResponse(raw []byte) (ports.IMDelivery, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ports.IMDelivery{Status: "delivered"}, nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(trimmed); err != nil {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: decode response: %w", err)
	}
	if trimmed[0] != '{' {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: decode response: response envelope must be a JSON object")
	}
	var out webhookResponse
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return ports.IMDelivery{}, fmt.Errorf("webhook im: decode response: %w", err)
	}
	status := strings.TrimSpace(out.Status)
	if status == "" {
		status = "delivered"
	}
	rawCopy := make(json.RawMessage, len(trimmed))
	copy(rawCopy, trimmed)
	return ports.IMDelivery{
		ProviderMessageID: out.MessageID,
		Status:            status,
		Raw:               rawCopy,
	}, nil
}

type webhookPayload struct {
	IdempotencyKey  string `json:"idempotency_key"`
	FinalReportID   int64  `json:"final_report_id,omitempty"`
	DiagnosisTaskID int64  `json:"diagnosis_task_id,omitempty"`
	CorrelationKey  string `json:"correlation_key,omitempty"`
	Title           string `json:"title"`
	Body            string `json:"body"`
	Severity        string `json:"severity,omitempty"`
}

type webhookResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}
