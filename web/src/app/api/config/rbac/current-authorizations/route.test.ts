import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("current rbac authorizations config route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          subject: "iam-user-1",
          department_keys: ["dep-1"],
          directory_users: [],
          decisions: [
            {
              permission: "directory.read",
              scope_kind: "global",
              scope_key: "",
              allowed: true,
              checked_at: "2026-06-22T10:00:00Z",
            },
          ],
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards current authorization checks with only browser-session authorization", async () => {
    const body = {
      requests: [{ permission: "directory.read", scope_kind: "global" }],
    };

    const response = await POST(
      new Request(
        "https://console.example.com/api/config/rbac/current-authorizations",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
            "x-extra-secret": "must-not-forward",
          },
          body: JSON.stringify(body),
        },
      ),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      subject: "iam-user-1",
      decisions: [{ allowed: true }],
    });
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/rbac/current-authorizations",
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
      new Request(
        "https://console.example.com/api/config/rbac/current-authorizations",
        {
          method: "POST",
          body: JSON.stringify({ requests: [] }),
        },
      ),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("clears browser-session cookies after backend auth failure", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "unauthorized" }, { status: 401 }),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/config/rbac/current-authorizations",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=expired.session.token`,
          },
          body: JSON.stringify({
            requests: [{ permission: "directory.read", scope_kind: "global" }],
          }),
        },
      ),
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
  });
});

function restoreEnv(key: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[key];
    return;
  }
  process.env[key] = value;
}
