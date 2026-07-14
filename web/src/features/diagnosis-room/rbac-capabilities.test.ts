import { describe, expect, it } from "vitest";

import {
  canCreateDiagnosisRoomByRBAC,
  diagnosisRoomAdministerAuthorizationKey,
  diagnosisRoomApproveAuthorizationKey,
  diagnosisRoomCreateAuthorizationKey,
  diagnosisRoomCreateNotificationChannelAuthorizationKey,
  diagnosisRoomCreateOperationsReadAuthorizationKey,
  diagnosisRoomParticipateAuthorizationKey,
  diagnosisRoomRBACAuthorizationChecks,
  diagnosisRoomRBACBlockReason,
  diagnosisRoomRBACPermissionItems,
  diagnosisRoomReadAuthorizationKey,
} from "./rbac-capabilities";

describe("diagnosis room rbac capabilities", () => {
  it("builds global create and scoped room action checks", () => {
    const checks = diagnosisRoomRBACAuthorizationChecks([
      "session-1",
      " session-1 ",
      "",
      "session-2",
    ]);

    expect(checks).toEqual([
      {
        key: diagnosisRoomCreateAuthorizationKey,
        permission: "diagnosis_room.participate",
      },
      {
        key: diagnosisRoomCreateOperationsReadAuthorizationKey,
        permission: "operations.read",
      },
      {
        key: diagnosisRoomReadAuthorizationKey("session-1"),
        permission: "diagnosis_room.read",
        scopeKey: "session-1",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomParticipateAuthorizationKey("session-1"),
        permission: "diagnosis_room.participate",
        scopeKey: "session-1",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomApproveAuthorizationKey("session-1"),
        permission: "diagnosis_room.approve",
        scopeKey: "session-1",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomAdministerAuthorizationKey("session-1"),
        permission: "diagnosis_room.administer",
        scopeKey: "session-1",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomReadAuthorizationKey("session-2"),
        permission: "diagnosis_room.read",
        scopeKey: "session-2",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomParticipateAuthorizationKey("session-2"),
        permission: "diagnosis_room.participate",
        scopeKey: "session-2",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomApproveAuthorizationKey("session-2"),
        permission: "diagnosis_room.approve",
        scopeKey: "session-2",
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomAdministerAuthorizationKey("session-2"),
        permission: "diagnosis_room.administer",
        scopeKey: "session-2",
        scopeKind: "diagnosis_room",
      },
    ]);
  });

  it("includes selected notification channel access in room creation checks", () => {
    expect(
      diagnosisRoomRBACAuthorizationChecks([], {
        closeNotificationChannelProfileID: 5,
      }),
    ).toEqual([
      {
        key: diagnosisRoomCreateAuthorizationKey,
        permission: "diagnosis_room.participate",
      },
      {
        key: diagnosisRoomCreateOperationsReadAuthorizationKey,
        permission: "operations.read",
      },
      {
        key: diagnosisRoomCreateNotificationChannelAuthorizationKey(5),
        permission: "notification_channel.test",
        scopeKey: "5",
        scopeKind: "notification_channel",
      },
    ]);
  });

  it("does not block direct credential flows when current-session rbac is not enforced", () => {
    expect(
      diagnosisRoomRBACBlockReason({
        action: "participate",
        allowed: false,
        checking: false,
        enforced: false,
      }),
    ).toBe("");
  });

  it("builds room permission status items for the selected session", () => {
    const allowedKeys = new Set([
      diagnosisRoomCreateAuthorizationKey,
      diagnosisRoomCreateOperationsReadAuthorizationKey,
      diagnosisRoomCreateNotificationChannelAuthorizationKey(5),
      diagnosisRoomReadAuthorizationKey("session-1"),
      diagnosisRoomApproveAuthorizationKey("session-1"),
      diagnosisRoomAdministerAuthorizationKey("session-1"),
    ]);

    const items = diagnosisRoomRBACPermissionItems({
      can: (key) => allowedKeys.has(key),
      closeNotificationChannelProfileID: 5,
      checking: false,
      enforced: true,
      sessionID: " session-1 ",
    });

    expect(
      items.map((item) => ({
        action: item.action,
        key: item.key,
        scopeLabel: item.scopeLabel,
        status: item.status,
      })),
    ).toEqual([
      {
        action: "create",
        key: diagnosisRoomCreateAuthorizationKey,
        scopeLabel: "Global",
        status: "allowed",
      },
      {
        action: "create",
        key: diagnosisRoomCreateOperationsReadAuthorizationKey,
        scopeLabel: "Global",
        status: "allowed",
      },
      {
        action: "create",
        key: diagnosisRoomCreateNotificationChannelAuthorizationKey(5),
        scopeLabel: "Notification channel #5",
        status: "allowed",
      },
      {
        action: "read",
        key: diagnosisRoomReadAuthorizationKey("session-1"),
        scopeLabel: "session-1",
        status: "allowed",
      },
      {
        action: "participate",
        key: diagnosisRoomParticipateAuthorizationKey("session-1"),
        scopeLabel: "session-1",
        status: "denied",
      },
      {
        action: "approve",
        key: diagnosisRoomApproveAuthorizationKey("session-1"),
        scopeLabel: "session-1",
        status: "allowed",
      },
      {
        action: "administer",
        key: diagnosisRoomAdministerAuthorizationKey("session-1"),
        scopeLabel: "session-1",
        status: "allowed",
      },
    ]);
  });

  it("marks scoped permissions as not selected without a room", () => {
    const items = diagnosisRoomRBACPermissionItems({
      can: () => false,
      checking: false,
      enforced: true,
      sessionID: "",
    });

    expect(items.map((item) => item.status)).toEqual([
      "denied",
      "denied",
      "not-selected",
      "not-selected",
      "not-selected",
      "not-selected",
    ]);
  });

  it("marks selected room permissions as not enforced for direct credential flows", () => {
    const items = diagnosisRoomRBACPermissionItems({
      can: () => false,
      checking: false,
      enforced: false,
      sessionID: "session-1",
    });

    expect(items.map((item) => item.status)).toEqual([
      "not-enforced",
      "not-enforced",
      "not-enforced",
      "not-enforced",
      "not-enforced",
      "not-enforced",
    ]);
  });

  it("requires all backend room creation permissions when rbac is enforced", () => {
    const allowedKeys = new Set([
      diagnosisRoomCreateAuthorizationKey,
      diagnosisRoomCreateOperationsReadAuthorizationKey,
    ]);

    expect(
      canCreateDiagnosisRoomByRBAC({
        can: (key) => allowedKeys.has(key),
        enforced: true,
      }),
    ).toBe(true);
    expect(
      canCreateDiagnosisRoomByRBAC({
        can: (key) => allowedKeys.has(key),
        closeNotificationChannelProfileID: 5,
        enforced: true,
      }),
    ).toBe(false);
    allowedKeys.add(diagnosisRoomCreateNotificationChannelAuthorizationKey(5));
    expect(
      canCreateDiagnosisRoomByRBAC({
        can: (key) => allowedKeys.has(key),
        closeNotificationChannelProfileID: 5,
        enforced: true,
      }),
    ).toBe(true);
  });

  it("returns action-specific block reasons when enforced", () => {
    expect(
      diagnosisRoomRBACBlockReason({
        action: "administer",
        allowed: false,
        checking: false,
        enforced: true,
      }),
    ).toBe("Current user is not authorized to administer this diagnosis room.");
    expect(
      diagnosisRoomRBACBlockReason({
        action: "approve",
        allowed: false,
        checking: false,
        enforced: true,
      }),
    ).toBe(
      "Current user is not authorized to approve this diagnosis conclusion.",
    );
    expect(
      diagnosisRoomRBACBlockReason({
        action: "create",
        allowed: false,
        checking: true,
        enforced: true,
      }),
    ).toBe("Checking diagnosis room permissions.");
  });
});
