import type { useTranslations } from "next-intl";

import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "@/features/diagnosis-room/status-copy";

import type {
  ReportConsultationAuditItem,
  ReportDiagnosisHandoff,
  ReportDiagnosisNextAction,
  ReportDiagnosisReadiness,
  ReportDiagnosisReviewStatus,
  ReportEvidenceFollowUp,
  ReportFinalNotificationReadiness,
  SubReportDiagnosisReadiness,
} from "./diagnosis-readiness";
import type { ReportDecisionRecord } from "./report-decision-records";
import type {
  ReportDeliveryProofState,
  ReportNotificationRetryChannelOption,
} from "./report-delivery-proof";
import type {
  ReportEvidenceCollectionResultDisplay,
  ReportEvidenceRequestDisplay,
} from "./report-evidence-display";
import type { ReportDiagnosisReviewReturnNoticeKind } from "./report-return-notice";
import type {
  ReportNotificationPurpose,
  ReportNotificationRetryResponse,
} from "./types";
import { formatDateTime } from "./format";

export type ReportDetailTranslator = ReturnType<
  typeof useTranslations<"ReportDetail">
>;

type LabelDetail = {
  detail: string;
  label: string;
};

type EvidenceTechnicalDetailInput = Pick<
  ReportEvidenceRequestDisplay,
  | "alert_source_profile_id"
  | "limit"
  | "query"
  | "step_seconds"
  | "template_id"
  | "window_seconds"
>;

const reviewStateKeys = {
  complete: "reviewState.complete",
  empty: "reviewState.empty",
  failed: "reviewState.failed",
  human_review: "reviewState.human_review",
  needs_evidence: "reviewState.needs_evidence",
  pending_diagnosis: "reviewState.pending_diagnosis",
} as const satisfies Record<ReportDiagnosisReviewStatus, string>;

const queueStateKeys = {
  complete: "queueState.complete",
  empty: "queueState.empty",
  failed: "queueState.failed",
  human_review: "queueState.human_review",
  needs_evidence: "queueState.needs_evidence",
  pending_diagnosis: "queueState.pending_diagnosis",
} as const satisfies Record<ReportDiagnosisReviewStatus, string>;

const stepStateKeys = {
  attention: "stepState.attention",
  done: "stepState.done",
  pending: "stepState.pending",
} as const;

function localizeReportReviewStatus(
  status: ReportDiagnosisReviewStatus,
  t: ReportDetailTranslator,
): string {
  return t(reviewStateKeys[status]);
}

export function localizeReportReadiness(
  readiness: ReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
): {
  queueDetail: string;
  queueLabel: string;
  statusDetail: string;
  statusLabel: string;
} {
  return {
    queueDetail: reportQueueDetail(readiness, locale, t),
    queueLabel: t(queueStateKeys[readiness.status]),
    statusDetail: reportReadinessDetail(readiness, locale, t),
    statusLabel: localizeReportReviewStatus(readiness.status, t),
  };
}

export function localizeSubReportReadiness(
  readiness: SubReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
): LabelDetail {
  const residual = residualEvidenceWork(readiness, t);
  switch (readiness.status) {
    case "complete":
      return {
        detail:
          readiness.currentCollectionSuggestions > 0
            ? t("subreportDetail.completeResidual", { residual })
            : t("subreportDetail.complete"),
        label: localizeReportReviewStatus(readiness.status, t),
      };
    case "empty":
      return {
        detail: t("subreportDetail.empty"),
        label: localizeReportReviewStatus(readiness.status, t),
      };
    case "failed":
      return {
        detail: readiness.failureReason
          ? t("subreportDetail.failedReason", {
              reason: readiness.failureReason,
            })
          : t("subreportDetail.failed"),
        label: localizeReportReviewStatus(readiness.status, t),
      };
    case "human_review":
      return {
        detail:
          readiness.currentCollectionSuggestions > 0
            ? t("subreportDetail.humanReviewResidual", { residual })
            : t("subreportDetail.humanReview"),
        label: localizeReportReviewStatus(readiness.status, t),
      };
    case "needs_evidence": {
      const work = blockingEvidenceWork(readiness, locale, t);
      return {
        detail:
          readiness.currentCollectionSuggestions > 0
            ? t("subreportDetail.needsEvidenceResidual", { residual, work })
            : t("subreportDetail.needsEvidence", { work }),
        label: localizeReportReviewStatus(readiness.status, t),
      };
    }
    case "pending_diagnosis":
      return {
        detail: t("subreportDetail.pending"),
        label: localizeReportReviewStatus(readiness.status, t),
      };
  }
}

export function localizeReportDiagnosisNextAction(
  action: ReportDiagnosisNextAction,
  locale: string,
  t: ReportDetailTranslator,
): {
  actionLabel: string;
  detail: string;
  statusLabel: string;
} {
  const readiness = localizeSubReportReadiness(action.readiness, locale, t);
  return {
    actionLabel: localizeSubReportDiagnosisAction(action, t),
    detail: readiness.detail,
    statusLabel: readiness.label,
  };
}

export function localizeSubReportDiagnosisAction(
  input: {
    hasConclusion: boolean;
    hasProgress: boolean;
    readiness: SubReportDiagnosisReadiness;
  },
  t: ReportDetailTranslator,
): string {
  switch (input.readiness.status) {
    case "failed":
      return t("reviewFailedDiagnosis");
    case "needs_evidence":
      return t("resolveEvidence");
    case "human_review":
      return t("confirmInRoom");
    case "pending_diagnosis":
      return input.hasProgress ? t("continueDiagnosis") : t("prepareDiagnosis");
    case "complete":
      return input.hasConclusion
        ? t("reviewConfirmedDiagnosis")
        : t("reviewDiagnosis");
    case "empty":
      return t("prepareDiagnosis");
  }
}

export function localizeReportDiagnosisHandoff(
  handoff: ReportDiagnosisHandoff,
  readiness: ReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
) {
  const localizedReadiness = localizeReportReadiness(readiness, locale, t);
  const statusLabelKeys = {
    complete: "handoff.stateComplete",
    empty: "handoff.stateEmpty",
    failed: "handoff.stateFailed",
    human_review: "handoff.stateHumanReview",
    needs_evidence: "handoff.stateNeedsEvidence",
    pending_diagnosis: "handoff.statePending",
  } as const satisfies Record<ReportDiagnosisReviewStatus, string>;

  return {
    statusDetail:
      readiness.blocked || readiness.canConfirm || readiness.ready > 0
        ? localizedReadiness.queueDetail
        : handoff.followUpCount > 0
          ? t("handoff.followUps", { count: handoff.followUpCount })
          : localizedReadiness.statusDetail,
    statusLabel: t(statusLabelKeys[handoff.status]),
    steps: handoff.steps.map((step) => ({
      ...localizeHandoffStep(step.key, handoff, readiness, locale, t),
      key: step.key,
      status: step.status,
      statusLabel: t(stepStateKeys[step.status]),
    })),
  };
}

export function localizeReportConsultationAuditItem(
  item: ReportConsultationAuditItem,
  locale: string,
  t: ReportDetailTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
) {
  const readiness = localizeSubReportReadiness(item.readiness, locale, t);
  return {
    statusDetail: readiness.detail,
    statusLabel: readiness.label,
    steps: item.steps.map((step) => ({
      ...localizeAuditStep(step.key, item, locale, t, tStatus),
      key: step.key,
      status: step.status,
      statusLabel: t(stepStateKeys[step.status]),
    })),
  };
}

export function localizeFinalNotificationReadiness(
  readiness: ReportFinalNotificationReadiness,
  t: ReportDetailTranslator,
): LabelDetail {
  return {
    detail: finalNotificationReadinessDetail(readiness, t),
    label:
      readiness.status === "ready"
        ? t("finalNotification.ready")
        : t("finalNotification.blocked"),
  };
}

export function localizeReportDiagnosisReviewReturnNotice(
  kind: ReportDiagnosisReviewReturnNoticeKind,
  finalNotificationReadiness: ReportFinalNotificationReadiness,
  t: ReportDetailTranslator,
): { detail: string; title: string } {
  switch (kind) {
    case "reviewed":
      return {
        detail: t("returnNotice.reviewedDetail"),
        title: t("returnNotice.reviewedTitle"),
      };
    case "confirmed_ready":
      return {
        detail: t("returnNotice.confirmedReady"),
        title: t("returnNotice.confirmedTitle"),
      };
    case "confirmed_blocked":
      return {
        detail: t("returnNotice.confirmedBlocked", {
          detail: localizeFinalNotificationReadiness(
            finalNotificationReadiness,
            t,
          ).detail,
        }),
        title: t("returnNotice.confirmedTitle"),
      };
  }
}

export function localizeReportNotificationPurpose(
  purpose: ReportNotificationPurpose,
  t: ReportDetailTranslator,
): string {
  return purpose === "final"
    ? t("delivery.purposeFinal")
    : t("delivery.purposeHandoff");
}

export function localizeReportDeliveryProofState(
  state: ReportDeliveryProofState,
  t: ReportDetailTranslator,
): {
  actionLabel: string;
  detail: string;
  statusLabel: string;
} {
  const notification = localizeReportNotificationPurpose(state.purpose, t);
  switch (state.status) {
    case "missing":
      return {
        actionLabel: t("delivery.reviewPolicies"),
        detail: t("delivery.missingDetail", { notification }),
        statusLabel: t("delivery.noProof"),
      };
    case "delivered":
      return {
        actionLabel: "",
        detail: state.delivery?.provider_message_id
          ? t("delivery.deliveredMessageDetail", {
              message: state.delivery.provider_message_id,
              notification,
            })
          : t("delivery.deliveredDetail", { notification }),
        statusLabel: t("delivery.delivered"),
      };
    case "failed":
      return {
        actionLabel: t("delivery.reviewSettings"),
        detail:
          state.delivery?.failure_reason ||
          t("delivery.failedDetail", { notification }),
        statusLabel: t("delivery.failed"),
      };
    case "pending":
      return {
        actionLabel: "",
        detail: t("delivery.pendingDetail", { notification }),
        statusLabel: t("delivery.pending"),
      };
  }
}

export function localizeReportDeliveryProofRetryLabel(
  hasDelivery: boolean,
  purpose: ReportNotificationPurpose,
  t: ReportDetailTranslator,
): string {
  return t(hasDelivery ? "delivery.retry" : "delivery.send", {
    notification: localizeReportNotificationPurpose(purpose, t),
  });
}

export function localizeReportNotificationRetrySuccessMessage(
  retryState: ReportNotificationRetryResponse["retry_state"],
  purpose: ReportNotificationPurpose,
  t: ReportDetailTranslator,
): string {
  const notification = localizeReportNotificationPurpose(purpose, t);
  switch (retryState) {
    case "already_delivered":
      return t("delivery.alreadyDelivered", { notification });
    case "already_pending":
      return t("delivery.alreadyPending", { notification });
    case "sent":
      return t("delivery.sent", { notification });
  }
}

export function localizeReportNotificationRetryChannelOption(
  option: ReportNotificationRetryChannelOption,
  t: ReportDetailTranslator,
): Pick<ReportNotificationRetryChannelOption, "detail" | "label"> {
  return option.kind === "legacy"
    ? {
        detail: t("delivery.legacyDetail"),
        label: t("delivery.legacyLabel"),
      }
    : { detail: option.detail, label: option.label };
}

export function localizeReportDecisionRecord(
  record: ReportDecisionRecord,
  locale: string,
  t: ReportDetailTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
) {
  const statusLabelKeys = {
    confirmed: "decision.confirmed",
    failed: "decision.failed",
    needs_evidence: "decision.needsEvidence",
    pending_diagnosis: "decision.pendingDiagnosis",
    recorded: "decision.recorded",
    room_closed: "decision.roomClosed",
    running: "decision.running",
  } as const satisfies Record<ReportDecisionRecord["status"], string>;
  return {
    detail: decisionRecordDetail(record, locale, t),
    notificationDetail: decisionNotificationDetail(record, locale, t),
    notificationLabel: decisionNotificationLabel(record.notificationEventKind, t),
    roomCloseDetail: decisionRoomCloseDetail(record, locale, t),
    roomStatus: record.roomLinked
      ? localizeDiagnosisRoomStatus(record.roomStatus, tStatus)
      : t("decision.notLinked"),
    statusLabel: t(statusLabelKeys[record.status]),
  };
}

function finalNotificationReadinessDetail(
  readiness: ReportFinalNotificationReadiness,
  t: ReportDetailTranslator,
): string {
  switch (readiness.reason.kind) {
    case "fallback":
      return t("finalNotification.fallback");
    case "ready":
      return t("finalNotification.readyDetail");
    case "no_linked_subreports":
      return t("finalNotification.noLinkedSubreports", {
        reportID: readiness.reason.reportID,
      });
    case "missing_evidence_snapshot":
      return t("finalNotification.missingEvidenceSnapshot", {
        subReport: finalNotificationSubReportLabel(readiness.reason, t),
      });
    case "unconfirmed_conclusion":
      return t("finalNotification.unconfirmedConclusion", {
        subReport: finalNotificationSubReportLabel(readiness.reason, t),
      });
    case "newer_diagnosis_progress":
      return t("finalNotification.newerDiagnosisProgress", {
        subReport: finalNotificationSubReportLabel(readiness.reason, t),
      });
    case "blocked":
      return t("finalNotification.blockedDetail");
  }
}

function finalNotificationSubReportLabel(
  reference: { subReportID: number; subReportTitle: string },
  t: ReportDetailTranslator,
): string {
  if (reference.subReportTitle.trim() !== "") {
    return reference.subReportTitle;
  }
  return reference.subReportID !== 0
    ? t("finalNotification.subReport", { id: reference.subReportID })
    : t("finalNotification.linkedSubReport");
}

export function localizeDecisionRecordDiagnosisAction(
  record: ReportDecisionRecord,
  t: ReportDetailTranslator,
): string {
  const keys = {
    confirmed: "reviewConfirmedDiagnosis",
    failed: "reviewFailedDiagnosis",
    needs_evidence: "resolveEvidence",
    pending_diagnosis: "continueDiagnosis",
    recorded: "confirmInRoom",
    room_closed: "reviewClosedRoom",
    running: "continueDiagnosis",
  } as const satisfies Record<ReportDecisionRecord["status"], string>;
  return t(keys[record.status]);
}

export function localizeReportEvidenceFollowUpKind(
  kind: ReportEvidenceFollowUp["kind"],
  t: ReportDetailTranslator,
): string {
  switch (kind) {
    case "collection_suggestion":
      return t("collectionSuggestion");
    case "evidence_request":
      return t("evidencePlan");
    case "missing_evidence":
      return t("missingEvidence");
  }
}

export function localizeReportEvidenceRequestDetail(
  request: ReportEvidenceRequestDisplay,
  t: ReportDetailTranslator,
): string {
  return evidenceTechnicalDetail(request, t);
}

export function localizeReportEvidenceCollectionResultDetail(
  result: ReportEvidenceCollectionResultDisplay & {
    alert_source_kind?: string;
    observed_alerts?: number;
    observed_metric_series?: number;
    reason_code?: string;
  },
  t: ReportDetailTranslator,
): string {
  return [
    result.tool,
    result.reason_code
      ? t("evidenceDetail.reason", { value: result.reason_code })
      : undefined,
    evidenceTechnicalDetail(result, t),
    result.alert_source_kind
      ? t("evidenceDetail.sourceKind", { value: result.alert_source_kind })
      : undefined,
    result.observed_alerts !== undefined
      ? t("evidenceDetail.alerts", { count: result.observed_alerts })
      : undefined,
    result.observed_metric_series !== undefined
      ? t("evidenceDetail.metricSeries", {
          count: result.observed_metric_series,
        })
      : undefined,
  ]
    .filter((part): part is string => Boolean(part))
    .join(" / ");
}

export function localizeReportConclusionSource(
  source: string | null | undefined,
  t: ReportDetailTranslator,
): string | undefined {
  const normalized = source?.trim().toLowerCase() ?? "";
  if (normalized === "") {
    return undefined;
  }
  const keys = {
    latest_assistant_turn: "conclusionSource.latest_assistant_turn",
    none: "conclusionSource.none",
  } as const;
  const key = keys[normalized as keyof typeof keys];
  return key === undefined ? source! : t(key);
}

export function localizeReportConclusionReason(
  reason: string | null | undefined,
  t: ReportDetailTranslator,
): string | undefined {
  const normalized = reason?.trim().toLowerCase() ?? "";
  if (normalized === "") {
    return undefined;
  }
  const keys = {
    assistant_marked_final: "conclusionReason.assistant_marked_final",
    assistant_marked_ready_for_review:
      "conclusionReason.assistant_marked_ready_for_review",
    room_closed_without_assistant_turn:
      "conclusionReason.room_closed_without_assistant_turn",
  } as const;
  const key = keys[normalized as keyof typeof keys];
  return key === undefined ? reason! : t(key);
}

function reportReadinessDetail(
  readiness: ReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
): string {
  const residual = residualEvidenceWork(readiness, t);
  switch (readiness.status) {
    case "complete":
      return readiness.currentCollectionSuggestions > 0
        ? t("readinessDetail.completeResidual", { residual })
        : t("readinessDetail.complete");
    case "empty":
      return t("readinessDetail.empty");
    case "failed":
      return t("readinessDetail.failed", { count: readiness.failedSubReports });
    case "human_review":
      return readiness.currentCollectionSuggestions > 0
        ? t("readinessDetail.humanReviewResidual", {
            count: readiness.ready,
            residual,
          })
        : t("readinessDetail.humanReview", { count: readiness.ready });
    case "needs_evidence": {
      const work = blockingEvidenceWork(readiness, locale, t);
      return readiness.currentCollectionSuggestions > 0
        ? t("readinessDetail.needsEvidenceResidual", { residual, work })
        : t("readinessDetail.needsEvidence", { work });
    }
    case "pending_diagnosis":
      return t("readinessDetail.pending", {
        pending: readiness.pendingSubReports,
        total: readiness.total,
      });
  }
}

function reportQueueDetail(
  readiness: ReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
): string {
  if (readiness.failedSubReports > 0) {
    return t("queueDetail.failed", { count: readiness.failedSubReports });
  }
  if (readiness.pendingSubReports > 0) {
    return t("queueDetail.pendingDiagnosis", {
      count: readiness.pendingSubReports,
    });
  }
  if (
    readiness.currentMissingEvidence +
      readiness.currentExecutableEvidenceRequests >
    0
  ) {
    return t("queueDetail.evidence", {
      work: blockingEvidenceWork(readiness, locale, t),
    });
  }
  if (readiness.ready > 0) {
    return t("queueDetail.ready", { count: readiness.ready });
  }
  if (readiness.status === "complete") {
    return t("queueDetail.complete");
  }
  if (readiness.status === "empty") {
    return t("queueDetail.empty");
  }
  if (readiness.pending > 0) {
    return t("queueDetail.pending", { count: readiness.pending });
  }
  if (readiness.done > 0) {
    return t("queueDetail.retained");
  }
  return t("queueDetail.none");
}

function blockingEvidenceWork(
  readiness: Pick<
    SubReportDiagnosisReadiness,
    "currentExecutableEvidenceRequests" | "currentMissingEvidence"
  >,
  locale: string,
  t: ReportDetailTranslator,
): string {
  const parts = [
    readiness.currentMissingEvidence > 0
      ? t("evidenceWork.missing", { count: readiness.currentMissingEvidence })
      : null,
    readiness.currentExecutableEvidenceRequests > 0
      ? t("evidenceWork.executable", {
          count: readiness.currentExecutableEvidenceRequests,
        })
      : null,
  ].filter((part): part is string => part !== null);
  return new Intl.ListFormat(locale, {
    style: "long",
    type: "conjunction",
  }).format(parts);
}

function residualEvidenceWork(
  readiness: Pick<
    SubReportDiagnosisReadiness,
    "currentCollectionSuggestions"
  >,
  t: ReportDetailTranslator,
): string {
  return t("evidenceWork.residual", {
    count: readiness.currentCollectionSuggestions,
  });
}

function localizeHandoffStep(
  key: ReportDiagnosisHandoff["steps"][number]["key"],
  handoff: ReportDiagnosisHandoff,
  readiness: ReportDiagnosisReadiness,
  locale: string,
  t: ReportDetailTranslator,
): LabelDetail {
  switch (key) {
    case "report_generation":
      return {
        detail: t("handoff.reportGeneratedDetail", {
          id: handoff.reportID,
          workflow: handoff.reportWorkflow,
        }),
        label: t("handoff.reportGenerated"),
      };
    case "evidence_snapshot":
      return {
        detail:
          handoff.evidenceSnapshotCount > 0
            ? t("handoff.evidenceSnapshotsDetail", {
                count: handoff.evidenceSnapshotCount,
              })
            : t("handoff.noEvidenceSnapshots"),
        label: t("handoff.evidenceSnapshots"),
      };
    case "ai_consultation":
      return {
        detail:
          readiness.failedSubReports > 0
            ? t("handoff.consultationFailed", {
                count: readiness.failedSubReports,
              })
            : readiness.pendingSubReports > 0
              ? t("handoff.consultationPending", {
                  count: readiness.pendingSubReports,
                })
              : readiness.reviewed > 0
                ? t("handoff.consultationDone", {
                    reviewed: readiness.reviewed,
                    total: readiness.total,
                  })
                : t("handoff.consultationEmpty"),
        label: t("handoff.aiConsultation"),
      };
    case "evidence_follow_up": {
      const blocking =
        readiness.currentMissingEvidence +
        readiness.currentExecutableEvidenceRequests;
      return {
        detail:
          blocking > 0
            ? t("handoff.evidenceBlocked", {
                work: blockingEvidenceWork(readiness, locale, t),
              })
            : readiness.currentCollectionSuggestions > 0
              ? t("handoff.evidenceResidual", {
                  residual: residualEvidenceWork(readiness, t),
                })
              : readiness.reviewed > 0
                ? t("handoff.evidenceClear")
                : t("handoff.evidenceNotStarted"),
        label: t("handoff.evidenceFollowUp"),
      };
    }
    case "operator_decision":
      return {
        detail:
          readiness.blocked
            ? reportQueueDetail(readiness, locale, t)
            : readiness.canConfirm || readiness.ready > 0
              ? t("handoff.operatorReady", { count: readiness.ready })
              : readiness.status === "complete"
                ? t("handoff.operatorComplete")
                : t("handoff.operatorWaiting"),
        label: t("handoff.operatorDecision"),
      };
  }
}

function localizeAuditStep(
  key: ReportConsultationAuditItem["steps"][number]["key"],
  item: ReportConsultationAuditItem,
  locale: string,
  t: ReportDetailTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): LabelDetail {
  const readiness = item.readiness;
  switch (key) {
    case "initial_report":
      return {
        detail: item.hasDiagnosisState
          ? t("audit.initialDone", {
              confidence: localizeDiagnosisRoomStatus(
                item.initialConfidence,
                tStatus,
              ),
              count: readiness.evidenceRequests,
            })
          : t("audit.initialPending"),
        label: t("audit.initialReport"),
      };
    case "supplemental_evidence": {
      const attached = readiness.supplementalEvidence + readiness.collectedEvidence;
      const blocking =
        readiness.currentMissingEvidence +
        readiness.currentExecutableEvidenceRequests;
      let detail: string;
      if (!item.hasDiagnosisState) {
        detail = t("audit.supplementalPending");
      } else if (readiness.status === "failed") {
        detail = t("audit.supplementalFailed");
      } else if (blocking > 0) {
        detail = t("audit.supplementalBlocked", {
          attached: attached > 0 ? attached : "none",
          work: blockingEvidenceWork(readiness, locale, t),
        });
      } else if (readiness.currentCollectionSuggestions > 0) {
        detail = t("audit.supplementalResidual", {
          attached: attached > 0 ? attached : "none",
          residual: residualEvidenceWork(readiness, t),
        });
      } else if (attached > 0) {
        detail = t("audit.supplementalRetained", { count: attached });
      } else if (readiness.evidenceRequests > 0) {
        detail = t("audit.supplementalCleared");
      } else {
        detail = t("audit.supplementalNone");
      }
      return { detail, label: t("audit.supplemental") };
    }
    case "confidence_revision": {
      const initial = localizeDiagnosisRoomStatus(item.initialConfidence, tStatus);
      const latest = localizeDiagnosisRoomStatus(readiness.latestConfidence, tStatus);
      let detail: string;
      if (!item.hasDiagnosisState) {
        detail = t("audit.confidencePending");
      } else if (readiness.status === "failed") {
        detail = t("audit.confidenceFailed");
      } else {
        switch (confidenceDirection(item.initialConfidence, readiness.latestConfidence)) {
          case "increased":
            detail = t("audit.confidenceIncreased", { initial, latest });
            break;
          case "decreased":
            detail = t("audit.confidenceDecreased", { initial, latest });
            break;
          case "same":
            detail = t("audit.confidenceSame", { latest });
            break;
          case "unknown":
            detail = t("audit.confidenceUnknown", { latest });
            break;
        }
      }
      return { detail, label: t("audit.confidenceRevision") };
    }
    case "final_decision": {
      let detail: string;
      if (!item.hasDiagnosisState) {
        detail = t("audit.finalPending");
      } else if (readiness.status === "failed") {
        detail = readiness.failureReason
          ? t("audit.finalFailedReason", { reason: readiness.failureReason })
          : t("audit.finalFailed");
      } else if (readiness.status === "needs_evidence") {
        detail = t("audit.finalEvidence");
      } else if (readiness.status === "human_review") {
        detail = t("audit.finalReview");
      } else if (item.confirmed) {
        detail = item.conclusionVersion
          ? t("audit.finalConfirmedVersion", {
              version: item.conclusionVersion,
            })
          : t("audit.finalConfirmed");
      } else {
        detail = t("audit.finalClear", { title: item.subReportTitle });
      }
      return { detail, label: t("audit.finalDecision") };
    }
  }
}

function decisionRecordDetail(
  record: ReportDecisionRecord,
  locale: string,
  t: ReportDetailTranslator,
): string {
  switch (record.status) {
    case "confirmed":
      return t("decision.confirmedDetail");
    case "room_closed":
      return t("decision.roomClosedDetail", {
        count: record.roomTurnCount,
        reason: record.roomCloseReason || t("decision.noCloseReason"),
      });
    case "recorded":
      return t("decision.recordedDetail");
    case "running":
      return t("decision.runningDetail");
    case "failed":
    case "needs_evidence":
    case "pending_diagnosis":
      return localizeSubReportReadiness(record.readiness, locale, t).detail;
  }
}

function decisionNotificationDetail(
  record: ReportDecisionRecord,
  locale: string,
  t: ReportDetailTranslator,
): string {
  if (!record.sessionID) {
    return t("decision.noSession");
  }
  if (!record.roomLinked) {
    return t("decision.noRoomProof");
  }
  if (!record.notificationEventKind) {
    return t("decision.noNotification");
  }
  return [
    record.notificationProviderStatus,
    record.notificationProviderMessageID
      ? t("decision.providerMessage", {
          message: record.notificationProviderMessageID,
        })
      : "",
    record.notificationOccurredAt
      ? t("decision.notificationRecorded", {
          time: formatDateTime(record.notificationOccurredAt, locale),
        })
      : "",
  ]
    .filter(Boolean)
    .join(" / ");
}

function decisionRoomCloseDetail(
  record: ReportDecisionRecord,
  locale: string,
  t: ReportDetailTranslator,
): string {
  if (!record.roomLinked) {
    return t("decision.noLifecycle");
  }
  if (record.roomStatus !== "closed") {
    return t("decision.roomOpen");
  }
  return t("decision.roomClosedAt", {
    reason: record.roomCloseReason || t("decision.closed"),
    time: formatDateTime(record.roomClosedAt, locale),
  });
}

function decisionNotificationLabel(
  eventKind: string,
  t: ReportDetailTranslator,
): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return t("notificationEvent.aiUpdate");
    case "diagnosis_room.final_ready_notification_sent":
      return t("notificationEvent.finalReady");
    case "diagnosis_room.close_notification_sent":
      return t("notificationEvent.close");
    case "":
      return t("notificationEvent.none");
    default:
      return eventKind;
  }
}

function evidenceTechnicalDetail(
  item: EvidenceTechnicalDetailInput,
  t: ReportDetailTranslator,
): string {
  return [
    item.query ? t("evidenceDetail.query", { value: item.query }) : undefined,
    item.template_id !== undefined
      ? t("evidenceDetail.template", { id: item.template_id })
      : undefined,
    item.alert_source_profile_id !== undefined
      ? t("evidenceDetail.sourceProfile", { id: item.alert_source_profile_id })
      : undefined,
    item.window_seconds !== undefined
      ? t("evidenceDetail.window", { seconds: item.window_seconds })
      : undefined,
    item.step_seconds !== undefined
      ? t("evidenceDetail.step", { seconds: item.step_seconds })
      : undefined,
    item.limit !== undefined
      ? t("evidenceDetail.limit", { count: item.limit })
      : undefined,
  ]
    .filter((part): part is string => part !== undefined)
    .join(" / ");
}

function confidenceDirection(initial: string, latest: string) {
  const initialRank = confidenceRank(initial);
  const latestRank = confidenceRank(latest);
  if (initialRank === undefined || latestRank === undefined) {
    return "unknown" as const;
  }
  if (latestRank > initialRank) {
    return "increased" as const;
  }
  if (latestRank < initialRank) {
    return "decreased" as const;
  }
  return "same" as const;
}

function confidenceRank(confidence: string): number | undefined {
  switch (confidence.trim().toLowerCase()) {
    case "none":
    case "pending":
      return 0;
    case "low":
      return 1;
    case "medium":
      return 2;
    case "high":
      return 3;
    default:
      return undefined;
  }
}
