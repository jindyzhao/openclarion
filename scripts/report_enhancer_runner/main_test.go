package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
)

func TestRunUsesProductionPromptAndPublishesValidatedSubReport(t *testing.T) {
	wantOutput := validSubReport("snapshot:11")
	var sawRequest atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Client-Request-Id") == "" {
			t.Error("X-Client-Request-Id is empty")
		}
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Name   string `json:"name"`
					Strict bool   `json:"strict"`
					Schema any    `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(request.Messages) != 2 ||
			!strings.Contains(request.Messages[0].Content, "reviewed guidance") ||
			!strings.Contains(request.Messages[1].Content, "Scenario: cascade") ||
			request.ResponseFormat.Type != string(ports.LLMOutputModeJSONSchema) ||
			request.ResponseFormat.JSONSchema.Name != reportdraft.SubReportSchemaID ||
			!request.ResponseFormat.JSONSchema.Strict || request.ResponseFormat.JSONSchema.Schema == nil {
			t.Errorf("request = %+v", request)
		}
		sawRequest.Store(true)
		writeCompletion(t, w, wantOutput)
	}))
	defer server.Close()

	paths := writeRunnerFixture(t)
	env := validRunnerEnv(server.URL + "/v1")
	if err := run(context.Background(), paths, func(key string) string { return env[key] }); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !sawRequest.Load() {
		t.Fatal("provider request was not observed")
	}
	raw, err := os.ReadFile(paths.Output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	report, err := reportdraft.ParseSubReport(ports.LLMResponse{
		Content:      raw,
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
		Model:        "test-model",
	})
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if report.Title != "CPU saturation" || !containsString(report.EvidenceRefs, "snapshot:11") {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunRetriesWhenSubReportOmitsCanonicalSnapshotRef(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		call := calls.Add(1)
		if call == 1 {
			writeCompletion(t, w, validSubReport("alert:cpu"))
			return
		}
		if !strings.Contains(request.Messages[0].Content, `evidence_refs must include "snapshot:11"`) {
			t.Errorf("retry feedback = %q", request.Messages[0].Content)
		}
		writeCompletion(t, w, validSubReport("snapshot:11"))
	}))
	defer server.Close()

	paths := writeRunnerFixture(t)
	env := validRunnerEnv(server.URL + "/v1")
	if err := run(context.Background(), paths, func(key string) string { return env[key] }); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
}

func TestRunPropagatesContextCancellationToProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer server.Close()

	paths := writeRunnerFixture(t)
	env := validRunnerEnv(server.URL + "/v1")
	env["OPENCLARION_REPORT_LLM_HTTP_TIMEOUT_SECONDS"] = "5"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := run(ctx, paths, func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("run err = %v, want context deadline", err)
	}
	if _, statErr := os.Lstat(paths.Output); !os.IsNotExist(statErr) {
		t.Fatalf("output stat err = %v, want absent output", statErr)
	}
}

func TestRunContractSmokeValidatesMountedInputsWithoutLLMConfiguration(t *testing.T) {
	paths := writeRunnerFixture(t)
	dir := filepath.Dir(paths.Evidence)
	paths.Conversation = filepath.Join(dir, "conversation.json")
	paths.Message = filepath.Join(dir, "message.json")
	if err := os.WriteFile(paths.Conversation, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Message, []byte(`{"role":"user","content":"contract smoke"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runContractSmoke(paths); err != nil {
		t.Fatalf("runContractSmoke: %v", err)
	}
	raw, err := os.ReadFile(paths.Output)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Runtime  string `json:"runtime"`
		Mode     string `json:"mode"`
		Contract string `json:"contract"`
		Inputs   struct {
			EvidenceSHA256     string   `json:"evidence_sha256"`
			ConversationSHA256 string   `json:"conversation_sha256"`
			MessageSHA256      string   `json:"message_sha256"`
			ConfigEntries      []string `json:"agent_config_entries"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output.Runtime != "report-enhancer-runner" || output.Mode != "contract-smoke" || output.Contract != "adr-0013" {
		t.Fatalf("output = %+v", output)
	}
	if len(output.Inputs.EvidenceSHA256) != 64 || len(output.Inputs.ConversationSHA256) != 64 || len(output.Inputs.MessageSHA256) != 64 {
		t.Fatalf("input hashes = %+v", output.Inputs)
	}
	if len(output.Inputs.ConfigEntries) != 1 || output.Inputs.ConfigEntries[0] != defaultInstructionsFile {
		t.Fatalf("config entries = %v", output.Inputs.ConfigEntries)
	}
}

func TestRunRejectsPayloadChecksumMismatchBeforeProviderCall(t *testing.T) {
	paths := writeRunnerFixture(t)
	raw, err := os.ReadFile(paths.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	var envelope evidenceEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.PayloadSHA256 = strings.Repeat("0", sha256.Size*2)
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Evidence, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	env := validRunnerEnv("https://llm.example.com/v1")
	err = run(context.Background(), paths, func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("run err = %v, want payload checksum mismatch", err)
	}
}

func TestReadStrictJSONFileRejectsAmbiguousAndIndirectInputs(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "duplicate", raw: `{"a":1,"a":2}`, want: "duplicate object key"},
		{name: "trailing", raw: `{} {}`, want: "trailing JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".json")
			if err := os.WriteFile(path, []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readStrictJSONFile(path, "fixture"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readStrictJSONFile err = %v, want %q", err, tt.want)
			}
		})
	}
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readStrictJSONFile(link, "fixture"); err == nil || !strings.Contains(err.Error(), "direct regular file") {
		t.Fatalf("readStrictJSONFile symlink err = %v", err)
	}
}

func TestValidateAgentConfigDirectoryRejectsExtraEntries(t *testing.T) {
	paths := writeRunnerFixture(t)
	if err := os.WriteFile(filepath.Join(paths.AgentConfig, "unreviewed.md"), []byte("extra"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateAgentConfigDirectory(paths.AgentConfig); err == nil || !strings.Contains(err.Error(), "contain only") {
		t.Fatalf("validateAgentConfigDirectory err = %v", err)
	}
}

func TestReadInstructionsRejectsInvalidText(t *testing.T) {
	paths := writeRunnerFixture(t)
	instructions := filepath.Join(paths.AgentConfig, defaultInstructionsFile)
	if err := os.WriteFile(instructions, []byte{'r', 'e', 'v', 0, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readInstructions(paths.AgentConfig); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("readInstructions err = %v", err)
	}
}

func TestWriteOutputRefusesExistingOrIndirectDestination(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "output.json")
	if err := os.WriteFile(output, []byte(`{"existing":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeOutput(output, json.RawMessage(`{"new":true}`)); err == nil || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("writeOutput existing err = %v", err)
	}
	// #nosec G304 -- output is created under t.TempDir in this test.
	raw, err := os.ReadFile(output)
	if err != nil || string(raw) != `{"existing":true}` {
		t.Fatalf("existing output changed: raw=%q err=%v", raw, err)
	}

	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	if err := writeOutput(filepath.Join(linkDir, "output.json"), json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "direct directory") {
		t.Fatalf("writeOutput symlink parent err = %v", err)
	}
}

func TestConfigFromEnvRejectsPartialAndUnsupportedConfiguration(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "partial",
			env:  map[string]string{"OPENCLARION_REPORT_LLM_BASE_URL": "https://llm.example.com/v1"},
			want: "are required",
		},
		{
			name: "output mode",
			env: map[string]string{
				"OPENCLARION_REPORT_LLM_BASE_URL":    "https://llm.example.com/v1",
				"OPENCLARION_REPORT_LLM_API_KEY":     "secret",
				"OPENCLARION_REPORT_LLM_MODEL":       "model",
				"OPENCLARION_REPORT_LLM_OUTPUT_MODE": "yaml",
			},
			want: "OUTPUT_MODE",
		},
		{
			name: "timeout cap",
			env: map[string]string{
				"OPENCLARION_REPORT_LLM_BASE_URL":             "https://llm.example.com/v1",
				"OPENCLARION_REPORT_LLM_API_KEY":              "secret",
				"OPENCLARION_REPORT_LLM_MODEL":                "model",
				"OPENCLARION_REPORT_LLM_HTTP_TIMEOUT_SECONDS": "301",
			},
			want: "exceeds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configFromEnv(func(key string) string { return tt.env[key] })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("configFromEnv err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestReportEnhancerIdempotencyKeyUsesLengthBoundaries(t *testing.T) {
	left := reportEnhancerIdempotencyKey([]byte("a"), []byte("bc"))
	right := reportEnhancerIdempotencyKey([]byte("ab"), []byte("c"))
	if left == right {
		t.Fatalf("length-distinct inputs produced the same idempotency key %q", left)
	}
}

func writeRunnerFixture(t *testing.T) runnerPaths {
	t.Helper()
	dir := t.TempDir()
	agentConfig := filepath.Join(dir, "agent-config")
	if err := os.MkdirAll(agentConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentConfig, defaultInstructionsFile), []byte("Use reviewed guidance and preserve evidence refs.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"schema_version":"evidence.snapshot.v1","events":[{"labels":{"alertname":"CPUHigh"}}]}`)
	digest := sha256.Sum256(payload)
	envelope := evidenceEnvelope{
		Schema:              evidenceSchema,
		EvidenceSnapshotID:  11,
		EvidenceSnapshotRef: "snapshot:11",
		EvidenceDigest:      hex.EncodeToString(digest[:]),
		PayloadSHA256:       hex.EncodeToString(digest[:]),
		Scenario:            "cascade",
		GroupIndex:          2,
		Payload:             payload,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(dir, "evidence.json")
	if err := os.WriteFile(evidencePath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return runnerPaths{
		Evidence:    evidencePath,
		AgentConfig: agentConfig,
		Output:      filepath.Join(outputDir, "output.json"),
	}
}

func validRunnerEnv(baseURL string) map[string]string {
	return map[string]string{
		"OPENCLARION_REPORT_LLM_BASE_URL": baseURL,
		"OPENCLARION_REPORT_LLM_API_KEY":  "test-key",
		"OPENCLARION_REPORT_LLM_MODEL":    "test-model",
	}
}

func validSubReport(evidenceRef string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"title":"CPU saturation",
		"summary":"CPU usage exceeded the alert threshold.",
		"severity":"warning",
		"confidence":"high",
		"findings":[{"label":"CPU pressure","detail":"CPU is above the configured threshold.","evidence_id":%q}],
		"recommended_actions":[{"label":"Inspect workload","detail":"Check the active workload and recent deployment changes.","priority":"medium"}],
		"evidence_refs":[%q]
	}`, evidenceRef, evidenceRef))
}

func writeCompletion(t *testing.T, w http.ResponseWriter, content json.RawMessage) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"model": "test-model",
		"choices": []map[string]any{{
			"message":       map[string]any{"content": string(content)},
			"finish_reason": "stop",
		}},
	}); err != nil {
		t.Errorf("encode completion: %v", err)
	}
}
