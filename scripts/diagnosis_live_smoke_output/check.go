// Command diagnosis_live_smoke_output validates the JSON proof produced by
// the manual M5 live browser diagnosis-room smoke gate.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxProofBytes                               int64 = 10 * 1024 * 1024
	maxProofSessionIDBytes                            = 128
	maxProofWorkflowIDBytes                           = 256
	maxProofRunIDBytes                                = 256
	maxProofEvidenceBytes                             = 512
	maxProofCloseReasonBytes                          = 128
	maxProofIdempotencyKeyBytes                       = 256
	maxProofProviderMessageBytes                      = 512
	maxProofFinalConclusionContentBytes               = 4096
	maxProofBrowserReadinessTextBytes                 = 512
	maxProofBrowserCollectionSummaryTextBytes         = 256
	maxProofBrowserConfirmBlockReasonBytes            = 128
	maxProofBrowserCompletionEvidenceBytes            = 128
	maxProofBrowserSupplementalLabelBytes             = 256
	maxProofBrowserSupplementalBlockReasonBytes       = 128
	maxProofBrowserSeedMissingBytes                   = 512

	closeNotificationClosedKind = "diagnosis_room.closed"
	closeNotificationSentKind   = "diagnosis_room.close_notification_sent"
)

var requiredNotificationProofEvents = []string{
	"diagnosis_room.assistant_turn_notification_sent",
	"diagnosis_room.final_ready_notification_sent",
	"diagnosis_room.close_notification_sent",
}

type smokeOutput struct {
	Passed             bool               `json:"passed"`
	CheckedAt          string             `json:"checked_at"`
	Request            proofRequest       `json:"request"`
	WebBaseURL         string             `json:"web_base_url"`
	APIBaseURL         string             `json:"api_base_url"`
	SessionID          string             `json:"session_id"`
	EvidenceSnapshotID json.RawMessage    `json:"evidence_snapshot_id"`
	CreatedRoom        *createdRoom       `json:"created_room"`
	MessageLength      int                `json:"message_length"`
	MessageSHA256      string             `json:"message_sha256"`
	Browser            *browserProof      `json:"browser"`
	CloseNotification  *closeProof        `json:"close_notification"`
	NotificationProof  *notificationProof `json:"notification_proof"`
	Evidence           string             `json:"evidence"`
}

type proofRequest struct {
	Mode                         string          `json:"mode"`
	SessionID                    string          `json:"session_id"`
	EvidenceSnapshotID           json.RawMessage `json:"evidence_snapshot_id"`
	NotificationChannelProfileID json.RawMessage `json:"notification_channel_profile_id"`
	RequireNotificationProof     bool            `json:"require_notification_proof"`
	MessageLength                int             `json:"message_length"`
	MessageSHA256                string          `json:"message_sha256"`
}

type createdRoom struct {
	SessionID          string `json:"session_id"`
	EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
	DiagnosisTaskID    int64  `json:"diagnosis_task_id"`
	ChatSessionID      int64  `json:"chat_session_id"`
	WorkflowID         string `json:"workflow_id"`
	RunID              string `json:"run_id"`
}

type browserProof struct {
	StateLoaded                       bool   `json:"state_loaded"`
	TurnResultObserved                bool   `json:"turn_result_observed"`
	AssistantTurnsBefore              int    `json:"assistant_turns_before"`
	AssistantTurnsAfter               int    `json:"assistant_turns_after"`
	AssistantTurnDelta                int    `json:"assistant_turn_delta"`
	TranscriptMessagesBefore          int    `json:"transcript_messages_before"`
	TranscriptMessagesAfter           int    `json:"transcript_messages_after"`
	ConnectionStatusAfterTurn         string `json:"connection_status_after_turn"`
	SubmittedMessageVisible           bool   `json:"submitted_message_visible"`
	SubmittedMessageLength            int    `json:"submitted_message_length"`
	SubmittedMessageSHA256            string `json:"submitted_message_sha256"`
	CompletedTurnText                 string `json:"completed_turn_text"`
	ConsultationInsightVisible        bool   `json:"consultation_insight_visible"`
	ConsultationProgressVisible       bool   `json:"consultation_progress_visible"`
	EvidenceReadinessVisible          bool   `json:"evidence_readiness_visible"`
	Confidence                        string `json:"confidence"`
	ConfidenceAriaValue               string `json:"confidence_aria_value"`
	EvidenceReadinessText             string `json:"evidence_readiness_text"`
	ToolRequestSeedRequested          bool   `json:"tool_request_seed_requested,omitempty"`
	ToolRequestSeedCount              int    `json:"tool_request_seed_count,omitempty"`
	ToolRequestSeedMatchedCount       int    `json:"tool_request_seed_matched_count,omitempty"`
	ToolRequestSeedMissing            string `json:"tool_request_seed_missing,omitempty"`
	OperatorSeedCollectionRequested   *bool  `json:"operator_seed_collection_requested,omitempty"`
	OperatorSeedCollectionTriggered   *bool  `json:"operator_seed_collection_triggered,omitempty"`
	OperatorSeedCollectionCount       int    `json:"operator_seed_collection_count,omitempty"`
	OperatorSeedCollectionBefore      int    `json:"operator_seed_collection_result_count_before,omitempty"`
	OperatorSeedCollectionAfter       int    `json:"operator_seed_collection_result_count_after,omitempty"`
	OperatorSeedAssistantBefore       int    `json:"operator_seed_collection_assistant_turns_before,omitempty"`
	OperatorSeedAssistantAfter        int    `json:"operator_seed_collection_assistant_turns_after,omitempty"`
	OperatorSeedAssistantDelta        int    `json:"operator_seed_collection_assistant_turn_delta,omitempty"`
	OperatorSeedConfidenceBefore      string `json:"operator_seed_collection_confidence_before,omitempty"`
	OperatorSeedConfidenceAfter       string `json:"operator_seed_collection_confidence_after,omitempty"`
	OperatorSeedCollectionSummary     string `json:"operator_seed_collection_summary_text,omitempty"`
	OperatorStagedCollectionRequested bool   `json:"operator_staged_collection_requested,omitempty"`
	OperatorStagedCollectionCount     int    `json:"operator_staged_collection_count,omitempty"`
	OperatorStagedCollectionTriggered *bool  `json:"operator_staged_collection_triggered,omitempty"`
	OperatorStagedCollectionMatched   int    `json:"operator_staged_collection_matched_count,omitempty"`
	OperatorStagedCollectionMissing   string `json:"operator_staged_collection_missing,omitempty"`
	OperatorStagedCollectionModes     string `json:"operator_staged_collection_modes,omitempty"`
	OperatorStagedCollectionBefore    int    `json:"operator_staged_collection_result_count_before,omitempty"`
	OperatorStagedCollectionAfter     int    `json:"operator_staged_collection_result_count_after,omitempty"`
	OperatorStagedAssistantBefore     int    `json:"operator_staged_collection_assistant_turns_before,omitempty"`
	OperatorStagedAssistantAfter      int    `json:"operator_staged_collection_assistant_turns_after,omitempty"`
	OperatorStagedAssistantDelta      int    `json:"operator_staged_collection_assistant_turn_delta,omitempty"`
	OperatorStagedConfidenceBefore    string `json:"operator_staged_collection_confidence_before,omitempty"`
	OperatorStagedConfidenceAfter     string `json:"operator_staged_collection_confidence_after,omitempty"`
	OperatorStagedCollectionSummary   string `json:"operator_staged_collection_summary_text,omitempty"`
	EvidencePlanCount                 int    `json:"evidence_plan_count,omitempty"`
	EvidenceCollectionResultCount     int    `json:"evidence_collection_result_count,omitempty"`
	EvidenceCollectionSummaryVisible  *bool  `json:"evidence_collection_summary_visible,omitempty"`
	EvidenceCollectionSummaryText     string `json:"evidence_collection_summary_text,omitempty"`
	PlannedEvidenceRequested          *bool  `json:"planned_evidence_collection_requested,omitempty"`
	PlannedEvidenceAvailable          *bool  `json:"planned_evidence_collection_available,omitempty"`
	PlannedEvidenceActionCount        int    `json:"planned_evidence_collection_action_count,omitempty"`
	PlannedEvidenceTool               string `json:"planned_evidence_collection_tool,omitempty"`
	PlannedEvidenceMode               string `json:"planned_evidence_collection_mode,omitempty"`
	PlannedEvidenceSatisfied          *bool  `json:"planned_evidence_collection_satisfied,omitempty"`
	PlannedEvidenceTriggered          *bool  `json:"planned_evidence_collection_triggered,omitempty"`
	PlannedEvidenceAssistantBefore    int    `json:"planned_evidence_assistant_turns_before,omitempty"`
	PlannedEvidenceAssistantAfter     int    `json:"planned_evidence_assistant_turns_after,omitempty"`
	PlannedEvidenceAssistantDelta     int    `json:"planned_evidence_assistant_turn_delta,omitempty"`
	PlannedEvidenceConfidenceBefore   string `json:"planned_evidence_confidence_before,omitempty"`
	PlannedEvidenceConfidenceAfter    string `json:"planned_evidence_confidence_after,omitempty"`
	PlannedEvidenceResultsBefore      int    `json:"planned_evidence_collection_result_count_before,omitempty"`
	PlannedEvidenceResultsAfter       int    `json:"planned_evidence_collection_result_count_after,omitempty"`
	PlannedEvidenceBackendResults     int    `json:"planned_evidence_backend_collection_result_count,omitempty"`
	PlannedEvidenceBackendCollected   int    `json:"planned_evidence_backend_collected_result_count,omitempty"`
	PlannedEvidenceSummaryText        string `json:"planned_evidence_collection_summary_text,omitempty"`
	PlannedEvidenceSummaryVisible     *bool  `json:"planned_evidence_collection_summary_visible,omitempty"`
	PlannedEvidenceFinalVisible       *bool  `json:"planned_evidence_final_conclusion_visible,omitempty"`
	PlannedEvidenceReadyVisible       *bool  `json:"planned_evidence_ready_for_confirmation_visible,omitempty"`
	PlannedEvidenceTimelineVisible    *bool  `json:"planned_evidence_timeline_visible,omitempty"`
	SupplementalEvidenceRequested     *bool  `json:"supplemental_evidence_requested,omitempty"`
	SupplementalEvidenceRequired      *bool  `json:"supplemental_evidence_required,omitempty"`
	SupplementalFollowUpAvailable     *bool  `json:"supplemental_follow_up_available,omitempty"`
	SupplementalFollowUpCount         int    `json:"supplemental_follow_up_count,omitempty"`
	SupplementalRequestLabel          string `json:"supplemental_request_label,omitempty"`
	SupplementalBlockReason           string `json:"supplemental_block_reason,omitempty"`
	SupplementalEvidenceSubmitted     *bool  `json:"supplemental_evidence_submitted,omitempty"`
	SupplementalEvidenceLength        int    `json:"supplemental_evidence_length,omitempty"`
	SupplementalEvidenceSHA256        string `json:"supplemental_evidence_sha256,omitempty"`
	SupplementalAssistantBefore       int    `json:"supplemental_assistant_turns_before,omitempty"`
	SupplementalAssistantAfter        int    `json:"supplemental_assistant_turns_after,omitempty"`
	SupplementalAssistantDelta        int    `json:"supplemental_assistant_turn_delta,omitempty"`
	SupplementalCompletionEvidence    string `json:"supplemental_completion_evidence_after,omitempty"`
	SupplementalConfidenceBefore      string `json:"supplemental_confidence_before,omitempty"`
	SupplementalConfidenceAfter       string `json:"supplemental_confidence_after,omitempty"`
	SupplementalHistoryVisible        *bool  `json:"supplemental_history_visible,omitempty"`
	SupplementalHistoryBefore         int    `json:"supplemental_history_count_before,omitempty"`
	SupplementalHistoryAfter          int    `json:"supplemental_history_count_after,omitempty"`
	SupplementalReviewQueueVisible    *bool  `json:"supplemental_review_queue_visible,omitempty"`
	SupplementalReviewQueueItemCount  int    `json:"supplemental_review_queue_item_count,omitempty"`
	SupplementalConfirmAvailable      *bool  `json:"supplemental_confirm_conclusion_available_after,omitempty"`
	SupplementalConfirmBlockReason    string `json:"supplemental_confirm_block_reason_after,omitempty"`
	ConfirmConclusionRequested        *bool  `json:"confirm_conclusion_requested,omitempty"`
	ConfirmConclusionAvailable        *bool  `json:"confirm_conclusion_available,omitempty"`
	ConfirmConclusionBlocked          *bool  `json:"confirm_conclusion_blocked,omitempty"`
	ConfirmConclusionBlockReason      string `json:"confirm_conclusion_block_reason,omitempty"`
	FinalConclusionConfirmed          *bool  `json:"final_conclusion_confirmed,omitempty"`
	FinalConclusionVisible            *bool  `json:"final_conclusion_visible,omitempty"`
	ConfirmedStateText                string `json:"confirmed_state_text,omitempty"`
	ConnectionStatusAfterConfirm      string `json:"connection_status_after_confirm,omitempty"`
	ConfirmButtonDisabled             *bool  `json:"confirm_button_disabled_after_confirm,omitempty"`
	CloseReasonVisible                *bool  `json:"close_reason_visible,omitempty"`
	ConclusionVersionVisible          *bool  `json:"conclusion_version_visible,omitempty"`
}

type closeProof struct {
	CheckedAt         string                 `json:"checked_at"`
	Request           closeProofRequest      `json:"request"`
	Signaled          bool                   `json:"signaled"`
	Workflow          closeProofWorkflow     `json:"workflow"`
	CloseEvent        closeProofEvent        `json:"close_event"`
	NotificationEvent closeNotificationEvent `json:"notification_event"`
}

type closeProofRequest struct {
	SessionID   string `json:"session_id"`
	WorkflowID  string `json:"workflow_id"`
	RunID       string `json:"run_id"`
	Reason      string `json:"reason"`
	WaitTimeout string `json:"wait_timeout"`
}

type closeProofWorkflow struct {
	SessionID       string               `json:"session_id"`
	ChatSessionID   int64                `json:"chat_session_id"`
	DiagnosisTaskID int64                `json:"diagnosis_task_id"`
	Status          string               `json:"status"`
	TurnCount       int                  `json:"turn_count"`
	ClosedAt        string               `json:"closed_at"`
	CloseReason     string               `json:"close_reason"`
	FinalConclusion closeFinalConclusion `json:"final_conclusion"`
}

type closeProofEvent struct {
	ID                int64                `json:"id"`
	Kind              string               `json:"kind"`
	OccurredAt        string               `json:"occurred_at"`
	ConclusionVersion string               `json:"conclusion_version"`
	FinalConclusion   closeFinalConclusion `json:"final_conclusion"`
}

type closeFinalConclusion struct {
	Status                  string   `json:"status"`
	Source                  string   `json:"source"`
	Reason                  string   `json:"reason,omitempty"`
	EvidenceSnapshotID      int64    `json:"evidence_snapshot_id,omitempty"`
	ConclusionVersion       string   `json:"conclusion_version,omitempty"`
	RecordedAt              string   `json:"recorded_at,omitempty"`
	ConfirmedBy             string   `json:"confirmed_by,omitempty"`
	SupplementalContextRefs []string `json:"supplemental_context_refs,omitempty"`
	AssistantTurnID         int64    `json:"assistant_turn_id,omitempty"`
	AssistantMessageID      string   `json:"assistant_message_id,omitempty"`
	AssistantSequence       int      `json:"assistant_sequence,omitempty"`
	AssistantOccurredAt     string   `json:"assistant_occurred_at,omitempty"`
	Content                 string   `json:"content,omitempty"`
	Confidence              string   `json:"confidence,omitempty"`
	RequiresHumanReview     *bool    `json:"requires_human_review,omitempty"`
}

type closeNotificationEvent struct {
	ID                int64  `json:"id"`
	Kind              string `json:"kind"`
	OccurredAt        string `json:"occurred_at"`
	IdempotencyKey    string `json:"idempotency_key"`
	ProviderMessageID string `json:"provider_message_id"`
	ProviderStatus    string `json:"provider_status"`
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
		fmt.Fprintf(os.Stderr, "[diagnosis-live-smoke-output] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[diagnosis-live-smoke-output] OK")
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: diagnosis_live_smoke_output <output.json>")
	}
	path := filepath.Clean(args[0])
	raw, err := readProofFile(path)
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
	_, err := validateCheckedAt(out.CheckedAt)
	if err != nil {
		return err
	}
	if err := validateHTTPURL("web_base_url", out.WebBaseURL); err != nil {
		return err
	}
	if err := validateHTTPURL("api_base_url", out.APIBaseURL); err != nil {
		return err
	}
	sessionID, err := validateProofID("session_id", out.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if out.MessageLength <= 0 {
		return fmt.Errorf("message_length must be > 0")
	}
	messageSHA256, err := validateSHA256Hex("message_sha256", out.MessageSHA256)
	if err != nil {
		return err
	}
	evidence, err := validateBoundedCleanString("evidence", out.Evidence, maxProofEvidenceBytes)
	if err != nil {
		return err
	}
	if !strings.Contains(evidence, "turn_result") {
		return fmt.Errorf("evidence must mention the turn_result browser round trip")
	}
	if out.Browser == nil {
		return fmt.Errorf("browser proof is required")
	}
	browserConfirmedConclusion, err := validateBrowserProof(*out.Browser, out.MessageLength, messageSHA256)
	if err != nil {
		return err
	}
	if out.CloseNotification != nil && !strings.Contains(evidence, "close_notification") {
		return fmt.Errorf("evidence must mention close_notification when close_notification proof is present")
	}
	notificationChannelID, notificationRequested, err := optionalPositiveInt64(out.Request.NotificationChannelProfileID)
	if err != nil {
		return fmt.Errorf("request.notification_channel_profile_id: %w", err)
	}
	if out.Request.RequireNotificationProof {
		if !notificationRequested {
			return fmt.Errorf("request.require_notification_proof requires request.notification_channel_profile_id")
		}
		if out.CloseNotification == nil {
			return fmt.Errorf("request.require_notification_proof requires close_notification proof")
		}
		if !strings.Contains(evidence, "ai_notification_delivery") {
			return fmt.Errorf("evidence must mention ai_notification_delivery when notification proof is required")
		}
	}
	if browserConfirmedConclusion && !strings.Contains(evidence, "confirm_conclusion") {
		return fmt.Errorf("evidence must mention confirm_conclusion when browser confirmation proof is present")
	}
	if browserSatisfiedPlannedEvidence(*out.Browser) && !strings.Contains(evidence, "planned_evidence_collection") {
		return fmt.Errorf("evidence must mention planned_evidence_collection when browser planned evidence proof is present")
	}
	if strings.Contains(evidence, "planned_evidence_collection") && !browserSatisfiedPlannedEvidence(*out.Browser) {
		return fmt.Errorf("evidence must not mention planned_evidence_collection without browser planned evidence proof")
	}
	if browserSubmittedSupplementalEvidence(*out.Browser) && !strings.Contains(evidence, "supplemental_evidence") {
		return fmt.Errorf("evidence must mention supplemental_evidence when browser supplemental evidence proof is present")
	}
	if strings.Contains(evidence, "supplemental_evidence") && !browserSubmittedSupplementalEvidence(*out.Browser) {
		return fmt.Errorf("evidence must not mention supplemental_evidence without browser supplemental evidence proof")
	}

	requestEvidenceSnapshotID, requestHasEvidenceSnapshot, err := validateProofRequest(out.Request, sessionID, out.MessageLength, messageSHA256, out.CreatedRoom != nil)
	if err != nil {
		return err
	}
	evidenceSnapshotID, hasEvidenceSnapshot, err := optionalPositiveInt64(out.EvidenceSnapshotID)
	if err != nil {
		return fmt.Errorf("evidence_snapshot_id: %w", err)
	}
	if requestHasEvidenceSnapshot {
		if !hasEvidenceSnapshot {
			return fmt.Errorf("request.evidence_snapshot_id must match evidence_snapshot_id")
		}
		if requestEvidenceSnapshotID != evidenceSnapshotID {
			return fmt.Errorf("request.evidence_snapshot_id must match evidence_snapshot_id")
		}
	} else if hasEvidenceSnapshot {
		return fmt.Errorf("request.evidence_snapshot_id must match evidence_snapshot_id")
	}
	if out.CreatedRoom != nil {
		if err := validateCreatedRoom(*out.CreatedRoom, sessionID, evidenceSnapshotID, hasEvidenceSnapshot); err != nil {
			return err
		}
	}
	if out.CloseNotification != nil {
		if err := validateCloseProof(*out.CloseNotification, sessionID, out.Browser.AssistantTurnsAfter, out.CreatedRoom); err != nil {
			return err
		}
	}
	return validateNotificationProof(out.NotificationProof, notificationChannelID, out.Request.RequireNotificationProof || notificationRequested)
}

func validateProofRequest(req proofRequest, sessionID string, messageLength int, messageSHA256 string, createdRoom bool) (int64, bool, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		return 0, false, fmt.Errorf("request.mode must be non-empty")
	}
	if mode != req.Mode {
		return 0, false, fmt.Errorf("request.mode must not contain leading or trailing whitespace")
	}
	switch mode {
	case "existing_session":
		if createdRoom {
			return 0, false, fmt.Errorf("request.mode existing_session must not include created_room")
		}
	case "create_room":
		if !createdRoom {
			return 0, false, fmt.Errorf("request.mode create_room requires created_room")
		}
	default:
		return 0, false, fmt.Errorf("request.mode = %q, want existing_session or create_room", mode)
	}
	requestSessionID, err := validateProofID("request.session_id", req.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return 0, false, err
	}
	if requestSessionID != sessionID {
		return 0, false, fmt.Errorf("request.session_id must match session_id")
	}
	if req.MessageLength <= 0 {
		return 0, false, fmt.Errorf("request.message_length must be > 0")
	}
	if req.MessageLength != messageLength {
		return 0, false, fmt.Errorf("request.message_length must match message_length")
	}
	requestMessageSHA256, err := validateSHA256Hex("request.message_sha256", req.MessageSHA256)
	if err != nil {
		return 0, false, err
	}
	if requestMessageSHA256 != messageSHA256 {
		return 0, false, fmt.Errorf("request.message_sha256 must match message_sha256")
	}
	value, hasValue, err := optionalPositiveInt64(req.EvidenceSnapshotID)
	if err != nil {
		return 0, false, fmt.Errorf("request.evidence_snapshot_id: %w", err)
	}
	if mode == "create_room" && !hasValue {
		return 0, false, fmt.Errorf("request.evidence_snapshot_id is required for create_room")
	}
	if _, hasNotificationChannel, err := optionalPositiveInt64(req.NotificationChannelProfileID); err != nil {
		return 0, false, fmt.Errorf("request.notification_channel_profile_id: %w", err)
	} else if req.RequireNotificationProof && !hasNotificationChannel {
		return 0, false, fmt.Errorf("request.require_notification_proof requires request.notification_channel_profile_id")
	}
	return value, hasValue, nil
}

func validateBrowserProof(proof browserProof, messageLength int, messageSHA256 string) (bool, error) {
	if !proof.StateLoaded {
		return false, fmt.Errorf("browser.state_loaded must be true")
	}
	if !proof.TurnResultObserved {
		return false, fmt.Errorf("browser.turn_result_observed must be true")
	}
	if !proof.SubmittedMessageVisible {
		return false, fmt.Errorf("browser.submitted_message_visible must be true")
	}
	if proof.SubmittedMessageLength <= 0 {
		return false, fmt.Errorf("browser.submitted_message_length must be > 0")
	}
	if proof.SubmittedMessageLength != messageLength {
		return false, fmt.Errorf("browser.submitted_message_length must match message_length")
	}
	submittedMessageSHA256, err := validateSHA256Hex("browser.submitted_message_sha256", proof.SubmittedMessageSHA256)
	if err != nil {
		return false, err
	}
	if submittedMessageSHA256 != messageSHA256 {
		return false, fmt.Errorf("browser.submitted_message_sha256 must match message_sha256")
	}
	if proof.AssistantTurnsBefore < 0 {
		return false, fmt.Errorf("browser.assistant_turns_before must be >= 0")
	}
	if proof.AssistantTurnsAfter <= proof.AssistantTurnsBefore {
		return false, fmt.Errorf("browser.assistant_turns_after must be greater than assistant_turns_before")
	}
	assistantTurnDelta := proof.AssistantTurnsAfter - proof.AssistantTurnsBefore
	if proof.AssistantTurnDelta != assistantTurnDelta {
		return false, fmt.Errorf("browser.assistant_turn_delta must equal assistant_turns_after - assistant_turns_before")
	}
	if proof.TranscriptMessagesBefore < 0 {
		return false, fmt.Errorf("browser.transcript_messages_before must be >= 0")
	}
	if proof.TranscriptMessagesBefore != proof.AssistantTurnsBefore*2 {
		return false, fmt.Errorf("browser.transcript_messages_before must equal assistant_turns_before * 2")
	}
	if proof.TranscriptMessagesAfter != proof.TranscriptMessagesBefore+(2*assistantTurnDelta) {
		return false, fmt.Errorf("browser.transcript_messages_after must equal transcript_messages_before + 2 * assistant_turn_delta")
	}
	if proof.TranscriptMessagesAfter != proof.AssistantTurnsAfter*2 {
		return false, fmt.Errorf("browser.transcript_messages_after must equal assistant_turns_after * 2")
	}
	status := strings.TrimSpace(proof.ConnectionStatusAfterTurn)
	if status == "" {
		return false, fmt.Errorf("browser.connection_status_after_turn must be non-empty")
	}
	if status != proof.ConnectionStatusAfterTurn {
		return false, fmt.Errorf("browser.connection_status_after_turn must not contain leading or trailing whitespace")
	}
	if status != "connected" {
		return false, fmt.Errorf("browser.connection_status_after_turn = %q, want connected", status)
	}
	completedTurnText, err := validateRequiredCleanString("browser.completed_turn_text", proof.CompletedTurnText)
	if err != nil {
		return false, err
	}
	turnNumber, ok := completedTurnNumber(completedTurnText)
	if !ok {
		return false, fmt.Errorf("browser.completed_turn_text must match a completed turn or loaded state log entry")
	}
	if turnNumber <= proof.AssistantTurnsBefore || turnNumber > proof.AssistantTurnsAfter {
		return false, fmt.Errorf("browser.completed_turn_text must be within observed assistant turn bounds")
	}
	if turnNumber != proof.AssistantTurnsAfter && !browserCompletedTurnTextMayLag(proof, turnNumber) {
		return false, fmt.Errorf("browser.completed_turn_text must match assistant_turns_after")
	}
	if !proof.ConsultationInsightVisible {
		return false, fmt.Errorf("browser.consultation_insight_visible must be true")
	}
	if !proof.ConsultationProgressVisible {
		return false, fmt.Errorf("browser.consultation_progress_visible must be true")
	}
	if !proof.EvidenceReadinessVisible {
		return false, fmt.Errorf("browser.evidence_readiness_visible must be true")
	}
	confidence, err := validateRequiredCleanString("browser.confidence", proof.Confidence)
	if err != nil {
		return false, err
	}
	switch confidence {
	case "low", "medium", "high":
	default:
		return false, fmt.Errorf("browser.confidence = %q, want low, medium, or high", confidence)
	}
	confidenceAriaValue, err := validateRequiredCleanString("browser.confidence_aria_value", proof.ConfidenceAriaValue)
	if err != nil {
		return false, err
	}
	if confidenceAriaValue != confidence+" confidence" {
		return false, fmt.Errorf("browser.confidence_aria_value must match browser.confidence")
	}
	readinessText, err := validateBoundedText(
		"browser.evidence_readiness_text",
		proof.EvidenceReadinessText,
		maxProofBrowserReadinessTextBytes,
	)
	if err != nil {
		return false, err
	}
	for _, label := range []string{"Plan", "Collected", "Missing", "Suggestions", "Next"} {
		if !strings.Contains(readinessText, label) {
			return false, fmt.Errorf("browser.evidence_readiness_text must include %s", label)
		}
	}
	if proof.ToolRequestSeedCount < 0 {
		return false, fmt.Errorf("browser.tool_request_seed_count must be >= 0")
	}
	if !proof.ToolRequestSeedRequested && proof.ToolRequestSeedCount != 0 {
		return false, fmt.Errorf("browser.tool_request_seed_count must be 0 unless tool_request_seed_requested is true")
	}
	if proof.ToolRequestSeedRequested && proof.ToolRequestSeedCount == 0 {
		return false, fmt.Errorf("browser.tool_request_seed_count must be > 0 when tool_request_seed_requested is true")
	}
	if proof.ToolRequestSeedMatchedCount < 0 {
		return false, fmt.Errorf("browser.tool_request_seed_matched_count must be >= 0")
	}
	if !proof.ToolRequestSeedRequested {
		if proof.ToolRequestSeedMatchedCount != 0 {
			return false, fmt.Errorf("browser.tool_request_seed_matched_count must be 0 unless tool_request_seed_requested is true")
		}
		if strings.TrimSpace(proof.ToolRequestSeedMissing) != "" {
			return false, fmt.Errorf("browser.tool_request_seed_missing must be empty unless tool_request_seed_requested is true")
		}
	}
	if proof.ToolRequestSeedRequested {
		if proof.ToolRequestSeedMatchedCount != proof.ToolRequestSeedCount {
			missing, err := validateBoundedText(
				"browser.tool_request_seed_missing",
				proof.ToolRequestSeedMissing,
				maxProofBrowserSeedMissingBytes,
			)
			if err != nil {
				return false, err
			}
			if missing == "" {
				return false, fmt.Errorf("browser.tool_request_seed_missing must describe unmatched seeded tool requests")
			}
			return false, fmt.Errorf("browser.tool_request_seed_matched_count must cover seeded tool requests")
		}
		if strings.TrimSpace(proof.ToolRequestSeedMissing) != "" {
			return false, fmt.Errorf("browser.tool_request_seed_missing must be empty when all seeded tool requests matched")
		}
	}
	if proof.EvidencePlanCount < 0 {
		return false, fmt.Errorf("browser.evidence_plan_count must be >= 0")
	}
	if proof.EvidenceCollectionResultCount < 0 {
		return false, fmt.Errorf("browser.evidence_collection_result_count must be >= 0")
	}
	if proof.EvidenceCollectionSummaryVisible != nil {
		if *proof.EvidenceCollectionSummaryVisible {
			if _, err := validateBoundedText(
				"browser.evidence_collection_summary_text",
				proof.EvidenceCollectionSummaryText,
				maxProofBrowserCollectionSummaryTextBytes,
			); err != nil {
				return false, err
			}
		} else if strings.TrimSpace(proof.EvidenceCollectionSummaryText) != "" {
			return false, fmt.Errorf("browser.evidence_collection_summary_text must be empty when summary is not visible")
		}
		if proof.EvidenceCollectionResultCount > 0 &&
			!*proof.EvidenceCollectionSummaryVisible &&
			proof.PlannedEvidenceMode != "already_final" {
			return false, fmt.Errorf("browser.evidence_collection_summary_visible must be true when collection results are present")
		}
	}
	if err := validateBrowserPlannedEvidenceProof(proof); err != nil {
		return false, err
	}
	if err := validateBrowserOperatorSeedCollectionProof(proof); err != nil {
		return false, err
	}
	if err := validateBrowserSupplementalEvidenceProof(proof); err != nil {
		return false, err
	}
	if proof.ConfirmConclusionRequested == nil || !*proof.ConfirmConclusionRequested {
		return false, nil
	}
	if proof.ConfirmConclusionAvailable == nil {
		return false, fmt.Errorf("browser.confirm_conclusion_available is required when confirmation is requested")
	}
	if !*proof.ConfirmConclusionAvailable {
		if proof.ConfirmConclusionBlocked == nil || !*proof.ConfirmConclusionBlocked {
			return false, fmt.Errorf("browser.confirm_conclusion_blocked must be true when requested confirmation is unavailable")
		}
		if _, err := validateBoundedCleanString(
			"browser.confirm_conclusion_block_reason",
			proof.ConfirmConclusionBlockReason,
			maxProofBrowserConfirmBlockReasonBytes,
		); err != nil {
			return false, err
		}
		if proof.FinalConclusionConfirmed != nil && *proof.FinalConclusionConfirmed {
			return false, fmt.Errorf("browser.final_conclusion_confirmed must not be true when requested confirmation is unavailable")
		}
		return false, nil
	}
	if proof.FinalConclusionConfirmed == nil || !*proof.FinalConclusionConfirmed {
		return false, fmt.Errorf("browser.final_conclusion_confirmed must be true when confirmation is requested")
	}
	if proof.FinalConclusionVisible == nil || !*proof.FinalConclusionVisible {
		return false, fmt.Errorf("browser.final_conclusion_visible must be true when confirmation is requested")
	}
	confirmedStateText, err := validateRequiredCleanString("browser.confirmed_state_text", proof.ConfirmedStateText)
	if err != nil {
		return false, err
	}
	wantConfirmedStateText := fmt.Sprintf("Loaded state: closed, %d turn(s).", proof.AssistantTurnsAfter)
	if confirmedStateText != wantConfirmedStateText {
		return false, fmt.Errorf("browser.confirmed_state_text = %q, want %q", confirmedStateText, wantConfirmedStateText)
	}
	statusAfterConfirm, err := validateRequiredCleanString("browser.connection_status_after_confirm", proof.ConnectionStatusAfterConfirm)
	if err != nil {
		return false, err
	}
	if statusAfterConfirm != "connected" {
		return false, fmt.Errorf("browser.connection_status_after_confirm = %q, want connected", statusAfterConfirm)
	}
	if proof.ConfirmButtonDisabled == nil || !*proof.ConfirmButtonDisabled {
		return false, fmt.Errorf("browser.confirm_button_disabled_after_confirm must be true when confirmation is requested")
	}
	if proof.CloseReasonVisible == nil || !*proof.CloseReasonVisible {
		return false, fmt.Errorf("browser.close_reason_visible must be true when confirmation is requested")
	}
	if proof.ConclusionVersionVisible == nil || !*proof.ConclusionVersionVisible {
		return false, fmt.Errorf("browser.conclusion_version_visible must be true when confirmation is requested")
	}
	return true, nil
}

func validateBrowserPlannedEvidenceProof(proof browserProof) error {
	if proof.PlannedEvidenceRequested == nil || !*proof.PlannedEvidenceRequested {
		if proof.PlannedEvidenceTriggered != nil && *proof.PlannedEvidenceTriggered {
			return fmt.Errorf("browser.planned_evidence_collection_triggered must not be true when planned evidence collection is not requested")
		}
		if proof.PlannedEvidenceSatisfied != nil && *proof.PlannedEvidenceSatisfied {
			return fmt.Errorf("browser.planned_evidence_collection_satisfied must not be true when planned evidence collection is not requested")
		}
		return nil
	}
	if proof.PlannedEvidenceSatisfied == nil || !*proof.PlannedEvidenceSatisfied {
		return fmt.Errorf("browser.planned_evidence_collection_satisfied must be true when planned evidence collection is requested")
	}
	switch proof.PlannedEvidenceMode {
	case "manual_update":
		return validateBrowserManualPlannedEvidenceProof(proof)
	case "auto_collected":
		return validateBrowserAutoCollectedPlannedEvidenceProof(proof)
	case "already_final":
		return validateBrowserAlreadyFinalPlannedEvidenceProof(proof)
	default:
		return fmt.Errorf("browser.planned_evidence_collection_mode must be manual_update, auto_collected, or already_final")
	}
}

func validateBrowserManualPlannedEvidenceProof(proof browserProof) error {
	if proof.PlannedEvidenceAvailable == nil {
		return fmt.Errorf("browser.planned_evidence_collection_available is required when planned evidence collection is requested")
	}
	if !*proof.PlannedEvidenceAvailable {
		return fmt.Errorf("browser.planned_evidence_collection_available must be true when planned evidence collection is requested")
	}
	if proof.PlannedEvidenceActionCount <= 0 {
		return fmt.Errorf("browser.planned_evidence_collection_action_count must be > 0 when planned evidence collection is available")
	}
	switch proof.PlannedEvidenceTool {
	case "active_alerts", "metric_query", "metric_range_query":
	default:
		return fmt.Errorf("browser.planned_evidence_collection_tool must be active_alerts, metric_query, or metric_range_query")
	}
	if proof.PlannedEvidenceTriggered == nil || !*proof.PlannedEvidenceTriggered {
		return fmt.Errorf("browser.planned_evidence_collection_triggered must be true when planned evidence collection is requested")
	}
	if proof.PlannedEvidenceAssistantBefore < proof.AssistantTurnsBefore {
		return fmt.Errorf("browser.planned_evidence_assistant_turns_before must be >= browser.assistant_turns_before")
	}
	if proof.PlannedEvidenceAssistantAfter <= proof.PlannedEvidenceAssistantBefore {
		return fmt.Errorf("browser.planned_evidence_assistant_turns_after must be greater than planned_evidence_assistant_turns_before")
	}
	if proof.PlannedEvidenceAssistantAfter > proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.planned_evidence_assistant_turns_after must not exceed browser.assistant_turns_after")
	}
	if proof.PlannedEvidenceAssistantDelta != proof.PlannedEvidenceAssistantAfter-proof.PlannedEvidenceAssistantBefore {
		return fmt.Errorf("browser.planned_evidence_assistant_turn_delta must equal planned_evidence_assistant_turns_after - planned_evidence_assistant_turns_before")
	}
	if _, err := validateConfidenceValue("browser.planned_evidence_confidence_before", proof.PlannedEvidenceConfidenceBefore); err != nil {
		return err
	}
	plannedEvidenceConfidenceAfter, err := validateConfidenceValue(
		"browser.planned_evidence_confidence_after",
		proof.PlannedEvidenceConfidenceAfter,
	)
	if err != nil {
		return err
	}
	if proof.PlannedEvidenceAssistantAfter == proof.AssistantTurnsAfter && plannedEvidenceConfidenceAfter != proof.Confidence {
		return fmt.Errorf("browser.planned_evidence_confidence_after must match browser.confidence when planned evidence collection is the latest assistant turn")
	}
	if proof.PlannedEvidenceResultsBefore < 0 {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_before must be >= 0")
	}
	if proof.PlannedEvidenceResultsAfter < proof.PlannedEvidenceResultsBefore {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must be >= planned_evidence_collection_result_count_before")
	}
	if proof.PlannedEvidenceAssistantAfter == proof.AssistantTurnsAfter &&
		proof.PlannedEvidenceResultsAfter != proof.EvidenceCollectionResultCount {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must match evidence_collection_result_count when planned evidence collection is the latest assistant turn")
	}
	if proof.PlannedEvidenceTimelineVisible == nil || !*proof.PlannedEvidenceTimelineVisible {
		return fmt.Errorf("browser.planned_evidence_timeline_visible must be true when planned evidence collection is triggered")
	}
	return nil
}

func validateBrowserAutoCollectedPlannedEvidenceProof(proof browserProof) error {
	if proof.PlannedEvidenceAvailable == nil {
		return fmt.Errorf("browser.planned_evidence_collection_available is required when planned evidence collection is requested")
	}
	if *proof.PlannedEvidenceAvailable {
		return fmt.Errorf("browser.planned_evidence_collection_available must be false when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceActionCount != 0 {
		return fmt.Errorf("browser.planned_evidence_collection_action_count must be 0 when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceTriggered != nil && *proof.PlannedEvidenceTriggered {
		return fmt.Errorf("browser.planned_evidence_collection_triggered must not be true when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceResultsBefore < 0 {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_before must be >= 0 when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceResultsAfter < proof.PlannedEvidenceResultsBefore {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must be >= planned_evidence_collection_result_count_before when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceResultsAfter != proof.EvidenceCollectionResultCount {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must match evidence_collection_result_count when planned evidence was auto-collected")
	}
	if proof.PlannedEvidenceBackendResults < 0 {
		return fmt.Errorf("browser.planned_evidence_backend_collection_result_count must be >= 0")
	}
	if proof.PlannedEvidenceBackendCollected < 0 {
		return fmt.Errorf("browser.planned_evidence_backend_collected_result_count must be >= 0")
	}
	if proof.PlannedEvidenceBackendCollected > proof.PlannedEvidenceBackendResults {
		return fmt.Errorf("browser.planned_evidence_backend_collected_result_count must not exceed planned_evidence_backend_collection_result_count")
	}
	if proof.PlannedEvidenceResultsAfter == 0 && proof.PlannedEvidenceBackendCollected == 0 {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after or planned_evidence_backend_collected_result_count must prove collected evidence")
	}
	if _, err := validateConfidenceValue("browser.planned_evidence_confidence_after", proof.PlannedEvidenceConfidenceAfter); err != nil {
		return err
	}
	if proof.PlannedEvidenceSummaryVisible != nil && !*proof.PlannedEvidenceSummaryVisible {
		if strings.TrimSpace(proof.PlannedEvidenceSummaryText) != "" {
			return fmt.Errorf("browser.planned_evidence_collection_summary_text must be empty when planned evidence collection summary is not visible")
		}
		if plannedEvidenceFinalStateVisible(proof) {
			return nil
		}
	}
	summary, err := validateBoundedText(
		"browser.planned_evidence_collection_summary_text",
		proof.PlannedEvidenceSummaryText,
		maxProofBrowserCollectionSummaryTextBytes,
	)
	if err != nil {
		return err
	}
	if strings.HasPrefix(summary, "0/") || strings.Contains(summary, " 0/") {
		return fmt.Errorf("browser.planned_evidence_collection_summary_text must not report zero collected evidence")
	}
	return nil
}

func validateBrowserAlreadyFinalPlannedEvidenceProof(proof browserProof) error {
	if proof.PlannedEvidenceAvailable == nil {
		return fmt.Errorf("browser.planned_evidence_collection_available is required when planned evidence collection is requested")
	}
	if *proof.PlannedEvidenceAvailable {
		return fmt.Errorf("browser.planned_evidence_collection_available must be false when diagnosis is already final")
	}
	if proof.PlannedEvidenceActionCount != 0 {
		return fmt.Errorf("browser.planned_evidence_collection_action_count must be 0 when diagnosis is already final")
	}
	if strings.TrimSpace(proof.PlannedEvidenceTool) != "" {
		return fmt.Errorf("browser.planned_evidence_collection_tool must be empty when diagnosis is already final")
	}
	if proof.PlannedEvidenceTriggered != nil && *proof.PlannedEvidenceTriggered {
		return fmt.Errorf("browser.planned_evidence_collection_triggered must not be true when diagnosis is already final")
	}
	if !plannedEvidenceFinalStateVisible(proof) {
		return fmt.Errorf("browser.planned_evidence_final_conclusion_visible or browser.planned_evidence_ready_for_confirmation_visible must be true when diagnosis is already final")
	}
	if proof.PlannedEvidenceResultsBefore < 0 {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_before must be >= 0")
	}
	if proof.PlannedEvidenceResultsAfter != proof.PlannedEvidenceResultsBefore {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must match planned_evidence_collection_result_count_before when diagnosis is already final")
	}
	if proof.PlannedEvidenceResultsAfter != proof.EvidenceCollectionResultCount {
		return fmt.Errorf("browser.planned_evidence_collection_result_count_after must match evidence_collection_result_count when diagnosis is already final")
	}
	if proof.PlannedEvidenceAssistantAfter <= 0 {
		return fmt.Errorf("browser.planned_evidence_assistant_turns_after must be > 0 when diagnosis is already final")
	}
	if proof.PlannedEvidenceAssistantAfter != proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.planned_evidence_assistant_turns_after must match browser.assistant_turns_after when diagnosis is already final")
	}
	plannedEvidenceConfidenceAfter, err := validateConfidenceValue(
		"browser.planned_evidence_confidence_after",
		proof.PlannedEvidenceConfidenceAfter,
	)
	if err != nil {
		return err
	}
	if plannedEvidenceConfidenceAfter != proof.Confidence {
		return fmt.Errorf("browser.planned_evidence_confidence_after must match browser.confidence when diagnosis is already final")
	}
	if proof.PlannedEvidenceSummaryVisible != nil {
		if *proof.PlannedEvidenceSummaryVisible {
			if _, err := validateBoundedText(
				"browser.planned_evidence_collection_summary_text",
				proof.PlannedEvidenceSummaryText,
				maxProofBrowserCollectionSummaryTextBytes,
			); err != nil {
				return err
			}
		} else if strings.TrimSpace(proof.PlannedEvidenceSummaryText) != "" {
			return fmt.Errorf("browser.planned_evidence_collection_summary_text must be empty when planned evidence collection summary is not visible")
		}
	}
	return nil
}

func plannedEvidenceFinalStateVisible(proof browserProof) bool {
	return (proof.PlannedEvidenceFinalVisible != nil && *proof.PlannedEvidenceFinalVisible) ||
		(proof.PlannedEvidenceReadyVisible != nil && *proof.PlannedEvidenceReadyVisible)
}

func validateBrowserOperatorSeedCollectionProof(proof browserProof) error {
	if proof.OperatorSeedCollectionRequested == nil || !*proof.OperatorSeedCollectionRequested {
		if proof.OperatorSeedCollectionTriggered != nil && *proof.OperatorSeedCollectionTriggered {
			return fmt.Errorf("browser.operator_seed_collection_triggered must not be true when operator seed collection is not requested")
		}
		if proof.OperatorSeedCollectionCount != 0 {
			return fmt.Errorf("browser.operator_seed_collection_count must be 0 when operator seed collection is not requested")
		}
		return nil
	}
	if !proof.ToolRequestSeedRequested {
		return fmt.Errorf("browser.operator_seed_collection_requested requires tool_request_seed_requested")
	}
	if proof.OperatorSeedCollectionCount != proof.ToolRequestSeedCount {
		return fmt.Errorf("browser.operator_seed_collection_count must match tool_request_seed_count")
	}
	if proof.OperatorSeedCollectionTriggered == nil || !*proof.OperatorSeedCollectionTriggered {
		return fmt.Errorf("browser.operator_seed_collection_triggered must be true when operator seed collection is requested")
	}
	if proof.OperatorSeedCollectionBefore < 0 {
		return fmt.Errorf("browser.operator_seed_collection_result_count_before must be >= 0")
	}
	if proof.OperatorSeedCollectionAfter <= 0 {
		return fmt.Errorf("browser.operator_seed_collection_result_count_after must be > 0")
	}
	if proof.OperatorSeedCollectionAfter > proof.EvidenceCollectionResultCount {
		return fmt.Errorf("browser.operator_seed_collection_result_count_after must not exceed evidence_collection_result_count")
	}
	if proof.OperatorSeedAssistantBefore < proof.AssistantTurnsBefore {
		return fmt.Errorf("browser.operator_seed_collection_assistant_turns_before must be within assistant turn bounds")
	}
	if proof.OperatorSeedAssistantAfter <= proof.OperatorSeedAssistantBefore {
		return fmt.Errorf("browser.operator_seed_collection_assistant_turns_after must be greater than before")
	}
	if proof.OperatorSeedAssistantAfter > proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.operator_seed_collection_assistant_turns_after must not exceed assistant_turns_after")
	}
	if proof.OperatorSeedAssistantDelta != proof.OperatorSeedAssistantAfter-proof.OperatorSeedAssistantBefore {
		return fmt.Errorf("browser.operator_seed_collection_assistant_turn_delta must equal after - before")
	}
	if _, err := validateConfidenceValue("browser.operator_seed_collection_confidence_before", proof.OperatorSeedConfidenceBefore); err != nil {
		return err
	}
	operatorConfidenceAfter, err := validateConfidenceValue(
		"browser.operator_seed_collection_confidence_after",
		proof.OperatorSeedConfidenceAfter,
	)
	if err != nil {
		return err
	}
	if proof.OperatorSeedAssistantAfter == proof.AssistantTurnsAfter && operatorConfidenceAfter != proof.Confidence {
		return fmt.Errorf("browser.operator_seed_collection_confidence_after must match browser.confidence")
	}
	if strings.TrimSpace(proof.OperatorSeedCollectionSummary) != "" {
		if _, err := validateBoundedText(
			"browser.operator_seed_collection_summary_text",
			proof.OperatorSeedCollectionSummary,
			maxProofBrowserCollectionSummaryTextBytes,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateBrowserSupplementalEvidenceProof(proof browserProof) error {
	if proof.SupplementalEvidenceRequested == nil || !*proof.SupplementalEvidenceRequested {
		if proof.SupplementalEvidenceSubmitted != nil && *proof.SupplementalEvidenceSubmitted {
			return fmt.Errorf("browser.supplemental_evidence_submitted must not be true when supplemental evidence is not requested")
		}
		return nil
	}
	if proof.SupplementalEvidenceRequired == nil {
		return fmt.Errorf("browser.supplemental_evidence_required is required when supplemental evidence is requested")
	}
	if proof.SupplementalFollowUpAvailable == nil {
		return fmt.Errorf("browser.supplemental_follow_up_available is required when supplemental evidence is requested")
	}
	if !*proof.SupplementalFollowUpAvailable {
		if proof.SupplementalEvidenceSubmitted != nil && *proof.SupplementalEvidenceSubmitted {
			return fmt.Errorf("browser.supplemental_evidence_submitted must not be true when no follow-up is available")
		}
		if *proof.SupplementalEvidenceRequired {
			return fmt.Errorf("browser.supplemental_follow_up_available must be true when supplemental evidence is required")
		}
		if _, err := validateBoundedCleanString(
			"browser.supplemental_block_reason",
			proof.SupplementalBlockReason,
			maxProofBrowserSupplementalBlockReasonBytes,
		); err != nil {
			return err
		}
		return nil
	}
	if proof.SupplementalFollowUpCount <= 0 {
		return fmt.Errorf("browser.supplemental_follow_up_count must be > 0 when follow-up is available")
	}
	if _, err := validateBoundedCleanString(
		"browser.supplemental_request_label",
		proof.SupplementalRequestLabel,
		maxProofBrowserSupplementalLabelBytes,
	); err != nil {
		return err
	}
	if proof.SupplementalEvidenceSubmitted == nil || !*proof.SupplementalEvidenceSubmitted {
		return fmt.Errorf("browser.supplemental_evidence_submitted must be true when follow-up is available")
	}
	if proof.SupplementalEvidenceLength <= 0 {
		return fmt.Errorf("browser.supplemental_evidence_length must be > 0 when supplemental evidence is submitted")
	}
	if _, err := validateSHA256Hex("browser.supplemental_evidence_sha256", proof.SupplementalEvidenceSHA256); err != nil {
		return err
	}
	if proof.SupplementalAssistantBefore < proof.AssistantTurnsBefore {
		return fmt.Errorf("browser.supplemental_assistant_turns_before must be >= browser.assistant_turns_before")
	}
	if proof.SupplementalAssistantAfter != proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.supplemental_assistant_turns_after must match browser.assistant_turns_after")
	}
	if proof.SupplementalAssistantAfter <= proof.SupplementalAssistantBefore {
		return fmt.Errorf("browser.supplemental_assistant_turns_after must be greater than supplemental_assistant_turns_before")
	}
	if proof.SupplementalAssistantDelta != proof.SupplementalAssistantAfter-proof.SupplementalAssistantBefore {
		return fmt.Errorf("browser.supplemental_assistant_turn_delta must equal supplemental_assistant_turns_after - supplemental_assistant_turns_before")
	}
	if proof.SupplementalAssistantAfter > proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.supplemental_assistant_turns_after must not exceed browser.assistant_turns_after")
	}
	if strings.TrimSpace(proof.SupplementalCompletionEvidence) != "" {
		completionEvidence, err := validateBoundedCleanString(
			"browser.supplemental_completion_evidence_after",
			proof.SupplementalCompletionEvidence,
			maxProofBrowserCompletionEvidenceBytes,
		)
		if err != nil {
			return err
		}
		turnNumber, ok := completedTurnNumber(completionEvidence)
		if !ok {
			return fmt.Errorf("browser.supplemental_completion_evidence_after must match a completed turn or loaded state log entry")
		}
		if turnNumber != proof.SupplementalAssistantAfter {
			return fmt.Errorf("browser.supplemental_completion_evidence_after must match browser.supplemental_assistant_turns_after")
		}
	}
	if _, err := validateConfidenceValue("browser.supplemental_confidence_before", proof.SupplementalConfidenceBefore); err != nil {
		return err
	}
	supplementalConfidenceAfter, err := validateConfidenceValue(
		"browser.supplemental_confidence_after",
		proof.SupplementalConfidenceAfter,
	)
	if err != nil {
		return err
	}
	if supplementalConfidenceAfter != proof.Confidence {
		return fmt.Errorf("browser.supplemental_confidence_after must match browser.confidence")
	}
	if proof.SupplementalHistoryVisible == nil || !*proof.SupplementalHistoryVisible {
		return fmt.Errorf("browser.supplemental_history_visible must be true when supplemental evidence is submitted")
	}
	if proof.SupplementalHistoryBefore < 0 {
		return fmt.Errorf("browser.supplemental_history_count_before must be >= 0")
	}
	if proof.SupplementalHistoryAfter <= proof.SupplementalHistoryBefore {
		return fmt.Errorf("browser.supplemental_history_count_after must be greater than supplemental_history_count_before")
	}
	if proof.SupplementalReviewQueueVisible == nil || !*proof.SupplementalReviewQueueVisible {
		return fmt.Errorf("browser.supplemental_review_queue_visible must be true when supplemental evidence is submitted")
	}
	if proof.SupplementalReviewQueueItemCount <= 0 {
		return fmt.Errorf("browser.supplemental_review_queue_item_count must be > 0 when supplemental evidence is submitted")
	}
	if proof.SupplementalConfirmAvailable == nil {
		return fmt.Errorf("browser.supplemental_confirm_conclusion_available_after is required when supplemental evidence is submitted")
	}
	if !*proof.SupplementalConfirmAvailable {
		if _, err := validateBoundedCleanString(
			"browser.supplemental_confirm_block_reason_after",
			proof.SupplementalConfirmBlockReason,
			maxProofBrowserConfirmBlockReasonBytes,
		); err != nil {
			return err
		}
	} else if strings.TrimSpace(proof.SupplementalConfirmBlockReason) != "" {
		return fmt.Errorf("browser.supplemental_confirm_block_reason_after must be empty when supplemental confirmation is available")
	}
	return nil
}

func browserCollectedPlannedEvidence(proof browserProof) bool {
	return proof.PlannedEvidenceRequested != nil &&
		*proof.PlannedEvidenceRequested &&
		proof.PlannedEvidenceMode == "manual_update" &&
		proof.PlannedEvidenceTriggered != nil &&
		*proof.PlannedEvidenceTriggered
}

func browserCollectedOperatorSeedEvidence(proof browserProof) bool {
	return proof.OperatorSeedCollectionRequested != nil &&
		*proof.OperatorSeedCollectionRequested &&
		proof.OperatorSeedCollectionTriggered != nil &&
		*proof.OperatorSeedCollectionTriggered
}

func browserCompletedTurnTextMayLag(proof browserProof, turnNumber int) bool {
	if browserSubmittedSupplementalEvidence(proof) {
		return false
	}
	if browserCollectedPlannedEvidence(proof) &&
		proof.PlannedEvidenceAssistantBefore == turnNumber &&
		proof.PlannedEvidenceAssistantAfter == proof.AssistantTurnsAfter {
		return true
	}
	return browserCollectedOperatorSeedEvidence(proof) &&
		proof.OperatorSeedAssistantBefore == turnNumber &&
		proof.OperatorSeedAssistantAfter == proof.AssistantTurnsAfter
}

func browserSatisfiedPlannedEvidence(proof browserProof) bool {
	return proof.PlannedEvidenceRequested != nil &&
		*proof.PlannedEvidenceRequested &&
		proof.PlannedEvidenceSatisfied != nil &&
		*proof.PlannedEvidenceSatisfied
}

func browserSubmittedSupplementalEvidence(proof browserProof) bool {
	return proof.SupplementalEvidenceRequested != nil &&
		*proof.SupplementalEvidenceRequested &&
		proof.SupplementalEvidenceSubmitted != nil &&
		*proof.SupplementalEvidenceSubmitted
}

func validateCreatedRoom(room createdRoom, sessionID string, proofEvidenceSnapshotID int64, hasProofEvidenceSnapshotID bool) error {
	roomSessionID, err := validateProofID("created_room.session_id", room.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if roomSessionID != sessionID {
		return fmt.Errorf("created_room.session_id must match session_id")
	}
	if room.EvidenceSnapshotID <= 0 {
		return fmt.Errorf("created_room.evidence_snapshot_id must be > 0")
	}
	if hasProofEvidenceSnapshotID && room.EvidenceSnapshotID != proofEvidenceSnapshotID {
		return fmt.Errorf("created_room.evidence_snapshot_id must match evidence_snapshot_id")
	}
	if room.DiagnosisTaskID <= 0 {
		return fmt.Errorf("created_room.diagnosis_task_id must be > 0")
	}
	if room.ChatSessionID <= 0 {
		return fmt.Errorf("created_room.chat_session_id must be > 0")
	}
	workflowID, err := validateProofID("created_room.workflow_id", room.WorkflowID, maxProofWorkflowIDBytes)
	if err != nil {
		return err
	}
	if workflowID != "diagnosis-room-"+sessionID {
		return fmt.Errorf("created_room.workflow_id must match the session workflow id")
	}
	if _, err := validateProofID("created_room.run_id", room.RunID, maxProofRunIDBytes); err != nil {
		return err
	}
	return nil
}

func validateCloseProof(proof closeProof, sessionID string, assistantTurnsAfter int, room *createdRoom) error {
	if _, err := validateCanonicalUTCTime("close_notification.checked_at", proof.CheckedAt); err != nil {
		return err
	}
	if !proof.Signaled {
		return fmt.Errorf("close_notification.signaled must be true")
	}
	requestSessionID, err := validateProofID("close_notification.request.session_id", proof.Request.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if requestSessionID != sessionID {
		return fmt.Errorf("close_notification.request.session_id must match session_id")
	}
	workflowID, err := validateProofID("close_notification.request.workflow_id", proof.Request.WorkflowID, maxProofWorkflowIDBytes)
	if err != nil {
		return err
	}
	if workflowID != "diagnosis-room-"+sessionID {
		return fmt.Errorf("close_notification.request.workflow_id must match the session workflow id")
	}
	if proof.Request.RunID != "" {
		if _, err := validateProofID("close_notification.request.run_id", proof.Request.RunID, maxProofRunIDBytes); err != nil {
			return err
		}
		if room != nil && proof.Request.RunID != room.RunID {
			return fmt.Errorf("close_notification.request.run_id must match created_room.run_id")
		}
	}
	reason, err := validateBoundedCleanString("close_notification.request.reason", proof.Request.Reason, maxProofCloseReasonBytes)
	if err != nil {
		return err
	}
	waitTimeout, err := validateRequiredCleanString("close_notification.request.wait_timeout", proof.Request.WaitTimeout)
	if err != nil {
		return err
	}
	parsedWaitTimeout, err := time.ParseDuration(waitTimeout)
	if err != nil || parsedWaitTimeout <= 0 {
		return fmt.Errorf("close_notification.request.wait_timeout must be a positive duration")
	}

	workflowSessionID, err := validateProofID("close_notification.workflow.session_id", proof.Workflow.SessionID, maxProofSessionIDBytes)
	if err != nil {
		return err
	}
	if workflowSessionID != sessionID {
		return fmt.Errorf("close_notification.workflow.session_id must match session_id")
	}
	if proof.Workflow.ChatSessionID <= 0 {
		return fmt.Errorf("close_notification.workflow.chat_session_id must be > 0")
	}
	if proof.Workflow.DiagnosisTaskID <= 0 {
		return fmt.Errorf("close_notification.workflow.diagnosis_task_id must be > 0")
	}
	if room != nil {
		if proof.Workflow.ChatSessionID != room.ChatSessionID {
			return fmt.Errorf("close_notification.workflow.chat_session_id must match created_room.chat_session_id")
		}
		if proof.Workflow.DiagnosisTaskID != room.DiagnosisTaskID {
			return fmt.Errorf("close_notification.workflow.diagnosis_task_id must match created_room.diagnosis_task_id")
		}
	}
	status, err := validateRequiredCleanString("close_notification.workflow.status", proof.Workflow.Status)
	if err != nil {
		return err
	}
	if status != "closed" {
		return fmt.Errorf("close_notification.workflow.status = %q, want closed", status)
	}
	if proof.Workflow.TurnCount != assistantTurnsAfter {
		return fmt.Errorf("close_notification.workflow.turn_count must match browser.assistant_turns_after")
	}
	closedAt, err := validateCanonicalUTCTime("close_notification.workflow.closed_at", proof.Workflow.ClosedAt)
	if err != nil {
		return err
	}
	closeReason, err := validateBoundedCleanString("close_notification.workflow.close_reason", proof.Workflow.CloseReason, maxProofCloseReasonBytes)
	if err != nil {
		return err
	}
	if closeReason != reason {
		return fmt.Errorf("close_notification.workflow.close_reason must match close_notification.request.reason")
	}
	if err := validateCloseFinalConclusion(
		"close_notification.workflow.final_conclusion",
		proof.Workflow.FinalConclusion,
		assistantTurnsAfter,
		closedAt,
	); err != nil {
		return err
	}

	closeEventAt, err := validateCloseProofEvent("close_notification.close_event", proof.CloseEvent, closeNotificationClosedKind)
	if err != nil {
		return err
	}
	if closeEventAt.Before(closedAt) {
		return fmt.Errorf("close_notification.close_event.occurred_at must not be before workflow.closed_at")
	}
	if proof.CloseEvent.ConclusionVersion != "diagnosis-room-close.v1" {
		return fmt.Errorf("close_notification.close_event.conclusion_version = %q, want diagnosis-room-close.v1", proof.CloseEvent.ConclusionVersion)
	}
	if err := validateCloseFinalConclusion(
		"close_notification.close_event.final_conclusion",
		proof.CloseEvent.FinalConclusion,
		assistantTurnsAfter,
		closedAt,
	); err != nil {
		return err
	}
	if !sameCloseFinalConclusion(proof.Workflow.FinalConclusion, proof.CloseEvent.FinalConclusion) {
		return fmt.Errorf("close_notification.close_event.final_conclusion must match close_notification.workflow.final_conclusion")
	}
	notificationEventAt, err := validateCloseNotificationEvent(
		proof.NotificationEvent,
		closeNotificationSentKind,
		proof.Workflow.DiagnosisTaskID,
	)
	if err != nil {
		return err
	}
	if notificationEventAt.Before(closeEventAt) {
		return fmt.Errorf("close_notification.notification_event.occurred_at must not be before close_event.occurred_at")
	}
	if proof.NotificationEvent.ID == proof.CloseEvent.ID {
		return fmt.Errorf("close_notification.notification_event.id must differ from close_event.id")
	}
	return nil
}

func validateCloseFinalConclusion(field string, conclusion closeFinalConclusion, assistantTurnsAfter int, closedAt time.Time) error {
	status, err := validateRequiredCleanString(field+".status", conclusion.Status)
	if err != nil {
		return err
	}
	source, err := validateRequiredCleanString(field+".source", conclusion.Source)
	if err != nil {
		return err
	}
	if conclusion.EvidenceSnapshotID <= 0 {
		return fmt.Errorf("%s.evidence_snapshot_id must be > 0", field)
	}
	if conclusion.ConclusionVersion != "diagnosis-room-close.v1" {
		return fmt.Errorf("%s.conclusion_version = %q, want diagnosis-room-close.v1", field, conclusion.ConclusionVersion)
	}
	recordedAt, err := validateCanonicalUTCTime(field+".recorded_at", conclusion.RecordedAt)
	if err != nil {
		return err
	}
	if !recordedAt.Equal(closedAt) {
		return fmt.Errorf("%s.recorded_at must match workflow.closed_at", field)
	}
	if conclusion.ConfirmedBy != "" {
		if _, err := validateBoundedCleanString(field+".confirmed_by", conclusion.ConfirmedBy, maxProofSessionIDBytes); err != nil {
			return err
		}
	}
	if assistantTurnsAfter > 0 {
		if status != "available" {
			return fmt.Errorf("%s.status = %q, want available", field, status)
		}
		if source != "latest_assistant_turn" {
			return fmt.Errorf("%s.source = %q, want latest_assistant_turn", field, source)
		}
		if conclusion.Reason != "" {
			return fmt.Errorf("%s.reason must be empty when status is available", field)
		}
		if conclusion.AssistantTurnID <= 0 {
			return fmt.Errorf("%s.assistant_turn_id must be > 0", field)
		}
		if _, err := validateProofID(field+".assistant_message_id", conclusion.AssistantMessageID, maxProofIdempotencyKeyBytes); err != nil {
			return err
		}
		wantAssistantSequence := assistantTurnsAfter * 2
		if conclusion.AssistantSequence != wantAssistantSequence {
			return fmt.Errorf("%s.assistant_sequence = %d, want %d", field, conclusion.AssistantSequence, wantAssistantSequence)
		}
		assistantOccurredAt, err := validateCanonicalUTCTime(field+".assistant_occurred_at", conclusion.AssistantOccurredAt)
		if err != nil {
			return err
		}
		if assistantOccurredAt.After(closedAt) {
			return fmt.Errorf("%s.assistant_occurred_at must not be after workflow.closed_at", field)
		}
		if _, err := validateBoundedText(field+".content", conclusion.Content, maxProofFinalConclusionContentBytes); err != nil {
			return err
		}
		confidence, err := validateRequiredCleanString(field+".confidence", conclusion.Confidence)
		if err != nil {
			return err
		}
		switch confidence {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("%s.confidence = %q, want low, medium, or high", field, confidence)
		}
		if conclusion.RequiresHumanReview == nil {
			return fmt.Errorf("%s.requires_human_review is required when status is available", field)
		}
		if len(conclusion.SupplementalContextRefs) == 0 {
			return fmt.Errorf("%s.supplemental_context_refs must not be empty when status is available", field)
		}
		for i, ref := range conclusion.SupplementalContextRefs {
			if _, err := validateProofID(fmt.Sprintf("%s.supplemental_context_refs[%d]", field, i), ref, maxProofIdempotencyKeyBytes); err != nil {
				return err
			}
		}
		return nil
	}

	if status != "not_available" {
		return fmt.Errorf("%s.status = %q, want not_available", field, status)
	}
	if source != "none" {
		return fmt.Errorf("%s.source = %q, want none", field, source)
	}
	reason, err := validateRequiredCleanString(field+".reason", conclusion.Reason)
	if err != nil {
		return err
	}
	if reason != "room_closed_without_assistant_turn" {
		return fmt.Errorf("%s.reason = %q, want room_closed_without_assistant_turn", field, reason)
	}
	if conclusion.AssistantTurnID != 0 ||
		conclusion.AssistantMessageID != "" ||
		conclusion.AssistantSequence != 0 ||
		conclusion.AssistantOccurredAt != "" ||
		conclusion.Content != "" ||
		conclusion.Confidence != "" ||
		conclusion.RequiresHumanReview != nil ||
		len(conclusion.SupplementalContextRefs) != 0 {
		return fmt.Errorf("%s must not include assistant-turn fields when status is not_available", field)
	}
	return nil
}

func sameCloseFinalConclusion(a, b closeFinalConclusion) bool {
	if a.Status != b.Status ||
		a.Source != b.Source ||
		a.Reason != b.Reason ||
		a.EvidenceSnapshotID != b.EvidenceSnapshotID ||
		a.ConclusionVersion != b.ConclusionVersion ||
		a.RecordedAt != b.RecordedAt ||
		a.ConfirmedBy != b.ConfirmedBy ||
		a.AssistantTurnID != b.AssistantTurnID ||
		a.AssistantMessageID != b.AssistantMessageID ||
		a.AssistantSequence != b.AssistantSequence ||
		a.AssistantOccurredAt != b.AssistantOccurredAt ||
		a.Content != b.Content ||
		a.Confidence != b.Confidence {
		return false
	}
	if len(a.SupplementalContextRefs) != len(b.SupplementalContextRefs) {
		return false
	}
	for i := range a.SupplementalContextRefs {
		if a.SupplementalContextRefs[i] != b.SupplementalContextRefs[i] {
			return false
		}
	}
	switch {
	case a.RequiresHumanReview == nil && b.RequiresHumanReview == nil:
		return true
	case a.RequiresHumanReview == nil || b.RequiresHumanReview == nil:
		return false
	default:
		return *a.RequiresHumanReview == *b.RequiresHumanReview
	}
}

func validateCloseProofEvent(field string, event closeProofEvent, wantKind string) (time.Time, error) {
	if event.ID <= 0 {
		return time.Time{}, fmt.Errorf("%s.id must be > 0", field)
	}
	kind, err := validateRequiredCleanString(field+".kind", event.Kind)
	if err != nil {
		return time.Time{}, err
	}
	if kind != wantKind {
		return time.Time{}, fmt.Errorf("%s.kind = %q, want %s", field, kind, wantKind)
	}
	occurredAt, err := validateCanonicalUTCTime(field+".occurred_at", event.OccurredAt)
	if err != nil {
		return time.Time{}, err
	}
	return occurredAt, nil
}

func validateCloseNotificationEvent(event closeNotificationEvent, wantKind string, diagnosisTaskID int64) (time.Time, error) {
	if event.ID <= 0 {
		return time.Time{}, fmt.Errorf("close_notification.notification_event.id must be > 0")
	}
	kind, err := validateRequiredCleanString("close_notification.notification_event.kind", event.Kind)
	if err != nil {
		return time.Time{}, err
	}
	if kind != wantKind {
		return time.Time{}, fmt.Errorf("close_notification.notification_event.kind = %q, want %s", kind, wantKind)
	}
	occurredAt, err := validateCanonicalUTCTime("close_notification.notification_event.occurred_at", event.OccurredAt)
	if err != nil {
		return time.Time{}, err
	}
	idempotencyKey, err := validateBoundedCleanString(
		"close_notification.notification_event.idempotency_key",
		event.IdempotencyKey,
		maxProofIdempotencyKeyBytes,
	)
	if err != nil {
		return time.Time{}, err
	}
	wantPrefix := "diagnosis_room:" + strconv.FormatInt(diagnosisTaskID, 10) + ":"
	if !strings.HasPrefix(idempotencyKey, wantPrefix) || !strings.HasSuffix(idempotencyKey, "/close_notification") {
		return time.Time{}, fmt.Errorf("close_notification.notification_event.idempotency_key must match diagnosis-room close notification format")
	}
	if event.ProviderMessageID != "" {
		if _, err := validateBoundedCleanString(
			"close_notification.notification_event.provider_message_id",
			event.ProviderMessageID,
			maxProofProviderMessageBytes,
		); err != nil {
			return time.Time{}, err
		}
	}
	status, err := validateRequiredCleanString("close_notification.notification_event.provider_status", event.ProviderStatus)
	if err != nil {
		return time.Time{}, err
	}
	switch status {
	case "accepted", "delivered":
		return occurredAt, nil
	default:
		return time.Time{}, fmt.Errorf("close_notification.notification_event.provider_status = %q, want accepted or delivered", status)
	}
}

func validateNotificationProof(proof *notificationProof, expectedChannelID int64, required bool) error {
	if proof == nil {
		if required {
			return fmt.Errorf("notification_proof is required")
		}
		return nil
	}
	if !required && !proof.Requested {
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
	if _, err := validateCanonicalUTCTime("notification_proof.checked_at", proof.CheckedAt); err != nil {
		return err
	}
	if len(proof.Entries) == 0 {
		return fmt.Errorf("notification_proof.entries must be non-empty")
	}
	seenEvents := map[string]bool{}
	for i, entry := range proof.Entries {
		if err := validateNotificationProofEntry(entry, expectedChannelID); err != nil {
			return fmt.Errorf("notification_proof.entries[%d]: %w", i, err)
		}
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
	if _, err := validateSHA256Hex("content_sha256", entry.ContentSHA256); err != nil {
		return err
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
		if _, err := validateBoundedCleanString("provider_message_id", entry.ProviderMessageID, maxProofProviderMessageBytes); err != nil {
			return err
		}
	}
	if entry.AssistantMessageID != "" {
		if _, err := validateProofID("assistant_message_id", entry.AssistantMessageID, maxProofIdempotencyKeyBytes); err != nil {
			return err
		}
	}
	_, err = validateCanonicalUTCTime("occurred_at", entry.OccurredAt)
	return err
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

func validateOptionalNonNegativeInt64(field string, raw json.RawMessage) error {
	valueBytes := bytes.TrimSpace(raw)
	if len(valueBytes) == 0 || string(valueBytes) == "null" {
		return nil
	}
	if !bytes.Equal(valueBytes, raw) {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if valueBytes[0] == '"' {
		return fmt.Errorf("%s must be null or a non-negative integer", field)
	}
	var number json.Number
	dec := json.NewDecoder(strings.NewReader(string(valueBytes)))
	dec.UseNumber()
	if err := dec.Decode(&number); err != nil {
		return fmt.Errorf("%s must be null or a non-negative integer: %w", field, err)
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil || value < 0 {
		return fmt.Errorf("%s must be null or a non-negative integer", field)
	}
	return nil
}

func completedTurnNumber(value string) (int, bool) {
	if strings.HasPrefix(value, "Turn ") && strings.HasSuffix(value, " completed.") {
		middle := strings.TrimSuffix(strings.TrimPrefix(value, "Turn "), " completed.")
		return positiveCanonicalInt(middle)
	}
	if strings.HasPrefix(value, "Loaded state: ") && strings.HasSuffix(value, " turn(s).") {
		comma := strings.LastIndex(value, ", ")
		if comma < 0 {
			return 0, false
		}
		middle := strings.TrimSuffix(value[comma+2:], " turn(s).")
		return positiveCanonicalInt(middle)
	}
	return 0, false
}

func positiveCanonicalInt(raw string) (int, bool) {
	middle := strings.TrimSpace(raw)
	if middle == "" {
		return 0, false
	}
	number, err := strconv.Atoi(middle)
	return number, err == nil && number > 0 && strconv.Itoa(number) == middle
}

func validateHTTPURL(field, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", field, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", field)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", field)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not include user info", field)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("%s must not include a query string", field)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("%s must not include a fragment", field)
	}
	return nil
}

func validateCheckedAt(raw string) (time.Time, error) {
	return validateCanonicalUTCTime("checked_at", raw)
}

func validateCanonicalUTCTime(field, raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return time.Time{}, fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("%s must be canonical UTC RFC3339", field)
	}
	if checkedAt.After(nowUTC()) {
		return time.Time{}, fmt.Errorf("%s must not be in the future", field)
	}
	return checkedAt.UTC(), nil
}

func validateRequiredCleanString(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "", fmt.Errorf("%s must be a single-line value", field)
	}
	return value, nil
}

func validateBoundedCleanString(field, raw string, maxBytes int) (string, error) {
	value, err := validateRequiredCleanString(field, raw)
	if err != nil {
		return "", err
	}
	if len(raw) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return value, nil
}

func validateBoundedText(field, raw string, maxBytes int) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if len(raw) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return value, nil
}

func validateConfidenceValue(field, raw string) (string, error) {
	confidence, err := validateRequiredCleanString(field, raw)
	if err != nil {
		return "", err
	}
	switch confidence {
	case "low", "medium", "high":
		return confidence, nil
	default:
		return "", fmt.Errorf("%s = %q, want low, medium, or high", field, confidence)
	}
}

func validateProofID(field, raw string, maxBytes int) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, " \r\n\t") {
		return "", fmt.Errorf("%s must not contain whitespace", field)
	}
	if len(raw) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return raw, nil
}

func validateSHA256Hex(field, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return "", fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if len(value) != 64 {
		return "", fmt.Errorf("%s must be a lowercase SHA-256 hex digest", field)
	}
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') {
			continue
		}
		return "", fmt.Errorf("%s must be a lowercase SHA-256 hex digest", field)
	}
	return value, nil
}

func optionalPositiveInt64(raw json.RawMessage) (int64, bool, error) {
	valueBytes := bytes.TrimSpace(raw)
	if len(valueBytes) == 0 || string(valueBytes) == "null" {
		return 0, false, nil
	}
	if !bytes.Equal(valueBytes, raw) {
		return 0, false, fmt.Errorf("must not contain leading or trailing whitespace")
	}
	if valueBytes[0] == '"' {
		return 0, false, fmt.Errorf("must be null or a positive integer")
	}

	var number json.Number
	dec := json.NewDecoder(strings.NewReader(string(valueBytes)))
	dec.UseNumber()
	if err := dec.Decode(&number); err != nil {
		return 0, false, fmt.Errorf("must be null or a positive integer")
	}
	value, err := number.Int64()
	if err != nil || value <= 0 || number.String() != strconv.FormatInt(value, 10) {
		return 0, false, fmt.Errorf("must be a positive integer")
	}
	return value, true, nil
}
