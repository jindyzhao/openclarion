"use client";

import { useSyncExternalStore } from "react";

export function useClientReady(): boolean {
  return useSyncExternalStore(subscribeClientReady, getClientReadySnapshot, getServerClientReadySnapshot);
}

function subscribeClientReady() {
  return () => {};
}

function getClientReadySnapshot() {
  return true;
}

function getServerClientReadySnapshot() {
  return false;
}
