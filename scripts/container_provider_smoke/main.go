// Command container_provider_smoke runs the Docker-backed ContainerProvider
// against a real local Docker daemon for the manual M4 runtime execution gate.
package main

import (
	"context"
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

	defaultAgentName      = "container-provider-smoke"
	defaultTimeoutSeconds = 60
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
	return handleProviderRunResult(cfg, result, err, stdout)
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

func handleProviderRunResult(cfg smokeConfig, result ports.ContainerRunResult, runErr error, stdout io.Writer) error {
	if cfg.ExpectError != "" {
		if runErr == nil {
			return fmt.Errorf("provider run succeeded, want error containing %q", cfg.ExpectError)
		}
		if !errorContainsAny(runErr, cfg.ExpectError) {
			return fmt.Errorf("provider run error does not contain %q: %w", cfg.ExpectError, runErr)
		}
		fmt.Fprintf(stdout, "[container-provider-smoke] OK - expected provider error containing %q\n", cfg.ExpectError)
		return nil
	}
	if runErr != nil {
		return fmt.Errorf("provider run: %w", runErr)
	}
	if err := validateSmokeOutput(result.Output); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[container-provider-smoke] OK - runtime_id=%s output_bytes=%d\n", result.RuntimeID, len(result.Output))
	return nil
}

func errorContainsAny(err error, patterns string) bool {
	for _, pattern := range strings.Split(patterns, "||") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.Contains(err.Error(), pattern) {
			return true
		}
	}
	return false
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
