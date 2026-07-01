import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("directory sync config route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          department_pages: 1,
          user_pages: 2,
          departments_upserted: 3,
          users_upserted: 4,
          synced_at: "2026-06-22T10:00:00Z",
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards sync body and only diagnosis session authorization to the backend", async () => {
    const body = {
      page_size: 20,
      updated_after: "2026-06-22T09:00:00Z",
    };

    const response = await POST(
      new Request("https://console.example.com/api/config/directory/sync", {
        method: "POST",
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
          "x-extra-secret": "must-not-forward",
        },
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      departments_upserted: 3,
      users_upserted: 4,
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/directory/sync",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("x-extra-secret")).toBeNull();
    expect(headers.get("cookie")).toBeNull();
  });

  it("requires explicit or browser-session authorization", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/config/directory/sync", {
        method: "POST",
        body: "{}",
      }),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects invalid JSON before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/config/directory/sync", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: "{",
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toMatchObject({
      error: expect.stringContaining("JSON"),
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
