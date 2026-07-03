import {
  createNotificationChannelProfile,
  fetchNotificationChannelProfiles
} from "@/features/settings/notification-channels/api";
import type { NotificationChannelProfileWriteRequest } from "@/features/settings/notification-channels/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchNotificationChannelProfiles({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<NotificationChannelProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createNotificationChannelProfile(body.data, { headers }),
    201,
  );
}
