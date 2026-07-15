import type { DiagnosisToolTemplate } from "@/features/settings/diagnosis-tool-templates/types";

type OperatorEvidenceAlertContext = {
  alert: {
    annotations?: Record<string, string> | null;
    labels?: Record<string, string> | null;
  };
};

export type OperatorEvidenceTemplateQueryResult = {
  missing: string[];
  query?: string;
};

export type OperatorEvidenceRangeField = "step" | "window";

export function operatorEvidenceRangeValues(
  field: OperatorEvidenceRangeField,
  current: number,
  peer: number,
): { stepSeconds: number; windowSeconds: number } {
  return field === "window"
    ? { stepSeconds: peer, windowSeconds: current }
    : { stepSeconds: current, windowSeconds: peer };
}

const maxPlaceholderValueBytes = 200;

const operatorEvidenceTemplatePlaceholderPattern =
  /\{\{\s*(label|annotation)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}/g;

export function operatorEvidenceTemplateHasParameterizedQuery(
  template: DiagnosisToolTemplate,
): boolean {
  return (
    template.tool !== "active_alerts" &&
    (template.query_template.includes("{{") ||
      template.query_template.includes("}}"))
  );
}

export function operatorEvidenceTemplateQuery(
  template: DiagnosisToolTemplate,
  alertContext: OperatorEvidenceAlertContext | undefined,
): OperatorEvidenceTemplateQueryResult {
  if (template.tool === "active_alerts") {
    return { missing: [] };
  }
  if (!operatorEvidenceTemplateHasParameterizedQuery(template)) {
    return { missing: [], query: template.query_template };
  }
  const missing: string[] = [];
  const query = template.query_template.replace(
    operatorEvidenceTemplatePlaceholderPattern,
    (_match, scope: "label" | "annotation", key: string) => {
      const values =
        scope === "label"
          ? alertContext?.alert.labels
          : alertContext?.alert.annotations;
      const value = values?.[key];
      if (value === undefined) {
        missing.push(`${scope}.${key}`);
        return "";
      }
      if (!safeOperatorEvidencePlaceholderValue(value)) {
        missing.push(`${scope}.${key} (unsafe)`);
        return "";
      }
      return value;
    },
  );
  return { missing: uniqueOperatorEvidenceStrings(missing), query };
}

export function operatorEvidenceTemplateSourceDisabledReason(
  template: Pick<DiagnosisToolTemplate, "alert_source_profile_id">,
  alertSourceProfileID: number,
): string {
  if (alertSourceProfileID <= 0) {
    return "";
  }
  if (template.alert_source_profile_id === alertSourceProfileID) {
    return "";
  }
  return `Template source #${template.alert_source_profile_id} is outside the current alert source #${alertSourceProfileID}.`;
}

export function safeOperatorEvidencePlaceholderValue(value: string): boolean {
  if (new TextEncoder().encode(value).length > maxPlaceholderValueBytes) {
    return false;
  }
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code <= 0x1f || code === 0x7f) {
      return false;
    }
  }
  return !value.includes("\"") && !value.includes("\\");
}

function uniqueOperatorEvidenceStrings(values: string[]): string[] {
  return Array.from(new Set(values));
}
