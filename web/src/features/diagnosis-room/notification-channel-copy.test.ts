import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import zhCN from "../../../messages/zh-CN.json";
import type {
  NotificationChannelProfile,
  NotificationChannelTestResult,
} from "@/features/settings/notification-channels/types";

import {
  localizeDiagnosisNotificationChannelOptions,
  localizeDiagnosisNotificationChannelProofSummary,
  localizeDiagnosisNotificationChannelSelectionError,
  localizeDiagnosisNotificationChannelSetupAction,
} from "./notification-channel-copy";

const t = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisRoom.notificationChannel",
});

describe("diagnosis notification channel copy", () => {
  it("localizes readiness without parsing English presentation text", () => {
    const channels = [
      channel({ id: 3, name: "AI room" }),
      channel({
        enabled: false,
        id: 4,
        latest_test_results: [],
        name: "Disabled room",
      }),
    ];

    expect(
      localizeDiagnosisNotificationChannelOptions(channels, "zh-CN", t),
    ).toEqual([
      {
        disabled: undefined,
        label: "#3 AI room",
        title: "AI room：已可用于诊断室通知",
        value: 3,
      },
      {
        disabled: true,
        label: "#4 Disabled room（已禁用）",
        title: "Disabled room：已禁用",
        value: 4,
      },
    ]);
    expect(
      localizeDiagnosisNotificationChannelSelectionError(4, channels, t),
    ).toBe("所选通知渠道尚未就绪：已禁用。");
    expect(
      localizeDiagnosisNotificationChannelSelectionError(99, channels, t),
    ).toBe("未找到所选通知渠道。");
  });

  it("localizes proof guidance while preserving the structured destination", () => {
    const untested = channel({
      id: 7,
      latest_test_results: [],
      name: "Untested WeCom",
    });
    const action = localizeDiagnosisNotificationChannelSetupAction(
      { channels: [untested] },
      t,
    );
    const summary = localizeDiagnosisNotificationChannelProofSummary(
      { channelID: 7, channels: [untested] },
      t,
    );

    expect(action).toEqual({
      detail:
        "企业微信渠道具备所需诊断范围。请打开渠道并执行缺失的 AI 诊断和关闭通知证明测试。",
      href: "/settings/notification-channels?channel_id=7",
      label: "执行企业微信 AI 证明测试",
    });
    expect(summary).toEqual({
      detail:
        "依赖此渠道承载 AI 诊断室前，请在最新渠道更新后执行当前AI 诊断样例 / 诊断关闭样例测试。",
      label: "渠道证明需要审核",
      status: "review",
    });
  });
});

function channel(
  overrides: Partial<NotificationChannelProfile> = {},
): NotificationChannelProfile {
  const id = overrides.id ?? 1;
  const kind = overrides.kind ?? "wecom";
  return {
    created_at: "2026-07-16T00:00:00Z",
    delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
    enabled: true,
    id,
    kind,
    labels: {},
    latest_test_results: [
      channelTestResult(id, kind, "ai_diagnosis_sample"),
      channelTestResult(id, kind, "diagnosis_close_sample"),
    ],
    name: "Channel",
    secret_ref: "secret/channel",
    updated_at: "2026-07-16T00:00:00Z",
    ...overrides,
  };
}

function channelTestResult(
  channelID: number,
  kind: NotificationChannelProfile["kind"],
  contentKind: NonNullable<NotificationChannelTestResult["content_kind"]>,
): NotificationChannelTestResult {
  return {
    channel_id: channelID,
    checked_at: "2026-07-16T00:01:00Z",
    content_kind: contentKind,
    content_sha256: "a".repeat(64),
    kind,
    message: "Notification channel test delivery succeeded.",
    provider_message_id: "provider-message-1",
    provider_status: "delivered",
    reason_code: "ok",
    status: "success",
  };
}
