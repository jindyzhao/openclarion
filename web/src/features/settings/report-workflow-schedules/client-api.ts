"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  ReportWorkflowSchedule,
  ReportWorkflowScheduleListResponse,
  ReportWorkflowScheduleWriteRequest
} from "./types";

export async function refreshReportWorkflowSchedules(): Promise<ApiResult<ReportWorkflowScheduleListResponse>> {
  return requestSameOriginJSON<ReportWorkflowScheduleListResponse>("/api/config/report-workflow-schedules");
}

export async function submitReportWorkflowSchedule(
  scheduleID: number | null,
  body: ReportWorkflowScheduleWriteRequest
): Promise<ApiResult<ReportWorkflowSchedule>> {
  if (scheduleID === null) {
    return requestSameOriginJSON<ReportWorkflowSchedule>("/api/config/report-workflow-schedules", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<ReportWorkflowSchedule>(`/api/config/report-workflow-schedules/${scheduleID}`, {
    method: "PUT",
    body
  });
}

export async function enableReportWorkflowScheduleAction(scheduleID: number): Promise<ApiResult<ReportWorkflowSchedule>> {
  return requestSameOriginJSON<ReportWorkflowSchedule>(`/api/config/report-workflow-schedules/${scheduleID}/enable`, {
    method: "POST"
  });
}

export async function disableReportWorkflowScheduleAction(scheduleID: number): Promise<ApiResult<ReportWorkflowSchedule>> {
  return requestSameOriginJSON<ReportWorkflowSchedule>(`/api/config/report-workflow-schedules/${scheduleID}/disable`, {
    method: "POST"
  });
}
