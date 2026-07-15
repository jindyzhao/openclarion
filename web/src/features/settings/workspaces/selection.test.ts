import { describe, expect, it } from "vitest";

import { selectedWorkspaceID } from "./selection";

const workspaces = [{ id: 1 }, { id: 7 }, { id: 9 }];

describe("selectedWorkspaceID", () => {
  it("preserves an explicit visible selection", () => {
    expect(selectedWorkspaceID(workspaces, 9, 7)).toBe(9);
  });

  it("prefers the current session workspace without an explicit selection", () => {
    expect(selectedWorkspaceID(workspaces, null, 7)).toBe(7);
  });

  it("falls back to the first visible workspace for stale or missing ids", () => {
    expect(selectedWorkspaceID(workspaces, 99, 88)).toBe(1);
    expect(selectedWorkspaceID([], null, null)).toBeNull();
  });
});
