import {
  normalizedDiagnosisAuthorization,
  type DiagnosisAuthorization,
} from "./authorization";

type DiagnosisReconnectConnection = {
  authorization: DiagnosisAuthorization;
  sessionID: string;
};

export type DiagnosisConnectionTargetInput = {
  formSessionID?: string;
  selectedRoomSessionID?: string;
  selectedSessionID?: string;
};

export type DiagnosisReconnectDecisionInput = {
  attempts: number;
  clientReady: boolean;
  connection: DiagnosisReconnectConnection | null;
  maxAttempts: number;
  manualDisconnect: boolean;
  timerActive: boolean;
};

export type DiagnosisReconnectDecision =
  | {
      kind: "skip";
      reason:
        | "client_not_ready"
        | "manual_disconnect"
        | "missing_connection";
    }
  | { kind: "already_scheduled" }
  | { kind: "exhausted" }
  | { kind: "schedule"; attempt: number; connection: DiagnosisReconnectConnection };

export function diagnosisReconnectDecision(
  input: DiagnosisReconnectDecisionInput,
): DiagnosisReconnectDecision {
  if (input.manualDisconnect) {
    return { kind: "skip", reason: "manual_disconnect" };
  }
  if (!input.clientReady) {
    return { kind: "skip", reason: "client_not_ready" };
  }
  const connection = normalizedDiagnosisReconnectConnection(input.connection);
  if (connection === null) {
    return { kind: "skip", reason: "missing_connection" };
  }
  if (input.timerActive) {
    return { kind: "already_scheduled" };
  }
  if (input.attempts >= input.maxAttempts) {
    return { kind: "exhausted" };
  }
  return { kind: "schedule", attempt: input.attempts + 1, connection };
}

export function diagnosisConnectionTargetSessionID(
  input: DiagnosisConnectionTargetInput,
): string {
  for (const candidate of [
    input.selectedRoomSessionID,
    input.selectedSessionID,
    input.formSessionID,
  ]) {
    const sessionID = candidate?.trim() ?? "";
    if (sessionID !== "") {
      return sessionID;
    }
  }
  return "";
}

function normalizedDiagnosisReconnectConnection(
  connection: DiagnosisReconnectConnection | null,
): DiagnosisReconnectConnection | null {
  const authorization =
    connection === null
      ? null
      : normalizedDiagnosisAuthorization(connection.authorization);
  const sessionID = connection?.sessionID.trim() ?? "";
  if (authorization === null || sessionID === "") {
    return null;
  }
  return { authorization, sessionID };
}
