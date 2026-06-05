import { replaceAlertSourceProfile } from "@/features/settings/alert-sources/api";
import type { AlertSourceProfileWriteRequest } from "@/features/settings/alert-sources/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ sourceId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { sourceId } = await context.params;
  const parsedID = Number.parseInt(sourceId, 10);
  if (!Number.isSafeInteger(parsedID) || parsedID < 1) {
    return apiResultResponse({
      ok: false,
      error: { message: "Alert source profile ID must be a positive integer.", status: 400 }
    });
  }

  const body = await readRequestJSON<AlertSourceProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await replaceAlertSourceProfile(parsedID, body.data));
}
