export function selectedWorkspaceID(
  workspaces: ReadonlyArray<{ id: number }>,
  selectedID: number | null,
  currentID: number | null,
): number | null {
  if (
    selectedID !== null &&
    workspaces.some((workspace) => workspace.id === selectedID)
  ) {
    return selectedID;
  }
  if (
    currentID !== null &&
    workspaces.some((workspace) => workspace.id === currentID)
  ) {
    return currentID;
  }
  return workspaces[0]?.id ?? null;
}

export function workspaceStatusChangeBlocked(
  workspace: { id: number; status: "active" | "disabled" },
  currentTenantID: number | null,
  sessionTenantKnown: boolean,
): boolean {
  if (!sessionTenantKnown) {
    return true;
  }
  return (
    workspace.status === "active" &&
    (workspace.id === 1 || workspace.id === currentTenantID)
  );
}

export function membershipDisableBlocked({
  currentSubject,
  currentTenantID,
  enabled,
  selectedTenantID,
  sessionTenantKnown,
  subject,
}: {
  currentSubject: string | null;
  currentTenantID: number | null;
  enabled: boolean;
  selectedTenantID: number | null;
  sessionTenantKnown: boolean;
  subject: string;
}): boolean {
  if (enabled) {
    return false;
  }
  if (!sessionTenantKnown) {
    return true;
  }
  return (
    currentSubject !== null &&
    currentTenantID !== null &&
    selectedTenantID === currentTenantID &&
    subject === currentSubject
  );
}
