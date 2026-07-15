import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";

import type {
  ReportConsultationAuditItem,
  ReportDiagnosisHandoff,
  ReportDiagnosisReadiness,
  ReportFinalNotificationReadiness,
  SubReportDiagnosisReadiness,
} from "./diagnosis-readiness";
import type { ReportDecisionRecord } from "./report-decision-records";
import { reportDeliveryProofState } from "./report-delivery-proof";
import {
  localizeFinalNotificationReadiness,
  localizeReportConclusionReason,
  localizeReportConclusionSource,
  localizeReportConsultationAuditItem,
  localizeReportDecisionRecord,
  localizeReportDeliveryProofRetryLabel,
  localizeReportDeliveryProofState,
  localizeReportDiagnosisHandoff,
  localizeReportDiagnosisReviewReturnNotice,
  localizeReportEvidenceCollectionResultDetail,
  localizeReportEvidenceRequestDetail,
  localizeReportNotificationRetryChannelOption,
  localizeReportNotificationRetrySuccessMessage,
  localizeReportReadiness,
  localizeSubReportDiagnosisAction,
  localizeSubReportReadiness,
} from "./report-detail-copy";
import type { FinalReportDetail } from "./types";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "ReportDetail",
});
const tZhCN = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "ReportDetail",
});
const tStatusEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisRoom.status",
});
const tStatusZhCN = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});

describe("report detail presentation copy", () => {
  it("localizes report and subreport readiness from stable state", () => {
    const report = reportReadiness({
      blocked: true,
      currentCollectionSuggestions: 1,
      currentExecutableEvidenceRequests: 1,
      currentMissingEvidence: 2,
      pending: 4,
      status: "needs_evidence",
    });
    expect(localizeReportReadiness(report, "en", tEn)).toEqual({
      queueDetail:
        "Resolve 2 missing evidence items and 1 executable evidence task.",
      queueLabel: "Evidence needed",
      statusDetail:
        "AI is waiting for 2 missing evidence items and 1 executable evidence task. Residual guidance remains documented without blocking confirmation: 1 residual collection suggestion.",
      statusLabel: "Needs evidence",
    });
    expect(
      localizeReportReadiness(report, "zh-CN", tZhCN).statusDetail,
    ).toBe(
      "AI 正在等待 2 项缺失证据和1 个待执行证据任务。仍保留 1 项剩余采集建议，但其不阻塞确认。",
    );

    const failed = subReportReadiness({
      failureReason: "provider timeout: zone-a",
      latestConfidence: "failed",
      status: "failed",
    });
    expect(localizeSubReportReadiness(failed, "zh-CN", tZhCN)).toEqual({
      detail: "AI 诊断在生成最终结论前失败：provider timeout: zone-a",
      label: "失败",
    });
  });

  it("localizes action labels without parsing presentation prose", () => {
    expect(
      localizeSubReportDiagnosisAction(
        {
          hasConclusion: false,
          hasProgress: false,
          readiness: subReportReadiness({ status: "pending_diagnosis" }),
        },
        tZhCN,
      ),
    ).toBe("准备诊断");
    expect(
      localizeSubReportDiagnosisAction(
        {
          hasConclusion: true,
          hasProgress: false,
          readiness: subReportReadiness({ status: "complete" }),
        },
        tEn,
      ),
    ).toBe("Review confirmed diagnosis");
  });

  it("localizes handoff and audit timelines including ICU branches", () => {
    const readiness = reportReadiness({
      blocked: true,
      currentMissingEvidence: 1,
      pending: 1,
      ready: 1,
      reviewed: 1,
      status: "needs_evidence",
      total: 1,
    });
    const handoff: ReportDiagnosisHandoff = {
      evidenceSnapshotCount: 1,
      followUpCount: 1,
      reportID: 101,
      reportWorkflow: "FinalReportWorkflow",
      status: "needs_evidence",
      steps: [
        { key: "report_generation", status: "done" },
        { key: "evidence_snapshot", status: "done" },
        { key: "ai_consultation", status: "done" },
        { key: "evidence_follow_up", status: "pending" },
        { key: "operator_decision", status: "attention" },
      ],
    };
    const handoffCopy = localizeReportDiagnosisHandoff(
      handoff,
      readiness,
      "zh-CN",
      tZhCN,
    );
    expect(handoffCopy.statusLabel).toBe("需要补充证据");
    expect(handoffCopy.steps[0]).toMatchObject({
      detail: "最终报告 #101 由 FinalReportWorkflow 生成。",
      label: "报告已生成",
      statusLabel: "已完成",
    });
    expect(handoffCopy.steps[4]!.detail).toBe("请处理 1 项缺失证据。");

    const audit = auditItem({
      initialConfidence: "vendor-confidence",
      readiness: subReportReadiness({
        collectedEvidence: 1,
        currentMissingEvidence: 1,
        evidenceRequests: 1,
        latestConfidence: "vendor-confidence",
        status: "needs_evidence",
      }),
    });
    const auditCopy = localizeReportConsultationAuditItem(
      audit,
      "zh-CN",
      tZhCN,
      tStatusZhCN,
    );
    expect(auditCopy.steps[0]!.detail).toContain("vendor-confidence");
    expect(auditCopy.steps[1]!.detail).toBe(
      "提升或确认置信度前仍需处理 1 项缺失证据。已保留 1 项补充或已采集证据。",
    );

    const failedAuditCopy = localizeReportConsultationAuditItem(
      auditItem({
        readiness: subReportReadiness({
          failureReason: "none",
          latestConfidence: "failed",
          status: "failed",
        }),
      }),
      "en",
      tEn,
      tStatusEn,
    );
    expect(failedAuditCopy.steps[3]!.detail).toBe(
      "AI diagnosis failed before a final conclusion: none",
    );
  });

  it("localizes notification readiness, return notices, and retry outcomes", () => {
    const fallback: ReportFinalNotificationReadiness = {
      notification_purpose: "final",
      ready: false,
      reason: { kind: "fallback" },
      source: "fallback",
      status: "blocked",
    };
    expect(localizeFinalNotificationReadiness(fallback, tZhCN)).toEqual({
      detail:
        "最终通知就绪状态不可用；请完成诊断审核后再重试最终交付。",
      label: "最终通知已阻塞",
    });
    const ready: ReportFinalNotificationReadiness = {
      notification_purpose: "final",
      ready: true,
      reason: { kind: "ready" },
      source: "api",
      status: "ready",
    };
    expect(localizeFinalNotificationReadiness(ready, tZhCN)).toEqual({
      detail: "所有关联子报告均已有操作员确认的 AI 结论，可以发送最终通知。",
      label: "最终通知已就绪",
    });
    const blocked: ReportFinalNotificationReadiness = {
      notification_purpose: "handoff",
      ready: false,
      reason: {
        kind: "unconfirmed_conclusion",
        subReportID: 501,
        subReportTitle: "Checkout API latency",
      },
      source: "api",
      status: "blocked",
    };
    expect(localizeFinalNotificationReadiness(blocked, tZhCN)).toEqual({
      detail: "Checkout API latency 尚无操作员确认的 AI 结论。",
      label: "最终通知已阻塞",
    });
    expect(
      localizeReportDiagnosisReviewReturnNotice(
        "confirmed_blocked",
        fallback,
        tZhCN,
      ).detail,
    ).toContain("最终通知仍被阻塞");
    expect(
      localizeReportDeliveryProofRetryLabel(false, "final", tZhCN),
    ).toBe("发送最终报告通知");
    expect(
      localizeReportNotificationRetrySuccessMessage(
        "already_pending",
        "handoff",
        tEn,
      ),
    ).toBe(
      "No duplicate send was started because report handoff notification is already pending.",
    );
  });

  it("localizes delivery state and only translates the legacy channel", () => {
    const failedDelivery = delivery({
      failure_reason: "vendor refused request",
      status: "failed",
    });
    expect(
      localizeReportDeliveryProofState(
        reportDeliveryProofState(failedDelivery),
        tZhCN,
      ),
    ).toEqual({
      actionLabel: "检查通知配置",
      detail: "vendor refused request",
      statusLabel: "失败",
    });
    expect(
      localizeReportNotificationRetryChannelOption(
        {
          detail: "",
          kind: "legacy",
          label: "",
          profileID: null,
          value: "legacy",
        },
        tZhCN,
      ),
    ).toEqual({ detail: "服务端回退提供方", label: "旧版回退" });
    expect(
      localizeReportNotificationRetryChannelOption(
        {
          detail: "#7 / vendor-channel",
          kind: "profile",
          label: "Primary on-call",
          profileID: 7,
          value: "7",
        },
        tZhCN,
      ),
    ).toEqual({
      detail: "#7 / vendor-channel",
      label: "Primary on-call",
    });
  });

  it("localizes decision records while preserving external proof values", () => {
    const record = decisionRecord({
      notificationEventKind: "vendor.notification.sent",
      notificationProviderStatus: "delivered",
      roomCloseReason: "operator_confirmed_vendor_case",
      roomStatus: "vendor-room-state",
      status: "room_closed",
    });
    const copy = localizeReportDecisionRecord(
      record,
      "zh-CN",
      tZhCN,
      tStatusZhCN,
    );
    expect(copy.statusLabel).toBe("诊断室已关闭");
    expect(copy.detail).toContain("operator_confirmed_vendor_case");
    expect(copy.notificationLabel).toBe("vendor.notification.sent");
    expect(copy.notificationDetail).toContain("delivered");
    expect(copy.notificationDetail).not.toContain("已交付");
    expect(copy.roomStatus).toBe("vendor-room-state");
  });

  it("localizes bounded technical labels and preserves unknown values", () => {
    expect(
      localizeReportEvidenceRequestDetail(
        {
          query: "sum(rate(checkout_requests_total[5m]))",
          reason: "Inspect traffic",
          tool: "metric_range_query",
          window_seconds: 1800,
        },
        tZhCN,
      ),
    ).toBe(
      "查询：sum(rate(checkout_requests_total[5m])) / 窗口 1800 秒",
    );
    expect(
      localizeReportEvidenceCollectionResultDetail(
        {
          observed_alerts: 2,
          reason_code: "vendor-limit",
          status: "collected",
          tool: "active_alerts",
        },
        tZhCN,
      ),
    ).toBe("active_alerts / 原因：vendor-limit / 2 条告警");
    expect(localizeReportConclusionSource("vendor_source", tZhCN)).toBe(
      "vendor_source",
    );
    expect(
      localizeReportConclusionSource(" Latest_Assistant_Turn ", tZhCN),
    ).toBe("最近一次助手轮次");
    expect(localizeReportConclusionReason("vendor_reason", tZhCN)).toBe(
      "vendor_reason",
    );
  });
});

function reportReadiness(
  overrides: Partial<ReportDiagnosisReadiness> = {},
): ReportDiagnosisReadiness {
  return {
    attention: 0,
    blocked: false,
    canConfirm: false,
    collectedEvidence: 0,
    currentCollectionSuggestions: 0,
    currentExecutableEvidenceRequests: 0,
    currentMissingEvidence: 0,
    done: 0,
    evidenceRequests: 0,
    failedSubReports: 0,
    humanReviewRequired: 0,
    latestConfidence: "pending",
    pending: 0,
    pendingSubReports: 0,
    ready: 0,
    reviewed: 0,
    status: "pending_diagnosis",
    supplementalEvidence: 0,
    total: 1,
    ...overrides,
  };
}

function subReportReadiness(
  overrides: Partial<SubReportDiagnosisReadiness> = {},
): SubReportDiagnosisReadiness {
  return {
    collectedEvidence: 0,
    currentCollectionSuggestions: 0,
    currentExecutableEvidenceRequests: 0,
    currentMissingEvidence: 0,
    evidenceRequests: 0,
    latestConfidence: "pending",
    reviewed: true,
    status: "pending_diagnosis",
    supplementalEvidence: 0,
    ...overrides,
  };
}

function auditItem(
  overrides: Partial<ReportConsultationAuditItem> = {},
): ReportConsultationAuditItem {
  return {
    conclusionVersion: "",
    confirmed: false,
    evidenceSnapshotID: 9001,
    hasDiagnosisState: true,
    initialConfidence: "low",
    readiness: subReportReadiness(),
    steps: [
      { key: "initial_report", status: "done" },
      { key: "supplemental_evidence", status: "pending" },
      { key: "confidence_revision", status: "done" },
      { key: "final_decision", status: "pending" },
    ],
    subReportID: 501,
    subReportTitle: "Checkout API latency",
    ...overrides,
  };
}

function decisionRecord(
  overrides: Partial<ReportDecisionRecord> = {},
): ReportDecisionRecord {
  return {
    confirmedBy: "",
    evidenceSnapshotID: 9001,
    notificationChannelProfileID: 7,
    notificationEventKind: "diagnosis_room.close_notification_sent",
    notificationFailed: false,
    notificationOccurredAt: "2026-06-18T08:11:01Z",
    notificationProviderMessageID: "provider-message-1",
    notificationProviderStatus: "delivered",
    readiness: subReportReadiness({ status: "complete" }),
    recordedAt: "2026-06-18T08:10:00Z",
    requiresHumanReview: false,
    roomClosedAt: "2026-06-18T08:11:00Z",
    roomCloseReason: "human_confirmed",
    roomLinked: true,
    roomStatus: "closed",
    roomTurnCount: 2,
    sessionID: "diagnosis-session-301",
    status: "confirmed",
    subReportID: 501,
    title: "Checkout API latency",
    version: "diagnosis-session-301:2",
    ...overrides,
  };
}

function delivery(
  overrides: Partial<FinalReportDetail["notification_deliveries"][number]> = {},
): FinalReportDetail["notification_deliveries"][number] {
  return {
    created_at: "2026-06-18T08:00:00Z",
    failure_reason: undefined,
    id: 801,
    idempotency_key: "final_report:101/notification/final",
    notification_purpose: "final",
    provider_message_id: undefined,
    provider_status: undefined,
    report_notification_channel_profile_id: null,
    status: "pending",
    updated_at: "2026-06-18T08:00:01Z",
    ...overrides,
  };
}
