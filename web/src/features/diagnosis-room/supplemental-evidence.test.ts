import { describe, expect, it } from "vitest";

import {
  supplementalEvidenceReviewedByAssistantSequence,
  supplementalEvidencePriorityFromText,
  supplementalEvidenceWirePayload,
} from "./supplemental-evidence";
import type { DiagnosisConsultationEvidenceRequest } from "./types";

describe("supplemental evidence helpers", () => {
  it("normalizes URL-sourced supplemental evidence priorities", () => {
    expect(supplementalEvidencePriorityFromText(" HIGH ")).toBe("high");
    expect(supplementalEvidencePriorityFromText("medium")).toBe("medium");
    expect(supplementalEvidencePriorityFromText("Low")).toBe("low");
    expect(supplementalEvidencePriorityFromText("urgent")).toBeUndefined();
  });

  it("requires matching assistant sequence proof before treating evidence as reviewed", () => {
    expect(
      supplementalEvidenceReviewedByAssistantSequence(
        { assistant_sequence: 4 },
        4,
      ),
    ).toBe(true);
    expect(
      supplementalEvidenceReviewedByAssistantSequence(
        { assistant_sequence: 3 },
        4,
      ),
    ).toBe(false);
    expect(
      supplementalEvidenceReviewedByAssistantSequence(
        { assistant_sequence: 4 },
        undefined,
      ),
    ).toBe(false);
    expect(
      supplementalEvidenceReviewedByAssistantSequence({}, 4),
    ).toBe(false);
  });

  it("strips local source request context from the WebSocket payload", () => {
    const request = supplementalRecoveryRequest();

    expect(supplementalEvidenceWirePayload(request, "  Operator evidence.  ")).toEqual({
      detail:
        "Provide verified alternative evidence for the skipped metric query.",
      evidence: "Operator evidence.",
      label: "metric_range_query evidence recovery",
      priority: "high",
    });
  });

});

function supplementalRecoveryRequest(): DiagnosisConsultationEvidenceRequest {
  return {
    detail: "Provide verified alternative evidence for the skipped metric query.",
    label: "metric_range_query evidence recovery",
    priority: "high",
    source_request: {
      alert_source_profile_id: 7,
      limit: 20,
      query: `rate(container_cpu_usage_seconds_total{namespace="prod"}[5m])`,
      reason: "Read CPU saturation trend.",
      step_seconds: 60,
      tool: "metric_range_query",
      window_seconds: 300,
    },
  };
}
