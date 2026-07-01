import { describe, expect, it } from "vitest";

import {
  diagnosisRoomAdministerAuthorizationKey,
  diagnosisRoomCreateAuthorizationKey,
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
        key: diagnosisRoomAdministerAuthorizationKey("session-2"),
        permission: "diagnosis_room.administer",
        scopeKey: "session-2",
        scopeKind: "diagnosis_room",
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
      diagnosisRoomReadAuthorizationKey("session-1"),
      diagnosisRoomAdministerAuthorizationKey("session-1"),
    ]);

    const items = diagnosisRoomRBACPermissionItems({
      can: (key) => allowedKeys.has(key),
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
    ]);
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
        action: "create",
        allowed: false,
        checking: true,
        enforced: true,
      }),
    ).toBe("Checking diagnosis room permissions.");
  });
});
