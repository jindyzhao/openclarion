import type {
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "@/features/diagnosis-room/api";
import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceRequest,
} from "@/features/diagnosis-room/types";
import { diagnosisFinalConclusionTraceabilityStatus } from "@/features/diagnosis-room/final-conclusion";
import { diagnosisRoomNextStep } from "@/features/diagnosis-room/next-step";
import {
  diagnosisNotificationDeliveryCoverage,
  type DiagnosisNotificationDeliveryCoveragePhaseStatus,
  diagnosisNotificationTimelineAnchorID,
} from "@/features/diagnosis-room/notification-content-proof";
import {
  diagnosisRoomAnchorHref,
  diagnosisRoomLinkHref,
  type DiagnosisRoomIntent,
} from "@/features/diagnosis-room/url-state";
import { notificationChannelEditHref } from "@/features/settings/notification-channels/format";

export type AlertDiagnosisDeliveryReviewAction = {
  danger: boolean;
  detail: string;
  href: string;
  label: string;
};

export type AlertDiagnosisRoomPrimaryAction = {
  danger: boolean;
  hint: string;
  href: string;
  iconKind: "attention" | "closed" | "review" | "room";
  label: string;
};

export type AlertDiagnosisEvidenceAction = {
  actionLabel: string;
  detail: string;
  href: string;
  key: string;
  kind: "executable" | "operator";
  label: string;
  priority?: string;
  tool?: string;
};

export type AlertDiagnosisEvidenceProgressSummary = {
  collectedEvidence: number;
  confidenceLabel: string;
  detail: string;
  evidenceLabel: string;
  initialConfidence?: string;
  latestConfidence: string;
  openEvidence: number;
  supplementalEvidence: number;
  timelineEntries: number;
};

export type AlertDiagnosisClosureSummary = {
  closeProofLabel: string;
  closeProofStatus: DiagnosisNotificationDeliveryCoveragePhaseStatus;
  color: "success" | "warning" | "error" | "default";
  confirmedBy: string;
  detail: string;
  label: string;
  notificationLabel: string;
  roomClosed: boolean;
  status: "complete" | "review" | "blocked" | "pending";
};

export function latestDiagnosisRoomNotification(
  room: DiagnosisRoomSummary,
): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  return timeline.length > 0 ? timeline[timeline.length - 1] ?? null : null;
}

export function latestFailedDiagnosisRoomNotification(
  room: DiagnosisRoomSummary,
): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const entry = timeline[index];
    if (entry && diagnosisRoomNotificationFailed(entry.provider_status)) {
      return entry;
    }
  }
  return null;
}

export function alertDiagnosisDeliveryReviewAction(
  room: DiagnosisRoomSummary,
): AlertDiagnosisDeliveryReviewAction | null {
  const timeline = room.notification_timeline ?? [];
  if (timeline.length === 0) {
    return null;
  }
  const coverage = diagnosisNotificationDeliveryCoverage(timeline);
  if (coverage.status === "ready") {
    return null;
  }
  return {
    danger: coverage.status === "blocked",
    detail: coverage.detail,
    href: diagnosisRoomAnchorHref({
      anchorID: diagnosisNotificationTimelineAnchorID,
      evidenceSnapshotID: room.evidence_snapshot_id,
      sessionID: room.session_id,
    }),
    label:
      coverage.status === "blocked"
        ? "Review notification failure"
        : "Review notification proof",
  };
}

export function alertDiagnosisRoomPrimaryAction(
  rooms: DiagnosisRoomSummary[],
): AlertDiagnosisRoomPrimaryAction | null {
  const failedNotificationRoom = rooms.find(
    (room) => latestFailedDiagnosisRoomNotification(room) !== null,
  );
  if (failedNotificationRoom) {
    const notification = latestFailedDiagnosisRoomNotification(
      failedNotificationRoom,
    );
    if (notification !== null) {
      return {
        danger: true,
        hint:
          "Review failed AI notification delivery before relying on downstream handoff.",
        href: diagnosisRoomNotificationChannelReviewHref(notification),
        iconKind: "attention",
        label: "Review channel",
      };
    }
  }

  const deliveryReviewRoom = rooms.find(
    (room) => alertDiagnosisDeliveryReviewAction(room) !== null,
  );
  if (deliveryReviewRoom) {
    const action = alertDiagnosisDeliveryReviewAction(deliveryReviewRoom);
    if (action !== null) {
      return {
        danger: action.danger,
        hint: action.detail,
        href: action.href,
        iconKind: "attention",
        label: action.label,
      };
    }
  }

  const prioritizedRoom = diagnosisRoomByNextStepPriority(rooms);
  if (prioritizedRoom === null) {
    return null;
  }
  const { room, step } = prioritizedRoom;
  return {
    danger: step.bucket === "attention" && step.color === "error",
    hint: step.detail,
    href: diagnosisRoomPrimaryActionHref(room, step.label),
    iconKind: diagnosisRoomPrimaryActionIconKind(step.bucket),
    label: diagnosisRoomPrimaryActionLabel(step.label),
  };
}

export function alertDiagnosisEvidenceActions(
  room: DiagnosisRoomSummary,
): AlertDiagnosisEvidenceAction[] {
  const source = room.latest_conclusion ?? room.latest_progress;
  if (source === undefined) {
    return [];
  }
  return [
    ...(source.evidence_requests ?? []).map((request, index) =>
      executableEvidenceAction(room, request, index),
    ),
    ...(source.missing_evidence_requests ?? []).map((request, index) =>
      operatorEvidenceAction(room, request, "missing", index),
    ),
    ...(source.evidence_collection_suggestions ?? []).map((request, index) =>
      operatorEvidenceAction(room, request, "suggestion", index),
    ),
  ];
}

export function alertDiagnosisEvidenceProgressSummary(
  room: DiagnosisRoomSummary,
): AlertDiagnosisEvidenceProgressSummary | null {
  const source = room.latest_conclusion ?? room.latest_progress;
  if (source === undefined) {
    return null;
  }
  const timeline = source.confidence_timeline ?? [];
  const firstTimeline = firstConfidenceTimelineEntry(timeline);
  const latestTimeline = latestConfidenceTimelineEntry(timeline);
  const latestConfidence =
    source.confidence ?? latestTimeline?.confidence ?? "pending";
  const initialConfidence = firstTimeline?.confidence;
  const confidenceLabel =
    initialConfidence !== undefined && initialConfidence !== latestConfidence
      ? `${initialConfidence} -> ${latestConfidence}`
      : latestConfidence;
  const plannedEvidence = sourceEvidenceRequestCount(source, latestTimeline);
  const missingEvidence =
    source.missing_evidence_requests?.length ??
    latestTimeline?.missing_evidence_requests?.length ??
    0;
  const collectionSuggestions =
    source.evidence_collection_suggestions?.length ??
    latestTimeline?.evidence_collection_suggestions?.length ??
    0;
  const openEvidence =
    plannedEvidence + missingEvidence + collectionSuggestions;
  const collectedEvidence = sourceCollectedEvidenceCount(source, timeline);
  const supplementalEvidence = source.supplemental_evidence?.length ?? 0;
  return {
    collectedEvidence,
    confidenceLabel,
    detail: evidenceProgressDetail({
      collectedEvidence,
      openEvidence,
      supplementalEvidence,
      timelineEntries: timeline.length,
    }),
    evidenceLabel: evidenceProgressLabel({
      collectedEvidence,
      openEvidence,
      supplementalEvidence,
    }),
    initialConfidence,
    latestConfidence,
    openEvidence,
    supplementalEvidence,
    timelineEntries: timeline.length,
  };
}

export function alertDiagnosisClosureSummary(
  room: DiagnosisRoomSummary,
): AlertDiagnosisClosureSummary | null {
  const conclusion = room.latest_conclusion;
  if (conclusion === undefined) {
    return null;
  }
  const deliveryCoverage = diagnosisNotificationDeliveryCoverage(
    room.notification_timeline ?? [],
  );
  const traceability = diagnosisFinalConclusionTraceabilityStatus({
    conclusion,
    notificationDelivery: deliveryCoverage,
  });
  const closePhase = deliveryCoverage.phases.find(
    (phase) => phase.key === "close",
  );
  return {
    closeProofLabel: closePhase
      ? `${closePhase.label}: ${closePhase.status}`
      : "Close: missing",
    closeProofStatus: closePhase?.status ?? "missing",
    color: traceability.color,
    confirmedBy: conclusion.confirmed_by?.trim() ?? "",
    detail: traceability.detail,
    label: traceability.label,
    notificationLabel: traceability.notificationLabel,
    roomClosed: room.room_status === "closed",
    status: traceability.status,
  };
}

export function diagnosisRoomNotificationFailed(status: string): boolean {
  switch (status.toLowerCase()) {
    case "failed":
    case "error":
      return true;
    default:
      return false;
  }
}

export function diagnosisRoomNotificationChannelReviewHref(
  notification: DiagnosisRoomNotificationTimelineEntry,
): string {
  return notification.notification_channel_profile_id === undefined
    ? "/settings/notification-channels"
    : notificationChannelEditHref(notification.notification_channel_profile_id);
}

function executableEvidenceAction(
  room: DiagnosisRoomSummary,
  request: DiagnosisEvidenceRequest,
  index: number,
): AlertDiagnosisEvidenceAction {
  return {
    actionLabel: "Collect",
    detail: request.reason,
    href: diagnosisRoomLinkHref({
      evidencePlan: request,
      evidenceSnapshotID: room.evidence_snapshot_id,
      intent: "confidence_review",
      sessionID: room.session_id,
    }),
    key: `executable:${index}:${request.tool}:${request.reason}`,
    kind: "executable",
    label: request.tool,
    tool: request.tool,
  };
}

function operatorEvidenceAction(
  room: DiagnosisRoomSummary,
  request: DiagnosisConsultationEvidenceRequest,
  prefix: "missing" | "suggestion",
  index: number,
): AlertDiagnosisEvidenceAction {
  return {
    actionLabel: prefix === "missing" ? "Provide" : "Review",
    detail: request.detail,
    href: diagnosisRoomLinkHref({
      evidenceSnapshotID: room.evidence_snapshot_id,
      intent: "confidence_review",
      sessionID: room.session_id,
      supplementalFollowUp: request,
    }),
    key: `${prefix}:${index}:${request.label}:${request.detail}`,
    kind: "operator",
    label: request.label,
    priority: request.priority,
  };
}

type DiagnosisRoomEvidenceState =
  | NonNullable<DiagnosisRoomSummary["latest_conclusion"]>
  | NonNullable<DiagnosisRoomSummary["latest_progress"]>;

type DiagnosisRoomConfidenceTimelineEntry = NonNullable<
  NonNullable<DiagnosisRoomEvidenceState["confidence_timeline"]>[number]
>;

function sourceEvidenceRequestCount(
  source: DiagnosisRoomEvidenceState,
  latestTimeline: DiagnosisRoomConfidenceTimelineEntry | undefined,
): number {
  if ("evidence_request_count" in source) {
    return source.evidence_request_count;
  }
  return (
    source.evidence_requests?.length ??
    latestTimeline?.evidence_requests?.length ??
    latestTimeline?.evidence_request_count ??
    0
  );
}

function sourceCollectedEvidenceCount(
  source: DiagnosisRoomEvidenceState,
  timeline: DiagnosisRoomConfidenceTimelineEntry[],
): number {
  const sourceResults =
    "evidence_collection_results" in source
      ? (source.evidence_collection_results ?? [])
      : [];
  const results =
    sourceResults.length > 0
      ? sourceResults
      : timeline.flatMap((item) => item.evidence_collection_results ?? []);
  return results.filter((result) => result.status === "collected").length;
}

function evidenceProgressLabel(input: {
  collectedEvidence: number;
  openEvidence: number;
  supplementalEvidence: number;
}): string {
  if (input.openEvidence > 0) {
    return "Evidence needed";
  }
  if (input.collectedEvidence > 0 || input.supplementalEvidence > 0) {
    return "Evidence retained";
  }
  return "Evidence pending";
}

function evidenceProgressDetail(input: {
  collectedEvidence: number;
  openEvidence: number;
  supplementalEvidence: number;
  timelineEntries: number;
}): string {
  const parts = [
    input.collectedEvidence > 0
      ? `${input.collectedEvidence} collected`
      : "",
    input.supplementalEvidence > 0
      ? `${input.supplementalEvidence} supplemental`
      : "",
    input.openEvidence > 0 ? `${input.openEvidence} open` : "",
    input.timelineEntries > 1
      ? `${input.timelineEntries} confidence updates`
      : "",
  ].filter(Boolean);
  if (parts.length === 0) {
    return "No collected evidence has been retained yet.";
  }
  return `${parts.join(", ")}.`;
}

function firstConfidenceTimelineEntry(
  timeline: DiagnosisRoomConfidenceTimelineEntry[],
): DiagnosisRoomConfidenceTimelineEntry | undefined {
  return timeline.reduce<DiagnosisRoomConfidenceTimelineEntry | undefined>(
    (first, item) => {
      if (first === undefined) {
        return item;
      }
      return timelineEntryOccurredAtMs(item) <= timelineEntryOccurredAtMs(first)
        ? item
        : first;
    },
    undefined,
  );
}

function latestConfidenceTimelineEntry(
  timeline: DiagnosisRoomConfidenceTimelineEntry[],
): DiagnosisRoomConfidenceTimelineEntry | undefined {
  return timeline.reduce<DiagnosisRoomConfidenceTimelineEntry | undefined>(
    (latest, item) => {
      if (latest === undefined) {
        return item;
      }
      return timelineEntryOccurredAtMs(item) >= timelineEntryOccurredAtMs(latest)
        ? item
        : latest;
    },
    undefined,
  );
}

function timelineEntryOccurredAtMs(
  item: DiagnosisRoomConfidenceTimelineEntry,
): number {
  const parsed = Date.parse(item.occurred_at);
  return Number.isFinite(parsed) ? parsed : Number.NEGATIVE_INFINITY;
}

function diagnosisRoomByNextStepPriority(rooms: DiagnosisRoomSummary[]) {
  const roomSteps = rooms.map((room) => ({
    room,
    step: diagnosisRoomNextStep(room),
  }));
  return (
    roomSteps.find(({ step }) => step.bucket === "attention") ??
    roomSteps.find(({ step }) => step.bucket === "ready") ??
    roomSteps.find(({ step }) => step.bucket === "active") ??
    roomSteps.find(({ step }) => step.bucket === "closed") ??
    null
  );
}

function diagnosisRoomPrimaryActionHref(
  room: DiagnosisRoomSummary,
  label: string,
): string {
  const evidencePlan = diagnosisRoomPrimaryEvidencePlan(room, label);
  const supplementalFollowUp =
    evidencePlan === undefined
      ? diagnosisRoomPrimarySupplementalFollowUp(room, label)
      : undefined;
  return diagnosisRoomLinkHref({
    evidencePlan,
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: diagnosisRoomPrimaryActionIntent(label),
    sessionID: room.session_id,
    supplementalFollowUp,
  });
}

function diagnosisRoomPrimaryActionIntent(
  label: string,
): DiagnosisRoomIntent | undefined {
  switch (label) {
    case "Collect evidence":
    case "Continue AI review":
    case "Improve confidence":
    case "Start AI review":
      return "confidence_review";
    case "Human review":
    case "Review AI report":
    case "Review conclusion":
      return "review_conclusion";
    default:
      return undefined;
  }
}

function diagnosisRoomPrimaryActionIconKind(
  bucket: ReturnType<typeof diagnosisRoomNextStep>["bucket"],
): AlertDiagnosisRoomPrimaryAction["iconKind"] {
  switch (bucket) {
    case "attention":
      return "attention";
    case "ready":
      return "review";
    case "closed":
      return "closed";
    case "active":
      return "room";
  }
}

function diagnosisRoomPrimaryActionLabel(label: string): string {
  switch (label) {
    case "AI review in progress":
    case "Continue AI review":
      return "Open room";
    case "Review AI report":
      return "Review report";
    case "Start AI review":
      return "Start review";
    default:
      return label;
  }
}

function diagnosisRoomPrimaryEvidencePlan(
  room: DiagnosisRoomSummary,
  label: string,
): DiagnosisEvidenceRequest | undefined {
  switch (label) {
    case "Collect evidence":
      return firstEvidenceRequest(room.latest_progress?.evidence_requests);
    case "Human review":
    case "Improve confidence":
    case "Review AI report":
    case "Review conclusion":
      return firstEvidenceRequest(room.latest_conclusion?.evidence_requests);
    default:
      return undefined;
  }
}

function diagnosisRoomPrimarySupplementalFollowUp(
  room: DiagnosisRoomSummary,
  label: string,
): DiagnosisConsultationEvidenceRequest | undefined {
  switch (label) {
    case "Collect evidence":
      return (
        firstSupplementalFollowUp(
          room.latest_progress?.missing_evidence_requests,
        ) ??
        firstSupplementalFollowUp(
          room.latest_progress?.evidence_collection_suggestions,
        )
      );
    case "Human review":
    case "Improve confidence":
    case "Review AI report":
    case "Review conclusion":
      return (
        firstSupplementalFollowUp(
          room.latest_conclusion?.missing_evidence_requests,
        ) ??
        firstSupplementalFollowUp(
          room.latest_conclusion?.evidence_collection_suggestions,
        )
      );
    default:
      return undefined;
  }
}

function firstEvidenceRequest(
  requests: readonly DiagnosisEvidenceRequest[] | undefined,
): DiagnosisEvidenceRequest | undefined {
  return requests?.[0];
}

function firstSupplementalFollowUp(
  requests: readonly DiagnosisConsultationEvidenceRequest[] | undefined,
): DiagnosisConsultationEvidenceRequest | undefined {
  return requests?.[0];
}
