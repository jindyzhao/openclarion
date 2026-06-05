"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyListResponse,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest
} from "./types";

export async function refreshReportWorkflowPolicies(): Promise<ApiResult<ReportWorkflowPolicyListResponse>> {
  return requestSameOriginJSON<ReportWorkflowPolicyListResponse>("/api/config/report-workflow-policies");
}

export async function submitReportWorkflowPolicy(
  policyID: number | null,
  body: ReportWorkflowPolicyWriteRequest
): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (policyID === null) {
    return requestSameOriginJSON<ReportWorkflowPolicy>("/api/config/report-workflow-policies", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<ReportWorkflowPolicy>(`/api/config/report-workflow-policies/${policyID}`, {
    method: "PUT",
    body
  });
}

export async function enableReportWorkflowPolicyAction(policyID: number): Promise<ApiResult<ReportWorkflowPolicy>> {
  return requestSameOriginJSON<ReportWorkflowPolicy>(`/api/config/report-workflow-policies/${policyID}/enable`, {
    method: "POST"
  });
}

export async function disableReportWorkflowPolicyAction(policyID: number): Promise<ApiResult<ReportWorkflowPolicy>> {
  return requestSameOriginJSON<ReportWorkflowPolicy>(`/api/config/report-workflow-policies/${policyID}/disable`, {
    method: "POST"
  });
}

export async function previewReportWorkflowPolicyImpactAction(
  policyID: number
): Promise<ApiResult<ReportWorkflowPolicyImpactPreviewResult>> {
  return requestSameOriginJSON<ReportWorkflowPolicyImpactPreviewResult>(
    `/api/config/report-workflow-policies/${policyID}/impact-preview`,
    {
      method: "POST"
    }
  );
}

export async function triggerReportWorkflowPolicyReplayAction(
  policyID: number,
  body: ReportWorkflowPolicyReplayRequest
): Promise<ApiResult<ReportReplayTriggerResponse>> {
  return requestSameOriginJSON<ReportReplayTriggerResponse>(
    `/api/config/report-workflow-policies/${policyID}/replay-window`,
    {
      method: "POST",
      body
    }
  );
}
