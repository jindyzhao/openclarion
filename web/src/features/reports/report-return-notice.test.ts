import { describe, expect, it } from "vitest";

import { reportDiagnosisReviewReturnNotice } from "./report-return-notice";
import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";
import type { FinalReportDetail } from "./types";

describe("report diagnosis review return notice", () => {
  it("keeps reviewed returns focused on diagnosis readiness", () => {
    expect(
      reportDiagnosisReviewReturnNotice("reviewed", finalNotificationReadiness()),
    ).toBe("reviewed");
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
    ).toBe("confirmed_ready");
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
    ).toBe("confirmed_blocked");
  });
});

function finalNotificationReadiness(
  overrides: Partial<FinalReportDetail["final_notification_readiness"]> = {},
): ReportFinalNotificationReadiness {
  return {
    detail: "Checkout API latency has no operator-confirmed AI conclusion yet.",
    notification_purpose: "handoff",
    ready: false,
    source: "api",
    status: "blocked",
    status_label: "Final notification blocked",
    ...overrides,
  };
}
