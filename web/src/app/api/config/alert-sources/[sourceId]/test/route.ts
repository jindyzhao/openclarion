import { testAlertSourceProfileConnection } from "@/features/settings/alert-sources/api";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ sourceId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { sourceId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(sourceId, "Alert source profile ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return authorizedBackendResultResponse(request, (headers) =>
    testAlertSourceProfileConnection(parsedID.data, { headers }),
  );
}
