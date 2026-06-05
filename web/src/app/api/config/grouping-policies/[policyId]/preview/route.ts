import { previewGroupingPolicy } from "@/features/settings/grouping-policies/api";
import { apiResultResponse } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ policyId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { policyId } = await context.params;
  const parsedID = Number.parseInt(policyId, 10);
  if (!Number.isSafeInteger(parsedID) || parsedID < 1) {
    return apiResultResponse({
      ok: false,
      error: { message: "Grouping policy ID must be a positive integer.", status: 400 }
    });
  }

  return apiResultResponse(await previewGroupingPolicy(parsedID));
}
