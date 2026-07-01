import { NextResponse } from "next/server";

import { fetchDiagnosisHandoffs } from "@/features/diagnosis-room/api";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

export const dynamic = "force-dynamic";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export async function GET(request: Request) {
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    const response = NextResponse.json<ErrorResponse>(
      { error: "authorization is required" },
      { status: 401 },
    );
    expireDiagnosisSessionCookieOnAuthFailure(response, request, 401);
    return response;
  }

  const limit = parseDiagnosisHandoffLimit(request);
  if (!limit.ok) {
    return apiResultResponse(limit);
  }
  const result = await fetchDiagnosisHandoffs(limit.data, {
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

function parseDiagnosisHandoffLimit(request: Request) {
  const raw = new URL(request.url).searchParams.get("limit");
  if (raw === null || raw.trim() === "") {
    return { ok: true as const, data: 20 };
  }
  if (!/^[0-9]+$/.test(raw.trim())) {
    return {
      ok: false as const,
      error: { message: "limit must be a positive integer.", status: 400 },
    };
  }
  const limit = Number(raw);
  if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100) {
    return {
      ok: false as const,
      error: { message: "limit must be between 1 and 100.", status: 400 },
    };
  }
  return { ok: true as const, data: limit };
}
