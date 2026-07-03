import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { DELETE, GET, POST } from "./route";

describe("diagnosis browser session route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("returns unauthenticated without contacting the backend when no cookie exists", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/auth/session"),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ authenticated: false });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("checks the backend with the session cookie bearer token", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      authenticated: true,
      checked_at: "2026-06-22T10:00:00Z",
      mode: "oidc",
      role_authorized: true,
      roles: ["owner"],
      subject: "operator-1",
    });
    const fetchMock = vi.mocked(fetch);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/auth/check",
    );
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
  });

  it("exchanges LDAP Basic credentials for an HttpOnly browser session cookie", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          token: "ldap.session.token",
          checked_at: "2026-06-22T10:00:00Z",
          expires_at: "2099-06-22T18:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
        { status: 201 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "POST",
        headers: {
          authorization: `Basic ${btoa("operator-1:ldap-password")}`,
        },
      }),
    );

    expect(response.status).toBe(201);
    await expect(response.json()).resolves.toEqual({
      authenticated: true,
      checked_at: "2026-06-22T10:00:00Z",
      mode: "ldap",
      role_authorized: true,
      roles: ["owner"],
      subject: "operator-1",
    });
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(
      `${diagnosisSessionCookieName}=ldap.session.token`,
    );
    expect(setCookie).toContain("HttpOnly");
    expect(setCookie).toContain("SameSite=lax");
    expect(setCookie).toContain("Secure");

    const fetchMock = vi.mocked(fetch);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/auth/session",
    );
    expect(init.method).toBe("POST");
    expect((init.headers as Headers).get("authorization")).toBe(
      `Basic ${btoa("operator-1:ldap-password")}`,
    );
  });

  it("requires explicit credentials before creating a browser session", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "POST",
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(response.headers.get("set-cookie")).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects malformed backend session issue responses without setting a cookie", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          token: "ldap.session.token",
          checked_at: "2026-06-22T10:00:00Z",
          expires_at: "2099-06-22T18:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["root"],
          subject: "operator-1",
        },
        { status: 201 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "POST",
        headers: {
          authorization: `Basic ${btoa("operator-1:ldap-password")}`,
        },
      }),
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: "diagnosis browser session response is invalid",
    });
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects expired backend session issue responses before setting a cookie", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          token: "ldap.session.token",
          checked_at: "2026-06-22T10:00:00Z",
          expires_at: "2000-01-01T00:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
        { status: 201 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "POST",
        headers: {
          authorization: `Basic ${btoa("operator-1:ldap-password")}`,
        },
      }),
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: "diagnosis browser session response is invalid",
    });
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("accepts identity-only backend session issue responses for local RBAC", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          token: "ldap.session.token",
          checked_at: "2026-06-22T10:00:00Z",
          expires_at: "2099-06-22T18:00:00Z",
          mode: "ldap",
          role_authorized: false,
          roles: [],
          subject: "operator-1",
        },
        { status: 201 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "POST",
        headers: {
          authorization: `Basic ${btoa("operator-1:ldap-password")}`,
        },
      }),
    );

    expect(response.status).toBe(201);
    await expect(response.json()).resolves.toEqual({
      authenticated: true,
      checked_at: "2026-06-22T10:00:00Z",
      mode: "ldap",
      role_authorized: false,
      roles: [],
      subject: "operator-1",
    });
    expect(response.headers.get("set-cookie")).toContain(
      `${diagnosisSessionCookieName}=ldap.session.token`,
    );
  });

  it("rejects malformed backend session status without clearing the cookie", async () => {
    for (const body of [
      {
        checked_at: "2026-06-22T10:00:00Z",
        mode: "none",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["root"],
        subject: "operator-1",
      },
      {
        checked_at: "invalid",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: "yes",
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: " operator-1",
      },
    ]) {
      vi.mocked(fetch).mockResolvedValueOnce(Response.json(body));

      const response = await GET(
        new Request("https://console.example.com/api/diagnosis/auth/session", {
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
          },
        }),
      );

      expect(response.status).toBe(502);
      await expect(response.json()).resolves.toEqual({
        error: "diagnosis browser session response is invalid",
      });
      expect(response.headers.get("set-cookie")).toBeNull();
    }
  });

  it("clears the cookie when the backend rejects the session", async () => {
    for (const status of [401, 403]) {
      vi.mocked(fetch).mockResolvedValueOnce(
        Response.json({ error: "authentication failed" }, { status }),
      );

      const response = await GET(
        new Request("https://console.example.com/api/diagnosis/auth/session", {
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
          },
        }),
      );

      expect(response.status).toBe(200);
      await expect(response.json()).resolves.toEqual({ authenticated: false });
      expect(response.headers.get("set-cookie")).toContain(
        `${diagnosisSessionCookieName}=`,
      );
    }
  });

  it("clears malformed session cookies without contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session%20token`,
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ authenticated: false });
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("deletes the browser session cookie", async () => {
    const response = await DELETE(
      new Request("https://console.example.com/api/diagnosis/auth/session", {
        method: "DELETE",
      }),
    );

    expect(response.status).toBe(204);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("HttpOnly");
    expect(setCookie).toContain("Secure");
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
