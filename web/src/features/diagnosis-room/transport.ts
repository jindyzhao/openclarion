import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

import type { DiagnosisServerFrame } from "./types";

type DiagnosisWSTicketRequest = components["schemas"]["DiagnosisWSTicketRequest"];
type DiagnosisWSTicketResponse = components["schemas"]["DiagnosisWSTicketResponse"];
type DiagnosisRoomCreateRequest = components["schemas"]["DiagnosisRoomCreateRequest"];
type DiagnosisRoomCreateResponse = components["schemas"]["DiagnosisRoomCreateResponse"];

export type DiagnosisWSTicketBundle = DiagnosisWSTicketResponse & {
  websocket_url: string;
};
export type DiagnosisRoomCreateBundle = DiagnosisRoomCreateResponse;

export async function createDiagnosisRoom(
  bearerToken: string,
  evidenceSnapshotID: number
): Promise<ApiResult<DiagnosisRoomCreateResponse>> {
  const body: DiagnosisRoomCreateRequest = { evidence_snapshot_id: evidenceSnapshotID };
  return requestSameOriginJSON<DiagnosisRoomCreateResponse>("/api/diagnosis/rooms", {
    method: "POST",
    headers: {
      authorization: `Bearer ${bearerToken.trim()}`
    },
    body
  });
}

export async function issueDiagnosisWSTicket(
  bearerToken: string,
  sessionID: string
): Promise<ApiResult<DiagnosisWSTicketBundle>> {
  const body: DiagnosisWSTicketRequest = { session_id: sessionID.trim() };
  return requestSameOriginJSON<DiagnosisWSTicketBundle>("/api/diagnosis/ws-ticket", {
    method: "POST",
    headers: {
      authorization: `Bearer ${bearerToken.trim()}`
    },
    body
  });
}

export function parseDiagnosisServerFrame(raw: string): DiagnosisServerFrame {
  const parsed = JSON.parse(raw) as unknown;
  if (!isRecord(parsed) || typeof parsed.type !== "string") {
    throw new Error("Diagnosis frame must contain a string type.");
  }
  switch (parsed.type) {
    case "ready":
    case "turn_result":
    case "state":
    case "error":
      return parsed as DiagnosisServerFrame;
    default:
      throw new Error(`Unsupported diagnosis frame type: ${parsed.type}`);
  }
}

export function nextDiagnosisMessageID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
