// Command custom_thin_runner is a minimal ADR-0013 runtime candidate used by
// the M4 local smoke gate. It proves the sandbox file contract without adding
// planning, memory, approval, or multi-agent behavior.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	defaultEvidencePath     = "/workspace/evidence.json"
	defaultConversationPath = "/workspace/conversation.json"
	defaultMessagePath      = "/workspace/message.json"
	defaultAgentConfigDir   = "/workspace/agent_config"
	defaultOutputPath       = "/workspace/out/output.json"
)

type runnerPaths struct {
	Evidence     string
	Conversation string
	Message      string
	AgentConfig  string
	Output       string
}

func main() {
	if err := run(defaultPaths()); err != nil {
		fmt.Fprintf(os.Stderr, "[custom-thin-runner] %v\n", err)
		os.Exit(1)
	}
}

func defaultPaths() runnerPaths {
	return runnerPaths{
		Evidence:     defaultEvidencePath,
		Conversation: defaultConversationPath,
		Message:      defaultMessagePath,
		AgentConfig:  defaultAgentConfigDir,
		Output:       defaultOutputPath,
	}
}

func run(paths runnerPaths) error {
	evidence, err := readJSONFile(paths.Evidence, "evidence")
	if err != nil {
		return err
	}
	conversation, err := readJSONFile(paths.Conversation, "conversation")
	if err != nil {
		return err
	}
	message, err := readJSONFile(paths.Message, "message")
	if err != nil {
		return err
	}
	configEntries, err := listConfigEntries(paths.AgentConfig)
	if err != nil {
		return err
	}

	output := map[string]any{
		"runtime":  "custom-thin-runner",
		"contract": "adr-0013",
		"inputs": map[string]any{
			"evidence_sha256":      sha256Hex(evidence),
			"conversation_sha256":  sha256Hex(conversation),
			"message_sha256":       sha256Hex(message),
			"agent_config_entries": configEntries,
		},
		"analysis": "ADR-0013 file contract validated without external tools",
	}
	return writeJSONObject(paths.Output, output)
}

func readJSONFile(path, label string) ([]byte, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean, label); err != nil {
		return nil, err
	}
	// #nosec G304 -- this candidate runtime only reads fixed ADR-0013 paths,
	// overridden in tests through runnerPaths.
	raw, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("read %s JSON: %w", label, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s JSON is empty", label)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("%s JSON is invalid: %w", label, err)
	}
	return raw, nil
}

func requireRegularFile(path, label string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat %s JSON: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s JSON path %s must be a regular file, not a symlink", label, clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s JSON path %s must be a regular file", label, clean)
	}
	return nil
}

func listConfigEntries(dir string) ([]string, error) {
	clean := filepath.Clean(dir)
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, fmt.Errorf("read agent config dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func writeJSONObject(path string, value map[string]any) error {
	clean := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(clean), 0o700); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal output JSON: %w", err)
	}
	raw = append(raw, '\n')
	tmp := clean + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write output JSON: %w", err)
	}
	if err := os.Rename(tmp, clean); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publish output JSON: %w", err)
	}
	return nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
