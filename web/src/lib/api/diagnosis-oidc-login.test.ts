import { Buffer } from "node:buffer";

import { afterEach, describe, expect, it } from "vitest";

import {
  diagnosisOIDCConfigFromEnv,
  diagnosisOIDCLoginURL,
  diagnosisOIDCDiscoveryURL,
  diagnosisOIDCStateSigningKey,
  normalizedDiagnosisOIDCReturnTo,
  oidcTokenEndpointRequest,
  sealDiagnosisOIDCStatePayload,
  unsealDiagnosisOIDCStatePayload,
  type DiagnosisOIDCConfig,
  type DiagnosisOIDCDiscovery,
  type DiagnosisOIDCStatePayload
} from "./diagnosis-oidc-login";

describe("diagnosis OIDC login helpers", () => {
  const originalEnv = {
    OPENCLARION_IAM_OIDC_CLIENT_ID: process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    OPENCLARION_IAM_OIDC_ISSUER: process.env.OPENCLARION_IAM_OIDC_ISSUER,
    OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY: process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    OPENCLARION_IAM_OIDC_USE_PKCE: process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
    OIDC_CLIENT_AUTH_METHOD: process.env.OIDC_CLIENT_AUTH_METHOD,
    OIDC_CLIENT_ID: process.env.OIDC_CLIENT_ID,
    OIDC_CLIENT_SECRET: process.env.OIDC_CLIENT_SECRET,
    OIDC_ISSUER: process.env.OIDC_ISSUER,
    OIDC_STATE_SIGNING_KEY: process.env.OIDC_STATE_SIGNING_KEY,
    OIDC_USE_PKCE: process.env.OIDC_USE_PKCE,
    OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID: process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
    OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL: process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
    OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY: process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY
  };

  afterEach(() => {
    for (const [key, value] of Object.entries(originalEnv)) {
      restoreEnv(key, value);
    }
  });

  it("accepts only diagnosis-room relative return paths", () => {
    expect(normalizedDiagnosisOIDCReturnTo("/diagnosis-room?session_id=room-1")).toBe(
      "/diagnosis-room?session_id=room-1"
    );
    expect(normalizedDiagnosisOIDCReturnTo("https://evil.example/diagnosis-room")).toBeNull();
    expect(normalizedDiagnosisOIDCReturnTo("/settings")).toBeNull();
    expect(normalizedDiagnosisOIDCReturnTo("/diagnosis-room-extra")).toBeNull();
    expect(normalizedDiagnosisOIDCReturnTo(" /diagnosis-room")).toBeNull();
  });

  it("seals and unseals state with authenticated encryption", () => {
    const key = Buffer.from("s".repeat(32), "utf8");
    const payload: DiagnosisOIDCStatePayload = {
      codeVerifier: "verifier-1",
      issuedAt: 1782813600,
      nonce: "nonce-1",
      returnTo: "/diagnosis-room?auth_mode=session",
      state: "state-1"
    };

    const sealed = sealDiagnosisOIDCStatePayload(payload, key);

    expect(unsealDiagnosisOIDCStatePayload(sealed, key)).toEqual(payload);
    const parts = sealed.split(".");
    const tag = parts[2] ?? "";
    parts[2] = tag === "A".repeat(tag.length) ? "B".repeat(tag.length) : "A".repeat(tag.length);
    expect(unsealDiagnosisOIDCStatePayload(parts.join("."), key)).toBeNull();
  });

  it("builds authorization and token requests without exposing client secret in the URL", () => {
    const config: DiagnosisOIDCConfig = {
      clientAuthMethod: "client_secret_basic",
      clientID: "open:clarion client",
      clientSecret: "client+secret-1",
      issuer: new URL("https://iam.example.com"),
      redirectURL: new URL("https://console.example.com/api/diagnosis/auth/oidc/callback"),
      scopes: "openid profile email",
      usePKCE: true
    };
    const discovery: DiagnosisOIDCDiscovery = {
      authorizationEndpoint: new URL("https://iam.example.com/oauth/authorize"),
      issuer: "https://iam.example.com",
      tokenEndpoint: new URL("https://iam.example.com/oauth/token"),
      tokenEndpointAuthMethodsSupported: ["client_secret_basic"]
    };
    const payload: DiagnosisOIDCStatePayload = {
      codeVerifier: "code-verifier-1",
      issuedAt: 1782813600,
      nonce: "nonce-1",
      returnTo: "/diagnosis-room",
      state: "state-1"
    };

    const loginURL = diagnosisOIDCLoginURL({ config, discovery, payload });
    expect(loginURL.searchParams.get("client_id")).toBe("open:clarion client");
    expect(loginURL.searchParams.get("client_secret")).toBeNull();
    expect(loginURL.searchParams.get("code_challenge_method")).toBe("S256");

    const request = oidcTokenEndpointRequest({
      code: "callback-code-1",
      config,
      discovery,
      payload
    });
    const headers = new Headers(request.headers);
    const body = request.body as URLSearchParams;

    expect(headers.get("authorization")).toBe(
      `Basic ${Buffer.from("open%3Aclarion+client:client%2Bsecret-1", "utf8").toString("base64")}`
    );
    expect(body.get("client_secret")).toBeNull();
    expect(body.get("code_verifier")).toBe("code-verifier-1");
  });

  it("rejects inferred public clients when PKCE is disabled", () => {
    delete process.env.OPENCLARION_IAM_OIDC_ISSUER;
    delete process.env.OPENCLARION_IAM_OIDC_CLIENT_ID;
    delete process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL;
    delete process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID;
    delete process.env.OIDC_CLIENT_AUTH_METHOD;
    delete process.env.OIDC_CLIENT_SECRET;
    process.env.OIDC_ISSUER = "https://iam.example.com";
    process.env.OIDC_CLIENT_ID = "openclarion";
    process.env.OIDC_USE_PKCE = "false";

    expect(diagnosisOIDCConfigFromEnv(new Request("https://console.example.com/diagnosis-room"))).toBeNull();
  });

  it("skips blank state signing key aliases before using lower-priority fallbacks", () => {
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY = "";
    process.env.OIDC_STATE_SIGNING_KEY = "   ";
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY = "fallback-state-signing-key-32-bytes";

    expect(diagnosisOIDCStateSigningKey()?.toString("utf8")).toBe("fallback-state-signing-key-32-bytes");
  });

  it("preserves issuer paths when building the discovery URL", () => {
    expect(diagnosisOIDCDiscoveryURL(new URL("https://iam.example.com/realms/openclarion")).toString()).toBe(
      "https://iam.example.com/realms/openclarion/.well-known/openid-configuration"
    );
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
