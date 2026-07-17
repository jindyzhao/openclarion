import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceCollectionResult,
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

export function diagnosisSupplementalEvidenceRecordsAwaitingReassessment(
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

export function diagnosisEvidenceCollectionResultsForReassessment(
  results: DiagnosisEvidenceCollectionResult[],
): DiagnosisEvidenceCollectionResult[] {
  return results
    .slice()
    .sort((left, right) => right.collected_at.localeCompare(left.collected_at))
    .slice(0, maxEvidenceCollectionReassessmentResults);
}

export function diagnosisSupplementalEvidenceExcerpt(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxSupplementalEvidenceExcerptLength) {
    return normalized;
  }
  return `${normalized
    .slice(0, maxSupplementalEvidenceExcerptLength - 3)
    .trimEnd()}...`;
}
