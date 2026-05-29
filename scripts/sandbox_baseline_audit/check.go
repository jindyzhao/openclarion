// Command sandbox_baseline_audit emits a machine-readable proof that the
// code-level M4/M5 sandbox baseline invariants are still satisfied. It does
// not start Docker; live daemon behavior remains covered by manual smoke
// targets.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	dockerprovider "github.com/openclarion/openclarion/internal/providers/container/docker"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type auditOutput struct {
	Tool   string       `json:"tool"`
	Status string       `json:"status"`
	Checks []auditCheck `json:"checks"`
}

type auditCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type auditProbe struct {
	name string
	run  func() error
}

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-baseline-audit] %v\n", err)
		os.Exit(1)
	}
}

func run(stdout io.Writer) error {
	checks := []auditProbe{
		{name: "fixed_file_contract", run: checkFixedFileContract},
		{name: "batch_network_none_spec", run: checkBatchNetworkNoneSpec},
		{name: "m5_turn_input_mounts", run: checkM5TurnInputMounts},
		{name: "docker_security_posture", run: checkDockerSecurityPosture},
		{name: "allowlist_enforcer_subset", run: checkAllowlistEnforcerSubset},
		{name: "allowlist_enforcer_drift_rejection", run: checkAllowlistEnforcerDriftRejection},
		{name: "raw_result_validation", run: checkRawResultValidation},
	}
	out := auditOutput{
		Tool:   "sandbox_baseline_audit",
		Status: "pass",
		Checks: make([]auditCheck, 0, len(checks)),
	}
	for _, check := range checks {
		if err := check.run(); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
		out.Checks = append(out.Checks, auditCheck{Name: check.name, Status: "pass"})
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func checkFixedFileContract() error {
	want := map[string]string{
		"evidence":     "/workspace/evidence.json",
		"conversation": "/workspace/conversation.json",
		"message":      "/workspace/message.json",
		"agent_config": "/workspace/agent_config",
		"output_dir":   "/workspace/out",
		"output_path":  "/workspace/out/output.json",
	}
	got := map[string]string{
		"evidence":     ports.SandboxEvidencePath,
		"conversation": ports.SandboxConversationPath,
		"message":      ports.SandboxMessagePath,
		"agent_config": ports.SandboxAgentConfigPath,
		"output_dir":   ports.SandboxOutputDir,
		"output_path":  ports.SandboxOutputPath,
	}
	for name, wantValue := range want {
		if got[name] != wantValue {
			return fmt.Errorf("%s path = %q, want %q", name, got[name], wantValue)
		}
	}
	if ports.DefaultContainerRunTimeout != ports.MaxContainerRunTimeout {
		return fmt.Errorf("default timeout %s must equal max timeout %s", ports.DefaultContainerRunTimeout, ports.MaxContainerRunTimeout)
	}
	if ports.DefaultContainerOutputBytes != ports.MaxContainerOutputBytes {
		return fmt.Errorf("default output cap %d must equal max output cap %d", ports.DefaultContainerOutputBytes, ports.MaxContainerOutputBytes)
	}
	return nil
}

func checkBatchNetworkNoneSpec() error {
	req := baselineRequest()
	spec, err := dockerprovider.BuildRunSpec(baselineConfig(), req, baselineWorkspace())
	if err != nil {
		return err
	}
	if spec.NetworkMode != "none" {
		return fmt.Errorf("network mode = %q, want none", spec.NetworkMode)
	}
	if err := requireReadonlyMount(spec, ports.SandboxEvidencePath); err != nil {
		return err
	}
	if err := requireReadonlyMount(spec, ports.SandboxAgentConfigPath); err != nil {
		return err
	}
	if spec.OutputMount.Target != ports.SandboxOutputDir || spec.OutputMount.ReadOnly {
		return fmt.Errorf("output mount = %#v, want writable %s", spec.OutputMount, ports.SandboxOutputDir)
	}
	if spec.OutputPath != ports.SandboxOutputPath {
		return fmt.Errorf("output path = %q, want %q", spec.OutputPath, ports.SandboxOutputPath)
	}
	if spec.Timeout != req.EffectiveTimeout() {
		return fmt.Errorf("timeout = %s, want %s", spec.Timeout, req.EffectiveTimeout())
	}
	if spec.OutputMaxBytes != req.EffectiveOutputMax() {
		return fmt.Errorf("output cap = %d, want %d", spec.OutputMaxBytes, req.EffectiveOutputMax())
	}
	return nil
}

func checkM5TurnInputMounts() error {
	req := baselineRequest()
	req.Conversation = json.RawMessage(`[{"role":"assistant","content":"previous"}]`)
	req.Message = json.RawMessage(`{"role":"user","content":"next"}`)
	workspace := baselineWorkspace()
	workspace.ConversationPath = "/tmp/openclarion-sandbox/conversation.json"
	workspace.MessagePath = "/tmp/openclarion-sandbox/message.json"
	spec, err := dockerprovider.BuildRunSpec(baselineConfig(), req, workspace)
	if err != nil {
		return err
	}
	if err := requireReadonlyMount(spec, ports.SandboxConversationPath); err != nil {
		return err
	}
	return requireReadonlyMount(spec, ports.SandboxMessagePath)
}

func checkDockerSecurityPosture() error {
	spec, err := dockerprovider.BuildRunSpec(baselineConfig(), baselineRequest(), baselineWorkspace())
	if err != nil {
		return err
	}
	if spec.User == "0" || spec.User == "0:0" || spec.User == "root" {
		return fmt.Errorf("user = %q, want non-root", spec.User)
	}
	if !spec.ReadonlyRootFS {
		return fmt.Errorf("readonly rootfs is disabled")
	}
	if spec.Privileged {
		return fmt.Errorf("privileged mode is enabled")
	}
	if !contains(spec.SecurityOpt, dockerprovider.NoNewPrivilegesSecurityOpt) {
		return fmt.Errorf("security opts = %v, want %s", spec.SecurityOpt, dockerprovider.NoNewPrivilegesSecurityOpt)
	}
	if !contains(spec.CapDrop, dockerprovider.DropAllCapabilities) {
		return fmt.Errorf("cap drop = %v, want %s", spec.CapDrop, dockerprovider.DropAllCapabilities)
	}
	if spec.MemoryBytes != dockerprovider.DefaultMemoryBytes {
		return fmt.Errorf("memory = %d, want %d", spec.MemoryBytes, dockerprovider.DefaultMemoryBytes)
	}
	if spec.NanoCPUs != dockerprovider.DefaultNanoCPUs {
		return fmt.Errorf("cpu = %d, want %d", spec.NanoCPUs, dockerprovider.DefaultNanoCPUs)
	}
	if spec.PidsLimit != dockerprovider.DefaultPidsLimit {
		return fmt.Errorf("pids = %d, want %d", spec.PidsLimit, dockerprovider.DefaultPidsLimit)
	}
	return nil
}

func checkAllowlistEnforcerSubset() error {
	policy := ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"Prometheus.Internal:9090"},
	}
	req := baselineRequest()
	req.Network = policy
	spec, err := dockerprovider.BuildRunSpec(baselineConfig(), req, baselineWorkspace())
	if err != nil {
		return err
	}
	if spec.NetworkMode != dockerprovider.DefaultAllowlistNetworkMode {
		return fmt.Errorf("allowlist network = %q, want %q", spec.NetworkMode, dockerprovider.DefaultAllowlistNetworkMode)
	}
	enforcer, err := dockerprovider.NewStaticAllowlistEnforcer(dockerprovider.DefaultAllowlistNetworkMode, []string{
		"prometheus.internal:9090",
		"api.openai.com:443",
	})
	if err != nil {
		return err
	}
	return enforcer.Validate(context.Background(), policy, spec.NetworkMode)
}

func checkAllowlistEnforcerDriftRejection() error {
	enforcer, err := dockerprovider.NewStaticAllowlistEnforcer(dockerprovider.DefaultAllowlistNetworkMode, []string{"prometheus.internal:9090"})
	if err != nil {
		return err
	}
	err = enforcer.Validate(context.Background(), ports.ContainerNetworkPolicy{
		Mode:          ports.ContainerNetworkAllowlist,
		AllowedEgress: []string{"api.openai.com:443"},
	}, dockerprovider.DefaultAllowlistNetworkMode)
	if err == nil {
		return fmt.Errorf("unexpectedly accepted unconfigured egress target")
	}
	return nil
}

func checkRawResultValidation() error {
	req := baselineRequest()
	started := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	result := ports.ContainerRunResult{
		InvocationID: req.InvocationID,
		AgentName:    req.AgentName,
		Output:       json.RawMessage(`{"summary":"ok"}`),
		ExitCode:     0,
		StartedAt:    started,
		FinishedAt:   started.Add(time.Second),
		RuntimeID:    "container-1",
	}
	if err := ports.ValidateContainerRunResult(req, result); err != nil {
		return err
	}
	badReq := baselineRequest()
	badReq.Evidence = json.RawMessage(`{"snapshot_id":11,"snapshot_id":12}`)
	if err := badReq.Validate(); err == nil {
		return fmt.Errorf("unexpectedly accepted duplicate-key evidence JSON")
	}
	badReq = baselineRequest()
	badReq.Conversation = json.RawMessage(`[{"role":"assistant","role":"user"}]`)
	if err := badReq.Validate(); err == nil {
		return fmt.Errorf("unexpectedly accepted duplicate-key conversation JSON")
	}
	badReq = baselineRequest()
	badReq.Message = json.RawMessage(`{"role":"user"} {"role":"assistant"}`)
	if err := badReq.Validate(); err == nil {
		return fmt.Errorf("unexpectedly accepted trailing message JSON")
	}
	if err := requireResultOutputRejected(req, result, json.RawMessage(`not-json`), "invalid JSON output"); err != nil {
		return err
	}
	if err := requireResultOutputRejected(req, result, json.RawMessage(`{"summary":"old","summary":"new"}`), "duplicate-key output JSON"); err != nil {
		return err
	}
	if err := requireResultOutputRejected(req, result, json.RawMessage(`{"summary":"ok"} {"summary":"again"}`), "trailing output JSON"); err != nil {
		return err
	}
	return nil
}

func requireResultOutputRejected(req ports.ContainerRunRequest, result ports.ContainerRunResult, output json.RawMessage, description string) error {
	result.Output = output
	if err := ports.ValidateContainerRunResult(req, result); err == nil {
		return fmt.Errorf("unexpectedly accepted %s", description)
	}
	return nil
}

func baselineConfig() dockerprovider.Config {
	return dockerprovider.Config{
		ImageRef:        "registry.example.com/openclarion/agent@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		User:            dockerprovider.DefaultUser,
		ReadonlyRootFS:  true,
		NoNewPrivileges: true,
	}
}

func baselineRequest() ports.ContainerRunRequest {
	return ports.ContainerRunRequest{
		InvocationID: "snapshot-11/group-0",
		AgentName:    "report-enhancer",
		Evidence:     json.RawMessage(`{"snapshot_id":11,"alerts":[]}`),
		Timeout:      time.Minute,
		OutputMax:    1024,
	}
}

func baselineWorkspace() dockerprovider.WorkspacePaths {
	return dockerprovider.WorkspacePaths{
		EvidencePath:   "/tmp/openclarion-sandbox/evidence.json",
		AgentConfigDir: "/tmp/openclarion-sandbox/agent_config",
		OutputDir:      "/tmp/openclarion-sandbox/out",
	}
}

func requireReadonlyMount(spec dockerprovider.RunSpec, target string) error {
	for _, mount := range spec.BindMounts {
		if mount.Target == target {
			if !mount.ReadOnly {
				return fmt.Errorf("mount %s is not readonly", target)
			}
			return nil
		}
	}
	return fmt.Errorf("mount %s not found", target)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
