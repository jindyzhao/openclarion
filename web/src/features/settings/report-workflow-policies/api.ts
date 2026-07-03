import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyListResponse,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchReportWorkflowPolicies(
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicyListResponse>> {
  return requestJSON<ReportWorkflowPolicyListResponse>("/api/v1/config/report-workflow-policies?limit=100", {
    headers: options.headers
  });
}

export async function createReportWorkflowPolicy(
  body: ReportWorkflowPolicyWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicy>> {
  return requestJSON<ReportWorkflowPolicy>("/api/v1/config/report-workflow-policies", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceReportWorkflowPolicy(
  policyID: number,
  body: ReportWorkflowPolicyWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function enableReportWorkflowPolicy(
  policyID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}/enable`, {
    method: "POST",
    headers: options.headers
  });
}

export async function disableReportWorkflowPolicy(
  policyID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicy>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicy>(`/api/v1/config/report-workflow-policies/${policyID}/disable`, {
    method: "POST",
    headers: options.headers
  });
}

export async function previewReportWorkflowPolicyImpact(
  policyID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicyImpactPreviewResult>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportWorkflowPolicyImpactPreviewResult>(
    `/api/v1/config/report-workflow-policies/${policyID}/impact-preview?limit=100`,
    {
      method: "POST",
      headers: options.headers
    }
  );
}

export async function previewReportWorkflowPolicyDraftImpact(
  body: ReportWorkflowPolicyWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportWorkflowPolicyImpactPreviewResult>> {
  return requestJSON<ReportWorkflowPolicyImpactPreviewResult>(
    "/api/v1/config/report-workflow-policies/impact-preview?limit=100",
    {
      method: "POST",
      body,
      headers: options.headers
    }
  );
}

export async function triggerReportWorkflowPolicyReplay(
  policyID: number,
  body: ReportWorkflowPolicyReplayRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<ReportReplayTriggerResponse>> {
  if (!positivePolicyID(policyID)) {
    return { ok: false, error: { message: "Report workflow policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<ReportReplayTriggerResponse>(`/api/v1/config/report-workflow-policies/${policyID}/replay-window`, {
    method: "POST",
    body,
    headers: options.headers
  });
}

function positivePolicyID(policyID: number): boolean {
  return Number.isSafeInteger(policyID) && policyID > 0;
}
