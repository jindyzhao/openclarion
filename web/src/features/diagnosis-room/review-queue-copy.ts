import type { useTranslations } from "next-intl";

import {
  diagnosisReviewQueueBlocker,
  type DiagnosisReviewQueueActionGate,
  type DiagnosisReviewQueueActionPlan,
  type DiagnosisReviewQueueInput,
  type DiagnosisReviewQueueItem,
  type DiagnosisReviewQueuePostEvidenceStatus,
  type DiagnosisReviewQueueSummary,
  type DiagnosisReviewQueueTaskProgress,
} from "./review-queue";
import {
  localizeDiagnosisRoomStatus,
  type DiagnosisRoomStatusTranslator,
} from "./status-copy";

export type DiagnosisReviewQueueTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.reviewQueue">
>;

export function localizeDiagnosisReviewQueueItem(
  item: DiagnosisReviewQueueItem,
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): DiagnosisReviewQueueItem {
  switch (item.kind) {
    case "collection_result":
      return {
        ...item,
        detail: localizeCollectionResultDetail(item.result, t),
        tag: localizeDiagnosisRoomStatus(item.result.status, tStatus),
        title: t("collectionResultTitle", { tool: item.result.tool }),
      };
    case "executable_evidence":
      return {
        ...item,
        detail: localizeExecutableEvidenceDetail(item.request, t),
        tag: item.request.tool,
        title: t("executableEvidenceTitle", { tool: item.request.tool }),
      };
    case "supplemental_evidence":
      return {
        ...item,
        detail: item.request.detail,
        tag: localizeDiagnosisRoomStatus(item.request.priority, tStatus),
        title: t(
          item.status === "attention"
            ? "missingEvidenceTitle"
            : "collectionSuggestionTitle",
          { label: item.request.label },
        ),
      };
    case "supplemental_evidence_record":
      return {
        ...item,
        detail: localizeSupplementalEvidenceRecordDetail(item, t),
        tag: localizeDiagnosisRoomStatus(
          item.unresolvedRequest?.priority ?? item.record.priority,
          tStatus,
        ),
        title: t(
          item.status === "attention"
            ? "submittedEvidenceStillRequestedTitle"
            : item.status === "pending"
              ? "submittedEvidenceAwaitingTitle"
              : "submittedEvidenceTitle",
          { label: item.record.label },
        ),
      };
    case "confirm":
      return {
        ...item,
        detail: t("confirmDetail", {
          status: localizeDiagnosisRoomStatus(
            input.conclusionStatus ?? "ready_for_review",
            tStatus,
          ),
        }),
        tag: t("statusReady"),
        title: t("confirmTitle"),
      };
    case "continue":
      return {
        ...item,
        detail: t(
          input.requiresHumanReview
            ? "continueOperatorDetail"
            : "continueDiagnosisDetail",
        ),
        tag: t("statusNext"),
        title: t(
          input.requiresHumanReview
            ? "continueOperatorTitle"
            : "continueDiagnosisTitle",
        ),
      };
  }
}

export function localizeDiagnosisReviewQueueSummary(
  summary: DiagnosisReviewQueueSummary,
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
): DiagnosisReviewQueueSummary {
  const blocker = localizeDiagnosisReviewQueueBlocker(input, t);
  let message: string;
  if (blocker !== "") {
    message = blocker;
  } else if (summary.attention > 0) {
    message = t("summaryAttention", { count: summary.attention });
  } else if (summary.pending > 0) {
    message = t("summaryPending", { count: summary.pending });
  } else if (summary.canConfirm || summary.ready > 0) {
    message = t("summaryReady");
  } else if (summary.done > 0) {
    message = t("summaryRetained");
  } else {
    message = t("summaryNextActions");
  }
  return {
    ...summary,
    blockingReason: blocker,
    message,
  };
}

export function localizeDiagnosisReviewQueueActionPlan(
  plan: DiagnosisReviewQueueActionPlan,
  summary: DiagnosisReviewQueueSummary,
  input: DiagnosisReviewQueueInput,
  localizedItems: DiagnosisReviewQueueItem[],
  t: DiagnosisReviewQueueTranslator,
): DiagnosisReviewQueueActionPlan {
  const itemsByKey = new Map(localizedItems.map((item) => [item.key, item]));
  const blocker = localizeDiagnosisReviewQueueBlocker(input, t);
  const message =
    blocker !== ""
      ? blocker
      : plan.status === "ready"
        ? t("actionPlanReady")
        : plan.status === "done"
          ? t("actionPlanDone")
          : summary.canConfirm
            ? t("actionPlanResidual")
            : t("actionPlanPending");
  return {
    ...plan,
    actions: plan.actions.map((action) => {
      const item = itemsByKey.get(action.key);
      return item === undefined
        ? action
        : {
            ...action,
            detail: item.detail,
            tag: item.tag,
            title: item.title,
          };
    }),
    message,
  };
}

export function localizeDiagnosisReviewQueueActionGate(
  gate: DiagnosisReviewQueueActionGate,
  t: DiagnosisReviewQueueTranslator,
): DiagnosisReviewQueueActionGate {
  if (!gate.disabled) {
    return gate;
  }
  if (gate.kind === "connection") {
    return { ...gate, reason: t("connectionRequired") };
  }
  return {
    ...gate,
    reason: t("actionsBlocked", { reason: gate.reason }),
  };
}

export function localizeDiagnosisReviewQueuePostEvidenceStatus(
  status: DiagnosisReviewQueuePostEvidenceStatus,
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): DiagnosisReviewQueuePostEvidenceStatus {
  const confidence = localizeReviewConfidence(input, t, tStatus);
  const counts = {
    confidence,
    reviewed: status.reviewed,
    submitted: status.submitted,
  };
  switch (status.status) {
    case "none":
      return {
        ...status,
        detail: t("postNoneDetail"),
        label: t("postNone"),
      };
    case "submitted":
      return {
        ...status,
        detail: t("postSubmittedDetail", counts),
        label: t("postSubmitted"),
      };
    case "blocked":
      return {
        ...status,
        detail: t("postBlockedDetail", {
          ...counts,
          blocker:
            localizeDiagnosisReviewQueueBlocker(input, t) ||
            t("resolveRemainingBlockers"),
        }),
        label: t("postBlocked"),
      };
    case "ready":
      return {
        ...status,
        detail: t("postReadyDetail", counts),
        label: t("postReady"),
      };
  }
}

export function localizeDiagnosisReviewQueueTaskProgress(
  progress: DiagnosisReviewQueueTaskProgress,
  items: DiagnosisReviewQueueItem[],
  summary: DiagnosisReviewQueueSummary,
  input: DiagnosisReviewQueueInput,
  postEvidenceStatus: DiagnosisReviewQueuePostEvidenceStatus,
  t: DiagnosisReviewQueueTranslator,
): DiagnosisReviewQueueTaskProgress {
  const phases = progress.phases.map((phase) => {
    const detail = localizeTaskPhaseDetail(
      phase.key,
      items,
      summary,
      input,
      postEvidenceStatus,
      t,
    );
    return {
      ...phase,
      action:
        phase.action === undefined
          ? undefined
          : {
              ...phase.action,
              label: localizeTaskActionLabel(phase.action, items, t),
            },
      detail,
      label: localizeTaskPhaseLabel(phase.key, t),
      statusLabel: localizeReviewQueueStatus(phase.status, t),
    };
  });
  const activePhase =
    phases.find((phase) => phase.status === "attention") ??
    phases.find((phase) => phase.status === "pending") ??
    phases.find((phase) => phase.status === "ready");
  return {
    ...progress,
    phases,
    statusLabel: localizeTaskProgressStatus(progress.status, t),
    summary:
      activePhase === undefined
        ? localizeTaskProgressStatus(progress.status, t)
        : t("taskProgressSummary", {
            detail: activePhase.detail,
            phase: activePhase.label,
          }),
  };
}

export function localizeDiagnosisReviewQueueNextAction(
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
): string {
  const blocker = diagnosisReviewQueueBlocker(input);
  switch (blocker?.kind) {
    case "collection_result":
      return t("nextResolveCollection");
    case "latest_missing_request":
    case "missing_evidence":
      return t("nextCollectMissing");
    case "planned_evidence":
      return t("nextRunCollection");
    case "reassessment":
      return t("nextReassess");
  }
  if (input.evidenceCollectionSuggestions.length > 0) {
    return t("nextReviewSuggestions");
  }
  if (
    input.collectionResults.some(
      (result) => result.status.trim().toLowerCase() === "collected",
    )
  ) {
    return t("nextReassess");
  }
  const status = input.conclusionStatus?.trim().toLowerCase();
  if (status === "final" || status === "ready_for_review") {
    return t("nextConfirm");
  }
  return input.requiresHumanReview
    ? t("nextOperatorReview")
    : t("nextContinue");
}

export function localizeReviewQueueStatus(
  status: DiagnosisReviewQueueItem["status"],
  t: DiagnosisReviewQueueTranslator,
): string {
  switch (status) {
    case "attention":
      return t("statusAttention");
    case "pending":
      return t("statusPending");
    case "ready":
      return t("statusReady");
    case "done":
      return t("statusDone");
  }
}

export function localizeDiagnosisReviewQueueBlocker(
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
): string {
  const blocker = diagnosisReviewQueueBlocker(input);
  if (blocker === null) {
    return "";
  }
  switch (blocker.kind) {
    case "collection_result":
      return t("blockerCollection", { tool: blocker.tool });
    case "latest_missing_request":
      return t("blockerLatestRequest", { label: blocker.label });
    case "missing_evidence":
      return t("blockerMissingEvidence");
    case "planned_evidence":
      return t("blockerPlannedEvidence");
    case "reassessment":
      return t("blockerReassessment");
  }
}

function localizeCollectionResultDetail(
  result: Extract<DiagnosisReviewQueueItem, { kind: "collection_result" }>["result"],
  t: DiagnosisReviewQueueTranslator,
): string {
  if (result.status.trim().toLowerCase() === "collected") {
    const details: string[] = [];
    if (result.observed_alerts > 0) {
      details.push(t("observedAlerts", { count: result.observed_alerts }));
    }
    if (result.observed_metric_series !== undefined) {
      details.push(
        t("observedMetricSeries", { count: result.observed_metric_series }),
      );
    }
    return details.length === 0
      ? t("collectionCompleted")
      : t("collectionCompletedWith", { details: details.join(" / ") });
  }
  const details = [
    result.reason_code
      ? t("collectionReason", { reason: result.reason_code })
      : "",
    result.message ? t("collectionMessage", { message: result.message }) : "",
  ].filter(Boolean);
  return details.length === 0
    ? t("collectionNeedsAttention")
    : details.join(" ");
}

function localizeExecutableEvidenceDetail(
  request: Extract<DiagnosisReviewQueueItem, { kind: "executable_evidence" }>["request"],
  t: DiagnosisReviewQueueTranslator,
): string {
  return [
    t("requestReason", { reason: request.reason }),
    request.query ? t("requestQuery", { query: request.query }) : "",
    request.window_seconds
      ? t("requestWindow", { seconds: request.window_seconds })
      : "",
    request.limit ? t("requestLimit", { limit: request.limit }) : "",
  ]
    .filter(Boolean)
    .join(" ");
}

function localizeSupplementalEvidenceRecordDetail(
  item: Extract<DiagnosisReviewQueueItem, { kind: "supplemental_evidence_record" }>,
  t: DiagnosisReviewQueueTranslator,
): string {
  const turnDetail =
    item.record.user_sequence > 0 && item.record.assistant_sequence > 0
      ? t("submittedEvidenceTurns", {
          assistant: item.record.assistant_sequence,
          user: item.record.user_sequence,
        })
      : t("submittedEvidenceRetained");
  if (item.status === "attention") {
    const latestRequest =
      item.unresolvedRequest !== undefined &&
      normalizedEvidenceText(item.unresolvedRequest.detail) !==
        normalizedEvidenceText(item.record.detail)
        ? t("latestRequestDetail", { detail: item.unresolvedRequest.detail })
        : "";
    return t("submittedEvidenceStillRequestedDetail", {
      detail: item.record.detail,
      latestRequest,
      turns: turnDetail,
    });
  }
  if (item.status === "pending") {
    return t("submittedEvidenceAwaitingDetail", {
      detail: item.record.detail,
    });
  }
  return t("submittedEvidenceReviewedDetail", {
    detail: item.record.detail,
    turns: turnDetail,
  });
}

function localizeReviewConfidence(
  input: DiagnosisReviewQueueInput,
  t: DiagnosisReviewQueueTranslator,
  tStatus: DiagnosisRoomStatusTranslator,
): string {
  const confidence = input.confidence?.trim();
  const localizedConfidence = confidence
    ? localizeDiagnosisRoomStatus(confidence, tStatus)
    : "";
  switch (input.confidenceProgress?.status) {
    case "improved":
      return confidence
        ? t("confidenceImprovedWithValue", { confidence: localizedConfidence })
        : t("confidenceImproved");
    case "declined":
      return confidence
        ? t("confidenceDeclinedWithValue", { confidence: localizedConfidence })
        : t("confidenceDeclined");
    case "stable":
      return confidence
        ? t("confidenceStableWithValue", { confidence: localizedConfidence })
        : t("confidenceStable");
    case "unknown":
    case undefined:
      return confidence
        ? t("confidenceLatest", { confidence: localizedConfidence })
        : t("confidenceUnavailable");
  }
}

function localizeTaskPhaseDetail(
  key: DiagnosisReviewQueueTaskProgress["phases"][number]["key"],
  items: DiagnosisReviewQueueItem[],
  summary: DiagnosisReviewQueueSummary,
  input: DiagnosisReviewQueueInput,
  postEvidenceStatus: DiagnosisReviewQueuePostEvidenceStatus,
  t: DiagnosisReviewQueueTranslator,
): string {
  if (key === "collect_evidence") {
    const collectionItems = items.filter(
      (item) =>
        item.kind === "collection_result" || item.kind === "executable_evidence",
    );
    const attention = collectionItems.filter(
      (item) => item.status === "attention",
    ).length;
    const pending = collectionItems.filter((item) => item.status !== "done").length;
    if (collectionItems.length === 0) {
      return t("phaseCollectionNone");
    }
    if (attention > 0) {
      return t("phaseCollectionAttention", { count: attention });
    }
    if (pending > 0) {
      return t("phaseCollectionPending", { count: pending });
    }
    return t("phaseCollectionDone", { count: collectionItems.length });
  }
  if (key === "supply_operator_evidence") {
    const supplementalItems = items.filter(
      (item) =>
        item.kind === "supplemental_evidence" ||
        item.kind === "supplemental_evidence_record",
    );
    const attention = supplementalItems.filter(
      (item) => item.status === "attention",
    ).length;
    const pending = supplementalItems.filter((item) => item.status !== "done").length;
    if (supplementalItems.length === 0) {
      return t("phaseSupplementalNone");
    }
    if (attention > 0) {
      return t("phaseSupplementalAttention", { count: attention });
    }
    if (pending > 0) {
      return t("phaseSupplementalPending", { count: pending });
    }
    return t("phaseSupplementalDone", { count: supplementalItems.length });
  }
  if (key === "ai_reassessment") {
    if (postEvidenceStatus.status !== "none") {
      return postEvidenceStatus.detail;
    }
    const openEvidence = items.some(
      (item) =>
        item.status !== "done" &&
        (item.kind === "collection_result" ||
          item.kind === "executable_evidence" ||
          item.kind === "supplemental_evidence" ||
          item.kind === "supplemental_evidence_record"),
    );
    if (openEvidence) {
      return t("phaseReassessmentWaits");
    }
    if (summary.canConfirm) {
      return t("phaseReassessmentReady");
    }
    if (
      items.some(
        (item) => item.kind === "collection_result" && item.status === "done",
      )
    ) {
      return t("phaseReassessmentCollected");
    }
    return t("phaseReassessmentContinue");
  }
  const blocker = localizeDiagnosisReviewQueueBlocker(input, t);
  if (summary.canConfirm) {
    return t("phaseConfirmationReady");
  }
  if (blocker !== "") {
    return blocker;
  }
  if (summary.ready > 0) {
    return t("phaseConfirmationReview");
  }
  return t("phaseConfirmationWaits");
}

function localizeTaskPhaseLabel(
  key: DiagnosisReviewQueueTaskProgress["phases"][number]["key"],
  t: DiagnosisReviewQueueTranslator,
): string {
  switch (key) {
    case "collect_evidence":
      return t("phaseCollection");
    case "supply_operator_evidence":
      return t("phaseSupplemental");
    case "ai_reassessment":
      return t("phaseReassessment");
    case "confirm_conclusion":
      return t("phaseConfirmation");
  }
}

function localizeTaskActionLabel(
  action: DiagnosisReviewQueueTaskProgress["phases"][number]["action"] extends infer T
    ? NonNullable<T>
    : never,
  items: DiagnosisReviewQueueItem[],
  t: DiagnosisReviewQueueTranslator,
): string {
  switch (action.kind) {
    case "confirm":
      return t("actionConfirm");
    case "request_reassessment":
      return t("actionReassess");
    case "use_evidence_plan": {
      const item = items.find((candidate) => candidate.key === action.itemKey);
      return item?.kind === "collection_result" && item.retryable
        ? t("actionRetryCollection")
        : t("actionUsePlan");
    }
    case "use_follow_up": {
      const item = items.find((candidate) => candidate.key === action.itemKey);
      if (item?.kind === "collection_result") {
        return t("actionAddEvidence");
      }
      if (item?.kind === "supplemental_evidence_record") {
        return t("actionUseLatestRequest");
      }
      return t("actionUseFollowUp");
    }
  }
}

function localizeTaskProgressStatus(
  status: DiagnosisReviewQueueTaskProgress["status"],
  t: DiagnosisReviewQueueTranslator,
): string {
  switch (status) {
    case "blocked":
      return t("taskStatusBlocked");
    case "done":
      return t("taskStatusDone");
    case "pending":
      return t("taskStatusPending");
    case "ready":
      return t("taskStatusReady");
  }
}

function normalizedEvidenceText(value: string): string {
  return value.trim().toLowerCase().replace(/\s+/g, " ");
}
