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
	maxProofBytes           int64 = 10 * 1024 * 1024
	maxProofSessionIDBytes        = 128
	maxProofWorkflowIDBytes       = 256
	maxProofRunIDBytes            = 256
	maxProofEvidenceBytes         = 512
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
	StateLoaded               bool   `json:"state_loaded"`
	TurnResultObserved        bool   `json:"turn_result_observed"`
	AssistantTurnsBefore      int    `json:"assistant_turns_before"`
	AssistantTurnsAfter       int    `json:"assistant_turns_after"`
	TranscriptMessagesBefore  int    `json:"transcript_messages_before"`
	TranscriptMessagesAfter   int    `json:"transcript_messages_after"`
	ConnectionStatusAfterTurn string `json:"connection_status_after_turn"`
	SubmittedMessageVisible   bool   `json:"submitted_message_visible"`
	SubmittedMessageLength    int    `json:"submitted_message_length"`
	SubmittedMessageSHA256    string `json:"submitted_message_sha256"`
	CompletedTurnText         string `json:"completed_turn_text"`
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
	if out.CreatedRoom == nil {
		return nil
	}
	return validateCreatedRoom(*out.CreatedRoom, sessionID, evidenceSnapshotID, hasEvidenceSnapshot)
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
	if proof.AssistantTurnsAfter != proof.AssistantTurnsBefore+1 {
		return fmt.Errorf("browser.assistant_turns_after must equal assistant_turns_before + 1")
	}
	if proof.TranscriptMessagesBefore < 0 {
		return fmt.Errorf("browser.transcript_messages_before must be >= 0")
	}
	if proof.TranscriptMessagesBefore != proof.AssistantTurnsBefore*2 {
		return fmt.Errorf("browser.transcript_messages_before must equal assistant_turns_before * 2")
	}
	if proof.TranscriptMessagesAfter != proof.TranscriptMessagesBefore+2 {
		return fmt.Errorf("browser.transcript_messages_after must equal transcript_messages_before + 2")
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
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("checked_at must be non-empty")
	}
	if value != raw {
		return time.Time{}, fmt.Errorf("checked_at must not contain leading or trailing whitespace")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("checked_at must be RFC3339: %w", err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("checked_at must be canonical UTC RFC3339")
	}
	if checkedAt.After(nowUTC()) {
		return time.Time{}, fmt.Errorf("checked_at must not be in the future")
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
