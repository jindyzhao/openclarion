import {
  createReportWorkflowPolicy,
  fetchReportWorkflowPolicies
} from "@/features/settings/report-workflow-policies/api";
import type { ReportWorkflowPolicyWriteRequest } from "@/features/settings/report-workflow-policies/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchReportWorkflowPolicies({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<ReportWorkflowPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createReportWorkflowPolicy(body.data, { headers }),
    201,
  );
}
