// Command diagnosis_live_convergence_smoke_output validates the JSON proof
// produced by the manual M5 backend-only diagnosis convergence smoke gate.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxProofBytes           int64 = 10 * 1024 * 1024
	maxProofSessionIDBytes        = 128
	maxProofWorkflowIDBytes       = 256
	maxProofRunIDBytes            = 256
	maxProofStageNameBytes        = 128
	maxProofMessageBytes          = 1024
	sha256HexLength               = 64
)

var requiredNotificationProofEvents = []string{
	"diagnosis_room.assistant_turn_notification_sent",
	"diagnosis_room.final_ready_notification_sent",
	"diagnosis_room.close_notification_sent",
}

type smokeOutput struct {
	Passed       bool              `json:"passed"`
	CheckedAt    string            `json:"checked_at"`
	Mode         string            `json:"mode"`
	Request      proofRequest      `json:"request"`
	Stages       []proofStage      `json:"stages"`
	SessionID    string            `json:"session_id"`
	CreatedRoom  *createdRoom      `json:"created_room,omitempty"`
	FinalState   proofState        `json:"final_state"`
	Confirmation confirmationProof `json:"confirmation"`
	Notification notificationProof `json:"notification_proof,omitempty"`
	Error        string            `json:"error,omitempty"`
}

type proofRequest struct {
	ExistingSessionID            *string         `json:"existing_session_id"`
	EvidenceSnapshotID           json.RawMessage `json:"evidence_snapshot_id"`
	NotificationChannelProfileID json.RawMessage `json:"notification_channel_profile_id"`
	RequireNotificationProof     bool            `json:"require_notification_proof"`
	CollectPlannedEvidence       bool            `json:"collect_planned_evidence"`
	SubmitSupplementalEvidence   bool            `json:"submit_supplemental_evidence"`
	ConfirmConclusionRequested   bool            `json:"confirm_conclusion_requested"`
	Mode                         string          `json:"mode"`
}

type proofStage map[string]json.RawMessage

type createdRoom struct {
	SessionID       string `json:"session_id"`
	DiagnosisTaskID int64  `json:"diagnosis_task_id"`
	WorkflowID      string `json:"workflow_id"`
	RunID           string `json:"run_id"`
}

type proofState struct {
	Type                     string `json:"type,omitempty"`
	Code                     string `json:"code,omitempty"`
	Message                  string `json:"message,omitempty"`
	Status                   string `json:"status,omitempty"`
	TurnCount                int    `json:"turn_count,omitempty"`
	InFlight                 bool   `json:"in_flight,omitempty"`
	Confidence               string `json:"confidence,omitempty"`
	RequiresHumanReview      bool   `json:"requires_human_review,omitempty"`
	ConclusionStatus         string `json:"conclusion_status,omitempty"`
	EvidenceRequests         int    `json:"evidence_requests,omitempty"`
	CollectionResults        int    `json:"collection_results,omitempty"`
	Missing                  int    `json:"missing,omitempty"`
	FinalConclusionAvailable bool   `json:"final_conclusion_available,omitempty"`
	CloseReason              string `json:"close_reason,omitempty"`
}

type confirmationProof struct {
	CheckedAt     string     `json:"checked_at"`
	Requested     bool       `json:"requested"`
	FinalState    proofState `json:"final_state"`
	Passed        bool       `json:"passed"`
	SkippedReason string     `json:"skipped_reason,omitempty"`
}

type notificationProof struct {
	CheckedAt     string                   `json:"checked_at"`
	Requested     bool                     `json:"requested"`
	Passed        bool                     `json:"passed"`
	SkippedReason string                   `json:"skipped_reason,omitempty"`
	Entries       []notificationProofEntry `json:"entries,omitempty"`
}

type notificationProofEntry struct {
	EventKind                    string          `json:"event_kind"`
	NotificationChannelProfileID json.RawMessage `json:"notification_channel_profile_id"`
	ProviderStatus               string          `json:"provider_status"`
	ProviderMessageID            string          `json:"provider_message_id,omitempty"`
	AssistantMessageID           string          `json:"assistant_message_id,omitempty"`
	AssistantTurnID              json.RawMessage `json:"assistant_turn_id,omitempty"`
	TurnCount                    json.RawMessage `json:"turn_count,omitempty"`
	ContentKind                  string          `json:"content_kind"`
	ContentSHA256                string          `json:"content_sha256"`
	RecommendedActionCount       json.RawMessage `json:"recommended_action_count,omitempty"`
	EvidenceRequestCount         json.RawMessage `json:"evidence_request_count,omitempty"`
	OccurredAt                   string          `json:"occurred_at"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "[diagnosis-live-convergence-smoke-output] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[diagnosis-live-convergence-smoke-output] OK")
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: diagnosis_live_convergence_smoke_output <output.json>")
	}
	raw, err := readProofFile(filepath.Clean(args[0]))
	if err != nil {
		return err
	}
	var out smokeOutput
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("decode JSON output: %w", err)
	}
	return validate(out)
}

func readProofFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if err := requireRegularFile(clean); err != nil {
		return nil, err
	}
	// #nosec G304,G703 -- this manual smoke checker opens the operator-supplied output JSON path.
	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxProofBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", clean, err)
	}
	if int64(len(raw)) > maxProofBytes {
		return nil, fmt.Errorf("%s exceeds maximum proof size %d bytes", clean, maxProofBytes)
	}
	return raw, nil
}

func requireRegularFile(path string) error {
	clean := filepath.Clean(path)
	info, err := os.Lstat(clean)
	if err != nil {
		return fmt.Errorf("stat %s: %w", clean, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", clean)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", clean)
	}
	return nil
}

func validate(out smokeOutput) error {
	if !out.Passed {
		return fmt.Errorf("passed must be true")
	}
	if out.Error != "" {
		return fmt.Errorf("error must be empty for a passed proof")
	}
	if out.Mode != "direct_ws_convergence" {
		return fmt.Errorf("mode = %q, want direct_ws_convergence", out.Mode)
	}
	if err := validateCheckedAt("checked_at", out.CheckedAt); err != nil {
		return err
	}
	sessionID, err := validateID("session_id", out.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if err := validateRequest(out.Request, sessionID, out.CreatedRoom); err != nil {
		return err
	}
	stageNames, err := validateStages(out.Stages)
	if err != nil {
		return err
	}
	if err := requireStage(stageNames, "initial_state"); err != nil {
		return err
	}
	switch out.Request.Mode {
	case "create_room":
		if err := requireStage(stageNames, "initial_turn_before"); err != nil {
			return err
		}
		if err := requireStage(stageNames, "initial_turn_frame"); err != nil {
			return err
		}
		if err := validateInitialEvidencePath(out.Stages); err != nil {
			return err
		}
	case "existing_session":
		if err := requireStage(stageNames, "using_existing_session"); err != nil {
			return err
		}
	}
	hasCollectStages := stageNames["collecting_evidence"] > 0 ||
		stageNames["collect_turn_before"] > 0 ||
		stageNames["collect_turn_frame"] > 0
	if hasCollectStages {
		if err := requireStage(stageNames, "collecting_evidence"); err != nil {
			return err
		}
		if err := requireStage(stageNames, "collect_turn_before"); err != nil {
			return err
		}
		if err := requireStage(stageNames, "collect_turn_frame"); err != nil {
			return err
		}
	} else if !stagesHavePositiveInt(out.Stages, "collection_results") {
		return fmt.Errorf("stages must include collect_evidence stages or existing collection_results proof")
	}
	skippedSupplemental := stageNames["supplemental_skipped_ready_for_review"] > 0
	if skippedSupplemental {
		if stageNames["submitting_supplemental_boundary"] > 0 ||
			stageNames["supplemental_turn_before"] > 0 ||
			stageNames["supplemental_turn_frame"] > 0 {
			return fmt.Errorf("supplemental skip proof must not include supplemental turn stages")
		}
	} else {
		if err := requireStage(stageNames, "submitting_supplemental_boundary"); err != nil {
			return err
		}
		if err := requireStage(stageNames, "supplemental_turn_before"); err != nil {
			return err
		}
		if err := requireStage(stageNames, "supplemental_turn_frame"); err != nil {
			return err
		}
	}
	if out.Request.ConfirmConclusionRequested {
		if err := requireStage(stageNames, "confirm_frame"); err != nil {
			return err
		}
	} else if stageNames["confirm_frame"] > 0 {
		return fmt.Errorf(`stages must not include "confirm_frame" when confirmation is not requested`)
	}
	if err := requireStage(stageNames, "proof_written"); err != nil {
		return err
	}
	if err := validateFinalReadyState(out.FinalState, out.Request.ConfirmConclusionRequested, skippedSupplemental); err != nil {
		return err
	}
	if out.Request.ConfirmConclusionRequested {
		if err := validateConfirmation(out.Confirmation, out.FinalState.TurnCount, skippedSupplemental); err != nil {
			return err
		}
	} else {
		if err := validateSkippedConfirmation(out.Confirmation); err != nil {
			return err
		}
	}
	notificationChannelID, notificationRequested, err := optionalPositiveInt64(out.Request.NotificationChannelProfileID)
	if err != nil {
		return fmt.Errorf("request.notification_channel_profile_id: %w", err)
	}
	if out.Request.RequireNotificationProof && !notificationRequested {
		return fmt.Errorf("request.require_notification_proof requires request.notification_channel_profile_id")
	}
	return validateNotificationProof(out.Notification, notificationChannelID, out.Request.RequireNotificationProof || notificationRequested)
}

func validateRequest(req proofRequest, sessionID string, room *createdRoom) error {
	switch req.Mode {
	case "create_room":
		if room == nil {
			return fmt.Errorf("request.mode create_room requires created_room")
		}
		if err := validateCreatedRoom(*room, sessionID); err != nil {
			return err
		}
		if req.ExistingSessionID != nil {
			return fmt.Errorf("request.existing_session_id must be null for create_room")
		}
		if _, ok, err := optionalPositiveInt64(req.EvidenceSnapshotID); err != nil {
			return fmt.Errorf("request.evidence_snapshot_id: %w", err)
		} else if !ok {
			return fmt.Errorf("request.evidence_snapshot_id is required for create_room")
		}
		if !req.ConfirmConclusionRequested {
			return fmt.Errorf("request.confirm_conclusion_requested must be true for create_room")
		}
	case "existing_session":
		if room != nil {
			return fmt.Errorf("request.mode existing_session must not include created_room")
		}
		if req.ExistingSessionID == nil {
			return fmt.Errorf("request.existing_session_id is required for existing_session")
		}
		existing, err := validateID("request.existing_session_id", *req.ExistingSessionID, maxProofSessionIDBytes)
		if err != nil {
			return err
		}
		if existing != sessionID {
			return fmt.Errorf("request.existing_session_id must match session_id")
		}
	default:
		return fmt.Errorf("request.mode = %q, want create_room or existing_session", req.Mode)
	}
	if !req.CollectPlannedEvidence {
		return fmt.Errorf("request.collect_planned_evidence must be true")
	}
	if !req.SubmitSupplementalEvidence {
		return fmt.Errorf("request.submit_supplemental_evidence must be true")
	}
	if _, ok, err := optionalPositiveInt64(req.NotificationChannelProfileID); err != nil {
		return fmt.Errorf("request.notification_channel_profile_id: %w", err)
	} else if req.RequireNotificationProof && !ok {
		return fmt.Errorf("request.require_notification_proof requires request.notification_channel_profile_id")
	}
	return nil
}

func validateCreatedRoom(room createdRoom, sessionID string) error {
	roomSessionID, err := validateID("created_room.session_id", room.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if roomSessionID != sessionID {
		return fmt.Errorf("created_room.session_id must match session_id")
	}
	if room.DiagnosisTaskID <= 0 {
		return fmt.Errorf("created_room.diagnosis_task_id must be > 0")
	}
	if _, err := validateID("created_room.workflow_id", room.WorkflowID, maxProofWorkflowIDBytes); err != nil {
		return err
	}
	if _, err := validateID("created_room.run_id", room.RunID, maxProofRunIDBytes); err != nil {
		return err
	}
	return nil
}

func validateStages(stages []proofStage) (map[string]int, error) {
	if len(stages) == 0 {
		return nil, fmt.Errorf("stages must be non-empty")
	}
	names := make(map[string]int, len(stages))
	for i, stage := range stages {
		name, err := stageString(stage, "name")
		if err != nil {
			return nil, fmt.Errorf("stages[%d].name: %w", i, err)
		}
		if _, err := validateBoundedCleanString("stage.name", name, maxProofStageNameBytes); err != nil {
			return nil, fmt.Errorf("stages[%d].name: %w", i, err)
		}
		at, err := stageString(stage, "at")
		if err != nil {
			return nil, fmt.Errorf("stages[%d].at: %w", i, err)
		}
		if err := validateCheckedAt("stage.at", at); err != nil {
			return nil, fmt.Errorf("stages[%d].at: %w", i, err)
		}
		if raw, ok := stage["message"]; ok {
			var message string
			if err := json.Unmarshal(raw, &message); err != nil {
				return nil, fmt.Errorf("stages[%d].message must be a string: %w", i, err)
			}
			if _, err := validateBoundedCleanString("stage.message", message, maxProofMessageBytes); err != nil {
				return nil, fmt.Errorf("stages[%d].message: %w", i, err)
			}
		}
		names[name]++
	}
	return names, nil
}

func stageString(stage proofStage, key string) (string, error) {
	raw, ok := stage[key]
	if !ok {
		return "", fmt.Errorf("is required")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("must be a string: %w", err)
	}
	return value, nil
}

func requireStage(names map[string]int, name string) error {
	if names[name] == 0 {
		return fmt.Errorf("stages must include %q", name)
	}
	return nil
}

func stagesHavePositiveInt(stages []proofStage, key string) bool {
	for _, stage := range stages {
		raw, ok := stage[key]
		if !ok {
			continue
		}
		var value int
		if err := json.Unmarshal(raw, &value); err == nil && value > 0 {
			return true
		}
	}
	return false
}

func validateInitialEvidencePath(stages []proofStage) error {
	found := false
	var details []string
	for i, stage := range stages {
		name, err := stageString(stage, "name")
		if err != nil {
			return fmt.Errorf("stages[%d].name: %w", i, err)
		}
		if name != "initial_turn_frame" {
			continue
		}
		found = true
		frameErr := validateInitialTurnFrame(stage)
		if frameErr == nil {
			return nil
		}
		details = append(details, fmt.Sprintf("stages[%d]: %v", i, frameErr))
	}
	if !found {
		return fmt.Errorf(`stages must include "initial_turn_frame"`)
	}
	return fmt.Errorf("initial_turn_frame must show an initial confidence-improvement evidence path: %s", strings.Join(details, "; "))
}

func validateInitialTurnFrame(stage proofStage) error {
	turnCount, ok, err := stageOptionalInt(stage, "turn_count")
	if err != nil {
		return fmt.Errorf("turn_count: %w", err)
	}
	if !ok || turnCount <= 0 {
		return fmt.Errorf("turn_count must be > 0")
	}
	confidence, ok, err := stageOptionalString(stage, "confidence")
	if err != nil {
		return fmt.Errorf("confidence: %w", err)
	}
	if !ok {
		return fmt.Errorf("confidence is required")
	}
	switch confidence {
	case "low", "medium":
	default:
		return fmt.Errorf("confidence = %q, want low or medium before evidence collection", confidence)
	}
	conclusionStatus, ok, err := stageOptionalString(stage, "conclusion_status")
	if err != nil {
		return fmt.Errorf("conclusion_status: %w", err)
	}
	if !ok || conclusionStatus != "needs_evidence" {
		return fmt.Errorf("conclusion_status = %q, want needs_evidence before evidence collection", conclusionStatus)
	}
	requiresHumanReview, ok, err := stageOptionalBool(stage, "requires_human_review")
	if err != nil {
		return fmt.Errorf("requires_human_review: %w", err)
	}
	if !ok || !requiresHumanReview {
		return fmt.Errorf("requires_human_review must be true before evidence collection")
	}
	evidenceRequests, _, err := stageOptionalInt(stage, "evidence_requests")
	if err != nil {
		return fmt.Errorf("evidence_requests: %w", err)
	}
	missingEvidence, _, err := stageOptionalInt(stage, "missing")
	if err != nil {
		return fmt.Errorf("missing: %w", err)
	}
	if evidenceRequests <= 0 && missingEvidence <= 0 {
		return fmt.Errorf("evidence_requests or missing must be positive before evidence collection")
	}
	return nil
}

func stageOptionalString(stage proofStage, key string) (string, bool, error) {
	raw, ok := stage[key]
	if !ok {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false, fmt.Errorf("must be a string: %w", err)
	}
	return value, true, nil
}

func stageOptionalInt(stage proofStage, key string) (int, bool, error) {
	raw, ok := stage[key]
	if !ok {
		return 0, false, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false, fmt.Errorf("must be an integer: %w", err)
	}
	return value, true, nil
}

func stageOptionalBool(stage proofStage, key string) (bool, bool, error) {
	raw, ok := stage[key]
	if !ok {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false, fmt.Errorf("must be a boolean: %w", err)
	}
	return value, true, nil
}

type readyStateClosure int

const (
	readyStateOpen readyStateClosure = iota
	readyStateHumanConfirmed
	readyStateOpenOrTerminal
)

func validateFinalReadyState(state proofState, confirmationRequested bool, supplementalSkipped bool) error {
	if confirmationRequested {
		return validateReadyState("final_state", state, readyStateOpen, supplementalSkipped)
	}
	return validateReadyState("final_state", state, readyStateOpenOrTerminal, supplementalSkipped)
}

func validateReadyState(field string, state proofState, closure readyStateClosure, supplementalSkipped bool) error {
	if state.Type != "" && state.Type != "state" {
		return fmt.Errorf("%s.type = %q, want state", field, state.Type)
	}
	if state.Code != "" {
		return fmt.Errorf("%s.code must be empty", field)
	}
	if _, err := validateBoundedCleanString(field+".message", state.Message, maxProofMessageBytes); err != nil {
		return err
	}
	switch closure {
	case readyStateHumanConfirmed:
		if state.Status != "closed" {
			return fmt.Errorf("%s.status = %q, want closed", field, state.Status)
		}
		if state.CloseReason != "human_confirmed" {
			return fmt.Errorf("%s.close_reason = %q, want human_confirmed", field, state.CloseReason)
		}
		if !state.FinalConclusionAvailable {
			return fmt.Errorf("%s.final_conclusion_available must be true", field)
		}
	case readyStateOpen:
		if state.Status != "open" {
			return fmt.Errorf("%s.status = %q, want open", field, state.Status)
		}
		if state.CloseReason != "" {
			return fmt.Errorf("%s.close_reason must be empty before confirmation", field)
		}
	case readyStateOpenOrTerminal:
		switch state.Status {
		case "open":
			if state.CloseReason != "" {
				return fmt.Errorf("%s.close_reason must be empty while open", field)
			}
		case "closed":
			if state.CloseReason == "" {
				return fmt.Errorf("%s.close_reason must be non-empty when closed", field)
			}
			if !state.FinalConclusionAvailable {
				return fmt.Errorf("%s.final_conclusion_available must be true when closed", field)
			}
		default:
			return fmt.Errorf("%s.status = %q, want open or closed", field, state.Status)
		}
	default:
		return fmt.Errorf("%s closure validation mode is invalid", field)
	}
	if state.InFlight {
		return fmt.Errorf("%s.in_flight must be false", field)
	}
	minTurnCount := 3
	if supplementalSkipped {
		minTurnCount = 2
	}
	if state.TurnCount < minTurnCount {
		return fmt.Errorf("%s.turn_count must be >= %d", field, minTurnCount)
	}
	readyForReview := false
	switch state.ConclusionStatus {
	case "ready_for_review":
		readyForReview = true
	case "final":
	default:
		return fmt.Errorf("%s.conclusion_status = %q, want ready_for_review or final", field, state.ConclusionStatus)
	}
	switch state.Confidence {
	case "medium", "high":
	default:
		return fmt.Errorf("%s.confidence = %q, want medium or high", field, state.Confidence)
	}
	if readyForReview && !state.RequiresHumanReview {
		return fmt.Errorf("%s.requires_human_review must be true", field)
	}
	if supplementalSkipped {
		if state.EvidenceRequests < 0 {
			return fmt.Errorf("%s.evidence_requests must be >= 0 when supplemental evidence is skipped", field)
		}
		if state.CollectionResults <= 0 {
			return fmt.Errorf("%s.collection_results must be positive when supplemental evidence is skipped", field)
		}
		if state.EvidenceRequests > 0 && state.CollectionResults < state.EvidenceRequests {
			return fmt.Errorf("%s.collection_results must cover evidence_requests when supplemental evidence is skipped", field)
		}
		if state.Missing < 0 {
			return fmt.Errorf("%s.missing must be >= 0 when supplemental evidence is skipped", field)
		}
		return nil
	}
	if state.EvidenceRequests != 0 {
		return fmt.Errorf("%s.evidence_requests must be 0 after residual evidence boundary", field)
	}
	if state.CollectionResults < 0 {
		return fmt.Errorf("%s.collection_results must be >= 0", field)
	}
	if state.Missing < 0 {
		return fmt.Errorf("%s.missing must be >= 0", field)
	}
	return nil
}

func validateConfirmation(proof confirmationProof, finalTurnCount int, supplementalSkipped bool) error {
	if !proof.Requested {
		return fmt.Errorf("confirmation.requested must be true")
	}
	if !proof.Passed {
		return fmt.Errorf("confirmation.passed must be true")
	}
	if proof.SkippedReason != "" {
		return fmt.Errorf("confirmation.skipped_reason must be empty")
	}
	if err := validateCheckedAt("confirmation.checked_at", proof.CheckedAt); err != nil {
		return err
	}
	if err := validateReadyState("confirmation.final_state", proof.FinalState, readyStateHumanConfirmed, supplementalSkipped); err != nil {
		return err
	}
	if proof.FinalState.TurnCount < finalTurnCount {
		return fmt.Errorf("confirmation.final_state.turn_count must be >= final_state.turn_count")
	}
	return nil
}

func validateSkippedConfirmation(proof confirmationProof) error {
	if proof.Requested {
		return fmt.Errorf("confirmation.requested must be false when confirmation is not requested")
	}
	if !proof.Passed {
		return fmt.Errorf("confirmation.passed must be true when confirmation is not requested")
	}
	if proof.SkippedReason != "confirmation_not_requested" {
		return fmt.Errorf("confirmation.skipped_reason = %q, want confirmation_not_requested", proof.SkippedReason)
	}
	if err := validateCheckedAt("confirmation.checked_at", proof.CheckedAt); err != nil {
		return err
	}
	if !isZeroProofState(proof.FinalState) {
		return fmt.Errorf("confirmation.final_state must be empty when confirmation is not requested")
	}
	return nil
}

func isZeroProofState(state proofState) bool {
	return state == proofState{}
}

func validateNotificationProof(proof notificationProof, expectedChannelID int64, required bool) error {
	if !required {
		return nil
	}
	if !proof.Requested {
		return fmt.Errorf("notification_proof.requested must be true")
	}
	if !proof.Passed {
		return fmt.Errorf("notification_proof.passed must be true")
	}
	if proof.SkippedReason != "" {
		return fmt.Errorf("notification_proof.skipped_reason must be empty")
	}
	if err := validateCheckedAt("notification_proof.checked_at", proof.CheckedAt); err != nil {
		return err
	}
	if len(proof.Entries) == 0 {
		return fmt.Errorf("notification_proof.entries must be non-empty")
	}
	for i, entry := range proof.Entries {
		if err := validateNotificationProofEntry(entry, expectedChannelID); err != nil {
			return fmt.Errorf("notification_proof.entries[%d]: %w", i, err)
		}
	}
	seenEvents := map[string]bool{}
	for _, entry := range proof.Entries {
		seenEvents[entry.EventKind] = true
	}
	for _, eventKind := range requiredNotificationProofEvents {
		if !seenEvents[eventKind] {
			return fmt.Errorf("notification_proof.entries missing required event_kind %q", eventKind)
		}
	}
	return nil
}

func validateNotificationProofEntry(entry notificationProofEntry, expectedChannelID int64) error {
	switch entry.EventKind {
	case "diagnosis_room.assistant_turn_notification_sent",
		"diagnosis_room.final_ready_notification_sent",
		"diagnosis_room.close_notification_sent":
	default:
		return fmt.Errorf("event_kind = %q, want AI diagnosis notification event", entry.EventKind)
	}
	channelID, ok, err := optionalPositiveInt64(entry.NotificationChannelProfileID)
	if err != nil {
		return fmt.Errorf("notification_channel_profile_id: %w", err)
	}
	if !ok {
		return fmt.Errorf("notification_channel_profile_id is required")
	}
	if expectedChannelID > 0 && channelID != expectedChannelID {
		return fmt.Errorf("notification_channel_profile_id = %d, want %d", channelID, expectedChannelID)
	}
	status := strings.ToLower(strings.TrimSpace(entry.ProviderStatus))
	if !notificationProviderStatusAccepted(status) {
		return fmt.Errorf("provider_status = %q, want accepted, delivered, sent, or success", entry.ProviderStatus)
	}
	if expectedContentKind := notificationProofExpectedContentKind(entry.EventKind); entry.ContentKind != expectedContentKind {
		return fmt.Errorf("content_kind = %q, want %s for event_kind %q", entry.ContentKind, expectedContentKind, entry.EventKind)
	}
	if len(entry.ContentSHA256) != sha256HexLength || len(strings.Trim(entry.ContentSHA256, "0123456789abcdef")) != 0 {
		return fmt.Errorf("content_sha256 must be 64 lowercase hex characters")
	}
	if err := validateOptionalNonNegativeInt64("assistant_turn_id", entry.AssistantTurnID); err != nil {
		return err
	}
	if err := validateOptionalNonNegativeInt64("turn_count", entry.TurnCount); err != nil {
		return err
	}
	if err := validateOptionalNonNegativeInt64("recommended_action_count", entry.RecommendedActionCount); err != nil {
		return err
	}
	if err := validateOptionalNonNegativeInt64("evidence_request_count", entry.EvidenceRequestCount); err != nil {
		return err
	}
	if entry.ProviderMessageID != "" {
		if _, err := validateID("provider_message_id", entry.ProviderMessageID, maxProofMessageBytes); err != nil {
			return err
		}
	}
	if entry.AssistantMessageID != "" {
		if _, err := validateID("assistant_message_id", entry.AssistantMessageID, maxProofMessageBytes); err != nil {
			return err
		}
	}
	return validateCheckedAt("occurred_at", entry.OccurredAt)
}

func notificationProofExpectedContentKind(eventKind string) string {
	switch eventKind {
	case "diagnosis_room.assistant_turn_notification_sent":
		return "assistant_message"
	case "diagnosis_room.final_ready_notification_sent", "diagnosis_room.close_notification_sent":
		return "final_conclusion"
	default:
		return ""
	}
}

func notificationProviderStatusAccepted(status string) bool {
	switch status {
	case "accepted", "delivered", "sent", "success":
		return true
	default:
		return false
	}
}

func validateCheckedAt(field, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s must be non-empty", field)
	}
	if trimmed != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return fmt.Errorf("%s must be canonical UTC RFC3339", field)
	}
	if checkedAt.After(nowUTC().Add(time.Minute)) {
		return fmt.Errorf("%s is in the future", field)
	}
	return nil
}

func validateID(field, value string, maxBytes int) (string, error) {
	id, err := validateBoundedCleanString(field, value, maxBytes)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return "", fmt.Errorf("%s must not contain whitespace", field)
	}
	return id, nil
}

func validateBoundedCleanString(field, value string, maxBytes int) (string, error) {
	if value != strings.TrimSpace(value) {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if len(value) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	if strings.ContainsAny(value, "\x00\r") {
		return "", fmt.Errorf("%s must not contain NUL or carriage return", field)
	}
	return value, nil
}

func validateOptionalNonNegativeInt64(field string, raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if len(bytes.Trim(raw, "0123456789")) != 0 {
		return fmt.Errorf("%s must be a non-negative integer or null", field)
	}
	if _, err := strconv.ParseInt(string(raw), 10, 64); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func optionalPositiveInt64(raw json.RawMessage) (int64, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false, nil
	}
	if len(bytes.Trim(raw, "0123456789")) != 0 {
		return 0, false, fmt.Errorf("must be a positive integer or null")
	}
	text := string(raw)
	if strings.HasPrefix(text, "0") {
		return 0, false, fmt.Errorf("must not contain leading zeroes")
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false, err
	}
	if value <= 0 {
		return 0, false, fmt.Errorf("must be > 0")
	}
	return value, true, nil
}
