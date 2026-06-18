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
	maxProofBytes                       int64 = 10 * 1024 * 1024
	maxProofSessionIDBytes                    = 128
	maxProofWorkflowIDBytes                   = 256
	maxProofRunIDBytes                        = 256
	maxProofEvidenceBytes                     = 512
	maxProofCloseReasonBytes                  = 128
	maxProofIdempotencyKeyBytes               = 256
	maxProofProviderMessageBytes              = 512
	maxProofFinalConclusionContentBytes       = 4096
	maxProofBrowserReadinessTextBytes         = 512

	closeNotificationClosedKind = "diagnosis_room.closed"
	closeNotificationSentKind   = "diagnosis_room.close_notification_sent"
)

type smokeOutput struct {
	Passed             bool            `json:"passed"`
	CheckedAt          string          `json:"checked_at"`
	Request            proofRequest    `json:"request"`
	WebBaseURL         string          `json:"web_base_url"`
	APIBaseURL         string          `json:"api_base_url"`
	SessionID          string          `json:"session_id"`
	EvidenceSnapshotID json.RawMessage `json:"evidence_snapshot_id"`
	CreatedRoom        *createdRoom    `json:"created_room"`
	MessageLength      int             `json:"message_length"`
	MessageSHA256      string          `json:"message_sha256"`
	Browser            *browserProof   `json:"browser"`
	CloseNotification  *closeProof     `json:"close_notification"`
	Evidence           string          `json:"evidence"`
}

type proofRequest struct {
	Mode               string          `json:"mode"`
	SessionID          string          `json:"session_id"`
	EvidenceSnapshotID json.RawMessage `json:"evidence_snapshot_id"`
	MessageLength      int             `json:"message_length"`
	MessageSHA256      string          `json:"message_sha256"`
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
	StateLoaded                 bool   `json:"state_loaded"`
	TurnResultObserved          bool   `json:"turn_result_observed"`
	AssistantTurnsBefore        int    `json:"assistant_turns_before"`
	AssistantTurnsAfter         int    `json:"assistant_turns_after"`
	AssistantTurnDelta          int    `json:"assistant_turn_delta"`
	TranscriptMessagesBefore    int    `json:"transcript_messages_before"`
	TranscriptMessagesAfter     int    `json:"transcript_messages_after"`
	ConnectionStatusAfterTurn   string `json:"connection_status_after_turn"`
	SubmittedMessageVisible     bool   `json:"submitted_message_visible"`
	SubmittedMessageLength      int    `json:"submitted_message_length"`
	SubmittedMessageSHA256      string `json:"submitted_message_sha256"`
	CompletedTurnText           string `json:"completed_turn_text"`
	ConsultationInsightVisible  bool   `json:"consultation_insight_visible"`
	ConsultationProgressVisible bool   `json:"consultation_progress_visible"`
	EvidenceReadinessVisible    bool   `json:"evidence_readiness_visible"`
	Confidence                  string `json:"confidence"`
	ConfidenceAriaValue         string `json:"confidence_aria_value"`
	EvidenceReadinessText       string `json:"evidence_readiness_text"`
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
	if err := validateBrowserProof(*out.Browser, out.MessageLength, messageSHA256); err != nil {
		return err
	}
	if out.CloseNotification != nil && !strings.Contains(evidence, "close_notification") {
		return fmt.Errorf("evidence must mention close_notification when close_notification proof is present")
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
		return validateCloseProof(*out.CloseNotification, sessionID, out.Browser.AssistantTurnsAfter, out.CreatedRoom)
	}
	return nil
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
	return value, hasValue, nil
}

func validateBrowserProof(proof browserProof, messageLength int, messageSHA256 string) error {
	if !proof.StateLoaded {
		return fmt.Errorf("browser.state_loaded must be true")
	}
	if !proof.TurnResultObserved {
		return fmt.Errorf("browser.turn_result_observed must be true")
	}
	if !proof.SubmittedMessageVisible {
		return fmt.Errorf("browser.submitted_message_visible must be true")
	}
	if proof.SubmittedMessageLength <= 0 {
		return fmt.Errorf("browser.submitted_message_length must be > 0")
	}
	if proof.SubmittedMessageLength != messageLength {
		return fmt.Errorf("browser.submitted_message_length must match message_length")
	}
	submittedMessageSHA256, err := validateSHA256Hex("browser.submitted_message_sha256", proof.SubmittedMessageSHA256)
	if err != nil {
		return err
	}
	if submittedMessageSHA256 != messageSHA256 {
		return fmt.Errorf("browser.submitted_message_sha256 must match message_sha256")
	}
	if proof.AssistantTurnsBefore < 0 {
		return fmt.Errorf("browser.assistant_turns_before must be >= 0")
	}
	if proof.AssistantTurnsAfter <= proof.AssistantTurnsBefore {
		return fmt.Errorf("browser.assistant_turns_after must be greater than assistant_turns_before")
	}
	assistantTurnDelta := proof.AssistantTurnsAfter - proof.AssistantTurnsBefore
	if proof.AssistantTurnDelta != assistantTurnDelta {
		return fmt.Errorf("browser.assistant_turn_delta must equal assistant_turns_after - assistant_turns_before")
	}
	if proof.TranscriptMessagesBefore < 0 {
		return fmt.Errorf("browser.transcript_messages_before must be >= 0")
	}
	if proof.TranscriptMessagesBefore != proof.AssistantTurnsBefore*2 {
		return fmt.Errorf("browser.transcript_messages_before must equal assistant_turns_before * 2")
	}
	if proof.TranscriptMessagesAfter != proof.TranscriptMessagesBefore+(2*assistantTurnDelta) {
		return fmt.Errorf("browser.transcript_messages_after must equal transcript_messages_before + 2 * assistant_turn_delta")
	}
	if proof.TranscriptMessagesAfter != proof.AssistantTurnsAfter*2 {
		return fmt.Errorf("browser.transcript_messages_after must equal assistant_turns_after * 2")
	}
	status := strings.TrimSpace(proof.ConnectionStatusAfterTurn)
	if status == "" {
		return fmt.Errorf("browser.connection_status_after_turn must be non-empty")
	}
	if status != proof.ConnectionStatusAfterTurn {
		return fmt.Errorf("browser.connection_status_after_turn must not contain leading or trailing whitespace")
	}
	if status != "connected" {
		return fmt.Errorf("browser.connection_status_after_turn = %q, want connected", status)
	}
	completedTurnText, err := validateRequiredCleanString("browser.completed_turn_text", proof.CompletedTurnText)
	if err != nil {
		return err
	}
	turnNumber, ok := completedTurnNumber(completedTurnText)
	if !ok {
		return fmt.Errorf("browser.completed_turn_text must match a completed turn log entry")
	}
	if turnNumber != proof.AssistantTurnsAfter {
		return fmt.Errorf("browser.completed_turn_text must match assistant_turns_after")
	}
	if !proof.ConsultationInsightVisible {
		return fmt.Errorf("browser.consultation_insight_visible must be true")
	}
	if !proof.ConsultationProgressVisible {
		return fmt.Errorf("browser.consultation_progress_visible must be true")
	}
	if !proof.EvidenceReadinessVisible {
		return fmt.Errorf("browser.evidence_readiness_visible must be true")
	}
	confidence, err := validateRequiredCleanString("browser.confidence", proof.Confidence)
	if err != nil {
		return err
	}
	switch confidence {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("browser.confidence = %q, want low, medium, or high", confidence)
	}
	confidenceAriaValue, err := validateRequiredCleanString("browser.confidence_aria_value", proof.ConfidenceAriaValue)
	if err != nil {
		return err
	}
	if confidenceAriaValue != confidence+" confidence" {
		return fmt.Errorf("browser.confidence_aria_value must match browser.confidence")
	}
	readinessText, err := validateBoundedText(
		"browser.evidence_readiness_text",
		proof.EvidenceReadinessText,
		maxProofBrowserReadinessTextBytes,
	)
	if err != nil {
		return err
	}
	for _, label := range []string{"Plan", "Collected", "Missing", "Suggestions", "Next"} {
		if !strings.Contains(readinessText, label) {
			return fmt.Errorf("browser.evidence_readiness_text must include %s", label)
		}
	}
	return nil
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

func completedTurnNumber(value string) (int, bool) {
	if !strings.HasPrefix(value, "Turn ") || !strings.HasSuffix(value, " completed.") {
		return 0, false
	}
	middle := strings.TrimSuffix(strings.TrimPrefix(value, "Turn "), " completed.")
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
