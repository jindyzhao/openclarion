import { normalizeForwardedAuthorization } from "./authorization";

export const diagnosisSessionCookieName = "openclarion_diagnosis_session";

type DiagnosisCookieResponse = {
  cookies: {
    set: (options: {
      name: string;
      value: string;
      httpOnly: boolean;
      sameSite: "lax";
      secure: boolean;
      path: string;
      expires?: Date;
      maxAge?: number;
    }) => void;
  };
};

type DiagnosisAuthorizationHeaders = {
  get(name: string): string | null;
};

export function diagnosisAuthorizationFromRequest(
  request: Request,
): string | null {
  return diagnosisAuthorizationFromHeaders(request.headers);
}

export function diagnosisAuthorizationFromHeaders(
  headers: DiagnosisAuthorizationHeaders,
): string | null {
  const rawAuthorization = headers.get("authorization");
  if (rawAuthorization !== null && rawAuthorization.trim() !== "") {
    return normalizeForwardedAuthorization(rawAuthorization);
  }
  const sessionToken = diagnosisSessionTokenFromCookieHeader(
    headers.get("cookie") ?? "",
  );
  return sessionToken === null ? null : `Bearer ${sessionToken}`;
}

export function diagnosisSessionCookieSecure(
  request: Request | string,
): boolean {
  if (typeof request !== "string") {
    return diagnosisRequestPublicOrigin(request).startsWith("https://");
  }
  return new URL(request).protocol === "https:";
}

export function diagnosisRequestPublicOrigin(request: Request): string {
  const requestURL = new URL(request.url);
  if (forwardedProtoIncludesHTTPS(request.headers.get("x-forwarded-proto"))) {
    return `https://${requestURL.host}`;
  }
  return requestURL.origin;
}

export function diagnosisRequestHasSessionCookie(request: Request): boolean {
  return (
    cookieValueFromHeader(
      request.headers.get("cookie") ?? "",
      diagnosisSessionCookieName,
    ) !== null
  );
}

export function diagnosisRequestUsesSessionCookieAuthorization(
  request: Request,
): boolean {
  return (
    (request.headers.get("authorization") ?? "").trim() === "" &&
    diagnosisRequestHasSessionCookie(request)
  );
}

export function expireDiagnosisSessionCookieOnAuthFailure(
  response: DiagnosisCookieResponse,
  request: Request,
  status: number | undefined,
) {
  if (
    diagnosisRequestUsesSessionCookieAuthorization(request) &&
    (status === 401 || status === 403)
  ) {
    expireDiagnosisSessionCookie(response, request);
  }
}

export function expireDiagnosisSessionCookie(
  response: DiagnosisCookieResponse,
  request: Request | string,
) {
  response.cookies.set({
    name: diagnosisSessionCookieName,
    value: "",
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: "/",
    expires: new Date(0),
  });
}

export function setDiagnosisSessionCookie(
  response: DiagnosisCookieResponse,
  request: Request | string,
  token: string,
  expires: Date,
) {
  response.cookies.set({
    name: diagnosisSessionCookieName,
    value: token,
    httpOnly: true,
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
    path: "/",
    expires,
  });
}

export function normalizedDiagnosisSessionToken(raw: string): string | null {
  const token = raw.trim();
  if (
    token === "" ||
    token !== raw ||
    !/^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(token)
  ) {
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
    const rawValue = rawValueParts.join("=");
    try {
      return decodeURIComponent(rawValue);
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
