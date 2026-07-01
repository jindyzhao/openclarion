import type { components } from "@/lib/api/openapi";

export type { ApiResult } from "@/lib/api/client";

export type FinalReportSummary = components["schemas"]["FinalReportSummary"];
export type FinalReportDetail = components["schemas"]["FinalReportDetail"];
export type ReportNotificationDeliveryProof =
  components["schemas"]["ReportNotificationDeliveryProof"];
export type ReportNotificationRetryRequest =
  components["schemas"]["ReportNotificationRetryRequest"];
export type ReportNotificationPurpose =
  components["schemas"]["ReportNotificationPurpose"];
export type ReportNotificationRetryResponse =
  components["schemas"]["ReportNotificationRetryResponse"];
