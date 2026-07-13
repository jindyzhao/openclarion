package docker

import (
	"context"
	"fmt"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// StaticAllowlistEnforcer validates that an allowlist-mode sandbox request is
// a subset of a provider-managed egress boundary, such as an Envoy/Squid proxy
// network or equivalent firewall rules. It does not create that infrastructure;
// it prevents Docker Engine creation when the request drifts beyond it.
type StaticAllowlistEnforcer struct {
	networkMode string
	allowed     map[string]bool
}

var _ EgressEnforcer = (*StaticAllowlistEnforcer)(nil)

// NewStaticAllowlistEnforcer creates an enforcer for an externally configured
// allowlist network. The allowed targets use the same host:port contract as
// ports.ContainerNetworkPolicy.
func NewStaticAllowlistEnforcer(networkMode string, allowedTargets []string) (*StaticAllowlistEnforcer, error) {
	if err := validateAllowlistNetworkMode(networkMode); err != nil {
		return nil, fmt.Errorf("docker egress enforcer: %w", err)
	}
	normalized, err := ports.NormalizeContainerEgressTargets(allowedTargets)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(normalized))
	for _, target := range normalized {
		allowed[target] = true
	}
	return &StaticAllowlistEnforcer{
		networkMode: networkMode,
		allowed:     allowed,
	}, nil
}

// Validate proves that the requested allowlist is compatible with the
// externally configured egress boundary before Docker creates a container.
func (e *StaticAllowlistEnforcer) Validate(ctx context.Context, policy ports.ContainerNetworkPolicy, networkMode string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e == nil {
		return fmt.Errorf("docker egress enforcer is nil")
	}
	if policy.EffectiveMode() != ports.ContainerNetworkAllowlist {
		return fmt.Errorf("docker egress enforcer requires network mode %q", ports.ContainerNetworkAllowlist)
	}
	if networkMode != e.networkMode {
		return fmt.Errorf("docker egress network mode = %q, want %q", networkMode, e.networkMode)
	}
	requested, err := ports.NormalizeContainerEgressTargets(policy.AllowedEgress)
	if err != nil {
		return err
	}
	for _, target := range requested {
		if !e.allowed[target] {
			return fmt.Errorf("docker egress target %q is not configured in the provider allowlist", target)
		}
	}
	return nil
}
