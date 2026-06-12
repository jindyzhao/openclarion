import type { components } from "@/lib/api/openapi";

export type DiagnosisToolKind = components["schemas"]["DiagnosisToolKind"];
export type DiagnosisToolTemplate = components["schemas"]["DiagnosisToolTemplate"];
export type DiagnosisToolTemplateListResponse = components["schemas"]["DiagnosisToolTemplateListResponse"];
export type DiagnosisToolTemplateWriteRequest = components["schemas"]["DiagnosisToolTemplateWriteRequest"];

export type DiagnosisToolTemplateFormState = {
  name: string;
  alertSourceProfileID: number | null;
  tool: DiagnosisToolKind;
  queryTemplate: string;
  defaultLimit: number | null;
  defaultWindowSeconds: number | null;
  maxWindowSeconds: number | null;
  defaultStepSeconds: number | null;
};
