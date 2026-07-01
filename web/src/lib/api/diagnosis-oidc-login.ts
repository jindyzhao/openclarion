import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
} from "node:crypto";

import type { NextResponse } from "next/server";

import {
  diagnosisRequestPublicOrigin,
  diagnosisSessionCookieSecure,
} from "./diagnosis-session";

export const diagnosisOIDCStateCookieName = "openclarion_diagnosis_oidc_state";
const diagnosisOIDCStateCookiePath = "/api/diagnosis/auth/oidc/callback";
const diagnosisOIDCStateCookieMaxAgeSeconds = 10 * 60;

export type DiagnosisOIDCConfig = {
  clientAuthMethod?: DiagnosisOIDCClientAuthMethod;
  clientID: string;
  clientSecret?: string;
  issuer: URL;
  redirectURL: URL;
  scopes: string;
  usePKCE: boolean;
};

type DiagnosisOIDCClientAuthMethod =
  "client_secret_basic" | "client_secret_post" | "none";

type DiagnosisOIDCBFFClientAuthMethod =
  DiagnosisOIDCClientAuthMethod | "auto" | "invalid";

export type DiagnosisOIDCDiscovery = {
  authorizationEndpoint: URL;
  issuer: string;
  tokenEndpoint: URL;
  tokenEndpointAuthMethodsSupported: DiagnosisOIDCClientAuthMethod[];
};

export type DiagnosisOIDCStatePayload = {
  codeVerifier?: string;
  issuedAt: number;
  nonce: string;
  returnTo: string;
  state: string;
};

export type DiagnosisOIDCTokenResponse = {
  accessToken?: string;
  idToken: string;
};

export type DiagnosisOIDCBFFReadiness = {
  browser_session_signing_key_configured: boolean;
  client_auth_method: DiagnosisOIDCBFFClientAuthMethod;
  client_id_configured: boolean;
  client_secret_configured: boolean;
  configured: boolean;
  issuer_configured: boolean;
  missing: Array<
    | "client_id"
    | "client_auth_method"
    | "client_secret"
    | "email_scope"
    | "issuer"
    | "openid_scope"
    | "pkce"
    | "profile_scope"
    | "session_signing_key"
    | "state_signing_key"
  >;
  pkce_enabled: boolean;
  redirect_url_configured: boolean;
  scopes_include_openid: boolean;
  state_signing_key_configured: boolean;
  status: "ready" | "blocked";
};

type CookieResponse = Pick<NextResponse, "cookies">;

const defaultDiagnosisOIDCReturnTo = "/diagnosis-room";
const defaultDiagnosisOIDCScopes = "openid profile email";
const oidcStateSigningKeyMinBytes = 32;
const oidcStateSealVersion = "v1";
const oidcStateGCMNonceBytes = 12;
const oidcStateGCMTagBytes = 16;

export function diagnosisOIDCConfigFromEnv(
  request: Request,
): DiagnosisOIDCConfig | null {
  const issuer = oidcURLConfigValue(
    oidcFirstEnvValue(
      process.env.OPENCLARION_IAM_OIDC_ISSUER,
      process.env.OIDC_ISSUER,
      process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
    ),
    { allowSearch: false },
  );
  const clientID = oidcStringConfigValue(
    oidcFirstEnvValue(
      process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
      process.env.OIDC_CLIENT_ID,
      process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
    ),
  );
  if (issuer === null || clientID === null) {
    return null;
  }
  const redirectURL =
    oidcURLConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL,
        process.env.OIDC_REDIRECT_URL,
      ),
    ) ??
    new URL(
      diagnosisOIDCStateCookiePath,
      diagnosisRequestPublicOrigin(request),
    );
  const clientSecret = oidcStringConfigValue(
    oidcFirstEnvValue(
      process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
      process.env.OIDC_CLIENT_SECRET,
    ),
  );
  const rawClientAuthMethod = oidcFirstEnvValue(
    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    process.env.OIDC_CLIENT_AUTH_METHOD,
  );
  const clientAuthMethod = oidcClientAuthMethodConfigValue(rawClientAuthMethod);
  if ((rawClientAuthMethod ?? "").trim() !== "" && clientAuthMethod === null) {
    return null;
  }
  const scopes =
    oidcStringConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_SCOPES,
        process.env.OIDC_SCOPES,
      ),
    ) ?? defaultDiagnosisOIDCScopes;
  if (!oidcScopesIncludeStandardLogin(scopes)) {
    return null;
  }
  const usePKCE =
    (
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
        process.env.OIDC_USE_PKCE,
      ) ?? ""
    )
      .trim()
      .toLowerCase() !== "false";
  if (
    clientAuthMethod !== null &&
    oidcClientAuthMethodRequiresSecret(clientAuthMethod) &&
    clientSecret === null
  ) {
    return null;
  }
  if (clientAuthMethod === "none" && !usePKCE) {
    return null;
  }
  return {
    clientAuthMethod: clientAuthMethod ?? undefined,
    clientID,
    clientSecret: clientSecret ?? undefined,
    issuer,
    redirectURL,
    scopes,
    usePKCE,
  };
}

export function diagnosisOIDCStateSigningKey(): Buffer | null {
  const raw =
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY ??
    process.env.OIDC_STATE_SIGNING_KEY ??
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY ??
    "";
  const value = raw.trim();
  if (value === "" || value !== raw) {
    return null;
  }
  const key = Buffer.from(value, "utf8");
  return key.length < oidcStateSigningKeyMinBytes ? null : key;
}

export function diagnosisBrowserSessionSigningKeyConfigured(): boolean {
  return oidcSigningKeyConfigValue(
    process.env.OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY,
  );
}

export function diagnosisOIDCBFFReadinessFromEnv(
  request: Request,
  force = false,
): DiagnosisOIDCBFFReadiness | undefined {
  if (!force && !diagnosisOIDCBFFEnvConfigured()) {
    return undefined;
  }
  const issuer =
    oidcURLConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_ISSUER,
        process.env.OIDC_ISSUER,
        process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
      ),
      { allowSearch: false },
    ) !== null;
  const clientID =
    oidcStringConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
        process.env.OIDC_CLIENT_ID,
        process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
      ),
    ) !== null;
  const clientSecret =
    oidcStringConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
        process.env.OIDC_CLIENT_SECRET,
      ),
    ) !== null;
  const redirectURLConfigured =
    oidcURLConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL,
        process.env.OIDC_REDIRECT_URL,
      ),
    ) !== null;
  const stateSigningKey = diagnosisOIDCStateSigningKey() !== null;
  const scopes =
    oidcStringConfigValue(
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_SCOPES,
        process.env.OIDC_SCOPES,
      ),
    ) ?? defaultDiagnosisOIDCScopes;
  const scopeSet = oidcScopeSet(scopes);
  const scopesIncludeOpenID = scopeSet.has("openid");
  const sessionSigningKey = diagnosisBrowserSessionSigningKeyConfigured();
  const rawClientAuthMethod = oidcFirstEnvValue(
    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    process.env.OIDC_CLIENT_AUTH_METHOD,
  );
  const clientAuthMethod = oidcClientAuthMethodConfigValue(rawClientAuthMethod);
  const clientAuthMethodValid =
    rawClientAuthMethod === undefined || clientAuthMethod !== null;
  const pkceEnabled =
    (
      oidcFirstEnvValue(
        process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
        process.env.OIDC_USE_PKCE,
      ) ?? ""
    )
      .trim()
      .toLowerCase() !== "false";
  const missing: DiagnosisOIDCBFFReadiness["missing"] = [];
  if (!issuer) {
    missing.push("issuer");
  }
  if (!clientID) {
    missing.push("client_id");
  }
  if (!stateSigningKey) {
    missing.push("state_signing_key");
  }
  if (!sessionSigningKey) {
    missing.push("session_signing_key");
  }
  if (!clientAuthMethodValid) {
    missing.push("client_auth_method");
  }
  if (
    clientAuthMethod !== null &&
    oidcClientAuthMethodRequiresSecret(clientAuthMethod) &&
    !clientSecret
  ) {
    missing.push("client_secret");
  }
  if (!scopesIncludeOpenID) {
    missing.push("openid_scope");
  }
  if (!scopeSet.has("profile")) {
    missing.push("profile_scope");
  }
  if (!scopeSet.has("email")) {
    missing.push("email_scope");
  }
  if (clientAuthMethod === "none" && !pkceEnabled) {
    missing.push("pkce");
  }
  void request;
  return {
    browser_session_signing_key_configured: sessionSigningKey,
    client_auth_method:
      clientAuthMethod ?? (clientAuthMethodValid ? "auto" : "invalid"),
    client_id_configured: clientID,
    client_secret_configured: clientSecret,
    configured: missing.length === 0,
    issuer_configured: issuer,
    missing,
    pkce_enabled: pkceEnabled,
    redirect_url_configured: redirectURLConfigured,
    scopes_include_openid: scopesIncludeOpenID,
    state_signing_key_configured: stateSigningKey,
    status: missing.length === 0 ? "ready" : "blocked",
  };
}

export function diagnosisOIDCLoginURL({
  config,
  discovery,
  payload,
}: {
  config: DiagnosisOIDCConfig;
  discovery: DiagnosisOIDCDiscovery;
  payload: DiagnosisOIDCStatePayload;
}): URL {
  const authorizationURL = new URL(discovery.authorizationEndpoint);
  authorizationURL.searchParams.set("client_id", config.clientID);
  authorizationURL.searchParams.set(
    "redirect_uri",
    config.redirectURL.toString(),
  );
  authorizationURL.searchParams.set("response_type", "code");
  authorizationURL.searchParams.set("scope", config.scopes);
  authorizationURL.searchParams.set("state", payload.state);
  authorizationURL.searchParams.set("nonce", payload.nonce);
  if (payload.codeVerifier !== undefined) {
    authorizationURL.searchParams.set(
      "code_challenge",
      oidcPKCECodeChallenge(payload.codeVerifier),
    );
    authorizationURL.searchParams.set("code_challenge_method", "S256");
  }
  return authorizationURL;
}

export function diagnosisOIDCDiscoveryURL(issuer: URL): URL {
  const path = `${issuer.pathname.replace(/\/+$/u, "")}/.well-known/openid-configuration`;
  return new URL(path, issuer.origin);
}

export function normalizedDiagnosisOIDCDiscovery(
  raw: unknown,
): DiagnosisOIDCDiscovery | null {
  if (raw === null || typeof raw !== "object") {
    return null;
  }
  const record = raw as Record<string, unknown>;
  if (
    typeof record.issuer !== "string" ||
    typeof record.authorization_endpoint !== "string" ||
    typeof record.token_endpoint !== "string"
  ) {
    return null;
  }
  const authorizationEndpoint = oidcURLConfigValue(
    record.authorization_endpoint,
  );
  const tokenEndpoint = oidcURLConfigValue(record.token_endpoint);
  if (authorizationEndpoint === null || tokenEndpoint === null) {
    return null;
  }
  return {
    authorizationEndpoint,
    issuer: record.issuer,
    tokenEndpoint,
    tokenEndpointAuthMethodsSupported: oidcTokenEndpointAuthMethods(
      record.token_endpoint_auth_methods_supported,
    ),
  };
}

export function diagnosisOIDCIssuerMatches(
  discoveryIssuer: string,
  configuredIssuer: URL,
): boolean {
  const discovery = oidcURLConfigValue(discoveryIssuer, { allowSearch: false });
  if (discovery === null) {
    return false;
  }
  return (
    oidcComparableIssuer(discovery) === oidcComparableIssuer(configuredIssuer)
  );
}

export function newDiagnosisOIDCStatePayload(
  returnTo: string,
  usePKCE: boolean,
): DiagnosisOIDCStatePayload {
  return {
    codeVerifier: usePKCE ? randomOIDCToken(32) : undefined,
    issuedAt: Date.now(),
    nonce: randomOIDCToken(32),
    returnTo,
    state: randomOIDCToken(32),
  };
}

export function sealDiagnosisOIDCStatePayload(
  payload: DiagnosisOIDCStatePayload,
  key: Buffer,
): string {
  const nonce = randomBytes(oidcStateGCMNonceBytes);
  const cipher = createCipheriv(
    "aes-256-gcm",
    oidcStateEncryptionKey(key),
    nonce,
  );
  const ciphertext = Buffer.concat([
    cipher.update(Buffer.from(JSON.stringify(payload), "utf8")),
    cipher.final(),
  ]);
  const tag = cipher.getAuthTag();
  return [
    oidcStateSealVersion,
    base64URLEncode(nonce),
    base64URLEncode(ciphertext),
    base64URLEncode(tag),
  ].join(".");
}

export function unsealDiagnosisOIDCStatePayload(
  sealed: string | null,
  key: Buffer,
): DiagnosisOIDCStatePayload | null {
  const value = sealed?.trim() ?? "";
  if (value === "" || value !== sealed) {
    return null;
  }
  const [version, encodedNonce, encodedCiphertext, encodedTag, ...extra] =
    value.split(".");
  if (
    version !== oidcStateSealVersion ||
    !isBase64URLValue(encodedNonce) ||
    !isBase64URLValue(encodedCiphertext) ||
    !isBase64URLValue(encodedTag) ||
    extra.length > 0
  ) {
    return null;
  }
  const nonce = base64URLDecode(encodedNonce);
  const ciphertext = base64URLDecode(encodedCiphertext);
  const tag = base64URLDecode(encodedTag);
  if (
    nonce.length !== oidcStateGCMNonceBytes ||
    ciphertext.length === 0 ||
    tag.length !== oidcStateGCMTagBytes
  ) {
    return null;
  }
  try {
    const decipher = createDecipheriv(
      "aes-256-gcm",
      oidcStateEncryptionKey(key),
      nonce,
    );
    decipher.setAuthTag(tag);
    const plaintext = Buffer.concat([
      decipher.update(ciphertext),
      decipher.final(),
    ]);
    const payload = JSON.parse(
      plaintext.toString("utf8"),
    ) as Partial<DiagnosisOIDCStatePayload>;
    if (!isDiagnosisOIDCStatePayload(payload)) {
      return null;
    }
    if (
      payload.issuedAt + diagnosisOIDCStateCookieMaxAgeSeconds * 1000 <
      Date.now()
    ) {
      return null;
    }
    return payload;
  } catch {
    return null;
  }
}

export function setDiagnosisOIDCStateCookie(
  response: CookieResponse,
  request: Request | string,
  payload: DiagnosisOIDCStatePayload,
  key: Buffer,
) {
  response.cookies.set({
    name: diagnosisOIDCStateCookieName,
    value: sealDiagnosisOIDCStatePayload(payload, key),
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: diagnosisOIDCStateCookiePath,
    maxAge: diagnosisOIDCStateCookieMaxAgeSeconds,
  });
}

export function expireDiagnosisOIDCStateCookie(
  response: CookieResponse,
  request: Request | string,
) {
  response.cookies.set({
    name: diagnosisOIDCStateCookieName,
    value: "",
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: diagnosisOIDCStateCookiePath,
    expires: new Date(0),
  });
}

export function diagnosisOIDCStateFromRequest(request: Request): string | null {
  for (const part of (request.headers.get("cookie") ?? "").split(";")) {
    const [rawName, ...rawValueParts] = part.split("=");
    if (rawName?.trim() !== diagnosisOIDCStateCookieName) {
      continue;
    }
    try {
      return decodeURIComponent(rawValueParts.join("="));
    } catch {
      return null;
    }
  }
  return null;
}

export function normalizedDiagnosisOIDCReturnTo(
  raw: string | null,
): string | null {
  const value = raw?.trim() ?? defaultDiagnosisOIDCReturnTo;
  if (
    value === "" ||
    value !== (raw ?? defaultDiagnosisOIDCReturnTo) ||
    !value.startsWith("/") ||
    value.startsWith("//")
  ) {
    return null;
  }
  try {
    const parsed = new URL(value, "https://openclarion.local");
    if (
      parsed.origin !== "https://openclarion.local" ||
      parsed.username !== "" ||
      parsed.password !== "" ||
      parsed.hash !== ""
    ) {
      return null;
    }
    return parsed.pathname + parsed.search;
  } catch {
    return null;
  }
}

export function diagnosisOIDCReturnToWithSessionContext(
  returnTo: string,
): string {
  const parsed = new URL(returnTo, "https://openclarion.local");
  parsed.searchParams.set("auth_mode", "session");
  parsed.searchParams.delete("oidc_auth_error");
  return parsed.pathname + parsed.search;
}

export function oidcCallbackQueryToken(
  searchParams: URLSearchParams,
  name: string,
  maxBytes: number,
): string | null {
  const values = searchParams.getAll(name);
  const value = values[0];
  if (values.length !== 1 || value === undefined || value === "") {
    return null;
  }
  if (
    value.trim() !== value ||
    /[\s\x00-\x1f\x7f]/u.test(value) ||
    new TextEncoder().encode(value).length > maxBytes
  ) {
    return null;
  }
  return value;
}

export function oidcIDTokenNonce(idToken: string): string | null {
  const parts = idToken.split(".");
  const payload = parts[1];
  if (parts.length !== 3 || payload === undefined || payload === "") {
    return null;
  }
  try {
    const claims = JSON.parse(
      Buffer.from(base64URLDecode(payload)).toString("utf8"),
    ) as Record<string, unknown>;
    return typeof claims.nonce === "string" ? claims.nonce : null;
  } catch {
    return null;
  }
}

function oidcTokenEndpointBody({
  code,
  config,
  payload,
}: {
  code: string;
  config: DiagnosisOIDCConfig;
  payload: DiagnosisOIDCStatePayload;
}): URLSearchParams {
  const body = new URLSearchParams({
    code,
    grant_type: "authorization_code",
    redirect_uri: config.redirectURL.toString(),
  });
  if (payload.codeVerifier !== undefined) {
    body.set("code_verifier", payload.codeVerifier);
  }
  return body;
}

export function oidcTokenEndpointRequest({
  code,
  config,
  discovery,
  payload,
}: {
  code: string;
  config: DiagnosisOIDCConfig;
  discovery: DiagnosisOIDCDiscovery;
  payload: DiagnosisOIDCStatePayload;
}): { body: URLSearchParams; headers: Headers } | null {
  const body = oidcTokenEndpointBody({ code, config, payload });
  const headers = new Headers({
    accept: "application/json",
    "content-type": "application/x-www-form-urlencoded",
  });
  const authMethod = diagnosisOIDCClientAuthMethod(config, discovery);
  switch (authMethod) {
    case "client_secret_basic":
      if (config.clientSecret === undefined) {
        return null;
      }
      headers.set(
        "authorization",
        `Basic ${Buffer.from(
          `${oidcHTTPBasicCredential(config.clientID)}:${oidcHTTPBasicCredential(
            config.clientSecret,
          )}`,
          "utf8",
        ).toString("base64")}`,
      );
      break;
    case "client_secret_post":
      if (config.clientSecret === undefined) {
        return null;
      }
      body.set("client_id", config.clientID);
      body.set("client_secret", config.clientSecret);
      break;
    case "none":
      body.set("client_id", config.clientID);
      break;
  }
  return { body, headers };
}

export function normalizedOIDCTokenResponse(
  raw: unknown,
): DiagnosisOIDCTokenResponse | null {
  if (raw === null || typeof raw !== "object") {
    return null;
  }
  const record = raw as Record<string, unknown>;
  const idToken = record.id_token;
  if (typeof idToken !== "string" || !isCleanOIDCTokenValue(idToken)) {
    return null;
  }
  const accessToken = record.access_token;
  const tokenType = record.token_type;
  if (accessToken === undefined) {
    return { idToken };
  }
  if (
    typeof accessToken !== "string" ||
    !isCleanOIDCTokenValue(accessToken) ||
    (tokenType !== undefined &&
      (typeof tokenType !== "string" || !/^Bearer$/i.test(tokenType)))
  ) {
    return null;
  }
  return { accessToken, idToken };
}

function diagnosisOIDCClientAuthMethod(
  config: DiagnosisOIDCConfig,
  discovery: DiagnosisOIDCDiscovery,
): DiagnosisOIDCClientAuthMethod {
  if (config.clientAuthMethod !== undefined) {
    return config.clientAuthMethod;
  }
  if (config.clientSecret === undefined) {
    return "none";
  }
  const supported = discovery.tokenEndpointAuthMethodsSupported;
  if (supported.includes("client_secret_basic")) {
    return "client_secret_basic";
  }
  if (supported.includes("client_secret_post")) {
    return "client_secret_post";
  }
  return "client_secret_basic";
}

function oidcClientAuthMethodRequiresSecret(
  method: DiagnosisOIDCClientAuthMethod,
): boolean {
  return method === "client_secret_basic" || method === "client_secret_post";
}

function oidcSigningKeyConfigValue(raw: string | undefined): boolean {
  const value = raw?.trim() ?? "";
  if (value === "" || value !== raw) {
    return false;
  }
  return lenientUTF8ByteLength(value) >= oidcStateSigningKeyMinBytes;
}

function oidcClientAuthMethodConfigValue(
  raw: string | undefined,
): DiagnosisOIDCClientAuthMethod | null {
  const value = oidcStringConfigValue(raw);
  switch (value) {
    case "client_secret_basic":
    case "client_secret_post":
    case "none":
      return value;
    default:
      return null;
  }
}

function diagnosisOIDCBFFEnvConfigured(): boolean {
  return [
    process.env.OPENCLARION_IAM_OIDC_ISSUER,
    process.env.OIDC_ISSUER,
    process.env.OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL,
    process.env.OPENCLARION_IAM_OIDC_CLIENT_ID,
    process.env.OIDC_CLIENT_ID,
    process.env.OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID,
    process.env.OPENCLARION_IAM_OIDC_CLIENT_SECRET,
    process.env.OIDC_CLIENT_SECRET,
    process.env.OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD,
    process.env.OIDC_CLIENT_AUTH_METHOD,
    process.env.OPENCLARION_IAM_OIDC_REDIRECT_URL,
    process.env.OIDC_REDIRECT_URL,
    process.env.OPENCLARION_IAM_OIDC_SCOPES,
    process.env.OIDC_SCOPES,
    process.env.OPENCLARION_IAM_OIDC_USE_PKCE,
    process.env.OIDC_USE_PKCE,
    process.env.OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY,
    process.env.OIDC_STATE_SIGNING_KEY,
  ].some((value) => (value ?? "").trim() !== "");
}

function oidcFirstEnvValue(
  ...values: Array<string | undefined>
): string | undefined {
  return values.find((value) => (value ?? "").trim() !== "");
}

function oidcScopesIncludeStandardLogin(scopes: string): boolean {
  const scopeSet = oidcScopeSet(scopes);
  return (
    scopeSet.has("openid") && scopeSet.has("profile") && scopeSet.has("email")
  );
}

function oidcScopeSet(scopes: string): Set<string> {
  return new Set(
    scopes
      .split(/[\s,]+/u)
      .map((scope) => scope.trim())
      .filter((scope) => scope !== ""),
  );
}

function isCleanOIDCTokenValue(value: string): boolean {
  return (
    value.trim() === value &&
    value !== "" &&
    new TextEncoder().encode(value).length <= 8192 &&
    !/[\s\x00-\x1f\x7f]/u.test(value)
  );
}

function lenientUTF8ByteLength(value: string): number {
  return Buffer.from(value, "utf8").length;
}

function oidcHTTPBasicCredential(value: string): string {
  return new URLSearchParams([["", value]]).toString().slice(1);
}

function oidcStringConfigValue(raw: string | undefined): string | null {
  const value = raw?.trim() ?? "";
  return value === "" || value !== raw ? null : value;
}

function oidcTokenEndpointAuthMethods(
  raw: unknown,
): DiagnosisOIDCClientAuthMethod[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const out: DiagnosisOIDCClientAuthMethod[] = [];
  raw.forEach((value) => {
    if (
      (value === "client_secret_basic" ||
        value === "client_secret_post" ||
        value === "none") &&
      !out.includes(value)
    ) {
      out.push(value);
    }
  });
  return out;
}

function oidcURLConfigValue(
  raw: string | undefined,
  options: { allowSearch?: boolean } = { allowSearch: true },
): URL | null {
  const value = oidcStringConfigValue(raw);
  if (value === null) {
    return null;
  }
  try {
    const parsed = new URL(value);
    const allowSearch = options.allowSearch !== false;
    if (
      (parsed.protocol !== "https:" && parsed.protocol !== "http:") ||
      parsed.username !== "" ||
      parsed.password !== "" ||
      (!allowSearch && parsed.search !== "") ||
      parsed.hash !== ""
    ) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function oidcComparableIssuer(issuer: URL): string {
  return issuer.toString().replace(/\/$/u, "");
}

function isDiagnosisOIDCStatePayload(
  payload: Partial<DiagnosisOIDCStatePayload>,
): payload is DiagnosisOIDCStatePayload {
  return (
    typeof payload.issuedAt === "number" &&
    Number.isFinite(payload.issuedAt) &&
    typeof payload.nonce === "string" &&
    payload.nonce !== "" &&
    typeof payload.returnTo === "string" &&
    normalizedDiagnosisOIDCReturnTo(payload.returnTo) === payload.returnTo &&
    typeof payload.state === "string" &&
    payload.state !== "" &&
    (payload.codeVerifier === undefined ||
      (typeof payload.codeVerifier === "string" && payload.codeVerifier !== ""))
  );
}

function oidcPKCECodeChallenge(codeVerifier: string): string {
  return base64URLEncode(createHash("sha256").update(codeVerifier).digest());
}

function randomOIDCToken(bytes: number): string {
  return base64URLEncode(randomBytes(bytes));
}

function oidcStateEncryptionKey(key: Buffer): Buffer {
  return createHash("sha256")
    .update("openclarion diagnosis oidc state")
    .update(key)
    .digest();
}

function base64URLEncode(input: Buffer): string {
  return input.toString("base64url");
}

function base64URLDecode(input: string): Buffer {
  return Buffer.from(input, "base64url");
}

function isBase64URLValue(value: string | undefined): value is string {
  return typeof value === "string" && /^[A-Za-z0-9_-]+$/u.test(value);
}
