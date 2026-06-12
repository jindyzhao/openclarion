import {
  createDiagnosisToolTemplate,
  fetchDiagnosisToolTemplates
} from "@/features/settings/diagnosis-tool-templates/api";
import type { DiagnosisToolTemplateWriteRequest } from "@/features/settings/diagnosis-tool-templates/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchDiagnosisToolTemplates());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<DiagnosisToolTemplateWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createDiagnosisToolTemplate(body.data), 201);
}
