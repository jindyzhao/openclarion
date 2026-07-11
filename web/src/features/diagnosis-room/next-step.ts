import type { DiagnosisRoomSummary } from "./api";
import {
  diagnosisNotificationDeliveryProofExpected,
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationTimelineReviewRequired,
} from "./notification-content-proof";
import { supplementalEvidenceReviewedByAssistantSequence } from "./supplemental-evidence";

export type DiagnosisRoomQueueFilter =
  | "all"
  | "attention"
  | "ready"
  | "active"
  | "closed";

type DiagnosisRoomQueueBucket = Exclude<DiagnosisRoomQueueFilter, "all">;

export type DiagnosisRoomNextStep = {
  bucket: DiagnosisRoomQueueBucket;
  color: string;
  detail: string;
  label: string;
};

export type DiagnosisRoomQueueOption = {
  count: number;
  label: string;
  value: DiagnosisRoomQueueFilter;
};

export function diagnosisRoomNextStep(
  room: DiagnosisRoomSummary,
): DiagnosisRoomNextStep {
  const failedNotification = latestFailedDiagnosisRoomNotification(room);
  if (failedNotification) {
    return {
      bucket: "attention",
      color: "error",
      detail:
        "Review the failed diagnosis-room notification before relying on downstream handoff.",
      label: "Notification failed",
    };
  }
  if (diagnosisRoomAIContentProofMissing(room)) {
    return {
      bucket: "attention",
      color: "warning",
      detail:
        "Review the diagnosis-room notification payload; AI delivery cannot be distinguished from raw alert forwarding until output digest proof is retained.",
      label: "AI proof missing",
    };
  }
  const deliveryCoverage = diagnosisNotificationDeliveryCoverage(
    room.notification_timeline ?? [],
  );
  if (
    deliveryCoverage.status === "blocked" ||
    deliveryCoverage.status === "review"
  ) {
    return {
      bucket: "attention",
      color: deliveryCoverage.color,
      detail: deliveryCoverage.detail,
      label: deliveryCoverage.label,
    };
  }
  if (
    deliveryCoverage.status === "pending" &&
    diagnosisNotificationDeliveryProofExpected(room)
  ) {
    return {
      bucket: "attention",
      color: "warning",
      detail:
        "Operator-confirmed conclusion is retained, but AI delivery proof has not started. Verify assistant update, final conclusion, and close notification proof before treating the room as delivered.",
      label: "AI delivery not started",
    };
  }
  if (room.task_status === "failed") {
    return {
      bucket: "attention",
      color: "error",
      detail:
        "Inspect the workflow failure and decide whether to restart the diagnosis room.",
      label: "Workflow failed",
    };
  }
  if (room.room_status !== "closed" && workflowVisibilityNeedsAttention(room)) {
    return {
      bucket: "attention",
      color: "error",
      detail: `Temporal reports workflow status ${room.workflow_visibility?.status ?? "unknown"}. Inspect the workflow before continuing the room.`,
      label: "Workflow unavailable",
    };
  }
  if (room.room_status === "closed" || room.task_status === "cancelled") {
    return {
      bucket: "closed",
      color: "default",
      detail: room.close_reason || "Diagnosis room is closed.",
      label: "Closed",
    };
  }
  if (!room.latest_conclusion) {
    const progress = room.latest_progress;
    if (progress) {
      if (progress.status === "failed") {
        return {
          bucket: "attention",
          color: "error",
          detail:
            progress.failure_reason ||
            "AI review failed before a final conclusion was recorded.",
          label: "AI review failed",
        };
      }
      const conclusionStatus = progress.conclusion_status?.toLowerCase() ?? "";
      if (conclusionStatus === "ready_for_review") {
        return {
          bucket: "ready",
          color: "success",
          detail:
            progress.confidence_rationale ||
            "AI review is ready for operator confirmation.",
          label: "Review AI report",
        };
      }
      if (diagnosisRoomProgressHasSupplementalEvidenceAwaitingReview(progress)) {
        return {
          bucket: "attention",
          color: "processing",
          detail:
            "Supplemental evidence has been submitted, but the latest AI turn has not retained matching review proof. Ask AI to reassess the submitted evidence before confirmation.",
          label: "Reassess evidence",
        };
      }
      if (diagnosisRoomProgressNeedsEvidence(progress)) {
        return {
          bucket: "attention",
          color: "warning",
          detail: diagnosisRoomProgressDetail(progress),
          label: "Collect evidence",
        };
      }
      return {
        bucket: "active",
        color: "processing",
        detail:
          progress.confidence_rationale ||
          "AI review is still investigating this evidence snapshot.",
        label: "AI review in progress",
      };
    }
    return room.turn_count > 0
      ? {
          bucket: "active",
          color: "processing",
          detail:
            "Continue the AI conversation or collect the requested evidence.",
          label: "Continue AI review",
        }
      : diagnosisRoomIsAutomatic(room)
        ? {
            bucket: "active",
            color: "processing",
            detail:
              "Automatic diagnosis has started from alert evidence. Wait for the first AI report or refresh the room state before sending an operator prompt.",
            label: "AI review queued",
          }
      : {
          bucket: "active",
          color: "processing",
          detail: "Send the first prompt so AI can produce a diagnosis report.",
          label: "Start AI review",
        };
  }
  if (room.latest_conclusion.requires_human_review) {
    return {
      bucket: "attention",
      color: "warning",
      detail:
        "Review the AI conclusion and add verified operator evidence if confidence is not sufficient.",
      label: "Human review",
    };
  }
  const confidence = room.latest_conclusion.confidence?.toLowerCase() ?? "";
  if (confidence === "low" || confidence === "medium") {
    return {
      bucket: "attention",
      color: "gold",
      detail: "Collect more evidence before final confirmation.",
      label: "Improve confidence",
    };
  }
  return {
    bucket: "ready",
    color: "success",
    detail: "AI produced a conclusion. Review it before closing the room.",
    label: "Review conclusion",
  };
}

export function diagnosisRoomQueueOptions(
  rooms: DiagnosisRoomSummary[],
): DiagnosisRoomQueueOption[] {
  const counts = diagnosisRoomQueueCounts(rooms);
  return [
    { count: rooms.length, label: "All", value: "all" },
    { count: counts.attention, label: "Attention", value: "attention" },
    { count: counts.ready, label: "Ready", value: "ready" },
    { count: counts.active, label: "Active", value: "active" },
    { count: counts.closed, label: "Closed", value: "closed" },
  ];
}

export function filterDiagnosisRoomsByQueue(
  rooms: DiagnosisRoomSummary[],
  filter: DiagnosisRoomQueueFilter,
): DiagnosisRoomSummary[] {
  if (filter === "all") {
    return rooms;
  }
  return rooms.filter((room) => diagnosisRoomNextStep(room).bucket === filter);
}

export function diagnosisRoomWorkflowUnavailable(
  room: DiagnosisRoomSummary,
): boolean {
  return (
    room.room_status !== "closed" && workflowVisibilityNeedsAttention(room)
  );
}

function diagnosisRoomQueueCounts(rooms: DiagnosisRoomSummary[]) {
  return rooms.reduce<Record<DiagnosisRoomQueueBucket, number>>(
    (counts, room) => {
      counts[diagnosisRoomNextStep(room).bucket] += 1;
      return counts;
    },
    { active: 0, attention: 0, closed: 0, ready: 0 },
  );
}

export function latestFailedDiagnosisRoomNotification(
  room: DiagnosisRoomSummary,
) {
  const timeline = room.notification_timeline ?? [];
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const entry = timeline[index];
    if (entry && isFailedNotificationStatus(entry.provider_status)) {
      return entry;
    }
  }
  return null;
}

function diagnosisRoomAIContentProofMissing(
  room: DiagnosisRoomSummary,
): boolean {
  return diagnosisNotificationTimelineReviewRequired(
    room.notification_timeline ?? [],
  );
}

function diagnosisRoomProgressNeedsEvidence(
  progress: NonNullable<DiagnosisRoomSummary["latest_progress"]>,
): boolean {
  const conclusionStatus = progress.conclusion_status?.toLowerCase() ?? "";
  return (
    conclusionStatus === "needs_evidence" ||
    (progress.evidence_request_count ?? 0) > 0 ||
    (progress.missing_evidence_requests?.length ?? 0) > 0 ||
    (progress.evidence_collection_suggestions?.length ?? 0) > 0
  );
}

function diagnosisRoomProgressHasSupplementalEvidenceAwaitingReview(
  progress: NonNullable<DiagnosisRoomSummary["latest_progress"]>,
): boolean {
  const records = progress.supplemental_evidence ?? [];
  if (records.length === 0) {
    return false;
  }
  const latestAssistantSequence = progress.assistant_sequence ?? 0;
  if (latestAssistantSequence <= 0) {
    return true;
  }
  return records.some(
    (record) =>
      !supplementalEvidenceReviewedByAssistantSequence(
        record,
        latestAssistantSequence,
      ),
  );
}

function diagnosisRoomProgressDetail(
  progress: NonNullable<DiagnosisRoomSummary["latest_progress"]>,
): string {
  const parts = [
    progress.evidence_request_count > 0
      ? `${progress.evidence_request_count} planned`
      : "",
    (progress.missing_evidence_requests?.length ?? 0) > 0
      ? `${progress.missing_evidence_requests?.length ?? 0} missing`
      : "",
    (progress.evidence_collection_suggestions?.length ?? 0) > 0
      ? `${progress.evidence_collection_suggestions?.length ?? 0} suggestion(s)`
      : "",
  ].filter(Boolean);
  if (parts.length > 0) {
    return `AI requested evidence: ${parts.join(", ")}.`;
  }
  return (
    progress.confidence_rationale ||
    "AI requested additional evidence before final confirmation."
  );
}

function isFailedNotificationStatus(status: string): boolean {
  switch (status.toLowerCase()) {
    case "failed":
    case "error":
      return true;
    default:
      return false;
  }
}

function diagnosisRoomIsAutomatic(room: DiagnosisRoomSummary): boolean {
  return (
    room.session_id.startsWith("diagnosis-session-auto-") ||
    room.workflow_id.startsWith("diagnosis-room-diagnosis-session-auto-")
  );
}

function workflowVisibilityNeedsAttention(room: DiagnosisRoomSummary): boolean {
  const status = room.workflow_visibility?.status?.toLowerCase() ?? "";
  switch (status) {
    case "not_found":
    case "completed":
    case "failed":
    case "canceled":
    case "cancelled":
    case "terminated":
    case "timed_out":
    case "continued_as_new":
      return true;
    default:
      return false;
  }
}
