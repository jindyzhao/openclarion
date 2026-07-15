import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import type { DiagnosisNotificationDeliveryCoverage } from "@/features/diagnosis-room/notification-content-proof";

import zhCN from "../../../messages/zh-CN.json";

import {
  localizeAlertDiagnosisClosure,
  localizeAlertDiagnosisEvidenceProgress,
  localizeAlertDiagnosisRoomPrimaryAction,
  localizeDiagnosisNotificationContentProof,
  localizeDiagnosisNotificationCoverage,
} from "./alerts-copy";
import type {
  AlertDiagnosisClosureSummary,
  AlertDiagnosisEvidenceProgressSummary,
  AlertDiagnosisRoomPrimaryAction,
} from "./diagnosis-delivery";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "Alerts",
});
const tNextStep = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.nextStep",
});
const tStatus = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.status",
});

describe("alert operational presentation copy", () => {
  it("localizes a primary room action from its stable next-step code", () => {
    const action = {
      danger: false,
      href: "/diagnosis-room?intent=confidence_review",
      iconKind: "attention",
      kind: "room_step",
      step: {
        bucket: "attention",
        code: "collect_evidence",
        color: "warning",
        detail: "AI requested evidence: 2 planned, 1 missing.",
        detailKey: "collect_evidence_counts",
        detailValues: { missing: 1, planned: 2, suggestions: 0 },
        label: "Collect evidence",
      },
    } satisfies AlertDiagnosisRoomPrimaryAction;

    const copy = localizeAlertDiagnosisRoomPrimaryAction(
      action,
      "zh-CN",
      t,
      tNextStep,
      tStatus,
    );

    expect(copy.label).toBe("收集证据");
    expect(copy.hint).toContain("计划 2 项");
    expect(copy.hint).toContain("缺失 1 项");
    expect(copy.hint).not.toContain("planned");
  });

  it("formats evidence progress with localized statuses and list rules", () => {
    const summary = {
      collectedEvidence: 1,
      evidenceState: "needed",
      initialConfidence: "low",
      latestConfidence: "medium",
      openEvidence: 1,
      supplementalEvidence: 1,
      timelineEntries: 2,
    } satisfies AlertDiagnosisEvidenceProgressSummary;

    expect(
      localizeAlertDiagnosisEvidenceProgress(
        summary,
        "zh-CN",
        t,
        tStatus,
      ),
    ).toEqual({
      confidenceLabel: "低 -> 中",
      detail: "已采集 1 项、已补充 1 项、待处理 1 项和2 次置信度更新。",
      evidenceLabel: "需要证据",
    });
  });

  it("localizes structured notification proof while retaining its digest", () => {
    expect(
      localizeDiagnosisNotificationContentProof(
        {
          content_kind: "assistant_message",
          content_sha256: "a".repeat(64),
          event_kind: "diagnosis_room.assistant_turn_notification_sent",
          occurred_at: "2026-07-15T01:00:00Z",
          provider_status: "delivered",
        },
        {
          color: "success",
          detail: "AI assistant message digest aaaaaaaaaaaa",
          digestPreview: "aaaaaaaaaaaa",
          evidenceRequestCount: 1,
          hasProof: true,
          kindLabel: "assistant message",
          label: "AI output proof",
          recommendedActionCount: 2,
        },
        "zh-CN",
        t,
      ),
    ).toEqual({
      detail: "AI 助手消息内容摘要 aaaaaaaaaaaa、2 个操作和1 个证据请求",
      label: "AI 输出证明",
    });
  });

  it("localizes delivery and closure states from structured coverage", () => {
    const deliveryCoverage: DiagnosisNotificationDeliveryCoverage = {
      color: "warning",
      detail: "1 of 3 AI notification phases are complete.",
      label: "AI delivery incomplete",
      phases: [
        {
          detail: "AI update delivered.",
          eventKind: "diagnosis_room.assistant_turn_notification_sent",
          key: "assistant_update",
          label: "AI update",
          status: "delivered",
        },
        {
          detail: "Final conclusion missing.",
          eventKind: "diagnosis_room.final_ready_notification_sent",
          key: "final_conclusion",
          label: "Final conclusion",
          status: "missing",
        },
        {
          detail: "Close missing.",
          eventKind: "diagnosis_room.close_notification_sent",
          key: "close",
          label: "Close",
          status: "missing",
        },
      ],
      readyCount: 1,
      requiredCount: 3,
      status: "review",
    };
    const closure = {
      closeProofStatus: "missing",
      confirmedBy: "owner-1",
      deliveryCoverage,
      roomClosed: true,
      traceability: {
        color: "warning",
        detail: "AI delivery proof is incomplete.",
        label: "Closure delivery proof incomplete",
        notificationLabel: "AI delivery incomplete",
        reviewOpenCount: 0,
        reviewResidualCount: 0,
        status: "review",
      },
    } satisfies AlertDiagnosisClosureSummary;

    expect(localizeDiagnosisNotificationCoverage(deliveryCoverage, t)).toEqual({
      detail:
        "3 个 AI 通知阶段中已完成 1 个。接受交付前，请保留 AI 进展、最终结论和关闭通知证明。",
      label: "AI 交付不完整",
      phases: [
        { key: "assistant_update", label: "AI 进展", status: "已送达" },
        { key: "final_conclusion", label: "最终结论", status: "缺失" },
        { key: "close", label: "关闭", status: "缺失" },
      ],
    });
    expect(localizeAlertDiagnosisClosure(closure, t)).toEqual({
      closeProofLabel: "关闭：缺失",
      detail:
        "操作员确认的结论已保留，但 AI 交付证明不完整：3 个 AI 通知阶段中已完成 1 个。接受交付前，请保留 AI 进展、最终结论和关闭通知证明。",
      label: "关闭交付证明不完整",
    });
  });
});
