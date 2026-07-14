import type { DiagnosisTurnStreamFrame } from "./types";

export type DiagnosisTurnPreview = DiagnosisTurnStreamFrame;

// Ignore stale snapshots and replace the draft when an Activity or model
// generation retries.
export function nextDiagnosisTurnPreview(
  current: DiagnosisTurnPreview | null,
  incoming: DiagnosisTurnStreamFrame,
): DiagnosisTurnPreview {
  if (
    current === null ||
    current.session_id !== incoming.session_id ||
    current.message_id !== incoming.message_id
  ) {
    return incoming;
  }

  if (incoming.activity_attempt !== current.activity_attempt) {
    return incoming.activity_attempt > current.activity_attempt
      ? incoming
      : current;
  }
  if (incoming.generation_attempt !== current.generation_attempt) {
    return incoming.generation_attempt > current.generation_attempt
      ? incoming
      : current;
  }
  if (incoming.sequence !== current.sequence) {
    return incoming.sequence > current.sequence ? incoming : current;
  }
  return current;
}
