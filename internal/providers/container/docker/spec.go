// Package docker translates the provider-neutral ContainerProvider
// contract into a Docker sandbox runtime specification and Docker Engine
// lifecycle calls.
package docker

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// DefaultUser is the non-root user used by the OpenClarion sandbox image.
	DefaultUser = "65532:65532"
	// DefaultMemoryBytes is the baseline sandbox memory limit.
	DefaultMemoryBytes int64 = 512 * 1024 * 1024
	// DefaultNanoCPUs is the baseline sandbox CPU limit.
	DefaultNanoCPUs int64 = 1_000_000_000
	// DefaultPidsLimit is the baseline process count limit for one sandbox.
	DefaultPidsLimit int64 = 256
	// DefaultWorkingDir is the container working directory shared by ADR-0013 paths.
	DefaultWorkingDir = "/workspace"
	// DefaultAllowlistNetworkMode is the dedicated Docker network expected when egress is explicitly allowlisted.
	DefaultAllowlistNetworkMode = "openclarion-sandbox-allowlist"
	// NoNewPrivilegesSecurityOpt is the Docker security option required by ADR-0005.
	NoNewPrivilegesSecurityOpt = "no-new-privileges"
	// DropAllCapabilities is the Docker capability drop value required by ADR-0005.
	DropAllCapabilities = "ALL"
)

var imageDigestPattern = regexp.MustCompile(`^[^\s@]+@sha256:[a-fA-F0-9]{64}$`)

// Config is the security-relevant Docker sandbox configuration.
// Zero values are normalized to conservative defaults where possible;
// explicit unsafe values are rejected.
type Config struct {
	ImageRef        string
	User            string
	MemoryBytes     int64
	NanoCPUs        int64
	PidsLimit       int64
	ReadonlyRootFS  bool
	NoNewPrivileges bool
	Privileged      bool
	CapDrop         []string
	OutputMaxBytes  int64
	Command         []string
	WorkingDir      string
	// AllowlistNetworkMode names the existing Docker network used when a
	// request explicitly asks for allowlisted egress through an external proxy
	// or firewall boundary.
	AllowlistNetworkMode string
	// EgressProxyURL is the credential-free HTTP proxy reachable only through
	// the dedicated allowlist network.
	EgressProxyURL string
}

// WorkspacePaths are host-side paths prepared by the control plane for
// one sandbox invocation. They are mounted into the fixed ADR-0013
// container paths.
type WorkspacePaths struct {
	EvidencePath     string
	ConversationPath string
	MessagePath      string
	AgentConfigDir   string
	OutputDir        string
}

// BindMount is one Docker bind mount in the generated sandbox spec.
type BindMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// RunSpec is the security-reviewed shape a future Docker Engine
// implementation should translate to container.Config and HostConfig.
type RunSpec struct {
	ImageRef        string
	User            string
	MemoryBytes     int64
	NanoCPUs        int64
	PidsLimit       int64
	ReadonlyRootFS  bool
	SecurityOpt     []string
	Privileged      bool
	CapDrop         []string
	Command         []string
	WorkingDir      string
	NetworkMode     string
	EgressProxyURL  string
	BindMounts      []BindMount
	OutputMount     BindMount
	OutputMaxBytes  int64
	Timeout         time.Duration
	OutputPath      string
	InvocationLabel string
}

// BuildRunSpec validates cfg, req, and workspace paths, then returns a
// Docker runtime spec that preserves the M4/M5 file-based contract.
func BuildRunSpec(cfg Config, req ports.ContainerRunRequest, workspace WorkspacePaths) (RunSpec, error) {
	if err := req.Validate(); err != nil {
		return RunSpec{}, err
	}
	normalized, err := cfg.Normalized()
	if err != nil {
		return RunSpec{}, err
	}
	binds, err := workspace.bindsFor(req)
	if err != nil {
		return RunSpec{}, err
	}
	if err := validateBindMounts(binds); err != nil {
		return RunSpec{}, err
	}
	if err := validateRuntimeCredentialNames(req.Credentials); err != nil {
		return RunSpec{}, err
	}
	networkMode, err := networkModeFor(req.Network, normalized.AllowlistNetworkMode)
	if err != nil {
		return RunSpec{}, err
	}
	egressProxyURL, err := egressProxyURLFor(req.Network, normalized.EgressProxyURL)
	if err != nil {
		return RunSpec{}, err
	}

	spec := RunSpec{
		ImageRef:        normalized.ImageRef,
		User:            normalized.User,
		MemoryBytes:     normalized.MemoryBytes,
		NanoCPUs:        normalized.NanoCPUs,
		PidsLimit:       normalized.PidsLimit,
		ReadonlyRootFS:  normalized.ReadonlyRootFS,
		SecurityOpt:     []string{NoNewPrivilegesSecurityOpt},
		Privileged:      false,
		CapDrop:         []string{DropAllCapabilities},
		Command:         cloneStringSlice(normalized.Command),
		WorkingDir:      normalized.WorkingDir,
		NetworkMode:     networkMode,
		EgressProxyURL:  egressProxyURL,
		BindMounts:      binds,
		OutputMount:     BindMount{Source: workspace.OutputDir, Target: ports.SandboxOutputDir, ReadOnly: false},
		OutputMaxBytes:  req.EffectiveOutputMax(),
		Timeout:         req.EffectiveTimeout(),
		OutputPath:      ports.SandboxOutputPath,
		InvocationLabel: req.InvocationID,
	}
	if err := ValidateRunSpec(spec, req); err != nil {
		return RunSpec{}, err
	}
	return spec, nil
}

// Normalized applies conservative defaults and validates the resulting config.
func (c Config) Normalized() (Config, error) {
	out := c
	if out.User == "" {
		out.User = DefaultUser
	}
	if out.MemoryBytes == 0 {
		out.MemoryBytes = DefaultMemoryBytes
	}
	if out.NanoCPUs == 0 {
		out.NanoCPUs = DefaultNanoCPUs
	}
	if out.PidsLimit == 0 {
		out.PidsLimit = DefaultPidsLimit
	}
	if out.OutputMaxBytes == 0 {
		out.OutputMaxBytes = ports.DefaultContainerOutputBytes
	}
	if out.WorkingDir == "" {
		out.WorkingDir = DefaultWorkingDir
	}
	if out.AllowlistNetworkMode == "" {
		out.AllowlistNetworkMode = DefaultAllowlistNetworkMode
	}
	if len(out.CapDrop) == 0 {
		out.CapDrop = []string{DropAllCapabilities}
	}
	if out.EgressProxyURL != "" {
		normalizedProxyURL, err := ports.NormalizeContainerEgressProxyURL(out.EgressProxyURL)
		if err != nil {
			return Config{}, err
		}
		out.EgressProxyURL = normalizedProxyURL
	}
	if err := out.Validate(); err != nil {
		return Config{}, err
	}
	return out, nil
}

// Validate rejects unsafe Docker sandbox configuration before runtime allocation.
func (c Config) Validate() error {
	if !imageDigestPattern.MatchString(c.ImageRef) {
		return fmt.Errorf("sandbox image must be pinned by sha256 digest")
	}
	if isRootUser(c.User) {
		return fmt.Errorf("sandbox user must be non-root")
	}
	if c.MemoryBytes <= 0 {
		return fmt.Errorf("sandbox memory limit must be positive")
	}
	if c.NanoCPUs <= 0 {
		return fmt.Errorf("sandbox CPU limit must be positive")
	}
	if c.PidsLimit <= 0 {
		return fmt.Errorf("sandbox PID limit must be positive")
	}
	if !c.ReadonlyRootFS {
		return fmt.Errorf("sandbox root filesystem must be readonly")
	}
	if !c.NoNewPrivileges {
		return fmt.Errorf("sandbox must set %s", NoNewPrivilegesSecurityOpt)
	}
	if c.Privileged {
		return fmt.Errorf("sandbox must not run privileged")
	}
	if !containsToken(c.CapDrop, DropAllCapabilities) {
		return fmt.Errorf("sandbox must drop all Linux capabilities")
	}
	if c.OutputMaxBytes <= 0 {
		return fmt.Errorf("sandbox output max bytes must be positive")
	}
	if c.OutputMaxBytes > ports.MaxContainerOutputBytes {
		return fmt.Errorf("sandbox output max bytes %d exceeds maximum %d", c.OutputMaxBytes, ports.MaxContainerOutputBytes)
	}
	if strings.TrimSpace(c.WorkingDir) != DefaultWorkingDir {
		return fmt.Errorf("sandbox working directory must be %q", DefaultWorkingDir)
	}
	if err := validateAllowlistNetworkMode(c.AllowlistNetworkMode); err != nil {
		return err
	}
	if c.EgressProxyURL != "" {
		if _, err := ports.NormalizeContainerEgressProxyURL(c.EgressProxyURL); err != nil {
			return err
		}
	}
	for _, arg := range c.Command {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("sandbox command arguments must be non-empty")
		}
	}
	return nil
}

// ValidateRunSpec checks the generated Docker runtime spec still
// satisfies ADR-0005 and ADR-0013 after all translation steps.
func ValidateRunSpec(spec RunSpec, req ports.ContainerRunRequest) error {
	if !imageDigestPattern.MatchString(spec.ImageRef) {
		return fmt.Errorf("run spec image must be pinned by sha256 digest")
	}
	if isRootUser(spec.User) {
		return fmt.Errorf("run spec user must be non-root")
	}
	if spec.MemoryBytes <= 0 || spec.NanoCPUs <= 0 || spec.PidsLimit <= 0 {
		return fmt.Errorf("run spec must set memory, CPU, and PID limits")
	}
	if !spec.ReadonlyRootFS {
		return fmt.Errorf("run spec root filesystem must be readonly")
	}
	if !containsToken(spec.SecurityOpt, NoNewPrivilegesSecurityOpt) {
		return fmt.Errorf("run spec must set %s", NoNewPrivilegesSecurityOpt)
	}
	if spec.Privileged {
		return fmt.Errorf("run spec must not run privileged")
	}
	if !containsToken(spec.CapDrop, DropAllCapabilities) {
		return fmt.Errorf("run spec must drop all Linux capabilities")
	}
	if spec.OutputPath != ports.SandboxOutputPath {
		return fmt.Errorf("run spec output path = %q, want %q", spec.OutputPath, ports.SandboxOutputPath)
	}
	if spec.WorkingDir != DefaultWorkingDir {
		return fmt.Errorf("run spec working directory = %q, want %q", spec.WorkingDir, DefaultWorkingDir)
	}
	expectedProxyURL, err := egressProxyURLFor(req.Network, spec.EgressProxyURL)
	if err != nil {
		return err
	}
	if expectedProxyURL != spec.EgressProxyURL {
		if req.Network.EffectiveMode() == ports.ContainerNetworkAllowlist {
			return fmt.Errorf("run spec egress proxy URL must be normalized")
		}
		return fmt.Errorf("run spec egress proxy must be empty when networking is disabled")
	}
	if err := validateRuntimeCredentialNames(req.Credentials); err != nil {
		return err
	}
	for _, arg := range spec.Command {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("run spec command arguments must be non-empty")
		}
	}
	if spec.Timeout != req.EffectiveTimeout() {
		return fmt.Errorf("run spec timeout = %s, want %s", spec.Timeout, req.EffectiveTimeout())
	}
	if spec.OutputMaxBytes <= 0 || spec.OutputMaxBytes > ports.MaxContainerOutputBytes {
		return fmt.Errorf("run spec output max bytes must be within limit")
	}
	if err := validateBindMounts(spec.BindMounts); err != nil {
		return err
	}
	if err := validateOutputMount(spec.OutputMount); err != nil {
		return err
	}
	return nil
}

func (w WorkspacePaths) bindsFor(req ports.ContainerRunRequest) ([]BindMount, error) {
	var binds []BindMount
	if strings.TrimSpace(w.EvidencePath) == "" {
		return nil, fmt.Errorf("evidence host path is required")
	}
	if strings.TrimSpace(w.AgentConfigDir) == "" {
		return nil, fmt.Errorf("agent config host path is required")
	}
	binds = append(binds,
		BindMount{Source: w.EvidencePath, Target: ports.SandboxEvidencePath, ReadOnly: true},
		BindMount{Source: w.AgentConfigDir, Target: ports.SandboxAgentConfigPath, ReadOnly: true},
	)
	if len(req.Conversation) != 0 {
		if strings.TrimSpace(w.ConversationPath) == "" {
			return nil, fmt.Errorf("conversation host path is required when conversation JSON is present")
		}
		binds = append(binds, BindMount{Source: w.ConversationPath, Target: ports.SandboxConversationPath, ReadOnly: true})
	}
	if len(req.Message) != 0 {
		if strings.TrimSpace(w.MessagePath) == "" {
			return nil, fmt.Errorf("message host path is required when message JSON is present")
		}
		binds = append(binds, BindMount{Source: w.MessagePath, Target: ports.SandboxMessagePath, ReadOnly: true})
	}
	return binds, nil
}

func validateBindMounts(binds []BindMount) error {
	if len(binds) == 0 {
		return fmt.Errorf("run spec requires readonly input bind mounts")
	}
	targets := map[string]bool{}
	for _, bind := range binds {
		if strings.TrimSpace(bind.Source) == "" {
			return fmt.Errorf("bind mount source is required")
		}
		if strings.TrimSpace(bind.Target) == "" {
			return fmt.Errorf("bind mount target is required")
		}
		if dangerousHostPath(bind.Source) || dangerousContainerPath(bind.Target) {
			return fmt.Errorf("bind mount %q -> %q is not allowed", bind.Source, bind.Target)
		}
		if !strings.HasPrefix(bind.Target, "/workspace/") {
			return fmt.Errorf("bind mount target %q must stay under /workspace", bind.Target)
		}
		if bind.Target == ports.SandboxOutputDir || strings.HasPrefix(bind.Target, ports.SandboxOutputDir+"/") {
			return fmt.Errorf("bind mount target %q must not overlap writable output mount", bind.Target)
		}
		if !bind.ReadOnly {
			return fmt.Errorf("bind mount target %q must be readonly", bind.Target)
		}
		if targets[bind.Target] {
			return fmt.Errorf("duplicate bind mount target %q", bind.Target)
		}
		targets[bind.Target] = true
	}
	return nil
}

func validateOutputMount(mount BindMount) error {
	if strings.TrimSpace(mount.Source) == "" {
		return fmt.Errorf("output mount source is required")
	}
	if dangerousHostPath(mount.Source) {
		return fmt.Errorf("output mount source %q is not allowed", mount.Source)
	}
	if mount.Target != ports.SandboxOutputDir {
		return fmt.Errorf("output mount target = %q, want %q", mount.Target, ports.SandboxOutputDir)
	}
	if dangerousContainerPath(mount.Target) {
		return fmt.Errorf("output mount target %q is not allowed", mount.Target)
	}
	if mount.ReadOnly {
		return fmt.Errorf("output mount %q must be writable", mount.Target)
	}
	return nil
}

func networkModeFor(policy ports.ContainerNetworkPolicy, allowlistNetworkMode string) (string, error) {
	if err := policy.Validate(); err != nil {
		return "", err
	}
	switch policy.EffectiveMode() {
	case ports.ContainerNetworkNone:
		return string(ports.ContainerNetworkNone), nil
	case ports.ContainerNetworkAllowlist:
		if err := validateAllowlistNetworkMode(allowlistNetworkMode); err != nil {
			return "", err
		}
		return allowlistNetworkMode, nil
	default:
		return "", fmt.Errorf("unsupported container network mode %q", policy.Mode)
	}
}

func egressProxyURLFor(policy ports.ContainerNetworkPolicy, proxyURL string) (string, error) {
	switch policy.EffectiveMode() {
	case ports.ContainerNetworkNone:
		return "", nil
	case ports.ContainerNetworkAllowlist:
		if proxyURL == "" {
			return "", fmt.Errorf("sandbox egress proxy URL is required for allowlist network mode")
		}
		return ports.NormalizeContainerEgressProxyURL(proxyURL)
	default:
		return "", fmt.Errorf("unsupported container network mode %q", policy.Mode)
	}
}

func validateRuntimeCredentialNames(credentials []ports.ContainerCredential) error {
	for _, credential := range credentials {
		if isReservedProxyEnv(credential.Name) {
			return fmt.Errorf("container credential %q is reserved by the Docker egress boundary", credential.Name)
		}
	}
	return nil
}

func isReservedProxyEnv(name string) bool {
	switch strings.ToLower(name) {
	case "http_proxy", "https_proxy", "all_proxy", "no_proxy":
		return true
	default:
		return false
	}
}

func validateAllowlistNetworkMode(networkMode string) error {
	trimmed := strings.TrimSpace(networkMode)
	if trimmed == "" {
		return fmt.Errorf("sandbox allowlist network mode is required")
	}
	if trimmed != networkMode || strings.ContainsAny(trimmed, " \t\r\n") {
		return fmt.Errorf("sandbox allowlist network mode %q contains whitespace", networkMode)
	}
	switch strings.ToLower(trimmed) {
	case string(ports.ContainerNetworkNone), "host", "bridge", "default":
		return fmt.Errorf("sandbox allowlist network mode %q must name a dedicated Docker network", networkMode)
	}
	if strings.Contains(trimmed, ":") || strings.ContainsAny(trimmed, "/\\") {
		return fmt.Errorf("sandbox allowlist network mode %q must be a Docker network name", networkMode)
	}
	return nil
}

func isRootUser(user string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(user))
	return trimmed == "" || trimmed == "root" || trimmed == "0" || strings.HasPrefix(trimmed, "0:")
}

func containsToken(values []string, token string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), token) {
			return true
		}
	}
	return false
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func dangerousHostPath(path string) bool {
	cleaned := strings.TrimSpace(path)
	return cleaned == "/var/run/docker.sock" || cleaned == "/run/docker.sock"
}

func dangerousContainerPath(path string) bool {
	cleaned := strings.TrimSpace(path)
	return cleaned == "/var/run/docker.sock" || cleaned == "/run/docker.sock" || cleaned == "/"
}
