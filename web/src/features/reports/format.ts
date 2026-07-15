import type { FinalReportSummary } from "./types";

export function formatDateTime(value: string, locale = "en"): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC"
  }).format(date);
}

export function severityClass(severity: FinalReportSummary["severity"]): string {
  switch (severity) {
    case "critical":
      return "pill pill-critical";
    case "warning":
      return "pill pill-warning";
    case "info":
      return "pill pill-info";
    default:
      return "pill pill-ok";
  }
}
