import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

describe("diagnosis handoffs route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ items: [] }, { status: 200 })),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards caller authorization to the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/handoffs?limit=2", {
        headers: {
          authorization: "Bearer token-1",
          "x-extra-header": "must-not-forward",
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ items: [] });
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/handoffs?limit=2",
    );
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-header")).toBeNull();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/handoffs?limit=2"),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects invalid limits before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/handoffs?limit=0", {
        headers: {
          authorization: "Bearer token-1",
        },
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "limit must be between 1 and 100.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
