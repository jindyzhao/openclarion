import { NextResponse } from "next/server";

import {
  appLocaleCookieName,
  isAppLocale,
  type AppLocale,
} from "@/i18n/config";
import { diagnosisSessionCookieSecure } from "@/lib/api/diagnosis-session";

const localeCookieMaxAgeSeconds = 365 * 24 * 60 * 60;

export async function PUT(request: Request): Promise<NextResponse> {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Request body must be valid JSON." },
      { status: 400 },
    );
  }

  const locale = localeFromBody(body);
  if (locale === undefined) {
    return NextResponse.json(
      { error: "Locale must be en or zh-CN." },
      { status: 400 },
    );
  }

  const response = new NextResponse(null, { status: 204 });
  response.cookies.set({
    name: appLocaleCookieName,
    value: locale,
    httpOnly: true,
    maxAge: localeCookieMaxAgeSeconds,
    path: "/",
    sameSite: "lax",
    secure: diagnosisSessionCookieSecure(request),
  });
  return response;
}

function localeFromBody(body: unknown): AppLocale | undefined {
  if (
    typeof body !== "object" ||
    body === null ||
    !("locale" in body) ||
    typeof body.locale !== "string" ||
    !isAppLocale(body.locale)
  ) {
    return undefined;
  }
  return body.locale;
}
