import {
  createNotificationChannelProfile,
  fetchNotificationChannelProfiles
} from "@/features/settings/notification-channels/api";
import type { NotificationChannelProfileWriteRequest } from "@/features/settings/notification-channels/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchNotificationChannelProfiles());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<NotificationChannelProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createNotificationChannelProfile(body.data), 201);
}
