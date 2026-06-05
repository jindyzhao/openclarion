"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  GroupingPolicy,
  GroupingPolicyListResponse,
  GroupingPolicyPreviewResult,
  GroupingPolicyWriteRequest
} from "./types";

export async function refreshGroupingPolicies(): Promise<ApiResult<GroupingPolicyListResponse>> {
  return requestSameOriginJSON<GroupingPolicyListResponse>("/api/config/grouping-policies");
}

export async function submitGroupingPolicy(
  policyID: number | null,
  body: GroupingPolicyWriteRequest
): Promise<ApiResult<GroupingPolicy>> {
  if (policyID === null) {
    return requestSameOriginJSON<GroupingPolicy>("/api/config/grouping-policies", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<GroupingPolicy>(`/api/config/grouping-policies/${policyID}`, {
    method: "PUT",
    body
  });
}

export async function runGroupingPolicyPreview(policyID: number): Promise<ApiResult<GroupingPolicyPreviewResult>> {
  return requestSameOriginJSON<GroupingPolicyPreviewResult>(`/api/config/grouping-policies/${policyID}/preview`, {
    method: "POST"
  });
}
