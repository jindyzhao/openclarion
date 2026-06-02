// Package prometheus provides a MetricsProvider implementation
// backed by the Prometheus HTTP API v1 (/api/v1/alerts).
//
// The package is intentionally thin: it wraps
// github.com/prometheus/client_golang/api + .../api/prometheus/v1,
// drops Prometheus's "pending" / "inactive" alerts to honour the
// MetricsProvider firing-only contract, and translates
// model.LabelSet into the plain map[string]string the rest of the
// codebase consumes.
//
// Authentication is opt-in via WithBearer. Request IDs propagate through the
// default transport so HTTP-triggered replay calls can be followed across the
// Prometheus boundary without exposing a generic RoundTripper option here.
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// sourceName is the Source identifier this provider writes into
// every ports.ActiveAlert it returns. The constant is package-
// private because downstream code MUST treat the Source as opaque;
// it participates in the AlertEvent (source, canonical_fingerprint,
// starts_at) natural key but is not interpreted otherwise.
const sourceName = "prometheus"

// Provider is the Prometheus-backed MetricsProvider. It is safe for
// concurrent use because the underlying v1.API client wraps a
// stateless *http.Client.
type Provider struct {
	api v1.API
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.MetricsProvider = (*Provider)(nil)

// providerConfig captures NewProvider-time tunables. Kept as a
// concrete struct (rather than directly mutating api.Config) so
// future Options can introduce non-api.Config concerns without
// reshaping the call site.
type providerConfig struct {
	bearerToken           string
	roundTripperDecorator func(http.RoundTripper) http.RoundTripper
}

// Option configures a Provider at construction time.
type Option func(*providerConfig)

// WithBearer attaches a Bearer token to every outbound HTTP
// request via prometheus/common/config's
// NewAuthorizationCredentialsRoundTripper. Passing an empty string
// is treated as "no auth" so callers can write
//
//	NewProvider(addr, WithBearer(os.Getenv("PROM_TOKEN")))
//
// without an extra empty-string guard at the call site.
func WithBearer(token string) Option {
	return func(c *providerConfig) { c.bearerToken = token }
}

// WithRoundTripperDecorator wraps the provider's internally constructed
// transport. It is intended for cross-cutting runtime concerns such as
// OpenTelemetry instrumentation while preserving Prometheus client defaults.
func WithRoundTripperDecorator(decorator func(http.RoundTripper) http.RoundTripper) Option {
	return func(c *providerConfig) { c.roundTripperDecorator = decorator }
}

// NewProvider constructs a Provider against the Prometheus HTTP API at addr
// (e.g. "http://prometheus:9090"). The underlying client reuses Prometheus's
// DefaultRoundTripper, wrapped only for optional Bearer auth, request-id
// propagation, and any caller-supplied transport decorator, so connection
// pooling and timeouts follow upstream defaults. Callers that need stricter
// timeouts MUST wrap the returned Provider rather than re-deriving http.Client
// behaviour here.
func NewProvider(addr string, opts ...Option) (*Provider, error) {
	if err := rejectCredentialedAddress(addr); err != nil {
		return nil, err
	}
	cfg := providerConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	var roundTripper http.RoundTripper = api.DefaultRoundTripper
	if cfg.bearerToken != "" {
		roundTripper = promconfig.NewAuthorizationCredentialsRoundTripper(
			"Bearer",
			promconfig.NewInlineSecret(cfg.bearerToken),
			roundTripper,
		)
	}
	roundTripper = correlation.RoundTripper(roundTripper)
	if cfg.roundTripperDecorator != nil {
		roundTripper = cfg.roundTripperDecorator(roundTripper)
	}

	apiCfg := api.Config{
		Address:      addr,
		RoundTripper: roundTripper,
	}

	client, err := api.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("prometheus: build api client: %w", err)
	}
	return &Provider{api: v1.NewAPI(client)}, nil
}

func rejectCredentialedAddress(addr string) error {
	parsed, err := url.Parse(addr)
	if err != nil {
		return fmt.Errorf("prometheus: address must be a valid URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("prometheus: address must not include userinfo")
	}
	return nil
}

// ListActiveAlerts calls Prometheus's /api/v1/alerts endpoint and
// returns the firing subset as []ports.ActiveAlert.
//
// State filtering happens here (not at the call site) so the
// MetricsProvider contract — "consumers MAY assume every returned
// alert is firing" — is locally verifiable.
//
// RawPayload is the JSON re-encoding of the deserialised v1.Alert
// struct rather than the raw bytes pulled off the wire: the
// client_golang HTTP plumbing buffers + parses the response
// internally and does not surface the raw alert subtree, so a
// re-marshal is the cheapest faithful approximation. Tests assert
// json.Valid on this field but not specific casing, because v1.Alert
// does not carry full json tags and the re-marshal uses Go field
// names ("Labels", "Annotations", ...).
func (p *Provider) ListActiveAlerts(ctx context.Context) ([]ports.ActiveAlert, error) {
	result, err := p.api.Alerts(ctx)
	if err != nil {
		return nil, fmt.Errorf("prometheus: list alerts: %w", err)
	}

	out := make([]ports.ActiveAlert, 0, len(result.Alerts))
	for _, a := range result.Alerts {
		if a.State != v1.AlertStateFiring {
			continue
		}
		raw, err := json.Marshal(a)
		if err != nil {
			// json.Marshal of a v1.Alert (composed of LabelSets,
			// time.Time, string, etc.) cannot realistically fail.
			// Wrap for completeness rather than panicking, so a
			// future field change surfaces as a clean error rather
			// than a goroutine crash.
			return nil, fmt.Errorf("prometheus: marshal alert raw payload: %w", err)
		}
		out = append(out, ports.ActiveAlert{
			Source:      sourceName,
			Labels:      labelSetToMap(a.Labels),
			Annotations: labelSetToMap(a.Annotations),
			StartsAt:    a.ActiveAt,
			RawPayload:  raw,
		})
	}
	return out, nil
}

// labelSetToMap converts a Prometheus model.LabelSet (a
// map[LabelName]LabelValue alias) into the plain map[string]string
// the DTO uses. A nil input is preserved as nil so callers can
// distinguish "field absent" from "field present but empty"; the
// ingestion library normalises both to "{}" before fingerprinting.
func labelSetToMap(in model.LabelSet) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[string(k)] = string(v)
	}
	return out
}
