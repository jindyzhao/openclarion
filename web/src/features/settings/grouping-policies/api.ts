import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  GroupingPolicy,
  GroupingPolicyListResponse,
  GroupingPolicyPreviewResult,
  GroupingPolicyWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchGroupingPolicies(
  options: BackendRequestOptions = {}
): Promise<ApiResult<GroupingPolicyListResponse>> {
  return requestJSON<GroupingPolicyListResponse>("/api/v1/config/grouping-policies?limit=100", {
    headers: options.headers
  });
}

export async function createGroupingPolicy(
  body: GroupingPolicyWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<GroupingPolicy>> {
  return requestJSON<GroupingPolicy>("/api/v1/config/grouping-policies", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceGroupingPolicy(
  policyID: number,
  body: GroupingPolicyWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<GroupingPolicy>> {
  if (!Number.isSafeInteger(policyID) || policyID < 1) {
    return { ok: false, error: { message: "Grouping policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<GroupingPolicy>(`/api/v1/config/grouping-policies/${policyID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function previewGroupingPolicy(
  policyID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<GroupingPolicyPreviewResult>> {
  if (!Number.isSafeInteger(policyID) || policyID < 1) {
    return { ok: false, error: { message: "Grouping policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<GroupingPolicyPreviewResult>(`/api/v1/config/grouping-policies/${policyID}/preview?limit=100`, {
    method: "POST",
    headers: options.headers
  });
}
