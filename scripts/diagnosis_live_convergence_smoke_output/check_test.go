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
		return time.Date(2026, 6, 20, 5, 0, 0, 0, time.UTC)
	}
}

func TestRunAcceptsCreatedRoomConvergenceProof(t *testing.T) {
	path := writeProof(t, validConvergenceProof())

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsPlannedEvidenceConvergenceProof(t *testing.T) {
	path := writeProof(t, validPlannedEvidenceConvergenceProof())

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsFinalConvergenceProof(t *testing.T) {
	proof := replaceAllProof(t, validPlannedEvidenceConvergenceProof(),
		`"conclusion_status":"ready_for_review"`,
		`"conclusion_status":"final"`)
	proof = replaceAllProof(t, proof,
		`"requires_human_review":true`,
		`"requires_human_review":false`)
	proof = replaceAllProof(t, proof,
		`"confidence":"low","requires_human_review":false,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2`,
		`"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2`)
	path := writeProof(t, proof)

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsExistingSessionConvergenceProof(t *testing.T) {
	path := writeProof(t, validExistingSessionConvergenceProof(t))

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsExistingSessionAutoEvidenceProof(t *testing.T) {
	path := writeProof(t, validExistingSessionAutoEvidenceProof())

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsExistingSessionWithoutConfirmationProof(t *testing.T) {
	path := writeProof(t, validExistingSessionWithoutConfirmationProof())

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsRecoveredTimeoutConvergenceProof(t *testing.T) {
	path := writeProof(t, validRecoveredTimeoutConvergenceProof(t))

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunAcceptsNotificationProof(t *testing.T) {
	path := writeProof(t, validNotificationConvergenceProof(t))

	if err := run([]string{path}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunRejectsSymlinkConvergenceProof(t *testing.T) {
	target := writeProof(t, validConvergenceProof())
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

func TestRunRejectsOversizedConvergenceProof(t *testing.T) {
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

func TestRunRejectsInvalidConvergenceProof(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "not passed",
			body: replaceProof(t, validConvergenceProof(), `"passed":true`, `"passed":false`),
			want: "passed must be true",
		},
		{
			name: "bad mode",
			body: replaceProof(t, validConvergenceProof(), `"mode":"direct_ws_convergence"`, `"mode":"browser"`),
			want: "mode",
		},
		{
			name: "unknown field",
			body: replaceProof(t, validConvergenceProof(), `"error":""`, `"error":"","unexpected":true`),
			want: `unknown field "unexpected"`,
		},
		{
			name: "duplicate key",
			body: replaceProof(t, validConvergenceProof(), `"passed":true`, `"passed":true,"passed":true`),
			want: `duplicate object key "passed"`,
		},
		{
			name: "create room without confirmation request",
			body: replaceProof(t, validConvergenceProof(), `"confirm_conclusion_requested":true`, `"confirm_conclusion_requested":false`),
			want: "request.confirm_conclusion_requested must be true for create_room",
		},
		{
			name: "missing collection stage",
			body: replaceProof(t, validConvergenceProof(), `{"at":"2026-06-20T04:01:10Z","name":"collecting_evidence","request_count":2,"tools":["active_alerts","metric_query"]},`, ""),
			want: `stages must include "collecting_evidence"`,
		},
		{
			name: "initial turn without confidence improvement path",
			body: replaceAllProof(t, validConvergenceProof(),
				`"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2`,
				`"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":0,"missing":0`),
			want: "initial_turn_frame must show an initial confidence-improvement evidence path",
		},
		{
			name: "still needs evidence",
			body: replaceAllProof(t, validConvergenceProof(), `"conclusion_status":"ready_for_review"`, `"conclusion_status":"needs_evidence"`),
			want: "conclusion_status",
		},
		{
			name: "ready for review without human review",
			body: replaceAllProof(t, validConvergenceProof(), `"requires_human_review":true`, `"requires_human_review":false`),
			want: "requires_human_review",
		},
		{
			name: "repeated evidence request",
			body: replaceAllProof(t, validConvergenceProof(), `"evidence_requests":0`, `"evidence_requests":1`),
			want: "evidence_requests",
		},
		{
			name: "low confidence",
			body: replaceAllProof(t, validConvergenceProof(), `"confidence":"medium"`, `"confidence":"low"`),
			want: "confidence",
		},
		{
			name: "confirmation skipped",
			body: replaceProof(t, validConvergenceProof(), `"requested":true`, `"requested":false`),
			want: "confirmation.requested",
		},
		{
			name: "confirmation requested on unconfirmed existing session proof",
			body: replaceProof(t, validExistingSessionWithoutConfirmationProof(), `"requested":false`, `"requested":true`),
			want: "confirmation.requested must be false",
		},
		{
			name: "confirmation not closed",
			body: replaceAllProof(t, validConvergenceProof(), `"status":"closed"`, `"status":"open"`),
			want: "confirmation.final_state.status",
		},
		{
			name: "confirmation without final conclusion",
			body: replaceAllProof(t, validConvergenceProof(), `"final_conclusion_available":true`, `"final_conclusion_available":false`),
			want: "final_conclusion_available",
		},
		{
			name: "supplemental skip with supplemental turn",
			body: replaceProof(t, validPlannedEvidenceConvergenceProof(),
				`{"at":"2026-06-20T04:04:01Z","name":"supplemental_skipped_ready_for_review","reason":"planned_evidence_already_converged","turn_count":2,"confidence":"high","collection_results":2,"evidence_requests":2,"missing":0}`,
				`{"at":"2026-06-20T04:04:01Z","name":"supplemental_skipped_ready_for_review","reason":"planned_evidence_already_converged","turn_count":2,"confidence":"high","collection_results":2,"evidence_requests":2,"missing":0},
    {"at":"2026-06-20T04:04:02Z","name":"supplemental_turn_frame","type":"state","status":"open","turn_count":3,"confidence":"high","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":0}`),
			want: "supplemental skip proof must not include supplemental turn stages",
		},
		{
			name: "supplemental skip under collected evidence",
			body: replaceAllProof(t, validPlannedEvidenceConvergenceProof(), `"collection_results":2`, `"collection_results":1`),
			want: "collection_results must cover evidence_requests",
		},
		{
			name: "supplemental skip negative missing evidence",
			body: replaceAllProof(t, validPlannedEvidenceConvergenceProof(), `"missing":0`, `"missing":-1`),
			want: "missing must be >= 0",
		},
		{
			name: "supplemental skip negative evidence requests",
			body: replaceAllProof(t, validPlannedEvidenceConvergenceProof(), `"evidence_requests":2`, `"evidence_requests":-1`),
			want: "evidence_requests must be >= 0",
		},
		{
			name: "notification requested without proof",
			body: replaceProof(t, validNotificationConvergenceProof(t), `"requested":true,
    "passed":true,`, `"requested":false,
    "passed":true,`),
			want: "notification_proof.requested",
		},
		{
			name: "notification wrong channel",
			body: replaceProof(t, validNotificationConvergenceProof(t), `"notification_channel_profile_id":7,`, `"notification_channel_profile_id":8,`),
			want: "notification_channel_profile_id = 7, want 8",
		},
		{
			name: "notification raw alert content",
			body: replaceProof(t, validNotificationConvergenceProof(t), `"content_kind":"assistant_message"`, `"content_kind":"raw_alert"`),
			want: "content_kind",
		},
		{
			name: "notification missing close phase",
			body: replaceProof(t, validNotificationConvergenceProof(t), `,
      {
        "event_kind":"diagnosis_room.close_notification_sent",
        "notification_channel_profile_id":7,
        "provider_status":"delivered",
        "provider_message_id":"wecom-close-1",
        "assistant_message_id":"msg-1/assistant",
        "assistant_turn_id":42,
        "turn_count":3,
        "content_kind":"final_conclusion",
        "content_sha256":"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
        "recommended_action_count":2,
        "evidence_request_count":0,
        "occurred_at":"2026-06-20T04:08:04Z"
      }`, ""),
			want: `missing required event_kind "diagnosis_room.close_notification_sent"`,
		},
		{
			name: "notification missing content digest",
			body: replaceProof(t, validNotificationConvergenceProof(t), `"content_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`, `"content_sha256":""`),
			want: "content_sha256",
		},
		{
			name: "notification queued provider status",
			body: replaceProof(t, validNotificationConvergenceProof(t), `"provider_status":"delivered"`, `"provider_status":"queued"`),
			want: "want accepted, delivered, sent, or success",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeProof(t, tt.body)
			err := run([]string{path})
			if err == nil {
				t.Fatal("run: want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRunRejectsUsage(t *testing.T) {
	err := run(nil)
	if err == nil {
		t.Fatal("run: want usage error")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error = %q, want usage", err.Error())
	}
}

func writeProof(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proof.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write proof: %v", err)
	}
	return path
}

func replaceProof(t *testing.T, body, old, replacement string) string {
	t.Helper()
	if !strings.Contains(body, old) {
		t.Fatalf("proof fixture missing %q", old)
	}
	return strings.Replace(body, old, replacement, 1)
}

func replaceAllProof(t *testing.T, body, old, replacement string) string {
	t.Helper()
	if !strings.Contains(body, old) {
		t.Fatalf("proof fixture missing %q", old)
	}
	return strings.ReplaceAll(body, old, replacement)
}

func createSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
}

func validConvergenceProof() string {
	return `{
  "passed":true,
  "checked_at":"2026-06-20T04:08:04.239Z",
  "mode":"direct_ws_convergence",
  "request":{
    "existing_session_id":null,
    "evidence_snapshot_id":310,
    "collect_planned_evidence":true,
    "submit_supplemental_evidence":true,
    "confirm_conclusion_requested":true,
    "mode":"create_room"
  },
  "stages":[
    {"at":"2026-06-20T04:00:00Z","name":"room_created","session_id":"diagnosis-session-test","diagnosis_task_id":539,"workflow_id":"diagnosis-room-diagnosis-session-test","run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"open","turn_count":0,"in_flight":false,"evidence_requests":0},
    {"at":"2026-06-20T04:00:02Z","name":"initial_turn_before","type":"state","status":"open","turn_count":0,"in_flight":false},
    {"at":"2026-06-20T04:01:00Z","name":"initial_turn_frame","type":"turn_result","status":"open","turn_count":1,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:01Z","name":"initial_turn_frame","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:10Z","name":"collecting_evidence","request_count":2,"tools":["active_alerts","metric_query"]},
    {"at":"2026-06-20T04:01:11Z","name":"collect_turn_before","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:04:00Z","name":"collect_turn_frame","type":"state","status":"open","turn_count":2,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"collection_results":2,"missing":3},
    {"at":"2026-06-20T04:04:01Z","name":"submitting_supplemental_boundary","label":"Application Logs","priority":"high","evidence_sha256_16":"34b344b51032420b"},
    {"at":"2026-06-20T04:04:02Z","name":"supplemental_turn_before","type":"state","status":"open","turn_count":2,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"collection_results":2,"missing":3},
    {"at":"2026-06-20T04:08:00Z","name":"supplemental_turn_frame","type":"turn_result","status":"open","turn_count":3,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":2},
    {"at":"2026-06-20T04:08:01Z","name":"supplemental_turn_frame","type":"state","status":"open","turn_count":3,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":2},
    {"at":"2026-06-20T04:08:04.239Z","name":"confirm_frame","type":"state","status":"closed","turn_count":3,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":2,"final_conclusion_available":true,"close_reason":"human_confirmed"},
    {"at":"2026-06-20T04:08:04.24Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}
  ],
  "session_id":"diagnosis-session-test",
  "created_room":{
    "session_id":"diagnosis-session-test",
    "diagnosis_task_id":539,
    "workflow_id":"diagnosis-room-diagnosis-session-test",
    "run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"
  },
  "final_state":{
    "type":"state",
    "message":"",
    "status":"open",
    "turn_count":3,
    "in_flight":false,
    "confidence":"medium",
    "requires_human_review":true,
    "conclusion_status":"ready_for_review",
    "evidence_requests":0,
    "collection_results":0,
    "missing":2,
    "final_conclusion_available":false,
    "close_reason":""
  },
  "confirmation":{
    "checked_at":"2026-06-20T04:08:04.239Z",
    "requested":true,
    "final_state":{
      "type":"state",
      "message":"",
      "status":"closed",
      "turn_count":3,
      "in_flight":false,
      "confidence":"medium",
      "requires_human_review":true,
      "conclusion_status":"ready_for_review",
      "evidence_requests":0,
      "collection_results":0,
      "missing":2,
      "final_conclusion_available":true,
      "close_reason":"human_confirmed"
    },
    "passed":true
  },
  "error":""
}`
}

func validPlannedEvidenceConvergenceProof() string {
	return `{
  "passed":true,
  "checked_at":"2026-06-20T04:08:04.239Z",
  "mode":"direct_ws_convergence",
  "request":{
    "existing_session_id":null,
    "evidence_snapshot_id":310,
    "collect_planned_evidence":true,
    "submit_supplemental_evidence":true,
    "confirm_conclusion_requested":true,
    "mode":"create_room"
  },
  "stages":[
    {"at":"2026-06-20T04:00:00Z","name":"room_created","session_id":"diagnosis-session-test","diagnosis_task_id":539,"workflow_id":"diagnosis-room-diagnosis-session-test","run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"open","turn_count":0,"in_flight":false,"evidence_requests":0},
    {"at":"2026-06-20T04:00:02Z","name":"initial_turn_before","type":"state","status":"open","turn_count":0,"in_flight":false},
    {"at":"2026-06-20T04:01:00Z","name":"initial_turn_frame","type":"turn_result","status":"open","turn_count":1,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:01Z","name":"initial_turn_frame","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:10Z","name":"collecting_evidence","request_count":2,"tools":["active_alerts","metric_query"]},
    {"at":"2026-06-20T04:01:11Z","name":"collect_turn_before","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:04:00Z","name":"collect_turn_frame","type":"state","status":"open","turn_count":2,"in_flight":false,"confidence":"high","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":2,"collection_results":2,"missing":0},
    {"at":"2026-06-20T04:04:01Z","name":"supplemental_skipped_ready_for_review","reason":"planned_evidence_already_converged","turn_count":2,"confidence":"high","collection_results":2,"evidence_requests":2,"missing":0},
    {"at":"2026-06-20T04:08:04.239Z","name":"confirm_frame","type":"state","status":"closed","turn_count":2,"in_flight":false,"confidence":"high","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":2,"collection_results":2,"missing":0,"final_conclusion_available":true,"close_reason":"human_confirmed"},
    {"at":"2026-06-20T04:08:04.24Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}
  ],
  "session_id":"diagnosis-session-test",
  "created_room":{
    "session_id":"diagnosis-session-test",
    "diagnosis_task_id":539,
    "workflow_id":"diagnosis-room-diagnosis-session-test",
    "run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"
  },
  "final_state":{
    "type":"state",
    "message":"",
    "status":"open",
    "turn_count":2,
    "in_flight":false,
    "confidence":"high",
    "requires_human_review":true,
    "conclusion_status":"ready_for_review",
    "evidence_requests":2,
    "collection_results":2,
    "missing":0,
    "final_conclusion_available":true,
    "close_reason":""
  },
  "confirmation":{
    "checked_at":"2026-06-20T04:08:04.239Z",
    "requested":true,
    "final_state":{
      "type":"state",
      "message":"",
      "status":"closed",
      "turn_count":2,
      "in_flight":false,
      "confidence":"high",
      "requires_human_review":true,
      "conclusion_status":"ready_for_review",
      "evidence_requests":2,
      "collection_results":2,
      "missing":0,
      "final_conclusion_available":true,
      "close_reason":"human_confirmed"
    },
    "passed":true
  },
  "error":""
}`
}

func validExistingSessionConvergenceProof(t *testing.T) string {
	t.Helper()
	body := validConvergenceProof()
	body = replaceProof(t, body,
		`"existing_session_id":null,
    "evidence_snapshot_id":310,`,
		`"existing_session_id":"diagnosis-session-test",
    "evidence_snapshot_id":null,`)
	body = replaceProof(t, body, `"mode":"create_room"`, `"mode":"existing_session"`)
	body = replaceProof(t, body,
		`    {"at":"2026-06-20T04:00:00Z","name":"room_created","session_id":"diagnosis-session-test","diagnosis_task_id":539,"workflow_id":"diagnosis-room-diagnosis-session-test","run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"open","turn_count":0,"in_flight":false,"evidence_requests":0},
    {"at":"2026-06-20T04:00:02Z","name":"initial_turn_before","type":"state","status":"open","turn_count":0,"in_flight":false},
    {"at":"2026-06-20T04:01:00Z","name":"initial_turn_frame","type":"turn_result","status":"open","turn_count":1,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:01Z","name":"initial_turn_frame","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},`,
		`    {"at":"2026-06-20T04:00:00Z","name":"using_existing_session","session_id":"diagnosis-session-test"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},`)
	body = replaceProof(t, body,
		`  "created_room":{
    "session_id":"diagnosis-session-test",
    "diagnosis_task_id":539,
    "workflow_id":"diagnosis-room-diagnosis-session-test",
    "run_id":"6b7dd94c-1358-474b-af9c-351b2f0888af"
  },
`,
		``)
	return body
}

func validExistingSessionAutoEvidenceProof() string {
	return `{
  "passed":true,
  "checked_at":"2026-06-20T04:08:04.239Z",
  "mode":"direct_ws_convergence",
  "request":{
    "existing_session_id":"diagnosis-session-test",
    "evidence_snapshot_id":null,
    "collect_planned_evidence":true,
    "submit_supplemental_evidence":true,
    "confirm_conclusion_requested":true,
    "mode":"existing_session"
  },
  "stages":[
    {"at":"2026-06-20T04:00:00Z","name":"using_existing_session","session_id":"diagnosis-session-test"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"open","turn_count":2,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":2,"collection_results":2,"missing":1,"final_conclusion_available":true},
    {"at":"2026-06-20T04:04:01Z","name":"submitting_supplemental_boundary","label":"Smoke test execution logs","priority":"low","evidence_sha256_16":"34b344b51032420b"},
    {"at":"2026-06-20T04:04:02Z","name":"supplemental_turn_before","type":"state","status":"open","turn_count":2,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":2,"collection_results":2,"missing":1},
    {"at":"2026-06-20T04:08:00Z","name":"supplemental_turn_frame","type":"turn_result","status":"open","turn_count":3,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":0},
    {"at":"2026-06-20T04:08:01Z","name":"supplemental_turn_frame","type":"state","status":"open","turn_count":3,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":0,"final_conclusion_available":true},
    {"at":"2026-06-20T04:08:04.239Z","name":"confirm_frame","type":"state","status":"closed","turn_count":3,"in_flight":false,"confidence":"medium","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":0,"missing":0,"final_conclusion_available":true,"close_reason":"human_confirmed"},
    {"at":"2026-06-20T04:08:04.24Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}
  ],
  "session_id":"diagnosis-session-test",
  "final_state":{
    "type":"state",
    "message":"",
    "status":"open",
    "turn_count":3,
    "in_flight":false,
    "confidence":"medium",
    "requires_human_review":true,
    "conclusion_status":"ready_for_review",
    "evidence_requests":0,
    "collection_results":0,
    "missing":0,
    "final_conclusion_available":true,
    "close_reason":""
  },
  "confirmation":{
    "checked_at":"2026-06-20T04:08:04.239Z",
    "requested":true,
    "final_state":{
      "type":"state",
      "message":"",
      "status":"closed",
      "turn_count":3,
      "in_flight":false,
      "confidence":"medium",
      "requires_human_review":true,
      "conclusion_status":"ready_for_review",
      "evidence_requests":0,
      "collection_results":0,
      "missing":0,
      "final_conclusion_available":true,
      "close_reason":"human_confirmed"
    },
    "passed":true
  },
  "error":""
}`
}

func validExistingSessionWithoutConfirmationProof() string {
	return `{
  "passed":true,
  "checked_at":"2026-06-20T04:08:04.239Z",
  "mode":"direct_ws_convergence",
  "request":{
    "existing_session_id":"diagnosis-session-test",
    "evidence_snapshot_id":null,
    "collect_planned_evidence":true,
    "submit_supplemental_evidence":true,
    "confirm_conclusion_requested":false,
    "mode":"existing_session"
  },
  "stages":[
    {"at":"2026-06-20T04:00:00Z","name":"using_existing_session","session_id":"diagnosis-session-test"},
    {"at":"2026-06-20T04:00:01Z","name":"initial_state","type":"state","status":"closed","turn_count":2,"in_flight":false,"confidence":"high","requires_human_review":true,"conclusion_status":"ready_for_review","evidence_requests":2,"collection_results":2,"missing":1,"final_conclusion_available":true,"close_reason":"idle_timeout"},
    {"at":"2026-06-20T04:04:01Z","name":"supplemental_skipped_ready_for_review","reason":"planned_evidence_already_converged","turn_count":2,"confidence":"high","collection_results":2,"evidence_requests":2,"missing":1},
    {"at":"2026-06-20T04:08:04.24Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}
  ],
  "session_id":"diagnosis-session-test",
  "final_state":{
    "type":"state",
    "message":"",
    "status":"closed",
    "turn_count":2,
    "in_flight":false,
    "confidence":"high",
    "requires_human_review":true,
    "conclusion_status":"ready_for_review",
    "evidence_requests":2,
    "collection_results":2,
    "missing":1,
    "final_conclusion_available":true,
    "close_reason":"idle_timeout"
  },
  "confirmation":{
    "checked_at":"2026-06-20T04:08:04.239Z",
    "requested":false,
    "skipped_reason":"confirmation_not_requested",
    "passed":true
  },
  "error":""
}`
}

func validRecoveredTimeoutConvergenceProof(t *testing.T) string {
	t.Helper()
	return replaceProof(t, validConvergenceProof(),
		`    {"at":"2026-06-20T04:01:00Z","name":"initial_turn_frame","type":"turn_result","status":"open","turn_count":1,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},
    {"at":"2026-06-20T04:01:01Z","name":"initial_turn_frame","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},`,
		`    {"at":"2026-06-20T04:01:00Z","name":"initial_turn_timeout_recovery","timeout_ms":360000,"recovery_timeout_ms":360000,"poll_ms":5000},
    {"at":"2026-06-20T04:01:01Z","name":"initial_turn_frame","type":"state","status":"open","turn_count":1,"in_flight":false,"confidence":"low","requires_human_review":true,"conclusion_status":"needs_evidence","evidence_requests":2,"missing":2},`)
}

func validNotificationConvergenceProof(t *testing.T) string {
	t.Helper()
	body := replaceProof(t, validConvergenceProof(),
		`"evidence_snapshot_id":310,`,
		`"evidence_snapshot_id":310,
    "notification_channel_profile_id":7,
    "require_notification_proof":true,`)
	body = replaceProof(t, body,
		`    {"at":"2026-06-20T04:08:04.24Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}`,
		`    {"at":"2026-06-20T04:08:04.24Z","name":"notification_proof","requested":true,"passed":true,"entry_count":3},
    {"at":"2026-06-20T04:08:04.25Z","name":"proof_written","output_path":".openclarion-private/diagnosis-live-convergence-smoke/latest.json","passed":true}`)
	body = replaceProof(t, body,
		`  "error":""`,
		`  "notification_proof":{
    "checked_at":"2026-06-20T04:08:04.24Z",
    "requested":true,
    "passed":true,
    "entries":[
      {
        "event_kind":"diagnosis_room.assistant_turn_notification_sent",
        "notification_channel_profile_id":7,
        "provider_status":"delivered",
        "provider_message_id":"wecom-assistant-1",
        "assistant_message_id":"msg-1/assistant",
        "assistant_turn_id":42,
        "turn_count":1,
        "content_kind":"assistant_message",
        "content_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        "recommended_action_count":2,
        "evidence_request_count":1,
        "occurred_at":"2026-06-20T04:08:03Z"
      },
      {
        "event_kind":"diagnosis_room.final_ready_notification_sent",
        "notification_channel_profile_id":7,
        "provider_status":"delivered",
        "provider_message_id":"wecom-final-1",
        "assistant_message_id":"msg-1/assistant",
        "assistant_turn_id":42,
        "turn_count":3,
        "content_kind":"final_conclusion",
        "content_sha256":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
        "recommended_action_count":2,
        "evidence_request_count":0,
        "occurred_at":"2026-06-20T04:08:03.5Z"
      },
      {
        "event_kind":"diagnosis_room.close_notification_sent",
        "notification_channel_profile_id":7,
        "provider_status":"delivered",
        "provider_message_id":"wecom-close-1",
        "assistant_message_id":"msg-1/assistant",
        "assistant_turn_id":42,
        "turn_count":3,
        "content_kind":"final_conclusion",
        "content_sha256":"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
        "recommended_action_count":2,
        "evidence_request_count":0,
        "occurred_at":"2026-06-20T04:08:04Z"
      }
    ]
  },
  "error":""`)
	return body
}
