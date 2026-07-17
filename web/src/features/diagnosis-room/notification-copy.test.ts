import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import type { DiagnosisRoomNotificationTimelineEntry } from "./api";
import {
  diagnosisNotificationContentProofDisplay,
  diagnosisNotificationContentProofSummary,
  diagnosisNotificationDeliveryCoverage,
} from "./notification-content-proof";
import {
  localizeDiagnosisNotificationContentProof,
  localizeDiagnosisNotificationContentProofSummary,
  localizeDiagnosisNotificationDeliveryCoverage,
  localizeDiagnosisNotificationEvent,
} from "./notification-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.notification",
});

describe("diagnosis notification presentation copy", () => {
  it("localizes retained AI output proof from structured metadata", () => {
    const entry = notificationEntry({
      content_kind: "assistant_message",
      content_sha256: "a".repeat(64),
      evidence_request_count: 2,
      recommended_action_count: 1,
    });

    const display = localizeDiagnosisNotificationContentProof(
      entry,
      diagnosisNotificationContentProofDisplay(entry),
      t,
    );

    expect(display.label).toBe("AI 输出证明");
    expect(display.detail).toContain("助手消息摘要");
    expect(display.detail).toContain("2 项证据请求");
  });

  it("localizes proof summaries and delivery phases without parsing English", () => {
    const entries = [
      notificationEntry({
        content_kind: "assistant_message",
        content_sha256: "b".repeat(64),
      }),
      notificationEntry({
        event_kind: "diagnosis_room.final_ready_notification_sent",
        provider_status: "failed",
      }),
    ];
    const summary = localizeDiagnosisNotificationContentProofSummary(
      diagnosisNotificationContentProofSummary(entries),
      t,
    );
    const coverage = localizeDiagnosisNotificationDeliveryCoverage(
      diagnosisNotificationDeliveryCoverage(entries),
      t,
    );

    expect(summary.detail).toContain("缺少 AI 输出摘要证明");
    expect(coverage.label).toBe("AI 交付失败");
    expect(coverage.phases.map((phase) => phase.label)).toEqual([
      "AI 更新",
      "最终结论",
      "关闭",
    ]);
    expect(
      localizeDiagnosisNotificationEvent(
        "diagnosis_room.close_notification_sent",
        t,
      ),
    ).toBe("关闭通知");
  });
});

function notificationEntry(
  overrides: Partial<DiagnosisRoomNotificationTimelineEntry> = {},
): DiagnosisRoomNotificationTimelineEntry {
  return {
    event_kind: "diagnosis_room.assistant_turn_notification_sent",
    occurred_at: "2026-07-16T00:00:00Z",
    provider_status: "delivered",
    ...overrides,
  };
}
