import type {
  DiagnosisConfidenceTimelineEntry,
  DiagnosisEvidenceCollectionResult,
  DiagnosisFinalConclusion,
} from "./types";
import { diagnosisFinalConclusionRetentionState } from "./final-conclusion";
import type { DiagnosisNotificationDeliveryCoverage } from "./notification-content-proof";

export type DiagnosisConsultationReassessmentStatus = {
  conclusionStatus?: string;
  confidence?: string;
  detail: string;
  evidenceCount: number;
  label: string;
  status: "not_needed" | "pending" | "reviewed";
  turnCount?: number;
};

export type DiagnosisConsultationConclusionLifecycleStatus = {
  conclusionStatus?: string;
  confirmedBy?: string;
  detail: string;
  label: string;
  notificationStatus?: DiagnosisNotificationDeliveryCoverage["status"];
  status:
    | "not_ready"
    | "ready_for_review"
    | "available"
    | "retained"
    | "delivery_pending"
    | "delivery_review"
    | "delivery_blocked"
    | "delivered";
};

export function diagnosisConsultationReassessmentStatus({
  autoFollowUpCount,
  collectionResults,
  confidenceTimeline,
}: {
  autoFollowUpCount: number;
  collectionResults: DiagnosisEvidenceCollectionResult[];
  confidenceTimeline: DiagnosisConfidenceTimelineEntry[];
}): DiagnosisConsultationReassessmentStatus {
  const collectedCount = collectionResults.filter(
    (result) => result.status.trim().toLowerCase() === "collected",
  ).length;
  if (collectedCount === 0) {
    return {
      detail: "No collected executable evidence is retained for AI reassessment.",
      evidenceCount: 0,
      label: "AI reassessment not needed",
      status: "not_needed",
    };
  }

  const checkpoint = latestEvidenceReassessmentCheckpoint(confidenceTimeline);
  if (checkpoint !== undefined) {
    const reviewedCount =
      checkpoint.evidence_collection_results?.filter(
        (result) => result.status.trim().toLowerCase() === "collected",
      ).length || collectedCount;
    const confidence = checkpoint.confidence?.trim();
    const conclusionStatus = checkpoint.conclusion_status?.trim();
    return {
      conclusionStatus: conclusionStatus || undefined,
      confidence: confidence || undefined,
      detail: [
        `AI reviewed ${reviewedCount} collected executable evidence item${reviewedCount === 1 ? "" : "s"}.`,
        confidence ? `Latest confidence: ${confidence}.` : "",
        conclusionStatus ? `Conclusion status: ${conclusionStatus}.` : "",
      ]
        .filter(Boolean)
        .join(" "),
      evidenceCount: reviewedCount,
      label: "AI reassessed evidence",
      status: "reviewed",
      turnCount: checkpoint.turn_count,
    };
  }

  return {
    detail:
      autoFollowUpCount > 0
        ? "Collected executable evidence is retained and an automatic follow-up was generated, but no retained confidence checkpoint has reviewed that evidence yet."
        : "Collected executable evidence is retained; ask AI to reassess confidence and conclusion status before final confirmation.",
    evidenceCount: collectedCount,
    label: "AI reassessment pending",
    status: "pending",
  };
}

export function diagnosisConsultationConclusionLifecycleStatus({
  conclusionStatus,
  finalConclusion,
  notificationDelivery,
}: {
  conclusionStatus?: string;
  finalConclusion?: DiagnosisFinalConclusion;
  notificationDelivery?: Pick<
    DiagnosisNotificationDeliveryCoverage,
    "detail" | "status"
  >;
}): DiagnosisConsultationConclusionLifecycleStatus {
  const normalizedConclusionStatus = normalizeLifecycleStatus(conclusionStatus);
  const retention = diagnosisFinalConclusionRetentionState(finalConclusion);
  const confirmedBy = finalConclusion?.confirmed_by?.trim();
  if (retention.status === "retained") {
    if (notificationDelivery?.status === "ready") {
      return {
        confirmedBy,
        detail: `${retention.detail} ${notificationDelivery.detail}`,
        label: "Conclusion delivered",
        notificationStatus: notificationDelivery.status,
        status: "delivered",
      };
    }
    if (notificationDelivery?.status === "blocked") {
      return {
        confirmedBy,
        detail: `${retention.detail} ${notificationDelivery.detail}`,
        label: "Conclusion delivery blocked",
        notificationStatus: notificationDelivery.status,
        status: "delivery_blocked",
      };
    }
    if (notificationDelivery?.status === "review") {
      return {
        confirmedBy,
        detail: `${retention.detail} ${notificationDelivery.detail}`,
        label: "Conclusion delivery needs review",
        notificationStatus: notificationDelivery.status,
        status: "delivery_review",
      };
    }
    if (notificationDelivery?.status === "pending") {
      return {
        confirmedBy,
        detail: `${retention.detail} ${notificationDelivery.detail}`,
        label: "Conclusion delivery pending",
        notificationStatus: notificationDelivery.status,
        status: "delivery_pending",
      };
    }
    return withOptionalNotificationStatus(
      {
        confirmedBy,
        detail: retention.detail,
        label: retention.label,
        status: "retained",
      },
      notificationDelivery?.status,
    );
  }

  if (retention.status === "needs_review" || retention.status === "ready") {
    return withOptionalNotificationStatus(
      {
        detail: retention.detail,
        label: retention.label,
        status: "available",
      },
      notificationDelivery?.status,
    );
  }

  if (
    normalizedConclusionStatus === "ready_for_review" ||
    normalizedConclusionStatus === "final"
  ) {
    return withOptionalNotificationStatus(
      {
        conclusionStatus: normalizedConclusionStatus,
        detail:
          "AI marked the diagnosis as reviewable; retain and confirm the final conclusion before closing the loop.",
        label: "Conclusion ready for review",
        status: "ready_for_review",
      },
      notificationDelivery?.status,
    );
  }

  return withOptionalNotificationStatus(
    {
      conclusionStatus: normalizedConclusionStatus || undefined,
      detail: "AI has not produced a reviewable final conclusion yet.",
      label: "Conclusion not ready",
      status: "not_ready",
    },
    notificationDelivery?.status,
  );
}

function latestEvidenceReassessmentCheckpoint(
  confidenceTimeline: DiagnosisConfidenceTimelineEntry[],
): DiagnosisConfidenceTimelineEntry | undefined {
  return confidenceTimeline
    .slice()
    .reverse()
    .find(
      (item) =>
        (item.evidence_collection_results ?? []).some(
          (result) => result.status.trim().toLowerCase() === "collected",
        ),
    );
}

function normalizeLifecycleStatus(status: string | undefined): string {
  return status?.trim().toLowerCase() ?? "";
}

function withOptionalNotificationStatus(
  lifecycle: Omit<
    DiagnosisConsultationConclusionLifecycleStatus,
    "notificationStatus"
  >,
  notificationStatus:
    | DiagnosisConsultationConclusionLifecycleStatus["notificationStatus"]
    | undefined,
): DiagnosisConsultationConclusionLifecycleStatus {
  if (notificationStatus === undefined) {
    return lifecycle;
  }
  return {
    ...lifecycle,
    notificationStatus,
  };
}
