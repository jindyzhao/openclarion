import type { components } from "@/lib/api/openapi";

export type GroupingPolicy = components["schemas"]["GroupingPolicy"];
export type GroupingPolicyListResponse = components["schemas"]["GroupingPolicyListResponse"];
export type GroupingPolicyPreviewGroup = components["schemas"]["GroupingPolicyPreviewGroup"];
export type GroupingPolicyPreviewResult = components["schemas"]["GroupingPolicyPreviewResult"];
export type GroupingPolicyWriteRequest = components["schemas"]["GroupingPolicyWriteRequest"];

export type GroupingPolicyFormState = {
  name: string;
  dimensionKeysText: string;
  severityKey: string;
  sourceFilterText: string;
  enabled: boolean;
};
