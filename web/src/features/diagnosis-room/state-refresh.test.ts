import { describe, expect, it } from "vitest";

import {
  latestDiagnosisTurnConclusionStatus,
  latestDiagnosisTurnCount,
  shouldQueryDiagnosisStateAfterTurn,
} from "./state-refresh";
import type { DiagnosisServerFrame } from "./types";

type DiagnosisTurnResultFrame = Extract<
  DiagnosisServerFrame,
  { type: "turn_result" }
>;

describe("diagnosis room state refresh strategy", () => {
  it("does not query state for an ordinary open turn without a terminal or evidence status", () => {
    expect(shouldQueryDiagnosisStateAfterTurn(turnResult())).toBe(false);
  });

  it("queries state when the latest assistant turn is final or ready for review", () => {
    expect(
      shouldQueryDiagnosisStateAfterTurn(
        turnResult({ consultationStatus: "final" }),
      ),
    ).toBe(true);
    expect(
      shouldQueryDiagnosisStateAfterTurn(
        turnResult({ consultationStatus: "ready_for_review" }),
      ),
    ).toBe(true);
  });

  it("queries state for a needs-evidence turn without auto follow-up", () => {
    expect(
      shouldQueryDiagnosisStateAfterTurn(
        turnResult({ consultationStatus: "needs_evidence" }),
      ),
    ).toBe(true);
  });

  it("queries state after auto evidence follow-up even when more evidence is still needed", () => {
    const frame = turnResult({
      consultationStatus: "needs_evidence",
      followUpStatus: "needs_evidence",
      followUpTurnCount: 2,
    });

    expect(shouldQueryDiagnosisStateAfterTurn(frame)).toBe(true);
    expect(latestDiagnosisTurnCount(frame)).toBe(2);
    expect(latestDiagnosisTurnConclusionStatus(frame)).toBe("needs_evidence");
  });

  it("uses the latest auto follow-up conclusion status over the original turn", () => {
    const frame = turnResult({
      consultationStatus: "needs_evidence",
      followUpStatus: "ready_for_review",
      followUpTurnCount: 3,
    });

    expect(latestDiagnosisTurnConclusionStatus(frame)).toBe(
      "ready_for_review",
    );
    expect(latestDiagnosisTurnCount(frame)).toBe(3);
    expect(shouldQueryDiagnosisStateAfterTurn(frame)).toBe(true);
  });
});

function turnResult({
  consultationStatus,
  followUpStatus,
  followUpTurnCount,
}: {
  consultationStatus?: string;
  followUpStatus?: string;
  followUpTurnCount?: number;
} = {}): DiagnosisTurnResultFrame {
  return {
    type: "turn_result",
    session_id: "s1",
    chat_session_id: 7,
    message_id: "msg-1",
    assistant_message_id: "msg-1/assistant",
    user_turn_id: 11,
    assistant_turn_id: 12,
    user_sequence: 1,
    assistant_sequence: 2,
    turn_count: 1,
    context_bytes: 256,
    status: "open",
    assistant_message: "Review current alert state.",
    requires_human_review: false,
    confidence: "medium",
    consultation_insight:
      consultationStatus === undefined
        ? undefined
        : { conclusion_status: consultationStatus },
    follow_up_turns:
      followUpStatus === undefined
        ? undefined
        : [
            {
              message_id: "msg-1/auto-evidence-1",
              user_message: "OpenClarion automatic evidence follow-up.",
              assistant_message_id: "msg-1/auto-evidence-1/assistant",
              user_turn_id: 13,
              assistant_turn_id: 14,
              user_sequence: 3,
              assistant_sequence: 4,
              turn_count: followUpTurnCount ?? 2,
              context_bytes: 512,
              assistant_message: "Evidence has been reassessed.",
              requires_human_review: true,
              confidence: "medium",
              consultation_insight: {
                conclusion_status: followUpStatus,
              },
              trigger: "collected_evidence",
            },
          ],
  };
}
