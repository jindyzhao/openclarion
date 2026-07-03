import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisOIDCStateCookieName } from "@/lib/api/diagnosis-oidc-login";

import { GET } from "./route";

describe("diagnosis OIDC login route", () => {
  const originalEnv = oidcEnvSnapshot();

  beforeEach(() => {
    clearStandardOIDCEnv();
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET = "client-secret-1";
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          authorization_endpoint: "https://iam.example.com/oauth/authorize",
          issuer: "https://iam.example.com/",
          token_endpoint: "https://iam.example.com/oauth/token",
        }),
      ),
    );
  });

  afterEach(() => {
    restoreOIDCEnv(originalEnv);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("discovers the provider, sets signed state, and redirects to authorization", async () => {
    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/auth/oidc/login?return_to=%2Fdiagnosis-room%3Fsession_id%3Droom-1",
      ),
    );

    expect(response.status).toBe(307);
    const location = new URL(response.headers.get("location") ?? "");
    expect(location.toString()).toContain(
      "https://iam.example.com/oauth/authorize",
    );
    expect(location.searchParams.get("client_id")).toBe("openclarion");
    expect(location.searchParams.get("redirect_uri")).toBe(
      "https://console.example.com/api/diagnosis/auth/oidc/callback",
    );
    expect(location.searchParams.get("response_type")).toBe("code");
    expect(location.searchParams.get("scope")).toBe("openid profile email");
    expect(location.searchParams.get("nonce")).not.toBe("");
    expect(location.searchParams.get("state")).not.toBe("");
    expect(location.searchParams.get("code_challenge_method")).toBe("S256");

    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisOIDCStateCookieName}=`);
    expect(setCookie).toContain("HttpOnly");
    expect(setCookie).toContain("SameSite=lax");
    expect(setCookie).toContain("Path=/api/diagnosis/auth/oidc/callback");

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe(
      "https://iam.example.com/.well-known/openid-configuration",
    );
  });

  it("uses standard OIDC env aliases for provider login", async () => {
    delete process.env.OPENCLARION_IAM_OIDC_ISSUER;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET;
    delete process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY;
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-standard";
    process.env.OIDC_REDIRECT_URL =
      "https://console.example.com/api/diagnosis/auth/oidc/callback";
    process.env.OIDC_SCOPES = "openid profile email phone";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OIDC_USE_PKCE = "false";

    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/auth/oidc/login?return_to=%2Fsettings%2Fdirectory-rbac",
      ),
    );

    expect(response.status).toBe(307);
    const location = new URL(response.headers.get("location") ?? "");
    expect(location.searchParams.get("client_id")).toBe("openclarion-standard");
    expect(location.searchParams.get("redirect_uri")).toBe(
      "https://console.example.com/api/diagnosis/auth/oidc/callback",
    );
    expect(location.searchParams.get("scope")).toBe(
      "openid profile email phone",
    );
    expect(location.searchParams.get("code_challenge")).toBeNull();
    expect(response.headers.get("set-cookie")).toContain(
      `${diagnosisOIDCStateCookieName}=`,
    );
  });

  it("redirects recoverably when OIDC config is incomplete", async () => {
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;

    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/auth/oidc/login?return_to=%2Fdiagnosis-room%3Fsession_id%3Droom-1",
      ),
    );

    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe(
      "https://console.example.com/diagnosis-room?session_id=room-1&auth_mode=session&oidc_auth_error=oidc_not_configured",
    );
    expect(fetch).not.toHaveBeenCalled();
  });
});

function oidcEnvSnapshot(): Record<string, string | undefined> {
  return {
    clientID: process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    clientSecret: process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
    issuer: process.env.OPENCLARION_IAM_OIDC_ISSUER,
    stateSigningKey: process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    standardClientID: process.env.OIDC_CLIENT_ID,
    standardClientSecret: process.env.OIDC_CLIENT_SECRET,
    standardClientAuthMethod: process.env.OIDC_CLIENT_AUTH_METHOD,
    standardIssuer: process.env.OIDC_ISSUER,
    standardRedirectURL: process.env.OIDC_REDIRECT_URL,
    standardScopes: process.env.OIDC_SCOPES,
    standardStateSigningKey: process.env.OIDC_STATE_SIGNING_KEY,
    standardUsePKCE: process.env.OIDC_USE_PKCE,
  };
}

function restoreOIDCEnv(snapshot: Record<string, string | undefined>) {
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_ID", snapshot.clientID);
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_SECRET", snapshot.clientSecret);
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
  restoreEnv("OIDC_SCOPES", snapshot.standardScopes);
  restoreEnv("OIDC_STATE_SIGNING_KEY", snapshot.standardStateSigningKey);
  restoreEnv("OIDC_USE_PKCE", snapshot.standardUsePKCE);
}

function clearStandardOIDCEnv() {
  delete process.env.OIDC_CLIENT_ID;
  delete process.env.OIDC_CLIENT_SECRET;
  delete process.env.OIDC_CLIENT_AUTH_METHOD;
  delete process.env.OIDC_ISSUER;
  delete process.env.OIDC_REDIRECT_URL;
  delete process.env.OIDC_SCOPES;
  delete process.env.OIDC_STATE_SIGNING_KEY;
  delete process.env.OIDC_USE_PKCE;
}

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
