// Package fake provides a deterministic in-memory IMProvider for
// workflow and usecase tests.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Result is one scripted provider outcome.
type Result struct {
	Delivery ports.IMDelivery
	Err      error
}

// Provider is a deterministic, concurrency-safe IMProvider. Each
// idempotency key owns an independent script; after a script is
// exhausted, the provider repeats the last result to keep retry tests
// stable.
type Provider struct {
	mu       sync.Mutex
	scripts  map[string][]Result
	calls    map[string]int
	requests map[string][]ports.IMNotification
}

// Compile-time assertion that *Provider satisfies the port.
var _ ports.IMProvider = (*Provider)(nil)

// New constructs a Provider from scripts keyed by
// ports.IMNotification.IdempotencyKey. Scripts are deep-copied so
// caller mutations after construction cannot change provider behavior.
func New(scripts map[string][]Result) *Provider {
	return &Provider{
		scripts:  cloneScripts(scripts),
		calls:    map[string]int{},
		requests: map[string][]ports.IMNotification{},
	}
}

// SendNotification records req and returns the next scripted Result
// for req.IdempotencyKey.
func (p *Provider) SendNotification(ctx context.Context, req ports.IMNotification) (ports.IMDelivery, error) {
	if err := ctx.Err(); err != nil {
		return ports.IMDelivery{}, err
	}
	if req.IdempotencyKey == "" {
		return ports.IMDelivery{}, fmt.Errorf("fake im: idempotency key must be non-empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	script, ok := p.scripts[req.IdempotencyKey]
	if !ok || len(script) == 0 {
		return ports.IMDelivery{}, fmt.Errorf("fake im: no script for idempotency key %q", req.IdempotencyKey)
	}
	p.requests[req.IdempotencyKey] = append(p.requests[req.IdempotencyKey], cloneNotification(req))

	call := p.calls[req.IdempotencyKey]
	p.calls[req.IdempotencyKey] = call + 1
	if call >= len(script) {
		call = len(script) - 1
	}

	result := script[call]
	if result.Err != nil {
		return ports.IMDelivery{}, result.Err
	}
	return cloneDelivery(result.Delivery), nil
}

// Calls returns how many SendNotification calls were made for the key.
func (p *Provider) Calls(idempotencyKey string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[idempotencyKey]
}

// Requests returns deep-copied requests recorded for the key.
func (p *Provider) Requests(idempotencyKey string) []ports.IMNotification {
	p.mu.Lock()
	defer p.mu.Unlock()
	in := p.requests[idempotencyKey]
	if in == nil {
		return nil
	}
	out := make([]ports.IMNotification, len(in))
	copy(out, in)
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
				Delivery: cloneDelivery(result.Delivery),
				Err:      result.Err,
			}
		}
		out[key] = copied
	}
	return out
}

func cloneNotification(in ports.IMNotification) ports.IMNotification {
	return in
}

func cloneDelivery(in ports.IMDelivery) ports.IMDelivery {
	return ports.IMDelivery{
		ProviderMessageID: in.ProviderMessageID,
		Status:            in.Status,
		Raw:               cloneRawMessage(in.Raw),
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
