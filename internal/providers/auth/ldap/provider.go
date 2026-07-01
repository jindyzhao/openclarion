// Package ldap provides an LDAP implementation of ports.AuthProvider.
package ldap

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	gldap "github.com/go-ldap/ldap/v3"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultDialTimeout        = 10 * time.Second
	defaultSearchTimeLimit    = 10
	defaultUserFilter         = "(|(uid={username})(sAMAccountName={username})(mail={username}))"
	defaultRoleAttribute      = "memberOf"
	usernameFilterPlaceholder = "{username}"
)

// Conn is the LDAP connection surface used by Provider.
type Conn interface {
	Bind(username, password string) error
	Search(searchRequest *gldap.SearchRequest) (*gldap.SearchResult, error)
	StartTLS(config *tls.Config) error
	Close()
}

// Dialer opens LDAP connections. Tests inject this boundary to avoid using a
// real directory.
type Dialer interface {
	DialURL(ctx context.Context, rawURL string) (Conn, error)
}

// Config holds LDAP AuthProvider configuration.
type Config struct {
	URL                    string
	BaseDN                 string
	BindDN                 string
	BindPassword           string
	UserFilter             string
	SubjectAttribute       string
	RoleAttribute          string
	OwnerRoleValues        []string
	AdminRoleValues        []string
	DefaultRoles           []ports.AuthRole
	StartTLS               bool
	AllowInsecurePlaintext bool
	Dialer                 Dialer
}

// Provider authenticates HTTP Basic credentials against LDAP and maps
// directory attributes into OpenClarion's provider-neutral AuthPrincipal shape.
type Provider struct {
	url              string
	baseDN           string
	bindDN           string
	bindPassword     string
	userFilter       string
	subjectAttribute string
	roleAttribute    string
	ownerRoles       map[string]struct{}
	adminRoles       map[string]struct{}
	defaultRoles     []ports.AuthRole
	startTLS         bool
	dialer           Dialer
}

var _ ports.AuthProvider = (*Provider)(nil)
var _ ports.AuthRoleMappingReporter = (*Provider)(nil)
var _ ports.AuthTransportPolicyReporter = (*Provider)(nil)

// NewProvider validates LDAP auth configuration and returns an AuthProvider.
func NewProvider(cfg Config) (*Provider, error) {
	ldapURL, err := normalizeLDAPURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	ldapScheme, err := ldapURLScheme(ldapURL)
	if err != nil {
		return nil, err
	}
	if ldapScheme == "ldaps" && cfg.StartTLS {
		return nil, fmt.Errorf("ldap auth: start tls requires ldap:// url; use ldaps:// without start tls")
	}
	if ldapScheme == "ldap" && !cfg.StartTLS && !cfg.AllowInsecurePlaintext {
		return nil, fmt.Errorf("ldap auth: ldap:// requires start tls or explicit insecure plaintext allowance")
	}
	baseDN, err := normalizeRequiredConfigValue(cfg.BaseDN, "base dn")
	if err != nil {
		return nil, err
	}
	bindDN, bindPassword, err := normalizeBindCredentials(cfg.BindDN, cfg.BindPassword)
	if err != nil {
		return nil, err
	}
	userFilter, err := normalizeUserFilter(cfg.UserFilter)
	if err != nil {
		return nil, err
	}
	subjectAttribute, err := normalizeOptionalAttribute(cfg.SubjectAttribute, "subject attribute")
	if err != nil {
		return nil, err
	}
	roleAttribute, err := normalizeOptionalAttribute(defaultedString(cfg.RoleAttribute, defaultRoleAttribute), "role attribute")
	if err != nil {
		return nil, err
	}
	ownerRoles, err := ldapRoleValueSet(cfg.OwnerRoleValues, "owner role values")
	if err != nil {
		return nil, err
	}
	adminRoles, err := ldapRoleValueSet(cfg.AdminRoleValues, "admin role values")
	if err != nil {
		return nil, err
	}
	defaultRoles, err := normalizeAuthRoles(cfg.DefaultRoles, "default roles")
	if err != nil {
		return nil, err
	}
	if len(ownerRoles) == 0 && len(adminRoles) == 0 && len(defaultRoles) == 0 {
		return nil, fmt.Errorf("ldap auth: configure at least one owner/admin role value or default role")
	}
	dialer := cfg.Dialer
	if dialer == nil {
		dialer = defaultDialer{}
	}
	return &Provider{
		url:              ldapURL,
		baseDN:           baseDN,
		bindDN:           bindDN,
		bindPassword:     bindPassword,
		userFilter:       userFilter,
		subjectAttribute: subjectAttribute,
		roleAttribute:    roleAttribute,
		ownerRoles:       ownerRoles,
		adminRoles:       adminRoles,
		defaultRoles:     defaultRoles,
		startTLS:         cfg.StartTLS,
		dialer:           dialer,
	}, nil
}

// AuthenticateAuthorization verifies HTTP Basic credentials against LDAP.
func (p *Provider) AuthenticateAuthorization(ctx context.Context, authorization string) (ports.AuthPrincipal, error) {
	if err := ctx.Err(); err != nil {
		return ports.AuthPrincipal{}, err
	}
	if p == nil || p.dialer == nil {
		return ports.AuthPrincipal{}, fmt.Errorf("ldap auth: provider is not configured")
	}
	username, password, err := basicCredentials(authorization)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	conn, err := p.dialer.DialURL(ctx, p.url)
	if err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("ldap auth: connect: %w", err)
	}
	defer conn.Close()
	if p.startTLS {
		tlsConfig, err := tlsConfigForLDAPURL(p.url)
		if err != nil {
			return ports.AuthPrincipal{}, err
		}
		if err := conn.StartTLS(tlsConfig); err != nil {
			return ports.AuthPrincipal{}, fmt.Errorf("ldap auth: start tls failed")
		}
	}
	if p.bindDN != "" {
		if err := conn.Bind(p.bindDN, p.bindPassword); err != nil {
			return ports.AuthPrincipal{}, fmt.Errorf("ldap auth: service bind failed")
		}
	}
	entry, err := p.searchUser(ctx, conn, username)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	if err := ctx.Err(); err != nil {
		return ports.AuthPrincipal{}, err
	}
	if err := conn.Bind(entry.DN, password); err != nil {
		return ports.AuthPrincipal{}, fmt.Errorf("ldap auth: invalid credentials")
	}
	subject, err := p.subject(username, entry)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	roles := p.roles(entry)
	claims, err := ldapClaims(subject, roles)
	if err != nil {
		return ports.AuthPrincipal{}, err
	}
	return ports.AuthPrincipal{
		Subject: subject,
		Roles:   roles,
		Claims:  claims,
	}, nil
}

// RoleMappingStatus returns a non-sensitive summary of LDAP owner/admin role
// mapping readiness. It intentionally exposes counts and normalized
// OpenClarion role names only, never LDAP role values or directory data.
func (p *Provider) RoleMappingStatus() ports.AuthRoleMappingStatus {
	if p == nil {
		return ports.AuthRoleMappingStatus{}
	}
	return ports.AuthRoleMappingStatus{
		OwnerMappingCount: len(p.ownerRoles),
		AdminMappingCount: len(p.adminRoles),
		DefaultRoles:      append([]ports.AuthRole(nil), p.defaultRoles...),
	}
}

// TransportPolicyStatus returns a non-sensitive summary of the LDAP credential
// transport policy. It intentionally reports only the security class.
func (p *Provider) TransportPolicyStatus() ports.AuthTransportPolicyStatus {
	if p == nil {
		return ports.AuthTransportPolicyStatus{}
	}
	if p.startTLS {
		return ports.AuthTransportPolicyStatus{Security: ports.AuthTransportSecurityStartTLS}
	}
	scheme, err := ldapURLScheme(p.url)
	if err != nil {
		return ports.AuthTransportPolicyStatus{}
	}
	if scheme == "ldaps" {
		return ports.AuthTransportPolicyStatus{Security: ports.AuthTransportSecurityTLS}
	}
	return ports.AuthTransportPolicyStatus{Security: ports.AuthTransportSecurityInsecurePlaintext}
}

func (p *Provider) searchUser(ctx context.Context, conn Conn, username string) (*gldap.Entry, error) {
	attributes := p.searchAttributes()
	filter := strings.ReplaceAll(p.userFilter, usernameFilterPlaceholder, gldap.EscapeFilter(username))
	if _, err := gldap.CompileFilter(filter); err != nil {
		return nil, fmt.Errorf("ldap auth: user filter is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request := gldap.NewSearchRequest(
		p.baseDN,
		gldap.ScopeWholeSubtree,
		gldap.NeverDerefAliases,
		2,
		defaultSearchTimeLimit,
		false,
		filter,
		attributes,
		nil,
	)
	request.EnforceSizeLimit = true
	result, err := conn.Search(request)
	if err != nil {
		return nil, fmt.Errorf("ldap auth: search user failed")
	}
	if len(result.Entries) != 1 {
		return nil, fmt.Errorf("ldap auth: invalid credentials")
	}
	if strings.TrimSpace(result.Entries[0].DN) == "" {
		return nil, fmt.Errorf("ldap auth: user entry is missing dn")
	}
	return result.Entries[0], nil
}

func (p *Provider) searchAttributes() []string {
	attributes := []string{}
	if p.subjectAttribute != "" && !strings.EqualFold(p.subjectAttribute, "dn") {
		attributes = appendSearchAttribute(attributes, p.subjectAttribute)
	}
	if p.roleAttribute != "" {
		attributes = appendSearchAttribute(attributes, p.roleAttribute)
	}
	return attributes
}

func appendSearchAttribute(attributes []string, attribute string) []string {
	for _, existing := range attributes {
		if strings.EqualFold(existing, attribute) {
			return attributes
		}
	}
	return append(attributes, attribute)
}

func (p *Provider) subject(username string, entry *gldap.Entry) (string, error) {
	if p.subjectAttribute == "" {
		return username, nil
	}
	if strings.EqualFold(p.subjectAttribute, "dn") {
		return entry.DN, nil
	}
	subject := strings.TrimSpace(entry.GetAttributeValue(p.subjectAttribute))
	if subject == "" {
		return "", fmt.Errorf("ldap auth: subject attribute is missing")
	}
	if subject != entry.GetAttributeValue(p.subjectAttribute) {
		return "", fmt.Errorf("ldap auth: subject attribute must not contain leading or trailing whitespace")
	}
	return subject, nil
}

func (p *Provider) roles(entry *gldap.Entry) []ports.AuthRole {
	roles := append([]ports.AuthRole(nil), p.defaultRoles...)
	for _, value := range entry.GetAttributeValues(p.roleAttribute) {
		key := roleMatchKey(value)
		if _, ok := p.ownerRoles[key]; ok && !slices.Contains(roles, ports.AuthRoleOwner) {
			roles = append(roles, ports.AuthRoleOwner)
		}
		if _, ok := p.adminRoles[key]; ok && !slices.Contains(roles, ports.AuthRoleAdmin) {
			roles = append(roles, ports.AuthRoleAdmin)
		}
	}
	return roles
}

func normalizeLDAPURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("ldap auth: url must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("ldap auth: parse url")
	}
	if parsed.Scheme != "ldap" && parsed.Scheme != "ldaps" {
		return "", fmt.Errorf("ldap auth: url scheme must be ldap or ldaps")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("ldap auth: url must be absolute")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("ldap auth: url must not include userinfo")
	}
	if parsed.RawQuery != "" {
		return "", fmt.Errorf("ldap auth: url must not include query")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("ldap auth: url must not include fragment")
	}
	return parsed.String(), nil
}

func ldapURLScheme(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("ldap auth: parse url")
	}
	return parsed.Scheme, nil
}

func tlsConfigForLDAPURL(raw string) (*tls.Config, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("ldap auth: parse url")
	}
	serverName := strings.TrimSpace(parsed.Hostname())
	if serverName == "" {
		return nil, fmt.Errorf("ldap auth: url must be absolute")
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}, nil
}

func normalizeRequiredConfigValue(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("ldap auth: %s must be non-empty", label)
	}
	if value != raw {
		return "", fmt.Errorf("ldap auth: %s must not contain leading or trailing whitespace", label)
	}
	return value, nil
}

func normalizeBindCredentials(rawDN, password string) (string, string, error) {
	bindDN := strings.TrimSpace(rawDN)
	hasBindDN := bindDN != ""
	hasPassword := password != ""
	if hasBindDN != hasPassword {
		return "", "", fmt.Errorf("ldap auth: bind dn and bind password must be configured together")
	}
	if !hasBindDN {
		return "", "", nil
	}
	if bindDN != rawDN {
		return "", "", fmt.Errorf("ldap auth: bind dn must not contain leading or trailing whitespace")
	}
	if strings.ContainsAny(password, "\x00\r\n") {
		return "", "", fmt.Errorf("ldap auth: bind password must not contain NUL or line breaks")
	}
	return bindDN, password, nil
}

func normalizeUserFilter(raw string) (string, error) {
	filter := strings.TrimSpace(raw)
	if filter == "" {
		filter = defaultUserFilter
	}
	if !strings.Contains(filter, usernameFilterPlaceholder) {
		return "", fmt.Errorf("ldap auth: user filter must contain %s", usernameFilterPlaceholder)
	}
	compiled := strings.ReplaceAll(filter, usernameFilterPlaceholder, gldap.EscapeFilter("fixture"))
	if _, err := gldap.CompileFilter(compiled); err != nil {
		return "", fmt.Errorf("ldap auth: user filter is invalid")
	}
	return filter, nil
}

func normalizeOptionalAttribute(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if value != raw {
		return "", fmt.Errorf("ldap auth: %s must not contain leading or trailing whitespace", label)
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("ldap auth: %s must not contain whitespace", label)
	}
	return value, nil
}

func ldapRoleValueSet(values []string, label string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("ldap auth: %s must not contain empty values", label)
		}
		out[roleMatchKey(value)] = struct{}{}
	}
	return out, nil
}

func normalizeAuthRoles(in []ports.AuthRole, label string) ([]ports.AuthRole, error) {
	out := make([]ports.AuthRole, 0, len(in))
	for _, role := range in {
		switch role {
		case ports.AuthRoleOwner, ports.AuthRoleAdmin:
			if !slices.Contains(out, role) {
				out = append(out, role)
			}
		default:
			return nil, fmt.Errorf("ldap auth: %s contains unsupported role %q", label, role)
		}
	}
	return out, nil
}

func basicCredentials(raw string) (string, string, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Basic") || fields[1] == "" {
		return "", "", fmt.Errorf("ldap auth: authorization header must be Basic credentials")
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", "", fmt.Errorf("ldap auth: invalid basic credentials")
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return "", "", fmt.Errorf("ldap auth: invalid basic credentials")
	}
	if username == "" || password == "" {
		return "", "", fmt.Errorf("ldap auth: invalid basic credentials")
	}
	if strings.ContainsFunc(username, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) || strings.ContainsAny(password, "\x00\r\n") {
		return "", "", fmt.Errorf("ldap auth: invalid basic credentials")
	}
	return username, password, nil
}

func ldapClaims(subject string, roles []ports.AuthRole) (json.RawMessage, error) {
	roleValues := make([]string, len(roles))
	for i, role := range roles {
		roleValues[i] = string(role)
	}
	raw, err := json.Marshal(map[string]any{
		"auth_provider": "ldap",
		"roles":         roleValues,
		"sub":           subject,
	})
	if err != nil {
		return nil, fmt.Errorf("ldap auth: marshal claims: %w", err)
	}
	return raw, nil
}

func roleMatchKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultedString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

type defaultDialer struct{}

func (defaultDialer) DialURL(ctx context.Context, rawURL string) (Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tlsConfig, err := tlsConfigForLDAPURL(rawURL)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	conn, err := gldap.DialURL(rawURL, gldap.DialWithDialer(dialer), gldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, err
	}
	return ldapConn{conn: conn}, nil
}

type ldapConn struct {
	conn *gldap.Conn
}

func (c ldapConn) Bind(username, password string) error {
	return c.conn.Bind(username, password)
}

func (c ldapConn) Search(searchRequest *gldap.SearchRequest) (*gldap.SearchResult, error) {
	return c.conn.Search(searchRequest)
}

func (c ldapConn) StartTLS(config *tls.Config) error {
	return c.conn.StartTLS(config)
}

func (c ldapConn) Close() {
	_ = c.conn.Close()
}
