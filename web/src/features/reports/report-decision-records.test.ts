import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";

import { reportDiagnosisDecisionRecords } from "./report-decision-records";
import type { FinalReportDetail } from "./types";

describe("report diagnosis decision records", () => {
  it("links a stored conclusion to exact closed room and notification proof", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            confirmed_by: "owner-1",
            content: "Checkout latency is confirmed after operator evidence review.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: false,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:2",
          },
        }),
      ]),
      [
        diagnosisRoom({
          close_reason: "human_confirmed",
          closed_at: "2026-06-18T08:11:00Z",
          notification_timeline: [
            {
              event_kind: "diagnosis_room.close_notification_sent",
              notification_channel_profile_id: 2,
              occurred_at: "2026-06-18T08:11:01Z",
              provider_message_id: "wecom-close-1",
              provider_status: "delivered",
            },
          ],
          room_status: "closed",
          session_id: "diagnosis-session-301",
          task_status: "succeeded",
        }),
      ],
    );

    expect(records).toEqual([
      expect.objectContaining({
        confirmedBy: "owner-1",
        notificationChannelProfileID: 2,
        notificationEventKind: "diagnosis_room.close_notification_sent",
        notificationFailed: false,
        notificationOccurredAt: "2026-06-18T08:11:01Z",
        notificationProviderMessageID: "wecom-close-1",
        notificationProviderStatus: "delivered",
        recordedAt: "2026-06-18T08:10:00Z",
        requiresHumanReview: false,
        roomClosedAt: "2026-06-18T08:11:00Z",
        roomCloseReason: "human_confirmed",
        roomLinked: true,
        roomStatus: "closed",
        roomTurnCount: 2,
        sessionID: "diagnosis-session-301",
        status: "confirmed",
        version: "diagnosis-session-301:2",
      }),
    ]);
  });

  it("keeps closed unconfirmed rooms separate from operator-confirmed conclusions", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            confirmed_by: "",
            content: "Checkout latency has enough evidence for a stored conclusion.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: true,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:2",
          },
        }),
      ]),
      [
        diagnosisRoom({
          close_reason: "operator_cancelled",
          closed_at: "2026-06-18T08:11:00Z",
          room_status: "closed",
          session_id: "diagnosis-session-301",
        }),
      ],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        roomCloseReason: "operator_cancelled",
        roomTurnCount: 2,
        status: "room_closed",
      }),
    );
  });

  it("keeps unresolved evidence separate from unavailable delivery proof", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: undefined,
          diagnosis_progress: {
            confidence: "medium",
            confidence_rationale: "Runtime context is still missing.",
            conclusion_status: "needs_evidence",
            diagnosis_task_id: 302,
            event_kind: "diagnosis_room.turn_persisted",
            evidence_request_count: 1,
            evidence_snapshot_id: 9003,
            missing_evidence_requests: [
              {
                detail: "Attach restart and deployment context.",
                label: "Runtime context",
                priority: "high",
              },
            ],
            occurred_at: "2026-06-18T08:12:00Z",
            recorded_at: "2026-06-18T08:12:01Z",
            requires_human_review: true,
            session_id: "diagnosis-session-302",
            status: "in_progress",
          },
          evidence_snapshot_id: 9003,
          id: 502,
          title: "Checkout JVM memory",
        }),
      ]),
      [],
    );

    expect(records).toEqual([
      expect.objectContaining({
        notificationChannelProfileID: null,
        notificationEventKind: "",
        notificationFailed: false,
        notificationProviderStatus: "",
        readiness: expect.objectContaining({ status: "needs_evidence" }),
        requiresHumanReview: true,
        roomLinked: false,
        roomStatus: "",
        sessionID: "diagnosis-session-302",
        status: "needs_evidence",
      }),
    ]);
  });

  it("uses recent room fallback by evidence snapshot when no report session is stored", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: undefined,
          diagnosis_progress: undefined,
          evidence_snapshot_id: 9003,
          id: 502,
          title: "Checkout JVM memory",
        }),
      ]),
      [
        diagnosisRoom({
          evidence_snapshot_id: 9003,
          notification_timeline: [
            {
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              notification_channel_profile_id: 2,
              occurred_at: "2026-06-18T08:12:01Z",
              provider_message_id: "wecom-assistant-9003",
              provider_status: "delivered",
            },
          ],
          session_id: "diagnosis-session-9003",
        }),
      ],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        notificationEventKind: "diagnosis_room.assistant_turn_notification_sent",
        notificationProviderMessageID: "wecom-assistant-9003",
        roomLinked: true,
        roomStatus: "open",
        sessionID: "diagnosis-session-9003",
        status: "pending_diagnosis",
      }),
    );
  });

  it("does not cross-link another room when the report stores a different session", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_progress: {
            confidence: "medium",
            confidence_rationale: "Runtime context is still missing.",
            conclusion_status: "needs_evidence",
            diagnosis_task_id: 302,
            event_kind: "diagnosis_room.turn_persisted",
            evidence_request_count: 1,
            evidence_snapshot_id: 9003,
            occurred_at: "2026-06-18T08:12:00Z",
            recorded_at: "2026-06-18T08:12:01Z",
            requires_human_review: true,
            session_id: "diagnosis-session-exact-missing",
            status: "in_progress",
          },
          evidence_snapshot_id: 9003,
          id: 502,
          title: "Checkout JVM memory",
        }),
      ]),
      [
        diagnosisRoom({
          evidence_snapshot_id: 9003,
          session_id: "diagnosis-session-other",
        }),
      ],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        notificationEventKind: "",
        roomLinked: false,
        roomStatus: "",
        sessionID: "diagnosis-session-exact-missing",
      }),
    );
  });

  it("uses newer progress instead of stale stored conclusions", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            content: "The initial AI conclusion looked complete.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: false,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:1",
          },
          diagnosis_progress: {
            confidence: "medium",
            confidence_rationale:
              "Supplemental evidence revealed a remaining DBA confirmation gap.",
            conclusion_status: "needs_evidence",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.turn_persisted",
            evidence_request_count: 0,
            evidence_snapshot_id: 9001,
            missing_evidence_requests: [
              {
                detail: "Attach DBA confirmation for the capacity action.",
                label: "DBA confirmation",
                priority: "high",
              },
            ],
            occurred_at: "2026-06-18T08:12:00Z",
            recorded_at: "2026-06-18T08:12:01Z",
            requires_human_review: true,
            session_id: "diagnosis-session-301",
            status: "in_progress",
          },
        }),
      ]),
      [],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        recordedAt: "2026-06-18T08:12:01Z",
        readiness: expect.objectContaining({ status: "needs_evidence" }),
        requiresHumanReview: true,
        status: "needs_evidence",
        version: "",
      }),
    );
  });

  it("preserves stored conclusion review requirements for action rendering", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            confirmed_by: "",
            content: "Checkout latency has enough evidence for a stored conclusion.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: false,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:2",
          },
        }),
      ]),
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        requiresHumanReview: false,
        status: "recorded",
      }),
    );
  });

  it("prefers final-ready delivery proof for stored conclusions", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            confirmed_by: "",
            content: "Checkout latency has enough evidence for a stored conclusion.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: true,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:2",
          },
        }),
      ]),
      [
        diagnosisRoom({
          notification_timeline: [
            {
              event_kind: "diagnosis_room.final_ready_notification_sent",
              notification_channel_profile_id: 2,
              occurred_at: "2026-06-18T08:10:01Z",
              provider_message_id: "wecom-final-ready-1",
              provider_status: "delivered",
            },
            {
              event_kind: "diagnosis_room.assistant_turn_notification_sent",
              notification_channel_profile_id: 2,
              occurred_at: "2026-06-18T08:10:30Z",
              provider_message_id: "wecom-later-assistant-1",
              provider_status: "delivered",
            },
          ],
          session_id: "diagnosis-session-301",
        }),
      ],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        notificationEventKind: "diagnosis_room.final_ready_notification_sent",
        notificationProviderMessageID: "wecom-final-ready-1",
        status: "recorded",
      }),
    );
  });

  it("uses exact diagnosis room proof embedded in report detail before recent room fallback", () => {
    const records = reportDiagnosisDecisionRecords(
      reportDetail([
        linkedSubReport({
          diagnosis_conclusion: {
            chat_session_id: 401,
            confidence: "high",
            confirmed_by: "",
            content: "Checkout latency has enough evidence for a stored conclusion.",
            diagnosis_task_id: 301,
            event_kind: "diagnosis_room.final_conclusion_ready",
            evidence_snapshot_id: 9001,
            recorded_at: "2026-06-18T08:10:00Z",
            requires_human_review: true,
            session_id: "diagnosis-session-301",
            source: "latest_assistant_turn",
            status: "available",
            conclusion_version: "diagnosis-session-301:2",
          },
          diagnosis_room: diagnosisRoom({
            notification_timeline: [
              {
                event_kind: "diagnosis_room.final_ready_notification_sent",
                notification_channel_profile_id: 2,
                occurred_at: "2026-06-18T08:10:01Z",
                provider_message_id: "wecom-embedded-final-ready-1",
                provider_status: "delivered",
              },
            ],
            session_id: "diagnosis-session-301",
          }),
        }),
      ]),
      [],
    );

    expect(records[0]).toEqual(
      expect.objectContaining({
        notificationEventKind: "diagnosis_room.final_ready_notification_sent",
        notificationProviderMessageID: "wecom-embedded-final-ready-1",
        roomLinked: true,
        roomStatus: "open",
        sessionID: "diagnosis-session-301",
      }),
    );
  });
});

function reportDetail(linkedSubReports: FinalReportDetail["linked_sub_reports"]): FinalReportDetail {
  return {
    confidence: "high",
    expected_sub_report_count: 1,
    failed_sub_report_count: 0,
    generation_status: "complete",
    content: {
      title: "Checkout latency incident",
    },
    correlation_key: "window:checkout-latency",
    created_at: "2026-06-18T08:00:00Z",
    created_by_workflow: "FinalReportWorkflow",
    executive_summary: "Checkout latency increased after deployment.",
    final_notification_readiness: {
      detail: "Checkout API latency has no operator-confirmed AI conclusion yet.",
      notification_purpose: "handoff",
      ready: false,
      status: "blocked",
      status_label: "Final notification blocked",
    },
    id: 101,
    linked_sub_reports: linkedSubReports,
    model: "example-llm-model",
    notification_deliveries: [],
    notification_text: "Checkout latency incident requires review.",
    output_mode: "json_schema",
    recommended_actions: [],
    severity: "warning",
    successful_sub_report_count: 1,
    sub_reports: linkedSubReports.map((subReport) => ({
      severity: subReport.severity,
      summary: subReport.summary,
      title: subReport.title,
    })),
    title: "Checkout latency incident",
  };
}

function linkedSubReport(
  overrides: Partial<FinalReportDetail["linked_sub_reports"][number]> = {},
): FinalReportDetail["linked_sub_reports"][number] {
  return {
    confidence: "high",
    content: {
      title: "Checkout API latency",
    },
    created_at: "2026-06-18T07:59:00Z",
    created_by_workflow: "ReportFanOutWorkflow",
    evidence_refs: ["alert:checkout-latency"],
    evidence_snapshot_id: 9001,
    findings: [],
    id: 501,
    model: "example-llm-model",
    output_mode: "json_schema",
    recommended_actions: [],
    scenario: "single_alert",
    severity: "warning",
    summary: "p95 latency exceeded the warning threshold.",
    title: "Checkout API latency",
    ...overrides,
  };
}

function diagnosisRoom(overrides: Partial<DiagnosisRoomSummary> = {}): DiagnosisRoomSummary {
  return {
    approval_mode: "single",
    chat_session_id: 401,
    close_reason: "",
    closed_at: null,
    created_at: "2026-06-18T08:09:00Z",
    diagnosis_task_id: 301,
    evidence_snapshot_id: 9001,
    last_activity_at: "2026-06-18T08:11:00Z",
    notification_timeline: [],
    room_status: "open",
    run_id: "run-301",
    session_id: "diagnosis-session-301",
    started_at: "2026-06-18T08:09:00Z",
    task_status: "running",
    turn_count: 2,
    updated_at: "2026-06-18T08:11:00Z",
    workflow_id: "diagnosis-room-diagnosis-session-301",
    ...overrides,
  };
}
