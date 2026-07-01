import { NextResponse } from "next/server";

import { triggerReportReplay } from "@/features/alerts/api";
import type { ReportReplayTriggerRequest } from "@/features/alerts/api";
import { normalizedReportReplayTriggerResponse } from "@/features/report-replay/replay-response";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

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

  const body = await readRequestJSON<ReportReplayTriggerRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const result = await triggerReportReplay(body.data, {
    headers: { authorization },
  });
  if (!result.ok) {
    const response = apiResultResponse(result);
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      result.error.status,
    );
    return response;
  }
  const replay = normalizedReportReplayTriggerResponse(result.data);
  if (replay === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "Report replay response is invalid" },
      { status: 502 },
    );
  }
  return NextResponse.json(replay, { status: 202 });
}
