const appLocales = ["en", "zh-CN"] as const;

export type AppLocale = (typeof appLocales)[number];

export const appLocaleCookieName = "openclarion_locale";
export const appTimeZone = "UTC";
export const defaultAppLocale: AppLocale = "en";

export function isAppLocale(value: string | undefined): value is AppLocale {
  return appLocales.some((locale) => locale === value);
}

export function appLocaleFromAcceptLanguage(
  acceptLanguage: string | null,
): AppLocale {
  if (acceptLanguage === null) {
    return defaultAppLocale;
  }
  const preferences = acceptLanguage
    .split(",")
    .map((entry) => {
      const [rawLanguage = "", ...parameters] = entry.trim().split(";");
      const qualityParameter = parameters.find((parameter) =>
        parameter.trim().toLowerCase().startsWith("q="),
      );
      const quality = qualityParameter
        ? Number(qualityParameter.trim().slice(2))
        : 1;
      return {
        language: rawLanguage.trim().toLowerCase(),
        quality:
          Number.isFinite(quality) && quality >= 0 && quality <= 1
            ? quality
            : 0,
      };
    })
    .filter((preference) => preference.quality > 0)
    .sort((left, right) => right.quality - left.quality);

  for (const preference of preferences) {
    if (preference.language === "zh" || preference.language.startsWith("zh-")) {
      return "zh-CN";
    }
    if (preference.language === "en" || preference.language.startsWith("en-")) {
      return "en";
    }
  }
  return defaultAppLocale;
}
