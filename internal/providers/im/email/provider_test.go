package email

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestConfigFromURLParsesSMTPSecretURL(t *testing.T) {
	cfg, err := ConfigFromURL("smtp://user:pass@smtp.example.test:587?from=alerts%40example.test&to=ops%40example.test&to=oncall%40example.test")
	if err != nil {
		t.Fatalf("ConfigFromURL: %v", err)
	}
	if cfg.ServerAddr != "smtp.example.test:587" || cfg.ServerName != "smtp.example.test" {
		t.Fatalf("server = %q/%q", cfg.ServerAddr, cfg.ServerName)
	}
	if cfg.Username != "user" || cfg.Password != "pass" {
		t.Fatalf("auth = %q/%q", cfg.Username, cfg.Password)
	}
	if cfg.TLSMode != tlsModeStartTLS {
		t.Fatalf("TLSMode = %q, want %q", cfg.TLSMode, tlsModeStartTLS)
	}
	if cfg.From.Address != "alerts@example.test" || len(cfg.To) != 2 || cfg.To[1].Address != "oncall@example.test" {
		t.Fatalf("addresses from=%+v to=%+v", cfg.From, cfg.To)
	}
}

func TestConfigFromURLParsesSMTPSDefaultPort(t *testing.T) {
	cfg, err := ConfigFromURL("smtps://user:pass@smtp.example.test?from=alerts%40example.test&to=ops%40example.test")
	if err != nil {
		t.Fatalf("ConfigFromURL: %v", err)
	}
	if cfg.ServerAddr != "smtp.example.test:465" || cfg.TLSMode != tlsModeImplicit {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromURLRejectsInvalidSecretURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "non-empty"},
		{name: "whitespace", raw: "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test\n", want: "control"},
		{name: "bad scheme", raw: "http://smtp.example.test?from=alerts%40example.test&to=ops%40example.test", want: "scheme"},
		{name: "missing host", raw: "smtp://?from=alerts%40example.test&to=ops%40example.test", want: "host"},
		{name: "missing from", raw: "smtp://smtp.example.test?to=ops%40example.test", want: "from"},
		{name: "missing to", raw: "smtp://smtp.example.test?from=alerts%40example.test", want: "to"},
		{name: "unknown query", raw: "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&debug=true", want: "unsupported"},
		{name: "bad starttls", raw: "smtp://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&starttls=maybe", want: "starttls"},
		{name: "starttls on smtps", raw: "smtps://smtp.example.test?from=alerts%40example.test&to=ops%40example.test&starttls=required", want: "smtps"},
		{name: "missing password", raw: "smtp://user@smtp.example.test?from=alerts%40example.test&to=ops%40example.test", want: "password"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConfigFromURL(tc.raw)
			if err == nil {
				t.Fatal("ConfigFromURL err = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestSendNotificationRendersEmailAndReturnsDelivery(t *testing.T) {
	sender := &recordingSender{}
	provider := mustProvider(t, Config{
		ServerAddr: "smtp.example.test:25",
		ServerName: "smtp.example.test",
		From:       mail.Address{Name: "OpenClarion", Address: "alerts@example.test"},
		To:         []mail.Address{{Address: "ops@example.test"}},
		TLSMode:    tlsModeNone,
		Sender:     sender,
	})

	delivery, err := provider.SendNotification(context.Background(), validNotification())
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if delivery.Status != "delivered" || string(delivery.Raw) != `{"provider":"email","recipient_count":1}` {
		t.Fatalf("delivery = %+v", delivery)
	}
	raw := string(sender.req.Message)
	for _, want := range []string{
		"From: \"OpenClarion\" <alerts@example.test>\r\n",
		"To: <ops@example.test>\r\n",
		"Subject: Payments degradation\r\n",
		"X-OpenClarion-Idempotency-Key: final_report:42/notification\r\n",
		"X-OpenClarion-Final-Report-Id: 42\r\n",
		"Payments degradation\r\nSeverity: warning\r\nCorrelation: window-1\r\nScale payments.",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("message missing %q:\n%s", want, raw)
		}
	}
}

func TestSendNotificationSanitizesSubjectHeader(t *testing.T) {
	sender := &recordingSender{}
	provider := mustProvider(t, Config{
		ServerAddr: "smtp.example.test:25",
		ServerName: "smtp.example.test",
		From:       mail.Address{Address: "alerts@example.test"},
		To:         []mail.Address{{Address: "ops@example.test"}},
		TLSMode:    tlsModeNone,
		Sender:     sender,
	})
	req := validNotification()
	req.Title = "Payments degraded\r\nBcc: attacker@example.test"
	if _, err := provider.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	raw := string(sender.req.Message)
	headers := raw[:strings.Index(raw, "\r\n\r\n")]
	if strings.Contains(headers, "\r\nBcc: attacker@example.test") {
		t.Fatalf("message headers contain injected Bcc header:\n%s", raw)
	}
	if !strings.Contains(headers, "Subject: Payments degraded Bcc: attacker@example.test") {
		t.Fatalf("message headers did not sanitize subject as a single line:\n%s", raw)
	}
}

func TestSendNotificationTruncatesBodyAtUTF8Boundary(t *testing.T) {
	sender := &recordingSender{}
	provider := mustProvider(t, Config{
		ServerAddr: "smtp.example.test:25",
		ServerName: "smtp.example.test",
		From:       mail.Address{Address: "alerts@example.test"},
		To:         []mail.Address{{Address: "ops@example.test"}},
		TLSMode:    tlsModeNone,
		Sender:     sender,
	})
	req := validNotification()
	req.Body = strings.Repeat(string(rune(0x1F680)), maxBodyBytes)
	if _, err := provider.SendNotification(context.Background(), req); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	raw := string(sender.req.Message)
	body := raw[strings.Index(raw, "\r\n\r\n")+4:]
	if len(body) > maxBodyBytes+64 {
		t.Fatalf("body length = %d, want bounded", len(body))
	}
	if !utf8.ValidString(body) {
		t.Fatal("body is not valid UTF-8")
	}
	if !strings.Contains(body, truncationSuffix) {
		t.Fatalf("body missing truncation suffix")
	}
}

func TestSendNotificationPropagatesIMError(t *testing.T) {
	wantErr := &ports.IMError{Message: "smtp rejected recipient", Retryable: false}
	provider := mustProvider(t, Config{
		ServerAddr: "smtp.example.test:25",
		ServerName: "smtp.example.test",
		From:       mail.Address{Address: "alerts@example.test"},
		To:         []mail.Address{{Address: "ops@example.test"}},
		TLSMode:    tlsModeNone,
		Sender:     recordingSenderFunc(func(context.Context, SendRequest) error { return wantErr }),
	})
	_, err := provider.SendNotification(context.Background(), validNotification())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestSendNotificationClassifiesSMTPStatusErrors(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		retryable bool
	}{
		{name: "temporary server reject", code: 421, retryable: true},
		{name: "auth reject", code: 535, retryable: false},
		{name: "recipient reject", code: 550, retryable: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := mustProvider(t, Config{
				ServerAddr: "smtp.example.test:25",
				ServerName: "smtp.example.test",
				From:       mail.Address{Address: "alerts@example.test"},
				To:         []mail.Address{{Address: "ops@example.test"}},
				TLSMode:    tlsModeNone,
				Sender: recordingSenderFunc(func(context.Context, SendRequest) error {
					return fmt.Errorf("smtp command failed: %w", &textproto.Error{Code: tc.code, Msg: "smtp fixture"})
				}),
			})

			_, err := provider.SendNotification(context.Background(), validNotification())
			if err == nil {
				t.Fatal("SendNotification err = nil, want SMTP status error")
			}
			var imErr *ports.IMError
			if !errors.As(err, &imErr) {
				t.Fatalf("err = %T %v, want *ports.IMError", err, err)
			}
			if imErr.Retryable != tc.retryable {
				t.Fatalf("IMError.Retryable = %v, want %v", imErr.Retryable, tc.retryable)
			}
		})
	}
}

func TestNewProviderFromURLSendsThroughSMTPServer(t *testing.T) {
	addr, messages := startFakeSMTPServer(t)
	provider, err := NewProviderFromURL("smtp://" + addr + "?from=alerts%40example.test&to=ops%40example.test&starttls=disabled")
	if err != nil {
		t.Fatalf("NewProviderFromURL: %v", err)
	}
	if _, err := provider.SendNotification(context.Background(), validNotification()); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	raw := <-messages
	if !strings.Contains(raw, "Subject: Payments degradation\r\n") ||
		!strings.Contains(raw, "Scale payments.") {
		t.Fatalf("smtp message =\n%s", raw)
	}
}

func mustProvider(t *testing.T, cfg Config) *Provider {
	t.Helper()
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return provider
}

type recordingSender struct {
	req SendRequest
}

func (s *recordingSender) SendEmail(_ context.Context, req SendRequest) error {
	s.req = req
	return nil
}

type recordingSenderFunc func(context.Context, SendRequest) error

func (f recordingSenderFunc) SendEmail(ctx context.Context, req SendRequest) error {
	return f(ctx, req)
}

func startFakeSMTPServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	var listenConfig net.ListenConfig
	ln, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	messages := make(chan string, 1)
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
	})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		writeSMTPLine(t, rw, "220 localhost ESMTP")
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			command := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				writeSMTPLine(t, rw, "250-localhost")
				writeSMTPLine(t, rw, "250 OK")
			case strings.HasPrefix(command, "MAIL FROM:"):
				writeSMTPLine(t, rw, "250 OK")
			case strings.HasPrefix(command, "RCPT TO:"):
				writeSMTPLine(t, rw, "250 OK")
			case command == "DATA":
				writeSMTPLine(t, rw, "354 End data with <CR><LF>.<CR><LF>")
				var message strings.Builder
				for {
					dataLine, err := rw.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(dataLine) == "." {
						break
					}
					message.WriteString(dataLine)
				}
				messages <- message.String()
				writeSMTPLine(t, rw, "250 OK queued")
			case command == "QUIT":
				writeSMTPLine(t, rw, "221 Bye")
				return
			default:
				writeSMTPLine(t, rw, "250 OK")
			}
		}
	}()
	return ln.Addr().String(), messages
}

func writeSMTPLine(t *testing.T, rw *bufio.ReadWriter, line string) {
	t.Helper()
	if _, err := rw.WriteString(line + "\r\n"); err != nil {
		t.Errorf("smtp write: %v", err)
		return
	}
	if err := rw.Flush(); err != nil {
		t.Errorf("smtp flush: %v", err)
	}
}
