package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const diagnosisWebSocketPath = "/ws/diagnosis"

// DiagnosisSessionResolver resolves the ownership metadata required before
// issuing or consuming a diagnosis WebSocket ticket.
type DiagnosisSessionResolver interface {
	ResolveDiagnosisSession(ctx context.Context, sessionID string) (diagnosisauth.SessionRef, error)
}

// DiagnosisWebSocketHandler owns the post-authenticated WebSocket connection.
// It is called only after a ticket is consumed successfully; ticket.Token is
// cleared before handoff.
type DiagnosisWebSocketHandler interface {
	ServeDiagnosisWebSocket(ctx context.Context, conn *websocket.Conn, ticket diagnosisauth.Ticket)
}

type diagnosisConfig struct {
	authProvider ports.AuthProvider
	authService  *diagnosisauth.Service
	sessions     DiagnosisSessionResolver
	wsHandler    DiagnosisWebSocketHandler
	now          func() time.Time
	checkOrigin  func(*http.Request) bool
}

func newDiagnosisConfig() diagnosisConfig {
	return diagnosisConfig{
		now:         func() time.Time { return time.Now().UTC() },
		checkOrigin: defaultDiagnosisWSCheckOrigin,
	}
}

// WithDiagnosisAuth enables POST /api/v1/diagnosis/ws-ticket and the
// authentication half of GET /ws/diagnosis.
func WithDiagnosisAuth(authProvider ports.AuthProvider, service diagnosisauth.Service, sessions DiagnosisSessionResolver) ServerOption {
	return func(s *Server) {
		s.diagnosis.authProvider = authProvider
		s.diagnosis.authService = &service
		s.diagnosis.sessions = sessions
	}
}

// WithDiagnosisWebSocketHandler enables authenticated WebSocket handoff after
// ticket consumption.
func WithDiagnosisWebSocketHandler(handler DiagnosisWebSocketHandler) ServerOption {
	return func(s *Server) {
		s.diagnosis.wsHandler = handler
	}
}

// WithDiagnosisRoomWorkflowClient enables the default authenticated
// WebSocket relay that forwards frames into DiagnosisRoomWorkflow.
func WithDiagnosisRoomWorkflowClient(workflows ports.DiagnosisRoomWorkflowClient, opts ...DiagnosisWebSocketRelayOption) ServerOption {
	return func(s *Server) {
		if workflows != nil {
			s.diagnosis.wsHandler = newDiagnosisWebSocketRelay(workflows, opts...)
		}
	}
}

// WithDiagnosisWebSocketOriginCheck overrides the default same-origin browser
// policy. A nil check leaves the existing policy unchanged.
func WithDiagnosisWebSocketOriginCheck(check func(*http.Request) bool) ServerOption {
	return func(s *Server) {
		if check != nil {
			s.diagnosis.checkOrigin = check
		}
	}
}

func withDiagnosisClock(now func() time.Time) ServerOption {
	return func(s *Server) {
		if now != nil {
			s.diagnosis.now = now
		}
	}
}

// RegisterDiagnosisWebSocketRoutes registers non-OpenAPI WebSocket upgrade
// routes on the shared ServeMux. The ticket issuance endpoint remains in the
// generated OpenAPI router.
func (s *Server) RegisterDiagnosisWebSocketRoutes(mux *http.ServeMux, middlewares ...api.MiddlewareFunc) {
	if mux == nil {
		return
	}
	handler := http.Handler(http.HandlerFunc(s.HandleDiagnosisWebSocket))
	for _, middleware := range middlewares {
		if middleware != nil {
			handler = middleware(handler)
		}
	}
	mux.Handle("GET "+diagnosisWebSocketPath, handler)
}

// IssueDiagnosisWSTicket implements api.ServerInterface.
func (s *Server) IssueDiagnosisWSTicket(w http.ResponseWriter, r *http.Request) {
	if !s.diagnosis.ticketConfigured() {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	bearer, err := authorizationBearerHeader(r.Header.Get("Authorization"))
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	body, err := decodeDiagnosisWSTicketRequest(r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	sessionID, err := normalizeRequiredID("session_id", body.SessionID)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	principal, err := s.diagnosis.authProvider.AuthenticateBearer(r.Context(), bearer)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	session, err := s.resolveDiagnosisSession(r.Context(), sessionID)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "resolve diagnosis session failed", "authentication failed")
		return
	}
	ticket, err := s.diagnosis.authService.IssueTicket(r.Context(), principal, session, s.diagnosis.now().UTC())
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "issue diagnosis WebSocket ticket failed", "authentication failed")
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusCreated, api.DiagnosisWSTicketResponse{
		Ticket:    ticket.Token,
		SessionID: ticket.SessionID,
		ExpiresAt: ticket.ExpiresAt,
	})
}

// HandleDiagnosisWebSocket authenticates a ticket and upgrades to WebSocket.
func (s *Server) HandleDiagnosisWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.diagnosis.webSocketConfigured() {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis WebSocket is not configured", nil)
		return
	}
	if !s.diagnosis.checkOrigin(r) {
		writeError(r.Context(), w, s.logger, http.StatusForbidden, "WebSocket origin is not allowed", nil)
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, "request is not a WebSocket upgrade", nil)
		return
	}
	sessionID, err := normalizeRequiredID("session_id", r.URL.Query().Get("session_id"))
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if token == "" {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "WebSocket ticket is required", nil)
		return
	}
	session, err := s.resolveDiagnosisSession(r.Context(), sessionID)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "resolve diagnosis session failed", "WebSocket ticket is invalid")
		return
	}
	ticket, err := s.diagnosis.authService.ConsumeTicket(r.Context(), token, session, s.diagnosis.now().UTC())
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "consume diagnosis WebSocket ticket failed", "WebSocket ticket is invalid")
		return
	}

	upgrader := websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		CheckOrigin:      s.diagnosis.checkOrigin,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logError(r.Context(), s.logger, "diagnosis WebSocket upgrade failed", slogError(err))
		return
	}
	defer func() {
		_ = conn.Close()
	}()
	s.diagnosis.wsHandler.ServeDiagnosisWebSocket(r.Context(), conn, ticket)
}

func (c diagnosisConfig) ticketConfigured() bool {
	return c.authProvider != nil && c.authService != nil && c.sessions != nil
}

func (c diagnosisConfig) webSocketConfigured() bool {
	return c.ticketConfigured() && c.wsHandler != nil && c.checkOrigin != nil
}

func (s *Server) resolveDiagnosisSession(ctx context.Context, sessionID string) (diagnosisauth.SessionRef, error) {
	session, err := s.diagnosis.sessions.ResolveDiagnosisSession(ctx, sessionID)
	if err != nil {
		return diagnosisauth.SessionRef{}, err
	}
	if session.SessionID != sessionID {
		return diagnosisauth.SessionRef{}, fmt.Errorf("diagnosis session resolver returned %q for %q: %w", session.SessionID, sessionID, domain.ErrInvariantViolation)
	}
	return session, nil
}

func decodeDiagnosisWSTicketRequest(r *http.Request) (api.DiagnosisWSTicketRequest, error) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var body api.DiagnosisWSTicketRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		return body, fmt.Errorf("invalid JSON request body: %w", err)
	}
	var extra struct{}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return body, fmt.Errorf("request body must contain exactly one JSON object")
	}
	if len(body.AdditionalProperties) != 0 {
		return body, fmt.Errorf("request body contains unknown fields")
	}
	return body, nil
}

func authorizationBearerHeader(header string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", fmt.Errorf("authorization header must be Bearer <token>")
	}
	return "Bearer " + fields[1], nil
}

func normalizeRequiredID(label, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must be non-empty", label)
	}
	if trimmed != value {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", label)
	}
	return value, nil
}

func writeDiagnosisServiceError(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, err error, fallback, unauthenticated string) {
	switch {
	case errors.Is(err, diagnosisauth.ErrUnauthenticated),
		errors.Is(err, diagnosisauth.ErrTicketExpired),
		errors.Is(err, diagnosisauth.ErrTicketConsumed):
		writeError(ctx, w, logger, http.StatusUnauthorized, unauthenticated, err)
	case errors.Is(err, diagnosisauth.ErrUnauthorized):
		writeError(ctx, w, logger, http.StatusForbidden, "unauthorized", err)
	case errors.Is(err, domain.ErrNotFound):
		writeError(ctx, w, logger, http.StatusNotFound, "diagnosis session not found", err)
	case errors.Is(err, domain.ErrInvariantViolation):
		writeError(ctx, w, logger, http.StatusBadRequest, err.Error(), nil)
	default:
		writeError(ctx, w, logger, http.StatusInternalServerError, fallback, err)
	}
}

func defaultDiagnosisWSCheckOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func slogError(err error) slog.Attr {
	return slog.Any("error", err)
}
