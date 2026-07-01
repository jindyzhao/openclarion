"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
  NotificationChannelTestResult,
  NotificationChannelTestContentKind,
  NotificationChannelProfileWriteRequest
} from "./types";

export async function refreshNotificationChannelProfiles(): Promise<ApiResult<NotificationChannelProfileListResponse>> {
  return requestSameOriginJSON<NotificationChannelProfileListResponse>("/api/config/notification-channels");
}

export async function submitNotificationChannelProfile(
  channelID: number | null,
  body: NotificationChannelProfileWriteRequest
): Promise<ApiResult<NotificationChannelProfile>> {
  if (channelID === null) {
    return requestSameOriginJSON<NotificationChannelProfile>("/api/config/notification-channels", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<NotificationChannelProfile>(`/api/config/notification-channels/${channelID}`, {
    method: "PUT",
    body
  });
}

export async function testNotificationChannel(
  channelID: number,
  contentKind?: NotificationChannelTestContentKind
): Promise<ApiResult<NotificationChannelTestResult>> {
  const query = contentKind === undefined ? "" : `?content_kind=${encodeURIComponent(contentKind)}`;
  return requestSameOriginJSON<NotificationChannelTestResult>(`/api/config/notification-channels/${channelID}/test${query}`, {
    method: "POST"
  });
}
