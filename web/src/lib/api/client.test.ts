import { afterEach, describe, expect, it, vi } from "vitest";

import { requestJSON } from "./client";

describe("requestJSON", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("returns a structured error when a successful backend response is not JSON", async () => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("not-json", { status: 200 })),
    );

    await expect(requestJSON<{ ok: true }>("/api/v1/status")).resolves.toEqual({
      ok: false,
      error: {
        message: "Response body must be valid JSON.",
        status: 502,
      },
    });

    expect(fetch).toHaveBeenCalledWith(
      new URL("/api/v1/status", "https://api.example.com"),
      expect.objectContaining({
        cache: "no-store",
        method: "GET",
      }),
    );
  });
});

function restoreEnv(key: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[key];
    return;
  }
  process.env[key] = value;
}
