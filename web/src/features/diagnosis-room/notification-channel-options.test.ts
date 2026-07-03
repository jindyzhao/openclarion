import { describe, expect, it } from "vitest";

import type {
  NotificationChannelProfile,
  NotificationChannelTestResult,
} from "@/features/settings/notification-channels/types";

import {
  diagnosisDefaultNotificationChannelProfileID,
  diagnosisNotificationChannelCreateBlockReason,
  diagnosisNotificationChannelOptions,
  diagnosisNotificationChannelProofSummary,
  diagnosisNotificationChannelSelectionError,
  diagnosisNotificationChannelSetupAction,
} from "./notification-channel-options";

describe("diagnosis notification channel options", () => {
  it("keeps ready channels selectable and explains unavailable channels", () => {
    const options = diagnosisNotificationChannelOptions([
      channel({
        delivery_scopes: ["report", "diagnosis_close"],
        enabled: true,
        id: 2,
        name: "Close only",
      }),
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
        kind: "webhook",
        name: "Generic webhook",
      }),
      channel({
        delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
        enabled: false,
        id: 1,
        name: "Disabled room",
      }),
    ]);

    expect(options).toEqual([
      {
        disabled: undefined,
        label: "#3 AI room",
        title: "AI room: ready for diagnosis room notifications",
        value: 3,
      },
      {
        disabled: true,
        label: "#1 Disabled room (disabled)",
        title: "Disabled room: disabled",
        value: 1,
      },
      {
        disabled: true,
        label: "#2 Close only (missing diagnosis_consultation)",
        title: "Close only: missing diagnosis_consultation",
        value: 2,
      },
      {
        disabled: true,
        label: "#4 Generic webhook (not Enterprise WeChat)",
        title: "Generic webhook: not Enterprise WeChat",
        value: 4,
      },
    ]);
  });

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
      diagnosisNotificationChannelSelectionError(undefined, channels),
    ).toBe("");
    expect(diagnosisNotificationChannelSelectionError(3, channels)).toBe("");
    expect(diagnosisNotificationChannelSelectionError(9, channels)).toBe(
      "Selected notification channel was not found.",
    );
    expect(diagnosisNotificationChannelSelectionError(4, channels)).toBe(
      "Selected notification channel is not ready: disabled; missing diagnosis_close.",
    );
    expect(
      diagnosisNotificationChannelSelectionError(
        5,
        channels.concat(
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 5,
            kind: "webhook",
            name: "Generic webhook",
          }),
        ),
      ),
    ).toBe(
      "Selected notification channel is not ready: not Enterprise WeChat.",
    );
    expect(
      diagnosisNotificationChannelSelectionError(
        6,
        channels.concat(
          channel({
            delivery_scopes: ["diagnosis_consultation", "diagnosis_close"],
            enabled: true,
            id: 6,
            kind: "wecom",
            name: "Direct endpoint",
            secret_ref: "https://endpoint.example.test/robot",
          }),
        ),
      ),
    ).toBe(
      "Selected notification channel is not ready: credential secret reference stores an endpoint URL.",
    );
    expect(
      diagnosisNotificationChannelSelectionError(
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
    ).toBe(
      "Selected notification channel is not ready: missing current ai diagnosis sample and diagnosis close sample proof.",
    );
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
      diagnosisNotificationChannelCreateBlockReason({
        channelID: undefined,
        channels: [ready],
      }),
    ).toBe("");
    expect(
      diagnosisNotificationChannelCreateBlockReason({
        channelID: 3,
        channels: [ready],
      }),
    ).toBe("");
    expect(
      diagnosisNotificationChannelCreateBlockReason({
        channelID: 4,
        channels: [ready, disabled],
      }),
    ).toBe("Selected notification channel is not ready: disabled.");
    expect(
      diagnosisNotificationChannelCreateBlockReason({
        channelID: undefined,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toBe("");
    expect(
      diagnosisNotificationChannelCreateBlockReason({
        channelID: 3,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toBe(
      "Load notification channels before creating a diagnosis room with Enterprise WeChat delivery.",
    );
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
      detail:
        "Select an Enterprise WeChat channel with current AI diagnosis and close sample proof.",
      label: "Channel proof pending.",
      status: "pending",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 3,
        channels: [ready],
      }),
    ).toEqual({
      detail:
        "AI diagnosis sample checked at 2026-06-21T01:01:00Z; Diagnosis close sample checked at 2026-06-21T01:01:00Z",
      label: "Channel proof ready.",
      status: "ready",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 4,
        channels: [ready, untested],
      }),
    ).toMatchObject({
      label: "AI delivery test proof needs review.",
      status: "review",
    });
    expect(
      diagnosisNotificationChannelProofSummary({
        channelID: 3,
        channels: [ready],
        failedToLoad: true,
      }),
    ).toEqual({
      detail:
        "Notification channels could not be loaded, so Enterprise WeChat proof cannot be checked.",
      label: "Channel proof unavailable.",
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
      detail:
        "Create an Enterprise WeChat channel before relying on AI diagnosis room updates and close notifications.",
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      label: "Create WeCom channel",
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
      detail:
        "No configured channel is ready for both AI diagnosis updates and close notifications. Use the Enterprise WeChat preset to add the required scopes.",
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      label: "Prepare WeCom channel",
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
      detail:
        "The selected Enterprise WeChat channel has the required diagnosis scopes. Open it and run the missing AI diagnosis and close notification proof tests.",
      href: "/settings/notification-channels?channel_id=7",
      label: "Run WeCom AI proof",
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
      label: "Run WeCom AI proof",
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
      detail:
        "Notification channels could not be loaded. Open the Enterprise WeChat preset to create or review a channel with diagnosis update and close scopes.",
      href: "/settings/notification-channels?intent=diagnosis-room-channel",
      label: "Open WeCom channel setup",
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
