import type {
  DiagnosisConsultationEvidenceRequest,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisFinalConclusion,
  DiagnosisSupplementalEvidenceRecord
} from "./types";
import {
  supplementalEvidenceReviewedByAssistantSequence,
  type DiagnosisSupplementalEvidenceReassessmentInput
} from "./supplemental-evidence";

type DiagnosisReviewQueueStatus = "attention" | "pending" | "ready" | "done";

export type DiagnosisReviewQueueItem =
  | {
      detail: string;
      key: string;
      kind: "collection_result";
      recoveryRequest?: DiagnosisConsultationEvidenceRequest;
      retryable: boolean;
      result: DiagnosisEvidenceCollectionResult;
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
    }
  | {
      detail: string;
      key: string;
      kind: "supplemental_evidence";
      request: DiagnosisConsultationEvidenceRequest;
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
    }
  | {
      detail: string;
      key: string;
      kind: "supplemental_evidence_record";
      record: DiagnosisSupplementalEvidenceRecord;
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
      unresolvedRequest?: DiagnosisConsultationEvidenceRequest;
    }
  | {
      detail: string;
      key: string;
      kind: "executable_evidence";
      request: DiagnosisEvidenceRequest;
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
    }
  | {
      detail: string;
      key: string;
      kind: "confirm";
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
    }
  | {
      detail: string;
      key: string;
      kind: "continue";
      status: DiagnosisReviewQueueStatus;
      tag: string;
      title: string;
    };

type DiagnosisSupplementalEvidenceRecordQueueItem = Extract<
  DiagnosisReviewQueueItem,
  { kind: "supplemental_evidence_record" }
>;

export type DiagnosisReviewQueueSummary = {
  attention: number;
  blockingReason: string;
  canConfirm: boolean;
  done: number;
  message: string;
  pending: number;
  ready: number;
  total: number;
};

export type DiagnosisReviewQueueActionPlan = {
  actions: DiagnosisReviewQueueActionPlanItem[];
  message: string;
  remaining: number;
  status: "blocked" | "ready" | "pending" | "done";
};

export type DiagnosisReviewQueueActionGate = {
  disabled: boolean;
  kind: "blocked" | "connection" | "ready";
  reason: string;
};

type DiagnosisReviewQueueConfidenceProgress = {
  label: string;
  status: "declined" | "improved" | "stable" | "unknown";
};

export type DiagnosisReviewQueuePostEvidenceStatus = {
  color: "error" | "processing" | "success" | "warning" | "default";
  detail: string;
  label: string;
  reviewed: number;
  status: "none" | "submitted" | "blocked" | "ready";
  submitted: number;
  unresolved: number;
};

type DiagnosisReviewQueueTaskPhaseStatus =
  | "attention"
  | "done"
  | "pending"
  | "ready";

type DiagnosisReviewQueueTaskPhaseKey =
  | "collect_evidence"
  | "supply_operator_evidence"
  | "ai_reassessment"
  | "confirm_conclusion";

export type DiagnosisReviewQueueTaskPhaseAction = {
  itemKey?: string;
  kind:
    | "confirm"
    | "request_reassessment"
    | "use_evidence_plan"
    | "use_follow_up";
  label: string;
};

type DiagnosisReviewQueueTaskPhase = {
  action?: DiagnosisReviewQueueTaskPhaseAction;
  detail: string;
  key: DiagnosisReviewQueueTaskPhaseKey;
  label: string;
  status: DiagnosisReviewQueueTaskPhaseStatus;
  statusLabel: string;
};

export type DiagnosisReviewQueueTaskProgress = {
  completed: number;
  percent: number;
  phases: DiagnosisReviewQueueTaskPhase[];
  status: "blocked" | "done" | "pending" | "ready";
  statusLabel: string;
  summary: string;
  total: number;
};

type DiagnosisReviewQueueActionPlanItem = {
  detail: string;
  key: string;
  status: DiagnosisReviewQueueStatus;
  tag: string;
  title: string;
};

export type DiagnosisReviewQueueInput = {
  canConfirmConclusion: boolean;
  collectionResults: DiagnosisEvidenceCollectionResult[];
  confidence?: string;
  confidenceProgress?: DiagnosisReviewQueueConfidenceProgress;
  conclusionStatus?: string;
  evidenceRequests: DiagnosisEvidenceRequest[];
  latestAssistantSequence?: number;
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[];
  evidenceCollectionSuggestions: DiagnosisConsultationEvidenceRequest[];
  requiresHumanReview: boolean;
  supplementalEvidence?: DiagnosisSupplementalEvidenceRecord[];
};

export function finalConclusionReviewQueueInput(input: {
  canConfirmConclusion?: boolean;
  collectionResults?: DiagnosisEvidenceCollectionResult[];
  finalConclusion: DiagnosisFinalConclusion | null | undefined;
  supplementalEvidence?: DiagnosisSupplementalEvidenceRecord[];
}): DiagnosisReviewQueueInput | null {
  if (!input.finalConclusion || input.finalConclusion.status !== "available") {
    return null;
  }
  return {
    canConfirmConclusion: input.canConfirmConclusion ?? false,
    collectionResults: input.collectionResults ?? [],
    confidence: input.finalConclusion.confidence,
    conclusionStatus: "ready_for_review",
    evidenceCollectionSuggestions:
      input.finalConclusion.evidence_collection_suggestions ?? [],
    evidenceRequests: input.finalConclusion.evidence_requests ?? [],
    latestAssistantSequence: input.finalConclusion.assistant_sequence,
    missingEvidenceRequests:
      input.finalConclusion.missing_evidence_requests ?? [],
    requiresHumanReview: input.finalConclusion.requires_human_review ?? false,
    supplementalEvidence: input.supplementalEvidence ?? []
  };
}

export function diagnosisReviewQueueItems(input: DiagnosisReviewQueueInput): DiagnosisReviewQueueItem[] {
  const failedResults = input.collectionResults.filter((result) => collectionResultNeedsAttention(result.status));
  const completedResults = input.collectionResults.filter((result) => result.status.toLowerCase() === "collected");
  const nonTerminalResults = input.collectionResults.filter((result) => {
    const status = result.status.toLowerCase();
    return status !== "collected" && !collectionResultNeedsAttention(status);
  });
  const supplementalEvidenceRecordItems = supplementalEvidenceRecordQueueItems(
    input.supplementalEvidence ?? [],
    unresolvedMissingEvidenceRequests(input),
    input.latestAssistantSequence
  );
  const unresolvedSupplementalEvidenceRecordItems = supplementalEvidenceRecordItems.filter(
    (item) => item.status === "attention"
  );
  const pendingSupplementalEvidenceRecordItems = supplementalEvidenceRecordItems.filter(
    (item) => item.status === "pending"
  );
  const retainedSupplementalEvidenceRecordItems = supplementalEvidenceRecordItems.filter(
    (item) => item.status !== "attention" && item.status !== "pending"
  );
  const activeSupplementalEvidenceTopics = new Set(
    [
      ...unresolvedSupplementalEvidenceRecordItems,
      ...pendingSupplementalEvidenceRecordItems
    ].map((item) =>
      consultationEvidenceRequestKey(item.record)
    )
  );
  const standaloneMissingEvidenceRequests = unresolvedMissingEvidenceRequests(input).filter(
    (request) => !activeSupplementalEvidenceTopics.has(consultationEvidenceRequestKey(request))
  );

  const items: DiagnosisReviewQueueItem[] = [
    ...failedResults.map(collectionResultQueueItem),
    ...unresolvedSupplementalEvidenceRecordItems,
    ...pendingSupplementalEvidenceRecordItems,
    ...standaloneMissingEvidenceRequests.map((request, index) =>
      supplementalEvidenceQueueItem(request, index, "Missing evidence", "attention")
    ),
    ...input.evidenceCollectionSuggestions.map((request, index) =>
      supplementalEvidenceQueueItem(request, index, "Collection suggestion", "pending")
    )
  ];

  items.push(...pendingDiagnosisEvidenceRequests(input.evidenceRequests, input.collectionResults).map(executableEvidenceQueueItem));

  if (input.canConfirmConclusion && diagnosisReviewQueueBlockingReason(input) === "") {
    items.push({
      detail: `AI marked the diagnosis ${normalizedConclusionStatus(input.conclusionStatus)}.`,
      key: "confirm-conclusion",
      kind: "confirm",
      status: "ready",
      tag: "ready",
      title: "Confirm conclusion"
    });
  }

  items.push(...nonTerminalResults.map(collectionResultQueueItem));
  items.push(...retainedSupplementalEvidenceRecordItems);
  items.push(...completedResults.map(collectionResultQueueItem));

  if (items.length === 0) {
    items.push({
      detail: input.requiresHumanReview
        ? "Review the latest assistant response and add operator evidence if needed."
        : "Send another diagnosis turn if more investigation is needed.",
      key: "continue-diagnosis",
      kind: "continue",
      status: "pending",
      tag: "next",
      title: input.requiresHumanReview ? "Review with operator" : "Continue diagnosis"
    });
  }

  return items;
}

export function diagnosisReviewQueueSummary(
  items: DiagnosisReviewQueueItem[],
  input: DiagnosisReviewQueueInput
): DiagnosisReviewQueueSummary {
  const attention = items.filter((item) => item.status === "attention").length;
  const pending = items.filter((item) => item.status === "pending").length;
  const ready = items.filter((item) => item.status === "ready").length;
  const done = items.filter((item) => item.status === "done").length;
  const blockingReason = diagnosisReviewQueueBlockingReason(input);
  const canConfirm = input.canConfirmConclusion && blockingReason === "";
  return {
    attention,
    blockingReason,
    canConfirm,
    done,
    message: reviewQueueSummaryMessage({
      attention,
      blockingReason,
      canConfirm,
      done,
      pending,
      ready
    }),
    pending,
    ready,
    total: items.length
  };
}

export function diagnosisReviewQueueActionPlan(
  items: DiagnosisReviewQueueItem[],
  summary: DiagnosisReviewQueueSummary
): DiagnosisReviewQueueActionPlan {
  const actionable = items.filter((item) => item.status !== "done");
  const hasOpenReviewWork = actionable.some((item) => item.kind !== "confirm");
  const actions = actionable.slice(0, 3).map((item) => ({
    detail: item.detail,
    key: item.key,
    status: item.status,
    tag: item.tag,
    title: item.title
  }));
  const remaining = Math.max(0, actionable.length - actions.length);

  if (summary.blockingReason !== "") {
    return {
      actions,
      message: summary.blockingReason,
      remaining,
      status: "blocked"
    };
  }
  if (summary.canConfirm) {
    if (hasOpenReviewWork) {
      return {
        actions,
        message:
          "Review the listed evidence suggestions before retaining the conclusion; confirm only after accepting any residual uncertainty.",
        remaining,
        status: "pending"
      };
    }
    return {
      actions,
      message: "Review the ready confirmation item, then retain the conclusion.",
      remaining,
      status: "ready"
    };
  }
  if (actions.length > 0) {
    return {
      actions,
      message: "Complete the listed operator actions before final confirmation.",
      remaining,
      status: "pending"
    };
  }
  return {
    actions: [],
    message: "No review actions are pending for the latest AI turn.",
    remaining: 0,
    status: "done"
  };
}

export function diagnosisReviewQueueActionGate({
  actionDisabledReason,
  connected,
}: {
  actionDisabledReason: string;
  connected: boolean;
}): DiagnosisReviewQueueActionGate {
  const reason = actionDisabledReason.trim();
  if (reason !== "") {
    return {
      disabled: true,
      kind: reviewQueueDisabledReasonIsConnection(reason)
        ? "connection"
        : "blocked",
      reason
    };
  }
  if (!connected) {
    return {
      disabled: true,
      kind: "connection",
      reason:
        "Open a live diagnosis-room connection before running review queue actions."
    };
  }
  return {
    disabled: false,
    kind: "ready",
    reason: ""
  };
}

export function diagnosisReviewQueueConnectionGateAllowsPreparation({
  actionDisabledReason,
  connected,
}: {
  actionDisabledReason: string;
  connected: boolean;
}): boolean {
  if (connected) {
    return false;
  }
  const normalized = actionDisabledReason.toLowerCase();
  return (
    normalized.includes("connect to a diagnosis room") ||
    normalized.includes("open a live diagnosis-room connection")
  );
}

export function diagnosisReviewQueueReassessmentInput(
  input: DiagnosisReviewQueueInput
): DiagnosisSupplementalEvidenceReassessmentInput {
  return {
    collectionResults: input.collectionResults,
    ...(input.latestAssistantSequence !== undefined
      ? { latestAssistantSequence: input.latestAssistantSequence }
      : {}),
    records: input.supplementalEvidence ?? []
  };
}

export function diagnosisReviewQueueTaskProgress(
  items: DiagnosisReviewQueueItem[],
  summary: DiagnosisReviewQueueSummary,
  postEvidenceStatus: DiagnosisReviewQueuePostEvidenceStatus
): DiagnosisReviewQueueTaskProgress {
  const evidenceCollectionPhase = reviewQueueEvidenceCollectionPhase(items);
  const supplementalEvidencePhase = reviewQueueSupplementalEvidencePhase(items);
  const hasRetainedCollectionEvidence = items.some(
    (item) => item.kind === "collection_result" && item.status === "done"
  );
  const phases: DiagnosisReviewQueueTaskPhase[] = [
    evidenceCollectionPhase,
    supplementalEvidencePhase,
    reviewQueueAIReassessmentPhase({
      evidenceCollectionPhase,
      hasRetainedCollectionEvidence,
      postEvidenceStatus,
      supplementalEvidencePhase,
      summary
    }),
    reviewQueueConfirmationPhase(summary)
  ];
  const completed = phases.filter((phase) => phase.status === "done").length;
  const status = reviewQueueTaskProgressStatus(phases);
  return {
    completed,
    percent: Math.round((completed / phases.length) * 100),
    phases,
    status,
    statusLabel: reviewQueueTaskProgressStatusLabel(status),
    summary: reviewQueueTaskProgressSummary(phases, status),
    total: phases.length
  };
}

export function diagnosisReviewQueuePostEvidenceStatus(
  input: DiagnosisReviewQueueInput
): DiagnosisReviewQueuePostEvidenceStatus {
  const records = input.supplementalEvidence ?? [];
  const submitted = records.length;
  if (submitted === 0) {
    return {
      color: "default",
      detail: "No supplemental evidence has been submitted for AI review.",
      label: "No supplemental evidence",
      reviewed: 0,
      status: "none",
      submitted: 0,
      unresolved: 0
    };
  }

  const unresolvedRecords = supplementalEvidenceRecordQueueItems(
    records,
    unresolvedMissingEvidenceRequests(input),
    input.latestAssistantSequence
  ).filter((item) => item.status === "attention");
  const reviewed = records.filter((record) =>
    supplementalEvidenceReviewedByLatestAssistant(
      record,
      input.latestAssistantSequence
    )
  ).length;
  const blockingReason = diagnosisReviewQueueBlockingReason(input);
  const confidenceDetail = reviewQueueConfidenceDetail(input);
  if (
    blockingReason ===
    "Wait for AI reassessment of submitted supplemental evidence before confirming."
  ) {
    return {
      color: "processing",
      detail: `${submitted} supplemental evidence update${submitted === 1 ? "" : "s"} submitted; ${reviewed} reviewed by the latest AI turn. ${confidenceDetail} Continue the conversation so AI can reassess the submitted evidence before confirmation.`,
      label: "Supplemental evidence awaiting AI review",
      reviewed,
      status: "submitted",
      submitted,
      unresolved: 0
    };
  }
  if (unresolvedRecords.length > 0 || blockingReason !== "") {
    return {
      color: "warning",
      detail: `${submitted} supplemental evidence update${submitted === 1 ? "" : "s"} submitted; ${reviewed} reviewed by the latest AI turn. ${confidenceDetail} ${blockingReason || "Resolve remaining review blockers before confirmation."}`,
      label: "Supplemental evidence still blocking",
      reviewed,
      status: "blocked",
      submitted,
      unresolved: unresolvedRecords.length
    };
  }
  if (input.canConfirmConclusion) {
    return {
      color: "success",
      detail: `${submitted} supplemental evidence update${submitted === 1 ? "" : "s"} submitted; ${reviewed} reviewed by the latest AI turn. ${confidenceDetail} No remaining review blockers; the conclusion can be confirmed and retained.`,
      label: "Supplemental evidence reviewed",
      reviewed,
      status: "ready",
      submitted,
      unresolved: 0
    };
  }
  return {
    color: "processing",
    detail: `${submitted} supplemental evidence update${submitted === 1 ? "" : "s"} submitted; ${reviewed} reviewed by the latest AI turn. ${confidenceDetail} Continue the consultation until AI marks the conclusion ready for review.`,
    label: "Supplemental evidence under review",
    reviewed,
    status: "submitted",
    submitted,
    unresolved: 0
  };
}

export function diagnosisReviewQueueNextAction(input: DiagnosisReviewQueueInput): string {
  const blockingReason = diagnosisReviewQueueBlockingReason(input);
  if (blockingReason.includes("evidence collection")) {
    return "Resolve evidence collection";
  }
  if (
    blockingReason === "Resolve missing evidence requests before confirming." ||
    blockingReason.startsWith("Resolve latest request for ")
  ) {
    return "Collect missing evidence";
  }
  if (blockingReason === "Collect planned executable evidence before confirming.") {
    return "Run evidence collection";
  }

  if (input.evidenceCollectionSuggestions.length > 0) {
    return "Review collection suggestions";
  }

  if (diagnosisReviewQueueHasCollectedEvidence(input.collectionResults)) {
    return "Ask AI to reassess";
  }

  const status = input.conclusionStatus?.trim().toLowerCase();
  if (status === "final" || status === "ready_for_review") {
    return "Ready for confirmation";
  }
  if (input.evidenceCollectionSuggestions.length > 0) {
    return "Review collection suggestions";
  }
  if (input.requiresHumanReview) {
    return "Review with operator";
  }
  return "Continue diagnosis";
}

function reviewQueueEvidenceCollectionPhase(
  items: DiagnosisReviewQueueItem[]
): DiagnosisReviewQueueTaskPhase {
  const collectionItems = items.filter(
    (item) =>
      item.kind === "collection_result" || item.kind === "executable_evidence"
  );
  if (collectionItems.length === 0) {
    return reviewQueueTaskPhase({
      detail: "No executable evidence collection is requested for the latest AI turn.",
      key: "collect_evidence",
      label: "Collect evidence",
      status: "done"
    });
  }
  const attention = collectionItems.filter(
    (item) => item.status === "attention"
  ).length;
  if (attention > 0) {
    return reviewQueueTaskPhase({
      action: reviewQueueEvidenceCollectionAction(collectionItems),
      detail: `${attention} evidence collection ${plural(
        attention,
        "item"
      )} ${needsVerb(attention)} operator recovery before confirmation.`,
      key: "collect_evidence",
      label: "Collect evidence",
      status: "attention"
    });
  }
  const pending = collectionItems.filter(
    (item) => item.status !== "done"
  ).length;
  if (pending > 0) {
    return reviewQueueTaskPhase({
      action: reviewQueueEvidenceCollectionAction(collectionItems),
      detail: `${pending} executable evidence ${plural(
        pending,
        "task"
      )} still ${needsVerb(pending)} collection.`,
      key: "collect_evidence",
      label: "Collect evidence",
      status: "pending"
    });
  }
  return reviewQueueTaskPhase({
    detail: `${collectionItems.length} evidence collection ${plural(
      collectionItems.length,
      "item"
    )} retained for AI review.`,
    key: "collect_evidence",
    label: "Collect evidence",
    status: "done"
  });
}

function reviewQueueSupplementalEvidencePhase(
  items: DiagnosisReviewQueueItem[]
): DiagnosisReviewQueueTaskPhase {
  const supplementalItems = items.filter(
    (item) =>
      item.kind === "supplemental_evidence" ||
      item.kind === "supplemental_evidence_record"
  );
  if (supplementalItems.length === 0) {
    return reviewQueueTaskPhase({
      detail: "No operator-supplied evidence is requested for the latest AI turn.",
      key: "supply_operator_evidence",
      label: "Supply operator evidence",
      status: "done"
    });
  }
  const attention = supplementalItems.filter(
    (item) => item.status === "attention"
  ).length;
  if (attention > 0) {
    return reviewQueueTaskPhase({
      action: reviewQueueSupplementalEvidenceAction(supplementalItems),
      detail: `${attention} operator evidence ${plural(
        attention,
        "item"
      )} still blocks confirmation.`,
      key: "supply_operator_evidence",
      label: "Supply operator evidence",
      status: "attention"
    });
  }
  const pending = supplementalItems.filter(
    (item) => item.status !== "done"
  ).length;
  if (pending > 0) {
    return reviewQueueTaskPhase({
      action: reviewQueueSupplementalEvidenceAction(supplementalItems),
      detail: `${pending} operator evidence ${plural(
        pending,
        "task"
      )} still ${needsVerb(pending)} submission or review.`,
      key: "supply_operator_evidence",
      label: "Supply operator evidence",
      status: "pending"
    });
  }
  return reviewQueueTaskPhase({
    detail: `${supplementalItems.length} supplemental evidence ${plural(
      supplementalItems.length,
      "item"
    )} retained for the latest AI review.`,
    key: "supply_operator_evidence",
    label: "Supply operator evidence",
    status: "done"
  });
}

function reviewQueueAIReassessmentPhase({
  evidenceCollectionPhase,
  hasRetainedCollectionEvidence,
  postEvidenceStatus,
  supplementalEvidencePhase,
  summary
}: {
  evidenceCollectionPhase: DiagnosisReviewQueueTaskPhase;
  hasRetainedCollectionEvidence: boolean;
  postEvidenceStatus: DiagnosisReviewQueuePostEvidenceStatus;
  supplementalEvidencePhase: DiagnosisReviewQueueTaskPhase;
  summary: DiagnosisReviewQueueSummary;
}): DiagnosisReviewQueueTaskPhase {
  if (postEvidenceStatus.status === "blocked") {
    return reviewQueueTaskPhase({
      detail: postEvidenceStatus.detail,
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "attention"
    });
  }
  if (postEvidenceStatus.status === "submitted") {
    return reviewQueueTaskPhase({
      action: {
        kind: "request_reassessment",
        label: "Ask AI to reassess"
      },
      detail: postEvidenceStatus.detail,
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "pending"
    });
  }
  if (postEvidenceStatus.status === "ready") {
    return reviewQueueTaskPhase({
      detail: postEvidenceStatus.detail,
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "done"
    });
  }
  if (
    evidenceCollectionPhase.status !== "done" ||
    supplementalEvidencePhase.status !== "done"
  ) {
    return reviewQueueTaskPhase({
      detail:
        "AI reassessment waits for the open evidence tasks to be completed.",
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "pending"
    });
  }
  if (summary.canConfirm) {
    return reviewQueueTaskPhase({
      detail: "Latest AI turn is ready for operator confirmation.",
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "done"
    });
  }
  if (hasRetainedCollectionEvidence) {
    return reviewQueueTaskPhase({
      action: {
        kind: "request_reassessment",
        label: "Ask AI to reassess"
      },
      detail:
        "Collected executable evidence is retained; ask AI to reassess confidence and conclusion status if the automatic evidence follow-up did not produce a ready conclusion.",
      key: "ai_reassessment",
      label: "AI reassessment",
      status: "pending"
    });
  }
  return reviewQueueTaskPhase({
    detail: "Continue the conversation until AI marks a conclusion ready for review.",
    key: "ai_reassessment",
    label: "AI reassessment",
    status: "pending"
  });
}

function reviewQueueConfirmationPhase(
  summary: DiagnosisReviewQueueSummary
): DiagnosisReviewQueueTaskPhase {
  if (summary.canConfirm) {
    return reviewQueueTaskPhase({
      action: {
        itemKey: "confirm-conclusion",
        kind: "confirm",
        label: "Confirm"
      },
      detail: "Operator can confirm and retain the AI conclusion.",
      key: "confirm_conclusion",
      label: "Confirm conclusion",
      status: "ready"
    });
  }
  if (summary.blockingReason !== "") {
    return reviewQueueTaskPhase({
      detail: summary.blockingReason,
      key: "confirm_conclusion",
      label: "Confirm conclusion",
      status: "attention"
    });
  }
  if (summary.ready > 0) {
    return reviewQueueTaskPhase({
      action: {
        itemKey: "confirm-conclusion",
        kind: "confirm",
        label: "Confirm"
      },
      detail: "Review the ready confirmation item before retaining the conclusion.",
      key: "confirm_conclusion",
      label: "Confirm conclusion",
      status: "ready"
    });
  }
  return reviewQueueTaskPhase({
    detail: "Final confirmation waits for AI reassessment to produce a ready conclusion.",
    key: "confirm_conclusion",
    label: "Confirm conclusion",
    status: "pending"
  });
}

function reviewQueueTaskPhase({
  action,
  detail,
  key,
  label,
  status
}: {
  action?: DiagnosisReviewQueueTaskPhaseAction;
  detail: string;
  key: DiagnosisReviewQueueTaskPhaseKey;
  label: string;
  status: DiagnosisReviewQueueTaskPhaseStatus;
}): DiagnosisReviewQueueTaskPhase {
  return {
    action,
    detail,
    key,
    label,
    status,
    statusLabel: reviewQueueTaskPhaseStatusLabel(status)
  };
}

function reviewQueueEvidenceCollectionAction(
  items: DiagnosisReviewQueueItem[]
): DiagnosisReviewQueueTaskPhaseAction | undefined {
  const retryable = items.find(
    (item): item is Extract<DiagnosisReviewQueueItem, { kind: "collection_result" }> =>
      item.kind === "collection_result" && item.retryable
  );
  if (retryable) {
    return {
      itemKey: retryable.key,
      kind: "use_evidence_plan",
      label: "Retry collection"
    };
  }
  const recovery = items.find(
    (item): item is Extract<DiagnosisReviewQueueItem, { kind: "collection_result" }> =>
      item.kind === "collection_result" && item.recoveryRequest !== undefined
  );
  if (recovery) {
    return {
      itemKey: recovery.key,
      kind: "use_follow_up",
      label: "Add evidence"
    };
  }
  const executable = items.find(
    (item): item is Extract<DiagnosisReviewQueueItem, { kind: "executable_evidence" }> =>
      item.kind === "executable_evidence" && item.status !== "done"
  );
  if (executable) {
    return {
      itemKey: executable.key,
      kind: "use_evidence_plan",
      label: "Use plan"
    };
  }
  return undefined;
}

function reviewQueueSupplementalEvidenceAction(
  items: DiagnosisReviewQueueItem[]
): DiagnosisReviewQueueTaskPhaseAction | undefined {
  const unresolvedRecord = items.find(
    (
      item
    ): item is Extract<
      DiagnosisReviewQueueItem,
      { kind: "supplemental_evidence_record" }
    > =>
      item.kind === "supplemental_evidence_record" &&
      item.unresolvedRequest !== undefined
  );
  if (unresolvedRecord) {
    return {
      itemKey: unresolvedRecord.key,
      kind: "use_follow_up",
      label: "Use latest request"
    };
  }
  const request = items.find(
    (item): item is Extract<DiagnosisReviewQueueItem, { kind: "supplemental_evidence" }> =>
      item.kind === "supplemental_evidence" && item.status !== "done"
  );
  if (request) {
    return {
      itemKey: request.key,
      kind: "use_follow_up",
      label: "Use follow-up"
    };
  }
  return undefined;
}

function reviewQueueTaskPhaseStatusLabel(
  status: DiagnosisReviewQueueTaskPhaseStatus
): string {
  switch (status) {
    case "attention":
      return "Attention";
    case "done":
      return "Done";
    case "pending":
      return "Pending";
    case "ready":
      return "Ready";
  }
}

function reviewQueueTaskProgressStatus(
  phases: DiagnosisReviewQueueTaskPhase[]
): DiagnosisReviewQueueTaskProgress["status"] {
  if (phases.some((phase) => phase.status === "attention")) {
    return "blocked";
  }
  if (phases.some((phase) => phase.status === "pending")) {
    return "pending";
  }
  if (phases.some((phase) => phase.status === "ready")) {
    return "ready";
  }
  if (phases.every((phase) => phase.status === "done")) {
    return "done";
  }
  return "pending";
}

function reviewQueueTaskProgressStatusLabel(
  status: DiagnosisReviewQueueTaskProgress["status"]
): string {
  switch (status) {
    case "blocked":
      return "Evidence tasks blocked";
    case "done":
      return "Task flow complete";
    case "pending":
      return "Evidence tasks pending";
    case "ready":
      return "Conclusion ready";
  }
}

function reviewQueueTaskProgressSummary(
  phases: DiagnosisReviewQueueTaskPhase[],
  status: DiagnosisReviewQueueTaskProgress["status"]
): string {
  const activePhase =
    phases.find((phase) => phase.status === "attention") ??
    phases.find((phase) => phase.status === "pending") ??
    phases.find((phase) => phase.status === "ready");
  if (activePhase) {
    return `${activePhase.label}: ${activePhase.detail}`;
  }
  return reviewQueueTaskProgressStatusLabel(status);
}

function reviewQueueConfidenceDetail(input: DiagnosisReviewQueueInput): string {
  const confidence = input.confidence?.trim();
  const progress = input.confidenceProgress?.label.trim();
  if (progress && confidence) {
    return `${progress}. Latest confidence: ${confidence}.`;
  }
  if (progress) {
    return `${progress}.`;
  }
  if (confidence) {
    return `Latest confidence: ${confidence}.`;
  }
  return "Latest confidence is unavailable.";
}

function reviewQueueDisabledReasonIsConnection(reason: string): boolean {
  const normalized = reason.toLowerCase();
  return normalized.includes("connect to a diagnosis room");
}

function diagnosisReviewQueueHasCollectedEvidence(
  results: DiagnosisEvidenceCollectionResult[]
): boolean {
  return results.some((result) => result.status.trim().toLowerCase() === "collected");
}

export function diagnosisReviewQueueBlockingReason(input: DiagnosisReviewQueueInput): string {
  const failedResult = input.collectionResults.find((result) =>
    collectionResultNeedsAttention(result.status)
  );
  if (failedResult) {
    return `Resolve ${failedResult.tool} evidence collection before confirming.`;
  }
  if (input.missingEvidenceRequests.length > 0) {
    if (reviewedReadyConclusionHasResidualMissingEvidence(input)) {
      return "";
    }
    const unresolvedSupplementalEvidence =
      firstUnresolvedSubmittedSupplementalEvidence(
        input.supplementalEvidence ?? [],
        unresolvedMissingEvidenceRequests(input),
        input.latestAssistantSequence
      );
    if (unresolvedSupplementalEvidence) {
      return `Resolve latest request for ${unresolvedSupplementalEvidence.request.label} before confirming.`;
    }
    if (
      firstPendingSubmittedSupplementalEvidence(
        input.supplementalEvidence ?? [],
        unresolvedMissingEvidenceRequests(input),
        input.latestAssistantSequence
      )
    ) {
      return "Wait for AI reassessment of submitted supplemental evidence before confirming.";
    }
    return "Resolve missing evidence requests before confirming.";
  }
  const pendingRequests = pendingDiagnosisEvidenceRequests(input.evidenceRequests, input.collectionResults);
  if (pendingRequests.length > 0) {
    return "Collect planned executable evidence before confirming.";
  }
  return "";
}

function reviewQueueSummaryMessage(input: {
  attention: number;
  blockingReason: string;
  canConfirm: boolean;
  done: number;
  pending: number;
  ready: number;
}): string {
  if (input.blockingReason !== "") {
    return input.blockingReason;
  }
  if (input.attention > 0) {
    return `${input.attention} item${input.attention === 1 ? "" : "s"} need operator attention before final confirmation.`;
  }
  if (input.pending > 0) {
    return `${input.pending} item${input.pending === 1 ? "" : "s"} still need operator action.`;
  }
  if (input.canConfirm || input.ready > 0) {
    return "AI conclusion is ready for operator confirmation.";
  }
  if (input.done > 0) {
    return "Submitted evidence is retained for the latest AI review.";
  }
  return "Next diagnosis actions are collected from the latest assistant turn.";
}

function reviewedReadyConclusionHasResidualMissingEvidence(input: DiagnosisReviewQueueInput): boolean {
  const status = input.conclusionStatus?.trim().toLowerCase();
  const supplementalEvidence = input.supplementalEvidence ?? [];
  return (
    (status === "final" || status === "ready_for_review") &&
    input.missingEvidenceRequests.every((request) =>
      supplementalEvidence.some(
        (record) =>
          consultationEvidenceRequestKey(record) === consultationEvidenceRequestKey(request) &&
          supplementalEvidenceReviewedByLatestAssistant(
            record,
            input.latestAssistantSequence
          )
      )
    )
  );
}

function unresolvedMissingEvidenceRequests(
  input: DiagnosisReviewQueueInput
): DiagnosisConsultationEvidenceRequest[] {
  if (!reviewedReadyConclusionHasResidualMissingEvidence(input)) {
    return input.missingEvidenceRequests;
  }
  const supplementalEvidence = input.supplementalEvidence ?? [];
  return input.missingEvidenceRequests.filter(
    (request) =>
      !supplementalEvidence.some(
        (record) =>
          consultationEvidenceRequestKey(record) === consultationEvidenceRequestKey(request) &&
          supplementalEvidenceReviewedByLatestAssistant(
            record,
            input.latestAssistantSequence
          )
      )
  );
}

function supplementalEvidenceReviewedByLatestAssistant(
  record: DiagnosisSupplementalEvidenceRecord,
  latestAssistantSequence: number | undefined
): boolean {
  return supplementalEvidenceReviewedByAssistantSequence(
    record,
    latestAssistantSequence
  );
}

function firstUnresolvedSubmittedSupplementalEvidence(
  records: DiagnosisSupplementalEvidenceRecord[],
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[],
  latestAssistantSequence: number | undefined
):
  | {
      record: DiagnosisSupplementalEvidenceRecord;
      request: DiagnosisConsultationEvidenceRequest;
    }
  | undefined {
  const unresolvedTopics = new Map(
    missingEvidenceRequests.map((request) => [
      consultationEvidenceRequestKey(request),
      request
    ])
  );
  for (const record of records
    .slice()
    .sort((left, right) => right.provided_at.localeCompare(left.provided_at))) {
    const request = unresolvedTopics.get(consultationEvidenceRequestKey(record));
    if (
      request &&
      supplementalEvidenceReviewedByLatestAssistant(
        record,
        latestAssistantSequence
      )
    ) {
      return { record, request };
    }
  }
  return undefined;
}

function firstPendingSubmittedSupplementalEvidence(
  records: DiagnosisSupplementalEvidenceRecord[],
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[],
  latestAssistantSequence: number | undefined
): DiagnosisSupplementalEvidenceRecord | undefined {
  const unresolvedTopics = new Set(
    missingEvidenceRequests.map(consultationEvidenceRequestKey)
  );
  return records
    .slice()
    .sort((left, right) => right.provided_at.localeCompare(left.provided_at))
    .find(
      (record) =>
        unresolvedTopics.has(consultationEvidenceRequestKey(record)) &&
        !supplementalEvidenceReviewedByLatestAssistant(
          record,
          latestAssistantSequence
        )
    );
}

function pendingDiagnosisEvidenceRequests(
  requests: DiagnosisEvidenceRequest[],
  results: DiagnosisEvidenceCollectionResult[]
): DiagnosisEvidenceRequest[] {
  const resultKeys = new Set(results.map((result) => evidenceRequestIdentity(collectionResultRequest(result))));
  const pending = new Map<string, DiagnosisEvidenceRequest>();
  for (const request of requests) {
    const key = evidenceRequestIdentity(request);
    if (resultKeys.has(key) || pending.has(key)) {
      continue;
    }
    pending.set(key, request);
  }
  return [...pending.values()];
}

function executableEvidenceQueueItem(
  request: DiagnosisEvidenceRequest,
  index: number
): DiagnosisReviewQueueItem {
  return {
    detail: executableEvidenceQueueDetail(request),
    key: `executable:${evidenceRequestIdentity(request)}:${index}`,
    kind: "executable_evidence",
    request,
    status: "pending",
    tag: request.tool,
    title: `Collect evidence: ${request.tool}`
  };
}

function executableEvidenceQueueDetail(request: DiagnosisEvidenceRequest): string {
  const details = [`Reason: ${request.reason}.`];
  if (request.query) {
    details.push(`Query: ${request.query}.`);
  }
  if (request.window_seconds) {
    details.push(`Window: ${request.window_seconds}s.`);
  }
  if (request.limit) {
    details.push(`Limit: ${request.limit}.`);
  }
  return details.join(" ");
}

function collectionResultQueueItem(result: DiagnosisEvidenceCollectionResult): DiagnosisReviewQueueItem {
  return {
    detail: collectionResultQueueDetail(result),
    key: collectionResultKey(result),
    kind: "collection_result",
    recoveryRequest: collectionResultRecoveryRequest(result),
    retryable: collectionResultCanRetry(result),
    result,
    status: collectionResultStatus(result.status),
    tag: result.status,
    title: `${result.tool} evidence`
  };
}

function collectionResultQueueDetail(result: DiagnosisEvidenceCollectionResult): string {
  const status = result.status.toLowerCase();
  if (status === "collected") {
    const details: string[] = [];
    if (result.observed_alerts > 0) {
      details.push(`${result.observed_alerts} alert${result.observed_alerts === 1 ? "" : "s"}`);
    }
    if (result.observed_metric_series !== undefined) {
      details.push(`${result.observed_metric_series} metric series`);
    }
    return details.length > 0 ? `Collected ${details.join(" and ")}.` : "Collection completed.";
  }
  const reason = result.reason_code ? `Reason: ${result.reason_code}.` : "";
  const message = result.message ? ` Detail: ${result.message}` : "";
  return `${reason}${message}`.trim() || "Evidence collection needs operator attention.";
}

function supplementalEvidenceQueueItem(
  request: DiagnosisConsultationEvidenceRequest,
  index: number,
  title: string,
  status: DiagnosisReviewQueueStatus
): DiagnosisReviewQueueItem {
  return {
    detail: request.detail,
    key: `${status}-${request.priority}-${request.label}-${index}`,
    kind: "supplemental_evidence",
    request,
    status,
    tag: request.priority,
    title: `${title}: ${request.label}`
  };
}

function supplementalEvidenceRecordQueueItems(
  records: DiagnosisSupplementalEvidenceRecord[],
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[],
  latestAssistantSequence: number | undefined
): DiagnosisSupplementalEvidenceRecordQueueItem[] {
  const unresolvedTopics = new Map(
    missingEvidenceRequests.map((request) => [
      consultationEvidenceRequestKey(request),
      request
    ])
  );
  return records
    .slice()
    .sort((left, right) => right.provided_at.localeCompare(left.provided_at))
    .slice(0, 5)
    .map((record) => {
      const unresolvedRequest = unresolvedTopics.get(consultationEvidenceRequestKey(record));
      const reviewedByLatest = supplementalEvidenceReviewedByLatestAssistant(
        record,
        latestAssistantSequence
      );
      const stillMissing = unresolvedRequest !== undefined && reviewedByLatest;
      const waitingForReview =
        unresolvedRequest !== undefined && !reviewedByLatest;
      return {
        detail: supplementalEvidenceRecordQueueDetail(
          record,
          stillMissing,
          waitingForReview,
          unresolvedRequest
        ),
        key: `supplemental-record:${record.user_message_id}:${record.assistant_message_id}`,
        kind: "supplemental_evidence_record",
        record,
        status: stillMissing ? "attention" : waitingForReview ? "pending" : "done",
        tag: unresolvedRequest?.priority ?? record.priority,
        title: stillMissing
          ? `Submitted evidence still requested: ${record.label}`
          : waitingForReview
            ? `Submitted evidence awaiting AI review: ${record.label}`
            : `Submitted evidence: ${record.label}`,
        unresolvedRequest
      };
    });
}

function supplementalEvidenceRecordQueueDetail(
  record: DiagnosisSupplementalEvidenceRecord,
  stillMissing: boolean,
  waitingForReview: boolean,
  unresolvedRequest: DiagnosisConsultationEvidenceRequest | undefined
): string {
  const turnDetail =
    record.user_sequence > 0 && record.assistant_sequence > 0
      ? `Submitted in user turn ${record.user_sequence} and reviewed by assistant turn ${record.assistant_sequence}.`
      : "Submitted supplemental evidence is retained for AI review.";
  if (stillMissing) {
    const latestRequestDetail =
      unresolvedRequest &&
      normalizedEvidenceTopicPart(unresolvedRequest.detail) !==
        normalizedEvidenceTopicPart(record.detail)
        ? ` Latest request: ${unresolvedRequest.detail}`
      : "";
    return `${record.detail} AI still lists this evidence as missing after the latest review.${latestRequestDetail} ${turnDetail}`;
  }
  if (waitingForReview) {
    return `${record.detail} Submitted evidence matches the latest missing evidence request and is waiting for AI reassessment. Continue the conversation to update confidence before confirmation.`;
  }
  return `${record.detail} ${turnDetail}`;
}

function consultationEvidenceRequestKey(
  item: DiagnosisConsultationEvidenceRequest | DiagnosisSupplementalEvidenceRecord
): string {
  return [
    normalizedEvidenceTopicPart(item.label),
    normalizedEvidenceTopicPart(item.detail)
  ].join("\n");
}

function normalizedEvidenceTopicPart(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/g, " ");
}

function collectionResultStatus(status: string): DiagnosisReviewQueueStatus {
  const normalized = status.toLowerCase();
  if (normalized === "collected") {
    return "done";
  }
  if (collectionResultNeedsAttention(normalized)) {
    return "attention";
  }
  return "pending";
}

function collectionResultNeedsAttention(status: string): boolean {
  const normalized = status.toLowerCase();
  return (
    normalized === "failed" ||
    normalized === "skipped" ||
    normalized === "unsupported"
  );
}

function collectionResultCanRetry(result: DiagnosisEvidenceCollectionResult): boolean {
  const status = result.status.toLowerCase();
  if (status === "failed") {
    return true;
  }
  if (status !== "skipped") {
    return false;
  }
  switch (result.reason_code) {
    case "collection_timed_out":
    case "provider_failed":
    case "provider_unavailable":
    case "source_unavailable":
      return true;
    default:
      return false;
  }
}

function collectionResultRecoveryRequest(
  result: DiagnosisEvidenceCollectionResult
): DiagnosisConsultationEvidenceRequest | undefined {
  if (collectionResultStatus(result.status) !== "attention" || collectionResultCanRetry(result)) {
    return undefined;
  }
  const request = collectionResultRequest(result);
  return {
    detail: collectionResultRecoveryDetail(result, request),
    label: `${result.tool} evidence recovery`,
    priority: "high",
    source_request: request
  };
}

function collectionResultRecoveryDetail(
  result: DiagnosisEvidenceCollectionResult,
  request: DiagnosisEvidenceRequest
): string {
  const reason = result.reason_code ? `Reason: ${result.reason_code}.` : "";
  const message = result.message ? ` Detail: ${result.message}` : "";
  const requested = request.reason ? ` Original request: ${request.reason}.` : "";
  return `${reason}${message}${requested} Provide verified alternative evidence or explain why this evidence cannot be collected as requested.`
    .trim()
    .replace(/\s+/g, " ");
}

function collectionResultKey(result: DiagnosisEvidenceCollectionResult): string {
  return [
    "collection",
    result.tool,
    result.status,
    result.reason_code,
    result.collected_at,
    result.template_id ?? "no-template",
    result.query ?? "no-query"
  ].join(":");
}

function collectionResultRequest(result: DiagnosisEvidenceCollectionResult): DiagnosisEvidenceRequest {
  return {
    ...result.request,
    alert_source_profile_id:
      result.alert_source_profile_id ?? result.request.alert_source_profile_id,
    limit: result.limit ?? result.request.limit,
    query: result.query ?? result.request.query,
    step_seconds: result.step_seconds ?? result.request.step_seconds,
    template_id: result.template_id ?? result.request.template_id,
    tool: result.tool || result.request.tool,
    window_seconds: result.window_seconds ?? result.request.window_seconds
  };
}

function evidenceRequestIdentity(request: DiagnosisEvidenceRequest): string {
  return [
    request.template_id ?? "no-template",
    request.alert_source_profile_id ?? "no-profile",
    request.tool,
    request.reason,
    request.query ?? "no-query",
    request.window_seconds ?? "no-window",
    request.step_seconds ?? "no-step",
    request.limit ?? "no-limit"
  ].join(":");
}

function normalizedConclusionStatus(status: string | undefined): string {
  const value = status?.trim();
  return value && value !== "" ? value : "ready for review";
}

function plural(count: number, label: string): string {
  return count === 1 ? label : `${label}s`;
}

function needsVerb(count: number): string {
  return count === 1 ? "needs" : "need";
}
