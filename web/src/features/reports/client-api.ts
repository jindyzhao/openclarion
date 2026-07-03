"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";

import type {
  ApiResult,
  ReportNotificationPurpose,
  ReportNotificationRetryResponse,
} from "./types";

export async function retryReportNotificationAction(
  reportID: number,
  reportNotificationChannelProfileID: number | null,
  notificationPurpose: ReportNotificationPurpose = "handoff",
): Promise<ApiResult<ReportNotificationRetryResponse>> {
  if (!Number.isSafeInteger(reportID) || reportID < 1) {
    return { ok: false, error: { message: "Report ID must be a positive integer.", status: 400 } };
  }
  if (
    reportNotificationChannelProfileID !== null &&
    (!Number.isSafeInteger(reportNotificationChannelProfileID) ||
      reportNotificationChannelProfileID < 1)
  ) {
    return {
      ok: false,
      error: {
        message: "Report notification channel profile ID must be a positive integer.",
        status: 400,
      },
    };
  }
  if (notificationPurpose !== "handoff" && notificationPurpose !== "final") {
    return {
      ok: false,
      error: {
        message: "Report notification purpose must be handoff or final.",
        status: 400,
      },
    };
  }
  return requestSameOriginJSON<ReportNotificationRetryResponse>(
    `/api/reports/${reportID}/notification/retry`,
    {
      method: "POST",
      body: {
        notification_purpose: notificationPurpose,
        report_notification_channel_profile_id: reportNotificationChannelProfileID,
      },
    },
  );
}
