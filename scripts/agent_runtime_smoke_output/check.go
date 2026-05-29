// Command agent_runtime_smoke_output validates the JSON file produced by a
// candidate M4/M5 sandbox runtime image during the manual smoke gate.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const defaultMaxBytes int64 = 10 * 1024 * 1024

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[agent-runtime-smoke-output] %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: agent_runtime_smoke_output <output.json>")
	}
	path := filepath.Clean(args[0])
	if err := requireRegularFile(path); err != nil {
		return err
	}
	// #nosec G304,G703 -- this manual smoke checker opens the operator-supplied output JSON path.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, defaultMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read output: %w", err)
	}
	size := int64(len(raw))
	if size == 0 {
		return errors.New("output.json is empty")
	}
	if size > defaultMaxBytes {
		return fmt.Errorf("output.json size %d exceeds maximum %d", size, defaultMaxBytes)
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
	fmt.Fprintf(stdout, "[agent-runtime-smoke-output] OK (%d bytes)\n", size)
	return nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
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
