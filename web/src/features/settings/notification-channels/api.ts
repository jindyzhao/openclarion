import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  NotificationChannelProfile,
  NotificationChannelProfileListResponse,
  NotificationChannelTestResult,
  NotificationChannelTestContentKind,
  NotificationChannelProfileWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchNotificationChannelProfiles(
  options: BackendRequestOptions = {}
): Promise<ApiResult<NotificationChannelProfileListResponse>> {
  return requestJSON<NotificationChannelProfileListResponse>("/api/v1/config/notification-channels?limit=100", {
    headers: options.headers
  });
}

export async function createNotificationChannelProfile(
  body: NotificationChannelProfileWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<NotificationChannelProfile>> {
  return requestJSON<NotificationChannelProfile>("/api/v1/config/notification-channels", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceNotificationChannelProfile(
  channelID: number,
  body: NotificationChannelProfileWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<NotificationChannelProfile>> {
  if (!positiveChannelID(channelID)) {
    return { ok: false, error: { message: "Notification channel ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<NotificationChannelProfile>(`/api/v1/config/notification-channels/${channelID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function testNotificationChannelProfile(
  channelID: number,
  contentKind?: string,
  options: BackendRequestOptions = {}
): Promise<ApiResult<NotificationChannelTestResult>> {
  if (!positiveChannelID(channelID)) {
    return { ok: false, error: { message: "Notification channel ID must be a positive integer.", status: 400 } };
  }
  const parsedContentKind = parseNotificationChannelTestContentKind(contentKind);
  if (!parsedContentKind.ok) {
    return parsedContentKind;
  }
  const query = parsedContentKind.data === undefined ? "" : `?content_kind=${encodeURIComponent(parsedContentKind.data)}`;
  return requestJSON<NotificationChannelTestResult>(`/api/v1/config/notification-channels/${channelID}/test${query}`, {
    method: "POST",
    headers: options.headers
  });
}

function positiveChannelID(channelID: number): boolean {
  return Number.isSafeInteger(channelID) && channelID > 0;
}

function parseNotificationChannelTestContentKind(
  value: string | undefined
): ApiResult<NotificationChannelTestContentKind | undefined> {
  const trimmed = value?.trim();
  if (trimmed === undefined || trimmed === "") {
    return { ok: true, data: undefined };
  }
  switch (trimmed) {
    case "transport_sample":
    case "ai_diagnosis_sample":
    case "diagnosis_close_sample":
      return { ok: true, data: trimmed };
    default:
      return {
        ok: false,
        error: {
          message: "Notification channel test content kind is invalid.",
          status: 400
        }
      };
  }
}
