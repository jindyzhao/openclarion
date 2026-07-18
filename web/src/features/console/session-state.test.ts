import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import {
  clearConsoleQueryCacheAfterSignOut,
  consoleBrowserSessionQueryKey,
  consoleBrowserSessionResult,
  consoleSessionLoginErrorKey,
  consoleSessionLoginMode,
  consoleSessionModeLabel,
  consoleSessionRefreshFailure,
  consoleSessionReturnTo,
  consoleSessionRolesLabel,
  replaceConsoleQueryCacheAfterAuthentication,
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
    queryClient.getMutationCache().build(queryClient, {
      mutationFn: async () => undefined,
    });

    await clearConsoleQueryCacheAfterSignOut(queryClient);

    expect(queryClient.getQueryData(["reports", "detail", 42])).toBeUndefined();
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
    expect(queryClient.getQueryData(consoleBrowserSessionQueryKey)).toEqual({
      data: { authenticated: false },
      ok: true,
    });
  });

  it("replaces anonymous and protected cache data after authentication", async () => {
    const queryClient = new QueryClient();
    queryClient.setQueryData(consoleBrowserSessionQueryKey, {
      data: { authenticated: false },
      ok: true,
    });
    queryClient.setQueryData(["dashboard"], { alerts: "anonymous" });
    queryClient.getMutationCache().build(queryClient, {
      mutationFn: async () => undefined,
    });
    const session = {
      authenticated: true as const,
      checked_at: "2026-07-18T04:00:00Z",
      mode: "static" as const,
      role_authorized: true,
      roles: ["admin"],
      subject: "operator-1",
      tenant_id: 1,
      tenant_key: "default",
    };

    await replaceConsoleQueryCacheAfterAuthentication(queryClient, session);

    expect(queryClient.getQueryData(["dashboard"])).toBeUndefined();
    expect(queryClient.getMutationCache().getAll()).toHaveLength(0);
    expect(queryClient.getQueryData(consoleBrowserSessionQueryKey)).toEqual({
      data: session,
      ok: true,
    });
  });

  it("selects the configured console sign-in flow", () => {
    expect(consoleSessionLoginMode(undefined)).toBe("unavailable");
    expect(
      consoleSessionLoginMode({
        configured: false,
        mode: "none",
        session_issuance_ready: false,
      }),
    ).toBe("unavailable");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "static",
        session_issuance_ready: true,
      }),
    ).toBe("static");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "ldap",
        session_issuance_ready: true,
      }),
    ).toBe("ldap");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "oidc",
        session_issuance_ready: true,
      }),
    ).toBe("oidc");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "ldap",
        session_issuance_ready: false,
      }),
    ).toBe("unavailable");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "ldap",
        session_issuance_ready: true,
        supported_modes: ["ldap", "oidc"],
      }),
    ).toBe("ldap");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "unknown",
        session_issuance_ready: true,
        supported_modes: ["static", "oidc"],
      }),
    ).toBe("static");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "oidc",
        session_issuance_ready: true,
        oidc_bff: {
          browser_session_signing_key_configured: false,
          client_auth_method: "auto",
          client_id_configured: false,
          client_secret_configured: false,
          configured: false,
          issuer_configured: false,
          missing: ["issuer"],
          pkce_enabled: true,
          redirect_url_configured: false,
          scopes_include_openid: true,
          state_signing_key_configured: false,
          status: "blocked",
        },
        supported_modes: ["oidc", "static"],
      }),
    ).toBe("static");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "oidc",
        session_issuance_ready: true,
        oidc_bff: {
          browser_session_signing_key_configured: false,
          client_auth_method: "auto",
          client_id_configured: false,
          client_secret_configured: false,
          configured: false,
          issuer_configured: false,
          missing: ["issuer"],
          pkce_enabled: true,
          redirect_url_configured: false,
          scopes_include_openid: true,
          state_signing_key_configured: false,
          status: "blocked",
        },
        supported_modes: ["oidc"],
      }),
    ).toBe("unavailable");
    expect(
      consoleSessionLoginMode({
        configured: true,
        mode: "unknown",
        session_issuance_ready: true,
      }),
    ).toBe("unavailable");
  });

  it("requires both anonymous session and auth-status refreshes to succeed", () => {
    const anonymousSession = consoleBrowserSessionResult({
      authenticated: false,
    });
    expect(
      consoleSessionRefreshFailure(
        {
          error: { message: "session unavailable", status: 503 },
          ok: false,
        },
        undefined,
      ),
    ).toEqual({
      error: "session unavailable",
      source: "browser-session",
    });
    expect(
      consoleSessionRefreshFailure(anonymousSession, {
        error: { message: "auth status unavailable", status: 503 },
        ok: false,
      }),
    ).toEqual({
      error: "auth status unavailable",
      source: "auth-status",
    });
    expect(
      consoleSessionRefreshFailure(anonymousSession, {
        data: {
          configured: true,
          mode: "ldap",
          session_issuance_ready: true,
        },
        ok: true,
      }),
    ).toBeNull();
    expect(
      consoleSessionRefreshFailure(
        {
          data: {
            authenticated: true,
            checked_at: "2026-07-18T04:00:00Z",
            mode: "oidc",
            role_authorized: true,
            roles: ["owner"],
            subject: "operator-1",
            tenant_id: 1,
            tenant_key: "default",
          },
          ok: true,
        },
        undefined,
      ),
    ).toBeNull();
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
