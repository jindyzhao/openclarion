package temporal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const diagnosisRoomAgentName = "diagnosis-assistant"

// DiagnosisTurnActivityInput is the workflow-to-activity payload for one
// stateless M5 sandbox invocation.
type DiagnosisTurnActivityInput struct {
	SessionID         string
	DiagnosisTaskID   int64
	MessageID         string
	UserSequence      int
	AssistantSequence int
	ActorSubject      string
	Evidence          json.RawMessage
	Conversation      []diagnosisroom.ConversationTurn
	Message           string
	Policy            diagnosisroom.Policy
}

// DiagnosisTurnActivityResult is the schema-validated assistant response
// returned from the sandbox activity to the Update handler.
type DiagnosisTurnActivityResult struct {
	InvocationID        string
	AssistantMessageID  string
	AssistantSequence   int
	AssistantMessage    string
	Output              diagnosisroom.TurnOutput
	RawOutput           json.RawMessage
	RuntimeID           string
	StartedAt           time.Time
	FinishedAt          time.Time
	RequiresHumanReview bool
	Confidence          string
	Insight             diagnosisroom.ConsultationInsight
}

// RunDiagnosisTurn calls the configured ContainerProvider once, validates the
// sandbox output.json contract, and returns only schema-accepted assistant
// content to the workflow.
func (a *Activities) RunDiagnosisTurn(ctx context.Context, req DiagnosisTurnActivityInput) (DiagnosisTurnActivityResult, error) {
	if a.containerProvider == nil {
		return DiagnosisTurnActivityResult{}, temporalsdk.NewNonRetryableApplicationError(
			"run-diagnosis-turn: container provider is not configured", errTypeInvalidInput, nil)
	}
	policy := diagnosisRoomPolicyOrDefault(req.Policy)
	if err := validateDiagnosisTurnActivityInput(policy, req); err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(err, "run-diagnosis-turn input")
	}

	containerReq, err := buildDiagnosisTurnContainerRequest(policy, req)
	if err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(err, "run-diagnosis-turn request")
	}
	result, err := a.containerProvider.Run(ctx, containerReq)
	if err != nil {
		return DiagnosisTurnActivityResult{}, fmt.Errorf("run-diagnosis-turn container: %w", err)
	}
	if err := ports.ValidateContainerRunResult(containerReq, result); err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(
			fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err),
			"run-diagnosis-turn result",
		)
	}

	output, err := diagnosisroom.ParseTurnOutput(result.Output)
	if err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(
			fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err),
			"run-diagnosis-turn output",
		)
	}
	return DiagnosisTurnActivityResult{
		InvocationID:        result.InvocationID,
		AssistantMessageID:  assistantMessageID(req.MessageID),
		AssistantSequence:   req.AssistantSequence,
		AssistantMessage:    output.Message,
		Output:              output,
		RawOutput:           cloneRawMessage(result.Output),
		RuntimeID:           result.RuntimeID,
		StartedAt:           result.StartedAt,
		FinishedAt:          result.FinishedAt,
		RequiresHumanReview: output.RequiresHumanReview,
		Confidence:          output.Confidence,
		Insight:             output.Insight(),
	}, nil
}

func validateDiagnosisTurnActivityInput(policy diagnosisroom.Policy, req DiagnosisTurnActivityInput) error {
	if err := diagnosisroom.ValidatePolicy(policy); err != nil {
		return err
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("diagnosis turn: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("diagnosis turn: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.MessageID) == "" {
		return fmt.Errorf("diagnosis turn: message_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.MessageID) != req.MessageID {
		return fmt.Errorf("diagnosis turn: message_id must not contain leading or trailing whitespace: %w", domain.ErrInvariantViolation)
	}
	if req.UserSequence <= 0 {
		return fmt.Errorf("diagnosis turn: user_sequence must be > 0: %w", domain.ErrInvariantViolation)
	}
	if req.AssistantSequence != req.UserSequence+1 {
		return fmt.Errorf("diagnosis turn: assistant_sequence must equal user_sequence + 1: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.ActorSubject) == "" {
		return fmt.Errorf("diagnosis turn: actor_subject must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("diagnosis turn: message must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len([]byte(req.Message)) > policy.MaxMessageBytes {
		return fmt.Errorf("diagnosis turn: message is %d bytes, max %d: %w", len([]byte(req.Message)), policy.MaxMessageBytes, domain.ErrInvariantViolation)
	}
	if match, blocked := diagnosisroom.MatchUnsafeInstruction(policy, req.Message); blocked {
		return fmt.Errorf("diagnosis turn: message matches unsafe denylist term %q: %w", match, domain.ErrInvariantViolation)
	}
	contextBytes, err := diagnosisroom.MountContextBytes(req.Evidence, req.Conversation, req.Message)
	if err != nil {
		return err
	}
	if contextBytes > policy.ContextBytes {
		return fmt.Errorf("diagnosis turn: mounted context is %d bytes, max %d: %w", contextBytes, policy.ContextBytes, domain.ErrInvariantViolation)
	}
	return nil
}

func buildDiagnosisTurnContainerRequest(policy diagnosisroom.Policy, req DiagnosisTurnActivityInput) (ports.ContainerRunRequest, error) {
	conversationRaw, err := json.Marshal(req.Conversation)
	if err != nil {
		return ports.ContainerRunRequest{}, fmt.Errorf("marshal conversation: %w", err)
	}
	messageRaw, err := json.Marshal(diagnosisroom.ConversationTurn{
		Role:    "user",
		Content: strings.TrimSpace(req.Message),
	})
	if err != nil {
		return ports.ContainerRunRequest{}, fmt.Errorf("marshal message: %w", err)
	}
	out := ports.ContainerRunRequest{
		InvocationID: diagnosisTurnInvocationID(req.SessionID, req.MessageID, req.DiagnosisTaskID),
		AgentName:    diagnosisRoomAgentName,
		Evidence:     cloneRawMessage(req.Evidence),
		Conversation: conversationRaw,
		Message:      messageRaw,
		Timeout:      policy.TurnTimeout,
		OutputMax:    ports.DefaultContainerOutputBytes,
		Network: ports.ContainerNetworkPolicy{
			Mode: ports.ContainerNetworkNone,
		},
		Metadata: map[string]string{
			"session_id":         req.SessionID,
			"diagnosis_task_id":  strconv.FormatInt(req.DiagnosisTaskID, 10),
			"message_id":         req.MessageID,
			"actor_subject":      req.ActorSubject,
			"user_sequence":      strconv.Itoa(req.UserSequence),
			"assistant_sequence": strconv.Itoa(req.AssistantSequence),
			"schema_id":          diagnosisroom.TurnOutputSchemaID,
		},
	}
	if err := out.Validate(); err != nil {
		return ports.ContainerRunRequest{}, fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err)
	}
	return out, nil
}

func diagnosisTurnInvocationID(sessionID, messageID string, taskID int64) string {
	sum := sha256.Sum256([]byte(sessionID + "\x00" + messageID))
	return "diagnosis-room/task-" + strconv.FormatInt(taskID, 10) + "/msg-" + hex.EncodeToString(sum[:])[:24]
}

func assistantMessageID(messageID string) string {
	return strings.TrimSpace(messageID) + "/assistant"
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
