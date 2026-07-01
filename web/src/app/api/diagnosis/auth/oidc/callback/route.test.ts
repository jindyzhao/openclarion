import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  diagnosisOIDCStateCookieName,
  sealDiagnosisOIDCStatePayload,
  type DiagnosisOIDCStatePayload,
} from "@/lib/api/diagnosis-oidc-login";
import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET } from "./route";

describe("diagnosis OIDC callback route", () => {
  const originalEnv = oidcEnvSnapshot();

  beforeEach(() => {
    clearStandardOIDCEnv();
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET = "client-secret-1";
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY = stateSigningKey();
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD;
  });

  afterEach(() => {
    restoreOIDCEnv(originalEnv);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("exchanges the authorization code, verifies nonce, sets session, and redirects", async () => {
    const payload = oidcStatePayload();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(oidcDiscoveryResponse())
        .mockResolvedValueOnce(
          Response.json({
            access_token: "access.token.one",
            id_token: fakeIDToken("nonce-1"),
            token_type: "Bearer",
          }),
        )
        .mockResolvedValueOnce(
          Response.json({
            checked_at: "2026-06-25T02:00:00Z",
            expires_at: "2099-06-25T02:00:00Z",
            mode: "oidc",
            role_authorized: true,
            roles: ["owner"],
            subject: "operator-1",
            token: "session.token.one",
          }),
        ),
    );

    const response = await GET(
      oidcCallbackRequest(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
        payload,
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?session_id=room-1&auth_mode=session",
    );
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=session.token.one`);
    expect(setCookie).toContain(`${diagnosisOIDCStateCookieName}=`);
    expect(setCookie).toContain("HttpOnly");
    expect(setCookie).toContain("Secure");

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    const [tokenURL, tokenInit] = fetchMock.mock.calls[1] as unknown as [
      URL,
      RequestInit,
    ];
    expect(tokenURL.toString()).toBe("https://iam.example.com/oauth/token");
    const tokenBody = tokenInit.body as URLSearchParams;
    expect(new Headers(tokenInit.headers).get("authorization")).toBe(
      `Basic ${Buffer.from("openclarion:client-secret-1", "utf8").toString("base64")}`,
    );
    expect(tokenBody.get("client_id")).toBeNull();
    expect(tokenBody.get("client_secret")).toBeNull();
    expect(tokenBody.get("code")).toBe("callback-code-1");
    expect(tokenBody.get("code_verifier")).toBe("code-verifier-1");

    const [backendURL, backendInit] = fetchMock.mock.calls[2] as unknown as [
      URL,
      RequestInit,
    ];
    expect(backendURL.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/auth/session",
    );
    expect(new Headers(backendInit.headers).get("authorization")).toBe(
      `Bearer ${fakeIDToken("nonce-1")}`,
    );
    expect(
      new Headers(backendInit.headers).get(
        "x-openclarion-oidc-access-token",
      ),
    ).toBe("access.token.one");
  });

  it("sets a browser session for OIDC identities that rely on local RBAC", async () => {
    const payload = oidcStatePayload();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(oidcDiscoveryResponse())
        .mockResolvedValueOnce(
          Response.json({
            access_token: "access.token.one",
            id_token: fakeIDToken("nonce-1"),
            token_type: "Bearer",
          }),
        )
        .mockResolvedValueOnce(
          Response.json({
            checked_at: "2026-06-25T02:00:00Z",
            expires_at: "2099-06-25T02:00:00Z",
            mode: "oidc",
            role_authorized: false,
            roles: [],
            subject: "operator-1",
            token: "session.token.one",
          }),
        ),
    );

    const response = await GET(
      oidcCallbackRequest(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
        payload,
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?session_id=room-1&auth_mode=session",
    );
    expect(response.headers.get("set-cookie")).toContain(
      `${diagnosisSessionCookieName}=session.token.one`,
    );
  });

  it("rejects nonce mismatch without creating a backend session", async () => {
    const payload = oidcStatePayload();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(oidcDiscoveryResponse())
        .mockResolvedValueOnce(Response.json({ id_token: fakeIDToken("nonce-2") })),
    );

    const response = await GET(
      oidcCallbackRequest(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
        payload,
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?session_id=room-1&auth_mode=session&oidc_auth_error=oidc_callback_failed",
    );
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("exchanges codes using standard OIDC env aliases", async () => {
    delete process.env.OPENCLARION_IAM_OIDC_ISSUER;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD;
    delete process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY;
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-standard";
    process.env.OIDC_CLIENT_SECRET = "standard-client-secret";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_post";
    process.env.OIDC_REDIRECT_URL =
      "https://console.example.com/api/diagnosis/auth/oidc/callback";
    process.env.OIDC_STATE_SIGNING_KEY = stateSigningKey();
    const payload = oidcStatePayload();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(oidcDiscoveryResponse())
        .mockResolvedValueOnce(Response.json({ id_token: fakeIDToken("nonce-1") }))
        .mockResolvedValueOnce(
          Response.json({
            checked_at: "2026-06-25T02:00:00Z",
            expires_at: "2099-06-25T02:00:00Z",
            mode: "oidc",
            role_authorized: false,
            roles: [],
            subject: "operator-1",
            token: "session.token.one",
          }),
        ),
    );

    const response = await GET(
      oidcCallbackRequest(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
        payload,
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("set-cookie")).toContain(
      `${diagnosisSessionCookieName}=session.token.one`,
    );
    const fetchMock = vi.mocked(fetch);
    const [, tokenInit] = fetchMock.mock.calls[1] as unknown as [
      URL,
      RequestInit,
    ];
    const tokenBody = tokenInit.body as URLSearchParams;
    expect(new Headers(tokenInit.headers).get("authorization")).toBeNull();
    expect(tokenBody.get("client_id")).toBe("openclarion-standard");
    expect(tokenBody.get("client_secret")).toBe("standard-client-secret");
  });

  it("maps backend role denial to a recoverable IAM role error", async () => {
    const payload = oidcStatePayload();
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(oidcDiscoveryResponse())
        .mockResolvedValueOnce(Response.json({ id_token: fakeIDToken("nonce-1") }))
        .mockResolvedValueOnce(
          Response.json({ error: "forbidden" }, { status: 403 }),
        ),
    );

    const response = await GET(
      oidcCallbackRequest(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
        payload,
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?session_id=room-1&auth_mode=session&oidc_auth_error=oidc_role_unauthorized",
    );
  });

  it("rejects missing or stale state before contacting IAM", async () => {
    vi.stubGlobal("fetch", vi.fn());

    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/auth/oidc/callback?code=callback-code-1&state=state-1",
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?auth_mode=session&oidc_auth_error=oidc_callback_missing",
    );
    expect(fetch).not.toHaveBeenCalled();
  });
});

function oidcStatePayload(): DiagnosisOIDCStatePayload {
  return {
    codeVerifier: "code-verifier-1",
    issuedAt: Date.now(),
    nonce: "nonce-1",
    returnTo: "/diagnosis-room?session_id=room-1&auth_mode=session",
    state: "state-1",
  };
}

function oidcCallbackRequest(
  url: string,
  payload: DiagnosisOIDCStatePayload,
): Request {
  const sealedState = sealDiagnosisOIDCStatePayload(
    payload,
    Buffer.from(stateSigningKey(), "utf8"),
  );
  return new Request(url, {
    headers: {
      cookie: `${diagnosisOIDCStateCookieName}=${encodeURIComponent(sealedState)}`,
    },
  });
}

function oidcDiscoveryResponse(): Response {
  return Response.json({
    authorization_endpoint: "https://iam.example.com/oauth/authorize",
    issuer: "https://iam.example.com/",
    token_endpoint: "https://iam.example.com/oauth/token",
  });
}

function fakeIDToken(nonce: string): string {
  return [
    Buffer.from(JSON.stringify({ alg: "none" })).toString("base64url"),
    Buffer.from(JSON.stringify({ nonce })).toString("base64url"),
    "signature",
  ].join(".");
}

function stateSigningKey(): string {
  return "s".repeat(32);
}

function oidcEnvSnapshot(): Record<string, string | undefined> {
  return {
    apiBaseURL: process.env.OPENCLARION_API_BASE_URL,
    clientID: process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    clientSecret: process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
    clientAuthMethod: process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    issuer: process.env.OPENCLARION_IAM_OIDC_ISSUER,
    stateSigningKey: process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    standardClientID: process.env.OIDC_CLIENT_ID,
    standardClientSecret: process.env.OIDC_CLIENT_SECRET,
    standardClientAuthMethod: process.env.OIDC_CLIENT_AUTH_METHOD,
    standardIssuer: process.env.OIDC_ISSUER,
    standardRedirectURL: process.env.OIDC_REDIRECT_URL,
    standardStateSigningKey: process.env.OIDC_STATE_SIGNING_KEY,
  };
}

function restoreOIDCEnv(snapshot: Record<string, string | undefined>) {
  restoreEnv("OPENCLARION_API_BASE_URL", snapshot.apiBaseURL);
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_ID", snapshot.clientID);
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_SECRET", snapshot.clientSecret);
  restoreEnv(
    "OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD",
    snapshot.clientAuthMethod,
  );
  restoreEnv("OPENCLARION_IAM_OIDC_ISSUER", snapshot.issuer);
  restoreEnv(
    "OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY",
    snapshot.stateSigningKey,
  );
  restoreEnv("OIDC_CLIENT_ID", snapshot.standardClientID);
  restoreEnv("OIDC_CLIENT_SECRET", snapshot.standardClientSecret);
  restoreEnv("OIDC_CLIENT_AUTH_METHOD", snapshot.standardClientAuthMethod);
  restoreEnv("OIDC_ISSUER", snapshot.standardIssuer);
  restoreEnv("OIDC_REDIRECT_URL", snapshot.standardRedirectURL);
  restoreEnv("OIDC_STATE_SIGNING_KEY", snapshot.standardStateSigningKey);
}

function clearStandardOIDCEnv() {
  delete process.env.OIDC_CLIENT_ID;
  delete process.env.OIDC_CLIENT_SECRET;
  delete process.env.OIDC_CLIENT_AUTH_METHOD;
  delete process.env.OIDC_ISSUER;
  delete process.env.OIDC_REDIRECT_URL;
  delete process.env.OIDC_STATE_SIGNING_KEY;
}

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
