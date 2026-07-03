import { describe, expect, it } from "vitest";

import { reportDiagnosisReviewReturnNotice } from "./report-return-notice";
import type { FinalReportDetail } from "./types";

describe("report diagnosis review return notice", () => {
  it("keeps reviewed returns focused on diagnosis readiness", () => {
    expect(
      reportDiagnosisReviewReturnNotice("reviewed", finalNotificationReadiness()),
    ).toEqual({
      detail:
        "Latest report data has been loaded. Check Diagnosis Readiness and Evidence Traceability before confirming the final report.",
      title: "Diagnosis evidence review returned",
    });
  });

  it("directs confirmed returns to final delivery when readiness is ready", () => {
    expect(
      reportDiagnosisReviewReturnNotice(
        "confirmed",
        finalNotificationReadiness({
          detail:
            "All linked subreports have operator-confirmed AI conclusions; final notification can be sent.",
          notification_purpose: "final",
          ready: true,
          status: "ready",
          status_label: "Final notification ready",
        }),
      ),
    ).toEqual({
      detail:
        "Latest report data has been loaded. Report Delivery Proof can send the final report notification.",
      title: "Diagnosis conclusion confirmed",
    });
  });

  it("keeps confirmed returns blocked when other subreports still need confirmation", () => {
    expect(
      reportDiagnosisReviewReturnNotice(
        "confirmed",
        finalNotificationReadiness({
          detail:
            "Database capacity has no operator-confirmed AI conclusion yet.",
        }),
      ),
    ).toEqual({
      detail:
        "Latest report data has been loaded. Final notification remains blocked: Database capacity has no operator-confirmed AI conclusion yet.",
      title: "Diagnosis conclusion confirmed",
    });
  });
});

function finalNotificationReadiness(
  overrides: Partial<FinalReportDetail["final_notification_readiness"]> = {},
): FinalReportDetail["final_notification_readiness"] {
  return {
    detail: "Checkout API latency has no operator-confirmed AI conclusion yet.",
    notification_purpose: "handoff",
    ready: false,
    status: "blocked",
    status_label: "Final notification blocked",
    ...overrides,
  };
}
