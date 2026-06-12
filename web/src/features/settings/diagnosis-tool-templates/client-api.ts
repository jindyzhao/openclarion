"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  DiagnosisToolTemplate,
  DiagnosisToolTemplateListResponse,
  DiagnosisToolTemplateWriteRequest
} from "./types";

export async function refreshDiagnosisToolTemplates(): Promise<ApiResult<DiagnosisToolTemplateListResponse>> {
  return requestSameOriginJSON<DiagnosisToolTemplateListResponse>("/api/config/diagnosis-tool-templates");
}

export async function submitDiagnosisToolTemplate(
  templateID: number | null,
  body: DiagnosisToolTemplateWriteRequest
): Promise<ApiResult<DiagnosisToolTemplate>> {
  if (templateID === null) {
    return requestSameOriginJSON<DiagnosisToolTemplate>("/api/config/diagnosis-tool-templates", {
      method: "POST",
      body
    });
  }
  return requestSameOriginJSON<DiagnosisToolTemplate>(`/api/config/diagnosis-tool-templates/${templateID}`, {
    method: "PUT",
    body
  });
}

export async function enableDiagnosisToolTemplateAction(templateID: number): Promise<ApiResult<DiagnosisToolTemplate>> {
  return requestSameOriginJSON<DiagnosisToolTemplate>(`/api/config/diagnosis-tool-templates/${templateID}/enable`, {
    method: "POST"
  });
}

export async function disableDiagnosisToolTemplateAction(templateID: number): Promise<ApiResult<DiagnosisToolTemplate>> {
  return requestSameOriginJSON<DiagnosisToolTemplate>(`/api/config/diagnosis-tool-templates/${templateID}/disable`, {
    method: "POST"
  });
}
