import type {
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "@/features/diagnosis-room/api";
import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceRequest,
} from "@/features/diagnosis-room/types";
import {
  diagnosisFinalConclusionTraceabilityStatus,
  type DiagnosisFinalConclusionTraceabilityStatus,
} from "@/features/diagnosis-room/final-conclusion";
import {
  diagnosisRoomNextStep,
  type DiagnosisRoomNextStep,
  type DiagnosisRoomNextStepCode,
} from "@/features/diagnosis-room/next-step";
import {
  diagnosisNotificationDeliveryCoverage,
  type DiagnosisNotificationDeliveryCoverage,
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
  coverage: DiagnosisNotificationDeliveryCoverage;
  danger: boolean;
  href: string;
  kind: "failure" | "proof";
};

type AlertDiagnosisRoomPrimaryActionBase = {
  danger: boolean;
  href: string;
  iconKind: "attention" | "closed" | "review" | "room";
};

export type AlertDiagnosisRoomPrimaryAction =
  | (AlertDiagnosisRoomPrimaryActionBase & {
      kind: "review_channel";
    })
  | (AlertDiagnosisRoomPrimaryActionBase & {
      kind: "review_delivery";
      reviewAction: AlertDiagnosisDeliveryReviewAction;
    })
  | (AlertDiagnosisRoomPrimaryActionBase & {
      kind: "room_step";
      step: DiagnosisRoomNextStep;
    });

export type AlertDiagnosisEvidenceAction = {
  action: "collect" | "provide" | "review";
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
  evidenceState: "needed" | "pending" | "retained";
  initialConfidence?: string;
  latestConfidence: string;
  openEvidence: number;
  supplementalEvidence: number;
  timelineEntries: number;
};

export type AlertDiagnosisClosureSummary = {
  closeProofStatus: DiagnosisNotificationDeliveryCoveragePhaseStatus;
  confirmedBy: string;
  deliveryCoverage: DiagnosisNotificationDeliveryCoverage;
  roomClosed: boolean;
  traceability: DiagnosisFinalConclusionTraceabilityStatus;
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
    coverage,
    danger: coverage.status === "blocked",
    href: diagnosisRoomAnchorHref({
      anchorID: diagnosisNotificationTimelineAnchorID,
      evidenceSnapshotID: room.evidence_snapshot_id,
      sessionID: room.session_id,
    }),
    kind: coverage.status === "blocked" ? "failure" : "proof",
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
        href: diagnosisRoomNotificationChannelReviewHref(notification),
        iconKind: "attention",
        kind: "review_channel",
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
        href: action.href,
        iconKind: "attention",
        kind: "review_delivery",
        reviewAction: action,
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
    href: diagnosisRoomPrimaryActionHref(room, step.code),
    iconKind: diagnosisRoomPrimaryActionIconKind(step.bucket),
    kind: "room_step",
    step,
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
    evidenceState: evidenceProgressState({
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
    closeProofStatus: closePhase?.status ?? "missing",
    confirmedBy: conclusion.confirmed_by?.trim() ?? "",
    deliveryCoverage,
    roomClosed: room.room_status === "closed",
    traceability,
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
    action: "collect",
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
    action: prefix === "missing" ? "provide" : "review",
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

function evidenceProgressState(input: {
  collectedEvidence: number;
  openEvidence: number;
  supplementalEvidence: number;
}): AlertDiagnosisEvidenceProgressSummary["evidenceState"] {
  if (input.openEvidence > 0) {
    return "needed";
  }
  if (input.collectedEvidence > 0 || input.supplementalEvidence > 0) {
    return "retained";
  }
  return "pending";
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
  code: DiagnosisRoomNextStepCode,
): string {
  const evidencePlan = diagnosisRoomPrimaryEvidencePlan(room, code);
  const supplementalFollowUp =
    evidencePlan === undefined
      ? diagnosisRoomPrimarySupplementalFollowUp(room, code)
      : undefined;
  return diagnosisRoomLinkHref({
    evidencePlan,
    evidenceSnapshotID: room.evidence_snapshot_id,
    intent: diagnosisRoomPrimaryActionIntent(code),
    sessionID: room.session_id,
    supplementalFollowUp,
  });
}

function diagnosisRoomPrimaryActionIntent(
  code: DiagnosisRoomNextStepCode,
): DiagnosisRoomIntent | undefined {
  switch (code) {
    case "collect_evidence":
    case "continue_ai_review":
    case "improve_confidence":
    case "reassess_evidence":
    case "start_ai_review":
      return "confidence_review";
    case "human_review":
    case "review_ai_report":
    case "review_conclusion":
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

function diagnosisRoomPrimaryEvidencePlan(
  room: DiagnosisRoomSummary,
  code: DiagnosisRoomNextStepCode,
): DiagnosisEvidenceRequest | undefined {
  switch (code) {
    case "collect_evidence":
      return firstEvidenceRequest(room.latest_progress?.evidence_requests);
    case "human_review":
    case "improve_confidence":
    case "review_ai_report":
    case "review_conclusion":
      return firstEvidenceRequest(room.latest_conclusion?.evidence_requests);
    default:
      return undefined;
  }
}

function diagnosisRoomPrimarySupplementalFollowUp(
  room: DiagnosisRoomSummary,
  code: DiagnosisRoomNextStepCode,
): DiagnosisConsultationEvidenceRequest | undefined {
  switch (code) {
    case "collect_evidence":
      return (
        firstSupplementalFollowUp(
          room.latest_progress?.missing_evidence_requests,
        ) ??
        firstSupplementalFollowUp(
          room.latest_progress?.evidence_collection_suggestions,
        )
      );
    case "human_review":
    case "improve_confidence":
    case "review_ai_report":
    case "review_conclusion":
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
