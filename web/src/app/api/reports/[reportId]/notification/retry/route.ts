import { NextResponse } from "next/server";

import { retryReportNotification } from "@/features/reports/api";
import type { ReportNotificationRetryRequest } from "@/features/reports/types";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ reportId: string }>;
};

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

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

  const { reportId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(reportId, "Report ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<ReportNotificationRetryRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const result = await retryReportNotification(parsedID.data, body.data, {
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
