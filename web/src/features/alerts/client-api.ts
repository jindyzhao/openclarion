"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type { ReportReplayTriggerRequest, ReportReplayTriggerResponse } from "./api";

export async function triggerReportReplayAction(
  body: ReportReplayTriggerRequest
): Promise<ApiResult<ReportReplayTriggerResponse>> {
  return requestSameOriginJSON<ReportReplayTriggerResponse>("/api/report-triggers/replay-window", {
    method: "POST",
    body
  });
}
