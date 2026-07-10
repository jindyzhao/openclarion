// Package netbox provides a read-only NetBox REST API CMDBProvider.
package netbox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/observability/correlation"
	"github.com/openclarion/openclarion/internal/providers/cmdb/internal/cmdbnorm"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultTimeout           = 10 * time.Second
	maxResponseBodySize      = 2 << 20
	objectLookupRequestLimit = 2
	maxContactAssignments    = 32
	contactRequestLimit      = maxContactAssignments + 1
	maxCustomAttributeFields = 32
	maxTokenBytes            = 4096
	contactPriorityInactive  = "inactive"
)

// ObjectType selects which NetBox object collection is searched.
type ObjectType string

const (
	// ObjectTypeAuto searches both supported object collections.
	ObjectTypeAuto ObjectType = "auto"
	// ObjectTypeDevice searches dcim devices only.
	ObjectTypeDevice ObjectType = "device"
	// ObjectTypeVirtualMachine searches virtualization virtual machines only.
	ObjectTypeVirtualMachine ObjectType = "virtual_machine"
)

// TokenScheme selects the NetBox Authorization header format.
type TokenScheme string

const (
	// TokenSchemeAuto infers the scheme from the token format.
	TokenSchemeAuto TokenScheme = "auto"
	// TokenSchemeBearer uses current NetBox v2 Bearer authentication.
	TokenSchemeBearer TokenScheme = "bearer"
	// TokenSchemeToken uses legacy NetBox v1 Token authentication.
	TokenSchemeToken TokenScheme = "token"
)

// Config holds NetBox provider configuration. BaseURL may be the NetBox root
// or its /api root. LookupLabel names the alert label whose value is passed to
// LookupFilter. LookupFilter defaults to name and is encoded as a query key.
type Config struct {
	BaseURL               string
	APIToken              string
	TokenScheme           TokenScheme
	LookupLabel           string
	LookupFilter          string
	ObjectType            ObjectType
	AttributeCustomFields []string
	HTTPClient            *http.Client
}

// Provider performs bounded, unique NetBox object and contact lookups.
type Provider struct {
	devicesEndpoint            string
	virtualMachinesEndpoint    string
	contactAssignmentsEndpoint string
	authorization              string
	lookupLabel                string
	lookupFilter               string
	objectType                 ObjectType
	attributeCustomFields      []string
	httpClient                 *http.Client
}

var _ ports.CMDBProvider = (*Provider)(nil)

// NewProvider validates configuration and constructs a reusable NetBox client.
func NewProvider(cfg Config) (*Provider, error) {
	apiRoot, err := normalizeAPIBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	lookupLabel, err := normalizeLookupLabel(cfg.LookupLabel)
	if err != nil {
		return nil, err
	}
	lookupFilter, err := normalizeLookupFilter(cfg.LookupFilter)
	if err != nil {
		return nil, err
	}
	objectType, err := normalizeObjectType(cfg.ObjectType)
	if err != nil {
		return nil, err
	}
	customFields, err := normalizeCustomAttributeFields(cfg.AttributeCustomFields)
	if err != nil {
		return nil, err
	}
	authorization, err := authorizationHeader(cfg.APIToken, cfg.TokenScheme)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout:   defaultTimeout,
			Transport: correlation.RoundTripper(nil),
		}
	}
	client = withoutRedirects(client)
	return &Provider{
		devicesEndpoint:            joinAPIEndpoint(apiRoot, "dcim", "devices"),
		virtualMachinesEndpoint:    joinAPIEndpoint(apiRoot, "virtualization", "virtual-machines"),
		contactAssignmentsEndpoint: joinAPIEndpoint(apiRoot, "tenancy", "contact-assignments"),
		authorization:              authorization,
		lookupLabel:                lookupLabel,
		lookupFilter:               lookupFilter,
		objectType:                 objectType,
		attributeCustomFields:      customFields,
		httpClient:                 client,
	}, nil
}

// LookupResource implements ports.CMDBProvider.
func (p *Provider) LookupResource(ctx context.Context, req ports.CMDBLookupRequest) (ports.CMDBLookupResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.CMDBLookupResult{}, err
	}
	if p == nil || p.httpClient == nil || p.lookupLabel == "" {
		return ports.CMDBLookupResult{}, fmt.Errorf("netbox cmdb: provider is not configured")
	}
	lookupValue, ok := req.Labels[p.lookupLabel]
	if !ok || lookupValue == "" {
		return ports.CMDBLookupResult{}, nil
	}
	lookupValue, err := cmdbnorm.NormalizeRequiredString("lookup label value", lookupValue)
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("netbox cmdb: %w", err)
	}

	matches := make([]matchedResource, 0, 2)
	if p.objectType == ObjectTypeAuto || p.objectType == ObjectTypeDevice {
		device, found, err := p.lookupDevice(ctx, lookupValue)
		if err != nil {
			return ports.CMDBLookupResult{}, err
		}
		if found {
			matches = append(matches, device)
		}
	}
	if p.objectType == ObjectTypeAuto || p.objectType == ObjectTypeVirtualMachine {
		virtualMachine, found, err := p.lookupVirtualMachine(ctx, lookupValue)
		if err != nil {
			return ports.CMDBLookupResult{}, err
		}
		if found {
			matches = append(matches, virtualMachine)
		}
	}
	if len(matches) == 0 {
		return ports.CMDBLookupResult{}, nil
	}
	if len(matches) > 1 {
		return ports.CMDBLookupResult{}, fmt.Errorf("netbox cmdb: lookup matched multiple object types")
	}

	owners, err := p.lookupOwners(ctx, matches[0].objectType, matches[0].resourceID)
	if err != nil {
		return ports.CMDBLookupResult{}, err
	}
	resource := matches[0].resource
	resource.Owners = owners
	resource, err = cmdbnorm.NormalizeResource(resource)
	if err != nil {
		return ports.CMDBLookupResult{}, fmt.Errorf("netbox cmdb: invalid mapped resource: %w", err)
	}
	return ports.CMDBLookupResult{Found: true, Resource: resource}, nil
}

type matchedResource struct {
	objectType string
	resourceID int64
	resource   ports.CMDBResource
}

func (p *Provider) lookupDevice(ctx context.Context, value string) (matchedResource, bool, error) {
	query := p.objectLookupQuery(value, p.objectFields(
		"id", "name", "display", "role", "tenant", "platform", "site",
		"location", "rack", "status", "primary_ip", "cluster", "tags",
	))
	raw, err := p.get(ctx, p.devicesEndpoint, query, "device lookup")
	if err != nil {
		return matchedResource{}, false, err
	}
	results, err := decodePage[deviceResponse](raw, 1)
	if err != nil {
		return matchedResource{}, false, fmt.Errorf("netbox cmdb: decode device lookup: %w", err)
	}
	if len(results) == 0 {
		return matchedResource{}, false, nil
	}
	resource, err := p.mapDevice(results[0])
	if err != nil {
		return matchedResource{}, false, fmt.Errorf("netbox cmdb: map device: %w", err)
	}
	return matchedResource{objectType: "dcim.device", resourceID: results[0].ID, resource: resource}, true, nil
}

func (p *Provider) lookupVirtualMachine(ctx context.Context, value string) (matchedResource, bool, error) {
	query := p.objectLookupQuery(value, p.objectFields(
		"id", "name", "display", "role", "tenant", "platform", "site",
		"cluster", "device", "status", "primary_ip", "tags",
	))
	raw, err := p.get(ctx, p.virtualMachinesEndpoint, query, "virtual machine lookup")
	if err != nil {
		return matchedResource{}, false, err
	}
	results, err := decodePage[virtualMachineResponse](raw, 1)
	if err != nil {
		return matchedResource{}, false, fmt.Errorf("netbox cmdb: decode virtual machine lookup: %w", err)
	}
	if len(results) == 0 {
		return matchedResource{}, false, nil
	}
	resource, err := p.mapVirtualMachine(results[0])
	if err != nil {
		return matchedResource{}, false, fmt.Errorf("netbox cmdb: map virtual machine: %w", err)
	}
	return matchedResource{
		objectType: "virtualization.virtualmachine",
		resourceID: results[0].ID,
		resource:   resource,
	}, true, nil
}

func (p *Provider) objectLookupQuery(value, fields string) url.Values {
	query := url.Values{}
	query.Set(p.lookupFilter, value)
	query.Set("limit", strconv.Itoa(objectLookupRequestLimit))
	query.Set("exclude", "config_context")
	query.Set("fields", fields)
	return query
}

func (p *Provider) objectFields(fields ...string) string {
	if len(p.attributeCustomFields) > 0 {
		fields = append(fields, "custom_fields")
	}
	return strings.Join(fields, ",")
}

func (p *Provider) lookupOwners(ctx context.Context, objectType string, objectID int64) ([]ports.CMDBOwner, error) {
	query := url.Values{}
	query.Set("object_type", objectType)
	query.Set("object_id", strconv.FormatInt(objectID, 10))
	query.Set("limit", strconv.Itoa(contactRequestLimit))
	query.Set("fields", "id,contact,role,priority")
	raw, err := p.get(ctx, p.contactAssignmentsEndpoint, query, "contact assignment lookup")
	if err != nil {
		return nil, err
	}
	assignments, err := decodePage[contactAssignmentResponse](raw, maxContactAssignments)
	if err != nil {
		return nil, fmt.Errorf("netbox cmdb: decode contact assignment lookup: %w", err)
	}
	owners := make([]ports.CMDBOwner, 0, len(assignments))
	for i, assignment := range assignments {
		if assignment.Contact.ID <= 0 {
			return nil, fmt.Errorf("netbox cmdb: contact assignment[%d] has invalid contact id", i)
		}
		priority, err := choiceString(assignment.Priority)
		if err != nil {
			return nil, fmt.Errorf("netbox cmdb: contact assignment[%d] has invalid priority: %w", i, err)
		}
		if priority == contactPriorityInactive {
			continue
		}
		owners = append(owners, ports.CMDBOwner{
			Subject: netBoxObjectID("tenancy.contact", assignment.Contact.ID),
			Team:    firstNonEmpty(assignment.Contact.Name, assignment.Contact.Display),
			Role:    refName(assignment.Role),
		})
	}
	sort.Slice(owners, func(i, j int) bool {
		if owners[i].Subject != owners[j].Subject {
			return owners[i].Subject < owners[j].Subject
		}
		return owners[i].Role < owners[j].Role
	})
	return owners, nil
}

func (p *Provider) get(ctx context.Context, endpoint string, query url.Values, operation string) ([]byte, error) {
	requestURL := endpoint
	if encoded := query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("netbox cmdb: build %s request: %w", operation, err)
	}
	req.Header.Set("Accept", "application/json")
	if p.authorization != "" {
		req.Header.Set("Authorization", p.authorization)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("netbox cmdb: %s request failed: %w", operation, sanitizeRequestError(err))
	}
	defer resp.Body.Close()
	raw, err := readBoundedBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("netbox cmdb: read %s response: %w", operation, err)
	}
	if err := validateAPIVersion(resp.Header.Get("API-Version")); err != nil {
		return nil, fmt.Errorf("netbox cmdb: %s: %w", operation, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("netbox cmdb: %s returned HTTP %d", operation, resp.StatusCode)
	}
	return raw, nil
}
