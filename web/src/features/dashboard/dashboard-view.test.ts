import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";

import { diagnosisRoomHealth } from "./dashboard-view";

describe("dashboard diagnosis room health", () => {
  it("counts an earlier notification failure after a later delivery", () => {
    const health = diagnosisRoomHealth([
      room({
        notification_timeline: [
          {
            event_kind: "diagnosis_room.final_ready_notification_sent",
            provider_status: "failed",
            occurred_at: "2026-06-20T00:04:00Z"
          },
          {
            event_kind: "diagnosis_room.close_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:05:00Z"
          }
        ]
      })
    ]);

    expect(health).toMatchObject({
      attention: 1,
      notificationFailures: 1
    });
  });
});

function room(overrides: Partial<DiagnosisRoomSummary> = {}): DiagnosisRoomSummary {
  return {
    approval_mode: "single",
    session_id: "diagnosis-session-1",
    chat_session_id: 202,
    diagnosis_task_id: 101,
    evidence_snapshot_id: 7,
    workflow_id: "diagnosis-room-diagnosis-session-1",
    run_id: "run-1",
    task_status: "running",
    room_status: "open",
    turn_count: 1,
    started_at: "2026-06-20T00:00:00Z",
    last_activity_at: "2026-06-20T00:01:00Z",
    closed_at: null,
    close_reason: "",
    created_at: "2026-06-20T00:00:00Z",
    updated_at: "2026-06-20T00:01:00Z",
    ...overrides
  };
}
