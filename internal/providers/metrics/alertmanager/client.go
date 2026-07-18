// Package alertmanager provides an ActiveAlertProvider backed by the
// Alertmanager HTTP API v2 (/api/v2/alerts).
package alertmanager

import (
	"bytes"
	"context"
	"encoding/json"
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
	sourceName           = "alertmanager"
	maxResponseBodyBytes = 4 << 20
)

// Provider is the Alertmanager-backed active-alert provider.
type Provider struct {
	client *http.Client
	alerts string
	bearer string
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.ActiveAlertProvider = (*Provider)(nil)

type providerConfig struct {
	bearerToken           string
	roundTripperDecorator func(http.RoundTripper) http.RoundTripper
}

// Option configures a Provider at construction time.
type Option func(*providerConfig)

// WithBearer attaches a Bearer token to every outbound Alertmanager request.
func WithBearer(token string) Option {
	return func(c *providerConfig) { c.bearerToken = token }
}

// WithRoundTripperDecorator wraps the provider transport for cross-cutting
// runtime concerns such as OpenTelemetry instrumentation.
func WithRoundTripperDecorator(decorator func(http.RoundTripper) http.RoundTripper) Option {
	return func(c *providerConfig) { c.roundTripperDecorator = decorator }
}

// NewProvider constructs a Provider against an Alertmanager URL. Operators may
// provide the service root, a route prefix, the /api/v2 prefix, or the full
// /api/v2/alerts endpoint.
func NewProvider(addr string, opts ...Option) (*Provider, error) {
	parsed, err := parseBaseURL(addr)
	if err != nil {
		return nil, err
	}
	cfg := providerConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	base := http.DefaultTransport
	rt := correlation.RoundTripper(base)
	if cfg.roundTripperDecorator != nil {
		rt = cfg.roundTripperDecorator(rt)
	}
	return &Provider{
		client: &http.Client{Transport: rt},
		alerts: alertListURL(parsed),
		bearer: cfg.bearerToken,
	}, nil
}

func parseBaseURL(addr string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(addr))
	if err != nil {
		return nil, fmt.Errorf("alertmanager: address must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("alertmanager: address scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("alertmanager: address host must be non-empty")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("alertmanager: address must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("alertmanager: address must not include query or fragment")
	}
	return parsed, nil
}

func alertListURL(base *url.URL) string {
	u := *base
	path := alertmanagerRoutePrefix(u.Path)
	switch {
	case path == "":
		u.Path = "/api/v2/alerts"
	case strings.HasSuffix(path, "/api/v2/alerts"):
		u.Path = path
	case strings.HasSuffix(path, "/api/v2"):
		u.Path = path + "/alerts"
	default:
		u.Path = path + "/api/v2/alerts"
	}
	query := url.Values{}
	query.Set("active", "true")
	query.Set("silenced", "false")
	query.Set("inhibited", "false")
	query.Set("unprocessed", "false")
	u.RawQuery = query.Encode()
	return u.String()
}

func alertmanagerRoutePrefix(pathname string) string {
	path := strings.TrimRight(pathname, "/")
	const marker = "/api/v2"
	index := strings.LastIndex(path, marker)
	if index < 0 {
		return stripKnownAlertmanagerTerminalPath(path)
	}
	after := path[index+len(marker):]
	if after != "" && !strings.HasPrefix(after, "/") {
		return path
	}
	return path[:index]
}

func stripKnownAlertmanagerTerminalPath(path string) string {
	for _, terminal := range []string{
		"/alerts/groups",
		"/alerts",
		"/silences",
		"/status",
		"/receivers",
	} {
		if path == terminal {
			return ""
		}
		if strings.HasSuffix(path, terminal) {
			return strings.TrimRight(strings.TrimSuffix(path, terminal), "/")
		}
	}
	return path
}

// ListActiveAlerts calls Alertmanager's /api/v2/alerts endpoint and returns
// active, unsuppressed alerts as []ports.ActiveAlert.
func (p *Provider) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	if p == nil || p.client == nil || p.alerts == "" {
		return nil, fmt.Errorf("alertmanager: provider is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.alerts, nil)
	if err != nil {
		return nil, fmt.Errorf("alertmanager: build alerts request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if p.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearer)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alertmanager: list alerts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("alertmanager: list alerts: status %d", resp.StatusCode)
	}
	if resp.Body == nil {
		return nil, fmt.Errorf("alertmanager: response body is nil")
	}
	raw, err := readLimitedResponseBody(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("alertmanager: read response body: %w", err)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("alertmanager: validate response JSON: %w", err)
	}

	var encodedAlerts []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&encodedAlerts); err != nil {
		return nil, fmt.Errorf("alertmanager: decode alerts: %w", err)
	}
	out := make([]ports.ActiveAlert, 0, len(encodedAlerts))
	for _, encoded := range encodedAlerts {
		alert, err := decodeAlert(encoded)
		if err != nil {
			return out, err
		}
		if !isUnsuppressedActiveAlert(alert.Status) {
			continue
		}
		if alert.StartsAt.IsZero() {
			return out, fmt.Errorf("alertmanager: active alert missing startsAt")
		}
		out = append(out, ports.ActiveAlert{
			Source:      sourceName,
			Labels:      alert.Labels,
			Annotations: alert.Annotations,
			StartsAt:    alert.StartsAt,
			RawPayload:  append(json.RawMessage(nil), encoded...),
		})
	}
	return out, nil
}

type gettableAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	Status      alertStatus       `json:"status"`
}

type alertStatus struct {
	State       string   `json:"state"`
	SilencedBy  []string `json:"silencedBy"`
	InhibitedBy []string `json:"inhibitedBy"`
	MutedBy     []string `json:"mutedBy"`
}

func decodeAlert(raw json.RawMessage) (gettableAlert, error) {
	var alert gettableAlert
	if err := json.Unmarshal(raw, &alert); err != nil {
		return gettableAlert{}, fmt.Errorf("alertmanager: decode alert: %w", err)
	}
	return alert, nil
}

func isUnsuppressedActiveAlert(status alertStatus) bool {
	return status.State == "active" &&
		len(status.SilencedBy) == 0 &&
		len(status.InhibitedBy) == 0 &&
		len(status.MutedBy) == 0
}

func readLimitedResponseBody(body io.Reader, limit int) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: int64(limit) + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return raw, nil
}
