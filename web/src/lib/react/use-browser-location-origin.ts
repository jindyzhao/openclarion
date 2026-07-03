import { useSyncExternalStore } from "react";

export function useBrowserLocationOrigin(): string | null {
  return useSyncExternalStore(
    subscribeBrowserLocationOrigin,
    getBrowserLocationOriginSnapshot,
    getServerBrowserLocationOriginSnapshot,
  );
}

function subscribeBrowserLocationOrigin() {
  return () => {};
}

function getBrowserLocationOriginSnapshot(): string | null {
  return typeof window === "undefined" ? null : window.location.origin;
}

function getServerBrowserLocationOriginSnapshot(): string | null {
  return null;
}
