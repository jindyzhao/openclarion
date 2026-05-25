// Package http implements the transport layer for OpenClarion's HTTP API.
// Handlers satisfy the generated ServerInterface from api/openapi.gen.go.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Server implements api.ServerInterface.
//
// uowFactory is held for use by future ingestion / diagnosis
// endpoints (M1-PR3 onward). The current /healthz handler does not
// touch the database; intentionally not used yet so that the DI
// graph is stable before workflow code lands.
type Server struct {
	logger     *slog.Logger
	uowFactory ports.UnitOfWorkFactory
}

// NewServer creates a new Server with the given dependencies. The
// UnitOfWorkFactory MUST be non-nil; transports that legitimately do
// not need persistence (e.g. trivial smoke binaries) should construct
// their own narrower struct rather than pass nil here.
func NewServer(logger *slog.Logger, uowFactory ports.UnitOfWorkFactory) *Server {
	return &Server{logger: logger, uowFactory: uowFactory}
}

// GetHealthz implements api.ServerInterface.
func (s *Server) GetHealthz(w http.ResponseWriter, r *http.Request) {
	resp := api.HealthResponse{
		Status: "ok",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error("failed to encode health response", "error", err)
	}
}

// Compile-time check that Server implements ServerInterface.
var _ api.ServerInterface = (*Server)(nil)
