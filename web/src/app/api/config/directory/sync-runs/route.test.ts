import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET } from "./route";

describe("directory sync runs config route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          items: [],
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards only diagnosis session authorization to the backend", async () => {
    const response = await GET(
      new Request(
        "https://console.example.com/api/config/directory/sync-runs?provider=ops_iam&limit=2",
        {
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
            "x-extra-secret": "must-not-forward",
          },
        },
      ),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ items: [] });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/directory/sync-runs?limit=2&provider=ops_iam",
    );
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("x-extra-secret")).toBeNull();
    expect(headers.get("cookie")).toBeNull();
  });

  it("rejects invalid limits before contacting the backend", async () => {
    const response = await GET(
      new Request(
        "https://console.example.com/api/config/directory/sync-runs?limit=0",
        {
          headers: {
            authorization: "Bearer token-1",
          },
        },
      ),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Directory sync run list limit must be a positive integer.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});

function restoreEnv(key: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[key];
    return;
  }
  process.env[key] = value;
}
