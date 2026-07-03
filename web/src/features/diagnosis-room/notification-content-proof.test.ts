import { describe, expect, it } from "vitest";

import type {
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "./api";
import {
  diagnosisNotificationContentProofRetryEntry,
  diagnosisNotificationContentProofDisplay,
  diagnosisNotificationContentProofRetryRequired,
  diagnosisNotificationContentProofSummary,
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationDeliveryCoveragePhaseColor,
  diagnosisNotificationDeliveryProofExpected,
  diagnosisNotificationDeliveryRecoveryHint,
  diagnosisNotificationTimelineReviewActionRequired,
  diagnosisNotificationTimelineAnchorID,
  diagnosisNotificationTimelineHref,
  diagnosisNotificationTimelineReviewRequired,
  isDiagnosisAIContentNotification,
} from "./notification-content-proof";

describe("diagnosis notification content proof", () => {
  it("summarizes valid assistant-message proof without exposing content", () => {
    expect(
      diagnosisNotificationContentProofDisplay(
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
          evidence_request_count: 2,
          recommended_action_count: 1,
        }),
      ),
    ).toEqual({
      color: "success",
      detail:
        "AI assistant message digest aaaaaaaaaaaa / 1 action(s) / 2 evidence request(s)",
      digestPreview: "aaaaaaaaaaaa",
      evidenceRequestCount: 2,
      hasProof: true,
      kindLabel: "assistant message",
      label: "AI output proof",
      recommendedActionCount: 1,
    });
  });

  it("marks AI notification proof as missing when the digest is absent", () => {
    expect(
      diagnosisNotificationContentProofDisplay(
        notification({
          content_kind: "assistant_message",
        }),
      ),
    ).toEqual({
      color: "warning",
      detail:
        "Delivery status is present, but the notification is missing AI content proof.",
      hasProof: false,
      label: "AI proof missing",
    });
  });

  it("requires final-conclusion proof for close notifications", () => {
    expect(
      diagnosisNotificationContentProofDisplay(
        notification({
          content_kind: "final_conclusion",
          content_sha256: "d".repeat(64),
          event_kind: "diagnosis_room.close_notification_sent",
        }),
      ),
    ).toEqual({
      color: "success",
      detail: "AI final conclusion digest dddddddddddd",
      digestPreview: "dddddddddddd",
      evidenceRequestCount: undefined,
      hasProof: true,
      kindLabel: "final conclusion",
      label: "AI output proof",
      recommendedActionCount: undefined,
    });
  });

  it("identifies AI content notification event kinds", () => {
    expect(
      isDiagnosisAIContentNotification(
        "diagnosis_room.assistant_turn_notification_sent",
      ),
    ).toBe(true);
    expect(
      isDiagnosisAIContentNotification(
        "diagnosis_room.final_ready_notification_sent",
      ),
    ).toBe(true);
    expect(
      isDiagnosisAIContentNotification(
        "diagnosis_room.close_notification_sent",
      ),
    ).toBe(true);
  });

  it("summarizes verified AI notification proof across the timeline", () => {
    expect(
      diagnosisNotificationContentProofSummary([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "b".repeat(64),
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "c".repeat(64),
          event_kind: "diagnosis_room.close_notification_sent",
        }),
      ]),
    ).toEqual({
      color: "success",
      detail:
        "3 AI notification(s) include output digest proof, so the timeline can distinguish AI diagnosis delivery from raw alert forwarding.",
      label: "AI proof verified",
      missingCount: 0,
      provenCount: 3,
      totalAICount: 3,
    });
  });

  it("requires assistant, final, and close delivery proof for complete AI coverage", () => {
    expect(
      diagnosisNotificationDeliveryCoverage([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "b".repeat(64),
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "c".repeat(64),
          event_kind: "diagnosis_room.close_notification_sent",
        }),
      ]),
    ).toEqual({
      color: "success",
      detail:
        "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
      label: "AI delivery complete",
      phases: [
        {
          detail:
            "AI update notification was delivered with retained AI output proof.",
          eventKind: "diagnosis_room.assistant_turn_notification_sent",
          key: "assistant_update",
          label: "AI update",
          status: "delivered",
        },
        {
          detail:
            "Final conclusion notification was delivered with retained AI output proof.",
          eventKind: "diagnosis_room.final_ready_notification_sent",
          key: "final_conclusion",
          label: "Final conclusion",
          status: "delivered",
        },
        {
          detail:
            "Close notification was delivered with retained AI output proof.",
          eventKind: "diagnosis_room.close_notification_sent",
          key: "close",
          label: "Close",
          status: "delivered",
        },
      ],
      readyCount: 3,
      requiredCount: 3,
      status: "ready",
    });
  });

  it("marks AI delivery coverage incomplete when close proof is missing", () => {
    const coverage = diagnosisNotificationDeliveryCoverage([
      notification({
        content_kind: "assistant_message",
        content_sha256: "a".repeat(64),
      }),
      notification({
        content_kind: "final_conclusion",
        content_sha256: "b".repeat(64),
        event_kind: "diagnosis_room.final_ready_notification_sent",
      }),
    ]);

    expect(coverage).toMatchObject({
      color: "warning",
      label: "AI delivery incomplete",
      readyCount: 2,
      requiredCount: 3,
      status: "review",
      phases: [
        { key: "assistant_update", status: "delivered" },
        { key: "final_conclusion", status: "delivered" },
        { key: "close", status: "missing" },
      ],
    });
    expect(diagnosisNotificationDeliveryRecoveryHint(coverage)).toEqual({
      actionLabel: "Review trigger chain",
      color: "warning",
      detail:
        "Close has no retained notification event, so there is no individual event to retry yet. Verify the close/final-ready/assistant notification trigger path and refresh proof after the backend records a delivery event.",
      label: "Delivery phase missing",
    });
  });

  it("explains all-missing delivery proof as not started", () => {
    const coverage = diagnosisNotificationDeliveryCoverage([]);

    expect(coverage).toMatchObject({
      label: "AI delivery not started",
      status: "pending",
    });
    expect(diagnosisNotificationDeliveryRecoveryHint(coverage)).toEqual({
      actionLabel: "Review trigger chain",
      color: "info",
      detail:
        "AI update, Final conclusion, Close have no retained notification event, so there is no individual event to retry yet. Verify the close/final-ready/assistant notification trigger path and refresh proof after the backend records a delivery event.",
      label: "Delivery not started",
    });
  });

  it("surfaces retry guidance for failed and unproven delivery phases", () => {
    const failedCoverage = diagnosisNotificationDeliveryCoverage([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          event_kind: "diagnosis_room.final_ready_notification_sent",
          provider_status: "failed",
        }),
    ]);
    const unprovenCoverage = diagnosisNotificationDeliveryCoverage([
      notification({
        event_kind: "diagnosis_room.final_ready_notification_sent",
        provider_status: "accepted",
      }),
    ]);

    expect(diagnosisNotificationDeliveryRecoveryHint(failedCoverage)).toEqual({
      actionLabel: "Retry failed delivery",
      color: "error",
      detail:
        "Final conclusion failed provider delivery. Retry the retained event from the timeline or room list before accepting downstream delivery.",
      label: "Delivery retry required",
    });
    expect(diagnosisNotificationDeliveryRecoveryHint(unprovenCoverage)).toEqual({
      actionLabel: "Retry proof",
      color: "warning",
      detail:
        "Final conclusion delivered without AI output digest proof. Re-send the retained AI notification so the timeline proves this was AI diagnosis content, not raw alert forwarding.",
      label: "AI proof retry required",
    });
  });

  it("blocks AI delivery coverage when a required notification failed", () => {
    expect(
      diagnosisNotificationDeliveryCoverage([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          event_kind: "diagnosis_room.final_ready_notification_sent",
          provider_status: "failed",
        }),
      ]),
    ).toMatchObject({
      color: "error",
      label: "AI delivery failed",
      readyCount: 1,
      requiredCount: 3,
      status: "blocked",
      phases: [
        { key: "assistant_update", status: "delivered" },
        { key: "final_conclusion", status: "failed" },
        { key: "close", status: "missing" },
      ],
    });
  });

  it("marks delivered AI notifications without digest proof as unproven", () => {
    expect(
      diagnosisNotificationDeliveryCoverage([
        notification({
          event_kind: "diagnosis_room.final_ready_notification_sent",
          provider_status: "accepted",
        }),
      ]),
    ).toMatchObject({
      label: "AI delivery incomplete",
      phases: [
        { key: "assistant_update", status: "missing" },
        { key: "final_conclusion", status: "unproven" },
        { key: "close", status: "missing" },
      ],
      status: "review",
    });
  });

  it("maps delivery coverage phase status to stable tag colors", () => {
    expect(diagnosisNotificationDeliveryCoveragePhaseColor("delivered")).toBe(
      "success",
    );
    expect(diagnosisNotificationDeliveryCoveragePhaseColor("failed")).toBe(
      "error",
    );
    expect(diagnosisNotificationDeliveryCoveragePhaseColor("pending")).toBe(
      "warning",
    );
    expect(diagnosisNotificationDeliveryCoveragePhaseColor("unproven")).toBe(
      "warning",
    );
    expect(diagnosisNotificationDeliveryCoveragePhaseColor("missing")).toBe(
      "default",
    );
  });

  it("summarizes missing proof for AI notifications", () => {
    expect(
      diagnosisNotificationContentProofSummary([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({ content_sha256: undefined }),
      ]),
    ).toEqual({
      color: "warning",
      detail: "1 of 2 AI notification(s) are missing AI output digest proof.",
      label: "AI proof missing",
      missingCount: 1,
      provenCount: 1,
      totalAICount: 2,
    });
  });

  it("summarizes close notifications without final-conclusion proof as missing", () => {
    expect(
      diagnosisNotificationContentProofSummary([
        notification({ event_kind: "diagnosis_room.close_notification_sent" }),
      ]),
    ).toEqual({
      color: "warning",
      detail: "1 of 1 AI notification(s) are missing AI output digest proof.",
      label: "AI proof missing",
      missingCount: 1,
      provenCount: 0,
      totalAICount: 1,
    });
  });

  it("summarizes timelines without AI content notifications as not required", () => {
    expect(
      diagnosisNotificationContentProofSummary([
        notification({
          event_kind: "diagnosis_room.operator_note_notification_sent",
        }),
      ]),
    ).toEqual({
      color: "default",
      detail:
        "No assistant, final-ready, or close AI notification has been retained in this room timeline.",
      label: "AI proof not required",
      missingCount: 0,
      provenCount: 0,
      totalAICount: 0,
    });
  });

  it("exposes a stable timeline review anchor when AI proof is missing", () => {
    expect(diagnosisNotificationTimelineAnchorID).toBe(
      "diagnosis-notification-timeline",
    );
    expect(diagnosisNotificationTimelineHref).toBe(
      "#diagnosis-notification-timeline",
    );
    expect(
      diagnosisNotificationTimelineReviewRequired([
        notification({
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
      ]),
    ).toBe(true);
    expect(
      diagnosisNotificationTimelineReviewRequired([
        notification({
          content_kind: "final_conclusion",
          content_sha256: "c".repeat(64),
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
      ]),
    ).toBe(false);
    expect(
      diagnosisNotificationTimelineReviewRequired([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "b".repeat(64),
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
        notification({
          content_kind: "final_conclusion",
          content_sha256: "c".repeat(64),
          event_kind: "diagnosis_room.close_notification_sent",
        }),
      ]),
    ).toBe(false);
  });

  it("expects delivery proof only after operator-confirmed closed rooms", () => {
    expect(
      diagnosisNotificationDeliveryProofExpected(
        deliveryProofRoom({
          latest_conclusion: {
            confirmed_by: "operator-1",
          } as NonNullable<DiagnosisRoomSummary["latest_conclusion"]>,
          room_status: "closed",
        }),
      ),
    ).toBe(true);
    expect(
      diagnosisNotificationDeliveryProofExpected(
        deliveryProofRoom({
          latest_conclusion: {} as NonNullable<
            DiagnosisRoomSummary["latest_conclusion"]
          >,
          room_status: "closed",
        }),
      ),
    ).toBe(false);
    expect(
      diagnosisNotificationDeliveryProofExpected(
        deliveryProofRoom({
          latest_conclusion: {
            confirmed_by: "operator-1",
          } as NonNullable<DiagnosisRoomSummary["latest_conclusion"]>,
          room_status: "open",
        }),
      ),
    ).toBe(false);
  });

  it("requires a timeline review action for missing proof and incomplete coverage", () => {
    expect(
      diagnosisNotificationTimelineReviewActionRequired(
        deliveryProofRoom({
          notification_timeline: [
            notification({
              event_kind: "diagnosis_room.final_ready_notification_sent",
            }),
          ],
        }),
        null,
      ),
    ).toBe(true);
    expect(
      diagnosisNotificationTimelineReviewActionRequired(
        deliveryProofRoom({
          notification_timeline: [
            notification({
              content_kind: "assistant_message",
              content_sha256: "a".repeat(64),
            }),
          ],
        }),
        null,
      ),
    ).toBe(true);
    expect(
      diagnosisNotificationTimelineReviewActionRequired(
        deliveryProofRoom({
          latest_conclusion: {
            confirmed_by: "operator-1",
          } as NonNullable<DiagnosisRoomSummary["latest_conclusion"]>,
          notification_timeline: [],
          room_status: "closed",
        }),
        null,
      ),
    ).toBe(true);
  });

  it("does not show the timeline review action for failed notifications or complete proof", () => {
    const failed = notification({
      event_kind: "diagnosis_room.final_ready_notification_sent",
      provider_status: "failed",
    });
    expect(
      diagnosisNotificationTimelineReviewActionRequired(
        deliveryProofRoom({
          notification_timeline: [failed],
        }),
        failed,
      ),
    ).toBe(false);
    expect(
      diagnosisNotificationTimelineReviewActionRequired(
        deliveryProofRoom({
          notification_timeline: [
            notification({
              content_kind: "assistant_message",
              content_sha256: "a".repeat(64),
            }),
            notification({
              content_kind: "final_conclusion",
              content_sha256: "b".repeat(64),
              event_kind: "diagnosis_room.final_ready_notification_sent",
            }),
            notification({
              content_kind: "final_conclusion",
              content_sha256: "c".repeat(64),
              event_kind: "diagnosis_room.close_notification_sent",
            }),
          ],
        }),
        null,
      ),
    ).toBe(false);
  });

  it("requires retry only for AI notifications missing output proof", () => {
    expect(
      diagnosisNotificationContentProofRetryRequired(
        notification({
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
      ),
    ).toBe(true);
    expect(
      diagnosisNotificationContentProofRetryRequired(
        notification({
          content_kind: "final_conclusion",
          content_sha256: "c".repeat(64),
          event_kind: "diagnosis_room.final_ready_notification_sent",
        }),
      ),
    ).toBe(false);
    expect(
      diagnosisNotificationContentProofRetryRequired(
        notification({
          event_kind: "diagnosis_room.close_notification_sent",
        }),
      ),
    ).toBe(true);
  });

  it("finds the latest AI notification that can be retried for proof", () => {
    const assistantMissingProof = notification({
      event_kind: "diagnosis_room.assistant_turn_notification_sent",
      occurred_at: "2026-06-21T03:30:00Z",
    });
    const operatorNotification = notification({
      event_kind: "diagnosis_room.operator_note_notification_sent",
      occurred_at: "2026-06-21T03:31:00Z",
    });
    const closeMissingProof = notification({
      event_kind: "diagnosis_room.close_notification_sent",
      occurred_at: "2026-06-21T03:32:00Z",
    });

    expect(
      diagnosisNotificationContentProofRetryEntry([
        assistantMissingProof,
        operatorNotification,
        closeMissingProof,
      ]),
    ).toBe(closeMissingProof);
  });

  it("does not select delivered AI notifications that already have proof", () => {
    expect(
      diagnosisNotificationContentProofRetryEntry([
        notification({
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
        }),
        notification({
          event_kind: "diagnosis_room.operator_note_notification_sent",
        }),
      ]),
    ).toBeNull();
  });
});

function notification(
  overrides: Partial<DiagnosisRoomNotificationTimelineEntry> = {},
): DiagnosisRoomNotificationTimelineEntry {
  return {
    event_kind: "diagnosis_room.assistant_turn_notification_sent",
    occurred_at: "2026-06-21T03:30:00Z",
    provider_status: "delivered",
    ...overrides,
  };
}

function deliveryProofRoom(
  overrides: Partial<
    Pick<
      DiagnosisRoomSummary,
      "latest_conclusion" | "notification_timeline" | "room_status"
    >
  > = {},
): Pick<
  DiagnosisRoomSummary,
  "latest_conclusion" | "notification_timeline" | "room_status"
> {
  return {
    notification_timeline: [],
    room_status: "open",
    ...overrides,
  };
}
