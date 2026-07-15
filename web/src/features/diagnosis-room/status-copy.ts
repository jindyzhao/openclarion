import type { useTranslations } from "next-intl";

export type DiagnosisRoomStatusTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.status">
>;

const statusKeys = {
  accepted: "accepted",
  available: "available",
  canceled: "cancelled",
  cancelled: "cancelled",
  closed: "closed",
  complete: "complete",
  completed: "complete",
  continued_as_new: "continued_as_new",
  delivered: "delivered",
  error: "error",
  failed: "failed",
  high: "high",
  in_progress: "in_progress",
  low: "low",
  medium: "medium",
  needs_evidence: "needs_evidence",
  not_found: "not_found",
  open: "open",
  partial: "partial",
  pending: "pending",
  queued: "queued",
  ready_for_review: "ready_for_review",
  running: "running",
  sent: "sent",
  success: "success",
  succeeded: "succeeded",
  terminated: "terminated",
  timed_out: "timed_out",
  unknown: "unknown",
} as const;

export function localizeDiagnosisRoomStatus(
  value: string,
  t: DiagnosisRoomStatusTranslator,
): string {
  const normalized = value.trim().toLowerCase();
  const key = statusKeys[normalized as keyof typeof statusKeys];
  return key === undefined ? value : t(key);
}
