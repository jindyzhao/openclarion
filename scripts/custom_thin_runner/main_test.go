package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesContractProof(t *testing.T) {
	root := t.TempDir()
	paths := writeFixture(t, root)

	if err := run(paths); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := os.ReadFile(paths.Output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if output["runtime"] != "custom-thin-runner" {
		t.Fatalf("runtime = %v", output["runtime"])
	}
	if output["contract"] != "adr-0013" {
		t.Fatalf("contract = %v", output["contract"])
	}
	inputs, ok := output["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("inputs missing or wrong type: %#v", output["inputs"])
	}
	if inputs["evidence_sha256"] == "" || inputs["conversation_sha256"] == "" || inputs["message_sha256"] == "" {
		t.Fatalf("input hashes must be populated: %#v", inputs)
	}
	entries, ok := inputs["agent_config_entries"].([]any)
	if !ok || len(entries) != 1 || entries[0] != "agent.yaml" {
		t.Fatalf("agent_config_entries = %#v", inputs["agent_config_entries"])
	}
}

func TestRunRejectsInvalidInputJSON(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError string
	}{
		{
			name:      "malformed",
			content:   `{"broken"`,
			wantError: "evidence JSON is invalid",
		},
		{
			name:      "duplicate key",
			content:   `{"snapshot_id":1,"snapshot_id":2}`,
			wantError: `duplicate object key "snapshot_id"`,
		},
		{
			name:      "trailing value",
			content:   `{"snapshot_id":1}[]`,
			wantError: "trailing JSON values",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			paths := writeFixture(t, root)
			if err := os.WriteFile(paths.Evidence, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write invalid evidence: %v", err)
			}

			err := run(paths)
			if err == nil {
				t.Fatal("run err = nil, want invalid JSON error")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("run err = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestRunRejectsSymlinkInputJSON(t *testing.T) {
	root := t.TempDir()
	paths := writeFixture(t, root)
	target := filepath.Join(root, "real-evidence.json")
	if err := os.Rename(paths.Evidence, target); err != nil {
		t.Fatalf("rename evidence: %v", err)
	}
	createSymlinkOrSkip(t, target, paths.Evidence)

	err := run(paths)
	if err == nil {
		t.Fatal("run err = nil, want symlink input rejection")
	}
	if !strings.Contains(err.Error(), "evidence JSON path") ||
		!strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("run err = %v, want evidence symlink rejection", err)
	}
}

func TestRunRejectsMissingAgentConfig(t *testing.T) {
	root := t.TempDir()
	paths := writeFixture(t, root)
	if err := os.RemoveAll(paths.AgentConfig); err != nil {
		t.Fatalf("remove agent config: %v", err)
	}

	err := run(paths)
	if err == nil {
		t.Fatal("run err = nil, want missing config error")
	}
	if !strings.Contains(err.Error(), "agent config dir") {
		t.Fatalf("run err = %v", err)
	}
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}

func writeFixture(t *testing.T, root string) runnerPaths {
	t.Helper()
	agentConfig := filepath.Join(root, "agent_config")
	output := filepath.Join(root, "out", "output.json")
	if err := os.MkdirAll(agentConfig, 0o700); err != nil {
		t.Fatalf("mkdir agent config: %v", err)
	}
	fixtures := map[string]string{
		"evidence.json":     `{"snapshot_id":1,"alerts":[]}`,
		"conversation.json": `[]`,
		"message.json":      `{"role":"user","content":"summarize"}`,
	}
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(agentConfig, "agent.yaml"), []byte("name: custom-thin-runner\n"), 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	return runnerPaths{
		Evidence:     filepath.Join(root, "evidence.json"),
		Conversation: filepath.Join(root, "conversation.json"),
		Message:      filepath.Join(root, "message.json"),
		AgentConfig:  agentConfig,
		Output:       output,
	}
}
