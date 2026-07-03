import { describe, expect, it } from "vitest";

import {
  diagnosisRoomAdministerAuthorizationKey,
  diagnosisRoomCreateAuthorizationKey,
  diagnosisRoomParticipateAuthorizationKey,
  diagnosisRoomReadAuthorizationKey,
  type DiagnosisRoomRBACPermissionItem,
} from "./rbac-capabilities";
import type {
  DiagnosisEvidenceCollectionResult,
  DiagnosisStateFrame,
} from "./types";
import {
  diagnosisActionIdentityBlockReason,
  diagnosisWorkflowReadiness,
  diagnosisWorkflowReadinessReviewQueueSource,
} from "./workflow-readiness";

const selectedRoomPermissions: DiagnosisRoomRBACPermissionItem[] = [
  {
    action: "create",
    key: diagnosisRoomCreateAuthorizationKey,
    label: "Create rooms",
    permission: "diagnosis_room.participate",
    scopeLabel: "Global",
    status: "allowed",
  },
  {
    action: "read",
    key: diagnosisRoomReadAuthorizationKey("room-1"),
    label: "Read room",
    permission: "diagnosis_room.read",
    scopeLabel: "room-1",
    status: "allowed",
  },
  {
    action: "participate",
    key: diagnosisRoomParticipateAuthorizationKey("room-1"),
    label: "Participate",
    permission: "diagnosis_room.participate",
    scopeLabel: "room-1",
    status: "allowed",
  },
  {
    action: "administer",
    key: diagnosisRoomAdministerAuthorizationKey("room-1"),
    label: "Administer",
    permission: "diagnosis_room.administer",
    scopeLabel: "room-1",
    status: "allowed",
  },
];

describe("diagnosis workflow readiness", () => {
  it("blocks room actions when the current actor subject is not known", () => {
    expect(
      diagnosisActionIdentityBlockReason(" ", "sending evidence updates"),
    ).toBe("Authenticate as an operator before sending evidence updates.");
    expect(
      diagnosisActionIdentityBlockReason(
        "iam:user-1",
        "sending evidence updates",
      ),
    ).toBe("");
  });

  it("prefers live review queue state and falls back to saved room summaries", () => {
    expect(
      diagnosisWorkflowReadinessReviewQueueSource({
        latestInsightLoaded: true,
        savedReviewQueueLoaded: true,
      }),
    ).toBe("live");
    expect(
      diagnosisWorkflowReadinessReviewQueueSource({
        latestInsightLoaded: false,
        savedReviewQueueLoaded: true,
      }),
    ).toBe("saved");
    expect(
      diagnosisWorkflowReadinessReviewQueueSource({
        latestInsightLoaded: false,
        savedReviewQueueLoaded: false,
      }),
    ).toBe("none");
  });

  it("keeps the manual test flow pending before identity and room selection", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Wait until AI marks the diagnosis final or ready for review.",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      permissionItems: selectedRoomPermissions.map((item) => ({
        ...item,
        status: item.action === "create" ? "allowed" : "not-selected",
      })),
      selectedSessionID: "",
      state: null,
    });

    expect(items.map((item) => [item.key, item.status])).toEqual([
      ["identity", "pending"],
      ["room", "pending"],
      ["connection", "pending"],
      ["permissions", "pending"],
      ["evidence", "pending"],
      ["conclusion", "pending"],
    ]);
  });

  it("blocks the flow when scoped room permission is denied", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Current user is not authorized to administer this diagnosis room.",
      connected: true,
      connectionStatus: "connected",
      latestInsight: null,
      permissionItems: selectedRoomPermissions.map((item) => ({
        ...item,
        status: item.action === "administer" ? "denied" : item.status,
      })),
      selectedRoomStatus: "open",
      selectedSessionID: "room-1",
      state: null,
    });

    expect(items.find((item) => item.key === "permissions")).toMatchObject({
      status: "blocked",
    });
    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "blocked",
    });
  });

  it("surfaces open evidence work before final confirmation", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Resolve remaining evidence review items before confirming the conclusion.",
      connected: true,
      connectionStatus: "connected",
      latestInsight: {
        collectionResults: [
          {
            collected_at: "2026-06-29T01:00:00Z",
            message: "Collected active alerts.",
            observed_alerts: 2,
            reason_code: "ok",
            request: {
              reason: "Check active alerts.",
              tool: "active_alerts",
            },
            status: "collected",
            tool: "active_alerts",
          },
        ],
        evidenceRequests: [
          {
            reason: "Check active alerts.",
            tool: "active_alerts",
          },
          {
            query: "rate(container_cpu_usage_seconds_total[5m])",
            reason: "Check CPU saturation.",
            tool: "prometheus_range",
          },
        ],
        insight: {
          conclusion_status: "needs_evidence",
          missing_evidence_requests: [
            {
              detail: "Attach owner remediation notes.",
              label: "Owner evidence",
              priority: "high",
            },
          ],
        },
      },
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "open",
      selectedSessionID: "room-1",
      state: null,
    });

    expect(items.find((item) => item.key === "evidence")).toMatchObject({
      metric: "1/1 collected",
      status: "attention",
    });
    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "pending",
    });
  });

  it("uses REST summary evidence gaps when live insight is not loaded", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Wait until AI marks the diagnosis final or ready for review.",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "open",
      selectedSessionID: "room-1",
      state: null,
      summaryEvidence: {
        collectionResults: [collectionResult("collected")],
        evidenceCollectionSuggestions: [
          {
            detail: "Collect a bounded JVM memory query.",
            label: "JVM memory",
            priority: "medium",
          },
        ],
        evidenceRequests: [
          {
            query: "rate(container_cpu_usage_seconds_total[5m])",
            reason: "Check CPU saturation.",
            tool: "prometheus_range",
          },
        ],
        missingEvidenceRequests: [
          {
            detail: "Attach owner remediation notes.",
            label: "Owner evidence",
            priority: "high",
          },
        ],
        supplementalEvidence: [],
      },
    });

    expect(items.find((item) => item.key === "evidence")).toMatchObject({
      metric: "1/1 collected",
      status: "attention",
    });
  });

  it("blocks REST summary evidence when collection failed", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Wait until AI marks the diagnosis final or ready for review.",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "open",
      selectedSessionID: "room-1",
      state: null,
      summaryEvidence: {
        collectionResults: [collectionResult("failed")],
        evidenceCollectionSuggestions: [],
        evidenceRequests: [],
        missingEvidenceRequests: [],
        supplementalEvidence: [],
      },
    });

    expect(items.find((item) => item.key === "evidence")).toMatchObject({
      metric: "0/1 collected",
      status: "blocked",
    });
  });

  it("keeps retained conclusions pending until delivery proof is available", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: false,
      connectionStatus: "closed",
      latestInsight: {
        collectionResults: [],
        evidenceRequests: [],
        insight: {
          conclusion_status: "final",
        },
      },
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: retainedConclusionState(),
    });

    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "pending",
    });
    expect(items.find((item) => item.key === "connection")).toMatchObject({
      status: "ready",
    });
  });

  it("does not require a live connection for closed room review", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: false,
      connectionStatus: "closed",
      latestInsight: null,
      notificationDelivery: {
        detail:
          "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
        label: "AI delivery complete",
        readyCount: 3,
        requiredCount: 3,
        status: "ready",
      },
      permissionItems: selectedRoomPermissions,
      retainedConclusion: retainedConclusionSummary(),
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: null,
      workflowVisibility: {
        status: "completed",
      },
    });

    expect(items.find((item) => item.key === "room")).toMatchObject({
      status: "ready",
    });
    expect(items.find((item) => item.key === "connection")).toMatchObject({
      status: "ready",
    });
  });

  it("blocks open rooms when workflow visibility is unavailable", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "Wait until AI marks the diagnosis final or ready for review.",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "open",
      selectedSessionID: "room-1",
      state: null,
      workflowVisibility: {
        status: "not_found",
      },
    });

    expect(items.find((item) => item.key === "room")).toMatchObject({
      status: "blocked",
    });
    expect(items.find((item) => item.key === "connection")).toMatchObject({
      status: "blocked",
    });
  });

  it("marks the loop ready after retained conclusion and complete delivery proof", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: false,
      connectionStatus: "closed",
      latestInsight: {
        collectionResults: [],
        evidenceRequests: [],
        insight: {
          conclusion_status: "final",
        },
      },
      notificationDelivery: {
        detail:
          "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
        label: "AI delivery complete",
        readyCount: 3,
        requiredCount: 3,
        status: "ready",
      },
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: retainedConclusionState(),
    });

    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "ready",
    });
  });

  it("uses retained REST conclusion summaries when live state is not loaded", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      notificationDelivery: {
        detail:
          "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
        label: "AI delivery complete",
        readyCount: 3,
        requiredCount: 3,
        status: "ready",
      },
      permissionItems: selectedRoomPermissions,
      retainedConclusion: retainedConclusionSummary(),
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: null,
    });

    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "ready",
    });
  });

  it("keeps retained REST conclusion summaries pending without delivery proof", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: false,
      connectionStatus: "idle",
      latestInsight: null,
      permissionItems: selectedRoomPermissions,
      retainedConclusion: retainedConclusionSummary(),
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: null,
    });

    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "pending",
    });
  });

  it("blocks retained conclusions when delivery proof failed", () => {
    const items = diagnosisWorkflowReadiness({
      actorSubject: "iam:user-1",
      canConfirmConclusion: false,
      confirmConclusionBlockReason: "",
      connected: true,
      connectionStatus: "connected",
      latestInsight: {
        collectionResults: [],
        evidenceRequests: [],
        insight: {
          conclusion_status: "final",
        },
      },
      notificationDelivery: {
        detail:
          "At least one required AI notification phase failed. Retry the failed delivery before treating the room as delivered.",
        label: "AI delivery failed",
        readyCount: 2,
        requiredCount: 3,
        status: "blocked",
      },
      permissionItems: selectedRoomPermissions,
      selectedRoomStatus: "closed",
      selectedSessionID: "room-1",
      state: retainedConclusionState(),
    });

    expect(items.find((item) => item.key === "conclusion")).toMatchObject({
      status: "blocked",
    });
  });
});

function retainedConclusionState(): DiagnosisStateFrame {
  return {
    chat_session_id: 1,
    conversation: [],
    diagnosis_task_id: 1,
    final_conclusion: {
      confidence: "high",
      confirmed_by: "iam:user-1",
      recorded_at: "2026-06-29T01:05:00Z",
      source: "latest_assistant_turn",
      status: "available",
    },
    in_flight: false,
    last_activity_at: "2026-06-29T01:05:00Z",
    owner_subject: "iam:user-1",
    seen_message_ids: [],
    session_id: "room-1",
    started_at: "2026-06-29T01:00:00Z",
    status: "closed",
    turn_count: 2,
    type: "state",
  };
}

function retainedConclusionSummary() {
  return {
    confidence: "high",
    confirmed_by: "iam:user-1",
    content: "CPU saturation was mitigated and verified.",
    recorded_at: "2026-06-29T01:05:00Z",
    source: "latest_assistant_turn",
    status: "available",
  };
}

function collectionResult(
  status: DiagnosisEvidenceCollectionResult["status"],
): DiagnosisEvidenceCollectionResult {
  return {
    collected_at: "2026-06-29T01:00:00Z",
    message: "Collected active alerts.",
    observed_alerts: 2,
    reason_code: status === "collected" ? "ok" : "provider_error",
    request: {
      reason: "Check active alerts.",
      tool: "active_alerts",
    },
    status,
    tool: "active_alerts",
  };
}
