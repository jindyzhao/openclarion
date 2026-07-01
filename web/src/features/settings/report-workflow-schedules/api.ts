import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleListResponse,
  ReportWorkflowScheduleWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchReportWorkflowSchedules(
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowScheduleListResponse>> {
  return requestJSON<ReportWorkflowScheduleListResponse>("/api/v1/config/report-workflow-schedules?limit=100", {
    headers: options.headers
  });
}

export async function createReportWorkflowSchedule(
  body: ReportWorkflowScheduleWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowSchedule>> {
  return requestJSON<ReportWorkflowSchedule>("/api/v1/config/report-workflow-schedules", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceReportWorkflowSchedule(
  scheduleID: number,
  body: ReportWorkflowScheduleWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function enableReportWorkflowSchedule(
  scheduleID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}/enable`, {
    method: "POST",
    headers: options.headers
  });
}

export async function disableReportWorkflowSchedule(
  scheduleID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}/disable`, {
    method: "POST",
    headers: options.headers
  });
}

function positiveScheduleID(scheduleID: number): boolean {
  return Number.isSafeInteger(scheduleID) && scheduleID > 0;
}
