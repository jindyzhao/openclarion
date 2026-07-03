import { NextResponse } from "next/server";

import {
  retryDiagnosisRoomNotification,
  type DiagnosisNotificationRetryRequest,
} from "@/features/diagnosis-room/api";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

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

  const body = await readRequestJSON<unknown>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  if (!isDiagnosisNotificationRetryRequest(body.data)) {
    return NextResponse.json<ErrorResponse>(
      {
        error:
          "Request body must be an object with a supported string event_kind.",
      },
      { status: 400 },
    );
  }

  const result = await retryDiagnosisRoomNotification(sessionID, body.data, {
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

function isDiagnosisNotificationRetryRequest(
  value: unknown,
): value is DiagnosisNotificationRetryRequest {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const body = value as Record<string, unknown>;
  const keys = Object.keys(body);
  if (keys.length !== 1 || keys[0] !== "event_kind") {
    return false;
  }
  switch (body.event_kind) {
    case "diagnosis_room.assistant_turn_notification_sent":
    case "diagnosis_room.final_ready_notification_sent":
    case "diagnosis_room.close_notification_sent":
      return true;
    default:
      return false;
  }
}
