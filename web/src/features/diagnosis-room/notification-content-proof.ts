import type {
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "./api";

const contentSHA256Pattern = /^[a-f0-9]{64}$/;
export const diagnosisNotificationTimelineAnchorID =
  "diagnosis-notification-timeline";
export const diagnosisNotificationTimelineHref = `#${diagnosisNotificationTimelineAnchorID}`;
const diagnosisAIContentNotificationEventKinds = [
  "diagnosis_room.assistant_turn_notification_sent",
  "diagnosis_room.final_ready_notification_sent",
  "diagnosis_room.close_notification_sent",
] as const;
const diagnosisAIContentNotificationEventKindSet = new Set<string>(
  diagnosisAIContentNotificationEventKinds,
);
const notificationSuccessStatuses = new Set([
  "accepted",
  "delivered",
  "sent",
  "success",
]);
const notificationFailureStatuses = new Set(["error", "failed"]);

export type DiagnosisNotificationContentProofDisplay = {
  color: "success" | "warning" | "default";
  detail: string;
  digestPreview?: string;
  evidenceRequestCount?: number;
  hasProof: boolean;
  kindLabel?: string;
  label: string;
  recommendedActionCount?: number;
};

export type DiagnosisNotificationContentProofSummary = {
  color: "success" | "warning" | "default";
  detail: string;
  label: string;
  missingCount: number;
  provenCount: number;
  totalAICount: number;
};

export type DiagnosisNotificationDeliveryCoveragePhaseStatus =
  | "delivered"
  | "failed"
  | "missing"
  | "pending"
  | "unproven";

type DiagnosisNotificationDeliveryCoveragePhase = {
  detail: string;
  eventKind: string;
  key: "assistant_update" | "final_conclusion" | "close";
  label: string;
  status: DiagnosisNotificationDeliveryCoveragePhaseStatus;
};

export type DiagnosisNotificationDeliveryCoverage = {
  color: "success" | "warning" | "error" | "default";
  detail: string;
  label: string;
  phases: DiagnosisNotificationDeliveryCoveragePhase[];
  readyCount: number;
  requiredCount: number;
  status: "ready" | "review" | "blocked" | "pending";
};

export type DiagnosisNotificationDeliveryRecoveryHint = {
  actionLabel: string;
  color: "success" | "warning" | "error" | "info";
  detail: string;
  label: string;
};

export function diagnosisNotificationContentProofDisplay(
  entry: DiagnosisRoomNotificationTimelineEntry,
): DiagnosisNotificationContentProofDisplay {
  const contentKind = entry.content_kind;
  const contentSHA256 = entry.content_sha256?.trim();
  if (
    contentKind !== undefined &&
    contentSHA256 !== undefined &&
    contentSHA256Pattern.test(contentSHA256)
  ) {
    const kindLabel = notificationContentKindLabel(contentKind);
    const digestPreview = contentSHA256.slice(0, 12);
    return {
      color: "success",
      detail: notificationContentProofDetail({
        digestPreview,
        evidenceRequestCount: entry.evidence_request_count,
        kindLabel,
        recommendedActionCount: entry.recommended_action_count,
      }),
      digestPreview,
      evidenceRequestCount: entry.evidence_request_count,
      hasProof: true,
      kindLabel,
      label: "AI output proof",
      recommendedActionCount: entry.recommended_action_count,
    };
  }

  if (isDiagnosisAIContentNotification(entry.event_kind)) {
    return {
      color: "warning",
      detail:
        "Delivery status is present, but the notification is missing AI content proof.",
      hasProof: false,
      label: "AI proof missing",
    };
  }

  return {
    color: "default",
    detail: "This notification does not require AI content proof.",
    hasProof: false,
    label: "No AI proof",
  };
}

export function diagnosisNotificationContentProofSummary(
  entries: DiagnosisRoomNotificationTimelineEntry[],
): DiagnosisNotificationContentProofSummary {
  const displays = entries
    .filter((entry) => isDiagnosisAIContentNotification(entry.event_kind))
    .map(diagnosisNotificationContentProofDisplay);
  const totalAICount = displays.length;
  if (totalAICount === 0) {
    return {
      color: "default",
      detail:
        "No assistant, final-ready, or close AI notification has been retained in this room timeline.",
      label: "AI proof not required",
      missingCount: 0,
      provenCount: 0,
      totalAICount,
    };
  }
  const provenCount = displays.filter((display) => display.hasProof).length;
  const missingCount = totalAICount - provenCount;
  if (missingCount > 0) {
    return {
      color: "warning",
      detail: `${missingCount} of ${totalAICount} AI notification(s) are missing AI output digest proof.`,
      label: "AI proof missing",
      missingCount,
      provenCount,
      totalAICount,
    };
  }
  return {
    color: "success",
    detail: `${provenCount} AI notification(s) include output digest proof, so the timeline can distinguish AI diagnosis delivery from raw alert forwarding.`,
    label: "AI proof verified",
    missingCount,
    provenCount,
    totalAICount,
  };
}

export function diagnosisNotificationTimelineReviewRequired(
  entries: DiagnosisRoomNotificationTimelineEntry[],
): boolean {
  return diagnosisNotificationContentProofSummary(entries).missingCount > 0;
}

export function diagnosisNotificationContentProofRetryRequired(
  entry: DiagnosisRoomNotificationTimelineEntry,
): boolean {
  return (
    isDiagnosisAIContentNotification(entry.event_kind) &&
    !diagnosisNotificationContentProofDisplay(entry).hasProof
  );
}

export function diagnosisNotificationContentProofRetryEntry(
  entries: DiagnosisRoomNotificationTimelineEntry[],
): DiagnosisRoomNotificationTimelineEntry | null {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (
      entry !== undefined &&
      diagnosisNotificationContentProofRetryRequired(entry)
    ) {
      return entry;
    }
  }
  return null;
}

export function isDiagnosisAIContentNotification(eventKind: string): boolean {
  return diagnosisAIContentNotificationEventKindSet.has(eventKind);
}

export function diagnosisNotificationDeliveryCoverage(
  entries: DiagnosisRoomNotificationTimelineEntry[],
): DiagnosisNotificationDeliveryCoverage {
  const phases = notificationCoveragePhaseDefinitions.map((phase) =>
    diagnosisNotificationDeliveryCoveragePhase(entries, phase),
  );
  const requiredCount = phases.length;
  const readyCount = phases.filter((phase) => phase.status === "delivered")
    .length;
  if (readyCount === requiredCount) {
    return {
      color: "success",
      detail:
        "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
      label: "AI delivery complete",
      phases,
      readyCount,
      requiredCount,
      status: "ready",
    };
  }
  if (phases.some((phase) => phase.status === "failed")) {
    return {
      color: "error",
      detail:
        "At least one required AI notification phase failed. Retry the failed delivery before treating the room as delivered.",
      label: "AI delivery failed",
      phases,
      readyCount,
      requiredCount,
      status: "blocked",
    };
  }
  if (phases.every((phase) => phase.status === "missing")) {
    return {
      color: "default",
      detail:
        "No AI diagnosis notification phases have been retained yet.",
      label: "AI delivery not started",
      phases,
      readyCount,
      requiredCount,
      status: "pending",
    };
  }
  return {
    color: "warning",
    detail: `${readyCount} of ${requiredCount} AI notification phases are complete. Retain assistant update, final conclusion, and close notification proof before accepting delivery.`,
    label: "AI delivery incomplete",
    phases,
    readyCount,
    requiredCount,
    status: "review",
  };
}

export function diagnosisNotificationDeliveryRecoveryHint(
  coverage: DiagnosisNotificationDeliveryCoverage,
): DiagnosisNotificationDeliveryRecoveryHint | null {
  const failedPhases = deliveryCoveragePhasesByStatus(coverage, "failed");
  if (failedPhases.length > 0) {
    return {
      actionLabel: "Retry failed delivery",
      color: "error",
      detail: `${phaseList(failedPhases)} failed provider delivery. Retry the retained event from the timeline or room list before accepting downstream delivery.`,
      label: "Delivery retry required",
    };
  }

  const unprovenPhases = deliveryCoveragePhasesByStatus(coverage, "unproven");
  if (unprovenPhases.length > 0) {
    return {
      actionLabel: "Retry proof",
      color: "warning",
      detail: `${phaseList(unprovenPhases)} delivered without AI output digest proof. Re-send the retained AI notification so the timeline proves this was AI diagnosis content, not raw alert forwarding.`,
      label: "AI proof retry required",
    };
  }

  const pendingPhases = deliveryCoveragePhasesByStatus(coverage, "pending");
  if (pendingPhases.length > 0) {
    return {
      actionLabel: "Refresh proof",
      color: "warning",
      detail: `${phaseList(pendingPhases)} ${phaseVerb(pendingPhases)} retained event(s) but no successful provider delivery yet. Refresh proof after the provider callback or inspect the notification worker if it remains pending.`,
      label: "Delivery pending",
    };
  }

  const missingPhases = deliveryCoveragePhasesByStatus(coverage, "missing");
  if (missingPhases.length > 0) {
    return {
      actionLabel: "Review trigger chain",
      color: coverage.status === "pending" ? "info" : "warning",
      detail: `${phaseList(missingPhases)} ${phaseVerb(missingPhases)} no retained notification event, so there is no individual event to retry yet. Verify the close/final-ready/assistant notification trigger path and refresh proof after the backend records a delivery event.`,
      label:
        coverage.status === "pending"
          ? "Delivery not started"
          : "Delivery phase missing",
    };
  }

  if (coverage.status === "ready") {
    return {
      actionLabel: "Review proof",
      color: "success",
      detail:
        "All required AI notification phases are delivered with retained output digest proof.",
      label: "Delivery proof complete",
    };
  }

  return null;
}

export function diagnosisNotificationDeliveryProofExpected(
  room: Pick<DiagnosisRoomSummary, "latest_conclusion" | "room_status">,
): boolean {
  return (
    room.room_status === "closed" &&
    (room.latest_conclusion?.confirmed_by?.trim() ?? "") !== ""
  );
}

export function diagnosisNotificationTimelineReviewActionRequired(
  room: Pick<
    DiagnosisRoomSummary,
    "latest_conclusion" | "notification_timeline" | "room_status"
  >,
  failedNotification: DiagnosisRoomNotificationTimelineEntry | null,
  deliveryCoverage: DiagnosisNotificationDeliveryCoverage =
    diagnosisNotificationDeliveryCoverage(room.notification_timeline ?? []),
): boolean {
  if (failedNotification !== null) {
    return false;
  }
  return (
    diagnosisNotificationTimelineReviewRequired(
      room.notification_timeline ?? [],
    ) ||
    deliveryCoverage.status === "blocked" ||
    deliveryCoverage.status === "review" ||
    (deliveryCoverage.status === "pending" &&
      diagnosisNotificationDeliveryProofExpected(room))
  );
}

function notificationContentKindLabel(contentKind: string): string {
  switch (contentKind) {
    case "assistant_message":
      return "assistant message";
    case "final_conclusion":
      return "final conclusion";
    default:
      return contentKind;
  }
}

const notificationCoveragePhaseDefinitions = [
  {
    eventKind: "diagnosis_room.assistant_turn_notification_sent",
    key: "assistant_update",
    label: "AI update",
  },
  {
    eventKind: "diagnosis_room.final_ready_notification_sent",
    key: "final_conclusion",
    label: "Final conclusion",
  },
  {
    eventKind: "diagnosis_room.close_notification_sent",
    key: "close",
    label: "Close",
  },
] as const;

function diagnosisNotificationDeliveryCoveragePhase(
  entries: DiagnosisRoomNotificationTimelineEntry[],
  phase: (typeof notificationCoveragePhaseDefinitions)[number],
): DiagnosisNotificationDeliveryCoveragePhase {
  const candidates = entries.filter(
    (entry) => entry.event_kind === phase.eventKind,
  );
  if (candidates.length === 0) {
    return {
      detail: `${phase.label} notification has not been retained.`,
      eventKind: phase.eventKind,
      key: phase.key,
      label: phase.label,
      status: "missing",
    };
  }

  const provenDelivered = candidates.find(
    (entry) =>
      notificationProviderStatusSucceeded(entry.provider_status) &&
      diagnosisNotificationContentProofDisplay(entry).hasProof,
  );
  if (provenDelivered !== undefined) {
    return {
      detail: `${phase.label} notification was delivered with retained AI output proof.`,
      eventKind: phase.eventKind,
      key: phase.key,
      label: phase.label,
      status: "delivered",
    };
  }

  const failed = candidates.find((entry) =>
    notificationProviderStatusFailed(entry.provider_status),
  );
  if (failed !== undefined) {
    return {
      detail: `${phase.label} notification failed provider delivery.`,
      eventKind: phase.eventKind,
      key: phase.key,
      label: phase.label,
      status: "failed",
    };
  }

  const unprovenDelivered = candidates.find((entry) =>
    notificationProviderStatusSucceeded(entry.provider_status),
  );
  if (unprovenDelivered !== undefined) {
    return {
      detail: `${phase.label} notification was delivered but is missing AI output proof.`,
      eventKind: phase.eventKind,
      key: phase.key,
      label: phase.label,
      status: "unproven",
    };
  }

  return {
    detail: `${phase.label} notification is retained but not delivered yet.`,
    eventKind: phase.eventKind,
    key: phase.key,
    label: phase.label,
    status: "pending",
  };
}

function deliveryCoveragePhasesByStatus(
  coverage: DiagnosisNotificationDeliveryCoverage,
  status: DiagnosisNotificationDeliveryCoveragePhaseStatus,
) {
  return coverage.phases.filter((phase) => phase.status === status);
}

function phaseList(
  phases: Pick<DiagnosisNotificationDeliveryCoveragePhase, "label">[],
): string {
  return phases.map((phase) => phase.label).join(", ");
}

function phaseVerb(
  phases: Pick<DiagnosisNotificationDeliveryCoveragePhase, "label">[],
): "has" | "have" {
  return phases.length === 1 ? "has" : "have";
}

export function diagnosisNotificationDeliveryCoveragePhaseColor(
  status: DiagnosisNotificationDeliveryCoveragePhaseStatus,
): "success" | "warning" | "error" | "default" {
  switch (status) {
    case "delivered":
      return "success";
    case "failed":
      return "error";
    case "pending":
    case "unproven":
      return "warning";
    case "missing":
      return "default";
  }
}

function notificationProviderStatusSucceeded(status: string): boolean {
  return notificationSuccessStatuses.has(status.trim().toLowerCase());
}

function notificationProviderStatusFailed(status: string): boolean {
  return notificationFailureStatuses.has(status.trim().toLowerCase());
}

function notificationContentProofDetail({
  digestPreview,
  evidenceRequestCount,
  kindLabel,
  recommendedActionCount,
}: {
  digestPreview: string;
  evidenceRequestCount?: number;
  kindLabel: string;
  recommendedActionCount?: number;
}): string {
  const parts = [`AI ${kindLabel} digest ${digestPreview}`];
  if (recommendedActionCount !== undefined) {
    parts.push(`${recommendedActionCount} action(s)`);
  }
  if (evidenceRequestCount !== undefined) {
    parts.push(`${evidenceRequestCount} evidence request(s)`);
  }
  return parts.join(" / ");
}
