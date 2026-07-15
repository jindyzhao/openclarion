package diagnosisauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/tenancy"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// DefaultSessionTTL is the default browser-session bearer lifetime.
	DefaultSessionTTL = 8 * time.Hour
	// HardMaxSessionTTL caps browser-session bearer lifetimes.
	HardMaxSessionTTL = 24 * time.Hour
	// MinSessionSigningKeyBytes is the minimum accepted HMAC key length.
	MinSessionSigningKeyBytes = 32

	legacySessionTokenVersion = 1
	sessionTokenVersion       = 2
	sessionTokenType          = "openclarion.diagnosis.session" // #nosec G101 -- token type identifier only.
	sessionTokenAlg           = "HS256"
)

// SessionTokenPolicy constrains signed browser-session bearer tokens.
type SessionTokenPolicy struct {
	TTL        time.Duration
	SigningKey string
}

// SessionToken is the issued browser-for-frontend bearer token.
type SessionToken struct {
	Token     string
	Subject   string
	Roles     []ports.AuthRole
	Provider  string
	TenantID  domain.TenantID
	TenantKey string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// SessionTokenService issues and verifies short-lived signed diagnosis session
// bearer tokens. The token is meant for a trusted BFF HttpOnly cookie, not page
// JavaScript.
type SessionTokenService struct {
	signingKey []byte
	ttl        time.Duration
	clock      func() time.Time
}

var _ ports.AuthProvider = (*SessionTokenService)(nil)

// DefaultSessionTokenPolicy returns the default browser-session token policy.
func DefaultSessionTokenPolicy(signingKey string) SessionTokenPolicy {
	return SessionTokenPolicy{TTL: DefaultSessionTTL, SigningKey: signingKey}
}

// ValidateSessionTokenPolicy rejects weak signing keys and unsafe lifetimes.
func ValidateSessionTokenPolicy(policy SessionTokenPolicy) error {
	if policy.TTL <= 0 || policy.TTL > HardMaxSessionTTL {
		return fmt.Errorf("diagnosis auth session: ttl %s must be in (0,%s]: %w", policy.TTL, HardMaxSessionTTL, domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(policy.SigningKey) == "" {
		return fmt.Errorf("diagnosis auth session: signing key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if policy.SigningKey != strings.TrimSpace(policy.SigningKey) || strings.ContainsAny(policy.SigningKey, "\x00\r\n") {
		return fmt.Errorf("diagnosis auth session: signing key must not contain leading/trailing whitespace, NUL, CR, or LF: %w", domain.ErrInvariantViolation)
	}
	if len([]byte(policy.SigningKey)) < MinSessionSigningKeyBytes {
		return fmt.Errorf("diagnosis auth session: signing key must be at least %d bytes: %w", MinSessionSigningKeyBytes, domain.ErrInvariantViolation)
	}
	return nil
}

// NewSessionTokenService constructs a signed diagnosis browser-session service.
func NewSessionTokenService(policy SessionTokenPolicy, clock func() time.Time) (*SessionTokenService, error) {
	if err := ValidateSessionTokenPolicy(policy); err != nil {
		return nil, err
	}
	if clock == nil {
		return nil, fmt.Errorf("diagnosis auth session: clock must be non-nil: %w", domain.ErrInvariantViolation)
	}
	return &SessionTokenService{
		signingKey: []byte(policy.SigningKey),
		ttl:        policy.TTL,
		clock:      clock,
	}, nil
}

// IssueToken signs a browser-session bearer token for an authenticated principal.
func (s *SessionTokenService) IssueToken(ctx context.Context, principal ports.AuthPrincipal, provider string) (SessionToken, error) {
	if err := ctx.Err(); err != nil {
		return SessionToken{}, err
	}
	if s == nil || len(s.signingKey) == 0 || s.clock == nil {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: service is not configured")
	}
	subject, err := normalizeSessionSubject(principal.Subject)
	if err != nil {
		return SessionToken{}, err
	}
	roles, err := normalizeSessionRoles(principal.Roles)
	if err != nil {
		return SessionToken{}, err
	}
	provider, err = normalizeSessionProvider(provider)
	if err != nil {
		return SessionToken{}, err
	}
	tenantIdentity, err := sessionTenantIdentity(principal)
	if err != nil {
		return SessionToken{}, err
	}
	issuedAt := s.clock().UTC()
	if issuedAt.IsZero() {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: now must be set: %w", domain.ErrInvariantViolation)
	}
	expiresAt := issuedAt.Add(s.ttl).UTC()
	payload := sessionTokenPayload{
		Version:   sessionTokenVersion,
		Type:      sessionTokenType,
		Provider:  provider,
		Subject:   subject,
		Roles:     roles,
		TenantID:  int64(tenantIdentity.ID),
		TenantKey: tenantIdentity.Key,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: marshal token payload: %w", err)
	}
	token, err := s.signPayload(rawPayload)
	if err != nil {
		return SessionToken{}, err
	}
	return SessionToken{
		Token:     token,
		Subject:   subject,
		Roles:     roles,
		Provider:  provider,
		TenantID:  tenantIdentity.ID,
		TenantKey: tenantIdentity.Key,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}

// RebindTenant re-signs an authenticated browser session for another
// authorized tenant without extending the original session lifetime.
func (s *SessionTokenService) RebindTenant(
	ctx context.Context,
	bearerToken string,
	identity tenancy.Identity,
) (SessionToken, error) {
	session, err := s.AuthenticateSession(ctx, bearerToken)
	if err != nil {
		return SessionToken{}, err
	}
	identity, err = tenancy.NewIdentity(identity.ID, identity.Key)
	if err != nil {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: rebind tenant: %w", err)
	}
	payload := sessionTokenPayload{
		Version:   sessionTokenVersion,
		Type:      sessionTokenType,
		Provider:  session.Provider,
		Subject:   session.Subject,
		Roles:     append([]ports.AuthRole(nil), session.Roles...),
		TenantID:  int64(identity.ID),
		TenantKey: identity.Key,
		IssuedAt:  session.IssuedAt.Unix(),
		ExpiresAt: session.ExpiresAt.Unix(),
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: marshal rebound token payload: %w", err)
	}
	token, err := s.signPayload(rawPayload)
	if err != nil {
		return SessionToken{}, err
	}
	session.Token = token
	session.TenantID = identity.ID
	session.TenantKey = identity.Key
	return session, nil
}

// AuthenticateBearer verifies a session bearer token and returns its principal.
func (s *SessionTokenService) AuthenticateBearer(ctx context.Context, bearerToken string) (ports.AuthPrincipal, error) {
	session, err := s.AuthenticateSession(ctx, bearerToken)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	claims, err := json.Marshal(struct {
		AuthProvider string `json:"auth_provider"`
		TenantKey    string `json:"tenant_key"`
	}{AuthProvider: session.Provider, TenantKey: session.TenantKey})
	if err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("diagnosis auth session: marshal authenticated claims: %w", err)
	}
	return ports.AuthPrincipal{
		Subject:   session.Subject,
		Roles:     session.Roles,
		Claims:    claims,
		TenantID:  session.TenantID,
		TenantKey: session.TenantKey,
	}, nil
}

// AuthenticateAuthorization verifies a session Authorization value and returns
// its embedded principal.
func (s *SessionTokenService) AuthenticateAuthorization(ctx context.Context, authorization string) (ports.AuthPrincipal, error) {
	return s.AuthenticateBearer(ctx, authorization)
}

// AuthenticateSession verifies a session bearer token and returns token metadata.
func (s *SessionTokenService) AuthenticateSession(ctx context.Context, bearerToken string) (SessionToken, error) {
	if err := ctx.Err(); err != nil {
		return SessionToken{}, err
	}
	if s == nil || len(s.signingKey) == 0 || s.clock == nil {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: service is not configured")
	}
	token, err := sessionBearerTokenValue(bearerToken)
	if err != nil {
		return SessionToken{}, err
	}
	payload, err := s.verifyToken(token)
	if err != nil {
		return SessionToken{}, err
	}
	if !supportedSessionTokenVersion(payload.Version) || payload.Type != sessionTokenType {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: token type is unsupported: %w", ErrUnauthenticated)
	}
	if payload.ExpiresAt <= s.clock().UTC().Unix() {
		return SessionToken{}, fmt.Errorf("diagnosis auth session: token expired: %w", ErrUnauthenticated)
	}
	subject, err := normalizeSessionSubject(payload.Subject)
	if err != nil {
		return SessionToken{}, fmt.Errorf("%w: %w", err, ErrUnauthenticated)
	}
	roles, err := normalizeSessionRoles(payload.Roles)
	if err != nil {
		return SessionToken{}, fmt.Errorf("%w: %w", err, ErrUnauthenticated)
	}
	provider, err := normalizeSessionProvider(payload.Provider)
	if err != nil {
		return SessionToken{}, fmt.Errorf("%w: %w", err, ErrUnauthenticated)
	}
	tenantIdentity := tenancy.DefaultIdentity()
	if payload.Version >= sessionTokenVersion {
		tenantIdentity, err = tenancy.NewIdentity(domain.TenantID(payload.TenantID), payload.TenantKey)
		if err != nil {
			return SessionToken{}, fmt.Errorf("diagnosis auth session: invalid tenant binding: %w", ErrUnauthenticated)
		}
	}
	return SessionToken{
		Token:     token,
		Subject:   subject,
		Roles:     roles,
		Provider:  provider,
		TenantID:  tenantIdentity.ID,
		TenantKey: tenantIdentity.Key,
		IssuedAt:  time.Unix(payload.IssuedAt, 0).UTC(),
		ExpiresAt: time.Unix(payload.ExpiresAt, 0).UTC(),
	}, nil
}

func (s *SessionTokenService) signPayload(rawPayload []byte) (string, error) {
	header := sessionTokenHeader{Version: sessionTokenVersion, Type: sessionTokenType, Alg: sessionTokenAlg}
	rawHeader, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("diagnosis auth session: marshal token header: %w", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(rawHeader)
	encodedPayload := base64.RawURLEncoding.EncodeToString(rawPayload)
	signedContent := encodedHeader + "." + encodedPayload
	signature := sessionSignature(s.signingKey, signedContent)
	return signedContent + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (s *SessionTokenService) verifyToken(token string) (sessionTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: token must have three parts: %w", ErrUnauthenticated)
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token header: %w", ErrUnauthenticated)
	}
	if err := strictjson.RejectDuplicateObjectKeys(rawHeader); err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token header: %w", ErrUnauthenticated)
	}
	var header sessionTokenHeader
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token header: %w", ErrUnauthenticated)
	}
	if !supportedSessionTokenVersion(header.Version) || header.Type != sessionTokenType || header.Alg != sessionTokenAlg {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: token header is unsupported: %w", ErrUnauthenticated)
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token payload: %w", ErrUnauthenticated)
	}
	if err := strictjson.RejectDuplicateObjectKeys(rawPayload); err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token payload: %w", ErrUnauthenticated)
	}
	wantSignature := sessionSignature(s.signingKey, parts[0]+"."+parts[1])
	gotSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token signature: %w", ErrUnauthenticated)
	}
	if subtle.ConstantTimeCompare(gotSignature, wantSignature) != 1 {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: token signature is invalid: %w", ErrUnauthenticated)
	}
	var payload sessionTokenPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: decode token payload: %w", ErrUnauthenticated)
	}
	if payload.Version != header.Version {
		return sessionTokenPayload{}, fmt.Errorf("diagnosis auth session: token versions do not match: %w", ErrUnauthenticated)
	}
	return payload, nil
}

func supportedSessionTokenVersion(version int) bool {
	return version == legacySessionTokenVersion || version == sessionTokenVersion
}

type sessionTokenHeader struct {
	Version int    `json:"v"`
	Type    string `json:"typ"`
	Alg     string `json:"alg"`
}

type sessionTokenPayload struct {
	Version   int              `json:"v"`
	Type      string           `json:"typ"`
	Provider  string           `json:"provider"`
	Subject   string           `json:"sub"`
	Roles     []ports.AuthRole `json:"roles"`
	TenantID  int64            `json:"tenant_id"`
	TenantKey string           `json:"tenant_key"`
	IssuedAt  int64            `json:"iat"`
	ExpiresAt int64            `json:"exp"`
}

func sessionTenantIdentity(principal ports.AuthPrincipal) (tenancy.Identity, error) {
	if principal.TenantID == 0 && strings.TrimSpace(principal.TenantKey) == "" {
		return tenancy.DefaultIdentity(), nil
	}
	if principal.TenantID <= 0 || strings.TrimSpace(principal.TenantKey) == "" {
		return tenancy.Identity{}, fmt.Errorf("diagnosis auth session: tenant binding must include id and key: %w", domain.ErrInvariantViolation)
	}
	identity, err := tenancy.NewIdentity(principal.TenantID, principal.TenantKey)
	if err != nil {
		return tenancy.Identity{}, fmt.Errorf("diagnosis auth session: tenant binding: %w", err)
	}
	return identity, nil
}

func sessionSignature(signingKey []byte, signedContent string) []byte {
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write([]byte(signedContent))
	return mac.Sum(nil)
}

func sessionBearerTokenValue(raw string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return normalizeSessionToken(fields[1])
	}
	if len(fields) == 1 {
		return normalizeSessionToken(fields[0])
	}
	return "", fmt.Errorf("diagnosis auth session: bearer token must contain exactly one token: %w", ErrUnauthenticated)
}

func normalizeSessionToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("diagnosis auth session: bearer token must be non-empty: %w", ErrUnauthenticated)
	}
	if token != raw || strings.ContainsAny(token, " \t\r\n") {
		return "", fmt.Errorf("diagnosis auth session: bearer token must be a single value without whitespace: %w", ErrUnauthenticated)
	}
	return token, nil
}

func normalizeSessionSubject(raw string) (string, error) {
	subject := strings.TrimSpace(raw)
	if subject == "" || subject != raw {
		return "", fmt.Errorf("diagnosis auth session: subject must be non-empty without surrounding whitespace")
	}
	return subject, nil
}

func normalizeSessionProvider(raw string) (string, error) {
	provider := strings.TrimSpace(raw)
	if provider == "" || provider != raw {
		return "", fmt.Errorf("diagnosis auth session: provider must be non-empty without surrounding whitespace")
	}
	return provider, nil
}

func normalizeSessionRoles(in []ports.AuthRole) ([]ports.AuthRole, error) {
	out := make([]ports.AuthRole, 0, len(in))
	for _, role := range in {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			out = append(out, role)
		default:
			return nil, fmt.Errorf("diagnosis auth session: unsupported role %q", role)
		}
	}
	return out, nil
}
