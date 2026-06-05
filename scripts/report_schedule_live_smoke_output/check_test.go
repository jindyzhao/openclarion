package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	nowUTC = func() time.Time {
		return time.Date(2026, 6, 6, 2, 0, 0, 0, time.UTC)
	}
}

func TestRunAcceptsScheduleLiveSmokeOutput(t *testing.T) {
	path := writeScheduleSmokeOutput(t, validScheduleOutput())
	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunRejectsSymlinkScheduleLiveSmokeOutput(t *testing.T) {
	target := writeScheduleSmokeOutput(t, validScheduleOutput())
	link := filepath.Join(t.TempDir(), "linked-output.json")
	createSymlinkOrSkip(t, target, link)

	err := run([]string{link})
	if err == nil {
		t.Fatal("run: want symlink rejection")
	}
	if !strings.Contains(err.Error(), "must be a regular file, not a symlink") {
		t.Fatalf("error = %q, want symlink rejection", err.Error())
	}
}

func TestRunRejectsIncompleteScheduleLiveSmokeOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "readiness only has no schedule action",
			body: strings.Replace(validScheduleOutput(), `  "schedule_action": {
    "schedule_time": "2026-06-06T00:45:00Z",
    "actual_time": "2026-06-06T00:45:01Z",
    "workflow_id": "report-policy-schedule-9",
    "run_id": "launcher-run-1"
  },
`, "", 1),
			want: "schedule_action.schedule_time",
		},
		{
			name: "disabled schedule",
			body: strings.Replace(validScheduleOutput(), `"enabled": true`, `"enabled": false`, 1),
			want: "persisted_schedule.enabled",
		},
		{
			name: "action before observed after",
			body: strings.Replace(validScheduleOutput(), `"actual_time": "2026-06-06T00:45:01Z"`, `"actual_time": "2026-06-05T23:59:59Z"`, 1),
			want: "schedule_action.actual_time",
		},
		{
			name: "launcher did not start report batch",
			body: strings.Replace(validScheduleOutput(), `"report_batch_workflow_started": true`, `"report_batch_workflow_started": false`, 1),
			want: "report_batch_workflow_started",
		},
		{
			name: "missing report result",
			body: strings.Replace(validScheduleOutput(), `,
  "report_workflow_result": {
    "sub_report_ids": [11],
    "final_report_id": 99,
    "notification_idempotency_key": "final_report:99/notification",
    "provider_message_id": "message-1",
    "notification_status": "accepted"
  }`, "", 1),
			want: "report_workflow_result",
		},
		{
			name: "failed notification",
			body: strings.Replace(validScheduleOutput(), `"notification_status": "accepted"`, `"notification_status": "failed"`, 1),
			want: "accepted or delivered",
		},
		{
			name: "subreport count mismatch",
			body: strings.Replace(validScheduleOutput(), `"snapshots": 1`, `"snapshots": 2`, 1),
			want: "sub_report_ids length",
		},
		{
			name: "schedule action is report batch workflow",
			body: strings.Replace(validScheduleOutput(), `"workflow_id": "report-policy-schedule-9"`, `"workflow_id": "report-batch-1"`, 1),
			want: "schedule_action.workflow_id",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := run([]string{writeScheduleSmokeOutput(t, tc.body)})
			if err == nil {
				t.Fatal("run: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func validScheduleOutput() string {
	return `{
  "checked_at": "2026-06-06T01:02:03.456Z",
  "request": {
    "schedule_id": 9,
    "policy_id": 42,
    "temporal_schedule_id": "openclarion-report-policy-42-hourly",
    "observed_after": "2026-06-06T00:00:00Z",
    "wait_timeout": "30m0s"
  },
  "persisted_schedule": {
    "id": 9,
    "report_workflow_policy_id": 42,
    "temporal_schedule_id": "openclarion-report-policy-42-hourly",
    "enabled": true,
    "interval": "1h0m0s",
    "offset": "15m0s",
    "replay_window": "1h0m0s",
    "replay_delay": "0s",
    "replay_limit": 100,
    "catchup_window": "10m0s"
  },
  "waited": true,
  "schedule_action": {
    "schedule_time": "2026-06-06T00:45:00Z",
    "actual_time": "2026-06-06T00:45:01Z",
    "workflow_id": "report-policy-schedule-9",
    "run_id": "launcher-run-1"
  },
  "launcher_workflow": {
    "schedule_id": 9,
    "report_workflow_policy_id": 42,
    "temporal_schedule_id": "openclarion-report-policy-42-hourly",
    "fire_time": "2026-06-06T00:45:00Z",
    "window_start": "2026-06-05T23:45:00Z",
    "window_end": "2026-06-06T00:45:00Z",
    "correlation_key": "report-workflow-schedule:9:policy:42:2026-06-05T23:45:00Z:2026-06-06T00:45:00Z",
    "workflow_id": "report-batch-1",
    "events_loaded": 2,
    "snapshots": 1,
    "report_batch_workflow_started": true,
    "report_batch_workflow_id": "report-batch-1",
    "report_batch_run_id": "report-run-1"
  },
  "report_workflow_result": {
    "sub_report_ids": [11],
    "final_report_id": 99,
    "notification_idempotency_key": "final_report:99/notification",
    "provider_message_id": "message-1",
    "notification_status": "accepted"
  }
}`
}

func writeScheduleSmokeOutput(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schedule-output.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write schedule output: %v", err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}
