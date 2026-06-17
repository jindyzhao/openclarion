package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/providers/container/fake"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
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
