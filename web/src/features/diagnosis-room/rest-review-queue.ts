import type { DiagnosisRoomSummary } from "./api";
import type { DiagnosisReviewQueueInput } from "./review-queue";
import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisSupplementalEvidenceRecord,
} from "./types";

type DiagnosisRoomProgressSummary = NonNullable<
  DiagnosisRoomSummary["latest_progress"]
>;
type DiagnosisRoomConclusionSummary = NonNullable<
  DiagnosisRoomSummary["latest_conclusion"]
>;
type DiagnosisRoomEvidenceRequestSummary = NonNullable<
  DiagnosisRoomProgressSummary["evidence_requests"]
>[number];
type DiagnosisRoomEvidenceCollectionResultSummary = NonNullable<
  DiagnosisRoomProgressSummary["evidence_collection_results"]
>[number];
type DiagnosisRoomConsultationEvidenceRequestSummary = NonNullable<
  DiagnosisRoomProgressSummary["missing_evidence_requests"]
>[number];
type DiagnosisRoomSupplementalEvidenceSummary = NonNullable<
  DiagnosisRoomProgressSummary["supplemental_evidence"]
>[number];
type DiagnosisRoomConfidenceTimelineEntry = NonNullable<
  DiagnosisRoomProgressSummary["confidence_timeline"]
>[number];

export function diagnosisRoomSummaryReviewQueueInput({
  canConfirmConclusion = false,
  room,
}: {
  canConfirmConclusion?: boolean;
  room: DiagnosisRoomSummary | null | undefined;
}): DiagnosisReviewQueueInput | null {
  if (!room) {
    return null;
  }
  if (room.latest_conclusion) {
    return diagnosisConclusionReviewQueueInput({
      canConfirmConclusion,
      conclusion: room.latest_conclusion,
    });
  }
  if (room.latest_progress) {
    return diagnosisProgressReviewQueueInput(room.latest_progress);
  }
  return null;
}

function diagnosisConclusionReviewQueueInput({
  canConfirmConclusion,
  conclusion,
}: {
  canConfirmConclusion: boolean;
  conclusion: DiagnosisRoomConclusionSummary;
}): DiagnosisReviewQueueInput {
  return {
    canConfirmConclusion,
    collectionResults: diagnosisRoomSummaryCollectionResults(conclusion),
    confidence: conclusion.confidence,
    conclusionStatus: "ready_for_review",
    evidenceCollectionSuggestions: consultationEvidenceRequests(
      conclusion.evidence_collection_suggestions,
    ),
    evidenceRequests: evidenceRequests(conclusion.evidence_requests),
    latestAssistantSequence: conclusion.assistant_sequence,
    missingEvidenceRequests: consultationEvidenceRequests(
      conclusion.missing_evidence_requests,
    ),
    requiresHumanReview: conclusion.requires_human_review ?? false,
    supplementalEvidence: supplementalEvidenceRecords(
      conclusion.supplemental_evidence,
    ),
  };
}

function diagnosisProgressReviewQueueInput(
  progress: DiagnosisRoomProgressSummary,
): DiagnosisReviewQueueInput {
  return {
    canConfirmConclusion: false,
    collectionResults: diagnosisRoomSummaryCollectionResults(progress),
    confidence: progress.confidence,
    conclusionStatus: progress.conclusion_status,
    evidenceCollectionSuggestions: consultationEvidenceRequests(
      progress.evidence_collection_suggestions,
    ),
    evidenceRequests: evidenceRequests(progress.evidence_requests),
    latestAssistantSequence: progress.assistant_sequence,
    missingEvidenceRequests: consultationEvidenceRequests(
      progress.missing_evidence_requests,
    ),
    requiresHumanReview: progress.requires_human_review,
    supplementalEvidence: supplementalEvidenceRecords(
      progress.supplemental_evidence,
    ),
  };
}

function diagnosisRoomSummaryCollectionResults(
  state: DiagnosisRoomProgressSummary | DiagnosisRoomConclusionSummary,
): DiagnosisEvidenceCollectionResult[] {
  const topLevelResults =
    "evidence_collection_results" in state
      ? state.evidence_collection_results
      : undefined;
  return uniqueCollectionResults([
    ...collectionResults(topLevelResults),
    ...collectionResultsFromTimeline(state.confidence_timeline),
  ]);
}

function collectionResultsFromTimeline(
  entries: DiagnosisRoomConfidenceTimelineEntry[] | undefined,
): DiagnosisEvidenceCollectionResult[] {
  return (entries ?? []).flatMap((entry) =>
    collectionResults(entry.evidence_collection_results),
  );
}

function collectionResults(
  results: DiagnosisRoomEvidenceCollectionResultSummary[] | undefined,
): DiagnosisEvidenceCollectionResult[] {
  return (results ?? []).map((result) => {
    const request = evidenceRequestFromCollectionResult(result);
    return {
      active_alerts: [],
      alert_source_kind: result.alert_source_kind,
      alert_source_profile_id: result.alert_source_profile_id,
      collected_at: result.collected_at,
      limit: result.limit,
      message: result.message ?? "",
      metric_result: undefined,
      observed_alerts: result.observed_alerts ?? 0,
      observed_metric_series: result.observed_metric_series,
      query: result.query,
      reason_code: result.reason_code ?? "",
      request,
      status: result.status,
      step_seconds: result.step_seconds,
      template_id: result.template_id,
      tool: result.tool,
      window_seconds: result.window_seconds,
    };
  });
}

function evidenceRequestFromCollectionResult(
  result: DiagnosisRoomEvidenceCollectionResultSummary,
): DiagnosisEvidenceRequest {
  return {
    alert_source_profile_id: result.alert_source_profile_id,
    limit: result.limit,
    query: result.query,
    reason:
      result.request_reason ??
      result.message ??
      "Retained provider-backed evidence collection result.",
    step_seconds: result.step_seconds,
    template_id: result.template_id,
    tool: result.tool,
    window_seconds: result.window_seconds,
  };
}

function uniqueCollectionResults(
  results: DiagnosisEvidenceCollectionResult[],
): DiagnosisEvidenceCollectionResult[] {
  const seen = new Set<string>();
  const out: DiagnosisEvidenceCollectionResult[] = [];
  for (const result of results) {
    const key = [
      result.tool,
      result.status,
      result.query ?? "",
      result.template_id ?? "",
      result.alert_source_profile_id ?? "",
      result.collected_at,
    ].join(":");
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(result);
  }
  return out;
}

function evidenceRequests(
  requests: DiagnosisRoomEvidenceRequestSummary[] | undefined,
): DiagnosisEvidenceRequest[] {
  return (requests ?? []).map((request) => ({
    alert_source_profile_id: request.alert_source_profile_id,
    limit: request.limit,
    query: request.query,
    reason: request.reason,
    step_seconds: request.step_seconds,
    template_id: request.template_id,
    tool: request.tool,
    window_seconds: request.window_seconds,
  }));
}

function consultationEvidenceRequests(
  requests: DiagnosisRoomConsultationEvidenceRequestSummary[] | undefined,
): DiagnosisConsultationEvidenceRequest[] {
  return (requests ?? []).map((request) => ({
    detail: request.detail,
    label: request.label,
    priority: request.priority,
  }));
}

function supplementalEvidenceRecords(
  records: DiagnosisRoomSupplementalEvidenceSummary[] | undefined,
): DiagnosisSupplementalEvidenceRecord[] {
  return (records ?? []).map((record) => ({
    assistant_message_id: record.assistant_message_id ?? "",
    assistant_sequence: record.assistant_sequence ?? 0,
    assistant_turn_id: record.assistant_turn_id ?? 0,
    detail: record.detail,
    evidence: record.evidence,
    label: record.label,
    priority: record.priority,
    provided_at: record.provided_at,
    user_message_id: record.user_message_id ?? "",
    user_sequence: record.user_sequence ?? 0,
    user_turn_id: record.user_turn_id ?? 0,
  }));
}
