import type { CurrentRBACAuthorizationCheck } from "@/features/settings/rbac-capabilities";

export const diagnosisRoomCreateAuthorizationKey = "diagnosisRoom.create";

type DiagnosisRoomRBACAction =
  | "administer"
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

export function diagnosisRoomRBACAuthorizationChecks(
  sessionIDs: readonly string[],
): CurrentRBACAuthorizationCheck[] {
  const checks: CurrentRBACAuthorizationCheck[] = [
    {
      key: diagnosisRoomCreateAuthorizationKey,
      permission: "diagnosis_room.participate",
    },
  ];
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
  can,
  checking,
  enforced,
  sessionID,
}: {
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
    ...diagnosisRoomScopedPermissionItems({
      can,
      checking,
      enforced,
      sessionID: normalizedSessionID,
    }),
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

export function diagnosisRoomRBACBlockReason({
  action,
  allowed,
  checking,
  enforced,
}: {
  action: "administer" | "create" | "participate" | "read";
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
    case "participate":
      return "Current user is not authorized to participate in this diagnosis room.";
    case "read":
      return "Current user is not authorized to read this diagnosis room.";
  }
}
