package webhook

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

func validNotification() ports.IMNotification {
	return ports.IMNotification{
		IdempotencyKey: "final_report:42/notification",
		FinalReportID:  42,
		CorrelationKey: "window-1",
		Title:          "Payments degradation",
		Body:           "Scale payments.",
		Severity:       "warning",
	}
}

func TestSendNotification_PostsJSONWithHeaders(t *testing.T) {
	var gotPayload webhookPayload
	var gotIDKey, gotReportID, gotDiagnosisTaskID, gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotIDKey = r.Header.Get(headerIdempotencyKey)
		gotReportID = r.Header.Get(headerReportID)
		gotDiagnosisTaskID = r.Header.Get(headerDiagnosisTaskID)
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message_id":"msg-42","status":"accepted"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL + "/notify#fragment", BearerToken: "test-bearer-value"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	delivery, err := p.SendNotification(context.Background(), validNotification())
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if gotIDKey != "final_report:42/notification" {
		t.Fatalf("idempotency header = %q", gotIDKey)
	}
	if gotReportID != "42" {
		t.Fatalf("report id header = %q", gotReportID)
	}
	if gotDiagnosisTaskID != "" {
		t.Fatalf("diagnosis task id header = %q", gotDiagnosisTaskID)
	}
	if gotAuth != "Bearer test-bearer-value" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotPayload.FinalReportID != 42 || gotPayload.Title != "Payments degradation" || gotPayload.Body != "Scale payments." {
		t.Fatalf("payload = %+v", gotPayload)
	}
	if delivery.ProviderMessageID != "msg-42" || delivery.Status != "accepted" {
		t.Fatalf("delivery = %+v", delivery)
	}
	if string(delivery.Raw) != `{"message_id":"msg-42","status":"accepted"}` {
		t.Fatalf("delivery.Raw = %s", delivery.Raw)
	}
}

func TestSendNotification_PostsWeComTextPayload(t *testing.T) {
	var gotPayload weComPayload
	var gotIDKey, gotReportID, gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotIDKey = r.Header.Get(headerIdempotencyKey)
		gotReportID = r.Header.Get(headerReportID)
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL + "/notify#fragment", Format: "wecom"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	delivery, err := p.SendNotification(context.Background(), validNotification())
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if gotIDKey != "" || gotReportID != "" || gotAuth != "" {
		t.Fatalf("unexpected OpenClarion headers idempotency=%q report=%q auth=%q", gotIDKey, gotReportID, gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotPayload.MsgType != "text" {
		t.Fatalf("msgtype = %q, want text", gotPayload.MsgType)
	}
	wantContent := "Payments degradation\nSeverity: warning\nCorrelation: window-1\nScale payments."
	if gotPayload.Text.Content != wantContent {
		t.Fatalf("content = %q, want %q", gotPayload.Text.Content, wantContent)
	}
	if delivery.ProviderMessageID != "" || delivery.Status != "delivered" {
		t.Fatalf("delivery = %+v", delivery)
	}
	if string(delivery.Raw) != `{"errcode":0,"errmsg":"ok"}` {
		t.Fatalf("delivery.Raw = %s", delivery.Raw)
	}
}

func TestSendNotification_PostsDiagnosisTaskNotification(t *testing.T) {
	var gotPayload webhookPayload
	var gotReportID, gotDiagnosisTaskID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReportID = r.Header.Get(headerReportID)
		gotDiagnosisTaskID = r.Header.Get(headerDiagnosisTaskID)
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	req := validNotification()
	req.IdempotencyKey = "diagnosis_room:7:abc/close_notification"
	req.FinalReportID = 0
	req.DiagnosisTaskID = 7
	req.CorrelationKey = "alert_group:12"
	if _, err := p.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if gotReportID != "" {
		t.Fatalf("report id header = %q", gotReportID)
	}
	if gotDiagnosisTaskID != "7" {
		t.Fatalf("diagnosis task id header = %q", gotDiagnosisTaskID)
	}
	if gotPayload.FinalReportID != 0 || gotPayload.DiagnosisTaskID != 7 || gotPayload.CorrelationKey != "alert_group:12" {
		t.Fatalf("payload = %+v", gotPayload)
	}
}

func TestSendNotification_PostsNotificationChannelTestNotification(t *testing.T) {
	var gotPayload webhookPayload
	var gotReportID, gotDiagnosisTaskID, gotNotificationChannelID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReportID = r.Header.Get(headerReportID)
		gotDiagnosisTaskID = r.Header.Get(headerDiagnosisTaskID)
		gotNotificationChannelID = r.Header.Get(headerNotificationChannelID)
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	req := ports.IMNotification{
		IdempotencyKey:        "notification_channel:9/test",
		NotificationChannelID: 9,
		CorrelationKey:        "notification-channel-test",
		Title:                 "OpenClarion notification channel test",
		Body:                  "This is a test notification from OpenClarion.",
		Severity:              "info",
	}
	if _, err := p.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if gotReportID != "" {
		t.Fatalf("report id header = %q", gotReportID)
	}
	if gotDiagnosisTaskID != "" {
		t.Fatalf("diagnosis task id header = %q", gotDiagnosisTaskID)
	}
	if gotNotificationChannelID != "9" {
		t.Fatalf("notification channel id header = %q", gotNotificationChannelID)
	}
	if gotPayload.FinalReportID != 0 ||
		gotPayload.DiagnosisTaskID != 0 ||
		gotPayload.NotificationChannelID != 9 ||
		gotPayload.CorrelationKey != "notification-channel-test" {
		t.Fatalf("payload = %+v", gotPayload)
	}
}

func TestSendNotification_EmptySuccessBodyDefaultsDelivered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	delivery, err := p.SendNotification(context.Background(), validNotification())
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if delivery.Status != "delivered" {
		t.Fatalf("Status = %q, want delivered", delivery.Status)
	}
}

func TestSendNotification_PreservesUnknownSuccessResponseMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message_id":"msg-42","status":"accepted","provider_trace":"trace-1"}`))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	delivery, err := p.SendNotification(context.Background(), validNotification())
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if delivery.ProviderMessageID != "msg-42" || delivery.Status != "accepted" {
		t.Fatalf("delivery = %+v", delivery)
	}
	if !strings.Contains(string(delivery.Raw), `"provider_trace":"trace-1"`) {
		t.Fatalf("delivery.Raw = %s, want unknown metadata preserved", delivery.Raw)
	}
}

func TestSendNotification_WeComRejectsNonzeroErrCode(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		retryable bool
		want      string
	}{
		{
			name: "invalid bot",
			body: `{"errcode":93000,"errmsg":"invalid bot"}`,
			want: "errcode 93000",
		},
		{
			name:      "rate limited",
			body:      `{"errcode":45009,"errmsg":"rate limited"}`,
			retryable: true,
			want:      "errcode 45009",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p, err := NewProvider(Config{URL: srv.URL, Format: "wecom"})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = p.SendNotification(context.Background(), validNotification())
			if err == nil {
				t.Fatal("SendNotification err = nil, want wecom errcode error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
			var imErr *ports.IMError
			if !errors.As(err, &imErr) {
				t.Fatalf("err = %T %v, want *ports.IMError", err, err)
			}
			if imErr.StatusCode != http.StatusOK {
				t.Fatalf("IMError.StatusCode = %d, want %d", imErr.StatusCode, http.StatusOK)
			}
			if imErr.Retryable != tc.retryable {
				t.Fatalf("IMError.Retryable = %v, want %v", imErr.Retryable, tc.retryable)
			}
		})
	}
}

func TestSendNotification_RejectsAmbiguousSuccessResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate key",
			body: `{"message_id":"old","message_id":"new","status":"accepted"}`,
			want: `duplicate object key "message_id"`,
		},
		{
			name: "trailing value",
			body: `{"message_id":"msg-42","status":"accepted"} {"status":"shadow"}`,
			want: "trailing JSON values",
		},
		{
			name: "non object envelope",
			body: `["msg-42"]`,
			want: "response envelope must be a JSON object",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p, err := NewProvider(Config{URL: srv.URL})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = p.SendNotification(context.Background(), validNotification())
			if err == nil {
				t.Fatal("SendNotification err = nil, want ambiguous response error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
			var imErr *ports.IMError
			if !errors.As(err, &imErr) {
				t.Fatalf("err = %T %v, want *ports.IMError", err, err)
			}
			if imErr.StatusCode != http.StatusOK {
				t.Fatalf("IMError.StatusCode = %d, want %d", imErr.StatusCode, http.StatusOK)
			}
			if imErr.Retryable {
				t.Fatalf("IMError.Retryable = true, want false for malformed 2xx response")
			}
		})
	}
}

func TestSendNotification_WeComRejectsAmbiguousSuccessResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate key",
			body: `{"errcode":0,"errcode":93000,"errmsg":"ok"}`,
			want: `duplicate object key "errcode"`,
		},
		{
			name: "trailing value",
			body: `{"errcode":0,"errmsg":"ok"} {"errcode":93000}`,
			want: "trailing JSON values",
		},
		{
			name: "non object envelope",
			body: `[{"errcode":0}]`,
			want: "response envelope must be a JSON object",
		},
		{
			name: "missing errcode",
			body: `{}`,
			want: "errcode must be present",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p, err := NewProvider(Config{URL: srv.URL, Format: "wecom"})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = p.SendNotification(context.Background(), validNotification())
			if err == nil {
				t.Fatal("SendNotification err = nil, want ambiguous response error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
			var imErr *ports.IMError
			if !errors.As(err, &imErr) {
				t.Fatalf("err = %T %v, want *ports.IMError", err, err)
			}
			if imErr.StatusCode != http.StatusOK {
				t.Fatalf("IMError.StatusCode = %d, want %d", imErr.StatusCode, http.StatusOK)
			}
			if imErr.Retryable {
				t.Fatalf("IMError.Retryable = true, want false for malformed 2xx response")
			}
		})
	}
}

func TestSendNotification_RejectsOversizedResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat(" ", maxBodyBytes+1)))
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.SendNotification(context.Background(), validNotification())
	if err == nil {
		t.Fatal("SendNotification err = nil, want oversized response error")
	}
	var imErr *ports.IMError
	if !errors.As(err, &imErr) {
		t.Fatalf("err = %T %v, want *ports.IMError", err, err)
	}
	if imErr.Retryable {
		t.Fatalf("IMError.Retryable = true, want false for oversized 2xx response")
	}
	if !strings.Contains(imErr.Message, "response body exceeds") {
		t.Fatalf("IMError.Message = %q, want response size error", imErr.Message)
	}
}

func TestSendNotification_PropagatesRequestID(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(correlation.RequestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := NewProvider(Config{URL: srv.URL})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx := correlation.ContextWithRequestID(context.Background(), "request-1")
	if _, err := p.SendNotification(ctx, validNotification()); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if seen != "request-1" {
		t.Fatalf("%s = %q, want request-1", correlation.RequestIDHeader, seen)
	}
}

func TestSendNotification_StatusErrorClassifiesRetryability(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		retryable bool
	}{
		{name: "bad request", status: http.StatusBadRequest, retryable: false},
		{name: "too many requests", status: http.StatusTooManyRequests, retryable: true},
		{name: "server error", status: http.StatusBadGateway, retryable: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream failed", tc.status)
			}))
			defer srv.Close()

			p, err := NewProvider(Config{URL: srv.URL})
			if err != nil {
				t.Fatalf("NewProvider: %v", err)
			}
			_, err = p.SendNotification(context.Background(), validNotification())
			if err == nil {
				t.Fatalf("SendNotification: want error, got nil")
			}
			var imErr *ports.IMError
			if !errors.As(err, &imErr) {
				t.Fatalf("err = %T %v, want *ports.IMError", err, err)
			}
			if imErr.StatusCode != tc.status || imErr.Retryable != tc.retryable {
				t.Fatalf("IMError = %+v, want status=%d retryable=%v", imErr, tc.status, tc.retryable)
			}
		})
	}
}

func TestNewProvider_RejectsInvalidURL(t *testing.T) {
	passwordURL := (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "opaque"),
		Host:   "example.com",
		Path:   "/hook",
	}).String()
	rawMarker := "raw-marker"
	tests := []struct {
		raw     string
		want    string
		wantNot string
	}{
		{raw: "", want: "non-empty"},
		{raw: "://bad", want: "parse"},
		{raw: "https://operator:" + rawMarker + "@example.com/\nhook", want: "parse", wantNot: rawMarker},
		{raw: "/relative", want: "scheme"},
		{raw: "ftp://example.com/hook", want: "scheme"},
		{raw: "https://operator@example.com/hook", want: "userinfo"},
		{raw: passwordURL, want: "userinfo"},
		{raw: "https://%6fperator@example.com/hook", want: "userinfo"},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			_, err := NewProvider(Config{URL: tc.raw})
			if err == nil {
				t.Fatalf("NewProvider(%q): want error, got nil", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewProvider(%q) error = %v, want substring %q", tc.raw, err, tc.want)
			}
			if tc.wantNot != "" && strings.Contains(err.Error(), tc.wantNot) {
				t.Fatalf("NewProvider(%q) error = %v, must not contain %q", tc.raw, err, tc.wantNot)
			}
		})
	}
}

func TestNewProvider_RejectsInvalidFormatConfig(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantSubstr string
	}{
		{
			name:       "unsupported format",
			cfg:        Config{URL: "https://example.invalid/report-hook", Format: "slack"},
			wantSubstr: "unsupported format",
		},
		{
			name: "wecom bearer token",
			cfg: Config{
				URL:         "https://example.invalid/report-hook",
				Format:      "wecom",
				BearerToken: "test-bearer-value",
			},
			wantSubstr: "bearer token is unsupported",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.cfg)
			if err == nil {
				t.Fatal("NewProvider err = nil, want config error")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantSubstr)
			}
			if strings.Contains(err.Error(), "test-bearer-value") {
				t.Fatalf("NewProvider error leaked bearer token: %v", err)
			}
		})
	}
}

func TestSendNotification_RejectsInvalidRequest(t *testing.T) {
	p, err := NewProvider(Config{URL: "https://example.invalid/hook"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	tests := []struct {
		name string
		edit func(*ports.IMNotification)
	}{
		{name: "empty idempotency", edit: func(r *ports.IMNotification) { r.IdempotencyKey = "" }},
		{name: "zero subject ids", edit: func(r *ports.IMNotification) {
			r.FinalReportID = 0
			r.DiagnosisTaskID = 0
		}},
		{name: "empty title", edit: func(r *ports.IMNotification) { r.Title = " " }},
		{name: "empty body", edit: func(r *ports.IMNotification) { r.Body = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validNotification()
			tc.edit(&req)
			if _, err := p.SendNotification(context.Background(), req); err == nil {
				t.Fatalf("SendNotification: want validation error, got nil")
			}
		})
	}
}

func TestSendNotification_ContextCanceledIsRetryableIMError(t *testing.T) {
	p, err := NewProvider(Config{URL: "https://example.invalid/hook"})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.SendNotification(ctx, validNotification())
	if err == nil {
		t.Fatalf("SendNotification: want error, got nil")
	}
	var imErr *ports.IMError
	if !errors.As(err, &imErr) {
		t.Fatalf("err = %T %v, want *ports.IMError", err, err)
	}
	if !imErr.Retryable {
		t.Fatalf("IMError.Retryable = false, want true")
	}
}
