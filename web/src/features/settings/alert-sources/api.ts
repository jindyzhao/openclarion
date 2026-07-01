import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  AlertSourceConnectionTestResult,
  AlertSourceProfile,
  AlertSourceProfileListResponse,
  AlertSourceProfileWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchAlertSourceProfiles(
  options: BackendRequestOptions = {}
): Promise<ApiResult<AlertSourceProfileListResponse>> {
  return requestJSON<AlertSourceProfileListResponse>("/api/v1/config/alert-sources?limit=100", {
    headers: options.headers
  });
}

export async function createAlertSourceProfile(
  body: AlertSourceProfileWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<AlertSourceProfile>> {
  return requestJSON<AlertSourceProfile>("/api/v1/config/alert-sources", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceAlertSourceProfile(
  sourceID: number,
  body: AlertSourceProfileWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<AlertSourceProfile>> {
  if (!Number.isSafeInteger(sourceID) || sourceID < 1) {
    return { ok: false, error: { message: "Alert source profile ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<AlertSourceProfile>(`/api/v1/config/alert-sources/${sourceID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function testAlertSourceProfileConnection(
  sourceID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<AlertSourceConnectionTestResult>> {
  if (!Number.isSafeInteger(sourceID) || sourceID < 1) {
    return { ok: false, error: { message: "Alert source profile ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<AlertSourceConnectionTestResult>(`/api/v1/config/alert-sources/${sourceID}/test`, {
    method: "POST",
    headers: options.headers
  });
}
