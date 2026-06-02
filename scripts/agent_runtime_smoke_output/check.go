// Command agent_runtime_smoke_output validates the JSON file produced by a
// candidate M4/M5 sandbox runtime image during the manual smoke gate.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const defaultMaxBytes int64 = 10 * 1024 * 1024

var digestPinnedImageRE = regexp.MustCompile(`^[^\s@]+@sha256:[A-Fa-f0-9]{64}$`)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[agent-runtime-smoke-output] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	path := filepath.Clean(cfg.outputPath)
	if err := requireRegularFile(path); err != nil {
		return err
	}
	// #nosec G304,G703 -- this manual smoke checker opens the operator-supplied output JSON path.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, cfg.outputMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}
	size := int64(len(raw))
	if size == 0 {
		return errors.New("output.json is empty")
	}
	if size > cfg.outputMaxBytes {
		return fmt.Errorf("output.json size %d exceeds maximum %d", size, cfg.outputMaxBytes)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("output.json is invalid JSON: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("output.json is invalid JSON: %w", err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("output.json must be a JSON object")
	}
	if len(object) == 0 {
		return errors.New("output.json must be a non-empty JSON object")
	}
	outputDigest := sha256.Sum256(raw)
	outputDigestHex := hex.EncodeToString(outputDigest[:])
	if cfg.proofPath != "" {
		if err := writeProof(cfg, size, outputDigestHex); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "[agent-runtime-smoke-output] OK (%d bytes)\n", size)
	return nil
}

type config struct {
	outputPath       string
	proofPath        string
	runtimeCandidate string
	source           string
	outputMaxBytes   int64
}

func parseArgs(args []string) (config, error) {
	cfg := config{
		source:         "agent_runtime_smoke_output",
		outputMaxBytes: defaultMaxBytes,
	}
	fs := flag.NewFlagSet("agent_runtime_smoke_output", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.proofPath, "proof", "", "optional path for retained smoke proof JSON")
	fs.StringVar(&cfg.runtimeCandidate, "runtime-candidate", "", "digest-pinned runtime candidate image ref, required with --proof")
	fs.StringVar(&cfg.source, "source", cfg.source, "proof source, usually a make target")
	fs.Int64Var(&cfg.outputMaxBytes, "output-max-bytes", cfg.outputMaxBytes, "maximum output.json bytes")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 1 {
		return config{}, errors.New("usage: agent_runtime_smoke_output [--proof proof.json] [--runtime-candidate image@sha256:<digest>] [--source source] [--output-max-bytes bytes] <output.json>")
	}
	cfg.outputPath = fs.Arg(0)
	if cfg.outputMaxBytes <= 0 {
		return config{}, errors.New("--output-max-bytes must be a positive integer")
	}
	if cfg.proofPath != "" && cfg.runtimeCandidate == "" {
		return config{}, errors.New("--runtime-candidate is required when --proof is set")
	}
	if cfg.runtimeCandidate != "" && !digestPinnedImageRE.MatchString(cfg.runtimeCandidate) {
		return config{}, fmt.Errorf("--runtime-candidate must be pinned by sha256 digest: %s", cfg.runtimeCandidate)
	}
	if cfg.source == "" {
		return config{}, errors.New("--source must not be empty")
	}
	return cfg, nil
}

type proofArtifact struct {
	Tool             string       `json:"tool"`
	Status           string       `json:"status"`
	Source           string       `json:"source"`
	RuntimeCandidate string       `json:"runtime_candidate"`
	Output           proofOutput  `json:"output"`
	Checks           []proofCheck `json:"checks"`
}

type proofOutput struct {
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	MaxBytes int64  `json:"max_bytes"`
	SHA256   string `json:"sha256"`
}

type proofCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func writeProof(cfg config, outputBytes int64, outputSHA256 string) error {
	proof := proofArtifact{
		Tool:             "agent-runtime-smoke",
		Status:           "pass",
		Source:           cfg.source,
		RuntimeCandidate: cfg.runtimeCandidate,
		Output: proofOutput{
			Path:     "/workspace/out/output.json",
			Bytes:    outputBytes,
			MaxBytes: cfg.outputMaxBytes,
			SHA256:   outputSHA256,
		},
		Checks: []proofCheck{
			{Name: "regular_output_file", Status: "pass"},
			{Name: "bounded_output_size", Status: "pass"},
			{Name: "valid_json_object", Status: "pass"},
			{Name: "duplicate_key_free", Status: "pass"},
			{Name: "non_empty_object", Status: "pass"},
		},
	}
	return writeJSONFile(cfg.proofPath, proof)
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

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	if err := validateNoSymlinkAncestors(clean); err != nil {
		return err
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat output: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
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
