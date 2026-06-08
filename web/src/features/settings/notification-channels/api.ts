import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
  NotificationChannelTestResult,
  NotificationChannelProfileWriteRequest
} from "./types";

export async function fetchNotificationChannelProfiles(): Promise<ApiResult<NotificationChannelProfileListResponse>> {
  return requestJSON<NotificationChannelProfileListResponse>("/api/v1/config/notification-channels?limit=100");
}

export async function createNotificationChannelProfile(
  body: NotificationChannelProfileWriteRequest
): Promise<ApiResult<NotificationChannelProfile>> {
  return requestJSON<NotificationChannelProfile>("/api/v1/config/notification-channels", {
    method: "POST",
    body
  });
}

export async function replaceNotificationChannelProfile(
  channelID: number,
  body: NotificationChannelProfileWriteRequest
): Promise<ApiResult<NotificationChannelProfile>> {
  if (!positiveChannelID(channelID)) {
    return { ok: false, error: { message: "Notification channel ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<NotificationChannelProfile>(`/api/v1/config/notification-channels/${channelID}`, {
    method: "PUT",
    body
  });
}

export async function testNotificationChannelProfile(
  channelID: number
): Promise<ApiResult<NotificationChannelTestResult>> {
  if (!positiveChannelID(channelID)) {
    return { ok: false, error: { message: "Notification channel ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<NotificationChannelTestResult>(`/api/v1/config/notification-channels/${channelID}/test`, {
    method: "POST"
  });
}

function positiveChannelID(channelID: number): boolean {
  return Number.isSafeInteger(channelID) && channelID > 0;
}
