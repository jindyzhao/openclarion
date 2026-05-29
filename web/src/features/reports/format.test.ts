import { describe, expect, test } from "vitest";

import { displayStatus, formatDateTime, severityClass } from "./format";

describe("report formatters", () => {
  test("formats valid timestamps in UTC", () => {
    expect(formatDateTime("2026-05-28T10:15:00Z")).toContain("2026");
  });

  test("preserves invalid timestamps", () => {
    expect(formatDateTime("not-a-date")).toBe("not-a-date");
  });

  test("maps severities to stable CSS classes", () => {
    expect(severityClass("critical")).toBe("pill pill-critical");
    expect(severityClass("warning")).toBe("pill pill-warning");
    expect(severityClass("info")).toBe("pill pill-info");
  });

  test("pluralizes the report count", () => {
    expect(displayStatus({ count: 1, fetchedAt: new Date("2026-05-28T10:15:00Z") })).toContain(
      "1 report"
    );
    expect(displayStatus({ count: 2, fetchedAt: new Date("2026-05-28T10:15:00Z") })).toContain(
      "2 reports"
    );
  });
});
