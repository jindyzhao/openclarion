import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";
import type { DiagnosisAuthStatusResponse } from "@/lib/api/diagnosis-auth-response";
import type { components } from "@/lib/api/openapi";

import {
  diagnosisAuthorizationHeaders,
  type DiagnosisAuthorization,
} from "./authorization";
import type { DiagnosisServerFrame } from "./types";

type DiagnosisWSTicketRequest =
  components["schemas"]["DiagnosisWSTicketRequest"];
type DiagnosisWSTicketResponse =
  components["schemas"]["DiagnosisWSTicketResponse"];
type DiagnosisAuthCheckResponse =
  components["schemas"]["DiagnosisAuthCheckResponse"];
type DiagnosisRoomCreateRequest =
  components["schemas"]["DiagnosisRoomCreateRequest"];
type DiagnosisRoomCreateResponse =
  components["schemas"]["DiagnosisRoomCreateResponse"];
type DiagnosisRoomCloseUnavailableRequest =
  components["schemas"]["DiagnosisRoomCloseUnavailableRequest"];
type DiagnosisRoomSummary = components["schemas"]["DiagnosisRoomSummary"];
type DiagnosisNotificationRetryRequest =
  components["schemas"]["DiagnosisNotificationRetryRequest"];
type DiagnosisNotificationRetryResponse =
  components["schemas"]["DiagnosisNotificationRetryResponse"];

export type DiagnosisWSTicketBundle = DiagnosisWSTicketResponse & {
  websocket_url: string;
};
export type DiagnosisAuthCheck = DiagnosisAuthCheckResponse;
export type DiagnosisAuthStatus = DiagnosisAuthStatusResponse;
export type DiagnosisBrowserSessionStatus =
  | {
      authenticated: true;
      checked_at: string;
      mode: DiagnosisAuthCheckResponse["mode"];
      role_authorized: boolean;
      roles: string[];
      subject: string;
    }
  | {
      authenticated: false;
    };
export type DiagnosisRoomCreateBundle = DiagnosisRoomCreateResponse;
export type DiagnosisNotificationRetryBundle =
  DiagnosisNotificationRetryResponse;
export type DiagnosisNotificationRetryEventKind =
  DiagnosisNotificationRetryRequest["event_kind"];

export async function checkDiagnosisAuthorization(
  authorization: DiagnosisAuthorization,
): Promise<ApiResult<DiagnosisAuthCheckResponse>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  return requestSameOriginJSON<DiagnosisAuthCheckResponse>(
    "/api/diagnosis/auth/check",
    {
      method: "POST",
      headers,
    },
  );
}

export async function fetchDiagnosisAuthStatus(): Promise<
  ApiResult<DiagnosisAuthStatusResponse>
> {
  return requestSameOriginJSON<DiagnosisAuthStatusResponse>(
    "/api/diagnosis/auth/status",
  );
}

export async function fetchDiagnosisBrowserSession(): Promise<
  ApiResult<DiagnosisBrowserSessionStatus>
> {
  return requestSameOriginJSON<DiagnosisBrowserSessionStatus>(
    "/api/diagnosis/auth/session",
  );
}

export async function createDiagnosisBrowserSession(
  authorization: DiagnosisAuthorization,
): Promise<ApiResult<DiagnosisBrowserSessionStatus>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null || Object.keys(headers).length === 0) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  return requestSameOriginJSON<DiagnosisBrowserSessionStatus>(
    "/api/diagnosis/auth/session",
    {
      method: "POST",
      headers,
    },
  );
}

export async function clearDiagnosisBrowserSession(): Promise<
  ApiResult<void>
> {
  return requestSameOriginJSON<void>("/api/diagnosis/auth/session", {
    method: "DELETE",
  });
}

export async function createDiagnosisRoom(
  authorization: DiagnosisAuthorization,
  evidenceSnapshotID: number,
  closeNotificationChannelProfileID?: number,
): Promise<ApiResult<DiagnosisRoomCreateResponse>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  const body: DiagnosisRoomCreateRequest = {
    evidence_snapshot_id: evidenceSnapshotID,
  };
  if (closeNotificationChannelProfileID !== undefined) {
    body.close_notification_channel_profile_id =
      closeNotificationChannelProfileID;
  }
  return requestSameOriginJSON<DiagnosisRoomCreateResponse>(
    "/api/diagnosis/rooms",
    {
      method: "POST",
      headers,
      body,
    },
  );
}

export async function closeUnavailableDiagnosisRoom(
  authorization: DiagnosisAuthorization,
  sessionID: string,
  reason = "workflow_unavailable",
): Promise<ApiResult<DiagnosisRoomSummary>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  const body: DiagnosisRoomCloseUnavailableRequest = { reason };
  return requestSameOriginJSON<DiagnosisRoomSummary>(
    `/api/diagnosis/rooms/${encodeURIComponent(sessionID.trim())}/close-unavailable`,
    {
      method: "POST",
      headers,
      body,
    },
  );
}

export async function retryDiagnosisRoomNotification(
  authorization: DiagnosisAuthorization,
  sessionID: string,
  eventKind: DiagnosisNotificationRetryEventKind,
): Promise<ApiResult<DiagnosisNotificationRetryResponse>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  const body: DiagnosisNotificationRetryRequest = { event_kind: eventKind };
  return requestSameOriginJSON<DiagnosisNotificationRetryResponse>(
    `/api/diagnosis/rooms/${encodeURIComponent(sessionID.trim())}/notifications/retry`,
    {
      method: "POST",
      headers,
      body,
    },
  );
}

export function isDiagnosisNotificationRetryEventKind(
  eventKind: string,
): eventKind is DiagnosisNotificationRetryEventKind {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
    case "diagnosis_room.final_ready_notification_sent":
    case "diagnosis_room.close_notification_sent":
      return true;
    default:
      return false;
  }
}

export async function issueDiagnosisWSTicket(
  authorization: DiagnosisAuthorization,
  sessionID: string,
): Promise<ApiResult<DiagnosisWSTicketBundle>> {
  const headers = diagnosisAuthorizationHeaders(authorization);
  if (headers === null) {
    return {
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    };
  }
  const body: DiagnosisWSTicketRequest = { session_id: sessionID.trim() };
  return requestSameOriginJSON<DiagnosisWSTicketBundle>(
    "/api/diagnosis/ws-ticket",
    {
      method: "POST",
      headers,
      body,
    },
  );
}

export function parseDiagnosisServerFrame(raw: string): DiagnosisServerFrame {
  const parsed = JSON.parse(raw) as unknown;
  if (!isRecord(parsed) || typeof parsed.type !== "string") {
    throw new Error("Diagnosis frame must contain a string type.");
  }
  switch (parsed.type) {
    case "ready":
      if (!isDiagnosisReadyFrame(parsed)) {
        throw new Error("Invalid ready diagnosis frame.");
      }
      return parsed;
    case "turn_stream":
      if (!isDiagnosisTurnStreamFrame(parsed)) {
        throw new Error("Invalid turn_stream diagnosis frame.");
      }
      return parsed;
    case "turn_result":
      if (!isDiagnosisTurnResultFrame(parsed)) {
        throw new Error("Invalid turn_result diagnosis frame.");
      }
      return parsed;
    case "state":
      if (!isDiagnosisStateFrame(parsed)) {
        throw new Error("Invalid state diagnosis frame.");
      }
      return parsed;
    case "error":
      if (!isDiagnosisErrorFrame(parsed)) {
        throw new Error("Invalid error diagnosis frame.");
      }
      return parsed;
    default:
      throw new Error(`Unsupported diagnosis frame type: ${parsed.type}`);
  }
}

export function nextDiagnosisMessageID(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isDiagnosisReadyFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "ready" }> {
  return isString(value.session_id) && isString(value.subject);
}

const maxDiagnosisTurnPreviewBytes = 128 * 1024;

function isDiagnosisTurnStreamFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "turn_stream" }> {
  if (
    !isString(value.session_id) ||
    value.session_id.length === 0 ||
    !isString(value.message_id) ||
    value.message_id.length === 0 ||
    !isString(value.assistant_message_id) ||
    value.assistant_message_id.length === 0 ||
    !isPositiveInteger(value.activity_attempt) ||
    !isNonNegativeInteger(value.generation_attempt) ||
    !isNonNegativeInteger(value.sequence) ||
    !isString(value.assistant_message) ||
    new TextEncoder().encode(value.assistant_message).length >
      maxDiagnosisTurnPreviewBytes
  ) {
    return false;
  }
  if (value.phase === "started") {
    return (
      value.generation_attempt === 0 &&
      value.sequence === 0 &&
      value.assistant_message === ""
    );
  }
  if (value.phase === "reset") {
    return (
      value.generation_attempt > 0 &&
      value.sequence === 0 &&
      value.assistant_message === ""
    );
  }
  return (
    value.phase === "delta" &&
    value.generation_attempt > 0 &&
    value.sequence > 0 &&
    value.assistant_message.length > 0
  );
}

function isDiagnosisTurnResultFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "turn_result" }> {
  return (
    isString(value.session_id) &&
    isFiniteNumber(value.chat_session_id) &&
    isString(value.message_id) &&
    isString(value.assistant_message_id) &&
    isFiniteNumber(value.user_turn_id) &&
    isFiniteNumber(value.assistant_turn_id) &&
    isFiniteNumber(value.user_sequence) &&
    isFiniteNumber(value.assistant_sequence) &&
    isFiniteNumber(value.turn_count) &&
    isFiniteNumber(value.context_bytes) &&
    isString(value.status) &&
    isString(value.assistant_message) &&
    typeof value.requires_human_review === "boolean" &&
    isString(value.confidence)
  );
}

function isDiagnosisStateFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "state" }> {
  return (
    isString(value.session_id) &&
    isFiniteNumber(value.chat_session_id) &&
    isFiniteNumber(value.diagnosis_task_id) &&
    isString(value.owner_subject) &&
    isString(value.status) &&
    isFiniteNumber(value.turn_count) &&
    isString(value.started_at) &&
    isString(value.last_activity_at) &&
    typeof value.in_flight === "boolean" &&
    Array.isArray(value.seen_message_ids) &&
    Array.isArray(value.conversation)
  );
}

function isDiagnosisErrorFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "error" }> {
  return isString(value.code) && isString(value.message);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isFiniteNumber(value: unknown): value is number {
	return typeof value === "number" && Number.isFinite(value);
}

function isPositiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
