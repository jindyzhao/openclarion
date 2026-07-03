import { replaceReportWorkflowPolicy } from "@/features/settings/report-workflow-policies/api";
import type { ReportWorkflowPolicyWriteRequest } from "@/features/settings/report-workflow-policies/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Report workflow policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<ReportWorkflowPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    replaceReportWorkflowPolicy(parsedID.data, body.data, { headers }),
  );
}
