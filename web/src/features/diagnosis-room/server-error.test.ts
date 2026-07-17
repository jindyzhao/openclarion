import { describe, expect, it } from "vitest";

import { diagnosisServerErrorPresentation } from "./server-error";

describe("diagnosisServerErrorPresentation", () => {
  it("returns nothing when there is no server error", () => {
    expect(diagnosisServerErrorPresentation(null)).toBeNull();
  });

  it("classifies confirmation rejections without parsing message prose", () => {
    const first = diagnosisServerErrorPresentation({
      code: "confirm_rejected",
      message: "resolve missing evidence requests before confirming",
    });
    const changedWording = diagnosisServerErrorPresentation({
      code: "confirm_rejected",
      message: "结论尚未满足确认条件",
    });

    expect(first).toMatchObject({
      action: "review_evidence_tasks",
      kind: "confirmation_rejected",
      type: "warning",
    });
    expect(changedWording).toMatchObject({
      action: "review_evidence_tasks",
      detail: "结论尚未满足确认条件",
      kind: "confirmation_rejected",
      type: "warning",
    });
  });

  it("classifies recoverable LLM failures by stable code", () => {
    expect(
      diagnosisServerErrorPresentation({
        code: "llm_timeout",
        message: "upstream timed out",
      }),
    ).toEqual({
      action: null,
      code: "llm_timeout",
      detail: "upstream timed out",
      kind: "recoverable",
      type: "warning",
    });
  });

  it("treats unknown errors as fatal request failures", () => {
    expect(
      diagnosisServerErrorPresentation({
        code: "backend_error",
        message: "backend rejected the request",
      }),
    ).toEqual({
      action: null,
      code: "backend_error",
      detail: "backend rejected the request",
      kind: "fatal",
      type: "error",
    });
  });
});
