import type { useTranslations } from "next-intl";

import type {
  DiagnosisConsultationConclusionLifecycleStatus,
  DiagnosisConsultationReassessmentStatus,
} from "./consultation-progress";
import type { DiagnosisEvidenceCollectionResult } from "./types";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

export type DiagnosisConsultationProgressTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.consultationProgress">
>;

export function localizeDiagnosisConsultationReassessment(
  status: DiagnosisConsultationReassessmentStatus,
  autoFollowUpCount: number,
  t: DiagnosisConsultationProgressTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): DiagnosisConsultationReassessmentStatus {
  switch (status.status) {
    case "not_needed":
      return {
        ...status,
        detail: t("reassessmentNotNeededDetail"),
        label: t("reassessmentNotNeeded"),
      };
    case "pending":
      return {
        ...status,
        detail: t(
          autoFollowUpCount > 0
            ? "reassessmentAutomaticPendingDetail"
            : "reassessmentPendingDetail",
        ),
        label: t("reassessmentPending"),
      };
    case "reviewed": {
      const details = [
        t("reassessmentReviewedCount", { count: status.evidenceCount }),
        status.confidence
          ? t("reassessmentConfidence", {
              confidence: localizeDiagnosisRoomStatus(
                status.confidence,
                tStatus,
              ),
            })
          : "",
        status.conclusionStatus
          ? t("reassessmentConclusionStatus", {
              status: localizeDiagnosisRoomStatus(
                status.conclusionStatus,
                tStatus,
              ),
            })
          : "",
      ].filter(Boolean);
      return {
        ...status,
        detail: details.join(" "),
        label: t("reassessmentReviewed"),
      };
    }
  }
}

export function localizeDiagnosisConsultationLifecycle(
  lifecycle: DiagnosisConsultationConclusionLifecycleStatus,
  t: DiagnosisConsultationProgressTranslator,
  notificationDetail = "",
): DiagnosisConsultationConclusionLifecycleStatus {
  switch (lifecycle.status) {
    case "delivered":
      return {
        ...lifecycle,
        detail: t("lifecycleDeliveredDetail", { notificationDetail }),
        label: t("lifecycleDelivered"),
      };
    case "delivery_blocked":
      return {
        ...lifecycle,
        detail: t("lifecycleDeliveryBlockedDetail", { notificationDetail }),
        label: t("lifecycleDeliveryBlocked"),
      };
    case "delivery_review":
      return {
        ...lifecycle,
        detail: t("lifecycleDeliveryReviewDetail", { notificationDetail }),
        label: t("lifecycleDeliveryReview"),
      };
    case "delivery_pending":
      return {
        ...lifecycle,
        detail: t("lifecycleDeliveryPendingDetail", { notificationDetail }),
        label: t("lifecycleDeliveryPending"),
      };
    case "retained":
      return {
        ...lifecycle,
        detail: t("lifecycleRetainedDetail"),
        label: t("lifecycleRetained"),
      };
    case "available":
      return {
        ...lifecycle,
        detail: t("lifecycleAvailableDetail"),
        label: t("lifecycleAvailable"),
      };
    case "ready_for_review":
      return {
        ...lifecycle,
        detail: t("lifecycleReadyDetail"),
        label: t("lifecycleReady"),
      };
    case "not_ready":
      return {
        ...lifecycle,
        detail: t("lifecycleNotReadyDetail"),
        label: t("lifecycleNotReady"),
      };
  }
}

export function localizeDiagnosisSupplementalEvidenceSummary(
  planCount: number,
  supplementalCount: number,
  t: DiagnosisConsultationProgressTranslator,
): string {
  if (planCount === 0 && supplementalCount === 0) {
    return t("supplementalNone");
  }
  return t("supplementalSummary", {
    plans: planCount,
    supplemental: supplementalCount,
  });
}

export function localizeDiagnosisCollectionProgressSummary(
  items: DiagnosisEvidenceCollectionResult[],
  t: DiagnosisConsultationProgressTranslator,
): string {
  const collected = items.filter(
    (item) => item.status.trim().toLowerCase() === "collected",
  ).length;
  const failed = items.filter(
    (item) => item.status.trim().toLowerCase() === "failed",
  ).length;
  const skipped = items.filter(
    (item) => item.status.trim().toLowerCase() === "skipped",
  ).length;
  const unsupported = items.filter(
    (item) => item.status.trim().toLowerCase() === "unsupported",
  ).length;
  return t("collectionSummary", {
    collected,
    failed,
    skipped,
    total: items.length,
    unsupported,
  });
}
