import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyListResponse,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest
} from "./types";

export async function fetchReportWorkflowPolicies(): Promise<ApiResult<ReportWorkflowPolicyListResponse>> {
  return requestJSON<ReportWorkflowPolicyListResponse>("/api/v1/config/report-workflow-policies?limit=100");
}

export async function createReportWorkflowPolicy(
  body: ReportWorkflowPolicyWriteRequest
): Promise<ApiResult<ReportWorkflowPolicy>> {
  return requestJSON<ReportWorkflowPolicy>("/api/v1/config/report-workflow-policies", {
    method: "POST",
    body
  });
}

export async function replaceReportWorkflowPolicy(
  policyID: number,
  body: ReportWorkflowPolicyWriteRequest
): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}`, {
    method: "PUT",
    body
  });
}

export async function enableReportWorkflowPolicy(policyID: number): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}/enable`, {
    method: "POST"
  });
}

export async function disableReportWorkflowPolicy(policyID: number): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}/disable`, {
    method: "POST"
  });
}

export async function previewReportWorkflowPolicyImpact(
  policyID: number
): Promise<ApiResult<ReportWorkflowPolicyImpactPreviewResult>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicyImpactPreviewResult>(
    `/api/v1/config/report-workflow-policies/${policyID}/impact-preview?limit=100`,
    {
      method: "POST"
    }
  );
}

export async function triggerReportWorkflowPolicyReplay(
  policyID: number,
  body: ReportWorkflowPolicyReplayRequest
): Promise<ApiResult<ReportReplayTriggerResponse>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportReplayTriggerResponse>(`/api/v1/config/report-workflow-policies/${policyID}/replay-window`, {
    method: "POST",
    body
  });
}

function positivePolicyID(policyID: number): boolean {
  return Number.isSafeInteger(policyID) && policyID > 0;
}
