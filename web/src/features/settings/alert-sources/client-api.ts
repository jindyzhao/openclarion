"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  AlertSourceProfile,
  AlertSourceProfileListResponse,
  AlertSourceProfileWriteRequest
} from "./types";

export async function refreshAlertSourceProfiles(): Promise<ApiResult<AlertSourceProfileListResponse>> {
  return requestSameOriginJSON<AlertSourceProfileListResponse>("/api/config/alert-sources");
}

export async function submitAlertSourceProfile(
  sourceID: number | null,
  body: AlertSourceProfileWriteRequest
): Promise<ApiResult<AlertSourceProfile>> {
  if (sourceID === null) {
    return requestSameOriginJSON<AlertSourceProfile>("/api/config/alert-sources", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<AlertSourceProfile>(`/api/config/alert-sources/${sourceID}`, {
    method: "PUT",
    body
  });
}
