import type { Route } from "next";

import type { NotificationChannelProfile } from "@/features/settings/notification-channels/types";

import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";
import type {
  FinalReportDetail,
  ReportNotificationDeliveryProof,
  ReportNotificationPurpose,
} from "./types";

type ReportNotificationDelivery =
  FinalReportDetail["notification_deliveries"][number];
export type ReportDeliveryProofState = {
  actionHref: Route | "";
  delivery: ReportNotificationDelivery | null;
  purpose: ReportNotificationPurpose;
  status: ReportNotificationDelivery["status"] | "missing";
};

export type ReportNotificationRetryChannelOption = {
  detail: string;
  kind: "legacy" | "profile";
  label: string;
  profileID: number | null;
  value: string;
};

export type ReportDeliveryProofOverlay = {
  deliveries: ReportNotificationDelivery[];
  reportID: number;
};

const legacyRetryChannelValue = "legacy";

export function reportDeliveryProofState(
  delivery: ReportNotificationDelivery | null,
  purpose: ReportNotificationPurpose = "final",
): ReportDeliveryProofState {
  if (!delivery) {
    return {
      actionHref: "/settings/report-workflow-policies",
      delivery: null,
      purpose,
      status: "missing",
    };
  }
  switch (delivery.status) {
    case "delivered":
      return {
        actionHref: "",
        delivery,
        purpose,
        status: "delivered",
      };
    case "failed":
      return {
        actionHref: "/settings/notification-channels",
        delivery,
        purpose,
        status: "failed",
      };
    case "pending":
      return {
        actionHref: "",
        delivery,
        purpose,
        status: "pending",
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
      kind: "profile" as const,
      label: channel.name,
      profileID: channel.id,
      value: String(channel.id),
    }));
  return [
    {
      detail: "",
      kind: "legacy",
      label: "",
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
