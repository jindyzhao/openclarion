"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  DiagnosisHandoffListResponse,
  DiagnosisRoomListResponse,
  DiagnosisRoomSummary,
} from "./api";

export async function refreshDiagnosisRooms(
  limit = 20,
): Promise<ApiResult<DiagnosisRoomListResponse>> {
  return requestSameOriginJSON<DiagnosisRoomListResponse>(
    `/api/diagnosis/rooms?limit=${limit}`,
  );
}

export async function refreshDiagnosisRoom(
  sessionID: string,
): Promise<ApiResult<DiagnosisRoomSummary>> {
  return requestSameOriginJSON<DiagnosisRoomSummary>(
    `/api/diagnosis/rooms/${encodeURIComponent(sessionID)}`,
  );
}

export async function refreshDiagnosisHandoffs(
  limit = 20,
): Promise<ApiResult<DiagnosisHandoffListResponse>> {
  return requestSameOriginJSON<DiagnosisHandoffListResponse>(
    `/api/diagnosis/handoffs?limit=${limit}`,
  );
}
