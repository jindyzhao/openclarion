import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";

import {
  alertDiagnosisClosureSummary,
  alertDiagnosisEvidenceActions,
  alertDiagnosisEvidenceProgressSummary,
  alertDiagnosisRoomPrimaryAction,
  alertDiagnosisDeliveryReviewAction,
  diagnosisRoomNotificationChannelReviewHref,
  latestFailedDiagnosisRoomNotification,
  latestDiagnosisRoomNotification,
} from "./diagnosis-delivery";

describe("alert diagnosis delivery helpers", () => {
  it("returns the latest diagnosis-room notification", () => {
    expect(
      latestDiagnosisRoomNotification(
        room({
          notification_timeline: [
            notification({
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              occurred_at: "2026-06-21T01:00:00Z",
            }),
            notification({
              event_kind: "diagnosis_room.final_ready_notification_sent",
              occurred_at: "2026-06-21T01:01:00Z",
            }),
          ],
        }),
      )?.event_kind,
    ).toBe("diagnosis_room.final_ready_notification_sent");
  });

  it("keeps earlier failed notifications visible after later deliveries", () => {
    expect(
      latestFailedDiagnosisRoomNotification(
        room({
          notification_timeline: [
            notification({
              event_kind: "diagnosis_room.final_ready_notification_sent",
              provider_status: "failed",
            }),
            notification({
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              provider_status: "delivered",
            }),
          ],
        }),
      )?.event_kind,
    ).toBe("diagnosis_room.final_ready_notification_sent");
  });

  it("links incomplete AI delivery coverage back to the room notification timeline", () => {
    expect(
      alertDiagnosisDeliveryReviewAction(
        room({
          notification_timeline: [
            notification({
              content_kind: "assistant_message",
              content_sha256: "a".repeat(64),
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              provider_status: "delivered",
            }),
          ],
        }),
      ),
    ).toMatchObject({
      coverage: {
        readyCount: 1,
        requiredCount: 3,
        status: "review",
      },
      danger: false,
      href: "/diagnosis-room?evidence_snapshot_id=7&session_id=diagnosis-session-1#diagnosis-notification-timeline",
      kind: "proof",
    });
  });

  it("marks failed delivery coverage review actions as dangerous", () => {
    expect(
      alertDiagnosisDeliveryReviewAction(
        room({
          notification_timeline: [
            notification({
              event_kind: "diagnosis_room.final_ready_notification_sent",
              provider_status: "failed",
            }),
          ],
        }),
      ),
    ).toMatchObject({
      danger: true,
      kind: "failure",
    });
  });

  it("does not ask for notification review when AI delivery coverage is complete", () => {
    expect(
      alertDiagnosisDeliveryReviewAction(
        room({
          notification_timeline: [
            notification({
              content_kind: "assistant_message",
              content_sha256: "a".repeat(64),
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
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
      ),
    ).toBeNull();
  });

  it("builds notification channel review links without exposing provider details", () => {
    expect(
      diagnosisRoomNotificationChannelReviewHref(
        notification({ notification_channel_profile_id: 9 }),
      ),
    ).toBe("/settings/notification-channels?channel_id=9");
    expect(diagnosisRoomNotificationChannelReviewHref(notification())).toBe(
      "/settings/notification-channels",
    );
  });

  it("prioritizes failed notification review as the primary alert action", () => {
    expect(
      alertDiagnosisRoomPrimaryAction([
        room({
          latest_progress: progress({
            conclusion_status: "needs_evidence",
            evidence_request_count: 2,
          }),
          notification_timeline: [
            notification({
              notification_channel_profile_id: 9,
              provider_status: "failed",
            }),
          ],
        }),
      ]),
    ).toEqual({
      danger: true,
      href: "/settings/notification-channels?channel_id=9",
      iconKind: "attention",
      kind: "review_channel",
    });
  });

  it("uses AI evidence requests as the primary alert action before final review", () => {
    expect(
      alertDiagnosisRoomPrimaryAction([
        room({
          latest_progress: progress({
            conclusion_status: "needs_evidence",
            evidence_request_count: 2,
            missing_evidence_requests: [
              {
                detail: "Attach current DBA remediation status.",
                label: "DBA status",
                priority: "high",
              },
            ],
          }),
          turn_count: 1,
        }),
      ]),
    ).toMatchObject({
      danger: false,
      href:
        "/diagnosis-room?evidence_snapshot_id=7&intent=confidence_review&session_id=diagnosis-session-1&follow_up_detail=Attach+current+DBA+remediation+status.&follow_up_label=DBA+status&follow_up_priority=high",
      iconKind: "attention",
      kind: "room_step",
      step: {
        code: "collect_evidence",
        detail: "AI requested evidence: 2 planned, 1 missing.",
      },
    });
  });

  it("links the primary alert action to the first executable evidence request", () => {
    const action = alertDiagnosisRoomPrimaryAction([
      room({
        latest_progress: progress({
          conclusion_status: "needs_evidence",
          evidence_request_count: 1,
          evidence_requests: [
            {
              alert_source_profile_id: 3,
              limit: 5,
              reason: "Collect the current sibling alerts.",
              tool: "active_alerts",
            },
          ],
        }),
      }),
    ]);

    expect(action).toMatchObject({
      kind: "room_step",
      step: {
        code: "collect_evidence",
        detail: "AI requested evidence: 1 planned.",
      },
    });
    const params = actionParams(action);
    expect(params.get("intent")).toBe("confidence_review");
    expect(params.get("session_id")).toBe("diagnosis-session-1");
    expect(params.get("evidence_tool")).toBe("active_alerts");
    expect(params.get("evidence_reason")).toBe(
      "Collect the current sibling alerts.",
    );
    expect(params.get("evidence_source_profile_id")).toBe("3");
    expect(params.get("evidence_limit")).toBe("5");
    expect(params.get("follow_up_label")).toBeNull();
  });

  it("links the primary alert action to the first supplemental follow-up when no executable request is available", () => {
    const action = alertDiagnosisRoomPrimaryAction([
      room({
        latest_progress: progress({
          conclusion_status: "needs_evidence",
          evidence_request_count: 0,
          missing_evidence_requests: [
            {
              detail: "Attach the current DBA remediation status.",
              label: "DBA status",
              priority: "high",
            },
          ],
        }),
      }),
    ]);

    expect(action).toMatchObject({
      kind: "room_step",
      step: {
        code: "collect_evidence",
        detail: "AI requested evidence: 1 missing.",
      },
    });
    const params = actionParams(action);
    expect(params.get("intent")).toBe("confidence_review");
    expect(params.get("follow_up_label")).toBe("DBA status");
    expect(params.get("follow_up_detail")).toBe(
      "Attach the current DBA remediation status.",
    );
    expect(params.get("follow_up_priority")).toBe("high");
    expect(params.get("evidence_tool")).toBeNull();
  });

  it("routes supplemental evidence reassessment by semantic code", () => {
    const action = alertDiagnosisRoomPrimaryAction([
      room({
        latest_progress: progress({
          assistant_sequence: 1,
          supplemental_evidence: [
            {
              detail: "DBA team expanded tablespace.",
              evidence: "Tablespace expansion completed.",
              label: "DBA status",
              priority: "high",
              provided_at: "2026-06-21T01:02:00Z",
            },
          ],
        }),
      }),
    ]);

    expect(action).toMatchObject({
      kind: "room_step",
      step: { code: "reassess_evidence" },
    });
    expect(actionParams(action).get("intent")).toBe("confidence_review");
  });

  it("lists executable and operator evidence actions from AI progress", () => {
    const actions = alertDiagnosisEvidenceActions(
      room({
        latest_progress: progress({
          evidence_requests: [
            {
              alert_source_profile_id: 3,
              limit: 5,
              reason: "Collect current sibling alerts.",
              tool: "active_alerts",
            },
          ],
          missing_evidence_requests: [
            {
              detail: "Attach current DBA remediation status.",
              label: "DBA status",
              priority: "high",
            },
          ],
        }),
      }),
    );

    expect(actions).toHaveLength(2);
    expect(actions[0]).toMatchObject({
      action: "collect",
      detail: "Collect current sibling alerts.",
      kind: "executable",
      label: "active_alerts",
    });
    expect(actions[1]).toMatchObject({
      action: "provide",
      detail: "Attach current DBA remediation status.",
      kind: "operator",
      label: "DBA status",
      priority: "high",
    });
    const executableParams = new URL(
      actions[0]?.href ?? "",
      "https://openclarion.local",
    ).searchParams;
    expect(executableParams.get("evidence_tool")).toBe("active_alerts");
    expect(executableParams.get("intent")).toBe("confidence_review");
    const operatorParams = new URL(
      actions[1]?.href ?? "",
      "https://openclarion.local",
    ).searchParams;
    expect(operatorParams.get("follow_up_label")).toBe("DBA status");
    expect(operatorParams.get("follow_up_priority")).toBe("high");
  });

  it("uses conclusion evidence requests before older progress requests", () => {
    const actions = alertDiagnosisEvidenceActions(
      room({
        latest_progress: progress({
          evidence_requests: [
            {
              reason: "Collect stale active alerts.",
              tool: "active_alerts",
            },
          ],
        }),
        latest_conclusion: conclusion({
          evidence_collection_suggestions: [
            {
              detail: "Attach the approved capacity change.",
              label: "Capacity change",
              priority: "medium",
            },
          ],
        }),
      }),
    );

    expect(actions).toHaveLength(1);
    expect(actions[0]).toMatchObject({
      action: "review",
      detail: "Attach the approved capacity change.",
      kind: "operator",
      label: "Capacity change",
    });
    expect(actions[0]?.href).not.toContain("active_alerts");
  });

  it("summarizes confidence movement and retained evidence from AI progress", () => {
    expect(
      alertDiagnosisEvidenceProgressSummary(
        room({
          latest_progress: progress({
            confidence: "medium",
            confidence_timeline: [
              {
                confidence: "low",
                confidence_rationale: "Initial alert evidence only.",
                event_kind: "diagnosis_room.turn_persisted",
                evidence_collection_results: [
                  {
                    collected_at: "2026-06-21T01:00:30Z",
                    message: "Collected active alerts.",
                    observed_alerts: 2,
                    status: "collected",
                    tool: "active_alerts",
                  },
                ],
                evidence_request_count: 1,
                occurred_at: "2026-06-21T01:00:00Z",
                requires_human_review: true,
              },
              {
                confidence: "medium",
                confidence_rationale: "Sibling alerts support the hypothesis.",
                event_kind: "diagnosis_room.turn_persisted",
                evidence_request_count: 0,
                missing_evidence_requests: [
                  {
                    detail: "Attach DBA remediation status.",
                    label: "DBA status",
                    priority: "high",
                  },
                ],
                occurred_at: "2026-06-21T01:01:00Z",
                requires_human_review: true,
              },
            ],
            missing_evidence_requests: [
              {
                detail: "Attach DBA remediation status.",
                label: "DBA status",
                priority: "high",
              },
            ],
            supplemental_evidence: [
              {
                detail: "DBA team expanded tablespace.",
                evidence: "Tablespace expansion completed.",
                label: "DBA status",
                priority: "high",
                provided_at: "2026-06-21T01:02:00Z",
              },
            ],
          }),
        }),
      ),
    ).toEqual({
      collectedEvidence: 1,
      evidenceState: "needed",
      initialConfidence: "low",
      latestConfidence: "medium",
      openEvidence: 1,
      supplementalEvidence: 1,
      timelineEntries: 2,
    });
  });

  it("summarizes retained conclusion evidence without open requests", () => {
    expect(
      alertDiagnosisEvidenceProgressSummary(
        room({
          latest_conclusion: conclusion({
            confidence: "high",
            confidence_timeline: [
              {
                confidence: "high",
                event_kind: "diagnosis_room.final_conclusion_ready",
                evidence_collection_results: [
                  {
                    collected_at: "2026-06-21T01:02:00Z",
                    observed_alerts: 1,
                    status: "collected",
                    tool: "active_alerts",
                  },
                ],
                evidence_request_count: 0,
                occurred_at: "2026-06-21T01:02:00Z",
                requires_human_review: false,
              },
            ],
          }),
        }),
      ),
    ).toMatchObject({
      collectedEvidence: 1,
      evidenceState: "retained",
      latestConfidence: "high",
      openEvidence: 0,
    });
  });

  it("summarizes confirmed closure and retained close notification proof", () => {
    expect(
      alertDiagnosisClosureSummary(
        room({
          close_reason: "operator confirmed final conclusion",
          latest_conclusion: conclusion({
            confirmed_by: "owner-1",
            confidence: "high",
            requires_human_review: false,
          }),
          notification_timeline: [
            notification({
              content_kind: "assistant_message",
              content_sha256: "a".repeat(64),
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              provider_status: "delivered",
            }),
            notification({
              content_kind: "final_conclusion",
              content_sha256: "b".repeat(64),
              event_kind: "diagnosis_room.final_ready_notification_sent",
              provider_status: "delivered",
            }),
            notification({
              content_kind: "final_conclusion",
              content_sha256: "c".repeat(64),
              event_kind: "diagnosis_room.close_notification_sent",
              provider_status: "delivered",
            }),
          ],
          room_status: "closed",
        }),
      ),
    ).toMatchObject({
      closeProofStatus: "delivered",
      confirmedBy: "owner-1",
      deliveryCoverage: { status: "ready" },
      roomClosed: true,
      traceability: {
        label: "Closure traceability complete",
        status: "complete",
      },
    });
  });

  it("summarizes stored conclusions that still need operator confirmation", () => {
    expect(
      alertDiagnosisClosureSummary(
        room({
          latest_conclusion: conclusion({
            confirmed_by: "",
            confidence: "medium",
            requires_human_review: true,
          }),
        }),
      ),
    ).toMatchObject({
      closeProofStatus: "missing",
      confirmedBy: "",
      deliveryCoverage: { status: "pending" },
      roomClosed: false,
      traceability: {
        label: "Closure traceability needs review",
        status: "review",
      },
    });
  });

  it("routes lower-confidence conclusions to confidence improvement", () => {
    expect(
      alertDiagnosisRoomPrimaryAction([
        room({
          latest_conclusion: conclusion({
            confidence: "medium",
            requires_human_review: false,
          }),
        }),
      ]),
    ).toMatchObject({
      danger: false,
      href:
        "/diagnosis-room?evidence_snapshot_id=7&intent=confidence_review&session_id=diagnosis-session-1",
      iconKind: "attention",
      kind: "room_step",
      step: {
        code: "improve_confidence",
        detail: "Collect more evidence before final confirmation.",
      },
    });
  });

  it("routes high-confidence conclusions to conclusion review", () => {
    expect(
      alertDiagnosisRoomPrimaryAction([
        room({
          latest_conclusion: conclusion({
            confidence: "high",
            requires_human_review: false,
          }),
        }),
      ]),
    ).toMatchObject({
      danger: false,
      href:
        "/diagnosis-room?evidence_snapshot_id=7&intent=review_conclusion&session_id=diagnosis-session-1",
      iconKind: "review",
      kind: "room_step",
      step: {
        code: "review_conclusion",
        detail: "AI produced a conclusion. Review it before closing the room.",
      },
    });
  });
});

function room(overrides: Partial<DiagnosisRoomSummary> = {}): DiagnosisRoomSummary {
  return {
    approval_mode: "single",
    chat_session_id: 202,
    close_reason: "",
    closed_at: null,
    created_at: "2026-06-21T01:00:00Z",
    diagnosis_task_id: 101,
    evidence_snapshot_id: 7,
    last_activity_at: "2026-06-21T01:00:00Z",
    latest_conclusion: undefined,
    latest_progress: undefined,
    notification_timeline: [],
    room_status: "open",
    run_id: "run-1",
    session_id: "diagnosis-session-1",
    started_at: "2026-06-21T01:00:00Z",
    task_status: "running",
    turn_count: 1,
    updated_at: "2026-06-21T01:00:00Z",
    workflow_id: "diagnosis-room-diagnosis-session-1",
    workflow_visibility: { status: "running" },
    ...overrides,
  };
}

function notification(
  overrides: Partial<
    NonNullable<DiagnosisRoomSummary["notification_timeline"]>[number]
  > = {},
): NonNullable<DiagnosisRoomSummary["notification_timeline"]>[number] {
  return {
    event_kind: "diagnosis_room.assistant_turn_notification_sent",
    occurred_at: "2026-06-21T01:00:00Z",
    provider_status: "delivered",
    ...overrides,
  };
}

function conclusion(
  overrides: Partial<NonNullable<DiagnosisRoomSummary["latest_conclusion"]>> = {},
): NonNullable<DiagnosisRoomSummary["latest_conclusion"]> {
  return {
    diagnosis_task_id: 101,
    session_id: "diagnosis-session-1",
    chat_session_id: 202,
    event_kind: "diagnosis_room.final_conclusion_ready",
    status: "available",
    evidence_snapshot_id: 7,
    source: "latest_assistant_turn",
    content: "Capacity pressure requires operator review.",
    confidence: "high",
    requires_human_review: false,
    recorded_at: "2026-06-21T01:02:01Z",
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
    occurred_at: "2026-06-21T01:01:00Z",
    recorded_at: "2026-06-21T01:01:01Z",
    ...overrides,
  };
}

function actionParams(
  action: ReturnType<typeof alertDiagnosisRoomPrimaryAction>,
): URLSearchParams {
  if (action === null) {
    throw new Error("expected primary action");
  }
  return new URL(action.href, "https://openclarion.local").searchParams;
}
