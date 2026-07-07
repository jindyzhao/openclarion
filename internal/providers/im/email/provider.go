// Package email provides an SMTP-backed ports.IMProvider implementation.
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout = 10 * time.Second

	tlsModeNone     = "none"
	tlsModeStartTLS = "starttls"
	tlsModeImplicit = "tls"

	startTLSRequired      = "required"
	startTLSOpportunistic = "opportunistic"
	startTLSDisabled      = "disabled"

	maxSubjectBytes = 180
	maxBodyBytes    = 64 * 1024

	truncationSuffix = "\n[truncated]"
)

// Config holds SMTP provider configuration.
type Config struct {
	ServerAddr string
	ServerName string
	Username   string
	Password   string
	From       mail.Address
	To         []mail.Address
	TLSMode    string
	Timeout    time.Duration
	Sender     Sender
}

// Sender sends one already-rendered email message.
type Sender interface {
	SendEmail(ctx context.Context, req SendRequest) error
}

// SendRequest is the fully rendered SMTP send request.
type SendRequest struct {
	ServerAddr string
	ServerName string
	Username   string
	Password   string
	From       mail.Address
	To         []mail.Address
	TLSMode    string
	Message    []byte
}

// Provider sends notifications through SMTP email.
type Provider struct {
	cfg    Config
	sender Sender
}

var _ ports.IMProvider = (*Provider)(nil)

// NewProvider constructs an SMTP email provider.
func NewProvider(cfg Config) (*Provider, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	sender := normalized.Sender
	if sender == nil {
		sender = smtpSender{timeout: normalized.Timeout}
	}
	normalized.Sender = nil
	return &Provider{cfg: normalized, sender: sender}, nil
}

// NewProviderFromURL constructs an SMTP email provider from a compact secret URL.
//
// Supported forms:
//   - smtp://user:pass@smtp.example:587?from=alerts%40example.com&to=ops%40example.com
//   - smtps://user:pass@smtp.example:465?from=alerts%40example.com&to=ops%40example.com
//
// smtp:// defaults to starttls=required. Tests and trusted private relays may set
// starttls=disabled explicitly.
func NewProviderFromURL(raw string) (*Provider, error) {
	cfg, err := ConfigFromURL(raw)
	if err != nil {
		return nil, err
	}
	return NewProvider(cfg)
}

// ConfigFromURL parses the deployment-managed SMTP URL secret.
func ConfigFromURL(raw string) (Config, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Config{}, fmt.Errorf("email im: smtp url must be non-empty")
	}
	if trimmed != raw || containsControlOrSpace(raw) {
		return Config{}, fmt.Errorf("email im: smtp url must not contain whitespace or control characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Config{}, fmt.Errorf("email im: parse smtp url")
	}
	if parsed.User == nil {
		parsed.User = url.User("")
	}
	host := parsed.Hostname()
	if host == "" {
		return Config{}, fmt.Errorf("email im: smtp url host must be non-empty")
	}
	if parsed.Fragment != "" {
		return Config{}, fmt.Errorf("email im: smtp url must not include fragment")
	}

	port := parsed.Port()
	var tlsMode string
	switch strings.ToLower(parsed.Scheme) {
	case "smtp":
		if port == "" {
			port = "25"
		}
		tlsMode = tlsModeStartTLS
	case "smtps":
		if port == "" {
			port = "465"
		}
		tlsMode = tlsModeImplicit
	default:
		return Config{}, fmt.Errorf("email im: smtp url scheme must be smtp or smtps")
	}

	values := parsed.Query()
	if len(values["from"]) != 1 {
		return Config{}, fmt.Errorf("email im: smtp url must include exactly one from query parameter")
	}
	from, err := parseAddress(values.Get("from"), "from")
	if err != nil {
		return Config{}, err
	}
	rawRecipients := values["to"]
	if len(rawRecipients) == 0 {
		return Config{}, fmt.Errorf("email im: smtp url must include at least one to query parameter")
	}
	recipients := make([]mail.Address, 0, len(rawRecipients))
	for _, rawRecipient := range rawRecipients {
		recipient, err := parseAddress(rawRecipient, "to")
		if err != nil {
			return Config{}, err
		}
		recipients = append(recipients, recipient)
	}
	if rawStartTLS := values.Get("starttls"); rawStartTLS != "" {
		if parsed.Scheme == "smtps" {
			return Config{}, fmt.Errorf("email im: starttls query parameter is invalid for smtps scheme")
		}
		switch strings.ToLower(rawStartTLS) {
		case startTLSRequired:
			tlsMode = tlsModeStartTLS
		case startTLSOpportunistic:
			tlsMode = startTLSOpportunistic
		case startTLSDisabled:
			tlsMode = tlsModeNone
		default:
			return Config{}, fmt.Errorf("email im: starttls must be required, opportunistic, or disabled")
		}
	}

	for key := range values {
		switch key {
		case "from", "to", "starttls":
		default:
			return Config{}, fmt.Errorf("email im: unsupported smtp url query parameter %q", key)
		}
	}

	username := parsed.User.Username()
	password, passwordSet := parsed.User.Password()
	if username == "" && passwordSet {
		return Config{}, fmt.Errorf("email im: smtp url username must be non-empty when password is set")
	}
	if username != "" && !passwordSet {
		return Config{}, fmt.Errorf("email im: smtp url password is required when username is set")
	}
	return Config{
		ServerAddr: net.JoinHostPort(host, port),
		ServerName: host,
		Username:   username,
		Password:   password,
		From:       from,
		To:         recipients,
		TLSMode:    tlsMode,
	}, nil
}

// SendNotification sends one notification as a plain-text email.
func (p *Provider) SendNotification(ctx context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	if p == nil {
		return ports.IMDelivery{}, fmt.Errorf("email im: provider is nil")
	}
	if err := validateNotification(req); err != nil {
		return ports.IMDelivery{}, err
	}
	message := renderMessage(p.cfg, req)
	sendReq := SendRequest{
		ServerAddr: p.cfg.ServerAddr,
		ServerName: p.cfg.ServerName,
		Username:   p.cfg.Username,
		Password:   p.cfg.Password,
		From:       p.cfg.From,
		To:         append([]mail.Address(nil), p.cfg.To...),
		TLSMode:    p.cfg.TLSMode,
		Message:    message,
	}
	if err := p.sender.SendEmail(ctx, sendReq); err != nil {
		var imErr *ports.IMError
		if errors.As(err, &imErr) {
			return ports.IMDelivery{}, imErr
		}
		return ports.IMDelivery{}, &ports.IMError{
			Message:   fmt.Sprintf("email im: send notification failed: %v", err),
			Retryable: true,
		}
	}
	raw, _ := json.Marshal(map[string]any{
		"provider":        "email",
		"recipient_count": len(p.cfg.To),
	})
	return ports.IMDelivery{
		Status: "delivered",
		Raw:    json.RawMessage(raw),
	}, nil
}

func normalizeConfig(cfg Config) (Config, error) {
	cfg.ServerAddr = strings.TrimSpace(cfg.ServerAddr)
	cfg.ServerName = strings.TrimSpace(cfg.ServerName)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.TLSMode = strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.Timeout < 0 {
		return Config{}, fmt.Errorf("email im: timeout must be non-negative")
	}
	if cfg.ServerAddr == "" {
		return Config{}, fmt.Errorf("email im: server address must be non-empty")
	}
	host, _, err := net.SplitHostPort(cfg.ServerAddr)
	if err != nil {
		return Config{}, fmt.Errorf("email im: server address must include host and port")
	}
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if containsControlOrSpace(cfg.ServerName) {
		return Config{}, fmt.Errorf("email im: server name must not contain whitespace or control characters")
	}
	from, err := normalizeAddress(cfg.From, "from")
	if err != nil {
		return Config{}, err
	}
	cfg.From = from
	if len(cfg.To) == 0 {
		return Config{}, fmt.Errorf("email im: at least one recipient is required")
	}
	to := make([]mail.Address, 0, len(cfg.To))
	for _, recipient := range cfg.To {
		normalized, err := normalizeAddress(recipient, "to")
		if err != nil {
			return Config{}, err
		}
		to = append(to, normalized)
	}
	cfg.To = to
	switch cfg.TLSMode {
	case "":
		cfg.TLSMode = tlsModeStartTLS
	case tlsModeNone, tlsModeStartTLS, startTLSOpportunistic, tlsModeImplicit:
	default:
		return Config{}, fmt.Errorf("email im: tls mode must be none, starttls, opportunistic, or tls")
	}
	if cfg.Username == "" && cfg.Password != "" {
		return Config{}, fmt.Errorf("email im: username is required when password is set")
	}
	return cfg, nil
}

func validateNotification(req ports.IMNotification) error {
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return fmt.Errorf("email im: idempotency key must be non-empty")
	}
	if req.FinalReportID == 0 && req.DiagnosisTaskID == 0 && req.NotificationChannelID == 0 {
		return fmt.Errorf("email im: notification subject id must be non-zero")
	}
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("email im: title must be non-empty")
	}
	if strings.TrimSpace(req.Body) == "" {
		return fmt.Errorf("email im: body must be non-empty")
	}
	return nil
}

func renderMessage(cfg Config, req ports.IMNotification) []byte {
	var buf bytes.Buffer
	writeHeader(&buf, "From", cfg.From.String())
	writeHeader(&buf, "To", addressList(cfg.To))
	writeHeader(&buf, "Date", time.Now().UTC().Format(time.RFC1123Z))
	subject := sanitizeHeaderValue(truncateUTF8Bytes(strings.TrimSpace(req.Title), maxSubjectBytes, "..."))
	writeHeader(&buf, "Subject", mime.QEncoding.Encode("utf-8", subject))
	writeHeader(&buf, "MIME-Version", "1.0")
	writeHeader(&buf, "Content-Type", `text/plain; charset="UTF-8"`)
	writeHeader(&buf, "Content-Transfer-Encoding", "8bit")
	writeHeader(&buf, "X-OpenClarion-Idempotency-Key", strings.TrimSpace(req.IdempotencyKey))
	if req.FinalReportID != 0 {
		writeHeader(&buf, "X-OpenClarion-Final-Report-Id", fmt.Sprintf("%d", req.FinalReportID))
	}
	if req.DiagnosisTaskID != 0 {
		writeHeader(&buf, "X-OpenClarion-Diagnosis-Task-Id", fmt.Sprintf("%d", req.DiagnosisTaskID))
	}
	if req.NotificationChannelID != 0 {
		writeHeader(&buf, "X-OpenClarion-Notification-Channel-Id", fmt.Sprintf("%d", req.NotificationChannelID))
	}
	buf.WriteString("\r\n")
	body := notificationTextContent(req)
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n")
	return buf.Bytes()
}

func writeHeader(buf *bytes.Buffer, key, value string) {
	value = sanitizeHeaderValue(value)
	buf.WriteString(key)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteString("\r\n")
}

func sanitizeHeaderValue(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func notificationTextContent(req ports.IMNotification) string {
	lines := []string{strings.TrimSpace(req.Title)}
	if severity := strings.TrimSpace(req.Severity); severity != "" {
		lines = append(lines, "Severity: "+severity)
	}
	if correlationKey := strings.TrimSpace(req.CorrelationKey); correlationKey != "" {
		lines = append(lines, "Correlation: "+correlationKey)
	}
	lines = append(lines, strings.TrimSpace(req.Body))
	return truncateUTF8Bytes(strings.Join(lines, "\n"), maxBodyBytes, truncationSuffix)
}

func truncateUTF8Bytes(value string, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if len(suffix) >= maxBytes {
		suffix = ""
	}
	limit := maxBytes - len(suffix)
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	truncated := strings.TrimRight(value[:limit], " \t\r\n")
	if truncated == "" {
		return suffix
	}
	return truncated + suffix
}

func parseAddress(raw, label string) (mail.Address, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return mail.Address{}, fmt.Errorf("email im: %s address is invalid", label)
	}
	addr := *parsed
	normalized, err := normalizeAddress(addr, label)
	if err != nil {
		return mail.Address{}, err
	}
	return normalized, nil
}

func normalizeAddress(addr mail.Address, label string) (mail.Address, error) {
	addr.Address = strings.TrimSpace(addr.Address)
	addr.Name = strings.TrimSpace(addr.Name)
	if addr.Address == "" {
		return mail.Address{}, fmt.Errorf("email im: %s address must be non-empty", label)
	}
	if containsControl(addr.Address) || containsControl(addr.Name) {
		return mail.Address{}, fmt.Errorf("email im: %s address must not contain control characters", label)
	}
	if _, err := mail.ParseAddress(addr.String()); err != nil {
		return mail.Address{}, fmt.Errorf("email im: %s address is invalid", label)
	}
	return addr, nil
}

func addressList(addresses []mail.Address) string {
	parts := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		parts = append(parts, addr.String())
	}
	return strings.Join(parts, ", ")
}

type smtpSender struct {
	timeout time.Duration
}

func (s smtpSender) SendEmail(ctx context.Context, req SendRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := dialSMTP(ctx, req)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	client, err := smtp.NewClient(conn, req.ServerName)
	if err != nil {
		return err
	}
	defer client.Close()

	if req.TLSMode == tlsModeStartTLS || req.TLSMode == startTLSOpportunistic {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: req.ServerName, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else if req.TLSMode == tlsModeStartTLS {
			return &ports.IMError{Message: "email im: smtp server does not advertise STARTTLS"}
		}
	}
	if req.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", req.Username, req.Password, req.ServerName)); err != nil {
			return err
		}
	}
	if err := client.Mail(req.From.Address); err != nil {
		return err
	}
	for _, recipient := range req.To {
		if err := client.Rcpt(recipient.Address); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(req.Message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func dialSMTP(ctx context.Context, req SendRequest) (net.Conn, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", req.ServerAddr)
	if err != nil {
		return nil, err
	}
	if req.TLSMode != tlsModeImplicit {
		return conn, nil
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: req.ServerName, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

func containsControlOrSpace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func containsControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
