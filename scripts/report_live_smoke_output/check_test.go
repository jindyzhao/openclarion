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
		return time.Date(2026, 5, 29, 2, 0, 0, 0, time.UTC)
	}
}

func TestRunAcceptsCompletedLiveSmokeOutput(t *testing.T) {
	tests := []struct {
		name              string
		status            string
		providerMessageID string
	}{
		{name: "accepted with provider message id", status: "accepted", providerMessageID: "webhook-message-99"},
		{name: "delivered with provider message id", status: "delivered", providerMessageID: "webhook-message-99"},
		{name: "delivered without provider message id", status: "delivered"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSmokeOutput(t, completedOutput(tc.status, tc.providerMessageID))
			if err := run([]string{path}); err != nil {
				t.Fatalf("run: %v", err)
			}
		})
	}
}

func TestRunRejectsSymlinkLiveSmokeOutput(t *testing.T) {
	target := writeSmokeOutput(t, completedOutput("accepted", "webhook-message-99"))
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

func TestRunRejectsOversizedLiveSmokeOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized-output.json")
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", int(maxProofBytes)+1)), 0o600); err != nil {
		t.Fatalf("write oversized proof: %v", err)
	}

	err := run([]string{path})
	if err == nil {
		t.Fatal("run: want oversized proof rejection")
	}
	if !strings.Contains(err.Error(), "exceeds maximum proof size") {
		t.Fatalf("error = %q, want oversized proof rejection", err.Error())
	}
}

func completedOutput(status, providerMessageID string) string {
	body := `{
  "checked_at": "2026-05-29T01:02:03.456Z",
  "request": {
    "window_start": "2026-05-29T00:00:00Z",
    "window_end": "2026-05-29T01:00:00Z",
    "limit": 10000,
    "scenario": "single_alert",
    "correlation_key": "incident-1",
    "workflow_id": "report-batch-1",
    "wait": true,
    "wait_timeout": "20m0s"
  },
  "started": true,
  "workflow_id": "report-batch-1",
  "run_id": "run-1",
  "waited": true,
  "workflow_result": {
    "sub_report_ids": [11],
    "final_report_id": 99,
    "notification_idempotency_key": "final_report:99/notification",
    "provider_message_id": "PROVIDER_MESSAGE_ID",
    "notification_status": "accepted"
  },
  "stats": {
    "ingested": {"total": 1, "saved": 1, "duplicate": 0, "failed": 0},
    "events_loaded": 1,
    "groups_built": 1,
    "groups_saved": 1,
    "groups_refreshed": 0,
    "groups_existing": 0,
    "snapshots_saved": 1,
    "snapshots_duplicate": 0,
    "groups_closed": 1,
    "failed": 0
  },
  "snapshots": [
    {"id": 7, "group_index": 0, "event_count": 1}
  ]
}`
	body = strings.Replace(body, `"notification_status": "accepted"`, `"notification_status": "`+status+`"`, 1)
	return strings.Replace(body, `"provider_message_id": "PROVIDER_MESSAGE_ID"`, `"provider_message_id": "`+providerMessageID+`"`, 1)
}

func TestRunRejectsIncompleteLiveSmokeOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing checked at",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z",`, "", 1),
			want: "checked_at",
		},
		{
			name: "bad checked at",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"not-time"`, 1),
			want: "checked_at",
		},
		{
			name: "future checked at",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2999-01-01T00:00:00Z"`, 1),
			want: "future",
		},
		{
			name: "checked at whitespace",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":" 2026-05-29T01:02:03Z "`, 1),
			want: "checked_at must not contain leading or trailing whitespace",
		},
		{
			name: "checked at offset",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2026-05-29T09:02:03+08:00"`, 1),
			want: "checked_at must be canonical UTC RFC3339",
		},
		{
			name: "checked at non canonical fractional seconds",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2026-05-29T01:02:03.000000000Z"`, 1),
			want: "checked_at must be canonical UTC RFC3339",
		},
		{
			name: "missing request",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"request":{"window_start":"2026-05-29T00:00:00Z","window_end":"2026-05-29T01:00:00Z","limit":10000,"scenario":"single_alert","correlation_key":"incident-1","workflow_id":"report-batch-1","wait":true,"wait_timeout":"20m0s"},`, "", 1),
			want: "request.window_start",
		},
		{
			name: "bad request window start",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"window_start":"2026-05-29T00:00:00Z"`, `"window_start":"not-time"`, 1),
			want: "request.window_start",
		},
		{
			name: "request window end before start",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"window_end":"2026-05-29T01:00:00Z"`, `"window_end":"2026-05-28T23:00:00Z"`, 1),
			want: "window_end must be after",
		},
		{
			name: "request future window end",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"window_end":"2026-05-29T01:00:00Z"`, `"window_end":"2026-05-29T03:00:00Z"`, 1),
			want: "window_end must not be after checked_at",
		},
		{
			name: "request window start offset",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"window_start":"2026-05-29T00:00:00Z"`, `"window_start":"2026-05-29T08:00:00+08:00"`, 1),
			want: "request.window_start must be canonical UTC RFC3339",
		},
		{
			name: "request limit zero",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"limit":10000`, `"limit":0`, 1),
			want: "request.limit",
		},
		{
			name: "request policy id negative",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"request":{"window_start"`, `"request":{"policy_id":-1,"window_start"`, 1),
			want: "request.policy_id",
		},
		{
			name: "request bad scenario",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"scenario":"single_alert"`, `"scenario":"other"`, 1),
			want: "request.scenario",
		},
		{
			name: "request did not wait",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"wait":true`, `"wait":false`, 1),
			want: "request.wait",
		},
		{
			name: "request bad wait timeout",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"wait_timeout":"20m0s"`, `"wait_timeout":"soon"`, 1),
			want: "request.wait_timeout",
		},
		{
			name: "request workflow id multiline",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"workflow_id":"report-batch-1"`, `"workflow_id":"report-batch-1\nnext"`, 1),
			want: "request.workflow_id",
		},
		{
			name: "request workflow id whitespace",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"workflow_id":"report-batch-1"`, `"workflow_id":"report batch 1"`, 1),
			want: "request.workflow_id must not contain whitespace",
		},
		{
			name: "workflow id mismatch",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"started":true,"workflow_id":"report-batch-1"`, `"started":true,"workflow_id":"other"`, 1),
			want: "request.workflow_id must match workflow_id",
		},
		{
			name: "workflow id whitespace",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"started":true,"workflow_id":"report-batch-1"`, `"started":true,"workflow_id":"report batch 1"`, 1),
			want: "workflow_id must not contain whitespace",
		},
		{
			name: "workflow id oversized",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"started":true,"workflow_id":"report-batch-1"`, `"started":true,"workflow_id":"`+strings.Repeat("w", 257)+`"`, 1),
			want: "workflow_id exceeds 256 bytes",
		},
		{
			name: "run id whitespace",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"run_id":"run"`, `"run_id":" run "`, 1),
			want: "run_id must not contain leading or trailing whitespace",
		},
		{
			name: "run id oversized",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"run_id":"run"`, `"run_id":"`+strings.Repeat("r", 257)+`"`, 1),
			want: "run_id exceeds 256 bytes",
		},
		{
			name: "not waited",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"waited":true`, `"waited":false`, 1),
			want: "waited",
		},
		{
			name: "missing notification status",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1"}`),
			want: "notification_status",
		},
		{
			name: "missing notification idempotency key",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"provider_message_id":"msg-1","notification_status":"accepted"}`),
			want: "notification_idempotency_key",
		},
		{
			name: "wrong notification idempotency key",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:3/notification","provider_message_id":"msg-1","notification_status":"accepted"}`),
			want: "notification_idempotency_key",
		},
		{
			name: "notification idempotency key whitespace",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":" final_report:2/notification ","provider_message_id":"msg-1","notification_status":"accepted"}`),
			want: "notification_idempotency_key must not contain leading or trailing whitespace",
		},
		{
			name: "provider message id whitespace",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":" msg-1 ","notification_status":"accepted"}`),
			want: "provider_message_id must not contain leading or trailing whitespace",
		},
		{
			name: "provider message id multiline",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1\nnext","notification_status":"accepted"}`),
			want: "provider_message_id must be a single-line value",
		},
		{
			name: "provider message id oversized",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"` + strings.Repeat("m", 513) + `","notification_status":"accepted"}`),
			want: "provider_message_id exceeds 512 bytes",
		},
		{
			name: "failed notification status",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1","notification_status":"failed"}`),
			want: "accepted or delivered",
		},
		{
			name: "notification status whitespace",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1","notification_status":" delivered "}`),
			want: "leading or trailing whitespace",
		},
		{
			name: "empty snapshots",
			body: validOutputWith(`"snapshots":[]`),
			want: "snapshots",
		},
		{
			name: "duplicate snapshot id",
			body: strings.Replace(validTwoSnapshotOutput(), `"id":2,"group_index":1`, `"id":1,"group_index":1`, 1),
			want: "snapshots[1].id duplicates id 1",
		},
		{
			name: "snapshot group index gap",
			body: strings.Replace(validTwoSnapshotOutput(), `"id":2,"group_index":1`, `"id":2,"group_index":2`, 1),
			want: "snapshots[1].group_index must equal snapshot index",
		},
		{
			name: "events loaded exceeds request limit",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"events_loaded":1`, `"events_loaded":10001`, 1),
			want: "stats.events_loaded must be <= request.limit",
		},
		{
			name: "group stats do not add up",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"groups_existing":0`, `"groups_existing":1`, 1),
			want: "stats group counts",
		},
		{
			name: "groups built differs from snapshots length",
			body: func() string {
				body := validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`)
				body = strings.Replace(body, `"groups_built":1`, `"groups_built":2`, 1)
				return strings.Replace(body, `"groups_saved":1`, `"groups_saved":2`, 1)
			}(),
			want: "stats.groups_built must equal snapshots length",
		},
		{
			name: "snapshot counters differ from snapshots length",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"snapshots_saved":1`, `"snapshots_saved":2`, 1),
			want: "stats snapshots saved+duplicate must equal snapshots length",
		},
		{
			name: "groups closed exceeds groups built",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"groups_closed":1`, `"groups_closed":2`, 1),
			want: "stats.groups_closed",
		},
		{
			name: "snapshot event counts do not match events loaded",
			body: strings.Replace(validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`), `"event_count":1`, `"event_count":2`, 1),
			want: "stats.events_loaded must equal sum",
		},
		{
			name: "ingest failure",
			body: validOutputWith(`"stats":{"ingested":{"total":1,"saved":0,"duplicate":0,"failed":1},"events_loaded":1,"groups_built":1,"groups_saved":1,"groups_refreshed":0,"groups_existing":0,"snapshots_saved":1,"snapshots_duplicate":0,"groups_closed":1,"failed":0}`),
			want: "stats.ingested.failed",
		},
		{
			name: "replay failure",
			body: validOutputWith(`"stats":{"ingested":{"total":1,"saved":1,"duplicate":0,"failed":0},"events_loaded":1,"groups_built":1,"groups_saved":1,"groups_refreshed":0,"groups_existing":0,"snapshots_saved":1,"snapshots_duplicate":0,"groups_closed":1,"failed":1}`),
			want: "stats.failed",
		},
		{
			name: "subreports and snapshots mismatch",
			body: validOutputWith(`"workflow_result":{"sub_report_ids":[1,2],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1","notification_status":"accepted"}`),
			want: "sub_report_ids length",
		},
		{
			name: "duplicate subreport id",
			body: strings.Replace(validTwoSnapshotOutput(), `"sub_report_ids":[1,2]`, `"sub_report_ids":[1,1]`, 1),
			want: "workflow_result.sub_report_ids[1] duplicates id 1",
		},
		{
			name: "duplicate top level key",
			body: strings.Replace(
				validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`),
				`"started":true,"workflow_id":"report-batch-1"`,
				`"started":true,"workflow_id":"report-batch-1","workflow_id":"other"`,
				1,
			),
			want: "duplicate object key",
		},
		{
			name: "unknown proof field",
			body: strings.Replace(
				validOutputWith(`"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","notification_status":"accepted"}`),
				`"started":true`,
				`"unexpected":"stale evidence","started":true`,
				1,
			),
			want: `unknown field "unexpected"`,
		},
		{
			name: "log polluted output",
			body: `{"started":true}
{"started":true}`,
			want: "trailing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSmokeOutput(t, tc.body)
			err := run([]string{path})
			if err == nil {
				t.Fatal("run: want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func validOutputWith(replacement string) string {
	body := `{"checked_at":"2026-05-29T01:02:03Z","request":{"window_start":"2026-05-29T00:00:00Z","window_end":"2026-05-29T01:00:00Z","limit":10000,"scenario":"single_alert","correlation_key":"incident-1","workflow_id":"report-batch-1","wait":true,"wait_timeout":"20m0s"},"started":true,"workflow_id":"report-batch-1","run_id":"run","waited":true,"workflow_result":{"sub_report_ids":[1],"final_report_id":2,"notification_idempotency_key":"final_report:2/notification","provider_message_id":"msg-1","notification_status":"accepted"},"stats":{"ingested":{"total":1,"saved":1,"duplicate":0,"failed":0},"events_loaded":1,"groups_built":1,"groups_saved":1,"groups_refreshed":0,"groups_existing":0,"snapshots_saved":1,"snapshots_duplicate":0,"groups_closed":1,"failed":0},"snapshots":[{"id":1,"group_index":0,"event_count":1}]}`
	for _, field := range []string{`"workflow_result":`, `"stats":`, `"snapshots":`} {
		if strings.HasPrefix(replacement, field) {
			start := strings.Index(body, field)
			if start == -1 {
				panic("fixture field not found")
			}
			next := nextTopLevelField(body, start+len(field))
			if next == -1 {
				return body[:start] + replacement + "}"
			}
			return body[:start] + replacement + "," + body[next:]
		}
	}
	panic("unsupported replacement")
}

func validTwoSnapshotOutput() string {
	body := validOutputWith(`"snapshots":[{"id":1,"group_index":0,"event_count":1},{"id":2,"group_index":1,"event_count":1}]`)
	body = strings.Replace(body, `"sub_report_ids":[1]`, `"sub_report_ids":[1,2]`, 1)
	body = strings.Replace(body, `"events_loaded":1`, `"events_loaded":2`, 1)
	body = strings.Replace(body, `"groups_built":1`, `"groups_built":2`, 1)
	body = strings.Replace(body, `"groups_saved":1`, `"groups_saved":2`, 1)
	body = strings.Replace(body, `"snapshots_saved":1`, `"snapshots_saved":2`, 1)
	body = strings.Replace(body, `"groups_closed":1`, `"groups_closed":2`, 1)
	return body
}

func nextTopLevelField(body string, start int) int {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(body); i++ {
		ch := body[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func writeSmokeOutput(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "output.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write output fixture: %v", err)
	}
	return path
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
}
