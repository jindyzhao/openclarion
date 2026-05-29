import type { components } from "@/lib/api/openapi";
import { requestJSON } from "@/lib/api/client";

import type { ApiResult, FinalReportDetail } from "./types";

type ReportListResponse = components["schemas"]["ReportListResponse"];

export async function fetchReports(): Promise<ApiResult<ReportListResponse>> {
  return requestJSON<ReportListResponse>("/api/v1/reports?limit=100");
}

export async function fetchReportDetail(reportID: string): Promise<ApiResult<FinalReportDetail>> {
  const id = Number.parseInt(reportID, 10);
  if (!Number.isSafeInteger(id) || id < 1) {
    return { ok: false, error: { message: "Report ID must be a positive integer." } };
  }
  return requestJSON<FinalReportDetail>(`/api/v1/reports/${id}`);
}
