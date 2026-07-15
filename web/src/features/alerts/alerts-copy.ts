import type { useTranslations } from "next-intl";

import type { DiagnosisRoomNotificationTimelineEntry } from "@/features/diagnosis-room/api";
import { localizeDiagnosisRoomNextStep } from "@/features/diagnosis-room/next-step-copy";
import {
  isDiagnosisAIContentNotification,
  type DiagnosisNotificationContentProofDisplay,
  type DiagnosisNotificationDeliveryCoverage,
} from "@/features/diagnosis-room/notification-content-proof";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "@/features/diagnosis-room/status-copy";

import type {
  AlertDiagnosisClosureSummary,
  AlertDiagnosisDeliveryReviewAction,
  AlertDiagnosisEvidenceAction,
  AlertDiagnosisEvidenceProgressSummary,
  AlertDiagnosisRoomPrimaryAction,
} from "./diagnosis-delivery";

export type AlertsTranslator = ReturnType<typeof useTranslations<"Alerts">>;
type DiagnosisRoomNextStepTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.nextStep">
>;

export type LocalizedAlertDiagnosisAction = {
  hint: string;
  label: string;
};

export type LocalizedDiagnosisNotificationCoverage = {
  detail: string;
  label: string;
  phases: Array<{
    key: DiagnosisNotificationDeliveryCoverage["phases"][number]["key"];
    label: string;
    status: string;
  }>;
};

export function localizeAlertDiagnosisDeliveryReviewAction(
  action: AlertDiagnosisDeliveryReviewAction,
  t: AlertsTranslator,
): LocalizedAlertDiagnosisAction {
  const coverage = localizeDiagnosisNotificationCoverage(action.coverage, t);
  return {
    hint: coverage.detail,
    label:
      action.kind === "failure"
        ? t("reviewNotificationFailure")
        : t("reviewNotificationProof"),
  };
}

export function localizeAlertDiagnosisRoomPrimaryAction(
  action: AlertDiagnosisRoomPrimaryAction,
  locale: string,
  t: AlertsTranslator,
  tNextStep: DiagnosisRoomNextStepTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): LocalizedAlertDiagnosisAction {
  switch (action.kind) {
    case "review_channel":
      return {
        hint: t("reviewFailedNotificationHint"),
        label: t("reviewChannel"),
      };
    case "review_delivery":
      return localizeAlertDiagnosisDeliveryReviewAction(
        action.reviewAction,
        t,
      );
    case "room_step": {
      const step = localizeDiagnosisRoomNextStep(
        action.step,
        locale,
        tNextStep,
        tStatus,
      );
      return {
        hint: step.detail,
        label: primaryRoomStepLabel(action.step.code, step.label, t),
      };
    }
  }
}

export function localizeAlertDiagnosisEvidenceAction(
  action: AlertDiagnosisEvidenceAction,
  t: AlertsTranslator,
): string {
  switch (action.action) {
    case "collect":
      return t("collectEvidence");
    case "provide":
      return t("provideEvidence");
    case "review":
      return t("reviewEvidence");
  }
}

export function localizeAlertDiagnosisEvidenceProgress(
  summary: AlertDiagnosisEvidenceProgressSummary,
  locale: string,
  t: AlertsTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): {
  confidenceLabel: string;
  detail: string;
  evidenceLabel: string;
} {
  const initialConfidence = summary.initialConfidence
    ? localizeDiagnosisRoomStatus(summary.initialConfidence, tStatus)
    : undefined;
  const latestConfidence = localizeDiagnosisRoomStatus(
    summary.latestConfidence,
    tStatus,
  );
  const parts = [
    countPart(t, "progressCollected", summary.collectedEvidence),
    countPart(t, "progressSupplemental", summary.supplementalEvidence),
    countPart(t, "progressOpen", summary.openEvidence),
    summary.timelineEntries > 1
      ? t("progressConfidenceUpdates", { count: summary.timelineEntries })
      : null,
  ].filter((part): part is string => part !== null);

  return {
    confidenceLabel:
      initialConfidence !== undefined &&
      summary.initialConfidence !== summary.latestConfidence
        ? t("confidenceChange", {
            initial: initialConfidence,
            latest: latestConfidence,
          })
        : latestConfidence,
    detail:
      parts.length === 0
        ? t("noRetainedEvidence")
        : t("progressDetail", {
            items: new Intl.ListFormat(locale, {
              style: "long",
              type: "conjunction",
            }).format(parts),
          }),
    evidenceLabel: t(evidenceStateKeys[summary.evidenceState]),
  };
}

export function localizeAlertDiagnosisClosure(
  summary: AlertDiagnosisClosureSummary,
  t: AlertsTranslator,
): {
  closeProofLabel: string;
  detail: string;
  label: string;
} {
  const coverage = localizeDiagnosisNotificationCoverage(
    summary.deliveryCoverage,
    t,
  );
  const trace = summary.traceability;
  const closeProofLabel = t("notificationPhaseStatus", {
    phase: t("notificationPhase.close"),
    status: localizeNotificationPhaseStatus(summary.closeProofStatus, t),
  });

  if (trace.status === "complete") {
    return {
      closeProofLabel,
      detail: t("closure.completeDetail", {
        count: trace.reviewResidualCount,
      }),
      label: t("closure.complete"),
    };
  }
  if (trace.status === "blocked") {
    return {
      closeProofLabel,
      detail: t("closure.blockedDetail", { delivery: coverage.detail }),
      label: t("closure.blocked"),
    };
  }
  if (trace.status === "pending") {
    if (summary.confirmedBy === "") {
      return {
        closeProofLabel,
        detail: t("closure.pendingDetail"),
        label: t("closure.pending"),
      };
    }
    return {
      closeProofLabel,
      detail: t("closure.deliveryPendingDetail", {
        ready: summary.deliveryCoverage.readyCount,
        required: summary.deliveryCoverage.requiredCount,
      }),
      label: t("closure.deliveryPending"),
    };
  }
  if (summary.confirmedBy === "") {
    return {
      closeProofLabel,
      detail: t("closure.needsReviewDetail", {
        count: trace.reviewOpenCount,
      }),
      label: t("closure.needsReview"),
    };
  }
  if (trace.reviewOpenCount > 0) {
    return {
      closeProofLabel,
      detail: t("closure.retainedBlockersDetail", {
        count: trace.reviewOpenCount,
      }),
      label: t("closure.retainedBlockers"),
    };
  }
  return {
    closeProofLabel,
    detail: t("closure.deliveryIncompleteDetail", {
      delivery: coverage.detail,
    }),
    label: t("closure.deliveryIncomplete"),
  };
}

export function localizeDiagnosisNotificationContentProof(
  entry: DiagnosisRoomNotificationTimelineEntry,
  proof: DiagnosisNotificationContentProofDisplay,
  locale: string,
  t: AlertsTranslator,
): Pick<DiagnosisNotificationContentProofDisplay, "detail" | "label"> {
  if (proof.hasProof) {
    const contentKind = localizedNotificationContentKind(
      entry.content_kind ?? "",
      t,
    );
    const parts = [
      t("notificationProofDigest", {
        digest: proof.digestPreview ?? "",
        kind: contentKind,
      }),
      proof.recommendedActionCount === undefined
        ? null
        : t("recommendedActionCount", {
            count: proof.recommendedActionCount,
          }),
      proof.evidenceRequestCount === undefined
        ? null
        : t("evidenceRequestCount", { count: proof.evidenceRequestCount }),
    ].filter((part): part is string => part !== null);
    return {
      detail: new Intl.ListFormat(locale, {
        style: "long",
        type: "conjunction",
      }).format(parts),
      label: t("notificationProofVerified"),
    };
  }
  if (isDiagnosisAIContentNotification(entry.event_kind)) {
    return {
      detail: t("notificationProofMissingDetail"),
      label: t("notificationProofMissing"),
    };
  }
  return {
    detail: t("notificationProofNotRequiredDetail"),
    label: t("notificationProofNotRequired"),
  };
}

export function localizeDiagnosisNotificationCoverage(
  coverage: DiagnosisNotificationDeliveryCoverage,
  t: AlertsTranslator,
): LocalizedDiagnosisNotificationCoverage {
  const copy = (() => {
    switch (coverage.status) {
      case "ready":
        return {
          detail: t("notificationCoverage.completeDetail"),
          label: t("notificationCoverage.complete"),
        };
      case "blocked":
        return {
          detail: t("notificationCoverage.failedDetail"),
          label: t("notificationCoverage.failed"),
        };
      case "pending":
        return {
          detail: t("notificationCoverage.pendingDetail"),
          label: t("notificationCoverage.pending"),
        };
      case "review":
        return {
          detail: t("notificationCoverage.incompleteDetail", {
            ready: coverage.readyCount,
            required: coverage.requiredCount,
          }),
          label: t("notificationCoverage.incomplete"),
        };
    }
  })();
  return {
    ...copy,
    phases: coverage.phases.map((phase) => ({
      key: phase.key,
      label: t(notificationPhaseKeys[phase.key]),
      status: localizeNotificationPhaseStatus(phase.status, t),
    })),
  };
}

const evidenceStateKeys = {
  needed: "evidenceState.needed",
  pending: "evidenceState.pending",
  retained: "evidenceState.retained",
} as const;

const notificationPhaseKeys = {
  assistant_update: "notificationPhase.assistant_update",
  close: "notificationPhase.close",
  final_conclusion: "notificationPhase.final_conclusion",
} as const;

function primaryRoomStepLabel(
  code: Extract<AlertDiagnosisRoomPrimaryAction, { kind: "room_step" }>["step"]["code"],
  fallback: string,
  t: AlertsTranslator,
): string {
  switch (code) {
    case "ai_review_in_progress":
    case "continue_ai_review":
      return t("openRoom");
    case "review_ai_report":
      return t("reviewReport");
    case "start_ai_review":
      return t("startReview");
    default:
      return fallback;
  }
}

function localizedNotificationContentKind(
  contentKind: string,
  t: AlertsTranslator,
): string {
  switch (contentKind) {
    case "assistant_message":
      return t("notificationContentKind.assistant_message");
    case "final_conclusion":
      return t("notificationContentKind.final_conclusion");
    default:
      return contentKind;
  }
}

function localizeNotificationPhaseStatus(
  status: DiagnosisNotificationDeliveryCoverage["phases"][number]["status"],
  t: AlertsTranslator,
): string {
  const keys = {
    delivered: "notificationPhaseStatusValue.delivered",
    failed: "notificationPhaseStatusValue.failed",
    missing: "notificationPhaseStatusValue.missing",
    pending: "notificationPhaseStatusValue.pending",
    unproven: "notificationPhaseStatusValue.unproven",
  } as const;
  return t(keys[status]);
}

function countPart(
  t: AlertsTranslator,
  key: "progressCollected" | "progressOpen" | "progressSupplemental",
  count: number,
): string | null {
  return count > 0 ? t(key, { count }) : null;
}
