package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/providers/container/fake"
	embeddingfake "github.com/openclarion/openclarion/internal/providers/embedding/fake"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/retrieval"
)

func TestRunDiagnosisTurn_CallsContainerAndParsesOutput(t *testing.T) {
	started := time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	req := validDiagnosisTurnActivityInput()
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	rawOutput := json.RawMessage(`{
		"schema_version": "diagnosis_turn.v1",
		"message": "CPU saturation is concentrated on api-1.",
		"findings": ["api-1 CPU exceeded threshold"],
		"recommended_actions": ["Inspect recent deployment"],
		"evidence_requests": [{
			"tool": "metric_query",
			"reason": "Need current CPU pressure.",
			"query": "avg(rate(container_cpu_usage_seconds_total[5m]))",
			"limit": 3
		}],
		"confidence": "high",
		"requires_human_review": true,
		"confidence_rationale": "CPU evidence is strong, but restart data is missing.",
		"missing_evidence_requests": [{
			"label": "Restart cause",
			"detail": "Inspect previous pod logs before finalizing.",
			"priority": "medium"
		}],
		"conclusion_status": "needs_evidence"
	}`)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output:       rawOutput,
				ExitCode:     0,
				StartedAt:    started,
				FinishedAt:   finished,
				RuntimeID:    "container-1",
			},
		}},
	})

	activities := NewActivities(nil, WithContainerProvider(provider))
	got, err := activities.RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if got.InvocationID != invocationID ||
		got.AssistantMessageID != "msg-1/assistant" ||
		got.AssistantSequence != 4 ||
		got.AssistantMessage != "CPU saturation is concentrated on api-1." ||
		got.Confidence != "high" ||
		!got.RequiresHumanReview ||
		got.RuntimeID != "container-1" {
		t.Fatalf("result = %+v", got)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].Tool != "metric_query" ||
		got.Output.EvidenceRequests[0].Query != "avg(rate(container_cpu_usage_seconds_total[5m]))" {
		t.Fatalf("evidence requests = %+v", got.Output.EvidenceRequests)
	}
	if got.Insight.ConfidenceRationale != "CPU evidence is strong, but restart data is missing." ||
		len(got.Insight.MissingEvidenceRequests) != 1 ||
		got.Insight.MissingEvidenceRequests[0].Label != "Restart cause" ||
		got.Insight.ConclusionStatus != "needs_evidence" {
		t.Fatalf("insight = %+v", got.Insight)
	}

	recorded := provider.Requests(invocationID)
	if len(recorded) != 1 {
		t.Fatalf("recorded requests len = %d, want 1", len(recorded))
	}
	containerReq := recorded[0]
	if containerReq.AgentName != diagnosisRoomAgentName ||
		containerReq.Timeout != req.Policy.TurnTimeout ||
		containerReq.OutputMax != ports.DefaultContainerOutputBytes ||
		containerReq.Network.Mode != ports.ContainerNetworkNone {
		t.Fatalf("container request = %+v", containerReq)
	}
	if containerReq.Metadata["session_id"] != req.SessionID ||
		containerReq.Metadata["message_id"] != req.MessageID ||
		containerReq.Metadata["schema_id"] != diagnosisroom.TurnOutputSchemaID {
		t.Fatalf("container metadata = %+v", containerReq.Metadata)
	}
	var conversation []diagnosisroom.ConversationTurn
	if err := json.Unmarshal(containerReq.Conversation, &conversation); err != nil {
		t.Fatalf("unmarshal conversation: %v", err)
	}
	if len(conversation) != 2 || conversation[0].Role != "user" || conversation[1].Role != "assistant" {
		t.Fatalf("conversation mount = %+v", conversation)
	}
	var message diagnosisroom.ConversationTurn
	if err := json.Unmarshal(containerReq.Message, &message); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	if message.Role != "user" || message.Content != req.Message {
		t.Fatalf("message mount = %+v", message)
	}
}

func TestRunDiagnosisTurn_PublishesTransientStreamingSnapshots(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.EnableStreaming = true
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	started := time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Stream: []ports.ContainerStreamChunk{
				{GenerationAttempt: 1, Sequence: 1, Delta: "CPU ", Text: "CPU "},
				{GenerationAttempt: 1, Sequence: 2, Delta: "is saturated.", Text: "CPU is saturated."},
				{GenerationAttempt: 2, Reset: true},
				{GenerationAttempt: 2, Sequence: 1, Delta: "CPU is saturated.", Text: "CPU is saturated."},
			},
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version":"diagnosis_turn.v1",
					"message":"CPU is saturated.",
					"confidence":"high",
					"requires_human_review":false,
					"conclusion_status":"final"
				}`),
				ExitCode:   0,
				StartedAt:  started,
				FinishedAt: started.Add(time.Second),
			},
		}},
	})
	sink := &recordingDiagnosisTurnStreamSink{}
	activities := NewActivities(
		nil,
		WithContainerProvider(provider),
		WithDiagnosisTurnStreamSink(sink),
	)

	result, err := activities.RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if result.AssistantMessage != "CPU is saturated." {
		t.Fatalf("AssistantMessage = %q", result.AssistantMessage)
	}
	if len(sink.events) != 5 {
		t.Fatalf("stream events = %#v", sink.events)
	}
	if sink.events[0].Phase != ports.DiagnosisTurnStreamStarted ||
		sink.events[1].AssistantMessage != "CPU " ||
		sink.events[2].AssistantMessage != "CPU is saturated." ||
		sink.events[2].Sequence != 2 ||
		sink.events[3].Phase != ports.DiagnosisTurnStreamReset ||
		sink.events[3].AssistantMessage != "" ||
		sink.events[3].GenerationAttempt != 2 ||
		sink.events[3].Sequence != 0 ||
		sink.events[4].AssistantMessage != "CPU is saturated." {
		t.Fatalf("stream events = %#v", sink.events)
	}
}

func TestRunDiagnosisTurn_MountsBoundedHistoricalReportContext(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.EnableHistoricalRetrieval = true
	baseContextBytes, err := diagnosisroom.MountContextBytes(req.Evidence, req.Conversation, req.Message)
	if err != nil {
		t.Fatalf("MountContextBytes: %v", err)
	}
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	startedAt := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	container := fake.New(map[string][]fake.Result{invocationID: {{Run: ports.ContainerRunResult{
		InvocationID: invocationID,
		AgentName:    diagnosisRoomAgentName,
		Output: json.RawMessage(`{
			"schema_version":"diagnosis_turn.v1",
			"message":"The prior report suggests checking deployment timing against current CPU evidence.",
			"findings":["Current CPU evidence is consistent with a deployment-related hypothesis."],
			"recommended_actions":["Verify the current deployment revision."],
			"evidence_requests":[],
			"confidence":"medium",
			"requires_human_review":true,
			"conclusion_status":"ready_for_review"
		}`),
		ExitCode:   0,
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(time.Second),
	}}}})
	repo := &diagnosisRetrievalReportRepo{rows: []domain.RetrievedChunk{{
		Chunk: domain.RetrievalChunk{
			SourceKind: domain.RetrievalSourceFinalReport,
			SourceID:   41,
			SourceRef:  "final_report:41",
			Content:    `{"title":"Previous CPU incident","executive_summary":"Deployment timing was causal."}`,
		},
		CosineDistance: 0.11,
	}}}
	activities := NewActivities(
		diagnosisRetrievalFactory{uow: diagnosisRetrievalUOW{reports: repo}},
		WithContainerProvider(container),
		WithEmbeddingProvider(embeddingfake.NewDeterministic("diagnosis-rag-test")),
	)

	got, err := activities.RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if got.ContextBytes <= baseContextBytes || got.ContextBytes > req.Policy.ContextBytes || len(got.RetrievalRefs) != 1 || got.RetrievalRefs[0] != "final_report:41" {
		t.Fatalf("context bytes/refs = %d/%v, base=%d max=%d", got.ContextBytes, got.RetrievalRefs, baseContextBytes, req.Policy.ContextBytes)
	}
	recorded := container.Requests(invocationID)
	if len(recorded) != 1 {
		t.Fatalf("container requests = %d, want 1", len(recorded))
	}
	var evidence map[string]json.RawMessage
	if err := json.Unmarshal(recorded[0].Evidence, &evidence); err != nil {
		t.Fatalf("decode mounted evidence: %v", err)
	}
	historical := string(evidence[diagnosiscontext.HistoricalReportContextKey])
	if !strings.Contains(historical, "final_report:41") || !strings.Contains(historical, "Never treat them as current evidence") {
		t.Fatalf("historical context = %s", historical)
	}
}

func TestRunDiagnosisTurn_HistoricalRetrievalFailureFailsOpen(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.EnableHistoricalRetrieval = true
	baseContextBytes, err := diagnosisroom.MountContextBytes(req.Evidence, req.Conversation, req.Message)
	if err != nil {
		t.Fatalf("MountContextBytes: %v", err)
	}
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	startedAt := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	container := fake.New(map[string][]fake.Result{invocationID: {{Run: ports.ContainerRunResult{
		InvocationID: invocationID,
		AgentName:    diagnosisRoomAgentName,
		Output: json.RawMessage(`{
			"schema_version":"diagnosis_turn.v1",
			"message":"Current evidence still supports CPU saturation.",
			"confidence":"medium",
			"requires_human_review":true
		}`),
		ExitCode:   0,
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(time.Second),
	}}}})
	activities := NewActivities(
		diagnosisRetrievalFactory{uow: diagnosisRetrievalUOW{reports: &diagnosisRetrievalReportRepo{}}},
		WithContainerProvider(container),
		WithEmbeddingProvider(embeddingfake.New("diagnosis-rag-test", nil)),
	)

	got, err := activities.RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if got.ContextBytes != baseContextBytes || len(got.RetrievalRefs) != 0 {
		t.Fatalf("context bytes/refs = %d/%v, want %d/empty", got.ContextBytes, got.RetrievalRefs, baseContextBytes)
	}
	recorded := container.Requests(invocationID)
	if len(recorded) != 1 || strings.Contains(string(recorded[0].Evidence), diagnosiscontext.HistoricalReportContextKey) {
		t.Fatalf("container requests = %#v", recorded)
	}
}

func TestRunDiagnosisTurn_HistoricalRetrievalPropagatesCancellation(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.EnableHistoricalRetrieval = true
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	container := fake.New(nil)
	activities := NewActivities(
		diagnosisRetrievalFactory{uow: diagnosisRetrievalUOW{reports: &diagnosisRetrievalReportRepo{}}},
		WithContainerProvider(container),
		WithEmbeddingProvider(embeddingfake.NewDeterministic("diagnosis-rag-test")),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := activities.RunDiagnosisTurn(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunDiagnosisTurn error = %v, want context.Canceled", err)
	}
	if calls := container.Requests(invocationID); len(calls) != 0 {
		t.Fatalf("container requests = %#v, want none after canceled retrieval", calls)
	}
}

func TestFitDiagnosisHistoricalContextShrinksEncodedCatalog(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	baseContextBytes, err := diagnosisroom.MountContextBytes(req.Evidence, req.Conversation, req.Message)
	if err != nil {
		t.Fatalf("MountContextBytes: %v", err)
	}
	items := make([]retrieval.ContextItem, 4)
	for i := range items {
		items[i] = retrieval.ContextItem{
			SourceRef:      fmt.Sprintf("sub_report:%d", i+1),
			SourceKind:     domain.RetrievalSourceSubReport,
			Content:        strings.Repeat(`"`, domain.RetrievalChunkMaxBytes),
			CosineDistance: 0.2,
		}
	}

	evidence, fitted, mountedBytes, err := fitDiagnosisHistoricalContext(req.Policy, req, items, baseContextBytes)
	if err != nil {
		t.Fatalf("fitDiagnosisHistoricalContext: %v", err)
	}
	if len(fitted) == 0 || mountedBytes <= baseContextBytes || mountedBytes > req.Policy.ContextBytes {
		t.Fatalf("fitted items/context = %d/%d, base=%d max=%d", len(fitted), mountedBytes, baseContextBytes, req.Policy.ContextBytes)
	}
	var mounted map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &mounted); err != nil {
		t.Fatalf("decode fitted evidence: %v", err)
	}
	if len(mounted[diagnosiscontext.HistoricalReportContextKey]) == 0 ||
		len(mounted[diagnosiscontext.HistoricalReportContextKey]) > domain.RetrievalContextMaxBytes {
		t.Fatalf("encoded historical catalog bytes = %d", len(mounted[diagnosiscontext.HistoricalReportContextKey]))
	}
}

func TestRunDiagnosisTurn_InjectsRuntimeCredentialsAndNetwork(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "CPU saturation is concentrated on api-1.",
					"findings": ["api-1 CPU exceeded threshold"],
					"recommended_actions": ["Inspect recent deployment"],
					"evidence_requests": [],
					"confidence": "high",
					"requires_human_review": false
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	activities := NewActivities(nil,
		WithContainerProvider(provider),
		WithContainerCredentials([]ContainerCredentialTemplate{
			{Name: "OPENCLARION_DIAGNOSIS_LLM_BASE_URL", Value: "https://llm.example.invalid/v1"},
			{Name: "OPENCLARION_DIAGNOSIS_LLM_API_KEY", Value: "test-api-key"},
			{Name: "OPENCLARION_DIAGNOSIS_LLM_MODEL", Value: "test-model"},
		}),
		WithContainerNetworkPolicy(ports.ContainerNetworkPolicy{
			Mode:          ports.ContainerNetworkAllowlist,
			AllowedEgress: []string{"llm.example.invalid:443"},
		}),
	)
	if _, err := activities.RunDiagnosisTurn(context.Background(), req); err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}

	recorded := provider.Requests(invocationID)
	if len(recorded) != 1 {
		t.Fatalf("recorded requests len = %d, want 1", len(recorded))
	}
	got := recorded[0]
	if got.Network.Mode != ports.ContainerNetworkAllowlist ||
		len(got.Network.AllowedEgress) != 1 ||
		got.Network.AllowedEgress[0] != "llm.example.invalid:443" {
		t.Fatalf("network = %+v", got.Network)
	}
	if len(got.Credentials) != 3 {
		t.Fatalf("credentials len = %d, want 3", len(got.Credentials))
	}
	wantCredentials := map[string]string{
		"OPENCLARION_DIAGNOSIS_LLM_BASE_URL": "https://llm.example.invalid/v1",
		"OPENCLARION_DIAGNOSIS_LLM_API_KEY":  "test-api-key",
		"OPENCLARION_DIAGNOSIS_LLM_MODEL":    "test-model",
	}
	for _, credential := range got.Credentials {
		if wantCredentials[credential.Name] != credential.Value {
			t.Fatalf("credential %q = %q", credential.Name, credential.Value)
		}
		if credential.ExpiresAt.IsZero() {
			t.Fatalf("credential %q expiry is zero", credential.Name)
		}
	}
}

func TestRunDiagnosisTurn_FillsEvidenceRequestIDsFromCatalog(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.Evidence = json.RawMessage(`{
		"alert": "cascade",
		"` + diagnosiscontext.AvailableDiagnosisToolsKey + `": {
			"items": [{
				"template_id": 7,
				"alert_source_profile_id": 5,
				"alert_source_kind": "prometheus",
				"tool": "metric_query",
				"query_template": "sum(up)",
				"default_limit": 5
			}]
		}
	}`)
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "Need current availability.",
					"evidence_requests": [{
						"tool": "metric_query",
						"query": "sum(up)",
						"reason": "Need current target availability."
					}],
					"confidence": "low",
					"requires_human_review": true,
					"confidence_rationale": "The current evidence is incomplete."
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].TemplateID != 7 ||
		got.Output.EvidenceRequests[0].AlertSourceProfileID != 5 ||
		got.Output.EvidenceRequests[0].Limit != 5 {
		t.Fatalf("evidence requests = %+v", got.Output.EvidenceRequests)
	}
	parsed, err := diagnosisroom.ParseTurnOutput(got.RawOutput)
	if err != nil {
		t.Fatalf("parse enriched raw output: %v", err)
	}
	if parsed.EvidenceRequests[0].TemplateID != 7 || parsed.EvidenceRequests[0].AlertSourceProfileID != 5 {
		t.Fatalf("raw output evidence requests = %+v", parsed.EvidenceRequests)
	}
}

func TestRunDiagnosisTurn_FillsParameterizedMetricIDsFromCatalog(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.Evidence = json.RawMessage(`{
		"alert": "oracle tablespace",
		"` + diagnosiscontext.AvailableDiagnosisToolsKey + `": {
			"items": [{
				"template_id": 11,
				"alert_source_profile_id": 5,
				"alert_source_kind": "prometheus",
				"tool": "metric_query",
				"query_template": "db_tablespace_pctusd{job=\"oracle_exporter\",ORACLE_SID=\"{{label.ORACLE_SID}}\",TABLESPACE=\"{{label.TABLESPACE}}\"}",
				"default_limit": 5,
				"evidence_request_example": {
					"template_id": 11,
					"alert_source_profile_id": 5,
					"tool": "metric_query",
					"reason": "Need current tablespace saturation.",
					"query": "db_tablespace_pctusd{job=\"oracle_exporter\",ORACLE_SID=\"sapprd1\",TABLESPACE=\"PSAPSR3USR\"}",
					"limit": 5
				}
			}]
		}
	}`)
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "Need current tablespace saturation.",
					"evidence_requests": [{
						"tool": "metric_query",
						"query": "db_tablespace_pctusd{job=\"oracle_exporter\",ORACLE_SID=\"sapprd1\",TABLESPACE=\"PSAPSR3USR\"}",
						"reason": "Need current tablespace saturation."
					}],
					"confidence": "low",
					"requires_human_review": true,
					"confidence_rationale": "The current evidence is incomplete."
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].TemplateID != 11 ||
		got.Output.EvidenceRequests[0].AlertSourceProfileID != 5 ||
		got.Output.EvidenceRequests[0].Limit != 5 {
		t.Fatalf("evidence requests = %+v", got.Output.EvidenceRequests)
	}
}

func TestRunDiagnosisTurn_DoesNotGuessAmbiguousCatalogTool(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.Evidence = json.RawMessage(`{
		"alert": "cascade",
		"` + diagnosiscontext.AvailableDiagnosisToolsKey + `": {
			"items": [
				{"template_id": 4, "alert_source_profile_id": 2, "alert_source_kind": "alertmanager", "tool": "active_alerts", "default_limit": 5},
				{"template_id": 5, "alert_source_profile_id": 3, "alert_source_kind": "alertmanager", "tool": "active_alerts", "default_limit": 5}
			]
		}
	}`)
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "Need current active alerts.",
					"evidence_requests": [{
						"tool": "active_alerts",
						"reason": "Need current sibling alerts.",
						"limit": 5
					}],
					"confidence": "low",
					"requires_human_review": true,
					"confidence_rationale": "The current evidence is incomplete."
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].TemplateID != 0 ||
		got.Output.EvidenceRequests[0].AlertSourceProfileID != 0 {
		t.Fatalf("evidence requests = %+v, want unresolved ambiguous request", got.Output.EvidenceRequests)
	}
}

func TestRunDiagnosisTurn_FillsAmbiguousActiveAlertsFromSnapshotSourceProfile(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.Evidence = json.RawMessage(`{
		"schema_version": "m1.evidence_snapshot.v1",
		"events": [{
			"id": 10,
			"source": "alertmanager",
			"alert_source_profile_id": 3
		}],
		"` + diagnosiscontext.AvailableDiagnosisToolsKey + `": {
			"items": [
				{"template_id": 4, "alert_source_profile_id": 2, "alert_source_kind": "alertmanager", "tool": "active_alerts", "default_limit": 5},
				{"template_id": 5, "alert_source_profile_id": 3, "alert_source_kind": "alertmanager", "tool": "active_alerts", "default_limit": 7}
			]
		}
	}`)
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "Need current active alerts.",
					"evidence_requests": [{
						"tool": "active_alerts",
						"reason": "Need current sibling alerts."
					}],
					"confidence": "low",
					"requires_human_review": true,
					"confidence_rationale": "The current evidence is incomplete."
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].TemplateID != 5 ||
		got.Output.EvidenceRequests[0].AlertSourceProfileID != 3 ||
		got.Output.EvidenceRequests[0].Limit != 7 {
		t.Fatalf("evidence requests = %+v, want source-profile resolved active_alerts", got.Output.EvidenceRequests)
	}
}

func TestRunDiagnosisTurn_FillsAmbiguousMetricFromCatalogScope(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.Evidence = json.RawMessage(`{
		"alert": "tablespace capacity",
		"` + diagnosiscontext.AvailableDiagnosisToolsKey + `": {
			"items": [
				{"template_id": 4, "alert_source_profile_id": 2, "alert_source_kind": "prometheus", "snapshot_source_scope": "supplemental", "tool": "metric_query", "query_template": "sum(up)", "default_limit": 5},
				{"template_id": 5, "alert_source_profile_id": 3, "alert_source_kind": "prometheus", "snapshot_source_scope": "matched", "tool": "metric_query", "query_template": "sum(up)", "default_limit": 7}
			]
		}
	}`)
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output: json.RawMessage(`{
					"schema_version": "diagnosis_turn.v1",
					"message": "Need current metric evidence.",
					"evidence_requests": [{
						"tool": "metric_query",
						"query": "sum(up)",
						"reason": "Need current metric evidence."
					}],
					"confidence": "low",
					"requires_human_review": true,
					"confidence_rationale": "The current evidence is incomplete."
				}`),
				ExitCode:   0,
				StartedAt:  time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if len(got.Output.EvidenceRequests) != 1 ||
		got.Output.EvidenceRequests[0].TemplateID != 5 ||
		got.Output.EvidenceRequests[0].AlertSourceProfileID != 3 ||
		got.Output.EvidenceRequests[0].Limit != 7 {
		t.Fatalf("evidence requests = %+v, want catalog-scope resolved metric request", got.Output.EvidenceRequests)
	}
}

func TestCollectDiagnosisEvidence_ReturnsSkippedWhenNotConfigured(t *testing.T) {
	activities := NewActivities(nil)
	got, err := activities.CollectDiagnosisEvidence(context.Background(), CollectDiagnosisEvidenceInput{
		SessionID:       "session-1",
		DiagnosisTaskID: 101,
		Requests: []diagnosisroom.EvidenceRequest{{
			Tool:   "active_alerts",
			Reason: "Need current sibling alerts.",
			Limit:  5,
		}},
	})
	if err != nil {
		t.Fatalf("CollectDiagnosisEvidence: %v", err)
	}
	if len(got.Items) != 1 ||
		got.Items[0].Status != diagnosisevidence.StatusSkipped ||
		got.Items[0].ReasonCode != diagnosisevidence.ReasonProviderUnavailable {
		t.Fatalf("items = %+v", got.Items)
	}
}

func TestRunDiagnosisTurn_RejectsInvalidContainerOutputAsNonRetryable(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output:       json.RawMessage(`{"schema_version":"diagnosis_turn.v1","message":"ok","confidence":"certain","requires_human_review":true}`),
				ExitCode:     0,
				StartedAt:    time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt:   time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	_, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err == nil {
		t.Fatal("RunDiagnosisTurn returned nil error")
	}
	if !strings.Contains(err.Error(), "run-diagnosis-turn output") {
		t.Fatalf("error = %v, want output context", err)
	}
	var appErr *temporalsdk.ApplicationError
	if !errors.As(err, &appErr) || appErr.Type() != errTypeInvariantViolation {
		t.Fatalf("error type = %T/%v, want non-retryable invariant application error", err, err)
	}
}

func TestRunDiagnosisTurn_NormalizesSupplementalResidualBoundaryOutput(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.MessageID = "msg-supplemental"
	req.SupplementalEvidence = &DiagnosisRoomSupplementalEvidence{
		Label:    "DBA storage confirmation",
		Detail:   "Confirm whether DBA can attach the detailed storage expansion ticket.",
		Priority: "high",
		Evidence: strings.Join([]string{
			"The requested operator or DBA artifact is not available in this live validation window.",
			"Operator accepts this as residual uncertainty for review purposes.",
		}, " "),
	}
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output:       json.RawMessage(`{"schema_version":"diagnosis_turn.v1","message":"Bounded diagnosis is ready for operator review.","confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence"}`),
				ExitCode:     0,
				StartedAt:    time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt:   time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	got, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("RunDiagnosisTurn: %v", err)
	}
	if got.Confidence != "medium" ||
		!got.RequiresHumanReview ||
		got.Insight.ConclusionStatus != "ready_for_review" ||
		len(got.Output.EvidenceRequests) != 0 ||
		len(got.Insight.MissingEvidenceRequests) != 0 ||
		len(got.Insight.EvidenceCollectionSuggestions) != 0 {
		t.Fatalf("result = %+v insight=%+v", got.Output, got.Insight)
	}
	parsed, err := diagnosisroom.ParseTurnOutput(got.RawOutput)
	if err != nil {
		t.Fatalf("RawOutput did not remain parseable: %v", err)
	}
	if parsed.Confidence != "medium" || parsed.ConclusionStatus != "ready_for_review" {
		t.Fatalf("parsed raw output = %+v", parsed)
	}
}

func TestRunDiagnosisTurn_DoesNotNormalizeLowConfidenceOutputWithoutResidualBoundary(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	req.MessageID = "msg-supplemental"
	req.SupplementalEvidence = &DiagnosisRoomSupplementalEvidence{
		Label:    "DBA storage confirmation",
		Detail:   "Confirm whether DBA can attach the detailed storage expansion ticket.",
		Priority: "high",
		Evidence: "Operator is still looking for the DBA ticket.",
	}
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Run: ports.ContainerRunResult{
				InvocationID: invocationID,
				AgentName:    diagnosisRoomAgentName,
				Output:       json.RawMessage(`{"schema_version":"diagnosis_turn.v1","message":"Need more evidence.","confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence"}`),
				ExitCode:     0,
				StartedAt:    time.Date(2026, 5, 28, 14, 0, 0, 0, time.UTC),
				FinishedAt:   time.Date(2026, 5, 28, 14, 0, 1, 0, time.UTC),
			},
		}},
	})

	_, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err == nil {
		t.Fatal("RunDiagnosisTurn returned nil error")
	}
	if !strings.Contains(err.Error(), "low-confidence or evidence-seeking output must include") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunDiagnosisTurn_RejectsContainerExitAsNonRetryableRuntimeFailure(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{
			Err: &ports.ContainerExitError{
				RuntimeID:  "runtime-1",
				ExitCode:   1,
				Diagnostic: `stderr_tail="diagnosis assistant output.json was missing"`,
			},
		}},
	})

	_, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err == nil {
		t.Fatal("RunDiagnosisTurn returned nil error")
	}
	if !strings.Contains(err.Error(), "run-diagnosis-turn container") {
		t.Fatalf("error = %v, want container context", err)
	}
	var appErr *temporalsdk.ApplicationError
	if !errors.As(err, &appErr) || appErr.Type() != errTypeRuntimeFailure {
		t.Fatalf("error type = %T/%v, want non-retryable runtime failure application error", err, err)
	}
}

func TestRunDiagnosisTurn_RetriesTransientLLMContainerExit(t *testing.T) {
	req := validDiagnosisTurnActivityInput()
	invocationID := diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID)
	exitErr := &ports.ContainerExitError{
		RuntimeID:  "runtime-1",
		ExitCode:   1,
		Diagnostic: `stderr_tail="[diagnosis-assistant-runner] diagnosis assistant LLM validation failed: llm retry failed: openai llm: post chat completion: context deadline exceeded"`,
	}
	provider := fake.New(map[string][]fake.Result{
		invocationID: {{Err: exitErr}},
	})

	_, err := NewActivities(nil, WithContainerProvider(provider)).RunDiagnosisTurn(context.Background(), req)
	if err == nil {
		t.Fatal("RunDiagnosisTurn returned nil error")
	}
	if !strings.Contains(err.Error(), "run-diagnosis-turn container") {
		t.Fatalf("error = %v, want container context", err)
	}
	var appErr *temporalsdk.ApplicationError
	if errors.As(err, &appErr) {
		t.Fatalf("error type = %T/%v, want retryable non-application error", err, err)
	}
	if !errors.Is(err, exitErr) {
		t.Fatalf("error = %v, want wrapped container exit error", err)
	}
}

func TestRunDiagnosisTurn_RejectsMissingContainerProvider(t *testing.T) {
	_, err := NewActivities(nil).RunDiagnosisTurn(context.Background(), validDiagnosisTurnActivityInput())
	if err == nil {
		t.Fatal("RunDiagnosisTurn returned nil error")
	}
	var appErr *temporalsdk.ApplicationError
	if !errors.As(err, &appErr) || appErr.Type() != errTypeInvalidInput {
		t.Fatalf("error type = %T/%v, want invalid input application error", err, err)
	}
}

func validDiagnosisTurnActivityInput() DiagnosisTurnActivityInput {
	policy := diagnosisroom.DefaultPolicy()
	policy.TurnTimeout = 90 * time.Second
	return DiagnosisTurnActivityInput{
		SessionID:         "session-1",
		DiagnosisTaskID:   1001,
		MessageID:         "msg-1",
		UserSequence:      3,
		AssistantSequence: 4,
		ActorSubject:      "owner-1",
		Evidence:          json.RawMessage(`{"alert":"cpu_saturation","severity":"warning"}`),
		Conversation: []diagnosisroom.ConversationTurn{
			{Role: "user", Content: "What happened?"},
			{Role: "assistant", Content: "CPU is high."},
		},
		Message: "What changed recently?",
		Policy:  policy,
	}
}

type recordingDiagnosisTurnStreamSink struct {
	events []ports.DiagnosisTurnStreamEvent
}

func (s *recordingDiagnosisTurnStreamSink) PublishDiagnosisTurnStream(event ports.DiagnosisTurnStreamEvent) {
	s.events = append(s.events, event)
}

type diagnosisRetrievalFactory struct {
	uow diagnosisRetrievalUOW
}

func (f diagnosisRetrievalFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return f.uow, nil
}

func (f diagnosisRetrievalFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, f.uow)
}

type diagnosisRetrievalUOW struct {
	ports.UnitOfWork
	reports ports.ReportRepository
}

func (u diagnosisRetrievalUOW) Reports() ports.ReportRepository { return u.reports }
func (diagnosisRetrievalUOW) Commit(context.Context) error      { return nil }
func (diagnosisRetrievalUOW) Rollback(context.Context) error    { return nil }

type diagnosisRetrievalReportRepo struct {
	ports.ReportRepository
	rows []domain.RetrievedChunk
}

func (r *diagnosisRetrievalReportRepo) SearchRetrievalChunks(_ context.Context, _ string, _ []float32, _ float64, limit int) ([]domain.RetrievedChunk, error) {
	if limit > len(r.rows) {
		limit = len(r.rows)
	}
	return append([]domain.RetrievedChunk(nil), r.rows[:limit]...), nil
}
