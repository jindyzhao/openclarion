import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

type DiagnosisRoomCloseUnavailableRequest =
  components["schemas"]["DiagnosisRoomCloseUnavailableRequest"];
type DiagnosisRoomSummary = components["schemas"]["DiagnosisRoomSummary"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ sessionId: string }>;
};

export async function POST(request: Request, context: RouteContext) {
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

  const body = await readOptionalCloseUnavailableBody(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const result = await requestJSON<DiagnosisRoomSummary>(
    `/api/v1/diagnosis/rooms/${encodeURIComponent(sessionID)}/close-unavailable`,
    {
      method: "POST",
      headers: { authorization },
      body: body.data,
    },
  );
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

async function readOptionalCloseUnavailableBody(request: Request) {
  let raw: string;
  try {
    raw = await request.text();
  } catch (error) {
    return {
      ok: false as const,
      error: {
        message:
          error instanceof Error ? error.message : "Request body is invalid.",
        status: 400,
      },
    };
  }
  if (raw.trim() === "") {
    return {
      ok: true as const,
      data: { reason: "workflow_unavailable" } satisfies DiagnosisRoomCloseUnavailableRequest,
    };
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!isCloseUnavailableRequest(parsed)) {
      return {
        ok: false as const,
        error: {
          message: "Request body must be an object with optional string reason.",
          status: 400,
        },
      };
    }
    return {
      ok: true as const,
      data: parsed,
    };
  } catch (error) {
    return {
      ok: false as const,
      error: {
        message:
          error instanceof Error ? error.message : "Request body must be valid JSON.",
        status: 400,
      },
    };
  }
}

function isCloseUnavailableRequest(
  value: unknown,
): value is DiagnosisRoomCloseUnavailableRequest {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const body = value as Record<string, unknown>;
  const keys = Object.keys(body);
  if (keys.some((key) => key !== "reason")) {
    return false;
  }
  return body.reason === undefined || typeof body.reason === "string";
}
