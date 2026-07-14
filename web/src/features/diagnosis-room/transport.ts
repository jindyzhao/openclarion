import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";
import type { DiagnosisAuthStatusResponse } from "@/lib/api/diagnosis-auth-response";
import type { components } from "@/lib/api/openapi";

import {
  diagnosisAuthorizationHeaders,
  type DiagnosisAuthorization,
} from "./authorization";
import type {
  DiagnosisApprovalAuthority,
  DiagnosisApprovalMode,
  DiagnosisConclusionApproval,
  DiagnosisServerFrame,
} from "./types";

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
  approvalMode: DiagnosisApprovalMode = "single",
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
    approval_mode: approvalMode,
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
    isDiagnosisContextBytes(value.context_bytes) &&
    isString(value.status) &&
    isString(value.assistant_message) &&
    typeof value.requires_human_review === "boolean" &&
    isString(value.confidence) &&
    isOptionalDiagnosisRetrievalRefs(value.retrieval_refs) &&
    isOptionalDiagnosisRetrievalEntries(value.follow_up_turns) &&
    isOptionalDiagnosisRetrievalEntries(value.confidence_timeline)
  );
}

function isDiagnosisStateFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "state" }> {
  const conclusionDigest = value.conclusion_digest;
  if (
    !(
      isString(value.session_id) &&
      isFiniteNumber(value.chat_session_id) &&
      isFiniteNumber(value.diagnosis_task_id) &&
      isString(value.owner_subject) &&
      isString(value.status) &&
      isFiniteNumber(value.turn_count) &&
      isString(value.started_at) &&
      isString(value.last_activity_at) &&
      isDiagnosisApprovalMode(value.approval_mode) &&
      (conclusionDigest === undefined || isConclusionDigest(conclusionDigest)) &&
      typeof value.approval_in_flight === "boolean" &&
      typeof value.in_flight === "boolean" &&
      Array.isArray(value.seen_message_ids) &&
      Array.isArray(value.conversation) &&
      isOptionalDiagnosisRetrievalEntries(value.follow_up_turns) &&
      isOptionalDiagnosisRetrievalEntries(value.confidence_timeline)
    )
  ) {
    return false;
  }
  return isConsistentDiagnosisApprovalState({
    approvals: value.approvals,
    conclusionDigest,
    mode: value.approval_mode,
    ownerSubject: value.owner_subject,
    pendingAuthorities: value.pending_approval_authorities,
  });
}

function isConsistentDiagnosisApprovalState({
  approvals,
  conclusionDigest,
  mode,
  ownerSubject,
  pendingAuthorities,
}: {
  approvals: unknown;
  conclusionDigest: string | undefined;
  mode: DiagnosisApprovalMode;
  ownerSubject: string;
  pendingAuthorities: unknown;
}): boolean {
  const approvalRows = approvals === undefined ? [] : approvals;
  const pending = pendingAuthorities === undefined ? [] : pendingAuthorities;
  if (
    !Array.isArray(approvalRows) ||
    !approvalRows.every(isDiagnosisConclusionApproval) ||
    !Array.isArray(pending) ||
    !pending.every(isDiagnosisApprovalAuthority) ||
    new Set(pending).size !== pending.length
  ) {
    return false;
  }
  if (conclusionDigest === undefined) {
    return approvalRows.length === 0 && pending.length === 0;
  }
  if (
    approvalRows.length > (mode === "single" ? 1 : 2) ||
    approvalRows.some(
      (approval) => approval.conclusion_digest !== conclusionDigest,
    ) ||
    new Set(approvalRows.map((approval) => approval.actor_subject)).size !==
      approvalRows.length ||
    new Set(approvalRows.map((approval) => approval.authority)).size !==
      approvalRows.length ||
    approvalRows.some(
      (approval) =>
        (approval.authority === "owner") !==
        (approval.actor_subject === ownerSubject),
    )
  ) {
    return false;
  }
  const approved = new Set(approvalRows.map((approval) => approval.authority));
  const expectedPending: DiagnosisApprovalAuthority[] =
    mode === "single"
      ? approvalRows.length === 0
        ? ["owner"]
        : []
      : (["owner", "leader"] as const).filter(
          (authority) => !approved.has(authority),
        );
  return (
    pending.length === expectedPending.length &&
    expectedPending.every((authority) => pending.includes(authority))
  );
}

function isDiagnosisApprovalMode(value: unknown): value is DiagnosisApprovalMode {
  return value === "single" || value === "owner_and_leader";
}

function isDiagnosisApprovalAuthority(
  value: unknown,
): value is DiagnosisApprovalAuthority {
  return value === "owner" || value === "leader";
}

function isDiagnosisConclusionApproval(
  value: unknown,
): value is DiagnosisConclusionApproval {
  return (
    isRecord(value) &&
    isPositiveInteger(value.id) &&
    isConclusionDigest(value.conclusion_digest) &&
    isString(value.actor_subject) &&
    value.actor_subject.length > 0 &&
    isDiagnosisApprovalAuthority(value.authority) &&
    isString(value.reason) &&
    value.reason.length > 0 &&
    isString(value.approved_at) &&
    value.approved_at.length > 0
  );
}

function isConclusionDigest(value: unknown): value is string {
  return isString(value) && /^[0-9a-f]{64}$/.test(value);
}

function isDiagnosisErrorFrame(
  value: Record<string, unknown>,
): value is Extract<DiagnosisServerFrame, { type: "error" }> {
  return isString(value.code) && isString(value.message);
}

const maxDiagnosisContextBytes = 2 * 1024 * 1024;
const maxDiagnosisRetrievalRefs = 10;

function isOptionalDiagnosisRetrievalEntries(value: unknown): boolean {
  return (
    value === undefined ||
    (Array.isArray(value) &&
      value.every(
        (item) =>
          isRecord(item) &&
          (item.context_bytes === undefined ||
            isDiagnosisContextBytes(item.context_bytes)) &&
          isOptionalDiagnosisRetrievalRefs(item.retrieval_refs),
      ))
  );
}

function isOptionalDiagnosisRetrievalRefs(value: unknown): boolean {
  if (value === undefined) {
    return true;
  }
  return (
    Array.isArray(value) &&
    value.length <= maxDiagnosisRetrievalRefs &&
    value.every(isDiagnosisRetrievalRef) &&
    new Set(value).size === value.length
  );
}

function isDiagnosisContextBytes(value: unknown): value is number {
  return isPositiveInteger(value) && value <= maxDiagnosisContextBytes;
}

function isDiagnosisRetrievalRef(value: unknown): value is string {
  return (
    isString(value) && /^(?:sub_report|final_report):[1-9][0-9]*$/.test(value)
  );
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
