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

export type DiagnosisRoomNextStepCode =
  | "ai_proof_missing"
  | "ai_review_failed"
  | "ai_review_in_progress"
  | "ai_review_queued"
  | "closed"
  | "collect_evidence"
  | "continue_ai_review"
  | "delivery_failed"
  | "delivery_incomplete"
  | "delivery_not_started"
  | "human_review"
  | "improve_confidence"
  | "notification_failed"
  | "reassess_evidence"
  | "review_ai_report"
  | "review_conclusion"
  | "start_ai_review"
  | "workflow_failed"
  | "workflow_unavailable";

export type DiagnosisRoomNextStepDetailKey =
  | DiagnosisRoomNextStepCode
  | "collect_evidence_counts";

type DiagnosisRoomNextStepDetailValues = {
  missing?: number;
  planned?: number;
  ready?: number;
  required?: number;
  status?: string;
  suggestions?: number;
};

export type DiagnosisRoomNextStep = {
  bucket: DiagnosisRoomQueueBucket;
  code: DiagnosisRoomNextStepCode;
  color: string;
  detail: string;
  detailKey?: DiagnosisRoomNextStepDetailKey;
  detailValues?: DiagnosisRoomNextStepDetailValues;
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
      code: "notification_failed",
      color: "error",
      detail:
        "Review the failed diagnosis-room notification before relying on downstream handoff.",
      detailKey: "notification_failed",
      label: "Notification failed",
    };
  }
  if (diagnosisRoomAIContentProofMissing(room)) {
    return {
      bucket: "attention",
      code: "ai_proof_missing",
      color: "warning",
      detail:
        "Review the diagnosis-room notification payload; AI delivery cannot be distinguished from raw alert forwarding until output digest proof is retained.",
      detailKey: "ai_proof_missing",
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
    const blocked = deliveryCoverage.status === "blocked";
    return {
      bucket: "attention",
      code: blocked ? "delivery_failed" : "delivery_incomplete",
      color: deliveryCoverage.color,
      detail: deliveryCoverage.detail,
      detailKey: blocked ? "delivery_failed" : "delivery_incomplete",
      detailValues: {
        ready: deliveryCoverage.readyCount,
        required: deliveryCoverage.requiredCount,
      },
      label: deliveryCoverage.label,
    };
  }
  if (
    deliveryCoverage.status === "pending" &&
    diagnosisNotificationDeliveryProofExpected(room)
  ) {
    return {
      bucket: "attention",
      code: "delivery_not_started",
      color: "warning",
      detail:
        "Operator-confirmed conclusion is retained, but AI delivery proof has not started. Verify assistant update, final conclusion, and close notification proof before treating the room as delivered.",
      detailKey: "delivery_not_started",
      label: "AI delivery not started",
    };
  }
  if (room.task_status === "failed") {
    return {
      bucket: "attention",
      code: "workflow_failed",
      color: "error",
      detail:
        "Inspect the workflow failure and decide whether to restart the diagnosis room.",
      detailKey: "workflow_failed",
      label: "Workflow failed",
    };
  }
  if (room.room_status !== "closed" && workflowVisibilityNeedsAttention(room)) {
    return {
      bucket: "attention",
      code: "workflow_unavailable",
      color: "error",
      detail: `Temporal reports workflow status ${room.workflow_visibility?.status ?? "unknown"}. Inspect the workflow before continuing the room.`,
      detailKey: "workflow_unavailable",
      detailValues: {
        status: room.workflow_visibility?.status ?? "unknown",
      },
      label: "Workflow unavailable",
    };
  }
  if (room.room_status === "closed" || room.task_status === "cancelled") {
    return {
      bucket: "closed",
      code: "closed",
      color: "default",
      detail: room.close_reason || "Diagnosis room is closed.",
      detailKey: room.close_reason ? undefined : "closed",
      label: "Closed",
    };
  }
  if (!room.latest_conclusion) {
    const progress = room.latest_progress;
    if (progress) {
      if (progress.status === "failed") {
        return {
          bucket: "attention",
          code: "ai_review_failed",
          color: "error",
          detail:
            progress.failure_reason ||
            "AI review failed before a final conclusion was recorded.",
          detailKey: progress.failure_reason ? undefined : "ai_review_failed",
          label: "AI review failed",
        };
      }
      const conclusionStatus = progress.conclusion_status?.toLowerCase() ?? "";
      if (conclusionStatus === "ready_for_review") {
        return {
          bucket: "ready",
          code: "review_ai_report",
          color: "success",
          detail:
            progress.confidence_rationale ||
            "AI review is ready for operator confirmation.",
          detailKey: progress.confidence_rationale
            ? undefined
            : "review_ai_report",
          label: "Review AI report",
        };
      }
      if (diagnosisRoomProgressHasSupplementalEvidenceAwaitingReview(progress)) {
        return {
          bucket: "attention",
          code: "reassess_evidence",
          color: "processing",
          detail:
            "Supplemental evidence has been submitted, but the latest AI turn has not retained matching review proof. Ask AI to reassess the submitted evidence before confirmation.",
          detailKey: "reassess_evidence",
          label: "Reassess evidence",
        };
      }
      if (diagnosisRoomProgressNeedsEvidence(progress)) {
        const detail = diagnosisRoomProgressDetail(progress);
        return {
          bucket: "attention",
          code: "collect_evidence",
          color: "warning",
          detail: detail.text,
          detailKey: detail.key,
          detailValues: detail.values,
          label: "Collect evidence",
        };
      }
      return {
        bucket: "active",
        code: "ai_review_in_progress",
        color: "processing",
        detail:
          progress.confidence_rationale ||
          "AI review is still investigating this evidence snapshot.",
        detailKey: progress.confidence_rationale
          ? undefined
          : "ai_review_in_progress",
        label: "AI review in progress",
      };
    }
    return room.turn_count > 0
      ? {
          bucket: "active",
          code: "continue_ai_review",
          color: "processing",
          detail:
            "Continue the AI conversation or collect the requested evidence.",
          detailKey: "continue_ai_review",
          label: "Continue AI review",
        }
      : diagnosisRoomIsAutomatic(room)
        ? {
            bucket: "active",
            code: "ai_review_queued",
            color: "processing",
            detail:
              "Automatic diagnosis has started from alert evidence. Wait for the first AI report or refresh the room state before sending an operator prompt.",
            detailKey: "ai_review_queued",
            label: "AI review queued",
          }
      : {
          bucket: "active",
          code: "start_ai_review",
          color: "processing",
          detail: "Send the first prompt so AI can produce a diagnosis report.",
          detailKey: "start_ai_review",
          label: "Start AI review",
        };
  }
  if (room.latest_conclusion.requires_human_review) {
    return {
      bucket: "attention",
      code: "human_review",
      color: "warning",
      detail:
        "Review the AI conclusion and add verified operator evidence if confidence is not sufficient.",
      detailKey: "human_review",
      label: "Human review",
    };
  }
  const confidence = room.latest_conclusion.confidence?.toLowerCase() ?? "";
  if (confidence === "low" || confidence === "medium") {
    return {
      bucket: "attention",
      code: "improve_confidence",
      color: "gold",
      detail: "Collect more evidence before final confirmation.",
      detailKey: "improve_confidence",
      label: "Improve confidence",
    };
  }
  return {
    bucket: "ready",
    code: "review_conclusion",
    color: "success",
    detail: "AI produced a conclusion. Review it before closing the room.",
    detailKey: "review_conclusion",
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
): {
  key?: DiagnosisRoomNextStepDetailKey;
  text: string;
  values?: DiagnosisRoomNextStepDetailValues;
} {
  const planned = progress.evidence_request_count;
  const missing = progress.missing_evidence_requests?.length ?? 0;
  const suggestions = progress.evidence_collection_suggestions?.length ?? 0;
  const parts = [
    planned > 0 ? `${planned} planned` : "",
    missing > 0 ? `${missing} missing` : "",
    suggestions > 0 ? `${suggestions} suggestion(s)` : "",
  ].filter(Boolean);
  if (parts.length > 0) {
    return {
      key: "collect_evidence_counts",
      text: `AI requested evidence: ${parts.join(", ")}.`,
      values: { missing, planned, suggestions },
    };
  }
  if (progress.confidence_rationale) {
    return { text: progress.confidence_rationale };
  }
  return {
    key: "collect_evidence",
    text: "AI requested additional evidence before final confirmation.",
  };
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
