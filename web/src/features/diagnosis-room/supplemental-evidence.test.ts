import { describe, expect, it } from "vitest";

import {
  supplementalEvidenceReviewedByAssistantSequence,
  supplementalEvidencePriorityFromText,
  supplementalEvidenceReassessmentMessage,
  supplementalEvidenceResidualBoundaryTemplate,
  supplementalEvidenceSubmissionMessage,
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

  it("includes source request context in submission messages", () => {
    const request = supplementalRecoveryRequest();
    const submission = supplementalEvidenceSubmissionMessage(
      request,
      "Operator provided query output.",
    );

    expect(submission).toContain(
      [
        "Original executable evidence request:",
        "Tool: metric_range_query",
        "Reason: Read CPU saturation trend.",
        `Query: rate(container_cpu_usage_seconds_total{namespace="prod"}[5m])`,
        "Alert source profile: 7",
        "Window: 300s",
        "Step: 60s",
        "Limit: 20",
      ].join("\n"),
    );
    expect(submission).toContain(
      "Evidence provided:\nOperator provided query output.",
    );
    expect(submission).toContain(
      [
        "Review instruction:",
        "1. Decide whether the submitted evidence satisfies the requested evidence item.",
        "2. State the updated confidence and explain whether confidence improved, stayed the same, or declined.",
        "3. Update missing_evidence_requests, evidence_requests, and evidence_collection_suggestions so only materially different remaining gaps are listed.",
        "4. Produce a bounded ready_for_review conclusion with requires_human_review=true when no additional executable evidence is needed.",
        "5. Keep final reserved for conclusions without unresolved evidence, and never fabricate unavailable facts.",
      ].join("\n"),
    );
  });

  it("builds a bounded residual uncertainty template for unavailable evidence", () => {
    expect(
      supplementalEvidenceResidualBoundaryTemplate(supplementalRecoveryRequest()),
    ).toBe(
      [
        "Operator reviewed the requested follow-up for metric_range_query evidence recovery.",
        "Requested artifact: Provide verified alternative evidence for the skipped metric query.",
        "The requested artifact is not available in this validation window.",
        "Operator accepts this as residual uncertainty for review purposes.",
        "Do not fabricate unavailable facts and do not repeat this same artifact request unless a new executable evidence path is available.",
      ].join("\n"),
    );
  });

  it("builds a bounded AI reassessment prompt for submitted evidence", () => {
    const prompt = supplementalEvidenceReassessmentMessage();

    expect(prompt).toContain("Reassess retained diagnosis evidence");
    expect(prompt).toContain("Update confidence, confidence rationale, remaining evidence gaps, and conclusion status");
    expect(prompt).toContain("satisfies, partially satisfies, or fails");
    expect(prompt).toContain("confidence improved, stayed the same, or declined");
    expect(prompt).toContain("Treat collected executable evidence as verified input");
    expect(prompt).toContain("missing_evidence_requests, evidence_requests, evidence_collection_suggestions");
    expect(prompt).toContain("conclusion_status, and requires_human_review");
    expect(prompt).toContain("Do not repeat an evidence request that has been satisfied");
    expect(prompt).toContain("ready_for_review with requires_human_review=true");
  });

  it("includes pending supplemental evidence excerpts in reassessment prompts", () => {
    const prompt = supplementalEvidenceReassessmentMessage({
      latestAssistantSequence: 4,
      records: [
        supplementalEvidenceRecord({
          assistant_sequence: 4,
          evidence: "Already reviewed.",
          label: "Reviewed evidence",
          provided_at: "2026-06-18T00:01:00Z",
        }),
        supplementalEvidenceRecord({
          assistant_sequence: 3,
          detail: "Attach the latest owner remediation note.",
          evidence: "Owner expanded the database tablespace at 00:05 UTC.",
          label: "Owner action",
          priority: "high",
          provided_at: "2026-06-18T00:05:00Z",
        }),
      ],
    });

    expect(prompt).toContain("Submitted evidence awaiting reassessment:");
    expect(prompt).toContain("1. Owner action (high)");
    expect(prompt).toContain(
      "Requested detail: Attach the latest owner remediation note.",
    );
    expect(prompt).toContain(
      "Evidence excerpt: Owner expanded the database tablespace at 00:05 UTC.",
    );
    expect(prompt).not.toContain("Reviewed evidence");
  });

  it("includes executable evidence collection results in reassessment prompts", () => {
    const prompt = supplementalEvidenceReassessmentMessage({
      collectionResults: [
        {
          collected_at: "2026-06-18T00:10:00Z",
          message: "PromQL query returned two saturated CPU series.",
          observed_alerts: 0,
          observed_metric_series: 2,
          query: `rate(container_cpu_usage_seconds_total{namespace="prod"}[5m])`,
          reason_code: "ok",
          request: {
            query: `rate(container_cpu_usage_seconds_total{namespace="prod"}[5m])`,
            reason: "Read CPU saturation trend.",
            tool: "metric_query",
          },
          status: "collected",
          tool: "metric_query",
        },
      ],
    });

    expect(prompt).toContain(
      "Executable evidence collection retained for reassessment:",
    );
    expect(prompt).toContain("1. metric_query (collected)");
    expect(prompt).toContain("Reason: Read CPU saturation trend.");
    expect(prompt).toContain(
      "Message: PromQL query returned two saturated CPU series.",
    );
    expect(prompt).toContain("Original executable evidence request:");
    expect(prompt).toContain(
      `Query: rate(container_cpu_usage_seconds_total{namespace="prod"}[5m])`,
    );
  });

  it("limits reassessment prompt evidence excerpts", () => {
    const longEvidence = `${"CPU sample ".repeat(60)}tail`;
    const prompt = supplementalEvidenceReassessmentMessage({
      records: [
        supplementalEvidenceRecord({
          evidence: longEvidence,
          label: "CPU range sample",
        }),
      ],
    });

    const excerptLine = prompt
      .split("\n")
      .find((line) => line.startsWith("Evidence excerpt: "));

    expect(excerptLine).toBeDefined();
    expect(excerptLine?.endsWith("...")).toBe(true);
    expect(excerptLine!.length).toBeLessThanOrEqual(
      "Evidence excerpt: ".length + 360,
    );
    expect(excerptLine).not.toContain("tail");
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

function supplementalEvidenceRecord(
  overrides: Partial<{
    assistant_sequence: number;
    detail: string;
    evidence: string;
    label: string;
    priority: string;
    provided_at: string;
  }> = {},
) {
  return {
    assistant_message_id: "assistant-1",
    assistant_sequence: overrides.assistant_sequence ?? 0,
    assistant_turn_id: 0,
    detail: overrides.detail ?? "Attach supporting operator evidence.",
    evidence: overrides.evidence ?? "Operator evidence.",
    label: overrides.label ?? "Operator action",
    priority: overrides.priority ?? "medium",
    provided_at: overrides.provided_at ?? "2026-06-18T00:00:00Z",
    user_message_id: "user-1",
    user_sequence: 1,
    user_turn_id: 1,
  };
}
