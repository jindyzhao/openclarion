import type { useTranslations } from "next-intl";

import type {
  DiagnosisRoomNextStep,
  DiagnosisRoomNextStepCode,
  DiagnosisRoomNextStepDetailKey,
} from "./next-step";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

type NextStepTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.nextStep">
>;
type StaticDetailKey = Exclude<
  DiagnosisRoomNextStepDetailKey,
  "collect_evidence_counts" | "delivery_incomplete" | "workflow_unavailable"
>;

const labelKeys = {
  ai_proof_missing: "labels.ai_proof_missing",
  ai_review_failed: "labels.ai_review_failed",
  ai_review_in_progress: "labels.ai_review_in_progress",
  ai_review_queued: "labels.ai_review_queued",
  closed: "labels.closed",
  collect_evidence: "labels.collect_evidence",
  continue_ai_review: "labels.continue_ai_review",
  delivery_failed: "labels.delivery_failed",
  delivery_incomplete: "labels.delivery_incomplete",
  delivery_not_started: "labels.delivery_not_started",
  human_review: "labels.human_review",
  improve_confidence: "labels.improve_confidence",
  notification_failed: "labels.notification_failed",
  reassess_evidence: "labels.reassess_evidence",
  review_ai_report: "labels.review_ai_report",
  review_conclusion: "labels.review_conclusion",
  start_ai_review: "labels.start_ai_review",
  workflow_failed: "labels.workflow_failed",
  workflow_unavailable: "labels.workflow_unavailable",
} as const satisfies Record<DiagnosisRoomNextStepCode, string>;

const staticDetailKeys = {
  ai_proof_missing: "details.ai_proof_missing",
  ai_review_failed: "details.ai_review_failed",
  ai_review_in_progress: "details.ai_review_in_progress",
  ai_review_queued: "details.ai_review_queued",
  closed: "details.closed",
  collect_evidence: "details.collect_evidence",
  continue_ai_review: "details.continue_ai_review",
  delivery_failed: "details.delivery_failed",
  delivery_not_started: "details.delivery_not_started",
  human_review: "details.human_review",
  improve_confidence: "details.improve_confidence",
  notification_failed: "details.notification_failed",
  reassess_evidence: "details.reassess_evidence",
  review_ai_report: "details.review_ai_report",
  review_conclusion: "details.review_conclusion",
  start_ai_review: "details.start_ai_review",
  workflow_failed: "details.workflow_failed",
} as const satisfies Record<StaticDetailKey, string>;

export function localizeDiagnosisRoomNextStep(
  step: DiagnosisRoomNextStep,
  locale: string,
  t: NextStepTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): Pick<DiagnosisRoomNextStep, "detail" | "label"> {
  return {
    detail: localizedDetail(step, locale, t, tStatus),
    label: t(labelKeys[step.code]),
  };
}

function localizedDetail(
  step: DiagnosisRoomNextStep,
  locale: string,
  t: NextStepTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  if (step.detailKey === undefined) {
    return step.detail;
  }
  if (step.detailKey === "delivery_incomplete") {
    return t("details.delivery_incomplete", {
      ready: step.detailValues?.ready ?? 0,
      required: step.detailValues?.required ?? 0,
    });
  }
  if (step.detailKey === "workflow_unavailable") {
    return t("details.workflow_unavailable", {
      status: localizeDiagnosisRoomStatus(
        step.detailValues?.status ?? "unknown",
        tStatus,
      ),
    });
  }
  if (step.detailKey === "collect_evidence_counts") {
    const parts = [
      countPart(t, "counts.planned", step.detailValues?.planned ?? 0),
      countPart(t, "counts.missing", step.detailValues?.missing ?? 0),
      countPart(t, "counts.suggestions", step.detailValues?.suggestions ?? 0),
    ].filter((part): part is string => part !== null);
    return t("details.collect_evidence_counts", {
      items: new Intl.ListFormat(locale, {
        style: "long",
        type: "conjunction",
      }).format(parts),
    });
  }
  return t(staticDetailKeys[step.detailKey]);
}

function countPart(
  t: NextStepTranslator,
  key: "counts.missing" | "counts.planned" | "counts.suggestions",
  count: number,
): string | null {
  return count > 0 ? t(key, { count }) : null;
}
