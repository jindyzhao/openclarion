import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisSupplementalEvidenceRecord,
} from "./types";

export type DiagnosisSupplementalEvidenceWirePayload = Pick<
  DiagnosisConsultationEvidenceRequest,
  "detail" | "label" | "priority"
> & {
  evidence: string;
};

export type DiagnosisSupplementalEvidencePriority = "high" | "medium" | "low";

export type DiagnosisSupplementalEvidenceReviewProof = {
  assistant_sequence?: number;
};

export type DiagnosisSupplementalEvidenceReassessmentInput = {
  collectionResults?: DiagnosisEvidenceCollectionResult[];
  latestAssistantSequence?: number;
  records?: DiagnosisSupplementalEvidenceRecord[];
};

const maxEvidenceCollectionReassessmentResults = 3;
const maxSupplementalEvidenceReassessmentRecords = 3;
const maxSupplementalEvidenceExcerptLength = 360;

export function supplementalEvidencePriorityFromText(
  value: string,
): DiagnosisSupplementalEvidencePriority | undefined {
  switch (value.trim().toLowerCase()) {
    case "high":
      return "high";
    case "medium":
      return "medium";
    case "low":
      return "low";
    default:
      return undefined;
  }
}

export function supplementalEvidenceWirePayload(
  request: DiagnosisConsultationEvidenceRequest,
  evidence: string,
): DiagnosisSupplementalEvidenceWirePayload {
  return {
    detail: request.detail,
    evidence: evidence.trim(),
    label: request.label,
    priority: request.priority,
  };
}

export function supplementalEvidenceReviewedByAssistantSequence(
  record: DiagnosisSupplementalEvidenceReviewProof,
  latestAssistantSequence: number | undefined,
): boolean {
  return (
    latestAssistantSequence !== undefined &&
    latestAssistantSequence > 0 &&
    record.assistant_sequence === latestAssistantSequence
  );
}

export function supplementalEvidenceResidualBoundaryTemplate(
  request: DiagnosisConsultationEvidenceRequest,
): string {
  return [
    `Operator reviewed the requested follow-up for ${request.label}.`,
    `Requested artifact: ${request.detail}`,
    "The requested artifact is not available in this validation window.",
    "Operator accepts this as residual uncertainty for review purposes.",
    "Do not fabricate unavailable facts and do not repeat this same artifact request unless a new executable evidence path is available.",
  ].join("\n");
}

export function supplementalEvidenceSubmissionMessage(
  request: DiagnosisConsultationEvidenceRequest,
  evidence: string,
): string {
  return [
    "Supplemental evidence update",
    "",
    `Request: ${request.label}`,
    `Priority: ${request.priority}`,
    `Requested detail: ${request.detail}`,
    ...diagnosisEvidenceRequestLines(request.source_request),
    "",
    "Evidence provided:",
    evidence,
    "",
    "Review instruction:",
    ...supplementalEvidenceReassessmentInstructionLines(),
  ].join("\n");
}

export function supplementalEvidenceReassessmentMessage(
  input: DiagnosisSupplementalEvidenceReassessmentInput = {},
): string {
  const pendingRecords = supplementalEvidenceRecordsAwaitingReassessment(
    input.records ?? [],
    input.latestAssistantSequence,
  );
  const collectionResults = evidenceCollectionResultsForReassessment(
    input.collectionResults ?? [],
  );
  return [
    "Reassess retained diagnosis evidence",
    "",
    "Review retained supplemental evidence and executable collection results against the latest evidence requests and missing evidence requests.",
    "For each retained evidence item, state whether it satisfies, partially satisfies, or fails the matching request.",
    "Update confidence, confidence rationale, remaining evidence gaps, and conclusion status from the retained evidence.",
    "Explain whether confidence improved, stayed the same, or declined, and tie that change to the retained evidence.",
    "Treat collected executable evidence as verified input. Treat failed, skipped, or unsupported executable evidence as unresolved evidence gaps unless other verified evidence resolves them.",
    "Return updated structured diagnosis fields: confidence, confidence_rationale, missing_evidence_requests, evidence_requests, evidence_collection_suggestions, conclusion_status, and requires_human_review.",
    "Do not repeat an evidence request that has been satisfied or explicitly marked unavailable unless a materially different gap remains.",
    "When the remaining evidence is bounded but operator confirmation is still required, produce ready_for_review with requires_human_review=true.",
    ...supplementalEvidenceReassessmentRecordLines(pendingRecords),
    ...evidenceCollectionReassessmentResultLines(collectionResults),
  ].join("\n");
}

function supplementalEvidenceRecordsAwaitingReassessment(
  records: DiagnosisSupplementalEvidenceRecord[],
  latestAssistantSequence: number | undefined,
): DiagnosisSupplementalEvidenceRecord[] {
  return records
    .filter(
      (record) =>
        !supplementalEvidenceReviewedByAssistantSequence(
          record,
          latestAssistantSequence,
        ),
    )
    .slice()
    .sort((left, right) => right.provided_at.localeCompare(left.provided_at))
    .slice(0, maxSupplementalEvidenceReassessmentRecords);
}

function supplementalEvidenceReassessmentRecordLines(
  records: DiagnosisSupplementalEvidenceRecord[],
): string[] {
  if (records.length === 0) {
    return [];
  }
  return [
    "",
    "Submitted evidence awaiting reassessment:",
    ...records.flatMap((record, index) => [
      `${index + 1}. ${record.label} (${record.priority})`,
      `Requested detail: ${record.detail}`,
      `Submitted at: ${record.provided_at}`,
      `Evidence excerpt: ${supplementalEvidenceExcerpt(record.evidence)}`,
    ]),
  ];
}

function evidenceCollectionResultsForReassessment(
  results: DiagnosisEvidenceCollectionResult[],
): DiagnosisEvidenceCollectionResult[] {
  return results
    .slice()
    .sort((left, right) => right.collected_at.localeCompare(left.collected_at))
    .slice(0, maxEvidenceCollectionReassessmentResults);
}

function evidenceCollectionReassessmentResultLines(
  results: DiagnosisEvidenceCollectionResult[],
): string[] {
  if (results.length === 0) {
    return [];
  }
  return [
    "",
    "Executable evidence collection retained for reassessment:",
    ...results.flatMap((result, index) => [
      `${index + 1}. ${result.tool} (${result.status})`,
      `Reason: ${result.request.reason}`,
      `Collected at: ${result.collected_at}`,
      `Message: ${supplementalEvidenceExcerpt(result.message)}`,
      ...diagnosisEvidenceRequestLines(result.request),
    ]),
  ];
}

function supplementalEvidenceExcerpt(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxSupplementalEvidenceExcerptLength) {
    return normalized;
  }
  return `${normalized
    .slice(0, maxSupplementalEvidenceExcerptLength - 3)
    .trimEnd()}...`;
}

function supplementalEvidenceReassessmentInstructionLines(): string[] {
  return [
    "1. Decide whether the submitted evidence satisfies the requested evidence item.",
    "2. State the updated confidence and explain whether confidence improved, stayed the same, or declined.",
    "3. Update missing_evidence_requests, evidence_requests, and evidence_collection_suggestions so only materially different remaining gaps are listed.",
    "4. Produce a bounded ready_for_review conclusion with requires_human_review=true when no additional executable evidence is needed.",
    "5. Keep final reserved for conclusions without unresolved evidence, and never fabricate unavailable facts.",
  ];
}

function diagnosisEvidenceRequestLines(
  request: DiagnosisEvidenceRequest | undefined,
): string[] {
  if (request === undefined) {
    return [];
  }
  const lines = [
    "",
    "Original executable evidence request:",
    `Tool: ${request.tool}`,
    `Reason: ${request.reason}`,
  ];
  if (request.query) {
    lines.push(`Query: ${request.query}`);
  }
  if (request.template_id) {
    lines.push(`Template: ${request.template_id}`);
  }
  if (request.alert_source_profile_id) {
    lines.push(`Alert source profile: ${request.alert_source_profile_id}`);
  }
  if (request.window_seconds) {
    lines.push(`Window: ${request.window_seconds}s`);
  }
  if (request.step_seconds) {
    lines.push(`Step: ${request.step_seconds}s`);
  }
  if (request.limit) {
    lines.push(`Limit: ${request.limit}`);
  }
  return lines;
}
