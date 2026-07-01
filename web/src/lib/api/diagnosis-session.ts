import { normalizeForwardedAuthorization } from "./authorization";

const diagnosisSessionCookieName = "openclarion_diagnosis_session";

type CookieResponse = {
  cookies: {
    set: (options: {
      name: string;
      value: string;
      httpOnly: boolean;
      sameSite: "lax";
      secure: boolean;
      path: string;
      expires?: Date;
    }) => void;
  };
};

export function diagnosisAuthorizationFromRequest(request: Request): string | null {
  const explicit = normalizeForwardedAuthorization(request.headers.get("authorization") ?? "");
  if (explicit !== null) {
    return explicit;
  }
  const sessionToken = diagnosisSessionTokenFromCookieHeader(request.headers.get("cookie") ?? "");
  return sessionToken === null ? null : `Bearer ${sessionToken}`;
}

export function diagnosisRequestHasSessionCookie(request: Request): boolean {
  return cookieValueFromHeader(request.headers.get("cookie") ?? "", diagnosisSessionCookieName) !== null;
}

function diagnosisRequestUsesSessionCookieAuthorization(request: Request): boolean {
  return (request.headers.get("authorization") ?? "").trim() === "" && diagnosisRequestHasSessionCookie(request);
}

export function diagnosisRequestPublicOrigin(request: Request): string {
  const requestURL = new URL(request.url);
  if (forwardedProtoIncludesHTTPS(request.headers.get("x-forwarded-proto"))) {
    return `https://${requestURL.host}`;
  }
  return requestURL.origin;
}

export function expireDiagnosisSessionCookie(response: CookieResponse, request: Request | string) {
  response.cookies.set({
    name: diagnosisSessionCookieName,
    value: "",
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: "/",
    expires: new Date(0)
  });
}

export function expireDiagnosisSessionCookieOnAuthFailure(response: CookieResponse, request: Request, status: number | undefined) {
  if (diagnosisRequestUsesSessionCookieAuthorization(request) && (status === 401 || status === 403)) {
    expireDiagnosisSessionCookie(response, request);
  }
}

export function setDiagnosisSessionCookie(response: CookieResponse, request: Request | string, token: string, expires: Date) {
  response.cookies.set({
    name: diagnosisSessionCookieName,
    value: token,
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: "/",
    expires
  });
}

export function diagnosisSessionCookieSecure(request: Request | string): boolean {
  const url = typeof request === "string" ? new URL(request) : new URL(diagnosisRequestPublicOrigin(request));
  return url.protocol === "https:";
}

export function normalizedDiagnosisSessionToken(raw: string): string | null {
  const token = raw.trim();
  if (token === "" || token !== raw || !/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(token)) {
    return null;
  }
  return token;
}

function diagnosisSessionTokenFromCookieHeader(header: string): string | null {
  const value = cookieValueFromHeader(header, diagnosisSessionCookieName);
  return value === null ? null : normalizedDiagnosisSessionToken(value);
}

function cookieValueFromHeader(header: string, cookieName: string): string | null {
  for (const part of header.split(";")) {
    const [rawName, ...rawValueParts] = part.split("=");
    const name = rawName?.trim();
    if (name !== cookieName) {
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

function forwardedProtoIncludesHTTPS(raw: string | null): boolean {
  return (raw ?? "")
    .split(",")
    .map((value) => value.trim().toLowerCase())
    .includes("https");
}
