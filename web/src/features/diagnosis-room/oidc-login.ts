import type { Route } from "next";

export function diagnosisOIDCLoginHref(returnTo: string): Route {
  const params = new URLSearchParams({ return_to: returnTo });
  return `/api/diagnosis/auth/oidc/login?${params.toString()}` as Route;
}
