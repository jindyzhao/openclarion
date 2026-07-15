import type { DiagnosisReviewReturnState } from "@/features/diagnosis-room/report-return";

import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";

export type ReportDiagnosisReviewReturnNoticeKind =
  | "confirmed_blocked"
  | "confirmed_ready"
  | "reviewed";

export function reportDiagnosisReviewReturnNotice(
  state: Exclude<DiagnosisReviewReturnState, "none">,
  finalNotificationReadiness: ReportFinalNotificationReadiness,
): ReportDiagnosisReviewReturnNoticeKind {
  if (state === "reviewed") {
    return "reviewed";
  }

  if (finalNotificationReadiness.ready) {
    return "confirmed_ready";
  }

  return "confirmed_blocked";
}
