// Package tracing owns OpenClarion's OpenTelemetry trace setup and HTTP
// instrumentation primitives.
package tracing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	temporalotel "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"

	"github.com/openclarion/openclarion/internal/observability/correlation"
)

const (
	defaultServiceName = "openclarion"
	defaultOperation   = "openclarion.http"
	temporalTracerName = "temporal-sdk-go"
)

// Config controls OpenTelemetry HTTP trace initialization.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Enabled        bool
}

// HTTPTracing contains the tracer provider and propagators used by the HTTP
// runtime. Keep this instance-scoped so tests can inject deterministic
// providers instead of mutating OpenTelemetry global state.
type HTTPTracing struct {
	provider   trace.TracerProvider
	propagator propagation.TextMapPropagator
	shutdown   func(context.Context) error
	enabled    bool
}

// ConfigFromEnv builds tracing configuration from standard OpenTelemetry env
// vars plus OPENCLARION_SERVICE_VERSION for the service.version resource
// attribute. OTLP is opt-in so local development does not emit background
// connection attempts to a collector that is not running.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	serviceName := strings.TrimSpace(getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	tracesExporter := strings.TrimSpace(strings.ToLower(getenv("OTEL_TRACES_EXPORTER")))
	if tracesExporter == "none" || envTruthy(getenv("OTEL_SDK_DISABLED")) {
		return Config{ServiceName: serviceName, ServiceVersion: serviceVersionFromEnv(getenv)}, nil
	}
	if tracesExporter != "" && tracesExporter != "otlp" {
		return Config{}, fmt.Errorf("unsupported OTEL_TRACES_EXPORTER %q (supported: otlp, none)", tracesExporter)
	}

	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		if err := validateOTLPEndpointEnv(key, getenv(key)); err != nil {
			return Config{}, err
		}
	}

	enabled := tracesExporter == "otlp" ||
		strings.TrimSpace(getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
	return Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersionFromEnv(getenv),
		Enabled:        enabled,
	}, nil
}

func validateOTLPEndpointEnv(key, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s parse endpoint", key)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not include userinfo", key)
	}
	return nil
}

// NewHTTPTracing creates the HTTP tracing runtime. When Config.Enabled is
// false, it returns a no-op provider with W3C TraceContext+Baggage propagation.
func NewHTTPTracing(ctx context.Context, cfg Config) (*HTTPTracing, error) {
	propagator := defaultPropagator()
	if !cfg.Enabled {
		return &HTTPTracing{
			provider:   noop.NewTracerProvider(),
			propagator: propagator,
			shutdown:   func(context.Context) error { return nil },
		}, nil
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP HTTP trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resourceForConfig(cfg)),
	)
	return &HTTPTracing{
		provider:   tp,
		propagator: propagator,
		shutdown:   tp.Shutdown,
		enabled:    true,
	}, nil
}

func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// Enabled reports whether traces are exported to an OTLP collector.
func (t *HTTPTracing) Enabled() bool {
	return t != nil && t.enabled
}

// Shutdown flushes and releases tracing resources. It is a no-op for disabled
// tracing.
func (t *HTTPTracing) Shutdown(ctx context.Context) error {
	if t == nil || t.shutdown == nil {
		return nil
	}
	return t.shutdown(ctx)
}

// Middleware instruments an HTTP handler using stable ServeMux route patterns
// when available. Raw request paths are intentionally avoided so span names
// remain low-cardinality.
func (t *HTTPTracing) Middleware(operation string) func(http.Handler) http.Handler {
	op := strings.TrimSpace(operation)
	if op == "" {
		op = defaultOperation
	}
	if t == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	middleware := otelhttp.NewMiddleware(op,
		otelhttp.WithTracerProvider(t.provider),
		otelhttp.WithPropagators(t.propagator),
		otelhttp.WithSpanNameFormatter(stableSpanName),
	)
	return func(next http.Handler) http.Handler {
		if next == nil {
			return nil
		}
		return middleware(next)
	}
}

// Transport instruments outbound HTTP requests with the same tracer provider
// and propagators as inbound HTTP middleware. The caller owns any additional
// transport wrappers such as authentication or retries.
func (t *HTTPTracing) Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if t == nil {
		return base
	}
	return otelhttp.NewTransport(base,
		otelhttp.WithTracerProvider(t.provider),
		otelhttp.WithPropagators(t.propagator),
	)
}

// HTTPClient returns an outbound client that propagates trace context and the
// bounded X-Request-ID value from the request context.
func (t *HTTPTracing) HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: correlation.RoundTripper(t.Transport(nil)),
	}
}

// TemporalInterceptor returns the Temporal SDK OpenTelemetry interceptor wired
// to this runtime's tracer provider and W3C propagator. It returns nil when
// tracing is disabled so callers can omit Temporal tracing without special
// no-op wiring.
func (t *HTTPTracing) TemporalInterceptor() (interceptor.Interceptor, error) {
	if t == nil || !t.Enabled() {
		return nil, nil
	}
	return temporalotel.NewTracingInterceptor(temporalotel.TracerOptions{
		Tracer:                  t.provider.Tracer(temporalTracerName),
		TextMapPropagator:       t.propagator,
		AllowInvalidParentSpans: true,
	})
}

func resourceForConfig(cfg Config) *resource.Resource {
	name := strings.TrimSpace(cfg.ServiceName)
	if name == "" {
		name = defaultServiceName
	}
	attrs := []attribute.KeyValue{semconv.ServiceName(name)}
	if version := strings.TrimSpace(cfg.ServiceVersion); version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	return resource.NewWithAttributes(semconv.SchemaURL, attrs...)
}

func stableSpanName(operation string, r *http.Request) string {
	if r != nil {
		if pattern := strings.TrimSpace(r.Pattern); pattern != "" {
			if strings.Contains(pattern, " ") {
				return pattern
			}
			return strings.TrimSpace(r.Method + " " + pattern)
		}
	}
	return operation
}

func serviceVersionFromEnv(getenv func(string) string) string {
	return strings.TrimSpace(getenv("OPENCLARION_SERVICE_VERSION"))
}

func envTruthy(raw string) bool {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}
