import type { useTranslations } from "next-intl";

import type {
  DiagnosisFinalConclusionConfidenceProgress,
  DiagnosisFinalConclusionDisplayInput,
  DiagnosisFinalConclusionRetentionState,
  DiagnosisFinalConclusionReviewItem,
  DiagnosisFinalConclusionTraceabilityResult,
} from "./final-conclusion";
import type { DiagnosisNotificationDeliveryCoverage } from "./notification-content-proof";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

export type FinalConclusionTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.finalConclusion">
>;

type TimestampFormatter = (value: string) => string;

export function localizeFinalConclusionRetention(
  retention: DiagnosisFinalConclusionRetentionState,
  conclusion: DiagnosisFinalConclusionDisplayInput,
  t: FinalConclusionTranslator,
  formatTimestamp: TimestampFormatter,
): DiagnosisFinalConclusionRetentionState {
  switch (retention.status) {
    case "missing":
      return {
        ...retention,
        detail: t(conclusion ? "retentionUnavailableDetail" : "retentionMissingDetail"),
        label: t(conclusion ? "retentionUnavailable" : "retentionMissing"),
      };
    case "retained":
      return {
        ...retention,
        detail: conclusion?.recorded_at?.trim()
          ? t("retentionRecordedDetail", {
              time: formatTimestamp(conclusion.recorded_at),
            })
          : t("retentionRetainedDetail"),
        label: t("retentionRetained"),
      };
    case "needs_review":
      return {
        ...retention,
        detail: t("retentionNeedsReviewDetail"),
        label: t("retentionNeedsReview"),
      };
    case "ready":
      return {
        ...retention,
        detail: t("retentionReadyDetail"),
        label: t("retentionReady"),
      };
  }
}

export function localizeFinalConclusionReviewItems(
  items: DiagnosisFinalConclusionReviewItem[],
  conclusion: NonNullable<DiagnosisFinalConclusionDisplayInput> | undefined,
  t: FinalConclusionTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
  formatTimestamp: TimestampFormatter,
): DiagnosisFinalConclusionReviewItem[] {
  return items.map((item) => {
    switch (item.key) {
      case "conclusion-available":
        return {
          ...item,
          detail: t("reviewUnavailableDetail"),
          title: t("reviewUnavailable"),
        };
      case "confidence-rationale": {
        const confidence = conclusion?.confidence?.trim() ?? "";
        const rationale = conclusion?.confidence_rationale?.trim() ?? "";
        return {
          ...item,
          detail:
            rationale !== ""
              ? rationale
              : confidence === ""
                ? t("reviewConfidenceMissingDetail")
                : t("reviewConfidenceWithoutRationale", {
                    confidence: localizeDiagnosisRoomStatus(confidence, tStatus),
                  }),
          title:
            confidence === ""
              ? t("reviewConfidenceMissing")
              : t("reviewConfidence", {
                  confidence: localizeDiagnosisRoomStatus(confidence, tStatus),
                }),
        };
      }
      case "evidence-gaps": {
        const missing = conclusion?.missing_evidence_requests?.length ?? 0;
        const suggestions =
          conclusion?.evidence_collection_suggestions?.length ?? 0;
        const executable = conclusion?.evidence_requests?.length ?? 0;
        const clear = missing + suggestions + executable === 0;
        return {
          ...item,
          detail: clear
            ? t("reviewEvidenceClearDetail")
            : t("reviewEvidenceCounts", { executable, missing, suggestions }),
          title: clear
            ? t("reviewEvidenceClear")
            : missing + executable > 0
              ? t("reviewEvidenceBlocked")
              : t("reviewEvidenceResidual"),
        };
      }
      case "retention":
        return {
          ...item,
          detail: conclusion?.confirmed_by?.trim()
            ? conclusion.recorded_at?.trim()
              ? t("retentionRecordedDetail", {
                  time: formatTimestamp(conclusion.recorded_at),
                })
              : t("retentionRetainedDetail")
            : conclusion?.requires_human_review
              ? t("reviewConfirmationRequired")
              : t("reviewConfirmationReady"),
          title: conclusion?.confirmed_by?.trim()
            ? t("reviewRetention")
            : t("reviewAwaitingConfirmation"),
        };
    }
  });
}

export function localizeFinalConclusionText(
  conclusion: DiagnosisFinalConclusionDisplayInput,
  t: FinalConclusionTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  if (!conclusion) {
    return "";
  }
  const content = conclusion.content?.trim();
  if (content) {
    return content;
  }
  const reason = conclusion.reason?.trim() ?? "";
  switch (reason.toLowerCase()) {
    case "assistant_marked_final":
      return t("textReasonAssistantFinal");
    case "assistant_marked_ready_for_review":
      return t("textReasonAssistantReady");
    case "room_closed_without_assistant_turn":
      return t("textReasonRoomClosed");
  }
  if (reason !== "") {
    return reason;
  }
  const status = conclusion.status?.trim() ?? "";
  if (status === "") {
    return "";
  }
  const confidence = conclusion.confidence?.trim() ?? "";
  if (status.toLowerCase() === "available" && confidence !== "") {
    return t("textAvailableWithConfidence", {
      confidence: localizeDiagnosisRoomStatus(confidence, tStatus),
    });
  }
  return localizeDiagnosisRoomStatus(status, tStatus);
}

export function localizeFinalConclusionReviewStatus(
  status: DiagnosisFinalConclusionReviewItem["status"],
  t: FinalConclusionTranslator,
): string {
  switch (status) {
    case "attention":
      return t("reviewStatusAttention");
    case "done":
      return t("reviewStatusDone");
    case "ready":
      return t("reviewStatusReady");
    case "residual":
      return t("reviewStatusResidual");
  }
}

export function localizeFinalConclusionConfidenceProgress(
  progress: DiagnosisFinalConclusionConfidenceProgress,
  t: FinalConclusionTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): DiagnosisFinalConclusionConfidenceProgress {
  const initial = localizeDiagnosisRoomStatus(progress.initialConfidence, tStatus);
  const final = localizeDiagnosisRoomStatus(progress.finalConfidence, tStatus);
  const rationale = progress.rationale.trim();
  const rationaleSuffix =
    rationale === "" ? "" : t("confidenceRationale", { rationale });
  switch (progress.status) {
    case "improved":
      return {
        ...progress,
        detail: t("confidenceImprovedDetail", {
          final,
          initial,
          rationale: rationaleSuffix,
        }),
        label: t("confidenceImproved"),
      };
    case "declined":
      return {
        ...progress,
        detail: t("confidenceDeclinedDetail", {
          final,
          initial,
          rationale: rationaleSuffix,
        }),
        label: t("confidenceDeclined"),
      };
    case "stable":
      return {
        ...progress,
        detail: t("confidenceStableDetail", {
          final,
          rationale: rationaleSuffix,
        }),
        label: t("confidenceStable"),
      };
    case "unknown":
      if (progress.initialConfidence === "unknown" || progress.finalConfidence === "unknown") {
        return {
          ...progress,
          detail: t("confidenceUnavailableDetail"),
          label: t("confidenceUnavailable"),
        };
      }
      return {
        ...progress,
        detail: t("confidenceMovedDetail", {
          final,
          initial,
          rationale: rationaleSuffix,
        }),
        label: t("confidenceRecorded"),
      };
  }
}

export function localizeFinalConclusionTraceability(
  traceability: DiagnosisFinalConclusionTraceabilityResult,
  t: FinalConclusionTranslator,
  delivery?: Pick<
    DiagnosisNotificationDeliveryCoverage,
    "detail" | "label" | "readyCount" | "requiredCount"
  >,
): DiagnosisFinalConclusionTraceabilityResult {
  const notificationLabel = delivery?.label ?? t("deliveryNotChecked");
  switch (traceability.kind) {
    case "conclusion_pending":
      return {
        ...traceability,
        detail: t("tracePendingDetail"),
        label: t("tracePending"),
        notificationLabel,
      };
    case "conclusion_review":
      return {
        ...traceability,
        detail: t("traceNeedsReviewDetail", {
          count: traceability.reviewOpenCount,
        }),
        label: t("traceNeedsReview"),
        notificationLabel,
      };
    case "retained_blockers":
      return {
        ...traceability,
        detail: t("traceRetainedBlockedDetail", {
          count: traceability.reviewOpenCount,
        }),
        label: t("traceRetainedBlocked"),
        notificationLabel,
      };
    case "delivery_unchecked":
      return {
        ...traceability,
        detail: t("traceDeliveryPendingDetail"),
        label: t("traceDeliveryPending"),
        notificationLabel,
      };
    case "complete":
      return {
        ...traceability,
        detail: t("traceCompleteDetail", {
          residual: traceability.reviewResidualCount,
        }),
        label: t("traceComplete"),
        notificationLabel,
      };
    case "delivery_blocked":
      return {
        ...traceability,
        detail: t("traceBlockedDetail", { detail: delivery?.detail ?? "" }),
        label: t("traceBlocked"),
        notificationLabel,
      };
    case "delivery_incomplete":
      return {
        ...traceability,
        detail: t("traceIncompleteDetail", { detail: delivery?.detail ?? "" }),
        label: t("traceIncomplete"),
        notificationLabel,
      };
    case "delivery_not_started":
      return {
        ...traceability,
        detail: t("traceNotStartedDetail", {
          ready: delivery?.readyCount ?? 0,
          required: delivery?.requiredCount ?? 0,
        }),
        label: t("traceDeliveryPending"),
        notificationLabel,
      };
  }
}
