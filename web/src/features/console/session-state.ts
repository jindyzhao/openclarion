import type { QueryClient } from "@tanstack/react-query";

import type { ApiResult } from "@/lib/api/client";
import type {
  DiagnosisAuthStatus,
  DiagnosisBrowserSessionStatus,
} from "@/features/diagnosis-room/transport";

export const consoleBrowserSessionQueryKey = [
  "console",
  "browser-session",
] as const;

export type ConsoleSessionLoginMode =
  | "ldap"
  | "oidc"
  | "static"
  | "unavailable";

export type ConsoleSessionRefreshFailure = {
  error?: string;
  source: "auth-status" | "browser-session";
};

export function consoleBrowserSessionResult(
  status: DiagnosisBrowserSessionStatus,
): ApiResult<DiagnosisBrowserSessionStatus> {
  return { data: status, ok: true };
}

export async function clearConsoleQueryCacheAfterSignOut(
  queryClient: QueryClient,
) {
  await queryClient.cancelQueries();
  queryClient.clear();
  queryClient.setQueryData(
    consoleBrowserSessionQueryKey,
    consoleBrowserSessionResult({ authenticated: false }),
  );
}

export async function replaceConsoleQueryCacheAfterAuthentication(
  queryClient: QueryClient,
  session: Extract<DiagnosisBrowserSessionStatus, { authenticated: true }>,
) {
  await queryClient.cancelQueries();
  queryClient.clear();
  queryClient.setQueryData(
    consoleBrowserSessionQueryKey,
    consoleBrowserSessionResult(session),
  );
}

export function consoleSessionLoginMode(
  status: DiagnosisAuthStatus | undefined,
): ConsoleSessionLoginMode {
  if (
    status === undefined ||
    !status.configured ||
    !status.session_issuance_ready
  ) {
    return "unavailable";
  }
  const oidcReady =
    status.oidc_bff === undefined || status.oidc_bff.status === "ready";
  const modes = [status.mode, ...(status.supported_modes ?? [])];
  for (const mode of modes) {
    if (mode === "oidc" && oidcReady) {
      return mode;
    }
    if (mode === "ldap" || mode === "static") {
      return mode;
    }
  }
  return "unavailable";
}

export function consoleSessionRefreshFailure(
  browserSession: ApiResult<DiagnosisBrowserSessionStatus> | undefined,
  authStatus: ApiResult<DiagnosisAuthStatus> | undefined,
): ConsoleSessionRefreshFailure | null {
  if (browserSession === undefined) {
    return { source: "browser-session" };
  }
  if (!browserSession.ok) {
    return {
      error: browserSession.error.message,
      source: "browser-session",
    };
  }
  if (browserSession.data.authenticated) {
    return null;
  }
  if (authStatus === undefined) {
    return { source: "auth-status" };
  }
  if (!authStatus.ok) {
    return { error: authStatus.error.message, source: "auth-status" };
  }
  return null;
}

export function consoleSessionReturnTo(
  pathname: string,
  rawSearch: string,
): string {
  const normalizedPathname =
    pathname.startsWith("/") && !pathname.startsWith("//")
      ? pathname
      : "/dashboard";
  const params = new URLSearchParams(rawSearch);
  params.delete("oidc_auth_error");
  params.delete("wecom_auth_error");
  params.delete("wecom_auto_login");
  if (normalizedPathname.startsWith("/diagnosis-room")) {
    params.set("auth_mode", "session");
  }
  const query = params.toString();
  return query === "" ? normalizedPathname : `${normalizedPathname}?${query}`;
}

export function consoleSessionModeLabel(
  mode: string,
  unknownLabel: string,
): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "oidc":
      return "OIDC";
    case "static":
      return "Static";
    default:
      return unknownLabel;
  }
}

export type ConsoleSessionLoginErrorKey =
  | "loginErrors.notConfigured"
  | "loginErrors.roleUnauthorized"
  | "loginErrors.callbackMissing"
  | "loginErrors.authFailed"
  | "loginErrors.callbackFailed"
  | "loginErrors.loginFailed";

export function consoleSessionLoginErrorKey(
  error: string | null,
): ConsoleSessionLoginErrorKey | null {
  switch (error) {
    case "oidc_not_configured":
      return "loginErrors.notConfigured";
    case "oidc_role_unauthorized":
      return "loginErrors.roleUnauthorized";
    case "oidc_callback_missing":
      return "loginErrors.callbackMissing";
    case "oidc_auth_failed":
      return "loginErrors.authFailed";
    case "oidc_callback_failed":
      return "loginErrors.callbackFailed";
    case "oidc_login_failed":
      return "loginErrors.loginFailed";
    default:
      return null;
  }
}

export function consoleSessionRolesLabel(
  roles: readonly string[],
  emptyLabel: string,
): string {
  return roles.length === 0 ? emptyLabel : roles.join(", ");
}
