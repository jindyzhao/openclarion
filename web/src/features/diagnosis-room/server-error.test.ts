import { describe, expect, it } from "vitest";

import { diagnosisServerErrorDisplay } from "./server-error";

describe("diagnosisServerErrorDisplay", () => {
  it("returns nothing when there is no server error", () => {
    expect(diagnosisServerErrorDisplay(null)).toBeNull();
  });

  it("explains confirmation rejections as operator-actionable warnings", () => {
    expect(
      diagnosisServerErrorDisplay({
        code: "confirm_rejected",
        message: "resolve missing evidence requests before confirming",
      }),
    ).toEqual({
      actionLabel: "Review evidence tasks",
      actionTitle:
        "Jump to the review queue that contains the evidence or reassessment task blocking confirmation.",
      description:
        "resolve missing evidence requests before confirming Open the review queue, add the requested operator evidence, ask AI to reassess submitted evidence when needed, then retry confirmation.",
      message: "Conclusion cannot be confirmed yet",
      type: "warning",
    });
  });

  it("maps planned evidence confirmation rejections to collection recovery", () => {
    expect(
      diagnosisServerErrorDisplay({
        code: "confirm_rejected",
        message: "collect planned executable evidence before confirming",
      }),
    ).toMatchObject({
      description:
        "collect planned executable evidence before confirming Open the review queue, run the pending executable evidence collection, let AI reassess the collected evidence, then retry confirmation.",
      message: "Conclusion cannot be confirmed yet",
      type: "warning",
    });
  });

  it("keeps recoverable LLM errors visible while guiding state recovery", () => {
    expect(
      diagnosisServerErrorDisplay({
        code: "llm_timeout",
        message:
          "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
      }),
    ).toEqual({
      description:
        "Diagnosis turn failed before an assistant response; upstream LLM request timed out. Query the latest room state; if the turn did not progress, retry the operator message or provide narrower evidence.",
      message: "Diagnosis request failed: llm_timeout",
      type: "warning",
    });
  });

  it("treats unknown errors as hard request failures", () => {
    expect(
      diagnosisServerErrorDisplay({
        code: "mock_backend_error",
        message: "mock backend rejected the diagnosis request",
      }),
    ).toEqual({
      description: "mock backend rejected the diagnosis request",
      message: "Diagnosis request failed: mock_backend_error",
      type: "error",
    });
  });
});
