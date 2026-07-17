import type { useTranslations } from "next-intl";

export type DiagnosisRoomStatusTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.status">
>;

const statusKeys = {
  accepted: "accepted",
  already_delivered: "already_delivered",
  attention: "attention",
  available: "available",
  blocked: "blocked",
  canceled: "cancelled",
  cancelled: "cancelled",
  closed: "closed",
  complete: "complete",
  completed: "complete",
  collected: "collected",
  connected: "connected",
  connecting: "connecting",
  continued_as_new: "continued_as_new",
  critical: "critical",
  delivered: "delivered",
  declined: "declined",
  done: "done",
  error: "error",
  failed: "failed",
  final: "final",
  firing: "firing",
  high: "high",
  info: "info",
  in_progress: "in_progress",
  improved: "improved",
  idle: "idle",
  low: "low",
  medium: "medium",
  missing: "missing",
  needs_evidence: "needs_evidence",
  not_needed: "not_needed",
  not_found: "not_found",
  open: "open",
  partial: "partial",
  pending: "pending",
  queued: "queued",
  ready: "ready",
  ready_for_review: "ready_for_review",
  review: "review",
  reviewed: "reviewed",
  resolved: "resolved",
  running: "running",
  sent: "sent",
  skipped: "skipped",
  stable: "stable",
  submitted: "submitted",
  success: "success",
  succeeded: "succeeded",
  terminated: "terminated",
  timed_out: "timed_out",
  ticketing: "ticketing",
  unproven: "unproven",
  unknown: "unknown",
  unsupported: "unsupported",
  warning: "warning",
} as const;

export function localizeDiagnosisRoomStatus(
  value: string,
  t: DiagnosisRoomStatusTranslator,
): string {
  const normalized = value.trim().toLowerCase();
  const key = statusKeys[normalized as keyof typeof statusKeys];
  return key === undefined ? value : t(key);
}
