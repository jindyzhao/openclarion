import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  AlertSourceProfile,
  AlertSourceProfileListResponse,
  AlertSourceProfileWriteRequest
} from "./types";

export async function fetchAlertSourceProfiles(): Promise<ApiResult<AlertSourceProfileListResponse>> {
  return requestJSON<AlertSourceProfileListResponse>("/api/v1/config/alert-sources?limit=100");
}

export async function createAlertSourceProfile(
  body: AlertSourceProfileWriteRequest
): Promise<ApiResult<AlertSourceProfile>> {
  return requestJSON<AlertSourceProfile>("/api/v1/config/alert-sources", {
    method: "POST",
    body
  });
}

export async function replaceAlertSourceProfile(
  sourceID: number,
  body: AlertSourceProfileWriteRequest
): Promise<ApiResult<AlertSourceProfile>> {
  if (!Number.isSafeInteger(sourceID) || sourceID < 1) {
    return { ok: false, error: { message: "Alert source profile ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<AlertSourceProfile>(`/api/v1/config/alert-sources/${sourceID}`, {
    method: "PUT",
    body
  });
}
