package docker

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	pinnedImage        = "registry.example.com/openclarion/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testEgressProxyURL = "http://openclarion-egress-proxy:18080"
)

func TestBuildRunSpecUsesSecureDockerDefaults(t *testing.T) {
	req := validRequest()
	spec, err := BuildRunSpec(validConfig(), req, validWorkspace())
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}

	if spec.ImageRef != pinnedImage {
		t.Fatalf("ImageRef = %q", spec.ImageRef)
	}
	if spec.User == "root" || spec.User == "0" {
		t.Fatalf("User = %q, want non-root", spec.User)
	}
	if !spec.ReadonlyRootFS {
		t.Fatalf("ReadonlyRootFS = false")
	}
	if !containsToken(spec.SecurityOpt, NoNewPrivilegesSecurityOpt) {
		t.Fatalf("SecurityOpt = %v, want %s", spec.SecurityOpt, NoNewPrivilegesSecurityOpt)
	}
	if spec.Privileged {
		t.Fatalf("Privileged = true")
	}
	if !containsToken(spec.CapDrop, DropAllCapabilities) {
		t.Fatalf("CapDrop = %v, want ALL", spec.CapDrop)
	}
	if spec.MemoryBytes != DefaultMemoryBytes {
		t.Fatalf("MemoryBytes = %d, want %d", spec.MemoryBytes, DefaultMemoryBytes)
	}
	if spec.NanoCPUs != DefaultNanoCPUs {
		t.Fatalf("NanoCPUs = %d, want %d", spec.NanoCPUs, DefaultNanoCPUs)
	}
	if spec.PidsLimit != DefaultPidsLimit {
		t.Fatalf("PidsLimit = %d, want %d", spec.PidsLimit, DefaultPidsLimit)
	}
	if spec.WorkingDir != DefaultWorkingDir {
		t.Fatalf("WorkingDir = %q, want %q", spec.WorkingDir, DefaultWorkingDir)
	}
	if spec.NetworkMode != "none" {
		t.Fatalf("NetworkMode = %q, want none", spec.NetworkMode)
	}
	if spec.EgressProxyURL != "" {
		t.Fatalf("EgressProxyURL = %q, want empty for network-none", spec.EgressProxyURL)
	}
	if spec.OutputMount.Target != ports.SandboxOutputDir || spec.OutputMount.ReadOnly {
		t.Fatalf("OutputMount = %#v", spec.OutputMount)
	}
	if spec.OutputMaxBytes != req.EffectiveOutputMax() {
		t.Fatalf("OutputMaxBytes = %d, want %d", spec.OutputMaxBytes, req.EffectiveOutputMax())
	}
	if spec.OutputPath != ports.SandboxOutputPath {
		t.Fatalf("OutputPath = %q, want %q", spec.OutputPath, ports.SandboxOutputPath)
	}
	assertBind(t, spec.BindMounts, ports.SandboxEvidencePath)
	assertBind(t, spec.BindMounts, ports.SandboxAgentConfigPath)
}

func TestBuildRunSpecMountsM5TurnInputsWhenPresent(t *testing.T) {
	req := validRequest()
	req.Conversation = json.RawMessage(`[{"role":"assistant","content":"previous"}]`)
	req.Message = json.RawMessage(`{"role":"user","content":"next"}`)
	workspace := validWorkspace()
	workspace.ConversationPath = "/tmp/openclarion-sandbox/conversation.json"
	workspace.MessagePath = "/tmp/openclarion-sandbox/message.json"
	spec, err := BuildRunSpec(validConfig(), req, workspace)
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}

	assertBind(t, spec.BindMounts, ports.SandboxConversationPath)
	assertBind(t, spec.BindMounts, ports.SandboxMessagePath)
}

func TestBuildRunSpecAllowlistUsesDedicatedNetwork(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	spec, err := BuildRunSpec(validConfig(), req, validWorkspace())
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}
	if spec.NetworkMode != DefaultAllowlistNetworkMode {
		t.Fatalf("NetworkMode = %q, want dedicated allowlist network", spec.NetworkMode)
	}
	if spec.EgressProxyURL != testEgressProxyURL {
		t.Fatalf("EgressProxyURL = %q, want %q", spec.EgressProxyURL, testEgressProxyURL)
	}
}

func TestBuildRunSpecAllowlistRequiresProxyURL(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	cfg := validConfig()
	cfg.EgressProxyURL = ""

	_, err := BuildRunSpec(cfg, req, validWorkspace())
	if err == nil || !strings.Contains(err.Error(), "egress proxy URL is required") {
		t.Fatalf("BuildRunSpec err = %v, want missing proxy URL", err)
	}
}

func TestBuildRunSpecRejectsCredentialProxyOverride(t *testing.T) {
	req := validRequest()
	req.Credentials = []ports.ContainerCredential{{
		Name:      "HTTPS_PROXY",
		Value:     "http://bypass.example.test:8080",
		ExpiresAt: time.Now().Add(time.Minute),
	}}

	_, err := BuildRunSpec(validConfig(), req, validWorkspace())
	if err == nil || !strings.Contains(err.Error(), "reserved by the Docker egress boundary") {
		t.Fatalf("BuildRunSpec err = %v, want reserved proxy env rejection", err)
	}
}

func TestBuildRunSpecAllowlistUsesConfiguredNetwork(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	cfg := validConfig()
	cfg.AllowlistNetworkMode = "openclarion-sandbox-egress-prod"

	spec, err := BuildRunSpec(cfg, req, validWorkspace())
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}
	if spec.NetworkMode != "openclarion-sandbox-egress-prod" {
		t.Fatalf("NetworkMode = %q, want configured allowlist network", spec.NetworkMode)
	}
}

func TestConfigRejectsUnsafeSecurityPosture(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name:    "mutable image tag",
			mutate:  func(cfg *Config) { cfg.ImageRef = "registry.example.com/openclarion/agent:latest" },
			wantErr: "digest",
		},
		{
			name:    "root user",
			mutate:  func(cfg *Config) { cfg.User = "0:0" },
			wantErr: "non-root",
		},
		{
			name:    "missing memory",
			mutate:  func(cfg *Config) { cfg.MemoryBytes = -1 },
			wantErr: "memory",
		},
		{
			name:    "missing cpu",
			mutate:  func(cfg *Config) { cfg.NanoCPUs = -1 },
			wantErr: "CPU",
		},
		{
			name:    "missing pids",
			mutate:  func(cfg *Config) { cfg.PidsLimit = -1 },
			wantErr: "PID",
		},
		{
			name:    "writable rootfs",
			mutate:  func(cfg *Config) { cfg.ReadonlyRootFS = false },
			wantErr: "readonly",
		},
		{
			name:    "missing no-new-privileges",
			mutate:  func(cfg *Config) { cfg.NoNewPrivileges = false },
			wantErr: "no-new-privileges",
		},
		{
			name:    "privileged",
			mutate:  func(cfg *Config) { cfg.Privileged = true },
			wantErr: "privileged",
		},
		{
			name:    "does not drop caps",
			mutate:  func(cfg *Config) { cfg.CapDrop = []string{"NET_RAW"} },
			wantErr: "capabilities",
		},
		{
			name:    "output max too large",
			mutate:  func(cfg *Config) { cfg.OutputMaxBytes = ports.MaxContainerOutputBytes + 1 },
			wantErr: "output max",
		},
		{
			name:    "wrong working dir",
			mutate:  func(cfg *Config) { cfg.WorkingDir = "/tmp" },
			wantErr: "working directory",
		},
		{
			name:    "empty command arg",
			mutate:  func(cfg *Config) { cfg.Command = []string{"agent", ""} },
			wantErr: "command",
		},
		{
			name:    "host allowlist network",
			mutate:  func(cfg *Config) { cfg.AllowlistNetworkMode = "host" },
			wantErr: "dedicated Docker network",
		},
		{
			name:    "bridge allowlist network",
			mutate:  func(cfg *Config) { cfg.AllowlistNetworkMode = "bridge" },
			wantErr: "dedicated Docker network",
		},
		{
			name:    "container namespace allowlist network",
			mutate:  func(cfg *Config) { cfg.AllowlistNetworkMode = "container:abc123" },
			wantErr: "Docker network name",
		},
		{
			name:    "whitespace allowlist network",
			mutate:  func(cfg *Config) { cfg.AllowlistNetworkMode = " openclarion-sandbox-allowlist " },
			wantErr: "contains whitespace",
		},
		{
			name:    "https proxy",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "https://openclarion-egress-proxy:18080" },
			wantErr: "absolute http URL",
		},
		{
			name:    "proxy userinfo",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://user@openclarion-egress-proxy:18080" },
			wantErr: "userinfo",
		},
		{
			name:    "proxy path",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://openclarion-egress-proxy:18080/proxy" },
			wantErr: "path",
		},
		{
			name:    "proxy query",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://openclarion-egress-proxy:18080/?tenant=test" },
			wantErr: "query or fragment",
		},
		{
			name:    "proxy whitespace",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = " http://openclarion-egress-proxy:18080" },
			wantErr: "whitespace",
		},
		{
			name:    "localhost proxy",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://localhost:18080" },
			wantErr: "loopback or unspecified",
		},
		{
			name:    "ipv4 loopback proxy",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://127.42.0.1:18080" },
			wantErr: "loopback or unspecified",
		},
		{
			name:    "unspecified proxy",
			mutate:  func(cfg *Config) { cfg.EgressProxyURL = "http://0.0.0.0:18080" },
			wantErr: "loopback or unspecified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(&cfg)
			_, err := cfg.Normalized()
			if err == nil {
				t.Fatal("Normalized err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Normalized err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildRunSpecRejectsUnsafeMounts(t *testing.T) {
	tests := []struct {
		name      string
		workspace WorkspacePaths
		wantErr   string
	}{
		{
			name:      "docker socket source",
			workspace: WorkspacePaths{EvidencePath: "/var/run/docker.sock", AgentConfigDir: "/tmp/agent-config"},
			wantErr:   "not allowed",
		},
		{
			name:      "missing evidence",
			workspace: WorkspacePaths{AgentConfigDir: "/tmp/agent-config"},
			wantErr:   "evidence host path is required",
		},
		{
			name:      "missing agent config",
			workspace: WorkspacePaths{EvidencePath: "/tmp/evidence.json"},
			wantErr:   "agent config host path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildRunSpec(validConfig(), validRequest(), tt.workspace)
			if err == nil {
				t.Fatal("BuildRunSpec err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("BuildRunSpec err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRunSpecRejectsPostTranslationDrift(t *testing.T) {
	req := validRequest()
	spec, err := BuildRunSpec(validConfig(), req, validWorkspace())
	if err != nil {
		t.Fatalf("BuildRunSpec: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*RunSpec)
		wantErr string
	}{
		{
			name:    "writable bind",
			mutate:  func(spec *RunSpec) { spec.BindMounts[0].ReadOnly = false },
			wantErr: "readonly",
		},
		{
			name:    "bind overlaps output",
			mutate:  func(spec *RunSpec) { spec.BindMounts[0].Target = ports.SandboxOutputPath },
			wantErr: "output mount",
		},
		{
			name:    "missing output mount",
			mutate:  func(spec *RunSpec) { spec.OutputMount = BindMount{} },
			wantErr: "output mount",
		},
		{
			name:    "readonly output mount",
			mutate:  func(spec *RunSpec) { spec.OutputMount.ReadOnly = true },
			wantErr: "writable",
		},
		{
			name:    "wrong output path",
			mutate:  func(spec *RunSpec) { spec.OutputPath = "/tmp/output.json" },
			wantErr: "output path",
		},
		{
			name:    "wrong working dir",
			mutate:  func(spec *RunSpec) { spec.WorkingDir = "/tmp" },
			wantErr: "working directory",
		},
		{
			name:    "proxy on network-none",
			mutate:  func(spec *RunSpec) { spec.EgressProxyURL = testEgressProxyURL },
			wantErr: "must be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := spec
			tt.mutate(&spec)
			err := ValidateRunSpec(spec, req)
			if err == nil {
				t.Fatal("ValidateRunSpec err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateRunSpec err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func validConfig() Config {
	return Config{
		ImageRef:        pinnedImage,
		User:            DefaultUser,
		ReadonlyRootFS:  true,
		NoNewPrivileges: true,
		EgressProxyURL:  testEgressProxyURL,
	}
}

func validRequest() ports.ContainerRunRequest {
	return ports.ContainerRunRequest{
		InvocationID: "snapshot-11/group-0",
		AgentName:    "report-enhancer",
		Evidence:     json.RawMessage(`{"snapshot_id":11,"alerts":[]}`),
		Timeout:      time.Minute,
		OutputMax:    1024,
	}
}

func validWorkspace() WorkspacePaths {
	return WorkspacePaths{
		EvidencePath:   "/tmp/openclarion-sandbox/evidence.json",
		AgentConfigDir: "/tmp/openclarion-sandbox/agent_config",
		OutputDir:      "/tmp/openclarion-sandbox/out",
	}
}

func assertBind(t *testing.T, binds []BindMount, target string) {
	t.Helper()
	for _, bind := range binds {
		if bind.Target == target {
			if !bind.ReadOnly {
				t.Fatalf("bind %q is not readonly", target)
			}
			return
		}
	}
	t.Fatalf("bind target %q not found in %#v", target, binds)
}
