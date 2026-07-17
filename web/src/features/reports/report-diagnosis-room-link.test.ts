import { describe, expect, it } from "vitest";

import type { DiagnosisRoomSummary } from "@/features/diagnosis-room/api";

import {
  reportDiagnosisRoomHref,
  reportDiagnosisRoomSessionQuery,
} from "./report-diagnosis-room-link";
import type { FinalReportDetail } from "./types";

describe("report diagnosis room link", () => {
  it("uses the session carried by a stored diagnosis conclusion", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_conclusion: {
          chat_session_id: 401,
          confidence: "high",
          content: "Checkout latency is ready for operator review.",
          diagnosis_task_id: 301,
          event_kind: "diagnosis_room.final_conclusion_ready",
          evidence_snapshot_id: 9001,
          recorded_at: "2026-06-18T08:10:00Z",
          requires_human_review: true,
          session_id: "diagnosis-session-conclusion",
          source: "latest_assistant_turn",
          status: "available",
        },
      }),
    ]);

    expect(reportDiagnosisRoomHref(report, report.linked_sub_reports[0]!)).toEqual({
      pathname: "/diagnosis-room",
      query: {
        evidence_snapshot_id: "9001",
        intent: "review_conclusion",
        report_id: "101",
        session_id: "diagnosis-session-conclusion",
        sub_report_id: "501",
      },
    });
  });

  it("falls back to the latest open room for the same evidence snapshot", () => {
    const report = reportDetail([linkedSubReport()]);
    const subReport = report.linked_sub_reports[0]!;

    expect(
      reportDiagnosisRoomSessionQuery(subReport, [
        diagnosisRoom({
          last_activity_at: "2026-06-18T08:20:00Z",
          room_status: "closed",
          session_id: "diagnosis-session-closed-newer",
        }),
        diagnosisRoom({
          last_activity_at: "2026-06-18T08:05:00Z",
          room_status: "open",
          session_id: "diagnosis-session-open-older",
        }),
        diagnosisRoom({
          evidence_snapshot_id: 9999,
          last_activity_at: "2026-06-18T08:30:00Z",
          room_status: "open",
          session_id: "diagnosis-session-other-evidence",
        }),
      ]),
    ).toEqual({
      session_id: "diagnosis-session-open-older",
    });
    expect(
      reportDiagnosisRoomHref(report, subReport, [
        diagnosisRoom({
          last_activity_at: "2026-06-18T08:05:00Z",
          room_status: "open",
          session_id: "diagnosis-session-open-older",
        }),
      ]).query.session_id,
    ).toBe("diagnosis-session-open-older");
  });

  it("prefers an embedded exact room over recent room fallback", () => {
    const report = reportDetail([
      linkedSubReport({
        diagnosis_room: diagnosisRoom({
          session_id: "diagnosis-session-embedded",
        }),
      }),
    ]);

    expect(
      reportDiagnosisRoomSessionQuery(report.linked_sub_reports[0]!, [
        diagnosisRoom({
          last_activity_at: "2026-06-18T08:30:00Z",
          session_id: "diagnosis-session-recent",
        }),
      ]),
    ).toEqual({
      session_id: "diagnosis-session-embedded",
    });
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
