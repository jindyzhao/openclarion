import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET, PUT } from "./route";

const membership = {
  id: 3,
  tenant_id: 2,
  subject: "operator-1",
  role: "owner",
  enabled: true,
  created_by: "bootstrap-1",
  created_at: "2026-07-11T10:00:00Z",
  updated_at: "2026-07-11T10:00:00Z",
};

describe("tenant membership BFF route", () => {
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

  it("lists and validates tenant memberships", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ items: [membership] })));
    const response = await GET(
      new Request("https://console.example.com/api/tenants/2/memberships", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
      { params: Promise.resolve({ tenantId: "2" }) },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ items: [membership] });
  });

  it("updates a membership through the backend", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(membership)));
    const body = { subject: "operator-1", role: "owner", enabled: true };
    const response = await PUT(
      new Request("https://console.example.com/api/tenants/2/memberships", {
        method: "PUT",
        headers: {
          "content-type": "application/json",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
        body: JSON.stringify(body),
      }),
      { params: Promise.resolve({ tenantId: "2" }) },
    );

    expect(response.status).toBe(200);
    const [url, init] = vi.mocked(fetch).mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/tenants/2/memberships",
    );
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(JSON.stringify(body));
  });

  it("rejects malformed backend membership lists", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({ items: [membership, { ...membership, id: 4 }] }),
      ),
    );
    const response = await GET(
      new Request("https://console.example.com/api/tenants/2/memberships", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
      { params: Promise.resolve({ tenantId: "2" }) },
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: "Tenant membership list response is invalid.",
    });
  });
});
