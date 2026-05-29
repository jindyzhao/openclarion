// Package metrics owns OpenClarion's Prometheus exposition and HTTP
// instrumentation primitives.
package metrics

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPMetrics contains the Prometheus registry and collectors used by
// the HTTP runtime. Keep this as an instance, not package-global state,
// so tests and future multi-binary runtimes can build isolated registries.
type HTTPMetrics struct {
	registry prometheus.Gatherer
	inFlight prometheus.Gauge
	duration *prometheus.HistogramVec
	requests *prometheus.CounterVec
}

// NewHTTPMetrics creates an isolated Prometheus registry with Go/process
// collectors plus low-cardinality HTTP request metrics.
func NewHTTPMetrics() *HTTPMetrics {
	reg := prometheus.NewRegistry()
	m := &HTTPMetrics{
		registry: reg,
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "openclarion",
			Subsystem: "http",
			Name:      "in_flight_requests",
			Help:      "Number of in-flight HTTP requests.",
		}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "openclarion",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds by handler and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"handler", "method"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "openclarion",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests by handler, method, and response code.",
		}, []string{"handler", "method", "code"}),
	}
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.inFlight,
		m.duration,
		m.requests,
	)
	return m
}

// Handler returns the /metrics scrape handler.
func (m *HTTPMetrics) Handler() http.Handler {
	if m == nil || m.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
		ErrorHandling:     promhttp.ContinueOnError,
	})
}

// Middleware instruments an HTTP handler using a stable, caller-supplied
// handler label. Do not pass raw paths here: route labels must stay
// bounded to avoid high-cardinality Prometheus series.
func (m *HTTPMetrics) Middleware(handlerLabel string) func(http.Handler) http.Handler {
	label := strings.TrimSpace(handlerLabel)
	if label == "" {
		label = "unknown"
	}
	return func(next http.Handler) http.Handler {
		if m == nil || next == nil {
			return next
		}
		labels := prometheus.Labels{"handler": label}
		return promhttp.InstrumentHandlerInFlight(m.inFlight,
			promhttp.InstrumentHandlerDuration(
				m.duration.MustCurryWith(labels),
				promhttp.InstrumentHandlerCounter(
					m.requests.MustCurryWith(labels),
					next,
				),
			),
		)
	}
}
