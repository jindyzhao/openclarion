import { describe, expect, it } from "vitest";

import type {
  NotificationChannelProfile,
  NotificationChannelTestResult,
} from "@/features/settings/notification-channels/types";

import {
  diagnosisDefaultNotificationChannelProfileID,
  diagnosisNotificationChannelCreateBlocker,
  diagnosisNotificationChannelProofSummary,
  diagnosisNotificationChannelReadinessIssues,
  diagnosisNotificationChannelSelectionStatus,
  diagnosisNotificationChannelSetupAction,
} from "./notification-channel-options";

describe("diagnosis notification channel options", () => {
  it("validates selected channel readiness", () => {
    const channels = [
      channel({
        delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
        enabled: true,
        id: 3,
        kind: "wecom",
        name: "AI room",
      }),
      channel({
        delivery_scopes: ["diagnosis_consultation"],
        enabled: false,
        id: 4,
        name: "Incomplete",
      }),
    ];

    expect(
      diagnosisNotificationChannelSelectionStatus(undefined, channels),
    ).toBe("none");
    expect(diagnosisNotificationChannelSelectionStatus(3, channels)).toBe(
      "ready",
    );
    expect(diagnosisNotificationChannelSelectionStatus(9, channels)).toBe(
      "not-found",
    );
    expect(diagnosisNotificationChannelSelectionStatus(4, channels)).toBe(
      "not-ready",
    );
    expect(diagnosisNotificationChannelReadinessIssues(channels[1]!)).toEqual([
      "disabled",
      "missing-close-scope",
    ]);
    const genericWebhook = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 5,
      kind: "webhook",
      name: "Generic webhook",
    });
    expect(
      diagnosisNotificationChannelSelectionStatus(
        5,
        channels.concat(genericWebhook),
      ),
    ).toBe("not-ready");
    expect(diagnosisNotificationChannelReadinessIssues(genericWebhook)).toEqual([
      "not-wecom",
    ]);
    const endpointChannel = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 6,
      kind: "wecom",
      name: "Direct endpoint",
      secret_ref: "https://endpoint.example.test/robot",
    });
    expect(
      diagnosisNotificationChannelSelectionStatus(
        6,
        channels.concat(endpointChannel),
      ),
    ).toBe("not-ready");
    expect(diagnosisNotificationChannelReadinessIssues(endpointChannel)).toEqual([
      "endpoint-secret",
    ]);
    expect(
      diagnosisNotificationChannelSelectionStatus(
        7,
        channels.concat(
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 7,
            kind: "wecom",
            latest_test_results: [],
            name: "Untested room",
          }),
        ),
      ),
    ).toBe("not-ready");
  });

  it("allows room creation without a notification channel and validates selected channels", () => {
    const ready = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 3,
      kind: "wecom",
      name: "AI room",
    });
    const disabled = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: false,
      id: 4,
      name: "Disabled",
    });

    expect(
      diagnosisNotificationChannelCreateBlocker({
        channelID: undefined,
        channels: [ready],
      }),
    ).toBeNull();
    expect(
      diagnosisNotificationChannelCreateBlocker({
        channelID: 3,
        channels: [ready],
      }),
    ).toBeNull();
    expect(
      diagnosisNotificationChannelCreateBlocker({
        channelID: 4,
        channels: [ready, disabled],
      }),
    ).toBe("not-ready");
    expect(
      diagnosisNotificationChannelCreateBlocker({
        channelID: undefined,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toBeNull();
    expect(
      diagnosisNotificationChannelCreateBlocker({
        channelID: 3,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toBe("load-failed");
  });

  it("summarizes selected Enterprise WeChat proof before room creation", () => {
    const ready = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 3,
      kind: "wecom",
      name: "AI room",
    });
    const untested = channel({
      delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
      enabled: true,
      id: 4,
      kind: "wecom",
      latest_test_results: [],
      name: "Untested",
    });

    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: undefined,
        channels: [ready],
      }),
    ).toEqual({
      kind: "not-selected",
      missingContentKinds: [],
      status: "pending",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 3,
        channels: [ready],
      }),
    ).toEqual({
      kind: "ready",
      missingContentKinds: [],
      proof: {
        aiDiagnosisCheckedAt: "2026-06-21T01:01:00Z",
        diagnosisCloseCheckedAt: "2026-06-21T01:01:00Z",
      },
      status: "ready",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 4,
        channels: [ready, untested],
      }),
    ).toMatchObject({
      kind: "review",
      missingContentKinds: [
        "ai_diagnosis_sample",
        "diagnosis_close_sample",
      ],
      status: "review",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 3,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toEqual({
      kind: "load-failed",
      missingContentKinds: [],
      status: "blocked",
    });
  });

  it("selects only unambiguous default channels", () => {
    expect(
      diagnosisDefaultNotificationChannelProfileID([
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 3,
          kind: "wecom",
          name: "AI room",
        }),
      ]),
    ).toBe(3);

    expect(
      diagnosisDefaultNotificationChannelProfileID([
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 3,
          kind: "wecom",
          name: "AI room",
        }),
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 4,
          kind: "wecom",
          labels: { role: "ai-room-delivery" },
          name: "Preferred AI room",
        }),
      ]),
    ).toBe(4);

    expect(
      diagnosisDefaultNotificationChannelProfileID([
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 3,
          kind: "webhook",
          name: "Generic AI room",
        }),
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 4,
          kind: "wecom",
          name: "Enterprise WeChat AI room",
        }),
      ]),
    ).toBe(4);

    expect(
      diagnosisDefaultNotificationChannelProfileID([
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 3,
          kind: "webhook",
          name: "Generic AI room",
        }),
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 4,
          kind: "wecom",
          name: "Enterprise WeChat primary",
        }),
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 5,
          kind: "wecom",
          name: "Enterprise WeChat backup",
        }),
      ]),
    ).toBeUndefined();

    expect(
      diagnosisDefaultNotificationChannelProfileID([
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 3,
          kind: "wecom",
          name: "AI room",
        }),
        channel({
          delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
          enabled: true,
          id: 4,
          kind: "wecom",
          name: "Backup AI room",
        }),
      ]),
    ).toBeUndefined();
  });

  it("guides operators to the WeCom preset when no diagnosis notification channel is ready", () => {
    expect(
      diagnosisNotificationChannelSetupAction({ channels: [] }),
    ).toEqual({
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      kind: "empty",
    });

    expect(
      diagnosisNotificationChannelSetupAction({
        channels: [
          channel({
            delivery_scopes: ["report", "diagnosis_close"],
            enabled: true,
            id: 2,
            name: "Close only",
          }),
        ],
      }),
    ).toEqual({
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      kind: "not-ready",
    });

    expect(
      diagnosisNotificationChannelSetupAction({
        channels: [
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 7,
            kind: "wecom",
            latest_test_results: [],
            name: "Untested WeCom",
          }),
        ],
      }),
    ).toEqual({
      href: "/settings/notification-channels?channel_id=7",
      kind: "proof-review",
    });

    expect(
      diagnosisNotificationChannelSetupAction({
        channels: [
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 3,
            kind: "wecom",
            name: "Ready channel",
          }),
        ],
      }),
    ).toBeNull();
  });

  it("prefers the labeled WeCom channel when multiple channels need AI proof", () => {
    expect(
      diagnosisNotificationChannelSetupAction({
        channels: [
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 8,
            kind: "wecom",
            latest_test_results: [],
            name: "Generic WeCom",
          }),
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 9,
            kind: "wecom",
            labels: { role: "ai-room-delivery" },
            latest_test_results: [],
            name: "AI room WeCom",
          }),
        ],
      }),
    ).toMatchObject({
      href: "/settings/notification-channels?channel_id=9",
      kind: "proof-review",
    });
  });

  it("keeps the WeCom setup action available when channel loading fails", () => {
    expect(
      diagnosisNotificationChannelSetupAction({
        channels: [
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 3,
            kind: "wecom",
            name: "Ready channel",
          }),
        ],
        failedToLoad: true,
      }),
    ).toEqual({
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      kind: "load-failed",
    });
  });
});

function channel(
  overrides: Partial<NotificationChannelProfile>,
): NotificationChannelProfile {
  const id = overrides.id ?? 1;
  const kind = overrides.kind ?? "wecom";
  return {
    created_at: "2026-06-21T01:00:00Z",
    delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
    enabled: true,
    id,
    kind,
    labels: {},
    latest_test_results: channelTestProofs(id, kind),
    name: "Channel",
    secret_ref: "secret/channel",
    updated_at: "2026-06-21T01:00:00Z",
    ...overrides,
  };
}

function channelTestProofs(
  channelID: number,
  kind: NotificationChannelProfile["kind"],
): NotificationChannelTestResult[] {
  return [
    channelTestResult(channelID, kind, "ai_diagnosis_sample"),
    channelTestResult(channelID, kind, "diagnosis_close_sample"),
  ];
}

function channelTestResult(
  channelID: number,
  kind: NotificationChannelProfile["kind"],
  contentKind: NonNullable<NotificationChannelTestResult["content_kind"]>,
): NotificationChannelTestResult {
  return {
    channel_id: channelID,
    checked_at: "2026-06-21T01:01:00Z",
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
