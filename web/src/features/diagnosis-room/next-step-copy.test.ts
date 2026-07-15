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
      tStatus,
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
      tStatus,
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
      tStatus,
    );

    expect(copy).toEqual({
      detail: "Collected evidence supports confirmation.",
      label: "审核 AI 报告",
    });
  });

  it("localizes only known status values", () => {
    expect(localizeDiagnosisRoomStatus("available", tStatus)).toBe("可用");
    expect(localizeDiagnosisRoomStatus("delivered", tStatus)).toBe("已送达");
    expect(localizeDiagnosisRoomStatus("in_progress", tStatus)).toBe("进行中");
    expect(localizeDiagnosisRoomStatus("vendor-specific", tStatus)).toBe(
      "vendor-specific",
    );
  });

  it.each([
    ["not_found", "未找到"],
    ["completed", "已完成"],
    ["failed", "失败"],
    ["canceled", "已取消"],
    ["terminated", "已终止"],
    ["timed_out", "已超时"],
    ["continued_as_new", "已继续为新运行"],
  ])("localizes known unavailable workflow status %s", (status, expected) => {
    const copy = localizeDiagnosisRoomNextStep(
      {
        bucket: "attention",
        code: "workflow_unavailable",
        color: "error",
        detail: `Temporal reports workflow status ${status}.`,
        detailKey: "workflow_unavailable",
        detailValues: { status },
        label: "Workflow unavailable",
      },
      "zh-CN",
      tNextStep,
      tStatus,
    );

    expect(copy.detail).toContain(expected);
    expect(copy.detail).not.toContain(status);
  });

  it("preserves unknown workflow statuses in localized detail", () => {
    const copy = localizeDiagnosisRoomNextStep(
      {
        bucket: "attention",
        code: "workflow_unavailable",
        color: "error",
        detail: "Temporal reports workflow status vendor-specific.",
        detailKey: "workflow_unavailable",
        detailValues: { status: "vendor-specific" },
        label: "Workflow unavailable",
      },
      "zh-CN",
      tNextStep,
      tStatus,
    );

    expect(copy.detail).toContain("vendor-specific");
  });
});
