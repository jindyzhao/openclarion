import { replaceDiagnosisToolTemplate } from "@/features/settings/diagnosis-tool-templates/api";
import type { DiagnosisToolTemplateWriteRequest } from "@/features/settings/diagnosis-tool-templates/types";
import { apiResultResponse, parsePositiveIntegerRouteParam, readRequestJSON } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ templateId: string }>;
};

export const dynamic = "force-dynamic";

export async function PUT(request: Request, context: RouteContext) {
  const { templateId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(templateId, "Diagnosis tool template ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  const body = await readRequestJSON<DiagnosisToolTemplateWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await replaceDiagnosisToolTemplate(parsedID.data, body.data));
}
