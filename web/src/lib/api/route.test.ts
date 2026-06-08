import { describe, expect, it } from "vitest";

import { parsePositiveIntegerRouteParam } from "./route";

describe("parsePositiveIntegerRouteParam", () => {
  it("accepts decimal positive integer route params", () => {
    expect(parsePositiveIntegerRouteParam("42", "Resource ID")).toEqual({ ok: true, data: 42 });
    expect(parsePositiveIntegerRouteParam(" 7 ", "Resource ID")).toEqual({ ok: true, data: 7 });
  });

  it("rejects partial, fractional, zero, and unsafe route params", () => {
    const expected = {
      ok: false,
      error: { message: "Resource ID must be a positive integer.", status: 400 }
    };

    expect(parsePositiveIntegerRouteParam("12abc", "Resource ID")).toEqual(expected);
    expect(parsePositiveIntegerRouteParam("1.5", "Resource ID")).toEqual(expected);
    expect(parsePositiveIntegerRouteParam("0", "Resource ID")).toEqual(expected);
    expect(parsePositiveIntegerRouteParam("9007199254740992", "Resource ID")).toEqual(expected);
  });
});
