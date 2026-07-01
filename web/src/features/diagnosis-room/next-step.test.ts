import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "./api";
import {
  diagnosisRoomNextStep,
  diagnosisRoomQueueOptions,
  diagnosisRoomWorkflowUnavailable,
  filterDiagnosisRoomsByQueue,
} from "./next-step";

describe("diagnosis room next step", () => {
  it("prioritizes failed notifications as attention", () => {
    const step = diagnosisRoomNextStep(
      room({
        notification_timeline: [
          {
            event_kind: "diagnosis_room.final_ready_notification_sent",
            provider_status: "failed",
            occurred_at: "2026-06-20T00:04:00Z",
          },
        ],
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "error",
      label: "Notification failed",
    });
  });

  it("keeps earlier failed notifications visible when a later notification exists", () => {
    const step = diagnosisRoomNextStep(
      room({
        notification_timeline: [
          {
            event_kind: "diagnosis_room.final_ready_notification_sent",
            provider_status: "failed",
            occurred_at: "2026-06-20T00:04:00Z",
          },
          {
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:05:00Z",
          },
        ],
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "error",
      label: "Notification failed",
    });
  });

  it("keeps failed close notifications visible after the room is closed", () => {
    const step = diagnosisRoomNextStep(
      room({
        close_reason: "operator confirmed final conclusion",
        closed_at: "2026-06-20T00:06:00Z",
        notification_timeline: [
          {
            event_kind: "diagnosis_room.close_notification_sent",
            provider_status: "failed",
            occurred_at: "2026-06-20T00:07:00Z",
          },
        ],
        room_status: "closed",
        task_status: "succeeded",
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "error",
      label: "Notification failed",
    });
  });

  it("classifies AI notifications without output proof as attention", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_conclusion: conclusion({
          confidence: "high",
          requires_human_review: false,
        }),
        notification_timeline: [
          {
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:04:00Z",
          },
        ],
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "warning",
      label: "AI proof missing",
    });
    expect(step.detail).toContain("raw alert forwarding");
  });

  it("classifies partial AI notification delivery proof as attention", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_conclusion: conclusion({
          confidence: "high",
          requires_human_review: false,
        }),
        notification_timeline: [
          {
            content_kind: "assistant_message",
            content_sha256: "a".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:04:00Z",
          },
        ],
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      label: "AI delivery incomplete",
    });
    expect(step.detail).toContain("AI notification phases");
  });

  it("keeps rooms ready when full AI notification delivery proof is retained", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_conclusion: conclusion({
          confidence: "high",
          requires_human_review: false,
        }),
        notification_timeline: [
          {
            content_kind: "assistant_message",
            content_sha256: "a".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:04:00Z",
          },
          {
            content_kind: "final_conclusion",
            content_sha256: "b".repeat(64),
            event_kind: "diagnosis_room.final_ready_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:05:00Z",
          },
          {
            content_kind: "final_conclusion",
            content_sha256: "c".repeat(64),
            event_kind: "diagnosis_room.close_notification_sent",
            provider_status: "delivered",
            occurred_at: "2026-06-20T00:06:00Z",
          },
        ],
      }),
    );

    expect(step).toMatchObject({
      bucket: "ready",
      label: "Review conclusion",
    });
  });

  it("keeps closed confirmed rooms in attention until AI delivery proof starts", () => {
    const step = diagnosisRoomNextStep(
      room({
        close_reason: "operator confirmed final conclusion",
        closed_at: "2026-06-20T00:06:00Z",
        latest_conclusion: conclusion({
          confidence: "high",
          confirmed_by: "operator-1",
          requires_human_review: false,
        }),
        notification_timeline: [],
        room_status: "closed",
        task_status: "succeeded",
      }),
    );

    expect(step).toEqual({
      bucket: "attention",
      color: "warning",
      detail:
        "Operator-confirmed conclusion is retained, but AI delivery proof has not started. Verify assistant update, final conclusion, and close notification proof before treating the room as delivered.",
      label: "AI delivery not started",
    });
  });

  it("classifies human review and lower confidence conclusions as attention", () => {
    expect(
      diagnosisRoomNextStep(
        room({
          latest_conclusion: conclusion({
            confidence: "high",
            requires_human_review: true,
          }),
        }),
      ),
    ).toMatchObject({ bucket: "attention", label: "Human review" });

    expect(
      diagnosisRoomNextStep(
        room({
          latest_conclusion: conclusion({
            confidence: "medium",
            requires_human_review: false,
          }),
        }),
      ),
    ).toMatchObject({ bucket: "attention", label: "Improve confidence" });
  });

  it("classifies high-confidence conclusions as ready", () => {
    expect(
      diagnosisRoomNextStep(
        room({
          latest_conclusion: conclusion({
            confidence: "high",
            requires_human_review: false,
          }),
        }),
      ),
    ).toMatchObject({ bucket: "ready", label: "Review conclusion" });
  });

  it("classifies rooms without conclusions as active", () => {
    expect(diagnosisRoomNextStep(room({ turn_count: 0 }))).toMatchObject({
      bucket: "active",
      label: "Start AI review",
    });
    expect(diagnosisRoomNextStep(room({ turn_count: 2 }))).toMatchObject({
      bucket: "active",
      label: "Continue AI review",
    });
  });

  it("does not ask operators to manually start zero-turn automatic rooms", () => {
    const step = diagnosisRoomNextStep(
      room({
        session_id: "diagnosis-session-auto-p3-s247",
        workflow_id: "diagnosis-room-diagnosis-session-auto-p3-s247",
        turn_count: 0,
      }),
    );

    expect(step).toEqual({
      bucket: "active",
      color: "processing",
      detail:
        "Automatic diagnosis has started from alert evidence. Wait for the first AI report or refresh the room state before sending an operator prompt.",
      label: "AI review queued",
    });
  });

  it("classifies latest progress evidence gaps as attention", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_progress: progress({
          conclusion_status: "needs_evidence",
          evidence_request_count: 2,
          missing_evidence_requests: [
            {
              label: "Owner action",
              detail: "Attach the current remediation status.",
              priority: "high",
            },
          ],
          evidence_collection_suggestions: [
            {
              label: "Recent active alerts",
              detail: "Collect active alerts for this service.",
              priority: "medium",
            },
          ],
        }),
        turn_count: 1,
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      label: "Collect evidence",
    });
    expect(step.detail).toContain("2 planned");
    expect(step.detail).toContain("1 missing");
  });

  it("prioritizes submitted supplemental evidence that the latest AI turn has not reviewed", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_progress: progress({
          assistant_sequence: 3,
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [
            {
              label: "Owner action",
              detail: "Attach the current remediation status.",
              priority: "high",
            },
          ],
          supplemental_evidence: [
            {
              assistant_sequence: 2,
              detail: "Attach the current remediation status.",
              evidence: "Owner started rollback at 10:12 UTC.",
              label: "Owner action",
              priority: "high",
              provided_at: "2026-06-20T00:03:00Z",
            },
          ],
        }),
        turn_count: 2,
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "processing",
      label: "Reassess evidence",
    });
    expect(step.detail).toContain("latest AI turn");
  });

  it("does not flag supplemental evidence reviewed by the latest AI turn", () => {
    const step = diagnosisRoomNextStep(
      room({
        latest_progress: progress({
          assistant_sequence: 3,
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [
            {
              label: "Owner action",
              detail: "Attach the current remediation status.",
              priority: "high",
            },
          ],
          supplemental_evidence: [
            {
              assistant_sequence: 3,
              detail: "Attach the current remediation status.",
              evidence: "Owner started rollback at 10:12 UTC.",
              label: "Owner action",
              priority: "high",
              provided_at: "2026-06-20T00:03:00Z",
            },
          ],
        }),
        turn_count: 2,
      }),
    );

    expect(step).toMatchObject({
      bucket: "attention",
      color: "warning",
      label: "Collect evidence",
    });
  });

  it("classifies ready-for-review progress as ready", () => {
    expect(
      diagnosisRoomNextStep(
        room({
          latest_progress: progress({
            conclusion_status: "ready_for_review",
            confidence: "high",
            confidence_rationale: "Collected evidence supports confirmation.",
          }),
          turn_count: 2,
        }),
      ),
    ).toMatchObject({
      bucket: "ready",
      label: "Review AI report",
    });
  });

  it("classifies open rooms with missing workflow visibility as attention", () => {
    expect(
      diagnosisRoomNextStep(
        room({
          workflow_visibility: {
            status: "not_found",
          },
        }),
      ),
    ).toMatchObject({
      bucket: "attention",
      label: "Workflow unavailable",
    });

    expect(
      diagnosisRoomNextStep(
        room({
          room_status: "closed",
          task_status: "succeeded",
          workflow_visibility: {
            status: "completed",
          },
        }),
      ),
    ).toMatchObject({ bucket: "closed", label: "Closed" });
  });

  it("marks terminal open workflow visibility as unavailable", () => {
    expect(
      diagnosisRoomWorkflowUnavailable(
        room({
          workflow_visibility: {
            status: "failed",
          },
        }),
      ),
    ).toBe(true);
    expect(
      diagnosisRoomWorkflowUnavailable(
        room({
          workflow_visibility: {
            status: "running",
          },
        }),
      ),
    ).toBe(false);
    expect(
      diagnosisRoomWorkflowUnavailable(
        room({
          room_status: "closed",
          workflow_visibility: {
            status: "completed",
          },
        }),
      ),
    ).toBe(false);
  });

  it("classifies closed rooms and filters queue counts", () => {
    const rooms = [
      room({ session_id: "attention", task_status: "failed" }),
      room({
        latest_conclusion: conclusion({
          confidence: "high",
          requires_human_review: false,
        }),
        session_id: "ready",
      }),
      room({ session_id: "active", turn_count: 1 }),
      room({
        close_reason: "operator confirmed",
        closed_at: "2026-06-20T00:05:00Z",
        room_status: "closed",
        session_id: "closed",
        task_status: "succeeded",
      }),
    ];

    expect(
      filterDiagnosisRoomsByQueue(rooms, "attention").map(
        (item) => item.session_id,
      ),
    ).toEqual(["attention"]);
    expect(
      filterDiagnosisRoomsByQueue(rooms, "ready").map(
        (item) => item.session_id,
      ),
    ).toEqual(["ready"]);
    expect(diagnosisRoomQueueOptions(rooms)).toEqual([
      { count: 4, label: "All", value: "all" },
      { count: 1, label: "Attention", value: "attention" },
      { count: 1, label: "Ready", value: "ready" },
      { count: 1, label: "Active", value: "active" },
      { count: 1, label: "Closed", value: "closed" },
    ]);
  });
});

function room(
  overrides: Partial<DiagnosisRoomSummary> = {},
): DiagnosisRoomSummary {
  return {
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
    ...overrides,
  };
}

function conclusion(
  overrides: Partial<
    NonNullable<DiagnosisRoomSummary["latest_conclusion"]>
  > = {},
): NonNullable<DiagnosisRoomSummary["latest_conclusion"]> {
  return {
    diagnosis_task_id: 101,
    session_id: "diagnosis-session-1",
    chat_session_id: 202,
    event_kind: "diagnosis_room.final_conclusion_ready",
    status: "available",
    source: "latest_assistant_turn",
    content: "Capacity pressure requires operator review.",
    recorded_at: "2026-06-20T00:03:00Z",
    ...overrides,
  };
}

function progress(
  overrides: Partial<NonNullable<DiagnosisRoomSummary["latest_progress"]>> = {},
): NonNullable<DiagnosisRoomSummary["latest_progress"]> {
  return {
    diagnosis_task_id: 101,
    session_id: "diagnosis-session-1",
    chat_session_id: 202,
    event_kind: "diagnosis_room.turn_persisted",
    status: "in_progress",
    evidence_snapshot_id: 7,
    confidence: "low",
    requires_human_review: true,
    evidence_request_count: 0,
    occurred_at: "2026-06-20T00:02:00Z",
    recorded_at: "2026-06-20T00:02:01Z",
    ...overrides,
  };
}
