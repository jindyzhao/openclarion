export const diagnosisRoomToolTemplatesQueryKey = [
  "diagnosis-room",
  "diagnosis-tool-templates",
] as const;

export const diagnosisRoomNotificationChannelsQueryKey = [
  "diagnosis-room",
  "notification-channels",
] as const;

export const diagnosisRoomAuthStatusQueryKey = [
  "diagnosis-room",
  "auth-status",
] as const;

export const diagnosisRoomBrowserSessionQueryKey = [
  "diagnosis-room",
  "browser-session",
] as const;

export const diagnosisHandoffListLimit = 100;
export const diagnosisHandoffListQueryKey = [
  "diagnosis-room",
  "handoffs",
  diagnosisHandoffListLimit,
] as const;

export const diagnosisRoomListLimit = 20;
export const diagnosisRoomListQueryKey = [
  "diagnosis-room",
  "rooms",
  diagnosisRoomListLimit,
] as const;

const diagnosisRoomDetailQueryKeyPrefix = ["diagnosis-room", "room"] as const;

export function diagnosisRoomDetailQueryKey(sessionID: string) {
  return [...diagnosisRoomDetailQueryKeyPrefix, sessionID] as const;
}

export function diagnosisRoomRefreshQueryKeys(sessionID?: string) {
  const trimmedSessionID = sessionID?.trim() ?? "";
  return [
    diagnosisRoomListQueryKey,
    diagnosisHandoffListQueryKey,
    ...(trimmedSessionID === ""
      ? []
      : [diagnosisRoomDetailQueryKey(trimmedSessionID)]),
  ] as const;
}
