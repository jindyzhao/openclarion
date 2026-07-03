import { NextResponse } from "next/server";

import { normalizedReportReplayTriggerResponse } from "@/features/report-replay/replay-response";
import { triggerReportWorkflowPolicyReplay } from "@/features/settings/report-workflow-policies/api";
import type { ReportWorkflowPolicyReplayRequest } from "@/features/settings/report-workflow-policies/types";
import type { components } from "@/lib/api/openapi";
import {
  diagnosisAuthorizationHeaders,
  protectedApiResultResponse,
} from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type ErrorResponse = components["schemas"]["ErrorResponse"];

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Report workflow policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<ReportWorkflowPolicyReplayRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return apiResultResponse(headers);
  }
  const result = await triggerReportWorkflowPolicyReplay(parsedID.data, body.data, {
    headers: headers.data,
  });
  if (!result.ok) {
    return protectedApiResultResponse(request, result);
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
