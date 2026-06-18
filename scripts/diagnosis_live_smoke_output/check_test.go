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

const testMessageSHA256 = "2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"

func TestRunAcceptsExistingRoomLiveSmokeProof(t *testing.T) {
	path := writeSmokeProof(t, `{
  "passed": true,
  "checked_at": "2026-05-29T01:02:03.456Z",
  "request": {
    "mode": "existing_session",
    "session_id": "manual-session-1",
    "evidence_snapshot_id": null,
    "message_length": 42,
    "message_sha256": "`+testMessageSHA256+`"
  },
  "web_base_url": "http://127.0.0.1:32101",
  "api_base_url": "https://openclarion.example.test",
  "session_id": "manual-session-1",
  "evidence_snapshot_id": null,
  "created_room": null,
  "message_length": 42,
  "message_sha256": "`+testMessageSHA256+`",
  "browser": {
    "state_loaded": true,
    "turn_result_observed": true,
    "assistant_turns_before": 1,
    "assistant_turns_after": 2,
    "assistant_turn_delta": 1,
    "transcript_messages_before": 2,
    "transcript_messages_after": 4,
    "connection_status_after_turn": "connected",
    "submitted_message_visible": true,
    "submitted_message_length": 42,
    "submitted_message_sha256": "`+testMessageSHA256+`",
    "completed_turn_text": "Turn 2 completed.",
    "consultation_insight_visible": true,
    "consultation_progress_visible": true,
    "evidence_readiness_visible": true,
    "confidence": "medium",
    "confidence_aria_value": "medium confidence",
    "evidence_readiness_text": "Plan 1 Collected 1 Missing 1 Suggestions 1 Next Collect missing evidence"
  },
  "evidence": "Playwright live diagnosis-room browser smoke passed one connect/query/submit/turn_result round trip."
}`)

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsCreatedRoomLiveSmokeProof(t *testing.T) {
	path := writeSmokeProof(t, `{
  "passed": true,
  "checked_at": "2026-05-29T01:02:03Z",
  "request": {
    "mode": "create_room",
    "session_id": "diagnosis-session-abc",
    "evidence_snapshot_id": 7,
    "message_length": 1,
    "message_sha256": "`+testMessageSHA256+`"
  },
  "web_base_url": "http://127.0.0.1:32101",
  "api_base_url": "http://127.0.0.1:8080",
  "session_id": "diagnosis-session-abc",
  "evidence_snapshot_id": 7,
  "created_room": {
    "session_id": "diagnosis-session-abc",
    "evidence_snapshot_id": 7,
    "diagnosis_task_id": 101,
    "chat_session_id": 202,
    "workflow_id": "diagnosis-room-diagnosis-session-abc",
    "run_id": "run-1"
  },
  "message_length": 1,
  "message_sha256": "`+testMessageSHA256+`",
  "browser": {
    "state_loaded": true,
    "turn_result_observed": true,
    "assistant_turns_before": 0,
    "assistant_turns_after": 1,
    "assistant_turn_delta": 1,
    "transcript_messages_before": 0,
    "transcript_messages_after": 2,
    "connection_status_after_turn": "connected",
    "submitted_message_visible": true,
    "submitted_message_length": 1,
    "submitted_message_sha256": "`+testMessageSHA256+`",
    "completed_turn_text": "Turn 1 completed.",
    "consultation_insight_visible": true,
    "consultation_progress_visible": true,
    "evidence_readiness_visible": true,
    "confidence": "high",
    "confidence_aria_value": "high confidence",
    "evidence_readiness_text": "Plan 1 Collected 1 Missing 0 Suggestions 0 Next Ready for confirmation"
  },
  "evidence": "connect/query/submit/turn_result completed"
}`)

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsLiveSmokeProofWithCloseNotification(t *testing.T) {
	path := writeSmokeProof(t, validCreatedProofWithCloseNotification())

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsLiveSmokeProofWithAutoEvidenceFollowUp(t *testing.T) {
	body := validExistingProof()
	body = replaceProof(t, body, `"assistant_turns_after":1`, `"assistant_turns_after":2`)
	body = replaceProof(t, body, `"assistant_turn_delta":1`, `"assistant_turn_delta":2`)
	body = replaceProof(t, body, `"transcript_messages_after":2`, `"transcript_messages_after":4`)
	body = replaceProof(t, body, `"completed_turn_text":"Turn 1 completed."`, `"completed_turn_text":"Turn 2 completed."`)
	path := writeSmokeProof(t, body)

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunRejectsSymlinkLiveSmokeProof(t *testing.T) {
	target := writeSmokeProof(t, validExistingProof())
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

func TestRunRejectsOversizedLiveSmokeProof(t *testing.T) {
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

func TestRunRejectsInvalidLiveSmokeProof(t *testing.T) {
	tests := []struct {
		name string
		body func(*testing.T) string
		want string
	}{
		{
			name: "not passed",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"passed":true`, `"passed":false`)
			},
			want: "passed",
		},
		{
			name: "bad checked at",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"not-time"`)
			},
			want: "checked_at",
		},
		{
			name: "future checked at",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2999-01-01T00:00:00Z"`)
			},
			want: "future",
		},
		{
			name: "checked at whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":" 2026-05-29T01:02:03Z "`)
			},
			want: "checked_at must not contain leading or trailing whitespace",
		},
		{
			name: "checked at offset",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2026-05-29T09:02:03+08:00"`)
			},
			want: "checked_at must be canonical UTC RFC3339",
		},
		{
			name: "checked at non canonical fractional seconds",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"checked_at":"2026-05-29T01:02:03Z"`, `"checked_at":"2026-05-29T01:02:03.000000000Z"`)
			},
			want: "checked_at must be canonical UTC RFC3339",
		},
		{
			name: "missing request",
			body: func(t *testing.T) string {
				return removeProof(t, validExistingProof(), `"request":{"mode":"existing_session","session_id":"s","evidence_snapshot_id":null,"message_length":1,"message_sha256":"`+testMessageSHA256+`"},`)
			},
			want: "request.mode",
		},
		{
			name: "request bad mode",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"mode":"existing_session"`, `"mode":"other"`)
			},
			want: "request.mode",
		},
		{
			name: "request create room without created room",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"mode":"existing_session"`, `"mode":"create_room"`)
			},
			want: "created_room",
		},
		{
			name: "request existing session with created room",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"mode":"create_room"`, `"mode":"existing_session"`)
			},
			want: "existing_session",
		},
		{
			name: "request session mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"request":{"mode":"existing_session","session_id":"s"`, `"request":{"mode":"existing_session","session_id":"other"`)
			},
			want: "request.session_id",
		},
		{
			name: "request message length mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence_snapshot_id":null,"message_length":1,"message_sha256":"`+testMessageSHA256+`"}`, `"evidence_snapshot_id":null,"message_length":2,"message_sha256":"`+testMessageSHA256+`"}`)
			},
			want: "request.message_length",
		},
		{
			name: "missing message hash",
			body: func(t *testing.T) string {
				return removeProof(t, validExistingProof(), `"message_sha256":"`+testMessageSHA256+`",`)
			},
			want: "message_sha256",
		},
		{
			name: "bad message hash",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"message_sha256":"`+testMessageSHA256+`"`, `"message_sha256":"ABC"`)
			},
			want: "message_sha256 must be a lowercase SHA-256 hex digest",
		},
		{
			name: "request message hash mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"message_sha256":"`+testMessageSHA256+`"}`, `"message_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
			},
			want: "request.message_sha256 must match message_sha256",
		},
		{
			name: "request evidence snapshot mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"request":{"mode":"create_room","session_id":"s","evidence_snapshot_id":7`, `"request":{"mode":"create_room","session_id":"s","evidence_snapshot_id":8`)
			},
			want: "request.evidence_snapshot_id",
		},
		{
			name: "request evidence snapshot string",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"request":{"mode":"create_room","session_id":"s","evidence_snapshot_id":7`, `"request":{"mode":"create_room","session_id":"s","evidence_snapshot_id":"7"`)
			},
			want: "request.evidence_snapshot_id",
		},
		{
			name: "request evidence snapshot missing while proof has evidence snapshot",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"session_id":"s","evidence_snapshot_id":null,"created_room":null`, `"session_id":"s","evidence_snapshot_id":7,"created_room":null`)
			},
			want: "request.evidence_snapshot_id",
		},
		{
			name: "bad api url",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"api_base_url":"http://127.0.0.1:8080"`, `"api_base_url":"file:///tmp/api"`)
			},
			want: "api_base_url",
		},
		{
			name: "api url with query",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"api_base_url":"http://127.0.0.1:8080"`, `"api_base_url":"http://127.0.0.1:8080?state=redacted"`)
			},
			want: "query string",
		},
		{
			name: "web url with fragment",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"web_base_url":"http://127.0.0.1:32101"`, `"web_base_url":"http://127.0.0.1:32101#diagnosis"`)
			},
			want: "fragment",
		},
		{
			name: "api url with user info",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"api_base_url":"http://127.0.0.1:8080"`, `"api_base_url":"http://user@127.0.0.1:8080"`)
			},
			want: "user info",
		},
		{
			name: "blank session",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"session_id":"s","evidence_snapshot_id"`, `"session_id":"","evidence_snapshot_id"`)
			},
			want: "session_id",
		},
		{
			name: "session id contains whitespace",
			body: func(_ *testing.T) string {
				body := validExistingProof()
				return strings.ReplaceAll(body, `"session_id":"s"`, `"session_id":"s x"`)
			},
			want: "session_id must not contain whitespace",
		},
		{
			name: "oversized session id",
			body: func(_ *testing.T) string {
				body := validExistingProof()
				longSessionID := strings.Repeat("a", maxProofSessionIDBytes+1)
				return strings.ReplaceAll(body, `"session_id":"s"`, `"session_id":"`+longSessionID+`"`)
			},
			want: "session_id exceeds 128 bytes",
		},
		{
			name: "request session id whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"request":{"mode":"existing_session","session_id":"s"`, `"request":{"mode":"existing_session","session_id":"s\t"`)
			},
			want: "request.session_id must not contain leading or trailing whitespace",
		},
		{
			name: "zero message length",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"created_room":null,"message_length":1`, `"created_room":null,"message_length":0`)
			},
			want: "message_length",
		},
		{
			name: "missing top-level message hash",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"message_length":1,"message_sha256":"`+testMessageSHA256+`","browser"`, `"message_length":1,"browser"`)
			},
			want: "message_sha256",
		},
		{
			name: "bad top-level message hash",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"message_length":1,"message_sha256":"`+testMessageSHA256+`","browser"`, `"message_length":1,"message_sha256":"ABC","browser"`)
			},
			want: "message_sha256 must be a lowercase SHA-256 hex digest",
		},
		{
			name: "weak evidence",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence":"turn_result"`, `"evidence":"browser opened"`)
			},
			want: "turn_result",
		},
		{
			name: "evidence whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence":"turn_result"`, `"evidence":" turn_result "`)
			},
			want: "evidence must not contain leading or trailing whitespace",
		},
		{
			name: "evidence multiline",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence":"turn_result"`, `"evidence":"turn_result\nextra log"`)
			},
			want: "evidence must be a single-line value",
		},
		{
			name: "evidence oversized",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence":"turn_result"`, `"evidence":"`+strings.Repeat("a", maxProofEvidenceBytes)+`turn_result"`)
			},
			want: "evidence exceeds 512 bytes",
		},
		{
			name: "close notification proof without evidence mention",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"evidence":"turn_result close_notification"`, `"evidence":"turn_result"`)
			},
			want: "evidence must mention close_notification",
		},
		{
			name: "missing browser proof",
			body: func(t *testing.T) string {
				return removeProof(t, validExistingProof(), `,"browser":{"state_loaded":true,"turn_result_observed":true,"assistant_turns_before":0,"assistant_turns_after":1,"assistant_turn_delta":1,"transcript_messages_before":0,"transcript_messages_after":2,"connection_status_after_turn":"connected","submitted_message_visible":true,"submitted_message_length":1,"submitted_message_sha256":"`+testMessageSHA256+`","completed_turn_text":"Turn 1 completed.","consultation_insight_visible":true,"consultation_progress_visible":true,"evidence_readiness_visible":true,"confidence":"medium","confidence_aria_value":"medium confidence","evidence_readiness_text":"Plan 1 Collected 1 Missing 1 Suggestions 1 Next Collect missing evidence"}`)
			},
			want: "browser proof",
		},
		{
			name: "browser assistant count did not increase",
			body: func(t *testing.T) string {
				body := replaceProof(t, validExistingProof(), `"assistant_turns_before":0`, `"assistant_turns_before":1`)
				body = replaceProof(t, body, `"transcript_messages_before":0`, `"transcript_messages_before":2`)
				body = replaceProof(t, body, `"transcript_messages_after":2`, `"transcript_messages_after":4`)
				return body
			},
			want: "assistant_turns_after",
		},
		{
			name: "browser transcript count did not increase by pair",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"transcript_messages_after":2`, `"transcript_messages_after":1`)
			},
			want: "transcript_messages_after",
		},
		{
			name: "browser assistant delta mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"assistant_turn_delta":1`, `"assistant_turn_delta":2`)
			},
			want: "assistant_turn_delta",
		},
		{
			name: "browser transcript count inconsistent before turn",
			body: func(t *testing.T) string {
				body := replaceProof(t, validExistingProof(), `"assistant_turns_before":0`, `"assistant_turns_before":1`)
				body = replaceProof(t, body, `"assistant_turns_after":1`, `"assistant_turns_after":2`)
				body = replaceProof(t, body, `"transcript_messages_before":0`, `"transcript_messages_before":1`)
				body = replaceProof(t, body, `"transcript_messages_after":2`, `"transcript_messages_after":3`)
				body = replaceProof(t, body, `"completed_turn_text":"Turn 1 completed."`, `"completed_turn_text":"Turn 2 completed."`)
				return body
			},
			want: "transcript_messages_before",
		},
		{
			name: "completed turn text mismatches assistant count",
			body: func(t *testing.T) string {
				body := replaceProof(t, validExistingProof(), `"assistant_turns_before":0`, `"assistant_turns_before":1`)
				body = replaceProof(t, body, `"assistant_turns_after":1`, `"assistant_turns_after":2`)
				body = replaceProof(t, body, `"transcript_messages_before":0`, `"transcript_messages_before":2`)
				body = replaceProof(t, body, `"transcript_messages_after":2`, `"transcript_messages_after":4`)
				return body
			},
			want: "completed_turn_text",
		},
		{
			name: "completed turn text has leading zero",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"completed_turn_text":"Turn 1 completed."`, `"completed_turn_text":"Turn 01 completed."`)
			},
			want: "completed_turn_text",
		},
		{
			name: "completed turn text whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"completed_turn_text":"Turn 1 completed."`, `"completed_turn_text":" Turn 1 completed. "`)
			},
			want: "browser.completed_turn_text must not contain leading or trailing whitespace",
		},
		{
			name: "browser status not connected",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"connection_status_after_turn":"connected"`, `"connection_status_after_turn":"closed"`)
			},
			want: "connection_status_after_turn",
		},
		{
			name: "browser missing submitted message length",
			body: func(t *testing.T) string {
				return removeProof(t, validExistingProof(), `"submitted_message_length":1,`)
			},
			want: "browser.submitted_message_length",
		},
		{
			name: "browser submitted message length mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"submitted_message_length":1`, `"submitted_message_length":2`)
			},
			want: "browser.submitted_message_length must match message_length",
		},
		{
			name: "browser missing submitted message hash",
			body: func(t *testing.T) string {
				return removeProof(t, validExistingProof(), `"submitted_message_sha256":"`+testMessageSHA256+`",`)
			},
			want: "browser.submitted_message_sha256",
		},
		{
			name: "browser submitted message hash mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"submitted_message_sha256":"`+testMessageSHA256+`"`, `"submitted_message_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`)
			},
			want: "browser.submitted_message_sha256 must match message_sha256",
		},
		{
			name: "browser missing consultation insight",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"consultation_insight_visible":true`, `"consultation_insight_visible":false`)
			},
			want: "browser.consultation_insight_visible",
		},
		{
			name: "browser bad confidence",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"confidence":"medium"`, `"confidence":"unknown"`)
			},
			want: "browser.confidence",
		},
		{
			name: "browser confidence aria mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"confidence_aria_value":"medium confidence"`, `"confidence_aria_value":"high confidence"`)
			},
			want: "browser.confidence_aria_value",
		},
		{
			name: "browser evidence readiness missing next step",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence_readiness_text":"Plan 1 Collected 1 Missing 1 Suggestions 1 Next Collect missing evidence"`, `"evidence_readiness_text":"Plan 1 Collected 1 Missing 1 Suggestions 1"`)
			},
			want: "browser.evidence_readiness_text",
		},
		{
			name: "evidence snapshot id string",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence_snapshot_id":null,"created_room":null`, `"evidence_snapshot_id":"7","created_room":null`)
			},
			want: "evidence_snapshot_id",
		},
		{
			name: "bad evidence snapshot id",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"evidence_snapshot_id":null,"created_room":null`, `"evidence_snapshot_id":1.5,"created_room":null`)
			},
			want: "evidence_snapshot_id",
		},
		{
			name: "created room mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"created_room":{"session_id":"s"`, `"created_room":{"session_id":"other"`)
			},
			want: "created_room.session_id",
		},
		{
			name: "created room workflow mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"workflow_id":"diagnosis-room-s"`, `"workflow_id":"other"`)
			},
			want: "workflow_id",
		},
		{
			name: "created room workflow id contains whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"workflow_id":"diagnosis-room-s"`, `"workflow_id":"diagnosis-room-s\ncontinued"`)
			},
			want: "created_room.workflow_id must not contain whitespace",
		},
		{
			name: "created room run id whitespace",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"run_id":"run"`, `"run_id":" run "`)
			},
			want: "created_room.run_id must not contain leading or trailing whitespace",
		},
		{
			name: "created room run id oversized",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProof(), `"run_id":"run"`, `"run_id":"`+strings.Repeat("a", maxProofRunIDBytes+1)+`"`)
			},
			want: "created_room.run_id exceeds 256 bytes",
		},
		{
			name: "close notification not signaled",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"signaled":true`, `"signaled":false`)
			},
			want: "close_notification.signaled",
		},
		{
			name: "close notification run id mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"run_id":"run"`, `"run_id":"other"`)
			},
			want: "close_notification.request.run_id",
		},
		{
			name: "close notification workflow turn mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"turn_count":1`, `"turn_count":2`)
			},
			want: "close_notification.workflow.turn_count",
		},
		{
			name: "close notification missing workflow final conclusion",
			body: func(t *testing.T) string {
				return removeProof(t, validCreatedProofWithCloseNotification(), `,"final_conclusion":{"status":"available","source":"latest_assistant_turn","evidence_snapshot_id":7,"conclusion_version":"diagnosis-room-close.v1","recorded_at":"2026-05-29T01:02:05Z","supplemental_context_refs":["chat_session:1/turn:3","chat_session:1/turn:4"],"assistant_turn_id":4,"assistant_message_id":"msg-1/assistant","assistant_sequence":2,"assistant_occurred_at":"2026-05-29T01:02:04Z","content":"CPU alert is still firing.","confidence":"medium","requires_human_review":true}`)
			},
			want: "close_notification.workflow.final_conclusion.status",
		},
		{
			name: "close notification final conclusion sequence mismatch",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"assistant_sequence":2`, `"assistant_sequence":3`)
			},
			want: "assistant_sequence",
		},
		{
			name: "close notification final conclusion event mismatch",
			body: func(t *testing.T) string {
				body := validCreatedProofWithCloseNotification()
				workflowConclusion := `"final_conclusion":{"status":"available","source":"latest_assistant_turn","evidence_snapshot_id":7,"conclusion_version":"diagnosis-room-close.v1","recorded_at":"2026-05-29T01:02:05Z","supplemental_context_refs":["chat_session:1/turn:3","chat_session:1/turn:4"],"assistant_turn_id":4,"assistant_message_id":"msg-1/assistant","assistant_sequence":2,"assistant_occurred_at":"2026-05-29T01:02:04Z","content":"CPU alert is still firing.","confidence":"medium","requires_human_review":true}`
				eventConclusion := `"final_conclusion":{"status":"available","source":"latest_assistant_turn","evidence_snapshot_id":7,"conclusion_version":"diagnosis-room-close.v1","recorded_at":"2026-05-29T01:02:05Z","supplemental_context_refs":["chat_session:1/turn:3","chat_session:1/turn:4"],"assistant_turn_id":4,"assistant_message_id":"msg-1/assistant","assistant_sequence":2,"assistant_occurred_at":"2026-05-29T01:02:04Z","content":"CPU alert recovered.","confidence":"medium","requires_human_review":true}`
				first := strings.Index(body, workflowConclusion)
				if first < 0 {
					t.Fatalf("workflow final conclusion fragment not found")
				}
				second := strings.Index(body[first+len(workflowConclusion):], workflowConclusion)
				if second < 0 {
					t.Fatalf("event final conclusion fragment not found")
				}
				offset := first + len(workflowConclusion) + second
				return body[:offset] + eventConclusion + body[offset+len(workflowConclusion):]
			},
			want: "must match close_notification.workflow.final_conclusion",
		},
		{
			name: "close notification bad idempotency key",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"idempotency_key":"diagnosis_room:1:abcdef/close_notification"`, `"idempotency_key":"final_report:1/notification"`)
			},
			want: "idempotency_key",
		},
		{
			name: "close notification bad provider status",
			body: func(t *testing.T) string {
				return replaceProof(t, validCreatedProofWithCloseNotification(), `"provider_status":"accepted"`, `"provider_status":"queued"`)
			},
			want: "accepted or delivered",
		},
		{
			name: "duplicate proof key",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"passed":true`, `"passed":true,"passed":false`)
			},
			want: "duplicate object key",
		},
		{
			name: "unknown proof field",
			body: func(t *testing.T) string {
				return replaceProof(t, validExistingProof(), `"passed":true`, `"unexpected":"stale evidence","passed":true`)
			},
			want: `unknown field "unexpected"`,
		},
		{
			name: "trailing output",
			body: func(_ *testing.T) string {
				return `{"passed":true}
{"passed":true}`
			},
			want: "trailing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeSmokeProof(t, tc.body(t))
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

func validExistingProof() string {
	return `{"passed":true,"checked_at":"2026-05-29T01:02:03Z","request":{"mode":"existing_session","session_id":"s","evidence_snapshot_id":null,"message_length":1,"message_sha256":"` + testMessageSHA256 + `"},"web_base_url":"http://127.0.0.1:32101","api_base_url":"http://127.0.0.1:8080","session_id":"s","evidence_snapshot_id":null,"created_room":null,"message_length":1,"message_sha256":"` + testMessageSHA256 + `","browser":{"state_loaded":true,"turn_result_observed":true,"assistant_turns_before":0,"assistant_turns_after":1,"assistant_turn_delta":1,"transcript_messages_before":0,"transcript_messages_after":2,"connection_status_after_turn":"connected","submitted_message_visible":true,"submitted_message_length":1,"submitted_message_sha256":"` + testMessageSHA256 + `","completed_turn_text":"Turn 1 completed.","consultation_insight_visible":true,"consultation_progress_visible":true,"evidence_readiness_visible":true,"confidence":"medium","confidence_aria_value":"medium confidence","evidence_readiness_text":"Plan 1 Collected 1 Missing 1 Suggestions 1 Next Collect missing evidence"},"evidence":"turn_result"}`
}

func validCreatedProof() string {
	return `{"passed":true,"checked_at":"2026-05-29T01:02:03Z","request":{"mode":"create_room","session_id":"s","evidence_snapshot_id":7,"message_length":1,"message_sha256":"` + testMessageSHA256 + `"},"web_base_url":"http://127.0.0.1:32101","api_base_url":"http://127.0.0.1:8080","session_id":"s","evidence_snapshot_id":7,"created_room":{"session_id":"s","evidence_snapshot_id":7,"diagnosis_task_id":1,"chat_session_id":1,"workflow_id":"diagnosis-room-s","run_id":"run"},"message_length":1,"message_sha256":"` + testMessageSHA256 + `","browser":{"state_loaded":true,"turn_result_observed":true,"assistant_turns_before":0,"assistant_turns_after":1,"assistant_turn_delta":1,"transcript_messages_before":0,"transcript_messages_after":2,"connection_status_after_turn":"connected","submitted_message_visible":true,"submitted_message_length":1,"submitted_message_sha256":"` + testMessageSHA256 + `","completed_turn_text":"Turn 1 completed.","consultation_insight_visible":true,"consultation_progress_visible":true,"evidence_readiness_visible":true,"confidence":"medium","confidence_aria_value":"medium confidence","evidence_readiness_text":"Plan 1 Collected 1 Missing 1 Suggestions 1 Next Collect missing evidence"},"evidence":"turn_result"}`
}

func validCreatedProofWithCloseNotification() string {
	return strings.Replace(
		validCreatedProof(),
		`,"evidence":"turn_result"}`,
		`,"close_notification":{"checked_at":"2026-05-29T01:03:00Z","request":{"session_id":"s","workflow_id":"diagnosis-room-s","run_id":"run","reason":"live_smoke_completed","wait_timeout":"2m0s"},"signaled":true,"workflow":{"session_id":"s","chat_session_id":1,"diagnosis_task_id":1,"status":"closed","turn_count":1,"closed_at":"2026-05-29T01:02:05Z","close_reason":"live_smoke_completed","final_conclusion":{"status":"available","source":"latest_assistant_turn","evidence_snapshot_id":7,"conclusion_version":"diagnosis-room-close.v1","recorded_at":"2026-05-29T01:02:05Z","supplemental_context_refs":["chat_session:1/turn:3","chat_session:1/turn:4"],"assistant_turn_id":4,"assistant_message_id":"msg-1/assistant","assistant_sequence":2,"assistant_occurred_at":"2026-05-29T01:02:04Z","content":"CPU alert is still firing.","confidence":"medium","requires_human_review":true}},"close_event":{"id":2,"kind":"diagnosis_room.closed","occurred_at":"2026-05-29T01:02:05Z","conclusion_version":"diagnosis-room-close.v1","final_conclusion":{"status":"available","source":"latest_assistant_turn","evidence_snapshot_id":7,"conclusion_version":"diagnosis-room-close.v1","recorded_at":"2026-05-29T01:02:05Z","supplemental_context_refs":["chat_session:1/turn:3","chat_session:1/turn:4"],"assistant_turn_id":4,"assistant_message_id":"msg-1/assistant","assistant_sequence":2,"assistant_occurred_at":"2026-05-29T01:02:04Z","content":"CPU alert is still firing.","confidence":"medium","requires_human_review":true}},"notification_event":{"id":3,"kind":"diagnosis_room.close_notification_sent","occurred_at":"2026-05-29T01:02:05.000001Z","idempotency_key":"diagnosis_room:1:abcdef/close_notification","provider_message_id":"msg-1","provider_status":"accepted"}},"evidence":"turn_result close_notification"}`,
		1,
	)
}

func replaceProof(t *testing.T, body, old, replacement string) string {
	t.Helper()
	if !strings.Contains(body, old) {
		t.Fatalf("proof fixture missing substring %q", old)
	}
	return strings.Replace(body, old, replacement, 1)
}

func removeProof(t *testing.T, body, old string) string {
	t.Helper()
	return replaceProof(t, body, old, "")
}

func writeSmokeProof(t *testing.T, body string) string {
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
