import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET } from "./route";

describe("directory users config route", () => {
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
        "https://console.example.com/api/config/directory/users?provider=ops_iam&limit=2",
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
      "https://api.example.com/api/v1/config/directory/users?limit=2&provider=ops_iam",
    );
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("x-extra-secret")).toBeNull();
    expect(headers.get("cookie")).toBeNull();
  });

  it("requires explicit or browser-session authorization", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/config/directory/users"),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("clears browser-session cookies after backend auth failure", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "unauthorized" }, { status: 403 }),
    );

    const response = await GET(
      new Request("https://console.example.com/api/config/directory/users", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=expired.session.token`,
        },
      }),
    );

    expect(response.status).toBe(403);
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
