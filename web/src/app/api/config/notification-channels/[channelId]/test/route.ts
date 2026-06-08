import { testNotificationChannelProfile } from "@/features/settings/notification-channels/api";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ channelId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(_request: Request, context: RouteContext) {
  const { channelId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(channelId, "Notification channel ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return apiResultResponse(await testNotificationChannelProfile(parsedID.data));
}
