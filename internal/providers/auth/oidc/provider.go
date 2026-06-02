// Package oidc provides an OpenID Connect implementation of ports.AuthProvider.
package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout   = 10 * time.Second
	defaultRoleClaim = "roles"
)

var (
	defaultOwnerRoleValues = []string{"owner"}
	defaultAdminRoleValues = []string{"admin"}
)

// Config holds OIDC AuthProvider configuration.
type Config struct {
	IssuerURL            string
	ClientID             string
	RoleClaim            string
	OwnerRoleValues      []string
	AdminRoleValues      []string
	SupportedSigningAlgs []string
	HTTPClient           *http.Client
}

// Provider verifies OIDC ID tokens and maps configured role claims into
// OpenClarion's provider-neutral AuthPrincipal shape.
type Provider struct {
	verifier   *gooidc.IDTokenVerifier
	roleClaim  string
	ownerRoles map[string]struct{}
	adminRoles map[string]struct{}
}

var _ ports.AuthProvider = (*Provider)(nil)

// NewProvider discovers the issuer metadata, builds an ID token verifier, and
// returns an AuthProvider. The configured HTTP client is also used for JWKS
// retrieval through the go-oidc context.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	issuer, err := normalizeIssuer(cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("oidc auth: client id must be non-empty")
	}
	roleClaim := strings.TrimSpace(cfg.RoleClaim)
	if roleClaim == "" {
		roleClaim = defaultRoleClaim
	}
	ownerRoles, err := roleValueSet(cfg.OwnerRoleValues, defaultOwnerRoleValues, "owner role values")
	if err != nil {
		return nil, err
	}
	adminRoles, err := roleValueSet(cfg.AdminRoleValues, defaultAdminRoleValues, "admin role values")
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	ctx = gooidc.ClientContext(ctx, httpClient)
	provider, err := gooidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc auth: discover provider: %w", err)
	}
	verifier := provider.VerifierContext(ctx, &gooidc.Config{
		ClientID:             clientID,
		SupportedSigningAlgs: append([]string(nil), cfg.SupportedSigningAlgs...),
	})
	return &Provider{
		verifier:   verifier,
		roleClaim:  roleClaim,
		ownerRoles: ownerRoles,
		adminRoles: adminRoles,
	}, nil
}

// AuthenticateBearer verifies a bearer ID token and maps its configured role
// claim into owner/admin roles.
func (p *Provider) AuthenticateBearer(ctx context.Context, bearerToken string) (ports.AuthPrincipal, error) {
	if p == nil || p.verifier == nil {
		return ports.AuthPrincipal{}, fmt.Errorf("oidc auth: provider is not configured")
	}
	rawToken, err := bearerTokenValue(bearerToken)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	idToken, err := p.verifier.Verify(ctx, rawToken)
	if err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("oidc auth: verify token: %w", err)
	}
	if err := rejectAmbiguousIDTokenClaims(rawToken); err != nil {
		return ports.AuthPrincipal{}, err
	}

	var claims map[string]json.RawMessage
	if err := idToken.Claims(&claims); err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("oidc auth: decode claims: %w", err)
	}
	rawClaims, err := json.Marshal(claims)
	if err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("oidc auth: marshal claims: %w", err)
	}
	roles, err := p.rolesFromClaims(claims)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	return ports.AuthPrincipal{
		Subject: idToken.Subject,
		Roles:   roles,
		Claims:  json.RawMessage(rawClaims),
	}, nil
}

func normalizeIssuer(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("oidc auth: issuer url must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("oidc auth: parse issuer url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("oidc auth: issuer url scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("oidc auth: issuer url must be absolute")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("oidc auth: issuer url must not include userinfo")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func bearerTokenValue(in string) (string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", fmt.Errorf("oidc auth: bearer token must be non-empty")
	}
	fields := strings.Fields(in)
	if len(fields) > 0 && strings.EqualFold(fields[0], "Bearer") {
		if len(fields) != 2 {
			return "", fmt.Errorf("oidc auth: Authorization bearer value must contain exactly one token")
		}
		return fields[1], nil
	}
	if len(fields) != 1 {
		return "", fmt.Errorf("oidc auth: bearer token must not contain whitespace")
	}
	return in, nil
}

func rejectAmbiguousIDTokenClaims(rawToken string) error {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("oidc auth: ID token must use JWT compact serialization")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("oidc auth: decode ID token claims payload: %w", err)
	}
	if err := strictjson.RejectDuplicateObjectKeys(payload); err != nil {
		return fmt.Errorf("oidc auth: ID token claims payload is ambiguous: %w", err)
	}
	return nil
}

func roleValueSet(values, defaults []string, label string) (map[string]struct{}, error) {
	if values == nil {
		values = defaults
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("oidc auth: %s must not contain empty values", label)
		}
		out[value] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("oidc auth: %s must be non-empty", label)
	}
	return out, nil
}

func (p *Provider) rolesFromClaims(claims map[string]json.RawMessage) ([]ports.AuthRole, error) {
	raw, ok := claims[p.roleClaim]
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	values, err := roleClaimValues(raw)
	if err != nil {
		return nil, err
	}
	var owner, admin bool
	for _, value := range values {
		if _, ok := p.ownerRoles[value]; ok {
			owner = true
		}
		if _, ok := p.adminRoles[value]; ok {
			admin = true
		}
	}
	roles := make([]ports.AuthRole, 0, 2)
	if owner {
		roles = append(roles, ports.AuthRoleOwner)
	}
	if admin {
		roles = append(roles, ports.AuthRoleAdmin)
	}
	return roles, nil
}

func roleClaimValues(raw json.RawMessage) ([]string, error) {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		one = strings.TrimSpace(one)
		if one == "" {
			return nil, fmt.Errorf("oidc auth: role claim string must be non-empty")
		}
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, fmt.Errorf("oidc auth: role claim must be a string or string array: %w", err)
	}
	out := make([]string, 0, len(many))
	for _, value := range many {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("oidc auth: role claim array must not contain empty values")
		}
		out = append(out, value)
	}
	return out, nil
}
