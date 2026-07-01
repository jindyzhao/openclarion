import type { components } from "@/lib/api/openapi";
import {
  requestJSON,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  ApiResult,
  FinalReportDetail,
  ReportNotificationRetryRequest,
  ReportNotificationRetryResponse,
} from "./types";

type ReportListResponse = components["schemas"]["ReportListResponse"];
type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchReports(
  options: BackendRequestOptions = {},
): Promise<ApiResult<ReportListResponse>> {
  return requestJSON<ReportListResponse>("/api/v1/reports?limit=100", {
    headers: options.headers,
  });
}

export async function fetchReportDetail(
  reportID: string,
  options: BackendRequestOptions = {},
): Promise<ApiResult<FinalReportDetail>> {
  const id = Number.parseInt(reportID, 10);
  if (!Number.isSafeInteger(id) || id < 1) {
    return { ok: false, error: { message: "Report ID must be a positive integer." } };
  }
  return requestJSON<FinalReportDetail>(`/api/v1/reports/${id}`, {
    headers: options.headers,
  });
}

export async function retryReportNotification(
  reportID: number,
  body: ReportNotificationRetryRequest = {},
  options: BackendRequestOptions = {},
): Promise<ApiResult<ReportNotificationRetryResponse>> {
  if (!Number.isSafeInteger(reportID) || reportID < 1) {
    return { ok: false, error: { message: "Report ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportNotificationRetryResponse>(`/api/v1/reports/${reportID}/notification/retry`, {
    method: "POST",
    body,
    headers: options.headers,
  });
}
