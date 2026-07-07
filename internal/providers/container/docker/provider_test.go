package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockerstdcopy "github.com/moby/moby/api/pkg/stdcopy"
	dockercontainer "github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestProviderRunCreatesSecureContainerCopiesOutputAndRemoves(t *testing.T) {
	req := validRequest()
	req.Conversation = json.RawMessage(`[]`)
	req.Message = json.RawMessage(`{"role":"user","content":"next"}`)
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider := newTestProvider(t, engine)

	got, err := provider.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.RuntimeID != "container-1" {
		t.Fatalf("RuntimeID = %q, want container-1", got.RuntimeID)
	}
	if string(got.Output) != `{"summary":"ok"}` {
		t.Fatalf("Output = %s", got.Output)
	}
	if got.StartedAt.IsZero() || got.FinishedAt.IsZero() || got.FinishedAt.Before(got.StartedAt) {
		t.Fatalf("bad timestamps: started=%s finished=%s", got.StartedAt, got.FinishedAt)
	}

	create := engine.createOptions
	if create.Config == nil || create.HostConfig == nil {
		t.Fatalf("create options missing config or host config: %#v", create)
	}
	if create.Config.Image != "" || create.Image != pinnedImage {
		t.Fatalf("image not pinned through create options: %#v", create)
	}
	if !create.Config.NetworkDisabled {
		t.Fatalf("NetworkDisabled = false, want true for network-none")
	}
	if create.Config.Labels[labelInvocationID] != req.InvocationID {
		t.Fatalf("invocation label = %q", create.Config.Labels[labelInvocationID])
	}
	if !create.HostConfig.ReadonlyRootfs {
		t.Fatalf("ReadonlyRootfs = false")
	}
	if create.HostConfig.NetworkMode != dockercontainer.NetworkMode("none") {
		t.Fatalf("NetworkMode = %q, want none", create.HostConfig.NetworkMode)
	}
	if create.HostConfig.Resources.PidsLimit == nil || *create.HostConfig.Resources.PidsLimit != DefaultPidsLimit {
		t.Fatalf("PidsLimit = %#v, want %d", create.HostConfig.Resources.PidsLimit, DefaultPidsLimit)
	}
	if len(create.HostConfig.Resources.Ulimits) != 1 ||
		create.HostConfig.Resources.Ulimits[0].Name != "fsize" ||
		create.HostConfig.Resources.Ulimits[0].Soft != req.EffectiveOutputMax() ||
		create.HostConfig.Resources.Ulimits[0].Hard != req.EffectiveOutputMax() {
		t.Fatalf("Ulimits = %#v", create.HostConfig.Resources.Ulimits)
	}
	if engine.waitOptions.Condition != dockercontainer.WaitConditionNotRunning {
		t.Fatalf("WaitCondition = %q, want %q", engine.waitOptions.Condition, dockercontainer.WaitConditionNotRunning)
	}
	assertEngineMounted(t, create, ports.SandboxEvidencePath)
	assertEngineMounted(t, create, ports.SandboxConversationPath)
	assertEngineMounted(t, create, ports.SandboxMessagePath)
	assertEngineMounted(t, create, ports.SandboxAgentConfigPath)
	assertEngineMountedWritable(t, create, ports.SandboxOutputDir)
	if engine.evidence != string(req.Evidence) {
		t.Fatalf("evidence workspace content = %s, want %s", engine.evidence, req.Evidence)
	}
	if len(create.Config.Env) != 0 {
		t.Fatalf("Env = %v, want empty by default", create.Config.Env)
	}
	engine.assertCalls(t, "create", "start", "wait", "copy", "remove")
}

func TestProviderRunInjectsShortLivedCredentialsIntoContainerEnv(t *testing.T) {
	req := validRequest()
	req.Credentials = []ports.ContainerCredential{{
		Name:      "OPENCLARION_PROVIDER_TOKEN",
		Value:     "value-for-test",
		ExpiresAt: time.Date(2026, 5, 28, 8, 1, 1, 0, time.UTC),
	}}
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider := newTestProvider(t, engine)

	if _, err := provider.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := engine.createOptions.Config.Env
	want := []string{"OPENCLARION_PROVIDER_TOKEN=value-for-test"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Env = %v, want %v", got, want)
	}
	engine.assertCalls(t, "create", "start", "wait", "copy", "remove")
}

func TestProviderRunRejectsInvalidCredentialLifetimeBeforeCreate(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Time
		wantErr string
	}{
		{
			name:    "expired",
			expires: time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC),
			wantErr: "expired",
		},
		{
			name:    "exceeds container timeout",
			expires: time.Date(2026, 5, 28, 8, 1, 2, 0, time.UTC),
			wantErr: "exceeds container timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest()
			req.Credentials = []ports.ContainerCredential{{
				Name:      "OPENCLARION_PROVIDER_TOKEN",
				Value:     "value-for-test",
				ExpiresAt: tt.expires,
			}}
			engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
			provider := newTestProvider(t, engine)

			_, err := provider.Run(context.Background(), req)
			if err == nil {
				t.Fatal("Run err = nil, want credential lifetime error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Run err = %v, want containing %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "value-for-test") {
				t.Fatalf("Run error leaked credential value: %v", err)
			}
			engine.assertCalls(t)
		})
	}
}

func TestProviderRunStopsKillsAndRemovesOnTimeout(t *testing.T) {
	req := validRequest()
	req.Timeout = 20 * time.Millisecond
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	engine.blockWait = true
	engine.stopErr = errors.New("stop failed")
	provider := newTestProvider(t, engine)

	_, err := provider.Run(context.Background(), req)
	if err == nil {
		t.Fatal("Run err = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("Run err = %v, want deadline exceeded", err)
	}
	engine.assertCalls(t, "create", "start", "wait", "stop", "kill", "remove")
}

func TestProviderRunRemovesContainerWhenOutputCopyFails(t *testing.T) {
	engine := newFakeEngine(nil)
	engine.copyErr = errors.New("copy failed")
	provider := newTestProvider(t, engine)

	_, err := provider.Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Run err = nil, want copy error")
	}
	if !strings.Contains(err.Error(), "copy failed") {
		t.Fatalf("Run err = %v, want copy failed", err)
	}
	engine.assertCalls(t, "create", "start", "wait", "copy", "remove")
}

func TestProviderRunAddsSanitizedFailureLogsOnNonZeroExit(t *testing.T) {
	engine := newFakeEngine(nil)
	engine.exitCode = 1
	engine.logStream = dockerLogStream(t,
		dockerLogFrame{
			stream: dockerstdcopy.Stdout,
			body:   "starting diagnosis runner with OPENCLARION_DIAGNOSIS_LLM_API_KEY=plain-secret\n",
		},
		dockerLogFrame{
			stream: dockerstdcopy.Stderr,
			body: "provider failed: Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456\n" +
				"webhook_url=https://example.invalid/hook?key=secret-token api_key=another-secret\n" +
				"module failed before output.json\n",
		},
	)
	provider := newTestProvider(t, engine)

	_, err := provider.Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Run err = nil, want exit error")
	}
	var exitErr *ports.ContainerExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode != 1 || exitErr.RuntimeID != "container-1" {
		t.Fatalf("Run err = %T/%v, want container exit error for container-1 code 1", err, err)
	}
	errText := err.Error()
	for _, want := range []string{
		"exited with code 1",
		"stdout_bytes=",
		"stderr_bytes=",
		"stdout_sha256=",
		"stderr_sha256=",
		"module failed before output.json",
		"<redacted>",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("Run err = %v, want containing %q", err, want)
		}
	}
	for _, leaked := range []string{
		"plain-secret",
		"abcdefghijklmnopqrstuvwxyz123456",
		"https://example.invalid",
		"secret-token",
		"another-secret",
	} {
		if strings.Contains(errText, leaked) {
			t.Fatalf("Run error leaked %q: %v", leaked, err)
		}
	}
	engine.assertCalls(t, "create", "start", "wait", "logs", "remove")
}

func TestProviderRunDoesNotMaskExitCodeWhenFailureLogsAreUnavailable(t *testing.T) {
	engine := newFakeEngine(nil)
	engine.exitCode = 2
	engine.logErr = errors.New("daemon denied token=plain-secret")
	provider := newTestProvider(t, engine)

	_, err := provider.Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("Run err = nil, want exit error")
	}
	errText := err.Error()
	if !strings.Contains(errText, "exited with code 2") {
		t.Fatalf("Run err = %v, want exit code", err)
	}
	if !strings.Contains(errText, "logs_unavailable") {
		t.Fatalf("Run err = %v, want logs_unavailable", err)
	}
	if strings.Contains(errText, "plain-secret") {
		t.Fatalf("Run error leaked log failure secret: %v", err)
	}
	engine.assertCalls(t, "create", "start", "wait", "logs", "remove")
}

func TestProviderRunFailsClosedForAllowlistWithoutEgressEnforcer(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider := newTestProvider(t, engine)

	_, err := provider.Run(context.Background(), req)
	if err == nil {
		t.Fatal("Run err = nil, want missing egress enforcer error")
	}
	if !strings.Contains(err.Error(), "requires an egress enforcer") {
		t.Fatalf("Run err = %v, want egress enforcer", err)
	}
	engine.assertCalls(t)
}

func TestProviderRunUsesEgressEnforcerBeforeCreatingAllowlistContainer(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	enforcer := &recordingEgressEnforcer{}
	provider := newTestProvider(t, engine)
	provider.egressEnforcer = enforcer

	if _, err := provider.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if enforcer.calls != 1 {
		t.Fatalf("enforcer calls = %d, want 1", enforcer.calls)
	}
	if enforcer.networkMode != DefaultAllowlistNetworkMode {
		t.Fatalf("enforcer network mode = %q", enforcer.networkMode)
	}
	if strings.Join(enforcer.allowedEgress, ",") != "prometheus.internal:9090" {
		t.Fatalf("enforcer allowed egress = %v", enforcer.allowedEgress)
	}
	if engine.createOptions.HostConfig.NetworkMode != dockercontainer.NetworkMode(DefaultAllowlistNetworkMode) {
		t.Fatalf("NetworkMode = %q, want allowlist network", engine.createOptions.HostConfig.NetworkMode)
	}
	if engine.createOptions.NetworkingConfig == nil ||
		engine.createOptions.NetworkingConfig.EndpointsConfig == nil ||
		engine.createOptions.NetworkingConfig.EndpointsConfig[DefaultAllowlistNetworkMode] == nil {
		t.Fatalf("NetworkingConfig.EndpointsConfig = %#v, want explicit allowlist endpoint", engine.createOptions.NetworkingConfig)
	}
	engine.assertCalls(t, "create", "start", "wait", "copy", "remove")
}

func TestProviderRunUsesConfiguredAllowlistNetworkEndpoint(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	enforcer := &recordingEgressEnforcer{}
	provider := newTestProvider(t, engine)
	provider.cfg.AllowlistNetworkMode = "openclarion-sandbox-egress-prod"
	provider.egressEnforcer = enforcer

	if _, err := provider.Run(context.Background(), req); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if enforcer.networkMode != "openclarion-sandbox-egress-prod" {
		t.Fatalf("enforcer network mode = %q", enforcer.networkMode)
	}
	if engine.createOptions.HostConfig.NetworkMode != dockercontainer.NetworkMode("openclarion-sandbox-egress-prod") {
		t.Fatalf("NetworkMode = %q, want configured allowlist network", engine.createOptions.HostConfig.NetworkMode)
	}
	if engine.createOptions.NetworkingConfig == nil ||
		engine.createOptions.NetworkingConfig.EndpointsConfig["openclarion-sandbox-egress-prod"] == nil {
		t.Fatalf("NetworkingConfig.EndpointsConfig = %#v, want configured allowlist endpoint", engine.createOptions.NetworkingConfig)
	}
	engine.assertCalls(t, "create", "start", "wait", "copy", "remove")
}

func TestProviderRunDoesNotCreateWhenEgressEnforcerRejects(t *testing.T) {
	req := validRequest()
	req.Network = ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"prometheus.internal:9090"},
	}
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider := newTestProvider(t, engine)
	provider.egressEnforcer = &recordingEgressEnforcer{err: errors.New("proxy missing")}

	_, err := provider.Run(context.Background(), req)
	if err == nil {
		t.Fatal("Run err = nil, want enforcer error")
	}
	if !strings.Contains(err.Error(), "proxy missing") {
		t.Fatalf("Run err = %v, want proxy missing", err)
	}
	engine.assertCalls(t)
}

func TestProviderRunRejectsMissingAgentConfigDir(t *testing.T) {
	req := validRequest()
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider, err := NewProvider(engine, validConfig(), t.TempDir())
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	_, err = provider.Run(context.Background(), req)
	if err == nil {
		t.Fatal("Run err = nil, want missing agent config error")
	}
	if !strings.Contains(err.Error(), "agent config dir") {
		t.Fatalf("Run err = %v, want agent config dir", err)
	}
	engine.assertCalls(t)
}

func TestPrepareWorkspaceWritesInputsReadableBySandboxUser(t *testing.T) {
	req := validRequest()
	req.Conversation = json.RawMessage(`[]`)
	req.Message = json.RawMessage(`{"role":"user","content":"next"}`)
	engine := newFakeEngine(tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)))
	provider := newTestProvider(t, engine)

	workspace, cleanup, err := provider.prepareWorkspace(req)
	if err != nil {
		t.Fatalf("prepareWorkspace: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}()

	assertSandboxInputReadable(t, workspace.EvidencePath)
	assertSandboxInputReadable(t, workspace.ConversationPath)
	assertSandboxInputReadable(t, workspace.MessagePath)
	assertSandboxOutputWritable(t, workspace.OutputDir)
}

func TestReadOutputArchiveValidatesOutput(t *testing.T) {
	tests := []struct {
		name    string
		archive io.Reader
		max     int64
		wantErr string
	}{
		{
			name:    "valid",
			archive: tarArchive(t, "output.json", []byte(`{"summary":"ok"}`)),
			max:     32,
		},
		{
			name:    "missing",
			archive: tarArchiveEntries(t),
			max:     32,
			wantErr: "not found",
		},
		{
			name:    "unexpected regular member",
			archive: tarArchive(t, "other.json", []byte(`{"summary":"ok"}`)),
			max:     32,
			wantErr: "unexpected output archive member",
		},
		{
			name:    "too large",
			archive: tarArchive(t, "output.json", []byte(`{"summary":"too-large"}`)),
			max:     8,
			wantErr: "exceeds maximum",
		},
		{
			name:    "invalid json",
			archive: tarArchive(t, "output.json", []byte(`not-json`)),
			max:     32,
			wantErr: "valid JSON",
		},
		{
			name:    "nested output member",
			archive: tarArchive(t, "nested/output.json", []byte(`{"summary":"ok"}`)),
			max:     32,
			wantErr: "top-level file",
		},
		{
			name:    "traversal output member",
			archive: tarArchive(t, "../output.json", []byte(`{"summary":"ok"}`)),
			max:     32,
			wantErr: "not allowed",
		},
		{
			name:    "absolute output member",
			archive: tarArchive(t, "/workspace/out/output.json", []byte(`{"summary":"ok"}`)),
			max:     32,
			wantErr: "not allowed",
		},
		{
			name:    "backslash output member",
			archive: tarArchive(t, `nested\output.json`, []byte(`{"summary":"ok"}`)),
			max:     32,
			wantErr: "not allowed",
		},
		{
			name: "unsafe directory member before valid output",
			archive: tarArchiveEntries(t,
				tarEntry{header: tar.Header{Name: "../", Typeflag: tar.TypeDir, Mode: 0o700}},
				tarEntry{header: tar.Header{Name: "output.json", Mode: 0o600}, content: []byte(`{"summary":"ok"}`)},
			),
			max:     32,
			wantErr: "not allowed",
		},
		{
			name: "symlink output member",
			archive: tarArchiveWithHeader(t, tar.Header{
				Name:     "output.json",
				Typeflag: tar.TypeSymlink,
				Linkname: "other.json",
			}, nil),
			max:     32,
			wantErr: "regular file",
		},
		{
			name: "hardlink output member",
			archive: tarArchiveWithHeader(t, tar.Header{
				Name:     "output.json",
				Typeflag: tar.TypeLink,
				Linkname: "other.json",
			}, nil),
			max:     32,
			wantErr: "regular file",
		},
		{
			name: "symlink member before valid output",
			archive: tarArchiveEntries(t,
				tarEntry{header: tar.Header{Name: "other.json", Typeflag: tar.TypeSymlink, Linkname: "output.json"}},
				tarEntry{header: tar.Header{Name: "output.json", Mode: 0o600}, content: []byte(`{"summary":"ok"}`)},
			),
			max:     32,
			wantErr: "regular file",
		},
		{
			name: "hardlink member before valid output",
			archive: tarArchiveEntries(t,
				tarEntry{header: tar.Header{Name: "other.json", Typeflag: tar.TypeLink, Linkname: "output.json"}},
				tarEntry{header: tar.Header{Name: "output.json", Mode: 0o600}, content: []byte(`{"summary":"ok"}`)},
			),
			max:     32,
			wantErr: "regular file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readOutputArchive(tt.archive, ports.SandboxOutputPath, tt.max)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("readOutputArchive: %v", err)
				}
				if string(got) != `{"summary":"ok"}` {
					t.Fatalf("output = %s", got)
				}
				return
			}
			if err == nil {
				t.Fatal("readOutputArchive err = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("readOutputArchive err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadOutputArchiveRejectsTarInsecurePathError(t *testing.T) {
	t.Setenv("GODEBUG", "tarinsecurepath=0")

	_, err := readOutputArchive(
		tarArchive(t, "../output.json", []byte(`{"summary":"ok"}`)),
		ports.SandboxOutputPath,
		32,
	)
	if err == nil {
		t.Fatal("readOutputArchive err = nil, want insecure path rejection")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("readOutputArchive err = %v, want project path rejection", err)
	}
}

type fakeEngine struct {
	t             *testing.T
	outputArchive io.ReadCloser
	blockWait     bool
	exitCode      int64
	stopErr       error
	copyErr       error
	logErr        error
	logStream     io.ReadCloser
	createOptions dockerclient.ContainerCreateOptions
	waitOptions   dockerclient.ContainerWaitOptions
	evidence      string
	calls         []string
}

type recordingEgressEnforcer struct {
	calls         int
	allowedEgress []string
	networkMode   string
	err           error
}

func (e *recordingEgressEnforcer) Validate(_ context.Context, policy ports.ContainerNetworkPolicy, networkMode string) error {
	e.calls++
	e.allowedEgress = append([]string(nil), policy.AllowedEgress...)
	e.networkMode = networkMode
	return e.err
}

func newFakeEngine(output io.ReadCloser) *fakeEngine {
	return &fakeEngine{outputArchive: output}
}

func (f *fakeEngine) ContainerCreate(_ context.Context, options dockerclient.ContainerCreateOptions) (dockerclient.ContainerCreateResult, error) {
	f.calls = append(f.calls, "create")
	f.createOptions = options
	for _, mount := range options.HostConfig.Mounts {
		if mount.Target == ports.SandboxEvidencePath {
			data, err := os.ReadFile(mount.Source)
			if err != nil {
				f.t.Fatalf("read mounted evidence: %v", err)
			}
			f.evidence = string(data)
		}
	}
	return dockerclient.ContainerCreateResult{ID: "container-1"}, nil
}

func (f *fakeEngine) ContainerStart(context.Context, string, dockerclient.ContainerStartOptions) (dockerclient.ContainerStartResult, error) {
	f.calls = append(f.calls, "start")
	return dockerclient.ContainerStartResult{}, nil
}

func (f *fakeEngine) ContainerWait(_ context.Context, _ string, options dockerclient.ContainerWaitOptions) dockerclient.ContainerWaitResult {
	f.calls = append(f.calls, "wait")
	f.waitOptions = options
	result := make(chan dockercontainer.WaitResponse, 1)
	errs := make(chan error, 1)
	if !f.blockWait {
		result <- dockercontainer.WaitResponse{StatusCode: f.exitCode}
		close(result)
		close(errs)
	}
	return dockerclient.ContainerWaitResult{Result: result, Error: errs}
}

func (f *fakeEngine) ContainerStop(context.Context, string, dockerclient.ContainerStopOptions) (dockerclient.ContainerStopResult, error) {
	f.calls = append(f.calls, "stop")
	return dockerclient.ContainerStopResult{}, f.stopErr
}

func (f *fakeEngine) ContainerKill(context.Context, string, dockerclient.ContainerKillOptions) (dockerclient.ContainerKillResult, error) {
	f.calls = append(f.calls, "kill")
	return dockerclient.ContainerKillResult{}, nil
}

func (f *fakeEngine) ContainerRemove(context.Context, string, dockerclient.ContainerRemoveOptions) (dockerclient.ContainerRemoveResult, error) {
	f.calls = append(f.calls, "remove")
	return dockerclient.ContainerRemoveResult{}, nil
}

func (f *fakeEngine) CopyFromContainer(context.Context, string, dockerclient.CopyFromContainerOptions) (dockerclient.CopyFromContainerResult, error) {
	f.calls = append(f.calls, "copy")
	return dockerclient.CopyFromContainerResult{Content: f.outputArchive}, f.copyErr
}

func (f *fakeEngine) ContainerLogs(context.Context, string, dockerclient.ContainerLogsOptions) (dockerclient.ContainerLogsResult, error) {
	f.calls = append(f.calls, "logs")
	if f.logErr != nil {
		return nil, f.logErr
	}
	if f.logStream == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return f.logStream, nil
}

func (f *fakeEngine) assertCalls(t *testing.T, want ...string) {
	t.Helper()
	if strings.Join(f.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
}

func newTestProvider(t *testing.T, engine *fakeEngine) *Provider {
	t.Helper()
	engine.t = t
	root := t.TempDir()
	agentConfigDir := filepath.Join(root, validRequest().AgentName)
	if err := os.Mkdir(agentConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir agent config: %v", err)
	}
	provider, err := NewProvider(engine, validConfig(), root, WithWorkspaceRoot(t.TempDir()))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	now := time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC)
	provider.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	return provider
}

func assertEngineMounted(t *testing.T, create dockerclient.ContainerCreateOptions, target string) {
	t.Helper()
	for _, mount := range create.HostConfig.Mounts {
		if mount.Target == target {
			if !mount.ReadOnly {
				t.Fatalf("mount %s is not readonly", target)
			}
			return
		}
	}
	t.Fatalf("mount target %s not found in %#v", target, create.HostConfig.Mounts)
}

func assertEngineMountedWritable(t *testing.T, create dockerclient.ContainerCreateOptions, target string) {
	t.Helper()
	for _, mount := range create.HostConfig.Mounts {
		if mount.Target == target {
			if mount.ReadOnly {
				t.Fatalf("mount %s is readonly", target)
			}
			return
		}
	}
	t.Fatalf("mount target %s not found in %#v", target, create.HostConfig.Mounts)
}

func assertSandboxInputReadable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sandbox input %s: %v", path, err)
	}
	if info.Mode().Perm() != sandboxInputFileMode {
		t.Fatalf("sandbox input %s mode = %v, want %v", path, info.Mode().Perm(), sandboxInputFileMode)
	}
}

func assertSandboxOutputWritable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sandbox output dir %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("sandbox output %s is not a directory", path)
	}
	if info.Mode().Perm() != sandboxOutputDirMode {
		t.Fatalf("sandbox output %s mode = %v, want %v", path, info.Mode().Perm(), sandboxOutputDirMode)
	}
}

func tarArchive(t *testing.T, name string, content []byte) io.ReadCloser {
	t.Helper()
	return tarArchiveWithHeader(t, tar.Header{Name: name, Mode: 0o600}, content)
}

func tarArchiveWithHeader(t *testing.T, header tar.Header, content []byte) io.ReadCloser {
	t.Helper()
	return tarArchiveEntries(t, tarEntry{header: header, content: content})
}

type tarEntry struct {
	header  tar.Header
	content []byte
}

func tarArchiveEntries(t *testing.T, entries ...tarEntry) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		entry.header.Size = int64(len(entry.content))
		if err := tw.WriteHeader(&entry.header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(entry.content) > 0 {
			if _, err := tw.Write(entry.content); err != nil {
				t.Fatalf("write tar content: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

type dockerLogFrame struct {
	stream dockerstdcopy.StdType
	body   string
}

func dockerLogStream(t *testing.T, frames ...dockerLogFrame) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	for _, frame := range frames {
		header := make([]byte, dockerLogFrameHeaderBytes)
		header[0] = byte(frame.stream)
		bodySize := uint64(len(frame.body))
		if bodySize > uint64(^uint32(0)) {
			t.Fatalf("docker log body length %d exceeds uint32 frame limit", bodySize)
		}
		// #nosec G115 -- bodySize is checked against the uint32 Docker frame limit above.
		binary.BigEndian.PutUint32(header[4:8], uint32(bodySize))
		if _, err := buf.Write(header); err != nil {
			t.Fatalf("write docker log header: %v", err)
		}
		if _, err := buf.WriteString(frame.body); err != nil {
			t.Fatalf("write docker log body: %v", err)
		}
	}
	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}
