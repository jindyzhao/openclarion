import { describe, expect, it } from "vitest";

import {
  diagnosisAuthorizationFromRequest,
  diagnosisRequestPublicOrigin,
  diagnosisRequestHasSessionCookie,
  diagnosisRequestUsesSessionCookieAuthorization,
  diagnosisSessionCookieName,
  diagnosisSessionCookieSecure,
  expireDiagnosisSessionCookie,
  expireDiagnosisSessionCookieOnAuthFailure,
  normalizedDiagnosisSessionToken,
} from "./diagnosis-session";

describe("diagnosis session authorization", () => {
  it("prefers explicit Authorization over the session cookie", () => {
    const request = new Request("https://console.example.com/api", {
      headers: {
        authorization: "Basic b3BlcmF0b3I6cGFzcw==",
        cookie: `${diagnosisSessionCookieName}=session.token.one`,
      },
    });

    expect(diagnosisAuthorizationFromRequest(request)).toBe(
      "Basic b3BlcmF0b3I6cGFzcw==",
    );
  });

  it("falls back to the diagnosis session cookie as Bearer auth", () => {
    const request = new Request("https://console.example.com/api", {
      headers: {
        cookie: `other=1; ${diagnosisSessionCookieName}=session.token.one`,
      },
    });

    expect(diagnosisAuthorizationFromRequest(request)).toBe(
      "Bearer session.token.one",
    );
    expect(diagnosisRequestHasSessionCookie(request)).toBe(true);
  });

  it("rejects malformed session cookie values", () => {
    const request = new Request("https://console.example.com/api", {
      headers: {
        cookie: `${diagnosisSessionCookieName}=session%20token`,
      },
    });

    expect(diagnosisAuthorizationFromRequest(request)).toBeNull();
    expect(diagnosisRequestHasSessionCookie(request)).toBe(true);
    expect(diagnosisRequestUsesSessionCookieAuthorization(request)).toBe(true);
    expect(normalizedDiagnosisSessionToken(" session.token.one")).toBeNull();
    expect(normalizedDiagnosisSessionToken("session-token-one")).toBeNull();
    expect(normalizedDiagnosisSessionToken("session.token")).toBeNull();
  });

  it("uses secure cookies only on HTTPS origins", () => {
    expect(diagnosisSessionCookieSecure("https://console.example.com")).toBe(
      true,
    );
    expect(diagnosisSessionCookieSecure("http://localhost:3000")).toBe(false);
    expect(
      diagnosisSessionCookieSecure(
        new Request("http://openclarion.internal/api", {
          headers: { "x-forwarded-proto": "https" },
        }),
      ),
    ).toBe(true);
    expect(
      diagnosisSessionCookieSecure(
        new Request("http://openclarion.internal/api", {
          headers: { "x-forwarded-proto": "http, https" },
        }),
      ),
    ).toBe(true);
    expect(
      diagnosisSessionCookieSecure(
        new Request("http://openclarion.internal/api", {
          headers: { "x-forwarded-proto": "http" },
        }),
      ),
    ).toBe(false);
    expect(
      diagnosisRequestPublicOrigin(
        new Request("http://openclarion.internal/api", {
          headers: {
            "x-forwarded-host": "public.example.com",
            "x-forwarded-proto": "https",
          },
        }),
      ),
    ).toBe("https://openclarion.internal");
  });

  it("sets an expired HttpOnly cookie when clearing the session", () => {
    const cookieSetCalls: unknown[] = [];
    expireDiagnosisSessionCookie(
      {
        cookies: {
          set: (options) => cookieSetCalls.push(options),
        },
      },
      "https://console.example.com",
    );

    expect(cookieSetCalls).toEqual([
      {
        name: diagnosisSessionCookieName,
        value: "",
        httpOnly: true,
        sameSite: "lax",
        secure: true,
        path: "/",
        expires: new Date(0),
      },
    ]);
  });

  it("expires the session cookie only for browser-session auth failures", () => {
    const sessionCookieRequest = new Request("https://console.example.com/api", {
      headers: {
        cookie: `${diagnosisSessionCookieName}=expired.session.token`,
      },
    });
    const explicitAuthRequest = new Request("https://console.example.com/api", {
      headers: {
        authorization: "Basic b3BlcmF0b3I6cGFzcw==",
        cookie: `${diagnosisSessionCookieName}=session.token.one`,
      },
    });
    const sessionCookieSetCalls: unknown[] = [];
    const forbiddenSetCalls: unknown[] = [];
    const explicitAuthSetCalls: unknown[] = [];
    const backendFailureSetCalls: unknown[] = [];

    expireDiagnosisSessionCookieOnAuthFailure(
      {
        cookies: {
          set: (options) => sessionCookieSetCalls.push(options),
        },
      },
      sessionCookieRequest,
      401,
    );
    expireDiagnosisSessionCookieOnAuthFailure(
      {
        cookies: {
          set: (options) => forbiddenSetCalls.push(options),
        },
      },
      sessionCookieRequest,
      403,
    );
    expireDiagnosisSessionCookieOnAuthFailure(
      {
        cookies: {
          set: (options) => explicitAuthSetCalls.push(options),
        },
      },
      explicitAuthRequest,
      401,
    );
    expireDiagnosisSessionCookieOnAuthFailure(
      {
        cookies: {
          set: (options) => backendFailureSetCalls.push(options),
        },
      },
      sessionCookieRequest,
      500,
    );

    expect(sessionCookieSetCalls).toHaveLength(1);
    expect(forbiddenSetCalls).toHaveLength(1);
    expect(explicitAuthSetCalls).toEqual([]);
    expect(backendFailureSetCalls).toEqual([]);
  });

});
