export function diagnosisCloseRoomBlockReason({
  actorSubject,
  closeInFlight,
  confirmInFlight,
  connected,
  rbacBlockReason,
  state,
  turnInFlight,
  locale = "en",
}: {
  actorSubject: string;
  closeInFlight: boolean;
  confirmInFlight: boolean;
  connected: boolean;
  rbacBlockReason: string;
  state: { status: string } | null;
  turnInFlight: boolean;
  locale?: string;
}): string {
  if (closeInFlight) {
    return roomCloseText(
      locale,
      "The diagnosis room close request is still processing.",
      "诊断室关闭请求仍在处理中。",
    );
  }
  if (confirmInFlight) {
    return roomCloseText(
      locale,
      "Wait for conclusion confirmation to finish before closing the room.",
      "结论确认完成后才能关闭诊断室。",
    );
  }
  if (!connected) {
    return roomCloseText(
      locale,
      "Connect to the diagnosis room before closing it.",
      "关闭前请先连接诊断室。",
    );
  }
  if (state === null) {
    return roomCloseText(
      locale,
      "Refresh the room state before closing it.",
      "关闭前请先刷新诊断室状态。",
    );
  }
  if (state.status === "closed") {
    return roomCloseText(
      locale,
      "This diagnosis room is already closed.",
      "当前诊断室已关闭。",
    );
  }
  if (turnInFlight) {
    return roomCloseText(
      locale,
      "Wait for the current diagnosis turn to finish before closing the room.",
      "当前诊断轮次完成后才能关闭诊断室。",
    );
  }
  if (actorSubject.trim() === "") {
    return roomCloseText(
      locale,
      "An authenticated operator identity is required to close the room.",
      "关闭诊断室需要已认证的操作员身份。",
    );
  }
  return rbacBlockReason;
}

function roomCloseText(
  locale: string,
  english: string,
  chinese: string,
): string {
  return locale === "zh-CN" ? chinese : english;
}
