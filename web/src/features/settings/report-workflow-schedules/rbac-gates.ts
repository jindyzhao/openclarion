import type { CurrentRBACAuthorizationCheck } from "../rbac-capabilities";
import { reportWorkflowPolicyManageKey } from "../report-workflow-policies/rbac-gates";

export function reportWorkflowScheduleManageKey(scheduleID: number): string {
  return `reportWorkflowScheduleManage:${scheduleID}`;
}

export function reportWorkflowSchedulePolicyAuthorizationChecks(
  policyIDs: readonly number[],
): CurrentRBACAuthorizationCheck[] {
  return uniquePositiveIntegers(policyIDs).map((policyID) => ({
    key: reportWorkflowPolicyManageKey(policyID),
    permission: "report_workflow.manage" as const,
    scopeKey: String(policyID),
    scopeKind: "report_workflow" as const,
  }));
}

export function reportWorkflowSchedulePolicyPermissionBlockReason({
  can,
  editingScheduleID,
  reportWorkflowPolicyID,
}: {
  can: (key: string) => boolean;
  editingScheduleID: number | null;
  reportWorkflowPolicyID: number | null | undefined;
}): string {
  if (editingScheduleID === null) {
    return "";
  }
  const policyID = positiveIntegerOrNull(reportWorkflowPolicyID);
  if (policyID === null || can(reportWorkflowPolicyManageKey(policyID))) {
    return "";
  }
  return `Current user is not authorized to manage report workflow policy #${policyID}.`;
}

function uniquePositiveIntegers(values: readonly number[]): number[] {
  const result: number[] = [];
  const seen = new Set<number>();
  for (const value of values) {
    const positive = positiveIntegerOrNull(value);
    if (positive === null || seen.has(positive)) {
      continue;
    }
    seen.add(positive);
    result.push(positive);
  }
  return result;
}

function positiveIntegerOrNull(value: number | null | undefined): number | null {
  if (
    typeof value !== "number" ||
    !Number.isSafeInteger(value) ||
    value <= 0
  ) {
    return null;
  }
  return value;
}
