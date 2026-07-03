import type { Route } from "next";

import type { NotificationChannelProfile } from "@/features/settings/notification-channels/types";

import type {
  FinalReportDetail,
  ReportNotificationDeliveryProof,
  ReportNotificationPurpose,
  ReportNotificationRetryResponse,
} from "./types";

type ReportNotificationDelivery =
  FinalReportDetail["notification_deliveries"][number];
type ReportNotificationRetryState =
  ReportNotificationRetryResponse["retry_state"];

export type ReportDeliveryProofState = {
  actionHref: Route | "";
  actionLabel: string;
  detail: string;
  status: ReportNotificationDelivery["status"] | "missing";
  statusLabel: string;
};

export type ReportNotificationRetryChannelOption = {
  detail: string;
  label: string;
  profileID: number | null;
  value: string;
};

export type ReportFinalNotificationReadiness =
  FinalReportDetail["final_notification_readiness"];

export type ReportDeliveryProofOverlay = {
  deliveries: ReportNotificationDelivery[];
  reportID: number;
};

const legacyRetryChannelValue = "legacy";

export function reportDeliveryProofState(
  delivery: ReportNotificationDelivery | null,
  purpose: ReportNotificationPurpose = "final",
): ReportDeliveryProofState {
  const notificationLabel = reportNotificationPurposeLabel(purpose);
  if (!delivery) {
    return {
      actionHref: "/settings/report-workflow-policies",
      actionLabel: "Review report workflow policies",
      detail: `No ${notificationLabel} delivery proof is retained for this report yet.`,
      status: "missing",
      statusLabel: "No delivery proof",
    };
  }
  switch (delivery.status) {
    case "delivered":
      return {
        actionHref: "",
        actionLabel: "",
        detail: delivery.provider_message_id
          ? `Latest ${notificationLabel} was delivered with provider message ${delivery.provider_message_id}.`
          : `Latest ${notificationLabel} was delivered.`,
        status: "delivered",
        statusLabel: "Delivered",
      };
    case "failed":
      return {
        actionHref: "/settings/notification-channels",
        actionLabel: "Review notification settings",
        detail: delivery.failure_reason || `Latest ${notificationLabel} delivery failed.`,
        status: "failed",
        statusLabel: "Failed",
      };
    case "pending":
      return {
        actionHref: "",
        actionLabel: "",
        detail: `${sentenceCase(notificationLabel)} delivery has been queued but no provider result is retained yet.`,
        status: "pending",
        statusLabel: "Pending",
      };
  }
}

export function reportDeliveryProofCanRetry(
  delivery: ReportNotificationDelivery | null,
): boolean {
  return !delivery || delivery.status === "failed";
}

export function reportDeliveryProofCanSubmit(
  delivery: ReportNotificationDelivery | null,
  purpose: ReportNotificationPurpose,
  finalReadiness?: ReportFinalNotificationReadiness,
): boolean {
  if (!reportDeliveryProofCanRetry(delivery)) {
    return false;
  }
  return purpose !== "final" || finalReadiness?.ready === true;
}

export function reportDeliveryProofRetryLabel(
  delivery: ReportNotificationDelivery | null,
  purpose: ReportNotificationPurpose = "final",
): string {
  const notificationLabel = reportNotificationPurposeLabel(purpose);
  return delivery ? `Retry ${notificationLabel}` : `Send ${notificationLabel}`;
}

export function reportNotificationRetrySuccessMessage(
  retryState: ReportNotificationRetryState,
  purpose: ReportNotificationPurpose,
): string {
  const notificationLabel = sentenceCase(reportNotificationPurposeLabel(purpose));
  switch (retryState) {
    case "already_delivered":
      return `${notificationLabel} was already delivered; no duplicate send was started.`;
    case "already_pending":
      return `${notificationLabel} is already pending; no duplicate send was started.`;
    case "sent":
      return `${notificationLabel} sent.`;
  }
}

export function reportNotificationDeliveryForPurpose(
  deliveries: ReportNotificationDelivery[],
  purpose: ReportNotificationPurpose,
): ReportNotificationDelivery | null {
  return (
    deliveries.find((delivery) => reportNotificationDeliveryPurpose(delivery) === purpose) ??
    null
  );
}

export function reportNotificationDeliveryPurpose(
  delivery: ReportNotificationDelivery,
): ReportNotificationPurpose {
  if (delivery.notification_purpose === "final" || delivery.notification_purpose === "handoff") {
    return delivery.notification_purpose;
  }
  if (delivery.idempotency_key.endsWith("/notification/final")) {
    return "final";
  }
  return "handoff";
}

export function upsertReportNotificationDeliveryProof(
  deliveries: ReportNotificationDelivery[],
  delivery: ReportNotificationDeliveryProof,
): ReportNotificationDelivery[] {
  return [delivery, ...deliveries.filter((item) => item.id !== delivery.id)];
}

export function reportDeliveryProofCurrentDeliveries(
  deliveries: ReportNotificationDelivery[],
  overlay: ReportDeliveryProofOverlay | null,
  reportID: number,
): ReportNotificationDelivery[] {
  if (overlay === null || overlay.reportID !== reportID) {
    return deliveries;
  }
  return overlay.deliveries.reduceRight(
    (items, delivery) => upsertReportNotificationDeliveryProof(items, delivery),
    deliveries,
  );
}

export function upsertReportNotificationDeliveryOverlay(
  overlay: ReportDeliveryProofOverlay | null,
  reportID: number,
  delivery: ReportNotificationDeliveryProof,
): ReportDeliveryProofOverlay {
  const deliveries = overlay?.reportID === reportID ? overlay.deliveries : [];
  return {
    deliveries: upsertReportNotificationDeliveryProof(deliveries, delivery),
    reportID,
  };
}

export function reportNotificationRetryChannelOptions(
  channels: NotificationChannelProfile[],
): ReportNotificationRetryChannelOption[] {
  const readyChannels = channels
    .filter((channel) => channel.enabled && channel.delivery_scopes.includes("report"))
    .map((channel) => ({
      detail: `#${channel.id} / ${channel.kind}`,
      label: channel.name,
      profileID: channel.id,
      value: String(channel.id),
    }));
  return [
    {
      detail: "Server fallback provider",
      label: "Legacy fallback",
      profileID: null,
      value: legacyRetryChannelValue,
    },
    ...readyChannels,
  ];
}

export function defaultReportNotificationRetryChannelValue(
  options: ReportNotificationRetryChannelOption[],
  preferredProfileID: number | null = null,
): string {
  if (preferredProfileID !== null) {
    const preferred = options.find((option) => option.profileID === preferredProfileID);
    if (preferred) {
      return preferred.value;
    }
  }
  return options.find((option) => option.profileID !== null)?.value ?? legacyRetryChannelValue;
}

export function selectedReportNotificationRetryChannelID(
  options: ReportNotificationRetryChannelOption[],
  selectedValue: string,
): number | null {
  return options.find((option) => option.value === selectedValue)?.profileID ?? null;
}

export function reportNotificationPurposeLabel(
  purpose: ReportNotificationPurpose,
): string {
  switch (purpose) {
    case "final":
      return "final report notification";
    case "handoff":
      return "report handoff notification";
  }
}

function sentenceCase(value: string): string {
  return value.length === 0 ? value : `${value[0]!.toUpperCase()}${value.slice(1)}`;
}
