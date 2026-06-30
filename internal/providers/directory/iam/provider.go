// Package iam provides an Ops IAM-backed DirectoryProvider implementation.
package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultScope    = "directory:read"
	defaultPageSize = 100
	maxPageSize     = 500
	discoveryPath   = "/.well-known/openid-configuration"
	// #nosec G101 -- OAuth endpoint path constant.
	tokenPath            = "/api/login/oauth/access_token"
	usersPath            = "/api/directory/users"
	departmentsPath      = "/api/directory/departments"
	maxResponseBodyBytes = 4 << 20
)

// Config holds Ops IAM directory provider configuration.
type Config struct {
	IssuerURL        string
	DirectoryBaseURL string
	TokenURL         string
	ClientID         string
	ClientSecret     string
	Scopes           []string
	HTTPClient       *http.Client
}

// Provider reads normalized directory records from Ops IAM.
type Provider struct {
	client          *http.Client
	directoryURL    *url.URL
	tokenConfig     clientcredentials.Config
	tokenHTTPClient *http.Client
	tokenMu         sync.Mutex
	cachedToken     *oauth2.Token
}

var _ ports.DirectoryProvider = (*Provider)(nil)

// NewProvider constructs an Ops IAM directory provider. It discovers the token
// endpoint from the issuer unless TokenURL is explicitly configured.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	issuer, err := normalizeBaseURL(cfg.IssuerURL, "issuer url")
	if err != nil {
		return nil, err
	}
	directoryBase := issuer
	if strings.TrimSpace(cfg.DirectoryBaseURL) != "" {
		directoryBase, err = normalizeBaseURL(cfg.DirectoryBaseURL, "directory base url")
		if err != nil {
			return nil, err
		}
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		return nil, fmt.Errorf("iam directory: client id must be non-empty")
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		return nil, fmt.Errorf("iam directory: client secret must be non-empty")
	}
	baseClient := cfg.HTTPClient
	if baseClient == nil {
		baseClient = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	tokenURL := strings.TrimSpace(cfg.TokenURL)
	if tokenURL == "" {
		tokenURL, err = discoverTokenEndpoint(ctx, baseClient, issuer)
		if err != nil {
			return nil, err
		}
	}
	normalizedTokenURL, err := normalizeTokenEndpointURL(tokenURL, "token url")
	if err != nil {
		return nil, err
	}
	tokenConfig := clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     normalizedTokenURL.String(),
		Scopes:       normalizedScopes(cfg.Scopes),
	}
	return &Provider{
		client:          baseClient,
		directoryURL:    directoryBase,
		tokenConfig:     tokenConfig,
		tokenHTTPClient: baseClient,
	}, nil
}

// ListDepartments returns one Ops IAM directory department page.
func (p *Provider) ListDepartments(ctx context.Context, req ports.DirectoryListRequest) (ports.DirectoryDepartmentPage, error) {
	if p == nil || p.client == nil || p.directoryURL == nil {
		return ports.DirectoryDepartmentPage{}, fmt.Errorf("iam directory: provider is not configured")
	}
	envelope, requestedPage, err := getDirectoryPage[iamDepartment](ctx, p, departmentsPath, req)
	if err != nil {
		return ports.DirectoryDepartmentPage{}, err
	}
	out := make([]ports.DirectoryDepartmentProjection, len(envelope.Data.Items))
	for i, item := range envelope.Data.Items {
		out[i] = ports.DirectoryDepartmentProjection{
			ExternalID:       item.ID,
			ParentExternalID: item.ParentID,
			Name:             item.Name,
			DisplayName:      item.DisplayName,
			Path:             item.Path,
			ParentPath:       item.ParentPath,
			Level:            item.Level,
			Source:           item.Source,
			MemberCount:      item.MemberCount,
			SourceUpdatedAt:  item.UpdatedAt,
		}
	}
	return ports.DirectoryDepartmentPage{
		Departments: out,
		NextCursor:  nextPageCursor(envelope.Data, requestedPage),
	}, nil
}

// ListUsers returns one Ops IAM directory user page.
func (p *Provider) ListUsers(ctx context.Context, req ports.DirectoryListRequest) (ports.DirectoryUserPage, error) {
	if p == nil || p.client == nil || p.directoryURL == nil {
		return ports.DirectoryUserPage{}, fmt.Errorf("iam directory: provider is not configured")
	}
	envelope, requestedPage, err := getDirectoryPage[iamUser](ctx, p, usersPath, req)
	if err != nil {
		return ports.DirectoryUserPage{}, err
	}
	out := make([]ports.DirectoryUserProjection, len(envelope.Data.Items))
	for i, item := range envelope.Data.Items {
		if item.Active == nil {
			return ports.DirectoryUserPage{}, fmt.Errorf("iam directory: user record %d missing required active field", i)
		}
		out[i] = ports.DirectoryUserProjection{
			Subject:               item.Subject,
			ExternalID:            item.WeComUserID,
			Username:              firstNonEmpty(item.PreferredUsername, item.Name),
			DisplayName:           item.DisplayName,
			Email:                 item.Email,
			JobTitle:              item.JobTitle,
			Department:            item.Department,
			Section:               item.Section,
			DepartmentPath:        item.DepartmentPath,
			DepartmentPaths:       append([]string(nil), item.DepartmentPaths...),
			DepartmentExternalIDs: mergedStringKeys(item.DepartmentIDs, item.DepartmentExternalIDs),
			Active:                *item.Active,
			SourceUpdatedAt:       item.UpdatedAt,
		}
		if out[i].ExternalID == "" {
			out[i].ExternalID = item.Subject
		}
	}
	return ports.DirectoryUserPage{
		Users:      out,
		NextCursor: nextPageCursor(envelope.Data, requestedPage),
	}, nil
}

func getDirectoryPage[T any](
	ctx context.Context,
	provider *Provider,
	pathname string,
	req ports.DirectoryListRequest,
) (directoryEnvelope[T], int, error) {
	var envelope directoryEnvelope[T]
	page, err := pageFromCursor(req.Cursor)
	if err != nil {
		return envelope, 0, err
	}
	pageSize, err := normalizePageSize(req.PageSize)
	if err != nil {
		return envelope, 0, err
	}
	endpoint := appendPath(provider.directoryURL, pathname)
	query := endpoint.Query()
	query.Set("p", strconv.Itoa(page))
	query.Set("pageSize", strconv.Itoa(pageSize))
	query.Set("include_disabled", "true")
	if req.UpdatedAfter != nil && !req.UpdatedAfter.IsZero() {
		query.Set("updated_after", req.UpdatedAfter.UTC().Format(time.RFC3339Nano))
	}
	endpoint.RawQuery = query.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return envelope, 0, fmt.Errorf("iam directory: build request")
	}
	httpReq.Header.Set("Accept", "application/json")
	authorization, err := provider.authorizationHeader(ctx)
	if err != nil {
		return envelope, 0, err
	}
	httpReq.Header.Set("Authorization", authorization)
	resp, err := provider.client.Do(httpReq)
	if err != nil {
		return envelope, 0, fmt.Errorf("iam directory: request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return envelope, 0, fmt.Errorf("iam directory: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return envelope, 0, fmt.Errorf("iam directory: provider returned HTTP %d", resp.StatusCode)
	}
	if err := strictjson.RejectDuplicateObjectKeys(body); err != nil {
		return envelope, 0, fmt.Errorf("iam directory: response JSON is ambiguous: %w", err)
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return envelope, 0, fmt.Errorf("iam directory: decode response: %w", err)
	}
	if envelope.Status != "ok" {
		return envelope, 0, fmt.Errorf("iam directory: provider status is not ok")
	}
	return envelope, page, nil
}

func (p *Provider) authorizationHeader(ctx context.Context) (string, error) {
	token, err := p.token(ctx)
	if err != nil {
		return "", fmt.Errorf("iam directory: obtain access token: %w", err)
	}
	accessToken := strings.TrimSpace(token.AccessToken)
	if accessToken == "" {
		return "", fmt.Errorf("iam directory: token endpoint returned an empty access token")
	}
	return "Bearer " + accessToken, nil
}

func (p *Provider) token(ctx context.Context) (*oauth2.Token, error) {
	p.tokenMu.Lock()
	if p.cachedToken != nil && p.cachedToken.Valid() {
		token := p.cachedToken
		p.tokenMu.Unlock()
		return token, nil
	}
	p.tokenMu.Unlock()

	tokenCtx := context.WithValue(ctx, oauth2.HTTPClient, p.tokenHTTPClient)
	token, err := p.tokenConfig.Token(tokenCtx)
	if err != nil {
		return nil, err
	}
	p.tokenMu.Lock()
	p.cachedToken = token
	p.tokenMu.Unlock()
	return token, nil
}

type directoryEnvelope[T any] struct {
	Status string           `json:"status"`
	Msg    string           `json:"msg"`
	Data   directoryPage[T] `json:"data"`
}

type directoryPage[T any] struct {
	Items    []T  `json:"items"`
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"pageSize"`
	HasMore  bool `json:"hasMore"`
}

func pageFromCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 1, nil
	}
	page, err := strconv.Atoi(cursor)
	if err != nil || page < 1 {
		return 0, fmt.Errorf("iam directory: cursor must be a positive page number")
	}
	return page, nil
}

func normalizePageSize(pageSize int) (int, error) {
	if pageSize == 0 {
		return defaultPageSize, nil
	}
	if pageSize < 1 || pageSize > maxPageSize {
		return 0, fmt.Errorf("iam directory: page size must be between 1 and %d", maxPageSize)
	}
	return pageSize, nil
}

func nextPageCursor[T any](page directoryPage[T], requestedPage int) string {
	if !page.HasMore {
		return ""
	}
	currentPage := page.Page
	if currentPage < 1 {
		currentPage = requestedPage
	}
	return strconv.Itoa(currentPage + 1)
}

type iamDepartment struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	DisplayName string     `json:"displayName"`
	Path        string     `json:"path"`
	ParentID    string     `json:"parent_id"`
	ParentPath  string     `json:"parent_path"`
	Level       int        `json:"level"`
	Source      string     `json:"source"`
	MemberCount int        `json:"member_count"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

type iamUser struct {
	Subject               string     `json:"sub"`
	Name                  string     `json:"name"`
	PreferredUsername     string     `json:"preferred_username"`
	DisplayName           string     `json:"displayName"`
	Email                 string     `json:"email"`
	JobTitle              string     `json:"job_title"`
	Department            string     `json:"department"`
	Section               string     `json:"section"`
	DepartmentPath        string     `json:"department_path"`
	DepartmentPaths       []string   `json:"department_paths"`
	DepartmentIDs         []string   `json:"department_ids"`
	DepartmentExternalIDs []string   `json:"department_external_ids"`
	WeComUserID           string     `json:"wecom_userid"`
	Active                *bool      `json:"active"`
	UpdatedAt             *time.Time `json:"updated_at"`
}

type discoveryDocument struct {
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
}

func discoverTokenEndpoint(ctx context.Context, client *http.Client, issuer *url.URL) (string, error) {
	endpoint := appendPath(issuer, discoveryPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("iam directory: build discovery request")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("iam directory: discover token endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return "", fmt.Errorf("iam directory: read discovery response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("iam directory: discovery returned HTTP %d", resp.StatusCode)
	}
	if err := strictjson.RejectDuplicateObjectKeys(body); err != nil {
		return "", fmt.Errorf("iam directory: discovery JSON is ambiguous: %w", err)
	}
	var doc discoveryDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("iam directory: decode discovery response: %w", err)
	}
	if strings.TrimSpace(doc.Issuer) == "" {
		return "", fmt.Errorf("iam directory: discovery issuer must be non-empty")
	}
	if doc.Issuer != discoveryIssuerPrefix(issuer) {
		return "", fmt.Errorf("iam directory: discovery issuer mismatch")
	}
	tokenEndpoint := strings.TrimSpace(doc.TokenEndpoint)
	if tokenEndpoint == "" {
		return appendPath(issuer, tokenPath).String(), nil
	}
	normalizedTokenEndpoint, err := normalizeTokenEndpointURL(tokenEndpoint, "token endpoint")
	if err != nil {
		return "", err
	}
	return normalizedTokenEndpoint.String(), nil
}

func discoveryIssuerPrefix(issuer *url.URL) string {
	out := *issuer
	out.Path = strings.TrimRight(out.Path, "/")
	out.RawQuery = ""
	out.Fragment = ""
	return out.String()
}

func normalizeBaseURL(raw, label string) (*url.URL, error) {
	parsed, err := normalizeAbsoluteHTTPURL(raw, label)
	if err != nil {
		return nil, err
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func normalizeTokenEndpointURL(raw, label string) (*url.URL, error) {
	parsed, err := normalizeAbsoluteHTTPURL(raw, label)
	if err != nil {
		return nil, err
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("iam directory: %s must not include a fragment", label)
	}
	return parsed, nil
}

func normalizeAbsoluteHTTPURL(raw, label string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("iam directory: %s must be non-empty", label)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("iam directory: parse %s", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("iam directory: %s scheme must be http or https", label)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("iam directory: %s must be absolute", label)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("iam directory: %s must not include userinfo", label)
	}
	return parsed, nil
}

func normalizedScopes(in []string) []string {
	if len(in) == 0 {
		return []string{defaultScope}
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, scope := range in {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return []string{defaultScope}
	}
	return out
}

func appendPath(base *url.URL, suffix string) *url.URL {
	out := *base
	out.Path = strings.TrimRight(out.Path, "/") + suffix
	out.RawQuery = ""
	out.Fragment = ""
	return &out
}

func readLimitedBody(body io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	limited := io.LimitReader(body, maxResponseBodyBytes+1)
	if _, err := buf.ReadFrom(limited); err != nil {
		return nil, err
	}
	if buf.Len() > maxResponseBodyBytes {
		return nil, fmt.Errorf("body exceeds %d bytes", maxResponseBodyBytes)
	}
	return buf.Bytes(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mergedStringKeys(primary []string, aliases ...[]string) []string {
	total := len(primary)
	for _, alias := range aliases {
		total += len(alias)
	}
	out := make([]string, 0, total)
	seen := map[string]struct{}{}
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	add(primary)
	for _, alias := range aliases {
		add(alias)
	}
	return out
}
