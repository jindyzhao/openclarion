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
  diagnosisRoomRBACBlocker,
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
      diagnosisRoomRBACBlocker({
        action: "participate",
        allowed: false,
        checking: false,
        enforced: false,
      }),
    ).toBeNull();
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
        scope: item.scope,
        status: item.status,
      })),
    ).toEqual([
      {
        action: "create",
        key: diagnosisRoomCreateAuthorizationKey,
        scope: { kind: "global" },
        status: "allowed",
      },
      {
        action: "create",
        key: diagnosisRoomCreateOperationsReadAuthorizationKey,
        scope: { kind: "global" },
        status: "allowed",
      },
      {
        action: "create",
        key: diagnosisRoomCreateNotificationChannelAuthorizationKey(5),
        scope: { channelID: 5, kind: "notification-channel" },
        status: "allowed",
      },
      {
        action: "read",
        key: diagnosisRoomReadAuthorizationKey("session-1"),
        scope: { kind: "room", sessionID: "session-1" },
        status: "allowed",
      },
      {
        action: "participate",
        key: diagnosisRoomParticipateAuthorizationKey("session-1"),
        scope: { kind: "room", sessionID: "session-1" },
        status: "denied",
      },
      {
        action: "approve",
        key: diagnosisRoomApproveAuthorizationKey("session-1"),
        scope: { kind: "room", sessionID: "session-1" },
        status: "allowed",
      },
      {
        action: "administer",
        key: diagnosisRoomAdministerAuthorizationKey("session-1"),
        scope: { kind: "room", sessionID: "session-1" },
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

  it("returns semantic blockers when enforced", () => {
    expect(
      diagnosisRoomRBACBlocker({
        action: "administer",
        allowed: false,
        checking: false,
        enforced: true,
      }),
    ).toEqual({ action: "administer", kind: "denied" });
    expect(
      diagnosisRoomRBACBlocker({
        action: "approve",
        allowed: false,
        checking: false,
        enforced: true,
      }),
    ).toEqual({ action: "approve", kind: "denied" });
    expect(
      diagnosisRoomRBACBlocker({
        action: "create",
        allowed: false,
        checking: true,
        enforced: true,
      }),
    ).toEqual({ kind: "checking" });
  });
});
