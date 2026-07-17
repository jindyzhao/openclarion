import type { useTranslations } from "next-intl";

import {
  diagnosisEvidenceCollectionResultsForReassessment,
  diagnosisSupplementalEvidenceExcerpt,
  diagnosisSupplementalEvidenceRecordsAwaitingReassessment,
  type DiagnosisSupplementalEvidenceReassessmentInput,
} from "./supplemental-evidence";
import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceRequest,
} from "./types";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

export type DiagnosisSupplementalEvidenceTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.supplementalEvidencePrompt">
>;

export function localizeSupplementalEvidenceResidualBoundaryTemplate(
  request: DiagnosisConsultationEvidenceRequest,
  t: DiagnosisSupplementalEvidenceTranslator,
): string {
  return [
    t("residualReviewed", { label: request.label }),
    t("requestedArtifact", { detail: request.detail }),
    t("residualUnavailable"),
    t("residualAccepted"),
    t("residualGuardrail"),
  ].join("\n");
}

export function localizeSupplementalEvidenceSubmissionMessage(
  request: DiagnosisConsultationEvidenceRequest,
  evidence: string,
  t: DiagnosisSupplementalEvidenceTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  return [
    t("submissionTitle"),
    "",
    t("request", { label: request.label }),
    t("priority", {
      priority: localizeDiagnosisRoomStatus(request.priority, tStatus),
    }),
    t("requestedDetail", { detail: request.detail }),
    ...localizedEvidenceRequestLines(request.source_request, t),
    "",
    t("evidenceProvided"),
    evidence,
    "",
    t("reviewInstruction"),
    ...reassessmentInstructionLines(t),
  ].join("\n");
}

export function localizeSupplementalEvidenceReassessmentMessage(
  input: DiagnosisSupplementalEvidenceReassessmentInput,
  t: DiagnosisSupplementalEvidenceTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  const records = diagnosisSupplementalEvidenceRecordsAwaitingReassessment(
    input.records ?? [],
    input.latestAssistantSequence,
  );
  const collectionResults =
    diagnosisEvidenceCollectionResultsForReassessment(
      input.collectionResults ?? [],
    );
  const lines = [
    t("reassessmentTitle"),
    "",
    t("reassessmentReviewRetained"),
    t("reassessmentClassify"),
    t("reassessmentUpdate"),
    t("reassessmentExplainConfidence"),
    t("reassessmentTrustBoundary"),
    t("reassessmentStructuredFields"),
    t("reassessmentNoRepeat"),
    t("reassessmentReadyBoundary"),
  ];
  if (records.length > 0) {
    lines.push("", t("submittedAwaiting"));
    records.forEach((record, index) => {
      lines.push(
        t("submittedEntry", {
          index: index + 1,
          label: record.label,
          priority: localizeDiagnosisRoomStatus(record.priority, tStatus),
        }),
        t("requestedDetail", { detail: record.detail }),
        t("submittedAt", { time: record.provided_at }),
        t("evidenceExcerpt", {
          evidence: diagnosisSupplementalEvidenceExcerpt(record.evidence),
        }),
      );
    });
  }
  if (collectionResults.length > 0) {
    lines.push("", t("collectionRetained"));
    collectionResults.forEach((result, index) => {
      lines.push(
        t("collectionEntry", {
          index: index + 1,
          status: localizeDiagnosisRoomStatus(result.status, tStatus),
          tool: result.tool,
        }),
        t("reason", { reason: result.request.reason }),
        t("collectedAt", { time: result.collected_at }),
        t("message", {
          message: diagnosisSupplementalEvidenceExcerpt(result.message),
        }),
        ...localizedEvidenceRequestLines(result.request, t),
      );
    });
  }
  return lines.join("\n");
}

function reassessmentInstructionLines(
  t: DiagnosisSupplementalEvidenceTranslator,
): string[] {
  return [
    t("instructionSatisfaction"),
    t("instructionConfidence"),
    t("instructionGaps"),
    t("instructionReady"),
    t("instructionFinal"),
  ];
}

function localizedEvidenceRequestLines(
  request: DiagnosisEvidenceRequest | undefined,
  t: DiagnosisSupplementalEvidenceTranslator,
): string[] {
  if (request === undefined) {
    return [];
  }
  const lines = [
    "",
    t("originalRequest"),
    t("tool", { tool: request.tool }),
    t("reason", { reason: request.reason }),
  ];
  if (request.query) {
    lines.push(t("query", { query: request.query }));
  }
  if (request.template_id) {
    lines.push(t("template", { id: request.template_id }));
  }
  if (request.alert_source_profile_id) {
    lines.push(t("source", { id: request.alert_source_profile_id }));
  }
  if (request.window_seconds) {
    lines.push(t("window", { seconds: request.window_seconds }));
  }
  if (request.step_seconds) {
    lines.push(t("step", { seconds: request.step_seconds }));
  }
  if (request.limit) {
    lines.push(t("limit", { limit: request.limit }));
  }
  return lines;
}
