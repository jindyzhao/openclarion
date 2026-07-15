import type { useTranslations } from "next-intl";

type StatusTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.status">
>;

const statusKeys = {
  accepted: "accepted",
  canceled: "cancelled",
  cancelled: "cancelled",
  closed: "closed",
  complete: "complete",
  delivered: "delivered",
  error: "error",
  failed: "failed",
  high: "high",
  low: "low",
  medium: "medium",
  needs_evidence: "needs_evidence",
  open: "open",
  partial: "partial",
  pending: "pending",
  queued: "queued",
  ready_for_review: "ready_for_review",
  running: "running",
  sent: "sent",
  success: "success",
  succeeded: "succeeded",
} as const;

export function localizeDiagnosisRoomStatus(
  value: string,
  t: StatusTranslator,
): string {
  const normalized = value.trim().toLowerCase();
  const key = statusKeys[normalized as keyof typeof statusKeys];
  return key === undefined ? value : t(key);
}
