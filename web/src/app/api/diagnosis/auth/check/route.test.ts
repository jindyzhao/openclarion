import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("diagnosis auth check route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          checked_at: "2026-06-21T04:00:00Z",
          mode: "ldap",
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

  it("forwards only the authorization header to the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-secret": "must-not-forward",
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      checked_at: "2026-06-21T04:00:00Z",
      mode: "ldap",
      role_authorized: true,
      roles: ["owner"],
      subject: "operator-1",
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/auth/check",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBeUndefined();

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("forwards Basic authorization for LDAP diagnosis auth", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
        headers: {
          authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
        },
      }),
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe(
      "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
    );
  });

  it("uses the diagnosis session cookie when Authorization is absent", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
        headers: {
          cookie: "openclarion_diagnosis_session=session.token.one",
        },
      }),
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
  });

  it("rejects malformed backend auth check responses", async () => {
    for (const body of [
      {
        checked_at: "2026-06-21T04:00:00Z",
        mode: "none",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-21T04:00:00Z",
        mode: "ldap",
        role_authorized: true,
        roles: ["viewer"],
        subject: "operator-1",
      },
      {
        checked_at: "not-a-date",
        mode: "ldap",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-21T04:00:00Z",
        mode: "ldap",
        role_authorized: "true",
        roles: ["owner"],
        subject: "operator-1",
      },
      {
        checked_at: "2026-06-21T04:00:00Z",
        mode: "ldap",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1\n",
      },
    ]) {
      vi.mocked(fetch).mockResolvedValueOnce(Response.json(body));

      const response = await POST(
        new Request("https://console.example.com/api/diagnosis/auth/check", {
          method: "POST",
          headers: {
            authorization: "Bearer token-1",
          },
        }),
      );

      expect(response.status).toBe(502);
      await expect(response.json()).resolves.toEqual({
        error: "diagnosis auth check response is invalid",
      });
      expect(response.headers.get("set-cookie")).toBeNull();
    }
  });

  it("clears the diagnosis session cookie when backend rejects a browser session", async () => {
    for (const status of [401, 403]) {
      vi.mocked(fetch).mockResolvedValueOnce(
        Response.json({ error: "invalid session" }, { status }),
      );

      const response = await POST(
        new Request("https://console.example.com/api/diagnosis/auth/check", {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=expired.session.token`,
          },
        }),
      );

      expect(response.status).toBe(status);
      const setCookie = response.headers.get("set-cookie") ?? "";
      expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
      expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
      expect(setCookie).toContain("HttpOnly");
    }
  });

  it("does not clear the diagnosis session cookie for explicit Authorization failures", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid bearer" }, { status: 401 }),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
        headers: {
          authorization: "Bearer expired.session.token",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
      }),
    );

    expect(response.status).toBe(401);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
      }),
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("clears malformed diagnosis session cookies before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/auth/check", {
        method: "POST",
        headers: {
          cookie: `${diagnosisSessionCookieName}=session%20token`,
        },
      }),
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(fetch).not.toHaveBeenCalled();
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
