import { describe, expect, it } from "vitest";

import {
  reportDeliveryProofCurrentDeliveries,
  reportDeliveryProofCanRetry,
  reportDeliveryProofCanSubmit,
  reportDeliveryProofState,
  reportNotificationDeliveryForPurpose,
  reportNotificationDeliveryPurpose,
  reportNotificationRetryChannelOptions,
  selectedReportNotificationRetryChannelID,
  defaultReportNotificationRetryChannelValue,
  upsertReportNotificationDeliveryOverlay,
  upsertReportNotificationDeliveryProof,
} from "./report-delivery-proof";
import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";
import type { NotificationChannelProfile } from "@/features/settings/notification-channels/types";
import type { FinalReportDetail } from "./types";

type ReportNotificationDelivery =
  FinalReportDetail["notification_deliveries"][number];

describe("report delivery proof state", () => {
  it("offers report workflow policy review when no delivery proof exists", () => {
    expect(reportDeliveryProofState(null)).toEqual({
      actionHref: "/settings/report-workflow-policies",
      delivery: null,
      purpose: "final",
      status: "missing",
    });
  });

  it("offers notification settings review when delivery failed", () => {
    expect(
      reportDeliveryProofState(
        delivery({
          failure_reason: "send-report-notification: im provider is not configured",
          status: "failed",
        }),
      ),
    ).toEqual({
      actionHref: "/settings/notification-channels",
      delivery: expect.objectContaining({
        failure_reason: "send-report-notification: im provider is not configured",
      }),
      purpose: "final",
      status: "failed",
    });
  });

  it("does not offer a repair action after delivery succeeds", () => {
    expect(
      reportDeliveryProofState(
        delivery({
          provider_message_id: "wecom-final-report-101",
          status: "delivered",
        }),
      ),
    ).toMatchObject({
      actionHref: "",
      delivery: expect.objectContaining({
        provider_message_id: "wecom-final-report-101",
      }),
      purpose: "final",
      status: "delivered",
    });
  });

  it("allows retry only when delivery proof is missing or failed", () => {
    expect(reportDeliveryProofCanRetry(null)).toBe(true);
    expect(reportDeliveryProofCanRetry(delivery({ status: "failed" }))).toBe(true);
    expect(reportDeliveryProofCanRetry(delivery({ status: "pending" }))).toBe(false);
    expect(reportDeliveryProofCanRetry(delivery({ status: "delivered" }))).toBe(false);
  });

  it("uses final notification readiness before allowing final sends", () => {
    expect(reportDeliveryProofCanSubmit(null, "final")).toBe(false);
    expect(
      reportDeliveryProofCanSubmit(null, "final", finalNotificationReadiness({ ready: false })),
    ).toBe(false);
    expect(reportDeliveryProofCanSubmit(null, "handoff")).toBe(true);
    expect(
      reportDeliveryProofCanSubmit(
        delivery({ status: "failed" }),
        "final",
        finalNotificationReadiness({ ready: true }),
      ),
    ).toBe(true);
    expect(
      reportDeliveryProofCanSubmit(
        delivery({ notification_purpose: "handoff", status: "failed" }),
        "handoff",
        finalNotificationReadiness({ ready: false }),
      ),
    ).toBe(true);
    expect(
      reportDeliveryProofCanSubmit(
        delivery({ status: "pending" }),
        "final",
        finalNotificationReadiness({ ready: true }),
      ),
    ).toBe(false);
    expect(
      reportDeliveryProofCanSubmit(
        delivery({ status: "delivered" }),
        "final",
        finalNotificationReadiness({ ready: true }),
      ),
    ).toBe(false);
  });

  it("describes delivery proof as a handoff before diagnosis confirmation completes", () => {
    expect(reportDeliveryProofState(null, "handoff")).toMatchObject({
      purpose: "handoff",
      status: "missing",
    });
    expect(
      reportDeliveryProofState(
        delivery({
          provider_message_id: "wecom-handoff-101",
          notification_purpose: "handoff",
          status: "delivered",
        }),
        "handoff",
      ),
    ).toMatchObject({
      purpose: "handoff",
      status: "delivered",
    });
    expect(
      reportDeliveryProofState(delivery({ status: "pending" }), "handoff"),
    ).toMatchObject({
      purpose: "handoff",
      status: "pending",
    });
  });

  it("places retried delivery proof first and replaces matching rows", () => {
    const previous = delivery({
      id: 801,
      status: "failed",
      updated_at: "2026-06-18T08:00:01Z",
    });
    const older = delivery({
      id: 800,
      idempotency_key: "final_report:100/notification/handoff",
      notification_purpose: "handoff",
      status: "failed",
    });
    const retried = delivery({
      id: 801,
      provider_message_id: "msg-801",
      provider_status: "accepted",
      status: "delivered",
      updated_at: "2026-06-18T08:05:00Z",
    });

    expect(upsertReportNotificationDeliveryProof([previous, older], retried)).toEqual([
      retried,
      older,
    ]);
  });

  it("keeps locally retried delivery proof scoped to the active report", () => {
    const serverDelivery = delivery({
      id: 801,
      idempotency_key: "final_report:101/notification/final",
      provider_message_id: "msg-server-101",
      status: "failed",
    });
    const retried = delivery({
      id: 802,
      idempotency_key: "final_report:101/notification/final",
      provider_message_id: "msg-retried-101",
      status: "delivered",
    });
    const nextReportDelivery = delivery({
      id: 901,
      idempotency_key: "final_report:202/notification/final",
      provider_message_id: "msg-server-202",
      status: "pending",
    });

    const overlay = upsertReportNotificationDeliveryOverlay(null, 101, retried);

    expect(
      reportDeliveryProofCurrentDeliveries([serverDelivery], overlay, 101),
    ).toEqual([retried, serverDelivery]);
    expect(
      reportDeliveryProofCurrentDeliveries([nextReportDelivery], overlay, 202),
    ).toEqual([nextReportDelivery]);
  });

  it("selects delivery proof for the active notification purpose", () => {
    const handoff = delivery({
      id: 810,
      idempotency_key: "final_report:101/notification/handoff",
      notification_purpose: "handoff",
      provider_message_id: "wecom-handoff-101",
      status: "delivered",
    });
    const final = delivery({
      id: 811,
      idempotency_key: "final_report:101/notification/final",
      notification_purpose: "final",
      provider_message_id: "wecom-final-101",
      status: "pending",
    });

    expect(reportNotificationDeliveryForPurpose([handoff], "final")).toBeNull();
    expect(reportNotificationDeliveryForPurpose([handoff, final], "handoff")).toBe(
      handoff,
    );
    expect(reportNotificationDeliveryForPurpose([handoff, final], "final")).toBe(
      final,
    );
  });

  it("treats legacy delivery keys as handoff proof", () => {
    const legacy = delivery({
      idempotency_key: "final_report:101/notification",
      notification_purpose: "handoff",
    });

    expect(reportNotificationDeliveryPurpose(legacy)).toBe("handoff");
    expect(reportNotificationDeliveryForPurpose([legacy], "handoff")).toBe(legacy);
    expect(reportNotificationDeliveryForPurpose([legacy], "final")).toBeNull();
  });

  it("builds retry channel options from enabled report-scoped channels", () => {
    const options = reportNotificationRetryChannelOptions([
      notificationChannel({ id: 2, name: "Disabled report", enabled: false }),
      notificationChannel({ id: 3, name: "Diagnosis only", delivery_scopes: ["diagnosis_consultation"] }),
      notificationChannel({ id: 4, name: "Report webhook" }),
    ]);

    expect(options).toEqual([
      {
        detail: "",
        kind: "legacy",
        label: "",
        profileID: null,
        value: "legacy",
      },
      {
        detail: "#4 / webhook",
        kind: "profile",
        label: "Report webhook",
        profileID: 4,
        value: "4",
      },
    ]);
    expect(defaultReportNotificationRetryChannelValue(options)).toBe("4");
    expect(defaultReportNotificationRetryChannelValue(options, 4)).toBe("4");
    expect(defaultReportNotificationRetryChannelValue(options, 99)).toBe("4");
    expect(selectedReportNotificationRetryChannelID(options, "4")).toBe(4);
    expect(selectedReportNotificationRetryChannelID(options, "legacy")).toBeNull();
  });

});

function delivery(
  overrides: Partial<ReportNotificationDelivery> = {},
): ReportNotificationDelivery {
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

function finalNotificationReadiness(
  overrides: Partial<
    Extract<ReportFinalNotificationReadiness, { source: "api" }>
  > = {},
): ReportFinalNotificationReadiness {
  return {
    detail: "Checkout API latency has no operator-confirmed AI conclusion yet.",
    notification_purpose: "handoff",
    ready: false,
    source: "api",
    status: "blocked",
    status_label: "Final notification blocked",
    ...overrides,
  };
}

function notificationChannel(
  overrides: Partial<NotificationChannelProfile> = {},
): NotificationChannelProfile {
  return {
    created_at: "2026-06-18T08:00:00Z",
    delivery_scopes: ["report"],
    enabled: true,
    id: 1,
    kind: "webhook",
    labels: {},
    latest_test_results: [],
    name: "Report webhook",
    secret_ref: "secret/openclarion/report-webhook",
    updated_at: "2026-06-18T08:00:01Z",
    ...overrides,
  };
}
