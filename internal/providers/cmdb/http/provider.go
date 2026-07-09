// Package httpcmdb provides a generic JSON-over-HTTP CMDBProvider.
package httpcmdb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/providers/cmdb/internal/cmdbnorm"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout      = 10 * time.Second
	maxRequestLabels    = 128
	maxResponseBodySize = 1 << 20
)

// Config holds generic HTTP CMDB provider configuration.
type Config struct {
	URL         string
	BearerToken string
	HTTPClient  *http.Client
}

// Provider looks up CMDB resources through a deployment-owned HTTP endpoint.
type Provider struct {
	endpoint    string
	bearerToken string
	httpClient  *http.Client
}

var _ ports.CMDBProvider = (*Provider)(nil)

// NewProvider constructs a generic HTTP CMDB provider.
func NewProvider(cfg Config) (*Provider, error) {
	endpoint, err := normalizeEndpoint(cfg.URL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout, Transport: correlation.RoundTripper(nil)}
	}
	return &Provider{
		endpoint:    endpoint,
		bearerToken: strings.TrimSpace(cfg.BearerToken),
		httpClient:  client,
	}, nil
}

// LookupResource implements ports.CMDBProvider.
func (p *Provider) LookupResource(ctx context.Context, req ports.CMDBLookupRequest) (ports.CMDBLookupResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.CMDBLookupResult{}, err
	}
	if p == nil || p.httpClient == nil || p.endpoint == "" {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: provider is not configured")
	}
	labels, err := normalizeLookupLabels(req.Labels)
	if err != nil {
		return ports.CMDBLookupResult{}, err
	}
	raw, err := json.Marshal(lookupRequest{Labels: labels})
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: marshal lookup request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: build lookup request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.bearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: lookup request failed: %w", err)
	}
	defer resp.Body.Close()
	rawBody, err := readBoundedBody(resp.Body)
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: lookup returned HTTP %d", resp.StatusCode)
	}
	result, err := decodeLookupResponse(rawBody)
	if err != nil {
		return ports.CMDBLookupResult{}, err
	}
	return result, nil
}

type lookupRequest struct {
	Labels map[string]string `json:"labels"`
}

type lookupResponse struct {
	Found    bool              `json:"found"`
	Resource *resourceResponse `json:"resource,omitempty"`
}

type resourceResponse struct {
	ID         string                 `json:"id"`
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Owners     []ownerResponse        `json:"owners,omitempty"`
	Topology   []topologyLinkResponse `json:"topology,omitempty"`
	Attributes map[string]string      `json:"attributes,omitempty"`
}

type ownerResponse struct {
	Subject string `json:"subject,omitempty"`
	Team    string `json:"team,omitempty"`
	Role    string `json:"role,omitempty"`
}

type topologyLinkResponse struct {
	Relation   string `json:"relation"`
	TargetID   string `json:"target_id"`
	TargetKind string `json:"target_kind"`
	TargetName string `json:"target_name,omitempty"`
}

func normalizeEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("http cmdb: URL must be valid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("http cmdb: URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("http cmdb: URL host must be non-empty")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("http cmdb: URL must not include userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("http cmdb: URL must not include query or fragment")
	}
	return parsed.String(), nil
}

func normalizeLookupLabels(in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return map[string]string{}, nil
	}
	if len(in) > maxRequestLabels {
		return nil, fmt.Errorf("http cmdb: lookup labels exceed %d entries", maxRequestLabels)
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		k, err := cmdbnorm.NormalizeRequiredString("label key", key)
		if err != nil {
			return nil, fmt.Errorf("http cmdb: %w", err)
		}
		v, err := cmdbnorm.NormalizeOptionalString("label value", value)
		if err != nil {
			return nil, fmt.Errorf("http cmdb: %w", err)
		}
		out[k] = v
	}
	return out, nil
}

func readBoundedBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxResponseBodySize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBodySize {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBodySize)
	}
	return raw, nil
}

func decodeLookupResponse(raw []byte) (ports.CMDBLookupResult, error) {
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: response is not strict JSON: %w", err)
	}
	var decoded lookupResponse
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decoded); err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: decode response: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: response has trailing JSON values")
	} else if !errors.Is(err, io.EOF) {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: decode response trailer: %w", err)
	}
	if !decoded.Found {
		if decoded.Resource != nil {
			return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: found=false response must not include resource")
		}
		return ports.CMDBLookupResult{}, nil
	}
	if decoded.Resource == nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: found=true response must include resource")
	}
	resource, err := cmdbnorm.NormalizeResource(decoded.Resource.toPort())
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("http cmdb: invalid resource: %w", err)
	}
	return ports.CMDBLookupResult{Found: true, Resource: resource}, nil
}

func (r resourceResponse) toPort() ports.CMDBResource {
	return ports.CMDBResource{
		ID:         r.ID,
		Kind:       r.Kind,
		Name:       r.Name,
		Owners:     ownersToPort(r.Owners),
		Topology:   topologyToPort(r.Topology),
		Attributes: cmdbnorm.CloneStringMap(r.Attributes),
	}
}

func ownersToPort(in []ownerResponse) []ports.CMDBOwner {
	out := make([]ports.CMDBOwner, len(in))
	for i, owner := range in {
		out[i] = ports.CMDBOwner{
			Subject: owner.Subject,
			Team:    owner.Team,
			Role:    owner.Role,
		}
	}
	return out
}

func topologyToPort(in []topologyLinkResponse) []ports.CMDBTopologyLink {
	out := make([]ports.CMDBTopologyLink, len(in))
	for i, link := range in {
		out[i] = ports.CMDBTopologyLink{
			Relation:   link.Relation,
			TargetID:   link.TargetID,
			TargetKind: link.TargetKind,
			TargetName: link.TargetName,
		}
	}
	return out
}
