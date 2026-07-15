import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";

import type { DiagnosisRoomNextStep } from "./next-step";
import { localizeDiagnosisRoomNextStep } from "./next-step-copy";
import { localizeDiagnosisRoomStatus } from "./status-copy";

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

describe("diagnosis room presentation copy", () => {
  it("renders bounded next-step codes from the Chinese catalog", () => {
    const copy = localizeDiagnosisRoomNextStep(
      {
        bucket: "attention",
        code: "notification_failed",
        color: "error",
        detail:
          "Review the failed diagnosis-room notification before relying on downstream handoff.",
        detailKey: "notification_failed",
        label: "Notification failed",
      },
      "zh-CN",
      tNextStep,
    );

    expect(copy).toEqual({
      detail: "依赖下游移交前，请先检查失败的诊断室通知。",
      label: "通知失败",
    });
  });

  it("formats structured evidence counts without parsing English copy", () => {
    const step = {
      bucket: "attention",
      code: "collect_evidence",
      color: "warning",
      detail: "AI requested evidence: 2 planned, 1 missing, 1 suggestion(s).",
      detailKey: "collect_evidence_counts",
      detailValues: { missing: 1, planned: 2, suggestions: 1 },
      label: "Collect evidence",
    } satisfies DiagnosisRoomNextStep;

    const copy = localizeDiagnosisRoomNextStep(
      step,
      "zh-CN",
      tNextStep,
    );

    expect(copy.label).toBe("收集证据");
    expect(copy.detail).toContain("计划 2 项");
    expect(copy.detail).toContain("缺失 1 项");
    expect(copy.detail).toContain("建议 1 项");
    expect(copy.detail).not.toContain("planned");
  });

  it("preserves operator or provider detail while localizing its status", () => {
    const copy = localizeDiagnosisRoomNextStep(
      {
        bucket: "ready",
        code: "review_ai_report",
        color: "success",
        detail: "Collected evidence supports confirmation.",
        label: "Review AI report",
      },
      "zh-CN",
      tNextStep,
    );

    expect(copy).toEqual({
      detail: "Collected evidence supports confirmation.",
      label: "审核 AI 报告",
    });
  });

  it("localizes only known status values", () => {
    expect(localizeDiagnosisRoomStatus("delivered", tStatus)).toBe("已送达");
    expect(localizeDiagnosisRoomStatus("vendor-specific", tStatus)).toBe(
      "vendor-specific",
    );
  });
});
