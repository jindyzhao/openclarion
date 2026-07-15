import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import {
  clearConsoleQueryCacheAfterSignOut,
  consoleBrowserSessionQueryKey,
  consoleBrowserSessionResult,
  consoleSessionLoginErrorKey,
  consoleSessionModeLabel,
  consoleSessionReturnTo,
  consoleSessionRolesLabel,
  isConsoleBrowserSessionQueryKey,
} from "./session-state";

describe("console browser session state", () => {
  it("preserves the current route while removing one-shot auth errors", () => {
    expect(
      consoleSessionReturnTo(
        "/reports/42",
        "tab=delivery&oidc_auth_error=oidc_callback_failed",
      ),
    ).toBe("/reports/42?tab=delivery");
  });

  it("normalizes diagnosis sign-in to browser session mode", () => {
    expect(
      consoleSessionReturnTo(
        "/diagnosis-room",
        "session_id=room-1&auth_mode=wecom&wecom_auto_login=1&wecom_auth_error=wecom_login_failed",
      ),
    ).toBe(
      "/diagnosis-room?session_id=room-1&auth_mode=session",
    );
  });

  it("falls back to the dashboard for a non-local pathname", () => {
    expect(consoleSessionReturnTo("//outside.example", "")).toBe(
      "/dashboard",
    );
  });

  it("matches only the shared browser session cache key", () => {
    expect(isConsoleBrowserSessionQueryKey(consoleBrowserSessionQueryKey)).toBe(
      true,
    );
    expect(
      isConsoleBrowserSessionQueryKey(["console", "browser-session", "old"]),
    ).toBe(false);
    expect(
      isConsoleBrowserSessionQueryKey(["diagnosis-room", "browser-session"]),
    ).toBe(false);
  });

  it("removes protected cache data and retains an anonymous session state", async () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(consoleBrowserSessionQueryKey, {
      data: {
        authenticated: true,
        checked_at: "2026-07-11T04:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
        tenant_id: 1,
        tenant_key: "default",
      },
      ok: true,
    });
    queryClient.setQueryData(["reports", "detail", 42], {
      conclusion: "protected",
    });

    await clearConsoleQueryCacheAfterSignOut(queryClient);

    expect(queryClient.getQueryData(["reports", "detail", 42])).toBeUndefined();
    expect(queryClient.getQueryData(consoleBrowserSessionQueryKey)).toEqual({
      data: { authenticated: false },
      ok: true,
    });
  });

  it("builds cache values and concise identity labels", () => {
    expect(consoleBrowserSessionResult({ authenticated: false })).toEqual({
      data: { authenticated: false },
      ok: true,
    });
    expect(consoleSessionModeLabel("oidc", "Unknown")).toBe("OIDC");
    expect(consoleSessionModeLabel("unsupported", "Unknown")).toBe("Unknown");
    expect(consoleSessionLoginErrorKey("oidc_not_configured")).toBe(
      "loginErrors.notConfigured",
    );
    expect(consoleSessionLoginErrorKey("provider-secret")).toBeNull();
    expect(consoleSessionRolesLabel(["owner", "admin"], "No roles")).toBe(
      "owner, admin",
    );
    expect(consoleSessionRolesLabel([], "No roles")).toBe("No roles");
  });
});
