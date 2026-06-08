import { replaceNotificationChannelProfile } from "@/features/settings/notification-channels/api";
import type { NotificationChannelProfileWriteRequest } from "@/features/settings/notification-channels/types";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

type RouteContext = {
  params: Promise<{ channelId: string }>;
};

export async function PUT(request: Request, context: RouteContext) {
  const { channelId } = await context.params;
  const channelID = parsePositiveIntegerRouteParam(channelId, "Notification channel ID");
  if (!channelID.ok) {
    return apiResultResponse(channelID);
  }

  const body = await readRequestJSON<NotificationChannelProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await replaceNotificationChannelProfile(channelID.data, body.data));
}
