import { describe, expect, it } from "vitest";

import {
  diagnosisStateTranscript,
  diagnosisTurnResultTranscript,
} from "./transcript";
import type { DiagnosisServerFrame, DiagnosisStateFrame } from "./types";

type DiagnosisTurnResultFrame = Extract<
  DiagnosisServerFrame,
  { type: "turn_result" }
>;

describe("diagnosis transcript helpers", () => {
  it("builds transcript turns from a turn result and its follow-ups", () => {
    expect(
      diagnosisTurnResultTranscript({
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
        assistant_message: "Need current active alerts.",
        requires_human_review: true,
        confidence: "low",
        evidence_timeline: [
          {
            turn_count: 1,
            message_id: "msg-1",
            assistant_message_id: "msg-1/assistant",
            actor_subject: "operator-1",
            trigger: "operator_turn",
          },
          {
            turn_count: 2,
            message_id: "msg-1/auto-evidence-1",
            assistant_message_id: "msg-1/auto-evidence-1/assistant",
            actor_subject: "operator-1",
            trigger: "collected_evidence",
          },
        ],
        follow_up_turns: [
          {
            message_id: "msg-1/auto-evidence-1",
            user_message: "OpenClarion automatic evidence follow-up.",
            assistant_message_id: "msg-1/auto-evidence-1/assistant",
            user_turn_id: 13,
            assistant_turn_id: 14,
            user_sequence: 3,
            assistant_sequence: 4,
            turn_count: 2,
            context_bytes: 512,
            assistant_message: "Collected evidence raises confidence.",
            requires_human_review: false,
            confidence: "high",
            trigger: "collected_evidence",
          },
        ],
      } satisfies DiagnosisTurnResultFrame),
    ).toEqual([
      {
        actor_subject: "openclarion:auto-diagnosis",
        content: "Need current active alerts.",
        id: "msg-1/assistant",
        role: "assistant",
      },
      {
        actor_subject: "operator-1",
        content: "OpenClarion automatic evidence follow-up.",
        id: "msg-1/auto-evidence-1",
        role: "user",
      },
      {
        actor_subject: "openclarion:auto-diagnosis",
        content: "Collected evidence raises confidence.",
        id: "msg-1/auto-evidence-1/assistant",
        role: "assistant",
      },
    ]);
  });

  it("adds collect-evidence follow-up turns when state conversation omits them", () => {
    expect(
      diagnosisStateTranscript({
        ...baseStateFrame(),
        conversation: [
          {
            role: "user",
            actor_subject: "reviewer-1",
            content: "Start diagnosis.",
          },
          {
            role: "assistant",
            actor_subject: "openclarion:auto-diagnosis",
            content: "Need operator-selected evidence.",
          },
        ],
        follow_up_turns: [
          {
            message_id: "collect-1/auto-evidence-1",
            user_message: "OpenClarion automatic evidence follow-up.",
            assistant_message_id: "collect-1/auto-evidence-1/assistant",
            user_turn_id: 13,
            assistant_turn_id: 14,
            user_sequence: 3,
            assistant_sequence: 4,
            turn_count: 2,
            context_bytes: 512,
            assistant_message: "Manual evidence has been reassessed.",
            requires_human_review: true,
            confidence: "medium",
            trigger: "collected_evidence",
          },
        ],
        evidence_timeline: [
          {
            turn_count: 2,
            message_id: "collect-1/auto-evidence-1",
            assistant_message_id: "collect-1/auto-evidence-1/assistant",
            actor_subject: "reviewer-1",
            trigger: "collected_evidence",
          },
        ],
      }),
    ).toMatchObject([
      {
        actor_subject: "reviewer-1",
        content: "Start diagnosis.",
        role: "user",
      },
      {
        actor_subject: "openclarion:auto-diagnosis",
        content: "Need operator-selected evidence.",
        role: "assistant",
      },
      {
        actor_subject: "reviewer-1",
        content: "OpenClarion automatic evidence follow-up.",
        id: "collect-1/auto-evidence-1",
        role: "user",
      },
      {
        actor_subject: "openclarion:auto-diagnosis",
        content: "Manual evidence has been reassessed.",
        id: "collect-1/auto-evidence-1/assistant",
        role: "assistant",
      },
    ]);
  });

  it("does not duplicate follow-up turns already present in state conversation", () => {
    const transcript = diagnosisStateTranscript({
      ...baseStateFrame(),
      conversation: [
        { role: "user", content: "Start diagnosis." },
        { role: "assistant", content: "Need operator-selected evidence." },
        {
          role: "user",
          content: "OpenClarion automatic evidence follow-up.",
        },
        {
          role: "assistant",
          content: "Manual evidence has been reassessed.",
        },
      ],
      follow_up_turns: [
        {
          message_id: "collect-1/auto-evidence-1",
          user_message: "OpenClarion automatic evidence follow-up.",
          assistant_message_id: "collect-1/auto-evidence-1/assistant",
          user_turn_id: 13,
          assistant_turn_id: 14,
          user_sequence: 3,
          assistant_sequence: 4,
          turn_count: 2,
          context_bytes: 512,
          assistant_message: "Manual evidence has been reassessed.",
          requires_human_review: true,
          confidence: "medium",
          trigger: "collected_evidence",
        },
      ],
    });

    expect(transcript).toHaveLength(4);
    expect(transcript.at(-1)).toMatchObject({
      content: "Manual evidence has been reassessed.",
      role: "assistant",
    });
  });
});

function baseStateFrame(): DiagnosisStateFrame {
  return {
    type: "state",
    session_id: "s1",
    chat_session_id: 7,
    diagnosis_task_id: 11,
    owner_subject: "owner-1",
    status: "open",
    turn_count: 2,
    started_at: "2026-06-20T00:00:00Z",
    last_activity_at: "2026-06-20T00:01:00Z",
    approval_mode: "single",
    approval_in_flight: false,
    in_flight: false,
    seen_message_ids: ["msg-1"],
    conversation: [],
  };
}
