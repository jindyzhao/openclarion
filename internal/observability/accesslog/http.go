// Package accesslog provides structured HTTP access logging middleware.
package accesslog

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
)

const defaultStatus = http.StatusOK

// Middleware logs one structured access record per completed request. It logs
// the ServeMux route pattern instead of the raw request target to avoid query
// string leakage and high-cardinality path values.
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if next == nil {
			return nil
		}
		if logger == nil {
			logger = slog.Default()
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			rec := &responseRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = defaultStatus
			}
			attrs := correlation.LogAttrs(r.Context())
			attrs = append(attrs,
				slog.String("method", r.Method),
				slog.String("route", routePattern(r)),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				slog.Int64("response_bytes", rec.bytes),
			)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http access", attrs...)
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = defaultStatus
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unmatched"
	}
	pattern := strings.TrimSpace(r.Pattern)
	if pattern == "" {
		return "unmatched"
	}
	return pattern
}
