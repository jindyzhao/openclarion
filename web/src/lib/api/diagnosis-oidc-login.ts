import { createCipheriv, createDecipheriv, createHash, randomBytes } from "node:crypto";

import type { NextResponse } from "next/server";

import { diagnosisRequestPublicOrigin, diagnosisSessionCookieSecure } from "./diagnosis-session";

const diagnosisOIDCStateCookieName = "openclarion_diagnosis_oidc_state";

const diagnosisOIDCStateCookiePath = "/api/diagnosis/auth/oidc/callback";
const diagnosisOIDCStateCookieMaxAgeSeconds = 10 * 60;
const defaultDiagnosisOIDCReturnTo = "/diagnosis-room";
const defaultDiagnosisOIDCScopes = "openid profile email";
const stateSigningKeyMinBytes = 32;
const stateSealVersion = "v1";
const gcmNonceBytes = 12;
const pkceVerifierBytes = 32;

type CookieResponse = Pick<NextResponse, "cookies">;
type ClientAuthMethod = "client_secret_basic" | "client_secret_post" | "none";

export type DiagnosisOIDCConfig = {
  clientAuthMethod?: ClientAuthMethod;
  clientID: string;
  clientSecret?: string;
  issuer: URL;
  redirectURL: URL;
  scopes: string;
  usePKCE: boolean;
};

export type DiagnosisOIDCDiscovery = {
  authorizationEndpoint: URL;
  issuer: string;
  tokenEndpoint: URL;
  tokenEndpointAuthMethodsSupported: ClientAuthMethod[];
};

export type DiagnosisOIDCStatePayload = {
  codeVerifier?: string;
  issuedAt: number;
  nonce: string;
  returnTo: string;
  state: string;
};

export type DiagnosisOIDCTokenResponse = {
  idToken: string;
};

export function diagnosisOIDCConfigFromEnv(request: Request): DiagnosisOIDCConfig | null {
  const issuer = oidcURLConfigValue(
    firstEnvValue(process.env.OPENCLARION_IAM_OIDC_ISSUER, process.env.OIDC_ISSUER, process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL),
    { allowSearch: false }
  );
  const clientID = oidcStringConfigValue(
    firstEnvValue(process.env.OPENCLARION_IAM_OIDC_CLIENT_ID, process.env.OIDC_CLIENT_ID, process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID)
  );
  if (issuer === null || clientID === null) {
    return null;
  }
  const redirectURL =
    oidcURLConfigValue(firstEnvValue(process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL, process.env.OIDC_REDIRECT_URL)) ??
    new URL(diagnosisOIDCStateCookiePath, diagnosisRequestPublicOrigin(request));
  const clientSecret = oidcStringConfigValue(
    firstEnvValue(process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET, process.env.OIDC_CLIENT_SECRET)
  );
  const rawAuthMethod = firstEnvValue(process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD, process.env.OIDC_CLIENT_AUTH_METHOD);
  const clientAuthMethod = oidcClientAuthMethod(rawAuthMethod);
  if ((rawAuthMethod ?? "").trim() !== "" && clientAuthMethod === null) {
    return null;
  }
  const scopes =
    oidcStringConfigValue(firstEnvValue(process.env.OPENCLARION_IAM_OIDC_SCOPES, process.env.OIDC_SCOPES)) ??
    defaultDiagnosisOIDCScopes;
  if (!scopeSet(scopes).has("openid")) {
    return null;
  }
  const usePKCE =
    (firstEnvValue(process.env.OPENCLARION_IAM_OIDC_USE_PKCE, process.env.OIDC_USE_PKCE) ?? "").trim().toLowerCase() !==
    "false";
  const publicClient = clientSecret === null && (clientAuthMethod === null || clientAuthMethod === "none");
  if ((clientAuthMethod === "none" || publicClient) && !usePKCE) {
    return null;
  }
  if (clientAuthMethod !== null && clientAuthMethod !== "none" && clientSecret === null) {
    return null;
  }
  return {
    clientAuthMethod: clientAuthMethod ?? undefined,
    clientID,
    clientSecret: clientSecret ?? undefined,
    issuer,
    redirectURL,
    scopes,
    usePKCE
  };
}

export function diagnosisOIDCStateSigningKey(): Buffer | null {
  const raw =
    firstEnvValue(
      process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
      process.env.OIDC_STATE_SIGNING_KEY,
      process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY
    ) ??
    "";
  const value = raw.trim();
  if (value === "" || value !== raw) {
    return null;
  }
  const key = Buffer.from(value, "utf8");
  return key.length < stateSigningKeyMinBytes ? null : key;
}

export function normalizedDiagnosisOIDCReturnTo(raw: string | null): string | null {
  const original = raw ?? defaultDiagnosisOIDCReturnTo;
  const value = original.trim();
  if (value === "" || value !== original) {
    return null;
  }
  let parsed: URL;
  try {
    parsed = new URL(value, "https://openclarion.local");
  } catch {
    return null;
  }
  if (parsed.origin !== "https://openclarion.local" || parsed.username !== "" || parsed.password !== "") {
    return null;
  }
  if (parsed.pathname !== "/diagnosis-room" && !parsed.pathname.startsWith("/diagnosis-room/")) {
    return null;
  }
  return `${parsed.pathname}${parsed.search}${parsed.hash}`;
}

export function diagnosisOIDCReturnToWithSessionContext(returnTo: string): string {
  const url = new URL(returnTo, "https://openclarion.local");
  url.searchParams.set("auth_mode", "session");
  return `${url.pathname}${url.search}${url.hash}`;
}

export function newDiagnosisOIDCStatePayload(returnTo: string, usePKCE: boolean): DiagnosisOIDCStatePayload {
  return {
    codeVerifier: usePKCE ? base64url(randomBytes(pkceVerifierBytes)) : undefined,
    issuedAt: Math.floor(Date.now() / 1000),
    nonce: base64url(randomBytes(16)),
    returnTo,
    state: base64url(randomBytes(16))
  };
}

export function setDiagnosisOIDCStateCookie(
  response: CookieResponse,
  request: Request,
  payload: DiagnosisOIDCStatePayload,
  key: Buffer
) {
  response.cookies.set({
    name: diagnosisOIDCStateCookieName,
    value: sealDiagnosisOIDCStatePayload(payload, key),
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: diagnosisOIDCStateCookiePath,
    expires: new Date(Date.now() + diagnosisOIDCStateCookieMaxAgeSeconds * 1000)
  });
}

export function expireDiagnosisOIDCStateCookie(response: CookieResponse, request: Request) {
  response.cookies.set({
    name: diagnosisOIDCStateCookieName,
    value: "",
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: diagnosisOIDCStateCookiePath,
    expires: new Date(0)
  });
}

export function diagnosisOIDCStateFromRequest(request: Request): string | null {
  const cookieHeader = request.headers.get("cookie") ?? "";
  for (const part of cookieHeader.split(";")) {
    const [rawName, ...rawValue] = part.split("=");
    if (rawName?.trim() !== diagnosisOIDCStateCookieName) {
      continue;
    }
    try {
      return decodeURIComponent(rawValue.join("="));
    } catch {
      return null;
    }
  }
  return null;
}

export function sealDiagnosisOIDCStatePayload(payload: DiagnosisOIDCStatePayload, key: Buffer): string {
  const nonce = randomBytes(gcmNonceBytes);
  const cipher = createCipheriv("aes-256-gcm", stateCipherKey(key), nonce);
  const ciphertext = Buffer.concat([cipher.update(JSON.stringify(payload), "utf8"), cipher.final()]);
  return [stateSealVersion, base64url(nonce), base64url(cipher.getAuthTag()), base64url(ciphertext)].join(".");
}

export function unsealDiagnosisOIDCStatePayload(raw: string | null, key: Buffer): DiagnosisOIDCStatePayload | null {
  if (raw === null) {
    return null;
  }
  const parts = raw.split(".");
  if (parts.length !== 4 || parts[0] !== stateSealVersion) {
    return null;
  }
  try {
    const nonce = Buffer.from(parts[1] ?? "", "base64url");
    const tag = Buffer.from(parts[2] ?? "", "base64url");
    const ciphertext = Buffer.from(parts[3] ?? "", "base64url");
    const decipher = createDecipheriv("aes-256-gcm", stateCipherKey(key), nonce);
    decipher.setAuthTag(tag);
    const plaintext = Buffer.concat([decipher.update(ciphertext), decipher.final()]);
    const parsed = JSON.parse(plaintext.toString("utf8")) as unknown;
    return isStatePayload(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function diagnosisOIDCDiscoveryURL(issuer: URL): URL {
  const url = new URL(issuer);
  url.pathname = `${url.pathname.replace(/\/+$/, "")}/.well-known/openid-configuration`;
  url.search = "";
  url.hash = "";
  return url;
}

export function normalizedDiagnosisOIDCDiscovery(value: unknown): DiagnosisOIDCDiscovery | null {
  if (!isRecord(value)) {
    return null;
  }
  const authorizationEndpoint = oidcURLConfigValue(value.authorization_endpoint);
  const tokenEndpoint = oidcURLConfigValue(value.token_endpoint);
  if (authorizationEndpoint === null || tokenEndpoint === null || typeof value.issuer !== "string") {
    return null;
  }
  const methods = Array.isArray(value.token_endpoint_auth_methods_supported)
    ? value.token_endpoint_auth_methods_supported.filter((method): method is ClientAuthMethod => oidcClientAuthMethod(method) !== null)
    : [];
  return {
    authorizationEndpoint,
    issuer: value.issuer,
    tokenEndpoint,
    tokenEndpointAuthMethodsSupported: methods
  };
}

export function diagnosisOIDCIssuerMatches(discovered: string, configured: URL): boolean {
  const normalized = oidcURLConfigValue(discovered, { allowSearch: false });
  return normalized !== null && normalized.toString() === configured.toString();
}

export function diagnosisOIDCLoginURL({
  config,
  discovery,
  payload
}: {
  config: DiagnosisOIDCConfig;
  discovery: DiagnosisOIDCDiscovery;
  payload: DiagnosisOIDCStatePayload;
}): URL {
  const url = new URL(discovery.authorizationEndpoint);
  url.searchParams.set("client_id", config.clientID);
  url.searchParams.set("redirect_uri", config.redirectURL.toString());
  url.searchParams.set("response_type", "code");
  url.searchParams.set("scope", config.scopes);
  url.searchParams.set("state", payload.state);
  url.searchParams.set("nonce", payload.nonce);
  if (payload.codeVerifier !== undefined) {
    url.searchParams.set("code_challenge", base64url(createHash("sha256").update(payload.codeVerifier).digest()));
    url.searchParams.set("code_challenge_method", "S256");
  }
  return url;
}

export function oidcTokenEndpointRequest({
  code,
  config,
  discovery,
  payload
}: {
  code: string;
  config: DiagnosisOIDCConfig;
  discovery: DiagnosisOIDCDiscovery;
  payload: DiagnosisOIDCStatePayload;
}): RequestInit {
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: config.redirectURL.toString()
  });
  if (payload.codeVerifier !== undefined) {
    body.set("code_verifier", payload.codeVerifier);
  }
  const method = selectedTokenAuthMethod(config, discovery);
  const headers = new Headers({ accept: "application/json", "content-type": "application/x-www-form-urlencoded" });
  if (method === "client_secret_basic") {
    const username = formURLEncodeOAuthCredential(config.clientID);
    const password = formURLEncodeOAuthCredential(config.clientSecret ?? "");
    headers.set("authorization", `Basic ${Buffer.from(`${username}:${password}`, "utf8").toString("base64")}`);
  } else {
    body.set("client_id", config.clientID);
    if (method === "client_secret_post") {
      body.set("client_secret", config.clientSecret ?? "");
    }
  }
  return { method: "POST", headers, body };
}

export function normalizedOIDCTokenResponse(value: unknown): DiagnosisOIDCTokenResponse | null {
  if (!isRecord(value) || typeof value.id_token !== "string" || value.id_token.trim() === "") {
    return null;
  }
  return { idToken: value.id_token };
}

export function oidcCallbackQueryToken(params: URLSearchParams, key: string, maxBytes: number): string | null {
  const values = params.getAll(key);
  if (values.length !== 1) {
    return null;
  }
  const raw = values[0] ?? "";
  const value = raw.trim();
  if (value === "" || value !== raw || Buffer.byteLength(value, "utf8") > maxBytes) {
    return null;
  }
  return value;
}

export function oidcIDTokenNonce(rawToken: string): string | null {
  const parts = rawToken.split(".");
  if (parts.length !== 3) {
    return null;
  }
  try {
    const payload = JSON.parse(Buffer.from(parts[1] ?? "", "base64url").toString("utf8")) as unknown;
    return isRecord(payload) && typeof payload.nonce === "string" ? payload.nonce : null;
  } catch {
    return null;
  }
}

function selectedTokenAuthMethod(config: DiagnosisOIDCConfig, discovery: DiagnosisOIDCDiscovery): ClientAuthMethod {
  if (config.clientAuthMethod !== undefined) {
    return config.clientAuthMethod;
  }
  const supported = discovery.tokenEndpointAuthMethodsSupported;
  if (config.clientSecret !== undefined && (supported.length === 0 || supported.includes("client_secret_basic"))) {
    return "client_secret_basic";
  }
  if (config.clientSecret !== undefined && supported.includes("client_secret_post")) {
    return "client_secret_post";
  }
  return "none";
}

function isStatePayload(value: unknown): value is DiagnosisOIDCStatePayload {
  return (
    isRecord(value) &&
    typeof value.issuedAt === "number" &&
    typeof value.nonce === "string" &&
    typeof value.returnTo === "string" &&
    typeof value.state === "string" &&
    (value.codeVerifier === undefined || typeof value.codeVerifier === "string")
  );
}

function stateCipherKey(key: Buffer): Buffer {
  return createHash("sha256").update(key).digest();
}

function firstEnvValue(...values: Array<string | undefined>): string | undefined {
  for (const value of values) {
    if (value !== undefined && value.trim() !== "") {
      return value;
    }
  }
  return undefined;
}

function oidcStringConfigValue(raw: unknown): string | null {
  if (typeof raw !== "string") {
    return null;
  }
  const value = raw.trim();
  return value === "" || value !== raw ? null : value;
}

function oidcURLConfigValue(raw: unknown, options: { allowSearch?: boolean } = {}): URL | null {
  if (typeof raw !== "string") {
    return null;
  }
  const value = raw.trim();
  if (value === "" || value !== raw) {
    return null;
  }
  try {
    const url = new URL(value);
    if ((url.protocol !== "https:" && url.protocol !== "http:") || url.username !== "" || url.password !== "" || url.hash !== "") {
      return null;
    }
    if (options.allowSearch === false && url.search !== "") {
      return null;
    }
    return url;
  } catch {
    return null;
  }
}

function oidcClientAuthMethod(raw: unknown): ClientAuthMethod | null {
  const value = typeof raw === "string" ? raw.trim() : "";
  switch (value) {
    case "client_secret_basic":
    case "client_secret_post":
    case "none":
      return value;
    case "":
      return null;
    default:
      return null;
  }
}

function scopeSet(scopes: string): Set<string> {
  return new Set(scopes.split(/[ \t\r\n]+/).map((scope) => scope.trim()).filter(Boolean));
}

function formURLEncodeOAuthCredential(value: string): string {
  const params = new URLSearchParams();
  params.set("v", value);
  return params.toString().slice("v=".length);
}

function base64url(buffer: Buffer): string {
  return buffer.toString("base64url");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
