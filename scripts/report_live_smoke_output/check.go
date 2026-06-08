// Command report_live_smoke_output validates `openclarion report-replay --wait`
// or `openclarion report-policy-replay --wait` JSON output for the manual live
// external report smoke gates.
package main

import (
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
}

type workflowResult struct {
	SubReportIDs               []int64 `json:"sub_report_ids"`
	FinalReportID              int64   `json:"final_report_id"`
	NotificationIdempotencyKey string  `json:"notification_idempotency_key"`
	ProviderMessageID          string  `json:"provider_message_id"`
	NotificationStatus         string  `json:"notification_status"`
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
	if len(args) != 1 {
		return fmt.Errorf("usage: report_live_smoke_output <output.json>")
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
	if eventTotal != stats.EventsLoaded {
		return fmt.Errorf("stats.events_loaded must equal sum of snapshot event_count")
	}
	return nil
}
