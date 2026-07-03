import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET, POST } from "./route";

describe("rbac assignments config route", () => {
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

  it("forwards assignment list requests with only diagnosis session authorization", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/config/rbac/assignments?limit=5", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
          "x-extra-secret": "must-not-forward",
        },
      }),
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
      "https://api.example.com/api/v1/config/rbac/assignments?limit=5",
    );
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("x-extra-secret")).toBeNull();
    expect(headers.get("cookie")).toBeNull();
  });

  it("forwards assignment writes with only caller authorization", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({
        id: 10,
        subject_kind: "user",
        subject_key: "iam-user-1",
        role: "operator",
        scope_kind: "global",
        scope_key: "",
        enabled: true,
        created_by: "admin-1",
        updated_by: "admin-1",
        created_at: "2026-06-22T10:00:00Z",
        updated_at: "2026-06-22T10:00:00Z",
      }),
    );
    const body = {
      subject_kind: "user",
      subject_key: "iam-user-1",
      role: "operator",
      scope_kind: "global",
      enabled: true,
    };

    const response = await POST(
      new Request("https://console.example.com/api/config/rbac/assignments", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-secret": "must-not-forward",
        },
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      subject_key: "iam-user-1",
      role: "operator",
    });
    const fetchMock = vi.mocked(fetch);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/rbac/assignments",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("requires authorization before listing assignments", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/config/rbac/assignments"),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects invalid list limits before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/config/rbac/assignments?limit=0", {
        headers: {
          authorization: "Bearer token-1",
        },
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "RBAC assignment list limit must be a positive integer.",
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
