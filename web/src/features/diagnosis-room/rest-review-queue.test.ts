import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "./api";
import {
  diagnosisReviewQueueBlockingReason,
  diagnosisReviewQueuePostEvidenceStatus,
  diagnosisReviewQueueSummary,
  diagnosisReviewQueueItems,
} from "./review-queue";
import { diagnosisRoomSummaryReviewQueueInput } from "./rest-review-queue";

describe("REST diagnosis room review queue input", () => {
  it("maps latest progress and blocks stale supplemental evidence until AI reassesses it", () => {
    const input = diagnosisRoomSummaryReviewQueueInput({
      room: room({
        latest_progress: progress({
          assistant_sequence: 3,
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [ownerActionRequest()],
          supplemental_evidence: [
            supplementalEvidence({
              assistant_sequence: 2,
            }),
          ],
        }),
      }),
    });

    expect(input).toMatchObject({
      canConfirmConclusion: false,
      latestAssistantSequence: 3,
      missingEvidenceRequests: [ownerActionRequest()],
      supplementalEvidence: [
        expect.objectContaining({
          assistant_sequence: 2,
          label: "Owner action",
        }),
      ],
    });
    expect(diagnosisReviewQueueBlockingReason(input!)).toBe(
      "Wait for AI reassessment of submitted supplemental evidence before confirming.",
    );
    expect(diagnosisReviewQueuePostEvidenceStatus(input!)).toMatchObject({
      reviewed: 0,
      status: "submitted",
      submitted: 1,
    });
  });

  it("keeps missing sequence proof blocking instead of treating REST evidence as reviewed", () => {
    const input = diagnosisRoomSummaryReviewQueueInput({
      room: room({
        latest_progress: progress({
          assistant_sequence: 3,
          missing_evidence_requests: [ownerActionRequest()],
          supplemental_evidence: [
            supplementalEvidence({
              assistant_sequence: undefined,
            }),
          ],
        }),
      }),
    });

    expect(input?.supplementalEvidence?.[0]?.assistant_sequence).toBe(0);
    expect(diagnosisReviewQueueBlockingReason(input!)).toBe(
      "Wait for AI reassessment of submitted supplemental evidence before confirming.",
    );
  });

  it("maps latest conclusions as confirmable when reviewed supplemental evidence clears the gap", () => {
    const input = diagnosisRoomSummaryReviewQueueInput({
      canConfirmConclusion: true,
      room: room({
        latest_conclusion: conclusion({
          assistant_sequence: 4,
          missing_evidence_requests: [ownerActionRequest()],
          supplemental_evidence: [
            supplementalEvidence({
              assistant_sequence: 4,
            }),
          ],
        }),
      }),
    });
    const items = diagnosisReviewQueueItems(input!);
    const summary = diagnosisReviewQueueSummary(items, input!);

    expect(diagnosisReviewQueueBlockingReason(input!)).toBe("");
    expect(summary).toMatchObject({
      canConfirm: true,
      ready: 1,
    });
  });
});

function room(overrides: Partial<DiagnosisRoomSummary> = {}): DiagnosisRoomSummary {
  return {
    chat_session_id: 202,
    close_reason: "",
    closed_at: null,
    created_at: "2026-06-20T00:00:00Z",
    diagnosis_task_id: 101,
    evidence_snapshot_id: 7,
    last_activity_at: "2026-06-20T00:01:00Z",
    room_status: "open",
    run_id: "run-1",
    session_id: "diagnosis-session-1",
    started_at: "2026-06-20T00:00:00Z",
    task_status: "running",
    turn_count: 2,
    updated_at: "2026-06-20T00:01:00Z",
    workflow_id: "diagnosis-room-diagnosis-session-1",
    ...overrides,
  };
}

function progress(
  overrides: Partial<NonNullable<DiagnosisRoomSummary["latest_progress"]>> = {},
): NonNullable<DiagnosisRoomSummary["latest_progress"]> {
  return {
    confidence: "medium",
    diagnosis_task_id: 101,
    event_kind: "diagnosis_room.turn_persisted",
    evidence_request_count: 0,
    evidence_snapshot_id: 7,
    occurred_at: "2026-06-20T00:02:00Z",
    recorded_at: "2026-06-20T00:02:01Z",
    requires_human_review: true,
    status: "in_progress",
    ...overrides,
  };
}

function conclusion(
  overrides: Partial<NonNullable<DiagnosisRoomSummary["latest_conclusion"]>> = {},
): NonNullable<DiagnosisRoomSummary["latest_conclusion"]> {
  return {
    chat_session_id: 202,
    content: "Rollback evidence supports bounded confirmation.",
    diagnosis_task_id: 101,
    event_kind: "diagnosis_room.final_conclusion_ready",
    recorded_at: "2026-06-20T00:04:00Z",
    session_id: "diagnosis-session-1",
    source: "latest_assistant_turn",
    status: "available",
    ...overrides,
  };
}

function ownerActionRequest() {
  return {
    detail: "Attach the current remediation status.",
    label: "Owner action",
    priority: "high",
  };
}

function supplementalEvidence(
  overrides: Partial<
    NonNullable<
      NonNullable<DiagnosisRoomSummary["latest_progress"]>["supplemental_evidence"]
    >[number]
  > = {},
): NonNullable<
  NonNullable<DiagnosisRoomSummary["latest_progress"]>["supplemental_evidence"]
>[number] {
  return {
    assistant_message_id: "msg-2/assistant",
    assistant_sequence: 2,
    assistant_turn_id: 12,
    detail: "Attach the current remediation status.",
    evidence: "Owner started rollback at 10:12 UTC.",
    label: "Owner action",
    priority: "high",
    provided_at: "2026-06-20T00:03:00Z",
    user_message_id: "msg-2",
    user_sequence: 1,
    user_turn_id: 11,
    ...overrides,
  };
}
