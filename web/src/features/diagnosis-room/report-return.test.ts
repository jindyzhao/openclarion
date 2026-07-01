import { describe, expect, it } from "vitest";

import {
  diagnosisReportReturnHref,
  diagnosisReviewReturnSearchParam,
  diagnosisReviewReturnState,
} from "./report-return";

describe("diagnosis room report return", () => {
  it("adds the diagnosis review marker to report return links", () => {
    expect(diagnosisReportReturnHref("/reports/101")).toBe(
      "/reports/101?diagnosis_review=1#diagnosis-readiness",
    );
    expect(diagnosisReportReturnHref("/reports/101?tab=readiness")).toBe(
      "/reports/101?tab=readiness&diagnosis_review=1#diagnosis-readiness",
    );
    expect(diagnosisReportReturnHref("/reports/101", "confirmed")).toBe(
      "/reports/101?diagnosis_review=confirmed#report-delivery-proof",
    );
  });

  it("routes confirmed returns to delivery proof from reviewed report links", () => {
    expect(
      diagnosisReportReturnHref(
        "/reports/101?diagnosis_review=1#diagnosis-readiness",
        "confirmed",
      ),
    ).toBe("/reports/101?diagnosis_review=confirmed#report-delivery-proof");
    expect(
      diagnosisReportReturnHref("/reports/101#custom-section", "confirmed"),
    ).toBe("/reports/101?diagnosis_review=confirmed#custom-section");
    expect(diagnosisReportReturnHref("/reports/101#custom-section")).toBe(
      "/reports/101?diagnosis_review=1#custom-section",
    );
  });

  it("recognizes explicit diagnosis review return markers", () => {
    expect(diagnosisReviewReturnSearchParam("1")).toBe(true);
    expect(diagnosisReviewReturnSearchParam(["true"])).toBe(true);
    expect(diagnosisReviewReturnSearchParam("confirmed")).toBe(true);
    expect(diagnosisReviewReturnSearchParam("0")).toBe(false);
    expect(diagnosisReviewReturnSearchParam(undefined)).toBe(false);
  });

  it("classifies diagnosis review return state", () => {
    expect(diagnosisReviewReturnState("confirmed")).toBe("confirmed");
    expect(diagnosisReviewReturnState("reviewed")).toBe("reviewed");
    expect(diagnosisReviewReturnState(["1"])).toBe("reviewed");
    expect(diagnosisReviewReturnState("0")).toBe("none");
  });
});
