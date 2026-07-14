import type { CurrentRBACAuthorizationCheck } from "@/features/settings/rbac-capabilities";

export const diagnosisRoomCreateAuthorizationKey = "diagnosisRoom.create";
export const diagnosisRoomCreateOperationsReadAuthorizationKey =
  "diagnosisRoom.create.operationsRead";

type DiagnosisRoomRBACAction =
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

export type DiagnosisRoomRBACPermissionItem = {
  action: DiagnosisRoomRBACAction;
  key: string;
  label: string;
  permission: CurrentRBACAuthorizationCheck["permission"];
  scopeLabel: string;
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
      label: "Create rooms",
      permission:
        "diagnosis_room.participate" as CurrentRBACAuthorizationCheck["permission"],
      scopeLabel: "Global",
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
      label: "Read evidence snapshots",
      permission: "operations.read" as CurrentRBACAuthorizationCheck["permission"],
      scopeLabel: "Global",
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
      label: "Use close channel",
      permission:
        "notification_channel.test" as CurrentRBACAuthorizationCheck["permission"],
      scopeLabel: `Notification channel #${channelID}`,
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
  const scopeLabel = selected ? sessionID : "No room selected";
  const scopedPermissions: Array<{
    action: Exclude<DiagnosisRoomRBACAction, "create">;
    key: string;
    label: string;
    permission: CurrentRBACAuthorizationCheck["permission"];
  }> = [
    {
      action: "read",
      key: selected ? diagnosisRoomReadAuthorizationKey(sessionID) : "",
      label: "Read room",
      permission:
        "diagnosis_room.read" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "participate",
      key: selected ? diagnosisRoomParticipateAuthorizationKey(sessionID) : "",
      label: "Participate",
      permission:
        "diagnosis_room.participate" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "approve",
      key: selected ? diagnosisRoomApproveAuthorizationKey(sessionID) : "",
      label: "Approve conclusion",
      permission:
        "diagnosis_room.approve" as CurrentRBACAuthorizationCheck["permission"],
    },
    {
      action: "administer",
      key: selected ? diagnosisRoomAdministerAuthorizationKey(sessionID) : "",
      label: "Administer",
      permission:
        "diagnosis_room.administer" as CurrentRBACAuthorizationCheck["permission"],
    },
  ];

  return scopedPermissions.map((permission) => ({
    ...permission,
    scopeLabel,
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

export function diagnosisRoomRBACBlockReason({
  action,
  allowed,
  checking,
  enforced,
}: {
  action: DiagnosisRoomRBACAction;
  allowed: boolean;
  checking: boolean;
  enforced: boolean;
}): string {
  if (!enforced || allowed) {
    return "";
  }
  if (checking) {
    return "Checking diagnosis room permissions.";
  }
  switch (action) {
    case "administer":
      return "Current user is not authorized to administer this diagnosis room.";
    case "create":
      return "Current user is not authorized to create diagnosis rooms.";
    case "approve":
      return "Current user is not authorized to approve this diagnosis conclusion.";
    case "participate":
      return "Current user is not authorized to participate in this diagnosis room.";
    case "read":
      return "Current user is not authorized to read this diagnosis room.";
  }
}
