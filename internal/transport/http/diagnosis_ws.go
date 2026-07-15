package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	rbacusecase "github.com/openclarion/openclarion/internal/usecases/rbac"
)

const diagnosisWebSocketPath = "/ws/diagnosis"

// #nosec G101 -- header name only; token values are supplied at runtime.
const diagnosisOIDCAccessTokenHeader = "X-OpenClarion-OIDC-Access-Token"

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
	authProvider      ports.AuthProvider
	authProviderNames []string
	authService       *diagnosisauth.Service
	sessionIssuer     *diagnosisauth.SessionTokenService
	sessions          DiagnosisSessionResolver
	wsHandler         DiagnosisWebSocketHandler
	now               func() time.Time
	checkOrigin       func(*http.Request) bool
}

func newDiagnosisConfig() diagnosisConfig {
	return diagnosisConfig{
		now:         func() time.Time { return time.Now().UTC() },
		checkOrigin: defaultDiagnosisWSCheckOrigin,
	}
}

// WithDiagnosisAuth enables POST /api/v1/diagnosis/ws-ticket and the
// authentication half of GET /ws/diagnosis.
func WithDiagnosisAuth(authProvider ports.AuthProvider, service diagnosisauth.Service, sessions DiagnosisSessionResolver, providerName ...string) ServerOption {
	return func(s *Server) {
		s.diagnosis.authProvider = authProvider
		s.diagnosis.authProviderNames = diagnosisAuthProviderNames(providerName...)
		s.diagnosis.authService = &service
		s.diagnosis.sessions = sessions
	}
}

// WithDiagnosisAuthSessionIssuer enables browser-session issuance after a
// successful diagnosis Authorization check.
func WithDiagnosisAuthSessionIssuer(issuer *diagnosisauth.SessionTokenService) ServerOption {
	return func(s *Server) {
		s.diagnosis.sessionIssuer = issuer
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
			opts = append([]DiagnosisWebSocketRelayOption{
				withDiagnosisWebSocketFrameAuthorizer(diagnosisWebSocketFrameAuthorizerFunc(s.AuthorizeDiagnosisWebSocketFrame)),
			}, opts...)
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

// GetDiagnosisAuthStatus implements api.ServerInterface.
func (s *Server) GetDiagnosisAuthStatus(w http.ResponseWriter, r *http.Request) {
	roleMapping := diagnosisAuthRoleMappingStatus(s.diagnosis.authProvider)
	transportPolicy := diagnosisAuthTransportPolicyStatus(s.diagnosis.authProvider)
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DiagnosisAuthStatusResponse{
		Configured:      s.diagnosis.authProvider != nil,
		Mode:            s.diagnosis.authStatusMode(),
		SupportedModes:  diagnosisAuthSupportedModes(s.diagnosis.authSupportedModes()),
		RoleMapping:     roleMapping,
		TransportPolicy: transportPolicy,
	})
}

// CheckDiagnosisAuth implements api.ServerInterface.
func (s *Server) CheckDiagnosisAuth(w http.ResponseWriter, r *http.Request, _ api.CheckDiagnosisAuthParams) {
	if s.diagnosis.authProvider == nil && s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	principal, err := s.authenticateLocalRBACPrincipal(r.Context(), r.Header)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	principal.Subject = subject
	principal, tenantCtx, err := s.bindAuthenticatedTenant(r.Context(), r.Header, principal)
	if err != nil {
		s.writeTenantBindingError(r.Context(), w, err)
		return
	}
	*r = *r.WithContext(tenantCtx)
	writeJSON(r.Context(), w, s.logger, http.StatusOK, api.DiagnosisAuthCheckResponse{
		Subject:        subject,
		Roles:          diagnosisAuthRoles(principal.Roles),
		Mode:           diagnosisAuthCheckResponseMode(s.diagnosis.authCheckMode(principal)),
		CheckedAt:      s.diagnosis.now().UTC(),
		RoleAuthorized: diagnosisAuthRoleAuthorized(principal.Roles),
		TenantID:       int64(principal.TenantID),
		TenantKey:      principal.TenantKey,
	})
}

// IssueDiagnosisAuthSession implements api.ServerInterface.
func (s *Server) IssueDiagnosisAuthSession(w http.ResponseWriter, r *http.Request, _ api.IssueDiagnosisAuthSessionParams) {
	if s.diagnosis.authProvider == nil && s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	if s.diagnosis.sessionIssuer == nil {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis browser session auth is not configured", nil)
		return
	}
	authorization, err := authorizationCredentialsHeader(r.Header.Get("Authorization"))
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	var (
		principal       ports.AuthPrincipal
		existingSession *diagnosisauth.SessionToken
	)
	if session, sessionErr := s.diagnosis.sessionIssuer.AuthenticateSession(r.Context(), authorization); sessionErr == nil {
		existingSession = &session
		principal = ports.AuthPrincipal{
			Subject:   session.Subject,
			Roles:     append([]ports.AuthRole(nil), session.Roles...),
			TenantID:  session.TenantID,
			TenantKey: session.TenantKey,
		}
	} else {
		if s.diagnosis.authProvider == nil {
			writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", sessionErr)
			return
		}
		auxiliaryCredentials, auxiliaryErr := diagnosisAuthAuxiliaryCredentials(r.Header)
		if auxiliaryErr != nil {
			writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", auxiliaryErr)
			return
		}
		principal, err = authenticateDiagnosisAuthorization(
			r.Context(),
			s.diagnosis.authProvider,
			authorization,
			auxiliaryCredentials,
		)
		if err != nil {
			writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
			return
		}
	}
	subject, err := sanitizeDiagnosisAuthSubject(principal.Subject)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusUnauthorized, "authentication failed", err)
		return
	}
	principal.Subject = subject
	principal, tenantCtx, err := s.bindSessionIssuanceTenant(r.Context(), r.Header, principal)
	if err != nil {
		s.writeTenantBindingError(r.Context(), w, err)
		return
	}
	*r = *r.WithContext(tenantCtx)
	identity := tenancy.Identity{ID: principal.TenantID, Key: principal.TenantKey}
	mode := diagnosisAuthCheckResponseMode(s.diagnosis.authCheckMode(principal))
	var session diagnosisauth.SessionToken
	if existingSession != nil {
		mode = existingSession.Provider
		session, err = s.diagnosis.sessionIssuer.RebindTenant(r.Context(), authorization, identity)
	} else {
		var sessionPrincipal ports.AuthPrincipal
		sessionPrincipal, err = diagnosisAuthSessionPrincipal(subject, principal.Roles, mode, identity)
		if err == nil {
			session, err = s.diagnosis.sessionIssuer.IssueToken(r.Context(), sessionPrincipal, mode)
		}
	}
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "issue diagnosis browser session failed", "authentication failed")
		return
	}
	writeJSON(r.Context(), w, s.logger, http.StatusCreated, api.DiagnosisAuthSessionResponse{
		Token:          session.Token,
		Subject:        session.Subject,
		Roles:          diagnosisAuthRoles(session.Roles),
		Mode:           diagnosisAuthSessionResponseMode(mode),
		CheckedAt:      session.IssuedAt,
		ExpiresAt:      session.ExpiresAt,
		RoleAuthorized: diagnosisAuthRoleAuthorized(session.Roles),
		TenantID:       int64(session.TenantID),
		TenantKey:      session.TenantKey,
	})
}

// IssueDiagnosisWSTicket implements api.ServerInterface.
func (s *Server) IssueDiagnosisWSTicket(w http.ResponseWriter, r *http.Request) {
	if !s.diagnosis.ticketConfigured() {
		writeError(r.Context(), w, s.logger, http.StatusServiceUnavailable, "diagnosis auth is not configured", nil)
		return
	}
	body, err := decodeDiagnosisWSTicketRequest(w, r)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	sessionID, err := normalizeRequiredID("session_id", body.SessionID)
	if err != nil {
		writeError(r.Context(), w, s.logger, http.StatusBadRequest, err.Error(), nil)
		return
	}
	principal, ok := s.authorizeLocalRBACPrincipalForScope(
		w,
		r,
		domain.RBACPermissionDiagnosisRoomRead,
		domain.RBACScopeKindDiagnosisRoom,
		sessionID,
	)
	if !ok {
		return
	}
	session, err := s.resolveDiagnosisSession(r.Context(), sessionID)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "resolve diagnosis session failed", "authentication failed")
		return
	}
	ticket, err := s.diagnosis.authService.IssueAuthorizedTicket(r.Context(), principal, session.SessionID, s.diagnosis.now().UTC())
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
	ticket, err := s.diagnosis.authService.ConsumeAuthorizedTicket(r.Context(), token, sessionID, s.diagnosis.now().UTC())
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "consume diagnosis WebSocket ticket failed", "WebSocket ticket is invalid")
		return
	}
	identity, err := tenancy.NewIdentity(ticket.TenantID, ticket.TenantKey)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, diagnosisauth.ErrUnauthenticated, "bind diagnosis WebSocket tenant failed", "WebSocket ticket is invalid")
		return
	}
	tenantCtx, err := tenancy.WithTenant(r.Context(), identity)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "bind diagnosis WebSocket tenant failed", "WebSocket ticket is invalid")
		return
	}
	*r = *r.WithContext(tenantCtx)
	session, err := s.resolveDiagnosisSession(r.Context(), sessionID)
	if err != nil {
		writeDiagnosisServiceError(r.Context(), w, s.logger, err, "resolve diagnosis session failed", "WebSocket ticket is invalid")
		return
	}
	if session.SessionID != ticket.SessionID {
		writeDiagnosisServiceError(r.Context(), w, s.logger, diagnosisauth.ErrUnauthorized, "diagnosis session mismatch after ticket consume", "WebSocket ticket is invalid")
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
	return (c.authProvider != nil || c.sessionIssuer != nil) && c.authService != nil && c.sessions != nil
}

func (c diagnosisConfig) webSocketConfigured() bool {
	return c.ticketConfigured() && c.wsHandler != nil && c.checkOrigin != nil
}

// AuthorizeDiagnosisWebSocketFrame verifies local RBAC for diagnosis-room
// WebSocket frames that mutate or read room state.
func (s *Server) AuthorizeDiagnosisWebSocketFrame(ctx context.Context, ticket diagnosisauth.Ticket, frameType string) error {
	permission, ok := diagnosisWSFramePermission(frameType)
	if !ok {
		return nil
	}
	subject, err := sanitizeDiagnosisAuthSubject(ticket.Subject)
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: subject is invalid: %w", diagnosisauth.ErrUnauthenticated)
	}
	identity, err := tenancy.NewIdentity(ticket.TenantID, ticket.TenantKey)
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: tenant binding is invalid: %w", diagnosisauth.ErrUnauthenticated)
	}
	if current, ok := tenancy.FromContext(ctx); ok && current != identity {
		return fmt.Errorf("diagnosis websocket frame authorization: tenant binding mismatch: %w", diagnosisauth.ErrUnauthorized)
	}
	ctx, err = tenancy.WithTenant(ctx, identity)
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: bind tenant: %w", err)
	}
	if strings.TrimSpace(ticket.SessionID) == "" {
		return fmt.Errorf("diagnosis websocket frame authorization: session id is required: %w", domain.ErrInvariantViolation)
	}
	if s.localRBACBootstrapAdminSubject(subject) {
		return nil
	}
	if s.rbacAuthorizer == nil {
		return fmt.Errorf("diagnosis websocket frame authorization: rbac authorizer is not configured: %w", domain.ErrInvariantViolation)
	}
	departmentKeys, err := s.localRBACDepartmentKeys(ctx, subject)
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: resolve local rbac principal: %w", err)
	}
	principal := domain.RBACPrincipal{
		Subject:        subject,
		DepartmentKeys: departmentKeys,
	}
	ownerAllowed, err := s.diagnosisRoomOwnerAuthorizes(ctx, principal, permission, domain.RBACScopeKindDiagnosisRoom, ticket.SessionID)
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: authorize owner: %w", err)
	}
	if ownerAllowed {
		return nil
	}
	decision, err := s.rbacAuthorizer.Authorize(ctx, rbacusecase.AuthorizeRequest{
		Principal:  principal,
		Permission: permission,
		ScopeKind:  domain.RBACScopeKindDiagnosisRoom,
		ScopeKey:   ticket.SessionID,
	})
	if err != nil {
		return fmt.Errorf("diagnosis websocket frame authorization: authorize %s: %w", frameType, err)
	}
	if !decision.Allowed {
		return fmt.Errorf("diagnosis websocket frame authorization: %s is not allowed: %w", frameType, diagnosisauth.ErrUnauthorized)
	}
	return nil
}

func (c diagnosisConfig) authStatusMode() string {
	if c.authProvider == nil {
		return string(api.DiagnosisAuthStatusResponseModeNone)
	}
	modes := c.authSupportedModes()
	if len(modes) == 0 {
		return string(api.DiagnosisAuthStatusResponseModeUnknown)
	}
	return modes[0]
}

func (c diagnosisConfig) authSupportedModes() []string {
	if c.authProvider == nil {
		return nil
	}
	out := make([]string, 0, len(c.authProviderNames)+1)
	for _, name := range c.authProviderNames {
		mode := diagnosisAuthProviderMode(name)
		if mode != "" && mode != string(api.DiagnosisAuthStatusResponseModeNone) && !slices.Contains(out, mode) {
			out = append(out, mode)
		}
	}
	if len(out) == 0 {
		return []string{string(api.DiagnosisAuthStatusResponseModeUnknown)}
	}
	return out
}

func (c diagnosisConfig) authCheckMode(principal ports.AuthPrincipal) string {
	if mode := diagnosisAuthProviderModeFromClaims(principal.Claims); mode != "" {
		return mode
	}
	return c.authStatusMode()
}

func diagnosisAuthProviderMode(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ldap":
		return string(api.DiagnosisAuthStatusResponseModeLdap)
	case "static":
		return string(api.DiagnosisAuthStatusResponseModeStatic)
	case "oidc":
		return string(api.DiagnosisAuthStatusResponseModeOidc)
	default:
		return string(api.DiagnosisAuthStatusResponseModeUnknown)
	}
}

func diagnosisAuthProviderModeFromClaims(claims json.RawMessage) string {
	if len(claims) == 0 {
		return ""
	}
	var values struct {
		AuthProvider string `json:"auth_provider"`
	}
	if err := json.Unmarshal(claims, &values); err != nil {
		return ""
	}
	mode := diagnosisAuthProviderMode(values.AuthProvider)
	if mode == string(api.DiagnosisAuthStatusResponseModeUnknown) ||
		mode == string(api.DiagnosisAuthStatusResponseModeNone) {
		return ""
	}
	return mode
}

func diagnosisAuthSupportedModes(modes []string) []api.DiagnosisAuthStatusResponseSupportedModesItem {
	out := make([]api.DiagnosisAuthStatusResponseSupportedModesItem, 0, len(modes))
	for _, mode := range modes {
		switch mode {
		case string(api.DiagnosisAuthStatusResponseModeLdap):
			out = append(out, api.DiagnosisAuthStatusResponseSupportedModesItemLdap)
		case string(api.DiagnosisAuthStatusResponseModeStatic):
			out = append(out, api.DiagnosisAuthStatusResponseSupportedModesItemStatic)
		case string(api.DiagnosisAuthStatusResponseModeOidc):
			out = append(out, api.DiagnosisAuthStatusResponseSupportedModesItemOidc)
		case string(api.DiagnosisAuthStatusResponseModeUnknown):
			out = append(out, api.DiagnosisAuthStatusResponseSupportedModesItemUnknown)
		}
	}
	return out
}

func diagnosisAuthRoleMappingStatus(sources ...any) *api.DiagnosisAuthRoleMappingStatus {
	var combined ports.AuthRoleMappingStatus
	var found bool
	for _, source := range sources {
		reporter, ok := source.(ports.AuthRoleMappingReporter)
		if !ok {
			continue
		}
		status := reporter.RoleMappingStatus()
		combined.OwnerMappingCount += status.OwnerMappingCount
		combined.AdminMappingCount += status.AdminMappingCount
		for _, role := range status.DefaultRoles {
			if !slices.Contains(combined.DefaultRoles, role) {
				combined.DefaultRoles = append(combined.DefaultRoles, role)
			}
		}
		found = true
	}
	if !found {
		return nil
	}
	roles := diagnosisAuthRoleMappingDefaultRoles(combined.DefaultRoles)
	return &api.DiagnosisAuthRoleMappingStatus{
		AdminMappingCount: combined.AdminMappingCount,
		Configured: combined.OwnerMappingCount > 0 ||
			combined.AdminMappingCount > 0 ||
			len(roles) > 0,
		DefaultRoles:      roles,
		OwnerMappingCount: combined.OwnerMappingCount,
	}
}

func diagnosisAuthRoleMappingDefaultRoles(roles []ports.AuthRole) []api.DiagnosisAuthRoleMappingStatusDefaultRolesItem {
	out := make([]api.DiagnosisAuthRoleMappingStatusDefaultRolesItem, 0, len(roles))
	for _, role := range roles {
		switch role {
		case ports.AuthRoleOwner:
			out = append(out, api.DiagnosisAuthRoleMappingStatusDefaultRolesItemOwner)
		case ports.AuthRoleAdmin:
			out = append(out, api.DiagnosisAuthRoleMappingStatusDefaultRolesItemAdmin)
		}
	}
	return out
}

func diagnosisAuthTransportPolicyStatus(source any) *api.DiagnosisAuthTransportPolicyStatus {
	reporter, ok := source.(ports.AuthTransportPolicyReporter)
	if !ok {
		return nil
	}
	switch reporter.TransportPolicyStatus().Security {
	case ports.AuthTransportSecurityTLS:
		return &api.DiagnosisAuthTransportPolicyStatus{
			Security: string(api.TLS),
		}
	case ports.AuthTransportSecurityStartTLS:
		return &api.DiagnosisAuthTransportPolicyStatus{
			Security: string(api.StartTLS),
		}
	case ports.AuthTransportSecurityInsecurePlaintext:
		return &api.DiagnosisAuthTransportPolicyStatus{
			Security: string(api.InsecurePlaintext),
		}
	default:
		return nil
	}
}

func diagnosisAuthProviderNames(providerName ...string) []string {
	out := make([]string, 0, len(providerName))
	for _, name := range providerName {
		mode := diagnosisAuthProviderMode(name)
		if mode == "" || slices.Contains(out, mode) {
			continue
		}
		out = append(out, mode)
	}
	if len(out) == 0 {
		return []string{string(api.DiagnosisAuthStatusResponseModeUnknown)}
	}
	return out
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

func decodeDiagnosisWSTicketRequest(w http.ResponseWriter, r *http.Request) (api.DiagnosisWSTicketRequest, error) {
	var body api.DiagnosisWSTicketRequest
	if err := decodeStrictJSONRequestBody(w, r, &body); err != nil {
		return body, err
	}
	if len(body.AdditionalProperties) != 0 {
		return body, fmt.Errorf("request body contains unknown fields")
	}
	return body, nil
}

func authorizationCredentialsHeader(header string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || fields[1] == "" {
		return "", fmt.Errorf("authorization header must be Bearer <token> or Basic <credentials>")
	}
	switch {
	case strings.EqualFold(fields[0], "Bearer"):
		return "Bearer " + fields[1], nil
	case strings.EqualFold(fields[0], "Basic"):
		return "Basic " + fields[1], nil
	default:
		return "", fmt.Errorf("authorization header must be Bearer <token> or Basic <credentials>")
	}
}

func diagnosisAuthAuxiliaryCredentials(header http.Header) (ports.AuthAuxiliaryCredentials, error) {
	rawAccessToken := strings.TrimSpace(header.Get(diagnosisOIDCAccessTokenHeader))
	if rawAccessToken == "" {
		return ports.AuthAuxiliaryCredentials{}, nil
	}
	if len(strings.Fields(rawAccessToken)) != 1 {
		return ports.AuthAuxiliaryCredentials{}, fmt.Errorf("%s must contain exactly one token", diagnosisOIDCAccessTokenHeader)
	}
	return ports.AuthAuxiliaryCredentials{OIDCAccessToken: rawAccessToken}, nil
}

func authenticateDiagnosisAuthorization(ctx context.Context, provider ports.AuthProvider, authorization string, credentials ports.AuthAuxiliaryCredentials) (ports.AuthPrincipal, error) {
	if credentials.OIDCAccessToken == "" {
		return provider.AuthenticateAuthorization(ctx, authorization)
	}
	enhanced, ok := provider.(ports.AuthProviderWithAuxiliaryCredentials)
	if !ok {
		return ports.AuthPrincipal{}, diagnosisauth.ErrUnauthenticated
	}
	return enhanced.AuthenticateAuthorizationWithAuxiliaryCredentials(ctx, authorization, credentials)
}

func sanitizeDiagnosisAuthSubject(subject string) (string, error) {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "", fmt.Errorf("auth subject must be non-empty")
	}
	if trimmed != subject {
		return "", fmt.Errorf("auth subject must not contain leading or trailing whitespace")
	}
	return subject, nil
}

func diagnosisAuthRoles(roles []ports.AuthRole) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			out = append(out, string(role))
		}
	}
	return out
}

func diagnosisAuthRoleAuthorized(roles []ports.AuthRole) bool {
	for _, role := range roles {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			return true
		}
	}
	return false
}

func diagnosisAuthSessionPrincipal(
	subject string,
	roles []ports.AuthRole,
	mode string,
	tenantIdentity tenancy.Identity,
) (ports.AuthPrincipal, error) {
	sessionRoles := make([]ports.AuthRole, 0, len(roles))
	for _, role := range roles {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			if !slices.Contains(sessionRoles, role) {
				sessionRoles = append(sessionRoles, role)
			}
		}
	}
	claims, err := json.Marshal(struct {
		AuthProvider string `json:"auth_provider"`
	}{
		AuthProvider: mode,
	})
	if err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("marshal diagnosis auth session claims: %w", err)
	}
	return ports.AuthPrincipal{
		Subject:   subject,
		Roles:     sessionRoles,
		Claims:    claims,
		TenantID:  tenantIdentity.ID,
		TenantKey: tenantIdentity.Key,
	}, nil
}

func diagnosisAuthCheckResponseMode(mode string) string {
	switch mode {
	case string(api.DiagnosisAuthCheckResponseModeLdap),
		string(api.DiagnosisAuthCheckResponseModeStatic),
		string(api.DiagnosisAuthCheckResponseModeOidc):
		return mode
	default:
		return string(api.DiagnosisAuthCheckResponseModeUnknown)
	}
}

func diagnosisAuthSessionResponseMode(mode string) string {
	switch mode {
	case string(api.DiagnosisAuthSessionResponseModeLdap),
		string(api.DiagnosisAuthSessionResponseModeStatic),
		string(api.DiagnosisAuthSessionResponseModeOidc):
		return mode
	default:
		return string(api.DiagnosisAuthSessionResponseModeUnknown)
	}
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
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func slogError(err error) slog.Attr {
	return slog.Any("error", err)
}
