import { triggerReportWorkflowPolicyReplay } from "@/features/settings/report-workflow-policies/api";
import type { ReportWorkflowPolicyReplayRequest } from "@/features/settings/report-workflow-policies/types";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

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

  return apiResultResponse(await triggerReportWorkflowPolicyReplay(parsedID.data, body.data), 202);
}
