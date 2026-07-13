package ports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	// SandboxEvidencePath is the readonly evidence input mounted for all sandbox invocations.
	SandboxEvidencePath = "/workspace/evidence.json"
	// SandboxConversationPath is the readonly conversation-history input mounted for M5 turns.
	SandboxConversationPath = "/workspace/conversation.json"
	// SandboxMessagePath is the readonly latest-user-message input mounted for M5 turns.
	SandboxMessagePath = "/workspace/message.json"
	// SandboxAgentConfigPath is the readonly opaque agent configuration directory.
	SandboxAgentConfigPath = "/workspace/agent_config"
	// SandboxOutputDir is the only writable sandbox path and must be size-capped by the concrete provider.
	SandboxOutputDir = "/workspace/out"
	// SandboxOutputPath is the structured response file read by the Go control plane.
	SandboxOutputPath = "/workspace/out/output.json"
	// ContainerEgressProxyReadinessPath verifies the live proxy configuration.
	ContainerEgressProxyReadinessPath = "/readyz"
	// ContainerEgressProxyReadinessFingerprintHeader carries the expected
	// canonical allowlist fingerprint without exposing its targets.
	ContainerEgressProxyReadinessFingerprintHeader = "X-OpenClarion-Egress-Allowlist-SHA256"

	// DefaultContainerRunTimeout is the default fixed lifetime for one sandbox invocation.
	DefaultContainerRunTimeout = 5 * time.Minute
	// MaxContainerRunTimeout is the maximum accepted lifetime for one sandbox invocation.
	MaxContainerRunTimeout = 5 * time.Minute
	// DefaultContainerOutputBytes is the default output.json size cap.
	DefaultContainerOutputBytes = 10 * 1024 * 1024
	// MaxContainerOutputBytes is the maximum accepted output.json size cap.
	MaxContainerOutputBytes = 10 * 1024 * 1024
)

// ContainerNetworkMode identifies the provider-neutral egress policy mode.
type ContainerNetworkMode string

const (
	// ContainerNetworkNone disables sandbox network access.
	ContainerNetworkNone ContainerNetworkMode = "none"
	// ContainerNetworkAllowlist permits only explicit egress targets.
	ContainerNetworkAllowlist ContainerNetworkMode = "allowlist"
)

var (
	containerInvocationIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
	containerAgentNamePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)
	containerCredentialNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)
	containerEgressHostPattern     = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
)

// ContainerNetworkPolicy is the provider-neutral sandbox egress contract.
// The default is network-none. Precise allowlist enforcement is a concrete
// provider responsibility and must be proven before M4 acceptance.
type ContainerNetworkPolicy struct {
	Mode          ContainerNetworkMode
	AllowedEgress []string
}

// ContainerCredential is one short-lived environment credential made
// available only for a single sandbox invocation. Values must never be
// logged or persisted outside the runtime boundary.
type ContainerCredential struct {
	Name      string
	Value     string
	ExpiresAt time.Time
}

// ContainerRunRequest is one stateless sandbox invocation shared by M4
// batch analysis and M5 per-turn diagnosis. The file paths are fixed by
// ADR-0013; concrete providers translate this DTO into their runtime's
// mount and execution configuration.
type ContainerRunRequest struct {
	InvocationID string
	AgentName    string
	Evidence     json.RawMessage
	Conversation json.RawMessage
	Message      json.RawMessage
	Timeout      time.Duration
	OutputMax    int64
	Network      ContainerNetworkPolicy
	Credentials  []ContainerCredential
	Metadata     map[string]string
}

// ContainerRunResult is the raw, schema-unvalidated result captured
// from SandboxOutputPath. Callers must still validate Output against
// the usecase-specific JSON Schema before persistence.
type ContainerRunResult struct {
	InvocationID string
	AgentName    string
	Output       json.RawMessage
	ExitCode     int
	StartedAt    time.Time
	FinishedAt   time.Time
	RuntimeID    string
}

// ContainerExitError is returned when a sandbox runtime starts successfully but
// the agent process exits non-zero. Diagnostic text must already be redacted by
// the concrete provider before crossing this port boundary.
type ContainerExitError struct {
	RuntimeID  string
	ExitCode   int
	Diagnostic string
}

func (e *ContainerExitError) Error() string {
	if e == nil {
		return "container exited"
	}
	runtimeID := strings.TrimSpace(e.RuntimeID)
	diagnostic := strings.TrimSpace(e.Diagnostic)
	if runtimeID == "" {
		if diagnostic == "" {
			return fmt.Sprintf("container exited with code %d", e.ExitCode)
		}
		return fmt.Sprintf("container exited with code %d; %s", e.ExitCode, diagnostic)
	}
	if diagnostic == "" {
		return fmt.Sprintf("container %s exited with code %d", runtimeID, e.ExitCode)
	}
	return fmt.Sprintf("container %s exited with code %d; %s", runtimeID, e.ExitCode, diagnostic)
}

// ContainerProvider owns sandbox lifecycle for one invocation. It must
// prepare readonly input mounts, run the container with a writable
// size-capped SandboxOutputDir, capture SandboxOutputPath, and clean up the
// runtime resource on success, error, and context cancellation.
type ContainerProvider interface {
	Run(ctx context.Context, req ContainerRunRequest) (ContainerRunResult, error)
}

// EffectiveTimeout returns the fixed timeout used when the caller does
// not override it explicitly.
func (r ContainerRunRequest) EffectiveTimeout() time.Duration {
	if r.Timeout == 0 {
		return DefaultContainerRunTimeout
	}
	return r.Timeout
}

// EffectiveOutputMax returns the output size cap used when the caller
// does not override it explicitly.
func (r ContainerRunRequest) EffectiveOutputMax() int64 {
	if r.OutputMax == 0 {
		return DefaultContainerOutputBytes
	}
	return r.OutputMax
}

// EffectiveMode returns the network policy mode used by providers.
func (p ContainerNetworkPolicy) EffectiveMode() ContainerNetworkMode {
	if p.Mode == "" {
		return ContainerNetworkNone
	}
	return p.Mode
}

// Validate checks the provider-neutral sandbox contract before any
// concrete runtime allocates a container.
func (r ContainerRunRequest) Validate() error {
	if strings.TrimSpace(r.InvocationID) == "" {
		return fmt.Errorf("container invocation id is required")
	}
	if !containerInvocationIDPattern.MatchString(r.InvocationID) {
		return fmt.Errorf("container invocation id %q is invalid", r.InvocationID)
	}
	if strings.TrimSpace(r.AgentName) == "" {
		return fmt.Errorf("container agent name is required")
	}
	if !containerAgentNamePattern.MatchString(r.AgentName) {
		return fmt.Errorf("container agent name %q is invalid", r.AgentName)
	}
	if err := validateRequiredJSONObject("evidence", r.Evidence); err != nil {
		return err
	}
	if err := validateOptionalJSON("conversation", r.Conversation); err != nil {
		return err
	}
	if err := validateOptionalJSON("message", r.Message); err != nil {
		return err
	}
	timeout := r.EffectiveTimeout()
	if timeout <= 0 {
		return fmt.Errorf("container timeout must be positive")
	}
	if timeout > MaxContainerRunTimeout {
		return fmt.Errorf("container timeout %s exceeds maximum %s", timeout, MaxContainerRunTimeout)
	}
	outputMax := r.EffectiveOutputMax()
	if outputMax <= 0 {
		return fmt.Errorf("container output max must be positive")
	}
	if outputMax > MaxContainerOutputBytes {
		return fmt.Errorf("container output max %d exceeds maximum %d", outputMax, MaxContainerOutputBytes)
	}
	if err := validateContainerCredentials(r.Credentials); err != nil {
		return err
	}
	return r.Network.Validate()
}

// ValidateCredentialExpirations checks that credentials are short-lived
// relative to the current container invocation. It must be called by
// concrete providers immediately before runtime allocation.
func (r ContainerRunRequest) ValidateCredentialExpirations(now time.Time) error {
	if len(r.Credentials) == 0 {
		return nil
	}
	if now.IsZero() {
		return fmt.Errorf("credential validation time is required")
	}
	deadline := now.Add(r.EffectiveTimeout())
	for _, credential := range r.Credentials {
		name := strings.TrimSpace(credential.Name)
		if !credential.ExpiresAt.After(now) {
			return fmt.Errorf("container credential %q is expired", name)
		}
		if credential.ExpiresAt.After(deadline) {
			return fmt.Errorf("container credential %q expiry exceeds container timeout", name)
		}
	}
	return nil
}

// Validate checks the egress policy shape. Concrete providers must
// prove enforcement separately; this method only prevents ambiguous
// or malformed policy requests from entering the runtime boundary.
func (p ContainerNetworkPolicy) Validate() error {
	switch p.EffectiveMode() {
	case ContainerNetworkNone:
		if len(p.AllowedEgress) != 0 {
			return fmt.Errorf("allowed egress requires network mode %q", ContainerNetworkAllowlist)
		}
	case ContainerNetworkAllowlist:
		if _, err := NormalizeContainerEgressTargets(p.AllowedEgress); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported container network mode %q", p.Mode)
	}
	return nil
}

// NormalizeContainerEgressTargets returns canonical host:port targets for
// allowlist-mode sandbox egress. It rejects URLs, paths, wildcards, whitespace,
// missing or invalid ports, and duplicates before a provider-specific enforcer
// sees the request.
func NormalizeContainerEgressTargets(targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("allowlist network mode requires at least one egress target")
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(targets))
	for _, target := range targets {
		out, err := normalizeContainerEgressTarget(target)
		if err != nil {
			return nil, err
		}
		if seen[out] {
			return nil, fmt.Errorf("duplicate allowed egress target %q", out)
		}
		seen[out] = true
		normalized = append(normalized, out)
	}
	return normalized, nil
}

// ContainerEgressAllowlistFingerprint returns an order-independent digest of
// the canonical allowlist. It can compare expected and live proxy
// configuration without exposing the configured targets.
func ContainerEgressAllowlistFingerprint(targets []string) (string, error) {
	normalized, err := NormalizeContainerEgressTargets(targets)
	if err != nil {
		return "", err
	}
	sort.Strings(normalized)
	digest := sha256.Sum256([]byte(strings.Join(normalized, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

// NormalizeContainerEgressProxyURL validates and canonicalizes the HTTP proxy
// endpoint with an explicit host:port shared by sandbox providers and their
// readiness probes.
func NormalizeContainerEgressProxyURL(raw string) (string, error) {
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\r\n\t ") {
		return "", fmt.Errorf("sandbox egress proxy URL must not contain whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.Opaque != "" {
		return "", fmt.Errorf("sandbox egress proxy URL must be an absolute http URL")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("sandbox egress proxy URL must not include userinfo")
	}
	if isContainerLocalEgressHost(parsed.Hostname()) {
		return "", fmt.Errorf("sandbox egress proxy URL host must not be loopback or unspecified")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" {
		return "", fmt.Errorf("sandbox egress proxy URL must not include a path")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", fmt.Errorf("sandbox egress proxy URL must not include query or fragment")
	}
	normalizedHost, err := NormalizeContainerEgressTargets([]string{parsed.Host})
	if err != nil {
		return "", fmt.Errorf("sandbox egress proxy URL host: %w", err)
	}
	parsed.Host = normalizedHost[0]
	parsed.Path = ""
	parsed.RawPath = ""
	return parsed.String(), nil
}

// ValidateContainerEgressURL verifies that an absolute HTTP(S) URL is covered
// by a normalized host:port allowlist.
func ValidateContainerEgressURL(rawURL string, targets []string) error {
	trimmedURL := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmedURL)
	if rawURL != trimmedURL || err != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("egress URL must be an absolute http or https URL without userinfo or surrounding whitespace")
	}
	allowed, err := NormalizeContainerEgressTargets(targets)
	if err != nil {
		return fmt.Errorf("allowed egress targets: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	if isContainerLocalEgressHost(host) {
		return fmt.Errorf("egress URL host must not be localhost, loopback, or unspecified")
	}
	port := parsed.Port()
	defaultPort := "80"
	if parsed.Scheme == "https" {
		defaultPort = "443"
	}
	if port == "" {
		port = defaultPort
	}
	exact := host + ":" + port
	for _, target := range allowed {
		if target == exact {
			return nil
		}
	}
	return fmt.Errorf("egress URL host must be listed in allowed targets")
}

// ValidateContainerRunResult checks the lifecycle and raw output
// invariants that a provider must satisfy before callers perform
// schema-specific validation.
func ValidateContainerRunResult(req ContainerRunRequest, result ContainerRunResult) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if result.InvocationID != req.InvocationID {
		return fmt.Errorf("container result invocation id = %q, want %q", result.InvocationID, req.InvocationID)
	}
	if result.AgentName != req.AgentName {
		return fmt.Errorf("container result agent name = %q, want %q", result.AgentName, req.AgentName)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("container exit code = %d, want 0", result.ExitCode)
	}
	if result.StartedAt.IsZero() {
		return fmt.Errorf("container result started_at is required")
	}
	if result.FinishedAt.IsZero() {
		return fmt.Errorf("container result finished_at is required")
	}
	if result.FinishedAt.Before(result.StartedAt) {
		return fmt.Errorf("container result finished_at precedes started_at")
	}
	if len(result.Output) == 0 {
		return fmt.Errorf("container output is required")
	}
	if int64(len(result.Output)) > req.EffectiveOutputMax() {
		return fmt.Errorf("container output size %d exceeds maximum %d", len(result.Output), req.EffectiveOutputMax())
	}
	if err := strictjson.RejectDuplicateObjectKeys(result.Output); err != nil {
		return fmt.Errorf("container output is not valid JSON: %w", err)
	}
	return nil
}

func validateRequiredJSONObject(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s JSON is required", name)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("%s JSON is invalid: %w", name, err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s JSON is invalid: %w", name, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("%s JSON must be an object", name)
	}
	return nil
}

func validateOptionalJSON(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("%s JSON is invalid: %w", name, err)
	}
	return nil
}

func normalizeContainerEgressTarget(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("allowed egress target must be non-empty")
	}
	if strings.TrimSpace(target) != target || strings.ContainsAny(target, " \t\r\n") {
		return "", fmt.Errorf("allowed egress target %q contains whitespace", target)
	}
	if strings.Contains(target, "://") {
		return "", fmt.Errorf("allowed egress target %q must be host:port, not a URL", target)
	}
	if strings.ContainsAny(target, "/?#@") {
		return "", fmt.Errorf("allowed egress target %q must be host:port", target)
	}
	if strings.Contains(target, "*") {
		return "", fmt.Errorf("allowed egress target %q must not contain wildcards", target)
	}

	host := target
	port := ""
	if strings.Count(target, ":") > 1 {
		return "", fmt.Errorf("allowed egress target %q must use host:port, not IPv6 literal syntax", target)
	}
	if strings.Contains(target, ":") {
		var err error
		host, port, err = net.SplitHostPort(target)
		if err != nil {
			return "", fmt.Errorf("allowed egress target %q must be host:port", target)
		}
	}
	if port == "" {
		return "", fmt.Errorf("allowed egress target %q must include an explicit port", target)
	}
	if !containerEgressHostPattern.MatchString(host) {
		return "", fmt.Errorf("allowed egress target %q has invalid host", target)
	}
	host = strings.ToLower(host)
	if isContainerLocalEgressHost(host) {
		return "", fmt.Errorf("allowed egress target %q host must not be localhost, loopback, or unspecified", target)
	}
	if err := validateContainerEgressPort(target, port); err != nil {
		return "", err
	}
	return host + ":" + port, nil
}

func isContainerLocalEgressHost(host string) bool {
	normalized := strings.TrimSuffix(strings.ToLower(host), ".")
	if normalized == "localhost" || strings.HasSuffix(normalized, ".localhost") {
		return true
	}
	ip := net.ParseIP(normalized)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func validateContainerEgressPort(target, port string) error {
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return fmt.Errorf("allowed egress target %q has invalid port", target)
	}
	return nil
}

func validateContainerCredentials(credentials []ContainerCredential) error {
	seen := map[string]bool{}
	for _, credential := range credentials {
		name := strings.TrimSpace(credential.Name)
		if name == "" {
			return fmt.Errorf("container credential name is required")
		}
		if name != credential.Name || !containerCredentialNamePattern.MatchString(name) {
			return fmt.Errorf("container credential name %q is invalid", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate container credential %q", name)
		}
		seen[name] = true
		if credential.Value == "" {
			return fmt.Errorf("container credential %q value is required", name)
		}
		if strings.ContainsAny(credential.Value, "\x00\r\n") {
			return fmt.Errorf("container credential %q value contains unsupported control characters", name)
		}
		if credential.ExpiresAt.IsZero() {
			return fmt.Errorf("container credential %q expiry is required", name)
		}
	}
	return nil
}
