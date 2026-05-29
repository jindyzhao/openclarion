// Command container_provider_smoke runs the Docker-backed ContainerProvider
// against a real local Docker daemon for the manual M4 runtime execution gate.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/moby/moby/client"
	dockerprovider "github.com/openclarion/openclarion/internal/providers/container/docker"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	envImageRef        = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_IMAGE"
	envCommandJSON     = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_COMMAND_JSON"
	envInvocationID    = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_INVOCATION_ID"
	envAgentName       = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_AGENT_NAME"
	envTimeoutSeconds  = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_TIMEOUT_SECONDS"
	envOutputMaxBytes  = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_OUTPUT_MAX_BYTES"
	envEvidenceJSON    = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_EVIDENCE_JSON"
	envConversation    = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_CONVERSATION_JSON"
	envMessage         = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_MESSAGE_JSON"
	envAgentConfigRoot = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_AGENT_CONFIG_ROOT"
	envWorkspaceRoot   = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_WORKSPACE_ROOT"
	envExpectError     = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_EXPECT_ERROR_CONTAINS"
	envProofPath       = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_PATH"
	envProofSource     = "OPENCLARION_CONTAINER_PROVIDER_SMOKE_PROOF_SOURCE"

	defaultAgentName      = "container-provider-smoke"
	defaultTimeoutSeconds = 60
	defaultProofSource    = "make container-provider-smoke"
)

var (
	defaultEvidenceJSON     = json.RawMessage(`{"provider_smoke":"ok","runtime":"docker-provider"}`)
	defaultConversationJSON = json.RawMessage(`[{"role":"assistant","content":"container provider smoke context"}]`)
	defaultMessageJSON      = json.RawMessage(`{"role":"user","content":"prove the Docker provider live path"}`)
)

type smokeConfig struct {
	ImageRef        string
	Command         []string
	InvocationID    string
	AgentName       string
	Timeout         time.Duration
	OutputMax       int64
	Evidence        json.RawMessage
	Conversation    json.RawMessage
	Message         json.RawMessage
	AgentConfigRoot string
	WorkspaceRoot   string
	ExpectError     string
	ProofPath       string
	ProofSource     string
}

func main() {
	if err := run(context.Background(), os.Environ(), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[container-provider-smoke] %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, environ []string, stdout io.Writer) error {
	cfg, cleanup, err := configFromEnv(environ)
	if err != nil {
		return err
	}
	defer func() {
		if err := cleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "[container-provider-smoke] cleanup: %v\n", err)
		}
	}()

	engine, err := dockerclient.New(dockerclient.FromEnv, dockerclient.WithUserAgent("openclarion-container-provider-smoke"))
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	defer engine.Close()

	providerOpts := []dockerprovider.ProviderOption{}
	if cfg.WorkspaceRoot != "" {
		providerOpts = append(providerOpts, dockerprovider.WithWorkspaceRoot(cfg.WorkspaceRoot))
	}
	provider, err := dockerprovider.NewProvider(engine, dockerConfig(cfg), cfg.AgentConfigRoot, providerOpts...)
	if err != nil {
		return fmt.Errorf("docker provider: %w", err)
	}

	req := requestFromConfig(cfg)
	result, err := provider.Run(ctx, req)
	outcome, err := handleProviderRunResult(cfg, result, err, stdout)
	if err != nil {
		return err
	}
	if cfg.ProofPath != "" {
		if err := writeProof(cfg, outcome); err != nil {
			return err
		}
	}
	return nil
}

func dockerConfig(cfg smokeConfig) dockerprovider.Config {
	return dockerprovider.Config{
		ImageRef:        cfg.ImageRef,
		ReadonlyRootFS:  true,
		NoNewPrivileges: true,
		CapDrop:         []string{dockerprovider.DropAllCapabilities},
		Command:         cloneStrings(cfg.Command),
	}
}

func requestFromConfig(cfg smokeConfig) ports.ContainerRunRequest {
	return ports.ContainerRunRequest{
		InvocationID: cfg.InvocationID,
		AgentName:    cfg.AgentName,
		Evidence:     append(json.RawMessage(nil), cfg.Evidence...),
		Conversation: append(json.RawMessage(nil), cfg.Conversation...),
		Message:      append(json.RawMessage(nil), cfg.Message...),
		Timeout:      cfg.Timeout,
		OutputMax:    cfg.OutputMax,
		Network: ports.ContainerNetworkPolicy{
			Mode: ports.ContainerNetworkNone,
		},
		Metadata: map[string]string{"smoke": "container-provider"},
	}
}

func configFromEnv(environ []string) (smokeConfig, func() error, error) {
	env := environMap(environ)
	cfg := smokeConfig{
		ImageRef:      strings.TrimSpace(env[envImageRef]),
		Command:       nil,
		InvocationID:  strings.TrimSpace(env[envInvocationID]),
		AgentName:     strings.TrimSpace(env[envAgentName]),
		Timeout:       defaultTimeoutSeconds * time.Second,
		OutputMax:     ports.DefaultContainerOutputBytes,
		Evidence:      defaultEvidenceJSON,
		Conversation:  defaultConversationJSON,
		Message:       defaultMessageJSON,
		WorkspaceRoot: strings.TrimSpace(env[envWorkspaceRoot]),
		ExpectError:   strings.TrimSpace(env[envExpectError]),
		ProofPath:     strings.TrimSpace(env[envProofPath]),
		ProofSource:   strings.TrimSpace(env[envProofSource]),
	}
	if cfg.ImageRef == "" {
		return smokeConfig{}, nil, fmt.Errorf("%s is required", envImageRef)
	}
	if cfg.InvocationID == "" {
		cfg.InvocationID = fmt.Sprintf("container-provider-smoke-%d", time.Now().UnixNano())
	}
	if cfg.AgentName == "" {
		cfg.AgentName = defaultAgentName
	}
	if cfg.ProofSource == "" {
		cfg.ProofSource = defaultProofSource
	}
	if err := validateProofSource(cfg.ProofSource); err != nil {
		return smokeConfig{}, nil, err
	}

	if commandRaw := strings.TrimSpace(env[envCommandJSON]); commandRaw != "" {
		command, err := parseCommandJSON(commandRaw)
		if err != nil {
			return smokeConfig{}, nil, err
		}
		cfg.Command = command
	}
	if raw := strings.TrimSpace(env[envTimeoutSeconds]); raw != "" {
		seconds, err := parsePositiveInt64(envTimeoutSeconds, raw)
		if err != nil {
			return smokeConfig{}, nil, err
		}
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	if raw := strings.TrimSpace(env[envOutputMaxBytes]); raw != "" {
		maxBytes, err := parsePositiveInt64(envOutputMaxBytes, raw)
		if err != nil {
			return smokeConfig{}, nil, err
		}
		cfg.OutputMax = maxBytes
	}
	if raw := strings.TrimSpace(env[envEvidenceJSON]); raw != "" {
		cfg.Evidence = json.RawMessage(raw)
	}
	if raw := strings.TrimSpace(env[envConversation]); raw != "" {
		cfg.Conversation = json.RawMessage(raw)
	}
	if raw := strings.TrimSpace(env[envMessage]); raw != "" {
		cfg.Message = json.RawMessage(raw)
	}

	cleanup := func() error { return nil }
	if root := strings.TrimSpace(env[envAgentConfigRoot]); root != "" {
		cfg.AgentConfigRoot = root
	} else {
		root, err := createDefaultAgentConfig(cfg.AgentName)
		if err != nil {
			return smokeConfig{}, nil, err
		}
		cfg.AgentConfigRoot = root
		cleanup = func() error {
			if err := os.RemoveAll(root); err != nil {
				return fmt.Errorf("remove generated agent config root %s: %w", root, err)
			}
			return nil
		}
	}

	if err := requestFromConfig(cfg).Validate(); err != nil {
		_ = cleanup()
		return smokeConfig{}, nil, err
	}
	if _, err := dockerConfig(cfg).Normalized(); err != nil {
		_ = cleanup()
		return smokeConfig{}, nil, err
	}
	return cfg, cleanup, nil
}

type runOutcome struct {
	Mode                string
	RuntimeID           string
	OutputBytes         int
	OutputSHA256        string
	MatchedErrorPattern string
}

func handleProviderRunResult(cfg smokeConfig, result ports.ContainerRunResult, runErr error, stdout io.Writer) (runOutcome, error) {
	if cfg.ExpectError != "" {
		if runErr == nil {
			return runOutcome{}, fmt.Errorf("provider run succeeded, want error containing %q", cfg.ExpectError)
		}
		matched := matchingErrorPattern(runErr, cfg.ExpectError)
		if matched == "" {
			return runOutcome{}, fmt.Errorf("provider run error does not contain %q: %w", cfg.ExpectError, runErr)
		}
		fmt.Fprintf(stdout, "[container-provider-smoke] OK - expected provider error containing %q\n", cfg.ExpectError)
		return runOutcome{
			Mode:                "expected_error",
			MatchedErrorPattern: matched,
		}, nil
	}
	if runErr != nil {
		return runOutcome{}, fmt.Errorf("provider run: %w", runErr)
	}
	if err := validateSmokeOutput(result.Output); err != nil {
		return runOutcome{}, err
	}
	outputDigest := sha256.Sum256(result.Output)
	fmt.Fprintf(stdout, "[container-provider-smoke] OK - runtime_id=%s output_bytes=%d\n", result.RuntimeID, len(result.Output))
	return runOutcome{
		Mode:         "success",
		RuntimeID:    result.RuntimeID,
		OutputBytes:  len(result.Output),
		OutputSHA256: hex.EncodeToString(outputDigest[:]),
	}, nil
}

func errorContainsAny(err error, patterns string) bool {
	return matchingErrorPattern(err, patterns) != ""
}

func matchingErrorPattern(err error, patterns string) string {
	for _, pattern := range strings.Split(patterns, "||") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(err.Error(), pattern) {
			return pattern
		}
	}
	return ""
}

type proofArtifact struct {
	Tool         string       `json:"tool"`
	Status       string       `json:"status"`
	Source       string       `json:"source"`
	ImageRef     string       `json:"image_ref"`
	InvocationID string       `json:"invocation_id,omitempty"`
	Mode         string       `json:"mode"`
	TimeoutSec   int64        `json:"timeout_seconds"`
	Output       *proofOutput `json:"output,omitempty"`
	Expected     *proofError  `json:"expected_error,omitempty"`
	Checks       []proofCheck `json:"checks"`
}

type proofOutput struct {
	Bytes    int    `json:"bytes"`
	MaxBytes int64  `json:"max_bytes"`
	SHA256   string `json:"sha256"`
}

type proofError struct {
	Pattern string `json:"pattern"`
	Matched string `json:"matched"`
}

type proofCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func writeProof(cfg smokeConfig, outcome runOutcome) error {
	proof := proofArtifact{
		Tool:         "container-provider-smoke",
		Status:       "pass",
		Source:       cfg.ProofSource,
		ImageRef:     cfg.ImageRef,
		InvocationID: cfg.InvocationID,
		Mode:         outcome.Mode,
		TimeoutSec:   int64(cfg.Timeout / time.Second),
		Checks: []proofCheck{
			{Name: "digest_pinned_image", Status: "pass"},
			{Name: "request_validated", Status: "pass"},
			{Name: "network_none", Status: "pass"},
			{Name: "readonly_rootfs", Status: "pass"},
			{Name: "no_new_privileges", Status: "pass"},
			{Name: "cap_drop_all", Status: "pass"},
		},
	}
	switch outcome.Mode {
	case "success":
		proof.Output = &proofOutput{
			Bytes:    outcome.OutputBytes,
			MaxBytes: cfg.OutputMax,
			SHA256:   outcome.OutputSHA256,
		}
		proof.Checks = append(proof.Checks,
			proofCheck{Name: "provider_run_succeeded", Status: "pass"},
			proofCheck{Name: "valid_json_object_output", Status: "pass"},
			proofCheck{Name: "duplicate_key_free_output", Status: "pass"},
		)
	case "expected_error":
		proof.Expected = &proofError{
			Pattern: cfg.ExpectError,
			Matched: outcome.MatchedErrorPattern,
		}
		proof.Checks = append(proof.Checks,
			proofCheck{Name: "expected_provider_error_observed", Status: "pass"},
		)
	default:
		return fmt.Errorf("unknown proof outcome mode %q", outcome.Mode)
	}
	return writeJSONFile(cfg.ProofPath, proof)
}

func validateProofSource(source string) error {
	switch source {
	case "make container-provider-smoke",
		"make container-provider-timeout-smoke",
		"make container-provider-output-cap-smoke":
		return nil
	default:
		return fmt.Errorf("%s must be a canonical container provider smoke make target", envProofSource)
	}
}

func writeJSONFile(path string, value any) error {
	clean := filepath.Clean(path)
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return err
	}
	if info, err := os.Lstat(clean); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s must be a regular file, not a symlink", clean)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", clean)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat proof: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return fmt.Errorf("create proof parent: %w", err)
	}
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return err
	}
	// #nosec G304 -- this manual smoke checker writes the operator-supplied proof JSON path.
	f, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = f.Close()
		return fmt.Errorf("write proof: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func validateNoSymlinkAncestors(cleanPath string) error {
	dir := filepath.Dir(cleanPath)
	for dir != "." && dir != string(filepath.Separator) {
		info, err := os.Lstat(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				next := filepath.Dir(dir)
				if next == dir {
					return nil
				}
				dir = next
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s parent directory %s must not be a symlink", cleanPath, dir)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s parent path %s must be a directory", cleanPath, dir)
		}
		next := filepath.Dir(dir)
		if next == dir {
			return nil
		}
		dir = next
	}
	return nil
}

func parseCommandJSON(raw string) ([]string, error) {
	var command []string
	if err := json.Unmarshal([]byte(raw), &command); err != nil {
		return nil, fmt.Errorf("%s must be a JSON string array: %w", envCommandJSON, err)
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("%s must not be empty when set", envCommandJSON)
	}
	for _, arg := range command {
		if strings.TrimSpace(arg) == "" {
			return nil, fmt.Errorf("%s arguments must be non-empty", envCommandJSON)
		}
	}
	return command, nil
}

func parsePositiveInt64(name, raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func createDefaultAgentConfig(agentName string) (string, error) {
	root, err := os.MkdirTemp("", "openclarion-container-provider-agent-config-*")
	if err != nil {
		return "", fmt.Errorf("create agent config root: %w", err)
	}
	agentDir := filepath.Join(root, agentName)
	// #nosec G301 -- the directory is mounted readonly into a non-root sandbox
	// user, so it needs search/read bits inside the container mount.
	if err := os.Mkdir(agentDir, 0o755); err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("create agent config dir: %w", err)
	}
	path := filepath.Join(agentDir, "agent.yaml")
	content := []byte("name: container-provider-smoke\nmode: docker-provider-live-smoke\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("write agent config: %w", err)
	}
	// #nosec G302 -- the file is mounted readonly into a non-root sandbox user.
	if err := os.Chmod(path, 0o644); err != nil {
		_ = os.RemoveAll(root)
		return "", fmt.Errorf("chmod agent config: %w", err)
	}
	return root, nil
}

func validateSmokeOutput(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("provider output is empty")
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("provider output is invalid JSON: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("provider output is invalid JSON: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("provider output must be a JSON object")
	}
	return nil
}

func environMap(environ []string) map[string]string {
	out := make(map[string]string, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
