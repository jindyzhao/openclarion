import { describe, expect, it } from "vitest";

import { nextDiagnosisTurnPreview } from "./stream-preview";
import type { DiagnosisTurnStreamFrame } from "./types";

function frame(
  overrides: Partial<DiagnosisTurnStreamFrame> = {},
): DiagnosisTurnStreamFrame {
  return {
    type: "turn_stream",
    phase: "delta",
    session_id: "session-1",
    message_id: "message-1",
    assistant_message_id: "message-1/assistant",
    activity_attempt: 1,
    generation_attempt: 1,
    sequence: 1,
    assistant_message: "Draft",
    ...overrides,
  };
}

describe("nextDiagnosisTurnPreview", () => {
  it("keeps the newest monotonic snapshot", () => {
    const current = frame({ sequence: 2, assistant_message: "Newer" });
    expect(nextDiagnosisTurnPreview(current, frame())).toBe(current);
    expect(
      nextDiagnosisTurnPreview(
        current,
        frame({ sequence: 3, assistant_message: "Newest" }),
      ).assistant_message,
    ).toBe("Newest");
  });

  it("resets content for activity and generation retries", () => {
    const current = frame({ sequence: 8, assistant_message: "Invalid draft" });
    expect(
      nextDiagnosisTurnPreview(
        current,
        frame({
          phase: "started",
          activity_attempt: 2,
          generation_attempt: 0,
          sequence: 0,
          assistant_message: "",
        }),
      ).assistant_message,
    ).toBe("");
    expect(
      nextDiagnosisTurnPreview(
        current,
        frame({
          generation_attempt: 2,
          sequence: 1,
          assistant_message: "Corrected",
        }),
      ).assistant_message,
    ).toBe("Corrected");
  });
});
