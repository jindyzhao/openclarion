import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import { normalizedDiagnosisAuthSessionResponse } from "@/lib/api/diagnosis-auth-response";
import {
  diagnosisOIDCConfigFromEnv,
  diagnosisOIDCDiscoveryURL,
  diagnosisOIDCIssuerMatches,
  diagnosisOIDCReturnToWithSessionContext,
  diagnosisOIDCStateFromRequest,
  diagnosisOIDCStateSigningKey,
  expireDiagnosisOIDCStateCookie,
  normalizedDiagnosisOIDCDiscovery,
  normalizedOIDCTokenResponse,
  oidcCallbackQueryToken,
  oidcIDTokenNonce,
  oidcTokenEndpointRequest,
  unsealDiagnosisOIDCStatePayload,
  type DiagnosisOIDCTokenResponse
} from "@/lib/api/diagnosis-oidc-login";
import {
  diagnosisRequestPublicOrigin,
  expireDiagnosisSessionCookie,
  setDiagnosisSessionCookie
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

const oidcCallbackCodeMaxBytes = 2048;
const oidcCallbackStateMaxBytes = 512;

export async function GET(request: Request) {
  const requestURL = new URL(request.url);
  const stateSigningKey = diagnosisOIDCStateSigningKey();
  const sealedState =
    stateSigningKey === null
      ? null
      : unsealDiagnosisOIDCStatePayload(diagnosisOIDCStateFromRequest(request), stateSigningKey);
  const fallbackReturnTo = sealedState?.returnTo ?? "/diagnosis-room?auth_mode=session";

  if (stateSigningKey === null || sealedState === null) {
    return oidcCallbackFailureRedirect(request, fallbackReturnTo, "oidc_callback_missing");
  }
  if (requestURL.searchParams.has("error")) {
    return oidcCallbackFailureRedirect(request, sealedState.returnTo, "oidc_auth_failed");
  }
  const code = oidcCallbackQueryToken(requestURL.searchParams, "code", oidcCallbackCodeMaxBytes);
  const state = oidcCallbackQueryToken(requestURL.searchParams, "state", oidcCallbackStateMaxBytes);
  if (code === null || state === null || state !== sealedState.state) {
    return oidcCallbackFailureRedirect(request, sealedState.returnTo, "oidc_callback_missing");
  }

  const config = diagnosisOIDCConfigFromEnv(request);
  if (config === null) {
    return oidcCallbackFailureRedirect(request, sealedState.returnTo, "oidc_not_configured");
  }

  const token = await exchangeOIDCCodeForIDToken({ code, config, payload: sealedState });
  if (token === null || oidcIDTokenNonce(token.idToken) !== sealedState.nonce) {
    return oidcCallbackFailureRedirect(request, sealedState.returnTo, "oidc_callback_failed");
  }

  const sessionResult = await requestJSON<unknown>("/api/v1/diagnosis/auth/session", {
    method: "POST",
    headers: { authorization: `Bearer ${token.idToken}` }
  });
  if (!sessionResult.ok) {
    return oidcCallbackFailureRedirect(request, sealedState.returnTo, oidcCallbackFailureError(sessionResult.error.status));
  }
  const session = normalizedDiagnosisAuthSessionResponse(sessionResult.data);
  if (session === null) {
    return NextResponse.json<ErrorResponse>({ error: "OIDC session response is invalid" }, { status: 502 });
  }
  const expires = new Date(session.expires_at);
  if (expires.getTime() <= Date.now()) {
    return NextResponse.json<ErrorResponse>({ error: "OIDC session response is invalid" }, { status: 502 });
  }

  const destination = new URL(
    diagnosisOIDCReturnToWithSessionContext(sealedState.returnTo),
    diagnosisRequestPublicOrigin(request)
  );
  const response = NextResponse.redirect(destination);
  expireDiagnosisOIDCStateCookie(response, request);
  setDiagnosisSessionCookie(response, request, session.token, expires);
  return response;
}

async function exchangeOIDCCodeForIDToken({
  code,
  config,
  payload
}: {
  code: string;
  config: NonNullable<ReturnType<typeof diagnosisOIDCConfigFromEnv>>;
  payload: NonNullable<ReturnType<typeof unsealDiagnosisOIDCStatePayload>>;
}): Promise<DiagnosisOIDCTokenResponse | null> {
  let discoveryResponse: Response;
  try {
    discoveryResponse = await fetch(diagnosisOIDCDiscoveryURL(config.issuer), {
      cache: "no-store",
      headers: { accept: "application/json" }
    });
  } catch {
    return null;
  }
  if (!discoveryResponse.ok) {
    return null;
  }
  const discovery = normalizedDiagnosisOIDCDiscovery(await discoveryResponse.json().catch(() => null));
  if (discovery === null || !diagnosisOIDCIssuerMatches(discovery.issuer, config.issuer)) {
    return null;
  }

  let tokenResponse: Response;
  try {
    tokenResponse = await fetch(
      discovery.tokenEndpoint,
      oidcTokenEndpointRequest({
        code,
        config,
        discovery,
        payload
      })
    );
  } catch {
    return null;
  }
  if (!tokenResponse.ok) {
    return null;
  }
  return normalizedOIDCTokenResponse(await tokenResponse.json().catch(() => null));
}

function oidcCallbackFailureRedirect(
  request: Request,
  returnTo: string,
  error: "oidc_auth_failed" | "oidc_callback_failed" | "oidc_callback_missing" | "oidc_not_configured" | "oidc_session_rejected"
): NextResponse {
  const destination = new URL(diagnosisOIDCReturnToWithSessionContext(returnTo), diagnosisRequestPublicOrigin(request));
  destination.searchParams.set("oidc_auth_error", error);
  const response = NextResponse.redirect(destination);
  expireDiagnosisSessionCookie(response, request);
  return response;
}

function oidcCallbackFailureError(status: number | undefined): "oidc_callback_failed" | "oidc_session_rejected" {
  return status === 401 || status === 403 ? "oidc_session_rejected" : "oidc_callback_failed";
}
