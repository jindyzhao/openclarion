// Package fake provides a deterministic in-memory AuthProvider for tests.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Result is one scripted authentication outcome.
type Result struct {
	Principal ports.AuthPrincipal
	Err       error
}

// Provider is a deterministic, concurrency-safe AuthProvider. Each bearer token
// owns an independent script; after a script is exhausted, the provider repeats
// the last result to keep retry tests stable.
type Provider struct {
	mu       sync.Mutex
	scripts  map[string][]Result
	calls    map[string]int
	requests []string
}

var _ ports.AuthProvider = (*Provider)(nil)

// New constructs a Provider from scripts keyed by bearer token.
func New(scripts map[string][]Result) *Provider {
	return &Provider{
		scripts: cloneScripts(scripts),
		calls:   map[string]int{},
	}
}

// AuthenticateBearer records bearerToken and returns the next scripted Result.
func (p *Provider) AuthenticateBearer(ctx context.Context, bearerToken string) (ports.AuthPrincipal, error) {
	if err := ctx.Err(); err != nil {
		return ports.AuthPrincipal{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	script, ok := p.scripts[bearerToken]
	if !ok || len(script) == 0 {
		return ports.AuthPrincipal{}, fmt.Errorf("fake auth: no script for bearer token")
	}
	p.requests = append(p.requests, bearerToken)
	call := p.calls[bearerToken]
	p.calls[bearerToken] = call + 1
	if call >= len(script) {
		call = len(script) - 1
	}
	result := script[call]
	if result.Err != nil {
		return ports.AuthPrincipal{}, result.Err
	}
	return clonePrincipal(result.Principal), nil
}

// Calls returns how many AuthenticateBearer calls were made for bearerToken.
func (p *Provider) Calls(bearerToken string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[bearerToken]
}

// Requests returns the recorded bearer tokens in call order.
func (p *Provider) Requests() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.requests))
	copy(out, p.requests)
	return out
}

func cloneScripts(in map[string][]Result) map[string][]Result {
	if in == nil {
		return nil
	}
	out := make(map[string][]Result, len(in))
	for key, script := range in {
		if script == nil {
			out[key] = nil
			continue
		}
		copied := make([]Result, len(script))
		for i, result := range script {
			copied[i] = Result{
				Principal: clonePrincipal(result.Principal),
				Err:       result.Err,
			}
		}
		out[key] = copied
	}
	return out
}

func clonePrincipal(in ports.AuthPrincipal) ports.AuthPrincipal {
	return ports.AuthPrincipal{
		Subject: in.Subject,
		Roles:   append([]ports.AuthRole(nil), in.Roles...),
		Claims:  cloneRawMessage(in.Claims),
	}
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
