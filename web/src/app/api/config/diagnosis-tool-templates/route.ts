import {
  createDiagnosisToolTemplate,
  fetchDiagnosisToolTemplates
} from "@/features/settings/diagnosis-tool-templates/api";
import type { DiagnosisToolTemplateWriteRequest } from "@/features/settings/diagnosis-tool-templates/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchDiagnosisToolTemplates({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<DiagnosisToolTemplateWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createDiagnosisToolTemplate(body.data, { headers }),
    201,
  );
}
