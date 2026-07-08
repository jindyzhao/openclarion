// Package fake provides a deterministic in-memory CMDBProvider for usecase and
// workflow tests.
package fake

import (
	"context"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Provider records lookup requests and returns a configured result or error.
type Provider struct {
	mu       sync.Mutex
	result   ports.CMDBLookupResult
	err      error
	requests []ports.CMDBLookupRequest
}

var _ ports.CMDBProvider = (*Provider)(nil)

// New constructs a Provider that returns result for every lookup.
func New(result ports.CMDBLookupResult) *Provider {
	return &Provider{result: cloneResult(result)}
}

// NewError constructs a Provider that returns err for every lookup. A nil err
// is allowed and behaves like New with an empty result.
func NewError(err error) *Provider {
	return &Provider{err: err}
}

// LookupResource records a deep copy of req and returns the configured result.
func (p *Provider) LookupResource(ctx context.Context, req ports.CMDBLookupRequest) (ports.CMDBLookupResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.CMDBLookupResult{}, err
	}
	if p == nil {
		return ports.CMDBLookupResult{}, nil
	}
	p.mu.Lock()
	p.requests = append(p.requests, cloneRequest(req))
	result := cloneResult(p.result)
	err := p.err
	p.mu.Unlock()
	if err != nil {
		return ports.CMDBLookupResult{}, err
	}
	return result, nil
}

// Requests returns independent copies of recorded lookup requests.
func (p *Provider) Requests() []ports.CMDBLookupRequest {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ports.CMDBLookupRequest, len(p.requests))
	for i := range p.requests {
		out[i] = cloneRequest(p.requests[i])
	}
	return out
}

func cloneRequest(in ports.CMDBLookupRequest) ports.CMDBLookupRequest {
	return ports.CMDBLookupRequest{Labels: cloneStringMap(in.Labels)}
}

func cloneResult(in ports.CMDBLookupResult) ports.CMDBLookupResult {
	return ports.CMDBLookupResult{
		Found:    in.Found,
		Resource: cloneResource(in.Resource),
	}
}

func cloneResource(in ports.CMDBResource) ports.CMDBResource {
	return ports.CMDBResource{
		ID:         in.ID,
		Kind:       in.Kind,
		Name:       in.Name,
		Owners:     append([]ports.CMDBOwner(nil), in.Owners...),
		Topology:   append([]ports.CMDBTopologyLink(nil), in.Topology...),
		Attributes: cloneStringMap(in.Attributes),
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
