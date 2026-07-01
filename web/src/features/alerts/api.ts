import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

export type AlertListResponse = components["schemas"]["AlertListResponse"];
export type AlertEventSummary = components["schemas"]["AlertEventSummary"];
export type AlertEvidenceSnapshotLink = components["schemas"]["AlertEvidenceSnapshotLink"];
export type ReportReplayTriggerRequest = components["schemas"]["ReportReplayTriggerRequest"];
export type ReportReplayTriggerResponse = components["schemas"]["ReportReplayTriggerResponse"];
type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchAlerts(
  limit = 100,
  options: BackendRequestOptions = {},
): Promise<ApiResult<AlertListResponse>> {
  return requestJSON<AlertListResponse>(`/api/v1/alerts?limit=${limit}`, {
    headers: options.headers,
  });
}

export async function triggerReportReplay(
  body: ReportReplayTriggerRequest,
  options: BackendRequestOptions = {},
): Promise<ApiResult<ReportReplayTriggerResponse>> {
  return requestJSON<ReportReplayTriggerResponse>("/api/v1/report-triggers/replay-window", {
    method: "POST",
    headers: options.headers,
    body
  });
}
