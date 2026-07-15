import { describe, expect, it } from "vitest";

import {
  membershipDisableBlocked,
  selectedWorkspaceID,
  workspaceStatusChangeBlocked,
} from "./selection";

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

describe("workspaceStatusChangeBlocked", () => {
  it("fails closed until the active session tenant is known", () => {
    expect(
      workspaceStatusChangeBlocked(
        { id: 7, status: "active" },
        null,
        false,
      ),
    ).toBe(true);
  });

  it("blocks disabling the default or active session workspace", () => {
    expect(
      workspaceStatusChangeBlocked({ id: 1, status: "active" }, 7, true),
    ).toBe(true);
    expect(
      workspaceStatusChangeBlocked({ id: 7, status: "active" }, 7, true),
    ).toBe(true);
    expect(
      workspaceStatusChangeBlocked({ id: 9, status: "active" }, 7, true),
    ).toBe(false);
  });
});

describe("membershipDisableBlocked", () => {
  it("fails closed while the current session identity is unknown", () => {
    expect(
      membershipDisableBlocked({
        currentSubject: null,
        currentTenantID: null,
        enabled: false,
        selectedTenantID: 7,
        sessionTenantKnown: false,
        subject: "operator-1",
      }),
    ).toBe(true);
  });

  it("blocks disabling the signed-in subject in the active workspace", () => {
    expect(
      membershipDisableBlocked({
        currentSubject: "operator-1",
        currentTenantID: 7,
        enabled: false,
        selectedTenantID: 7,
        sessionTenantKnown: true,
        subject: "operator-1",
      }),
    ).toBe(true);
  });

  it("allows other membership updates", () => {
    expect(
      membershipDisableBlocked({
        currentSubject: "operator-1",
        currentTenantID: 7,
        enabled: true,
        selectedTenantID: 7,
        sessionTenantKnown: true,
        subject: "operator-1",
      }),
    ).toBe(false);
    expect(
      membershipDisableBlocked({
        currentSubject: "operator-1",
        currentTenantID: 7,
        enabled: false,
        selectedTenantID: 9,
        sessionTenantKnown: true,
        subject: "operator-1",
      }),
    ).toBe(false);
  });
});
