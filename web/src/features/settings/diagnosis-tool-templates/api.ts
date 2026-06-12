import { requestJSON, type ApiResult } from "@/lib/api/client";

import type {
  DiagnosisToolTemplate,
  DiagnosisToolTemplateListResponse,
  DiagnosisToolTemplateWriteRequest
} from "./types";

export async function fetchDiagnosisToolTemplates(): Promise<ApiResult<DiagnosisToolTemplateListResponse>> {
  return requestJSON<DiagnosisToolTemplateListResponse>("/api/v1/config/diagnosis-tool-templates?limit=100");
}

export async function createDiagnosisToolTemplate(
  body: DiagnosisToolTemplateWriteRequest
): Promise<ApiResult<DiagnosisToolTemplate>> {
  return requestJSON<DiagnosisToolTemplate>("/api/v1/config/diagnosis-tool-templates", {
    method: "POST",
    body
  });
}

export async function replaceDiagnosisToolTemplate(
  templateID: number,
  body: DiagnosisToolTemplateWriteRequest
): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}`, {
    method: "PUT",
    body
  });
}

export async function enableDiagnosisToolTemplate(templateID: number): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}/enable`, {
    method: "POST"
  });
}

export async function disableDiagnosisToolTemplate(templateID: number): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (!positiveTemplateID(templateID)) {
    return { ok: false, error: { message: "Diagnosis tool template ID must be a positive integer.", status: 400 } };
  }
  return requestJSON<DiagnosisToolTemplate>(`/api/v1/config/diagnosis-tool-templates/${templateID}/disable`, {
    method: "POST"
  });
}

function positiveTemplateID(templateID: number): boolean {
  return Number.isSafeInteger(templateID) && templateID > 0;
}
