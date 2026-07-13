package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	dockerstdcopy "github.com/moby/moby/api/pkg/stdcopy"
	dockercontainer "github.com/moby/moby/api/types/container"
	dockermount "github.com/moby/moby/api/types/mount"
	dockernetwork "github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	defaultStopTimeoutSeconds = 2
	removeTimeout             = 10 * time.Second
	outputCopyTimeout         = 10 * time.Second
	failureLogTimeout         = 3 * time.Second
	failureLogTailLines       = "80"
	failureLogCaptureBytes    = 64 * 1024
	failureLogEmitBytes       = 4096
	dockerLogFrameHeaderBytes = 8
	sandboxInputFileMode      = 0o644
	sandboxOutputDirMode      = 0o777
	egressReadinessTimeout    = 10 * time.Second
	egressReadinessMemory     = 128 * 1024 * 1024
	egressReadinessPidsLimit  = 64
	diagnosisRunnerEntrypoint = "/diagnosis-assistant-runner"
	egressReadinessCommand    = "readiness"
	labelComponent            = "openclarion.component"
	labelInvocationID         = "openclarion.invocation_id"
	labelAgentName            = "openclarion.agent_name"
)

// EngineClient is the small Docker Engine surface used by Provider.
// Tests use a fake implementation so cleanup, timeout, and output-copy
// semantics stay covered without requiring a local Docker daemon.
type EngineClient interface {
	NetworkInspect(ctx context.Context, networkID string, options dockerclient.NetworkInspectOptions) (dockerclient.NetworkInspectResult, error)
	ContainerCreate(ctx context.Context, options dockerclient.ContainerCreateOptions) (dockerclient.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options dockerclient.ContainerStartOptions) (dockerclient.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options dockerclient.ContainerWaitOptions) dockerclient.ContainerWaitResult
	ContainerStop(ctx context.Context, containerID string, options dockerclient.ContainerStopOptions) (dockerclient.ContainerStopResult, error)
	ContainerKill(ctx context.Context, containerID string, options dockerclient.ContainerKillOptions) (dockerclient.ContainerKillResult, error)
	ContainerRemove(ctx context.Context, containerID string, options dockerclient.ContainerRemoveOptions) (dockerclient.ContainerRemoveResult, error)
	CopyFromContainer(ctx context.Context, containerID string, options dockerclient.CopyFromContainerOptions) (dockerclient.CopyFromContainerResult, error)
	ContainerLogs(ctx context.Context, containerID string, options dockerclient.ContainerLogsOptions) (dockerclient.ContainerLogsResult, error)
}

// EgressEnforcer verifies that a provider-specific network mode enforces
// the requested allowlist before Docker Engine can create the container.
type EgressEnforcer interface {
	Validate(ctx context.Context, policy ports.ContainerNetworkPolicy, networkMode string) error
}

// Provider executes one ADR-0013 sandbox invocation through Docker Engine.
type Provider struct {
	engine             EngineClient
	cfg                Config
	agentConfigRoot    string
	workspaceRoot      string
	stopTimeoutSeconds int
	egressEnforcer     EgressEnforcer
	now                func() time.Time
}

var _ ports.ContainerProvider = (*Provider)(nil)

// ProviderOption customizes the Docker provider.
type ProviderOption func(*Provider)

// WithWorkspaceRoot sets the host directory used for per-invocation input files.
func WithWorkspaceRoot(path string) ProviderOption {
	return func(p *Provider) {
		p.workspaceRoot = path
	}
}

// WithStopTimeoutSeconds sets the graceful stop timeout used before kill.
func WithStopTimeoutSeconds(seconds int) ProviderOption {
	return func(p *Provider) {
		p.stopTimeoutSeconds = seconds
	}
}

// WithEgressEnforcer enables allowlist-mode runs after an external proxy or
// firewall controller has verified the requested egress policy.
func WithEgressEnforcer(enforcer EgressEnforcer) ProviderOption {
	return func(p *Provider) {
		p.egressEnforcer = enforcer
	}
}

// NewProvider constructs a Docker-backed ContainerProvider from an injected
// EngineClient and an agent-config root directory.
func NewProvider(engine EngineClient, cfg Config, agentConfigRoot string, opts ...ProviderOption) (*Provider, error) {
	if engine == nil {
		return nil, fmt.Errorf("docker provider engine client is required")
	}
	normalized, err := cfg.Normalized()
	if err != nil {
		return nil, err
	}
	if agentConfigRoot == "" {
		return nil, fmt.Errorf("docker provider agent config root is required")
	}
	p := &Provider{
		engine:             engine,
		cfg:                normalized,
		agentConfigRoot:    agentConfigRoot,
		stopTimeoutSeconds: defaultStopTimeoutSeconds,
		now:                time.Now,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.stopTimeoutSeconds < 0 {
		return nil, fmt.Errorf("docker provider stop timeout must be non-negative")
	}
	return p, nil
}

// NewProviderFromEnv constructs a Docker-backed ContainerProvider using the
// standard Docker environment variables handled by the official Go SDK.
func NewProviderFromEnv(cfg Config, agentConfigRoot string, opts ...ProviderOption) (*Provider, error) {
	engine, err := dockerclient.New(dockerclient.FromEnv, dockerclient.WithUserAgent("openclarion"))
	if err != nil {
		return nil, err
	}
	return NewProvider(engine, cfg, agentConfigRoot, opts...)
}

// CheckEgressReadiness starts the configured diagnosis image on the same
// internal network as real turns and requires its readiness command to verify
// the live proxy allowlist fingerprint before the control plane starts serving.
func (p *Provider) CheckEgressReadiness(
	ctx context.Context,
	llmBaseURL string,
	policy ports.ContainerNetworkPolicy,
) (err error) {
	if p == nil {
		return fmt.Errorf("docker provider is nil")
	}
	if ctx == nil {
		return fmt.Errorf("docker readiness context is required")
	}
	if policy.EffectiveMode() != ports.ContainerNetworkAllowlist {
		return fmt.Errorf("docker egress readiness requires network mode %q", ports.ContainerNetworkAllowlist)
	}
	if err := ports.ValidateContainerEgressURL(llmBaseURL, policy.AllowedEgress); err != nil {
		return fmt.Errorf("docker egress readiness LLM target: %w", err)
	}
	allowed, err := ports.NormalizeContainerEgressTargets(policy.AllowedEgress)
	if err != nil {
		return fmt.Errorf("docker egress readiness allowlist: %w", err)
	}
	if p.cfg.EgressProxyURL == "" {
		return fmt.Errorf("docker egress readiness proxy URL is required")
	}

	probeCtx, cancel := context.WithTimeout(ctx, egressReadinessTimeout)
	defer cancel()
	if err := p.validateEgress(probeCtx, policy, p.cfg.AllowlistNetworkMode); err != nil {
		return err
	}

	create, err := p.engine.ContainerCreate(
		probeCtx,
		buildEgressReadinessCreateOptions(p.cfg, llmBaseURL, allowed),
	)
	if err != nil {
		return fmt.Errorf("docker egress readiness container create: %w", err)
	}
	if create.ID == "" {
		return fmt.Errorf("docker egress readiness container create returned empty id")
	}
	defer func() {
		err = errors.Join(err, p.removeContainer(create.ID))
	}()
	if _, err := p.engine.ContainerStart(probeCtx, create.ID, dockerclient.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("docker egress readiness container start %s: %w", create.ID, err)
	}
	wait := p.engine.ContainerWait(probeCtx, create.ID, dockerclient.ContainerWaitOptions{
		Condition: dockercontainer.WaitConditionNotRunning,
	})
	response, err := p.waitForExit(probeCtx, create.ID, wait)
	if err != nil {
		return fmt.Errorf("docker egress readiness: %w", err)
	}
	if response.Error != nil && response.Error.Message != "" {
		return fmt.Errorf("docker egress readiness container %s wait error: %s", create.ID, response.Error.Message)
	}
	if response.StatusCode != 0 {
		return &ports.ContainerExitError{
			RuntimeID:  create.ID,
			ExitCode:   int(response.StatusCode),
			Diagnostic: p.failureLogDiagnostic(create.ID),
		}
	}
	return nil
}

// Run prepares readonly input files, starts a sandbox container, waits for
// completion, copies output.json, validates raw lifecycle invariants, and
// removes the runtime resource on every path.
func (p *Provider) Run(ctx context.Context, req ports.ContainerRunRequest) (result ports.ContainerRunResult, err error) {
	if p == nil {
		return ports.ContainerRunResult{}, fmt.Errorf("docker provider is nil")
	}
	if err := req.Validate(); err != nil {
		return ports.ContainerRunResult{}, err
	}

	workspace, cleanup, err := p.prepareWorkspace(req)
	if err != nil {
		return ports.ContainerRunResult{}, err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	spec, err := BuildRunSpec(p.cfg, req, workspace)
	if err != nil {
		return ports.ContainerRunResult{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	if err := p.validateEgress(runCtx, req.Network, spec.NetworkMode); err != nil {
		return ports.ContainerRunResult{}, err
	}
	if err := req.ValidateCredentialExpirations(p.now().UTC()); err != nil {
		return ports.ContainerRunResult{}, err
	}

	create, err := p.engine.ContainerCreate(runCtx, buildCreateOptions(spec, req))
	if err != nil {
		return ports.ContainerRunResult{}, fmt.Errorf("docker container create: %w", err)
	}
	if create.ID == "" {
		return ports.ContainerRunResult{}, fmt.Errorf("docker container create returned empty id")
	}
	result.RuntimeID = create.ID
	defer func() {
		err = errors.Join(err, p.removeContainer(result.RuntimeID))
	}()

	startedAt := p.now().UTC()
	if _, err := p.engine.ContainerStart(runCtx, result.RuntimeID, dockerclient.ContainerStartOptions{}); err != nil {
		return result, fmt.Errorf("docker container start %s: %w", result.RuntimeID, err)
	}

	wait := p.engine.ContainerWait(runCtx, result.RuntimeID, dockerclient.ContainerWaitOptions{
		Condition: dockercontainer.WaitConditionNotRunning,
	})

	waitResponse, err := p.waitForExit(runCtx, result.RuntimeID, wait)
	finishedAt := p.now().UTC()
	result.InvocationID = req.InvocationID
	result.AgentName = req.AgentName
	result.StartedAt = startedAt
	result.FinishedAt = finishedAt
	if err != nil {
		return result, err
	}
	result.ExitCode = int(waitResponse.StatusCode)
	if waitResponse.Error != nil && waitResponse.Error.Message != "" {
		return result, fmt.Errorf("docker container %s wait error: %s", result.RuntimeID, waitResponse.Error.Message)
	}
	if result.ExitCode != 0 {
		return result, &ports.ContainerExitError{
			RuntimeID:  result.RuntimeID,
			ExitCode:   result.ExitCode,
			Diagnostic: p.failureLogDiagnostic(result.RuntimeID),
		}
	}

	output, err := p.copyOutput(ctx, result.RuntimeID, spec.OutputPath, req.EffectiveOutputMax())
	if err != nil {
		return result, err
	}
	result.Output = output
	if err := ports.ValidateContainerRunResult(req, result); err != nil {
		return result, fmt.Errorf("docker container result invalid: %w", err)
	}
	return result, nil
}

func (p *Provider) validateEgress(ctx context.Context, policy ports.ContainerNetworkPolicy, networkMode string) error {
	if policy.EffectiveMode() != ports.ContainerNetworkAllowlist {
		return nil
	}
	if p.egressEnforcer == nil {
		return fmt.Errorf("docker provider requires an egress enforcer for allowlist network mode")
	}
	if err := p.egressEnforcer.Validate(ctx, policy, networkMode); err != nil {
		return fmt.Errorf("docker provider egress allowlist is not enforced: %w", err)
	}
	network, err := p.engine.NetworkInspect(ctx, networkMode, dockerclient.NetworkInspectOptions{})
	if err != nil {
		return fmt.Errorf("docker provider inspect allowlist network %q: %w", networkMode, err)
	}
	if network.Network.Name != networkMode {
		return fmt.Errorf("docker provider allowlist network name = %q, want %q", network.Network.Name, networkMode)
	}
	if !network.Network.Internal {
		return fmt.Errorf("docker provider allowlist network %q must be internal", networkMode)
	}
	if network.Network.Ingress || network.Network.ConfigOnly {
		return fmt.Errorf("docker provider allowlist network %q must not be ingress or config-only", networkMode)
	}
	return nil
}

func (p *Provider) waitForExit(ctx context.Context, containerID string, wait dockerclient.ContainerWaitResult) (dockercontainer.WaitResponse, error) {
	resultCh := wait.Result
	errorCh := wait.Error
	for resultCh != nil || errorCh != nil {
		select {
		case err, ok := <-errorCh:
			if !ok {
				errorCh = nil
				continue
			}
			if err == nil {
				errorCh = nil
				continue
			}
			return dockercontainer.WaitResponse{}, fmt.Errorf("docker container wait %s: %w", containerID, err)
		case response, ok := <-resultCh:
			if !ok {
				resultCh = nil
				continue
			}
			return response, nil
		case <-ctx.Done():
			stopErr := p.stopContainer(containerID)
			return dockercontainer.WaitResponse{}, errors.Join(fmt.Errorf("docker container %s timed out or was cancelled: %w", containerID, ctx.Err()), stopErr)
		}
	}
	return dockercontainer.WaitResponse{}, fmt.Errorf("docker container wait %s ended without result", containerID)
}

func (p *Provider) stopContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(p.stopTimeoutSeconds+2)*time.Second)
	defer cancel()
	timeout := p.stopTimeoutSeconds
	if _, err := p.engine.ContainerStop(ctx, containerID, dockerclient.ContainerStopOptions{Timeout: &timeout}); err != nil {
		if _, killErr := p.engine.ContainerKill(ctx, containerID, dockerclient.ContainerKillOptions{}); killErr != nil {
			return errors.Join(fmt.Errorf("docker container stop %s: %w", containerID, err), fmt.Errorf("docker container kill %s: %w", containerID, killErr))
		}
		return fmt.Errorf("docker container stop %s: %w", containerID, err)
	}
	return nil
}

func (p *Provider) removeContainer(containerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	_, err := p.engine.ContainerRemove(ctx, containerID, dockerclient.ContainerRemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	})
	if err != nil {
		return fmt.Errorf("docker container remove %s: %w", containerID, err)
	}
	return nil
}

func (p *Provider) copyOutput(ctx context.Context, containerID, outputPath string, outputMax int64) (json.RawMessage, error) {
	copyCtx, cancel := context.WithTimeout(ctx, outputCopyTimeout)
	defer cancel()
	copied, err := p.engine.CopyFromContainer(copyCtx, containerID, dockerclient.CopyFromContainerOptions{
		SourcePath: outputPath,
	})
	if err != nil {
		return nil, fmt.Errorf("docker container copy %s:%s: %w", containerID, outputPath, err)
	}
	if copied.Content == nil {
		return nil, fmt.Errorf("docker container copy %s:%s returned empty content", containerID, outputPath)
	}
	defer copied.Content.Close()
	output, err := readOutputArchive(copied.Content, outputPath, outputMax)
	if err != nil {
		return nil, fmt.Errorf("docker container output %s:%s: %w", containerID, outputPath, err)
	}
	return output, nil
}

func (p *Provider) failureLogDiagnostic(containerID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), failureLogTimeout)
	defer cancel()
	logs, err := p.engine.ContainerLogs(ctx, containerID, dockerclient.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       failureLogTailLines,
	})
	if err != nil {
		return fmt.Sprintf("logs_unavailable=%q", sanitizeFailureLogText(err.Error()))
	}
	if logs == nil {
		return "logs_unavailable=\"empty log stream\""
	}
	defer logs.Close()

	stdout := newFailureLogTail()
	stderr := newFailureLogTail()
	if err := copyDockerLogFrames(logs, stdout, stderr); err != nil {
		return fmt.Sprintf("logs_unavailable=%q", sanitizeFailureLogText(err.Error()))
	}
	stdoutTail := sanitizeFailureLogText(stdout.safeString())
	stderrTail := sanitizeFailureLogText(stderr.safeString())
	fields := []string{
		fmt.Sprintf("stdout_bytes=%d", stdout.total),
		fmt.Sprintf("stderr_bytes=%d", stderr.total),
		fmt.Sprintf("stdout_sha256=%s", failureLogSHA256([]byte(stdoutTail))),
		fmt.Sprintf("stderr_sha256=%s", failureLogSHA256([]byte(stderrTail))),
	}
	if stdoutTail != "" {
		fields = append(fields, fmt.Sprintf("stdout_tail=%q", stdoutTail))
	}
	if stderrTail != "" {
		fields = append(fields, fmt.Sprintf("stderr_tail=%q", stderrTail))
	}
	return "logs " + strings.Join(fields, " ")
}

func copyDockerLogFrames(reader io.Reader, stdout io.Writer, stderr io.Writer) error {
	header := make([]byte, dockerLogFrameHeaderBytes)
	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return fmt.Errorf("docker log stream ended with a partial frame header")
			}
			return fmt.Errorf("read docker log frame header: %w", err)
		}
		size := int64(binary.BigEndian.Uint32(header[4:8]))
		var target io.Writer
		switch dockerstdcopy.StdType(header[0]) {
		case dockerstdcopy.Stdin, dockerstdcopy.Stdout:
			target = stdout
		case dockerstdcopy.Stderr:
			target = stderr
		case dockerstdcopy.Systemerr:
			systemErr := newFailureLogTail()
			if err := copyDockerLogFramePayload(reader, systemErr, size); err != nil {
				return err
			}
			return fmt.Errorf("docker log system stream: %s", sanitizeFailureLogText(systemErr.safeString()))
		default:
			if err := copyDockerLogFramePayload(reader, io.Discard, size); err != nil {
				return err
			}
			return fmt.Errorf("docker log stream has unsupported stream type %d", header[0])
		}
		if err := copyDockerLogFramePayload(reader, target, size); err != nil {
			return err
		}
	}
}

func copyDockerLogFramePayload(reader io.Reader, target io.Writer, size int64) error {
	if size == 0 {
		return nil
	}
	if _, err := io.CopyN(target, reader, size); err != nil {
		return fmt.Errorf("read docker log frame payload: %w", err)
	}
	return nil
}

type failureLogTail struct {
	buf       []byte
	total     int64
	truncated bool
}

func newFailureLogTail() *failureLogTail {
	return &failureLogTail{buf: make([]byte, 0, failureLogCaptureBytes)}
}

func (b *failureLogTail) Write(p []byte) (int, error) {
	written := len(p)
	b.total += int64(written)
	if written >= failureLogCaptureBytes {
		b.buf = append(b.buf[:0], p[written-failureLogCaptureBytes:]...)
		b.truncated = true
		return written, nil
	}
	b.buf = append(b.buf, p...)
	if overflow := len(b.buf) - failureLogCaptureBytes; overflow > 0 {
		copy(b.buf, b.buf[overflow:])
		b.buf = b.buf[:failureLogCaptureBytes]
		b.truncated = true
	}
	return written, nil
}

func (b *failureLogTail) safeString() string {
	raw := b.buf
	if b.truncated {
		newline := bytes.IndexByte(raw, '\n')
		if newline < 0 || newline+1 >= len(raw) {
			return ""
		}
		raw = raw[newline+1:]
	}
	return string(raw)
}

func failureLogSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

var failureLogRedactors = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{
		pattern:     regexp.MustCompile(`(?i)(bearer[[:space:]]+)[A-Za-z0-9._~+/\-=]+`),
		replacement: `${1}<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|token|secret|password|authorization|webhook(?:[_-]?url)?)[[:space:]]*[:=][[:space:]]*["']?)[^"',;[:space:]]+`),
		replacement: `${1}<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)((?:[A-Z0-9_]*(?:API_KEY|TOKEN|SECRET|PASSWORD|WEBHOOK_URL|BASE_URL|ENDPOINT|URL))=)[^[:space:]]+`),
		replacement: `${1}<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`(?i)([?&](?:key|token|access_token|api_key|apikey|secret)=)[^&[:space:]"']+`),
		replacement: `${1}<redacted>`,
	},
	{
		pattern:     regexp.MustCompile(`https?://[^[:space:]"']+`),
		replacement: `<redacted-url>`,
	},
	{
		pattern:     regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._~+/\-]{31,}`),
		replacement: `<redacted-token>`,
	},
}

func sanitizeFailureLogText(raw string) string {
	if raw == "" || failureLogEmitBytes <= 0 {
		return ""
	}
	out := strings.ToValidUTF8(raw, "?")
	out = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		case '\r':
			return '\n'
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, out)
	for _, redactor := range failureLogRedactors {
		out = redactor.pattern.ReplaceAllString(out, redactor.replacement)
	}
	out = strings.TrimSpace(out)
	if len(out) <= failureLogEmitBytes {
		return out
	}
	out = out[len(out)-failureLogEmitBytes:]
	if newline := strings.IndexByte(out, '\n'); newline >= 0 && newline+1 < len(out) {
		out = out[newline+1:]
	}
	return strings.TrimSpace(out)
}

func (p *Provider) prepareWorkspace(req ports.ContainerRunRequest) (WorkspacePaths, func() error, error) {
	dir, err := os.MkdirTemp(p.workspaceRoot, "openclarion-sandbox-*")
	if err != nil {
		return WorkspacePaths{}, nil, fmt.Errorf("create sandbox workspace: %w", err)
	}
	cleanup := func() error {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove sandbox workspace %s: %w", dir, err)
		}
		return nil
	}

	agentConfigDir := filepath.Join(p.agentConfigRoot, req.AgentName)
	info, err := os.Stat(agentConfigDir)
	if err != nil {
		_ = cleanup()
		return WorkspacePaths{}, nil, fmt.Errorf("agent config dir %s: %w", agentConfigDir, err)
	}
	if !info.IsDir() {
		_ = cleanup()
		return WorkspacePaths{}, nil, fmt.Errorf("agent config path %s is not a directory", agentConfigDir)
	}
	outputDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		_ = cleanup()
		return WorkspacePaths{}, nil, fmt.Errorf("create output dir: %w", err)
	}
	// #nosec G302 -- this private host directory is bind-mounted as the only
	// writable sandbox path for an arbitrary non-root container UID.
	if err := os.Chmod(outputDir, sandboxOutputDirMode); err != nil {
		_ = cleanup()
		return WorkspacePaths{}, nil, fmt.Errorf("chmod output dir: %w", err)
	}

	evidencePath := filepath.Join(dir, "evidence.json")
	if err := writeSandboxInputFile(evidencePath, req.Evidence); err != nil {
		_ = cleanup()
		return WorkspacePaths{}, nil, fmt.Errorf("write evidence input: %w", err)
	}
	workspace := WorkspacePaths{
		EvidencePath:   evidencePath,
		AgentConfigDir: agentConfigDir,
		OutputDir:      outputDir,
	}
	if len(req.Conversation) != 0 {
		workspace.ConversationPath = filepath.Join(dir, "conversation.json")
		if err := writeSandboxInputFile(workspace.ConversationPath, req.Conversation); err != nil {
			_ = cleanup()
			return WorkspacePaths{}, nil, fmt.Errorf("write conversation input: %w", err)
		}
	}
	if len(req.Message) != 0 {
		workspace.MessagePath = filepath.Join(dir, "message.json")
		if err := writeSandboxInputFile(workspace.MessagePath, req.Message); err != nil {
			_ = cleanup()
			return WorkspacePaths{}, nil, fmt.Errorf("write message input: %w", err)
		}
	}
	return workspace, cleanup, nil
}

func writeSandboxInputFile(path string, content json.RawMessage) error {
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return err
	}
	// #nosec G302 -- these files are mounted readonly into a non-root sandbox user;
	// the parent temp directory remains private to the control-plane process.
	if err := os.Chmod(path, sandboxInputFileMode); err != nil {
		return err
	}
	return nil
}

func buildCreateOptions(spec RunSpec, req ports.ContainerRunRequest) dockerclient.ContainerCreateOptions {
	networkDisabled := spec.NetworkMode == string(ports.ContainerNetworkNone)
	pidsLimit := spec.PidsLimit
	mounts := make([]dockermount.Mount, 0, len(spec.BindMounts)+1)
	for _, bind := range spec.BindMounts {
		mounts = append(mounts, dockermount.Mount{
			Type:     dockermount.TypeBind,
			Source:   bind.Source,
			Target:   bind.Target,
			ReadOnly: bind.ReadOnly,
		})
	}
	mounts = append(mounts, dockermount.Mount{
		Type:     dockermount.TypeBind,
		Source:   spec.OutputMount.Source,
		Target:   spec.OutputMount.Target,
		ReadOnly: spec.OutputMount.ReadOnly,
	})
	return dockerclient.ContainerCreateOptions{
		Config: &dockercontainer.Config{
			User:            spec.User,
			Cmd:             cloneStringSlice(spec.Command),
			Env:             runtimeEnv(spec, req.Credentials),
			WorkingDir:      spec.WorkingDir,
			NetworkDisabled: networkDisabled,
			Labels: map[string]string{
				labelComponent:    "agent-sandbox",
				labelInvocationID: req.InvocationID,
				labelAgentName:    req.AgentName,
			},
			Tty:       false,
			OpenStdin: false,
		},
		HostConfig: &dockercontainer.HostConfig{
			NetworkMode:    dockercontainer.NetworkMode(spec.NetworkMode),
			Privileged:     spec.Privileged,
			ReadonlyRootfs: spec.ReadonlyRootFS,
			SecurityOpt:    cloneStringSlice(spec.SecurityOpt),
			CapDrop:        cloneStringSlice(spec.CapDrop),
			AutoRemove:     false,
			RestartPolicy:  dockercontainer.RestartPolicy{},
			Resources: dockercontainer.Resources{
				Memory:    spec.MemoryBytes,
				NanoCPUs:  spec.NanoCPUs,
				PidsLimit: &pidsLimit,
				Ulimits: []*dockercontainer.Ulimit{{
					Name: "fsize",
					Soft: spec.OutputMaxBytes,
					Hard: spec.OutputMaxBytes,
				}},
			},
			Mounts: mounts,
		},
		NetworkingConfig: networkingConfigFor(spec),
		Image:            spec.ImageRef,
	}
}

func buildEgressReadinessCreateOptions(
	cfg Config,
	llmBaseURL string,
	allowed []string,
) dockerclient.ContainerCreateOptions {
	pidsLimit := int64(egressReadinessPidsLimit)
	networkMode := cfg.AllowlistNetworkMode
	return dockerclient.ContainerCreateOptions{
		Config: &dockercontainer.Config{
			User:       cfg.User,
			Entrypoint: []string{diagnosisRunnerEntrypoint},
			Cmd:        []string{egressReadinessCommand},
			Env: []string{
				"OPENCLARION_DIAGNOSIS_LLM_BASE_URL=" + llmBaseURL,
				"OPENCLARION_SANDBOX_EGRESS_ALLOWED=" + strings.Join(allowed, ","),
				"OPENCLARION_SANDBOX_EGRESS_PROXY_URL=" + cfg.EgressProxyURL,
			},
			Labels: map[string]string{
				labelComponent: "agent-sandbox-readiness",
			},
			Tty:       false,
			OpenStdin: false,
		},
		HostConfig: &dockercontainer.HostConfig{
			NetworkMode:    dockercontainer.NetworkMode(networkMode),
			Privileged:     false,
			ReadonlyRootfs: true,
			SecurityOpt:    []string{NoNewPrivilegesSecurityOpt},
			CapDrop:        []string{DropAllCapabilities},
			AutoRemove:     false,
			RestartPolicy:  dockercontainer.RestartPolicy{},
			Resources: dockercontainer.Resources{
				Memory:    egressReadinessMemory,
				NanoCPUs:  cfg.NanoCPUs,
				PidsLimit: &pidsLimit,
			},
		},
		NetworkingConfig: &dockernetwork.NetworkingConfig{
			EndpointsConfig: map[string]*dockernetwork.EndpointSettings{
				networkMode: {},
			},
		},
		Image: cfg.ImageRef,
	}
}

func networkingConfigFor(spec RunSpec) *dockernetwork.NetworkingConfig {
	if spec.NetworkMode == "" || spec.NetworkMode == string(ports.ContainerNetworkNone) {
		return &dockernetwork.NetworkingConfig{}
	}
	return &dockernetwork.NetworkingConfig{
		EndpointsConfig: map[string]*dockernetwork.EndpointSettings{
			spec.NetworkMode: &dockernetwork.EndpointSettings{},
		},
	}
}

func credentialEnv(credentials []ports.ContainerCredential) []string {
	if len(credentials) == 0 {
		return nil
	}
	out := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		out = append(out, credential.Name+"="+credential.Value)
	}
	return out
}

func runtimeEnv(spec RunSpec, credentials []ports.ContainerCredential) []string {
	out := credentialEnv(credentials)
	if spec.EgressProxyURL == "" {
		return out
	}
	return append(out,
		"HTTP_PROXY="+spec.EgressProxyURL,
		"HTTPS_PROXY="+spec.EgressProxyURL,
		"ALL_PROXY=",
		"NO_PROXY=",
		"http_proxy="+spec.EgressProxyURL,
		"https_proxy="+spec.EgressProxyURL,
		"all_proxy=",
		"no_proxy=",
	)
}

func readOutputArchive(reader io.Reader, outputPath string, outputMax int64) (json.RawMessage, error) {
	if outputMax <= 0 {
		return nil, fmt.Errorf("output max must be positive")
	}
	tr := tar.NewReader(reader)
	wantName := path.Base(outputPath)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s not found in archive", wantName)
		}
		if errors.Is(err, tar.ErrInsecurePath) {
			if header == nil {
				return nil, fmt.Errorf("read archive header: %w", err)
			}
		} else if err != nil {
			return nil, fmt.Errorf("read archive header: %w", err)
		}
		memberName, err := outputArchiveMemberName(header.Name)
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("output archive member %q must be a regular file", header.Name)
		}
		if memberName != wantName {
			return nil, fmt.Errorf("unexpected output archive member %q, want %q", header.Name, wantName)
		}
		if header.Size > outputMax {
			return nil, fmt.Errorf("output size %d exceeds maximum %d", header.Size, outputMax)
		}
		var buf bytes.Buffer
		limited := io.LimitReader(tr, outputMax+1)
		if _, err := io.Copy(&buf, limited); err != nil {
			return nil, fmt.Errorf("read output file: %w", err)
		}
		if int64(buf.Len()) > outputMax {
			return nil, fmt.Errorf("output size %d exceeds maximum %d", buf.Len(), outputMax)
		}
		out := json.RawMessage(buf.Bytes())
		if !json.Valid(out) {
			return nil, fmt.Errorf("output is not valid JSON")
		}
		return out, nil
	}
}

func outputArchiveMemberName(name string) (string, error) {
	clean := path.Clean(name)
	if name == "" || clean == "." || clean == ".." || path.IsAbs(name) || strings.Contains(name, `\`) || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("output archive member path %q is not allowed", name)
	}
	if clean != name {
		return "", fmt.Errorf("output archive member path %q is not normalized", name)
	}
	if path.Base(clean) != clean {
		return "", fmt.Errorf("output archive member %q must be a top-level file", name)
	}
	return clean, nil
}
