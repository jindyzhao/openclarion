import { enableReportWorkflowPolicy } from "@/features/settings/report-workflow-policies/api";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Report workflow policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return apiResultResponse(await enableReportWorkflowPolicy(parsedID.data));
}
