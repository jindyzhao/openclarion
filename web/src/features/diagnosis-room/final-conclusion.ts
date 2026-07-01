import type { DiagnosisNotificationDeliveryCoverage } from "./notification-content-proof";

export type DiagnosisFinalConclusionDisplayInput = {
  status?: string;
  source?: string;
  reason?: string;
  confidence?: string;
  confidence_rationale?: string;
  content?: string;
  confirmed_by?: string;
  recorded_at?: string;
  requires_human_review?: boolean;
  evidence_requests?: unknown[];
  missing_evidence_requests?: unknown[];
  evidence_collection_suggestions?: unknown[];
} | null | undefined;

export type DiagnosisFinalConclusionRetentionState = {
  detail: string;
  label: string;
  status: "missing" | "needs_review" | "ready" | "retained";
};

export type DiagnosisFinalConclusionReviewItem = {
  detail: string;
  key: string;
  status: "attention" | "done" | "ready" | "residual";
  title: string;
};

export type DiagnosisFinalConclusionConfidenceTimelineEntry = {
  confidence?: string;
  confidence_rationale?: string;
  turn_count?: number;
  trigger?: string;
};

export type DiagnosisFinalConclusionConfidenceProgress = {
  detail: string;
  finalConfidence: string;
  initialConfidence: string;
  label: string;
  status: "declined" | "improved" | "stable" | "unknown";
};

export type DiagnosisFinalConclusionTraceabilityStatus = {
  color: "success" | "warning" | "error" | "default";
  detail: string;
  label: string;
  notificationLabel: string;
  reviewOpenCount: number;
  reviewResidualCount: number;
  status: "complete" | "review" | "blocked" | "pending";
};

export type DiagnosisFinalConclusionTraceabilityInput = {
  conclusion: DiagnosisFinalConclusionDisplayInput;
  notificationDelivery?: Pick<
    DiagnosisNotificationDeliveryCoverage,
    "detail" | "label" | "readyCount" | "requiredCount" | "status"
  >;
};

type EvidenceGapSummaryKind =
  | "Executable"
  | "Missing"
  | "Suggestion";

const sourceLabels: Record<string, string> = {
  latest_assistant_turn: "Latest assistant turn",
  none: "No assistant conclusion",
};

const reasonLabels: Record<string, string> = {
  assistant_marked_final: "AI marked final",
  assistant_marked_ready_for_review: "AI marked ready for review",
  room_closed_without_assistant_turn: "Room closed without assistant turn",
};

export function diagnosisFinalConclusionSourceLabel(
  source: string | null | undefined,
): string | undefined {
  const normalized = normalizeEnumValue(source);
  if (normalized === "") {
    return undefined;
  }
  return sourceLabels[normalized] ?? humanizeEnumValue(normalized);
}

export function diagnosisFinalConclusionReasonLabel(
  reason: string | null | undefined,
): string | undefined {
  const normalized = normalizeEnumValue(reason);
  if (normalized === "") {
    return undefined;
  }
  return reasonLabels[normalized] ?? humanizeEnumValue(normalized);
}

export function diagnosisFinalConclusionStatusLabel(
  conclusion: DiagnosisFinalConclusionDisplayInput,
): string {
  if (!conclusion) {
    return "-";
  }
  const status = normalizeEnumValue(conclusion.status);
  if (status === "") {
    return "-";
  }
  if (status === "available") {
    const confidence = normalizeEnumValue(conclusion.confidence);
    return confidence === "" ? "available" : `available (${confidence})`;
  }
  return humanizeEnumValue(status);
}

export function diagnosisFinalConclusionText(
  conclusion: DiagnosisFinalConclusionDisplayInput,
): string {
  if (!conclusion) {
    return "";
  }
  const content = conclusion.content?.trim();
  if (content) {
    return content;
  }
  return (
    diagnosisFinalConclusionReasonLabel(conclusion.reason) ??
    diagnosisFinalConclusionStatusLabel(conclusion)
  );
}

export function diagnosisFinalConclusionRetentionState(
  conclusion: DiagnosisFinalConclusionDisplayInput,
): DiagnosisFinalConclusionRetentionState {
  if (!conclusion) {
    return {
      detail: "No AI final conclusion has been retained for this room.",
      label: "No retained conclusion",
      status: "missing",
    };
  }
  if (normalizeEnumValue(conclusion.status) !== "available") {
    return {
      detail:
        diagnosisFinalConclusionReasonLabel(conclusion.reason) ??
        "AI has not produced a reviewable final conclusion.",
      label: "Conclusion not available",
      status: "missing",
    };
  }
  if (conclusion.confirmed_by?.trim()) {
    return {
      detail: conclusion.recorded_at?.trim()
        ? `Operator-confirmed conclusion was retained at ${conclusion.recorded_at}.`
        : "Operator-confirmed conclusion has been retained.",
      label: "Conclusion retained",
      status: "retained",
    };
  }
  if (conclusion.requires_human_review) {
    return {
      detail:
        "AI conclusion is available, but operator confirmation is required before final report notification.",
      label: "Operator confirmation required",
      status: "needs_review",
    };
  }
  return {
    detail:
      "AI conclusion is available for operator confirmation before final report notification.",
    label: "Conclusion ready",
    status: "ready",
  };
}

export function diagnosisFinalConclusionReviewItems(
  conclusion: DiagnosisFinalConclusionDisplayInput,
): DiagnosisFinalConclusionReviewItem[] {
  if (!conclusion || normalizeEnumValue(conclusion.status) !== "available") {
    return [
      {
        detail: "Wait for AI to produce a reviewable conclusion.",
        key: "conclusion-available",
        status: "attention",
        title: "Conclusion unavailable",
      },
    ];
  }

  const items: DiagnosisFinalConclusionReviewItem[] = [];
  const confidence = normalizeEnumValue(conclusion.confidence);
  items.push({
    detail: conclusion.confidence_rationale?.trim()
      ? conclusion.confidence_rationale.trim()
      : confidence === ""
        ? "AI did not provide a confidence rationale for this conclusion."
        : `AI reported ${confidence} confidence without additional rationale.`,
    key: "confidence-rationale",
    status: confidence === "high" ? "done" : "residual",
    title: confidence === "" ? "Confidence rationale missing" : `Confidence: ${confidence}`,
  });

  const missingEvidenceCount = conclusion.missing_evidence_requests?.length ?? 0;
  const suggestionCount = conclusion.evidence_collection_suggestions?.length ?? 0;
  const executableRequestCount = conclusion.evidence_requests?.length ?? 0;
  if (missingEvidenceCount > 0 || executableRequestCount > 0) {
    items.push({
      detail: diagnosisFinalConclusionEvidenceGapDetail(conclusion),
      key: "evidence-gaps",
      status: "attention",
      title: "Evidence blockers need review",
    });
  } else if (suggestionCount > 0) {
    items.push({
      detail: diagnosisFinalConclusionEvidenceGapDetail(conclusion),
      key: "evidence-gaps",
      status: "residual",
      title: "Residual collection suggestions",
    });
  } else {
    items.push({
      detail: "No remaining missing evidence, collection suggestions, or executable evidence requests were retained with the final conclusion.",
      key: "evidence-gaps",
      status: "done",
      title: "Evidence gaps cleared",
    });
  }

  if (conclusion.confirmed_by?.trim()) {
    items.push({
      detail: conclusion.recorded_at?.trim()
        ? `Operator-confirmed conclusion was retained at ${conclusion.recorded_at}.`
        : "Operator-confirmed conclusion has been retained.",
      key: "retention",
      status: "done",
      title: "Conclusion retained",
    });
  } else {
    items.push({
      detail: conclusion.requires_human_review
        ? "Operator confirmation is required before retaining this conclusion."
        : "Operator can confirm this conclusion when the review queue has no blockers.",
      key: "retention",
      status: conclusion.requires_human_review ? "attention" : "ready",
      title: "Awaiting operator confirmation",
    });
  }

  return items;
}

function diagnosisFinalConclusionEvidenceGapDetail(
  conclusion: NonNullable<DiagnosisFinalConclusionDisplayInput>,
): string {
  const countSummary = [
    evidenceGapCountSummary(
      conclusion.missing_evidence_requests?.length ?? 0,
      "missing evidence request(s)",
    ),
    evidenceGapCountSummary(
      conclusion.evidence_collection_suggestions?.length ?? 0,
      "collection suggestion(s)",
    ),
    evidenceGapCountSummary(
      conclusion.evidence_requests?.length ?? 0,
      "executable evidence request(s)",
    ),
  ]
    .filter((item) => item !== "")
    .join(", ");

  const actionableSummaries = [
    ...evidenceGapItemSummaries(
      "Missing",
      conclusion.missing_evidence_requests,
    ),
    ...evidenceGapItemSummaries(
      "Suggestion",
      conclusion.evidence_collection_suggestions,
    ),
    ...evidenceGapItemSummaries("Executable", conclusion.evidence_requests),
  ].slice(0, 3);

  if (actionableSummaries.length === 0) {
    return countSummary;
  }
  return `${countSummary}. Next evidence: ${actionableSummaries.join("; ")}`;
}

function evidenceGapCountSummary(count: number, label: string): string {
  return count > 0 ? `${count} ${label}` : "";
}

function evidenceGapItemSummaries(
  kind: EvidenceGapSummaryKind,
  items: unknown[] | undefined,
): string[] {
  return (items ?? [])
    .map((item) => evidenceGapItemSummary(kind, item))
    .filter((item) => item !== "");
}

function evidenceGapItemSummary(
  kind: EvidenceGapSummaryKind,
  item: unknown,
): string {
  if (!isRecord(item)) {
    return "";
  }
  const label =
    evidenceGapStringValue(item, "label") ||
    evidenceGapStringValue(item, "tool") ||
    "";
  const detail =
    evidenceGapStringValue(item, "detail") ||
    evidenceGapStringValue(item, "reason") ||
    evidenceGapStringValue(item, "query") ||
    "";
  const summary =
    label !== "" && detail !== ""
      ? `${label} - ${detail}`
      : label || detail;
  return summary === "" ? "" : `${kind}: ${truncateEvidenceGapSummary(summary)}`;
}

function evidenceGapStringValue(
  item: Record<string, unknown>,
  key: string,
): string {
  const value = item[key];
  return typeof value === "string" ? value.trim() : "";
}

function truncateEvidenceGapSummary(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= 180) {
    return normalized;
  }
  return `${normalized.slice(0, 177).trimEnd()}...`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function diagnosisFinalConclusionConfidenceProgress(
  conclusion: DiagnosisFinalConclusionDisplayInput,
  timeline: DiagnosisFinalConclusionConfidenceTimelineEntry[] | undefined,
): DiagnosisFinalConclusionConfidenceProgress {
  const checkpoints = (timeline ?? []).filter(
    (item) => normalizeEnumValue(item.confidence) !== "",
  );
  const firstCheckpoint = checkpoints[0];
  const lastCheckpoint = checkpoints.at(-1);
  const initialConfidence = normalizeEnumValue(firstCheckpoint?.confidence);
  const finalConfidence =
    normalizeEnumValue(conclusion?.confidence) ||
    normalizeEnumValue(lastCheckpoint?.confidence);

  if (initialConfidence === "" || finalConfidence === "") {
    return {
      detail:
        "Confidence progress is unavailable until at least one checkpoint and final confidence are present.",
      finalConfidence: finalConfidence || "unknown",
      initialConfidence: initialConfidence || "unknown",
      label: "Confidence progress unavailable",
      status: "unknown",
    };
  }

  const initialRank = confidenceRank(initialConfidence);
  const finalRank = confidenceRank(finalConfidence);
  const latestRationale =
    conclusion?.confidence_rationale?.trim() ||
    lastCheckpoint?.confidence_rationale?.trim() ||
    "";
  const rationaleSuffix = latestRationale === "" ? "" : ` ${latestRationale}`;

  if (initialRank === null || finalRank === null) {
    return {
      detail: `Confidence moved from ${initialConfidence} to ${finalConfidence}.${rationaleSuffix}`,
      finalConfidence,
      initialConfidence,
      label: "Confidence progress recorded",
      status: "unknown",
    };
  }
  if (finalRank > initialRank) {
    return {
      detail: `Confidence improved from ${initialConfidence} to ${finalConfidence}.${rationaleSuffix}`,
      finalConfidence,
      initialConfidence,
      label: "Confidence improved",
      status: "improved",
    };
  }
  if (finalRank < initialRank) {
    return {
      detail: `Confidence declined from ${initialConfidence} to ${finalConfidence}.${rationaleSuffix}`,
      finalConfidence,
      initialConfidence,
      label: "Confidence declined",
      status: "declined",
    };
  }
  return {
    detail: `Confidence remained ${finalConfidence}.${rationaleSuffix}`,
    finalConfidence,
    initialConfidence,
    label: "Confidence stable",
    status: "stable",
  };
}

export function diagnosisFinalConclusionTraceabilityStatus({
  conclusion,
  notificationDelivery,
}: DiagnosisFinalConclusionTraceabilityInput): DiagnosisFinalConclusionTraceabilityStatus {
  const retention = diagnosisFinalConclusionRetentionState(conclusion);
  const reviewItems = diagnosisFinalConclusionReviewItems(conclusion);
  const reviewOpenCount = reviewItems.filter(
    (item) => item.status === "attention" || item.status === "ready",
  )
    .length;
  const reviewResidualCount = reviewItems.filter(
    (item) => item.status === "residual",
  ).length;
  const notificationLabel = notificationDelivery?.label ?? "AI delivery not checked";

  if (retention.status === "missing") {
    return {
      color: "default",
      detail:
        "Wait for AI to retain a reviewable final conclusion before checking operator confirmation and notification delivery proof.",
      label: "Closure traceability pending",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "pending",
    };
  }

  if (retention.status !== "retained") {
    return {
      color: "warning",
      detail: `${retention.detail} ${reviewOpenCount} blocking review item(s) are still open before closure can be accepted.`,
      label: "Closure traceability needs review",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "review",
    };
  }

  if (reviewOpenCount > 0) {
    return {
      color: "warning",
      detail: `Operator-confirmed conclusion is retained, but ${reviewOpenCount} blocking review item(s) are still open in the retained output.`,
      label: "Retained with blockers",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "review",
    };
  }

  if (notificationDelivery === undefined) {
    return {
      color: "default",
      detail:
        "Operator-confirmed conclusion is retained; load the notification timeline to verify AI delivery proof.",
      label: "Closure delivery proof pending",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "pending",
    };
  }

  if (notificationDelivery.status === "ready") {
    const residualDetail =
      reviewResidualCount === 0
        ? "review checklist is clear"
        : `blocking review checklist is clear with ${reviewResidualCount} residual review item(s) documented`;
    return {
      color: "success",
      detail: `Operator-confirmed conclusion is retained, ${residualDetail}, and Enterprise WeChat AI delivery proof covers all required phases.`,
      label: "Closure traceability complete",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "complete",
    };
  }

  if (notificationDelivery.status === "blocked") {
    return {
      color: "error",
      detail: `Operator-confirmed conclusion is retained, but AI delivery proof is blocked: ${notificationDelivery.detail}`,
      label: "Closure traceability blocked",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "blocked",
    };
  }

  if (notificationDelivery.status === "review") {
    return {
      color: "warning",
      detail: `Operator-confirmed conclusion is retained, but AI delivery proof is incomplete: ${notificationDelivery.detail}`,
      label: "Closure delivery proof incomplete",
      notificationLabel,
      reviewOpenCount,
      reviewResidualCount,
      status: "review",
    };
  }

  return {
    color: "default",
    detail: `Operator-confirmed conclusion is retained; AI delivery proof has not started (${notificationDelivery.readyCount} of ${notificationDelivery.requiredCount} phase(s) complete).`,
    label: "Closure delivery proof pending",
    notificationLabel,
    reviewOpenCount,
    reviewResidualCount,
    status: "pending",
  };
}

function normalizeEnumValue(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

function confidenceRank(confidence: string): number | null {
  switch (normalizeEnumValue(confidence)) {
    case "low":
      return 1;
    case "medium":
      return 2;
    case "high":
      return 3;
    default:
      return null;
  }
}

function humanizeEnumValue(value: string): string {
  return value
    .split("_")
    .filter(Boolean)
    .map((part, index) =>
      index === 0 ? part.charAt(0).toUpperCase() + part.slice(1) : part,
    )
    .join(" ");
}
