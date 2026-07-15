import type { useTranslations } from "next-intl";

export type ReportListTranslator = ReturnType<
  typeof useTranslations<"ReportList">
>;

const severityKeys = {
  critical: "severityValue.critical",
  info: "severityValue.info",
  warning: "severityValue.warning",
} as const;

const confidenceKeys = {
  high: "confidenceValue.high",
  low: "confidenceValue.low",
  medium: "confidenceValue.medium",
} as const;

export function localizeReportSeverity(
  value: string,
  t: ReportListTranslator,
): string {
  return localizeKnownValue(value, severityKeys, t);
}

export function localizeReportConfidence(
  value: string,
  t: ReportListTranslator,
): string {
  return localizeKnownValue(value, confidenceKeys, t);
}

function localizeKnownValue<Key extends string>(
  value: string,
  keys: Readonly<Record<string, Key>>,
  t: (key: Key) => string,
): string {
  const key = keys[value.trim().toLowerCase()];
  return key === undefined ? value : t(key);
}
