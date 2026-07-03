import { NextResponse } from "next/server";

import { fetchDiagnosisRoom } from "@/features/diagnosis-room/api";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ sessionId: string }>;
};

export async function GET(request: Request, context: RouteContext) {
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    const response = NextResponse.json<ErrorResponse>(
      { error: "authorization is required" },
      { status: 401 },
    );
    expireDiagnosisSessionCookieOnAuthFailure(response, request, 401);
    return response;
  }
  const { sessionId } = await context.params;
  const sessionID = sessionId.trim();
  if (sessionID === "") {
    return NextResponse.json<ErrorResponse>(
      { error: "session_id is required" },
      { status: 400 },
    );
  }

  const result = await fetchDiagnosisRoom(sessionID, {
    headers: { authorization },
  });
  const response = apiResultResponse(result);
  if (!result.ok) {
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      result.error.status,
    );
  }
  return response;
}
