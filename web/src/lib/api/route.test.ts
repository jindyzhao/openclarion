import { describe, expect, it } from "vitest";

import { readRequestJSON } from "./route";

describe("route api helpers", () => {
  it("returns a stable invalid JSON error without parser internals", async () => {
    await expect(
      readRequestJSON(
        new Request("https://console.example.com/api/example", {
          method: "POST",
          body: "{",
        }),
      ),
    ).resolves.toEqual({
      ok: false,
      error: {
        message: "Request body must be valid JSON.",
        status: 400,
      },
    });
  });

  it("parses valid JSON request bodies", async () => {
    await expect(
      readRequestJSON<{ value: string }>(
        new Request("https://console.example.com/api/example", {
          method: "POST",
          body: JSON.stringify({ value: "ok" }),
        }),
      ),
    ).resolves.toEqual({
      ok: true,
      data: { value: "ok" },
    });
  });
});
