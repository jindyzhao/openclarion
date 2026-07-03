// Package static provides an explicitly configured bearer-token AuthProvider
// for local and private-network diagnosis-room deployments that do not have an
// OIDC issuer.
package static

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Config holds the static bearer identity accepted by Provider.
type Config struct {
	Token   string
	Subject string
	Roles   []ports.AuthRole
}

// Provider verifies one deployment-managed bearer token and returns a fixed
// principal. The raw token is not retained after construction.
type Provider struct {
	tokenDigest [sha256.Size]byte
	subject     string
	roles       []ports.AuthRole
	claims      json.RawMessage
}

var _ ports.AuthProvider = (*Provider)(nil)
var _ ports.AuthRoleMappingReporter = (*Provider)(nil)

// NewProvider constructs a Provider from an explicit token, subject, and role
// set. Callers should source Token from an ignored private environment file or
// deployment secret, never from tracked configuration.
func NewProvider(cfg Config) (*Provider, error) {
	token, err := normalizeToken(cfg.Token)
	if err != nil {
		return nil, err
	}
	subject := strings.TrimSpace(cfg.Subject)
	if subject == "" {
		return nil, fmt.Errorf("static auth: subject must be non-empty")
	}
	if subject != cfg.Subject {
		return nil, fmt.Errorf("static auth: subject must not contain leading or trailing whitespace")
	}
	roles, err := normalizeRoles(cfg.Roles)
	if err != nil {
		return nil, err
	}
	claims, err := staticClaims(subject, roles)
	if err != nil {
		return nil, err
	}
	return &Provider{
		tokenDigest: sha256.Sum256([]byte(token)),
		subject:     subject,
		roles:       roles,
		claims:      claims,
	}, nil
}

// AuthenticateAuthorization accepts either a complete Authorization value
// ("Bearer <token>") or the raw token value.
func (p *Provider) AuthenticateAuthorization(ctx context.Context, authorization string) (ports.AuthPrincipal, error) {
	if err := ctx.Err(); err != nil {
		return ports.AuthPrincipal{}, err
	}
	if p == nil {
		return ports.AuthPrincipal{}, fmt.Errorf("static auth: provider is not configured")
	}
	token, err := bearerTokenValue(authorization)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	gotDigest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(gotDigest[:], p.tokenDigest[:]) != 1 {
		return ports.AuthPrincipal{}, fmt.Errorf("static auth: invalid bearer token")
	}
	return ports.AuthPrincipal{
		Subject: p.subject,
		Roles:   append([]ports.AuthRole(nil), p.roles...),
		Claims:  append(json.RawMessage(nil), p.claims...),
	}, nil
}

// RoleMappingStatus returns the static provider's fixed OpenClarion role set
// without exposing the static subject or bearer token.
func (p *Provider) RoleMappingStatus() ports.AuthRoleMappingStatus {
	if p == nil {
		return ports.AuthRoleMappingStatus{}
	}
	return ports.AuthRoleMappingStatus{
		DefaultRoles: append([]ports.AuthRole(nil), p.roles...),
	}
}

func normalizeToken(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", fmt.Errorf("static auth: token must be non-empty")
	}
	if token != raw || len(strings.Fields(token)) != 1 {
		return "", fmt.Errorf("static auth: token must be a single value without whitespace")
	}
	return token, nil
}

func bearerTokenValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("static auth: bearer token must be non-empty")
	}
	fields := strings.Fields(raw)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return normalizeToken(fields[1])
	}
	if len(fields) == 1 {
		return normalizeToken(fields[0])
	}
	return "", fmt.Errorf("static auth: bearer token must contain exactly one token")
}

func normalizeRoles(in []ports.AuthRole) ([]ports.AuthRole, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("static auth: at least one role is required")
	}
	out := make([]ports.AuthRole, 0, len(in))
	for _, role := range in {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			if !slices.Contains(out, role) {
				out = append(out, role)
			}
		default:
			return nil, fmt.Errorf("static auth: role %q is unsupported", role)
		}
	}
	return out, nil
}

func staticClaims(subject string, roles []ports.AuthRole) (json.RawMessage, error) {
	roleValues := make([]string, len(roles))
	for i, role := range roles {
		roleValues[i] = string(role)
	}
	raw, err := json.Marshal(map[string]any{
		"auth_provider": "static",
		"roles":         roleValues,
		"sub":           subject,
	})
	if err != nil {
		return nil, fmt.Errorf("static auth: marshal claims: %w", err)
	}
	return raw, nil
}
