import type { useTranslations } from "next-intl";

import type { DiagnosisRoomNotificationTimelineEntry } from "./api";
import {
  isDiagnosisAIContentNotification,
  type DiagnosisNotificationContentProofDisplay,
  type DiagnosisNotificationContentProofSummary,
  type DiagnosisNotificationDeliveryCoverage,
  type DiagnosisNotificationDeliveryRecoveryHint,
} from "./notification-content-proof";

export type DiagnosisNotificationTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.notification">
>;

export function localizeDiagnosisNotificationEvent(
  eventKind: string,
  t: DiagnosisNotificationTranslator,
): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return t("eventAIUpdate");
    case "diagnosis_room.final_ready_notification_sent":
      return t("eventFinalReady");
    case "diagnosis_room.close_notification_sent":
      return t("eventClose");
    default:
      return eventKind;
  }
}

export function localizeDiagnosisNotificationContentProof(
  entry: DiagnosisRoomNotificationTimelineEntry,
  display: DiagnosisNotificationContentProofDisplay,
  t: DiagnosisNotificationTranslator,
): DiagnosisNotificationContentProofDisplay {
  if (display.hasProof) {
    const parts = [
      t("proofDigest", {
        digest: display.digestPreview ?? "",
        kind: localizeContentKind(entry.content_kind ?? "", t),
      }),
    ];
    if (display.recommendedActionCount !== undefined) {
      parts.push(t("proofActions", { count: display.recommendedActionCount }));
    }
    if (display.evidenceRequestCount !== undefined) {
      parts.push(t("proofEvidenceRequests", {
        count: display.evidenceRequestCount,
      }));
    }
    return {
      ...display,
      detail: parts.join(" / "),
      kindLabel: localizeContentKind(entry.content_kind ?? "", t),
      label: t("proofOutput"),
    };
  }
  if (isDiagnosisAIContentNotification(entry.event_kind)) {
    return {
      ...display,
      detail: t("proofMissingDetail"),
      label: t("proofMissing"),
    };
  }
  return {
    ...display,
    detail: t("proofNotRequiredDetail"),
    label: t("proofNotRequired"),
  };
}

export function localizeDiagnosisNotificationContentProofSummary(
  summary: DiagnosisNotificationContentProofSummary,
  t: DiagnosisNotificationTranslator,
): DiagnosisNotificationContentProofSummary {
  if (summary.totalAICount === 0) {
    return {
      ...summary,
      detail: t("summaryNotRequiredDetail"),
      label: t("summaryNotRequired"),
    };
  }
  if (summary.missingCount > 0) {
    return {
      ...summary,
      detail: t("summaryMissingDetail", {
        missing: summary.missingCount,
        total: summary.totalAICount,
      }),
      label: t("summaryMissing"),
    };
  }
  return {
    ...summary,
    detail: t("summaryVerifiedDetail", { count: summary.provenCount }),
    label: t("summaryVerified"),
  };
}

export function localizeDiagnosisNotificationDeliveryCoverage(
  coverage: DiagnosisNotificationDeliveryCoverage,
  t: DiagnosisNotificationTranslator,
): DiagnosisNotificationDeliveryCoverage {
  const localized = {
    ...coverage,
    phases: coverage.phases.map((phase) => ({
      ...phase,
      label: localizeCoveragePhase(phase.key, t),
    })),
  };
  switch (coverage.status) {
    case "ready":
      return {
        ...localized,
        detail: t("coverageReadyDetail"),
        label: t("coverageReady"),
      };
    case "blocked":
      return {
        ...localized,
        detail: t("coverageFailedDetail"),
        label: t("coverageFailed"),
      };
    case "pending":
      return {
        ...localized,
        detail: t("coverageNotStartedDetail"),
        label: t("coverageNotStarted"),
      };
    case "review":
      return {
        ...localized,
        detail: t("coverageIncompleteDetail", {
          ready: coverage.readyCount,
          required: coverage.requiredCount,
        }),
        label: t("coverageIncomplete"),
      };
  }
}

export function localizeDiagnosisNotificationRecoveryHint(
  coverage: DiagnosisNotificationDeliveryCoverage,
  locale: string,
  t: DiagnosisNotificationTranslator,
): DiagnosisNotificationDeliveryRecoveryHint | null {
  const failed = localizedPhases(coverage, "failed", locale, t);
  if (failed !== null) {
    return {
      actionLabel: t("recoveryFailedAction"),
      color: "error",
      detail: t("recoveryFailedDetail", { phases: failed }),
      label: t("recoveryFailed"),
    };
  }
  const unproven = localizedPhases(coverage, "unproven", locale, t);
  if (unproven !== null) {
    return {
      actionLabel: t("recoveryUnprovenAction"),
      color: "warning",
      detail: t("recoveryUnprovenDetail", { phases: unproven }),
      label: t("recoveryUnproven"),
    };
  }
  const pending = localizedPhases(coverage, "pending", locale, t);
  if (pending !== null) {
    return {
      actionLabel: t("recoveryPendingAction"),
      color: "warning",
      detail: t("recoveryPendingDetail", { phases: pending }),
      label: t("recoveryPending"),
    };
  }
  const missing = localizedPhases(coverage, "missing", locale, t);
  if (missing !== null) {
    return {
      actionLabel: t("recoveryMissingAction"),
      color: coverage.status === "pending" ? "info" : "warning",
      detail: t("recoveryMissingDetail", { phases: missing }),
      label: t(
        coverage.status === "pending" ? "recoveryNotStarted" : "recoveryMissing",
      ),
    };
  }
  if (coverage.status === "ready") {
    return {
      actionLabel: t("recoveryReadyAction"),
      color: "success",
      detail: t("recoveryReadyDetail"),
      label: t("recoveryReady"),
    };
  }
  return null;
}

function localizeContentKind(
  contentKind: string,
  t: DiagnosisNotificationTranslator,
): string {
  switch (contentKind) {
    case "assistant_message":
      return t("contentAssistantMessage");
    case "final_conclusion":
      return t("contentFinalConclusion");
    default:
      return contentKind;
  }
}

function localizeCoveragePhase(
  key: DiagnosisNotificationDeliveryCoverage["phases"][number]["key"],
  t: DiagnosisNotificationTranslator,
): string {
  switch (key) {
    case "assistant_update":
      return t("phaseAssistant");
    case "final_conclusion":
      return t("phaseFinal");
    case "close":
      return t("phaseClose");
  }
}

function localizedPhases(
  coverage: DiagnosisNotificationDeliveryCoverage,
  status: DiagnosisNotificationDeliveryCoverage["phases"][number]["status"],
  locale: string,
  t: DiagnosisNotificationTranslator,
): string | null {
  const phases = coverage.phases
    .filter((phase) => phase.status === status)
    .map((phase) => localizeCoveragePhase(phase.key, t));
  if (phases.length === 0) {
    return null;
  }
  return new Intl.ListFormat(locale, {
    style: "long",
    type: "conjunction",
  }).format(phases);
}
