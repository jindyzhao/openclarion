import { cookies, headers } from "next/headers";
import { getRequestConfig } from "next-intl/server";

import {
  appLocaleCookieName,
  appLocaleFromAcceptLanguage,
  appTimeZone,
  isAppLocale,
} from "./config";

export default getRequestConfig(async () => {
  const cookieStore = await cookies();
  const requestedLocale = cookieStore.get(appLocaleCookieName)?.value;
  const locale = isAppLocale(requestedLocale)
    ? requestedLocale
    : appLocaleFromAcceptLanguage((await headers()).get("accept-language"));
  const messages =
    locale === "zh-CN"
      ? (await import("../../messages/zh-CN.json")).default
      : (await import("../../messages/en.json")).default;

  return { locale, messages, timeZone: appTimeZone };
});
