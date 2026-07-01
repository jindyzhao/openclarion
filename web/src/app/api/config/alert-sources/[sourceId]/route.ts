import { replaceAlertSourceProfile } from "@/features/settings/alert-sources/api";
import type { AlertSourceProfileWriteRequest } from "@/features/settings/alert-sources/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ sourceId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { sourceId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(sourceId, "Alert source profile ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<AlertSourceProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    replaceAlertSourceProfile(parsedID.data, body.data, { headers }),
  );
}
