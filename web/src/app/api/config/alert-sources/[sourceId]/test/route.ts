import { testAlertSourceProfileConnection } from "@/features/settings/alert-sources/api";
import { apiResultResponse } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ sourceId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { sourceId } = await context.params;
  const parsedID = Number.parseInt(sourceId, 10);
  if (!Number.isSafeInteger(parsedID) || parsedID < 1) {
    return apiResultResponse({
      ok: false,
      error: { message: "Alert source profile ID must be a positive integer.", status: 400 }
    });
  }

  return apiResultResponse(await testAlertSourceProfileConnection(parsedID));
}
