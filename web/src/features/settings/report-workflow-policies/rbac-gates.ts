import type { CurrentRBACAuthorizationCheck } from "../rbac-capabilities";

export function reportWorkflowPolicyReadKey(policyID: number): string {
  return `reportWorkflowPolicyRead:${policyID}`;
}

export function reportWorkflowPolicyManageKey(policyID: number): string {
  return `reportWorkflowPolicyManage:${policyID}`;
}

export function reportWorkflowPolicyAlertSourceReadKey(
  alertSourceProfileID: number,
): string {
  return `reportWorkflowPolicyAlertSourceRead:${alertSourceProfileID}`;
}

export function reportWorkflowPolicyGroupingPolicyReadKey(
  groupingPolicyID: number,
): string {
  return `reportWorkflowPolicyGroupingPolicyRead:${groupingPolicyID}`;
}

export function reportWorkflowPolicyNotificationChannelTestKey(
  notificationChannelProfileID: number,
): string {
  return `reportWorkflowPolicyNotificationChannelTest:${notificationChannelProfileID}`;
}

export function reportWorkflowPolicyRelationAuthorizationChecks({
  alertSourceProfileIDs,
  groupingPolicyIDs,
  notificationChannelProfileIDs,
}: {
  alertSourceProfileIDs: readonly number[];
  groupingPolicyIDs: readonly number[];
  notificationChannelProfileIDs: readonly number[];
}): CurrentRBACAuthorizationCheck[] {
  return [
    ...uniquePositiveIntegers(alertSourceProfileIDs).map((id) => ({
      key: reportWorkflowPolicyAlertSourceReadKey(id),
      permission: "alert_source.read" as const,
      scopeKey: String(id),
      scopeKind: "alert_source" as const,
    })),
    ...uniquePositiveIntegers(groupingPolicyIDs).map((id) => ({
      key: reportWorkflowPolicyGroupingPolicyReadKey(id),
      permission: "grouping_policy.read" as const,
      scopeKey: String(id),
      scopeKind: "grouping_policy" as const,
    })),
    ...uniquePositiveIntegers(notificationChannelProfileIDs).map((id) => ({
      key: reportWorkflowPolicyNotificationChannelTestKey(id),
      permission: "notification_channel.test" as const,
      scopeKey: String(id),
      scopeKind: "notification_channel" as const,
    })),
  ];
}

export function reportWorkflowPolicyBindingPermissionBlockReason({
  alertSourceProfileID,
  can,
  editingPolicyID,
  groupingPolicyID,
  reportNotificationChannelProfileID,
}: {
  alertSourceProfileID: number | null | undefined;
  can: (key: string) => boolean;
  editingPolicyID: number | null;
  groupingPolicyID: number | null | undefined;
  reportNotificationChannelProfileID: number | null | undefined;
}): string {
  if (editingPolicyID === null) {
    return "";
  }
  const blockers: string[] = [];
  const alertSourceID = positiveIntegerOrNull(alertSourceProfileID);
  if (
    alertSourceID !== null &&
    !can(reportWorkflowPolicyAlertSourceReadKey(alertSourceID))
  ) {
    blockers.push(`read alert source #${alertSourceID}`);
  }
  const selectedGroupingPolicyID = positiveIntegerOrNull(groupingPolicyID);
  if (
    selectedGroupingPolicyID !== null &&
    !can(reportWorkflowPolicyGroupingPolicyReadKey(selectedGroupingPolicyID))
  ) {
    blockers.push(`read grouping policy #${selectedGroupingPolicyID}`);
  }
  const channelID = positiveIntegerOrNull(reportNotificationChannelProfileID);
  if (
    channelID !== null &&
    !can(reportWorkflowPolicyNotificationChannelTestKey(channelID))
  ) {
    blockers.push(`test notification channel #${channelID}`);
  }
  if (blockers.length === 0) {
    return "";
  }
  return `Current user is not authorized to ${blockers.join(", ")}.`;
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
