import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET, POST } from "./route";

describe("tenant list BFF route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
  });

  afterEach(() => {
    if (originalAPIBaseURL === undefined) {
      delete process.env.OPENCLARION_API_BASE_URL;
    } else {
      process.env.OPENCLARION_API_BASE_URL = originalAPIBaseURL;
    }
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards the HttpOnly session as server-side bearer authorization", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          items: [
            {
              id: 1,
              key: "default",
              name: "Default",
              status: "active",
              created_at: "2026-07-11T10:00:00Z",
              updated_at: "2026-07-11T10:00:00Z",
            },
          ],
        }),
      ),
    );
    const response = await GET(
      new Request("https://console.example.com/api/tenants", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
    );
    expect(response.status).toBe(200);
    const [, init] = vi.mocked(fetch).mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect((init.headers as Headers).get("authorization")).toBe(
      "Bearer session.token.one",
    );
  });

  it("creates a workspace with the HttpOnly session credential", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            id: 4,
            key: "security",
            name: "Security",
            status: "active",
            created_at: "2026-07-11T10:00:00Z",
            updated_at: "2026-07-11T10:00:00Z",
          },
          { status: 201 },
        ),
      ),
    );
    const response = await POST(
      new Request("https://console.example.com/api/tenants", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
        body: JSON.stringify({ key: "security", name: "Security" }),
      }),
    );

    expect(response.status).toBe(201);
    await expect(response.json()).resolves.toMatchObject({
      id: 4,
      key: "security",
    });
    const [url, init] = vi.mocked(fetch).mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe("https://api.example.com/api/v1/tenants");
    expect(init.method).toBe("POST");
    expect((init.headers as Headers).get("authorization")).toBe(
      "Bearer session.token.one",
    );
    expect(init.body).toBe(JSON.stringify({ key: "security", name: "Security" }));
  });
});
