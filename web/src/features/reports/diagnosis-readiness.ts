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

type DiagnosisReviewStatus =
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
  status: DiagnosisReviewStatus;
  statusDetail: string;
  statusLabel: string;
  supplementalEvidence: number;
};

export type ReportDiagnosisReadiness = {
  attention: number;
  blockingReason: string;
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
  reviewQueueDetail: string;
  reviewQueueLabel: string;
  status: DiagnosisReviewStatus;
  statusDetail: string;
  statusLabel: string;
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
  actionLabel: string;
  detail: string;
  evidenceSnapshotID: number;
  status: DiagnosisReviewStatus;
  statusLabel: string;
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
  detail: string;
  key: ReportDiagnosisHandoffStepKey;
  label: string;
  status: ReportDiagnosisHandoffStepStatus;
  statusLabel: string;
};

export type ReportDiagnosisHandoff = {
  statusDetail: string;
  statusLabel: string;
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
  detail: string;
  key: ReportConsultationAuditStepKey;
  label: string;
  status: ReportConsultationAuditStepStatus;
  statusLabel: string;
};

export type ReportConsultationAuditItem = {
  evidenceSnapshotID: number;
  status: DiagnosisReviewStatus;
  statusDetail: string;
  statusLabel: string;
  steps: ReportConsultationAuditStep[];
  subReportID: number;
  subReportTitle: string;
};

export type ReportFinalNotificationReadiness =
  FinalReportDetail["final_notification_readiness"];

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
    status,
  });
  return {
    ...aggregate,
    attention: queue.attention,
    blockingReason: queue.blockingReason,
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
    reviewQueueDetail: queue.detail,
    reviewQueueLabel: queue.label,
    status,
    statusDetail: reportStatusDetail(status, {
      currentCollectionSuggestions: aggregate.currentCollectionSuggestions,
      currentExecutableEvidenceRequests:
        aggregate.currentExecutableEvidenceRequests,
      currentMissingEvidence: aggregate.currentMissingEvidence,
      failedSubReports: aggregate.failedSubReports,
      pendingSubReports,
      readySubReports,
      total: report.linked_sub_reports.length,
    }),
    statusLabel: statusLabel(status),
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
    actionLabel: subReportDiagnosisActionLabel(nextAction.subReport),
    detail: nextAction.readiness.statusDetail,
    evidenceSnapshotID: nextAction.subReport.evidence_snapshot_id,
    status: nextAction.readiness.status,
    statusLabel: nextAction.readiness.statusLabel,
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
    statusDetail: reportDiagnosisHandoffDetail(readiness, followUps.length),
    statusLabel: reportDiagnosisHandoffLabel(readiness),
    steps: [
      {
        detail: `Final report #${report.id} was generated by ${report.created_by_workflow}.`,
        key: "report_generation",
        label: "Report generated",
        status: "done",
        statusLabel: "Done",
      },
      {
        detail:
          evidenceSnapshotCount > 0
            ? `${evidenceSnapshotCount} evidence ${plural(
                evidenceSnapshotCount,
                "snapshot",
              )} linked to the AI diagnosis path.`
            : "No linked evidence snapshots are available for AI review.",
        key: "evidence_snapshot",
        label: "Evidence snapshots",
        status: evidenceSnapshotCount > 0 ? "done" : "pending",
        statusLabel: evidenceSnapshotCount > 0 ? "Done" : "Pending",
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
    return {
      evidenceSnapshotID: subReport.evidence_snapshot_id,
      status: readiness.status,
      statusDetail: readiness.statusDetail,
      statusLabel: readiness.statusLabel,
      steps: reportConsultationAuditSteps(subReport, readiness, state),
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
      final_notification_readiness?: ReportFinalNotificationReadiness | null;
    }
  ).final_notification_readiness;
  return readiness ?? fallbackFinalNotificationReadiness;
}

const fallbackFinalNotificationReadiness: ReportFinalNotificationReadiness = {
  detail:
    "Final notification readiness is unavailable; complete diagnosis review before retrying final delivery.",
  notification_purpose: "final",
  ready: false,
  status: "blocked",
  status_label: "Final notification blocked",
};

export function subReportDiagnosisActionLabel(
  subReport: LinkedSubReport,
): string {
  const readiness = subReportDiagnosisReadiness(subReport);
  switch (readiness.status) {
    case "failed":
      return "Review failed diagnosis";
    case "needs_evidence":
      return "Resolve evidence";
    case "human_review":
      return "Confirm diagnosis";
    case "pending_diagnosis":
      return subReport.diagnosis_progress
        ? "Continue diagnosis"
        : "Prepare diagnosis";
    case "complete":
      return subReport.diagnosis_conclusion
        ? "Review confirmed diagnosis"
        : "Review diagnosis";
    case "empty":
      return "Prepare diagnosis";
  }
}

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

function reportDiagnosisNextActionPriority(status: DiagnosisReviewStatus) {
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
  status: DiagnosisReviewStatus;
}) {
  const blockingEvidenceWork =
    input.currentMissingEvidence + input.currentExecutableEvidenceRequests;
  const residualEvidenceWork = input.currentCollectionSuggestions;
  const attention = input.failedSubReports;
  const pending =
    input.pendingSubReports + blockingEvidenceWork + residualEvidenceWork;
  const ready = input.readySubReports;
  const done = input.completeSubReports;
  const blockingReason = reportReviewQueueBlockingReason(input);
  const canConfirm = ready > 0 && blockingReason === "";
  return {
    attention,
    blockingReason,
    canConfirm,
    detail: reportReviewQueueDetail({
      attention,
      blockingReason,
      canConfirm,
      done,
      pending,
      ready,
      status: input.status,
    }),
    done,
    label: reportReviewQueueLabel(input.status),
    pending,
    ready,
  };
}

function reportDiagnosisHandoffConsultationStep(
  readiness: ReportDiagnosisReadiness,
): ReportDiagnosisHandoffStep {
  if (readiness.failedSubReports > 0) {
    return {
      detail: `Resolve ${readiness.failedSubReports} failed diagnosis ${plural(
        readiness.failedSubReports,
        "room",
      )} before continuing the handoff.`,
      key: "ai_consultation",
      label: "AI consultation",
      status: "attention",
      statusLabel: "Attention",
    };
  }
  if (readiness.pendingSubReports > 0) {
    return {
      detail: `Start or continue AI diagnosis for ${readiness.pendingSubReports} ${plural(
        readiness.pendingSubReports,
        "linked subreport",
      )}.`,
      key: "ai_consultation",
      label: "AI consultation",
      status: "pending",
      statusLabel: "Pending",
    };
  }
  if (readiness.reviewed > 0) {
    const verb = readiness.total === 1 ? "has" : "have";
    return {
      detail: `${readiness.reviewed} of ${readiness.total} linked ${plural(
        readiness.total,
        "subreport",
      )} ${verb} AI diagnosis state.`,
      key: "ai_consultation",
      label: "AI consultation",
      status: "done",
      statusLabel: "Done",
    };
  }
  return {
    detail: "No linked subreports are available for AI consultation.",
    key: "ai_consultation",
    label: "AI consultation",
    status: "pending",
    statusLabel: "Pending",
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
      detail: `Resolve ${blockingEvidenceWorkDescription(readiness)} before final confirmation.`,
      key: "evidence_follow_up",
      label: "Evidence follow-up",
      status: "pending",
      statusLabel: "Pending",
    };
  }
  if (readiness.currentCollectionSuggestions > 0) {
    return {
      detail: `${residualEvidenceWorkDescription(readiness)} ${residualEvidenceWorkVerb(readiness)} documented as residual collection guidance; no blocking evidence remains.`,
      key: "evidence_follow_up",
      label: "Evidence follow-up",
      status: "done",
      statusLabel: "Done",
    };
  }
  if (readiness.reviewed > 0) {
    return {
      detail: "No open evidence requests remain on the latest AI turn.",
      key: "evidence_follow_up",
      label: "Evidence follow-up",
      status: "done",
      statusLabel: "Done",
    };
  }
  return {
    detail: "Evidence follow-up starts after AI diagnosis records a first turn.",
    key: "evidence_follow_up",
    label: "Evidence follow-up",
    status: "pending",
    statusLabel: "Pending",
  };
}

function reportDiagnosisHandoffDecisionStep(
  readiness: ReportDiagnosisReadiness,
): ReportDiagnosisHandoffStep {
  if (readiness.blockingReason !== "") {
    return {
      detail: readiness.blockingReason,
      key: "operator_decision",
      label: "Operator decision",
      status: "attention",
      statusLabel: "Attention",
    };
  }
  if (readiness.canConfirm || readiness.ready > 0) {
    return {
      detail: `${readiness.ready} AI ${plural(
        readiness.ready,
        "conclusion",
      )} ready for operator confirmation.`,
      key: "operator_decision",
      label: "Operator decision",
      status: "pending",
      statusLabel: "Pending",
    };
  }
  if (readiness.status === "complete") {
    return {
      detail: "Final AI conclusions are complete with no open operator action.",
      key: "operator_decision",
      label: "Operator decision",
      status: "done",
      statusLabel: "Done",
    };
  }
  return {
    detail: "Operator decision waits for AI consultation and evidence follow-up.",
    key: "operator_decision",
    label: "Operator decision",
    status: "pending",
    statusLabel: "Pending",
  };
}

function reportDiagnosisHandoffLabel(readiness: ReportDiagnosisReadiness) {
  switch (readiness.status) {
    case "complete":
      return "Complete";
    case "empty":
      return "No handoff";
    case "failed":
      return "Blocked";
    case "human_review":
      return "Ready for operator confirmation";
    case "needs_evidence":
      return "Evidence follow-up required";
    case "pending_diagnosis":
      return "AI consultation pending";
  }
}

function reportDiagnosisHandoffDetail(
  readiness: ReportDiagnosisReadiness,
  followUpCount: number,
) {
  if (readiness.blockingReason !== "") {
    return readiness.reviewQueueDetail;
  }
  if (readiness.canConfirm || readiness.ready > 0) {
    return readiness.reviewQueueDetail;
  }
  if (followUpCount > 0) {
    return `${followUpCount} evidence ${plural(
      followUpCount,
      "follow-up",
    )} ready for the diagnosis room.`;
  }
  return readiness.statusDetail;
}

function reportConsultationAuditSteps(
  subReport: LinkedSubReport,
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep[] {
  return [
    reportConsultationInitialStep(state),
    reportConsultationSupplementalEvidenceStep(readiness, state),
    reportConsultationConfidenceRevisionStep(readiness, state),
    reportConsultationFinalDecisionStep(subReport, readiness, state),
  ];
}

function reportConsultationInitialStep(
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep({
      detail:
        "AI diagnosis has not produced the initial report for this subreport.",
      key: "initial_report",
      label: "Initial AI report",
      status: "pending",
    });
  }
  const timeline = state.confidence_timeline ?? [];
  const evidenceRequestCount = diagnosisStateEvidenceRequestCount(
    state,
    timeline,
  );
  const initialConfidence =
    firstTimelineEntry(timeline)?.confidence ??
    diagnosisStateLatestConfidence(state, latestTimelineEntry(timeline));
  return reportConsultationAuditStep({
    detail: `Initial AI report recorded ${initialConfidence} confidence and ${evidenceRequestCount} evidence ${plural(
      evidenceRequestCount,
      "request",
    )}.`,
    key: "initial_report",
    label: "Initial AI report",
    status: "done",
  });
}

function reportConsultationSupplementalEvidenceStep(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep({
      detail:
        "Evidence collection starts after AI produces an initial report.",
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "pending",
    });
  }
  if (diagnosisStateFailed(state)) {
    return reportConsultationAuditStep({
      detail:
        "Evidence collection is blocked until the failed diagnosis run is retried.",
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "attention",
    });
  }

  const attachedEvidence =
    readiness.supplementalEvidence + readiness.collectedEvidence;
  const blockingEvidenceWork =
    readiness.currentMissingEvidence +
    readiness.currentExecutableEvidenceRequests;
  if (blockingEvidenceWork > 0) {
    return reportConsultationAuditStep({
      detail: `${blockingEvidenceWorkDescription(readiness)} ${blockingEvidenceWorkVerb(readiness)} before confidence can be raised or confirmed.${
        attachedEvidence > 0
          ? ` ${attachedEvidence} supplemental or collected evidence ${plural(
              attachedEvidence,
              "item",
            )} already retained.`
          : ""
      }`,
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "pending",
    });
  }
  if (readiness.currentCollectionSuggestions > 0) {
    return reportConsultationAuditStep({
      detail: `${residualEvidenceWorkDescription(readiness)} ${residualEvidenceWorkVerb(readiness)} documented as residual guidance; operator can accept this uncertainty or collect more evidence before confirmation.${
        attachedEvidence > 0
          ? ` ${attachedEvidence} supplemental or collected evidence ${plural(
              attachedEvidence,
              "item",
            )} already retained.`
          : ""
      }`,
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "done",
    });
  }
  if (attachedEvidence > 0) {
    return reportConsultationAuditStep({
      detail: `${attachedEvidence} supplemental or collected evidence ${plural(
        attachedEvidence,
        "item",
      )} retained for the AI diagnosis path.`,
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "done",
    });
  }
  if (readiness.evidenceRequests > 0) {
    return reportConsultationAuditStep({
      detail:
        "AI requested evidence, and no open evidence work remains on the latest turn.",
      key: "supplemental_evidence",
      label: "Supplemental evidence",
      status: "done",
    });
  }
  return reportConsultationAuditStep({
    detail: "AI did not require supplemental evidence for this subreport.",
    key: "supplemental_evidence",
    label: "Supplemental evidence",
    status: "done",
  });
}

function reportConsultationConfidenceRevisionStep(
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep({
      detail:
        "Confidence revision waits for the first AI diagnosis turn.",
      key: "confidence_revision",
      label: "Confidence revision",
      status: "pending",
    });
  }
  if (diagnosisStateFailed(state)) {
    return reportConsultationAuditStep({
      detail: "Confidence revision stopped because the diagnosis run failed.",
      key: "confidence_revision",
      label: "Confidence revision",
      status: "attention",
    });
  }

  const timeline = state.confidence_timeline ?? [];
  const first = firstTimelineEntry(timeline);
  const latest = latestTimelineEntry(timeline);
  const initialConfidence =
    first?.confidence ?? diagnosisStateLatestConfidence(state, latest);
  const latestConfidence = readiness.latestConfidence;
  const direction = confidenceDirection(initialConfidence, latestConfidence);
  if (direction === "increased") {
    return reportConsultationAuditStep({
      detail: `Confidence increased from ${initialConfidence} to ${latestConfidence} after evidence review.`,
      key: "confidence_revision",
      label: "Confidence revision",
      status: "done",
    });
  }
  if (direction === "decreased") {
    return reportConsultationAuditStep({
      detail: `Confidence moved from ${initialConfidence} to ${latestConfidence}; keep the evidence trail available for operator review.`,
      key: "confidence_revision",
      label: "Confidence revision",
      status: "pending",
    });
  }
  if (direction === "same") {
    return reportConsultationAuditStep({
      detail: `Confidence remains ${latestConfidence} after the latest AI turn.`,
      key: "confidence_revision",
      label: "Confidence revision",
      status: "done",
    });
  }
  return reportConsultationAuditStep({
    detail: `Latest retained confidence is ${latestConfidence}.`,
    key: "confidence_revision",
    label: "Confidence revision",
    status: "done",
  });
}

function reportConsultationFinalDecisionStep(
  subReport: LinkedSubReport,
  readiness: SubReportDiagnosisReadiness,
  state?: ReportDiagnosisState,
): ReportConsultationAuditStep {
  if (!state) {
    return reportConsultationAuditStep({
      detail:
        "Final decision waits for an AI diagnosis room to produce the initial report.",
      key: "final_decision",
      label: "Final decision",
      status: "pending",
    });
  }
  if (readiness.status === "failed") {
    return reportConsultationAuditStep({
      detail: readiness.statusDetail,
      key: "final_decision",
      label: "Final decision",
      status: "attention",
    });
  }
  if (readiness.status === "needs_evidence") {
    return reportConsultationAuditStep({
      detail:
        "Final decision waits for the operator to provide the requested evidence and rerun AI review.",
      key: "final_decision",
      label: "Final decision",
      status: "pending",
    });
  }
  if (readiness.status === "human_review") {
    return reportConsultationAuditStep({
      detail:
        "AI conclusion is ready for operator confirmation before final report delivery.",
      key: "final_decision",
      label: "Final decision",
      status: "pending",
    });
  }
  if (diagnosisStateConfirmed(state)) {
    return reportConsultationAuditStep({
      detail: `Operator confirmed the AI conclusion${
        diagnosisStateConclusionVersion(state)
          ? ` as ${diagnosisStateConclusionVersion(state)}`
          : ""
      }.`,
      key: "final_decision",
      label: "Final decision",
      status: "done",
    });
  }
  return reportConsultationAuditStep({
    detail: `${subReport.title} has no open AI evidence work.`,
    key: "final_decision",
    label: "Final decision",
    status: "done",
  });
}

function reportConsultationAuditStep({
  detail,
  key,
  label,
  status,
}: {
  detail: string;
  key: ReportConsultationAuditStepKey;
  label: string;
  status: ReportConsultationAuditStepStatus;
}): ReportConsultationAuditStep {
  return {
    detail,
    key,
    label,
    status,
    statusLabel: reportConsultationAuditStepStatusLabel(status),
  };
}

function reportConsultationAuditStepStatusLabel(
  status: ReportConsultationAuditStepStatus,
) {
  switch (status) {
    case "attention":
      return "Attention";
    case "done":
      return "Done";
    case "pending":
      return "Pending";
  }
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
    detail: evidenceRequestFollowUpDetail(request),
    evidenceSnapshotID: subReport.evidence_snapshot_id,
    kind: "evidence_request",
    label: request.reason,
    priority: "action",
    request,
    subReportID: subReport.id,
    subReportTitle: subReport.title,
  };
}

function evidenceRequestFollowUpDetail(request: ReportEvidenceRequest): string {
  const parts = [
    request.tool,
    request.query ? `query ${request.query}` : undefined,
    request.template_id !== undefined ? `template #${request.template_id}` : undefined,
    request.alert_source_profile_id !== undefined
      ? `source #${request.alert_source_profile_id}`
      : undefined,
    request.window_seconds !== undefined ? `window ${request.window_seconds}s` : undefined,
    request.step_seconds !== undefined ? `step ${request.step_seconds}s` : undefined,
    request.limit !== undefined ? `limit ${request.limit}` : undefined,
  ].filter((part): part is string => part !== undefined);
  return parts.join(" / ");
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

function reportReviewQueueBlockingReason(input: {
  currentCollectionSuggestions: number;
  currentExecutableEvidenceRequests: number;
  currentMissingEvidence: number;
  failedSubReports: number;
  pendingSubReports: number;
}) {
  if (input.failedSubReports > 0) {
    return `Resolve ${input.failedSubReports} failed diagnosis ${plural(
      input.failedSubReports,
      "room",
    )} before confirming.`;
  }
  if (input.pendingSubReports > 0) {
    return `Start or continue AI diagnosis for ${input.pendingSubReports} ${plural(
      input.pendingSubReports,
      "linked subreport",
    )}.`;
  }
  if (
    input.currentMissingEvidence > 0 ||
    input.currentExecutableEvidenceRequests > 0
  ) {
    return `Resolve ${blockingEvidenceWorkDescription(input)}.`;
  }
  return "";
}

function blockingEvidenceWorkDescription(input: {
  currentExecutableEvidenceRequests: number;
  currentMissingEvidence: number;
}) {
  const parts: string[] = [];
  if (input.currentMissingEvidence > 0) {
    parts.push(
      `${input.currentMissingEvidence} missing evidence ${plural(
        input.currentMissingEvidence,
        "item",
      )}`,
    );
  }
  if (input.currentExecutableEvidenceRequests > 0) {
    parts.push(
      `${input.currentExecutableEvidenceRequests} executable evidence ${plural(
        input.currentExecutableEvidenceRequests,
        "task",
      )}`,
    );
  }
  return joinedEvidenceParts(parts);
}

function blockingEvidenceWorkVerb(input: {
  currentExecutableEvidenceRequests: number;
  currentMissingEvidence: number;
}) {
  const count =
    input.currentExecutableEvidenceRequests + input.currentMissingEvidence;
  return count === 1 ? "remains" : "remain";
}

function residualEvidenceWorkDescription(input: {
  currentCollectionSuggestions: number;
}) {
  if (input.currentCollectionSuggestions === 0) {
    return "No residual collection suggestions";
  }
  return `${input.currentCollectionSuggestions} residual collection ${plural(
    input.currentCollectionSuggestions,
    "suggestion",
  )}`;
}

function residualEvidenceWorkVerb(input: { currentCollectionSuggestions: number }) {
  return input.currentCollectionSuggestions === 1 ? "remains" : "remain";
}

function joinedEvidenceParts(parts: string[]) {
  if (parts.length === 0) {
    return "no evidence work";
  }
  if (parts.length === 1) {
    return parts[0]!;
  }
  return `${parts.slice(0, -1).join(", ")} and ${parts.at(-1)}`;
}

function reportReviewQueueDetail(input: {
  attention: number;
  blockingReason: string;
  canConfirm: boolean;
  done: number;
  pending: number;
  ready: number;
  status: DiagnosisReviewStatus;
}) {
  if (input.blockingReason !== "") {
    return input.blockingReason;
  }
  if (input.canConfirm || input.ready > 0) {
    return `${input.ready} AI ${plural(input.ready, "conclusion")} ready for operator confirmation.`;
  }
  if (input.status === "complete") {
    return "All linked AI conclusions are complete with no open evidence work.";
  }
  if (input.status === "empty") {
    return "No linked subreports are available for AI review.";
  }
  if (input.pending > 0) {
    return `${input.pending} review ${plural(input.pending, "item")} still need operator action.`;
  }
  if (input.done > 0) {
    return "Reviewed AI evidence is retained for traceability.";
  }
  return "No AI review queue items are available yet.";
}

function reportReviewQueueLabel(status: DiagnosisReviewStatus) {
  switch (status) {
    case "complete":
      return "Complete";
    case "empty":
      return "No queue";
    case "failed":
      return "Blocked";
    case "human_review":
      return "Confirmable";
    case "needs_evidence":
      return "Evidence needed";
    case "pending_diagnosis":
      return "Diagnosis pending";
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
      statusDetail: "Open a diagnosis room to start AI review.",
      statusLabel: statusLabel("pending_diagnosis"),
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
    statusDetail: subReportStatusDetail(
      status,
      {
        currentCollectionSuggestions,
        currentExecutableEvidenceRequests,
        currentMissingEvidence,
      },
      state,
    ),
    statusLabel: statusLabel(status),
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
}): DiagnosisReviewStatus {
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
): DiagnosisReviewStatus {
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

function statusLabel(status: DiagnosisReviewStatus) {
  switch (status) {
    case "complete":
      return "Complete";
    case "empty":
      return "No subreports";
    case "failed":
      return "Failed";
    case "human_review":
      return "Needs confirmation";
    case "needs_evidence":
      return "Needs evidence";
    case "pending_diagnosis":
      return "Pending diagnosis";
  }
}

function reportStatusDetail(
  status: DiagnosisReviewStatus,
  input: {
    currentCollectionSuggestions: number;
    currentExecutableEvidenceRequests: number;
    currentMissingEvidence: number;
    failedSubReports: number;
    pendingSubReports: number;
    readySubReports: number;
    total: number;
  },
) {
  switch (status) {
    case "complete":
      return input.currentCollectionSuggestions > 0
        ? `All linked subreports have AI conclusions and no blocking evidence requests. ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented.`
        : "All linked subreports have AI conclusions and no open evidence requests.";
    case "empty":
      return "This report has no linked subreports to review.";
    case "failed":
      return `AI diagnosis failed for ${input.failedSubReports} ${plural(
        input.failedSubReports,
        "linked subreport",
      )}. Reopen the diagnosis room after resolving the failure.`;
    case "human_review":
      return `AI conclusion is ready for operator confirmation on ${input.readySubReports} ${plural(
        input.readySubReports,
        "subreport",
      )}.${input.currentCollectionSuggestions > 0 ? ` ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented as residual guidance.` : ""}`;
    case "needs_evidence":
      return `AI is waiting for ${blockingEvidenceWorkDescription(input)}.${input.currentCollectionSuggestions > 0 ? ` ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented but do not block confirmation.` : ""}`;
    case "pending_diagnosis":
      return `${input.pendingSubReports} of ${input.total} ${plural(
        input.total,
        "linked subreport",
      )} still need AI diagnosis.`;
  }
}

function subReportStatusDetail(
  status: DiagnosisReviewStatus,
  input: {
    currentCollectionSuggestions: number;
    currentExecutableEvidenceRequests: number;
    currentMissingEvidence: number;
  },
  state?: ReportDiagnosisState,
) {
  switch (status) {
    case "complete":
      return input.currentCollectionSuggestions > 0
        ? `AI conclusion is complete with no blocking evidence requests. ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented.`
        : "AI conclusion is complete with no open evidence requests.";
    case "empty":
      return "No subreport is available for AI review.";
    case "failed": {
      const reason = state ? diagnosisStateFailureReason(state)?.trim() : "";
      return reason
        ? `AI diagnosis failed before a final conclusion: ${reason}.`
        : "AI diagnosis failed before a final conclusion.";
    }
    case "human_review":
      return input.currentCollectionSuggestions > 0
        ? `AI conclusion is ready for operator confirmation. ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented as residual guidance.`
        : "AI conclusion is ready for operator confirmation.";
    case "needs_evidence":
      return `AI requested ${blockingEvidenceWorkDescription(input)}.${input.currentCollectionSuggestions > 0 ? ` ${residualEvidenceWorkDescription(input)} ${residualEvidenceWorkVerb(input)} documented but do not block confirmation.` : ""}`;
    case "pending_diagnosis":
      return "Open a diagnosis room to start AI review.";
  }
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

function plural(count: number, label: string) {
  return count === 1 ? label : `${label}s`;
}
