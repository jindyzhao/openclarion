import type { QueryClient } from "@tanstack/react-query";

import type { ApiResult } from "@/lib/api/client";
import type { DiagnosisBrowserSessionStatus } from "@/features/diagnosis-room/transport";

export const consoleBrowserSessionQueryKey = [
  "console",
  "browser-session",
] as const;

export function consoleBrowserSessionResult(
  status: DiagnosisBrowserSessionStatus,
): ApiResult<DiagnosisBrowserSessionStatus> {
  return { data: status, ok: true };
}

export function isConsoleBrowserSessionQueryKey(
  queryKey: readonly unknown[],
): boolean {
  return (
    queryKey.length === consoleBrowserSessionQueryKey.length &&
    queryKey.every(
      (value, index) => value === consoleBrowserSessionQueryKey[index],
    )
  );
}

export async function clearConsoleQueryCacheAfterSignOut(
  queryClient: QueryClient,
) {
  await queryClient.cancelQueries();
  queryClient.removeQueries({
    predicate: (query) =>
      !isConsoleBrowserSessionQueryKey(query.queryKey),
  });
  queryClient.setQueryData(
    consoleBrowserSessionQueryKey,
    consoleBrowserSessionResult({ authenticated: false }),
  );
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
