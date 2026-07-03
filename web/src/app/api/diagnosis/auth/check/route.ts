import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import { normalizedDiagnosisAuthCheckResponse } from "@/lib/api/diagnosis-auth-response";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    const response = NextResponse.json<ErrorResponse>(
      { error: "authorization is required" },
      { status: 401 },
    );
    expireDiagnosisSessionCookieOnAuthFailure(response, request, 401);
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
    const response = apiResultResponse(result);
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      result.error.status,
    );
    return response;
  }
  const authCheck = normalizedDiagnosisAuthCheckResponse(result.data);
  if (authCheck === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis auth check response is invalid" },
      { status: 502 },
    );
  }
  return NextResponse.json(authCheck);
}
