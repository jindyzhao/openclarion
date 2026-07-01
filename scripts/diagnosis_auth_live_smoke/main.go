// Command diagnosis_auth_live_smoke verifies live diagnosis auth wiring through
// the running OpenClarion HTTP API and writes a sanitized proof artifact.
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

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const maxResponseBytes int64 = 1024 * 1024

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

type config struct {
	apiBaseURL             string
	authMode               string
	bearerToken            string
	bearerTokenEnv         string
	ldapUsername           string
	ldapUsernameEnv        string
	ldapPassword           string
	ldapPasswordEnv        string
	expectedBackendMode    string
	requiredSupportedModes []string
	issueSession           bool
	outputPath             string
	timeout                time.Duration
}

type proof struct {
	Passed    bool              `json:"passed"`
	CheckedAt string            `json:"checked_at"`
	Request   proofRequest      `json:"request"`
	Status    authStatusProof   `json:"status"`
	Check     authCheckProof    `json:"check"`
	Session   *authSessionProof `json:"session,omitempty"`
	Evidence  string            `json:"evidence"`
}

type proofRequest struct {
	APIBaseURL             string   `json:"api_base_url"`
	AuthMode               string   `json:"auth_mode"`
	ExpectedBackendMode    string   `json:"expected_backend_mode,omitempty"`
	RequiredSupportedModes []string `json:"required_supported_modes,omitempty"`
	IssueSession           bool     `json:"issue_session,omitempty"`
	Timeout                string   `json:"timeout"`
}

type authStatusProof struct {
	HTTPStatus     int               `json:"http_status"`
	Configured     bool              `json:"configured"`
	Mode           string            `json:"mode"`
	SupportedModes []string          `json:"supported_modes,omitempty"`
	RoleMapping    *roleMappingProof `json:"role_mapping,omitempty"`
}

type roleMappingProof struct {
	AdminMappingCount int      `json:"admin_mapping_count"`
	Configured        bool     `json:"configured"`
	DefaultRoles      []string `json:"default_roles"`
	OwnerMappingCount int      `json:"owner_mapping_count"`
}

type authCheckProof struct {
	HTTPStatus     int      `json:"http_status"`
	Subject        string   `json:"subject,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	RoleAuthorized bool     `json:"role_authorized"`
	RoleCount      int      `json:"role_count"`
}

type authSessionProof struct {
	HTTPStatus     int      `json:"http_status"`
	Subject        string   `json:"subject,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	CheckedAt      string   `json:"checked_at"`
	ExpiresAt      string   `json:"expires_at"`
	RoleAuthorized bool     `json:"role_authorized"`
	RoleCount      int      `json:"role_count"`
	TokenPresent   bool     `json:"token_present"`
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[diagnosis-auth-live-smoke] FAIL: %v\n", err)
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

	status, err := getAuthStatus(ctx, client, cfg)
	if err != nil {
		return err
	}
	authHeader, err := authorizationHeader(cfg)
	if err != nil {
		return err
	}
	check, err := checkAuth(ctx, client, cfg, authHeader)
	if err != nil {
		return err
	}
	var session *authSessionProof
	if cfg.issueSession {
		issued, err := issueAuthSession(ctx, client, cfg, authHeader)
		if err != nil {
			return err
		}
		session = &authSessionProof{
			HTTPStatus:     issued.HTTPStatus,
			Subject:        issued.Body.Subject,
			Roles:          append([]string(nil), issued.Body.Roles...),
			Mode:           strings.TrimSpace(issued.Body.Mode),
			CheckedAt:      issued.Body.CheckedAt.UTC().Format(time.RFC3339Nano),
			ExpiresAt:      issued.Body.ExpiresAt.UTC().Format(time.RFC3339Nano),
			RoleAuthorized: issued.Body.RoleAuthorized,
			RoleCount:      len(issued.Body.Roles),
			TokenPresent:   strings.TrimSpace(issued.Body.Token) != "",
		}
	}
	out := proof{
		Passed:    true,
		CheckedAt: nowUTC().Format(time.RFC3339Nano),
		Request: proofRequest{
			APIBaseURL:             cfg.apiBaseURL,
			AuthMode:               cfg.authMode,
			ExpectedBackendMode:    cfg.expectedBackendMode,
			RequiredSupportedModes: append([]string(nil), cfg.requiredSupportedModes...),
			IssueSession:           cfg.issueSession,
			Timeout:                cfg.timeout.String(),
		},
		Status: authStatusProof{
			HTTPStatus:     status.HTTPStatus,
			Configured:     status.Body.Configured,
			Mode:           status.Body.Mode,
			SupportedModes: statusSupportedModes(status.Body),
			RoleMapping:    statusRoleMapping(status.Body),
		},
		Check: authCheckProof{
			HTTPStatus:     check.HTTPStatus,
			Subject:        check.Body.Subject,
			Roles:          append([]string(nil), check.Body.Roles...),
			Mode:           strings.TrimSpace(check.Body.Mode),
			RoleAuthorized: check.Body.RoleAuthorized,
			RoleCount:      len(check.Body.Roles),
		},
		Session: session,
	}
	out.Evidence = authEvidence(out)
	if err := validateProof(out); err != nil {
		return fmt.Errorf("internal proof validation failed: %w", err)
	}
	if err := writeProof(cfg.outputPath, out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[diagnosis-auth-live-smoke] OK - live smoke output: %s\n", cfg.outputPath)
	return nil
}

func parseArgs(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("diagnosis_auth_live_smoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.apiBaseURL, "api-base-url", "", "OpenClarion API base URL")
	fs.StringVar(&cfg.authMode, "auth-mode", "", "auth mode for check credentials: ldap or bearer")
	fs.StringVar(&cfg.bearerToken, "bearer-token", "", "unsupported direct bearer token value; use --bearer-token-env")
	fs.StringVar(&cfg.bearerTokenEnv, "bearer-token-env", "", "environment variable name containing the bearer token for bearer auth mode")
	fs.StringVar(&cfg.ldapUsername, "ldap-username", "", "LDAP username for ldap auth mode")
	fs.StringVar(&cfg.ldapUsernameEnv, "ldap-username-env", "", "environment variable name containing the LDAP username for ldap auth mode")
	fs.StringVar(&cfg.ldapPassword, "ldap-password", "", "unsupported direct LDAP password value; use --ldap-password-env")
	fs.StringVar(&cfg.ldapPasswordEnv, "ldap-password-env", "", "environment variable name containing the LDAP password for ldap auth mode")
	fs.StringVar(&cfg.expectedBackendMode, "expected-backend-mode", "", "optional expected backend mode: ldap, static, oidc, unknown, or none")
	requiredSupportedModes := fs.String("required-supported-modes", "", "optional comma-separated backend modes that must be present in status.supported_modes")
	fs.BoolVar(&cfg.issueSession, "issue-session", false, "also verify POST /api/v1/diagnosis/auth/session issues a sanitized browser-session token proof")
	fs.StringVar(&cfg.outputPath, "output", "", "sanitized proof JSON output path")
	fs.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments")
	}
	cfg.apiBaseURL = strings.TrimRight(strings.TrimSpace(cfg.apiBaseURL), "/")
	if err := validateBaseURL(cfg.apiBaseURL); err != nil {
		return config{}, err
	}
	cfg.authMode = strings.ToLower(strings.TrimSpace(cfg.authMode))
	var err error
	switch cfg.authMode {
	case "ldap":
		cfg.ldapUsername, err = resolveEnvBackedValue(cfg.ldapUsername, cfg.ldapUsernameEnv, "--ldap-username")
		if err != nil {
			return config{}, err
		}
		cfg.ldapPassword, err = resolveSecretEnvBackedValue(cfg.ldapPassword, cfg.ldapPasswordEnv, "--ldap-password")
		if err != nil {
			return config{}, err
		}
		cfg.ldapUsername = strings.TrimSpace(cfg.ldapUsername)
		if cfg.ldapUsername == "" || cfg.ldapPassword == "" {
			return config{}, fmt.Errorf("--ldap-username and --ldap-password are required for ldap auth mode")
		}
		if strings.ContainsAny(cfg.ldapUsername, "\x00\r\n\t ") || strings.ContainsAny(cfg.ldapPassword, "\x00\r\n") {
			return config{}, fmt.Errorf("LDAP credentials are malformed")
		}
	case "bearer":
		cfg.bearerToken, err = resolveSecretEnvBackedValue(cfg.bearerToken, cfg.bearerTokenEnv, "--bearer-token")
		if err != nil {
			return config{}, err
		}
		cfg.bearerToken = strings.TrimSpace(cfg.bearerToken)
		if cfg.bearerToken == "" || strings.ContainsAny(cfg.bearerToken, "\r\n\t ") {
			return config{}, fmt.Errorf("--bearer-token must be a single token for bearer auth mode")
		}
	default:
		return config{}, fmt.Errorf("--auth-mode must be ldap or bearer")
	}
	cfg.expectedBackendMode = strings.ToLower(strings.TrimSpace(cfg.expectedBackendMode))
	if cfg.expectedBackendMode != "" && !validBackendMode(cfg.expectedBackendMode) {
		return config{}, fmt.Errorf("--expected-backend-mode must be ldap, static, oidc, unknown, or none")
	}
	cfg.requiredSupportedModes, err = parseSupportedModeCSV(*requiredSupportedModes, "--required-supported-modes")
	if err != nil {
		return config{}, err
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

func parseSupportedModeCSV(raw, label string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		mode := strings.ToLower(strings.TrimSpace(part))
		if mode == "" {
			return nil, fmt.Errorf("%s must not contain empty modes", label)
		}
		if !validSupportedMode(mode) {
			return nil, fmt.Errorf("%s contains unsupported mode %q", label, mode)
		}
		if slices.Contains(out, mode) {
			return nil, fmt.Errorf("%s contains duplicate mode %q", label, mode)
		}
		out = append(out, mode)
	}
	return out, nil
}

func resolveEnvBackedValue(value, envName, label string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return value, nil
	}
	if value != "" {
		return "", fmt.Errorf("%s and %s-env cannot both be set", label, label)
	}
	if !validEnvVarName(envName) {
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

type statusResponse struct {
	HTTPStatus int
	Body       api.DiagnosisAuthStatusResponse
}

type checkResponse struct {
	HTTPStatus int
	Body       api.DiagnosisAuthCheckResponse
}

type sessionResponse struct {
	HTTPStatus int
	Body       api.DiagnosisAuthSessionResponse
}

type responseMeta struct {
	StatusCode int
}

func statusSupportedModes(body api.DiagnosisAuthStatusResponse) []string {
	out := make([]string, 0, len(body.SupportedModes))
	for _, mode := range body.SupportedModes {
		out = append(out, strings.TrimSpace(string(mode)))
	}
	return out
}

func statusRoleMapping(body api.DiagnosisAuthStatusResponse) *roleMappingProof {
	if body.RoleMapping == nil {
		return nil
	}
	defaultRoles := make([]string, 0, len(body.RoleMapping.DefaultRoles))
	for _, role := range body.RoleMapping.DefaultRoles {
		defaultRoles = append(defaultRoles, strings.TrimSpace(string(role)))
	}
	return &roleMappingProof{
		AdminMappingCount: body.RoleMapping.AdminMappingCount,
		Configured:        body.RoleMapping.Configured,
		DefaultRoles:      defaultRoles,
		OwnerMappingCount: body.RoleMapping.OwnerMappingCount,
	}
}

func getAuthStatus(ctx context.Context, client *http.Client, cfg config) (statusResponse, error) {
	endpoint, err := apiURL(cfg.apiBaseURL, "api", "v1", "diagnosis", "auth", "status")
	if err != nil {
		return statusResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return statusResponse{}, fmt.Errorf("build status request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return statusResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return statusResponse{}, fmt.Errorf("auth status endpoint returned HTTP %d", resp.StatusCode)
	}
	var body api.DiagnosisAuthStatusResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return statusResponse{}, fmt.Errorf("decode auth status response: %w", err)
	}
	return statusResponse{
		HTTPStatus: resp.StatusCode,
		Body:       body,
	}, nil
}

func checkAuth(ctx context.Context, client *http.Client, cfg config, authHeader string) (checkResponse, error) {
	endpoint, err := apiURL(cfg.apiBaseURL, "api", "v1", "diagnosis", "auth", "check")
	if err != nil {
		return checkResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return checkResponse{}, fmt.Errorf("build check request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authHeader)
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return checkResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return checkResponse{}, fmt.Errorf("auth check endpoint returned HTTP %d", resp.StatusCode)
	}
	var body api.DiagnosisAuthCheckResponse
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return checkResponse{}, fmt.Errorf("decode auth check response: %w", err)
	}
	return checkResponse{HTTPStatus: resp.StatusCode, Body: body}, nil
}

func issueAuthSession(ctx context.Context, client *http.Client, cfg config, authHeader string) (sessionResponse, error) {
	endpoint, err := apiURL(cfg.apiBaseURL, "api", "v1", "diagnosis", "auth", "session")
	if err != nil {
		return sessionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return sessionResponse{}, fmt.Errorf("build session request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authHeader)
	resp, raw, err := doJSONRequest(client, req)
	if err != nil {
		return sessionResponse{}, err
	}
	if resp.StatusCode != http.StatusCreated {
		return sessionResponse{}, fmt.Errorf("auth session endpoint returned HTTP %d", resp.StatusCode)
	}
	var body api.DiagnosisAuthSessionResponse
	if err := strictjson.Unmarshal(raw, &body); err != nil {
		return sessionResponse{}, fmt.Errorf("decode auth session response: %w", err)
	}
	return sessionResponse{HTTPStatus: resp.StatusCode, Body: body}, nil
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
	return responseMeta{StatusCode: resp.StatusCode}, raw, nil
}

func apiURL(base string, parts ...string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(append([]string{u.Path}, parts...)...)
	return u.String(), nil
}

func authorizationHeader(cfg config) (string, error) {
	switch cfg.authMode {
	case "ldap":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.ldapUsername+":"+cfg.ldapPassword)), nil
	case "bearer":
		return "Bearer " + cfg.bearerToken, nil
	default:
		return "", fmt.Errorf("unsupported auth mode")
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
	checkedAt, err := validateCheckedAt(out.CheckedAt)
	if err != nil {
		return err
	}
	if err := validateRequest(out.Request, checkedAt); err != nil {
		return err
	}
	if out.Status.HTTPStatus != http.StatusOK {
		return fmt.Errorf("status.http_status must be 200")
	}
	if !out.Status.Configured {
		return fmt.Errorf("status.configured must be true")
	}
	if !validBackendMode(out.Status.Mode) {
		return fmt.Errorf("status.mode is unsupported")
	}
	if err := validateSupportedModes(out.Status); err != nil {
		return err
	}
	if out.Request.ExpectedBackendMode != "" && out.Status.Mode != out.Request.ExpectedBackendMode {
		if len(out.Status.SupportedModes) == 0 ||
			out.Request.ExpectedBackendMode == string(api.DiagnosisAuthStatusResponseModeNone) ||
			!slices.Contains(out.Status.SupportedModes, out.Request.ExpectedBackendMode) {
			return fmt.Errorf("status.mode = %q, want %s", out.Status.Mode, out.Request.ExpectedBackendMode)
		}
	}
	for _, mode := range out.Request.RequiredSupportedModes {
		if !slices.Contains(out.Status.SupportedModes, mode) {
			return fmt.Errorf("status.supported_modes missing required mode %q", mode)
		}
	}
	statusModes := acceptedStatusModes(out.Status)
	if out.Request.AuthMode == "ldap" && !slices.Contains(statusModes, "ldap") {
		return fmt.Errorf("status accepted modes = %q, want ldap for ldap auth mode", strings.Join(statusModes, ","))
	}
	if err := validateStatusRoleMapping(out); err != nil {
		return err
	}
	if out.Check.HTTPStatus != http.StatusOK {
		return fmt.Errorf("check.http_status must be 200")
	}
	if err := validateCheckMode(out); err != nil {
		return err
	}
	if strings.TrimSpace(out.Check.Subject) == "" || out.Check.Subject != strings.TrimSpace(out.Check.Subject) {
		return fmt.Errorf("check.subject must be non-empty without surrounding whitespace")
	}
	if len(out.Check.Roles) == 0 || out.Check.RoleCount != len(out.Check.Roles) {
		return fmt.Errorf("check.roles must be non-empty and match role_count")
	}
	if !out.Check.RoleAuthorized {
		return fmt.Errorf("check.role_authorized must be true")
	}
	for _, role := range out.Check.Roles {
		if role != "owner" && role != "admin" {
			return fmt.Errorf("check.roles contains unsupported role")
		}
	}
	if err := validateSessionProof(out); err != nil {
		return err
	}
	wantEvidence := authEvidence(out)
	if out.Evidence != wantEvidence {
		return fmt.Errorf("evidence must be %q", wantEvidence)
	}
	return nil
}

func validateSessionProof(out proof) error {
	if out.Request.IssueSession && out.Session == nil {
		return fmt.Errorf("session proof is required when request.issue_session is true")
	}
	if !out.Request.IssueSession && out.Session != nil {
		return fmt.Errorf("session proof is present while request.issue_session is false")
	}
	if out.Session == nil {
		return nil
	}
	session := out.Session
	if session.HTTPStatus != http.StatusCreated {
		return fmt.Errorf("session.http_status must be 201")
	}
	if !session.TokenPresent {
		return fmt.Errorf("session.token_present must be true")
	}
	if session.Mode == "" || session.Mode != strings.TrimSpace(session.Mode) || !validSupportedMode(session.Mode) {
		return fmt.Errorf("session.mode is unsupported")
	}
	if out.Request.AuthMode == "ldap" && session.Mode != "ldap" {
		return fmt.Errorf("session.mode = %q, want ldap for ldap auth mode", session.Mode)
	}
	if strings.TrimSpace(session.Subject) == "" || session.Subject != strings.TrimSpace(session.Subject) {
		return fmt.Errorf("session.subject must be non-empty without surrounding whitespace")
	}
	if session.Subject != out.Check.Subject {
		return fmt.Errorf("session.subject must match check.subject")
	}
	if len(session.Roles) == 0 || session.RoleCount != len(session.Roles) {
		return fmt.Errorf("session.roles must be non-empty and match role_count")
	}
	if !session.RoleAuthorized {
		return fmt.Errorf("session.role_authorized must be true")
	}
	for _, role := range session.Roles {
		if role != "owner" && role != "admin" {
			return fmt.Errorf("session.roles contains unsupported role")
		}
	}
	if !slices.Equal(sortedStrings(session.Roles), sortedStrings(out.Check.Roles)) {
		return fmt.Errorf("session.roles must match check.roles")
	}
	checkedAt, err := validateCheckedAt(session.CheckedAt)
	if err != nil {
		return fmt.Errorf("session.checked_at is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("session.expires_at must be RFC3339: %w", err)
	}
	if expiresAt.UTC().Format(time.RFC3339Nano) != session.ExpiresAt {
		return fmt.Errorf("session.expires_at must be canonical UTC RFC3339")
	}
	if !expiresAt.After(checkedAt) {
		return fmt.Errorf("session.expires_at must be after session.checked_at")
	}
	return nil
}

func validateSupportedModes(status authStatusProof) error {
	seen := make(map[string]struct{}, len(status.SupportedModes))
	for _, mode := range status.SupportedModes {
		if mode != strings.TrimSpace(mode) || !validSupportedMode(mode) {
			return fmt.Errorf("status.supported_modes contains unsupported mode")
		}
		if _, ok := seen[mode]; ok {
			return fmt.Errorf("status.supported_modes contains duplicate mode")
		}
		seen[mode] = struct{}{}
	}
	if len(status.SupportedModes) > 0 &&
		status.Mode != string(api.DiagnosisAuthStatusResponseModeNone) &&
		status.Mode != string(api.DiagnosisAuthStatusResponseModeUnknown) &&
		!slices.Contains(status.SupportedModes, status.Mode) {
		return fmt.Errorf("status.supported_modes must include status.mode")
	}
	return nil
}

func validateStatusRoleMapping(out proof) error {
	mapping := out.Status.RoleMapping
	if mapping == nil {
		switch out.Request.AuthMode {
		case "ldap":
			return fmt.Errorf("status.role_mapping is required for %s auth mode", out.Request.AuthMode)
		default:
			return nil
		}
	}
	if mapping.OwnerMappingCount < 0 || mapping.AdminMappingCount < 0 {
		return fmt.Errorf("status.role_mapping counts must be non-negative")
	}
	seenDefaultRoles := make(map[string]struct{}, len(mapping.DefaultRoles))
	for _, role := range mapping.DefaultRoles {
		if role != strings.TrimSpace(role) || (role != "owner" && role != "admin") {
			return fmt.Errorf("status.role_mapping.default_roles contains unsupported role")
		}
		if _, ok := seenDefaultRoles[role]; ok {
			return fmt.Errorf("status.role_mapping.default_roles contains duplicate role")
		}
		seenDefaultRoles[role] = struct{}{}
	}
	mappingPaths := mapping.OwnerMappingCount + mapping.AdminMappingCount + len(mapping.DefaultRoles)
	if mapping.Configured != (mappingPaths > 0) {
		return fmt.Errorf("status.role_mapping.configured does not match mapping counts")
	}
	if out.Request.AuthMode == "ldap" && !mapping.Configured {
		return fmt.Errorf("status.role_mapping.configured must be true for %s auth mode", out.Request.AuthMode)
	}
	return nil
}

func acceptedStatusModes(status authStatusProof) []string {
	if len(status.SupportedModes) > 0 {
		return append([]string(nil), status.SupportedModes...)
	}
	if status.Mode == "" || status.Mode == string(api.DiagnosisAuthStatusResponseModeNone) {
		return nil
	}
	return []string{status.Mode}
}

func validateCheckMode(out proof) error {
	if out.Check.Mode == "" {
		return nil
	}
	if out.Check.Mode != strings.TrimSpace(out.Check.Mode) || !validSupportedMode(out.Check.Mode) {
		return fmt.Errorf("check.mode is unsupported")
	}
	if len(out.Status.SupportedModes) > 0 && !slices.Contains(out.Status.SupportedModes, out.Check.Mode) {
		return fmt.Errorf("check.mode = %q is absent from status.supported_modes", out.Check.Mode)
	}
	switch out.Request.AuthMode {
	case "ldap":
		if out.Check.Mode != "ldap" {
			return fmt.Errorf("check.mode = %q, want ldap for ldap auth mode", out.Check.Mode)
		}
	}
	return nil
}

func validateBaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("--api-base-url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("--api-base-url must be an absolute URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("--api-base-url must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("--api-base-url must not contain query or fragment")
	}
	return nil
}

func validateCheckedAt(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("checked_at must be non-empty")
	}
	if value != raw {
		return time.Time{}, fmt.Errorf("checked_at must not contain leading or trailing whitespace")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("checked_at must be RFC3339: %w", err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("checked_at must be canonical UTC RFC3339")
	}
	if checkedAt.After(nowUTC()) {
		return time.Time{}, fmt.Errorf("checked_at must not be in the future")
	}
	return checkedAt.UTC(), nil
}

func validateRequest(req proofRequest, checkedAt time.Time) error {
	if err := validateBaseURL(req.APIBaseURL); err != nil {
		return fmt.Errorf("request.api_base_url is invalid: %w", err)
	}
	if req.AuthMode != "ldap" && req.AuthMode != "bearer" {
		return fmt.Errorf("request.auth_mode must be ldap or bearer")
	}
	if req.ExpectedBackendMode != "" && !validBackendMode(req.ExpectedBackendMode) {
		return fmt.Errorf("request.expected_backend_mode is unsupported")
	}
	for _, mode := range req.RequiredSupportedModes {
		if mode != strings.TrimSpace(mode) || !validSupportedMode(mode) {
			return fmt.Errorf("request.required_supported_modes contains unsupported mode")
		}
	}
	timeout, err := time.ParseDuration(req.Timeout)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("request.timeout must be a positive duration")
	}
	if checkedAt.IsZero() {
		return fmt.Errorf("checked_at must be valid")
	}
	return nil
}

func validBackendMode(value string) bool {
	return slices.Contains([]string{"ldap", "static", "oidc", "unknown", "none"}, value)
}

func validSupportedMode(value string) bool {
	return value != string(api.DiagnosisAuthStatusResponseModeNone) && validBackendMode(value)
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

func authEvidence(out proof) string {
	roles := append([]string(nil), out.Check.Roles...)
	slices.Sort(roles)
	sessionSuffix := ""
	if out.Session != nil {
		sessionSuffix = ":session"
	}
	if len(out.Status.SupportedModes) > 0 || out.Check.Mode != "" {
		statusModes := acceptedStatusModes(out.Status)
		slices.Sort(statusModes)
		checkMode := out.Check.Mode
		if checkMode == "" {
			checkMode = "unknown"
		}
		return fmt.Sprintf(
			"diagnosis_auth_check success:%s:%s:%s:%s:%s%s",
			out.Status.Mode,
			checkMode,
			out.Request.AuthMode,
			strings.Join(statusModes, ","),
			strings.Join(roles, ","),
			sessionSuffix,
		)
	}
	return fmt.Sprintf("diagnosis_auth_check success:%s:%s:%s%s", out.Status.Mode, out.Request.AuthMode, strings.Join(roles, ","), sessionSuffix)
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	slices.Sort(out)
	return out
}
