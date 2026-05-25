// Package http implements the transport layer for OpenClarion's HTTP API.
// Handlers satisfy the generated ServerInterface from api/openapi.gen.go.
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/openclarion/openclarion/api"
)

// Server implements api.ServerInterface.
type Server struct {
	logger *slog.Logger
}

// NewServer creates a new Server with the given dependencies.
func NewServer(logger *slog.Logger) *Server {
	return &Server{logger: logger}
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
