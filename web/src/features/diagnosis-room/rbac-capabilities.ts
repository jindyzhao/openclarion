import type { CurrentRBACAuthorizationCheck } from "@/features/settings/rbac-capabilities";

export const diagnosisRoomCreateAuthorizationKey = "diagnosisRoom.create";
export const diagnosisRoomCreateOperationsReadAuthorizationKey =
  "diagnosisRoom.create.operationsRead";

export type DiagnosisRoomRBACAction =
  | "administer"
  | "approve"
  | "create"
  | "participate"
  | "read";

export type DiagnosisRoomRBACPermissionStatus =
  | "allowed"
  | "checking"
  | "denied"
  | "not-enforced"
  | "not-selected";

export type DiagnosisRoomRBACBlocker =
  | { kind: "checking" }
  | { action: DiagnosisRoomRBACAction; kind: "denied" };

type DiagnosisRoomRBACPermissionScope =
  | { kind: "global" }
  | { channelID: number; kind: "notification-channel" }
  | { kind: "room"; sessionID: string | null };

export type DiagnosisRoomRBACPermissionItem = {
  action: DiagnosisRoomRBACAction;
  key: string;
  permission: CurrentRBACAuthorizationCheck["permission"];
  scope: DiagnosisRoomRBACPermissionScope;
  status: DiagnosisRoomRBACPermissionStatus;
};

export function diagnosisRoomReadAuthorizationKey(sessionID: string): string {
  return `diagnosisRoom.${sessionID}.read`;
}

export function diagnosisRoomParticipateAuthorizationKey(
  sessionID: string,
): string {
  return `diagnosisRoom.${sessionID}.participate`;
}

export function diagnosisRoomAdministerAuthorizationKey(
  sessionID: string,
): string {
  return `diagnosisRoom.${sessionID}.administer`;
}

export function diagnosisRoomApproveAuthorizationKey(
  sessionID: string,
): string {
  return `diagnosisRoom.${sessionID}.approve`;
}

export function diagnosisRoomCreateNotificationChannelAuthorizationKey(
  channelID: number,
): string {
  return `diagnosisRoom.create.notificationChannel:${channelID}`;
}

export function diagnosisRoomRBACAuthorizationChecks(
  sessionIDs: readonly string[],
  options: {
    closeNotificationChannelProfileID?: number | null;
  } = {},
): CurrentRBACAuthorizationCheck[] {
  const checks: CurrentRBACAuthorizationCheck[] = [
    {
      key: diagnosisRoomCreateAuthorizationKey,
      permission: "diagnosis_room.participate",
    },
    {
      key: diagnosisRoomCreateOperationsReadAuthorizationKey,
      permission: "operations.read",
    },
  ];
  const closeNotificationChannelProfileID =
    positiveIntegerOrNull(options.closeNotificationChannelProfileID);
  if (closeNotificationChannelProfileID !== null) {
    checks.push({
      key: diagnosisRoomCreateNotificationChannelAuthorizationKey(
        closeNotificationChannelProfileID,
      ),
      permission: "notification_channel.test",
      scopeKey: String(closeNotificationChannelProfileID),
      scopeKind: "notification_channel",
    });
  }
  const seen = new Set<string>();
  for (const sessionID of sessionIDs) {
    const normalized = sessionID.trim();
    if (normalized === "" || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    checks.push(
      {
        key: diagnosisRoomReadAuthorizationKey(normalized),
        permission: "diagnosis_room.read",
        scopeKey: normalized,
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomParticipateAuthorizationKey(normalized),
        permission: "diagnosis_room.participate",
        scopeKey: normalized,
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomApproveAuthorizationKey(normalized),
        permission: "diagnosis_room.approve",
        scopeKey: normalized,
        scopeKind: "diagnosis_room",
      },
      {
        key: diagnosisRoomAdministerAuthorizationKey(normalized),
        permission: "diagnosis_room.administer",
        scopeKey: normalized,
        scopeKind: "diagnosis_room",
      },
    );
  }
  return checks;
}

export function diagnosisRoomRBACPermissionItems({
  closeNotificationChannelProfileID,
  can,
  checking,
  enforced,
  sessionID,
}: {
  closeNotificationChannelProfileID?: number | null;
  can: (key: string) => boolean;
  checking: boolean;
  enforced: boolean;
  sessionID: string | undefined;
}): DiagnosisRoomRBACPermissionItem[] {
  const normalizedSessionID = sessionID?.trim() ?? "";
  return [
    {
      action: "create",
      key: diagnosisRoomCreateAuthorizationKey,
      permission:
        "diagnosis_room.participate" as CurrentRBACAuthorizationCheck["permission"],
      scope: { kind: "global" },
      status: diagnosisRoomRBACPermissionStatus({
        can,
        checking,
        enforced,
        key: diagnosisRoomCreateAuthorizationKey,
        selected: true,
      }),
    },
    {
      action: "create",
      key: diagnosisRoomCreateOperationsReadAuthorizationKey,
      permission: "operations.read" as CurrentRBACAuthorizationCheck["permission"],
      scope: { kind: "global" },
      status: diagnosisRoomRBACPermissionStatus({
        can,
        checking,
        enforced,
        key: diagnosisRoomCreateOperationsReadAuthorizationKey,
        selected: true,
      }),
    },
    ...diagnosisRoomCreateNotificationChannelPermissionItems({
      can,
      checking,
      closeNotificationChannelProfileID,
      enforced,
    }),
    ...diagnosisRoomScopedPermissionItems({
      can,
      checking,
      enforced,
      sessionID: normalizedSessionID,
    }),
  ];
}

export function canCreateDiagnosisRoomByRBAC({
  can,
  closeNotificationChannelProfileID,
  enforced,
}: {
  can: (key: string) => boolean;
  closeNotificationChannelProfileID?: number | null;
  enforced: boolean;
}): boolean {
  if (!enforced) {
    return true;
  }
  return diagnosisRoomCreateAuthorizationKeys({
    closeNotificationChannelProfileID,
  }).every((key) => can(key));
}

function diagnosisRoomCreateAuthorizationKeys({
  closeNotificationChannelProfileID,
}: {
  closeNotificationChannelProfileID?: number | null;
}): string[] {
  const keys = [
    diagnosisRoomCreateAuthorizationKey,
    diagnosisRoomCreateOperationsReadAuthorizationKey,
  ];
  const channelID = positiveIntegerOrNull(closeNotificationChannelProfileID);
  if (channelID !== null) {
    keys.push(diagnosisRoomCreateNotificationChannelAuthorizationKey(channelID));
  }
  return keys;
}

function diagnosisRoomCreateNotificationChannelPermissionItems({
  can,
  checking,
  closeNotificationChannelProfileID,
  enforced,
}: {
  can: (key: string) => boolean;
  checking: boolean;
  closeNotificationChannelProfileID?: number | null;
  enforced: boolean;
}): DiagnosisRoomRBACPermissionItem[] {
  const channelID = positiveIntegerOrNull(closeNotificationChannelProfileID);
  if (channelID === null) {
    return [];
  }
  const key = diagnosisRoomCreateNotificationChannelAuthorizationKey(channelID);
  return [
    {
      action: "create",
      key,
      permission:
        "notification_channel.test" as CurrentRBACAuthorizationCheck["permission"],
      scope: { channelID, kind: "notification-channel" },
      status: diagnosisRoomRBACPermissionStatus({
        can,
        checking,
        enforced,
        key,
        selected: true,
      }),
    },
  ];
}

function diagnosisRoomScopedPermissionItems({
  can,
  checking,
  enforced,
  sessionID,
}: {
  can: (key: string) => boolean;
  checking: boolean;
  enforced: boolean;
  sessionID: string;
}): DiagnosisRoomRBACPermissionItem[] {
  const selected = sessionID !== "";
  const scopedPermissions: Array<{
    action: Exclude<DiagnosisRoomRBACAction, "create">;
    key: string;
    permission: CurrentRBACAuthorizationCheck["permission"];
  }> = [
    {
      action: "read",
      key: selected ? diagnosisRoomReadAuthorizationKey(sessionID) : "",
      permission:
        "diagnosis_room.read" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "participate",
      key: selected ? diagnosisRoomParticipateAuthorizationKey(sessionID) : "",
      permission:
        "diagnosis_room.participate" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "approve",
      key: selected ? diagnosisRoomApproveAuthorizationKey(sessionID) : "",
      permission:
        "diagnosis_room.approve" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "administer",
      key: selected ? diagnosisRoomAdministerAuthorizationKey(sessionID) : "",
      permission:
        "diagnosis_room.administer" as CurrentRBACAuthorizationCheck["permission"],
    },
  ];

  return scopedPermissions.map((permission) => ({
    ...permission,
    scope: { kind: "room", sessionID: selected ? sessionID : null },
    status: diagnosisRoomRBACPermissionStatus({
      can,
      checking,
      enforced,
      key: permission.key,
      selected,
    }),
  }));
}

function diagnosisRoomRBACPermissionStatus({
  can,
  checking,
  enforced,
  key,
  selected,
}: {
  can: (key: string) => boolean;
  checking: boolean;
  enforced: boolean;
  key: string;
  selected: boolean;
}): DiagnosisRoomRBACPermissionStatus {
  if (!selected) {
    return "not-selected";
  }
  if (!enforced) {
    return "not-enforced";
  }
  if (checking) {
    return "checking";
  }
  return can(key) ? "allowed" : "denied";
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

export function diagnosisRoomRBACBlocker({
  action,
  allowed,
  checking,
  enforced,
}: {
  action: DiagnosisRoomRBACAction;
  allowed: boolean;
  checking: boolean;
  enforced: boolean;
}): DiagnosisRoomRBACBlocker | null {
  if (!enforced || allowed) {
    return null;
  }
  if (checking) {
    return { kind: "checking" };
  }
  return { action, kind: "denied" };
}
