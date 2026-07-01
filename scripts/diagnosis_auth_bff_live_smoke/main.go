// Command diagnosis_auth_bff_live_smoke verifies diagnosis browser-session
// promotion through the OpenClarion web BFF and writes a sanitized proof.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	diagnosisSessionCookieName = "openclarion_diagnosis_session"
	maxResponseBytes           = 1024 * 1024
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

type config struct {
	webBaseURL      string
	authMode        string
	bearerToken     string
	bearerTokenEnv  string
	ldapUsername    string
	ldapUsernameEnv string
	ldapPassword    string
	ldapPasswordEnv string
	outputPath      string
	timeout         time.Duration
}

type proof struct {
	Passed    bool           `json:"passed"`
	CheckedAt string         `json:"checked_at"`
	Request   proofRequest   `json:"request"`
	Issue     sessionProof   `json:"issue"`
	Check     checkProof     `json:"check"`
	Clear     clearProof     `json:"clear"`
	PostClear postClearProof `json:"post_clear"`
	Evidence  string         `json:"evidence"`
}

type proofRequest struct {
	WebBaseURL string `json:"web_base_url"`
	AuthMode   string `json:"auth_mode"`
	Timeout    string `json:"timeout"`
}

type sessionProof struct {
	HTTPStatus         int         `json:"http_status"`
	Subject            string      `json:"subject"`
	Roles              []string    `json:"roles"`
	Mode               string      `json:"mode"`
	CheckedAt          string      `json:"checked_at"`
	RoleAuthorized     bool        `json:"role_authorized"`
	RoleCount          int         `json:"role_count"`
	SessionCookie      bool        `json:"session_cookie_present"`
	SessionCookieAttrs cookieProof `json:"session_cookie_attrs"`
}

type cookieProof struct {
	HTTPOnly bool   `json:"http_only"`
	Path     string `json:"path"`
	SameSite string `json:"same_site"`
	Secure   bool   `json:"secure"`
}

type checkProof struct {
	HTTPStatus     int      `json:"http_status"`
	Subject        string   `json:"subject"`
	Roles          []string `json:"roles"`
	Mode           string   `json:"mode"`
	CheckedAt      string   `json:"checked_at"`
	RoleAuthorized bool     `json:"role_authorized"`
	RoleCount      int      `json:"role_count"`
}

type clearProof struct {
	HTTPStatus           int         `json:"http_status"`
	SessionCookieCleared bool        `json:"session_cookie_cleared"`
	SessionCookieAttrs   cookieProof `json:"session_cookie_attrs"`
}

type postClearProof struct {
	HTTPStatus    int  `json:"http_status"`
	Authenticated bool `json:"authenticated"`
}

type responseMeta struct {
	StatusCode int
	Header     http.Header
}

type sessionStatusResponse struct {
	Authenticated  bool      `json:"authenticated"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Mode           string    `json:"mode,omitempty"`
	RoleAuthorized bool      `json:"role_authorized,omitempty"`
	Roles          []string  `json:"roles,omitempty"`
	Subject        string    `json:"subject,omitempty"`
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[diagnosis-auth-bff-live-smoke] FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, client *http.Client) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("http client is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	issue, sessionCookie, err := issueBrowserSession(ctx, client, cfg)
	if err != nil {
		return err
	}
	check, err := checkBrowserSession(ctx, client, cfg, sessionCookie)
	if err != nil {
		return err
	}
	clearProofResult, err := clearBrowserSession(ctx, client, cfg, sessionCookie)
	if err != nil {
		return err
	}
	postClear, err := checkBrowserSessionCleared(ctx, client, cfg)
	if err != nil {
		return err
	}
	out := proof{
		Passed:    true,
		CheckedAt: nowUTC().Format(time.RFC3339Nano),
		Request: proofRequest{
			WebBaseURL: cfg.webBaseURL,
			AuthMode:   cfg.authMode,
			Timeout:    cfg.timeout.String(),
		},
		Issue:     issue,
		Check:     check,
		Clear:     clearProofResult,
		PostClear: postClear,
	}
	out.Evidence = bffAuthEvidence(out)
	if err := validateProof(out); err != nil {
		return fmt.Errorf("internal proof validation failed: %w", err)
	}
	if err := writeProof(cfg.outputPath, out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[diagnosis-auth-bff-live-smoke] OK - live smoke output: %s\n", cfg.outputPath)
	return nil
}

func parseArgs(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("diagnosis_auth_bff_live_smoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.webBaseURL, "web-base-url", "", "OpenClarion web base URL")
	fs.StringVar(&cfg.authMode, "auth-mode", "", "credential mode: bearer or ldap")
	fs.StringVar(&cfg.bearerToken, "bearer-token", "", "unsupported direct bearer token value; use --bearer-token-env")
	fs.StringVar(&cfg.bearerTokenEnv, "bearer-token-env", "", "environment variable name containing a bearer token or Bearer header")
	fs.StringVar(&cfg.ldapUsername, "ldap-username", "", "LDAP username; prefer --ldap-username-env")
	fs.StringVar(&cfg.ldapUsernameEnv, "ldap-username-env", "", "environment variable name containing the LDAP username")
	fs.StringVar(&cfg.ldapPassword, "ldap-password", "", "unsupported direct LDAP password value; use --ldap-password-env")
	fs.StringVar(&cfg.ldapPasswordEnv, "ldap-password-env", "", "environment variable name containing the LDAP password")
	fs.StringVar(&cfg.outputPath, "output", "", "sanitized proof JSON output path")
	fs.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments")
	}
	cfg.webBaseURL = strings.TrimRight(strings.TrimSpace(cfg.webBaseURL), "/")
	if err := validateBaseURL(cfg.webBaseURL); err != nil {
		return config{}, err
	}
	cfg.authMode = strings.ToLower(strings.TrimSpace(cfg.authMode))
	if cfg.authMode == "" {
		switch {
		case strings.TrimSpace(cfg.bearerTokenEnv) != "" || cfg.bearerToken != "":
			cfg.authMode = "bearer"
		case strings.TrimSpace(cfg.ldapUsernameEnv) != "" || strings.TrimSpace(cfg.ldapPasswordEnv) != "" ||
			cfg.ldapUsername != "" || cfg.ldapPassword != "":
			cfg.authMode = "ldap"
		default:
			return config{}, fmt.Errorf("--auth-mode must be bearer or ldap")
		}
	}
	if cfg.authMode != "bearer" && cfg.authMode != "ldap" {
		return config{}, fmt.Errorf("--auth-mode must be bearer or ldap")
	}
	var err error
	switch cfg.authMode {
	case "bearer":
		if anySet(cfg.ldapUsername, cfg.ldapUsernameEnv, cfg.ldapPassword, cfg.ldapPasswordEnv) {
			return config{}, fmt.Errorf("LDAP credentials require --auth-mode ldap")
		}
		cfg.bearerToken, err = resolveSecretEnvBackedValue(cfg.bearerToken, cfg.bearerTokenEnv, "--bearer-token")
		if err != nil {
			return config{}, err
		}
		cfg.bearerToken, err = normalizedBearerToken(cfg.bearerToken)
		if err != nil {
			return config{}, err
		}
	case "ldap":
		if anySet(cfg.bearerToken, cfg.bearerTokenEnv) {
			return config{}, fmt.Errorf("bearer token requires --auth-mode bearer")
		}
		cfg.ldapUsername, err = resolveEnvBackedValue(cfg.ldapUsername, cfg.ldapUsernameEnv, "--ldap-username")
		if err != nil {
			return config{}, err
		}
		cfg.ldapPassword, err = resolveSecretEnvBackedValue(cfg.ldapPassword, cfg.ldapPasswordEnv, "--ldap-password")
		if err != nil {
			return config{}, err
		}
		if cfg.ldapUsername == "" || cfg.ldapPassword == "" {
			return config{}, fmt.Errorf("--ldap-username and --ldap-password are required")
		}
		if strings.ContainsAny(cfg.ldapUsername, "\x00\r\n\t ") || strings.ContainsAny(cfg.ldapPassword, "\x00\r\n") {
			return config{}, fmt.Errorf("LDAP credentials are malformed")
		}
	}
	cfg.outputPath = filepath.Clean(strings.TrimSpace(cfg.outputPath))
	if cfg.outputPath == "." || cfg.outputPath == "" {
		return config{}, fmt.Errorf("--output is required")
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be greater than zero")
	}
	return cfg, nil
}

func issueBrowserSession(ctx context.Context, client *http.Client, cfg config) (sessionProof, *http.Cookie, error) {
	endpoint, err := webURL(cfg.webBaseURL, "api", "diagnosis", "auth", "session")
	if err != nil {
		return sessionProof{}, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return sessionProof{}, nil, fmt.Errorf("build session issue request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", cfg.authorizationHeader())
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return sessionProof{}, nil, err
	}
	if resp.StatusCode != http.StatusCreated {
		return sessionProof{}, nil, fmt.Errorf("BFF auth session issue endpoint returned HTTP %d", resp.StatusCode)
	}
	var body sessionStatusResponse
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return sessionProof{}, nil, fmt.Errorf("decode BFF auth session issue response: %w", err)
	}
	sessionCookie := responseCookie(resp.Header.Values("Set-Cookie"), diagnosisSessionCookieName)
	if sessionCookie == nil || sessionCookie.Value == "" {
		return sessionProof{}, nil, fmt.Errorf("BFF auth session issue response did not set a diagnosis session cookie")
	}
	return sessionProof{
		HTTPStatus:         resp.StatusCode,
		Subject:            body.Subject,
		Roles:              append([]string(nil), body.Roles...),
		Mode:               strings.TrimSpace(body.Mode),
		CheckedAt:          body.CheckedAt.UTC().Format(time.RFC3339Nano),
		RoleAuthorized:     body.RoleAuthorized,
		RoleCount:          len(body.Roles),
		SessionCookie:      true,
		SessionCookieAttrs: cookieProofFromCookie(sessionCookie),
	}, sessionCookie, nil
}

func checkBrowserSession(ctx context.Context, client *http.Client, cfg config, sessionCookie *http.Cookie) (checkProof, error) {
	endpoint, err := webURL(cfg.webBaseURL, "api", "diagnosis", "auth", "session")
	if err != nil {
		return checkProof{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return checkProof{}, fmt.Errorf("build session check request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie)
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return checkProof{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return checkProof{}, fmt.Errorf("BFF auth session check endpoint returned HTTP %d", resp.StatusCode)
	}
	var body sessionStatusResponse
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return checkProof{}, fmt.Errorf("decode BFF auth session check response: %w", err)
	}
	return checkProof{
		HTTPStatus:     resp.StatusCode,
		Subject:        body.Subject,
		Roles:          append([]string(nil), body.Roles...),
		Mode:           strings.TrimSpace(body.Mode),
		CheckedAt:      body.CheckedAt.UTC().Format(time.RFC3339Nano),
		RoleAuthorized: body.RoleAuthorized,
		RoleCount:      len(body.Roles),
	}, nil
}

func clearBrowserSession(ctx context.Context, client *http.Client, cfg config, sessionCookie *http.Cookie) (clearProof, error) {
	endpoint, err := webURL(cfg.webBaseURL, "api", "diagnosis", "auth", "session")
	if err != nil {
		return clearProof{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return clearProof{}, fmt.Errorf("build session clear request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie)
	resp, _, err := doJSONRequest(client, req)
	if err != nil {
		return clearProof{}, err
	}
	if resp.StatusCode != http.StatusNoContent {
		return clearProof{}, fmt.Errorf("BFF auth session clear endpoint returned HTTP %d", resp.StatusCode)
	}
	clearCookie := responseCookie(resp.Header.Values("Set-Cookie"), diagnosisSessionCookieName)
	if clearCookie == nil {
		return clearProof{}, fmt.Errorf("BFF auth session clear response did not clear the diagnosis session cookie")
	}
	return clearProof{
		HTTPStatus:           resp.StatusCode,
		SessionCookieCleared: sessionCookieCleared(clearCookie),
		SessionCookieAttrs:   cookieProofFromCookie(clearCookie),
	}, nil
}

func checkBrowserSessionCleared(ctx context.Context, client *http.Client, cfg config) (postClearProof, error) {
	endpoint, err := webURL(cfg.webBaseURL, "api", "diagnosis", "auth", "session")
	if err != nil {
		return postClearProof{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return postClearProof{}, fmt.Errorf("build post-clear session check request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return postClearProof{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return postClearProof{}, fmt.Errorf("BFF auth post-clear session check endpoint returned HTTP %d", resp.StatusCode)
	}
	var body sessionStatusResponse
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return postClearProof{}, fmt.Errorf("decode BFF auth post-clear session check response: %w", err)
	}
	return postClearProof{
		HTTPStatus:    resp.StatusCode,
		Authenticated: body.Authenticated,
	}, nil
}

func doJSONRequest(client *http.Client, req *http.Request) (responseMeta, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return responseMeta{}, nil, fmt.Errorf("call %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return responseMeta{}, nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(raw)) > maxResponseBytes {
		return responseMeta{}, nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBytes)
	}
	return responseMeta{StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, raw, nil
}

func responseCookie(rawCookies []string, name string) *http.Cookie {
	for _, raw := range rawCookies {
		cookie, err := http.ParseSetCookie(raw)
		if err != nil || cookie.Name != name {
			continue
		}
		return cookie
	}
	return nil
}

func cookieProofFromCookie(cookie *http.Cookie) cookieProof {
	return cookieProof{
		HTTPOnly: cookie.HttpOnly,
		Path:     cookie.Path,
		SameSite: sameSiteLabel(cookie.SameSite),
		Secure:   cookie.Secure,
	}
}

func sessionCookieCleared(cookie *http.Cookie) bool {
	return cookie.Value == "" && (cookie.MaxAge < 0 || !cookie.Expires.IsZero())
}

func sameSiteLabel(value http.SameSite) string {
	switch value {
	case http.SameSiteLaxMode:
		return "lax"
	case http.SameSiteStrictMode:
		return "strict"
	case http.SameSiteNoneMode:
		return "none"
	default:
		return ""
	}
}

func writeProof(outputPath string, out proof) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proof: %w", err)
	}
	raw = append(raw, '\n')
	// #nosec G304 -- manual smoke output path is supplied by the operator.
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func validateProof(out proof) error {
	if !out.Passed {
		return fmt.Errorf("passed must be true")
	}
	if err := validateCheckedAt(out.CheckedAt); err != nil {
		return err
	}
	if err := validateBaseURL(out.Request.WebBaseURL); err != nil {
		return fmt.Errorf("request.web_base_url is invalid: %w", err)
	}
	if out.Request.AuthMode != "bearer" && out.Request.AuthMode != "ldap" {
		return fmt.Errorf("request.auth_mode must be bearer or ldap")
	}
	if _, err := time.ParseDuration(out.Request.Timeout); err != nil {
		return fmt.Errorf("request.timeout must be a duration: %w", err)
	}
	if err := validateIssueProof(out.Issue); err != nil {
		return err
	}
	if err := validateCheckProof(out.Check, out.Issue); err != nil {
		return err
	}
	if err := validateClearProof(out.Clear); err != nil {
		return err
	}
	if err := validatePostClearProof(out.PostClear); err != nil {
		return err
	}
	if out.Evidence != bffAuthEvidence(out) {
		return fmt.Errorf("evidence mismatch")
	}
	return nil
}

func validateIssueProof(issue sessionProof) error {
	if issue.HTTPStatus != http.StatusCreated {
		return fmt.Errorf("issue.http_status must be 201")
	}
	if !issue.SessionCookie || !issue.SessionCookieAttrs.HTTPOnly ||
		issue.SessionCookieAttrs.Path != "/" ||
		issue.SessionCookieAttrs.SameSite != "lax" {
		return fmt.Errorf("issue session cookie proof is incomplete")
	}
	if err := validatePrincipal(issue.Subject, issue.Mode, issue.Roles, issue.RoleAuthorized, issue.RoleCount); err != nil {
		return fmt.Errorf("issue principal is invalid: %w", err)
	}
	if err := validateCheckedAt(issue.CheckedAt); err != nil {
		return fmt.Errorf("issue.checked_at is invalid: %w", err)
	}
	return nil
}

func validateCheckProof(check checkProof, issue sessionProof) error {
	if check.HTTPStatus != http.StatusOK {
		return fmt.Errorf("check.http_status must be 200")
	}
	if err := validatePrincipal(check.Subject, check.Mode, check.Roles, check.RoleAuthorized, check.RoleCount); err != nil {
		return fmt.Errorf("check principal is invalid: %w", err)
	}
	if err := validateCheckedAt(check.CheckedAt); err != nil {
		return fmt.Errorf("check.checked_at is invalid: %w", err)
	}
	if check.Subject != issue.Subject ||
		check.Mode != issue.Mode ||
		!slices.Equal(sortedStrings(check.Roles), sortedStrings(issue.Roles)) {
		return fmt.Errorf("check proof must match issued session principal")
	}
	return nil
}

func validateClearProof(proof clearProof) error {
	if proof.HTTPStatus != http.StatusNoContent {
		return fmt.Errorf("clear.http_status must be 204")
	}
	if !proof.SessionCookieCleared {
		return fmt.Errorf("clear.session_cookie_cleared must be true")
	}
	if !proof.SessionCookieAttrs.HTTPOnly ||
		proof.SessionCookieAttrs.Path != "/" ||
		proof.SessionCookieAttrs.SameSite != "lax" {
		return fmt.Errorf("clear session cookie proof is incomplete")
	}
	return nil
}

func validatePostClearProof(postClear postClearProof) error {
	if postClear.HTTPStatus != http.StatusOK {
		return fmt.Errorf("post_clear.http_status must be 200")
	}
	if postClear.Authenticated {
		return fmt.Errorf("post_clear.authenticated must be false")
	}
	return nil
}

func validatePrincipal(subject, mode string, roles []string, roleAuthorized bool, roleCount int) error {
	if strings.TrimSpace(subject) == "" || subject != strings.TrimSpace(subject) {
		return fmt.Errorf("subject must be non-empty without surrounding whitespace")
	}
	switch mode {
	case "ldap", "oidc", "static", "wecom":
	default:
		return fmt.Errorf("mode must be ldap, oidc, static, or wecom")
	}
	if len(roles) == 0 || roleCount != len(roles) {
		return fmt.Errorf("roles must be non-empty and match role_count")
	}
	if !roleAuthorized {
		return fmt.Errorf("role_authorized must be true")
	}
	for _, role := range roles {
		if role != "owner" && role != "admin" {
			return fmt.Errorf("roles contains unsupported role")
		}
	}
	return nil
}

func validateCheckedAt(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("checked_at must be non-empty")
	}
	if value != raw {
		return fmt.Errorf("checked_at must not contain surrounding whitespace")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("checked_at must be RFC3339: %w", err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return fmt.Errorf("checked_at must be canonical UTC RFC3339")
	}
	if checkedAt.After(nowUTC()) {
		return fmt.Errorf("checked_at must not be in the future")
	}
	return nil
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("--web-base-url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("--web-base-url must be an absolute URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("--web-base-url must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("--web-base-url must not contain query or fragment")
	}
	return nil
}

func webURL(base string, parts ...string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(append([]string{u.Path}, parts...)...)
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func resolveEnvBackedValue(value, envName, label string) (string, error) {
	rawEnvName := envName
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return value, nil
	}
	if value != "" {
		return "", fmt.Errorf("%s and %s-env cannot both be set", label, label)
	}
	if rawEnvName != envName || !validEnvVarName(envName) {
		return "", fmt.Errorf("%s-env must be a valid environment variable name", label)
	}
	envValue, ok := os.LookupEnv(envName)
	if !ok || envValue == "" {
		return "", fmt.Errorf("%s-env environment variable must be set and non-empty", label)
	}
	return envValue, nil
}

func resolveSecretEnvBackedValue(value, envName, label string) (string, error) {
	if value != "" {
		return "", fmt.Errorf("%s must be provided with %s-env to avoid leaking credentials in process arguments", label, label)
	}
	return resolveEnvBackedValue("", envName, label)
}

func (cfg config) authorizationHeader() string {
	if cfg.authMode == "ldap" {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.ldapUsername+":"+cfg.ldapPassword))
	}
	return "Bearer " + cfg.bearerToken
}

func normalizedBearerToken(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("bearer token is required")
	}
	fields := strings.Fields(value)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		value = fields[1]
	} else if len(fields) != 1 {
		return "", fmt.Errorf("bearer token is malformed")
	}
	if strings.ContainsAny(value, "\x00\r\n\t ") {
		return "", fmt.Errorf("bearer token is malformed")
	}
	return value, nil
}

func anySet(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func validEnvVarName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func bffAuthEvidence(out proof) string {
	roles := append([]string(nil), out.Issue.Roles...)
	slices.Sort(roles)
	return fmt.Sprintf(
		"diagnosis_auth_bff_session success:%s:%s:%s:%s",
		out.Issue.Mode,
		out.Issue.Subject,
		strings.Join(roles, ","),
		out.Issue.SessionCookieAttrs.SameSite,
	)
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}
