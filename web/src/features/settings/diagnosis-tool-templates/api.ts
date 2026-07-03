import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";

import type {
  DiagnosisToolTemplate,
  DiagnosisToolTemplateListResponse,
  DiagnosisToolTemplateWriteRequest
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchDiagnosisToolTemplates(
  options: BackendRequestOptions = {}
): Promise<ApiResult<DiagnosisToolTemplateListResponse>> {
  return requestJSON<DiagnosisToolTemplateListResponse>("/api/v1/config/diagnosis-tool-templates?limit=100", {
    headers: options.headers
  });
}

export async function createDiagnosisToolTemplate(
  body: DiagnosisToolTemplateWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<DiagnosisToolTemplate>> {
  return requestJSON<DiagnosisToolTemplate>("/api/v1/config/diagnosis-tool-templates", {
    method: "POST",
    body,
    headers: options.headers
  });
}

export async function replaceDiagnosisToolTemplate(
  templateID: number,
  body: DiagnosisToolTemplateWriteRequest,
  options: BackendRequestOptions = {}
): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}`, {
    method: "PUT",
    body,
    headers: options.headers
  });
}

export async function enableDiagnosisToolTemplate(
  templateID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}/enable`, {
    method: "POST",
    headers: options.headers
  });
}

export async function disableDiagnosisToolTemplate(
  templateID: number,
  options: BackendRequestOptions = {}
): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}/disable`, {
    method: "POST",
    headers: options.headers
  });
}

function positiveTemplateID(templateID: number): boolean {
  return Number.isSafeInteger(templateID) && templateID > 0;
}
