import type {
  DiagnosisConversationTurn,
  DiagnosisEvidenceTimelineEntry,
  DiagnosisServerFrame,
  DiagnosisStateFrame,
} from "./types";

type DiagnosisTurnResultFrame = Extract<
  DiagnosisServerFrame,
  { type: "turn_result" }
>;

export type DiagnosisTranscriptTurn = DiagnosisConversationTurn & {
  id: string;
};

const autoDiagnosisActorSubject = "openclarion:auto-diagnosis";

export function diagnosisTurnResultTranscript(
  frame: DiagnosisTurnResultFrame,
): DiagnosisTranscriptTurn[] {
  const actorByMessageID = diagnosisTimelineActorByMessageID(
    frame.evidence_timeline,
  );
  const turns: DiagnosisTranscriptTurn[] = [
    {
      id: frame.assistant_message_id,
      role: "assistant",
      actor_subject: autoDiagnosisActorSubject,
      content: frame.assistant_message,
    },
  ];
  for (const followUp of frame.follow_up_turns ?? []) {
    turns.push({
      id: followUp.message_id,
      role: "user",
      ...optionalActorSubject(actorByMessageID.get(followUp.message_id)),
      content: followUp.user_message,
    });
    turns.push({
      id: followUp.assistant_message_id,
      role: "assistant",
      actor_subject: autoDiagnosisActorSubject,
      content: followUp.assistant_message,
    });
  }
  return turns;
}

export function diagnosisStateTranscript(
  frame: DiagnosisStateFrame,
): DiagnosisTranscriptTurn[] {
  const actorByMessageID = diagnosisTimelineActorByMessageID(
    frame.evidence_timeline,
  );
  const turns: DiagnosisTranscriptTurn[] = frame.conversation.map(
    (turn, index) => ({
      actor_subject: turn.actor_subject,
      id: `state-${index}-${turn.role}`,
      role: turn.role,
      content: turn.content,
    }),
  );
  const assistantContents = new Set(
    frame.conversation
      .filter((turn) => turn.role === "assistant")
      .map((turn) => turn.content),
  );
  for (const followUp of frame.follow_up_turns ?? []) {
    if (assistantContents.has(followUp.assistant_message)) {
      continue;
    }
    turns.push({
      id: followUp.message_id,
      role: "user",
      ...optionalActorSubject(actorByMessageID.get(followUp.message_id)),
      content: followUp.user_message,
    });
    turns.push({
      id: followUp.assistant_message_id,
      role: "assistant",
      actor_subject: autoDiagnosisActorSubject,
      content: followUp.assistant_message,
    });
    assistantContents.add(followUp.assistant_message);
  }
  return turns;
}

function diagnosisTimelineActorByMessageID(
  timeline: DiagnosisEvidenceTimelineEntry[] | undefined,
): Map<string, string> {
  const actorByMessageID = new Map<string, string>();
  for (const entry of timeline ?? []) {
    const messageID = normalizedNonEmpty(entry.message_id);
    const actorSubject = normalizedNonEmpty(entry.actor_subject);
    if (messageID === "" || actorSubject === "") {
      continue;
    }
    actorByMessageID.set(messageID, actorSubject);
  }
  return actorByMessageID;
}

function normalizedNonEmpty(value: string | undefined): string {
  return value?.trim() ?? "";
}

function optionalActorSubject(
  actorSubject: string | undefined,
): Pick<DiagnosisConversationTurn, "actor_subject"> | Record<string, never> {
  const normalized = normalizedNonEmpty(actorSubject);
  return normalized === "" ? {} : { actor_subject: normalized };
}
