package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestRunPublishesValidatedOutput(t *testing.T) {
	finalOutput := `{"schema_version":"diagnosis_turn.v1","message":"CPU is saturated on api-1.","findings":["CPU exceeded the alert threshold."],"recommended_actions":["Inspect the current deployment revision."],"confidence":"high","requires_human_review":false,"conclusion_status":"final"}`
	providerOutput := `{"schema_version":"diagnosis_turn.v1","message":"CPU is saturated on api-1.","findings":["CPU exceeded the alert threshold."],"recommended_actions":["Inspect the current deployment revision."],"evidence_requests":null,"confidence":"high","requires_human_review":false,"confidence_rationale":null,"missing_evidence_requests":null,"evidence_collection_suggestions":null,"tool_request_suggestions":null,"conclusion_status":"final"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("request headers = %#v", r.Header)
		}
		var request struct {
			Stream         bool `json:"stream"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !request.Stream || request.ResponseFormat.Type != string(ports.LLMOutputModeJSONSchema) {
			t.Errorf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		midpoint := strings.Index(providerOutput, "saturated")
		writeTestSSECompletion(t, w, providerOutput[:midpoint], "")
		writeTestSSECompletion(t, w, providerOutput[midpoint:], "")
		writeTestSSECompletion(t, w, "", "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
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
	streamRaw, err := os.ReadFile(filepath.Join(filepath.Dir(paths.Output), filepath.Base(ports.SandboxStreamPath)))
	if err != nil {
		t.Fatalf("read preview stream: %v", err)
	}
	var preview strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(streamRaw)), "\n") {
		var record struct {
			SchemaVersion string `json:"schema_version"`
			Delta         string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode preview record: %v", err)
		}
		if record.SchemaVersion != ports.ContainerStreamSchemaVersion {
			t.Fatalf("preview schema = %q", record.SchemaVersion)
		}
		preview.WriteString(record.Delta)
	}
	if preview.String() != "CPU is saturated on api-1." {
		t.Fatalf("preview = %q", preview.String())
	}
}

func TestRunFallsBackWhenStreamingIsUnsupported(t *testing.T) {
	providerOutput := `{"schema_version":"diagnosis_turn.v1","message":"Use the validated fallback.","findings":[],"recommended_actions":[],"confidence":"medium","requires_human_review":false,"conclusion_status":"final"}`
	var mu sync.Mutex
	streamCalls := 0
	nonStreamingCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		mu.Lock()
		if request.Stream {
			streamCalls++
		} else {
			nonStreamingCalls++
		}
		mu.Unlock()
		if request.Stream {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"stream is not supported","type":"invalid_request_error","param":"stream","code":"unsupported_value"}}`)
			return
		}
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
		t.Fatal(err)
	}
	parsed, err := diagnosisroom.ParseTurnOutput(rawOutput)
	if err != nil || parsed.Message != "Use the validated fallback." {
		t.Fatalf("fallback output = %s parsed=%+v err=%v", rawOutput, parsed, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if streamCalls != 1 || nonStreamingCalls != 1 {
		t.Fatalf("provider calls stream=%d non-streaming=%d, want 1/1", streamCalls, nonStreamingCalls)
	}
}

func TestCheckSandboxReadinessChecksAllowlistAndProxy(t *testing.T) {
	const allowedTarget = "llm.example.invalid:443"
	fingerprint, err := ports.ContainerEgressAllowlistFingerprint([]string{allowedTarget})
	if err != nil {
		t.Fatalf("ContainerEgressAllowlistFingerprint: %v", err)
	}
	client := readinessTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Scheme != "http" ||
			r.URL.Host != "proxy.example.invalid:18080" ||
			r.URL.Path != ports.ContainerEgressProxyReadinessPath ||
			r.Header.Get(ports.ContainerEgressProxyReadinessFingerprintHeader) != fingerprint {
			http.Error(w, "proxy not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":   "https://llm.example.invalid/v1",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":   allowedTarget,
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL": "http://proxy.example.invalid:18080/",
	}
	if err = checkSandboxReadinessWithClient(
		context.Background(),
		func(key string) string { return env[key] },
		client,
	); err != nil {
		t.Fatalf("checkSandboxReadinessWithClient: %v", err)
	}
}

func TestDispatchRunnerReadinessRejectsUncoveredLLMTarget(t *testing.T) {
	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":   "https://llm.example.invalid/v1",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":   "other.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL": "http://proxy.example.invalid:18080",
	}
	err := dispatchRunner(
		context.Background(),
		[]string{readinessCommand},
		func(key string) string { return env[key] },
	)
	if err == nil || !strings.Contains(err.Error(), "host must be listed") {
		t.Fatalf("dispatchRunner readiness error = %v, want uncovered LLM target", err)
	}
}

func TestDispatchRunnerPreservesNormalCommandArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "custom args", args: []string{"--mode", "diagnosis"}},
		{name: "non-exact readiness command", args: []string{readinessCommand, "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := dispatchRunner(context.Background(), tt.args, nil)
			if err == nil || err.Error() != "environment reader is required" {
				t.Fatalf("dispatchRunner error = %v, want normal runner error", err)
			}
		})
	}
}

func TestCheckSandboxReadinessRejectsUnhealthyProxy(t *testing.T) {
	client := readinessTestClient(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":   "https://llm.example.invalid/v1",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":   "llm.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL": "http://proxy.example.invalid:18080",
	}
	err := checkSandboxReadinessWithClient(
		context.Background(),
		func(key string) string { return env[key] },
		client,
	)
	if err == nil || !strings.Contains(err.Error(), "readiness status = 503") {
		t.Fatalf("checkSandboxReadinessWithClient error = %v, want unhealthy proxy status", err)
	}
}

func TestCheckSandboxReadinessRejectsStaleProxyAllowlist(t *testing.T) {
	staleFingerprint, err := ports.ContainerEgressAllowlistFingerprint([]string{"stale.example.invalid:443"})
	if err != nil {
		t.Fatalf("ContainerEgressAllowlistFingerprint: %v", err)
	}
	client := readinessTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(ports.ContainerEgressProxyReadinessFingerprintHeader) != staleFingerprint {
			http.Error(w, "proxy not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL":   "https://llm.example.invalid/v1",
		"OPENCLARION_SANDBOX_EGRESS_ALLOWED":   "llm.example.invalid:443",
		"OPENCLARION_SANDBOX_EGRESS_PROXY_URL": "http://proxy.example.invalid:18080",
	}
	err = checkSandboxReadinessWithClient(
		context.Background(),
		func(key string) string { return env[key] },
		client,
	)
	if err == nil || !strings.Contains(err.Error(), "readiness status = 503") {
		t.Fatalf("checkSandboxReadinessWithClient error = %v, want stale proxy rejection", err)
	}
}

func readinessTestClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSandboxEgressProxyReadinessURLRejectsCredentialsWithoutLeak(t *testing.T) {
	// #nosec G101 -- test-only credential-bearing URL verifies redaction.
	const rawURL = "http://operator:secret@proxy.example.invalid:18080"
	_, err := sandboxEgressProxyReadinessURL(rawURL)
	if err == nil || !strings.Contains(err.Error(), "userinfo") {
		t.Fatalf("sandboxEgressProxyReadinessURL error = %v, want credential rejection", err)
	}
	if strings.Contains(err.Error(), "operator") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("sandboxEgressProxyReadinessURL leaked credentials: %v", err)
	}
}

func TestRunPreservesLatestUserLanguageAcrossValidationRetry(t *testing.T) {
	providerOutputs := []string{
		`{"schema_version":"diagnosis_turn.v1","message":"","confidence":"high","requires_human_review":false}`,
		`{"schema_version":"diagnosis_turn.v1","message":"CPU 使用率过高。","confidence":"high","requires_human_review":false}`,
	}
	var mu sync.Mutex
	requests := make([][]ports.LLMMessage, 0, len(providerOutputs))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []ports.LLMMessage `json:"messages"`
			Stream   bool               `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		mu.Lock()
		index := len(requests)
		requests = append(requests, request.Messages)
		mu.Unlock()
		if index >= len(providerOutputs) {
			http.Error(w, "unexpected provider call", http.StatusInternalServerError)
			return
		}
		if !request.Stream {
			t.Error("stream = false, want true")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeTestSSECompletion(t, w, providerOutputs[index], "")
		writeTestSSECompletion(t, w, "", "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	paths := writeRunnerFixture(t)
	const latestUserMessage = "请继续分析当前告警。"
	if err := os.WriteFile(
		paths.Message,
		[]byte(`{"role":"user","content":"`+latestUserMessage+`"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": server.URL + "/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	if err := run(context.Background(), paths, func(key string) string { return env[key] }); err != nil {
		t.Fatalf("run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("provider requests = %d, want one validation retry", len(requests))
	}
	second := requests[1]
	latestUser := ""
	for _, message := range second {
		if message.Role == ports.LLMRoleUser {
			latestUser = message.Content
		}
	}
	if latestUser != latestUserMessage {
		t.Fatalf("latest retry user message = %q, want %q", latestUser, latestUserMessage)
	}
	if latest := second[len(second)-1]; latest.Role != ports.LLMRoleUser || latest.Content != latestUserMessage {
		t.Fatalf("last retry message = %+v, want original user turn", latest)
	}
	feedback := second[0]
	if feedback.Role != ports.LLMRoleSystem || !strings.Contains(feedback.Content, "failed validation") {
		t.Fatalf("retry feedback = %+v, want application-owned system instruction", feedback)
	}
}

func writeTestSSECompletion(t *testing.T, w http.ResponseWriter, content, finishReason string) {
	t.Helper()
	chunk := map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion.chunk",
		"created": 1,
		"model":   "test-model",
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{"content": content},
			"finish_reason": finishReason,
		}},
	}
	raw, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(w, "data: %s\n\n", raw)
}

func TestRunRejectsNullEvidence(t *testing.T) {
	paths := writeRunnerFixture(t)
	if err := os.WriteFile(paths.Evidence, []byte("null"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": "https://llm.example.test/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	err := run(context.Background(), paths, func(key string) string { return env[key] })
	if err == nil || !strings.Contains(err.Error(), "evidence must be a JSON object") {
		t.Fatalf("run error = %v, want null evidence rejection", err)
	}
	if _, statErr := os.Stat(paths.Output); !os.IsNotExist(statErr) {
		t.Fatalf("output exists after null evidence rejection: %v", statErr)
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
		ports.LLMOutputModeJSONSchema,
		json.RawMessage(`{"type":"object"}`),
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
		"validation criteria belong in message and recommended_actions",
		"Set evidence_requests to JSON null",
		"Never use start, end, step",
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
			ports.LLMOutputModeJSONSchema,
			json.RawMessage(`{"type":"object"}`),
		)
		if err == nil || !strings.Contains(err.Error(), "conversation turn[0]") {
			t.Fatalf("diagnosisMessages turn %+v error = %v", turn, err)
		}
	}
}

func TestDiagnosisMessagesJSONObjectModeIncludesAuthoritativeSchema(t *testing.T) {
	structuredSchema, err := diagnosisroom.TurnOutputStructuredSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence := json.RawMessage(`{
		"snapshot_id": 19,
		"openclarion_available_diagnosis_tools": {
			"usage": "Copy an exact example.",
			"items": [{"evidence_request_example":{"tool":"active_alerts"}}]
		}
	}`)
	messages, err := diagnosisMessages(
		"Return strict JSON.",
		evidence,
		nil,
		diagnosisroom.ConversationTurn{Role: "user", Content: "Provide three response steps."},
		ports.LLMOutputModeJSONObject,
		structuredSchema,
	)
	if err != nil {
		t.Fatal(err)
	}
	system := messages[0].Content
	for _, required := range []string{
		"Backend-approved executable diagnosis tools are present",
		"JSON object mode, which does not enforce field-level schema",
		"Follow this exact server-owned response schema",
		`"evidence_requests"`,
		`"additionalProperties":false`,
	} {
		if !strings.Contains(system, required) {
			t.Fatalf("system message missing %q", required)
		}
	}
}

func TestDiagnosisMessagesRejectsInvalidOutputConfiguration(t *testing.T) {
	tests := []struct {
		name             string
		outputMode       ports.LLMOutputMode
		structuredSchema json.RawMessage
		wantErr          string
	}{
		{
			name:       "JSON object mode without schema",
			outputMode: ports.LLMOutputModeJSONObject,
			wantErr:    "structured schema is required",
		},
		{
			name:             "unsupported output mode",
			outputMode:       ports.LLMOutputMode("text"),
			structuredSchema: json.RawMessage(`{"type":"object"}`),
			wantErr:          `output mode "text" is unsupported`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := diagnosisMessages(
				"Return strict JSON.",
				json.RawMessage(`{"snapshot_id":1}`),
				nil,
				diagnosisroom.ConversationTurn{Role: "user", Content: "Diagnose."},
				tc.outputMode,
				tc.structuredSchema,
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestEvidenceHasExecutableDiagnosisToolsDetectsAvailabilityAndRejectsMalformedCatalog(t *testing.T) {
	tests := []struct {
		name     string
		evidence json.RawMessage
		want     bool
		wantErr  string
	}{
		{name: "absent", evidence: json.RawMessage(`{"snapshot_id":1}`)},
		{
			name: "empty",
			evidence: json.RawMessage(`{
				"openclarion_available_diagnosis_tools":{"usage":"none","items":[]}
			}`),
		},
		{
			name: "available",
			evidence: json.RawMessage(`{
				"openclarion_available_diagnosis_tools":{
					"usage":"copy",
					"items":[{
						"template_id":7,
						"name":"Current alerts",
						"alert_source_profile_id":3,
						"alert_source_name":"Primary Alertmanager",
						"alert_source_kind":"alertmanager",
						"snapshot_source_scope":"matched",
						"tool":"active_alerts",
						"default_limit":5,
						"evidence_request_example":{
							"template_id":7,
							"alert_source_profile_id":3,
							"tool":"active_alerts",
							"reason":"Collect bounded evidence with Current alerts.",
							"limit":5
						}
					}]
				}
			}`),
			want: true,
		},
		{
			name: "unknown catalog field",
			evidence: json.RawMessage(`{
				"openclarion_available_diagnosis_tools":{"usage":"copy","items":[],"extra":true}
			}`),
			wantErr: "unknown field",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evidenceHasExecutableDiagnosisTools(tc.evidence)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("available = %t, error = %v, want %t", got, err, tc.want)
			}
		})
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

func TestValidateDiagnosisResponsePreservesLargeNumericIDs(t *testing.T) {
	const largeTemplateID int64 = 9007199254740993
	const largeProfileID int64 = 9007199254740995
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
			"message":"Collect the configured metric.",
			"evidence_requests":[{
				"template_id":9007199254740993,
				"alert_source_profile_id":9007199254740995,
				"tool":"metric_query",
				"reason":"Need the configured metric."
			}],
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
	parsed, ok := accepted.Parsed.(diagnosisroom.TurnOutput)
	if !ok || len(parsed.EvidenceRequests) != 1 {
		t.Fatalf("accepted parsed output = %#v", accepted.Parsed)
	}
	request := parsed.EvidenceRequests[0]
	if request.TemplateID != largeTemplateID || request.AlertSourceProfileID != largeProfileID {
		t.Fatalf("large IDs changed: template=%d profile=%d", request.TemplateID, request.AlertSourceProfileID)
	}
	for _, want := range []string{
		`"template_id":9007199254740993`,
		`"alert_source_profile_id":9007199254740995`,
	} {
		if !strings.Contains(string(accepted.Content), want) {
			t.Fatalf("normalized content lost %s: %s", want, accepted.Content)
		}
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
