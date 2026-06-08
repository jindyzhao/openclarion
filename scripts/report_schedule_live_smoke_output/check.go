// Command report_schedule_live_smoke_output validates retained JSON output from
// `openclarion report-schedule-live-smoke`.
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
	maxProofBytes                   int64 = 10 * 1024 * 1024
	maxProofWorkflowIDBytes               = 256
	maxProofRunIDBytes                    = 256
	maxProofCorrelationKeyBytes           = 256
	maxProofProviderMessageIDBytes        = 512
	maxProofNotificationKeyBytes          = 256
	maxProofTemporalScheduleIDBytes       = 200
)

type scheduleSmokeOutput struct {
	CheckedAt            string                 `json:"checked_at"`
	Request              proofRequest           `json:"request"`
	PersistedSchedule    persistedSchedule      `json:"persisted_schedule"`
	Waited               bool                   `json:"waited"`
	ScheduleAction       scheduleAction         `json:"schedule_action"`
	LauncherWorkflow     launcherWorkflowResult `json:"launcher_workflow"`
	ReportWorkflowResult *reportWorkflowResult  `json:"report_workflow_result"`
}

type proofRequest struct {
	ScheduleID         int64  `json:"schedule_id"`
	PolicyID           int64  `json:"policy_id"`
	TemporalScheduleID string `json:"temporal_schedule_id"`
	ObservedAfter      string `json:"observed_after"`
	WaitTimeout        string `json:"wait_timeout"`
}

type persistedSchedule struct {
	ID                     int64  `json:"id"`
	ReportWorkflowPolicyID int64  `json:"report_workflow_policy_id"`
	TemporalScheduleID     string `json:"temporal_schedule_id"`
	Enabled                bool   `json:"enabled"`
	Interval               string `json:"interval"`
	Offset                 string `json:"offset"`
	ReplayWindow           string `json:"replay_window"`
	ReplayDelay            string `json:"replay_delay"`
	ReplayLimit            int    `json:"replay_limit"`
	CatchupWindow          string `json:"catchup_window"`
}

type scheduleAction struct {
	ScheduleTime string `json:"schedule_time"`
	ActualTime   string `json:"actual_time"`
	WorkflowID   string `json:"workflow_id"`
	RunID        string `json:"run_id"`
}

type launcherWorkflowResult struct {
	ScheduleID                 int64  `json:"schedule_id"`
	ReportWorkflowPolicyID     int64  `json:"report_workflow_policy_id"`
	TemporalScheduleID         string `json:"temporal_schedule_id"`
	FireTime                   string `json:"fire_time"`
	WindowStart                string `json:"window_start"`
	WindowEnd                  string `json:"window_end"`
	CorrelationKey             string `json:"correlation_key"`
	WorkflowID                 string `json:"workflow_id"`
	EventsLoaded               int    `json:"events_loaded"`
	Snapshots                  int    `json:"snapshots"`
	ReportBatchWorkflowStarted bool   `json:"report_batch_workflow_started"`
	ReportBatchWorkflowID      string `json:"report_batch_workflow_id"`
	ReportBatchRunID           string `json:"report_batch_run_id"`
}

type reportWorkflowResult struct {
	SubReportIDs               []int64 `json:"sub_report_ids"`
	FinalReportID              int64   `json:"final_report_id"`
	NotificationIdempotencyKey string  `json:"notification_idempotency_key"`
	ProviderMessageID          string  `json:"provider_message_id"`
	NotificationStatus         string  `json:"notification_status"`
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "[report-schedule-live-smoke-output] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "[report-schedule-live-smoke-output] OK")
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: report_schedule_live_smoke_output <output.json>")
	}
	raw, err := readProofFile(filepath.Clean(args[0]))
	if err != nil {
		return err
	}
	var out scheduleSmokeOutput
	if err := strictjson.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("decode JSON output: %w", err)
	}
	return validate(out)
}

func readProofFile(path string) ([]byte, error) {
	if err := requireRegularFile(path); err != nil {
		return nil, err
	}
	// #nosec G304,G703 -- this manual smoke checker opens the operator-supplied output JSON path.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxProofBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if int64(len(raw)) > maxProofBytes {
		return nil, fmt.Errorf("%s exceeds maximum proof size %d bytes", path, maxProofBytes)
	}
	return raw, nil
}

func requireRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must be a regular file, not a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", path)
	}
	return nil
}

func validate(out scheduleSmokeOutput) error {
	checkedAt, err := validateCheckedAt(out.CheckedAt)
	if err != nil {
		return err
	}
	observedAfter, err := validateRequest(out.Request, checkedAt)
	if err != nil {
		return err
	}
	if err := validatePersistedSchedule(out.PersistedSchedule, out.Request); err != nil {
		return err
	}
	actionActual, err := validateScheduleAction(out.ScheduleAction, observedAfter, checkedAt)
	if err != nil {
		return err
	}
	if !out.Waited {
		return fmt.Errorf("waited must be true")
	}
	if err := validateLauncher(out.LauncherWorkflow, out.PersistedSchedule, out.ScheduleAction, actionActual, checkedAt); err != nil {
		return err
	}
	if out.ReportWorkflowResult == nil {
		return fmt.Errorf("report_workflow_result must be present")
	}
	if err := validateReportWorkflowResult(*out.ReportWorkflowResult, out.LauncherWorkflow); err != nil {
		return err
	}
	return nil
}

func validateRequest(req proofRequest, checkedAt time.Time) (time.Time, error) {
	if req.ScheduleID <= 0 {
		return time.Time{}, fmt.Errorf("request.schedule_id must be > 0")
	}
	if req.PolicyID <= 0 {
		return time.Time{}, fmt.Errorf("request.policy_id must be > 0")
	}
	if req.TemporalScheduleID != "" {
		if err := validateBoundedID("request.temporal_schedule_id", req.TemporalScheduleID, maxProofTemporalScheduleIDBytes); err != nil {
			return time.Time{}, err
		}
	}
	observedAfter, err := parseProofTime("request.observed_after", req.ObservedAfter)
	if err != nil {
		return time.Time{}, err
	}
	if observedAfter.After(checkedAt) {
		return time.Time{}, fmt.Errorf("request.observed_after must not be after checked_at")
	}
	value := strings.TrimSpace(req.WaitTimeout)
	if value == "" {
		return time.Time{}, fmt.Errorf("request.wait_timeout must be non-empty")
	}
	if value != req.WaitTimeout {
		return time.Time{}, fmt.Errorf("request.wait_timeout must not contain leading or trailing whitespace")
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("request.wait_timeout must be a valid duration: %w", err)
	}
	if duration <= 0 {
		return time.Time{}, fmt.Errorf("request.wait_timeout must be > 0")
	}
	return observedAfter, nil
}

func validatePersistedSchedule(schedule persistedSchedule, req proofRequest) error {
	if schedule.ID != req.ScheduleID {
		return fmt.Errorf("persisted_schedule.id must match request.schedule_id")
	}
	if schedule.ReportWorkflowPolicyID != req.PolicyID {
		return fmt.Errorf("persisted_schedule.report_workflow_policy_id must match request.policy_id")
	}
	if err := validateBoundedID("persisted_schedule.temporal_schedule_id", schedule.TemporalScheduleID, maxProofTemporalScheduleIDBytes); err != nil {
		return err
	}
	if req.TemporalScheduleID != "" && schedule.TemporalScheduleID != req.TemporalScheduleID {
		return fmt.Errorf("persisted_schedule.temporal_schedule_id must match request.temporal_schedule_id")
	}
	if !schedule.Enabled {
		return fmt.Errorf("persisted_schedule.enabled must be true")
	}
	if err := validatePositiveDuration("persisted_schedule.interval", schedule.Interval); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("persisted_schedule.offset", schedule.Offset); err != nil {
		return err
	}
	if err := validatePositiveDuration("persisted_schedule.replay_window", schedule.ReplayWindow); err != nil {
		return err
	}
	if err := validateNonNegativeDuration("persisted_schedule.replay_delay", schedule.ReplayDelay); err != nil {
		return err
	}
	if schedule.ReplayLimit <= 0 {
		return fmt.Errorf("persisted_schedule.replay_limit must be > 0")
	}
	if err := validatePositiveDuration("persisted_schedule.catchup_window", schedule.CatchupWindow); err != nil {
		return err
	}
	return nil
}

func validateScheduleAction(action scheduleAction, observedAfter, checkedAt time.Time) (time.Time, error) {
	if _, err := parseProofTime("schedule_action.schedule_time", action.ScheduleTime); err != nil {
		return time.Time{}, err
	}
	actualTime, err := parseProofTime("schedule_action.actual_time", action.ActualTime)
	if err != nil {
		return time.Time{}, err
	}
	if actualTime.Before(observedAfter) {
		return time.Time{}, fmt.Errorf("schedule_action.actual_time must be at or after request.observed_after")
	}
	if actualTime.After(checkedAt) {
		return time.Time{}, fmt.Errorf("schedule_action.actual_time must not be after checked_at")
	}
	if _, err := validateProofID("schedule_action.workflow_id", action.WorkflowID, maxProofWorkflowIDBytes); err != nil {
		return time.Time{}, err
	}
	if _, err := validateProofID("schedule_action.run_id", action.RunID, maxProofRunIDBytes); err != nil {
		return time.Time{}, err
	}
	return actualTime, nil
}

func validateLauncher(launcher launcherWorkflowResult, schedule persistedSchedule, action scheduleAction, actionActual, checkedAt time.Time) error {
	if launcher.ScheduleID != schedule.ID {
		return fmt.Errorf("launcher_workflow.schedule_id must match persisted schedule")
	}
	if launcher.ReportWorkflowPolicyID != schedule.ReportWorkflowPolicyID {
		return fmt.Errorf("launcher_workflow.report_workflow_policy_id must match persisted schedule")
	}
	if launcher.TemporalScheduleID != schedule.TemporalScheduleID {
		return fmt.Errorf("launcher_workflow.temporal_schedule_id must match persisted schedule")
	}
	fireTime, err := parseProofTime("launcher_workflow.fire_time", launcher.FireTime)
	if err != nil {
		return err
	}
	if fireTime.After(checkedAt) {
		return fmt.Errorf("launcher_workflow.fire_time must not be after checked_at")
	}
	windowStart, err := parseProofTime("launcher_workflow.window_start", launcher.WindowStart)
	if err != nil {
		return err
	}
	windowEnd, err := parseProofTime("launcher_workflow.window_end", launcher.WindowEnd)
	if err != nil {
		return err
	}
	if !windowEnd.After(windowStart) {
		return fmt.Errorf("launcher_workflow.window_end must be after window_start")
	}
	if windowEnd.After(checkedAt) {
		return fmt.Errorf("launcher_workflow.window_end must not be after checked_at")
	}
	if actionActual.Before(fireTime.Add(-time.Minute)) {
		return fmt.Errorf("schedule_action.actual_time must be close to or after launcher_workflow.fire_time")
	}
	if err := validateOptionalBoundedCleanString("launcher_workflow.correlation_key", launcher.CorrelationKey, maxProofCorrelationKeyBytes); err != nil {
		return err
	}
	if _, err := validateProofID("launcher_workflow.workflow_id", launcher.WorkflowID, maxProofWorkflowIDBytes); err != nil {
		return err
	}
	if launcher.EventsLoaded <= 0 {
		return fmt.Errorf("launcher_workflow.events_loaded must be > 0")
	}
	if launcher.Snapshots <= 0 {
		return fmt.Errorf("launcher_workflow.snapshots must be > 0")
	}
	if !launcher.ReportBatchWorkflowStarted {
		return fmt.Errorf("launcher_workflow.report_batch_workflow_started must be true")
	}
	reportWorkflowID, err := validateProofID("launcher_workflow.report_batch_workflow_id", launcher.ReportBatchWorkflowID, maxProofWorkflowIDBytes)
	if err != nil {
		return err
	}
	if reportWorkflowID != launcher.WorkflowID {
		return fmt.Errorf("launcher_workflow.workflow_id must match report_batch_workflow_id")
	}
	if _, err := validateProofID("launcher_workflow.report_batch_run_id", launcher.ReportBatchRunID, maxProofRunIDBytes); err != nil {
		return err
	}
	if action.WorkflowID == launcher.ReportBatchWorkflowID {
		return fmt.Errorf("schedule_action.workflow_id must be the launcher workflow, not the report batch workflow")
	}
	return nil
}

func validateReportWorkflowResult(result reportWorkflowResult, launcher launcherWorkflowResult) error {
	if len(result.SubReportIDs) == 0 {
		return fmt.Errorf("report_workflow_result.sub_report_ids must be non-empty")
	}
	if len(result.SubReportIDs) != launcher.Snapshots {
		return fmt.Errorf("report_workflow_result.sub_report_ids length must match launcher_workflow.snapshots")
	}
	seenSubReportIDs := make(map[int64]struct{}, len(result.SubReportIDs))
	for i, id := range result.SubReportIDs {
		if id <= 0 {
			return fmt.Errorf("report_workflow_result.sub_report_ids[%d] must be > 0", i)
		}
		if _, ok := seenSubReportIDs[id]; ok {
			return fmt.Errorf("report_workflow_result.sub_report_ids[%d] duplicates id %d", i, id)
		}
		seenSubReportIDs[id] = struct{}{}
	}
	if result.FinalReportID <= 0 {
		return fmt.Errorf("report_workflow_result.final_report_id must be > 0")
	}
	if err := validateNotificationIdempotencyKey(result.NotificationIdempotencyKey, result.FinalReportID); err != nil {
		return err
	}
	if err := validateProviderMessageID(result.ProviderMessageID); err != nil {
		return err
	}
	if err := validateNotificationStatus(result.NotificationStatus); err != nil {
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

func validatePositiveDuration(field, raw string) error {
	duration, err := parseDurationField(field, raw)
	if err != nil {
		return err
	}
	if duration <= 0 {
		return fmt.Errorf("%s must be > 0", field)
	}
	return nil
}

func validateNonNegativeDuration(field, raw string) error {
	duration, err := parseDurationField(field, raw)
	if err != nil {
		return err
	}
	if duration < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	return nil
}

func parseDurationField(field, raw string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("%s must be non-empty", field)
	}
	if value != raw {
		return 0, fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", field, err)
	}
	return duration, nil
}

func validateProofID(field, raw string, maxBytes int) (string, error) {
	value, err := validateOptionalProofID(field, raw, maxBytes)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s must be non-empty", field)
	}
	return value, nil
}

func validateOptionalProofID(field, raw string, maxBytes int) (string, error) {
	if err := validateOptionalBoundedCleanString(field, raw, maxBytes); err != nil {
		return "", err
	}
	value := strings.TrimSpace(raw)
	if strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("%s must not contain whitespace", field)
	}
	return value, nil
}

func validateBoundedID(field, raw string, maxBytes int) error {
	value, err := validateProofID(field, raw, maxBytes)
	if err != nil {
		return err
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("%s must not contain whitespace", field)
	}
	return nil
}

func validateOptionalBoundedCleanString(field, raw string, maxBytes int) error {
	value := strings.TrimSpace(raw)
	if value != raw {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single-line value", field)
	}
	return nil
}

func validateNotificationIdempotencyKey(raw string, finalReportID int64) error {
	if err := validateOptionalBoundedCleanString("report_workflow_result.notification_idempotency_key", raw, maxProofNotificationKeyBytes); err != nil {
		return err
	}
	expected := fmt.Sprintf("final_report:%d/notification", finalReportID)
	if raw != expected {
		return fmt.Errorf("report_workflow_result.notification_idempotency_key must equal %q", expected)
	}
	return nil
}

func validateProviderMessageID(raw string) error {
	return validateOptionalBoundedCleanString("report_workflow_result.provider_message_id", raw, maxProofProviderMessageIDBytes)
}

func validateNotificationStatus(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("report_workflow_result.notification_status must be non-empty")
	}
	if value != raw {
		return fmt.Errorf("report_workflow_result.notification_status must not contain leading or trailing whitespace")
	}
	switch value {
	case "accepted", "delivered":
		return nil
	default:
		return fmt.Errorf("report_workflow_result.notification_status must be accepted or delivered")
	}
}
