import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("rbac authorization preview config route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          allowed: true,
          checked_at: "2026-06-22T10:00:00Z",
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards preview body with only browser-session authorization", async () => {
    const body = {
      subject: "iam-user-1",
      department_keys: ["dep-1"],
      permission: "diagnosis_room.participate",
      scope_kind: "diagnosis_room",
      scope_key: "room-1",
    };

    const response = await POST(
      new Request("https://console.example.com/api/config/rbac/authorize", {
        method: "POST",
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
          "x-extra-secret": "must-not-forward",
        },
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      allowed: true,
      checked_at: "2026-06-22T10:00:00Z",
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/rbac/authorize",
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
      new Request("https://console.example.com/api/config/rbac/authorize", {
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
      new Request("https://console.example.com/api/config/rbac/authorize", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: "{",
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Request body must be valid JSON.",
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
