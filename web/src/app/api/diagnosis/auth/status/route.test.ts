import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

describe("diagnosis auth status route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;
  const originalOIDCEnv = oidcEnvSnapshot();

  beforeEach(() => {
    clearOIDCEnv();
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          configured: true,
          mode: "ldap",
          supported_modes: ["ldap"],
          transport_policy: { security: "start_tls" },
        }),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    restoreOIDCEnv(originalOIDCEnv);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("fetches backend diagnosis auth status without forwarding credentials", async () => {
    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      configured: true,
      mode: "ldap",
      supported_modes: ["ldap"],
      transport_policy: { security: "start_tls" },
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/auth/status",
    );
    expect(init.method).toBe("GET");
    expect(init.body).toBeUndefined();

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBeNull();
  });

  it("returns only validated non-sensitive status fields", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({
        configured: true,
        mode: "ldap",
        role_mapping: {
          admin_mapping_count: 1,
          configured: true,
          default_roles: ["owner"],
          owner_mapping_count: 2,
          secret_user_ids: ["operator-1"],
        },
        supported_modes: ["ldap"],
        transport_policy: {
          endpoint: "ldaps://secret.example.test",
          security: "tls",
        },
        token_like_extra: "must-not-return",
      }),
    );

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      configured: true,
      mode: "ldap",
      role_mapping: {
        admin_mapping_count: 1,
        configured: true,
        default_roles: ["owner"],
        owner_mapping_count: 2,
      },
      supported_modes: ["ldap"],
      transport_policy: { security: "tls" },
    });
  });

  it("rejects malformed backend diagnosis auth status", async () => {
    for (const body of [
      {
        configured: true,
        mode: "desktop",
        supported_modes: ["oidc"],
      },
      {
        configured: true,
        mode: "oidc",
        supported_modes: ["oidc", "desktop"],
      },
      {
        configured: true,
        mode: "oidc",
        supported_modes: ["none"],
      },
      {
        configured: true,
        mode: "oidc",
        oidc_bff: { status: "ready" },
      },
      {
        configured: true,
        mode: "oidc",
        role_mapping: {
          admin_mapping_count: -1,
          configured: true,
          default_roles: ["owner"],
          owner_mapping_count: 0,
        },
        supported_modes: ["oidc"],
      },
      {
        configured: true,
        mode: "oidc",
        role_mapping: {
          admin_mapping_count: 0,
          configured: true,
          default_roles: ["viewer"],
          owner_mapping_count: 0,
        },
        supported_modes: ["oidc"],
      },
      {
        configured: true,
        mode: "ldap",
        supported_modes: ["ldap"],
        transport_policy: { security: "plain" },
      },
    ]) {
      vi.mocked(fetch).mockResolvedValueOnce(Response.json(body));

      const response = await GET(statusRequest());

      expect(response.status).toBe(502);
      await expect(response.json()).resolves.toEqual({
        error: "diagnosis auth status response is invalid",
      });
    }
  });

  it("adds non-sensitive OIDC BFF readiness when the backend advertises OIDC", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({
        configured: true,
        mode: "oidc",
        supported_modes: ["oidc"],
      }),
    );

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      configured: true,
      mode: "oidc",
      oidc_bff: {
        browser_session_signing_key_configured: false,
        client_auth_method: "auto",
        client_id_configured: false,
        client_secret_configured: false,
        configured: false,
        issuer_configured: false,
        missing: [
          "issuer",
          "client_id",
          "state_signing_key",
          "session_signing_key",
        ],
        pkce_enabled: true,
        redirect_url_configured: false,
        scopes_include_openid: true,
        state_signing_key_configured: false,
        status: "blocked",
      },
      supported_modes: ["oidc"],
    });
  });

  it("reports ready OIDC BFF wiring without exposing configured values", async () => {
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-web";
    process.env.OIDC_CLIENT_SECRET = "client-secret";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_basic";
    process.env.OIDC_REDIRECT_URL =
      "https://console.example.com/api/diagnosis/auth/oidc/callback";
    process.env.OIDC_SCOPES = "openid profile email phone";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "b".repeat(32);

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.oidc_bff).toEqual({
      browser_session_signing_key_configured: true,
      client_auth_method: "client_secret_basic",
      client_id_configured: true,
      client_secret_configured: true,
      configured: true,
      issuer_configured: true,
      missing: [],
      pkce_enabled: true,
      redirect_url_configured: true,
      scopes_include_openid: true,
      state_signing_key_configured: true,
      status: "ready",
    });
    expect(JSON.stringify(body)).not.toContain("client-secret");
    expect(JSON.stringify(body)).not.toContain("openclarion-web");
    expect(JSON.stringify(body)).not.toContain("iam.example.com");
  });

  it("blocks OIDC BFF readiness when configured scopes cannot request ID tokens", async () => {
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-web";
    process.env.OIDC_CLIENT_SECRET = "client-secret";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_basic";
    process.env.OIDC_SCOPES = "profile email phone";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "b".repeat(32);

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.oidc_bff).toMatchObject({
      client_auth_method: "client_secret_basic",
      client_id_configured: true,
      client_secret_configured: true,
      configured: false,
      issuer_configured: true,
      missing: ["openid_scope"],
      scopes_include_openid: false,
      state_signing_key_configured: true,
      status: "blocked",
    });
    expect(JSON.stringify(body)).not.toContain("profile email phone");
  });

  it("blocks OIDC BFF readiness when configured scopes miss the standard profile contract", async () => {
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-web";
    process.env.OIDC_CLIENT_SECRET = "client-secret";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_basic";
    process.env.OIDC_SCOPES = "openid roles";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "b".repeat(32);

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.oidc_bff).toMatchObject({
      client_auth_method: "client_secret_basic",
      configured: false,
      missing: ["profile_scope", "email_scope"],
      scopes_include_openid: true,
      status: "blocked",
    });
    expect(JSON.stringify(body)).not.toContain("openid roles");
  });

  it("blocks OIDC BFF readiness when explicit client secret auth lacks a secret", async () => {
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-web";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_post";
    process.env.OIDC_SCOPES = "openid profile email phone";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "b".repeat(32);

    const response = await GET(statusRequest());

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      oidc_bff: {
        client_auth_method: "client_secret_post",
        client_id_configured: true,
        client_secret_configured: false,
        configured: false,
        missing: ["client_secret"],
        status: "blocked",
      },
    });
  });

  it("blocks invalid or unsafe public OIDC BFF client settings", async () => {
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-web";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_jwt";
    process.env.OIDC_SCOPES = "openid profile email phone";
    process.env.OIDC_STATE_SIGNING_KEY = "s".repeat(32);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "b".repeat(32);

    const invalidMethodResponse = await GET(statusRequest());

    expect(invalidMethodResponse.status).toBe(200);
    await expect(invalidMethodResponse.json()).resolves.toMatchObject({
      oidc_bff: {
        client_auth_method: "invalid",
        configured: false,
        missing: ["client_auth_method"],
        status: "blocked",
      },
    });

    process.env.OIDC_CLIENT_AUTH_METHOD = "none";
    process.env.OIDC_USE_PKCE = "false";

    const publicClientResponse = await GET(statusRequest());

    expect(publicClientResponse.status).toBe(200);
    await expect(publicClientResponse.json()).resolves.toMatchObject({
      oidc_bff: {
        client_auth_method: "none",
        configured: false,
        missing: ["pkce"],
        pkce_enabled: false,
        status: "blocked",
      },
    });
  });
});

function statusRequest(): Request {
  return new Request("https://console.example.com/api/diagnosis/auth/status");
}

function oidcEnvSnapshot(): Record<string, string | undefined> {
  return {
    clientAuthMethod: process.env.OIDC_CLIENT_AUTH_METHOD,
    clientID: process.env.OIDC_CLIENT_ID,
    clientSecret: process.env.OIDC_CLIENT_SECRET,
    issuer: process.env.OIDC_ISSUER,
    redirectURL: process.env.OIDC_REDIRECT_URL,
    scopes: process.env.OIDC_SCOPES,
    stateSigningKey: process.env.OIDC_STATE_SIGNING_KEY,
    usePKCE: process.env.OIDC_USE_PKCE,
    iamClientAuthMethod: process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    iamClientID: process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    iamClientSecret: process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
    iamIssuer: process.env.OPENCLARION_IAM_OIDC_ISSUER,
    iamRedirectURL: process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL,
    iamScopes: process.env.OPENCLARION_IAM_OIDC_SCOPES,
    iamStateSigningKey: process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    iamUsePKCE: process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
    legacyClientID: process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
    legacyIssuer: process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
    sessionSigningKey: process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY,
  };
}

function restoreOIDCEnv(snapshot: Record<string, string | undefined>) {
  restoreEnv("OIDC_CLIENT_AUTH_METHOD", snapshot.clientAuthMethod);
  restoreEnv("OIDC_CLIENT_ID", snapshot.clientID);
  restoreEnv("OIDC_CLIENT_SECRET", snapshot.clientSecret);
  restoreEnv("OIDC_ISSUER", snapshot.issuer);
  restoreEnv("OIDC_REDIRECT_URL", snapshot.redirectURL);
  restoreEnv("OIDC_SCOPES", snapshot.scopes);
  restoreEnv("OIDC_STATE_SIGNING_KEY", snapshot.stateSigningKey);
  restoreEnv("OIDC_USE_PKCE", snapshot.usePKCE);
  restoreEnv(
    "OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD",
    snapshot.iamClientAuthMethod,
  );
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_ID", snapshot.iamClientID);
  restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_SECRET", snapshot.iamClientSecret);
  restoreEnv("OPENCLARION_IAM_OIDC_ISSUER", snapshot.iamIssuer);
  restoreEnv("OPENCLARION_IAM_OIDC_REDIRECT_URL", snapshot.iamRedirectURL);
  restoreEnv("OPENCLARION_IAM_OIDC_SCOPES", snapshot.iamScopes);
  restoreEnv(
    "OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY",
    snapshot.iamStateSigningKey,
  );
  restoreEnv("OPENCLARION_IAM_OIDC_USE_PKCE", snapshot.iamUsePKCE);
  restoreEnv("OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID", snapshot.legacyClientID);
  restoreEnv("OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL", snapshot.legacyIssuer);
  restoreEnv(
    "OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY",
    snapshot.sessionSigningKey,
  );
}

function clearOIDCEnv() {
  restoreOIDCEnv({});
}

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
