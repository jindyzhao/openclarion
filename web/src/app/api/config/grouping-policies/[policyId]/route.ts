import { replaceGroupingPolicy } from "@/features/settings/grouping-policies/api";
import type { GroupingPolicyWriteRequest } from "@/features/settings/grouping-policies/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(policyId, "Grouping policy ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<GroupingPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    replaceGroupingPolicy(parsedID.data, body.data, { headers }),
  );
}
