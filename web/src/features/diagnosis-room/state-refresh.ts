import type { DiagnosisServerFrame } from "./types";

type DiagnosisTurnResultFrame = Extract<
  DiagnosisServerFrame,
  { type: "turn_result" }
>;

export function latestDiagnosisFollowUpTurn(frame: DiagnosisTurnResultFrame) {
  const followUps = frame.follow_up_turns ?? [];
  return followUps.length > 0 ? followUps.at(-1) : undefined;
}

export function latestDiagnosisTurnCount(
  frame: DiagnosisTurnResultFrame,
): number {
  return latestDiagnosisFollowUpTurn(frame)?.turn_count ?? frame.turn_count;
}

export function latestDiagnosisTurnConclusionStatus(
  frame: DiagnosisTurnResultFrame,
): string | undefined {
  return (
    latestDiagnosisFollowUpTurn(frame)?.consultation_insight
      ?.conclusion_status ?? frame.consultation_insight?.conclusion_status
  );
}

export function shouldQueryDiagnosisStateAfterTurn(
  frame: DiagnosisTurnResultFrame,
): boolean {
  const status = latestDiagnosisTurnConclusionStatus(frame);
  const hasAutoFollowUp = (frame.follow_up_turns ?? []).length > 0;
  return (
    hasAutoFollowUp ||
    status === "final" ||
    status === "ready_for_review" ||
    (status === "needs_evidence" && (frame.follow_up_turns ?? []).length === 0)
  );
}
