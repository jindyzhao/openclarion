import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { PATCH } from "./route";

describe("tenant status BFF route", () => {
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

  it("patches tenant status with server-side session authorization", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          id: 2,
          key: "platform",
          name: "Platform",
          status: "disabled",
          created_at: "2026-07-11T10:00:00Z",
          updated_at: "2026-07-11T11:00:00Z",
        }),
      ),
    );
    const response = await PATCH(
      new Request("https://console.example.com/api/tenants/2", {
        method: "PATCH",
        headers: {
          "content-type": "application/json",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
        body: JSON.stringify({ status: "disabled" }),
      }),
      { params: Promise.resolve({ tenantId: "2" }) },
    );

    expect(response.status).toBe(200);
    const [url, init] = vi.mocked(fetch).mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe("https://api.example.com/api/v1/tenants/2");
    expect(init.method).toBe("PATCH");
    expect(init.body).toBe(JSON.stringify({ status: "disabled" }));
  });

  it("rejects an invalid tenant id before contacting the backend", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const response = await PATCH(
      new Request("https://console.example.com/api/tenants/not-a-number", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ status: "disabled" }),
      }),
      { params: Promise.resolve({ tenantId: "not-a-number" }) },
    );

    expect(response.status).toBe(400);
    expect(fetch).not.toHaveBeenCalled();
  });
});
