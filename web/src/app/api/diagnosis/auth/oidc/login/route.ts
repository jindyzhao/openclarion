import { NextResponse } from "next/server";

import {
  diagnosisOIDCConfigFromEnv,
  diagnosisOIDCDiscoveryURL,
  diagnosisOIDCIssuerMatches,
  diagnosisOIDCLoginURL,
  diagnosisOIDCReturnToWithSessionContext,
  diagnosisOIDCStateSigningKey,
  newDiagnosisOIDCStatePayload,
  normalizedDiagnosisOIDCDiscovery,
  normalizedDiagnosisOIDCReturnTo,
  setDiagnosisOIDCStateCookie
} from "@/lib/api/diagnosis-oidc-login";
import { diagnosisRequestPublicOrigin } from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const normalizedReturnTo = normalizedDiagnosisOIDCReturnTo(requestURL.searchParams.get("return_to"));
  if (normalizedReturnTo === null) {
    return NextResponse.json<ErrorResponse>({ error: "OIDC return path is invalid" }, { status: 400 });
  }
  const returnTo = diagnosisOIDCReturnToWithSessionContext(normalizedReturnTo);
  const config = diagnosisOIDCConfigFromEnv(request);
  const stateSigningKey = diagnosisOIDCStateSigningKey();
  if (config === null || stateSigningKey === null) {
    return oidcLoginFailureRedirect(request, returnTo, "oidc_not_configured");
  }

  let discoveryResponse: Response;
  try {
    discoveryResponse = await fetch(diagnosisOIDCDiscoveryURL(config.issuer), {
      cache: "no-store",
      headers: { accept: "application/json" }
    });
  } catch {
    return oidcLoginFailureRedirect(request, returnTo, "oidc_login_failed");
  }
  if (!discoveryResponse.ok) {
    return oidcLoginFailureRedirect(request, returnTo, "oidc_login_failed");
  }
  const discovery = normalizedDiagnosisOIDCDiscovery(await discoveryResponse.json().catch(() => null));
  if (discovery === null || !diagnosisOIDCIssuerMatches(discovery.issuer, config.issuer)) {
    return oidcLoginFailureRedirect(request, returnTo, "oidc_login_failed");
  }

  const payload = newDiagnosisOIDCStatePayload(returnTo, config.usePKCE);
  const response = NextResponse.redirect(diagnosisOIDCLoginURL({ config, discovery, payload }));
  setDiagnosisOIDCStateCookie(response, request, payload, stateSigningKey);
  return response;
}

function oidcLoginFailureRedirect(
  request: Request,
  returnTo: string,
  error: "oidc_login_failed" | "oidc_not_configured"
): NextResponse {
  const destination = new URL(returnTo, diagnosisRequestPublicOrigin(request));
  destination.searchParams.set("auth_mode", "session");
  destination.searchParams.set("oidc_auth_error", error);
  return NextResponse.redirect(destination);
}
