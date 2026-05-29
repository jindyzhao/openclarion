import { describe, expect, it } from "vitest";

import { formatCount, formatSuccessRate } from "./format";

describe("dashboard formatters", () => {
  it("formats nullable success rates", () => {
    expect(formatSuccessRate(0.875)).toBe("88%");
    expect(formatSuccessRate(null)).toBe("n/a");
  });

  it("formats counts", () => {
    expect(formatCount(12000)).toBe("12,000");
  });
});
