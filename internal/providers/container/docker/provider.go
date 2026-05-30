package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

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
	sandboxInputFileMode      = 0o644
	sandboxOutputDirMode      = 0o777
	labelComponent            = "openclarion.component"
	labelInvocationID         = "openclarion.invocation_id"
	labelAgentName            = "openclarion.agent_name"
)

// EngineClient is the small Docker Engine surface used by Provider.
// Tests use a fake implementation so cleanup, timeout, and output-copy
// semantics stay covered without requiring a local Docker daemon.
type EngineClient interface {
	ContainerCreate(ctx context.Context, options dockerclient.ContainerCreateOptions) (dockerclient.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options dockerclient.ContainerStartOptions) (dockerclient.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options dockerclient.ContainerWaitOptions) dockerclient.ContainerWaitResult
	ContainerStop(ctx context.Context, containerID string, options dockerclient.ContainerStopOptions) (dockerclient.ContainerStopResult, error)
	ContainerKill(ctx context.Context, containerID string, options dockerclient.ContainerKillOptions) (dockerclient.ContainerKillResult, error)
	ContainerRemove(ctx context.Context, containerID string, options dockerclient.ContainerRemoveOptions) (dockerclient.ContainerRemoveResult, error)
	CopyFromContainer(ctx context.Context, containerID string, options dockerclient.CopyFromContainerOptions) (dockerclient.CopyFromContainerResult, error)
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
		return result, fmt.Errorf("docker container %s exited with code %d", result.RuntimeID, result.ExitCode)
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
			Env:             credentialEnv(req.Credentials),
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
		NetworkingConfig: &dockernetwork.NetworkingConfig{},
		Image:            spec.ImageRef,
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
		if err != nil {
			return nil, fmt.Errorf("read archive header: %w", err)
		}
		memberName, err := outputArchiveMemberName(header.Name)
		if err != nil {
			return nil, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if memberName != wantName {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return nil, fmt.Errorf("output archive member %q must be a regular file", header.Name)
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
