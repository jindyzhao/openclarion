import type { DiagnosisEvidenceRequest } from "./types";

export const diagnosisEvidencePlanSearchParamNames = [
  "evidence_tool",
  "evidence_reason",
  "evidence_query",
  "evidence_template_id",
  "evidence_source_profile_id",
  "evidence_window_seconds",
  "evidence_step_seconds",
  "evidence_limit",
] as const;

type SearchParamsReader = {
  get(name: string): string | null;
};

export function diagnosisEvidencePlanURLQuery(
  request: DiagnosisEvidenceRequest,
): Record<string, string> {
  const query: Record<string, string> = {
    evidence_reason: request.reason,
    evidence_tool: request.tool,
  };
  setOptionalStringQuery(query, "evidence_query", request.query);
  setOptionalNumberQuery(query, "evidence_template_id", request.template_id);
  setOptionalNumberQuery(
    query,
    "evidence_source_profile_id",
    request.alert_source_profile_id,
  );
  setOptionalNumberQuery(query, "evidence_window_seconds", request.window_seconds);
  setOptionalNumberQuery(query, "evidence_step_seconds", request.step_seconds);
  setOptionalNumberQuery(query, "evidence_limit", request.limit);
  return query;
}

export function diagnosisEvidencePlanSearchParam(
  searchParams: SearchParamsReader,
): DiagnosisEvidenceRequest | undefined {
  const tool = boundedTextSearchParam(searchParams, "evidence_tool", 40);
  const reason = boundedTextSearchParam(searchParams, "evidence_reason", 500);
  if (!isEvidencePlanTool(tool) || reason === undefined) {
    return undefined;
  }

  const query = boundedTextSearchParam(searchParams, "evidence_query", 500);
  const templateID = positiveIntegerSearchParam(
    searchParams,
    "evidence_template_id",
  );
  const alertSourceProfileID = positiveIntegerSearchParam(
    searchParams,
    "evidence_source_profile_id",
  );
  const windowSeconds = positiveIntegerSearchParam(
    searchParams,
    "evidence_window_seconds",
  );
  const stepSeconds = positiveIntegerSearchParam(
    searchParams,
    "evidence_step_seconds",
  );
  const limit = positiveIntegerSearchParam(searchParams, "evidence_limit");

  if (tool === "active_alerts" && query !== undefined) {
    return undefined;
  }
  if (tool !== "active_alerts" && query === undefined && templateID === undefined) {
    return undefined;
  }

  return {
    ...(templateID !== undefined ? { template_id: templateID } : {}),
    ...(alertSourceProfileID !== undefined
      ? { alert_source_profile_id: alertSourceProfileID }
      : {}),
    ...(query !== undefined ? { query } : {}),
    ...(windowSeconds !== undefined ? { window_seconds: windowSeconds } : {}),
    ...(stepSeconds !== undefined ? { step_seconds: stepSeconds } : {}),
    ...(limit !== undefined ? { limit } : {}),
    reason,
    tool,
  };
}

export function diagnosisEvidencePlanIdentity(
  request: DiagnosisEvidenceRequest,
): string {
  return [
    request.tool,
    request.reason,
    request.query ?? "no-query",
    request.template_id ?? "no-template",
    request.alert_source_profile_id ?? "no-profile",
    request.window_seconds ?? "no-window",
    request.step_seconds ?? "no-step",
    request.limit ?? "no-limit",
  ].join(":");
}

function setOptionalStringQuery(
  query: Record<string, string>,
  name: string,
  value: string | undefined,
) {
  if (value !== undefined && value.trim() !== "") {
    query[name] = value;
  }
}

function setOptionalNumberQuery(
  query: Record<string, string>,
  name: string,
  value: number | undefined,
) {
  if (value !== undefined && Number.isSafeInteger(value) && value > 0) {
    query[name] = String(value);
  }
}

function boundedTextSearchParam(
  searchParams: SearchParamsReader,
  name: string,
  maxLength: number,
): string | undefined {
  const raw = searchParams.get(name);
  if (raw === null) {
    return undefined;
  }
  const value = raw.trim();
  if (
    value === "" ||
    value.length > maxLength ||
    /[\u0000-\u001f\u007f]/u.test(value)
  ) {
    return undefined;
  }
  return value;
}

function positiveIntegerSearchParam(
  searchParams: SearchParamsReader,
  name: string,
): number | undefined {
  const raw = searchParams.get(name);
  if (raw === null || !/^[1-9]\d*$/u.test(raw)) {
    return undefined;
  }
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}

function isEvidencePlanTool(value: unknown): value is DiagnosisEvidenceRequest["tool"] {
  return (
    value === "active_alerts" ||
    value === "metric_query" ||
    value === "metric_range_query"
  );
}
