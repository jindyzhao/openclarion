import { describe, expect, it } from "vitest";

import {
  diagnosisFinalConclusionConfidenceProgress,
  diagnosisFinalConclusionReviewItems,
  diagnosisFinalConclusionReasonLabel,
  diagnosisFinalConclusionRetentionState,
  diagnosisFinalConclusionSourceLabel,
  diagnosisFinalConclusionStatusLabel,
  diagnosisFinalConclusionText,
  diagnosisFinalConclusionTraceabilityStatus,
} from "./final-conclusion";

describe("diagnosis final conclusion display", () => {
  it("describes final-ready conclusion reasons without leaking enum text", () => {
    expect(
      diagnosisFinalConclusionReasonLabel("assistant_marked_ready_for_review"),
    ).toBe("AI marked ready for review");
    expect(diagnosisFinalConclusionReasonLabel("assistant_marked_final")).toBe(
      "AI marked final",
    );
  });

  it("formats known sources and falls back for unknown enum values", () => {
    expect(diagnosisFinalConclusionSourceLabel("latest_assistant_turn")).toBe(
      "Latest assistant turn",
    );
    expect(diagnosisFinalConclusionReasonLabel("operator_confirmed_final")).toBe(
      "Operator confirmed final",
    );
    expect(diagnosisFinalConclusionReasonLabel("   ")).toBeUndefined();
  });

  it("uses content first and readable reason fallback for conclusion text", () => {
    expect(
      diagnosisFinalConclusionText({
        content: "CPU saturation is bounded by rollout evidence.",
        reason: "assistant_marked_ready_for_review",
        status: "available",
      }),
    ).toBe("CPU saturation is bounded by rollout evidence.");
    expect(
      diagnosisFinalConclusionText({
        reason: "assistant_marked_ready_for_review",
        status: "available",
      }),
    ).toBe("AI marked ready for review");
  });

  it("keeps status summaries compact for room metadata", () => {
    expect(
      diagnosisFinalConclusionStatusLabel({
        confidence: "medium",
        status: "available",
      }),
    ).toBe("available (medium)");
    expect(diagnosisFinalConclusionStatusLabel({ status: "not_available" })).toBe(
      "Not available",
    );
    expect(diagnosisFinalConclusionStatusLabel(undefined)).toBe("-");
  });

  it("summarizes final conclusion retention state", () => {
    expect(diagnosisFinalConclusionRetentionState(undefined)).toEqual({
      detail: "No AI final conclusion has been retained for this room.",
      label: "No retained conclusion",
      status: "missing",
    });
    expect(
      diagnosisFinalConclusionRetentionState({
        reason: "assistant_marked_ready_for_review",
        status: "not_available",
      }),
    ).toEqual({
      detail: "AI marked ready for review",
      label: "Conclusion not available",
      status: "missing",
    });
    expect(
      diagnosisFinalConclusionRetentionState({
        requires_human_review: true,
        status: "available",
      }),
    ).toEqual({
      detail:
        "AI conclusion is available, but operator confirmation is required before final report notification.",
      label: "Operator confirmation required",
      status: "needs_review",
    });
    expect(
      diagnosisFinalConclusionRetentionState({
        requires_human_review: false,
        status: "available",
      }),
    ).toEqual({
      detail:
        "AI conclusion is available for operator confirmation before final report notification.",
      label: "Conclusion ready",
      status: "ready",
    });
    expect(
      diagnosisFinalConclusionRetentionState({
        confirmed_by: "operator-1",
        recorded_at: "2026-06-21T10:00:00Z",
        status: "available",
      }),
    ).toEqual({
      detail:
        "Operator-confirmed conclusion was retained at 2026-06-21T10:00:00Z.",
      label: "Conclusion retained",
      status: "retained",
    });
  });

  it("builds review items for final conclusions with remaining evidence gaps", () => {
    expect(
      diagnosisFinalConclusionReviewItems({
        confidence: "medium",
        confidence_rationale: "Operator restart evidence has not been attached.",
        evidence_collection_suggestions: [{}],
        evidence_requests: [{}],
        missing_evidence_requests: [{}],
        requires_human_review: true,
        status: "available",
      }),
    ).toEqual([
      {
        detail: "Operator restart evidence has not been attached.",
        key: "confidence-rationale",
        status: "residual",
        title: "Confidence: medium",
      },
      {
        detail:
          "1 missing evidence request(s), 1 collection suggestion(s), 1 executable evidence request(s)",
        key: "evidence-gaps",
        status: "attention",
        title: "Evidence blockers need review",
      },
      {
        detail:
          "Operator confirmation is required before retaining this conclusion.",
        key: "retention",
        status: "attention",
        title: "Awaiting operator confirmation",
      },
    ]);
  });

  it("adds actionable evidence gap labels to final conclusion review items", () => {
    expect(
      diagnosisFinalConclusionReviewItems({
        confidence: "medium",
        evidence_collection_suggestions: [
          {
            detail: "Collect a bounded CPU range query before final review.",
            label: "CPU saturation sample",
            priority: "high",
          },
        ],
        evidence_requests: [
          {
            reason: "Verify sibling alerts in the active Alertmanager window.",
            tool: "active_alerts",
          },
        ],
        missing_evidence_requests: [
          {
            detail: "Attach the latest owner remediation note.",
            label: "Owner action",
            priority: "high",
          },
        ],
        requires_human_review: true,
        status: "available",
      })[1],
    ).toEqual({
      detail:
        "1 missing evidence request(s), 1 collection suggestion(s), 1 executable evidence request(s). Next evidence: Missing: Owner action - Attach the latest owner remediation note.; Suggestion: CPU saturation sample - Collect a bounded CPU range query before final review.; Executable: active_alerts - Verify sibling alerts in the active Alertmanager window.",
      key: "evidence-gaps",
      status: "attention",
      title: "Evidence blockers need review",
    });
  });

  it("keeps collection suggestions as residual uncertainty", () => {
    expect(
      diagnosisFinalConclusionReviewItems({
        confidence: "high",
        evidence_collection_suggestions: [
          {
            detail: "Collect one more bounded CPU sample if the operator wants stronger evidence.",
            label: "CPU residual sample",
            priority: "low",
          },
        ],
        status: "available",
      })[1],
    ).toEqual({
      detail:
        "1 collection suggestion(s). Next evidence: Suggestion: CPU residual sample - Collect one more bounded CPU sample if the operator wants stronger evidence.",
      key: "evidence-gaps",
      status: "residual",
      title: "Residual collection suggestions",
    });
  });

  it("marks cleared evidence gaps and retained confirmation", () => {
    expect(
      diagnosisFinalConclusionReviewItems({
        confidence: "high",
        confirmed_by: "operator-1",
        recorded_at: "2026-06-21T10:00:00Z",
        status: "available",
      }),
    ).toEqual([
      {
        detail: "AI reported high confidence without additional rationale.",
        key: "confidence-rationale",
        status: "done",
        title: "Confidence: high",
      },
      {
        detail:
          "No remaining missing evidence, collection suggestions, or executable evidence requests were retained with the final conclusion.",
        key: "evidence-gaps",
        status: "done",
        title: "Evidence gaps cleared",
      },
      {
        detail:
          "Operator-confirmed conclusion was retained at 2026-06-21T10:00:00Z.",
        key: "retention",
        status: "done",
        title: "Conclusion retained",
      },
    ]);
  });

  it("asks operators to wait when no final conclusion is available", () => {
    expect(diagnosisFinalConclusionReviewItems(undefined)).toEqual([
      {
        detail: "Wait for AI to produce a reviewable conclusion.",
        key: "conclusion-available",
        status: "attention",
        title: "Conclusion unavailable",
      },
    ]);
  });

  it("summarizes confidence improvement from timeline to final conclusion", () => {
    expect(
      diagnosisFinalConclusionConfidenceProgress(
        {
          confidence: "high",
          confidence_rationale:
            "Supplemental restart logs and active alert evidence support the conclusion.",
          status: "available",
        },
        [
          { confidence: "low", turn_count: 1 },
          { confidence: "medium", turn_count: 2 },
        ],
      ),
    ).toEqual({
      detail:
        "Confidence improved from low to high. Supplemental restart logs and active alert evidence support the conclusion.",
      finalConfidence: "high",
      initialConfidence: "low",
      label: "Confidence improved",
      status: "improved",
    });
  });

  it("summarizes stable and declined confidence progress", () => {
    expect(
      diagnosisFinalConclusionConfidenceProgress(
        { confidence: "medium", status: "available" },
        [{ confidence: "medium", confidence_rationale: "Evidence is bounded." }],
      ),
    ).toEqual({
      detail: "Confidence remained medium. Evidence is bounded.",
      finalConfidence: "medium",
      initialConfidence: "medium",
      label: "Confidence stable",
      status: "stable",
    });

    expect(
      diagnosisFinalConclusionConfidenceProgress(
        { confidence: "low", status: "available" },
        [{ confidence: "high" }],
      ),
    ).toEqual({
      detail: "Confidence declined from high to low.",
      finalConfidence: "low",
      initialConfidence: "high",
      label: "Confidence declined",
      status: "declined",
    });
  });

  it("keeps confidence progress unknown when checkpoints are incomplete", () => {
    expect(
      diagnosisFinalConclusionConfidenceProgress(
        { confidence: "high", status: "available" },
        [],
      ),
    ).toEqual({
      detail:
        "Confidence progress is unavailable until at least one checkpoint and final confidence are present.",
      finalConfidence: "high",
      initialConfidence: "unknown",
      label: "Confidence progress unavailable",
      status: "unknown",
    });
  });

  it("marks closure traceability complete only after retention and AI delivery proof", () => {
    expect(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: {
          confidence: "high",
          confirmed_by: "operator-1",
          recorded_at: "2026-06-21T10:00:00Z",
          status: "available",
        },
        notificationDelivery: {
          detail:
            "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
          label: "AI delivery complete",
          readyCount: 3,
          requiredCount: 3,
          status: "ready",
        },
      }),
    ).toEqual({
      color: "success",
      detail:
        "Operator-confirmed conclusion is retained, review checklist is clear, and Enterprise WeChat AI delivery proof covers all required phases.",
      label: "Closure traceability complete",
      notificationLabel: "AI delivery complete",
      reviewOpenCount: 0,
      reviewResidualCount: 0,
      status: "complete",
    });
  });

  it("allows documented residual uncertainty after operator confirmation", () => {
    expect(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: {
          confidence: "medium",
          confirmed_by: "operator-1",
          evidence_collection_suggestions: [
            {
              detail: "Collect another bounded CPU sample if time allows.",
              label: "Residual CPU sample",
              priority: "low",
            },
          ],
          status: "available",
        },
        notificationDelivery: {
          detail:
            "Enterprise WeChat timeline covers AI updates, final conclusion, and close notification with retained AI output proof.",
          label: "AI delivery complete",
          readyCount: 3,
          requiredCount: 3,
          status: "ready",
        },
      }),
    ).toEqual({
      color: "success",
      detail:
        "Operator-confirmed conclusion is retained, blocking review checklist is clear with 2 residual review item(s) documented, and Enterprise WeChat AI delivery proof covers all required phases.",
      label: "Closure traceability complete",
      notificationLabel: "AI delivery complete",
      reviewOpenCount: 0,
      reviewResidualCount: 2,
      status: "complete",
    });
  });


  it("keeps closure traceability in review before operator confirmation", () => {
    expect(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: {
          confidence: "medium",
          confidence_rationale: "Waiting for supplemental log evidence.",
          missing_evidence_requests: [{}],
          requires_human_review: true,
          status: "available",
        },
        notificationDelivery: {
          detail: "No AI diagnosis notification phases have been retained yet.",
          label: "AI delivery not started",
          readyCount: 0,
          requiredCount: 3,
          status: "pending",
        },
      }),
    ).toEqual({
      color: "warning",
      detail:
        "AI conclusion is available, but operator confirmation is required before final report notification. 2 blocking review item(s) are still open before closure can be accepted.",
      label: "Closure traceability needs review",
      notificationLabel: "AI delivery not started",
      reviewOpenCount: 2,
      reviewResidualCount: 1,
      status: "review",
    });
  });

  it("surfaces blocked delivery proof after retention", () => {
    expect(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: {
          confidence: "high",
          confirmed_by: "operator-1",
          status: "available",
        },
        notificationDelivery: {
          detail:
            "At least one required AI notification phase failed. Retry the failed delivery before treating the room as delivered.",
          label: "AI delivery failed",
          readyCount: 2,
          requiredCount: 3,
          status: "blocked",
        },
      }),
    ).toEqual({
      color: "error",
      detail:
        "Operator-confirmed conclusion is retained, but AI delivery proof is blocked: At least one required AI notification phase failed. Retry the failed delivery before treating the room as delivered.",
      label: "Closure traceability blocked",
      notificationLabel: "AI delivery failed",
      reviewOpenCount: 0,
      reviewResidualCount: 0,
      status: "blocked",
    });
  });

  it("keeps retained conclusions pending until delivery proof starts", () => {
    expect(
      diagnosisFinalConclusionTraceabilityStatus({
        conclusion: {
          confidence: "high",
          confirmed_by: "operator-1",
          status: "available",
        },
        notificationDelivery: {
          detail: "No AI diagnosis notification phases have been retained yet.",
          label: "AI delivery not started",
          readyCount: 0,
          requiredCount: 3,
          status: "pending",
        },
      }),
    ).toEqual({
      color: "default",
      detail:
        "Operator-confirmed conclusion is retained; AI delivery proof has not started (0 of 3 phase(s) complete).",
      label: "Closure delivery proof pending",
      notificationLabel: "AI delivery not started",
      reviewOpenCount: 0,
      reviewResidualCount: 0,
      status: "pending",
    });
  });
});
