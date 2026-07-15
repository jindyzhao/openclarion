export function formatSuccessRate(value: number | null): string {
  if (value === null) {
    return "n/a";
  }
  return `${Math.round(value * 100)}%`;
}

export function formatCount(value: number, locale = "en-US"): string {
  return new Intl.NumberFormat(locale).format(value);
}
