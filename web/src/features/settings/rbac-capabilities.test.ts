import { describe, expect, it } from "vitest";

import {
  currentRBACAuthorizationCheckBatches,
  currentRBACAuthorizationNeedsSignIn,
  currentRBACAuthorizationsFromState,
  currentRBACAuthorizationStateAfterActivationChange,
  type CurrentRBACAuthorizationState,
} from "./rbac-capabilities";

describe("current RBAC authorization view", () => {
  it("chunks current authorization checks below the backend request limit", () => {
    const checks = Array.from({ length: 53 }, (_, index) => ({
      key: `check-${index}`,
      permission: "directory.read" as const,
    }));

    expect(
      currentRBACAuthorizationCheckBatches(checks, 50).map(
        (batch) => batch.length,
      ),
    ).toEqual([50, 3]);
  });

  it("does not expose stale permissions when checks are inactive", () => {
    const state: CurrentRBACAuthorizationState = {
      allowed: { "diagnosisRoom.create": true },
      departmentKeys: ["dep-2"],
      directoryUsers: [],
      fingerprint: "previous",
      kind: "ready",
      subject: "iam-user-1",
    };

    const view = currentRBACAuthorizationsFromState({
      active: false,
      current: true,
      state,
    });

    expect(view.can("diagnosisRoom.create")).toBe(false);
    expect(view.isChecking).toBe(false);
    expect(view.notice).toBeNull();
  });

  it("suppresses stale authorization errors while checks are inactive", () => {
    const view = currentRBACAuthorizationsFromState({
      active: false,
      current: true,
      state: {
        fingerprint: "previous",
        kind: "error",
        message: "authentication failed",
      },
    });

    expect(view.notice).toBeNull();
  });

  it("reports pending and ready active checks", () => {
    const loading = currentRBACAuthorizationsFromState({
      active: true,
      current: false,
      state: { fingerprint: "previous", kind: "loading" },
    });
    expect(loading.isChecking).toBe(true);

    const currentLoading = currentRBACAuthorizationsFromState({
      active: true,
      current: true,
      state: { fingerprint: "current", kind: "loading" },
    });
    expect(currentLoading.isChecking).toBe(true);
    expect(currentLoading.can("directory.read")).toBe(false);

    const ready = currentRBACAuthorizationsFromState({
      active: true,
      current: true,
      state: {
        allowed: { "directory.read": true },
        departmentKeys: ["dep-2"],
        directoryUsers: [],
        fingerprint: "current",
        kind: "ready",
        subject: "iam-user-1",
      },
    });
    expect(ready.can("directory.read")).toBe(true);
    expect(ready.can("rbac.manage")).toBe(false);
    expect(ready.isChecking).toBe(false);
  });

  it("clears stale ready state when checks become inactive", () => {
    const state = currentRBACAuthorizationStateAfterActivationChange({
      active: false,
      current: {
        allowed: { "diagnosisRoom.create": true },
        departmentKeys: ["dep-2"],
        directoryUsers: [],
        fingerprint: "previous",
        kind: "ready",
        subject: "iam-user-1",
      },
      fingerprint: "current",
    });

    expect(state).toEqual({ fingerprint: "current", kind: "loading" });
  });

  it("marks a new active fingerprint as loading before the next result", () => {
    const state = currentRBACAuthorizationStateAfterActivationChange({
      active: true,
      current: {
        allowed: { "diagnosisRoom.create": true },
        departmentKeys: ["dep-2"],
        directoryUsers: [],
        fingerprint: "previous",
        kind: "ready",
        subject: "iam-user-1",
      },
      fingerprint: "current",
    });

    expect(state).toEqual({ fingerprint: "current", kind: "loading" });
  });

  it("requires sign-in only for unauthenticated authorization checks", () => {
    expect(
      currentRBACAuthorizationNeedsSignIn({
        fingerprint: "current",
        kind: "error",
        message: "Authentication required.",
        status: 401,
      }),
    ).toBe(true);
    expect(
      currentRBACAuthorizationNeedsSignIn({
        fingerprint: "current",
        kind: "error",
        message: "rbac authorization failed",
        status: 500,
      }),
    ).toBe(false);
    expect(
      currentRBACAuthorizationNeedsSignIn({
        fingerprint: "current",
        kind: "loading",
      }),
    ).toBe(false);
  });
});
