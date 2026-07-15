import type { FinalReportDetail } from "./types";
import { reportEvidenceCollectionResultForRequest } from "./report-evidence-display";

export type ReportDiagnosisConclusion = NonNullable<
  FinalReportDetail["linked_sub_reports"][number]["diagnosis_conclusion"]
>;
export type ReportDiagnosisProgress = NonNullable<
  FinalReportDetail["linked_sub_reports"][number]["diagnosis_progress"]
>;

type LinkedSubReport = FinalReportDetail["linked_sub_reports"][number];
export type ReportDiagnosisState = ReportDiagnosisConclusion | ReportDiagnosisProgress;
type ConfidenceTimelineEntry =
  | NonNullable<ReportDiagnosisConclusion["confidence_timeline"]>[number]
  | NonNullable<ReportDiagnosisProgress["confidence_timeline"]>[number];

export type ReportDiagnosisReviewStatus =
  | "complete"
  | "empty"
  | "failed"
  | "human_review"
  | "needs_evidence"
  | "pending_diagnosis";

export type SubReportDiagnosisReadiness = {
  collectedEvidence: number;
  currentExecutableEvidenceRequests: number;
  currentCollectionSuggestions: number;
  currentMissingEvidence: number;
  evidenceRequests: number;
  failureReason?: string;
  latestConfidence: string;
  reviewed: boolean;
  status: ReportDiagnosisReviewStatus;
  supplementalEvidence: number;
};

export type ReportDiagnosisReadiness = {
  attention: number;
  blocked: boolean;
  canConfirm: boolean;
  collectedEvidence: number;
  currentExecutableEvidenceRequests: number;
  currentCollectionSuggestions: number;
  currentMissingEvidence: number;
  done: number;
  evidenceRequests: number;
  failedSubReports: number;
  humanReviewRequired: number;
  latestConfidence: string;
  pending: number;
  pendingSubReports: number;
  ready: number;
  reviewed: number;
  status: ReportDiagnosisReviewStatus;
  supplementalEvidence: number;
  total: number;
};

type ReportEvidenceFollowUpKind =
  | "collection_suggestion"
  | "evidence_request"
  | "missing_evidence";

type ReportEvidenceRequest = NonNullable<ReportDiagnosisProgress["evidence_requests"]>[number];
type ReportEvidenceCollectionResult = NonNullable<
  ConfidenceTimelineEntry["evidence_collection_results"]
>[number];

export type ReportEvidenceFollowUp = {
  detail: string;
  evidenceSnapshotID: number;
  kind: ReportEvidenceFollowUpKind;
  label: string;
  priority: string;
  request?: ReportEvidenceRequest;
  subReportID: number;
  subReportTitle: string;
};

export type ReportDiagnosisNextAction = {
  evidenceSnapshotID: number;
  hasConclusion: boolean;
  hasProgress: boolean;
  readiness: SubReportDiagnosisReadiness;
  subReportID: number;
  title: string;
};

type ReportDiagnosisHandoffStepKey =
  | "ai_consultation"
  | "evidence_follow_up"
  | "evidence_snapshot"
  | "operator_decision"
  | "report_generation";

type ReportDiagnosisHandoffStepStatus = "attention" | "done" | "pending";

type ReportDiagnosisHandoffStep = {
  key: ReportDiagnosisHandoffStepKey;
  status: ReportDiagnosisHandoffStepStatus;
};

export type ReportDiagnosisHandoff = {
  evidenceSnapshotCount: number;
  followUpCount: number;
  reportID: number;
  reportWorkflow: string;
  status: ReportDiagnosisReviewStatus;
  steps: ReportDiagnosisHandoffStep[];
};

type ReportConsultationAuditStepKey =
  | "initial_report"
  | "supplemental_evidence"
  | "confidence_revision"
  | "final_decision";

type ReportConsultationAuditStepStatus =
  | "attention"
  | "done"
  | "pending";

type ReportConsultationAuditStep = {
  key: ReportConsultationAuditStepKey;
  status: ReportConsultationAuditStepStatus;
};

export type ReportConsultationAuditItem = {
  conclusionVersion: string;
  confirmed: boolean;
  evidenceSnapshotID: number;
  hasDiagnosisState: boolean;
  initialConfidence: string;
  readiness: SubReportDiagnosisReadiness;
  steps: ReportConsultationAuditStep[];
  subReportID: number;
  subReportTitle: string;
};

type APIReportFinalNotificationReadiness =
  FinalReportDetail["final_notification_readiness"];

export type ReportFinalNotificationReadiness =
  | (APIReportFinalNotificationReadiness & { source: "api" })
  | {
      notification_purpose: "final";
      ready: false;
      source: "fallback";
      status: "blocked";
    };

export function diagnosisReadiness(
  report: FinalReportDetail,
): ReportDiagnosisReadiness {
  const subReports = report.linked_sub_reports.map(subReportDiagnosisReadiness);
  const states = report.linked_sub_reports
    .map(subReportDiagnosisState)
    .filter((state): state is ReportDiagnosisState => state !== undefined);
  const reviewed = subReports.filter((subReport) => subReport.reviewed).length;
  const pendingSubReports = report.linked_sub_reports.length - reviewed;
  const readySubReports = subReports.filter(
    (subReport) => subReport.status === "human_review",
  ).length;
  const completeSubReports = subReports.filter(
    (subReport) => subReport.status === "complete",
  ).length;
  const latestState = states.reduce<ReportDiagnosisState | undefined>(
    (latest, state) => {
      if (!latest) {
        return state;
      }
      return Date.parse(diagnosisStateRecordedAt(state)) >=
        Date.parse(diagnosisStateRecordedAt(latest))
        ? state
        : latest;
    },
    undefined,
  );
  const aggregate = subReports.reduce(
    (sum, subReport) => ({
      collectedEvidence: sum.collectedEvidence + subReport.collectedEvidence,
      currentCollectionSuggestions:
        sum.currentCollectionSuggestions +
        subReport.currentCollectionSuggestions,
      currentExecutableEvidenceRequests:
        sum.currentExecutableEvidenceRequests +
        subReport.currentExecutableEvidenceRequests,
      currentMissingEvidence:
        sum.currentMissingEvidence + subReport.currentMissingEvidence,
      evidenceRequests: sum.evidenceRequests + subReport.evidenceRequests,
      failedSubReports:
        sum.failedSubReports + (subReport.status === "failed" ? 1 : 0),
      supplementalEvidence:
        sum.supplementalEvidence + subReport.supplementalEvidence,
    }),
    {
      collectedEvidence: 0,
      currentCollectionSuggestions: 0,
      currentExecutableEvidenceRequests: 0,
      currentMissingEvidence: 0,
      evidenceRequests: 0,
      failedSubReports: 0,
      supplementalEvidence: 0,
    },
  );
  const humanReviewRequired = states.filter(
    (state) => state.requires_human_review && !diagnosisStateConfirmed(state),
  ).length;
  const status = reportStatus({
    currentCollectionSuggestions: aggregate.currentCollectionSuggestions,
    currentExecutableEvidenceRequests:
      aggregate.currentExecutableEvidenceRequests,
    currentMissingEvidence: aggregate.currentMissingEvidence,
    failedSubReports: aggregate.failedSubReports,
    pendingSubReports,
    readySubReports,
    total: report.linked_sub_reports.length,
  });
  const queue = reportReviewQueueSummary({
    currentCollectionSuggestions: aggregate.currentCollectionSuggestions,
    currentExecutableEvidenceRequests:
      aggregate.currentExecutableEvidenceRequests,
    currentMissingEvidence: aggregate.currentMissingEvidence,
    completeSubReports,
    failedSubReports: aggregate.failedSubReports,
    pendingSubReports,
    readySubReports,
  });
  return {
    ...aggregate,
    attention: queue.attention,
    blocked: queue.blocked,
    canConfirm: queue.canConfirm,
    done: queue.done,
    humanReviewRequired,
    latestConfidence: latestState
      ? diagnosisStateLatestConfidence(latestState)
      : "pending",
    pending: queue.pending,
    pendingSubReports,
    ready: queue.ready,
    reviewed,
    status,
    total: report.linked_sub_reports.length,
  };
}

export function reportEvidenceFollowUps(
  report: FinalReportDetail,
): ReportEvidenceFollowUp[] {
  return report.linked_sub_reports.flatMap((subReport) => {
    const state = subReportDiagnosisState(subReport);
    if (!state) {
      return [];
    }
    if (diagnosisStateConfirmed(state)) {
      return [];
    }
    const latestTimeline = latestTimelineEntry(state.confidence_timeline ?? []);
    const missingEvidence =
      diagnosisStateMissingEvidenceRequests(state) ??
      latestTimeline?.missing_evidence_requests ??
      [];
    const collectionSuggestions =
      diagnosisStateCollectionSuggestions(state) ??
      latestTimeline?.evidence_collection_suggestions ??
      [];
    const evidenceRequests = diagnosisStateEvidenceRequests(
      state,
      latestTimeline,
    ).filter((request) =>
      diagnosisStateExecutableEvidenceRequestIsCurrent(state, request),
    );
    return [
      ...evidenceRequests.map((request) =>
        reportEvidenceRequestFollowUp(subReport, request),
      ),
      ...missingEvidence.map((request) =>
        reportEvidenceFollowUp(
          subReport,
          "missing_evidence",
          request,
        ),
      ),
      ...collectionSuggestions.map((request) =>
        reportEvidenceFollowUp(
          subReport,
          "collection_suggestion",
          request,
        ),
      ),
    ];
  });
}

export function reportDiagnosisNextAction(
  report: FinalReportDetail,
): ReportDiagnosisNextAction | null {
  const nextAction = report.linked_sub_reports
    .map((subReport, index) => ({
      index,
      readiness: subReportDiagnosisReadiness(subReport),
      subReport,
    }))
    .filter(({ readiness }) => readiness.status !== "complete")
    .sort(compareReportDiagnosisNextAction)[0];
  if (!nextAction) {
    return null;
  }
  return {
    evidenceSnapshotID: nextAction.subReport.evidence_snapshot_id,
    hasConclusion: nextAction.subReport.diagnosis_conclusion !== undefined,
    hasProgress: nextAction.subReport.diagnosis_progress !== undefined,
    readiness: nextAction.readiness,
    subReportID: nextAction.subReport.id,
    title: nextAction.subReport.title,
  };
}

export function reportDiagnosisHandoff(
  report: FinalReportDetail,
): ReportDiagnosisHandoff {
  const readiness = diagnosisReadiness(report);
  const followUps = reportEvidenceFollowUps(report);
  const evidenceSnapshotCount = new Set(
    report.linked_sub_reports.map((subReport) => subReport.evidence_snapshot_id),
  ).size;
  return {
    evidenceSnapshotCount,
    followUpCount: followUps.length,
    reportID: report.id,
    reportWorkflow: report.created_by_workflow,
    status: readiness.status,
    steps: [
      {
        key: "report_generation",
        status: "done",
      },
      {
        key: "evidence_snapshot",
        status: evidenceSnapshotCount > 0 ? "done" : "pending",
      },
      reportDiagnosisHandoffConsultationStep(readiness),
      reportDiagnosisHandoffEvidenceStep(readiness),
      reportDiagnosisHandoffDecisionStep(readiness),
    ],
  };
}

export function reportConsultationAuditTimeline(
  report: FinalReportDetail,
): ReportConsultationAuditItem[] {
  return report.linked_sub_reports.map((subReport) => {
    const readiness = subReportDiagnosisReadiness(subReport);
    const state = subReportDiagnosisState(subReport);
    const timeline = state?.confidence_timeline ?? [];
    return {
      conclusionVersion: state
        ? (diagnosisStateConclusionVersion(state) ?? "")
        : "",
      confirmed: state ? diagnosisStateConfirmed(state) : false,
      evidenceSnapshotID: subReport.evidence_snapshot_id,
      hasDiagnosisState: state !== undefined,
      initialConfidence: state
        ? (firstTimelineEntry(timeline)?.confidence ??
          diagnosisStateLatestConfidence(state, latestTimelineEntry(timeline)))
        : "pending",
      readiness,
      steps: reportConsultationAuditSteps(readiness, state),
      subReportID: subReport.id,
      subReportTitle: subReport.title,
    };
  });
}

export function reportFinalNotificationReadiness(
  report: FinalReportDetail,
): ReportFinalNotificationReadiness {
  const readiness = (
    report as {
      final_notification_readiness?: APIReportFinalNotificationReadiness | null;
    }
  ).final_notification_readiness;
  return readiness
    ? { ...readiness, source: "api" }
    : fallbackFinalNotificationReadiness;
}

const fallbackFinalNotificationReadiness: ReportFinalNotificationReadiness = {
  notification_purpose: "final",
  ready: false,
  source: "fallback",
  status: "blocked",
};

function compareReportDiagnosisNextAction(
  left: {
    index: number;
    readiness: SubReportDiagnosisReadiness;
  },
  right: {
    index: number;
    readiness: SubReportDiagnosisReadiness;
  },
) {
  const priority =
    reportDiagnosisNextActionPriority(left.readiness.status) -
    reportDiagnosisNextActionPriority(right.readiness.status);
  if (priority !== 0) {
    return priority;
  }
  return left.index - right.index;
}

function reportDiagnosisNextActionPriority(
  status: ReportDiagnosisReviewStatus,
) {
  switch (status) {
    case "failed":
      return 0;
    case "pending_diagnosis":
      return 1;
    case "needs_evidence":
      return 2;
    case "human_review":
      return 3;
    case "complete":
    case "empty":
      return 4;
  }
}

function reportReviewQueueSummary(input: {
  completeSubReports: number;
  currentCollectionSuggestions: number;
  currentExecutableEvidenceRequests: number;
  currentMissingEvidence: number;
  failedSubReports: number;
  pendingSubReports: number;
  readySubReports: number;
}) {
  const blockingEvidenceWork =
    input.currentMissingEvidence + input.currentExecutableEvidenceRequests;
  const residualEvidenceWork = input.currentCollectionSuggestions;
  const attention = input.failedSubReports;
  const pending =
    input.pendingSubReports + blockingEvidenceWork + residualEvidenceWork;
  const ready = input.readySubReports;
  const done = input.completeSubReports;
  const blocked =
    input.failedSubReports > 0 ||
    input.pendingSubReports > 0 ||
    blockingEvidenceWork > 0;
  const canConfirm = ready > 0 && !blocked;
  return {
    attention,
    blocked,
    canConfirm,
    done,
    pending,
    ready,
  };
}

function reportDiagnosisHandoffConsultationStep(
  readiness: ReportDiagnosisReadiness,
): ReportDiagnosisHandoffStep {
  if (readiness.failedSubReports > 0) {
    return {
      key: "ai_consultation",
      status: "attention",
    };
  }
  if (readiness.pendingSubReports > 0) {
    return {
      key: "ai_consultation",
      status: "pending",
    };
  }
  if (readiness.reviewed > 0) {
    return {
      key: "ai_consultation",
      status: "done",
    };
  }
  return {
    key: "ai_consultation",
    status: "pending",
  };
}

function reportDiagnosisHandoffEvidenceStep(
  readiness: ReportDiagnosisReadiness,
): ReportDiagnosisHandoffStep {
  const blockingEvidenceWork =
    readiness.currentMissingEvidence +
    readiness.currentExecutableEvidenceRequests;
  if (blockingEvidenceWork > 0) {
    return {
      key: "evidence_follow_up",
      status: "pending",
    };
  }
  if (readiness.currentCollectionSuggestions > 0) {
    return {
      key: "evidence_follow_up",
      status: "done",
    };
  }
  if (readiness.reviewed > 0) {
    return {
      key: "evidence_follow_up",
      status: "done",
    };
  }
  return {
    key: "evidence_follow_up",
    status: "pending",
  };
}

function reportDiagnosisHandoffDecisionStep(
  readiness: ReportDiagnosisReadiness,
): ReportDiagnosisHandoffStep {
  if (readiness.blocked) {
    return {
      key: "operator_decision",
      status: "attention",
    };
  }
  if (readiness.canConfirm || readiness.ready > 0) {
    return {
      key: "operator_decision",
      status: "pending",
    };
  }
  if (readiness.status === "complete") {
    return {
      key: "operator_decision",
      status: "done",
    };
  }
  return {
    key: "operator_decision",
    status: "pending",
  };
}

function reportConsultationAuditSteps(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep[] {
  return [
    reportConsultationInitialStep(state),
    reportConsultationSupplementalEvidenceStep(readiness, state),
    reportConsultationConfidenceRevisionStep(readiness, state),
    reportConsultationFinalDecisionStep(readiness, state),
  ];
}

function reportConsultationInitialStep(
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep("initial_report", "pending");
  }
  return reportConsultationAuditStep("initial_report", "done");
}

function reportConsultationSupplementalEvidenceStep(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep("supplemental_evidence", "pending");
  }
  if (diagnosisStateFailed(state)) {
    return reportConsultationAuditStep("supplemental_evidence", "attention");
  }

  const blockingEvidenceWork =
    readiness.currentMissingEvidence +
    readiness.currentExecutableEvidenceRequests;
  if (blockingEvidenceWork > 0) {
    return reportConsultationAuditStep("supplemental_evidence", "pending");
  }
  return reportConsultationAuditStep("supplemental_evidence", "done");
}

function reportConsultationConfidenceRevisionStep(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep("confidence_revision", "pending");
  }
  if (diagnosisStateFailed(state)) {
    return reportConsultationAuditStep("confidence_revision", "attention");
  }

  const timeline = state.confidence_timeline ?? [];
  const first = firstTimelineEntry(timeline);
  const latest = latestTimelineEntry(timeline);
  const initialConfidence =
    first?.confidence ?? diagnosisStateLatestConfidence(state, latest);
  const latestConfidence = readiness.latestConfidence;
  const direction = confidenceDirection(initialConfidence, latestConfidence);
  if (direction === "decreased") {
    return reportConsultationAuditStep("confidence_revision", "pending");
  }
  return reportConsultationAuditStep("confidence_revision", "done");
}

function reportConsultationFinalDecisionStep(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep("final_decision", "pending");
  }
  if (readiness.status === "failed") {
    return reportConsultationAuditStep("final_decision", "attention");
  }
  if (
    readiness.status === "needs_evidence" ||
    readiness.status === "human_review"
  ) {
    return reportConsultationAuditStep("final_decision", "pending");
  }
  return reportConsultationAuditStep("final_decision", "done");
}

function reportConsultationAuditStep(
  key: ReportConsultationAuditStepKey,
  status: ReportConsultationAuditStepStatus,
): ReportConsultationAuditStep {
  return { key, status };
}

function reportEvidenceFollowUp(
  subReport: LinkedSubReport,
  kind: ReportEvidenceFollowUpKind,
  request: NonNullable<ConfidenceTimelineEntry["missing_evidence_requests"]>[number],
): ReportEvidenceFollowUp {
  return {
    detail: request.detail,
    evidenceSnapshotID: subReport.evidence_snapshot_id,
    kind,
    label: request.label,
    priority: request.priority,
    subReportID: subReport.id,
    subReportTitle: subReport.title,
  };
}

function reportEvidenceRequestFollowUp(
  subReport: LinkedSubReport,
  request: ReportEvidenceRequest,
): ReportEvidenceFollowUp {
  return {
    detail: "",
    evidenceSnapshotID: subReport.evidence_snapshot_id,
    kind: "evidence_request",
    label: request.reason,
    priority: "action",
    request,
    subReportID: subReport.id,
    subReportTitle: subReport.title,
  };
}

function isExecutableReportEvidenceRequest(request: ReportEvidenceRequest) {
  switch (request.tool) {
    case "active_alerts":
      return request.query === undefined;
    case "metric_query":
    case "metric_range_query":
      return request.query !== undefined || request.template_id !== undefined;
    default:
      return false;
  }
}

export function subReportDiagnosisReadiness(
  subReport: LinkedSubReport,
): SubReportDiagnosisReadiness {
  const state = subReportDiagnosisState(subReport);
  if (!state) {
    return {
      collectedEvidence: 0,
      currentCollectionSuggestions: 0,
      currentExecutableEvidenceRequests: 0,
      currentMissingEvidence: 0,
      evidenceRequests: 0,
      failureReason: undefined,
      latestConfidence: "pending",
      reviewed: false,
      status: "pending_diagnosis",
      supplementalEvidence: 0,
    };
  }

  const timeline = state.confidence_timeline ?? [];
  const latestTimeline = latestTimelineEntry(timeline);
  const confirmed = diagnosisStateConfirmed(state);
  const currentMissingEvidence = confirmed
    ? 0
    : diagnosisStateMissingEvidenceRequests(state)?.length ??
      latestTimeline?.missing_evidence_requests?.length ??
      0;
  const currentCollectionSuggestions = confirmed
    ? 0
    : diagnosisStateCollectionSuggestions(state)?.length ??
      latestTimeline?.evidence_collection_suggestions?.length ??
      0;
  const currentExecutableEvidenceRequests = confirmed
    ? 0
    : diagnosisStateCurrentExecutableEvidenceRequests(
        state,
        latestTimeline,
      ).length;
  const status = subReportStatus(state, {
    currentCollectionSuggestions,
    currentExecutableEvidenceRequests,
    currentMissingEvidence,
  });

  return {
    collectedEvidence: collectedEvidenceCount(timeline),
    currentCollectionSuggestions,
    currentExecutableEvidenceRequests,
    currentMissingEvidence,
    evidenceRequests: diagnosisStateEvidenceRequestCount(state, timeline),
    failureReason: diagnosisStateFailureReason(state),
    latestConfidence: diagnosisStateLatestConfidence(state, latestTimeline),
    reviewed: true,
    status,
    supplementalEvidence: state.supplemental_evidence?.length ?? 0,
  };
}

function reportStatus(input: {
  currentCollectionSuggestions: number;
  currentExecutableEvidenceRequests: number;
  currentMissingEvidence: number;
  failedSubReports: number;
  pendingSubReports: number;
  readySubReports: number;
  total: number;
}): ReportDiagnosisReviewStatus {
  if (input.total === 0) {
    return "empty";
  }
  if (input.failedSubReports > 0) {
    return "failed";
  }
  if (input.pendingSubReports > 0) {
    return "pending_diagnosis";
  }
  if (
    input.currentMissingEvidence > 0 ||
    input.currentExecutableEvidenceRequests > 0
  ) {
    return "needs_evidence";
  }
  if (input.readySubReports > 0) {
    return "human_review";
  }
  return "complete";
}

function subReportStatus(
  state: ReportDiagnosisState,
  input: {
    currentCollectionSuggestions: number;
    currentExecutableEvidenceRequests: number;
    currentMissingEvidence: number;
  },
): ReportDiagnosisReviewStatus {
  if (diagnosisStateFailed(state)) {
    return "failed";
  }
  if (diagnosisStateConfirmed(state)) {
    return "complete";
  }
  if (
    input.currentMissingEvidence > 0 ||
    input.currentExecutableEvidenceRequests > 0
  ) {
    return "needs_evidence";
  }
  if (diagnosisStateNeedsOperatorConfirmation(state)) {
    return "human_review";
  }
  return "complete";
}

export function subReportDiagnosisState(
  subReport: LinkedSubReport,
): ReportDiagnosisState | undefined {
  const conclusion = subReport.diagnosis_conclusion;
  const progress = subReport.diagnosis_progress;
  if (conclusion === undefined) {
    return progress;
  }
  if (progress === undefined) {
    return conclusion;
  }
  return diagnosisStateIsAfter(progress, conclusion)
    ? progress
    : conclusion;
}

function diagnosisStateRecordedAt(state: ReportDiagnosisState) {
  return state.recorded_at;
}

function diagnosisStateIsAfter(
  left: ReportDiagnosisState,
  right: ReportDiagnosisState,
) {
  return diagnosisStateRecordedAtMs(left) > diagnosisStateRecordedAtMs(right);
}

function diagnosisStateRecordedAtMs(state: ReportDiagnosisState) {
  const parsed = Date.parse(diagnosisStateRecordedAt(state));
  return Number.isFinite(parsed) ? parsed : Number.NEGATIVE_INFINITY;
}

function diagnosisStateMissingEvidenceRequests(state: ReportDiagnosisState) {
  return "missing_evidence_requests" in state
    ? state.missing_evidence_requests
    : undefined;
}

function diagnosisStateCollectionSuggestions(state: ReportDiagnosisState) {
  return "evidence_collection_suggestions" in state
    ? state.evidence_collection_suggestions
    : undefined;
}

function diagnosisStateEvidenceRequests(
  state: ReportDiagnosisState,
  latestTimeline?: ConfidenceTimelineEntry,
): ReportEvidenceRequest[] {
  if ("evidence_requests" in state && state.evidence_requests !== undefined) {
    return state.evidence_requests;
  }
  return latestTimeline?.evidence_requests ?? [];
}

function diagnosisStateCurrentExecutableEvidenceRequests(
  state: ReportDiagnosisState,
  latestTimeline?: ConfidenceTimelineEntry,
): ReportEvidenceRequest[] {
  return diagnosisStateEvidenceRequests(state, latestTimeline).filter(
    (request) => diagnosisStateExecutableEvidenceRequestIsCurrent(state, request),
  );
}

function diagnosisStateExecutableEvidenceRequestIsCurrent(
  state: ReportDiagnosisState,
  request: ReportEvidenceRequest,
) {
  if (!isExecutableReportEvidenceRequest(request)) {
    return false;
  }
  const collectedEvidenceResults = diagnosisStateEvidenceCollectionResults(
    state,
    state.confidence_timeline ?? [],
  ).filter((result) => result.status === "collected");
  return (
    reportEvidenceCollectionResultForRequest(
      request,
      collectedEvidenceResults,
    ) === undefined
  );
}

function diagnosisStateEvidenceCollectionResults(
  state: ReportDiagnosisState,
  timeline: ConfidenceTimelineEntry[],
): ReportEvidenceCollectionResult[] {
  const results: ReportEvidenceCollectionResult[] = [];
  if (
    "evidence_collection_results" in state &&
    state.evidence_collection_results !== undefined
  ) {
    results.push(...state.evidence_collection_results);
  }
  for (const item of timeline) {
    if (item.evidence_collection_results !== undefined) {
      results.push(...item.evidence_collection_results);
    }
  }
  return results;
}

function diagnosisStateEvidenceRequestCount(
  state: ReportDiagnosisState,
  timeline: ConfidenceTimelineEntry[],
) {
  if ("evidence_request_count" in state) {
    return state.evidence_request_count;
  }
  if ("evidence_requests" in state && state.evidence_requests !== undefined) {
    return state.evidence_requests.length;
  }
  return evidenceRequestCount(timeline);
}

function diagnosisStateFailed(state: ReportDiagnosisState) {
  return "failure_reason" in state && state.status === "failed";
}

function diagnosisStateConfirmed(state: ReportDiagnosisState) {
  return "confirmed_by" in state && Boolean(state.confirmed_by);
}

function diagnosisStateNeedsOperatorConfirmation(state: ReportDiagnosisState) {
  if (diagnosisStateConfirmed(state)) {
    return false;
  }
  if ("content" in state && state.status === "available") {
    return true;
  }
  const conclusionStatus =
    "conclusion_status" in state
      ? state.conclusion_status?.trim().toLowerCase()
      : undefined;
  return (
    conclusionStatus === "final" ||
    conclusionStatus === "ready_for_review" ||
    Boolean(state.requires_human_review)
  );
}

function diagnosisStateFailureReason(state: ReportDiagnosisState) {
  return "failure_reason" in state ? state.failure_reason : undefined;
}

function diagnosisStateConclusionVersion(state: ReportDiagnosisState) {
  return "conclusion_version" in state ? state.conclusion_version : undefined;
}

function diagnosisStateLatestConfidence(
  state: ReportDiagnosisState,
  latestTimeline?: ConfidenceTimelineEntry,
) {
  if (diagnosisStateFailed(state)) {
    return "failed";
  }
  return state.confidence ?? latestTimeline?.confidence ?? "pending";
}

function evidenceRequestCount(timeline: ConfidenceTimelineEntry[]) {
  return timeline.reduce((sum, item) => {
    if (item.evidence_requests) {
      return sum + item.evidence_requests.length;
    }
    return sum + (item.evidence_request_count ?? 0);
  }, 0);
}

function collectedEvidenceCount(timeline: ConfidenceTimelineEntry[]) {
  return timeline.reduce(
    (sum, item) =>
      sum +
      (item.evidence_collection_results?.filter(
        (result) => result.status === "collected",
      ).length ?? 0),
    0,
  );
}

function latestTimelineEntry(timeline: ConfidenceTimelineEntry[]) {
  return timeline.reduce<ConfidenceTimelineEntry | undefined>(
    (latest, item) => {
      if (!latest) {
        return item;
      }
      return Date.parse(item.occurred_at) >= Date.parse(latest.occurred_at)
        ? item
        : latest;
    },
    undefined,
  );
}

function firstTimelineEntry(timeline: ConfidenceTimelineEntry[]) {
  return timeline.reduce<ConfidenceTimelineEntry | undefined>(
    (first, item) => {
      if (!first) {
        return item;
      }
      return timelineEntryOccurredAtMs(item) <= timelineEntryOccurredAtMs(first)
        ? item
        : first;
    },
    undefined,
  );
}

function timelineEntryOccurredAtMs(item: ConfidenceTimelineEntry) {
  const parsed = Date.parse(item.occurred_at);
  return Number.isFinite(parsed) ? parsed : Number.POSITIVE_INFINITY;
}

function confidenceDirection(
  initialConfidence: string,
  latestConfidence: string,
) {
  const initialRank = confidenceRank(initialConfidence);
  const latestRank = confidenceRank(latestConfidence);
  if (initialRank === undefined || latestRank === undefined) {
    return "unknown";
  }
  if (latestRank > initialRank) {
    return "increased";
  }
  if (latestRank < initialRank) {
    return "decreased";
  }
  return "same";
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
