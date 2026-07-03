import type { DiagnosisRoomRBACPermissionItem } from "./rbac-capabilities";
import {
  diagnosisFinalConclusionTraceabilityStatus,
  type DiagnosisFinalConclusionDisplayInput,
} from "./final-conclusion";
import type { DiagnosisNotificationDeliveryCoverage } from "./notification-content-proof";
import type {
  DiagnosisConnectionStatus,
  DiagnosisConsultationEvidenceRequest,
  DiagnosisConsultationInsight,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisStateFrame,
  DiagnosisSupplementalEvidenceRecord,
} from "./types";

export type DiagnosisWorkflowReadinessStatus =
  | "attention"
  | "blocked"
  | "pending"
  | "ready";

export type DiagnosisWorkflowReadinessItem = {
  detail: string;
  key:
    | "conclusion"
    | "connection"
    | "evidence"
    | "identity"
    | "permissions"
    | "room";
  label: string;
  metric?: string;
  status: DiagnosisWorkflowReadinessStatus;
};

export type DiagnosisWorkflowReadinessReviewQueueSource =
  | "live"
  | "none"
  | "saved";

type DiagnosisWorkflowReadinessLatestInsight = {
  collectionResults: DiagnosisEvidenceCollectionResult[];
  evidenceRequests: DiagnosisEvidenceRequest[];
  insight: DiagnosisConsultationInsight;
  supplementalEvidence?: DiagnosisSupplementalEvidenceRecord[];
};

type DiagnosisWorkflowReadinessEvidence = {
  collectionResults: DiagnosisEvidenceCollectionResult[];
  evidenceCollectionSuggestions: DiagnosisConsultationEvidenceRequest[];
  evidenceRequests: DiagnosisEvidenceRequest[];
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[];
  supplementalEvidence?: DiagnosisSupplementalEvidenceRecord[];
};

type DiagnosisWorkflowNotificationDelivery = Pick<
  DiagnosisNotificationDeliveryCoverage,
  "detail" | "label" | "readyCount" | "requiredCount" | "status"
>;

type DiagnosisWorkflowVisibility = {
  status: string;
};

export function diagnosisWorkflowReadiness(input: {
  actorSubject?: string;
  canConfirmConclusion: boolean;
  confirmConclusionBlockReason: string;
  connected: boolean;
  connectionStatus: DiagnosisConnectionStatus;
  latestInsight?: DiagnosisWorkflowReadinessLatestInsight | null;
  notificationDelivery?: DiagnosisWorkflowNotificationDelivery;
  permissionItems: DiagnosisRoomRBACPermissionItem[];
  retainedConclusion?: DiagnosisFinalConclusionDisplayInput;
  summaryEvidence?: DiagnosisWorkflowReadinessEvidence | null;
  selectedRoomStatus?: string;
  selectedSessionID: string;
  state?: DiagnosisStateFrame | null;
  workflowVisibility?: DiagnosisWorkflowVisibility;
}): DiagnosisWorkflowReadinessItem[] {
  return [
    diagnosisWorkflowIdentityItem(input.actorSubject),
    diagnosisWorkflowRoomItem({
      selectedRoomStatus: input.selectedRoomStatus,
      selectedSessionID: input.selectedSessionID,
      state: input.state,
      workflowVisibility: input.workflowVisibility,
    }),
    diagnosisWorkflowConnectionItem({
      connected: input.connected,
      connectionStatus: input.connectionStatus,
      selectedRoomStatus: input.selectedRoomStatus,
      selectedSessionID: input.selectedSessionID,
      state: input.state,
      workflowVisibility: input.workflowVisibility,
    }),
    diagnosisWorkflowPermissionItem(input.permissionItems),
    diagnosisWorkflowEvidenceItem(
      input.latestInsight
        ? diagnosisWorkflowEvidenceFromLatestInsight(input.latestInsight)
        : input.summaryEvidence,
    ),
    diagnosisWorkflowConclusionItem({
      canConfirmConclusion: input.canConfirmConclusion,
      confirmConclusionBlockReason: input.confirmConclusionBlockReason,
      latestInsight: input.latestInsight,
      notificationDelivery: input.notificationDelivery,
      retainedConclusion: input.retainedConclusion,
      state: input.state,
    }),
  ];
}

export function diagnosisWorkflowReadinessReviewQueueSource(input: {
  latestInsightLoaded: boolean;
  savedReviewQueueLoaded: boolean;
}): DiagnosisWorkflowReadinessReviewQueueSource {
  if (input.latestInsightLoaded) {
    return "live";
  }
  if (input.savedReviewQueueLoaded) {
    return "saved";
  }
  return "none";
}

export function diagnosisActionIdentityBlockReason(
  actorSubject: string | undefined,
  actionLabel: string,
): string {
  const normalizedSubject = actorSubject?.trim() ?? "";
  if (normalizedSubject !== "") {
    return "";
  }
  return `Authenticate as an operator before ${actionLabel}.`;
}

function diagnosisWorkflowIdentityItem(
  actorSubject: string | undefined,
): DiagnosisWorkflowReadinessItem {
  const normalizedSubject = actorSubject?.trim() ?? "";
  if (normalizedSubject === "") {
    return {
      detail: "Sign in or connect with operator credentials before taking room actions.",
      key: "identity",
      label: "Identity",
      status: "pending",
    };
  }
  return {
    detail: `Current actor: ${normalizedSubject}.`,
    key: "identity",
    label: "Identity",
    status: "ready",
  };
}

function diagnosisWorkflowRoomItem(input: {
  selectedRoomStatus?: string;
  selectedSessionID: string;
  state?: DiagnosisStateFrame | null;
  workflowVisibility?: DiagnosisWorkflowVisibility;
}): DiagnosisWorkflowReadinessItem {
  const sessionID = input.selectedSessionID.trim();
  if (sessionID === "") {
    return {
      detail: "No diagnosis room is selected.",
      key: "room",
      label: "Room",
      status: "pending",
    };
  }
  if (input.state?.latest_error) {
    return {
      detail: `${input.state.latest_error.code}: ${input.state.latest_error.message}`,
      key: "room",
      label: "Room",
      status: "blocked",
    };
  }
  const roomStatus = input.state?.status || input.selectedRoomStatus || "";
  if (
    diagnosisWorkflowVisibilityUnavailable({
      roomStatus,
      workflowVisibility: input.workflowVisibility,
    })
  ) {
    return {
      detail: `Temporal reports workflow status ${input.workflowVisibility?.status ?? "unknown"}; inspect the workflow before continuing this room.`,
      key: "room",
      label: "Room",
      status: "blocked",
    };
  }
  if (roomStatus.trim() === "") {
    return {
      detail: `Room ${sessionID} is selected; metadata is still loading.`,
      key: "room",
      label: "Room",
      status: "pending",
    };
  }
  return {
    detail: `Room ${sessionID} is ${roomStatus}.`,
    key: "room",
    label: "Room",
    status: "ready",
  };
}

function diagnosisWorkflowConnectionItem(input: {
  connected: boolean;
  connectionStatus: DiagnosisConnectionStatus;
  selectedRoomStatus?: string;
  selectedSessionID: string;
  state?: DiagnosisStateFrame | null;
  workflowVisibility?: DiagnosisWorkflowVisibility;
}): DiagnosisWorkflowReadinessItem {
  if (input.selectedSessionID.trim() === "") {
    return {
      detail: "Select a room before opening a live connection.",
      key: "connection",
      label: "Connection",
      status: "pending",
    };
  }
  const roomStatus = input.state?.status || input.selectedRoomStatus || "";
  if (
    diagnosisWorkflowVisibilityUnavailable({
      roomStatus,
      workflowVisibility: input.workflowVisibility,
    })
  ) {
    return {
      detail: `Live conversation cannot continue because Temporal reports workflow status ${input.workflowVisibility?.status ?? "unknown"}.`,
      key: "connection",
      label: "Connection",
      status: "blocked",
    };
  }
  if (input.connected) {
    return {
      detail: "WebSocket is connected for live conversation and state refresh.",
      key: "connection",
      label: "Connection",
      status: "ready",
    };
  }
  if (roomStatus.trim().toLowerCase() === "closed") {
    return {
      detail: "Room is closed; live WebSocket is not required for read-only review.",
      key: "connection",
      label: "Connection",
      status: "ready",
    };
  }
  switch (input.connectionStatus) {
    case "connecting":
    case "ticketing":
      return {
        detail: "Connection is being established.",
        key: "connection",
        label: "Connection",
        status: "pending",
      };
    case "closed":
      return {
        detail: "Connection closed; reopen it before sending new evidence or confirmation.",
        key: "connection",
        label: "Connection",
        status: "attention",
      };
    case "error":
      return {
        detail: "Connection failed; reconnect before continuing the live review.",
        key: "connection",
        label: "Connection",
        status: "blocked",
      };
    case "connected":
      return {
        detail: "Connection handshake completed but the browser socket is not open.",
        key: "connection",
        label: "Connection",
        status: "attention",
      };
    case "idle":
      return {
        detail: "Open a connection to continue live diagnosis.",
        key: "connection",
        label: "Connection",
        status: "pending",
      };
  }
}

function diagnosisWorkflowPermissionItem(
  permissionItems: DiagnosisRoomRBACPermissionItem[],
): DiagnosisWorkflowReadinessItem {
  const scopedItems = permissionItems.filter((item) => item.action !== "create");
  if (scopedItems.some((item) => item.status === "denied")) {
    return {
      detail: "At least one selected-room action is denied for the current actor.",
      key: "permissions",
      label: "Permissions",
      status: "blocked",
    };
  }
  if (permissionItems.some((item) => item.status === "checking")) {
    return {
      detail: "RBAC decisions are still being checked.",
      key: "permissions",
      label: "Permissions",
      status: "pending",
    };
  }
  if (scopedItems.every((item) => item.status === "not-selected")) {
    return {
      detail: "Select a room to evaluate scoped room permissions.",
      key: "permissions",
      label: "Permissions",
      status: "pending",
    };
  }
  if (permissionItems.every((item) => item.status === "not-enforced")) {
    return {
      detail: "Direct credential flow is active; local RBAC is not enforced.",
      key: "permissions",
      label: "Permissions",
      status: "ready",
    };
  }
  return {
    detail: "Required selected-room actions are available to the current actor.",
    key: "permissions",
    label: "Permissions",
    status: "ready",
  };
}

function diagnosisWorkflowEvidenceItem(
  evidence: DiagnosisWorkflowReadinessEvidence | null | undefined,
): DiagnosisWorkflowReadinessItem {
  if (!evidence) {
    return {
      detail: "No AI consultation insight has been retained for the selected room.",
      key: "evidence",
      label: "Evidence",
      status: "pending",
    };
  }

  const failedResults = evidence.collectionResults.filter((result) =>
    collectionResultNeedsAttention(result.status),
  ).length;
  const collectedResults = evidence.collectionResults.filter(
    (result) => result.status.toLowerCase() === "collected",
  ).length;
  const pendingPlans = pendingEvidencePlanCount(
    evidence.evidenceRequests,
    evidence.collectionResults,
  );
  const missingRequests = evidence.missingEvidenceRequests.length;
  const suggestions = evidence.evidenceCollectionSuggestions.length;
  const supplemental =
    evidence.supplementalEvidence?.filter(
      (record) => record.evidence.trim() !== "",
    ).length ?? 0;
  const metric = `${collectedResults}/${evidence.collectionResults.length} collected`;

  if (failedResults > 0) {
    return {
      detail: `${failedResults} evidence collection result(s) need operator attention.`,
      key: "evidence",
      label: "Evidence",
      metric,
      status: "blocked",
    };
  }
  if (pendingPlans > 0 || missingRequests > 0 || suggestions > 0) {
    return {
      detail: `${pendingPlans} executable plan(s), ${missingRequests} missing request(s), and ${suggestions} suggestion(s) remain open.`,
      key: "evidence",
      label: "Evidence",
      metric,
      status: "attention",
    };
  }
  if (collectedResults > 0 || supplemental > 0) {
    return {
      detail: "Evidence has been collected or supplied for AI review.",
      key: "evidence",
      label: "Evidence",
      metric,
      status: "ready",
    };
  }
  return {
    detail: "AI did not retain additional evidence requirements for this insight.",
    key: "evidence",
    label: "Evidence",
    metric,
    status: "ready",
  };
}

function diagnosisWorkflowEvidenceFromLatestInsight(
  latestInsight: DiagnosisWorkflowReadinessLatestInsight,
): DiagnosisWorkflowReadinessEvidence {
  return {
    collectionResults: latestInsight.collectionResults,
    evidenceCollectionSuggestions:
      latestInsight.insight.evidence_collection_suggestions ?? [],
    evidenceRequests: latestInsight.evidenceRequests,
    missingEvidenceRequests:
      latestInsight.insight.missing_evidence_requests ?? [],
    supplementalEvidence: latestInsight.supplementalEvidence,
  };
}

function diagnosisWorkflowConclusionItem(input: {
  canConfirmConclusion: boolean;
  confirmConclusionBlockReason: string;
  latestInsight?: DiagnosisWorkflowReadinessLatestInsight | null;
  notificationDelivery?: DiagnosisWorkflowNotificationDelivery;
  retainedConclusion?: DiagnosisFinalConclusionDisplayInput;
  state?: DiagnosisStateFrame | null;
}): DiagnosisWorkflowReadinessItem {
  const conclusion =
    input.state?.final_conclusion ?? input.retainedConclusion;
  const traceability = diagnosisFinalConclusionTraceabilityStatus({
    conclusion,
    notificationDelivery: input.notificationDelivery,
  });
  if (traceability.status === "complete") {
    return {
      detail: traceability.detail,
      key: "conclusion",
      label: "Conclusion",
      status: "ready",
    };
  }
  if (traceability.status === "blocked") {
    return {
      detail: traceability.detail,
      key: "conclusion",
      label: "Conclusion",
      status: "blocked",
    };
  }
  if (input.canConfirmConclusion) {
    return {
      detail: "AI conclusion is ready for operator confirmation.",
      key: "conclusion",
      label: "Conclusion",
      status: "ready",
    };
  }
  if (
    conclusion?.confirmed_by?.trim() &&
    traceability.status === "review"
  ) {
    return {
      detail: traceability.detail,
      key: "conclusion",
      label: "Conclusion",
      status: "attention",
    };
  }
  if (
    conclusion?.confirmed_by?.trim() &&
    traceability.status === "pending"
  ) {
    return {
      detail: traceability.detail,
      key: "conclusion",
      label: "Conclusion",
      status: "pending",
    };
  }
  const conclusionStatus =
    input.latestInsight?.insight.conclusion_status?.trim() ?? "";
  if (input.confirmConclusionBlockReason !== "") {
    return {
      detail: input.confirmConclusionBlockReason,
      key: "conclusion",
      label: "Conclusion",
      status: conclusionBlockReasonIsAuthorization(
        input.confirmConclusionBlockReason,
      )
        ? "blocked"
        : "pending",
    };
  }
  if (conclusionStatus === "") {
    return {
      detail: "AI has not produced a reviewable conclusion yet.",
      key: "conclusion",
      label: "Conclusion",
      status: "pending",
    };
  }
  return {
    detail: `Latest conclusion status is ${conclusionStatus}.`,
    key: "conclusion",
    label: "Conclusion",
    status: "pending",
  };
}

function collectionResultNeedsAttention(status: string): boolean {
  const normalized = status.toLowerCase();
  return (
    normalized === "failed" ||
    normalized === "skipped" ||
    normalized === "unsupported"
  );
}

function diagnosisWorkflowVisibilityUnavailable(input: {
  roomStatus: string;
  workflowVisibility?: DiagnosisWorkflowVisibility;
}): boolean {
  return (
    input.roomStatus.trim().toLowerCase() !== "closed" &&
    workflowVisibilityNeedsAttention(input.workflowVisibility)
  );
}

function workflowVisibilityNeedsAttention(
  workflowVisibility: DiagnosisWorkflowVisibility | undefined,
): boolean {
  const status = workflowVisibility?.status.toLowerCase() ?? "";
  switch (status) {
    case "not_found":
    case "completed":
    case "failed":
    case "canceled":
    case "cancelled":
    case "terminated":
    case "timed_out":
    case "continued_as_new":
      return true;
    default:
      return false;
  }
}

function pendingEvidencePlanCount(
  evidenceRequests: DiagnosisEvidenceRequest[],
  collectionResults: DiagnosisEvidenceCollectionResult[],
): number {
  return evidenceRequests.filter((request) =>
    collectionResults.every(
      (result) => !evidenceRequestsMatch(result.request, request),
    ),
  ).length;
}

function evidenceRequestsMatch(
  left: DiagnosisEvidenceRequest,
  right: DiagnosisEvidenceRequest,
): boolean {
  return (
    left.tool === right.tool &&
    normalizedValue(left.reason) === normalizedValue(right.reason) &&
    normalizedValue(left.query) === normalizedValue(right.query) &&
    left.template_id === right.template_id &&
    left.alert_source_profile_id === right.alert_source_profile_id
  );
}

function normalizedValue(value: string | undefined): string {
  return value?.trim().toLowerCase() ?? "";
}

function conclusionBlockReasonIsAuthorization(reason: string): boolean {
  const normalized = reason.toLowerCase();
  return (
    normalized.includes("not authorized") || normalized.includes("permission")
  );
}
