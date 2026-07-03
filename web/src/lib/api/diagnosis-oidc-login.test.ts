import { afterEach, describe, expect, it, vi } from "vitest";

import {
  diagnosisBrowserSessionSigningKeyConfigured,
  diagnosisOIDCConfigFromEnv,
  diagnosisOIDCIssuerMatches,
  diagnosisOIDCLoginURL,
  diagnosisOIDCStateSigningKey,
  newDiagnosisOIDCStatePayload,
  normalizedOIDCTokenResponse,
  normalizedDiagnosisOIDCReturnTo,
  oidcIDTokenNonce,
  oidcTokenEndpointRequest,
  sealDiagnosisOIDCStatePayload,
  unsealDiagnosisOIDCStatePayload,
} from "./diagnosis-oidc-login";

describe("diagnosis OIDC login helpers", () => {
  const originalEnv = {
    legacyClientID: process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
    legacyIssuer: process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
    issuer: process.env.OPENCLARION_IAM_OIDC_ISSUER,
    clientID: process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    clientSecret: process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
    redirectURL: process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL,
    scopes: process.env.OPENCLARION_IAM_OIDC_SCOPES,
    stateSigningKey: process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    sessionSigningKey: process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY,
    clientAuthMethod: process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    usePKCE: process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
    standardClientID: process.env.OIDC_CLIENT_ID,
    standardClientSecret: process.env.OIDC_CLIENT_SECRET,
    standardClientAuthMethod: process.env.OIDC_CLIENT_AUTH_METHOD,
    standardIssuer: process.env.OIDC_ISSUER,
    standardRedirectURL: process.env.OIDC_REDIRECT_URL,
    standardScopes: process.env.OIDC_SCOPES,
    standardStateSigningKey: process.env.OIDC_STATE_SIGNING_KEY,
    standardUsePKCE: process.env.OIDC_USE_PKCE,
  };

  afterEach(() => {
    restoreEnv(
      "OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID",
      originalEnv.legacyClientID,
    );
    restoreEnv(
      "OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL",
      originalEnv.legacyIssuer,
    );
    restoreEnv("OPENCLARION_IAM_OIDC_ISSUER", originalEnv.issuer);
    restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_ID", originalEnv.clientID);
    restoreEnv("OPENCLARION_IAM_OIDC_CLIENT_SECRET", originalEnv.clientSecret);
    restoreEnv("OPENCLARION_IAM_OIDC_REDIRECT_URL", originalEnv.redirectURL);
    restoreEnv("OPENCLARION_IAM_OIDC_SCOPES", originalEnv.scopes);
    restoreEnv(
      "OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY",
      originalEnv.stateSigningKey,
    );
    restoreEnv(
      "OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY",
      originalEnv.sessionSigningKey,
    );
    restoreEnv(
      "OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD",
      originalEnv.clientAuthMethod,
    );
    restoreEnv("OPENCLARION_IAM_OIDC_USE_PKCE", originalEnv.usePKCE);
    restoreEnv("OIDC_CLIENT_ID", originalEnv.standardClientID);
    restoreEnv("OIDC_CLIENT_SECRET", originalEnv.standardClientSecret);
    restoreEnv("OIDC_CLIENT_AUTH_METHOD", originalEnv.standardClientAuthMethod);
    restoreEnv("OIDC_ISSUER", originalEnv.standardIssuer);
    restoreEnv("OIDC_REDIRECT_URL", originalEnv.standardRedirectURL);
    restoreEnv("OIDC_SCOPES", originalEnv.standardScopes);
    restoreEnv("OIDC_STATE_SIGNING_KEY", originalEnv.standardStateSigningKey);
    restoreEnv("OIDC_USE_PKCE", originalEnv.standardUsePKCE);
    vi.useRealTimers();
  });

  it("loads standard OIDC config and builds a dynamic callback URL", () => {
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET = "client-secret-1";
    process.env.OPENCLARION_IAM_OIDC_SCOPES = "openid profile email roles";

    const config = diagnosisOIDCConfigFromEnv(
      new Request("http://console.example.com/diagnosis-room", {
        headers: { "x-forwarded-proto": "https" },
      }),
    );

    expect(config).toMatchObject({
      clientID: "openclarion",
      clientSecret: "client-secret-1",
      scopes: "openid profile email roles",
      usePKCE: true,
    });
    expect(config?.issuer.toString()).toBe("https://iam.example.com/");
    expect(config?.redirectURL.toString()).toBe(
      "https://console.example.com/api/diagnosis/auth/oidc/callback",
    );
  });

  it("prefers IAM and standard OIDC config over legacy diagnosis OIDC aliases", () => {
    delete process.env.OPENCLARION_IAM_OIDC_ISSUER;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;
    process.env.OIDC_ISSUER = "https://iam-standard.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-standard";
    process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL =
      "https://legacy.example.com";
    process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID = "legacy-openclarion";

    const standardConfig = diagnosisOIDCConfigFromEnv(
      new Request("https://console.example.com"),
    );

    expect(standardConfig?.issuer.toString()).toBe(
      "https://iam-standard.example.com/",
    );
    expect(standardConfig?.clientID).toBe("openclarion-standard");

    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion-iam";

    const iamConfig = diagnosisOIDCConfigFromEnv(
      new Request("https://console.example.com"),
    );

    expect(iamConfig?.issuer.toString()).toBe("https://iam.example.com/");
    expect(iamConfig?.clientID).toBe("openclarion-iam");
  });

  it("loads standard OIDC optional aliases for browser sign-in", () => {
    delete process.env.OPENCLARION_IAM_OIDC_ISSUER;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD;
    delete process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL;
    delete process.env.OPENCLARION_IAM_OIDC_SCOPES;
    delete process.env.OPENCLARION_IAM_OIDC_USE_PKCE;
    delete process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY;
    process.env.OIDC_ISSUER = "https://iam-standard.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-standard";
    process.env.OIDC_CLIENT_SECRET = "standard-client-secret";
    process.env.OIDC_CLIENT_AUTH_METHOD = "client_secret_post";
    process.env.OIDC_REDIRECT_URL =
      "https://console.example.com/api/diagnosis/auth/oidc/callback";
    process.env.OIDC_SCOPES = "openid profile email phone";
    process.env.OIDC_USE_PKCE = "false";
    process.env.OIDC_STATE_SIGNING_KEY = "c".repeat(32);

    const config = diagnosisOIDCConfigFromEnv(
      new Request("https://console.example.com"),
    );

    expect(config).toMatchObject({
      clientAuthMethod: "client_secret_post",
      clientID: "openclarion-standard",
      clientSecret: "standard-client-secret",
      scopes: "openid profile email phone",
      usePKCE: false,
    });
    expect(config?.issuer.toString()).toBe("https://iam-standard.example.com/");
    expect(config?.redirectURL.toString()).toBe(
      "https://console.example.com/api/diagnosis/auth/oidc/callback",
    );
    expect(diagnosisOIDCStateSigningKey()?.length).toBe(32);
  });

  it("uses least-privilege standard OIDC scopes by default", () => {
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    delete process.env.OPENCLARION_IAM_OIDC_SCOPES;

    const config = diagnosisOIDCConfigFromEnv(
      new Request("https://console.example.com"),
    );

    expect(config?.scopes).toBe("openid profile email");
  });

  it("rejects invalid OIDC client auth method configuration", () => {
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD = "client_secret_jwt";

    expect(
      diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
    ).toBeNull();
  });

  it("rejects OIDC configuration that cannot complete the browser code flow", () => {
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "https://iam.example.com";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";

    process.env.OPENCLARION_IAM_OIDC_SCOPES = "profile email phone";
    expect(
      diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
    ).toBeNull();
    process.env.OPENCLARION_IAM_OIDC_SCOPES = "openid roles";
    expect(
      diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
    ).toBeNull();

    process.env.OPENCLARION_IAM_OIDC_SCOPES = "openid profile email phone";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD = "client_secret_basic";
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET;
    expect(
      diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
    ).toBeNull();

    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD = "none";
    process.env.OPENCLARION_IAM_OIDC_USE_PKCE = "false";
    expect(
      diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
    ).toBeNull();
  });

  it("ignores blank higher-priority OIDC aliases before reading standard values", () => {
    process.env.OPENCLARION_IAM_OIDC_ISSUER = "";
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "";
    process.env.OIDC_ISSUER = "https://iam-standard.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion-standard";
    process.env.OIDC_SCOPES = "openid profile email phone";

    const config = diagnosisOIDCConfigFromEnv(
      new Request("https://console.example.com"),
    );

    expect(config?.issuer.toString()).toBe("https://iam-standard.example.com/");
    expect(config?.clientID).toBe("openclarion-standard");
  });

  it("rejects unsafe OIDC issuer URL configuration", () => {
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID = "openclarion";
    for (const issuer of [
      "https://iam.example.com?tenant=ops",
      "https://operator:secret@iam.example.com",
      "https://iam.example.com#fragment",
    ]) {
      process.env.OPENCLARION_IAM_OIDC_ISSUER = issuer;
      expect(
        diagnosisOIDCConfigFromEnv(new Request("http://console.example.com")),
      ).toBeNull();
    }
  });

  it("seals state cookies and rejects tampered or expired state", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-25T02:00:00Z"));
    const key = Buffer.from("a".repeat(32), "utf8");
    const payload = newDiagnosisOIDCStatePayload(
      "/diagnosis-room?auth_mode=session",
      true,
    );

    const sealed = sealDiagnosisOIDCStatePayload(payload, key);

    expect(sealed.split(".")[0]).toBe("v1");
    expect(sealed).not.toContain(
      Buffer.from(JSON.stringify(payload), "utf8").toString("base64url"),
    );
    expect(unsealDiagnosisOIDCStatePayload(sealed, key)).toEqual(payload);
    expect(unsealDiagnosisOIDCStatePayload(`${sealed}x`, key)).toBeNull();
    expect(
      unsealDiagnosisOIDCStatePayload(
        sealed,
        Buffer.from("b".repeat(32), "utf8"),
      ),
    ).toBeNull();
    vi.setSystemTime(new Date("2026-06-25T02:11:00Z"));
    expect(unsealDiagnosisOIDCStatePayload(sealed, key)).toBeNull();
  });

  it("rejects unsafe return paths and extracts nonce from ID tokens", () => {
    expect(normalizedDiagnosisOIDCReturnTo(null)).toBe("/diagnosis-room");
    expect(
      normalizedDiagnosisOIDCReturnTo(
        "/diagnosis-room?session_id=room-1&auth_mode=session",
      ),
    ).toBe("/diagnosis-room?session_id=room-1&auth_mode=session");
    expect(normalizedDiagnosisOIDCReturnTo("/settings/directory-rbac")).toBe(
      "/settings/directory-rbac",
    );
    expect(normalizedDiagnosisOIDCReturnTo("//evil.example.com")).toBeNull();
    expect(normalizedDiagnosisOIDCReturnTo("/diagnosis-room#token")).toBeNull();
    expect(oidcIDTokenNonce(fakeIDToken("nonce-1"))).toBe("nonce-1");
    expect(oidcIDTokenNonce("not-a-token")).toBeNull();
  });

  it("matches issuer identifiers across trailing slash normalization", () => {
    expect(
      diagnosisOIDCIssuerMatches(
        "https://iam.example.com",
        new URL("https://iam.example.com/"),
      ),
    ).toBe(true);
    expect(
      diagnosisOIDCIssuerMatches(
        "https://iam.example.com/realms/ops",
        new URL("https://iam.example.com/realms/ops"),
      ),
    ).toBe(true);
    expect(
      diagnosisOIDCIssuerMatches(
        "https://iam.example.com/other",
        new URL("https://iam.example.com/realms/ops"),
      ),
    ).toBe(false);
    expect(
      diagnosisOIDCIssuerMatches(
        "https://iam.example.com?tenant=ops",
        new URL("https://iam.example.com/"),
      ),
    ).toBe(false);
  });

  it("builds an authorization URL with nonce, state, and PKCE", () => {
    const config = {
      clientID: "openclarion",
      issuer: new URL("https://iam.example.com"),
      redirectURL: new URL(
        "https://console.example.com/api/diagnosis/auth/oidc/callback",
      ),
      scopes: "openid profile email roles",
      usePKCE: true,
    };
    const payload = {
      codeVerifier: "verifier-1",
      issuedAt: Date.now(),
      nonce: "nonce-1",
      returnTo: "/diagnosis-room?auth_mode=session",
      state: "state-1",
    };

    const url = diagnosisOIDCLoginURL({
      config,
      discovery: {
        authorizationEndpoint: new URL(
          "https://iam.example.com/oauth/authorize",
        ),
        issuer: "https://iam.example.com/",
        tokenEndpoint: new URL("https://iam.example.com/oauth/token"),
        tokenEndpointAuthMethodsSupported: [],
      },
      payload,
    });

    expect(url.origin).toBe("https://iam.example.com");
    expect(url.searchParams.get("client_id")).toBe("openclarion");
    expect(url.searchParams.get("nonce")).toBe("nonce-1");
    expect(url.searchParams.get("state")).toBe("state-1");
    expect(url.searchParams.get("code_challenge_method")).toBe("S256");
    expect(url.searchParams.get("code_challenge")).not.toBe("");
  });

  it("requires a sufficiently long state signing key", () => {
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY = "short";
    expect(diagnosisOIDCStateSigningKey()).toBeNull();
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY = "b".repeat(32);
    expect(diagnosisOIDCStateSigningKey()?.length).toBe(32);
  });

  it("requires a sufficiently long browser session signing key for BFF readiness", () => {
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "short";
    expect(diagnosisBrowserSessionSigningKeyConfigured()).toBe(false);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = " c".repeat(16);
    expect(diagnosisBrowserSessionSigningKeyConfigured()).toBe(false);
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "c".repeat(32);
    expect(diagnosisBrowserSessionSigningKeyConfigured()).toBe(true);
  });

  it("normalizes OIDC token responses with optional access tokens", () => {
    expect(
      normalizedOIDCTokenResponse({
        access_token: "access.token.one",
        id_token: "id.token.one",
        token_type: "Bearer",
      }),
    ).toEqual({
      accessToken: "access.token.one",
      idToken: "id.token.one",
    });
    expect(
      normalizedOIDCTokenResponse({
        id_token: "id.token.one",
      }),
    ).toEqual({ idToken: "id.token.one" });
    expect(
      normalizedOIDCTokenResponse({
        access_token: "access token",
        id_token: "id.token.one",
        token_type: "Bearer",
      }),
    ).toBeNull();
    expect(
      normalizedOIDCTokenResponse({
        access_token: "access.token.one",
        id_token: "id.token.one",
        token_type: "MAC",
      }),
    ).toBeNull();
    expect(
      normalizedOIDCTokenResponse({
        id_token: "id token one",
      }),
    ).toBeNull();
    expect(
      normalizedOIDCTokenResponse({
        id_token: `id.token.${"x".repeat(8193)}`,
      }),
    ).toBeNull();
  });

  it("builds token requests with supported OIDC client auth methods", () => {
    const baseConfig = {
      clientID: "openclarion",
      clientSecret: "client-secret-1",
      issuer: new URL("https://iam.example.com"),
      redirectURL: new URL(
        "https://console.example.com/api/diagnosis/auth/oidc/callback",
      ),
      scopes: "openid profile email roles",
      usePKCE: true,
    };
    const payload = {
      codeVerifier: "verifier-1",
      issuedAt: Date.now(),
      nonce: "nonce-1",
      returnTo: "/diagnosis-room?auth_mode=session",
      state: "state-1",
    };
    const discovery = {
      authorizationEndpoint: new URL("https://iam.example.com/oauth/authorize"),
      issuer: "https://iam.example.com/",
      tokenEndpoint: new URL("https://iam.example.com/oauth/token"),
      tokenEndpointAuthMethodsSupported: [
        "client_secret_basic" as const,
        "client_secret_post" as const,
      ],
    };

    const basic = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config: baseConfig,
      discovery,
      payload,
    });
    expect(basic?.headers.get("authorization")).toBe(
      `Basic ${Buffer.from("openclarion:client-secret-1", "utf8").toString("base64")}`,
    );
    expect(basic?.body.get("client_secret")).toBeNull();

    const encodedBasic = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config: {
        ...baseConfig,
        clientID: "open:clarion",
        clientSecret: "client secret",
      },
      discovery,
      payload,
    });
    expect(encodedBasic?.headers.get("authorization")).toBe(
      `Basic ${Buffer.from("open%3Aclarion:client+secret", "utf8").toString("base64")}`,
    );

    const post = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config: { ...baseConfig, clientAuthMethod: "client_secret_post" },
      discovery,
      payload,
    });
    expect(post?.headers.get("authorization")).toBeNull();
    expect(post?.body.get("client_id")).toBe("openclarion");
    expect(post?.body.get("client_secret")).toBe("client-secret-1");

    const publicClient = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config: { ...baseConfig, clientSecret: undefined },
      discovery,
      payload,
    });
    expect(publicClient?.headers.get("authorization")).toBeNull();
    expect(publicClient?.body.get("client_id")).toBe("openclarion");
    expect(publicClient?.body.get("client_secret")).toBeNull();

    const missingSecret = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config: {
        ...baseConfig,
        clientAuthMethod: "client_secret_basic",
        clientSecret: undefined,
      },
      discovery,
      payload,
    });
    expect(missingSecret).toBeNull();
  });
});

function fakeIDToken(nonce: string): string {
  return [
    Buffer.from(JSON.stringify({ alg: "none" })).toString("base64url"),
    Buffer.from(JSON.stringify({ nonce })).toString("base64url"),
    "signature",
  ].join(".");
}

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
