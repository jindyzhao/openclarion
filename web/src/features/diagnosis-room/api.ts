import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

export type DiagnosisRoomListResponse =
  components["schemas"]["DiagnosisRoomListResponse"];
export type DiagnosisRoomSummary =
  components["schemas"]["DiagnosisRoomSummary"];
export type DiagnosisHandoffListResponse =
  components["schemas"]["DiagnosisHandoffListResponse"];
export type DiagnosisRoomNotificationTimelineEntry =
  components["schemas"]["DiagnosisRoomNotificationTimelineEntry"];
export type DiagnosisRoomAuditTimelineEntry =
  components["schemas"]["DiagnosisRoomAuditTimelineEntry"];
export type DiagnosisNotificationRetryRequest =
  components["schemas"]["DiagnosisNotificationRetryRequest"];
export type DiagnosisNotificationRetryResponse =
  components["schemas"]["DiagnosisNotificationRetryResponse"];
type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchDiagnosisRoom(
  sessionID: string,
  options: BackendRequestOptions = {},
): Promise<ApiResult<DiagnosisRoomSummary>> {
  return requestJSON<DiagnosisRoomSummary>(
    `/api/v1/diagnosis/rooms/${encodeURIComponent(sessionID)}`,
    { headers: options.headers },
  );
}

export async function fetchDiagnosisRooms(
  limit = 20,
  options: BackendRequestOptions = {},
): Promise<ApiResult<DiagnosisRoomListResponse>> {
  return requestJSON<DiagnosisRoomListResponse>(
    `/api/v1/diagnosis/rooms?limit=${limit}`,
    { headers: options.headers },
  );
}

export async function fetchDiagnosisHandoffs(
  limit = 20,
  options: BackendRequestOptions = {},
): Promise<ApiResult<DiagnosisHandoffListResponse>> {
  return requestJSON<DiagnosisHandoffListResponse>(
    `/api/v1/diagnosis/handoffs?limit=${limit}`,
    { headers: options.headers },
  );
}

export async function retryDiagnosisRoomNotification(
  sessionID: string,
  body: DiagnosisNotificationRetryRequest,
  options: BackendRequestOptions = {},
): Promise<ApiResult<DiagnosisNotificationRetryResponse>> {
  return requestJSON<DiagnosisNotificationRetryResponse>(
    `/api/v1/diagnosis/rooms/${encodeURIComponent(sessionID)}/notifications/retry`,
    {
      method: "POST",
      headers: options.headers,
      body,
    },
  );
}
