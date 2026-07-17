export function diagnosisCloseRoomBlockReason({
  actorSubject,
  closeInFlight,
  confirmInFlight,
  connected,
  messages,
  rbacBlockReason,
  state,
  turnInFlight,
}: {
  actorSubject: string;
  closeInFlight: boolean;
  confirmInFlight: boolean;
  connected: boolean;
  messages: {
    alreadyClosed: string;
    closeProcessing: string;
    connectRequired: string;
    identityRequired: string;
    refreshRequired: string;
    waitForConfirmation: string;
    waitForTurn: string;
  };
  rbacBlockReason: string;
  state: { status: string } | null;
  turnInFlight: boolean;
}): string {
  if (closeInFlight) {
    return messages.closeProcessing;
  }
  if (confirmInFlight) {
    return messages.waitForConfirmation;
  }
  if (!connected) {
    return messages.connectRequired;
  }
  if (state === null) {
    return messages.refreshRequired;
  }
  if (state.status === "closed") {
    return messages.alreadyClosed;
  }
  if (turnInFlight) {
    return messages.waitForTurn;
  }
  if (actorSubject.trim() === "") {
    return messages.identityRequired;
  }
  return rbacBlockReason;
}
