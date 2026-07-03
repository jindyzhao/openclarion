import { describe, expect, it } from "vitest";

import { reportWorkflowPolicyManageKey } from "../report-workflow-policies/rbac-gates";
import {
  reportWorkflowSchedulePolicyAuthorizationChecks,
  reportWorkflowSchedulePolicyPermissionBlockReason,
} from "./rbac-gates";

describe("report workflow schedule rbac gates", () => {
  it("builds scoped manage checks for bindable workflow policies", () => {
    expect(reportWorkflowSchedulePolicyAuthorizationChecks([7, 7, 0])).toEqual([
      {
        key: reportWorkflowPolicyManageKey(7),
        permission: "report_workflow.manage",
        scopeKey: "7",
        scopeKind: "report_workflow",
      },
    ]);
  });

  it("does not require policy manage permission while creating a new schedule", () => {
    expect(
      reportWorkflowSchedulePolicyPermissionBlockReason({
        can: () => false,
        editingScheduleID: null,
        reportWorkflowPolicyID: 7,
      }),
    ).toBe("");
  });

  it("requires selected policy manage permission while editing an existing schedule", () => {
    expect(
      reportWorkflowSchedulePolicyPermissionBlockReason({
        can: () => false,
        editingScheduleID: 9,
        reportWorkflowPolicyID: 7,
      }),
    ).toBe(
      "Current user is not authorized to manage report workflow policy #7.",
    );
    expect(
      reportWorkflowSchedulePolicyPermissionBlockReason({
        can: (key) => key === reportWorkflowPolicyManageKey(7),
        editingScheduleID: 9,
        reportWorkflowPolicyID: 7,
      }),
    ).toBe("");
  });
});
