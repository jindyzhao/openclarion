// Command report_live_smoke_output validates `openclarion report-replay --wait`
// or `openclarion report-policy-replay --wait` JSON output for the manual live
// external report smoke gates.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxProofBytes                  int64 = 10 * 1024 * 1024
	maxProofWorkflowIDBytes              = 256
	maxProofRunIDBytes                   = 256
	maxProofCorrelationKeyBytes          = 256
	maxProofProviderMessageIDBytes       = 512
	maxProofNotificationKeyBytes         = 256
	maxProofSessionIDBytes               = 128
	maxProofReviewSourceBytes            = 128
	maxProofReviewReasonBytes            = 512
)

type smokeOutput struct {
	CheckedAt      string          `json:"checked_at"`
	Request        proofRequest    `json:"request"`
	Started        bool            `json:"started"`
	WorkflowID     string          `json:"workflow_id"`
	RunID          string          `json:"run_id"`
	Waited         bool            `json:"waited"`
	WorkflowResult *workflowResult `json:"workflow_result"`
	Stats          replayStats     `json:"stats"`
	Snapshots      []snapshotRef   `json:"snapshots"`
	AIReview       *aiReviewProof  `json:"ai_review,omitempty"`
}

type workflowResult struct {
	SubReportIDs               []int64 `json:"sub_report_ids"`
	FinalReportID              int64   `json:"final_report_id"`
	NotificationIdempotencyKey string  `json:"notification_idempotency_key"`
	ProviderMessageID          string  `json:"provider_message_id"`
	NotificationStatus         string  `json:"notification_status"`
}

type aiReviewProof struct {
	Status             string                     `json:"status"`
	ReviewedAt         string                     `json:"reviewed_at"`
	FinalReportID      int64                      `json:"final_report_id"`
	ReviewedSubReports []aiSubReportReview        `json:"reviewed_sub_reports"`
	PendingSubReports  []aiPendingSubReportReview `json:"pending_sub_reports,omitempty"`
}

type aiSubReportReview struct {
	SubReportID                       int64  `json:"sub_report_id"`
	EvidenceSnapshotID                int64  `json:"evidence_snapshot_id"`
	DiagnosisTaskID                   int64  `json:"diagnosis_task_id"`
	SessionID                         string `json:"session_id"`
	ChatSessionID                     int64  `json:"chat_session_id"`
	ConclusionStatus                  string `json:"conclusion_status"`
	ConclusionSource                  string `json:"conclusion_source"`
	Confidence                        string `json:"confidence"`
	RequiresHumanReview               *bool  `json:"requires_human_review"`
	ConfidenceTimelineCount           int    `json:"confidence_timeline_count"`
	EvidenceRequestCount              int    `json:"evidence_request_count"`
	MissingEvidenceRequestCount       int    `json:"missing_evidence_request_count"`
	EvidenceCollectionSuggestionCount int    `json:"evidence_collection_suggestion_count"`
	EvidenceCollectionResultCount     int    `json:"evidence_collection_result_count"`
	SupplementalEvidenceCount         int    `json:"supplemental_evidence_count"`
	NotificationTimelineCount         int    `json:"notification_timeline_count"`
}

func (r aiSubReportReview) evidenceWorkCount() int {
	return r.EvidenceRequestCount +
		r.MissingEvidenceRequestCount +
		r.EvidenceCollectionSuggestionCount +
		r.EvidenceCollectionResultCount +
		r.SupplementalEvidenceCount
}

type aiPendingSubReportReview struct {
	SubReportID        int64                `json:"sub_report_id"`
	EvidenceSnapshotID int64                `json:"evidence_snapshot_id"`
	Reason             string               `json:"reason"`
	TaskStates         []aiPendingTaskState `json:"task_states"`
}

type aiPendingTaskState struct {
	DiagnosisTaskID                   int64  `json:"diagnosis_task_id"`
	TaskStatus                        string `json:"task_status,omitempty"`
	FailureReason                     string `json:"failure_reason,omitempty"`
	LatestTurnStatus                  string `json:"latest_turn_status,omitempty"`
	LatestConfidence                  string `json:"latest_confidence,omitempty"`
	LatestRequiresHumanReview         *bool  `json:"latest_requires_human_review,omitempty"`
	ConfidenceTimelineCount           int    `json:"confidence_timeline_count,omitempty"`
	EvidenceRequestCount              int    `json:"evidence_request_count,omitempty"`
	MissingEvidenceRequestCount       int    `json:"missing_evidence_request_count,omitempty"`
	EvidenceCollectionSuggestionCount int    `json:"evidence_collection_suggestion_count,omitempty"`
	EvidenceCollectionResultCount     int    `json:"evidence_collection_result_count,omitempty"`
	SupplementalEvidenceCount         int    `json:"supplemental_evidence_count,omitempty"`
	NotificationTimelineCount         int    `json:"notification_timeline_count,omitempty"`
	FinalReadyEventCount              int    `json:"final_ready_event_count,omitempty"`
	FailedEventCount                  int    `json:"failed_event_count,omitempty"`
	ClosedEventCount                  int    `json:"closed_event_count,omitempty"`
	LastEventKind                     string `json:"last_event_kind,omitempty"`
	LastEventAt                       string `json:"last_event_at,omitempty"`
}

func (s aiPendingTaskState) evidenceWorkCount() int {
	return s.EvidenceRequestCount +
		s.MissingEvidenceRequestCount +
		s.EvidenceCollectionSuggestionCount +
		s.EvidenceCollectionResultCount +
		s.SupplementalEvidenceCount
}

type proofRequest struct {
	PolicyID       int64  `json:"policy_id"`
	WindowStart    string `json:"window_start"`
	WindowEnd      string `json:"window_end"`
	Limit          int    `json:"limit"`
	Scenario       string `json:"scenario"`
	CorrelationKey string `json:"correlation_key"`
	WorkflowID     string `json:"workflow_id"`
	Wait           bool   `json:"wait"`
	WaitTimeout    string `json:"wait_timeout"`
}

type snapshotRef struct {
	ID         int64 `json:"id"`
	GroupIndex int   `json:"group_index"`
	EventCount int   `json:"event_count"`
}

type replayStats struct {
	Ingested           ingestStats `json:"ingested"`
	EventsLoaded       int         `json:"events_loaded"`
	GroupsBuilt        int         `json:"groups_built"`
	GroupsSaved        int         `json:"groups_saved"`
	GroupsRefreshed    int         `json:"groups_refreshed"`
	GroupsExisting     int         `json:"groups_existing"`
	SnapshotsSaved     int         `json:"snapshots_saved"`
	SnapshotsDuplicate int         `json:"snapshots_duplicate"`
	GroupsClosed       int         `json:"groups_closed"`
	Failed             int         `json:"failed"`
}

type ingestStats struct {
	Total     int `json:"total"`
	Saved     int `json:"saved"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "[report-live-smoke-output] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[report-live-smoke-output] OK")
}

func run(args []string) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	path := filepath.Clean(cfg.path)
	raw, err := readProofFile(path)
	if err != nil {
		return err
	}
	var out smokeOutput
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("decode JSON output: %w", err)
	}
	return validate(out, cfg.requireAIReview)
}

type config struct {
	path            string
	requireAIReview bool
}

func parseArgs(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("report_live_smoke_output", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&cfg.requireAIReview, "require-ai-review", false, "require linked diagnosis-room AI review proof")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 1 {
		return config{}, fmt.Errorf("usage: report_live_smoke_output [--require-ai-review] <output.json>")
	}
	cfg.path = fs.Arg(0)
	return cfg, nil
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

func validate(out smokeOutput, requireAIReview bool) error {
	checkedAt, err := validateCheckedAt(out.CheckedAt)
	if err != nil {
		return err
	}
	if err := validateProofRequest(out.Request, checkedAt); err != nil {
		return err
	}
	if !out.Started {
		return fmt.Errorf("started must be true")
	}
	workflowID, err := validateProofID("workflow_id", out.WorkflowID, maxProofWorkflowIDBytes)
	if err != nil {
		return err
	}
	if out.Request.WorkflowID != "" && out.Request.WorkflowID != workflowID {
		return fmt.Errorf("request.workflow_id must match workflow_id")
	}
	if _, err := validateProofID("run_id", out.RunID, maxProofRunIDBytes); err != nil {
		return err
	}
	if len(out.Snapshots) == 0 {
		return fmt.Errorf("snapshots must be non-empty")
	}
	if err := validateSnapshots(out.Snapshots); err != nil {
		return err
	}
	if err := validateStats(out.Stats, out.Snapshots, out.Request.Limit); err != nil {
		return err
	}
	if !out.Waited {
		return fmt.Errorf("waited must be true")
	}
	if out.WorkflowResult == nil {
		return fmt.Errorf("workflow_result must be present")
	}
	if len(out.WorkflowResult.SubReportIDs) == 0 {
		return fmt.Errorf("workflow_result.sub_report_ids must be non-empty")
	}
	if len(out.WorkflowResult.SubReportIDs) != len(out.Snapshots) {
		return fmt.Errorf("workflow_result.sub_report_ids length must match snapshots length")
	}
	seenSubReportIDs := make(map[int64]struct{}, len(out.WorkflowResult.SubReportIDs))
	for i, id := range out.WorkflowResult.SubReportIDs {
		if id <= 0 {
			return fmt.Errorf("workflow_result.sub_report_ids[%d] must be > 0", i)
		}
		if _, ok := seenSubReportIDs[id]; ok {
			return fmt.Errorf("workflow_result.sub_report_ids[%d] duplicates id %d", i, id)
		}
		seenSubReportIDs[id] = struct{}{}
	}
	if out.WorkflowResult.FinalReportID <= 0 {
		return fmt.Errorf("workflow_result.final_report_id must be > 0")
	}
	if err := validateNotificationIdempotencyKey(
		out.WorkflowResult.NotificationIdempotencyKey,
		out.WorkflowResult.FinalReportID,
	); err != nil {
		return err
	}
	if err := validateProviderMessageID(out.WorkflowResult.ProviderMessageID); err != nil {
		return err
	}
	if err := validateNotificationStatus(out.WorkflowResult.NotificationStatus); err != nil {
		return err
	}
	if err := validateAIReview(out.AIReview, *out.WorkflowResult, out.Snapshots, requireAIReview); err != nil {
		return err
	}
	return nil
}

func validateCheckedAt(raw string) (time.Time, error) {
	return validateCheckedAtField("checked_at", raw)
}

func validateCheckedAtField(field, raw string) (time.Time, error) {
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

func validateSnapshots(snapshots []snapshotRef) error {
	seenSnapshotIDs := make(map[int64]struct{}, len(snapshots))
	for i, snapshot := range snapshots {
		if snapshot.ID <= 0 {
			return fmt.Errorf("snapshots[%d].id must be > 0", i)
		}
		if _, ok := seenSnapshotIDs[snapshot.ID]; ok {
			return fmt.Errorf("snapshots[%d].id duplicates id %d", i, snapshot.ID)
		}
		seenSnapshotIDs[snapshot.ID] = struct{}{}
		if snapshot.GroupIndex != i {
			return fmt.Errorf("snapshots[%d].group_index must equal snapshot index", i)
		}
		if snapshot.EventCount <= 0 {
			return fmt.Errorf("snapshots[%d].event_count must be > 0", i)
		}
	}
	return nil
}

func validateProofRequest(req proofRequest, checkedAt time.Time) error {
	if req.PolicyID < 0 {
		return fmt.Errorf("request.policy_id must be omitted or positive")
	}
	windowStart, err := parseProofTime("request.window_start", req.WindowStart)
	if err != nil {
		return err
	}
	windowEnd, err := parseProofTime("request.window_end", req.WindowEnd)
	if err != nil {
		return err
	}
	if !windowEnd.After(windowStart) {
		return fmt.Errorf("request.window_end must be after request.window_start")
	}
	if windowEnd.After(checkedAt) {
		return fmt.Errorf("request.window_end must not be after checked_at")
	}
	if req.Limit <= 0 {
		return fmt.Errorf("request.limit must be > 0")
	}
	switch strings.TrimSpace(req.Scenario) {
	case "single_alert", "cascade", "alert_storm":
	default:
		return fmt.Errorf("request.scenario = %q, want single_alert, cascade, or alert_storm", req.Scenario)
	}
	if strings.TrimSpace(req.Scenario) != req.Scenario {
		return fmt.Errorf("request.scenario must not contain leading or trailing whitespace")
	}
	if err := validateOptionalBoundedCleanString("request.correlation_key", req.CorrelationKey, maxProofCorrelationKeyBytes); err != nil {
		return err
	}
	if _, err := validateOptionalProofID("request.workflow_id", req.WorkflowID, maxProofWorkflowIDBytes); err != nil {
		return err
	}
	if !req.Wait {
		return fmt.Errorf("request.wait must be true")
	}
	value := strings.TrimSpace(req.WaitTimeout)
	if value == "" {
		return fmt.Errorf("request.wait_timeout must be non-empty")
	}
	if value != req.WaitTimeout {
		return fmt.Errorf("request.wait_timeout must not contain leading or trailing whitespace")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("request.wait_timeout must be a valid duration: %w", err)
	}
	if duration <= 0 {
		return fmt.Errorf("request.wait_timeout must be > 0")
	}
	return nil
}

func parseProofTime(field, raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return time.Time{}, fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	if parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("%s must be canonical UTC RFC3339", field)
	}
	return parsed.UTC(), nil
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

func validateRequiredBoundedCleanString(field, raw string, maxBytes int) (string, error) {
	value, err := validateRequiredCleanString(field, raw)
	if err != nil {
		return "", err
	}
	if len(raw) > maxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return value, nil
}

func validateOptionalBoundedCleanString(field, raw string, maxBytes int) error {
	if strings.TrimSpace(raw) != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return fmt.Errorf("%s must be a single-line value", field)
	}
	if len(raw) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return nil
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

func validateOptionalProofID(field, raw string, maxBytes int) (string, error) {
	if raw == "" {
		return "", nil
	}
	value, err := validateProofID(field, raw, maxBytes)
	if err != nil {
		return "", err
	}
	return value, nil
}

func validateOptionalBoundedProviderMessageID(field, raw string, maxBytes int) error {
	if raw == "" {
		return nil
	}
	if err := validateOptionalBoundedCleanString(field, raw, maxBytes); err != nil {
		return err
	}
	return nil
}

func validateNotificationIdempotencyKey(key string, finalReportID int64) error {
	value, err := validateRequiredBoundedCleanString(
		"workflow_result.notification_idempotency_key",
		key,
		maxProofNotificationKeyBytes,
	)
	if err != nil {
		return err
	}
	want := fmt.Sprintf("final_report:%d/notification", finalReportID)
	if value != want {
		return fmt.Errorf("workflow_result.notification_idempotency_key = %q, want %q", value, want)
	}
	return nil
}

func validateProviderMessageID(id string) error {
	return validateOptionalBoundedProviderMessageID(
		"workflow_result.provider_message_id",
		id,
		maxProofProviderMessageIDBytes,
	)
}

func validateNotificationStatus(status string) error {
	value := strings.TrimSpace(status)
	if value == "" {
		return fmt.Errorf("workflow_result.notification_status must be non-empty")
	}
	if value != status {
		return fmt.Errorf("workflow_result.notification_status must not contain leading or trailing whitespace")
	}
	switch value {
	case "accepted", "delivered":
		return nil
	default:
		return fmt.Errorf("workflow_result.notification_status = %q, want accepted or delivered", value)
	}
}

func validateAIReview(review *aiReviewProof, result workflowResult, snapshots []snapshotRef, required bool) error {
	if review == nil {
		if required {
			return fmt.Errorf("ai_review must be present when --require-ai-review is set")
		}
		return nil
	}
	status, err := validateRequiredCleanString("ai_review.status", review.Status)
	if err != nil {
		return err
	}
	switch status {
	case "complete", "pending_evidence":
	default:
		return fmt.Errorf("ai_review.status = %q, want complete or pending_evidence", status)
	}
	if _, err := validateCheckedAtField("ai_review.reviewed_at", review.ReviewedAt); err != nil {
		return err
	}
	if review.FinalReportID != result.FinalReportID {
		return fmt.Errorf("ai_review.final_report_id must match workflow_result.final_report_id")
	}
	expectedSnapshots := make(map[int64]int64, len(result.SubReportIDs))
	for i, subReportID := range result.SubReportIDs {
		expectedSnapshots[subReportID] = snapshots[i].ID
	}
	if status == "complete" && len(review.PendingSubReports) != 0 {
		return fmt.Errorf("ai_review.pending_sub_reports must be empty when status is complete")
	}
	if status == "complete" && len(review.ReviewedSubReports) != len(result.SubReportIDs) {
		return fmt.Errorf("ai_review.reviewed_sub_reports length must match workflow_result.sub_report_ids length when status is complete")
	}
	if status == "pending_evidence" && len(review.PendingSubReports) == 0 {
		return fmt.Errorf("ai_review.pending_sub_reports must be non-empty when status is pending_evidence")
	}
	if len(review.ReviewedSubReports)+len(review.PendingSubReports) != len(result.SubReportIDs) {
		return fmt.Errorf("ai_review reviewed and pending subreport count must match workflow_result.sub_report_ids length")
	}
	seen := make(map[int64]struct{}, len(result.SubReportIDs))
	for i, item := range review.ReviewedSubReports {
		expectedSnapshot, ok := expectedSnapshots[item.SubReportID]
		if !ok {
			return fmt.Errorf("ai_review.reviewed_sub_reports[%d].sub_report_id is not in workflow_result.sub_report_ids", i)
		}
		if _, ok := seen[item.SubReportID]; ok {
			return fmt.Errorf("ai_review.reviewed_sub_reports[%d].sub_report_id duplicates id %d", i, item.SubReportID)
		}
		if err := validateAISubReportReview(i, item, expectedSnapshot); err != nil {
			return err
		}
		seen[item.SubReportID] = struct{}{}
	}
	for i, item := range review.PendingSubReports {
		expectedSnapshot, ok := expectedSnapshots[item.SubReportID]
		if !ok {
			return fmt.Errorf("ai_review.pending_sub_reports[%d].sub_report_id is not in workflow_result.sub_report_ids", i)
		}
		if _, ok := seen[item.SubReportID]; ok {
			return fmt.Errorf("ai_review.pending_sub_reports[%d].sub_report_id duplicates id %d", i, item.SubReportID)
		}
		if err := validateAIPendingSubReportReview(i, item, expectedSnapshot); err != nil {
			return err
		}
		seen[item.SubReportID] = struct{}{}
	}
	return nil
}

func validateAISubReportReview(index int, review aiSubReportReview, evidenceSnapshotID int64) error {
	field := func(name string) string {
		return fmt.Sprintf("ai_review.reviewed_sub_reports[%d].%s", index, name)
	}
	if review.EvidenceSnapshotID != evidenceSnapshotID {
		return fmt.Errorf("%s must match the workflow snapshot for sub_report_id %d", field("evidence_snapshot_id"), review.SubReportID)
	}
	if review.DiagnosisTaskID <= 0 {
		return fmt.Errorf("%s must be > 0", field("diagnosis_task_id"))
	}
	if _, err := validateProofID(field("session_id"), review.SessionID, maxProofSessionIDBytes); err != nil {
		return err
	}
	if review.ChatSessionID <= 0 {
		return fmt.Errorf("%s must be > 0", field("chat_session_id"))
	}
	status, err := validateRequiredCleanString(field("conclusion_status"), review.ConclusionStatus)
	if err != nil {
		return err
	}
	if status != "available" {
		return fmt.Errorf("%s = %q, want available", field("conclusion_status"), status)
	}
	if _, err := validateRequiredBoundedCleanString(field("conclusion_source"), review.ConclusionSource, maxProofReviewSourceBytes); err != nil {
		return err
	}
	confidence, err := validateRequiredCleanString(field("confidence"), review.Confidence)
	if err != nil {
		return err
	}
	switch confidence {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("%s = %q, want low, medium, or high", field("confidence"), confidence)
	}
	if review.RequiresHumanReview == nil {
		return fmt.Errorf("%s must be present", field("requires_human_review"))
	}
	if review.ConfidenceTimelineCount <= 0 {
		return fmt.Errorf("%s must be > 0", field("confidence_timeline_count"))
	}
	if review.EvidenceRequestCount < 0 ||
		review.MissingEvidenceRequestCount < 0 ||
		review.EvidenceCollectionSuggestionCount < 0 ||
		review.EvidenceCollectionResultCount < 0 ||
		review.SupplementalEvidenceCount < 0 ||
		review.NotificationTimelineCount < 0 {
		return fmt.Errorf("%s counts must be non-negative", field("evidence"))
	}
	if review.evidenceWorkCount() == 0 {
		return fmt.Errorf("%s must include at least one evidence guidance, collection result, or supplemental evidence item", field("evidence"))
	}
	return nil
}

func validateAIPendingSubReportReview(index int, review aiPendingSubReportReview, evidenceSnapshotID int64) error {
	field := func(name string) string {
		return fmt.Sprintf("ai_review.pending_sub_reports[%d].%s", index, name)
	}
	if review.EvidenceSnapshotID != evidenceSnapshotID {
		return fmt.Errorf("%s must match the workflow snapshot for sub_report_id %d", field("evidence_snapshot_id"), review.SubReportID)
	}
	if _, err := validateRequiredBoundedCleanString(field("reason"), review.Reason, maxProofReviewReasonBytes); err != nil {
		return err
	}
	if len(review.TaskStates) == 0 {
		return fmt.Errorf("%s must be non-empty", field("task_states"))
	}
	for i, state := range review.TaskStates {
		if err := validateAIPendingTaskState(fmt.Sprintf("%s[%d]", field("task_states"), i), state); err != nil {
			return err
		}
	}
	return nil
}

func validateAIPendingTaskState(field string, state aiPendingTaskState) error {
	if state.DiagnosisTaskID <= 0 {
		return fmt.Errorf("%s.diagnosis_task_id must be > 0", field)
	}
	taskStatus, err := validateRequiredCleanString(field+".task_status", state.TaskStatus)
	if err != nil {
		return err
	}
	if taskStatus == "failed" {
		if _, err := validateRequiredBoundedCleanString(field+".failure_reason", state.FailureReason, maxProofReviewReasonBytes); err != nil {
			return err
		}
		if state.FailedEventCount <= 0 {
			return fmt.Errorf("%s.failed_event_count must be > 0 when task_status is failed", field)
		}
		if state.ClosedEventCount < 0 ||
			state.ConfidenceTimelineCount < 0 ||
			state.EvidenceRequestCount < 0 ||
			state.MissingEvidenceRequestCount < 0 ||
			state.EvidenceCollectionSuggestionCount < 0 ||
			state.EvidenceCollectionResultCount < 0 ||
			state.SupplementalEvidenceCount < 0 ||
			state.NotificationTimelineCount < 0 ||
			state.FinalReadyEventCount < 0 {
			return fmt.Errorf("%s counts must be non-negative", field)
		}
		if _, err := validateRequiredCleanString(field+".last_event_kind", state.LastEventKind); err != nil {
			return err
		}
		if _, err := validateCheckedAtField(field+".last_event_at", state.LastEventAt); err != nil {
			return err
		}
		return nil
	}
	status, err := validateRequiredCleanString(field+".latest_turn_status", state.LatestTurnStatus)
	if err != nil {
		return err
	}
	switch status {
	case "investigating", "needs_evidence", "ready_for_review":
	default:
		return fmt.Errorf("%s.latest_turn_status = %q, want investigating, needs_evidence, or ready_for_review", field, status)
	}
	confidence, err := validateRequiredCleanString(field+".latest_confidence", state.LatestConfidence)
	if err != nil {
		return err
	}
	switch confidence {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("%s.latest_confidence = %q, want low, medium, or high", field, confidence)
	}
	if state.LatestRequiresHumanReview == nil {
		return fmt.Errorf("%s.latest_requires_human_review must be present", field)
	}
	if state.ConfidenceTimelineCount <= 0 {
		return fmt.Errorf("%s.confidence_timeline_count must be > 0", field)
	}
	if state.EvidenceRequestCount < 0 ||
		state.MissingEvidenceRequestCount < 0 ||
		state.EvidenceCollectionSuggestionCount < 0 ||
		state.EvidenceCollectionResultCount < 0 ||
		state.SupplementalEvidenceCount < 0 ||
		state.NotificationTimelineCount < 0 ||
		state.FinalReadyEventCount < 0 ||
		state.FailedEventCount < 0 ||
		state.ClosedEventCount < 0 {
		return fmt.Errorf("%s counts must be non-negative", field)
	}
	if state.evidenceWorkCount() == 0 {
		return fmt.Errorf("%s must include at least one evidence guidance, collection result, or supplemental evidence item", field)
	}
	if state.NotificationTimelineCount == 0 {
		return fmt.Errorf("%s.notification_timeline_count must be > 0", field)
	}
	if state.FinalReadyEventCount != 0 || state.ClosedEventCount != 0 {
		return fmt.Errorf("%s must not include final-ready or closed events while pending evidence", field)
	}
	if _, err := validateRequiredCleanString(field+".last_event_kind", state.LastEventKind); err != nil {
		return err
	}
	if _, err := validateCheckedAtField(field+".last_event_at", state.LastEventAt); err != nil {
		return err
	}
	return nil
}

func validateStats(stats replayStats, snapshots []snapshotRef, requestLimit int) error {
	if stats.Ingested.Total <= 0 {
		return fmt.Errorf("stats.ingested.total must be > 0")
	}
	if stats.Ingested.Saved < 0 || stats.Ingested.Duplicate < 0 || stats.Ingested.Failed < 0 {
		return fmt.Errorf("stats.ingested counts must be non-negative")
	}
	if stats.Ingested.Saved+stats.Ingested.Duplicate+stats.Ingested.Failed != stats.Ingested.Total {
		return fmt.Errorf("stats.ingested counts must add up to total")
	}
	if stats.Ingested.Failed != 0 {
		return fmt.Errorf("stats.ingested.failed must be 0")
	}
	if stats.EventsLoaded <= 0 {
		return fmt.Errorf("stats.events_loaded must be > 0")
	}
	if stats.EventsLoaded > requestLimit {
		return fmt.Errorf("stats.events_loaded must be <= request.limit")
	}
	if stats.GroupsBuilt <= 0 {
		return fmt.Errorf("stats.groups_built must be > 0")
	}
	if stats.GroupsSaved < 0 || stats.GroupsRefreshed < 0 || stats.GroupsExisting < 0 ||
		stats.SnapshotsSaved < 0 || stats.SnapshotsDuplicate < 0 || stats.GroupsClosed < 0 ||
		stats.Failed < 0 {
		return fmt.Errorf("stats counts must be non-negative")
	}
	if stats.Failed != 0 {
		return fmt.Errorf("stats.failed must be 0")
	}
	groupOutcomes := stats.GroupsSaved + stats.GroupsRefreshed + stats.GroupsExisting + stats.Failed
	if groupOutcomes != stats.GroupsBuilt {
		return fmt.Errorf("stats group counts must add up to groups_built")
	}
	snapshotCount := len(snapshots)
	if stats.GroupsBuilt != snapshotCount {
		return fmt.Errorf("stats.groups_built must equal snapshots length")
	}
	if stats.SnapshotsSaved+stats.SnapshotsDuplicate != snapshotCount {
		return fmt.Errorf("stats snapshots saved+duplicate must equal snapshots length")
	}
	if stats.GroupsClosed > stats.GroupsBuilt {
		return fmt.Errorf("stats.groups_closed must be <= groups_built")
	}
	eventTotal := 0
	for _, snapshot := range snapshots {
		eventTotal += snapshot.EventCount
	}
	if eventTotal > stats.EventsLoaded {
		return fmt.Errorf("sum of snapshot event_count must be <= stats.events_loaded")
	}
	return nil
}
