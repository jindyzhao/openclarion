package temporal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/diagnosisquery"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosiscontext"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisRoomAgentName                  = "diagnosis-assistant"
	diagnosisToolSnapshotSourceScopeMatched = "matched"
)

// DiagnosisTurnActivityInput is the workflow-to-activity payload for one
// stateless M5 sandbox invocation.
type DiagnosisTurnActivityInput struct {
	SessionID            string
	DiagnosisTaskID      int64
	MessageID            string
	UserSequence         int
	AssistantSequence    int
	ActorSubject         string
	Evidence             json.RawMessage
	Conversation         []diagnosisroom.ConversationTurn
	Message              string
	SupplementalEvidence *DiagnosisRoomSupplementalEvidence
	Policy               diagnosisroom.Policy
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

	containerReq, err := buildDiagnosisTurnContainerRequest(
		policy,
		req,
		a.diagnosisContainerNetworkPolicy(),
		a.diagnosisContainerCredentials(policy.TurnTimeout),
	)
	if err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(err, "run-diagnosis-turn request")
	}
	result, err := a.containerProvider.Run(ctx, containerReq)
	if err != nil {
		var exitErr *ports.ContainerExitError
		if errors.As(err, &exitErr) {
			if retryableDiagnosisTurnContainerExit(exitErr) {
				return DiagnosisTurnActivityResult{}, fmt.Errorf("run-diagnosis-turn container: %w", err)
			}
			return DiagnosisTurnActivityResult{}, temporalsdk.NewNonRetryableApplicationError(
				fmt.Sprintf("run-diagnosis-turn container: %v", err), errTypeRuntimeFailure, err)
		}
		return DiagnosisTurnActivityResult{}, fmt.Errorf("run-diagnosis-turn container: %w", err)
	}
	if err := ports.ValidateContainerRunResult(containerReq, result); err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(
			fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err),
			"run-diagnosis-turn result",
		)
	}

	output, rawOutput, err := parseDiagnosisTurnActivityOutput(result.Output, req)
	if err != nil {
		return DiagnosisTurnActivityResult{}, mapActivityError(
			fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err),
			"run-diagnosis-turn output",
		)
	}
	if enriched, changed := enrichDiagnosisTurnOutputEvidenceRequests(output, req.Evidence); changed {
		normalized, err := json.Marshal(enriched)
		if err != nil {
			return DiagnosisTurnActivityResult{}, mapActivityError(
				fmt.Errorf("%w: marshal enriched diagnosis turn output: %w", domain.ErrInvariantViolation, err),
				"run-diagnosis-turn output",
			)
		}
		output = enriched
		rawOutput = normalized
	}
	return DiagnosisTurnActivityResult{
		InvocationID:        result.InvocationID,
		AssistantMessageID:  assistantMessageID(req.MessageID),
		AssistantSequence:   req.AssistantSequence,
		AssistantMessage:    output.Message,
		Output:              output,
		RawOutput:           rawOutput,
		RuntimeID:           result.RuntimeID,
		StartedAt:           result.StartedAt,
		FinishedAt:          result.FinishedAt,
		RequiresHumanReview: output.RequiresHumanReview,
		Confidence:          output.Confidence,
		Insight:             output.Insight(),
	}, nil
}

func parseDiagnosisTurnActivityOutput(
	raw json.RawMessage,
	req DiagnosisTurnActivityInput,
) (diagnosisroom.TurnOutput, json.RawMessage, error) {
	output, err := diagnosisroom.ParseTurnOutput(raw)
	if err == nil {
		return output, cloneRawMessage(raw), nil
	}
	repaired, ok := repairSupplementalResidualBoundaryOutput(raw, req.SupplementalEvidence, err)
	if !ok {
		return diagnosisroom.TurnOutput{}, nil, err
	}
	output, repairErr := diagnosisroom.ParseTurnOutput(repaired)
	if repairErr != nil {
		return diagnosisroom.TurnOutput{}, nil, err
	}
	return output, repaired, nil
}

func repairSupplementalResidualBoundaryOutput(
	raw json.RawMessage,
	supplemental *DiagnosisRoomSupplementalEvidence,
	parseErr error,
) (json.RawMessage, bool) {
	if supplemental == nil ||
		parseErr == nil ||
		!strings.Contains(parseErr.Error(), "low-confidence or evidence-seeking output must include") ||
		!diagnosisRoomSupplementalEvidenceAcceptsResidualBoundary(supplemental) {
		return nil, false
	}
	var output diagnosisroom.TurnOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, false
	}
	confidence := strings.TrimSpace(output.Confidence)
	conclusionStatus := strings.TrimSpace(output.ConclusionStatus)
	needsImprovementPath := confidence == "low" || conclusionStatus == "needs_evidence"
	hasImprovementPath := len(output.EvidenceRequests) > 0 ||
		len(output.MissingEvidenceRequests) > 0 ||
		len(output.EvidenceCollectionSuggestions) > 0
	if !needsImprovementPath || hasImprovementPath {
		return nil, false
	}
	output.RequiresHumanReview = true
	output.ConclusionStatus = "ready_for_review"
	if confidence == "low" {
		output.Confidence = "medium"
	}
	if strings.TrimSpace(output.ConfidenceRationale) == "" {
		output.ConfidenceRationale = "Operator supplemental evidence states the requested non-executable artifact is unavailable and accepts residual uncertainty; the diagnosis is ready for bounded human review."
	}
	normalized, err := json.Marshal(output)
	if err != nil {
		return nil, false
	}
	return normalized, true
}

func diagnosisRoomSupplementalEvidenceAcceptsResidualBoundary(in *DiagnosisRoomSupplementalEvidence) bool {
	if in == nil || strings.TrimSpace(in.Evidence) == "" {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		in.Label,
		in.Detail,
		in.Evidence,
	}, " "))
	acceptsResidual := strings.Contains(text, "residual uncertainty") &&
		(strings.Contains(text, "accept") || strings.Contains(text, "accepted"))
	unavailableArtifact := strings.Contains(text, "not available") ||
		strings.Contains(text, "unavailable") ||
		strings.Contains(text, "cannot provide") ||
		strings.Contains(text, "can't provide") ||
		strings.Contains(text, "not provided")
	return acceptsResidual && unavailableArtifact
}

func retryableDiagnosisTurnContainerExit(err *ports.ContainerExitError) bool {
	if err == nil {
		return false
	}
	diagnostic := strings.ToLower(strings.TrimSpace(err.Diagnostic))
	if diagnostic == "" {
		return false
	}
	if strings.Contains(diagnostic, "diagnosis assistant llm validation failed") ||
		strings.Contains(diagnostic, "llm retry failed") ||
		strings.Contains(diagnostic, "openai llm:") {
		return transientLLMFailureText(diagnostic)
	}
	return false
}

func transientLLMFailureText(text string) bool {
	switch {
	case strings.Contains(text, "context deadline exceeded"):
		return true
	case strings.Contains(text, "deadline exceeded"):
		return true
	case strings.Contains(text, "i/o timeout"):
		return true
	case strings.Contains(text, "tls handshake timeout"):
		return true
	case strings.Contains(text, "connection reset"):
		return true
	case strings.Contains(text, "connection refused"):
		return true
	case strings.Contains(text, "temporary failure"):
		return true
	default:
		return false
	}
}

func enrichDiagnosisTurnOutputEvidenceRequests(
	output diagnosisroom.TurnOutput,
	evidence json.RawMessage,
) (diagnosisroom.TurnOutput, bool) {
	tools := diagnosisToolCatalogFromEvidence(evidence)
	if len(tools) == 0 || len(output.EvidenceRequests) == 0 {
		return output, false
	}
	preferredSourceProfiles := diagnosisEvidenceSourceProfileIDs(evidence)
	changed := false
	requests := append([]diagnosisroom.EvidenceRequest(nil), output.EvidenceRequests...)
	for i, req := range requests {
		enriched, ok := enrichDiagnosisEvidenceRequest(req, tools, preferredSourceProfiles)
		if ok {
			requests[i] = enriched
			changed = true
		}
	}
	if !changed {
		return output, false
	}
	output.EvidenceRequests = requests
	return output, true
}

func diagnosisToolCatalogFromEvidence(evidence json.RawMessage) []diagnosiscontext.AvailableDiagnosisTool {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &top); err != nil {
		return nil
	}
	raw, ok := top[diagnosiscontext.AvailableDiagnosisToolsKey]
	if !ok {
		return nil
	}
	var catalog struct {
		Items []diagnosiscontext.AvailableDiagnosisTool `json:"items"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil
	}
	return catalog.Items
}

func diagnosisEvidenceSourceProfileIDs(evidence json.RawMessage) map[int64]struct{} {
	var snapshot struct {
		Events []struct {
			AlertSourceProfileID int64 `json:"alert_source_profile_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(evidence, &snapshot); err != nil {
		return nil
	}
	if len(snapshot.Events) == 0 {
		return nil
	}
	out := make(map[int64]struct{})
	for _, event := range snapshot.Events {
		if event.AlertSourceProfileID > 0 {
			out[event.AlertSourceProfileID] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func enrichDiagnosisEvidenceRequest(
	req diagnosisroom.EvidenceRequest,
	tools []diagnosiscontext.AvailableDiagnosisTool,
	preferredSourceProfiles map[int64]struct{},
) (diagnosisroom.EvidenceRequest, bool) {
	if req.TemplateID > 0 && req.AlertSourceProfileID > 0 {
		return req, false
	}
	matches := make([]diagnosiscontext.AvailableDiagnosisTool, 0, 1)
	for _, tool := range tools {
		if !diagnosisToolCatalogItemMatchesRequest(tool, req) {
			continue
		}
		matches = append(matches, tool)
	}
	if len(matches) > 1 {
		matches = diagnosisToolMatchesForEvidenceSource(matches, preferredSourceProfiles)
	}
	if len(matches) != 1 {
		return req, false
	}
	match := matches[0]
	changed := false
	if req.TemplateID == 0 {
		req.TemplateID = match.TemplateID
		changed = true
	}
	if req.AlertSourceProfileID == 0 {
		req.AlertSourceProfileID = match.AlertSourceProfileID
		changed = true
	}
	if req.Limit == 0 && match.DefaultLimit > 0 {
		req.Limit = match.DefaultLimit
		changed = true
	}
	if req.Tool == domain.DiagnosisToolKindMetricRangeQuery {
		if req.WindowSeconds == 0 && match.DefaultWindowSeconds > 0 {
			req.WindowSeconds = match.DefaultWindowSeconds
			changed = true
		}
		if req.StepSeconds == 0 && match.DefaultStepSeconds > 0 {
			req.StepSeconds = match.DefaultStepSeconds
			changed = true
		}
	}
	return req, changed
}

func diagnosisToolMatchesForEvidenceSource(
	matches []diagnosiscontext.AvailableDiagnosisTool,
	preferredSourceProfiles map[int64]struct{},
) []diagnosiscontext.AvailableDiagnosisTool {
	if len(matches) <= 1 {
		return matches
	}
	if len(preferredSourceProfiles) > 0 {
		filtered := make([]diagnosiscontext.AvailableDiagnosisTool, 0, len(matches))
		for _, match := range matches {
			if _, ok := preferredSourceProfiles[match.AlertSourceProfileID]; ok {
				filtered = append(filtered, match)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}
	filtered := make([]diagnosiscontext.AvailableDiagnosisTool, 0, len(matches))
	for _, match := range matches {
		if match.SnapshotSourceScope == diagnosisToolSnapshotSourceScopeMatched {
			filtered = append(filtered, match)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return matches
}

func diagnosisToolCatalogItemMatchesRequest(
	tool diagnosiscontext.AvailableDiagnosisTool,
	req diagnosisroom.EvidenceRequest,
) bool {
	if tool.Tool != string(req.Tool) {
		return false
	}
	if req.TemplateID > 0 && tool.TemplateID != req.TemplateID {
		return false
	}
	if req.AlertSourceProfileID > 0 && tool.AlertSourceProfileID != req.AlertSourceProfileID {
		return false
	}
	switch req.Tool {
	case domain.DiagnosisToolKindMetricQuery, domain.DiagnosisToolKindMetricRangeQuery:
		if !diagnosisToolCatalogQueryMatchesRequest(tool, req.Query) {
			return false
		}
	}
	return true
}

func diagnosisToolCatalogQueryMatchesRequest(
	tool diagnosiscontext.AvailableDiagnosisTool,
	query string,
) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return false
	}
	if strings.TrimSpace(tool.EvidenceRequest.Query) == query {
		return true
	}
	return diagnosisquery.MatchesTemplate(tool.QueryTemplate, query)
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

func buildDiagnosisTurnContainerRequest(
	policy diagnosisroom.Policy,
	req DiagnosisTurnActivityInput,
	network ports.ContainerNetworkPolicy,
	credentials []ports.ContainerCredential,
) (ports.ContainerRunRequest, error) {
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
		Network:      cloneContainerNetworkPolicy(network),
		Credentials:  cloneContainerCredentials(credentials),
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

func (a *Activities) diagnosisContainerNetworkPolicy() ports.ContainerNetworkPolicy {
	if a == nil {
		return ports.ContainerNetworkPolicy{Mode: ports.ContainerNetworkNone}
	}
	network := cloneContainerNetworkPolicy(a.containerNetwork)
	if network.Mode == "" {
		return ports.ContainerNetworkPolicy{Mode: ports.ContainerNetworkNone}
	}
	return network
}

func (a *Activities) diagnosisContainerCredentials(timeout time.Duration) []ports.ContainerCredential {
	if a == nil || len(a.containerCredentials) == 0 {
		return nil
	}
	if timeout == 0 {
		timeout = ports.DefaultContainerRunTimeout
	}
	expiresAt := time.Now().UTC().Add(timeout)
	out := make([]ports.ContainerCredential, 0, len(a.containerCredentials))
	for _, credential := range a.containerCredentials {
		out = append(out, ports.ContainerCredential{
			Name:      credential.Name,
			Value:     credential.Value,
			ExpiresAt: expiresAt,
		})
	}
	return out
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

func cloneContainerCredentialTemplates(in []ContainerCredentialTemplate) []ContainerCredentialTemplate {
	if in == nil {
		return nil
	}
	out := make([]ContainerCredentialTemplate, len(in))
	copy(out, in)
	return out
}

func cloneContainerCredentials(in []ports.ContainerCredential) []ports.ContainerCredential {
	if in == nil {
		return nil
	}
	out := make([]ports.ContainerCredential, len(in))
	copy(out, in)
	return out
}

func cloneContainerNetworkPolicy(in ports.ContainerNetworkPolicy) ports.ContainerNetworkPolicy {
	return ports.ContainerNetworkPolicy{
		Mode:          in.Mode,
		AllowedEgress: cloneStrings(in.AllowedEgress),
	}
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
