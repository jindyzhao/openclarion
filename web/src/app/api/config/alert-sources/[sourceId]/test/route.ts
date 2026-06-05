import { testAlertSourceProfileConnection } from "@/features/settings/alert-sources/api";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ sourceId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { sourceId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(sourceId, "Alert source profile ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return apiResultResponse(await testAlertSourceProfileConnection(parsedID.data));
}
