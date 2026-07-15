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
