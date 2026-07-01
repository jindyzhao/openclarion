import { enableDiagnosisToolTemplate } from "@/features/settings/diagnosis-tool-templates/api";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, parsePositiveIntegerRouteParam } from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ templateId: string }>;
};

export const dynamic = "force-dynamic";

export async function POST(request: Request, context: RouteContext) {
  const { templateId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(templateId, "Diagnosis tool template ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }

  return authorizedBackendResultResponse(request, (headers) =>
    enableDiagnosisToolTemplate(parsedID.data, { headers }),
  );
}
