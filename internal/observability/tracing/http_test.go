package tracing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	temporalotel "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func TestConfigFromEnvDefaultsToDisabledNoop(t *testing.T) {
	cfg, err := ConfigFromEnv(mapGetenv(nil))
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.ServiceName != defaultServiceName {
		t.Fatalf("service name = %q, want %q", cfg.ServiceName, defaultServiceName)
	}
	if cfg.Enabled {
		t.Fatalf("enabled = true, want false")
	}
}

func TestConfigFromEnvEnablesOTLPTraceExporter(t *testing.T) {
	cfg, err := ConfigFromEnv(mapGetenv(map[string]string{
		"OTEL_SERVICE_NAME":           "openclarion-api",
		"OPENCLARION_SERVICE_VERSION": "test-version",
		"OTEL_TRACES_EXPORTER":        "otlp",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("enabled = false, want true")
	}
	if cfg.ServiceName != "openclarion-api" {
		t.Fatalf("service name = %q, want openclarion-api", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "test-version" {
		t.Fatalf("service version = %q, want test-version", cfg.ServiceVersion)
	}
}

func TestConfigFromEnvRejectsUnsupportedExporter(t *testing.T) {
	if _, err := ConfigFromEnv(mapGetenv(map[string]string{"OTEL_TRACES_EXPORTER": "stdout"})); err == nil {
		t.Fatalf("ConfigFromEnv returned nil error, want unsupported exporter error")
	}
}

func TestConfigFromEnvRejectsCredentialedOTLPEndpoint(t *testing.T) {
	rawEndpointMarker := "raw-marker"
	tests := []struct {
		name       string
		env        map[string]string
		want       string
		wantDetail string
		wantNot    string
	}{
		{
			name:       "generic endpoint username",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://operator@collector.example.test"},
			want:       "OTEL_EXPORTER_OTLP_ENDPOINT",
			wantDetail: "userinfo",
		},
		{
			name:       "generic endpoint password",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": credentialedOTLPEndpoint("")},
			want:       "OTEL_EXPORTER_OTLP_ENDPOINT",
			wantDetail: "userinfo",
		},
		{
			name:       "malformed generic endpoint does not leak raw input",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://operator:" + rawEndpointMarker + "@collector.example.test/\notlp"},
			want:       "OTEL_EXPORTER_OTLP_ENDPOINT",
			wantDetail: "parse endpoint",
			wantNot:    rawEndpointMarker,
		},
		{
			name:       "generic endpoint escaped userinfo",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://operator%40team@collector.example.test"},
			want:       "OTEL_EXPORTER_OTLP_ENDPOINT",
			wantDetail: "userinfo",
		},
		{
			name:       "traces endpoint username",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "https://operator@collector.example.test/v1/traces"},
			want:       "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
			wantDetail: "userinfo",
		},
		{
			name:       "traces endpoint password",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": credentialedOTLPEndpoint("/v1/traces")},
			want:       "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
			wantDetail: "userinfo",
		},
		{
			name:       "malformed traces endpoint does not leak raw input",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "https://operator:" + rawEndpointMarker + "@collector.example.test/\nv1/traces"},
			want:       "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
			wantDetail: "parse endpoint",
			wantNot:    rawEndpointMarker,
		},
		{
			name:       "traces endpoint escaped userinfo",
			env:        map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "https://operator%40team@collector.example.test/v1/traces"},
			want:       "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
			wantDetail: "userinfo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ConfigFromEnv(mapGetenv(tc.env))
			if err == nil {
				t.Fatalf("ConfigFromEnv: want userinfo error")
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("error = %q, want %q and %q", err.Error(), tc.want, tc.wantDetail)
			}
			if tc.wantNot != "" && strings.Contains(err.Error(), tc.wantNot) {
				t.Fatalf("error = %q, must not contain %q", err.Error(), tc.wantNot)
			}
		})
	}
}

func credentialedOTLPEndpoint(path string) string {
	return (&url.URL{
		Scheme: "https",
		User:   url.UserPassword("operator", "opaque"),
		Host:   "collector.example.test",
		Path:   path,
	}).String()
}

func TestHTTPTracingMiddlewareCreatesStableServerSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})
	tracing := &HTTPTracing{
		provider:   provider,
		propagator: defaultPropagator(),
		shutdown:   provider.Shutdown,
		enabled:    true,
	}
	handler := tracing.Middleware("api")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		if !span.SpanContext().IsValid() {
			t.Fatalf("request context has no valid active span")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/reports/42", nil)
	req.Pattern = "GET /api/v1/reports/{report_id}"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "GET /api/v1/reports/{report_id}" {
		t.Fatalf("span name = %q, want stable route pattern", spans[0].Name)
	}
	if spans[0].SpanKind != trace.SpanKindServer {
		t.Fatalf("span kind = %v, want server", spans[0].SpanKind)
	}
}

func TestHTTPTracingHTTPClientPropagatesTraceAndRequestID(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})
	tracing := &HTTPTracing{
		provider:   provider,
		propagator: defaultPropagator(),
		shutdown:   provider.Shutdown,
		enabled:    true,
	}

	var seenTraceparent, seenRequestID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTraceparent = r.Header.Get("Traceparent")
		seenRequestID = r.Header.Get(correlation.RequestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	ctx := correlation.ContextWithRequestID(context.Background(), "request-1")
	ctx, parent := provider.Tracer("openclarion-test").Start(ctx, "parent")
	parentTraceID := parent.SpanContext().TraceID().String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := tracing.HTTPClient(5 * time.Second).Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	parent.End()

	if !strings.Contains(seenTraceparent, parentTraceID) {
		t.Fatalf("Traceparent = %q, want trace id %s", seenTraceparent, parentTraceID)
	}
	if seenRequestID != "request-1" {
		t.Fatalf("%s = %q, want request-1", correlation.RequestIDHeader, seenRequestID)
	}
	var sawClient bool
	for _, span := range exporter.GetSpans() {
		if span.SpanKind == trace.SpanKindClient && span.SpanContext.TraceID().String() == parentTraceID {
			sawClient = true
		}
	}
	if !sawClient {
		t.Fatalf("exported spans did not include a client span on trace %s: %+v", parentTraceID, exporter.GetSpans())
	}
}

func TestHTTPTracingExportsToOTLPHTTPCollector(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		received <- raw
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL+"/v1/traces")
	cfg, err := ConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	tracing, err := NewHTTPTracing(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewHTTPTracing: %v", err)
	}

	handler := tracing.Middleware("api")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/dashboard", nil)
	req.Pattern = "GET /api/v1/dashboard"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tracing.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case raw := <-received:
		if len(raw) == 0 {
			t.Fatalf("collector received empty OTLP trace payload")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("collector did not receive an OTLP trace export")
	}
}

func TestHTTPTracingTemporalInterceptorRecordsWorkflowSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown tracer provider: %v", err)
		}
	})
	tracing := &HTTPTracing{
		provider:   provider,
		propagator: defaultPropagator(),
		shutdown:   provider.Shutdown,
		enabled:    true,
	}

	tracingInterceptor, err := tracing.TemporalInterceptor()
	if err != nil {
		t.Fatalf("TemporalInterceptor: %v", err)
	}
	if tracingInterceptor == nil {
		t.Fatalf("TemporalInterceptor returned nil for enabled tracing")
	}

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(temporalTraceWorkflow)
	env.SetWorkerOptions(worker.Options{
		Interceptors: []interceptor.WorkerInterceptor{tracingInterceptor},
	})

	env.ExecuteWorkflow(temporalTraceWorkflow)
	if !env.IsWorkflowCompleted() {
		t.Fatalf("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}

	if !spanWithAttribute(recorder.Ended(), "openclarion.temporal.workflow_span", "present") {
		t.Fatalf("recorded spans did not include workflow span attribute: %+v", recorder.Ended())
	}
}

func TestHTTPTracingTemporalInterceptorDisabledIsNil(t *testing.T) {
	tracing, err := NewHTTPTracing(context.Background(), Config{})
	if err != nil {
		t.Fatalf("NewHTTPTracing: %v", err)
	}
	tracingInterceptor, err := tracing.TemporalInterceptor()
	if err != nil {
		t.Fatalf("TemporalInterceptor: %v", err)
	}
	if tracingInterceptor != nil {
		t.Fatalf("TemporalInterceptor = %T, want nil when tracing is disabled", tracingInterceptor)
	}
}

func TestHTTPTracingShutdownNoopWhenNil(t *testing.T) {
	var tracing *HTTPTracing
	if err := tracing.Shutdown(context.Background()); err != nil {
		t.Fatalf("nil shutdown returned error: %v", err)
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func temporalTraceWorkflow(ctx workflow.Context) error {
	span, ok := temporalotel.SpanFromWorkflowContext(ctx)
	if !ok {
		return errors.New("workflow context has no OpenTelemetry span")
	}
	span.SetAttributes(attribute.String("openclarion.temporal.workflow_span", "present"))
	return nil
}

func spanWithAttribute(spans []sdktrace.ReadOnlySpan, key string, value string) bool {
	for _, span := range spans {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == key && attr.Value.AsString() == value {
				return true
			}
		}
	}
	return false
}
