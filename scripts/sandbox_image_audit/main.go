// Command sandbox_image_audit verifies that a configured diagnosis sandbox
// image looks like the live diagnosis assistant runtime rather than a generic
// file-contract runner.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	toolName                  = "sandbox_image_audit"
	defaultDockerBin          = "docker"
	defaultExpectedEntrypoint = "/diagnosis-assistant-runner"
	maxDockerInspectBytes     = 1 << 20
)

var digestRefPattern = regexp.MustCompile(`^[^\s@]+@sha256:([a-f0-9]{64})$`)

type config struct {
	imageRef           string
	proofOut           string
	expectedEntrypoint string
	dockerBin          string
}

type dockerImageConfig struct {
	Entrypoint []string          `json:"Entrypoint"`
	Cmd        []string          `json:"Cmd"`
	User       string            `json:"User"`
	WorkingDir string            `json:"WorkingDir"`
	Labels     map[string]string `json:"Labels"`
}

type auditOutput struct {
	Tool                  string            `json:"tool"`
	Status                string            `json:"status"`
	CheckedAt             string            `json:"checked_at"`
	ImageRefSHA256        string            `json:"image_ref_sha256"`
	ImageDigest           string            `json:"image_digest"`
	ExpectedEntrypoint    string            `json:"expected_entrypoint"`
	Entrypoint            []string          `json:"entrypoint"`
	Cmd                   []string          `json:"cmd,omitempty"`
	User                  string            `json:"user,omitempty"`
	WorkingDir            string            `json:"working_dir,omitempty"`
	Labels                map[string]string `json:"labels,omitempty"`
	GenericContractRunner bool              `json:"generic_contract_runner"`
	SecretValuesRetained  bool              `json:"secret_values_retained"`
}

type imageInspector interface {
	InspectImageConfig(ctx context.Context, imageRef string) (dockerImageConfig, error)
}

type dockerInspector struct {
	bin string
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, dockerInspector{bin: defaultDockerBin}, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[sandbox-image-audit] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[sandbox-image-audit] OK")
}

func run(ctx context.Context, args []string, getenv func(string) string, inspector imageInspector, stdout io.Writer) error {
	cfg, err := parseArgs(args, getenv)
	if err != nil {
		return err
	}
	if inspector == nil {
		return fmt.Errorf("image inspector must be configured")
	}
	out, err := auditImage(ctx, cfg, inspector, nowUTC())
	if err != nil {
		return err
	}
	if cfg.proofOut != "" {
		if err := writeProofFile(cfg.proofOut, out); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("write audit output: %w", err)
	}
	return nil
}

func parseArgs(args []string, getenv func(string) string) (config, error) {
	cfg := config{
		expectedEntrypoint: defaultExpectedEntrypoint,
		dockerBin:          defaultDockerBin,
	}
	fs := flag.NewFlagSet("sandbox_image_audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.imageRef, "image-ref", "", "digest-pinned diagnosis sandbox image ref; defaults to OPENCLARION_SANDBOX_IMAGE_REF")
	fs.StringVar(&cfg.proofOut, "proof-out", "", "optional private JSON proof output path")
	fs.StringVar(&cfg.expectedEntrypoint, "expected-entrypoint", defaultExpectedEntrypoint, "required first entrypoint argument")
	fs.StringVar(&cfg.dockerBin, "docker-bin", defaultDockerBin, "docker executable")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("usage: sandbox_image_audit [--image-ref REF] [--proof-out PATH]")
	}
	cfg.imageRef = strings.TrimSpace(cfg.imageRef)
	if cfg.imageRef == "" && getenv != nil {
		cfg.imageRef = strings.TrimSpace(getenv("OPENCLARION_SANDBOX_IMAGE_REF"))
	}
	cfg.expectedEntrypoint = strings.TrimSpace(cfg.expectedEntrypoint)
	cfg.dockerBin = strings.TrimSpace(cfg.dockerBin)
	cfg.proofOut = strings.TrimSpace(cfg.proofOut)
	if cfg.imageRef == "" {
		return config{}, fmt.Errorf("OPENCLARION_SANDBOX_IMAGE_REF is required")
	}
	if cfg.expectedEntrypoint == "" {
		return config{}, fmt.Errorf("--expected-entrypoint must be non-empty")
	}
	if cfg.dockerBin == "" {
		return config{}, fmt.Errorf("--docker-bin must be non-empty")
	}
	if strings.ContainsAny(cfg.imageRef+cfg.expectedEntrypoint+cfg.dockerBin, "\r\n\t") {
		return config{}, fmt.Errorf("arguments must be single-line values")
	}
	if cfg.proofOut != "" && strings.ContainsAny(cfg.proofOut, "\r\n\t") {
		return config{}, fmt.Errorf("--proof-out must be a single-line path")
	}
	return cfg, nil
}

func auditImage(ctx context.Context, cfg config, inspector imageInspector, checkedAt time.Time) (auditOutput, error) {
	match := digestRefPattern.FindStringSubmatch(cfg.imageRef)
	if match == nil {
		return auditOutput{}, fmt.Errorf("sandbox image must be pinned as name@sha256:<64 lowercase hex digest>")
	}
	imageConfig, err := inspector.InspectImageConfig(ctx, cfg.imageRef)
	if err != nil {
		return auditOutput{}, err
	}
	out := auditOutput{
		Tool:                  toolName,
		Status:                "pass",
		CheckedAt:             checkedAt.UTC().Format(time.RFC3339Nano),
		ImageRefSHA256:        sha256Hex(cfg.imageRef),
		ImageDigest:           "sha256:" + match[1],
		ExpectedEntrypoint:    cfg.expectedEntrypoint,
		Entrypoint:            cloneStrings(imageConfig.Entrypoint),
		Cmd:                   cloneStrings(imageConfig.Cmd),
		User:                  strings.TrimSpace(imageConfig.User),
		WorkingDir:            strings.TrimSpace(imageConfig.WorkingDir),
		Labels:                sanitizedLabels(imageConfig.Labels),
		GenericContractRunner: hasArg(imageConfig.Entrypoint, "custom-thin-runner") || hasArg(imageConfig.Cmd, "custom-thin-runner"),
		SecretValuesRetained:  false,
	}
	if len(out.Entrypoint) == 0 {
		return auditOutput{}, fmt.Errorf("sandbox image config has empty entrypoint")
	}
	if out.Entrypoint[0] != cfg.expectedEntrypoint {
		return auditOutput{}, fmt.Errorf("sandbox image entrypoint %q, want %q", out.Entrypoint[0], cfg.expectedEntrypoint)
	}
	if out.GenericContractRunner {
		return auditOutput{}, fmt.Errorf("sandbox image appears to be the generic custom-thin-runner, not the diagnosis assistant runtime")
	}
	return out, nil
}

func (i dockerInspector) InspectImageConfig(ctx context.Context, imageRef string) (dockerImageConfig, error) {
	bin := strings.TrimSpace(i.bin)
	if bin == "" {
		bin = defaultDockerBin
	}
	cmd := exec.CommandContext(ctx, bin, "image", "inspect", imageRef, "--format", "{{json .Config}}") // #nosec G204 -- manual audit runs the configured Docker-compatible CLI with fixed arguments.
	raw, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if msg != "" {
				return dockerImageConfig{}, fmt.Errorf("docker image inspect failed: %s", singleLine(msg))
			}
		}
		return dockerImageConfig{}, fmt.Errorf("docker image inspect failed: %w", err)
	}
	if len(raw) > maxDockerInspectBytes {
		return dockerImageConfig{}, fmt.Errorf("docker image inspect output exceeds %d bytes", maxDockerInspectBytes)
	}
	var cfg dockerImageConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return dockerImageConfig{}, fmt.Errorf("decode docker image config: %w", err)
	}
	return cfg, nil
}

func writeProofFile(path string, out auditOutput) error {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return fmt.Errorf("create proof dir: %w", err)
	}
	if _, err := os.Lstat(clean); err == nil {
		return fmt.Errorf("proof output path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat proof output: %w", err)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal proof: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(clean, raw, 0o600); err != nil { // #nosec G703 -- manual audit writes the operator-selected proof path after cleaning and refusing overwrite.
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func sanitizedLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || strings.ContainsAny(key+value, "\r\n\t") {
			continue
		}
		if len(key) > 128 || len(value) > 256 {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

func hasArg(args []string, needle string) bool {
	for _, arg := range args {
		if strings.Contains(arg, needle) {
			return true
		}
	}
	return false
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func singleLine(value string) string {
	fields := strings.Fields(value)
	value = strings.Join(fields, " ")
	if len(value) > 240 {
		return value[:240]
	}
	return value
}
