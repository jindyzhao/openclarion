import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleListResponse,
  ReportWorkflowScheduleWriteRequest
} from "./types";

export async function fetchReportWorkflowSchedules(): Promise<ApiResult<ReportWorkflowScheduleListResponse>> {
  return requestJSON<ReportWorkflowScheduleListResponse>("/api/v1/config/report-workflow-schedules?limit=100");
}

export async function createReportWorkflowSchedule(
  body: ReportWorkflowScheduleWriteRequest
): Promise<ApiResult<ReportWorkflowSchedule>> {
  return requestJSON<ReportWorkflowSchedule>("/api/v1/config/report-workflow-schedules", {
    method: "POST",
    body
  });
}

export async function replaceReportWorkflowSchedule(
  scheduleID: number,
  body: ReportWorkflowScheduleWriteRequest
): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}`, {
    method: "PUT",
    body
  });
}

export async function enableReportWorkflowSchedule(scheduleID: number): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}/enable`, {
    method: "POST"
  });
}

export async function disableReportWorkflowSchedule(scheduleID: number): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (!positiveScheduleID(scheduleID)) {
    return { ok: false, error: { message: "Report workflow schedule ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowSchedule>(`/api/v1/config/report-workflow-schedules/${scheduleID}/disable`, {
    method: "POST"
  });
}

function positiveScheduleID(scheduleID: number): boolean {
  return Number.isSafeInteger(scheduleID) && scheduleID > 0;
}
