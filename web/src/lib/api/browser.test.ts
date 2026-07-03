import { afterEach, describe, expect, it, vi } from "vitest";

import { requestSameOriginJSON } from "./browser";

describe("requestSameOriginJSON", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("returns a structured error when a successful same-origin response is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 200 })),
    );

    await expect(
      requestSameOriginJSON<{ ok: true }>("/api/status"),
    ).resolves.toEqual({
      error: {
        message: "Response body must be valid JSON.",
        status: 502,
      },
      ok: false,
    });

    expect(fetch).toHaveBeenCalledWith(
      "/api/status",
      expect.objectContaining({
        cache: "no-store",
        credentials: "same-origin",
        method: "GET",
      }),
    );
  });

  it("sends JSON bodies with same-origin credentials for browser session routes", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ ok: true })),
    );

    await expect(
      requestSameOriginJSON<{ ok: true }>("/api/diagnosis/ws-ticket", {
        body: { session_id: "session-1" },
        method: "POST",
      }),
    ).resolves.toEqual({ data: { ok: true }, ok: true });

    expect(fetch).toHaveBeenCalledWith(
      "/api/diagnosis/ws-ticket",
      expect.objectContaining({
        body: JSON.stringify({ session_id: "session-1" }),
        cache: "no-store",
        credentials: "same-origin",
        method: "POST",
      }),
    );
    const [, init] = vi.mocked(fetch).mock.calls[0] as unknown as [
      RequestInfo | URL,
      RequestInit,
    ];
    const headers = init.headers as Headers;
    expect(headers.get("accept")).toBe("application/json");
    expect(headers.get("content-type")).toBe("application/json");
  });
});
