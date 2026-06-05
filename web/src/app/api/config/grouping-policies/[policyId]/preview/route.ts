import { previewGroupingPolicy } from "@/features/settings/grouping-policies/api";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Grouping policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return apiResultResponse(await previewGroupingPolicy(parsedID.data));
}
