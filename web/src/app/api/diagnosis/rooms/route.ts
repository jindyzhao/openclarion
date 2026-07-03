import { NextResponse } from "next/server";

import { fetchDiagnosisRooms } from "@/features/diagnosis-room/api";
import { requestJSON } from "@/lib/api/client";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type DiagnosisRoomCreateRequest = components["schemas"]["DiagnosisRoomCreateRequest"];
type DiagnosisRoomCreateResponse = components["schemas"]["DiagnosisRoomCreateResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

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
  const limit = parseDiagnosisRoomLimit(request);
  if (!limit.ok) {
    return apiResultResponse(limit);
  }
  const result = await fetchDiagnosisRooms(limit.data, {
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

  const body = await readRequestJSON<DiagnosisRoomCreateRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const result = await requestJSON<DiagnosisRoomCreateResponse>("/api/v1/diagnosis/rooms", {
      method: "POST",
      headers: { authorization },
      body: body.data
    });
  const response = apiResultResponse(result, 201);
  if (!result.ok) {
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      result.error.status,
    );
  }
  return response;
}

function parseDiagnosisRoomLimit(request: Request) {
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
