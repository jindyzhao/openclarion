import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  GroupingPolicy,
  GroupingPolicyListResponse,
  GroupingPolicyPreviewResult,
  GroupingPolicyWriteRequest
} from "./types";

export async function fetchGroupingPolicies(): Promise<ApiResult<GroupingPolicyListResponse>> {
  return requestJSON<GroupingPolicyListResponse>("/api/v1/config/grouping-policies?limit=100");
}

export async function createGroupingPolicy(body: GroupingPolicyWriteRequest): Promise<ApiResult<GroupingPolicy>> {
  return requestJSON<GroupingPolicy>("/api/v1/config/grouping-policies", {
    method: "POST",
    body
  });
}

export async function replaceGroupingPolicy(
  policyID: number,
  body: GroupingPolicyWriteRequest
): Promise<ApiResult<GroupingPolicy>> {
  if (!Number.isSafeInteger(policyID) || policyID < 1) {
    return { ok: false, error: { message: "Grouping policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<GroupingPolicy>(`/api/v1/config/grouping-policies/${policyID}`, {
    method: "PUT",
    body
  });
}

export async function previewGroupingPolicy(policyID: number): Promise<ApiResult<GroupingPolicyPreviewResult>> {
  if (!Number.isSafeInteger(policyID) || policyID < 1) {
    return { ok: false, error: { message: "Grouping policy ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<GroupingPolicyPreviewResult>(`/api/v1/config/grouping-policies/${policyID}/preview?limit=100`, {
    method: "POST"
  });
}
