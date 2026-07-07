package docker

import (
	"context"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestStaticAllowlistEnforcerAcceptsConfiguredSubset(t *testing.T) {
	enforcer, err := NewStaticAllowlistEnforcer(DefaultAllowlistNetworkMode, []string{
		"Prometheus.Internal:9090",
		"api.openai.com:443",
	})
	if err != nil {
		t.Fatalf("NewStaticAllowlistEnforcer: %v", err)
	}
	policy := ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}

	if err := enforcer.Validate(context.Background(), policy, DefaultAllowlistNetworkMode); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestStaticAllowlistEnforcerRejectsDriftBeforeDockerCreate(t *testing.T) {
	enforcer, err := NewStaticAllowlistEnforcer(DefaultAllowlistNetworkMode, []string{"prometheus.internal:9090"})
	if err != nil {
		t.Fatalf("NewStaticAllowlistEnforcer: %v", err)
	}

	tests := []struct {
		name        string
		policy      ports.ContainerNetworkPolicy
		networkMode string
		wantErr     string
	}{
		{
			name: "target not configured",
			policy: ports.ContainerNetworkPolicy{
				Mode:          ports.ContainerNetworkAllowlist,
				AllowedEgress: []string{"api.openai.com:443"},
			},
			networkMode: DefaultAllowlistNetworkMode,
			wantErr:     "not configured",
		},
		{
			name: "wrong docker network",
			policy: ports.ContainerNetworkPolicy{
				Mode:          ports.ContainerNetworkAllowlist,
				AllowedEgress: []string{"prometheus.internal:9090"},
			},
			networkMode: "bridge",
			wantErr:     "network mode",
		},
		{
			name: "not allowlist mode",
			policy: ports.ContainerNetworkPolicy{
				Mode: ports.ContainerNetworkNone,
			},
			networkMode: DefaultAllowlistNetworkMode,
			wantErr:     "requires network mode",
		},
		{
			name: "invalid requested target",
			policy: ports.ContainerNetworkPolicy{
				Mode:          ports.ContainerNetworkAllowlist,
				AllowedEgress: []string{"https://prometheus.internal"},
			},
			networkMode: DefaultAllowlistNetworkMode,
			wantErr:     "not a URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := enforcer.Validate(context.Background(), tt.policy, tt.networkMode)
			if err == nil {
				t.Fatal("Validate err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestStaticAllowlistEnforcerHonorsContextCancellation(t *testing.T) {
	enforcer, err := NewStaticAllowlistEnforcer(DefaultAllowlistNetworkMode, []string{"prometheus.internal:9090"})
	if err != nil {
		t.Fatalf("NewStaticAllowlistEnforcer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = enforcer.Validate(ctx, ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}, DefaultAllowlistNetworkMode)
	if err == nil {
		t.Fatal("Validate err = nil, want context error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Validate err = %v, want context canceled", err)
	}
}

func TestStaticAllowlistEnforcerRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		networkMode string
		targets     []string
		wantErr     string
	}{
		{
			name:    "missing network mode",
			targets: []string{"prometheus.internal:9090"},
			wantErr: "network mode",
		},
		{
			name:        "host network mode",
			networkMode: "host",
			targets:     []string{"prometheus.internal:9090"},
			wantErr:     "dedicated Docker network",
		},
		{
			name:        "invalid target",
			networkMode: DefaultAllowlistNetworkMode,
			targets:     []string{"prometheus.internal/path"},
			wantErr:     "host[:port]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStaticAllowlistEnforcer(tt.networkMode, tt.targets)
			if err == nil {
				t.Fatal("NewStaticAllowlistEnforcer err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewStaticAllowlistEnforcer err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
