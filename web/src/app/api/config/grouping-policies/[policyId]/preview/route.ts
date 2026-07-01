import { previewGroupingPolicy } from "@/features/settings/grouping-policies/api";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Grouping policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return authorizedBackendResultResponse(request, (headers) =>
    previewGroupingPolicy(parsedID.data, { headers }),
  );
}
