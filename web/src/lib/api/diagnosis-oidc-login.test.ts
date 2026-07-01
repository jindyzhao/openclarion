import { Buffer } from "node:buffer";

import { describe, expect, it } from "vitest";

import {
  diagnosisOIDCLoginURL,
  diagnosisOIDCDiscoveryURL,
  normalizedDiagnosisOIDCReturnTo,
  oidcTokenEndpointRequest,
  sealDiagnosisOIDCStatePayload,
  unsealDiagnosisOIDCStatePayload,
  type DiagnosisOIDCConfig,
  type DiagnosisOIDCDiscovery,
  type DiagnosisOIDCStatePayload
} from "./diagnosis-oidc-login";

describe("diagnosis OIDC login helpers", () => {
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
      clientID: "openclarion",
      clientSecret: "client-secret-1",
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
    expect(loginURL.searchParams.get("client_id")).toBe("openclarion");
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
      `Basic ${Buffer.from("openclarion:client-secret-1", "utf8").toString("base64")}`
    );
    expect(body.get("client_secret")).toBeNull();
    expect(body.get("code_verifier")).toBe("code-verifier-1");
  });

  it("preserves issuer paths when building the discovery URL", () => {
    expect(diagnosisOIDCDiscoveryURL(new URL("https://iam.example.com/realms/openclarion")).toString()).toBe(
      "https://iam.example.com/realms/openclarion/.well-known/openid-configuration"
    );
  });
});
