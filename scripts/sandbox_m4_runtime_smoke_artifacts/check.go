// Command sandbox_m4_runtime_smoke_artifacts validates the retained runtime
// smoke artifact bundle used by the M4 review-evidence and packet helpers.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const maxArtifactBytes int64 = 1024 * 1024

var expectedArtifacts = map[string]artifactExpectation{
	"agent-runtime-smoke.json": {
		Tool:   "agent-runtime-smoke",
		Source: "make agent-runtime-smoke",
		Kind:   artifactAgentRuntime,
	},
	"container-provider-smoke.json": {
		Tool:   "container-provider-smoke",
		Source: "make container-provider-smoke",
		Kind:   artifactContainerProviderSuccess,
	},
	"container-provider-timeout-smoke.json": {
		Tool:   "container-provider-smoke",
		Source: "make container-provider-timeout-smoke",
		Kind:   artifactContainerProviderExpectedError,
	},
	"container-provider-output-cap-smoke.json": {
		Tool:   "container-provider-smoke",
		Source: "make container-provider-output-cap-smoke",
		Kind:   artifactContainerProviderExpectedError,
	},
	"egress-allowdeny-smoke.json": {
		Tool:   "egress-allowdeny-smoke",
		Source: "make egress-allowdeny-smoke",
		Kind:   artifactEgressAllowDeny,
	},
}

var (
	agentRuntimeChecks = []string{
		"regular_output_file",
		"bounded_output_size",
		"valid_json_object",
		"duplicate_key_free",
		"non_empty_object",
	}
	containerProviderCommonChecks = []string{
		"digest_pinned_image",
		"request_validated",
		"network_none",
		"readonly_rootfs",
		"no_new_privileges",
		"cap_drop_all",
	}
	containerProviderSuccessChecks = []string{
		"provider_run_succeeded",
		"valid_json_object_output",
		"duplicate_key_free_output",
	}
	containerProviderExpectedErrorChecks = []string{
		"expected_provider_error_observed",
	}
	egressAllowDenyChecks = []string{
		"digest_pinned_image",
		"sandbox_network_internal",
		"upstream_network_separate",
		"proxy_dual_network",
		"allowed_target_via_proxy",
		"denied_target_blocked_by_proxy",
		"direct_bypass_failed",
		"non_root_readonly_no_new_privileges_cap_drop",
	}
)

type artifactKind string

const (
	artifactAgentRuntime                   artifactKind = "agent-runtime"
	artifactContainerProviderSuccess       artifactKind = "container-provider-success"
	artifactContainerProviderExpectedError artifactKind = "container-provider-expected-error"
	artifactEgressAllowDeny                artifactKind = "egress-allowdeny"
)

type artifactExpectation struct {
	Tool   string
	Source string
	Kind   artifactKind
}

type proofCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type agentRuntimeArtifact struct {
	Tool             string             `json:"tool"`
	Status           string             `json:"status"`
	Source           string             `json:"source"`
	RuntimeCandidate string             `json:"runtime_candidate"`
	Output           agentRuntimeOutput `json:"output"`
	Checks           []proofCheck       `json:"checks"`
}

type agentRuntimeOutput struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	MaxBytes int64  `json:"max_bytes"`
	SHA256   string `json:"sha256"`
}

type containerProviderArtifact struct {
	Tool         string                   `json:"tool"`
	Status       string                   `json:"status"`
	Source       string                   `json:"source"`
	ImageRef     string                   `json:"image_ref"`
	InvocationID string                   `json:"invocation_id,omitempty"`
	Mode         string                   `json:"mode"`
	TimeoutSec   int64                    `json:"timeout_seconds"`
	Output       *containerProviderOutput `json:"output,omitempty"`
	Expected     *containerProviderError  `json:"expected_error,omitempty"`
	Checks       []proofCheck             `json:"checks"`
}

type containerProviderOutput struct {
	Bytes    int    `json:"bytes"`
	MaxBytes int64  `json:"max_bytes"`
	SHA256   string `json:"sha256"`
}

type containerProviderError struct {
	Pattern string `json:"pattern"`
	Matched string `json:"matched"`
}

type egressAllowDenyArtifact struct {
	Tool       string              `json:"tool"`
	Status     string              `json:"status"`
	Source     string              `json:"source"`
	ImageRef   string              `json:"image_ref"`
	RunID      string              `json:"run_id,omitempty"`
	TimeoutSec int64               `json:"timeout_seconds"`
	Topology   egressProofTopology `json:"topology"`
	Checks     []proofCheck        `json:"checks"`
}

type egressProofTopology struct {
	SandboxNetwork     string `json:"sandbox_network"`
	UpstreamNetwork    string `json:"upstream_network"`
	ProxyTarget        string `json:"proxy_target"`
	AllowedTarget      string `json:"allowed_target"`
	DeniedTarget       string `json:"denied_target"`
	DirectBypassTarget string `json:"direct_bypass_target"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-m4-runtime-smoke-artifacts] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	root, err := parseArgs(args)
	if err != nil {
		return err
	}
	if err := validateBundle(root); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "[sandbox-m4-runtime-smoke-artifacts] OK (%d artifacts)\n", len(expectedArtifacts))
	return nil
}

func parseArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("sandbox_m4_runtime_smoke_artifacts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", "", "runtime-smoke artifact bundle directory")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 || strings.TrimSpace(*root) == "" {
		return "", errors.New("usage: sandbox_m4_runtime_smoke_artifacts --root <artifact-dir>")
	}
	return filepath.Clean(*root), nil
}

func validateBundle(root string) error {
	if err := requireDirectDirectory(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read artifact directory: %w", err)
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := expectedArtifacts[name]; !ok {
			return fmt.Errorf("unexpected artifact %q", name)
		}
		seen[name] = struct{}{}
	}
	for name, expectation := range expectedArtifacts {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("missing required artifact %q", name)
		}
		path := filepath.Join(root, name)
		raw, err := readRegularFileCapped(path, maxArtifactBytes)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if len(raw) == 0 {
			return fmt.Errorf("%s: artifact is empty", name)
		}
		if err := validateArtifact(raw, expectation); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func validateArtifact(raw []byte, expectation artifactExpectation) error {
	switch expectation.Kind {
	case artifactAgentRuntime:
		var artifact agentRuntimeArtifact
		if err := strictjson.Unmarshal(raw, &artifact); err != nil {
			return err
		}
		if err := validateCommon(artifact.Tool, artifact.Status, artifact.Source, expectation); err != nil {
			return err
		}
		if !immutableImageReference(artifact.RuntimeCandidate) {
			return errors.New("runtime_candidate must be name@sha256:<64-lowercase-hex-digest>")
		}
		if artifact.Output.Path != "/workspace/out/output.json" {
			return fmt.Errorf("output.path = %q, want /workspace/out/output.json", artifact.Output.Path)
		}
		if artifact.Output.Bytes <= 0 || artifact.Output.MaxBytes <= 0 || artifact.Output.Bytes > artifact.Output.MaxBytes {
			return errors.New("output byte counts must be positive and within max_bytes")
		}
		if !lowerHex(artifact.Output.SHA256, 64) {
			return errors.New("output.sha256 must be 64 lowercase hex characters")
		}
		return requirePassedChecks(artifact.Checks, agentRuntimeChecks...)
	case artifactContainerProviderSuccess, artifactContainerProviderExpectedError:
		var artifact containerProviderArtifact
		if err := strictjson.Unmarshal(raw, &artifact); err != nil {
			return err
		}
		if err := validateCommon(artifact.Tool, artifact.Status, artifact.Source, expectation); err != nil {
			return err
		}
		if !immutableImageReference(artifact.ImageRef) {
			return errors.New("image_ref must be name@sha256:<64-lowercase-hex-digest>")
		}
		if artifact.TimeoutSec <= 0 {
			return errors.New("timeout_seconds must be positive")
		}
		if err := requirePassedChecks(artifact.Checks, containerProviderCommonChecks...); err != nil {
			return err
		}
		switch expectation.Kind {
		case artifactContainerProviderSuccess:
			if artifact.Mode != "success" {
				return fmt.Errorf("mode = %q, want success", artifact.Mode)
			}
			if artifact.Output == nil {
				return errors.New("output is required for success mode")
			}
			if artifact.Expected != nil {
				return errors.New("expected_error must be absent for success mode")
			}
			if artifact.Output.Bytes <= 0 || artifact.Output.MaxBytes <= 0 || int64(artifact.Output.Bytes) > artifact.Output.MaxBytes {
				return errors.New("output byte counts must be positive and within max_bytes")
			}
			if !lowerHex(artifact.Output.SHA256, 64) {
				return errors.New("output.sha256 must be 64 lowercase hex characters")
			}
			return requirePassedChecks(artifact.Checks, containerProviderSuccessChecks...)
		case artifactContainerProviderExpectedError:
			if artifact.Mode != "expected_error" {
				return fmt.Errorf("mode = %q, want expected_error", artifact.Mode)
			}
			if artifact.Output != nil {
				return errors.New("output must be absent for expected_error mode")
			}
			if artifact.Expected == nil || strings.TrimSpace(artifact.Expected.Pattern) == "" || strings.TrimSpace(artifact.Expected.Matched) == "" {
				return errors.New("expected_error pattern and matched values are required")
			}
			return requirePassedChecks(artifact.Checks, containerProviderExpectedErrorChecks...)
		}
	case artifactEgressAllowDeny:
		var artifact egressAllowDenyArtifact
		if err := strictjson.Unmarshal(raw, &artifact); err != nil {
			return err
		}
		if err := validateCommon(artifact.Tool, artifact.Status, artifact.Source, expectation); err != nil {
			return err
		}
		if !immutableImageReference(artifact.ImageRef) {
			return errors.New("image_ref must be name@sha256:<64-lowercase-hex-digest>")
		}
		if artifact.TimeoutSec <= 0 {
			return errors.New("timeout_seconds must be positive")
		}
		if artifact.Topology.SandboxNetwork != "internal" {
			return fmt.Errorf("topology.sandbox_network = %q, want internal", artifact.Topology.SandboxNetwork)
		}
		if artifact.Topology.UpstreamNetwork != "separate" {
			return fmt.Errorf("topology.upstream_network = %q, want separate", artifact.Topology.UpstreamNetwork)
		}
		for field, value := range map[string]string{
			"topology.proxy_target":         artifact.Topology.ProxyTarget,
			"topology.allowed_target":       artifact.Topology.AllowedTarget,
			"topology.denied_target":        artifact.Topology.DeniedTarget,
			"topology.direct_bypass_target": artifact.Topology.DirectBypassTarget,
		} {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " \r\n\t") {
				return fmt.Errorf("%s must be non-empty and whitespace-free", field)
			}
		}
		if artifact.Topology.DirectBypassTarget != artifact.Topology.AllowedTarget {
			return errors.New("topology.direct_bypass_target must match allowed_target")
		}
		return requirePassedChecks(artifact.Checks, egressAllowDenyChecks...)
	default:
		return fmt.Errorf("unsupported artifact kind %q", expectation.Kind)
	}
	return nil
}

func validateCommon(tool, status, source string, expectation artifactExpectation) error {
	if tool != expectation.Tool {
		return fmt.Errorf("tool = %q, want %q", tool, expectation.Tool)
	}
	if status != "pass" {
		return fmt.Errorf("status = %q, want pass", status)
	}
	if source != expectation.Source {
		return fmt.Errorf("source = %q, want %q", source, expectation.Source)
	}
	return nil
}

func requirePassedChecks(checks []proofCheck, required ...string) error {
	if len(checks) == 0 {
		return errors.New("checks must not be empty")
	}
	seen := make(map[string]string, len(checks))
	for _, check := range checks {
		name := strings.TrimSpace(check.Name)
		if name == "" || name != check.Name {
			return errors.New("check names must be non-empty and unpadded")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate check %q", name)
		}
		seen[name] = check.Status
	}
	for _, name := range required {
		status, ok := seen[name]
		if !ok {
			return fmt.Errorf("missing required check %q", name)
		}
		if status != "pass" {
			return fmt.Errorf("check %q status = %q, want pass", name, status)
		}
	}
	return nil
}

func requireDirectDirectory(root string) error {
	// #nosec G703 -- this offline evidence checker inspects an
	// operator-supplied retained artifact directory before reading artifacts.
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("artifact directory must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New("artifact path must be a directory")
	}
	return nil
}

func readRegularFileCapped(filePath string, maxBytes int64) ([]byte, error) {
	// #nosec G703 -- this offline evidence checker stats an operator-supplied
	// artifact path before enforcing direct regular-file reads.
	info, err := os.Lstat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("artifact must be a regular file, not a symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("artifact must be a regular file")
	}
	// #nosec G304,G703 -- this offline evidence checker opens an
	// operator-supplied retained artifact path after direct regular-file checks.
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("artifact exceeds maximum %d bytes", maxBytes)
	}
	return raw, nil
}

func immutableImageReference(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \r\n\t") {
		return false
	}
	name, digest, ok := strings.Cut(value, "@sha256:")
	if !ok || name == "" {
		return false
	}
	return lowerHex(digest, 64)
}

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
