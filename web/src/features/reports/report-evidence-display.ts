export type ReportEvidenceRequestDisplay = {
  alert_source_profile_id?: number;
  limit?: number;
  query?: string;
  reason: string;
  step_seconds?: number;
  template_id?: number;
  tool: string;
  window_seconds?: number;
};

export type ReportEvidenceCollectionResultDisplay = {
  alert_source_profile_id?: number;
  limit?: number;
  query?: string;
  request_reason?: string;
  step_seconds?: number;
  status: string;
  template_id?: number;
  tool: string;
  window_seconds?: number;
};

export function reportEvidenceRequestDetail(
  request: ReportEvidenceRequestDisplay,
) {
  const parts = [
    request.query ? `query: ${request.query}` : undefined,
    request.template_id !== undefined ? `template #${request.template_id}` : undefined,
    request.alert_source_profile_id !== undefined
      ? `source #${request.alert_source_profile_id}`
      : undefined,
    request.window_seconds !== undefined ? `window ${request.window_seconds}s` : undefined,
    request.step_seconds !== undefined ? `step ${request.step_seconds}s` : undefined,
    request.limit !== undefined ? `limit ${request.limit}` : undefined,
  ].filter((part): part is string => part !== undefined);
  return parts.join(" / ");
}

export function reportEvidenceRequestKey(
  request: ReportEvidenceRequestDisplay,
) {
  return reportEvidenceRequestIdentity(request);
}

function reportEvidenceRequestIdentity(
  request: ReportEvidenceRequestDisplay,
) {
  return [
    request.tool,
    request.reason,
    request.query ?? "",
    request.template_id ?? "",
    request.alert_source_profile_id ?? "",
    request.window_seconds ?? "",
    request.step_seconds ?? "",
    request.limit ?? "",
  ].join(":");
}

export function reportEvidenceCollectionResultForRequest(
  request: ReportEvidenceRequestDisplay,
  results: ReportEvidenceCollectionResultDisplay[],
) {
  return results.find((result) =>
    reportEvidenceCollectionResultMatchesRequest(request, result),
  );
}

function reportEvidenceCollectionResultMatchesRequest(
  request: ReportEvidenceRequestDisplay,
  result: ReportEvidenceCollectionResultDisplay,
) {
  return (
    result.tool === request.tool &&
    result.request_reason === request.reason &&
    optionalResultFieldMatches(request.query, result.query) &&
    optionalResultFieldMatches(request.template_id, result.template_id) &&
    optionalResultFieldMatches(
      request.alert_source_profile_id,
      result.alert_source_profile_id,
    ) &&
    optionalResultFieldMatches(request.window_seconds, result.window_seconds) &&
    optionalResultFieldMatches(request.step_seconds, result.step_seconds) &&
    optionalResultFieldMatches(request.limit, result.limit)
  );
}

function optionalResultFieldMatches<T>(requestValue: T | undefined, resultValue: T | undefined) {
  return requestValue === undefined || resultValue === requestValue;
}
