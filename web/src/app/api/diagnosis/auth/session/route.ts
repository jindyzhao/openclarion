import { NextResponse } from "next/server";

import { normalizeForwardedAuthorization } from "@/lib/api/authorization";
import { requestJSON } from "@/lib/api/client";
import {
  diagnosisAuthorizationFromRequest,
  diagnosisRequestHasSessionCookie,
  expireDiagnosisSessionCookie,
  setDiagnosisSessionCookie,
} from "@/lib/api/diagnosis-session";
import {
  normalizedDiagnosisAuthSessionResponse,
  normalizedDiagnosisAuthCheckResponse,
  type DiagnosisAuthCheckResponse,
} from "@/lib/api/diagnosis-auth-response";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

type ErrorResponse = components["schemas"]["ErrorResponse"];

type DiagnosisSessionStatusResponse =
  | {
      authenticated: true;
      checked_at: string;
      mode: DiagnosisAuthCheckResponse["mode"];
      role_authorized: boolean;
      roles: string[];
      subject: string;
    }
  | {
      authenticated: false;
    };

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const authorization = normalizeForwardedAuthorization(
    request.headers.get("authorization") ?? "",
  );
  if (authorization === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "authorization is required" },
      { status: 401 },
    );
  }
  const result = await requestJSON<unknown>(
    "/api/v1/diagnosis/auth/session",
    {
      method: "POST",
      headers: { authorization },
    },
  );
  if (!result.ok) {
    return apiResultResponse(result);
  }
  const session = normalizedDiagnosisAuthSessionResponse(result.data);
  if (session === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis browser session response is invalid" },
      { status: 502 },
    );
  }
  const expires = new Date(session.expires_at);
  if (expires.getTime() <= Date.now()) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis browser session response is invalid" },
      { status: 502 },
    );
  }
  const response = NextResponse.json<DiagnosisSessionStatusResponse>(
    {
      authenticated: true,
      checked_at: session.checked_at,
      mode: session.mode,
      role_authorized: session.role_authorized,
      roles: session.roles,
      subject: session.subject,
    },
    { status: 201 },
  );
  setDiagnosisSessionCookie(response, request, session.token, expires);
  return response;
}

export async function GET(request: Request) {
  if (!diagnosisRequestHasSessionCookie(request)) {
    return NextResponse.json<DiagnosisSessionStatusResponse>({
      authenticated: false,
    });
  }
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    const response = NextResponse.json<DiagnosisSessionStatusResponse>({
      authenticated: false,
    });
    expireDiagnosisSessionCookie(response, request);
    return response;
  }
  const result = await requestJSON<unknown>(
    "/api/v1/diagnosis/auth/check",
    {
      method: "POST",
      headers: { authorization },
    },
  );
  if (!result.ok) {
    if (result.error.status === 401 || result.error.status === 403) {
      const response = NextResponse.json<DiagnosisSessionStatusResponse>({
        authenticated: false,
      });
      expireDiagnosisSessionCookie(response, request);
      return response;
    }
    return apiResultResponse(result);
  }
  const session = normalizedDiagnosisAuthCheckResponse(result.data);
  if (session === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis browser session response is invalid" },
      { status: 502 },
    );
  }
  return NextResponse.json<DiagnosisSessionStatusResponse>({
    authenticated: true,
    checked_at: session.checked_at,
    mode: session.mode,
    role_authorized: session.role_authorized,
    roles: session.roles,
    subject: session.subject,
  });
}

export async function DELETE(request: Request) {
  const response = new NextResponse(null, { status: 204 });
  expireDiagnosisSessionCookie(response, request);
  return response;
}
