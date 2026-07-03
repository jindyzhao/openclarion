import type { DiagnosisReviewReturnState } from "@/features/diagnosis-room/report-return";

import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";

export type ReportDiagnosisReviewReturnNotice = {
  detail: string;
  title: string;
};

export function reportDiagnosisReviewReturnNotice(
  state: Exclude<DiagnosisReviewReturnState, "none">,
  finalNotificationReadiness: ReportFinalNotificationReadiness,
): ReportDiagnosisReviewReturnNotice {
  if (state === "reviewed") {
    return {
      detail:
        "Latest report data has been loaded. Check Diagnosis Readiness and Evidence Traceability before confirming the final report.",
      title: "Diagnosis evidence review returned",
    };
  }

  if (finalNotificationReadiness.ready) {
    return {
      detail:
        "Latest report data has been loaded. Report Delivery Proof can send the final report notification.",
      title: "Diagnosis conclusion confirmed",
    };
  }

  return {
    detail: `Latest report data has been loaded. Final notification remains blocked: ${finalNotificationReadiness.detail}`,
    title: "Diagnosis conclusion confirmed",
  };
}
