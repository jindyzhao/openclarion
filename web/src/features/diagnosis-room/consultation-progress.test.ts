import { describe, expect, it } from "vitest";

import {
  diagnosisConsultationConclusionLifecycleStatus,
  diagnosisConsultationReassessmentStatus,
} from "./consultation-progress";
import type {
  DiagnosisEvidenceCollectionResult,
  DiagnosisFinalConclusion,
} from "./types";

describe("diagnosis consultation progress", () => {
  it("does not require reassessment when no executable evidence was collected", () => {
    expect(
      diagnosisConsultationReassessmentStatus({
        autoFollowUpCount: 0,
        collectionResults: [
          evidenceResult({
            status: "failed",
          }),
        ],
        confidenceTimeline: [],
      }),
    ).toEqual({
      detail: "No collected executable evidence is retained for AI reassessment.",
      evidenceCount: 0,
      label: "AI reassessment not needed",
      status: "not_needed",
    });
  });

  it("marks collected executable evidence as pending until AI records a confidence checkpoint", () => {
    expect(
      diagnosisConsultationReassessmentStatus({
        autoFollowUpCount: 0,
        collectionResults: [evidenceResult()],
        confidenceTimeline: [],
      }),
    ).toEqual({
      detail:
        "Collected executable evidence is retained; ask AI to reassess confidence and conclusion status before final confirmation.",
      evidenceCount: 1,
      label: "AI reassessment pending",
      status: "pending",
    });
  });

  it("shows AI reassessment when confidence timeline reviewed collected evidence", () => {
    expect(
      diagnosisConsultationReassessmentStatus({
        autoFollowUpCount: 1,
        collectionResults: [evidenceResult()],
        confidenceTimeline: [
          {
            confidence: "medium",
            occurred_at: "2026-06-18T00:00:00Z",
            requires_human_review: true,
            turn_count: 1,
          },
          {
            confidence: "high",
            conclusion_status: "ready_for_review",
            evidence_collection_results: [evidenceResult()],
            occurred_at: "2026-06-18T00:02:00Z",
            requires_human_review: true,
            turn_count: 2,
          },
        ],
      }),
    ).toEqual({
      conclusionStatus: "ready_for_review",
      confidence: "high",
      detail:
        "AI reviewed 1 collected executable evidence item. Latest confidence: high. Conclusion status: ready_for_review.",
      evidenceCount: 1,
      label: "AI reassessed evidence",
      status: "reviewed",
      turnCount: 2,
    });
  });

  it("keeps conclusion lifecycle hidden before AI marks it reviewable", () => {
    expect(
      diagnosisConsultationConclusionLifecycleStatus({
        conclusionStatus: "needs_evidence",
      }),
    ).toEqual({
      conclusionStatus: "needs_evidence",
      detail: "AI has not produced a reviewable final conclusion yet.",
      label: "Conclusion not ready",
      status: "not_ready",
    });
  });

  it("surfaces AI-ready conclusions before final retention is available", () => {
    expect(
      diagnosisConsultationConclusionLifecycleStatus({
        conclusionStatus: "ready_for_review",
      }),
    ).toEqual({
      conclusionStatus: "ready_for_review",
      detail:
        "AI marked the diagnosis as reviewable; retain and confirm the final conclusion before closing the loop.",
      label: "Conclusion ready for review",
      status: "ready_for_review",
    });
  });

  it("shows retained final conclusions waiting for operator confirmation", () => {
    expect(
      diagnosisConsultationConclusionLifecycleStatus({
        finalConclusion: finalConclusion({
          confirmed_by: undefined,
          requires_human_review: true,
        }),
      }),
    ).toEqual({
      detail:
        "AI conclusion is available, but operator confirmation is required before final report notification.",
      label: "Operator confirmation required",
      status: "available",
    });
  });

  it("marks confirmed final conclusions as pending delivery before proof starts", () => {
    expect(
      diagnosisConsultationConclusionLifecycleStatus({
        finalConclusion: finalConclusion({
          confirmed_by: "operator@example.com",
        }),
        notificationDelivery: {
          detail: "No AI diagnosis notification phases have been retained yet.",
          status: "pending",
        },
      }),
    ).toEqual({
      confirmedBy: "operator@example.com",
      detail:
        "Operator-confirmed conclusion was retained at 2026-06-18T00:03:00Z. No AI diagnosis notification phases have been retained yet.",
      label: "Conclusion delivery pending",
      notificationStatus: "pending",
      status: "delivery_pending",
    });
  });

  it("marks confirmed conclusions as delivered when notification proof is complete", () => {
    expect(
      diagnosisConsultationConclusionLifecycleStatus({
        finalConclusion: finalConclusion({
          confirmed_by: "operator@example.com",
        }),
        notificationDelivery: {
          detail:
            "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
          status: "ready",
        },
      }),
    ).toEqual({
      confirmedBy: "operator@example.com",
      detail:
        "Operator-confirmed conclusion was retained at 2026-06-18T00:03:00Z. Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
      label: "Conclusion delivered",
      notificationStatus: "ready",
      status: "delivered",
    });
  });
});

function evidenceResult(
  overrides: Partial<DiagnosisEvidenceCollectionResult> = {},
): DiagnosisEvidenceCollectionResult {
  return {
    collected_at: "2026-06-18T00:01:00Z",
    message: "Evidence collected.",
    observed_alerts: 0,
    reason_code: "ok",
    request: {
      query: `up{job="api"}`,
      reason: "Read service availability.",
      tool: "metric_query",
    },
    status: "collected",
    tool: "metric_query",
    ...overrides,
  };
}

function finalConclusion(
  overrides: Partial<DiagnosisFinalConclusion> = {},
): DiagnosisFinalConclusion {
  return {
    confidence: "high",
    content: "Service recovered after capacity intervention.",
    recorded_at: "2026-06-18T00:03:00Z",
    requires_human_review: false,
    source: "latest_assistant_turn",
    status: "available",
    ...overrides,
  };
}
