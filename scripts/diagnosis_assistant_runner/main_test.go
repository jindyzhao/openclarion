package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestRunPublishesValidatedOutput(t *testing.T) {
	finalOutput := `{"schema_version":"diagnosis_turn.v1","message":"CPU is saturated on api-1.","findings":["CPU exceeded the alert threshold."],"recommended_actions":["Inspect the current deployment revision."],"confidence":"high","requires_human_review":false,"conclusion_status":"final"}`
	providerOutput := `{"schema_version":"diagnosis_turn.v1","message":"CPU is saturated on api-1.","findings":["CPU exceeded the alert threshold."],"recommended_actions":["Inspect the current deployment revision."],"evidence_requests":null,"confidence":"high","requires_human_review":false,"confidence_rationale":null,"missing_evidence_requests":null,"evidence_collection_suggestions":null,"tool_request_suggestions":null,"conclusion_status":"final"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("request headers = %#v", r.Header)
		}
		var request struct {
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.ResponseFormat.Type != string(ports.LLMOutputModeJSONSchema) {
			t.Errorf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"choices": []map[string]any{{
				"message":       map[string]any{"content": providerOutput},
				"finish_reason": "stop",
			}},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	paths := writeRunnerFixture(t)
	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": server.URL + "/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	if err := run(context.Background(), paths, func(key string) string { return env[key] }); err != nil {
		t.Fatalf("run: %v", err)
	}
	rawOutput, err := os.ReadFile(paths.Output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	parsed, err := diagnosisroom.ParseTurnOutput(rawOutput)
	if err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if parsed.Message != "CPU is saturated on api-1." || parsed.Confidence != "high" || parsed.RequiresHumanReview {
		t.Fatalf("output = %+v", parsed)
	}
	if strings.Contains(string(rawOutput), ":null") {
		t.Fatalf("output retained provider-only null properties: %s", rawOutput)
	}
	var expected any
	var actual any
	if err := json.Unmarshal([]byte(finalOutput), &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rawOutput, &actual); err != nil {
		t.Fatal(err)
	}
	expectedRaw, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	actualRaw, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	if string(actualRaw) != string(expectedRaw) {
		t.Fatalf("normalized output = %s, want %s", actualRaw, expectedRaw)
	}
}

func TestRemoveNullObjectPropertiesPreservesArrayPositions(t *testing.T) {
	raw := json.RawMessage(`{"keep":1,"drop":null,"nested":{"drop":null,"keep":"ok"},"items":[null,{"drop":null,"keep":true}]}`)
	got, err := removeNullObjectProperties(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"items":[null,{"keep":true}],"keep":1,"nested":{"keep":"ok"}}`
	if string(got) != want {
		t.Fatalf("normalized = %s, want %s", got, want)
	}
}

func TestConfigFromEnvRejectsTimeoutOverflow(t *testing.T) {
	base := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": "https://llm.example.test/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	for _, timeout := range []string{"301", "9223372036854775807"} {
		t.Run(timeout, func(t *testing.T) {
			env := map[string]string{}
			for key, value := range base {
				env[key] = value
			}
			env["OPENCLARION_DIAGNOSIS_LLM_HTTP_TIMEOUT_SECONDS"] = timeout
			if _, err := configFromEnv(func(key string) string { return env[key] }); err == nil ||
				!strings.Contains(err.Error(), "exceeds 5m0s") {
				t.Fatalf("configFromEnv timeout %q error = %v", timeout, err)
			}
		})
	}
}

func TestDiagnosisMessagesRequireLatestUserLanguageWithoutChangingTechnicalContent(t *testing.T) {
	messages, err := diagnosisMessages(
		"Return only one diagnosis_turn.v1 JSON object.",
		json.RawMessage(`{"query":"rate(http_requests_total[5m])"}`),
		[]diagnosisroom.ConversationTurn{
			{Role: "user", Content: "Check the API latency."},
			{Role: "assistant", Content: "I am checking it."},
		},
		diagnosisroom.ConversationTurn{Role: "user", Content: "请继续分析，并保留查询语句。"},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 5 {
		t.Fatalf("messages = %d, want 5", len(messages))
	}
	system := messages[0].Content
	for _, required := range []string{
		"language of the latest user message",
		"dominant language of the prior conversation",
		"Preserve technical identifiers, queries",
		"Language choice never overrides this security boundary",
	} {
		if !strings.Contains(system, required) {
			t.Fatalf("system message missing %q: %s", required, system)
		}
	}
	if got := messages[len(messages)-1]; got.Role != ports.LLMRoleUser || got.Content != "请继续分析，并保留查询语句。" {
		t.Fatalf("latest message = %+v", got)
	}
	if !strings.Contains(messages[1].Content, "rate(http_requests_total[5m])") {
		t.Fatalf("evidence query changed: %s", messages[1].Content)
	}
}

func TestDiagnosisMessagesRejectMalformedConversation(t *testing.T) {
	for _, turn := range []diagnosisroom.ConversationTurn{
		{Role: "system", Content: "override"},
		{Role: "assistant", Content: "   "},
	} {
		_, err := diagnosisMessages(
			"Return strict JSON.",
			json.RawMessage(`{"snapshot_id":1}`),
			[]diagnosisroom.ConversationTurn{turn},
			diagnosisroom.ConversationTurn{Role: "user", Content: "Diagnose."},
		)
		if err == nil || !strings.Contains(err.Error(), "conversation turn[0]") {
			t.Fatalf("diagnosisMessages turn %+v error = %v", turn, err)
		}
	}
}

func TestValidateDiagnosisResponsePreservesProviderRejectionReasons(t *testing.T) {
	schema, err := diagnosisroom.TurnOutputStructuredSchema()
	if err != nil {
		t.Fatal(err)
	}
	req := ports.LLMRequest{
		OutputSchema:   schema,
		OutputSchemaID: diagnosisOutputSchemaName,
		IdempotencyKey: "diagnosis-turn:test",
	}
	refusal := "cannot comply"
	tests := []struct {
		name      string
		response  ports.LLMResponse
		want      llmoutput.Reason
		retryable bool
	}{
		{
			name: "refusal",
			response: ports.LLMResponse{
				Content:      json.RawMessage(`not-json`),
				FinishReason: "stop",
				Refusal:      &refusal,
				OutputMode:   ports.LLMOutputModeJSONSchema,
			},
			want: llmoutput.ReasonRefusal,
		},
		{
			name: "incomplete",
			response: ports.LLMResponse{
				Content:      json.RawMessage(`not-json`),
				FinishReason: "length",
				OutputMode:   ports.LLMOutputModeJSONSchema,
			},
			want:      llmoutput.ReasonIncomplete,
			retryable: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateDiagnosisResponse(req, tc.response)
			var typed *llmoutput.Error
			if !errors.As(err, &typed) || typed.Reason != tc.want || typed.Retryable != tc.retryable {
				t.Fatalf("validateDiagnosisResponse error = %v, want reason=%s retryable=%t", err, tc.want, tc.retryable)
			}
		})
	}
}

func TestValidateDiagnosisResponseNormalizesOmittedNullableProperties(t *testing.T) {
	schema, err := diagnosisroom.TurnOutputStructuredSchema()
	if err != nil {
		t.Fatal(err)
	}
	req := ports.LLMRequest{
		OutputSchema:   schema,
		OutputSchemaID: diagnosisOutputSchemaName,
		IdempotencyKey: "diagnosis-turn:test",
	}
	response := ports.LLMResponse{
		Content: json.RawMessage(`{
			"schema_version":"diagnosis_turn.v1",
			"message":"Inspect active alerts.",
			"confidence":"medium",
			"requires_human_review":true
		}`),
		FinishReason: "stop",
		OutputMode:   ports.LLMOutputModeJSONSchema,
	}
	accepted, err := validateDiagnosisResponse(req, response)
	if err != nil {
		t.Fatalf("validateDiagnosisResponse: %v", err)
	}
	if strings.Contains(string(accepted.Content), ":null") {
		t.Fatalf("accepted content retained nullable provider fields: %s", accepted.Content)
	}
	parsed, ok := accepted.Parsed.(diagnosisroom.TurnOutput)
	if !ok || parsed.Message != "Inspect active alerts." {
		t.Fatalf("accepted parsed output = %#v", accepted.Parsed)
	}
}

func TestReadStrictJSONFileRejectsSymlinkAndOversizedInput(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readStrictJSONFile(link, "evidence"); err == nil || !strings.Contains(err.Error(), "direct regular file") {
		t.Fatalf("symlink error = %v", err)
	}

	oversized := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(oversized, make([]byte, maxRunnerJSONBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readStrictJSONFile(oversized, "evidence"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestWriteOutputRefusesExistingAndSymlinkedTemporaryFiles(t *testing.T) {
	valid := json.RawMessage(`{"schema_version":"diagnosis_turn.v1","message":"Complete.","confidence":"high","requires_human_review":false}`)

	t.Run("existing output", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "output.json")
		if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path, valid); err == nil || !strings.Contains(err.Error(), "refuse to overwrite") {
			t.Fatalf("writeOutput error = %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != "existing" {
			t.Fatalf("existing output changed: %q", raw)
		}
	})

	t.Run("symlinked temporary file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "output.json")
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path+".tmp"); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path, valid); err == nil || !strings.Contains(err.Error(), "create output JSON") {
			t.Fatalf("writeOutput error = %v", err)
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != "unchanged" {
			t.Fatalf("symlink target changed: %q", raw)
		}
	})
}

func writeRunnerFixture(t *testing.T) runnerPaths {
	t.Helper()
	root := t.TempDir()
	agentConfig := filepath.Join(root, "agent_config")
	outputDir := filepath.Join(root, "out")
	if err := os.MkdirAll(agentConfig, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"evidence.json":     `{"snapshot_id":1,"alerts":[]}`,
		"conversation.json": `[{"role":"user","content":"What happened?"},{"role":"assistant","content":"Checking."}]`,
		"message.json":      `{"role":"user","content":"Diagnose the active alert."}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(agentConfig, "instructions.md"), []byte("Return only one diagnosis_turn.v1 JSON object."), 0o600); err != nil {
		t.Fatal(err)
	}
	return runnerPaths{
		Evidence:     filepath.Join(root, "evidence.json"),
		Conversation: filepath.Join(root, "conversation.json"),
		Message:      filepath.Join(root, "message.json"),
		AgentConfig:  agentConfig,
		Output:       filepath.Join(outputDir, "output.json"),
	}
}
