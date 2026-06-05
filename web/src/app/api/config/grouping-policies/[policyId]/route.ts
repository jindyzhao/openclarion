import { replaceGroupingPolicy } from "@/features/settings/grouping-policies/api";
import type { GroupingPolicyWriteRequest } from "@/features/settings/grouping-policies/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = Number.parseInt(policyId, 10);
  if (!Number.isSafeInteger(parsedID) || parsedID < 1) {
    return apiResultResponse({
      ok: false,
      error: { message: "Grouping policy ID must be a positive integer.", status: 400 }
    });
  }

  const body = await readRequestJSON<GroupingPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await replaceGroupingPolicy(parsedID, body.data));
}
