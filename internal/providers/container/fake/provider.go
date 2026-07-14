// Package fake provides a deterministic in-memory ContainerProvider
// for workflow and usecase tests.
package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// Result is one scripted sandbox outcome.
type Result struct {
	Run    ports.ContainerRunResult
	Stream []ports.ContainerStreamChunk
	Err    error
}

// Provider is a deterministic, concurrency-safe ContainerProvider.
// Each invocation ID owns an independent script; after a script is
// exhausted, the provider repeats the last result to keep retry tests
// stable.
type Provider struct {
	mu       sync.Mutex
	scripts  map[string][]Result
	calls    map[string]int
	requests map[string][]ports.ContainerRunRequest
}

var (
	_ ports.ContainerProvider          = (*Provider)(nil)
	_ ports.StreamingContainerProvider = (*Provider)(nil)
)

// New constructs a Provider from scripts keyed by
// ports.ContainerRunRequest.InvocationID. Scripts are deep-copied so
// caller mutations after construction cannot change provider behavior.
func New(scripts map[string][]Result) *Provider {
	return &Provider{
		scripts:  cloneScripts(scripts),
		calls:    map[string]int{},
		requests: map[string][]ports.ContainerRunRequest{},
	}
}

// Run records req and returns the next scripted Result for req.InvocationID.
func (p *Provider) Run(ctx context.Context, req ports.ContainerRunRequest) (ports.ContainerRunResult, error) {
	return p.run(ctx, req, nil)
}

// RunStreaming emits the scripted preview before returning the same validated
// final result as Run.
func (p *Provider) RunStreaming(
	ctx context.Context,
	req ports.ContainerRunRequest,
	onChunk ports.ContainerStreamHandler,
) (ports.ContainerRunResult, error) {
	if onChunk == nil {
		return ports.ContainerRunResult{}, fmt.Errorf("fake container: stream callback is required")
	}
	return p.run(ctx, req, onChunk)
}

func (p *Provider) run(
	ctx context.Context,
	req ports.ContainerRunRequest,
	onChunk ports.ContainerStreamHandler,
) (ports.ContainerRunResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.ContainerRunResult{}, err
	}
	if err := req.Validate(); err != nil {
		return ports.ContainerRunResult{}, err
	}

	p.mu.Lock()
	script, ok := p.scripts[req.InvocationID]
	if !ok || len(script) == 0 {
		p.mu.Unlock()
		return ports.ContainerRunResult{}, fmt.Errorf("fake container: no script for invocation id %q", req.InvocationID)
	}
	p.requests[req.InvocationID] = append(p.requests[req.InvocationID], cloneRequest(req))

	call := p.calls[req.InvocationID]
	p.calls[req.InvocationID] = call + 1
	if call >= len(script) {
		call = len(script) - 1
	}

	result := Result{
		Run:    cloneRunResult(script[call].Run),
		Stream: cloneStreamChunks(script[call].Stream),
		Err:    script[call].Err,
	}
	p.mu.Unlock()
	if result.Err != nil {
		return ports.ContainerRunResult{}, result.Err
	}
	for _, chunk := range result.Stream {
		if err := ctx.Err(); err != nil {
			return ports.ContainerRunResult{}, err
		}
		if onChunk != nil {
			if err := onChunk(chunk); err != nil {
				return ports.ContainerRunResult{}, fmt.Errorf("fake container: stream callback: %w", err)
			}
		}
	}
	out := result.Run
	if err := ports.ValidateContainerRunResult(req, out); err != nil {
		return ports.ContainerRunResult{}, fmt.Errorf("fake container: invalid scripted result for invocation id %q: %w", req.InvocationID, err)
	}
	return out, nil
}

// Calls returns how many Run calls were made for the invocation ID.
func (p *Provider) Calls(invocationID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[invocationID]
}

// Requests returns deep-copied requests recorded for the invocation ID.
func (p *Provider) Requests(invocationID string) []ports.ContainerRunRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	in := p.requests[invocationID]
	if in == nil {
		return nil
	}
	out := make([]ports.ContainerRunRequest, len(in))
	for i, req := range in {
		out[i] = cloneRequest(req)
	}
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
				Run:    cloneRunResult(result.Run),
				Stream: cloneStreamChunks(result.Stream),
				Err:    result.Err,
			}
		}
		out[key] = copied
	}
	return out
}

func cloneStreamChunks(in []ports.ContainerStreamChunk) []ports.ContainerStreamChunk {
	if in == nil {
		return nil
	}
	out := make([]ports.ContainerStreamChunk, len(in))
	copy(out, in)
	return out
}

func cloneRequest(in ports.ContainerRunRequest) ports.ContainerRunRequest {
	return ports.ContainerRunRequest{
		InvocationID: in.InvocationID,
		AgentName:    in.AgentName,
		Evidence:     cloneRawMessage(in.Evidence),
		Conversation: cloneRawMessage(in.Conversation),
		Message:      cloneRawMessage(in.Message),
		Timeout:      in.Timeout,
		OutputMax:    in.OutputMax,
		Network: ports.ContainerNetworkPolicy{
			Mode:          in.Network.Mode,
			AllowedEgress: cloneStringSlice(in.Network.AllowedEgress),
		},
		Credentials: cloneCredentials(in.Credentials),
		Metadata:    cloneStringMap(in.Metadata),
	}
}

func cloneRunResult(in ports.ContainerRunResult) ports.ContainerRunResult {
	return ports.ContainerRunResult{
		InvocationID: in.InvocationID,
		AgentName:    in.AgentName,
		Output:       cloneRawMessage(in.Output),
		ExitCode:     in.ExitCode,
		StartedAt:    in.StartedAt,
		FinishedAt:   in.FinishedAt,
		RuntimeID:    in.RuntimeID,
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

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneCredentials(in []ports.ContainerCredential) []ports.ContainerCredential {
	if in == nil {
		return nil
	}
	out := make([]ports.ContainerCredential, len(in))
	copy(out, in)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
