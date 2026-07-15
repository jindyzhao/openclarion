import { describe, expect, it } from "vitest";

import { reportDiagnosisReviewReturnNotice } from "./report-return-notice";
import type { ReportFinalNotificationReadiness } from "./diagnosis-readiness";

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
          notification_purpose: "final",
          ready: true,
          reason: { kind: "ready" },
          status: "ready",
        }),
      ),
    ).toBe("confirmed_ready");
  });

  it("keeps confirmed returns blocked when other subreports still need confirmation", () => {
    expect(
      reportDiagnosisReviewReturnNotice(
        "confirmed",
        finalNotificationReadiness({
          reason: {
            kind: "unconfirmed_conclusion",
            subReportID: 502,
            subReportTitle: "Database capacity",
          },
        }),
      ),
    ).toBe("confirmed_blocked");
  });
});

function finalNotificationReadiness(
  overrides: Partial<
    Extract<ReportFinalNotificationReadiness, { source: "api" }>
  > = {},
): ReportFinalNotificationReadiness {
  return {
    notification_purpose: "handoff",
    ready: false,
    reason: {
      kind: "unconfirmed_conclusion",
      subReportID: 501,
      subReportTitle: "Checkout API latency",
    },
    source: "api",
    status: "blocked",
    ...overrides,
  };
}
