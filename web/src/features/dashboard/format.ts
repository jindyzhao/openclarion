export function formatSuccessRate(
  value: number | null,
  locale: string,
): string | null {
  if (value === null) {
    return null;
  }
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 0,
    style: "percent",
  }).format(value);
}

export function formatCount(value: number, locale = "en-US"): string {
  return new Intl.NumberFormat(locale).format(value);
}
