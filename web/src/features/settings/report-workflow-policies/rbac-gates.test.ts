import { describe, expect, it } from "vitest";

import {
  reportWorkflowPolicyAlertSourceReadKey,
  reportWorkflowPolicyBindingPermissionBlockReason,
  reportWorkflowPolicyGroupingPolicyReadKey,
  reportWorkflowPolicyNotificationChannelTestKey,
  reportWorkflowPolicyRelationAuthorizationChecks,
} from "./rbac-gates";

describe("report workflow policy rbac gates", () => {
  it("builds scoped authorization checks for bindable resources", () => {
    expect(
      reportWorkflowPolicyRelationAuthorizationChecks({
        alertSourceProfileIDs: [1, 1, 0],
        groupingPolicyIDs: [2],
        notificationChannelProfileIDs: [3],
      }),
    ).toEqual([
      {
        key: reportWorkflowPolicyAlertSourceReadKey(1),
        permission: "alert_source.read",
        scopeKey: "1",
        scopeKind: "alert_source",
      },
      {
        key: reportWorkflowPolicyGroupingPolicyReadKey(2),
        permission: "grouping_policy.read",
        scopeKey: "2",
        scopeKind: "grouping_policy",
      },
      {
        key: reportWorkflowPolicyNotificationChannelTestKey(3),
        permission: "notification_channel.test",
        scopeKey: "3",
        scopeKind: "notification_channel",
      },
    ]);
  });

  it("does not require binding permissions while creating a new policy", () => {
    expect(
      reportWorkflowPolicyBindingPermissionBlockReason({
        alertSourceProfileID: 1,
        can: () => false,
        editingPolicyID: null,
        groupingPolicyID: 2,
        reportNotificationChannelProfileID: 3,
      }),
    ).toBe("");
  });

  it("requires selected binding permissions while editing an existing policy", () => {
    const allowed = new Set([
      reportWorkflowPolicyAlertSourceReadKey(1),
      reportWorkflowPolicyGroupingPolicyReadKey(2),
    ]);

    expect(
      reportWorkflowPolicyBindingPermissionBlockReason({
        alertSourceProfileID: 1,
        can: (key) => allowed.has(key),
        editingPolicyID: 7,
        groupingPolicyID: 2,
        reportNotificationChannelProfileID: 3,
      }),
    ).toBe("Current user is not authorized to test notification channel #3.");

    allowed.add(reportWorkflowPolicyNotificationChannelTestKey(3));
    expect(
      reportWorkflowPolicyBindingPermissionBlockReason({
        alertSourceProfileID: 1,
        can: (key) => allowed.has(key),
        editingPolicyID: 7,
        groupingPolicyID: 2,
        reportNotificationChannelProfileID: 3,
      }),
    ).toBe("");
  });
});
