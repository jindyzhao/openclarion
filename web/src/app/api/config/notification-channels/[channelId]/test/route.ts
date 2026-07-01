import { testNotificationChannelProfile } from "@/features/settings/notification-channels/api";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ channelId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { channelId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(channelId, "Notification channel ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const contentKind = new URL(request.url).searchParams.get("content_kind") ?? undefined;
  return authorizedBackendResultResponse(request, (headers) =>
    testNotificationChannelProfile(parsedID.data, contentKind, { headers }),
  );
}
